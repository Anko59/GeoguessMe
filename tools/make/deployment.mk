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

# Final/runtime images audited by `make audit-images` (F-01). The defaults are
# the digest-pinned third-party runtime/deployment images; override with
# AUDIT_IMAGES=... The backend/web application images are appended automatically
# when BACKEND_IMAGE/WEB_IMAGE are set (CI) or when the geoguessme-*-:local
# images exist (built with `make build-images`). Images already present in the
# host daemon are exported and scanned via --input so private registry
# credentials never need to enter the Trivy container.
AUDIT_IMAGES ?= postgres:15-alpine@sha256:3d0f7584ed7d04e27fa050d6683a74746608faf21f202be78460d679cc56461f \
	caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648 \
	cloudflare/cloudflared:2026.7.3@sha256:e39ee8da81ad5e05d77f38d2f51c60ca51bf2a8450ac3abab50c17fdb91d91bf \
	hashicorp/terraform:1.15.8@sha256:7ae513256f7ce67879e218ae8593d6fbe216ec9e123abe6c94e4e10704857963 \
	ghcr.io/getsops/sops:v3.13.3@sha256:857f5a151ac0b2bfc55c1e4e5581d66fb8e268e4d106b38e74191f3bac9d58ea \
	restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510

audit-images: ## Scan final/runtime images for FIXED High/Critical CVEs (blocking gate) and write JSON reports + SPDX SBOMs under security/image-reports/.
	@bash tools/quality/image-scan-exceptions-check.sh
	@set -eu; \
	mkdir -p security/image-reports/.trivy-cache; \
	images="$(AUDIT_IMAGES)"; \
	if [ -n "$${BACKEND_IMAGE:-}" ] && [ -n "$${WEB_IMAGE:-}" ]; then \
		images="$$images $${BACKEND_IMAGE} $${WEB_IMAGE}"; \
	elif docker image inspect geoguessme-backend:local >/dev/null 2>&1; then \
		images="$$images geoguessme-backend:local geoguessme-web:local"; \
	else \
		echo 'audit-images: warning: app images skipped (set BACKEND_IMAGE/WEB_IMAGE or run `make build-images`)' >&2; \
	fi; \
	for img in $$images; do \
		safe=$$(printf '%s' "$$img" | tr '/:@' '___'); \
		mkdir -p "security/image-reports/$$safe"; \
		bash tools/quality/image-scan-exceptions-check.sh --emit "$$img" "security/image-reports/$$safe/ignore.trivy"; \
		if docker image inspect "$$img" >/dev/null 2>&1; then \
			base_name=$$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.base.name" }}' "$$img"); \
			base_digest=$$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.base.digest" }}' "$$img"); \
			[ "$$base_name" = '<no value>' ] && base_name=''; \
			[ "$$base_digest" = '<no value>' ] && base_digest=''; \
			if [ -n "$$base_name" ] || [ -n "$$base_digest" ]; then \
				[ -n "$$base_name" ] && [ -n "$$base_digest" ] || { echo "audit-images: $$img has incomplete OCI base-image provenance" >&2; exit 1; }; \
				case "$$base_name" in *@sha256:*) echo "audit-images: $$img base.name must not contain a digest" >&2; exit 1;; esac; \
				printf '%s' "$$base_digest" | grep -Eq '^sha256:[0-9a-f]{64}$$' || { echo "audit-images: $$img has malformed base.digest" >&2; exit 1; }; \
				bash tools/quality/image-scan-exceptions-check.sh --append "$$base_name@$$base_digest" "security/image-reports/$$safe/ignore.trivy"; \
			fi; \
			docker save "$$img" -o "security/image-reports/$$safe/image.tar"; \
			scan_target="--input /workspace/security/image-reports/$$safe/image.tar"; \
		else \
			scan_target="$$img"; \
		fi; \
		echo "==> audit-images: $$img"; \
		$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) trivy trivy image --severity HIGH,CRITICAL --exit-code 0 --format json --output "/workspace/security/image-reports/$$safe/report.json" $$scan_target; \
		$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) trivy trivy image --skip-db-update --format spdx-json --output "/workspace/security/image-reports/$$safe/sbom.spdx.json" $$scan_target; \
		$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) trivy trivy image --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 --ignorefile "/workspace/security/image-reports/$$safe/ignore.trivy" --format table $$scan_target; \
		echo "    audit-images: $$img OK (report: security/image-reports/$$safe/report.json, sbom: security/image-reports/$$safe/sbom.spdx.json)"; \
	done; \
	echo 'audit-images: complete'

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
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -c 'dir=backend/internal/database/migrations/$$(date +%Y); mkdir -p "$$dir"; latest=$$(for path in backend/internal/database/migrations/*/*.sql backend/internal/database/migrations/*.sql; do basename "$$path"; done | sed "s/^0*\([0-9]*\)_.*/\1/" | sort -n | tail -1); next=$$(( $${latest:-0} + 1 )); file=$$(printf "%s/%03d_%s.sql" "$$dir" $$next "$(NAME)"); printf -- "-- %03d %s\n" $$next "$(NAME)" > "$$file"; echo "created $$file"'

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
