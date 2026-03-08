# Makefile for hotreload

.PHONY: build run-demo test clean install

# Build the hotreload tool
build:
	@echo "Building hotreload..."
	@cd cmd/hotreload && go build -o ../../bin/hotreload .
	@echo "Built: bin/hotreload"

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go get github.com/fsnotify/fsnotify

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run the test server with hotreload
run-demo: build
	@echo "Starting test server with hotreload..."
	@./bin/hotreload --root ./testserver --build "go build -o ./server ./testserver" --exec "./server"

# Run the test server directly (without hotreload)
run-server:
	@echo "Starting test server directly..."
	@cd testserver && go run main.go

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f testserver/server

# Build and run tests
check: test build

# Format code
fmt:
	@go fmt ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build the hotreload tool"
	@echo "  install    - Install dependencies"
	@echo "  test       - Run tests"
	@echo "  run-demo   - Run test server with hotreload"
	@echo "  run-server - Run test server directly"
	@echo "  clean      - Clean build artifacts"
	@echo "  check      - Run tests and build"
	@echo "  fmt        - Format code"

