package push

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"geoguessme/handlers"
)

// HTTP exposes the authenticated push REST endpoints. Routes are wired in
// main.go behind handlers.AuthMiddleware, so every request carries a userID.
type HTTP struct {
	svc *Service
}

// NewHTTP returns the push HTTP handler group.
func NewHTTP(svc *Service) *HTTP { return &HTTP{svc: svc} }

type subscribeKeys struct {
	P256DH string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type subscribeRequest struct {
	Endpoint string        `json:"endpoint"`
	Keys     subscribeKeys `json:"keys"`
	// Some clients send a flat shape; tolerate it for resilience.
	P256DH string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// Subscribe stores a browser push subscription for the authenticated user.
// Re-subscribing the same endpoint refreshes its keys.
func (h *HTTP) Subscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writePushError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if h.svc.Keys() == nil {
		writePushError(w, http.StatusServiceUnavailable, "push_disabled", "Push notifications are not configured")
		return
	}
	var req subscribeRequest
	if !decodePushJSON(w, r, &req) {
		return
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		writePushError(w, http.StatusBadRequest, "invalid_subscription", "endpoint is required")
		return
	}
	if err := validateEndpoint(endpoint); err != nil {
		writePushError(w, http.StatusBadRequest, "invalid_subscription", err.Error())
		return
	}
	p256dh := strings.TrimSpace(firstNonEmpty(req.Keys.P256DH, req.P256DH))
	auth := strings.TrimSpace(firstNonEmpty(req.Keys.Auth, req.Auth))
	if err := validateSubscriptionKeys(p256dh, auth); err != nil {
		writePushError(w, http.StatusBadRequest, "invalid_subscription", err.Error())
		return
	}
	sub := &Subscription{
		UserID:    handlers.GetUserIDFromContext(r),
		Endpoint:  endpoint,
		P256DH:    p256dh,
		Auth:      auth,
		UserAgent: r.UserAgent(),
	}
	if err := h.svc.store.Upsert(r.Context(), sub); err != nil {
		writePushError(w, http.StatusInternalServerError, "internal_error", "Unable to store subscription")
		return
	}
	writePushJSON(w, http.StatusCreated, map[string]any{"id": sub.ID})
}

// Unsubscribe removes a single subscription by endpoint for the user.
func (h *HTTP) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writePushError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	_ = decodePushJSON(w, r, &req) // body optional; endpoint may be omitted to clear all
	endpoint := strings.TrimSpace(req.Endpoint)
	userID := handlers.GetUserIDFromContext(r)
	if endpoint == "" {
		// Removing every subscription for the device is a valid sign-out action.
		subs, err := h.svc.store.ListForUser(r.Context(), userID)
		if err != nil {
			writePushError(w, http.StatusInternalServerError, "internal_error", "Unable to remove subscriptions")
			return
		}
		for i := range subs {
			_ = h.svc.store.Delete(r.Context(), userID, subs[i].Endpoint)
		}
		writePushJSON(w, http.StatusOK, map[string]any{"removed": len(subs)})
		return
	}
	if err := h.svc.store.Delete(r.Context(), userID, endpoint); err != nil {
		if errors.Is(err, ErrNoSubscription) {
			writePushError(w, http.StatusNotFound, "not_found", "Subscription not found")
			return
		}
		writePushError(w, http.StatusInternalServerError, "internal_error", "Unable to remove subscription")
		return
	}
	writePushJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// VapidPublicKey returns the base64url application server public key the browser
// needs to scope its PushManager subscription.
func (h *HTTP) VapidPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writePushError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	keys := h.svc.Keys()
	if keys == nil {
		writePushError(w, http.StatusServiceUnavailable, "push_disabled", "Push notifications are not configured")
		return
	}
	writePushJSON(w, http.StatusOK, map[string]string{"public_key": keys.PublicKeyBase64URL()})
}

func validateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return errors.New("endpoint must be an absolute URL")
	}
	// Push service endpoints are HTTPS in production. HTTP is permitted only so
	// the isolated test stack can capture deliveries through a local server.
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return errors.New("endpoint must use HTTPS")
	}
	return nil
}

func validateSubscriptionKeys(p256dh, auth string) error {
	if p256dh == "" || auth == "" {
		return errors.New("p256dh and auth keys are required")
	}
	pub, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(p256dh, "="))
	if err != nil || len(pub) != 65 || pub[0] != 0x04 {
		return errors.New("p256dh must be a 65-byte uncompressed P-256 public key")
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(auth, "="))
	if err != nil || len(secret) != 16 {
		return errors.New("auth must be a 16-byte secret")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func decodePushJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil {
		return true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			// An empty body is allowed: handlers validate the resulting zero value
			// (a missing endpoint) with a precise error instead of a parse failure.
			return true
		}
		writePushError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return false
	}
	return true
}

func writePushJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writePushError(w http.ResponseWriter, status int, code, message string) {
	writePushJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
