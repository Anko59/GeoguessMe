package push

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geoguessme/handlers"
)

func newHTTPService(store Store) *Service {
	keys, _ := GenerateKeyPair()
	return NewService(Deps{Store: store, Keys: keys, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func userRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(handlers.WithUserID(context.Background(), "user-1"))
}

func TestVapidPublicKeyEndpoint(t *testing.T) {
	svc := newHTTPService(&fakeStore{})
	h := NewHTTP(svc)

	rec := httptest.NewRecorder()
	h.VapidPublicKey(rec, userRequest(http.MethodGet, "/", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "public_key") {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// Push disabled (no keys) surfaces a 503 rather than an empty key.
	disabled := NewService(Deps{Store: &fakeStore{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec = httptest.NewRecorder()
	NewHTTP(disabled).VapidPublicKey(rec, userRequest(http.MethodGet, "/", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled status = %d", rec.Code)
	}
}

func TestSubscribeValidatesAndStores(t *testing.T) {
	store := &fakeStore{}
	h := NewHTTP(newHTTPService(store))

	body := `{"endpoint":"https://fcm.googleapis.com/fcm/send/abc","keys":{"p256dh":"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4","auth":"BTBZMqHH6r4Tts7J_aSIgg"}}`
	rec := httptest.NewRecorder()
	h.Subscribe(rec, userRequest(http.MethodPost, "/", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("subscribe status = %d (%s)", rec.Code, rec.Body.String())
	}

	cases := map[string]string{
		"missing endpoint": `{"keys":{"p256dh":"x","auth":"y"}}`,
		"bad scheme":       `{"endpoint":"ftp://example/x","keys":{"p256dh":"x","auth":"y"}}`,
		"bad keys":         `{"endpoint":"https://example/x","keys":{"p256dh":"short","auth":"BTBZMqHH6r4Tts7J_aSIgg"}}`,
		"bad auth length":  `{"endpoint":"https://example/x","keys":{"p256dh":"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4","auth":"too-short"}}`,
		"malformed json":   `{not json`,
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.Subscribe(rec, userRequest(http.MethodPost, "/", b))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSubscribeRejectsUnsupportedMethod(t *testing.T) {
	h := NewHTTP(newHTTPService(&fakeStore{}))
	rec := httptest.NewRecorder()
	h.Subscribe(rec, userRequest(http.MethodPatch, "/", "{}"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSubscribeEnforcesPerUserCap(t *testing.T) {
	store := &fakeStore{subsByUser: map[string][]Subscription{}}
	const p256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	const auth = "BTBZMqHH6r4Tts7J_aSIgg"
	// Five existing endpoints fill the default cap of five.
	for i := 0; i < 5; i++ {
		store.subsByUser["user-1"] = append(store.subsByUser["user-1"], Subscription{
			ID: fmt.Sprintf("s%d", i), UserID: "user-1", Endpoint: fmt.Sprintf("https://fcm.googleapis.com/e%d", i),
		})
	}
	h := NewHTTP(newHTTPService(store))

	// A sixth distinct endpoint is rejected with a 409.
	rec := httptest.NewRecorder()
	h.Subscribe(rec, userRequest(http.MethodPost, "/", fmt.Sprintf(`{"endpoint":"https://fcm.googleapis.com/new","keys":{"p256dh":"%s","auth":"%s"}}`, p256dh, auth)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("sixth subscribe status = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "subscription_limit") {
		t.Fatalf("expected subscription_limit error code, got %s", rec.Body.String())
	}

	// Refreshing an existing endpoint stays allowed.
	rec = httptest.NewRecorder()
	h.Subscribe(rec, userRequest(http.MethodPost, "/", fmt.Sprintf(`{"endpoint":"https://fcm.googleapis.com/e0","keys":{"p256dh":"%s","auth":"%s"}}`, p256dh, auth)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("refresh status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
}

func TestUnsubscribeByEndpointAndAll(t *testing.T) {
	store := &fakeStore{subsByUser: map[string][]Subscription{
		"user-1": {{ID: "s1", UserID: "user-1", Endpoint: "https://example/a"}, {ID: "s2", UserID: "user-1", Endpoint: "https://example/b"}},
	}}
	h := NewHTTP(newHTTPService(store))

	rec := httptest.NewRecorder()
	h.Unsubscribe(rec, userRequest(http.MethodDelete, "/", `{"endpoint":"https://example/a"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete one status = %d (%s)", rec.Code, rec.Body.String())
	}

	// Deleting an unknown endpoint returns 404, not a server error.
	rec = httptest.NewRecorder()
	h.Unsubscribe(rec, userRequest(http.MethodDelete, "/", `{"endpoint":"https://example/missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing endpoint status = %d", rec.Code)
	}

	// An empty body removes every subscription for the user (sign-out).
	rec = httptest.NewRecorder()
	h.Unsubscribe(rec, userRequest(http.MethodDelete, "/", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete all status = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestSubscribeDisabledWhenNoKeys(t *testing.T) {
	disabled := NewService(Deps{Store: &fakeStore{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	h := NewHTTP(disabled)
	rec := httptest.NewRecorder()
	h.Subscribe(rec, userRequest(http.MethodPost, "/", `{"endpoint":"https://example/x","keys":{"p256dh":"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4","auth":"BTBZMqHH6r4Tts7J_aSIgg"}}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled subscribe status = %d", rec.Code)
	}
}
