.PHONY: help build run test docker-build docker-run docker-push clean

# Variables
IMAGE_NAME ?= indexfeed-service
IMAGE_TAG ?= latest
REGISTRY ?= ghcr.io/kumarabd/ingestion-plane
GRPC_PORT ?= 50070
BINARY_NAME ?= indexfeed

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	go build -o $(BINARY_NAME) main.go

run: build ## Run the service locally
	./$(BINARY_NAME)

test: ## Run tests
	go test -v ./...

lint: ## Run linter
	golangci-lint run

docker-build: ## Build Docker image
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-run: ## Run Docker container
	docker run -d \
		--name indexfeed \
		-p $(GRPC_PORT):50070 \
		-e QDRANT_URL=http://localhost:6333 \
		-e POSTGRES_URL=postgresql://postgres:postgres@localhost:5432/ingestion_plane \
		$(IMAGE_NAME):$(IMAGE_TAG)

docker-stop: ## Stop Docker container
	docker stop indexfeed || true
	docker rm indexfeed || true

docker-push: ## Push Docker image to registry
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-logs: ## View Docker container logs
	docker logs -f indexfeed

clean: ## Clean build artifacts
	rm -f $(BINARY_NAME)
	go clean

.DEFAULT_GOAL := help


