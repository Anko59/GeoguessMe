package main

import (
	"context"
	"errors"
	"testing"

	authsvc "geoguessme/internal/auth"
)

type fakeLegacyProvisioner struct {
	results map[string]authsvc.LegacyProvisionResult
	fail    map[string]bool
}

func (f fakeLegacyProvisioner) ProvisionLegacyUser(_ context.Context, email, _ string) (authsvc.LegacyProvisionResult, error) {
	if f.fail[email] {
		return authsvc.LegacyProvisionResult{}, errors.New("sanitized provider failure")
	}
	return f.results[email], nil
}

func TestProvisionLegacyUsersContinuesAndReturnsAggregateFailure(t *testing.T) {
	provisioner := fakeLegacyProvisioner{
		results: map[string]authsvc.LegacyProvisionResult{
			"new@example.test":      {Created: true, ActionEmailSent: true},
			"existing@example.test": {},
		},
		fail: map[string]bool{"failed@example.test": true},
	}
	summary, err := provisionLegacyUsers(t.Context(), provisioner, []string{
		"new@example.test", "failed@example.test", "existing@example.test",
	}, "https://geoguessme.example/login")
	if err == nil || summary.Created != 1 || summary.Existing != 1 || summary.ActionEmails != 1 || summary.Failed != 1 {
		t.Fatalf("summary = %+v, error = %v", summary, err)
	}
}
