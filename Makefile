.PHONY: build run bench clean test install-deps

# Build configuration
BINARY_NAME=escabelo
BENCH_BINARY=bench
BUILD_DIR=bin
DATA_DIR=data

# Default target
all: build

# Install dependencies
install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Build the main server
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/escabelo

# Build the benchmark tool
build-bench:
	@echo "Building $(BENCH_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BENCH_BINARY) ./cmd/bench

# Build the client tool
build-client:
	@echo "Building client..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/client ./cmd/client

# Build the simple test tool (uses build tag to avoid conflicts)
build-test:
	@echo "Building test..."
	@mkdir -p $(BUILD_DIR)
	go build -tags simplebench -o $(BUILD_DIR)/test ./cmd/bench/test.go

# Build all
build-all: build build-bench build-client build-test

# Run the server with default settings
run: build
	@echo "Starting server..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run with custom settings
run-custom: build
	@echo "Starting server with custom settings..."
	./$(BUILD_DIR)/$(BINARY_NAME) \
		-port=8080 \
		-data-dir=./data \
		-memtable-size=67108864 \
		-compaction-interval=5m \
		-wal-sync-interval=1s

# Run benchmark (requires server to be running)
bench: build-bench
	@echo "Running benchmark..."
	./$(BUILD_DIR)/$(BENCH_BINARY) \
		-addr=localhost:8080 \
		-duration=30s \
		-concurrency=10 \
		-read-ratio=0.8 \
		-key-count=10000

# Run intensive benchmark
bench-intensive: build-bench
	@echo "Running intensive benchmark..."
	./$(BUILD_DIR)/$(BENCH_BINARY) \
		-addr=localhost:8080 \
		-duration=60s \
		-concurrency=50 \
		-read-ratio=0.8 \
		-key-count=100000

# Run write-heavy benchmark
bench-write: build-bench
	@echo "Running write-heavy benchmark..."
	./$(BUILD_DIR)/$(BENCH_BINARY) \
		-addr=localhost:8080 \
		-duration=30s \
		-concurrency=20 \
		-read-ratio=0.2 \
		-key-count=50000

# Run simple test - write mode
test-write: build-test
	@echo "Running write test..."
	./$(BUILD_DIR)/test -mode=write -ops=10000 -c=10

# Run simple test - read mode
test-read: build-test
	@echo "Running read test..."
	./$(BUILD_DIR)/test -mode=read -ops=10000 -c=10

# Run simple test - mixed mode
test-mixed: build-test
	@echo "Running mixed test (70% read / 30% write)..."
	./$(BUILD_DIR)/test -mode=mixed -ops=10000 -c=10

# Run all simple tests in sequence
test-all: build-test
	@echo "Running all tests..."
	@echo "\n=== Write Test ==="
	./$(BUILD_DIR)/test -mode=write -ops=5000 -c=5
	@echo "\n=== Read Test ==="
	./$(BUILD_DIR)/test -mode=read -ops=5000 -c=5
	@echo "\n=== Mixed Test ==="
	./$(BUILD_DIR)/test -mode=mixed -ops=5000 -c=5

# Clean build artifacts and data
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -rf $(DATA_DIR)

# Clean only data
clean-data:
	@echo "Cleaning data directory..."
	rm -rf $(DATA_DIR)

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Development: run server with hot reload (requires air)
dev:
	@echo "Starting development server..."
	air

# Quick test: build, run server in background, run benchmark, stop server
quick-test: build-all
	@echo "Running quick test..."
	@./$(BUILD_DIR)/$(BINARY_NAME) -data-dir=./test-data &
	@SERVER_PID=$$!; \
	sleep 2; \
	./$(BUILD_DIR)/$(BENCH_BINARY) -addr=localhost:8080 -duration=10s -concurrency=5; \
	kill $$SERVER_PID; \
	rm -rf ./test-data

# Help
help:
	@echo "Escabelo Key-Value Store - Makefile Commands"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build          - Build the server binary"
	@echo "  make build-bench    - Build the benchmark binary"
	@echo "  make build-client   - Build the interactive client"
	@echo "  make build-test     - Build the simple test tool"
	@echo "  make build-all      - Build all binaries"
	@echo ""
	@echo "Run Commands:"
	@echo "  make run            - Run server with default settings"
	@echo "  make run-custom     - Run server with custom settings"
	@echo ""
	@echo "Benchmark Commands:"
	@echo "  make bench          - Run standard benchmark (30s, 10 clients)"
	@echo "  make bench-intensive - Run intensive benchmark (60s, 50 clients)"
	@echo "  make bench-write    - Run write-heavy benchmark"
	@echo ""
	@echo "Simple Test Commands:"
	@echo "  make test-write     - Run write test (10k ops, 10 workers)"
	@echo "  make test-read      - Run read test (10k ops, 10 workers)"
	@echo "  make test-mixed     - Run mixed test (70% read / 30% write)"
	@echo "  make test-all       - Run all simple tests in sequence"
	@echo ""
	@echo "Utility Commands:"
	@echo "  make clean          - Remove build artifacts and data"
	@echo "  make clean-data     - Remove only data directory"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make fmt            - Format code"
	@echo "  make quick-test     - Build, test, and cleanup"
	@echo ""
	@echo "Development:"
	@echo "  make dev            - Run with hot reload (requires air)"
	@echo "  make install-deps   - Install Go dependencies"
