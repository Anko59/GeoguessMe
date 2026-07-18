.DEFAULT_GOAL := help

.PHONY: help bootstrap hooks-install hooks-check tools-clean tools-self-test \
	dev up down restart status logs logs-backend logs-frontend \
	format format-check fmt fmt-check lint lint-go lint-frontend lint-css lint-docs \
	lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style \
	type-check test-unit test-backend test-frontend test-race test-backend-race test-structure-regression \
	test-integration test-e2e test-e2e-ui test-e2e-repeat test-all coverage audit \
	build build-backend build-frontend build-images \
	migrate-up migrate-status migration-new db-backup db-restore \
	backup-rehearsal restore-rehearsal restart-rehearsal migration-test load-test \
	compose-validate container-verify smoke smoke-rehearsal \
	prod-config prod-migrate prod-up prod-down prod-logs \
	quality verify pre-commit pre-push ci clean reset-dev

COMPOSE_DEV  := docker compose -p geoguessme-dev -f deployment/compose.dev.yaml --project-directory .
COMPOSE_TEST := docker compose -f deployment/compose.test.yaml --project-directory .
COMPOSE_PROD := docker compose -p geoguessme-prod -f deployment/compose.production.yaml --project-directory .
COMPOSE_TOOLS := docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory .
TOOLS_USER := --user $(shell id -u):$(shell id -g)
GEOGUESSME_TEST_WEB_PORT ?= 8080
GEOGUESSME_TEST_MAILPIT_PORT ?= 8025
TEST_BASE_URL := http://localhost:$(GEOGUESSME_TEST_WEB_PORT)
TEST_ENV := GEOGUESSME_TEST_WEB_PORT=$(GEOGUESSME_TEST_WEB_PORT) GEOGUESSME_TEST_MAILPIT_PORT=$(GEOGUESSME_TEST_MAILPIT_PORT) GEOGUESSME_TEST_PUBLIC_URL=$(TEST_BASE_URL) MAILPIT_BASE_URL=http://localhost:$(GEOGUESSME_TEST_MAILPIT_PORT)

##@ Setup
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0,5) }' $(MAKEFILE_LIST)

bootstrap: ## Build/pull pinned tools, fill locked caches, install hooks, and self-test.
	$(COMPOSE_TOOLS) build go-tools node-tools
	$(COMPOSE_TOOLS) pull playwright shellcheck shfmt hadolint actionlint sqlfluff caddy
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools npm ci --prefix /workspace/frontend --cache /npm-cache
	$(MAKE) hooks-install
	$(MAKE) hooks-check
	$(MAKE) tools-self-test

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
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'go version && goimports </dev/null >/dev/null && golangci-lint version && govulncheck -version'
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools bash -c 'node --version && npm --version && prettier --version && eslint --version && tsc --version'
	$(COMPOSE_TOOLS) run --rm --no-deps playwright node --version
	$(COMPOSE_TOOLS) run --rm --no-deps shellcheck shellcheck --version
	$(COMPOSE_TOOLS) run --rm --no-deps shfmt shfmt --version
	$(COMPOSE_TOOLS) run --rm --no-deps hadolint hadolint --version
	$(COMPOSE_TOOLS) run --rm --no-deps actionlint actionlint -version
	$(COMPOSE_TOOLS) run --rm --no-deps sqlfluff sqlfluff --version
	$(COMPOSE_TOOLS) run --rm --no-deps caddy caddy version

tools-clean: ## Remove only project-specific tool containers, networks, and caches.
	$(COMPOSE_TOOLS) down --volumes --remove-orphans

##@ Development
dev: ## Start the Docker development stack.
	$(COMPOSE_DEV) up -d --build

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
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'git ls-files -z "*.go" | xargs -0 -r gofmt -w'
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) node-tools-write bash -c 'git ls-files -z | while IFS= read -r -d "" f; do case "$$f" in *.ts|*.tsx|*.js|*.jsx|*.css|*.json|*.md|*.yaml|*.yml) if [ -f "$$f" ]; then printf "%s\\0" "$$f"; fi;; esac; done | xargs -0 -r prettier --write'
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) shfmt-write shfmt -w -i 4 -ci $$(git ls-files '*.sh' | while IFS= read -r f; do test -f "$$f" && printf '%s ' "$$f"; done)

