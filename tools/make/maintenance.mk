# Maintenance targets: cache/artifact reporting, pruning, disk cleanup, and
# destructive reset operations that require explicit confirmation. Fragment of
# the root Makefile.

##@ Maintenance
maintenance-report: ## Print an agent-readable snapshot of structural maintenance pressure.
	tools/quality/debt/report.sh

cache-status: ## Report project-only Docker images, build cache, volumes, and artifacts (read-only).
	bash tools/quality/cache-status.sh

prune-report: ## Preview project-scoped Docker prune (dry-run, no changes).
	PROJECT_PREFIX=geoguessme bash tools/quality/prune.sh --dry-run --include-build-cache

prune: ## Prune project-scoped Docker artifacts and cache; requires CONFIRM=prune.
	@test "$(CONFIRM)" = "prune" || { echo 'Refusing to prune without CONFIRM=prune. Use make prune-report to preview, then CONFIRM=prune make prune to execute.' >&2; exit 2; }
	PROJECT_PREFIX=geoguessme CONFIRM=prune bash tools/quality/prune.sh --force $(ARGS)

disk-cleanup-report: ## Preview project disk cleanup (dry-run, no changes).
	bash tools/quality/disk-cleanup.sh --dry-run

disk-cleanup: ## Clean project disk artifacts; requires CONFIRM=disk-cleanup.
	@test "$(CONFIRM)" = "disk-cleanup" || { echo 'Refusing to clean without CONFIRM=disk-cleanup. Use make disk-cleanup-report to preview, then CONFIRM=disk-cleanup make disk-cleanup to execute.' >&2; exit 2; }
	CONFIRM=disk-cleanup bash tools/quality/disk-cleanup.sh --force $(ARGS)

build-cache-prune: ## Remove dangling build cache to prevent unbounded growth.
	docker builder prune --force

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
