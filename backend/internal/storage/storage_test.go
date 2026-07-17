package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalStoreLifecycleAndKeyValidation(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("private media")
	if err := store.Put(context.Background(), "photos/example.txt", bytes.NewReader(content), int64(len(content)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	size, err := store.Stat(context.Background(), "photos/example.txt")
	if err != nil || size != int64(len(content)) {
		t.Fatalf("stat = %d, %v", size, err)
	}
	object, err := store.Get(context.Background(), "photos/example.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(object)
	if closeErr := object.Close(); err != nil || closeErr != nil {
		t.Fatalf("read/close = %v/%v", err, closeErr)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content = %q, want %q", got, content)
	}
	if err := store.Delete(context.Background(), "photos/example.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "photos/example.txt"); err != nil {
		t.Fatalf("delete should be idempotent: %v", err)
	}
	if _, err := store.Get(context.Background(), "photos/example.txt"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing get error = %v", err)
	}
	if _, err := store.Stat(context.Background(), "photos/example.txt"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing stat error = %v", err)
	}
	if err := store.Health(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"../escape", "../../escape", filepath.Join(string(os.PathSeparator), "absolute")} {
		if err := store.Put(context.Background(), key, bytes.NewReader(content), int64(len(content)), ""); err == nil {
			t.Errorf("Put(%q) unexpectedly succeeded", key)
		}
		if _, err := store.Get(context.Background(), key); err == nil {
			t.Errorf("Get(%q) unexpectedly succeeded", key)
		}
	}
}

func TestLocalStoreRejectsPartialWritesAndCanceledHealth(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "too-short", bytes.NewReader([]byte("abc")), 4, ""); err == nil {
		t.Fatal("short write unexpectedly succeeded")
	}
	if err := store.Put(context.Background(), "too-long", bytes.NewReader([]byte("abcde")), 4, ""); err == nil {
		t.Fatal("oversized write unexpectedly succeeded")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Health(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled health = %v", err)
	}
}

func TestS3ConfigurationAndNotFoundMapping(t *testing.T) {
	if _, err := NewS3Store("://bad", "us-east-1", "bucket", "key", "secret", true); err == nil {
		t.Fatal("invalid endpoint unexpectedly succeeded")
	}
	if _, err := NewS3Store("http://", "us-east-1", "bucket", "key", "secret", true); err == nil {
		t.Fatal("endpoint without host unexpectedly succeeded")
	}
	for _, err := range []error{errors.New("code:NoSuchKey"), errors.New("HTTP 404"), errors.New("permission denied")} {
		if got := isS3NotFound(err); got != (err.Error() != "permission denied") {
			t.Errorf("isS3NotFound(%q) = %v", err, got)
		}
	}
	if isS3NotFound(nil) {
		t.Fatal("nil error reported as not found")
	}
}

func TestS3OperationsAgainstDeterministicEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "7")
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("payload"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store, err := NewS3Store(server.URL, "us-east-1", "bucket", "key", "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(context.Background(), "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "photos/a", strings.NewReader("payload"), 7, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	object, err := store.Get(context.Background(), "photos/a")
	if err != nil {
		t.Fatal(err)
	}
	if body, err := io.ReadAll(object); err != nil || string(body) != "payload" {
		t.Fatalf("S3 body = %q, %v", body, err)
	}
	if err := object.Close(); err != nil {
		t.Fatal(err)
	}
	size, err := store.Stat(context.Background(), "photos/a")
	if err != nil || size != 7 {
		t.Fatalf("S3 stat = %d, %v", size, err)
	}
	if err := store.Delete(context.Background(), "photos/a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}
