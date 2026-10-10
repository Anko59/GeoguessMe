package chat

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
)

func TestViewerQueryFiltersBeforeLimitAndRedactsReplyLink(t *testing.T) {
	query, args := viewerMessageQuery(`SELECT `+messageColumns+` FROM messages m WHERE m.group_id=$1 ORDER BY m.created_at DESC LIMIT $2`, []any{"group", 2}, "viewer")
	for _, clause := range []string{
		"CASE WHEN EXISTS", "THEN NULL ELSE m.reply_to_id END",
		"b.blocker_id = $3 AND b.blocked_id = reply.user_id",
		"b.blocker_id = reply.user_id AND b.blocked_id = $3",
		"b.blocker_id = $3 AND b.blocked_id = m.user_id",
		"b.blocker_id = m.user_id AND b.blocked_id = $3",
	} {
		if !strings.Contains(query, clause) {
			t.Fatalf("missing visibility clause %q: %s", clause, query)
		}
	}
	if strings.Index(query, messageVisibility("$3")) > strings.Index(query, " ORDER BY") || len(args) != 3 || args[2] != "viewer" {
		t.Fatalf("visibility must precede pagination: %s; %v", query, args)
	}
}

func TestViewerPaginationAllDirectionsUsesSQLVisibility(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for _, direction := range []string{"latest", "forward", "backward"} {
		t.Run(direction, func(t *testing.T) {
			r, mock := newChatRepo(t)
			param := "$3"
			args := []any{"group-1", 2, "viewer"}
			cursor := ""
			if direction != "latest" {
				param = "$5"
				args = []any{"group-1", now, "anchor", 3, "viewer"}
				cursor = encodeMessageCursor(now, "anchor")
			}
			if direction == "backward" {
				args[3] = 2
				mock.ExpectQuery("SELECT created_at").WithArgs("anchor", "group-1").WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(now))
			}
			mock.ExpectQuery(regexp.QuoteMeta(messageVisibility(param)) + `.*ORDER BY.*LIMIT`).WithArgs(args...).WillReturnRows(messageRowsByID([]string{"visible"}, []time.Time{now.Add(time.Second)}))
			var page MessagesPage
			var err error
			if direction == "backward" {
				mock.ExpectQuery("SELECT message_id, reaction, COUNT").WithArgs([]string{"visible"}, "viewer").WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
				page, err = r.GetGroupMessagesPageBeforeForViewer(t.Context(), "group-1", "anchor", 2, "viewer")
			} else {
				page, err = r.GetGroupMessagesPage(t.Context(), "group-1", cursor, 2, "viewer")
			}
			if err != nil || len(page.Items) != 1 || page.Items[0].ID != "visible" {
				t.Fatalf("visible page = %+v, %v", page, err)
			}
		})
	}
}

func TestBlockedMessagesAndMediaAreUnavailable(t *testing.T) {
	r, mock := newChatRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(messageVisibility("$2"))).WithArgs("blocked-message", "viewer").WillReturnRows(messageRowsByID(nil, nil))
	if msg, err := r.GetMessageForViewer(t.Context(), "blocked-message", "viewer"); err != nil || msg != nil {
		t.Fatalf("blocked message = %+v, %v", msg, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(messageVisibility("$2"))).WithArgs("blocked-media", "viewer").WillReturnError(pgx.ErrNoRows)
	if media, err := r.GetChatMediaForViewer(t.Context(), "blocked-media", "viewer"); err != nil || media != nil {
		t.Fatalf("blocked media = %+v, %v", media, err)
	}
}

func TestBlockedReplyRejectedInTextAndMedia(t *testing.T) {
	for _, media := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "media"}[media], func(t *testing.T) {
			r, mock := newChatRepo(t)
			parent := "blocked-parent"
			msg := &models.Message{ID: "reply", GroupID: "group", UserID: "viewer", Username: "Viewer", ReplyToID: &parent}
			if media {
				mock.ExpectBegin()
			}
			mock.ExpectQuery(regexp.QuoteMeta(messageVisibility("$3"))).WithArgs(parent, "group", "viewer").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
			var err error
			if media {
				mock.ExpectRollback()
				err = r.CreateChatMediaMessage(t.Context(), msg, &models.ChatMedia{ID: "asset", GroupID: "group", UserID: "viewer"})
			} else {
				err = r.SaveMessage(t.Context(), msg)
			}
			if !errors.Is(err, ErrInvalidMessageReply) {
				t.Fatalf("blocked reply = %v", err)
			}
		})
	}
}

func TestBlockedReactionCannotMutate(t *testing.T) {
	r, mock := newChatRepo(t)
	mock.ExpectExec(regexp.QuoteMeta(messageVisibility("$2"))).WithArgs("blocked", "viewer", "like").WillReturnResult(pgxmock.NewResult("INSERT", 0))
	if err := r.SetMessageReaction(t.Context(), "blocked", "viewer", "like"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(messageVisibility("$2"))).WithArgs("blocked", "viewer", "like").WillReturnResult(pgxmock.NewResult("DELETE", 0))
	if err := r.DeleteMessageReaction(t.Context(), "blocked", "viewer", "like"); err != nil {
		t.Fatal(err)
	}
}

