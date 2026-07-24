.DEFAULT_GOAL := help

.PHONY: help bootstrap bootstrap-integration bootstrap-e2e hooks-install hooks-check tools-clean tools-self-test \
	dev up down restart status logs logs-backend logs-frontend \
	format format-check fmt fmt-check lint lint-go lint-frontend lint-css lint-docs \
	lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style \
	type-check test-unit test-backend test-frontend test-race test-backend-race test-structure-regression \
	test-cache-status-regression test-ci-classifier test-e2e-regression test-dev-workflow-regression cache-status structure-check \
	test-prod-container-verify-regression test-migration-fixture-regression \
	test-prune-regression prune-report prune artifacts-clean \
	test-ci-retention-regression test-artifacts-clean-regression \
	test-disk-cleanup-regression disk-cleanup-report disk-cleanup \
	test-integration test-e2e test-e2e-pr test-e2e-ui test-e2e-repeat test-all coverage audit \
	build build-backend build-frontend build-images clean-build build-cache-prune test-build-caching \
	migrate-up migrate-status migration-new db-backup db-restore \
	backup-rehearsal restore-rehearsal restart-rehearsal reconnect-rehearsal test-restart-regression migration-test load-test \
	compose-validate container-verify smoke smoke-rehearsal prod-container-verify \
	prod-config prod-migrate prod-up prod-down prod-logs \
	hosted-config hosted-contract-test cloudflared-access-ssh terraform-fmt terraform-fmt-check terraform-init terraform-validate terraform-test terraform-plan terraform-apply secrets-encrypt secrets-generate \
	preflight preflight-docs pr-backend pr-frontend quality verify pre-commit pre-push ci clean reset-dev deps-go-security-update deps-npm-security-update

COMPOSE_DEV  := docker compose -p geoguessme-dev -f deployment/compose.dev.yaml --project-directory .
COMPOSE_TEST := docker compose -f deployment/compose.test.yaml --project-directory .
COMPOSE_PROD := docker compose -p geoguessme-prod -f deployment/compose.production.yaml --project-directory .
COMPOSE_TOOLS := docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory .
COMPOSE_TOOLS_RUN := $(COMPOSE_TOOLS) run -T
TERRAFORM = $(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) terraform terraform
TERRAFORM_ISOLATED = $(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) -e TF_DATA_DIR=/tmp/geoguessme-terraform terraform sh -ec
TOOLS_USER := --user $(shell id -u):$(shell id -g)
# Cleanup targets may need to remove artifacts created by older root-running
# containers. The paths are explicit allowlisted build/test directories.
ARTIFACTS_USER := --user 0:0
GEOGUESSME_TEST_WEB_PORT ?= 18080
GEOGUESSME_TEST_MAILPIT_PORT ?= 18025
TEST_BASE_URL := http://localhost:$(GEOGUESSME_TEST_WEB_PORT)
TEST_ENV := GEOGUESSME_TEST_WEB_PORT=$(GEOGUESSME_TEST_WEB_PORT) GEOGUESSME_TEST_MAILPIT_PORT=$(GEOGUESSME_TEST_MAILPIT_PORT) GEOGUESSME_TEST_PUBLIC_URL=$(TEST_BASE_URL) MAILPIT_BASE_URL=http://localhost:$(GEOGUESSME_TEST_MAILPIT_PORT)

# Optional Docker build flags for CI cache integration (type=local or type=gha).
# Unset locally so that builds use the default Docker daemon cache.
DOCKER_BUILD_FLAGS ?=
DOCKER_COMPOSE_BUILD_FLAGS ?=

##@ Setup
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0,5) }' $(MAKEFILE_LIST)