fmt: format ## Compatibility alias for format.

format-check: ## Check formatting without rewriting files.
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'test -z "$$(git ls-files -z "*.go" | xargs -0 -r gofmt -l)"'
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools bash -c 'git ls-files -z | while IFS= read -r -d "" f; do case "$$f" in *.ts|*.tsx|*.js|*.jsx|*.css|*.json|*.md|*.yaml|*.yml) if [ -f "$$f" ]; then printf "%s\\0" "$$f"; fi;; esac; done | xargs -0 -r prettier --check'
	$(COMPOSE_TOOLS) run --rm --no-deps shfmt shfmt -d -i 4 -ci $$(git ls-files '*.sh' | while IFS= read -r f; do test -f "$$f" && printf '%s ' "$$f"; done)

fmt-check: format-check ## Compatibility alias for format-check.

lint-go: ## Run strict Go analyzers.
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'cd backend && golangci-lint run ./...'

lint-frontend: ## Run ESLint with zero warnings.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools npm --prefix /workspace/frontend run lint -- --max-warnings=0

lint-css: ## Run Stylelint.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools bash -c 'cd frontend && stylelint --config /workspace/.stylelintrc.json --config-basedir /workspace/frontend "src/**/*.css"'

lint-docs: ## Run Markdownlint.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools markdownlint README.md CONTRIBUTING.md AGENTS.md docs/*.md deployment/*.md

lint-shell: ## Run ShellCheck on every tracked shell script.
	$(COMPOSE_TOOLS) run --rm --no-deps shellcheck shellcheck -x $$(find . -type f -name '*.sh' -not -path './.git/*' -not -path './frontend/node_modules/*' -not -path './frontend/coverage/*' -print | sort)

lint-docker: ## Run Hadolint on every Dockerfile.
	$(COMPOSE_TOOLS) run --rm --no-deps hadolint hadolint $$(find . -type f \( -name 'Dockerfile' -o -name 'Dockerfile.*' -o -name '*.Dockerfile' \) -not -path './.git/*' -not -path './frontend/node_modules/*' -print | sort)

