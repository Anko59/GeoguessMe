.DEFAULT_GOAL := help

# GeoGuessMe quality harness.
#
# The root Makefile is a thin orchestrator: every target lives in an included
# fragment grouped by responsibility (setup, quality, tests,
# deployment/rehearsals, maintenance) or in the documentation/API-contract
# fragments owned by earlier PRs. This keeps the root file far below the
# 500-line structure limit while preserving every public target name.
#
# Include order matches the order targets appear in `make help`, because the
# help target iterates $(MAKEFILE_LIST). The canonical .PHONY list below is
# enforced by tools/quality/test/check-makefile-fragments-regression.sh: every
# documented target must be declared phony and resolve.

include tools/make/setup.mk
include tools/make/quality.mk
include tools/make/tests.mk
include tools/make/deployment.mk
include tools/make/maintenance.mk
include tools/make/docs-agent-config.mk
include tools/make/openapi-contract.mk

.PHONY: help impact bootstrap bootstrap-preflight bootstrap-integration bootstrap-e2e hooks-install hooks-check tools-self-test tools-clean \
	dev up down restart status logs logs-backend logs-frontend \
	format format-check fmt fmt-check lint lint-go lint-frontend lint-dead-code lint-debt-markers lint-css lint-docs \
	lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style \
	structure-check type-check archcheck \
	test-unit test-backend test-frontend test-reconnect-harness test-race test-backend-race test-structure-regression test-debt-markers-regression \
	test-makefile-fragments-regression test-archcheck-regression test-ci-retention-regression test-cache-status-regression \
	test-ci-classifier test-e2e-regression test-dev-workflow-regression test-restart-regression \
	test-migration-fixture-regression test-image-scan-exceptions-regression test-integration test-e2e test-e2e-pr test-e2e-ui test-e2e-repeat test-all \
	test-prune-regression test-disk-cleanup-regression test-prod-container-verify-regression \
	test-artifacts-clean-regression test-build-caching test-docs-agent-config coverage \
	audit deps-go-security-update deps-npm-security-update deps-npm-lock \
	build build-backend build-frontend build-images clean-build build-security-tool-images audit-images \
	migrate-up migrate-status migration-new db-backup db-restore \
	backup-rehearsal restore-rehearsal restart-rehearsal reconnect-rehearsal migration-test load-test \
	compose-validate container-verify smoke smoke-rehearsal prod-container-verify \
	prod-config prod-migrate prod-up prod-down prod-logs \
	hosted-config hosted-contract-test cloudflared-access-ssh deployment-hash-check terraform-fmt terraform-fmt-check terraform-init terraform-validate terraform-test terraform-plan terraform-apply secrets-encrypt secrets-generate \
	vapid-keys \
	preflight preflight-docs pr-backend pr-frontend quality verify pre-commit pre-push ci \
	maintenance-report cache-status prune-report prune disk-cleanup-report disk-cleanup build-cache-prune artifacts-clean clean reset-dev \
	openapi-generate openapi-check