bootstrap: ## Build/pull pinned tools, fill locked caches, install hooks, and self-test.
	@# frontend/node_modules is gitignored, so a fresh checkout lacks the host
	@# mountpoint that the read-only workspace bind mount needs for the
	@# geoguessme-tools_frontend-node-modules named volume. Create the stub so
	@# the node-tools and playwright services can start on a clean checkout.
	@mkdir -p frontend/node_modules
	$(COMPOSE_TOOLS) build go-tools go-security node-tools
	$(COMPOSE_TOOLS) pull playwright shellcheck shfmt hadolint actionlint sqlfluff caddy cloudflared
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools sh -c 'npm ci --prefix /workspace/frontend --cache /npm-cache && chown -R $(shell id -u):$(shell id -g) /workspace/frontend/node_modules /npm-cache'
	$(MAKE) hooks-install
	$(MAKE) hooks-check
	$(MAKE) tools-self-test

bootstrap-integration: ## Prepare only the Go tools needed by backend integration CI.
	@mkdir -p frontend/node_modules
	$(COMPOSE_TOOLS) build go-tools

bootstrap-e2e: ## Prepare only the Node and Playwright tools needed by E2E CI.
	@mkdir -p frontend/node_modules
	$(COMPOSE_TOOLS) build node-tools
	$(COMPOSE_TOOLS) pull playwright
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools sh -c 'npm ci --prefix /workspace/frontend --cache /npm-cache && chown -R $(shell id -u):$(shell id -g) /workspace/frontend/node_modules /npm-cache'

hooks-install: ## Configure Git to use the tracked .githooks directory.
	git config core.hooksPath .githooks

hooks-check: ## Verify tracked hooks, Docker prerequisites, and canonical targets.
	@test "$$(git config --get core.hooksPath)" = ".githooks" || { echo "core.hooksPath must be .githooks"; exit 1; }
	@test -x .githooks/pre-commit && test -x .githooks/pre-push
	@command -v docker >/dev/null
	@docker compose version >/dev/null
	@grep -q 'make pre-commit' .githooks/pre-commit
	@grep -q 'make pre-push' .githooks/pre-push
	@echo "hooks-check PASSED"

tools-self-test: ## Run a short self-test inside each tool image.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'go version && goimports </dev/null >/dev/null && golangci-lint version'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security sh -c 'go version && govulncheck -version && psql --version && gcc --version'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'node --version && npm --version && prettier --version && eslint --version && tsc --version'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps playwright node --version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps shellcheck shellcheck --version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps shfmt shfmt --version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps hadolint hadolint --version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps actionlint actionlint -version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sqlfluff sqlfluff --version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps caddy caddy version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps cloudflared version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps terraform terraform version
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sops sops --version
	tools/quality/test/check-tool-image-split.sh

tools-clean: ## Remove only project-specific tool containers, networks, and caches.
	$(COMPOSE_TOOLS) down --volumes --remove-orphans

##@ Development
dev: ## Start the Docker development stack.
	$(COMPOSE_DEV) up -d --build --renew-anon-volumes

up: dev ## Alias for dev.

down: ## Stop the development stack and keep named application volumes.
	$(COMPOSE_DEV) down

restart: ## Restart development services.
	$(COMPOSE_DEV) restart

status: ## Show development service status.
	$(COMPOSE_DEV) ps

logs: ## Tail development logs.
	$(COMPOSE_DEV) logs -f

logs-backend: ## Tail backend logs.
	$(COMPOSE_DEV) logs -f backend

logs-frontend: ## Tail frontend logs.
	$(COMPOSE_DEV) logs -f frontend

##@ Code quality
format: ## Format tracked source/configuration files in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'git ls-files -z "*.go" | xargs -0 -r gofmt -w && git ls-files -z "*.go" | xargs -0 -r goimports -w'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write bash -c 'git ls-files -z | while IFS= read -r -d "" f; do case "$$f" in *.ts|*.tsx|*.js|*.jsx|*.css|*.html|*.json|*.md|*.yaml|*.yml) if [ -f "$$f" ]; then printf "%s\\0" "$$f"; fi;; esac; done | xargs -0 -r prettier --write'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) sqlfluff-write sqlfluff fix --dialect postgres backend/internal/database/migrations
	git ls-files -z '*.sh' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) shfmt-write shfmt -w -i 4 -ci

fmt: format ## Compatibility alias for format.

