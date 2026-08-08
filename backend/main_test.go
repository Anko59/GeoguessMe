package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"geoguessme/internal/config"
	"geoguessme/internal/storage"
)

func TestParseLevelAndPlainResponse(t *testing.T) {
	for value, want := range map[string]string{"debug": "DEBUG", "warn": "WARN", "error": "ERROR", "unknown": "INFO"} {
		if got := parseLevel(value).String(); got != want {
			t.Errorf("parseLevel(%q) = %s, want %s", value, got, want)
		}
	}
	recorder := httptest.NewRecorder()
	writePlain(recorder, http.StatusAccepted, "accepted\n")
	if recorder.Code != http.StatusAccepted || recorder.Body.String() != "accepted\n" {
		t.Fatalf("plain response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestBuildStoreAndReadinessFailures(t *testing.T) {
	cfg := &config.Config{StorageDriver: "local", UploadDir: t.TempDir(), S3Endpoint: "://bad", S3Region: "us-east-1", S3Bucket: "bucket", S3AccessKey: "key", S3SecretKey: "secret"}
	local, err := buildStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := local.(*storage.LocalStore); !ok {
		t.Fatalf("local store type = %T", local)
	}
	cfg.StorageDriver = "s3"
	if _, err := buildStore(cfg); err == nil {
		t.Fatal("invalid S3 configuration accepted")
	}
	if err := ready(context.Background(), nil, local); err == nil {
		t.Fatal("ready accepted missing database")
	}
	if err := local.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessSuccess(t *testing.T) {
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
	mock.ExpectPing()
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := ready(context.Background(), mock, store); err != nil {
		t.Fatalf("ready returned an error for healthy dependencies: %v", err)
	}
}

func TestConfigurePushDisablesProductionWithoutVapidKeys(t *testing.T) {
	cfg := &config.Config{Environment: config.EnvProduction}
	svc := configurePush(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if svc.Keys() != nil {
		t.Fatal("production without VAPID keys must disable Push")
	}
}

func TestConfigurePushMintsDevelopmentKeysWithContact(t *testing.T) {
	cfg := &config.Config{Environment: config.EnvDevelopment}
	svc := configurePush(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if svc.Keys() == nil {
		t.Fatal("development without VAPID keys must receive ephemeral Push keys")
	}
	// The loaded configuration must stay read-only: push setup returns the
	// resolved keypair through the service instead of mutating config fields.
	if cfg.VapidPublicKey != "" || cfg.VapidPrivateKey != "" || cfg.VapidSubject != "" {
		t.Fatalf("configurePush mutated the loaded configuration: %+v", cfg)
	}
}
