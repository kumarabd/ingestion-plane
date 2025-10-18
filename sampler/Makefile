.PHONY: help build run test docker-build docker-run docker-push clean

# Variables
IMAGE_NAME ?= sampler-service
IMAGE_TAG ?= latest
REGISTRY ?= ghcr.io/kumarabd/ingestion-plane
GRPC_PORT ?= 50060
BINARY_NAME ?= sampler

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
		--name sampler \
		-p $(GRPC_PORT):50060 \
		-e REDIS_URL=redis://localhost:6379 \
		$(IMAGE_NAME):$(IMAGE_TAG)

docker-stop: ## Stop Docker container
	docker stop sampler || true
	docker rm sampler || true

docker-push: ## Push Docker image to registry
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-logs: ## View Docker container logs
	docker logs -f sampler

clean: ## Clean build artifacts
	rm -f $(BINARY_NAME)
	go clean

.DEFAULT_GOAL := help


