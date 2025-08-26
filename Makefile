# ElasticETL Makefile

# Variables
BINARY_NAME=elasticetl
BUILD_DIR=build
CMD_DIR=cmd/elasticetl
CONFIG_FILE=configs/config.yaml

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-X main.version=1.0.0 -X main.buildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S')"

.PHONY: all build clean test deps run help

# Default target
all: deps build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for current platform
build-local:
	@echo "Building $(BINARY_NAME) for local platform..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./$(CMD_DIR)
	@echo "Build complete: $(BINARY_NAME)"

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Run the application with default config
run: build-local
	@echo "Running $(BINARY_NAME) with default config..."
	./$(BINARY_NAME) -config $(CONFIG_FILE)

# Run with debug logging
run-debug: build-local
	@echo "Running $(BINARY_NAME) in debug mode..."
	./$(BINARY_NAME) -config $(CONFIG_FILE) -log-level debug

# Run with simple example config
run-example: build-local
	@echo "Running $(BINARY_NAME) with example config..."
	./$(BINARY_NAME) -config examples/simple-config.json

# Validate configuration
validate-config: build-local
	@echo "Validating configuration..."
	./$(BINARY_NAME) -config $(CONFIG_FILE) -version > /dev/null && echo "Configuration is valid"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run

# Install the binary to GOPATH/bin
install: build-local
	@echo "Installing $(BINARY_NAME)..."
	cp $(BINARY_NAME) $(GOPATH)/bin/

# Create a release package
package: build-all
	@echo "Creating release packages..."
	@mkdir -p $(BUILD_DIR)/packages
	tar -czf $(BUILD_DIR)/packages/$(BINARY_NAME)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64 -C .. configs examples README.md
	tar -czf $(BUILD_DIR)/packages/$(BINARY_NAME)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64 -C .. configs examples README.md
	zip -j $(BUILD_DIR)/packages/$(BINARY_NAME)-windows-amd64.zip $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe configs/* examples/* README.md

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint
	$(GOMOD) download

# Show help
help:
	@echo "ElasticETL Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build          - Build the application for current platform"
	@echo "  build-local    - Build the application in current directory"
	@echo "  build-all      - Build for all supported platforms"
	@echo "  build-linux    - Build for Linux"
	@echo "  build-darwin   - Build for macOS"
	@echo "  build-windows  - Build for Windows"
	@echo "  deps           - Install dependencies"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Clean build artifacts"
	@echo "  run            - Build and run with default config"
	@echo "  run-debug      - Build and run in debug mode"
	@echo "  run-example    - Build and run with example config"
	@echo "  validate-config- Validate configuration file"
	@echo "  fmt            - Format code"
	@echo "  lint           - Lint code (requires golangci-lint)"
	@echo "  install        - Install binary to GOPATH/bin"
	@echo "  package        - Create release packages"
	@echo "  dev-setup      - Setup development environment"
	@echo "  help           - Show this help message"
