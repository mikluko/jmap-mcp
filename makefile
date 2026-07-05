.DEFAULT_GOAL := help

.PHONY: help build image package publish test test-coverage lint clean install run-stdio run-http \
        release/major release/minor release/micro

# Version derived from git tags (e.g. v0.9.0 -> 0.9.0). Without a tag, fall
# back to a SemVer-valid pre-release (0.0.0-<sha>) so `helm package --version`
# accepts it. --always is intentionally omitted: a bare SHA would shadow the
# fallback and break the chart version.
GIT_VERSION := $(shell git describe --tags --dirty 2>/dev/null || echo "0.0.0-$(shell git rev-parse --short HEAD)")
VERSION := $(GIT_VERSION:v%=%)
KO_DOCKER_REPO := ghcr.io/mikluko/jmap-mcp
CHART_REPO := ghcr.io/mikluko/helm-charts
MCP_SERVER_NAME := io.github.mikluko/jmap-mcp
export KO_DOCKER_REPO

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_\/-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: image package server-json ## Build and push container image, Helm chart, and server.json

image: ## Build and push container image with ko
	@echo "Building and pushing image: $(KO_DOCKER_REPO):$(VERSION)"
	VERSION=$(VERSION) ko build --bare --tags $(VERSION) \
		--image-label io.modelcontextprotocol.server.name=$(MCP_SERVER_NAME) \
		--image-label org.opencontainers.image.source=https://github.com/mikluko/jmap-mcp \
		--image-label org.opencontainers.image.description="MCP server for email management via JMAP" \
		--image-annotation io.modelcontextprotocol.server.name=$(MCP_SERVER_NAME) \
		--image-annotation org.opencontainers.image.source=https://github.com/mikluko/jmap-mcp

package: ## Package and push Helm chart to OCI registry
	@echo "Packaging and pushing chart: $(CHART_REPO)/jmap-mcp:$(VERSION)"
	@helm package charts/jmap-mcp --version $(VERSION) --app-version $(VERSION) --destination .build/
	@helm push .build/jmap-mcp-$(VERSION).tgz oci://$(CHART_REPO)
	@rm .build/jmap-mcp-$(VERSION).tgz
	@echo "Chart pushed successfully"

# Test targets
test: ## Run tests
	go test ./... -v

test-coverage: ## Run tests with coverage
	go test ./... -cover -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint: ## Run linters
	golangci-lint run ./...

# Build targets
clean: ## Clean build artifacts
	rm -f jmap-mcp coverage.out coverage.html
	rm -rf dist/

install: ## Install the binary
	go install -ldflags="-X main.version=$(VERSION)"

server-json: ## Generate server.json from template
	@mkdir -p .build
	VERSION=$(VERSION) envsubst '$$VERSION' < server.json.tmpl > .build/server.json

# MCP Registry
publish: server-json ## Publish server to MCP Registry (requires mcp-publisher)
	mcp-publisher login github -token $$(gh auth token) && mcp-publisher publish .build/server.json

# --- release: bump version -> tag -> push -> build ----------------------------
# release/<level> bumps the chosen component (major|minor|micro) via
# caarlos0/svu, creates an annotated tag, pushes it, and runs `build` to
# publish the image, chart, and server.json at the new version. You pick the
# level explicitly; svu does not infer it from commit history. On main a clean
# worktree is required; off main it produces a prerelease tagged with the
# short HEAD hash (-dirty on a dirty tree). RELEASE=vX.Y.Z forces an exact
# version and skips svu.
RELEASE ?=
REMOTE  ?= origin

# svu_rev,<level>: resolve the version string for the given bump level.
define svu_rev
	branch=$$(git branch --show-current); \
	dirty=$$(git status --porcelain); \
	hash=$$(git rev-parse --short=7 HEAD); \
	if [ "$$branch" = main ] && [ -n "$$dirty" ]; then \
		echo 'worktree dirty; commit or stash before tagging on main' >&2; exit 1; \
	fi; \
	if [ -n '$(RELEASE)' ]; then rel='$(RELEASE)'; \
	elif [ "$$branch" = main ]; then rel=$$(svu $(1)); \
	else rel=$$(svu $(1) --prerelease "$$hash$$([ -n "$$dirty" ] && printf -- -dirty)"); \
	fi
endef

define do_release
	@set -e; $(call svu_rev,$(1)); \
	printf 'tag %s on branch %s, push to %s, and build? [y/N] ' "$$rel" "$$branch" '$(REMOTE)'; \
	read ans; [ "$$ans" = y ] || [ "$$ans" = Y ] || { echo aborted; exit 1; }; \
	git tag -a "$$rel" -m "$$rel"; \
	git push $(REMOTE) "$$rel"; \
	$(MAKE) build
endef

release/major: ## Bump major, tag, push, build (RELEASE=vX.Y.Z to force)
	$(call do_release,major)
release/minor: ## Bump minor, tag, push, build (RELEASE=vX.Y.Z to force)
	$(call do_release,minor)
release/micro: ## Bump patch, tag, push, build (RELEASE=vX.Y.Z to force)
	$(call do_release,patch)

# Local development
run-stdio: ## Run in stdio mode locally
	@if [ -z "$(JMAP_SESSION_URL)" ]; then \
		echo "Error: JMAP_SESSION_URL not set"; \
		exit 1; \
	fi
	@if [ -z "$(JMAP_AUTH_TOKEN)" ]; then \
		echo "Error: JMAP_AUTH_TOKEN not set"; \
		exit 1; \
	fi
	./jmap-mcp -mode stdio

run-http: ## Run in HTTP mode locally
	@if [ -z "$(JMAP_SESSION_URL)" ]; then \
		echo "Error: JMAP_SESSION_URL not set"; \
		exit 1; \
	fi
	./jmap-mcp -mode http -listen localhost:8080
