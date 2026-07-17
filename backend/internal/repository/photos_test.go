package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func pgErr(code string) error { return &pgconn.PgError{Code: code} }

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(pgErr("23505")) {
		t.Error("23505 should be a unique violation")
	}
	if isUniqueViolation(pgErr("40001")) {
		t.Error("40001 should not be a unique violation")
	}
	if isUniqueViolation(errors.New("not pg")) {
		t.Error("non-pg error should not be a unique violation")
	}
}

func TestIsRetryable(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		if !isRetryable(pgErr(code)) {
			t.Errorf("%s should be retryable", code)
		}
	}
	for _, code := range []string{"23505", "23503", "23502"} {
		if isRetryable(pgErr(code)) {
			t.Errorf("%s should not be retryable", code)
		}
	}
	if isRetryable(errors.New("network error")) {
		t.Error("non-pg error should not be retryable")
	}
}
