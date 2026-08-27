package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/config"
	"geoguessme/internal/database"
	"geoguessme/internal/repository"
)

type legacyUserProvisioner interface {
	ProvisionLegacyUser(context.Context, string, string) (authsvc.LegacyProvisionResult, error)
}

type legacyProvisionSummary struct {
	Created      int
	Existing     int
	ActionEmails int
	Failed       int
}

func runLegacyIdentityMigration(ctx context.Context, cfg *config.Config, pool database.Pool, args []string, output io.Writer) error {
	if len(args) == 0 || (args[0] != "plan" && args[0] != "apply") {
		return errors.New("usage: geoguessme legacy-identity-migration [plan|apply --confirm]")
	}
	apply := args[0] == "apply"
	if apply && (len(args) != 2 || args[1] != "--confirm") {
		return errors.New("refusing provisioning without: apply --confirm")
	}
	if !apply && len(args) != 1 {
		return errors.New("usage: geoguessme legacy-identity-migration [plan|apply --confirm]")
	}
	inventory, err := repository.NewRepository(pool).LegacyIdentityMigrationInventory(ctx)
	if err != nil {
		return fmt.Errorf("inventory legacy identities: %w", err)
	}
	fmt.Fprintf(output, "legacy accounts: total=%d linked=%d ready=%d pending_email=%d missing_email=%d\n",
		inventory.Total, inventory.Linked, inventory.Verified, inventory.Pending, inventory.Missing)
	if !apply {
		fmt.Fprintln(output, "plan only: no Keycloak users or emails were changed")
		return nil
	}
	admin, err := authsvc.NewKeycloakAdmin(cfg.OIDCIssuerURL, cfg.OIDCClientID, cfg.OIDCClientSecret)
	if err != nil {
		return err
	}
	redirectURI := strings.TrimRight(cfg.PublicURL, "/") + "/login"
	summary, provisionErr := provisionLegacyUsers(ctx, admin, inventory.VerifiedEmails, redirectURI)
	fmt.Fprintf(output, "Keycloak provisioning: created=%d existing=%d action_emails=%d failed=%d\n",
		summary.Created, summary.Existing, summary.ActionEmails, summary.Failed)
	return provisionErr
}

func provisionLegacyUsers(ctx context.Context, provisioner legacyUserProvisioner, emails []string, redirectURI string) (legacyProvisionSummary, error) {
	summary := legacyProvisionSummary{}
	var firstErr error
	for _, email := range emails {
		result, err := provisioner.ProvisionLegacyUser(ctx, email, redirectURI)
		if err != nil {
			summary.Failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if result.Created {
			summary.Created++
		} else {
			summary.Existing++
		}
		if result.ActionEmailSent {
			summary.ActionEmails++
		}
	}
	if firstErr != nil {
		return summary, fmt.Errorf("%d legacy Keycloak provisioning operation(s) failed; first error: %w", summary.Failed, firstErr)
	}
	return summary, nil
}
