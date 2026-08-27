// Package party serves the Party Time transport slice: reading a group's
// current party state and starting a party under the single-active-window
// and recharge rules. It follows the handlers/auth sub-package pattern: a
// small handler slice with explicit dependencies, composed by the
// application composition root in backend/app.go. Persistence lives in
// internal/repository/party; this package performs transport parsing,
// membership delegation, the announcement side effects, and response writing.
package party

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"geoguessme/handlers"
	chatHub "geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/models"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/repository/party"

	"github.com/google/uuid"
)

// Notifier fans the Party Time push notification out to the group members.
// It is defined at the consumer boundary so tests substitute a recorder; the
// concrete *push.Service satisfies it structurally.
type Notifier interface {
	NotifyPartyStarted(ctx context.Context, groupID, excludeUserID, starterUsername string)
}

// ProfileLookup resolves the starter's display name for the announcement
// text. The concrete repository collection satisfies it.
type ProfileLookup interface {
	GetUserByID(ctx context.Context, userID string) (*models.User, error)
}

// API serves the party slice from injected dependencies.
type API struct {
	groups   *groups.Repository
	windows  *party.Repository
	messages *chatrepo.Repository
	profiles ProfileLookup
	push     Notifier
	hub      *chatHub.Hub
	cfg      *config.Config
	clock    func() time.Time
}

// NewAPI constructs the party transport with its explicit dependencies.
func NewAPI(groupsRepo *groups.Repository, windows *party.Repository, messages *chatrepo.Repository, profiles ProfileLookup, push Notifier, hub *chatHub.Hub, cfg *config.Config, clock func() time.Time) *API {
	return &API{groups: groupsRepo, windows: windows, messages: messages, profiles: profiles, push: push, hub: hub, cfg: cfg, clock: clock}
}

// HandleParty routes the shared /api/v1/group/party endpoint by method, the
// same pattern as the group photo and notification endpoints.
func (a *API) HandleParty(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.partyStatus(w, r)
	case http.MethodPost:
		a.startParty(w, r)
	default:
		handlers.MethodNotAllowed(w)
	}
}

func (a *API) requireMember(w http.ResponseWriter, r *http.Request, groupID, userID string) bool {
	if err := a.groups.RequireMember(r.Context(), groupID, userID); err != nil {
		handlers.WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return false
	}
	return true
}

// statusResponse is the wire shape of a group's party state. started_at and
// ends_at are present only while a party is active; next_available_at is
// present while the recharge cooldown still blocks a new start. server_time
// lets clients correct for clock skew, matching the gameplay endpoints.
type statusResponse struct {
	Active          bool       `json:"active"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	EndsAt          *time.Time `json:"ends_at,omitempty"`
	NextAvailableAt *time.Time `json:"next_available_at,omitempty"`
	ServerTime      time.Time  `json:"server_time"`
}

// buildStatus derives the wire state from the latest recorded window. A
// future-dated window cannot exist, so the latest started window fully
// determines both the active flag and the recharge deadline.
func buildStatus(now time.Time, cooldown time.Duration, window *party.Window) statusResponse {
	response := statusResponse{ServerTime: now}
	if window == nil {
		return response
	}
	nextAvailable := window.NextAvailableAt(cooldown)
	if window.Active(now) {
		startedAt, endsAt := window.StartedAt, window.EndsAt
		response.Active = true
		response.StartedAt = &startedAt
		response.EndsAt = &endsAt
		response.NextAvailableAt = &nextAvailable
		return response
	}
	if now.Before(nextAvailable) {
		response.NextAvailableAt = &nextAvailable
	}
	return response
}

func (a *API) partyStatus(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := handlers.ValidateID(groupID, "group_id"); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	window, err := a.windows.Status(r.Context(), groupID, a.clock())
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load party status")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, buildStatus(a.clock(), a.cfg.PartyTimeCooldown, window))
}

type startPartyRequest struct {
	GroupID string `json:"group_id"`
}

// startParty opens a new one-hour party window for the group, announces it
// as a persisted system chat message, and fans a push notification to every
// other member. The recharge rule (one party per configured cooldown from
// the previous end) is enforced atomically by the persistence layer.
func (a *API) startParty(w http.ResponseWriter, r *http.Request) {
	var req startPartyRequest
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	groupID := strings.TrimSpace(req.GroupID)
	if err := handlers.ValidateID(groupID, "group_id"); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	now := a.clock().UTC()
	window, err := a.windows.Start(r.Context(), groupID, userID, now, a.cfg.PartyTimeDuration, a.cfg.PartyTimeCooldown)
	if err != nil {
		switch {
		case errors.Is(err, party.ErrNotFound):
			handlers.WriteError(w, http.StatusNotFound, "group_not_found", "Group not found")
		case errors.Is(err, party.ErrPartyActive):
			handlers.WriteError(w, http.StatusConflict, "party_active", "Party time is already running")
		case errors.Is(err, party.ErrPartyRecharging):
			handlers.WriteError(w, http.StatusConflict, "party_recharging", "Party time is recharging; try again later")
		default:
			handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start party time")
		}
		return
	}
	username := a.starterName(r.Context(), userID)
	a.persistAnnouncement(w, r, groupID, userID, username, now, window)
	if a.push != nil {
		a.push.NotifyPartyStarted(r.Context(), groupID, userID, username)
	}
	handlers.WriteJSON(w, http.StatusCreated, buildStatus(now, a.cfg.PartyTimeCooldown, window))
}

// starterName resolves the display name used in the announcement copy; a
// missing profile degrades to "Someone" exactly like the push resolver does.
func (a *API) starterName(ctx context.Context, userID string) string {
	if a.profiles != nil {
		if user, err := a.profiles.GetUserByID(ctx, userID); err == nil && user != nil && user.Username != "" {
			return user.Username
		}
	}
	return "Someone"
}

// persistAnnouncement writes the system chat message and broadcasts it to
// the live group sockets. A persistence failure answers 500 so the client
// knows the announcement was not recorded even though the window started.
func (a *API) persistAnnouncement(w http.ResponseWriter, r *http.Request, groupID, userID, starterUsername string, now time.Time, window *party.Window) {
	message := &models.Message{
		ID:        uuid.NewString(),
		GroupID:   groupID,
		UserID:    userID,
		Kind:      "system",
		Content:   announcementText(starterUsername, window),
		CreatedAt: now,
	}
	if err := a.messages.SaveMessage(r.Context(), message); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to announce the party")
		return
	}
	if a.hub != nil {
		a.hub.BroadcastPersisted(*message)
	}
}

// announcementText composes the persistent chat copy shown in history and
// pushed to members' devices.
func announcementText(starterUsername string, window *party.Window) string {
	return "🎉 " + starterUsername + " started Party Time! Post a challenge and your guesses score double until " +
		window.EndsAt.UTC().Format("15:04 UTC") + "."
}