lint-actions: ## Run actionlint on tracked workflows.
	$(COMPOSE_TOOLS) run --rm --no-deps actionlint actionlint $$(git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml' | while IFS= read -r f; do test -f "$$f" && printf '%s ' "$$f"; done)

lint-sql: ## Run SQLFluff against migrations.
	$(COMPOSE_TOOLS) run --rm --no-deps sqlfluff sqlfluff lint --dialect postgres backend/internal/database/migrations

lint-caddy: ## Validate and format-check Caddy configuration.
	$(COMPOSE_TOOLS) run --rm --no-deps caddy caddy validate --config /workspace/deployment/caddy/Caddyfile
	$(COMPOSE_TOOLS) run --rm --no-deps caddy caddy fmt --diff /workspace/deployment/caddy/Caddyfile

lint-openapi: ## Validate the split OpenAPI contract with Redocly.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools npm --prefix /workspace/frontend exec -- redocly lint /workspace/docs/openapi.yaml

check-e2e-style: ## Reject synchronization and selector patterns that hide flakiness.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools bash -c '! find frontend/e2e -type f -name "*.ts" -print0 | xargs -0 -r grep -nE "waitForTimeout|networkidle|\.last\(\)|nth-child|\.nth\("'

lint: structure-check format-check lint-go lint-frontend lint-css lint-docs lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style ## Run every strict lint gate.

# Git worktree directories so structure-check can resolve .git inside containers.
GIT_COMMON_DIR := $(abspath $(shell git rev-parse --git-common-dir 2>/dev/null))
GIT_DIR_WORKTREE := $(abspath $(shell git rev-parse --git-dir 2>/dev/null))

structure-check: ## Enforce tracked-file and directory structure limits.
	$(COMPOSE_TOOLS) run --rm --no-deps \
		$(if $(GIT_COMMON_DIR),-v $(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro) \
		$(if $(GIT_DIR_WORKTREE),-v $(GIT_DIR_WORKTREE):$(GIT_DIR_WORKTREE):ro) \
		go-tools /workspace/tools/quality/structure-check

type-check: ## Run the TypeScript compiler without emitting files.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools bash -c 'cd frontend && tsc --noEmit'

test-unit: test-backend test-frontend ## Run backend and frontend unit tests.

test-backend: ## Run Go unit tests, excluding live integration tests.
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'cd backend && go test $$(go list ./... | grep -v /integration_test)'

test-structure-regression: ## Run structure-check regression tests in Docker.
	$(COMPOSE_TOOLS) run --rm --no-deps \
		$(if $(GIT_COMMON_DIR),-v $(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro) \
		$(if $(GIT_DIR_WORKTREE),-v $(GIT_DIR_WORKTREE):$(GIT_DIR_WORKTREE):ro) \
		go-tools /workspace/tools/quality/test/check-structure-regression.sh

test-frontend: ## Run frontend unit tests.
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools npm --prefix /workspace/frontend test -- --run

test-race: ## Run Go unit tests with the race detector.
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'cd backend && go test -race $$(go list ./... | grep -v /integration_test)'

test-backend-race: test-race ## Compatibility alias for test-race.

test-integration: ## Run the isolated integration stack and tests in Docker.
	$(TEST_ENV) tools/quality/run-integration.sh

test-e2e: ## Run all Playwright projects in the isolated stack.
	$(TEST_ENV) tools/quality/run-e2e.sh

test-e2e-ui: ## Run Playwright UI mode in Docker.
	$(TEST_ENV) GEOGUESSME_TEST_PROJECT=geoguessme-e2e-ui tools/quality/run-e2e.sh --ui

test-e2e-repeat: ## Run E2E suite COUNT times to catch flakes. Usage: make test-e2e-repeat COUNT=5 (range 1..20)
	@case "$(COUNT)" in ''|*[!0-9]*) echo "COUNT must be an integer in 1..20"; exit 2;; esac
	@if [ "$(COUNT)" -lt 1 ] || [ "$(COUNT)" -gt 20 ]; then echo "COUNT must be in 1..20"; exit 2; fi
	@i=1; while [ $$i -le $(COUNT) ]; do \
		echo "=== E2E run $$i of $(COUNT) ==="; \
		$(TEST_ENV) tools/quality/run-e2e.sh || { echo "E2E run $$i failed"; exit 1; }; \
		i=$$((i+1)); \
	done
	@echo "All $(COUNT) E2E runs passed"

test-all: test-unit test-integration test-e2e ## Run unit, integration, and E2E suites.

coverage: ## Enforce and report backend/frontend coverage thresholds.
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'cd backend && go test -coverprofile=/tmp/backend-coverage.out $$(go list ./... | grep -v /integration_test) 2>&1 | tee /tmp/backend-test-output.txt && go tool cover -func=/tmp/backend-coverage.out | tee -a /tmp/backend-test-output.txt && /workspace/tools/quality/coverage-threshold < /tmp/backend-test-output.txt'
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools-write npm --prefix /workspace/frontend test -- --run --coverage

audit: ## Run dependency vulnerability audits in Docker.
	$(COMPOSE_TOOLS) run --rm --no-deps go-tools sh -c 'cd backend && govulncheck ./...'
	$(COMPOSE_TOOLS) run --rm --no-deps node-tools npm --prefix /workspace/frontend audit --audit-level=high

##@ Build
build: build-frontend build-backend ## Build production frontend and backend artifacts in Docker.

build-backend: ## Build the backend binary in Docker.
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'cd backend && go build -trimpath -o bin/geoguessme .'

build-frontend: ## Build the frontend bundle in Docker.
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) node-tools-write npm --prefix /workspace/frontend run build

build-images: ## Build production images without cache.
	docker build --pull --no-cache -f deployment/docker/backend.Dockerfile -t geoguessme-backend:local .
	docker build --pull --no-cache -f deployment/docker/frontend.Dockerfile -t geoguessme-web:local .

##@ Database and deployment
compose-validate: ## Validate every Compose file.
	docker compose -f deployment/compose.dev.yaml --project-directory . config --quiet
	docker compose -f deployment/compose.test.yaml --project-directory . config --quiet
	BACKEND_IMAGE=geoguessme-backend:local WEB_IMAGE=geoguessme-web:local docker compose -f deployment/compose.production.yaml --project-directory . config --quiet
	docker compose -f deployment/compose.tools.yaml --project-directory . config --quiet

migrate-up: ## Apply pending migrations through the backend container.
	$(COMPOSE_DEV) run --rm backend migrate up

migrate-status: ## Show migration status through the backend container.
	$(COMPOSE_DEV) run --rm backend migrate status

migration-new: ## Create a migration file after checking NAME.
	@test -n "$(NAME)" || { echo "usage: make migration-new NAME=description"; exit 2; }
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'latest=$$(find backend/internal/database/migrations -name "*.sql" -printf "%f\n" | sed "s/^0*\([0-9]*\)_.*/\1/" | sort -n | tail -1); next=$$((10#$${latest:-0}+1)); file=$$(printf "backend/internal/database/migrations/%03d_%s.sql" $$next "$(NAME)"); printf -- "-- %03d %s\n" $$next "$(NAME)" > "$$file"; echo "created $$file"'

db-backup: ## Create a PostgreSQL backup through the tool container.
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 2; }
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) -e DATABASE_URL="$(DATABASE_URL)" -e BACKUP_DIR=/workspace/backups go-tools /workspace/deployment/scripts/backup-postgres.sh

