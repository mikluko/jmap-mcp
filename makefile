.DEFAULT_GOAL := help

.PHONY: help test test-coverage lint clean install run-stdio run-http

GIT_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-$(shell git rev-parse --short HEAD)")
VERSION := $(GIT_VERSION:v%=%)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run tests
	go test ./... -v

test-coverage: ## Run tests with coverage
	go test ./... -cover -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint: ## Run linters
	golangci-lint run ./...

clean: ## Clean build artifacts
	rm -f jmap-mcp coverage.out coverage.html
	rm -rf dist/

install: ## Install the binary
	go install -ldflags="-X main.version=$(VERSION)"

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
