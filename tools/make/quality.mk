# Quality targets: format, lint, structure, type checks, security audit, the
# durable architecture rules, and the aggregate gates (preflight, quality,
# verify). Fragment of the root Makefile.
#
# Contract currentness (openapi-check) and agent/documentation reference
# validity (lint-docs) are enforced by their own fragments and run inside the
# same gates; this fragment deliberately does not duplicate them.

##@ Quality
impact: ## Explain test capabilities selected by changed paths (BASE=<git-ref>, HEAD=<git-ref>).
	tools/quality/ci/report-impact.sh "$(if $(BASE),$(BASE),origin/dev)" "$(if $(HEAD),$(HEAD),HEAD)"

format: ## Format tracked source/configuration files in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'git ls-files -z "*.go" | xargs -0 -r gofmt -w && git ls-files -z "*.go" | xargs -0 -r goimports -w'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write bash -c 'git ls-files -z | while IFS= read -r -d "" f; do case "$$f" in *.ts|*.tsx|*.js|*.jsx|*.css|*.html|*.json|*.md|*.yaml|*.yml) if [ -f "$$f" ]; then printf "%s\\0" "$$f"; fi;; esac; done | xargs -0 -r prettier --write'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) sqlfluff-write sqlfluff fix --config backend/.sqlfluff --dialect postgres backend/internal/database/migrations
	git ls-files -z '*.sh' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) shfmt-write shfmt -w -i 4 -ci

fmt: format ## Compatibility alias for format.

format-check: ## Check formatting without rewriting files.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'test -z "$$(git ls-files -z "*.go" | xargs -0 -r gofmt -l)" && test -z "$$(git ls-files -z "*.go" | xargs -0 -r goimports -l)"'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'git ls-files -z | while IFS= read -r -d "" f; do case "$$f" in *.ts|*.tsx|*.js|*.jsx|*.css|*.html|*.json|*.md|*.yaml|*.yml) if [ -f "$$f" ]; then printf "%s\\0" "$$f"; fi;; esac; done | xargs -0 -r prettier --check'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sqlfluff sqlfluff lint --config backend/.sqlfluff --dialect postgres backend/internal/database/migrations
	git ls-files -z '*.sh' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps shfmt shfmt -d -i 4 -ci

fmt-check: format-check ## Compatibility alias for format-check.

lint-go: ## Run strict Go analyzers.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && golangci-lint run ./...'

lint-frontend: ## Run ESLint with zero warnings.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend run lint -- --max-warnings=0

lint-dead-code: ## Reject unused frontend files, exports, and dependencies.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend run lint:dead-code

lint-debt-markers: ## Require durable ownership for deferred code work.
	tools/quality/debt/check-markers.sh

lint-css: ## Run Stylelint.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'cd frontend && stylelint --config /workspace/frontend/.stylelintrc.json --config-basedir /workspace/frontend "src/**/*.css"'

lint-shell: ## Run ShellCheck on every tracked shell script.
	find . -type f -name '*.sh' -not -path './.git/*' -not -path './frontend/node_modules/*' -not -path './frontend/coverage/*' -print0 | sort -z | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps shellcheck shellcheck -x

lint-docker: ## Run Hadolint on every Dockerfile.
	find . -type f \( -name 'Dockerfile' -o -name 'Dockerfile.*' -o -name '*.Dockerfile' \) -not -path './.git/*' -not -path './frontend/node_modules/*' -print0 | sort -z | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps hadolint hadolint

lint-actions: ## Run actionlint on tracked workflows.
	git ls-files -z '.github/workflows/*.yml' '.github/workflows/*.yaml' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps actionlint actionlint

lint-sql: ## Run SQLFluff against migrations.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sqlfluff sqlfluff lint --config backend/.sqlfluff --dialect postgres backend/internal/database/migrations

lint-caddy: ## Validate and format-check Caddy configuration.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps caddy caddy validate --config /workspace/deployment/caddy/Caddyfile
	$(COMPOSE_TOOLS_RUN) --rm --no-deps caddy caddy fmt --diff /workspace/deployment/caddy/Caddyfile

