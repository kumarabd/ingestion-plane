# Miner Service Dockerfile
# Multi-stage build for Python service with Drain3

FROM python:3.11-slim as builder

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    POETRY_VERSION=1.7.1 \
    POETRY_HOME="/opt/poetry" \
    POETRY_NO_INTERACTION=1 \
    POETRY_VIRTUALENVS_IN_PROJECT=1 \
    POETRY_VIRTUALENVS_CREATE=1

# Install poetry
RUN pip install "poetry==$POETRY_VERSION"

# Set working directory
WORKDIR /app

# Copy dependency files
COPY pyproject.toml poetry.lock ./

# Install dependencies
RUN poetry install --no-root --only main

# Copy application code
COPY miner/main.py miner/__init__.py ./
COPY drain3.ini ./
COPY contracts/ contracts/

# Final runtime stage
FROM python:3.11-slim

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    VIRTUAL_ENV=/app/.venv \
    PATH="/app/.venv/bin:$PATH"

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r miner && useradd -r -g miner miner

# Set working directory
WORKDIR /app

# Copy virtual environment and application from builder
COPY --from=builder --chown=miner:miner /app/.venv /app/.venv
COPY --from=builder --chown=miner:miner /app/main.py /app/main.py
COPY --from=builder --chown=miner:miner /app/__init__.py /app/__init__.py
COPY --from=builder --chown=miner:miner /app/drain3.ini /app/drain3.ini
COPY --from=builder --chown=miner:miner /app/contracts /app/contracts

# Switch to non-root user
USER miner

# Expose gRPC port
EXPOSE 50051

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import grpc; channel = grpc.insecure_channel('localhost:50051'); channel.close()" || exit 1

# Run the service
CMD ["python", "main.py"]


