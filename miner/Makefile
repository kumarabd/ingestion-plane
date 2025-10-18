.PHONY: help build run test docker-build docker-run docker-push clean

# Variables
IMAGE_NAME ?= miner-service
IMAGE_TAG ?= latest
REGISTRY ?= ghcr.io/kumarabd/ingestion-plane
GRPC_PORT ?= 50051

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install dependencies using Poetry
	poetry install

run: ## Run the service locally
	poetry run python main.py

test: ## Run tests
	poetry run pytest

lint: ## Run linter
	poetry run black --check .
	poetry run flake8 .

format: ## Format code
	poetry run black .

docker-build: ## Build Docker image
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-run: ## Run Docker container
	docker run -d \
		--name miner \
		-p $(GRPC_PORT):50051 \
		-e MINER_REDIS_URL=redis://localhost:6379/0 \
		$(IMAGE_NAME):$(IMAGE_TAG)

docker-stop: ## Stop Docker container
	docker stop miner || true
	docker rm miner || true

docker-push: ## Push Docker image to registry
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

docker-logs: ## View Docker container logs
	docker logs -f miner

clean: ## Clean build artifacts
	rm -rf __pycache__
	rm -rf .pytest_cache
	rm -rf .coverage
	rm -rf htmlcov
	poetry env remove --all

.DEFAULT_GOAL := help


