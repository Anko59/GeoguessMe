package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/email"
	"geoguessme/internal/middleware"
	"geoguessme/internal/push"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// newCompositionPool returns an isolated mock pool for one composition-test
// instance. Expectations are verified at cleanup, so any query that leaks
// across instances (for example because two Apps shared a global pool) fails
// the test.
func newCompositionPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return mock
}

// TestAppInstancesAreIndependent proves the composition root creates isolated
// application instances: two Apps built on different dependencies share no
// mutable state, each answers from its own repository, and each builds its own
// working route table.
func TestBuildRateLimitPolicies(t *testing.T) {
	// Hand-built configurations without explicit policies fall back to the
	// mandated defaults so the route table keeps its per-route limits.
	fallback := buildRateLimitPolicies(&config.Config{})
	require.Len(t, fallback, len(middleware.DefaultPolicies()))
	names := make(map[string]bool, len(fallback))
	for _, p := range fallback {
		names[p.Name] = true
	}
	for _, wanted := range []string{"login", "signup", "email", "reset", "push", "default"} {
		require.True(t, names[wanted], "fallback must include policy %q", wanted)
	}

	// Explicit configuration converts buckets and applies fail-closed names.
	cfg := &config.Config{
		RateLimitPolicies: []config.RateLimitPolicy{
			{Name: "login", Buckets: []config.RateLimitBucket{{Type: "identity", Limit: 7, Window: time.Minute}}},
			{Name: "default", Buckets: []config.RateLimitBucket{{Type: "trustedIP", Limit: 9, Window: time.Minute}}},
		},
		RateLimitFailClosed: []string{"login"},
	}
	policies := buildRateLimitPolicies(cfg)
	require.Len(t, policies, 2)
	require.Equal(t, "login", policies[0].Name)
	require.True(t, policies[0].FailClosed)
	require.Equal(t, middleware.BucketIdentity, policies[0].Buckets[0].Type)
	require.Equal(t, 7, policies[0].Buckets[0].Limit)
	require.False(t, policies[1].FailClosed)
}

