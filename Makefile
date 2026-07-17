.DEFAULT_GOAL := help
.PHONY: help bootstrap install-backend install-frontend \
	dev up down restart status logs logs-backend logs-frontend \
	fmt fmt-check lint vet audit \
	test test-backend test-backend-race test-frontend test-integration test-e2e test-e2e-ui test-all coverage \
	build build-backend build-frontend build-images ci \
	migrate-up migrate-status migration-new \
	db-backup db-restore \
	prod-config prod-migrate prod-up prod-down prod-logs smoke \
	clean reset-dev

GO            ?= go
NODE          ?= npm
BACKEND_PKGS  := $(shell cd backend && $(GO) list ./... | grep -v /integration_test)
COMPOSE_DEV   := docker compose -p geoguessme-dev -f deployment/compose.dev.yaml --project-directory .
COMPOSE_TEST  := docker compose -f deployment/compose.test.yaml --project-directory .
COMPOSE_PROD  := docker compose -p geoguessme-prod -f deployment/compose.production.yaml --project-directory .
TEST_BASE_URL ?= http://localhost:8080
GEOGUESSME_TEST_WEB_PORT    ?= 8080
GEOGUESSME_TEST_MAILPIT_PORT ?= 8025
TEST_ENV := GEOGUESSME_TEST_WEB_PORT=$(GEOGUESSME_TEST_WEB_PORT) GEOGUESSME_TEST_MAILPIT_PORT=$(GEOGUESSME_TEST_MAILPIT_PORT)

##@ Setup
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0,5) }' $(MAKEFILE_LIST)

bootstrap: install-backend install-frontend ## Install all dependencies. Sets up lefthook for commit gates.
	cd frontend && npx lefthook install 2>/dev/null || echo "lefthook not installed (optional)"

install-tools: ## Run the tool installation helper.
	tools/quality/install-tools.sh

install-backend: ## Download Go module dependencies.
	cd backend && $(GO) mod download

install-frontend: ## Install frontend dependencies (does not run inside lint).
	cd frontend && $(NODE) install
	cd frontend && $(NODE) exec playwright install --with-deps chromium

##@ Development
dev: ## Start the development stack (PostgreSQL, MinIO, Mailpit, hot reload).
	$(COMPOSE_DEV) up -d --build

up: ## Alias for dev.
	$(COMPOSE_DEV) up -d --build

down: ## Stop the development stack (keeps volumes).
	$(COMPOSE_DEV) down

restart: ## Restart the development services.
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
fmt: ## Format Go sources.
	cd backend && $(GO) fmt ./...

format: ## Auto-format Go (gofmt), TS/JS/JSON/CSS/MD/YAML (Prettier).
	cd backend && $(GO) fmt ./...
	cd frontend && npx prettier --write 'src/**/*.{ts,tsx,css}' 'e2e/**/*.ts' '*.ts' '*.json' 2>/dev/null || true

fmt-check: ## Fail if Go sources are not formatted (read-only).
	cd backend && test -z "$$(gofmt -l .)"

format-check: ## Fail if any supported file is not formatted (read-only).
	cd backend && test -z "$$(gofmt -l .)"
	cd frontend && npx prettier --check 'src/**/*.{ts,tsx,css,json}' 'e2e/**/*.ts' '*.ts' '*.json' 2>/dev/null || true

vet: ## Run go vet.
	cd backend && $(GO) vet ./...

lint: lint-frontend lint-css lint-docs lint-shell lint-docker lint-openapi ## Lint all available formats.

lint-frontend: ## Lint TypeScript with ESLint (zero warnings).
	cd frontend && $(NODE) run lint

lint-css: ## Lint CSS with Stylelint.
	cd frontend && npx stylelint 'src/**/*.css' 2>/dev/null || echo "stylelint: no issues or not available"

lint-docs: ## Lint Markdown.
	npx markdownlint '*.md' 'docs/*.md' 'deployment/*.md' 2>/dev/null || echo "markdownlint: no issues or not available"

lint-shell: ## Lint shell scripts.
	tools/quality/lint-shell.sh

lint-docker: ## Lint Dockerfiles.
	tools/quality/lint-docker.sh

lint-actions: ## Lint GitHub Actions workflows.
	tools/quality/lint-actions.sh

lint-sql: ## Lint SQL migrations.
	tools/quality/lint-sql.sh

lint-caddy: ## Lint Caddy configuration.
	tools/quality/lint-caddy.sh

lint-openapi: ## Validate OpenAPI schema.
	npx @redocly/cli lint docs/openapi.yaml 2>/dev/null || echo "redocly: skip"

##@ Tests
test: test-backend test-frontend ## Run backend unit and frontend unit tests.

test-backend: ## Run Go unit/package tests (excludes the live-server integration suite).
	cd backend && $(GO) test $(BACKEND_PKGS)

test-backend-race: ## Run Go unit tests with the race detector.
	cd backend && $(GO) test -race $(BACKEND_PKGS)

test-frontend: ## Run frontend unit tests.
	cd frontend && $(NODE) test -- --run

