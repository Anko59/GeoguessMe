# Build, deployment, and rehearsal targets: production artifacts, Compose,
# migrations, rehearsals, hosted/terraform infrastructure, and smoke tests.
# Fragment of the root Makefile.

##@ Deployment and rehearsals
build: build-frontend build-backend ## Build production frontend and backend artifacts in Docker.

build-backend: ## Build the backend binary in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'cd backend && go build -trimpath -o bin/geoguessme .'

build-frontend: prepare-frontend-cache ## Build the frontend bundle in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write npm --prefix /workspace/frontend run build

build-images: prepare-app-runtime prepare-database-runtime build-keycloak-image ## Build production images with normal Docker layer caching.
	@set -eu; caddy=$$(bash tools/quality/dependency-images/selected.sh caddy-runtime build); \
	docker build --pull $(DOCKER_BUILD_FLAGS) -f deployment/docker/backend.Dockerfile -t "$(LOCAL_BACKEND_IMAGE)" .; \
	docker build $(DOCKER_BUILD_FLAGS) --build-arg CADDY_RUNTIME_IMAGE="$$caddy" -f deployment/docker/frontend.Dockerfile -t "$(LOCAL_WEB_IMAGE)" .

build-keycloak-image: ## Build the digest-pinned Keycloak image.
	bash tools/quality/dependency-images/lifecycle.sh prepare-local keycloak
	@set -eu; keycloak=$$(bash tools/quality/dependency-images/selected.sh keycloak); \
	docker tag "$$keycloak" "$(LOCAL_KEYCLOAK_IMAGE)"

clean-build: prepare-app-runtime prepare-database-runtime ## Build production images from scratch without any layer cache.
	@set -eu; caddy=$$(bash tools/quality/dependency-images/selected.sh caddy-runtime build); \
	docker build --pull --no-cache $(DOCKER_BUILD_FLAGS) -f deployment/docker/backend.Dockerfile -t "$(LOCAL_BACKEND_IMAGE)" .; \
	docker build --no-cache $(DOCKER_BUILD_FLAGS) --build-arg CADDY_RUNTIME_IMAGE="$$caddy" -f deployment/docker/frontend.Dockerfile -t "$(LOCAL_WEB_IMAGE)" .
	$(MAKE) build-keycloak-image

compose-validate: ## Validate every Compose file.
	docker compose --profile social -f deployment/compose.dev.yaml --project-directory . config --quiet
	docker compose -f deployment/compose.test.yaml --project-directory . config --quiet
	GEOGUESSME_IDENTITY_ENV_FILE=deployment/env/identity.env.example docker compose -f deployment/compose.identity.yaml --project-directory . config --quiet
	WEB_IMAGE=example.invalid/geoguessme-web@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb GEOGUESSME_WATCH_AGENT_ENV=deployment/env/watch-agent.env.example GEOGUESSME_WATCH_METRICS_DIR=$(abspath deployment/env) docker compose -f deployment/compose.watch.yaml --project-directory . config --quiet
	BACKEND_IMAGE=geoguessme-backend:local WEB_IMAGE=geoguessme-web:local docker compose --profile social -f deployment/compose.production.yaml --project-directory . config --quiet
	COMPOSE_PROJECT_NAME=geoguessme-dev GEOGUESSME_ENV_FILE=deployment/env/dev.env.example GEOGUESSME_WEB_PORT=8082 BACKEND_IMAGE=geoguessme-backend:local WEB_IMAGE=geoguessme-web:local docker compose --profile social -f deployment/compose.production.yaml -f deployment/compose.hosted.yaml --project-directory . config --quiet
	docker compose -f deployment/compose.tools.yaml --project-directory . config --quiet

migrate-up: prepare-database-runtime ## Apply pending migrations through the backend container.
	$(WITH_SELECTED) postgres -- $(COMPOSE_DEV) run --rm backend migrate up

migrate-status: prepare-database-runtime ## Show migration status through the backend container.
	$(WITH_SELECTED) postgres -- $(COMPOSE_DEV) run --rm backend migrate status

