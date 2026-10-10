package moderation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geoguessme/handlers"
	"geoguessme/internal/repository/blocking"
)

type blockStoreStub struct {
	owner, target, action string
	err                   error
	items                 []blocking.Block
}

func (s *blockStoreStub) Block(_ context.Context, owner, target string) error {
	s.owner, s.target, s.action = owner, target, "block"
	return s.err
}
func (s *blockStoreStub) Unblock(_ context.Context, owner, target string) error {
	s.owner, s.target, s.action = owner, target, "unblock"
	return s.err
}
func (s *blockStoreStub) List(_ context.Context, owner string) ([]blocking.Block, error) {
	s.owner = owner
	return s.items, s.err
}

func TestBlockChangeValidationAndPrivacy(t *testing.T) {
	const owner = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const target = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	for _, tc := range []struct {
		name, method, target string
		err                  error
		status               int
		action               string
	}{
		{"block", http.MethodPost, target, nil, 204, "block"},
		{"unblock", http.MethodDelete, target, nil, 204, "unblock"},
		{"ineligible", http.MethodPost, target, blocking.ErrUnavailable, 404, "block"},
		{"database", http.MethodPost, target, errors.New("database"), 500, "block"},
		{"self", http.MethodPost, owner, nil, 400, ""},
		{"malformed", http.MethodPost, "oops", nil, 400, ""},
		{"method", http.MethodGet, target, nil, 405, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &blockStoreStub{err: tc.err}
			serialized := 0
			api := BlockAPI{Store: store, SerializeChanges: func(f func() error) error { serialized++; return f() }}
			r := httptest.NewRequestWithContext(context.Background(), tc.method, "/", nil)
			r.SetPathValue("id", tc.target)
			r = r.WithContext(handlers.WithUserID(r.Context(), owner))
			w := httptest.NewRecorder()
			api.Change(w, r)
			if w.Code != tc.status || store.action != tc.action {
				t.Fatalf("status=%d action=%s body=%s", w.Code, store.action, w.Body.String())
			}
			if tc.action != "" && (store.owner != owner || store.target != target || serialized != 1) {
				t.Fatal("ownership or barrier lost")
			}
		})
	}
}

func TestBlockListEmptyAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		store := &blockStoreStub{}
		if fail {
			store.err = errors.New("database")
		}
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		r = r.WithContext(handlers.WithUserID(r.Context(), "owner"))
		w := httptest.NewRecorder()
		(&BlockAPI{Store: store}).List(w, r)
		if fail {
			if w.Code != 500 {
				t.Fatal(w.Code)
			}
		} else if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[]}` || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if store.owner != "owner" {
			t.Fatal("list ownership lost")
		}
	}
}
