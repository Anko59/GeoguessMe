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
	"geoguessme/internal/push"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
)

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	// `vapid-keys` only prints a fresh Web Push keypair; it must not require
	// any database, storage, or configuration validation.
	if command == "vapid-keys" {
		printVapidKeys()
		return
	}
	cfg, err := config.LoadValidated()
	if err != nil {
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
	mailer := email.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, TLSMode: cfg.SMTPTLS, DialTimeout: cfg.SMTPDialTimeout, Timeout: cfg.SMTPTimeout}
	handlers.Configure(cfg, store, mailer)
	hub := handlers.NewChatHub()
	handlers.HubInstance = hub
	go hub.Run()
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	pushSvc := configurePush(cfg, logger)
	pushSvc.Start(workerCtx, 2)
	handlers.Push = pushSvc
	app := NewApp(cfg, database.DB, repository.NewRepository(database.DB), store, mailer, pushSvc, hub, logger, time.Now)
	go (repository.CleanupRunner{Store: store, Interval: time.Hour, Logger: app.Logger, Backlog: app.Metrics.SetCleanupBacklog}).Run(workerCtx)

	srv := &http.Server{Addr: ":" + app.Config.Port, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		logger.Info("server listening", "port", app.Config.Port, "environment", app.Config.Environment)
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
	// Cancel background worker contexts so in-flight cleanup and push delivery
	// stop promptly, then drain the push queue. Stop is idempotent.
	stopWorkers()
	pushSvc.Stop()
}

func buildStore(cfg *config.Config) (storage.ObjectStore, error) {
	if strings.EqualFold(cfg.StorageDriver, "local") {
		return storage.NewLocalStore(cfg.UploadDir)
	}
	return storage.NewS3Store(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UsePathStyle)
}

// configurePush builds the async push notification service. Production treats
// an omitted VAPID configuration as an explicit opt-out: minting a new keypair
// there would invalidate every subscription on the next restart. Development
// and test retain ephemeral keys for convenient local end-to-end coverage.
func configurePush(cfg *config.Config, logger *slog.Logger) *push.Service {
	if cfg.Environment == config.EnvProduction && cfg.VapidPublicKey == "" && cfg.VapidPrivateKey == "" {
		logger.Info("Web Push is disabled because no VAPID keypair is configured")
		return push.NewService(push.Deps{Config: cfg, Logger: logger})
	}
	keyPair, ephemeral, err := push.ResolveKeyPair(cfg.VapidPublicKey, cfg.VapidPrivateKey)
	if err != nil {
		logger.Error("VAPID key configuration is invalid; push notifications disabled", "error", err)
		return push.NewService(push.Deps{Config: cfg, Logger: logger})
	}
	// The resolved keypair is returned through the service rather than written
	// back into the loaded configuration: the config stays read-only after
	// startup and the push handler reads keys from the service.
	subject := cfg.VapidSubject
	if ephemeral {
		subject = "mailto:dev@geoguessme.invalid"
		logger.Warn("VAPID keys not configured; generated ephemeral keys. Existing browser subscriptions will not survive a restart.", "public_key", keyPair.PublicKeyBase64URL())
	}
	sender := push.NewSender(keyPair, subject, nil)
	return push.NewService(push.Deps{Store: push.NewStore(), Deliver: sender, Keys: keyPair, Config: cfg, Logger: logger})
}

// printVapidKeys generates a fresh Web Push keypair and prints it in the
// KEY=value form used by the environment files. Operators run `geoguessme
// vapid-keys` once per deployment and store the output in production.env.
func printVapidKeys() {
	keyPair, err := push.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to generate VAPID keys: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\n", keyPair.PublicKeyBase64URL())
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", keyPair.PrivateKeyBase64URL())
	fmt.Println("# Set VAPID_SUBJECT to a mailto: or https: contact URL, e.g.")
	fmt.Println("# VAPID_SUBJECT=mailto:operator@example.com")
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