migration-new: ## Create a migration file after checking NAME.
	@test -n "$(NAME)" || { echo "usage: make migration-new NAME=description"; exit 2; }
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'dir=backend/internal/database/migrations/$$(date +%Y); mkdir -p "$$dir"; latest=$$(for path in backend/internal/database/migrations/*/*.sql backend/internal/database/migrations/*.sql; do basename "$$path"; done | sed "s/^0*\([0-9]*\)_.*/\1/" | sort -n | tail -1); next=$$(( $${latest:-0} + 1 )); file=$$(printf "%s/%03d_%s.sql" "$$dir" $$next "$(NAME)"); printf -- "-- %03d %s\n" $$next "$(NAME)" > "$$file"; echo "created $$file"'

db-backup: ## Create a PostgreSQL backup through the tool container.
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 2; }
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) -e DATABASE_URL="$(DATABASE_URL)" -e BACKUP_DIR=/workspace/backups go-security /workspace/deployment/scripts/backup-postgres.sh

db-restore: ## Restore a PostgreSQL backup through the tool container.
	@test -n "$(FILE)" || { echo "usage: make db-restore FILE=backups/file.sql.gz"; exit 2; }
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 2; }
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) -e DATABASE_URL="$(DATABASE_URL)" go-security /workspace/deployment/scripts/restore-postgres.sh "$(FILE)"

backup-rehearsal: build-images ## Run the disposable backup/restore rehearsal.
	$(WITH_SELECTED) postgres -- deployment/scripts/backup-restore-rehearsal.sh

restart-rehearsal: build-images ## Run the disposable restart/reconnect rehearsal.
	$(WITH_SELECTED) postgres -- deployment/scripts/restart-rehearsal.sh

reconnect-rehearsal: build-images ## Run the load/reconnect/catch-up rehearsal with exact-once evidence.
	$(WITH_SELECTED) postgres -- deployment/scripts/reconnect-rehearsal.sh

migration-test: build-images ## Run concurrent, idempotent, and legacy-fixture migration tests.
	$(WITH_SELECTED) postgres -- deployment/scripts/migration-concurrency.sh

operational-gate: build-images container-verify prod-container-verify migration-test backup-rehearsal restart-rehearsal reconnect-rehearsal test-restart-regression smoke ## Run the dev-pipeline operational gate: containers, migrations, rehearsals, smoke. The full release gate (`make verify`) additionally runs the complete browser matrix, load-test, and audit-images on the nightly schedule.

load-test: build-images ## Run the documented disposable load profile.
	$(WITH_SELECTED) postgres -- deployment/scripts/load-test.sh

container-verify: build-images ## Verify runtime image hardening and health checks.
	$(WITH_SELECTED) postgres -- deployment/scripts/container-verify.sh

prod-container-verify: build-images ## Full production-container verification: images, compose, stack, health, smoke, teardown.
	$(WITH_SELECTED) postgres -- deployment/scripts/prod-container-verify.sh

prod-config: ## Validate production image and secret configuration.
	@test -n "$$BACKEND_IMAGE" || { echo "BACKEND_IMAGE is required"; exit 2; }
	@test -n "$$WEB_IMAGE" || { echo "WEB_IMAGE is required"; exit 2; }
	@case "$$BACKEND_IMAGE" in *@sha256:*) ;; *) echo "BACKEND_IMAGE must include an immutable @sha256 digest"; exit 2;; esac
	@case "$$WEB_IMAGE" in *@sha256:*) ;; *) echo "WEB_IMAGE must include an immutable @sha256 digest"; exit 2;; esac
	@test -f deployment/env/production.env || { echo "deployment/env/production.env is required"; exit 2; }
	@echo "production configuration OK"

prod-migrate: prod-config prepare-database-runtime ## Run the production migration job.
	$(WITH_SELECTED) postgres -- $(COMPOSE_PROD) run --rm migration migrate up

prod-legacy-identity-plan: prod-config prepare-database-runtime ## Count legacy migration categories without changing Keycloak.
	$(WITH_SELECTED) postgres -- $(COMPOSE_PROD) run --rm migration legacy-identity-migration plan

prod-legacy-identity-provision: prod-config prepare-database-runtime ## Provision verified legacy emails in Keycloak; requires CONFIRM=provision.
	@test "$(CONFIRM)" = provision || { echo "Refusing without CONFIRM=provision"; exit 2; }
	$(WITH_SELECTED) postgres -- $(COMPOSE_PROD) run --rm migration legacy-identity-migration apply --confirm

