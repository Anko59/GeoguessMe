package moderation

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"geoguessme/handlers"
	"geoguessme/internal/email"
	"geoguessme/internal/repository/moderation"
)

type ContentReportStore interface {
	CreateContentReport(ctx context.Context, reporterID, kind, targetID, reason, details string) (moderation.ContentReport, error)
}

type ContentReportAPI struct {
	Store   ContentReportStore
	Mailer  email.Sender
	Logger  *slog.Logger
	Contact string
}

type contentReportRequest struct {
	Reason  string `json:"reason"`
	Details string `json:"details"`
}

type contentReportResponse struct {
	ID string `json:"id"`
}

// ReportMessage and ReportUser accept notices only for visible, non-self
// targets. Details are bounded before persistence and never echoed to clients.
func (a *ContentReportAPI) ReportMessage(w http.ResponseWriter, r *http.Request) {
	a.submit(w, r, "message", r.PathValue("id"))
}

func (a *ContentReportAPI) ReportUser(w http.ResponseWriter, r *http.Request) {
	a.submit(w, r, "user", r.PathValue("id"))
}

func (a *ContentReportAPI) submit(w http.ResponseWriter, r *http.Request, kind, targetID string) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if strings.TrimSpace(targetID) == "" {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid report target")
		return
	}
	var req contentReportRequest
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	switch req.Reason {
	case "illegal_content", "harassment", "sexual_content", "other":
	default:
		handlers.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid report reason")
		return
	}
	req.Details = strings.TrimSpace(req.Details)
	if req.Reason == "illegal_content" && req.Details == "" {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_request", "Explain why this content may be illegal")
		return
	}
	if !utf8.ValidString(req.Details) || utf8.RuneCountInString(req.Details) > 2000 {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_request", "Report details must be at most 2000 characters")
		return
	}
	result, err := a.Store.CreateContentReport(r.Context(), handlers.GetUserIDFromContext(r), kind, targetID, req.Reason, req.Details)
	if errors.Is(err, moderation.ErrReportTargetUnavailable) {
		handlers.WriteError(w, http.StatusNotFound, "not_found", "Report target not found")
		return
	}
	if err != nil {
		a.Logger.Error("content report persistence failed", "error", err)
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to submit report")
		return
	}
	if result.Created {
		a.Logger.Info("content report received", "report_id", result.ID)
		// Notifications are metadata-only: authorized operators look up the
		// receipt in the durable database queue, not in email content.
		body := "A new content report is awaiting review. Receipt ID: " + result.ID
		if err := a.Mailer.Send(a.Contact, "GeoGuessMe content report", body); err != nil {
			a.Logger.Error("content report notification failed", "report_id", result.ID, "error", err)
		}
	}
	handlers.WriteJSON(w, http.StatusOK, contentReportResponse{ID: result.ID})
}
