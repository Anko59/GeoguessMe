package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// PushNotifier fans Web Push notifications to group members. It is injected
// into the gameplay and chat handler slices from the push.Service
// implementation; handlers reference it only via this interface to avoid
// importing the push package and creating a cycle (push imports handlers for
// context helpers, handlers must not import push).
type PushNotifier interface {
	NotifyNewChallenge(ctx context.Context, groupID, excludeUserID, photoID string)
	NotifyNewMessage(ctx context.Context, groupID, senderUserID, content string)
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error APIError `json:"error"`
}

// WriteJSON writes a JSON response with the given status. It is exported so
// the auth handler sub-package (handlers/auth) shares the exact same response
// envelope as the rest of the transport layer.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteError writes the stable JSON error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorEnvelope{Error: APIError{Code: code, Message: message}})
}

// MethodNotAllowed writes the standard 405 envelope.
func MethodNotAllowed(w http.ResponseWriter) {
	WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
}

// DecodeJSON strictly decodes a JSON request body, rejecting trailing values
// and unknown fields. It writes the invalid-request envelope on failure and
// returns false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return false
	}
	return true
}

type contextKey string

const (
	userIDKey            contextKey = "userID"
	migrationRequiredKey contextKey = "migrationRequired"
)

// GetUserIDFromContext returns the authenticated user identifier carried by
// the request context, or an empty string for anonymous requests.
func GetUserIDFromContext(r *http.Request) string {
	userID, _ := r.Context().Value(userIDKey).(string)
	return userID
}

// WithUserID returns a context carrying the authenticated user identifier. It
// lets packages that own handlers wired behind AuthMiddleware (such as the
// push REST endpoints and the handlers/auth sub-package) populate a request
// context without depending on the unexported context key.
func WithUserID(parent context.Context, userID string) context.Context {
	return context.WithValue(parent, userIDKey, userID)
}

// MigrationRequired reports whether the authenticated account is still using
// the read-only legacy migration session.
func MigrationRequired(r *http.Request) bool {
	required, _ := r.Context().Value(migrationRequiredKey).(bool)
	return required
}

// WithMigrationRequired marks a request as read-only until its canonical user
// is linked to Keycloak.
func WithMigrationRequired(parent context.Context, required bool) context.Context {
	return context.WithValue(parent, migrationRequiredKey, required)
}
