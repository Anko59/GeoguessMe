package handlers

import (
	"encoding/json"
	"net/http"

	"geoguessme/internal/config"
	"geoguessme/internal/email"
	"geoguessme/internal/storage"
)

var (
	RuntimeConfig *config.Config
	MediaStore    storage.ObjectStore
	Mailer        email.Sender = email.Noop{}
)

func Configure(cfg *config.Config, store storage.ObjectStore, sender email.Sender) {
	RuntimeConfig = cfg
	MediaStore = store
	if sender != nil {
		Mailer = sender
	}
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error APIError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: APIError{Code: code, Message: message}})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
}
