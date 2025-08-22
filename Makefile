# Makefile for the kds project
#
# This file provides a set of commands to standardize development and release tasks.

# --- Variables ---
# The name of the binary to be produced.
BINARY_NAME=kds

# --- Main Commands ---

.PHONY: help
help: ## Display this help screen.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: release
release: lint test ## Create a new version tag and push it to trigger the release pipeline.
	@if [ -z "$(VERSION)" ]; then \
		echo "\033[31mError: VERSION is not set.\033[0m"; \
		echo "Usage: make release VERSION=vX.Y.Z"; \
		exit 1; \
	fi
	@if [ -n "$(shell git status --porcelain)" ]; then \
		echo "\033[31mError: Git working directory is not clean.\033[0m"; \
		echo "Please commit or stash your changes before releasing."; \
		exit 1; \
	fi
	@echo "\033[32m--> Tagging version $(VERSION)\033[0m"
	@git tag -a "$(VERSION)" -m "Release $(VERSION)"
	@echo "\033[32m--> Pushing tag to origin to trigger release workflow\033[0m"
	@git push origin "$(VERSION)"
	@echo "\033[32m✅ Release for $(VERSION) has been triggered successfully!\033[0m"


# --- Development & QA Commands ---

.PHONY: lint
lint: ## Run the linter on the entire project.
	@echo "--> Running linter..."
	@golangci-lint run ./...

.PHONY: test
test: ## Run all tests with the race detector.
	@echo "--> Running tests..."
	@go test -v -race ./...

.PHONY: build
build: ## Build a local binary for the current OS.
	@echo "--> Building local binary..."
	@go build -o $(BINARY_NAME) .

.PHONY: clean
clean: ## Remove build artifacts.
	@echo "--> Cleaning up..."
	@rm -f $(BINARY_NAME)
	@rm -rf dist/