package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/auth"
	"geoguessme/internal/config"
	"geoguessme/internal/database"
	"geoguessme/internal/email"
	"geoguessme/internal/middleware"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
)

func main() {
	cfg := config.Load()
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}
	if err := database.ConnectWithLimits(cfg.DatabaseURL, cfg.DatabaseMinConns, cfg.DatabaseMaxConns); err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	auth.InitWithSettings(cfg.JWTSecret, "geoguessme", "geoguessme-web", cfg.AccessTokenTTL)
	switch command {
	case "migrate":
		if len(os.Args) < 3 || os.Args[2] == "up" {
			if err := database.MigrateUp(ctx, logger); err != nil {
				logger.Error("migration failed", "error", err)
				os.Exit(1)
			}
			return
		}
		if os.Args[2] == "status" {
			statuses, err := database.MigrationStatus(ctx)
			if err != nil {
				logger.Error("migration status failed", "error", err)
				os.Exit(1)
			}
			for _, status := range statuses {
				fmt.Printf("%03d %-30s applied %s\n", status.Version, status.Name, status.AppliedAt.Format(time.RFC3339))
			}
			return
		}
		fmt.Fprintln(os.Stderr, "usage: geoguessme migrate [up|status]")
		os.Exit(2)
	case "serve":
		// Schema changes are intentionally not run here. Deployments execute the
		// migration job before starting the API process.
	case "healthcheck":
		if err := database.DB.Ping(ctx); err != nil {
			logger.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	default:
		fmt.Fprintln(os.Stderr, "usage: geoguessme [migrate up|migrate status|serve]")
		os.Exit(2)
	}
	store, err := buildStore(cfg)
	if err != nil {
		logger.Error("storage initialization failed", "error", err)
		os.Exit(1)
	}
	if s3, ok := store.(*storage.S3Store); ok {
		if err := s3.EnsureBucket(ctx, cfg.S3Region); err != nil {
			logger.Error("storage bucket unavailable", "error", err)
			os.Exit(1)
		}
	}
	handlers.Configure(cfg, store, email.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, TLSMode: cfg.SMTPTLS, DialTimeout: cfg.SMTPDialTimeout, Timeout: cfg.SMTPTimeout})
	handlers.InitChat()
	metrics := &middleware.Metrics{}
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go (repository.CleanupRunner{Store: store, Interval: time.Hour, Logger: logger, Backlog: metrics.SetCleanupBacklog}).Run(workerCtx)

	mux := http.NewServeMux()
	authLimit := middleware.RateLimitByIdentity(cfg.RateLimitRequests, cfg.RateLimitWindow, cfg.TrustedProxyCIDRs)
	protected := func(handler http.HandlerFunc) http.Handler { return http.HandlerFunc(handlers.AuthMiddleware(handler)) }

	mux.Handle("/api/v1/auth/signup", authLimit(http.HandlerFunc(handlers.Signup)))
	mux.Handle("/api/v1/auth/login", authLimit(http.HandlerFunc(handlers.Login)))
	mux.Handle("/api/v1/auth/refresh", http.HandlerFunc(handlers.Refresh))
	mux.Handle("/api/v1/auth/logout", http.HandlerFunc(handlers.Logout))
	mux.Handle("/api/v1/auth/verify/request", authLimit(protected(handlers.RequestVerification)))
	mux.Handle("/api/v1/auth/verify", authLimit(http.HandlerFunc(handlers.VerifyEmail)))
	mux.Handle("/api/v1/auth/password/forgot", authLimit(http.HandlerFunc(handlers.ForgotPassword)))
	mux.Handle("/api/v1/auth/password/reset", authLimit(http.HandlerFunc(handlers.ResetPassword)))
	mux.Handle("/api/v1/auth/account", protected(handlers.DeleteAccount))

	mux.Handle("/api/v1/user/groups", protected(handlers.GetUserGroups))
	mux.Handle("/api/v1/group/create", protected(handlers.CreateGroup))
	mux.Handle("/api/v1/group/join", protected(handlers.JoinGroup))
	mux.Handle("/api/v1/group/details", protected(handlers.GetGroupDetails))
	mux.Handle("/api/v1/group/members", protected(handlers.GetGroupMembers))
	mux.Handle("/api/v1/group/leaderboard", protected(handlers.GetLeaderboard))
	mux.Handle("/api/v1/group/messages", protected(handlers.GetGroupMessages))
	mux.Handle("/api/v1/photo/upload", protected(handlers.UploadPhoto))
	mux.Handle("/api/v1/ws/ticket", protected(handlers.CreateWebSocketTicket))
	mux.HandleFunc("/api/v1/ws", handlers.HandleChat)
	mux.Handle("/api/v1/challenges/{photoID}/accept", protected(handlers.AcceptChallenge))
	mux.Handle("/api/v1/challenges/{photoID}/guess", protected(handlers.SubmitChallengeGuess))
	mux.Handle("/api/v1/challenges/{photoID}/results", protected(handlers.GetChallengeResults))
	mux.Handle("/api/v1/challenges/{photoID}/media", protected(handlers.ServeChallengeMedia))

	registerSystemRoutes(mux, cfg, metrics, store)

	var handler http.Handler = mux
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.CORS(cfg.AllowedOrigins)(handler)
	handler = middleware.RequestLog(logger, metrics, handler)
	handler = middleware.Recover(logger, handler)
	handler = middleware.RequestID(handler)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		logger.Info("server listening", "port", cfg.Port, "environment", cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "error", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
	if handlers.HubInstance != nil {
		handlers.HubInstance.Stop()
	}
}

func buildStore(cfg *config.Config) (storage.ObjectStore, error) {
	if strings.EqualFold(os.Getenv("STORAGE_DRIVER"), "local") {
		return storage.NewLocalStore(cfg.UploadDir)
	}
	return storage.NewS3Store(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UsePathStyle)
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

var _ = repository.CleanupAuthTokens