func TestAppInstancesAreIndependent(t *testing.T) {
	cfgA := &config.Config{Environment: config.EnvTest, AllowedOrigins: []string{"http://localhost:8080"}, JWTSecret: "composition-secret-A-that-is-longer-than-32-bytes"}
	cfgB := &config.Config{Environment: config.EnvTest, AllowedOrigins: []string{"http://localhost:8080"}, JWTSecret: "composition-secret-B-that-is-longer-than-32-bytes"}
	storeA, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	poolA := newCompositionPool(t)
	poolB := newCompositionPool(t)
	now := time.Now().UTC()

	// Each instance's repository answers the pilot query from its own pool with
	// different data. If the instances shared state, one response would leak
	// into the other and the second pool's expectations would be unsatisfied.
	poolA.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("a1", "Alpha", "AAA111", now))
	poolB.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("b1", "Beta", "BBB222", now))

	loggerA := slog.New(slog.NewTextHandler(io.Discard, nil))
	loggerB := slog.New(slog.NewJSONHandler(io.Discard, nil))

	appA := NewApp(cfgA, poolA, repository.NewRepository(poolA), storeA, email.SMTP{Host: "mail-a.example"}, push.NewService(push.Deps{Config: cfgA, Logger: loggerA}), chat.NewHub(nil, nil), loggerA, time.Now)
	appB := NewApp(cfgB, poolB, repository.NewRepository(poolB), storeB, email.SMTP{Host: "mail-b.example"}, push.NewService(push.Deps{Config: cfgB, Logger: loggerB}), chat.NewHub(nil, nil), loggerB, time.Now)

	// The dependency graph is per-instance: no pointer is shared between Apps.
	for name, shared := range map[string]bool{
		"config": appA.Config == appB.Config, "db": appA.DB == appB.DB,
		"repos": appA.Repos == appB.Repos, "store": appA.Store == appB.Store,
		"mailer": appA.Mailer == appB.Mailer, "push": appA.Push == appB.Push,
		"hub": appA.Hub == appB.Hub, "logger": appA.Logger == appB.Logger,
		"metrics": appA.Metrics == appB.Metrics, "groups": appA.Groups == appB.Groups,
		"chat": appA.Chat == appB.Chat, "auth": appA.Auth == appB.Auth,
		"authapi": appA.AuthAPI == appB.AuthAPI, "feed": appA.Feed == appB.Feed,
	} {
		if shared {
			t.Fatalf("composition instances share the %s dependency", name)
		}
	}

	// Routing the pilot endpoint through instance A must not affect instance B:
	// each request is served from the owning instance's injected repository.
	reqA := httptest.NewRequest(http.MethodGet, "/api/v1/user/groups", nil)
	reqA = reqA.WithContext(handlers.WithUserID(reqA.Context(), "user-1"))
	recA := httptest.NewRecorder()
	appA.Groups.GetUserGroups(recA, reqA)

	reqB := httptest.NewRequest(http.MethodGet, "/api/v1/user/groups", nil)
	reqB = reqB.WithContext(handlers.WithUserID(reqB.Context(), "user-1"))
	recB := httptest.NewRecorder()
	appB.Groups.GetUserGroups(recB, reqB)

	if recA.Code != http.StatusOK {
		t.Fatalf("instance A status = %d (%s)", recA.Code, recA.Body.String())
	}
	if recB.Code != http.StatusOK {
		t.Fatalf("instance B status = %d (%s)", recB.Code, recB.Body.String())
	}
	if !strings.Contains(recA.Body.String(), "Alpha") || strings.Contains(recA.Body.String(), "Beta") {
		t.Fatalf("instance A leaked or lost state: %s", recA.Body.String())
	}
	if !strings.Contains(recB.Body.String(), "Beta") || strings.Contains(recB.Body.String(), "Alpha") {
		t.Fatalf("instance B leaked or lost state: %s", recB.Body.String())
	}

	// Each App builds its own complete route table and serves a route without
	// any package-global wiring.
	for name, instance := range map[string]struct {
		app  *App
		pool pgxmock.PgxPoolIface
	}{"A": {appA, poolA}, "B": {appB, poolB}} {
		app := instance.app
		routes := app.routes()
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
			t.Fatalf("instance %s /health/live = %d %q", name, rec.Code, rec.Body.String())
		}
		// The group rail must receive the inbox-capable repository rather
		// than the older facade that only implements UserGroups.
		instance.pool.ExpectQuery("SELECT g.id, g.name,").WithArgs("user-1").
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "unread", "message_id", "kind", "username", "created_at"}))
		inboxRequest := httptest.NewRequest(http.MethodGet, "/api/v1/user/groups/inbox", nil)
		inboxRequest = inboxRequest.WithContext(handlers.WithUserID(inboxRequest.Context(), "user-1"))
		inboxRecorder := httptest.NewRecorder()
		app.Groups.GetUserGroupsInbox(inboxRecorder, inboxRequest)
		require.Equal(t, http.StatusOK, inboxRecorder.Code, "instance %s: %s", name, inboxRecorder.Body.String())
		require.JSONEq(t, "[]", inboxRecorder.Body.String())
		// Every public-feed operation must reject anonymous requests before
		// touching its repository or storage, including original-photo access.
		for _, route := range []struct{ method, path string }{
			{"GET", "/feed"}, {"POST", "/feed/challenges"},
			{"GET", "/feed/challenges/post"}, {"DELETE", "/feed/challenges/post"},
			{"GET", "/feed/challenges/post/media"}, {"GET", "/feed/challenges/post/play"},
			{"GET", "/feed/challenges/post/guess"}, {"POST", "/feed/challenges/post/guess"},
			{"PUT", "/feed/challenges/post/reaction"}, {"DELETE", "/feed/challenges/post/reaction"},
			{"GET", "/feed/challenges/post/comments"}, {"POST", "/feed/challenges/post/comments"},
			{"DELETE", "/feed/challenges/post/comments/comment"},
		} {
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, httptest.NewRequest(route.method, "/api/v1"+route.path, nil))
			require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", route.method, route.path)
		}
	}
}

func TestFeedReadsPreserveMutationRateLimit(t *testing.T) {
	middleware.ResetRateLimiter()
	t.Cleanup(middleware.ResetRateLimiter)
	cfg := &config.Config{
		Environment: config.EnvTest,
		JWTSecret:   "feed-rate-test-secret-longer-than-32-bytes",
	}
	pool := newCompositionPool(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := NewApp(cfg, pool, repository.NewRepository(pool), newLocalStore(t), email.SMTP{},
		push.NewService(push.Deps{Config: cfg, Logger: logger}), chat.NewHub(nil, nil), logger, time.Now)
	routes := app.routes()
	token, err := app.Auth.GenerateAccessToken("viewer", 0)
	require.NoError(t, err)
	request := func(method, path, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		if bearer != "" {
			pool.ExpectQuery("SELECT auth_version").WithArgs("viewer").
				WillReturnRows(pgxmock.NewRows([]string{"auth_version", "oidc_linked"}).AddRow(0, false))
		}
		req := httptest.NewRequest(method, "/api/v1/feed"+path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		return rec
	}
	// Invalid IDs reach validation without requiring repository or media
	// fixtures. Browsing more than the mutation quota must remain possible.
	reads := []string{"/challenges/invalid", "/challenges/invalid/media", "/challenges/invalid/play", "/challenges/invalid/guess", "/challenges/invalid/comments"}
	for range 3 {
		for _, path := range reads {
			require.Equal(t, http.StatusBadRequest, request(http.MethodGet, path, token).Code, path)
		}
	}
	for range 10 {
		require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "/challenges/invalid/comments", token).Code)
	}
	blocked := request(http.MethodPost, "/challenges/invalid/comments", token)
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
	// An exhausted mutation quota must not hide photos or remove auth checks.
	for _, path := range reads {
		require.Equal(t, http.StatusBadRequest, request(http.MethodGet, path, token).Code, path)
		require.Equal(t, http.StatusUnauthorized, request(http.MethodGet, path, "").Code, path)
	}
}
