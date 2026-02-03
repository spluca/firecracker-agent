.PHONY: help proto build test lint clean install run dev fmt deps setup-protoc

BINARY_NAME=fc-agent
PROTO_DIR=api/proto/firecracker/v1
BUILD_DIR=bin
GO=go
PROTOC=protoc

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

proto: ## Generate protobuf code
	@echo "Generating protobuf code..."
	$(PROTOC) --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/firecracker.proto

build: proto ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/fc-agent/main.go

test: ## Run tests
	$(GO) test -v -race -coverprofile=coverage.out ./...

test-integration: ## Run integration tests
	$(GO) test -v -tags=integration ./test/integration/...

coverage: test ## Generate coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run

clean: ## Clean build artifacts
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f $(PROTO_DIR)/*.pb.go

install: build ## Install binary to /usr/local/bin
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo mkdir -p /etc/fc-agent
	sudo cp configs/agent.yaml /etc/fc-agent/
	sudo cp scripts/fc-agent.service /etc/systemd/system/
	sudo systemctl daemon-reload

run: build ## Run the agent
	$(BUILD_DIR)/$(BINARY_NAME) --config configs/agent.yaml

dev: ## Run with hot reload
	$(GO) run cmd/fc-agent/main.go --config configs/agent.yaml

fmt: ## Format code
	$(GO) fmt ./...

deps: ## Download dependencies
	$(GO) mod download
	$(GO) mod tidy

setup-protoc: ## Install protoc compiler
	@echo "Installing protoc and Go plugins..."
	@if [ "$$(uname)" = "Linux" ]; then \
		wget -q https://github.com/protocolbuffers/protobuf/releases/download/v25.2/protoc-25.2-linux-x86_64.zip; \
		unzip -q protoc-25.2-linux-x86_64.zip -d /tmp/protoc; \
		sudo mv /tmp/protoc/bin/protoc /usr/local/bin/; \
		sudo mv /tmp/protoc/include/* /usr/local/include/; \
		rm -rf protoc-25.2-linux-x86_64.zip /tmp/protoc; \
	fi
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "✅ protoc installed successfully"

.DEFAULT_GOAL := help
