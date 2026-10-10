package moderation

import (
	"context"
	"errors"
	"net/http"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/repository/blocking"

	"github.com/google/uuid"
)

type BlockStore interface {
	Block(context.Context, string, string) error
	Unblock(context.Context, string, string) error
	List(context.Context, string) ([]blocking.Block, error)
}

// SerializeChanges shares the hub's delivery barrier: after a successful block
// response, messages queued before the block cannot escape through a live socket.
type BlockAPI struct {
	Store            BlockStore
	SerializeChanges func(func() error) error
}

func (a *BlockAPI) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	items, err := a.Store.List(r.Context(), handlers.GetUserIDFromContext(r))
	if err != nil {
		handlers.WriteError(w, 500, "internal_error", "Unable to load blocked players")
		return
	}
	if items == nil {
		items = []blocking.Block{}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, 200, map[string]any{"items": items})
}

func (a *BlockAPI) Change(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		handlers.MethodNotAllowed(w)
		return
	}
	owner, target := handlers.GetUserIDFromContext(r), r.PathValue("id")
	if _, err := uuid.Parse(target); err != nil || target == owner {
		handlers.WriteError(w, 400, "invalid_request", "Choose another player")
		return
	}
	change := func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch r.Method {
		case http.MethodPost:
			return a.Store.Block(ctx, owner, target)
		case http.MethodDelete:
			return a.Store.Unblock(ctx, owner, target)
		default:
			return errors.New("unsupported method")
		}
	}
	var err error
	if a.SerializeChanges != nil {
		err = a.SerializeChanges(change)
	} else {
		err = change()
	}
	if errors.Is(err, blocking.ErrUnavailable) {
		handlers.WriteError(w, 404, "not_found", "Player not found")
		return
	}
	if err != nil {
		handlers.WriteError(w, 500, "internal_error", "Unable to change blocked players")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusNoContent)
}