func TestReactionAggregatesHideBlockedContributors(t *testing.T) {
	r, mock := newChatRepo(t)
	mock.ExpectQuery(`b.blocker_id=\$2 AND b.blocked_id=u.id.*b.blocker_id=u.id AND b.blocked_id=\$2.*GROUP BY`).WithArgs([]string{"visible"}, "viewer").WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}).AddRow("visible", "like", 1, true, []string{"Viewer"}))
	messages := []models.Message{{ID: "visible"}}
	if err := r.enrichMessageReactions(t.Context(), messages, "viewer"); err != nil || len(messages[0].Reactions) != 1 || messages[0].Reactions[0].Count != 1 {
		t.Fatalf("visible reaction aggregate = %+v, %v", messages, err)
	}
	predicate := strings.ReplaceAll(messageVisibility("$2"), "m.user_id", "mr.user_id")
	mock.ExpectQuery(regexp.QuoteMeta(messageVisibility("$2"))+`.*`+regexp.QuoteMeta(predicate)+`.*GROUP BY.*ORDER BY`).WithArgs("group", "viewer").WillReturnRows(pgxmock.NewRows([]string{"reaction", "count"}).AddRow("like", 1))
	usage, err := r.ReactionUsageForGroupForViewer(t.Context(), "group", "viewer")
	if err != nil || len(usage) != 1 || usage[0].Count != 1 {
		t.Fatalf("visible reaction usage = %+v, %v", usage, err)
	}
}

func TestSendAuthorizationRecheckedAtomicallyAfterStaleValidation(t *testing.T) {
	for _, change := range []string{"removed member", "reply blocked after validation"} {
		t.Run(change, func(t *testing.T) {
			r, mock := newChatRepo(t)
			msg := &models.Message{ID: "send", GroupID: "group", UserID: "viewer", Username: "Viewer", Kind: "text", Content: "hello", CreatedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
			if change == "reply blocked after validation" {
				parent := "parent"
				msg.ReplyToID = &parent
				mock.ExpectQuery(regexp.QuoteMeta(messageVisibility("$3"))).WithArgs(parent, "group", "viewer").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
			}
			// A state change between validation and INSERT is represented by
			// the authoritative guarded statement refusing the write. No sleeps
			// or timing assumptions are needed to reproduce that interleaving.
			mock.ExpectExec(`INSERT INTO messages.*SELECT.*WHERE EXISTS \(SELECT 1 FROM group_members WHERE group_id=\$2 AND user_id=\$3\).*`+regexp.QuoteMeta(messageVisibility("$3"))).WithArgs(msg.ID, msg.GroupID, msg.UserID, msg.Kind, msg.PhotoID, msg.ReplyToID, msg.Content, msg.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 0))
			if err := r.SaveMessage(t.Context(), msg); !errors.Is(err, ErrMessageForbidden) {
				t.Fatalf("stale authorized send = %v", err)
			}
		})
	}
}

func TestMediaSendAuthorizationRecheckedAndAssetRolledBack(t *testing.T) {
	for _, change := range []string{"removed member", "reply blocked after validation"} {
		t.Run(change, func(t *testing.T) {
			r, mock := newChatRepo(t)
			msg := &models.Message{ID: "send", GroupID: "group", UserID: "viewer", Username: "Viewer", Content: "attachment", CreatedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
			asset := &models.ChatMedia{ID: "asset", GroupID: "group", UserID: "viewer", StorageKey: "key", MIMEType: "image/png", CreatedAt: msg.CreatedAt}
			mock.ExpectBegin()
			if change == "reply blocked after validation" {
				parent := "parent"
				msg.ReplyToID = &parent
				mock.ExpectQuery(regexp.QuoteMeta(messageVisibility("$3"))).WithArgs(parent, "group", "viewer").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
			}
			mock.ExpectExec("INSERT INTO chat_media").WithArgs(asset.ID, asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
			mock.ExpectExec(`INSERT INTO messages.*SELECT.*WHERE EXISTS \(SELECT 1 FROM group_members WHERE group_id=\$2 AND user_id=\$3\).*`+regexp.QuoteMeta(messageVisibility("$3"))).WithArgs(msg.ID, msg.GroupID, msg.UserID, asset.ID, msg.ReplyToID, msg.Content, msg.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 0))
			mock.ExpectRollback()
			if err := r.CreateChatMediaMessage(t.Context(), msg, asset); !errors.Is(err, ErrMessageForbidden) {
				t.Fatalf("stale authorized media send = %v", err)
			}
			if msg.MediaID != nil || msg.Kind == "media" {
				t.Fatalf("failed media send reported committed metadata: %+v", msg)
			}
		})
	}
}