test-integration: ## Run backend integration tests against the isolated test stack.
	tools/quality/run-integration.sh

test-e2e: ## Run desktop + mobile Playwright suites against the isolated test stack.
	tools/quality/run-e2e.sh

test-e2e-ui: ## Run the Playwright UI mode against the isolated test stack.
	GEOGUESSME_TEST_PROJECT=geoguessme-e2e-ui tools/quality/run-e2e.sh --ui

test-all: test-backend test-frontend test-integration test-e2e ## Run unit, integration, and Playwright suites.

coverage: ## Generate a Go coverage report.
	cd backend && $(GO) test -coverprofile=coverage.out $(BACKEND_PKGS) && $(GO) tool cover -func=coverage.out | tail -1

##@ Build
build: build-frontend build-backend ## Build the frontend bundle and backend binary.

build-backend: ## Build the backend binary.
	cd backend && $(GO) build -o bin/geoguessme .

build-frontend: ## Build the frontend production bundle.
	cd frontend && $(NODE) run build

build-images: ## Build the production backend and web images.
	docker build -f deployment/docker/backend.Dockerfile -t geoguessme-backend:local .
	docker build -f deployment/docker/frontend.Dockerfile -t geoguessme-web:local .

##@ Database
migrate-up: ## Apply pending migrations against the configured DATABASE_URL.
	cd backend && $(GO) run . migrate up

migrate-status: ## Show applied and pending migrations.
	cd backend && $(GO) run . migrate status

migration-new: ## Create a new migration: make migration-new NAME=add_thing
	@test -n "$(NAME)" || { echo "usage: make migration-new NAME=description"; exit 2; }
	@latest=$$(ls backend/internal/database/migrations/*.sql 2>/dev/null | sed 's#.*/0*\([0-9]*\)_.*#\1#' | sort -n | tail -1); \
	 next=$$((10#$${latest:-0}+1)); file=$$(printf "backend/internal/database/migrations/%03d_%s.sql" $$next "$(NAME)"); \
	 printf -- "-- %03d %s\n" $$next "$(NAME)" > $$file; echo "created $$file"

db-backup: ## Back up the database. Set DATABASE_URL and optionally BACKUP_DIR.
	DATABASE_URL="$${DATABASE_URL:-}" BACKUP_DIR="$${BACKUP_DIR:-./backups}" deployment/scripts/backup-postgres.sh

db-restore: ## Restore the database from a dump: make db-restore FILE=backups/x.sql.gz
	@test -n "$(FILE)" || { echo "usage: make db-restore FILE=backups/geoguessme-*.sql.gz"; exit 2; }
	DATABASE_URL="$${DATABASE_URL:-}" deployment/scripts/restore-postgres.sh "$(FILE)"

##@ Production
prod-config: ## Validate that production images and env are configured.
	@test -n "$$BACKEND_IMAGE" || { echo "export BACKEND_IMAGE=<registry>/geoguessme-backend:<tag>"; exit 2; }
	@test -n "$$WEB_IMAGE" || { echo "export WEB_IMAGE=<registry>/geoguessme-web:<tag>"; exit 2; }
	@test -f deployment/env/production.env || { echo "create deployment/env/production.env from production.env.example"; exit 2; }
	@echo "production configuration OK"

prod-migrate: prod-config ## Run the production migration job.
	$(COMPOSE_PROD) run --rm migration migrate up

prod-up: prod-config ## Start the production stack.
	$(COMPOSE_PROD) up -d

prod-down: ## Stop the production stack (keeps volumes).
	$(COMPOSE_PROD) down

prod-logs: ## Tail production logs.
	$(COMPOSE_PROD) logs -f

smoke: ## Smoke-test a running gateway (default http://localhost).
	deployment/scripts/smoke-test.sh $${BASE_URL:-http://localhost}

##@ Maintenance
clean: ## Remove generated build/test artifacts (never touches persistent volumes).
	rm -rf backend/bin backend/tmp backend/coverage.out
	rm -rf frontend/dist frontend/coverage frontend/test-results frontend/playwright-report frontend/blob-report
	rm -rf coverage
	find . -type d -name node_modules -prune -o -type f -name '*.test' -print -delete 2>/dev/null || true

reset-dev: ## Delete development volumes. Requires CONFIRM=reset-dev.
ifeq ($(CONFIRM),reset-dev)
	$(COMPOSE_DEV) down -v --remove-orphans
else
	@echo "This deletes the development database and media volumes. Re-run with CONFIRM=reset-dev."
	@exit 2
endif

##@ CI
ci: fmt-check vet test-backend test-backend-race test-frontend lint audit build-backend build-frontend ## Run fast checks as CI does locally (excl. integration/e2e).
	tools/quality/structure-check

quality: format-check vet lint audit test-backend test-frontend build-backend build-frontend ## All quality gates (fmt + lint + audit + build + tests).
	tools/quality/structure-check

audit: ## Run vulnerability and dependency checks.
	cd backend && $(GO) vet ./... && command -v govulncheck >/dev/null && govulncheck ./... || true
	cd frontend && $(NODE) audit 2>/dev/null || true
