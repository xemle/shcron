.PHONY: all build run test lint clean format cross-build release

APP_NAME := shcron
APP_PATH := ./cmd/$(APP_NAME)
BUILD_DIR := build
VERSION := $(shell git describe --tags --always --dirty="-dev" --abbrev=7)

# Default target
all: build

# Build the application for the current OS and architecture
build:
	@echo "Building $(APP_NAME)..."
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(APP_NAME) $(APP_PATH)
	@echo "Build complete: $(APP_NAME)"

# Run the application locally (builds first if not already built)
run: build
	@echo "Running $(APP_NAME)..."
	./$(APP_NAME) "1s" echo "Hello from shcron"

# Run all tests
test:
	@echo "Running tests..."
	go test ./... -v

# Run linting (requires golangci-lint installed: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	@echo "Running linters..."
	golangci-lint run ./...

# Format code (requires gofmt)
format:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Code formatted."

# Clean compiled binaries and temporary files
clean:
	@echo "Cleaning up..."
	go clean -modcache
	rm -rf $(APP_NAME) $(BUILD_DIR)
	@echo "Cleanup complete."

# Cross-compile for various platforms
cross-build: clean
	@echo "Building for multiple platforms..."
	mkdir -p $(BUILD_DIR)

	# Linux
	@echo "  Building for Linux (amd64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)_linux_amd64 $(APP_PATH)
	@echo "  Building for Linux (arm64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)_linux_arm64 $(APP_PATH)

	# Windows
	@echo "  Building for Windows (amd64)..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)_windows_amd64.exe $(APP_PATH)

	# macOS
	@echo "  Building for macOS (amd64 - Intel)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)_darwin_amd64 $(APP_PATH)
	@echo "  Building for macOS (arm64 - Apple Silicon)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)_darwin_arm64 $(APP_PATH)

	@echo "All cross-platform builds complete in $(BUILD_DIR)/"
	ls -lh $(BUILD_DIR)

# This target would be used by your GitHub Actions release workflow
release: cross-build
	@echo "Release process (handled by GitHub Actions)"
	# Add any final packaging or checksum steps here if desired locally