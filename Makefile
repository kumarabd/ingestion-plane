# Ingestion Plane - Contracts Generation Makefile
# Generates Go and Rust stubs from protobuf definitions

# Variables
PROTO_DIR := contracts
GENERATED_DIR := contracts
PROTOC := protoc
PROTOC_GO_PLUGIN := protoc-gen-go
PROTOC_GO_GRPC_PLUGIN := protoc-gen-go-grpc
PROTOC_RUST_PLUGIN := protoc-gen-prost
PROTOC_RUST_GRPC_PLUGIN := protoc-gen-tonic

# Go module path
GO_MODULE := github.com/ingestion-plane/contracts

# Rust crate name
RUST_CRATE := ingestion-plane-contracts

# Find all .proto files
PROTO_FILES := $(shell find $(PROTO_DIR) -name "*.proto")

# Default target
.PHONY: all
all: gen-go gen-rust gen-python

# Generate Go stubs
.PHONY: gen-go
gen-go:
	@echo "Generating Go stubs..."
	$(PROTOC) -I $(PROTO_DIR) \
		--go_out=paths=source_relative:$(GENERATED_DIR) \
		--go-grpc_out=paths=source_relative:$(GENERATED_DIR) \
		$(PROTO_DIR)/**/**/**.proto
	@echo "Go stubs generated in $(GENERATED_DIR)"

# Generate Rust stubs
.PHONY: gen-rust
gen-rust:
	@echo "Generating Rust stubs..."
	$(PROTOC) -I $(PROTO_DIR) \
		--plugin=protoc-gen-prost=$(shell which $(PROTOC_RUST_PLUGIN)) \
		--plugin=protoc-gen-tonic=$(shell which $(PROTOC_RUST_GRPC_PLUGIN)) \
		--prost_out=$(GENERATED_DIR) \
		--tonic_out=$(GENERATED_DIR) \
		$(PROTO_DIR)/**/**/**.proto
	@echo "Rust stubs generated in $(GENERATED_DIR)"

# Generate Python stubs
.PHONY: gen-python
gen-python:
	@echo "Generating Python stubs..."
	python3 -m grpc_tools.protoc -I $(PROTO_DIR) \
		--python_out=$(GENERATED_DIR) \
		--grpc_python_out=$(GENERATED_DIR) \
		$(PROTO_DIR)/**/**/**.proto
	@echo "Python stubs generated in $(GENERATED_DIR)"

# Create output directories
$(GENERATED_DIR):
	@mkdir -p $(GENERATED_DIR)

# Clean generated files
.PHONY: clean
clean:
	@echo "Cleaning generated files..."
	@find $(GENERATED_DIR) -name "*.pb.go" -delete
	@find $(GENERATED_DIR) -name "*_grpc.pb.go" -delete
	@find $(GENERATED_DIR) -name "*.rs" -delete
	@find $(GENERATED_DIR) -name "*.tonic.rs" -delete
	@find $(GENERATED_DIR) -name "*_pb2.py" -delete
	@find $(GENERATED_DIR) -name "*_pb2_grpc.py" -delete
	@find $(GENERATED_DIR) -name "*.swagger.json" -delete
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
	@cargo install protoc-gen-prost
	@cargo install protoc-gen-tonic
	@echo "Rust protobuf plugins installed"

# Install Python protobuf dependencies
.PHONY: install-python-deps
install-python-deps:
	@echo "Installing Python protobuf dependencies..."
	@pip3 install grpcio-tools protobuf
	@echo "Python protobuf dependencies installed"

# Install all dependencies
.PHONY: install-deps
install-deps: install-go-deps install-rust-deps install-python-deps

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
	@echo "  all           - Generate Go, Rust, and Python stubs (default)"
	@echo "  gen-go        - Generate Go stubs from protobuf files"
	@echo "  gen-rust      - Generate Rust stubs from protobuf files"
	@echo "  gen-python    - Generate Python stubs from protobuf files"
	@echo "  clean         - Remove all generated files from contracts directory"
	@echo "  install-deps  - Install all required protobuf plugins"
	@echo "  install-go-deps   - Install Go protobuf plugins"
	@echo "  install-rust-deps - Install Rust protobuf plugins"
	@echo "  install-python-deps - Install Python protobuf dependencies"
	@echo "  validate      - Validate all protobuf files"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Prerequisites:"
	@echo "  - protoc (Protocol Buffers compiler)"
	@echo "  - Go protobuf plugins (install with: make install-go-deps)"
	@echo "  - Rust protobuf plugins (install with: make install-rust-deps)"
	@echo "  - Python protobuf dependencies (install with: make install-python-deps)"
	@echo ""
	@echo "Example usage:"
	@echo "  make install-deps  # Install all dependencies"
	@echo "  make validate      # Validate protobuf files"
	@echo "  make gen-go        # Generate Go stubs in contracts directory"
	@echo "  make gen-rust      # Generate Rust stubs in contracts directory"
	@echo "  make gen-python    # Generate Python stubs in contracts directory"
	@echo "  make all           # Generate Go, Rust, and Python stubs"
	@echo "  make clean         # Clean generated files from contracts directory"

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
