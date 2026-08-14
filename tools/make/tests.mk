# Test targets: unit, regression, integration, E2E, and coverage. Fragment of
# the root Makefile. Aggregate gates in tools/make/quality.mk select from these.

##@ Tests
test-debt-markers-regression: ## Exercise owned and unowned maintenance-marker fixtures.
	tools/quality/debt/check-markers-test.sh

test-unit: test-backend test-frontend test-reconnect-harness ## Run application and operational-tool unit tests.

test-backend: ## Run Go unit tests, excluding live integration tests.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && go test $$(go list ./... | grep -v /integration_test)'

test-frontend: ## Run frontend unit tests.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools npm --prefix /workspace/frontend test -- --run

test-reconnect-harness: ## Run reconnect rehearsal harness unit tests.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd tools/load/reconnect-rehearsal && go test ./...'

test-race: ## Run Go unit tests with the race detector.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security sh -c 'cd backend && go test -race $$(go list ./... | grep -v /integration_test)'

test-backend-race: test-race ## Compatibility alias for test-race.

test-structure-regression: ## Run structure-check regression tests in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		$(if $(GIT_DIR_WORKTREE),-v $(GIT_DIR_WORKTREE):$(GIT_DIR_WORKTREE):ro) \
		go-tools /workspace/tools/quality/test/check-structure-regression.sh

test-makefile-fragments-regression: ## Verify the Makefile fragment split preserves every target, help, and gate ordering.
	bash tools/quality/check-makefile-fragments-regression.sh

test-archcheck-regression: ## Run architecture-checker regression tests.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd /workspace/tools/quality/archcheck && go test -count=1 ./...'

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
	bash tools/quality/test/check-restart-regression.sh && bash tools/quality/test/check-restart-regression.sh --determinism

test-migration-fixture-regression: ## Run migration fixture regression tests.
	bash tools/quality/test/check-migration-fixture-regression.sh

test-image-scan-exceptions-regression: ## Run image-scan exceptions regression tests.
	bash tools/quality/test/image-scan-exceptions/test-image-scan-exceptions-regression.sh

test-integration: build-images ## Run the isolated integration stack and tests in Docker.
	$(TEST_ENV) tools/quality/run-integration.sh

test-e2e: build-images ## Run all Playwright projects in the isolated stack.
	$(TEST_ENV) tools/quality/run-e2e.sh

test-e2e-pr: build-images ## Run the Chromium PR browser suite in the isolated stack.
	$(TEST_ENV) GEOGUESSME_E2E_PROJECTS=desktop tools/quality/run-e2e.sh

test-e2e-ui: build-images ## Run Playwright UI mode in Docker.
	$(TEST_ENV) GEOGUESSME_TEST_PROJECT=geoguessme-e2e-ui tools/quality/run-e2e.sh --ui

test-qa-agent: ## Validate the provider-neutral QA contract and MCP lifecycle in Docker.
	bash tools/qa/test-agent.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps playwright node --check /workspace/tools/qa/browser-mcp.mjs
	$(COMPOSE_TOOLS_RUN) --rm --no-deps playwright node /workspace/tools/qa/test-mcp.mjs
	$(COMPOSE_TOOLS_RUN) --rm --no-deps playwright node /workspace/tools/qa/test-mailbox.mjs

test-qa-mailbox-live: ## Verify the disposable QA mailbox provider from the Playwright container.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps -e QA_LIVE_MAILBOX=1 playwright node /workspace/tools/qa/test-mcp.mjs

qa-agent: ## Run the local LLM-driven source-blind QA agent against deployed dev.
	QA_BASE_URL="$(QA_BASE_URL)" QA_BUILD_SHA="$(QA_BUILD_SHA)" QA_BUDGET="$(QA_BUDGET)" QA_RUNTIME="$(QA_RUNTIME)" QA_REPORT_DIR="$(abspath $(QA_REPORT_DIR))" QA_MAILBOX_PROVIDER="$(QA_MAILBOX_PROVIDER)" QA_ACCOUNT_PASSWORD="$(QA_ACCOUNT_PASSWORD)" QA_ACCOUNT_OWNER_USERNAME="$(QA_ACCOUNT_OWNER_USERNAME)" QA_ACCOUNT_MEMBER_USERNAME="$(QA_ACCOUNT_MEMBER_USERNAME)" QA_ACCOUNT_OUTSIDER_USERNAME="$(QA_ACCOUNT_OUTSIDER_USERNAME)" \
		bash tools/qa/run-local.sh

qa-agent-fast: ## Run the short local LLM QA budget against deployed dev.
	$(MAKE) qa-agent QA_BUDGET=fast

qa-agent-full: ## Run the full local LLM QA budget against deployed dev.
	$(MAKE) qa-agent QA_BUDGET=full

qa-agent-nightly: ## Run the extended local LLM QA budget against deployed dev.
	$(MAKE) qa-agent QA_BUDGET=nightly

qa-browser-mcp: ## Run the Dockerized provider-neutral browser MCP server.
	@test -n "$$(printenv QA_BASE_URL)" || { echo 'QA_BASE_URL is required' >&2; exit 2; }
	@mkdir -p "$(QA_REPORT_DIR)"
	@$(COMPOSE_TOOLS) run -T --rm --no-deps --user "$(shell id -u):$(shell id -g)" \
		-e QA_BASE_URL -e QA_BUILD_SHA -e QA_RUNTIME -e QA_BUDGET \
		-e QA_ACCESS_CLIENT_ID -e QA_ACCESS_CLIENT_SECRET \
		-e QA_ACCOUNT_PASSWORD -e QA_ACCOUNT_OWNER_USERNAME -e QA_ACCOUNT_MEMBER_USERNAME -e QA_ACCOUNT_OUTSIDER_USERNAME \
		-e QA_MAILBOX_PROVIDER -e QA_FAKE_LATITUDE -e QA_FAKE_LONGITUDE -e QA_FAKE_LOCATION_ACCURACY \
		-e QA_ARTIFACT_DIR=/tmp/qa-artifacts \
		-e QA_HOST_ARTIFACT_DIR="$(abspath $(QA_REPORT_DIR))" \
		-v "$(abspath $(QA_REPORT_DIR)):/tmp/qa-artifacts" \
		playwright node /workspace/tools/qa/browser-mcp.mjs

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

test-prune-regression: ## Run prune.sh regression tests.
	bash tools/quality/test/check-prune-regression.sh

test-disk-cleanup-regression: ## Run disk-cleanup.sh regression tests.
	bash tools/quality/test/check-disk-cleanup-regression.sh

test-prod-container-verify-regression: ## Run prod-container-verify.sh regression tests.
	bash tools/quality/test/check-prod-container-verify-regression.sh

test-artifacts-clean-regression: ## Verify artifacts-clean target structure and safety.
	bash tools/quality/test/check-cache-status-regression.sh

test-build-caching: ## Run build-caching regression self-tests.
	tools/quality/test/check-build-caching.sh

coverage: ## Enforce and report backend/frontend coverage thresholds.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && go test -coverprofile=/tmp/backend-coverage.out $$(go list ./... | grep -v /integration_test) 2>&1 | tee /tmp/backend-test-output.txt && go tool cover -func=/tmp/backend-coverage.out | tee -a /tmp/backend-test-output.txt && /workspace/tools/quality/coverage-threshold < /tmp/backend-test-output.txt'
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools-write npm --prefix /workspace/frontend test -- --run --coverage
