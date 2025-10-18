---
layout: page
title: Getting Started
permalink: /docs/implement/getting-started/
---

# Getting Started with Ingestion Plane

This guide will help you set up and run the Ingestion Plane system, from deploying infrastructure to sending your first logs.

## Prerequisites

Before you begin, ensure you have:

- **Docker & Docker Compose**: For running infrastructure services
- **Go 1.21+**: For building Gateway, Sampler, IndexFeed, Planner
- **Python 3.10+**: For running Miner service
- **Poetry**: Python dependency management
- **Make**: For building protobuf contracts

## Quick Start (Local Development)

### 1. Start Infrastructure Services

```bash
cd deploy

# Start Redis, PostgreSQL, Qdrant, Loki (both instances), and Grafana
docker-compose up -d

# Verify all services are healthy
docker-compose ps

# Expected output:
# redis-server         Up (healthy)
# postgres-server      Up (healthy)
# qdrant-server        Up (healthy)
# loki-server          Up (healthy)
# loki-raw-server      Up (healthy)
# grafana-server       Up (healthy)
```

**Services Started:**
- Redis: `localhost:6379` - Shared state
- PostgreSQL: `localhost:5432` - Metadata storage
- Qdrant: `localhost:6333` - Vector store
- Loki (Processed): `localhost:3100` - Sampled logs (30-day retention)
- Loki-Raw: `localhost:3101` - Raw logs (7-day retention)
- Grafana: `localhost:3000` - Dashboards (admin/admin)

### 2. Generate Protobuf Contracts

```bash
cd contracts

# Generate Python stubs (for Miner)
make gen-python

# Generate Go stubs (for Gateway, Sampler, IndexFeed, Planner)
make gen-go
```

### 3. Start Miner Service (Python)

```bash
cd miner

# Install dependencies
poetry install

# Run service
poetry run python main.py

# Service starts on port 50051
# You should see: "Miner service started on 50051"
```

### 4. Start Sampler Service (Go)

```bash
cd sampler

# Build and run
go run main.go

# Service starts on port 50060
# You should see: "Sampler service started on 50060"
```

### 5. Start IndexFeed Service (Go)

```bash
cd indexfeed

# Build and run
go run main.go

# Service starts on port 50070
# You should see: "IndexFeed service started on 50070"
```

### 6. Start Gateway Service (Go)

```bash
cd gateway

# Build
make build

# Run with local configuration
./gateway -config config-local.yaml

# Gateway starts on port 8001
# You should see: "Starting HTTP server on 0.0.0.0:8001"
```

## Verify Installation

### Check Service Health

```bash
# Gateway health
curl http://localhost:8001/healthz
# Expected: {"status":"ok","time":"..."}

# Loki (Processed) health
curl http://localhost:3100/ready
# Expected: ready

# Loki-Raw health  
curl http://localhost:3101/ready
# Expected: ready

# Grafana health
curl http://localhost:3000/api/health
# Expected: {"database":"ok",...}
```

### Send Test Logs

#### JSON API

```bash
curl -X POST http://localhost:8001/api/v1/logs \
  -H "Content-Type: application/json" \
  -d '{
    "records": [
      {
        "timestamp": "2024-01-01T12:00:00Z",
        "labels": {
          "service": "api",
          "env": "dev",
          "severity": "info"
        },
        "payload": "User 12345 logged in successfully"
      }
    ]
  }'

# Expected: 200 OK
```

#### Loki Push API (Goes to Both Raw and Processed)

```bash
curl -X POST http://localhost:8001/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {
          "service": "api",
          "env": "dev",
          "severity": "info"
        },
        "values": [
          ["1704110400000000000", "User authentication successful"]
        ]
      }
    ]
  }'

# Expected: {"status":"success"}
```

### Query Logs in Grafana

1. **Open Grafana**: http://localhost:3000
2. **Login**: admin / admin
3. **Navigate to Explore**
4. **Select Datasource**:
   - **Loki (Raw)** - See unmodified logs
   - **Loki (Processed)** - See sampled, enriched logs

5. **Run Queries**:

**Raw logs:**
```logql
{type="raw", service="api"}
```

**Processed logs:**
```logql
{type="processed", service="api", gateway="true"}
```

**With template information:**
```logql
{type="processed"} | json | line_format "{% raw %}{{.message}} [{{.template_id}}]{% endraw %}"
```

## Configuration

### Gateway Configuration

Edit `gateway/config-local.yaml`:

```yaml
# Adjust ports
server:
  http:
    port: "8001"  # Change if needed

# Configure services
miner:
  addr: "localhost:50051"
sampler:
  addr: "localhost:50060"
indexfeed:
  addr: "localhost:50070"

# Loki for processed logs
loki:
  addr: "http://localhost:3100"
  labels:
    static:
      gateway: "true"
      type: "processed"

# Loki for raw logs
loki_raw:
  addr: "http://localhost:3101"
  labels:
    static:
      gateway: "true"
      type: "raw"
```

