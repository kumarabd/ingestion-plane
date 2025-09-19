# Gateway Makefile

# Variables
CONTRACTS_DIR := ../contracts
GENERATED_DIR := generated
PROTOC := protoc
PROTOC_GO_PLUGIN := protoc-gen-go
PROTOC_GO_GRPC_PLUGIN := protoc-gen-go-grpc

# Go module path
GO_MODULE := github.com/kumarabd/ingestion-plane/gateway

# Find all .proto files in contracts directory
PROTO_FILES := $(shell find $(CONTRACTS_DIR) -name "*.proto")

# Default target
.PHONY: all
all: gen-proto build

# Generate protobuf code
.PHONY: gen-proto
generate_proto:
	$(PROTOC) -I $(CONTRACTS_DIR) \
	--go_out=paths=source_relative:$(GENERATED_DIR) \
	--go-grpc_out=paths=source_relative:$(GENERATED_DIR) \
	--openapiv2_out $(GENERATED_DIR) \
	--openapiv2_opt logtostderr=true \
	--openapiv2_opt generate_unbound_methods=true \
	$(CONTRACTS_DIR)/**/**/**.proto

# Create generated directory
$(GENERATED_DIR):
	@mkdir -p $(GENERATED_DIR)

# Build the application
.PHONY: build
build: generate_proto
	@echo "Building gateway..."
	@go build -o bin/gateway cmd/main.go
	@echo "Gateway built successfully"

# Run the application
.PHONY: run
run: build
	@echo "Running gateway..."
	@./bin/gateway

# Test the application
.PHONY: test
test:
	@echo "Running tests..."
	@go test ./...

# Clean generated files
.PHONY: clean
clean:
	@echo "Cleaning generated files..."
	@rm -rf $(GENERATED_DIR) bin/
	@echo "Clean complete"

# Install Go protobuf plugins
.PHONY: install-deps
install-deps:
	@echo "Installing Go protobuf plugins..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Go protobuf plugins installed"

# Validate protobuf files
.PHONY: validate
validate:
	@echo "Validating protobuf files..."
	@for proto_file in $(PROTO_FILES); do \
		echo "Validating $$proto_file..."; \
		$(PROTOC) --proto_path=$(CONTRACTS_DIR) --descriptor_set_out=/dev/null $$proto_file; \
	done
	@echo "All protobuf files are valid"

# Show help
.PHONY: help
help:
	@echo "Gateway Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  all           - Generate protobuf code and build (default)"
	@echo "  gen-proto     - Generate Go code from protobuf files"
	@echo "  build         - Build the gateway binary"
	@echo "  run           - Build and run the gateway"
	@echo "  test          - Run tests"
	@echo "  clean         - Remove generated files and binaries"
	@echo "  install-deps  - Install required protobuf plugins"
	@echo "  validate      - Validate all protobuf files"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Prerequisites:"
	@echo "  - protoc (Protocol Buffers compiler)"
	@echo "  - Go protobuf plugins (install with: make install-deps)"
	@echo ""
	@echo "Example usage:"
	@echo "  make install-deps  # Install all dependencies"
	@echo "  make validate      # Validate protobuf files"
	@echo "  make gen-proto     # Generate protobuf code from contracts"
	@echo "  make build         # Build the gateway"
	@echo "  make run           # Run the gateway"
	@echo ""
	@echo "Note: Protobuf files are located in ../contracts/ directory"

# Check if protoc is installed
.PHONY: check-protoc
check-protoc:
	@which $(PROTOC) > /dev/null || (echo "Error: protoc not found. Please install Protocol Buffers compiler." && exit 1)
	@echo "protoc found: $$($(PROTOC) --version)"

# List all proto files
.PHONY: list-proto
list-proto:
	@echo "Found protobuf files in contracts directory:"
	@for proto_file in $(PROTO_FILES); do \
		echo "  $$proto_file"; \
	done