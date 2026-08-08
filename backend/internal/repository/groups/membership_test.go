package groups

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

// TestRequireMemberCentralizesMembership pins the canonical membership gate
// that every gameplay handler delegates to (roadmap PR 6 item E): a member
// passes, a non-member gets the shared ErrNotMember sentinel, and a
// persistence failure propagates untouched.
func TestRequireMemberCentralizesMembership(t *testing.T) {
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	if err := repo.RequireMember(ctx, "g1", "u1"); err != nil {
		t.Fatalf("member gate = %v, want nil", err)
	}

	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	if err := repo.RequireMember(ctx, "g1", "u2"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("non-member gate = %v, want ErrNotMember", err)
	}

	dbErr := errors.New("connection lost")
	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u3").WillReturnError(dbErr)
	if err := repo.RequireMember(ctx, "g1", "u3"); !errors.Is(err, dbErr) {
		t.Fatalf("persistence failure gate = %v, want %v", err, dbErr)
	}
}
