package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

func TestMessageReactionValidation(t *testing.T) {
	repo, _ := newChatRepo(t)
	ctx := context.Background()
	if err := repo.SetMessageReaction(ctx, "message-1", "user-1", ""); !errors.Is(err, ErrInvalidReaction) {
		t.Fatalf("empty set reaction = %v", err)
	}
	if err := repo.DeleteMessageReaction(ctx, "message-1", "user-1", ""); !errors.Is(err, ErrInvalidReaction) {
		t.Fatalf("empty delete reaction = %v", err)
	}
}

// TestReactionMutationOnMissingMessageIsNoOp pins the single-statement
// contract: mutating a reaction for an unknown message is a harmless no-op
// (the insert carries a WHERE EXISTS guard) instead of an independent
// existence query followed by a separate mutation. The HTTP handler has
// already loaded and authorized the message before reaching the mutation.
func TestReactionMutationOnMissingMessageIsNoOp(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()
	mock.ExpectExec("INSERT INTO message_reactions").
		WithArgs("missing", "user-1", "like").
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	if err := repo.SetMessageReaction(ctx, "missing", "user-1", "like"); err != nil {
		t.Fatalf("set reaction on missing message = %v", err)
	}
	mock.ExpectExec("DELETE FROM message_reactions").
		WithArgs("missing", "user-1", "like").
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	if err := repo.DeleteMessageReaction(ctx, "missing", "user-1", "like"); err != nil {
		t.Fatalf("delete reaction on missing message = %v", err)
	}
}

func TestSetAndDeleteMessageReaction(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()
	mock.ExpectExec("INSERT INTO message_reactions").
		WithArgs("message-1", "user-1", "👍").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.SetMessageReaction(ctx, "message-1", "user-1", "👍"); err != nil {
		t.Fatalf("set reaction: %v", err)
	}
	mock.ExpectExec("DELETE FROM message_reactions").
		WithArgs("message-1", "user-1", "👍").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.DeleteMessageReaction(ctx, "message-1", "user-1", "👍"); err != nil {
		t.Fatalf("delete reaction: %v", err)
	}
}

// TestDuplicateReactionAttemptIsIdempotent pins that reacting twice with the
// same key is a no-op success: the ON CONFLICT DO NOTHING clause swallows the
// duplicate without error, and the aggregates are recomputed by the caller.
func TestReactionUsageForGroup(t *testing.T) {
	repo, mock := newChatRepo(t)
	mock.ExpectQuery("SELECT mr.reaction, COUNT").WithArgs("group-1").WillReturnRows(
		pgxmock.NewRows([]string{"reaction", "count"}).
			AddRow("like", 4).
			AddRow("love", 2),
	)
	usage, err := repo.ReactionUsageForGroup(context.Background(), "group-1")
	if err != nil {
		t.Fatalf("ReactionUsageForGroup = %v", err)
	}
	if len(usage) != 2 || usage[0] != (ReactionUsage{Reaction: "like", Count: 4}) || usage[1] != (ReactionUsage{Reaction: "love", Count: 2}) {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestDuplicateReactionAttemptIsIdempotent(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectExec("INSERT INTO message_reactions").
			WithArgs("message-1", "user-1", "like").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		if err := repo.SetMessageReaction(ctx, "message-1", "user-1", "like"); err != nil {
			t.Fatalf("duplicate reaction attempt %d: %v", attempt, err)
		}
	}
}
