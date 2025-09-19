# Ingestion Plane - Contracts Generation Makefile
# Generates Go and Rust stubs from protobuf definitions

# Variables
PROTO_DIR := contracts
GO_OUT_DIR := generated/go
RUST_OUT_DIR := generated/rust
PROTOC := protoc
PROTOC_GO_PLUGIN := protoc-gen-go
PROTOC_GO_GRPC_PLUGIN := protoc-gen-go-grpc
PROTOC_RUST_PLUGIN := protoc-gen-rust
PROTOC_RUST_GRPC_PLUGIN := protoc-gen-rust-grpc

# Go module path
GO_MODULE := github.com/ingestion-plane/contracts

# Rust crate name
RUST_CRATE := ingestion-plane-contracts

# Find all .proto files
PROTO_FILES := $(shell find $(PROTO_DIR) -name "*.proto")

# Default target
.PHONY: all
all: gen-go gen-rust

# Generate Go stubs
.PHONY: gen-go
gen-go: $(GO_OUT_DIR)
	@echo "Generating Go stubs..."
	@for proto_file in $(PROTO_FILES); do \
		echo "Processing $$proto_file..."; \
		$(PROTOC) \
			--proto_path=$(PROTO_DIR) \
			--go_out=$(GO_OUT_DIR) \
			--go_opt=paths=source_relative \
			--go-grpc_out=$(GO_OUT_DIR) \
			--go-grpc_opt=paths=source_relative \
			$$proto_file; \
	done
	@echo "Go stubs generated in $(GO_OUT_DIR)"

# Generate Rust stubs
.PHONY: gen-rust
gen-rust: $(RUST_OUT_DIR)
	@echo "Generating Rust stubs..."
	@for proto_file in $(PROTO_FILES); do \
		echo "Processing $$proto_file..."; \
		$(PROTOC) \
			--proto_path=$(PROTO_DIR) \
			--rust_out=$(RUST_OUT_DIR) \
			--grpc-rust_out=$(RUST_OUT_DIR) \
			$$proto_file; \
	done
	@echo "Rust stubs generated in $(RUST_OUT_DIR)"

# Create output directories
$(GO_OUT_DIR):
	@mkdir -p $(GO_OUT_DIR)

$(RUST_OUT_DIR):
	@mkdir -p $(RUST_OUT_DIR)

# Clean generated files
.PHONY: clean
clean:
	@echo "Cleaning generated files..."
	@rm -rf $(GO_OUT_DIR) $(RUST_OUT_DIR)
	@echo "Clean complete"

# Install Go protobuf plugins
.PHONY: install-go-deps
install-go-deps:
	@echo "Installing Go protobuf plugins..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Go protobuf plugins installed"

# Install Rust protobuf plugins
.PHONY: install-rust-deps
install-rust-deps:
	@echo "Installing Rust protobuf plugins..."
	@cargo install protoc-gen-rust
	@cargo install protoc-gen-rust-grpc
	@echo "Rust protobuf plugins installed"

# Install all dependencies
.PHONY: install-deps
install-deps: install-go-deps install-rust-deps

# Validate protobuf files
.PHONY: validate
validate:
	@echo "Validating protobuf files..."
	@for proto_file in $(PROTO_FILES); do \
		echo "Validating $$proto_file..."; \
		$(PROTOC) --proto_path=$(PROTO_DIR) --descriptor_set_out=/dev/null $$proto_file; \
	done
	@echo "All protobuf files are valid"

# Show help
.PHONY: help
help:
	@echo "Ingestion Plane - Contracts Generation"
	@echo ""
	@echo "Available targets:"
	@echo "  all           - Generate both Go and Rust stubs (default)"
	@echo "  gen-go        - Generate Go stubs from protobuf files"
	@echo "  gen-rust      - Generate Rust stubs from protobuf files"
	@echo "  clean         - Remove all generated files"
	@echo "  install-deps  - Install all required protobuf plugins"
	@echo "  install-go-deps   - Install Go protobuf plugins"
	@echo "  install-rust-deps - Install Rust protobuf plugins"
	@echo "  validate      - Validate all protobuf files"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Prerequisites:"
	@echo "  - protoc (Protocol Buffers compiler)"
	@echo "  - Go protobuf plugins (install with: make install-go-deps)"
	@echo "  - Rust protobuf plugins (install with: make install-rust-deps)"
	@echo ""
	@echo "Example usage:"
	@echo "  make install-deps  # Install all dependencies"
	@echo "  make validate      # Validate protobuf files"
	@echo "  make gen-go        # Generate Go stubs"
	@echo "  make gen-rust      # Generate Rust stubs"
	@echo "  make all           # Generate both Go and Rust stubs"

# Check if protoc is installed
.PHONY: check-protoc
check-protoc:
	@which $(PROTOC) > /dev/null || (echo "Error: protoc not found. Please install Protocol Buffers compiler." && exit 1)
	@echo "protoc found: $$($(PROTOC) --version)"

# List all proto files
.PHONY: list-proto
list-proto:
	@echo "Found protobuf files:"
	@for proto_file in $(PROTO_FILES); do \
		echo "  $$proto_file"; \
	done