db-restore: ## Restore a PostgreSQL backup through the tool container.
	@test -n "$(FILE)" || { echo "usage: make db-restore FILE=backups/file.sql.gz"; exit 2; }
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 2; }
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) -e DATABASE_URL="$(DATABASE_URL)" go-tools /workspace/deployment/scripts/restore-postgres.sh "$(FILE)"

backup-rehearsal: ## Run the disposable backup/restore rehearsal.
	deployment/scripts/backup-restore-rehearsal.sh

restore-rehearsal: backup-rehearsal ## Compatibility alias for restore rehearsal.

restart-rehearsal: ## Run the disposable restart/reconnect rehearsal.
	deployment/scripts/restart-rehearsal.sh

migration-test: ## Run concurrent, idempotent, and legacy-fixture migration tests.
	deployment/scripts/migration-concurrency.sh

load-test: ## Run the documented disposable load profile.
	deployment/scripts/load-test.sh

container-verify: build-images ## Verify runtime image hardening and health checks.
	deployment/scripts/container-verify.sh

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

smoke: ## Run the smoke test against a selected disposable/staging URL.
	if [ -n "$${BASE_URL:-}" ]; then deployment/scripts/smoke-test.sh "$$BASE_URL"; else deployment/scripts/smoke-rehearsal.sh; fi

smoke-rehearsal: ## Run the smoke test against a disposable test stack.
	deployment/scripts/smoke-rehearsal.sh

##@ Gates
quality: structure-check format-check lint test-structure-regression type-check audit test-unit test-race coverage build-images compose-validate ## Run all local quality gates.

verify: quality test-integration test-e2e container-verify compose-validate migration-test backup-rehearsal restart-rehearsal smoke load-test ## Run the complete release gate.

pre-commit: ## Run the strict Dockerized commit gate.
	tools/quality/pre-commit.sh

pre-push: ## Run the complete verification gate before pushing.
	$(MAKE) verify

ci: ## Run the same Dockerized quality and verification targets used locally.
	$(MAKE) verify

##@ Maintenance
clean: ## Remove generated artifacts without touching Docker/application volumes.
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'rm -rf backend/bin backend/tmp backend/coverage.out'
	$(COMPOSE_TOOLS) run --rm --no-deps $(TOOLS_USER) node-tools-write sh -c 'rm -rf frontend/dist frontend/coverage frontend/test-results frontend/playwright-report frontend/blob-report'

reset-dev: ## Delete development volumes; requires CONFIRM=reset-dev.
ifeq ($(CONFIRM),reset-dev)
	$(COMPOSE_DEV) down -v --remove-orphans
else
	@echo "This deletes development database and media volumes. Re-run with CONFIRM=reset-dev."
	@exit 2
endif
