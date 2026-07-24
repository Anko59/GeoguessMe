package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func mustReceiverSubscription(t *testing.T, endpoint string) *Subscription {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate receiver key: %v", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatalf("generate auth secret: %v", err)
	}
	return &Subscription{Endpoint: endpoint, P256DH: b64(priv.PublicKey().Bytes()), Auth: b64(auth)}
}

// statusServer returns the configured status for every POST and records the
// last request so tests can assert headers and body shape.
func statusServer(t *testing.T, status int) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var last atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		last.Store(map[string]string{
			"content-encoding": r.Header.Get("Content-Encoding"),
			"authorization":    r.Header.Get("Authorization"),
			"ttl":              r.Header.Get("TTL"),
			"body-prefix":      string(body[:min(len(body), 16)]),
		})
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, &last
}

func TestSenderSuccessAndHeaders(t *testing.T) {
	server, last := statusServer(t, http.StatusCreated)
	keys, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sender := NewSender(keys, "mailto:ops@example.com", server.Client())
	sub := mustReceiverSubscription(t, server.URL+"/push/abc")
	if err := sender.Send(context.Background(), sub, []byte(`{"title":"hi"}`)); err != nil {
		t.Fatalf("send: %v", err)
	}
	captured, ok := last.Load().(map[string]string)
	if !ok {
		t.Fatal("no request captured")
	}
	if captured["content-encoding"] != "aes128gcm" {
		t.Fatalf("content-encoding = %q", captured["content-encoding"])
	}
	if !strings.HasPrefix(captured["authorization"], "vapid t=") || !strings.Contains(captured["authorization"], ", k=") {
		t.Fatalf("authorization = %q", captured["authorization"])
	}
	if captured["ttl"] != "86400" {
		t.Fatalf("ttl = %q", captured["ttl"])
	}
	// The body must start with the 16-byte salt, i.e. be non-empty and not the
	// raw JSON payload (which would mean encryption was skipped).
	if len(captured["body-prefix"]) == 0 || strings.HasPrefix(captured["body-prefix"], "{") {
		t.Fatalf("body was not encrypted: %q", captured["body-prefix"])
	}
}

func TestSenderStatusClassification(t *testing.T) {
	keys, _ := GenerateKeyPair()
	cases := []struct {
		name   string
		status int
		gone   bool
	}{
		{"created", http.StatusCreated, false},
		{"ok", http.StatusOK, false},
		{"gone", http.StatusGone, true},
		{"not found", http.StatusNotFound, true},
		{"bad request", http.StatusBadRequest, true},
		{"forbidden", http.StatusForbidden, true},
		{"too many requests", http.StatusTooManyRequests, false},
		{"server error", http.StatusInternalServerError, false},
		{"service unavailable", http.StatusServiceUnavailable, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server, _ := statusServer(t, c.status)
			sender := NewSender(keys, "mailto:ops@example.com", server.Client())
			sub := mustReceiverSubscription(t, server.URL+"/push/abc")
			err := sender.Send(context.Background(), sub, []byte(`{}`))
			if c.gone && !errors.Is(err, ErrSubscriptionGone) {
				t.Fatalf("status %d: expected ErrSubscriptionGone, got %v", c.status, err)
			}
			if !c.gone && errors.Is(err, ErrSubscriptionGone) {
				t.Fatalf("status %d: must not be classified gone, got %v", c.status, err)
			}
		})
	}
}

func TestSenderRejectsInvalidSubscription(t *testing.T) {
	server, _ := statusServer(t, http.StatusCreated)
	keys, _ := GenerateKeyPair()
	sender := NewSender(keys, "mailto:ops@example.com", server.Client())
	bad := &Subscription{Endpoint: server.URL + "/push/abc", P256DH: "not-a-real-key", Auth: b64(make([]byte, 16))}
	if err := sender.Send(context.Background(), bad, []byte(`{}`)); !errors.Is(err, ErrSubscriptionGone) {
		t.Fatalf("expected gone error for bad p256dh, got %v", err)
	}
}

func TestSenderRejectsBadEndpointScheme(t *testing.T) {
	keys, _ := GenerateKeyPair()
	sender := NewSender(keys, "mailto:ops@example.com", nil)
	sub := mustReceiverSubscription(t, "ftp://example.test/push")
	if err := sender.Send(context.Background(), sub, []byte(`{}`)); err == nil {
		t.Fatal("expected error for ftp endpoint")
	}
}