format-check: ## Check formatting without rewriting files.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'test -z "$$(git ls-files -z "*.go" | xargs -0 -r gofmt -l)" && test -z "$$(git ls-files -z "*.go" | xargs -0 -r goimports -l)"'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'git ls-files -z | while IFS= read -r -d "" f; do case "$$f" in *.ts|*.tsx|*.js|*.jsx|*.css|*.html|*.json|*.md|*.yaml|*.yml) if [ -f "$$f" ]; then printf "%s\\0" "$$f"; fi;; esac; done | xargs -0 -r prettier --check'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sqlfluff sqlfluff lint --dialect postgres backend/internal/database/migrations
	git ls-files -z '*.sh' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps shfmt shfmt -d -i 4 -ci

fmt-check: format-check ## Compatibility alias for format-check.

lint-go: ## Run strict Go analyzers.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && golangci-lint run ./...'

lint-frontend: ## Run ESLint with zero warnings.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend run lint -- --max-warnings=0

lint-css: ## Run Stylelint.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'cd frontend && stylelint --config /workspace/.stylelintrc.json --config-basedir /workspace/frontend "src/**/*.css"'

lint-docs: ## Run Markdownlint.
	git ls-files -z '*.md' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools markdownlint

lint-shell: ## Run ShellCheck on every tracked shell script.
	find . -type f -name '*.sh' -not -path './.git/*' -not -path './frontend/node_modules/*' -not -path './frontend/coverage/*' -print0 | sort -z | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps shellcheck shellcheck -x

lint-docker: ## Run Hadolint on every Dockerfile.
	find . -type f \( -name 'Dockerfile' -o -name 'Dockerfile.*' -o -name '*.Dockerfile' \) -not -path './.git/*' -not -path './frontend/node_modules/*' -print0 | sort -z | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps hadolint hadolint

lint-actions: ## Run actionlint on tracked workflows.
	git ls-files -z '.github/workflows/*.yml' '.github/workflows/*.yaml' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps actionlint actionlint

lint-sql: ## Run SQLFluff against migrations.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sqlfluff sqlfluff lint --dialect postgres backend/internal/database/migrations

lint-caddy: ## Validate and format-check Caddy configuration.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps caddy caddy validate --config /workspace/deployment/caddy/Caddyfile
	$(COMPOSE_TOOLS_RUN) --rm --no-deps caddy caddy fmt --diff /workspace/deployment/caddy/Caddyfile

lint-openapi: ## Validate the split OpenAPI contract with Redocly.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend exec -- redocly lint /workspace/docs/openapi.yaml

check-e2e-style: ## Reject synchronization and selector patterns that hide flakiness.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c '! find frontend/e2e -type f -name "*.ts" -print0 | xargs -0 -r grep -nE "waitForTimeout|networkidle|\.last\(\)|nth-child|\.nth\("'

lint: structure-check format-check lint-go lint-frontend lint-css lint-docs lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style ## Run every strict lint gate.

# Git worktree directories so structure-check can resolve .git inside containers.
GEOGUESSME_GIT_COMMON_DIR := $(abspath $(shell git rev-parse --git-common-dir 2>/dev/null))
GIT_DIR_WORKTREE := $(abspath $(shell git rev-parse --git-dir 2>/dev/null))
export GEOGUESSME_GIT_COMMON_DIR GIT_DIR_WORKTREE
ARGS ?=

structure-check: ## Enforce tracked-file and directory structure limits.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		$(if $(GIT_DIR_WORKTREE),-v $(GIT_DIR_WORKTREE):$(GIT_DIR_WORKTREE):ro) \
		go-tools /workspace/tools/quality/structure-check

type-check: ## Run the TypeScript compiler without emitting files.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'cd frontend && tsc --noEmit'

test-unit: test-backend test-frontend ## Run backend and frontend unit tests.

test-backend: ## Run Go unit tests, excluding live integration tests.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && go test $$(go list ./... | grep -v /integration_test)'

