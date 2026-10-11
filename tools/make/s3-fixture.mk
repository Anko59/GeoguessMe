# Local S3 fixture tooling; the maintained client reuses the existing backend
# SDK/module graph without adding a package manager or compiler to the host.
GEOGUESSME_S3_FIXTURE_NETWORK ?= geoguessme-dev_default
export GEOGUESSME_S3_FIXTURE_NETWORK
GEOGUESSME_S3_FIXTURE_VOLUME_MAX ?= 32
override GEOGUESSME_S3_FIXTURE_VOLUME_MAX := $(value GEOGUESSME_S3_FIXTURE_VOLUME_MAX)
export GEOGUESSME_S3_FIXTURE_VOLUME_MAX
S3_FIXTURE_ENDPOINT ?= http://minio:9000
S3_FIXTURE_ACCESS_KEY ?= minioadmin
S3_FIXTURE_SECRET_KEY ?= minioadmin
S3_FIXTURE_REGION ?= us-east-1
# Environment data is literal, including '$' in credentials/keys. Never expand
# it as a Make macro or interpolate it into a shell recipe.
override S3_FIXTURE_ENDPOINT := $(value S3_FIXTURE_ENDPOINT)
override S3_FIXTURE_ACCESS_KEY := $(value S3_FIXTURE_ACCESS_KEY)
override S3_FIXTURE_SECRET_KEY := $(value S3_FIXTURE_SECRET_KEY)
override S3_FIXTURE_REGION := $(value S3_FIXTURE_REGION)
override SOURCE_S3_FIXTURE_ENDPOINT := $(value SOURCE_S3_FIXTURE_ENDPOINT)
override SOURCE_S3_FIXTURE_ACCESS_KEY := $(value SOURCE_S3_FIXTURE_ACCESS_KEY)
override SOURCE_S3_FIXTURE_SECRET_KEY := $(value SOURCE_S3_FIXTURE_SECRET_KEY)
override SOURCE_S3_FIXTURE_REGION := $(value SOURCE_S3_FIXTURE_REGION)
export S3_FIXTURE_ENDPOINT S3_FIXTURE_ACCESS_KEY S3_FIXTURE_SECRET_KEY S3_FIXTURE_REGION
export SOURCE_S3_FIXTURE_ENDPOINT SOURCE_S3_FIXTURE_ACCESS_KEY SOURCE_S3_FIXTURE_SECRET_KEY SOURCE_S3_FIXTURE_REGION
override S3_FIXTURE_COMMAND := $(value S3_FIXTURE_COMMAND)
override S3_FIXTURE_ARGS_JSON := $(value S3_FIXTURE_ARGS_JSON)
export S3_FIXTURE_COMMAND S3_FIXTURE_ARGS_JSON
S3_FIXTURE_CLIENT ?= s3-fixture-client
S3_FIXTURE_FILES := /workspace/tools/quality/s3-fixture/main.go /workspace/tools/quality/s3-fixture/copy.go /workspace/tools/quality/s3-fixture/verify.go

##@ Local S3 fixture
validate-s3-fixture-config: ## Require a positive bounded fixture volume count before starting/migrating data.
	@case "$$GEOGUESSME_S3_FIXTURE_VOLUME_MAX" in ''|*[!0-9]*|0*) echo 'GEOGUESSME_S3_FIXTURE_VOLUME_MAX must be an integer 1..4096' >&2; exit 2;; esac; \
		test "$$GEOGUESSME_S3_FIXTURE_VOLUME_MAX" -le 4096

