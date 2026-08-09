# Documentation and agent-config quality checks.
#
# Fragments keep the root Makefile under the 500-line structure limit and group
# responsibility. This fragment owns the Markdownlint gate, the
# docs/agent-config checker, and its regression suite.
#
# Recursive assignments keep the COMPOSE_TOOLS_RUN prefix decoupled from the
# include position in the root Makefile.

DOCS_AGENT_CHECK = $(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/tools/quality/docs-agent-config/check-docs-agent-config.sh
DOCS_AGENT_TEST = $(COMPOSE_TOOLS_RUN) --rm --no-deps go-tools /workspace/tools/quality/docs-agent-config/test-check-docs-agent-config.sh

lint-docs: ## Run Markdownlint and the docs/agent-config checker.
	git ls-files -z '*.md' | xargs -0 -r $(COMPOSE_TOOLS_RUN) --rm --no-deps node-tools markdownlint
	$(DOCS_AGENT_CHECK)

test-docs-agent-config: ## Run docs/agent-config checker regression tests.
	$(DOCS_AGENT_TEST)