test-structure-regression: ## Run structure-check regression tests in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		$(if $(GIT_DIR_WORKTREE),-v $(GIT_DIR_WORKTREE):$(GIT_DIR_WORKTREE):ro) \
		go-tools /workspace/tools/quality/test/check-structure-regression.sh

test-ci-retention-regression: ## Verify CI workflow has bounded retention and cache scopes.
	bash tools/quality/test/check-ci-retention-regression.sh

test-cache-status-regression: ## Run cache-status regression tests.
	bash tools/quality/test/check-cache-status-regression.sh

test-ci-classifier: ## Verify deterministic CI path classification.
	bash tools/quality/ci/test-classify-changes.sh

test-e2e-regression: ## Verify E2E artifact, argument, and browser-selection safeguards.
	bash tools/quality/test/check-e2e-regression.sh

test-dev-workflow-regression: ## Verify dev rebuilds refresh anonymous dependency volumes.
	bash tools/quality/test/check-dev-workflow-regression.sh

test-restart-regression: ## Run restart-rehearsal regression tests.
	bash tools/quality/test/check-restart-regression.sh

test-migration-fixture-regression: ## Run migration fixture regression tests.
	bash tools/quality/test/check-migration-fixture-regression.sh

cache-status: ## Report project-only Docker images, build cache, volumes, and artifacts (read-only).
	bash tools/quality/cache-status.sh

test-frontend: ## Run frontend unit tests.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend test -- --run

test-race: ## Run Go unit tests with the race detector.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security sh -c 'cd backend && go test -race $$(go list ./... | grep -v /integration_test)'

test-backend-race: test-race ## Compatibility alias for test-race.

test-integration: build-images ## Run the isolated integration stack and tests in Docker.
	$(TEST_ENV) tools/quality/run-integration.sh

test-e2e: build-images ## Run all Playwright projects in the isolated stack.
	$(TEST_ENV) tools/quality/run-e2e.sh

test-e2e-pr: build-images ## Run the Chromium PR browser suite in the isolated stack.
	$(TEST_ENV) GEOGUESSME_E2E_PROJECTS=desktop tools/quality/run-e2e.sh

test-e2e-ui: build-images ## Run Playwright UI mode in Docker.
	$(TEST_ENV) GEOGUESSME_TEST_PROJECT=geoguessme-e2e-ui tools/quality/run-e2e.sh --ui

test-e2e-repeat: build-images ## Run E2E suite COUNT times to catch flakes. Usage: make test-e2e-repeat COUNT=5 (range 1..20)
	@case "$(COUNT)" in ''|*[!0-9]*) echo "COUNT must be an integer in 1..20"; exit 2;; esac
	@if [ "$(COUNT)" -lt 1 ] || [ "$(COUNT)" -gt 20 ]; then echo "COUNT must be in 1..20"; exit 2; fi
	@i=1; while [ $$i -le $(COUNT) ]; do \
		echo "=== E2E run $$i of $(COUNT) ==="; \
		$(TEST_ENV) tools/quality/run-e2e.sh || { echo "E2E run $$i failed"; exit 1; }; \
		i=$$((i+1)); \
	done
	@echo "All $(COUNT) E2E runs passed"

test-all: build-images test-unit test-integration test-e2e ## Run unit, integration, and E2E suites.

coverage: ## Enforce and report backend/frontend coverage thresholds.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && go test -coverprofile=/tmp/backend-coverage.out $$(go list ./... | grep -v /integration_test) 2>&1 | tee /tmp/backend-test-output.txt && go tool cover -func=/tmp/backend-coverage.out | tee -a /tmp/backend-test-output.txt && /workspace/tools/quality/coverage-threshold < /tmp/backend-test-output.txt'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools-write npm --prefix /workspace/frontend test -- --run --coverage

audit: ## Run dependency vulnerability audits in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security sh -c 'cd backend && govulncheck ./...'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend audit --audit-level=high

deps-go-security-update: ## Update vulnerable Go security modules and normalize metadata.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'cd backend && GOPATH=/tmp/go GOCACHE=/tmp/go-build-cache go get golang.org/x/crypto@v0.54.0 golang.org/x/text@v0.40.0 && GOPATH=/tmp/go GOCACHE=/tmp/go-build-cache go mod tidy'