prod-up: prod-config prepare-database-runtime ## Start the production stack.
	@set -eu; POSTGRES_IMAGE=$$(bash tools/quality/dependency-images/selected.sh postgres); export POSTGRES_IMAGE; \
	if grep -Eq '^OIDC_ENABLED=(true|1)$$' deployment/env/production.env; then \
		$(COMPOSE_PROD) --profile social up -d; \
	else \
		$(COMPOSE_PROD) up -d; \
	fi

prod-down: ## Stop production services and keep data volumes.
	$(COMPOSE_PROD) --profile social down

prod-logs: ## Tail production logs.
	$(COMPOSE_PROD) logs -f

hosted-config: ## Validate production and dev hosted Compose expansion.
	COMPOSE_PROJECT_NAME=geoguessme-production GEOGUESSME_ENV_FILE=deployment/env/production.env.example GEOGUESSME_WEB_PORT=8081 BACKEND_IMAGE=example.invalid/geoguessme-backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa WEB_IMAGE=example.invalid/geoguessme-web@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb docker compose -f deployment/compose.production.yaml -f deployment/compose.hosted.yaml --project-directory . config --quiet
	COMPOSE_PROJECT_NAME=geoguessme-dev GEOGUESSME_ENV_FILE=deployment/env/dev.env.example GEOGUESSME_WEB_PORT=8082 GEOGUESSME_BACKEND_MEMORY=512M GEOGUESSME_DATABASE_MEMORY=768M BACKEND_IMAGE=example.invalid/geoguessme-backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa WEB_IMAGE=example.invalid/geoguessme-web@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb docker compose -f deployment/compose.production.yaml -f deployment/compose.hosted.yaml --project-directory . config --quiet

hosted-contract-test: ## Verify deployment ordering, isolation, locking, rollback, and header contracts.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools bash /workspace/deployment/scripts/hosted/test/backup-pipeline-contracts.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh /workspace/deployment/scripts/hosted/test/dependency-image-contracts.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/keycloak-image-contracts.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/watch-deploy-contracts.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/contracts.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/runtime-hash-contracts.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/prune-releases.sh
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/deployment/scripts/hosted/test/runtime-bundle.sh

watch-config: ## Validate the isolated monitoring Compose topology with example secrets.
	WEB_IMAGE=example.invalid/geoguessme-web@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb GEOGUESSME_WATCH_AGENT_ENV=$(abspath deployment/env/watch-agent.env.example) GEOGUESSME_WATCH_METRICS_DIR=$(abspath deployment/env) docker compose -f deployment/compose.watch.yaml --project-directory deployment config --quiet

watch-rehearsal: watch-config build-images build-socket-proxy-image ## Exercise monitoring ingestion, filtering, path routing, and loopback binding in a disposable stack.
	$(WITH_SELECTED) postgres socket-proxy -- deployment/scripts/watch/rehearsal.sh

cloudflared-access-ssh: ## Proxy SSH through Access; requires HOST and service-token env vars.
	@test -n "$(HOST)" || { echo 'HOST is required' >&2; exit 2; }
	@test -n "$${TUNNEL_SERVICE_TOKEN_ID:-}" || { echo 'TUNNEL_SERVICE_TOKEN_ID is required' >&2; exit 2; }
	@test -n "$${TUNNEL_SERVICE_TOKEN_SECRET:-}" || { echo 'TUNNEL_SERVICE_TOKEN_SECRET is required' >&2; exit 2; }
	@bash tools/quality/dependency-images/lifecycle.sh prepare-local cloudflared
	@$(WITH_SELECTED) cloudflared -- $(COMPOSE_TOOLS_RUN) --rm --no-deps cloudflared access ssh --hostname "$(HOST)"

export OPS_SSH_COMMAND

credentials-preflight: ## Safely report local keyring, SSH-agent, and operator tooling availability.
	@bash tools/ops/credentials.sh preflight

terraform-credentials-preflight: ## Report Terraform cloud credential availability without printing values.
	@bash tools/ops/credentials.sh terraform-preflight

ops-ssh: ## Open the documented operator SSH route; set HOST=dev|production and optional OPS_SSH_COMMAND.
	@case "$(HOST)" in dev|production) ;; *) echo 'HOST must be dev or production' >&2; exit 2 ;; esac
	@bash tools/ops/credentials.sh ssh "$(HOST)"

