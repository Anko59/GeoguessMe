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

	"github.com/pashagolub/pgxmock/v4"
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
		"authapi": appA.AuthAPI == appB.AuthAPI,
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
	for name, app := range map[string]*App{"A": appA, "B": appB} {
		rec := httptest.NewRecorder()
		app.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
			t.Fatalf("instance %s /health/live = %d %q", name, rec.Code, rec.Body.String())
		}
	}
}
