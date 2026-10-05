# Immutable dependency lifecycle. Preparation is explicit; audits never build.
# Defaults are pure preparation/config locators, not stateful consumer refs.
# Consumers select frozen prepared IDs (or caller-supplied registry digests)
# at recipe execution time, after their explicit preparation prerequisites.
CADDY_RUNTIME_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local caddy-runtime)
KEYCLOAK_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local keycloak)
RESTIC_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local restic)
SOPS_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local sops)
SOCKET_PROXY_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local socket-proxy)
POSTGRES_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local postgres)
CLOUDFLARED_IMAGE ?= $(shell bash tools/quality/dependency-images/image-ref.sh local cloudflared)
# Freeze shell-derived exported values to avoid recursive export expansion.
override CADDY_RUNTIME_IMAGE := $(CADDY_RUNTIME_IMAGE)
override KEYCLOAK_IMAGE := $(KEYCLOAK_IMAGE)
override RESTIC_IMAGE := $(RESTIC_IMAGE)
override SOPS_IMAGE := $(SOPS_IMAGE)
override SOCKET_PROXY_IMAGE := $(SOCKET_PROXY_IMAGE)
override POSTGRES_IMAGE := $(POSTGRES_IMAGE)
override CLOUDFLARED_IMAGE := $(CLOUDFLARED_IMAGE)
export CADDY_RUNTIME_IMAGE KEYCLOAK_IMAGE RESTIC_IMAGE SOPS_IMAGE SOCKET_PROXY_IMAGE POSTGRES_IMAGE CLOUDFLARED_IMAGE
IMAGE_AUDIT_PLATFORM ?= linux/amd64
export IMAGE_AUDIT_PLATFORM
AUDIT_IMAGES ?= $(shell awk '!/^\043/ && NF {print $$2}' deployment/images/runtime.tsv)
AUDIT_APPLICATION_IMAGES ?= true
# Treat this finite flag as literal data, never a recursive Make expression.
override AUDIT_APPLICATION_IMAGES := $(value AUDIT_APPLICATION_IMAGES)
export AUDIT_APPLICATION_IMAGES

##@ Dependency artifacts
bootstrap-security-tools: ## Build the Dockerized JSON and security-tool runner.
	$(COMPOSE_TOOLS) build go-security

SECURITY_SCRIPT_PATHS ?= tools/quality/image-audit tools/quality/dependency-images
format-security: ## Format security scripts including newly added files in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) shfmt-write shfmt -w -i 4 -ci $(SECURITY_SCRIPT_PATHS)

prepare-app-runtime: ## Prepare the input-keyed Caddy artifact without rebuilding unchanged inputs.
	bash tools/quality/dependency-images/lifecycle.sh prepare-local caddy-runtime

prepare-database-runtime: ## Prepare the shared compatible PostgreSQL artifact for application and identity stacks.
	bash tools/quality/dependency-images/lifecycle.sh prepare-local postgres

build-sops-image: ## Explicitly prepare SOPS; reuse its immutable content-keyed local artifact.
	bash tools/quality/dependency-images/lifecycle.sh prepare-local sops

build-socket-proxy-image: ## Explicitly prepare socket-proxy; reuse unchanged security inputs.
	bash tools/quality/dependency-images/lifecycle.sh prepare-local socket-proxy

build-security-tool-images: ## Explicitly prepare cached security dependencies (not part of audit-images).
	bash tools/quality/dependency-images/lifecycle.sh prepare-local

publish-security-images: ## Build, scan, sign and publish only missing content-keyed dependency artifacts.
	bash tools/quality/dependency-images/lifecycle.sh publish

resolve-security-images: ## Resolve and verify already-published dependency digests; never build.
	bash tools/quality/dependency-images/lifecycle.sh resolve

recover-reviewed-keycloak: ## Recover only the explicitly reviewed interrupted Keycloak publication in protected CI.
	bash tools/quality/dependency-images/recover-keycloak.sh

test-security-workflows: test-reviewed-keycloak-recovery

test-reviewed-keycloak-recovery: ## Verify exact-origin recovery guards, audit-before-sign ordering, and idempotency without registry writes.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security bash /workspace/tools/quality/dependency-images/test/recovery/test-recovery.sh

audit-image-set: ## Scan an explicitly supplied immutable image set with complete aggregate reporting.
	bash tools/quality/image-audit/audit.sh

audit-images: ## Scan the full required runtime/deployment inventory without building anything.
	@set -eu; case "$${AUDIT_APPLICATION_IMAGES-}" in true|false) ;; *) \
		printf '%s\n' 'image-audit: POLICY: AUDIT_APPLICATION_IMAGES must be true or false' >&2; exit 2 ;; esac; \
	refs="$(AUDIT_IMAGES)"; \
	for component in postgres cloudflared sops socket-proxy keycloak restic caddy-runtime; do \
		if selection=$$(bash tools/quality/dependency-images/selected.sh "$$component" audit); then \
			refs="$$refs $$selection"; \
		else \
			refs="$$refs !missing-selection-$$component"; \
		fi; \
	done; \
	IMAGE_AUDIT_REFS="$$refs $(if $(filter true,$(AUDIT_APPLICATION_IMAGES)),$(BACKEND_IMAGE) $(WEB_IMAGE),)" \
		bash tools/quality/image-audit/audit.sh