lint-openapi: ## Validate the split OpenAPI contract with Redocly.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend exec -- redocly lint /workspace/docs/openapi.yaml

check-e2e-style: ## Reject synchronization and selector patterns that hide flakiness.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c '! find frontend/e2e -type f -name "*.ts" -print0 | xargs -0 -r grep -nE "waitForTimeout|networkidle|\.last\(\)|nth-child|\.nth\("'

lint: structure-check format-check lint-go lint-frontend lint-dead-code lint-debt-markers lint-css lint-docs lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style ## Run every strict lint gate.

structure-check: ## Enforce tracked-file and directory structure limits.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		$(if $(GIT_DIR_WORKTREE),-v $(GIT_DIR_WORKTREE):$(GIT_DIR_WORKTREE):ro) \
		go-tools /workspace/tools/quality/structure-check

type-check: ## Run the TypeScript compiler without emitting files.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'cd frontend && tsc --noEmit'

audit: ## Run dependency vulnerability audits in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security sh -c 'cd backend && govulncheck ./...'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend audit --audit-level=high

deps-go-security-update: ## Update vulnerable Go security modules and normalize metadata.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'cd backend && GOPATH=/tmp/go GOCACHE=/tmp/go-build-cache go get golang.org/x/crypto@v0.54.0 golang.org/x/text@v0.40.0 && GOPATH=/tmp/go GOCACHE=/tmp/go-build-cache go mod tidy'

deps-npm-security-update: ## Apply compatible npm security fixes to the frontend lockfile.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write npm --prefix /workspace/frontend --cache /tmp/npm-cache audit fix --package-lock-only

deps-npm-lock: ## Refresh the frontend lockfile after an intentional manifest edit.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write npm --prefix /workspace/frontend --cache /tmp/npm-cache install --package-lock-only --ignore-scripts

ARCHCHECK := $(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd /workspace/tools/quality/archcheck && go run .'

archcheck: ## Run the durable architecture rules (mutable globals, SQL in handlers, env reads) via tools/quality/archcheck.
	$(ARCHCHECK)

##@ Gates
preflight: structure-check format-check lint openapi-check archcheck test-makefile-fragments-regression test-structure-regression test-debt-markers-regression test-docs-agent-config test-ci-classifier test-e2e-regression test-dev-workflow-regression hosted-contract-test terraform-fmt-check terraform-test type-check audit test-unit compose-validate ## Run the fast local and pull-request gate.

preflight-docs: structure-check format-check lint-docs test-docs-agent-config test-ci-classifier ## Run the documentation-only pull-request gate.

pr-backend: test-integration ## Run backend live-stack checks selected by CI.

pr-frontend: test-e2e-pr ## Run the Chromium E2E checks selected by CI.

quality: structure-check format-check lint openapi-check archcheck test-structure-regression test-debt-markers-regression test-docs-agent-config test-makefile-fragments-regression test-archcheck-regression test-ci-retention-regression test-e2e-regression test-dev-workflow-regression test-prod-container-verify-regression test-migration-fixture-regression test-image-scan-exceptions-regression test-artifacts-clean-regression hosted-contract-test terraform-fmt-check terraform-test type-check audit test-unit test-race coverage build-images compose-validate ## Run all local quality gates.

verify: quality test-integration test-e2e container-verify compose-validate prod-container-verify migration-test backup-rehearsal restart-rehearsal reconnect-rehearsal test-restart-regression test-artifacts-clean-regression smoke load-test audit-images ## Run the complete release gate, including digest-pinned image scanning (audit-images).

pre-commit: ## Run the strict Dockerized commit gate.
	tools/quality/pre-commit.sh

pre-push: ## Run the fast deterministic gate before pushing.
	$(MAKE) preflight

ci: ## Run the fast deterministic pull-request gate.
	$(MAKE) preflight