deps-npm-security-update: ## Apply compatible npm security fixes to the frontend lockfile.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write npm --prefix /workspace/frontend --cache /tmp/npm-cache audit fix --package-lock-only

##@ Build
build: build-frontend build-backend ## Build production frontend and backend artifacts in Docker.

build-backend: ## Build the backend binary in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'cd backend && go build -trimpath -o bin/geoguessme .'

build-frontend: ## Build the frontend bundle in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write npm --prefix /workspace/frontend run build

build-images: ## Build production images with normal Docker layer caching.
	docker build --pull $(DOCKER_BUILD_FLAGS) -f deployment/docker/backend.Dockerfile -t geoguessme-backend:local .
	docker build --pull $(DOCKER_BUILD_FLAGS) -f deployment/docker/frontend.Dockerfile -t geoguessme-web:local .

clean-build: ## Build production images from scratch without any layer cache.
	docker build --pull --no-cache $(DOCKER_BUILD_FLAGS) -f deployment/docker/backend.Dockerfile -t geoguessme-backend:local .
	docker build --pull --no-cache $(DOCKER_BUILD_FLAGS) -f deployment/docker/frontend.Dockerfile -t geoguessme-web:local .

build-cache-prune: ## Remove dangling build cache to prevent unbounded growth.
	docker builder prune --force

test-build-caching: ## Run build-caching regression self-tests.
	tools/quality/test/check-build-caching.sh

##@ Database and deployment
compose-validate: ## Validate every Compose file.
	docker compose -f deployment/compose.dev.yaml --project-directory . config --quiet
	docker compose -f deployment/compose.test.yaml --project-directory . config --quiet
	BACKEND_IMAGE=geoguessme-backend:local WEB_IMAGE=geoguessme-web:local docker compose -f deployment/compose.production.yaml --project-directory . config --quiet
	COMPOSE_PROJECT_NAME=geoguessme-dev GEOGUESSME_ENV_FILE=deployment/env/dev.env.example GEOGUESSME_WEB_PORT=8082 BACKEND_IMAGE=geoguessme-backend:local WEB_IMAGE=geoguessme-web:local docker compose -f deployment/compose.production.yaml -f deployment/compose.hosted.yaml --project-directory . config --quiet
	docker compose -f deployment/compose.tools.yaml --project-directory . config --quiet

migrate-up: ## Apply pending migrations through the backend container.
	$(COMPOSE_DEV) run --rm backend migrate up

migrate-status: ## Show migration status through the backend container.
	$(COMPOSE_DEV) run --rm backend migrate status

migration-new: ## Create a migration file after checking NAME.
	@test -n "$(NAME)" || { echo "usage: make migration-new NAME=description"; exit 2; }
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'latest=$$(find backend/internal/database/migrations -name "*.sql" -printf "%f\n" | sed "s/^0*\([0-9]*\)_.*/\1/" | sort -n | tail -1); next=$$((10#$${latest:-0}+1)); file=$$(printf "backend/internal/database/migrations/%03d_%s.sql" $$next "$(NAME)"); printf -- "-- %03d %s\n" $$next "$(NAME)" > "$$file"; echo "created $$file"'

db-backup: ## Create a PostgreSQL backup through the tool container.
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 2; }
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) -e DATABASE_URL="$(DATABASE_URL)" -e BACKUP_DIR=/workspace/backups go-security /workspace/deployment/scripts/backup-postgres.sh

db-restore: ## Restore a PostgreSQL backup through the tool container.
	@test -n "$(FILE)" || { echo "usage: make db-restore FILE=backups/file.sql.gz"; exit 2; }
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 2; }
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) -e DATABASE_URL="$(DATABASE_URL)" go-security /workspace/deployment/scripts/restore-postgres.sh "$(FILE)"

backup-rehearsal: build-images ## Run the disposable backup/restore rehearsal.
	deployment/scripts/backup-restore-rehearsal.sh

restore-rehearsal: backup-rehearsal ## Compatibility alias for restore rehearsal.

