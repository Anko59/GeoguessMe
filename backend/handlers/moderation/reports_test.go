package moderation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geoguessme/handlers"
	moderationrepo "geoguessme/internal/repository/moderation"
)

type reportStoreStub struct {
	called  int
	kind    string
	reason  string
	details string
	result  moderationrepo.ContentReport
	err     error
}

func (s *reportStoreStub) CreateContentReport(_ context.Context, _, kind, _, reason, details string) (moderationrepo.ContentReport, error) {
	s.called++
	s.kind, s.reason, s.details = kind, reason, details
	return s.result, s.err
}

type reportMailStub struct {
	calls int
	body  string
}

func (m *reportMailStub) Send(_, _, body string) error { m.calls++; m.body = body; return nil }

func TestReportUserUsesUserTarget(t *testing.T) {
	store := &reportStoreStub{result: moderationrepo.ContentReport{ID: "receipt-1", Created: false}}
	api := ContentReportAPI{Store: store, Mailer: &reportMailStub{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/user-1/report", strings.NewReader(`{"reason":"other"}`))
	r.SetPathValue("id", "user-1")
	r = r.WithContext(handlers.WithUserID(r.Context(), "reporter-id"))
	w := httptest.NewRecorder()
	api.ReportUser(w, r)
	if w.Code != http.StatusOK || store.kind != "user" {
		t.Fatalf("user report status=%d kind=%s", w.Code, store.kind)
	}
}

func TestReportMessageValidationAndNotification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		status   int
		created  bool
		storeErr error
		calls    int
		mails    int
	}{
		{"new", `{"reason":"harassment","details":"  context  "}`, 200, true, nil, 1, 1},
		{"duplicate", `{"reason":"other"}`, 200, false, nil, 1, 0},
		{"unknown", `{"reason":"other"}`, 404, false, moderationrepo.ErrReportTargetUnavailable, 1, 0},
		{"failure", `{"reason":"other"}`, 500, false, errors.New("db down"), 1, 0},
		{"invalid reason", `{"reason":"invalid"}`, 400, false, nil, 0, 0},
		{"unexplained illegality", `{"reason":"illegal_content"}`, 400, false, nil, 0, 0},
		{"unknown field", `{"reason":"other","admin":true}`, 400, false, nil, 0, 0},
		{"too long", `{"reason":"other","details":"` + strings.Repeat("x", 2001) + `"}`, 400, false, nil, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &reportStoreStub{result: moderationrepo.ContentReport{ID: "report-id", Created: tc.created}, err: tc.storeErr}
			mail := &reportMailStub{}
			api := ContentReportAPI{Store: store, Mailer: mail, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Contact: "privacy@example.test"}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/messages/message-id/report", strings.NewReader(tc.body))
			r.SetPathValue("id", "message-id")
			r = r.WithContext(handlers.WithUserID(r.Context(), "reporter-id"))
			w := httptest.NewRecorder()
			api.ReportMessage(w, r)
			if w.Code != tc.status || store.called != tc.calls || mail.calls != tc.mails {
				t.Fatalf("status=%d store=%d mail=%d body=%s", w.Code, store.called, mail.calls, w.Body.String())
			}
			if tc.name == "new" {
				if store.kind != "message" || store.reason != "harassment" || store.details != "context" {
					t.Fatalf("incorrect store call: %+v", store)
				}
				if strings.Contains(mail.body, "context") || strings.Contains(mail.body, "message-id") || !strings.Contains(mail.body, "report-id") {
					t.Fatalf("notification leaked notice details or lost receipt: %q", mail.body)
				}
			}
		})
	}
}
