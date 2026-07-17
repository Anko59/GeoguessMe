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

bootstrap: install-backend install-frontend ## Install all dependencies (Go modules + frontend + Playwright browsers).

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

fmt-check: ## Fail if any Go source is not formatted (read-only).
	cd backend && test -z "$$(gofmt -l .)"

vet: ## Run go vet.
	cd backend && $(GO) vet ./...

lint: ## Lint frontend (assumes dependencies are installed).
	cd frontend && $(NODE) run lint

audit: ## Run Go dependency vulnerability scan.
	cd backend && $(GO) vet ./... && $(GO) list -m all > /dev/null

##@ Tests
test: test-backend test-frontend ## Run backend unit and frontend unit tests.

test-backend: ## Run Go unit/package tests (excludes the live-server integration suite).
	cd backend && $(GO) test $(BACKEND_PKGS)

test-backend-race: ## Run Go unit tests with the race detector.
	cd backend && $(GO) test -race $(BACKEND_PKGS)

test-frontend: ## Run frontend unit tests.
	cd frontend && $(NODE) test -- --run

test-integration: ## Run backend integration tests against the isolated test stack.
	@set -e; \
	 trap 'rc=$$?; $(COMPOSE_TEST) -p geoguessme-integration down -v --remove-orphans >/dev/null 2>&1 || true; exit $$rc' EXIT; \
	 $(TEST_ENV) $(COMPOSE_TEST) -p geoguessme-integration up -d --build --wait; \
	 cd backend && TEST_BASE_URL=$(TEST_BASE_URL) $(GO) test ./integration_test -count=1

test-e2e: ## Run desktop + mobile Playwright suites against the isolated test stack.
	@set -e; \
	 trap 'rc=$$?; $(COMPOSE_TEST) -p geoguessme-e2e down -v --remove-orphans >/dev/null 2>&1 || true; exit $$rc' EXIT; \
	 $(TEST_ENV) $(COMPOSE_TEST) -p geoguessme-e2e up -d --build --wait; \
	 deployment/scripts/wait-for-health.sh $(TEST_BASE_URL) 120; \
	 cd frontend && PLAYWRIGHT_BASE_URL=$(TEST_BASE_URL) $(NODE) exec playwright test --project=desktop --project=mobile

test-e2e-ui: ## Run the Playwright UI mode against the isolated test stack.
	@set -e; \
	 trap 'rc=$$?; $(COMPOSE_TEST) -p geoguessme-e2e down -v --remove-orphans >/dev/null 2>&1 || true; exit $$rc' EXIT; \
	 $(TEST_ENV) $(COMPOSE_TEST) -p geoguessme-e2e up -d --build --wait; \
	 deployment/scripts/wait-for-health.sh $(TEST_BASE_URL) 120; \
	 cd frontend && PLAYWRIGHT_BASE_URL=$(TEST_BASE_URL) $(NODE) exec playwright test --ui

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
ci: fmt-check vet test-backend test-backend-race test-frontend lint audit ## Run the same checks as CI locally.
