package main

import (
	"log/slog"
	"net/http"
	"time"

	"geoguessme/handlers"
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
// dependencies, PR 5 migrated the chat slice (messages, reactions, chat media,
// WebSocket tickets) onto the ChatAPI, and PR 6 migrated the gameplay slice
// (groups, challenges, guesses, media delivery, leaderboard) onto the GameAPI.
// The remaining handlers still read the legacy package globals
// (handlers.RuntimeConfig, handlers.MediaStore, handlers.Push, database.DB);
// each later migration replaces one more global with a field here. The
// authentication service is also still a package-global seam (auth.Init*) and
// is intentionally not a field yet; PR 7 migrates it.
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
	// Hub is the realtime chat hub. The chat handlers reach it through the
	// ChatAPI; the gameplay handlers reach it through the GameAPI.
	Hub *chat.Hub
	// Logger is the JSON process logger.
	Logger *slog.Logger
	// Clock is the injectable time source (time.Now in production); the chat
	// and gameplay migrations consume it for deterministic tests.
	Clock func() time.Time
	// Metrics records request counters and the storage-cleanup backlog.
	Metrics *middleware.Metrics

	// Groups is the first handler slice migrated off package globals onto
	// injected dependencies (PR 4 pilot).
	Groups *handlers.GroupAPI
	// Chat is the chat handler slice migrated onto injected dependencies
	// (PR 5): messages, reactions, chat media, and WebSocket tickets.
	Chat *handlers.ChatAPI
	// Game is the gameplay handler slice migrated onto injected dependencies
	// (PR 6): groups, challenges, guesses, media delivery, and leaderboard.
	Game *handlers.GameAPI
}

// NewApp constructs an application instance from explicit dependencies. Each
// call produces an independent App: no package-global mutable state is created
// or read here. main wires the legacy handler globals separately for the
// handlers that are not yet migrated.
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
		Metrics: &middleware.Metrics{},
		Groups:  handlers.NewGroupAPI(repos),
		Chat:    handlers.NewChatAPI(repos.Chat, store, cfg, hub, clock, repos),
		Game:    handlers.NewGameAPI(repos.Groups, repos.Chat, repos, store, cfg, pushSvc, hub, clock),
	}
}

// routes builds the complete HTTP handler: the route table followed by the
// middleware chain. The middleware order is load-bearing and is preserved
// exactly: SecurityHeaders → CORS → RequestLog → Recover → RequestID. PR 4
// only relocates the wiring from main into this method; the route set, the
// JSON error envelope, and the middleware order are unchanged.
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	authLimit := middleware.RateLimitByIdentity(a.Config.RateLimitRequests, a.Config.RateLimitWindow, a.Config.TrustedProxyCIDRs)
	protected := func(handler http.HandlerFunc) http.Handler { return http.HandlerFunc(handlers.AuthMiddleware(handler)) }

	mux.Handle("/api/v1/auth/signup", authLimit(http.HandlerFunc(handlers.Signup)))
	mux.Handle("/api/v1/auth/login", authLimit(http.HandlerFunc(handlers.Login)))
	mux.Handle("/api/v1/auth/refresh", http.HandlerFunc(handlers.Refresh))
	mux.Handle("/api/v1/auth/logout", http.HandlerFunc(handlers.Logout))
	mux.Handle("/api/v1/auth/verify/request", authLimit(protected(handlers.RequestVerification)))
	mux.Handle("/api/v1/auth/verify", authLimit(http.HandlerFunc(handlers.VerifyEmail)))
	mux.Handle("/api/v1/auth/password/forgot", authLimit(http.HandlerFunc(handlers.ForgotPassword)))
	mux.Handle("/api/v1/auth/password/reset", authLimit(http.HandlerFunc(handlers.ResetPassword)))
	mux.Handle("/api/v1/auth/password/change", authLimit(protected(handlers.ChangePassword)))
	mux.Handle("/api/v1/auth/profile", authLimit(protected(handlers.UpdateProfile)))
	mux.Handle("/api/v1/auth/profile/avatar", authLimit(protected(handlers.UploadAvatar)))
	mux.Handle("/api/v1/auth/account", protected(handlers.DeleteAccount))

	mux.Handle("/api/v1/user/groups", protected(a.Groups.GetUserGroups))
	mux.Handle("/api/v1/user/profile/{userID}", protected(handlers.GetPublicProfile))
	mux.Handle("/api/v1/group/create", protected(a.Game.CreateGroup))
	mux.Handle("/api/v1/group/join", protected(a.Game.JoinGroup))
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
	mux.Handle("/api/v1/push/subscribe", protected(pushHTTP.Subscribe))
	mux.Handle("/api/v1/push/unsubscribe", protected(pushHTTP.Unsubscribe))
	mux.Handle("/api/v1/push/vapid-public-key", protected(pushHTTP.VapidPublicKey))
	mux.Handle("/api/v1/challenges/{photoID}/accept", protected(a.Game.AcceptChallenge))
	mux.Handle("/api/v1/challenges/{photoID}/media-delivered", protected(a.Game.ConfirmChallengeMediaDelivered))
	mux.Handle("/api/v1/challenges/{photoID}/guess", protected(a.Game.SubmitChallengeGuess))
	mux.Handle("/api/v1/challenges/{photoID}/results", protected(a.Game.GetChallengeResults))
	mux.Handle("/api/v1/challenges/{photoID}/media", protected(a.Game.ServeChallengeMedia))
	mux.Handle("/api/v1/users/{userID}/avatar", protected(handlers.ServeUserAvatar))
	// Link-preview endpoint for group invites: unauthenticated, returns HTML
	// with Open Graph meta tags for messengers and redirects browsers.
	mux.HandleFunc("GET /invite/{code}", a.Game.HandleInvitePreview)

	registerSystemRoutes(mux, a.Config, a.Metrics, a.Store)

	var handler http.Handler = mux
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.CORS(a.Config.AllowedOrigins)(handler)
	handler = middleware.RequestLog(a.Logger, a.Metrics, handler)
	handler = middleware.Recover(a.Logger, handler)
	handler = middleware.RequestID(handler)
	return handler
}