deployment-hash-check: ## Verify installed host runtime definitions match the deployed revision (via Access SSH).
	@case "$(ENVIRONMENT)" in dev) ;; production) ;; *) echo 'ENVIRONMENT=dev|production is required' >&2; exit 2 ;; esac
	@test -n "$${TUNNEL_SERVICE_TOKEN_ID:-}" || { echo 'TUNNEL_SERVICE_TOKEN_ID is required' >&2; exit 2; }
	@test -n "$${TUNNEL_SERVICE_TOKEN_SECRET:-}" || { echo 'TUNNEL_SERVICE_TOKEN_SECRET is required' >&2; exit 2; }
	@test -n "$${DEPLOY_SSH_PRIVATE_KEY:-}" || { echo 'DEPLOY_SSH_PRIVATE_KEY is required' >&2; exit 2; }
	@test -n "$${DEPLOY_SSH_KNOWN_HOSTS:-}" || { echo 'DEPLOY_SSH_KNOWN_HOSTS is required' >&2; exit 2; }
	@bash tools/quality/dependency-images/lifecycle.sh prepare-local cloudflared
	@set -eu; CLOUDFLARED_IMAGE=$$(bash tools/quality/dependency-images/selected.sh cloudflared); export CLOUDFLARED_IMAGE; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT INT TERM; \
	case "$(ENVIRONMENT)" in dev) target_host=deploy.geoguessme.com ;; production) target_host=deploy-prod.geoguessme.com ;; esac; \
	printf '%s\n' "$$DEPLOY_SSH_PRIVATE_KEY" >"$$tmp/deploy"; \
	printf '%s\n' "$$DEPLOY_SSH_KNOWN_HOSTS" >"$$tmp/known_hosts"; \
	chmod 0600 "$$tmp/deploy" "$$tmp/known_hosts"; \
	ssh -i "$$tmp/deploy" -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=20 \
		-o UserKnownHostsFile="$$tmp/known_hosts" \
		-o ProxyCommand='$(COMPOSE_TOOLS_RUN) --rm --no-deps cloudflared access ssh --hostname %h' \
		"deploy@$$target_host" "verify $(ENVIRONMENT)"

terraform-fmt: ## Format infrastructure code in the pinned Terraform container.
	$(TERRAFORM) fmt -recursive

terraform-fmt-check: ## Check infrastructure formatting in the pinned Terraform container.
	$(TERRAFORM) fmt -check -recursive

terraform-init: ## Initialize the R2 backend; requires infra/terraform/backend.hcl.
	@test -f infra/terraform/backend.hcl || { echo 'copy backend.hcl.example to backend.hcl and fill it first'; exit 2; }
	$(TERRAFORM) init -backend-config=backend.hcl

terraform-validate: ## Initialize without remote state and validate Terraform.
	$(TERRAFORM_ISOLATED) 'terraform init -backend=false && terraform validate'

terraform-cloud-init-test: ## Prove exact compact user-data decoding using the real Ubuntu cloud-init parser without network or cloud access.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools bash /workspace/tools/quality/cloud-init/test-runner.sh
	DOCKER_BUILD_FLAGS="$(DOCKER_BUILD_FLAGS)" bash tools/quality/cloud-init/run-test.sh

terraform-test: terraform-cloud-init-test ## Validate isolated infrastructure and fully mocked resources offline, without operator state or credentials.

terraform-plan: terraform-init ## Create a reviewed plan in a mode-0700 directory.
	@install -d -m 0700 infra/terraform/.tfplan
	$(TERRAFORM) plan -out=.tfplan/geoguessme.tfplan
	@chmod 0600 infra/terraform/.tfplan/geoguessme.tfplan
	@echo 'Plan written to infra/terraform/.tfplan/geoguessme.tfplan (mode 0600).'

terraform-apply: ## Apply the exact reviewed plan; requires CONFIRM=apply.
	@test "$(CONFIRM)" = apply || { echo 'Refusing without CONFIRM=apply'; exit 2; }
	@test -f infra/terraform/.tfplan/geoguessme.tfplan || { echo 'run make terraform-plan first'; exit 2; }
	$(TERRAFORM) apply .tfplan/geoguessme.tfplan
	@rm -f infra/terraform/.tfplan/geoguessme.tfplan
	@echo 'Plan applied and removed.'

vapid-keys: ## Print a fresh Web Push keypair for VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY.
	@$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -c 'cd backend && go run . vapid-keys'

