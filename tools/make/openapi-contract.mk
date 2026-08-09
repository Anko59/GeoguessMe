# OpenAPI to frontend TypeScript contract generation and drift gate.
#
# Fragments keep the root Makefile under the 500-line structure limit and group
# responsibility. This fragment owns the pinned openapi-typescript generator,
# the committed generated contract in frontend/src/types/, and the drift gate
# that fails when the committed contract no longer matches docs/openapi.yaml.
#
# Recursive assignments keep the COMPOSE_TOOLS_RUN prefix decoupled from the
# include position in the root Makefile.

.PHONY: openapi-generate openapi-check

OPENAPI_GENERATED_FILE := frontend/src/types/openapi.generated.ts

# The generate pipeline is deterministic: run the pinned openapi-typescript
# generator against the split spec, prepend the repository's required
# generated-file marker, then format with the repository Prettier config. The
# committed file is produced by exactly this pipeline, so the drift gate and
# `make format-check` can never disagree with it.
define openapi-contract-pipeline
	set -euo pipefail; \
	openapi-typescript docs/openapi.yaml -o /tmp/openapi-contract.generated.ts; \
	{ printf "// GENERATED FILE\n"; cat /tmp/openapi-contract.generated.ts; } > /tmp/openapi-contract.marked.ts; \
	prettier --config /workspace/.prettierrc --write /tmp/openapi-contract.marked.ts >/dev/null
endef

openapi-generate: ## Regenerate the committed frontend API contract from the OpenAPI spec.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools-write bash -c 'cd /workspace && $(openapi-contract-pipeline); cp /tmp/openapi-contract.marked.ts $(OPENAPI_GENERATED_FILE)'

openapi-check: ## Fail when the committed frontend API contract differs from a fresh generation.
	$(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools bash -c 'cd /workspace && $(openapi-contract-pipeline); diff -u $(OPENAPI_GENERATED_FILE) /tmp/openapi-contract.marked.ts'
