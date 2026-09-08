# Capacitor Android build and device-test targets. The SDK, Gradle, emulator,
# adb, and Maestro all run in the pinned mobile-tools container; only Docker,
# Make, and KVM are required on the host.

MOBILE_API_ORIGIN ?= https://geoguessme.com
MOBILE_WEB_ORIGIN ?= https://geoguessme.com
CAPACITOR_SERVER_URL ?=
MOBILE_TOOLS_SERVICE := $(if $(wildcard /dev/kvm),mobile-tools-kvm,mobile-tools)
MOBILE_TOOLS_RUN := $(COMPOSE_TOOLS_RUN) --rm --no-deps \
	-e HOST_UID=$(shell id -u) -e HOST_GID=$(shell id -g) $(MOBILE_TOOLS_SERVICE)

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

mobile-test: ## Run Maestro against the built APK and an isolated emulator.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps \
		-e HOST_UID=$(shell id -u) -e HOST_GID=$(shell id -g) \
		-e GEOGUESSME_MOBILE_WEB_PORT \
		-e MOBILE_USERNAME -e MOBILE_PASSWORD -e MOBILE_GROUP_NAME \
		$(MOBILE_TOOLS_SERVICE) tools/mobile/run-maestro.sh

test-mobile: ## Run the complete backend, APK, emulator, seed, and Maestro loop.
	tools/mobile/run-e2e.sh