secrets-encrypt: build-sops-image ## Encrypt ENV=dev|production from its example using RECIPIENT.
	@case "$(ENV)" in dev|production) ;; *) echo 'ENV must be dev or production'; exit 2;; esac
	@test -n "$(RECIPIENT)" || { echo 'RECIPIENT is required'; exit 2; }
	cp deployment/env/$(ENV).env.example deployment/secrets/$(ENV).env.enc
	$(WITH_SELECTED) sops -- $(COMPOSE_TOOLS_RUN) --rm --no-deps sops sops --encrypt --input-type dotenv --output-type dotenv --age "$(RECIPIENT)" --in-place /workspace/deployment/secrets/$(ENV).env.enc

secrets-generate: build-sops-image ## Generate and SOPS-encrypt ENV=dev|production without a plaintext file.
	@case "$(ENV)" in dev|production) ;; *) echo 'ENV must be dev or production'; exit 2;; esac
	@test -n "$(RECIPIENT)" || { echo 'RECIPIENT is required'; exit 2; }
	@mkdir -p deployment/secrets
	@set -eu; SOPS_IMAGE=$$(bash tools/quality/dependency-images/selected.sh sops); export SOPS_IMAGE; \
	temporary=$$(mktemp deployment/secrets/.$(ENV).env.enc.XXXXXX); \
	trap 'rm -f "$$temporary"' EXIT INT TERM; \
	bash -o pipefail -c '$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) \
		-e TARGET_ENV=$(ENV) -e BREVO_SMTP_USERNAME -e BREVO_SMTP_PASSWORD \
		-e GHCR_USERNAME -e GHCR_TOKEN -e MEDIA_ACCESS_KEY_ID -e MEDIA_SECRET_ACCESS_KEY \
		-e BACKUP_ACCESS_KEY_ID -e BACKUP_SECRET_ACCESS_KEY -e CLOUDFLARE_ACCOUNT_ID \
		-e KEYCLOAK_CLIENT_SECRET \
		-e VAPID_PUBLIC_KEY -e VAPID_PRIVATE_KEY -e VAPID_SUBJECT \
		go-tools sh /workspace/deployment/scripts/generate-hosted-secret.sh | \
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sops sops --config /dev/null --encrypt \
		--input-type dotenv --output-type dotenv --age "$(RECIPIENT)" /dev/stdin' \
		>"$$temporary"; \
	test -s "$$temporary"; \
	chmod 0600 "$$temporary"; \
	mv "$$temporary" deployment/secrets/$(ENV).env.enc; \
	trap - EXIT INT TERM

identity-secrets-generate: build-sops-image ## Generate shared Keycloak secrets and encrypt them for both host age recipients.
	@test -n "$(RECIPIENT)" || { echo 'RECIPIENT must contain both host age recipients'; exit 2; }
	@mkdir -p deployment/secrets
	@set -eu; SOPS_IMAGE=$$(bash tools/quality/dependency-images/selected.sh sops); export SOPS_IMAGE; \
	temporary=$$(mktemp deployment/secrets/.identity.env.enc.XXXXXX); \
	trap 'rm -f "$$temporary"' EXIT INT TERM; \
	bash -o pipefail -c '$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) \
		-e GOOGLE_OAUTH_CLIENT_ID -e GOOGLE_OAUTH_CLIENT_SECRET \
		-e KEYCLOAK_SMTP_USERNAME -e KEYCLOAK_SMTP_PASSWORD \
		-e PRODUCTION_OIDC_CLIENT_SECRET -e DEV_OIDC_CLIENT_SECRET \
		go-tools sh /workspace/deployment/scripts/hosted/generate-identity-secret.sh | \
	$(COMPOSE_TOOLS_RUN) --rm --no-deps sops sops --config /dev/null --encrypt \
		--input-type dotenv --output-type dotenv --age "$(RECIPIENT)" /dev/stdin' \
		>"$$temporary"; \
	test -s "$$temporary"; \
	chmod 0600 "$$temporary"; \
	mv "$$temporary" deployment/secrets/identity.env.enc; \
	trap - EXIT INT TERM

smoke: build-images ## Run the smoke test against a selected disposable/staging URL.
	if [ -n "$${BASE_URL:-}" ]; then deployment/scripts/smoke-test.sh "$$BASE_URL"; else $(WITH_SELECTED) postgres -- deployment/scripts/smoke-rehearsal.sh; fi

smoke-rehearsal: build-images ## Run the smoke test against a disposable test stack.
	$(WITH_SELECTED) postgres -- deployment/scripts/smoke-rehearsal.sh
