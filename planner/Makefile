.PHONY: help build run test docker-build docker-run docker-push clean

# Variables
IMAGE_NAME ?= planner-service
IMAGE_TAG ?= latest
REGISTRY ?= ghcr.io/kumarabd/ingestion-plane
HTTP_PORT ?= 8080
BINARY_NAME ?= planner

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	go build -o $(BINARY_NAME) ./cmd/main.go

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
		--name planner \
		-p $(HTTP_PORT):8080 \
		-e QDRANT_ADDR=localhost:6334 \
		-e PG_URI=postgres://postgres:postgres@localhost:5432/ingestion_plane \
		$(IMAGE_NAME):$(IMAGE_TAG)

docker-stop: ## Stop Docker container
	docker stop planner || true
	docker rm planner || true

docker-push: ## Push Docker image to registry
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-logs: ## View Docker container logs
	docker logs -f planner

clean: ## Clean build artifacts
	rm -f $(BINARY_NAME)
	go clean

.DEFAULT_GOAL := help