restart-rehearsal: build-images ## Run the disposable restart/reconnect rehearsal.
	deployment/scripts/restart-rehearsal.sh

reconnect-rehearsal: build-images ## Run the load/reconnect/catch-up rehearsal with exact-once evidence.
	deployment/scripts/reconnect-rehearsal.sh

migration-test: build-images ## Run concurrent, idempotent, and legacy-fixture migration tests.
	deployment/scripts/migration-concurrency.sh || deployment/scripts/migration-concurrency.sh

load-test: build-images ## Run the documented disposable load profile.
	deployment/scripts/load-test.sh

container-verify: build-images ## Verify runtime image hardening and health checks.
	deployment/scripts/container-verify.sh

prod-container-verify: build-images ## Full production-container verification: images, compose, stack, health, smoke, teardown.
	deployment/scripts/prod-container-verify.sh

prod-config: ## Validate production image and secret configuration.
	@test -n "$$BACKEND_IMAGE" || { echo "BACKEND_IMAGE is required"; exit 2; }
	@test -n "$$WEB_IMAGE" || { echo "WEB_IMAGE is required"; exit 2; }
	@case "$$BACKEND_IMAGE" in *@sha256:*) ;; *) echo "BACKEND_IMAGE must include an immutable @sha256 digest"; exit 2;; esac
	@case "$$WEB_IMAGE" in *@sha256:*) ;; *) echo "WEB_IMAGE must include an immutable @sha256 digest"; exit 2;; esac
	@test -f deployment/env/production.env || { echo "deployment/env/production.env is required"; exit 2; }
	@echo "production configuration OK"

prod-migrate: prod-config ## Run the production migration job.
	$(COMPOSE_PROD) run --rm migration migrate up

prod-up: prod-config ## Start the production stack.
	$(COMPOSE_PROD) up -d

prod-down: ## Stop production services and keep data volumes.
	$(COMPOSE_PROD) down

prod-logs: ## Tail production logs.
	$(COMPOSE_PROD) logs -f

hosted-config: ## Validate production and dev hosted Compose expansion.
	COMPOSE_PROJECT_NAME=geoguessme-production GEOGUESSME_ENV_FILE=deployment/env/production.env.example GEOGUESSME_WEB_PORT=8081 BACKEND_IMAGE=example.invalid/geoguessme-backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa WEB_IMAGE=example.invalid/geoguessme-web@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb docker compose -f deployment/compose.production.yaml -f deployment/compose.hosted.yaml --project-directory . config --quiet
	COMPOSE_PROJECT_NAME=geoguessme-dev GEOGUESSME_ENV_FILE=deployment/env/dev.env.example GEOGUESSME_WEB_PORT=8082 GEOGUESSME_BACKEND_MEMORY=512M GEOGUESSME_DATABASE_MEMORY=768M BACKEND_IMAGE=example.invalid/geoguessme-backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa WEB_IMAGE=example.invalid/geoguessme-web@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb docker compose -f deployment/compose.production.yaml -f deployment/compose.hosted.yaml --project-directory . config --quiet

hosted-contract-test: ## Verify deployment ordering, isolation, locking, rollback, and header contracts.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/contracts.sh

cloudflared-access-ssh: ## Proxy SSH through Access; requires HOST and service-token env vars.
	@test -n "$(HOST)" || { echo 'HOST is required' >&2; exit 2; }
	@test -n "$${TUNNEL_SERVICE_TOKEN_ID:-}" || { echo 'TUNNEL_SERVICE_TOKEN_ID is required' >&2; exit 2; }
	@test -n "$${TUNNEL_SERVICE_TOKEN_SECRET:-}" || { echo 'TUNNEL_SERVICE_TOKEN_SECRET is required' >&2; exit 2; }
	@$(COMPOSE_TOOLS_RUN) --rm --no-deps cloudflared access ssh --hostname "$(HOST)"

terraform-fmt: ## Format infrastructure code in the pinned Terraform container.
	$(TERRAFORM) fmt -recursive

