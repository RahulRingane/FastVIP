# Project information
PROJECT_NAME := fastVIP
MODULE_NAME := github.com/fastVIP/fastVIP
BUILD_TIME := $(shell date +%Y-%m-%d\ %H:%M:%S)
BUILD_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
#BUILD_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Build configuration
BUILD_DIR := build

# Linker flags for build information
LDFLAGS := -ldflags "-X 'main.BuildTime=$(BUILD_TIME)' \
                     -X 'main.BuildCommit=$(BUILD_COMMIT)' \
                     -s -w -extldflags -static"

# Default target
.PHONY: all
all: clean build

.PHONY: help
help: ## show help
	@echo "Available targets: "
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## build the binary
	@echo "Building $(PROJECT_NAME) ..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o build/fastVIP cmd/fastVIP/main.go
	@echo "✓ Build completed."

.PHONY: build-dev
build-dev: ## build the binary with debug info
	@echo "Building $(PROJECT_NAME) for development..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -tags integration -race -o build/fastVIP cmd/fastVIP/main.go
	@echo "✓ Development build completed."

.PHONY: build-linux
build-linux: ## build the binary for Linux
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags integration $(LDFLAGS) -o build/fastVIP-linux-amd64 cmd/fastVIP/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags integration $(LDFLAGS) -o build/fastVIP-linux-arm64 cmd/fastVIP/main.go
	@echo "✓ Linux build completed"

.PHONY: build-docker
build-docker: ## build docker image: fastVIP/fastVIP
	@echo "Building fastVIP/fastVIP:latest ..."
	@docker build -t fastVIP/fastVIP .
	@echo "✓ Build completed."

.PHONY: test
test: ## run unit tests (all platforms, using fake IPVS)
	@echo "Running unit tests..."
	@go test -v ./...
	@echo "✓ Tests completed"

.PHONY: test-cov
test-cov: ## run tests with coverage (all platforms, using fake IPVS)
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report generated: coverage.html"

# test-linux runs tests with real IPVS handle, serially (-p 1) because IPVS is a global kernel resource.
# Must be run as root on Linux.
.PHONY: test-linux
test-linux: ## run unit tests with real IPVS (Linux only)
	@echo "Running unit tests for linux..."
	@go test -count=1 -p 1 -tags integration ./...
	@echo "✓ Tests completed"

# e2e tests compile the fastVIP binary and verify IPVS kernel rules end-to-end.
# Must be run as root on Linux.
.PHONY: test-e2e
test-e2e: ## run end-to-end tests for Linux
	@echo "Running e2e tests for linux..."
	@go test -count=1 -v -p 1 -tags integration ./tests/e2e/
	@echo "✓ Tests completed"

.PHONY: test-docker
test-docker: ## run tests inside a Docker container
	@echo "Running containerized tests for macOS/Linux..."
	@bash tests/e2e/run-e2e-container.sh
	@echo "✓ Containerized tests completed"

.PHONY: clean
clean: ## clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "✓ Clean completed"

.PHONY: update
update: ## update dependencies
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "✓ Dependencies updated"

.PHONY: align
align: ## fieldalignment for structs
	@echo "Align structs' fields..."
	@echo "Installing fieldalignment..."
	@go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
	@echo "Running fieldalignment..."
	@fieldalignment -fix ./...
	@echo "✓ alignment completed"