# Capacitor Android build and device-test targets. The SDK, Gradle, emulator,
# adb, and Maestro all run in the pinned mobile-tools container; only Docker,
# Make, and KVM are required on the host.

MOBILE_API_ORIGIN ?= https://geoguessme.com
MOBILE_WEB_ORIGIN ?= https://geoguessme.com
CAPACITOR_SERVER_URL ?=
MOBILE_KEYSTORE_PATH ?= /workspace/.local/mobile/upload-keystore.jks
MOBILE_KEY_ALIAS ?= geoguessme-upload
MOBILE_RELEASE_ARTIFACT ?= frontend/android/app/build/outputs/bundle/release/app-release.aab
MOBILE_RELEASE_MANIFEST ?= .local/mobile/artifacts/android-release-manifest.json
MOBILE_SOURCE_SHA ?= $(shell git rev-parse HEAD)
MOBILE_SOURCE_TREE ?= $(shell git rev-parse HEAD^{tree})
MOBILE_EXPECTED_UPLOAD_CERT_SHA256 ?=
MOBILE_REQUIRE_EXPECTED_CERT ?= false
PLAY_API_PACKAGE_NAME ?= com.geoguessme.app
PLAY_API_BASE_URL ?= https://androidpublisher.googleapis.com
PLAY_API_AAB ?= android-release/app-release.aab
PLAY_API_MANIFEST ?= android-release/android-release-manifest.json
PLAY_RELEASE_TRACK ?=
PLAY_RELEASE_STATUS ?= completed
MOBILE_TOOLS_SERVICE := $(if $(wildcard /dev/kvm),mobile-tools-kvm,mobile-tools)
MOBILE_TOOLS_RUN := $(COMPOSE_TOOLS_RUN) --rm --no-deps \
	-e HOST_UID=$(shell id -u) -e HOST_GID=$(shell id -g) $(MOBILE_TOOLS_SERVICE)
MOBILE_TOOLS_RELEASE_RUN := $(COMPOSE_TOOLS_RUN) --rm --no-deps \
	-e HOST_UID=$(shell id -u) -e HOST_GID=$(shell id -g) \
	-e MOBILE_KEYSTORE_PATH=$(MOBILE_KEYSTORE_PATH) \
	-e MOBILE_KEY_ALIAS=$(MOBILE_KEY_ALIAS) \
	-e MOBILE_KEYSTORE_PASSWORD -e MOBILE_KEY_PASSWORD \
	$(MOBILE_TOOLS_SERVICE)
MOBILE_TOOLS_ARTIFACT_RUN := $(COMPOSE_TOOLS_RUN) --rm --no-deps \
	-e HOST_UID=$(shell id -u) -e HOST_GID=$(shell id -g) \
	-e MOBILE_EXPECTED_UPLOAD_CERT_SHA256="$(MOBILE_EXPECTED_UPLOAD_CERT_SHA256)" \
	-e MOBILE_REQUIRE_EXPECTED_CERT="$(MOBILE_REQUIRE_EXPECTED_CERT)" \
	-e GITHUB_RUN_ID \
	-e MOBILE_SOURCE_SHA="$(MOBILE_SOURCE_SHA)" \
	-e MOBILE_SOURCE_TREE="$(MOBILE_SOURCE_TREE)" \
	-e MOBILE_REQUIRE_PROVENANCE=true \
	$(MOBILE_TOOLS_SERVICE)
PLAY_API_RUN := $(COMPOSE_TOOLS_RUN) --rm --no-deps \
	-e PLAY_ACCESS_TOKEN \
	-e PLAY_API_BASE_URL="$(PLAY_API_BASE_URL)" \
	-e PLAY_API_PACKAGE_NAME="$(PLAY_API_PACKAGE_NAME)" \
	-e PLAY_RELEASE_TRACK="$(PLAY_RELEASE_TRACK)" \
	-e PLAY_RELEASE_STATUS="$(PLAY_RELEASE_STATUS)" \
	go-tools

##@ Mobile
mobile-init: ## Generate the tracked Capacitor Android project when absent.
	@if test -d frontend/android; then echo 'frontend/android already exists'; else \
		$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) node-tools-write \
		sh -ec 'npm --prefix frontend run build && cd frontend && npx cap add android'; \
	fi

