package main

import (
	"log/slog"
	"net/http"
	"time"

	"geoguessme/handlers"
	authhandlers "geoguessme/handlers/auth"
	"geoguessme/internal/auth"
	"geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/database"
	"geoguessme/internal/email"
	"geoguessme/internal/middleware"
	"geoguessme/internal/push"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
)

// App is the application composition root: it holds every runtime dependency
// and owns route registration. Process lifecycle (connection creation,
// migrations, background workers, server start, graceful shutdown) stays in
// main; main builds an App with NewApp and runs it.
//
// PR 4 migrated one read-only slice (GET /api/v1/user/groups) onto injected
// dependencies, PR 5 migrated the chat slice, PR 6 migrated the gameplay
// slice, and PR 7 migrated the authentication/profile/avatar slice and the
// token service. No handler reads a package global anymore; every handler
// reaches its dependencies through a field here.
type App struct {
	// Config is the validated startup configuration.
	Config *config.Config
	// DB is the connected PostgreSQL pool. The repository collection is bound
	// to it; readiness and migrations still use the database package seam.
	DB database.Pool
	// Repos is the concrete persistence collection used by migrated slices.
	Repos *repository.Repository
	// Store is the object store for user, group, and challenge media.
	Store storage.ObjectStore
	// Mailer sends verification and password-reset email.
	Mailer email.Sender
	// Push fans Web Push notifications to subscribers.
	Push *push.Service
	// Hub is the realtime chat hub.
	Hub *chat.Hub
	// Logger is the JSON process logger.
	Logger *slog.Logger
	// Clock is the injectable time source (time.Now in production); the chat
	// and gameplay migrations consume it for deterministic tests.
	Clock func() time.Time
	// Metrics records request counters and the storage-cleanup backlog.
	Metrics *middleware.Metrics
	// Auth is the explicit token service (issuance, validation).
	Auth *auth.Service

	// Groups is the first handler slice migrated off package globals onto
	// injected dependencies (PR 4 pilot).
	Groups *handlers.GroupAPI
	// Chat is the chat handler slice migrated onto injected dependencies
	// (PR 5): messages, reactions, chat media, and WebSocket tickets.
	Chat *handlers.ChatAPI
	// Game is the gameplay handler slice migrated onto injected dependencies
	// (PR 6): groups, challenges, guesses, media delivery, and leaderboard.
	Game *handlers.GameAPI
	// AuthAPI is the authentication/profile/avatar handler slice migrated
	// onto injected dependencies (PR 7).
	AuthAPI *authhandlers.AuthAPI
}

// NewApp constructs an application instance from explicit dependencies. Each
// call produces an independent App: no package-global mutable state is created
// or read here.
func NewApp(
	cfg *config.Config,
	db database.Pool,
	repos *repository.Repository,
	store storage.ObjectStore,
	mailer email.Sender,
	pushSvc *push.Service,
	hub *chat.Hub,
	logger *slog.Logger,
	clock func() time.Time,
) *App {
	authService := auth.NewService(cfg.JWTSecret, "geoguessme", "geoguessme-web", cfg.AccessTokenTTL)
	return &App{
		Config:  cfg,
		DB:      db,
		Repos:   repos,
		Store:   store,
		Mailer:  mailer,
		Push:    pushSvc,
		Hub:     hub,
		Logger:  logger,
		Clock:   clock,
		Metrics: &middleware.Metrics{ExtraMetrics: pushSvc.MetricsText},
		Auth:    authService,
		Groups:  handlers.NewGroupAPI(repos),
		Chat:    handlers.NewChatAPI(repos.Chat, repos.Groups, store, cfg, hub, clock, repos),
		Game:    handlers.NewGameAPI(repos.Groups, repos.Chat, repos, store, cfg, pushSvc, hub, clock),
		AuthAPI: authhandlers.NewAuthAPI(repos, cfg, store, mailer, authService, hub),
	}
}

