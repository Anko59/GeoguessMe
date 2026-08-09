# Build, deployment, and rehearsal targets: production artifacts, Compose,
# migrations, rehearsals, hosted/terraform infrastructure, and smoke tests.
# Fragment of the root Makefile.

##@ Deployment and rehearsals
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
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'latest=$$(for path in backend/internal/database/migrations/*.sql; do basename "$$path"; done | sed "s/^0*\([0-9]*\)_.*/\1/" | sort -n | tail -1); next=$$(( $${latest:-0} + 1 )); file=$$(printf "backend/internal/database/migrations/%03d_%s.sql" $$next "$(NAME)"); printf -- "-- %03d %s\n" $$next "$(NAME)" > "$$file"; echo "created $$file"'

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
	deployment/scripts/migration-concurrency.sh

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

vapid-keys: ## Print a fresh Web Push keypair for VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY.
	@$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && go run . vapid-keys'

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
		-e VAPID_PUBLIC_KEY -e VAPID_PRIVATE_KEY -e VAPID_SUBJECT \
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
