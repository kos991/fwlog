.PHONY: all build clean run install deps

BINARY_NAME=nat-query-service
GO_FILES=main.go
BUILD_DIR=./build
INSTALL_DIR=/opt/nat-query

# Compiler flags
CGO_ENABLED=1
LDFLAGS=-ldflags="-s -w -X main.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'v1.0.0')"
GCFLAGS=-gcflags="all=-trimpath=$(PWD)"
ASMFLAGS=-asmflags="all=-trimpath=$(PWD)"

all: clean build

build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) $(GCFLAGS) $(ASMFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(GO_FILES)
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "📦 Binary size: $$(du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1)"

build-static:
	@echo "🔨 Building static binary..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -tags netgo -ldflags="-s -w -extldflags '-static'" -o $(BUILD_DIR)/$(BINARY_NAME)-static $(GO_FILES)
	@echo "✅ Static build complete"

run: build
	@echo "🚀 Starting service..."
	$(BUILD_DIR)/$(BINARY_NAME)

dev:
	@echo "🔧 Running in development mode..."
	go run $(GO_FILES)

install: build
	@echo "📦 Installing to system..."
	sudo mkdir -p $(INSTALL_DIR)
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/
	sudo chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✅ Installed to $(INSTALL_DIR)/$(BINARY_NAME)"

install-service: install
	@echo "⚙️  Installing systemd service..."
	sudo cp nat-query-service.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "✅ Service installed. Use:"
	@echo "   sudo systemctl start nat-query-service"
	@echo "   sudo systemctl enable nat-query-service"

uninstall:
	@echo "🗑️  Uninstalling..."
	sudo systemctl stop nat-query-service 2>/dev/null || true
	sudo systemctl disable nat-query-service 2>/dev/null || true
	sudo rm -f /etc/systemd/system/nat-query-service.service
	sudo rm -rf $(INSTALL_DIR)
	sudo systemctl daemon-reload
	@echo "✅ Uninstalled"

clean:
	@echo "🧹 Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	@echo "✅ Clean complete"

deps:
	@echo "📥 Downloading dependencies..."
	go mod download
	go mod tidy
	@echo "✅ Dependencies ready"

test:
	@echo "🧪 Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Tests complete. Coverage report: coverage.html"

bench:
	@echo "⚡ Running benchmarks..."
	go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof

fmt:
	@echo "🎨 Formatting code..."
	go fmt ./...
	gofmt -s -w .
	@echo "✅ Code formatted"

lint:
	@echo "🔍 Linting code..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; }
	golangci-lint run
	@echo "✅ Lint complete"

docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t nat-query-service:latest .
	@echo "✅ Docker image built"

docker-run:
	@echo "🐳 Running Docker container..."
	docker run -d -p 8080:8080 \
		-v /data/sangfor_fw_log:/data/sangfor_fw_log \
		-v /data/index:/data/index \
		--name nat-query-service \
		nat-query-service:latest

help:
	@echo "Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make run            - Build and run"
	@echo "  make dev            - Run in development mode"
	@echo "  make install        - Install to system"
	@echo "  make install-service- Install systemd service"
	@echo "  make uninstall      - Remove from system"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make test           - Run tests"
	@echo "  make bench          - Run benchmarks"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Lint code"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run Docker container"
