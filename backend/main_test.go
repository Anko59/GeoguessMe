package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"geoguessme/internal/config"
	"geoguessme/internal/database"
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
	cfg := &config.Config{UploadDir: t.TempDir(), S3Endpoint: "://bad", S3Region: "us-east-1", S3Bucket: "bucket", S3AccessKey: "key", S3SecretKey: "secret"}
	previous := os.Getenv("STORAGE_DRIVER")
	if err := os.Setenv("STORAGE_DRIVER", "local"); err != nil {
		t.Fatal(err)
	}
	local, err := buildStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := local.(*storage.LocalStore); !ok {
		t.Fatalf("local store type = %T", local)
	}
	if previous == "" {
		_ = os.Unsetenv("STORAGE_DRIVER")
	} else {
		_ = os.Setenv("STORAGE_DRIVER", previous)
	}
	if _, err := buildStore(cfg); err == nil {
		t.Fatal("invalid S3 configuration accepted")
	}
	oldDB := database.DB
	database.DB = nil
	if err := ready(context.Background(), local); err == nil {
		t.Fatal("ready accepted missing database")
	}
	database.DB = oldDB
	if err := local.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = mock
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
		database.DB = previous
	})
	mock.ExpectPing()
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := ready(context.Background(), store); err != nil {
		t.Fatalf("ready returned an error for healthy dependencies: %v", err)
	}
}
