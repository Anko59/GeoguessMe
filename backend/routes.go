package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"geoguessme/internal/config"
	"geoguessme/internal/database"
	"geoguessme/internal/middleware"
	"geoguessme/internal/storage"
)

// registerSystemRoutes wires the environment-gated test controls, health, and
// metrics endpoints onto mux. It is extracted from main so route behaviour is
// unit-testable without booting the full API server. The metrics authentication
// decision is centralized in config.Config.MetricsAuthRequired so every caller
// applies the same rule.
func registerSystemRoutes(mux *http.ServeMux, cfg *config.Config, metrics *middleware.Metrics, store storage.ObjectStore) {
	// Test-only control endpoints: available only when APP_ENV=test so the
	// integration suite can manipulate rate-limiter state without restarts.
	if cfg.IsTest() {
		mux.HandleFunc("POST /api/v1/test/rate-limit/reset", func(w http.ResponseWriter, _ *http.Request) {
			middleware.ResetRateLimiter()
			writePlain(w, http.StatusOK, "rate limiter state cleared\n")
		})
		mux.HandleFunc("POST /api/v1/test/rate-limit/clock/advance", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Seconds int `json:"seconds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Seconds <= 0 {
				writePlain(w, http.StatusBadRequest, "invalid seconds\n")
				return
			}
			middleware.AdvanceTestClock(time.Duration(body.Seconds) * time.Second)
			writePlain(w, http.StatusOK, "clock advanced\n")
		})
	}

	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writePlain(w, http.StatusOK, "ok\n")
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := ready(r.Context(), store); err != nil {
			writePlain(w, http.StatusServiceUnavailable, "not ready\n")
			return
		}
		writePlain(w, http.StatusOK, "ready\n")
	})

	metricsHandler := metrics.Handler
	if cfg.MetricsAuthRequired() {
		metricsHandler = middleware.MetricsAuth(cfg.MetricsToken, metrics.Handler)
	}
	mux.HandleFunc("/metrics", metricsHandler)
}

// ready reports whether every runtime dependency can serve traffic. It backs
// the /health/ready endpoint and is kept separate from process lifecycle so it
// can be exercised directly in tests.
func ready(ctx context.Context, store storage.ObjectStore) error {
	if database.DB == nil {
		return fmt.Errorf("database unavailable")
	}
	if err := database.DB.Ping(ctx); err != nil {
		return err
	}
	return store.Health(ctx)
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