s3-fixture: ## Run a local S3 operation: S3_FIXTURE_COMMAND='ensure BUCKET' (or put/head/get/list/copy/verify).
	@test -n "$$S3_FIXTURE_COMMAND$$S3_FIXTURE_ARGS_JSON" || { echo 'S3_FIXTURE_COMMAND or S3_FIXTURE_ARGS_JSON is required' >&2; exit 2; }
	@$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		-e S3_FIXTURE_ENDPOINT -e S3_FIXTURE_ACCESS_KEY -e S3_FIXTURE_SECRET_KEY -e S3_FIXTURE_REGION \
		-e SOURCE_S3_FIXTURE_ENDPOINT -e SOURCE_S3_FIXTURE_ACCESS_KEY -e SOURCE_S3_FIXTURE_SECRET_KEY -e SOURCE_S3_FIXTURE_REGION \
		-e S3_FIXTURE_COMMAND -e S3_FIXTURE_ARGS_JSON \
		$(S3_FIXTURE_CLIENT) sh -ec 'cd /workspace/backend; go run $(S3_FIXTURE_FILES) $$S3_FIXTURE_COMMAND'

s3-fixture-host: ## Explicit Linux loopback client for verified local development migration.
	$(MAKE) --no-print-directory -s s3-fixture S3_FIXTURE_CLIENT=s3-fixture-host-client

test-s3-fixture: ## Verify local S3 client authentication, integrity, and safe migration contracts in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools sh -ec 'cd /workspace/backend; go test /workspace/tools/quality/s3-fixture/*.go; bash /workspace/tools/quality/s3-fixture/test-migration.sh; bash /workspace/tools/quality/s3-fixture/test-recovery.sh'

test-s3-fixture-race: ## Run fixture integrity and HTTP contract tests with race detection in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps go-security sh -ec 'cd /workspace/backend; CGO_ENABLED=1 go test -race /workspace/tools/quality/s3-fixture/*.go'

test-s3-fixture-snapshot: ## Prove snapshot permissions, read-only source, isolation and private archive ownership using disposable bytes.
	bash tools/quality/s3-fixture/test-snapshot-permissions.sh

test-s3-fixture-integration: validate-s3-fixture-config ## Verify the real S3 image rejects bad/anonymous auth, preserves data, and safely copies objects.
	bash tools/quality/s3-fixture/test-integration.sh

format-s3-fixture: ## Format the fixture client, including newly added Go source files, in Docker.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) go-tools-write sh -ec 'gofmt -w /workspace/tools/quality/s3-fixture/*.go; goimports -w /workspace/tools/quality/s3-fixture/*.go'

s3-fixture-stage: validate-s3-fixture-config ## Start only the isolated migration target; never mount retired MinIO data.
	docker compose -p geoguessme-s3-migration -f deployment/compose.s3-migration.yaml --project-directory . up -d --wait --wait-timeout 120

s3-fixture-stage-down: ## Stop migration staging without deleting either data volume.
	docker compose -p geoguessme-s3-migration -f deployment/compose.s3-migration.yaml --project-directory . down

dev-s3-stage: s3-fixture-stage ## Start the isolated S3 migration staging target.
dev-s3-stage-stop: s3-fixture-stage-down ## Stop S3 migration staging, preserving development data.
dev-s3-recovery-source: ## Explicit offline recovery: private raw snapshot, then loopback-only archived reader (CONFIRM=legacy-s3-recovery).
	CONFIRM="$(CONFIRM)" bash tools/quality/s3-fixture/recovery.sh start

dev-s3-recovery-source-stop: ## Stop only the explicitly managed archived reader; preserve both volumes and the snapshot.
	bash tools/quality/s3-fixture/recovery.sh stop

dev-s3-migrate: ## Copy and independently verify quiesced local MinIO data; never delete the source volume.
	bash tools/quality/s3-fixture/migrate.sh

dev-s3-guard: validate-s3-fixture-config ## Refuse development startup when legacy MinIO data has not been safely migrated.
	bash tools/quality/s3-fixture/guard.sh

verify-s3-upstream: ## Verify the exact official S3 fixture digest and protected release signing identity.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps supply-chain-verifier verify \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com \
		--certificate-identity https://github.com/seaweedfs/seaweedfs/.github/workflows/container_release_unified.yml@refs/tags/4.48 \
		chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d