terraform-fmt-check: ## Check infrastructure formatting in the pinned Terraform container.
	$(TERRAFORM) fmt -check -recursive

terraform-init: ## Initialize the R2 backend; requires infra/terraform/backend.hcl.
	@test -f infra/terraform/backend.hcl || { echo 'copy backend.hcl.example to backend.hcl and fill it first'; exit 2; }
	$(TERRAFORM) init -backend-config=backend.hcl

terraform-validate: ## Initialize without remote state and validate Terraform.
	$(TERRAFORM_ISOLATED) 'terraform init -backend=false && terraform validate'

terraform-test: ## Exercise a fresh, mocked infrastructure plan and assertions.
	$(TERRAFORM_ISOLATED) 'terraform init -backend=false && terraform validate && terraform test'

terraform-plan: terraform-init ## Create a reviewed infrastructure plan.
	$(TERRAFORM) plan -out=geoguessme.tfplan

terraform-apply: ## Apply the exact reviewed plan; requires CONFIRM=apply.
	@test "$(CONFIRM)" = apply || { echo 'Refusing without CONFIRM=apply'; exit 2; }
	@test -f infra/terraform/geoguessme.tfplan || { echo 'run make terraform-plan first'; exit 2; }
	$(TERRAFORM) apply geoguessme.tfplan

secrets-encrypt: ## Encrypt ENV=dev|production from its example using RECIPIENT.
	@case "$(ENV)" in dev|production) ;; *) echo 'ENV must be dev or production'; exit 2;; esac
	@test -n "$(RECIPIENT)" || { echo 'RECIPIENT is required'; exit 2; }
	cp deployment/env/$(ENV).env.example deployment/secrets/$(ENV).env.enc
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sops sops --encrypt --input-type dotenv --output-type dotenv --age "$(RECIPIENT)" --in-place /workspace/deployment/secrets/$(ENV).env.enc

secrets-generate: ## Generate and SOPS-encrypt ENV=dev|production without a plaintext file.
	@case "$(ENV)" in dev|production) ;; *) echo 'ENV must be dev or production'; exit 2;; esac
	@test -n "$(RECIPIENT)" || { echo 'RECIPIENT is required'; exit 2; }
	@mkdir -p deployment/secrets
	@temporary=$$(mktemp deployment/secrets/.$(ENV).env.enc.XXXXXX); \
	trap 'rm -f "$$temporary"' EXIT INT TERM; \
	bash -o pipefail -c '$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) \
		-e TARGET_ENV=$(ENV) -e BREVO_SMTP_USERNAME -e BREVO_SMTP_PASSWORD \
		-e GHCR_USERNAME -e GHCR_TOKEN -e MEDIA_ACCESS_KEY_ID -e MEDIA_SECRET_ACCESS_KEY \
		-e BACKUP_ACCESS_KEY_ID -e BACKUP_SECRET_ACCESS_KEY -e CLOUDFLARE_ACCOUNT_ID \
		go-tools sh /workspace/deployment/scripts/generate-hosted-secret.sh | \
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sops sops --config /dev/null --encrypt \
		--input-type dotenv --output-type dotenv --age "$(RECIPIENT)" /dev/stdin' \
		>"$$temporary"; \
	test -s "$$temporary"; \
	chmod 0600 "$$temporary"; \
	mv "$$temporary" deployment/secrets/$(ENV).env.enc; \
	trap - EXIT INT TERM

smoke: build-images ## Run the smoke test against a selected disposable/staging URL.
	if [ -n "$${BASE_URL:-}" ]; then deployment/scripts/smoke-test.sh "$$BASE_URL"; else deployment/scripts/smoke-rehearsal.sh; fi

smoke-rehearsal: build-images ## Run the smoke test against a disposable test stack.
	deployment/scripts/smoke-rehearsal.sh

##@ Gates
preflight: structure-check format-check lint test-structure-regression test-ci-classifier test-e2e-regression test-dev-workflow-regression hosted-contract-test terraform-fmt-check terraform-test type-check audit test-unit compose-validate ## Run the fast local and pull-request gate.