// routes builds the complete HTTP handler: the route table followed by the
// middleware chain. The middleware order is load-bearing and is preserved
// exactly: SecurityHeaders → CORS → RequestLog → Recover → RequestID. PR 4
// only relocates the wiring from main into this method; the route set, the
// JSON error envelope, and the middleware order are unchanged.
// buildRateLimitPolicies converts the validated configuration into middleware
// policies. Configurations that never populated the policy slice (hand-built
// in composition tests) fall back to the mandated defaults so the route table
// keeps its per-route limits.
func buildRateLimitPolicies(cfg *config.Config) []middleware.Policy {
	if len(cfg.RateLimitPolicies) == 0 {
		return middleware.DefaultPolicies()
	}
	failClosed := make(map[string]bool, len(cfg.RateLimitFailClosed))
	for _, name := range cfg.RateLimitFailClosed {
		failClosed[name] = true
	}
	out := make([]middleware.Policy, 0, len(cfg.RateLimitPolicies))
	for _, p := range cfg.RateLimitPolicies {
		mp := middleware.Policy{Name: p.Name, FailClosed: failClosed[p.Name]}
		for _, b := range p.Buckets {
			mp.Buckets = append(mp.Buckets, middleware.BucketSpec{
				Type:   middleware.BucketType(b.Type),
				Limit:  b.Limit,
				Window: b.Window,
			})
		}
		out = append(out, mp)
	}
	return out
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	middleware.SetStoreCapacity(a.Config.RateLimitStoreCap)

	// Per-route rate-limit policies (F-04). Each policy is a set of
	// simultaneous buckets (route/global/trusted-IP/identity/user) with the
	// limits mandated by the security remediation plan; the shared bounded
	// store behind them is process-wide and enforces every policy at once.
	// Authenticated routes limit inside the auth middleware so the user
	// bucket (and the identity fallback) see the authenticated user.
	policies := buildRateLimitPolicies(a.Config)
	limits := make(map[string]func(http.Handler) http.Handler, len(policies))
	for _, p := range policies {
		p := p
		limits[p.Name] = middleware.PolicyMiddleware(p, middleware.PolicyOptions{
			TrustedCIDRs: a.Config.TrustedProxyCIDRs,
			Identity: func(r *http.Request) string {
				if id := middleware.ExtractIdentity(r); id != "" {
					return id
				}
				return handlers.GetUserIDFromContext(r)
			},
			User: handlers.GetUserIDFromContext,
		})
	}
	limit := func(name string) func(http.Handler) http.Handler {
		if mw, ok := limits[name]; ok {
			return mw
		}
		// Config validation guarantees every wired policy name exists; the
		// pass-through keeps hand-built test configurations serving without
		// silently weakening a real deployment.
		return func(next http.Handler) http.Handler { return next }
	}
	// limited wraps a handler with its policy so the auth middleware runs
	// first: the user bucket (and the identity fallback) then see the
	// authenticated user for protected routes.
	limited := func(name string, handler http.HandlerFunc) http.HandlerFunc {
		limitedHandler := limit(name)(handler)
		return limitedHandler.ServeHTTP
	}
	protected := func(handler http.HandlerFunc) http.Handler {
		return a.AuthAPI.AuthMiddleware(handler)
	}

	mux.Handle("/api/v1/auth/signup", limit("signup")(http.HandlerFunc(a.AuthAPI.Signup)))
	mux.Handle("/api/v1/auth/login", limit("login")(http.HandlerFunc(a.AuthAPI.Login)))
	mux.Handle("/api/v1/auth/refresh", limit("default")(http.HandlerFunc(a.AuthAPI.Refresh)))
	mux.Handle("/api/v1/auth/logout", limit("default")(http.HandlerFunc(a.AuthAPI.Logout)))
	mux.Handle("/api/v1/auth/verify/request", protected(limited("email", a.AuthAPI.RequestVerification)))
	mux.Handle("/api/v1/auth/verify", limit("default")(http.HandlerFunc(a.AuthAPI.VerifyEmail)))
	mux.Handle("/api/v1/auth/password/forgot", limit("email")(http.HandlerFunc(a.AuthAPI.ForgotPassword)))
	mux.Handle("/api/v1/auth/password/reset", limit("reset")(http.HandlerFunc(a.AuthAPI.ResetPassword)))
	mux.Handle("/api/v1/auth/password/change", protected(limited("default", a.AuthAPI.ChangePassword)))
	mux.Handle("/api/v1/auth/profile", protected(limited("default", a.AuthAPI.UpdateProfile)))
	mux.Handle("/api/v1/auth/profile/avatar", protected(limited("default", a.AuthAPI.UploadAvatar)))
	mux.Handle("/api/v1/auth/account", protected(a.AuthAPI.DeleteAccount))

	mux.Handle("/api/v1/user/groups", protected(a.Groups.GetUserGroups))
	mux.Handle("/api/v1/user/profile/{userID}", protected(a.AuthAPI.GetPublicProfile))
	mux.Handle("/api/v1/group/create", protected(a.Game.CreateGroup))
	mux.Handle("/api/v1/group/join", protected(a.Game.JoinGroup))
	mux.Handle("POST /api/v1/group/invites", protected(a.Game.CreateInvite))
	mux.Handle("GET /api/v1/group/invites", protected(a.Game.ListInvites))
	mux.Handle("POST /api/v1/group/invites/preview", limit("default")(http.HandlerFunc(a.Game.PreviewInvite)))
	mux.Handle("DELETE /api/v1/group/invites/{inviteID}", protected(a.Game.RevokeInvite))
	mux.Handle("/api/v1/group/details", protected(a.Game.GetGroupDetails))
	mux.Handle("/api/v1/group/members", protected(a.Game.GetGroupMembers))
	mux.Handle("/api/v1/group/leaderboard", protected(a.Game.GetLeaderboard))
	mux.Handle("/api/v1/group/photo", protected(a.Game.GroupPhoto))
	mux.Handle("/api/v1/group/notifications", protected(a.Game.GroupNotifications))
	mux.Handle("/api/v1/group/messages", protected(a.Chat.GetGroupMessages))
	mux.Handle("/api/v1/group/message-reactions/{messageID}", protected(a.Chat.SetMessageReaction))
	mux.Handle("/api/v1/group/messages/media", protected(a.Chat.UploadChatMedia))
	mux.Handle("/api/v1/group/messages/media/{mediaID}", protected(a.Chat.ServeChatMedia))
	mux.Handle("/api/v1/photo/upload", protected(a.Game.UploadPhoto))
	mux.Handle("/api/v1/ws/ticket", protected(a.Chat.CreateWebSocketTicket))
	mux.HandleFunc("/api/v1/ws", a.Chat.HandleChat)
	pushHTTP := push.NewHTTP(a.Push)
	mux.Handle("/api/v1/push/subscribe", protected(limited("push", pushHTTP.Subscribe)))
	mux.Handle("/api/v1/push/unsubscribe", protected(limited("push", pushHTTP.Unsubscribe)))
	mux.Handle("/api/v1/push/vapid-public-key", protected(pushHTTP.VapidPublicKey))
	mux.Handle("/api/v1/challenges/{photoID}/accept", protected(a.Game.AcceptChallenge))
	mux.Handle("/api/v1/challenges/{photoID}/media-delivered", protected(a.Game.ConfirmChallengeMediaDelivered))
	mux.Handle("/api/v1/challenges/{photoID}/guess", protected(a.Game.SubmitChallengeGuess))
	mux.Handle("/api/v1/challenges/{photoID}/results", protected(a.Game.GetChallengeResults))
	mux.Handle("/api/v1/challenges/{photoID}/media", protected(a.Game.ServeChallengeMedia))
	mux.Handle("/api/v1/users/{userID}/avatar", protected(a.AuthAPI.ServeUserAvatar))
	registerSystemRoutes(mux, a.Config, a.DB, a.Metrics, a.Store)

	var handler http.Handler = mux
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.CORS(a.Config.AllowedOrigins)(handler)
	handler = middleware.RequestLog(a.Logger, a.Metrics, handler)
	handler = middleware.Recover(a.Logger, handler)
	handler = middleware.RequestID(handler)
	return handler
}