### Miner Configuration

Edit `miner/drain3.ini`:

```ini
[DRAIN]
sim_th = 0.4         # Lower = more clusters, Higher = fewer clusters
depth = 4            # Tree depth
max_clusters = 1000  # Maximum templates

[MASKING]
# Adjust masking patterns as needed
```

### Sampler Configuration

Edit enforcement rules in `gateway/config-local.yaml`:

```yaml
enforcement:
  debug: true   # Enforce sampling on DEBUG (reduce volume)
  info: false   # Keep all INFO logs
  warn: false   # Keep all WARN logs
  error: false  # Always keep ERROR logs (never sampled)
  by_namespace:
    staging: true      # Enforce in staging namespace
    production: false  # Don't enforce in production
```

## Testing the Pipeline

### 1. Send Multiple Logs

Create a test script:

```bash
#!/bin/bash
# test-logs.sh

for i in {1..100}; do
  curl -X POST http://localhost:8001/api/v1/logs \
    -H "Content-Type: application/json" \
    -d "{
      \"records\": [{
        \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
        \"labels\": {
          \"service\": \"test-app\",
          \"env\": \"dev\",
          \"severity\": \"info\"
        },
        \"payload\": \"User $i logged in successfully\"
      }]
    }" &
done

wait
echo "Sent 100 logs"
```

### 2. Verify Processing

**Check Gateway Logs:**
```bash
# In Gateway terminal, you should see:
# - "Enqueueing entries to Loki sink"
# - "Miner processing..."
# - "Sampler decision..."
# - "Successfully sent batch to Loki"
```

**Check Metrics:**
```bash
curl http://localhost:8001/metrics | grep gateway_ingest
```

**Check Grafana:**
- Go to Loki (Processed) datasource
- Query: `{type="processed", service="test-app"}`
- You should see sampled logs with template_id annotations

- Go to Loki (Raw) datasource  
- Query: `{type="raw", service="test-app"}`
- You should see all 100 original logs

### 3. Verify Template Discovery

**Check Redis:**
```bash
docker exec -it redis-server redis-cli

# List templates
KEYS template:*

# Get a template
GET template:abc123...
```

**Check Miner Logs:**
```bash
# In Miner terminal, you should see:
# - "New template discovered"
# - "Template ID: abc123..."
```

## Common Configurations

### High-Volume Production

```yaml
# gateway/config.yaml
server:
  http:
    bounds:
      max_batch: 5000  # Increase for higher throughput

loki:
  max_batch_entries: 10000
  flush_interval: "200ms"  # Faster flushing

miner:
  max_batch: 2000
  timeout: "1s"

sampler:
  max_batch: 5000
```

### Development/Testing

```yaml
# gateway/config-local.yaml
loki:
  mock_mode: true  # Print to stdout instead of Loki

miner:
  shadow_only: true  # Don't actually drop logs

enforcement:
  debug: false  # Keep all logs for testing
```

## Troubleshooting

### Services Won't Start

**Check ports are free:**
```bash
lsof -i :8001  # Gateway
lsof -i :50051 # Miner
lsof -i :50060 # Sampler
lsof -i :50070 # IndexFeed
```

**Check infrastructure:**
```bash
docker-compose ps
# All should show "Up (healthy)"
```

### Logs Not Appearing

**Check Gateway is receiving:**
```bash
curl http://localhost:8001/metrics | grep ingest_requests_total
```

**Check Loki connectivity:**
```bash
# Test processed Loki
curl http://localhost:3100/ready

# Test raw Loki
curl http://localhost:3101/ready
```

**Check sampling decisions:**
```bash
curl http://localhost:8001/metrics | grep sampler_decisions_total
```

### High Memory Usage

**Reduce buffer sizes:**
```yaml
loki:
  max_buffer_bytes: 134217728  # 128MB (half of default)
  max_buffer_entries: 500000
```

**Enable faster flushing:**
```yaml
loki:
  flush_interval: "200ms"  # Down from 400ms
```

## Next Steps

- [User Guide](user-guide/) - Detailed usage instructions
- [System Architecture](../learn/architecture/) - Understanding the system
- [Gateway Service](../reference/gateway-service/) - Gateway documentation
- [Component Services](../reference/component-services/) - Other services
- [Troubleshooting](troubleshooting/) - Common issues and solutions

## Support

- [GitHub Issues](https://github.com/kumarabd/ingestion-plane/issues)
- [Documentation](../learn/overview/)
- [API Reference](../reference/api-reference/)