preflight-docs: structure-check format-check lint-docs test-ci-classifier ## Run the documentation-only pull-request gate.

pr-backend: test-integration ## Run backend live-stack checks selected by CI.

pr-frontend: test-e2e-pr ## Run the Chromium E2E checks selected by CI.

quality: structure-check format-check lint test-structure-regression test-ci-retention-regression test-e2e-regression test-dev-workflow-regression test-prod-container-verify-regression test-migration-fixture-regression test-artifacts-clean-regression hosted-contract-test terraform-fmt-check terraform-test type-check audit test-unit test-race coverage build-images compose-validate ## Run all local quality gates.

verify: quality test-integration test-e2e container-verify compose-validate prod-container-verify migration-test backup-rehearsal restart-rehearsal reconnect-rehearsal test-restart-regression test-artifacts-clean-regression smoke load-test ## Run the complete release gate.

pre-commit: ## Run the strict Dockerized commit gate.
	tools/quality/pre-commit.sh

pre-push: ## Run the fast deterministic gate before pushing.
	$(MAKE) preflight

ci: ## Run the fast deterministic pull-request gate.
	$(MAKE) preflight

##@ Maintenance
prune-report: ## Preview project-scoped Docker prune (dry-run, no changes).
	PROJECT_PREFIX=geoguessme bash tools/quality/prune.sh --dry-run --include-build-cache

prune: ## Prune project-scoped Docker artifacts and cache; requires CONFIRM=prune.
	@test "$(CONFIRM)" = "prune" || { echo 'Refusing to prune without CONFIRM=prune. Use make prune-report to preview, then CONFIRM=prune make prune to execute.' >&2; exit 2; }
	PROJECT_PREFIX=geoguessme CONFIRM=prune bash tools/quality/prune.sh --force $(ARGS)

test-prune-regression: ## Run prune.sh regression tests.
	bash tools/quality/test/check-prune-regression.sh

disk-cleanup-report: ## Preview project disk cleanup (dry-run, no changes).
	bash tools/quality/disk-cleanup.sh --dry-run

disk-cleanup: ## Clean project disk artifacts; requires CONFIRM=disk-cleanup.
	@test "$(CONFIRM)" = "disk-cleanup" || { echo 'Refusing to clean without CONFIRM=disk-cleanup. Use make disk-cleanup-report to preview, then CONFIRM=disk-cleanup make disk-cleanup to execute.' >&2; exit 2; }
	CONFIRM=disk-cleanup bash tools/quality/disk-cleanup.sh --force $(ARGS)

test-disk-cleanup-regression: ## Run disk-cleanup.sh regression tests.
	bash tools/quality/test/check-disk-cleanup-regression.sh

test-prod-container-verify-regression: ## Run prod-container-verify.sh regression tests.
	bash tools/quality/test/check-prod-container-verify-regression.sh

test-artifacts-clean-regression: ## Verify artifacts-clean target structure and safety.
	bash tools/quality/test/check-cache-status-regression.sh

artifacts-clean: ## Remove generated workspace artifacts without touching Docker caches or volumes.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(ARTIFACTS_USER) go-tools-write sh -c 'rm -rf backend/bin backend/tmp backend/coverage.out'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(ARTIFACTS_USER) node-tools-write sh -c 'rm -rf frontend/dist frontend/coverage frontend/test-results frontend/playwright-report frontend/blob-report'

clean: build-cache-prune ## Remove generated artifacts and build cache without touching Docker/application volumes.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(ARTIFACTS_USER) go-tools-write sh -c 'rm -rf backend/bin backend/tmp backend/coverage.out'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(ARTIFACTS_USER) node-tools-write sh -c 'rm -rf frontend/dist frontend/coverage frontend/test-results frontend/playwright-report frontend/blob-report'

reset-dev: ## Delete development volumes; requires CONFIRM=reset-dev.
ifeq ($(CONFIRM),reset-dev)
	$(COMPOSE_DEV) down -v --remove-orphans
else
	@echo "This deletes development database and media volumes. Re-run with CONFIRM=reset-dev."
	@exit 2
endif
