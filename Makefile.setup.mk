# Setup and development targets, plus the shared variables used by every
# fragment. This fragment is included first so its `:=` variables are defined
# before any recipe that uses them.
#
# Fragments keep the root Makefile under the 500-line structure limit and group
# ownership by responsibility. Include order in the root Makefile matches the
# order targets appear in `make help` (the help target iterates MAKEFILE_LIST).

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

# Git worktree directories so structure-check can resolve .git inside containers.
GEOGUESSME_GIT_COMMON_DIR := $(abspath $(shell git rev-parse --git-common-dir 2>/dev/null))
GIT_DIR_WORKTREE := $(abspath $(shell git rev-parse --git-dir 2>/dev/null))
export GEOGUESSME_GIT_COMMON_DIR GIT_DIR_WORKTREE
ARGS ?=

##@ Setup
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\n"} /^[a-zA-Z0-9_.-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0,5) }' $(MAKEFILE_LIST)

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