mobile-prepare: ## Install the pinned Android SDK packages and create the test AVD.
	mkdir -p .local/mobile/android-sdk .local/mobile/android-home .local/mobile/gradle .local/mobile/artifacts
	$(COMPOSE_TOOLS) build mobile-tools
	$(MOBILE_TOOLS_RUN) tools/mobile/prepare-android.sh

mobile-sync: mobile-init ## Build shared web assets and sync them into the Android project.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps $(TOOLS_USER) \
		-e VITE_API_ORIGIN=$(MOBILE_API_ORIGIN) -e VITE_WEB_ORIGIN=$(MOBILE_WEB_ORIGIN) \
		-e CAPACITOR_SERVER_URL=$(CAPACITOR_SERVER_URL) node-tools-write \
		sh -ec 'npm --prefix frontend run build && cd frontend && npx cap sync android'

mobile-build: mobile-prepare mobile-sync ## Build a debug APK entirely through Docker.
	$(MOBILE_TOOLS_RUN) tools/mobile/build-android.sh

mobile-keystore: mobile-prepare ## Create a local upload keystore from exported passwords.
	$(MOBILE_TOOLS_RELEASE_RUN) \
		tools/mobile/create-upload-keystore.sh

mobile-build-release: mobile-prepare mobile-sync ## Build the signed release AAB entirely through Docker.
	$(MOBILE_TOOLS_RELEASE_RUN) \
		tools/mobile/build-android-release.sh

mobile-verify-release: mobile-prepare ## Verify the package, version, signature, and certificate of a release AAB.
	$(MOBILE_TOOLS_ARTIFACT_RUN) \
		tools/mobile/verify-release-bundle.sh verify "$(MOBILE_RELEASE_ARTIFACT)"

mobile-release-manifest: mobile-verify-release ## Create non-secret provenance metadata for a verified release AAB.
	$(MOBILE_TOOLS_ARTIFACT_RUN) \
		tools/mobile/verify-release-bundle.sh manifest "$(MOBILE_RELEASE_ARTIFACT)" "$(MOBILE_RELEASE_MANIFEST)"

play-api-check: ## Read the Play app identity with a caller-issued OAuth token.
	@test -n "$${PLAY_ACCESS_TOKEN:-}" || { echo 'PLAY_ACCESS_TOKEN is required and must not be passed on the command line' >&2; exit 2; }
	$(PLAY_API_RUN) sh -c 'cd /workspace/tools/mobile/play-publisher && go run . inspect-app --package-name "$$PLAY_API_PACKAGE_NAME"'

play-api-publish: ## Upload, validate, and commit the verified Android bundle to Play.
	@test -n "$${PLAY_ACCESS_TOKEN:-}" || { echo 'PLAY_ACCESS_TOKEN is required and must not be passed on the command line' >&2; exit 2; }
	@test -n "$(PLAY_RELEASE_TRACK)" || { echo 'PLAY_RELEASE_TRACK is required' >&2; exit 2; }
	@test -f "$(PLAY_API_AAB)" || { echo 'PLAY_API_AAB does not point to a file' >&2; exit 2; }
	@test -f "$(PLAY_API_MANIFEST)" || { echo 'PLAY_API_MANIFEST does not point to a file' >&2; exit 2; }
	$(PLAY_API_RUN) sh -c 'cd /workspace/tools/mobile/play-publisher && go run . publish-bundle \
		--package-name "$$PLAY_API_PACKAGE_NAME" \
		--bundle "/workspace/$(PLAY_API_AAB)" \
		--manifest "/workspace/$(PLAY_API_MANIFEST)" \
		--track "$$PLAY_RELEASE_TRACK" \
		--status "$$PLAY_RELEASE_STATUS"'

mobile-test: ## Run Maestro against the built APK and an isolated emulator.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		-e HOST_UID=$(shell id -u) -e HOST_GID=$(shell id -g) \
		-e GEOGUESSME_MOBILE_WEB_PORT \
		-e MOBILE_USERNAME -e MOBILE_PASSWORD -e MOBILE_GROUP_NAME \
		$(MOBILE_TOOLS_SERVICE) tools/mobile/run-maestro.sh

test-mobile: ## Run the complete backend, APK, emulator, seed, and Maestro loop.
	tools/mobile/run-e2e.sh
