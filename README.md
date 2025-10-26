# Ingestion Plane

An intelligent log processing system that performs online template mining, smart sampling, and semantic indexing while maintaining dual Loki instances for raw and processed log storage.

## Overview

The Ingestion Plane is a microservices-based architecture designed to optimize log management costs while preserving searchability and debuggability. It achieves **60-90% log volume reduction** through intelligent sampling while maintaining complete raw logs for compliance.

### Key Features

- **Dual Storage Strategy**: Separate raw (compliance) and processed (analysis) Loki instances
- **Multi-Protocol Ingestion**: OTLP, Loki Push API, and JSON endpoints with dual publishing
- **Online Template Mining**: Real-time pattern discovery using Drain3 algorithm
- **Smart Sampling**: Intelligent keep/suppress decisions preserving signal while reducing noise
- **Semantic Search**: Natural language queries over log patterns via vector embeddings
- **Cost Optimization**: Significant reduction in storage and ingestion costs
- **Complete Audit Trail**: All raw logs preserved for compliance and debugging

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       LOG SOURCES                            │
│     Promtail / Vector / OTLP / Loki API / JSON              │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   GATEWAY (Go:8001)                          │
│  Multi-Protocol → Normalize → Pipeline Orchestration        │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              INGESTION LAYER                         │   │
│  │  /loki/api/v1/push  /v1/ingest/json  /v1/ingest/otlp │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │   │
│  │  │   Loki API  │  │   JSON API  │  │   OTLP API  │   │   │
│  │  │             │  │             │  │             │   │   │
│  │  │ Raw → Loki  │  │ Raw → Loki  │  │ Raw → Loki  │   │   │
│  │  │ Raw → Proc  │  │ Raw → Proc  │  │ Raw → Proc  │   │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘   │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              PROCESSING PIPELINE                     │   │
│  │  Raw Logs → Normalize → Mine → Sample → Index       │   │
│  └─────────────────────────────────────────────────────┘   │
└──┬─────────┬─────────┬─────────┬─────────┬─────────────────┘
   │         │         │         │         │
   │         ▼         ▼         ▼         │
   │    ┌────────┐ ┌────────┐ ┌────────┐  │
   │    │ MINER  │ │SAMPLER │ │ INDEX  │  │
   │    │:50051  │ │:50060  │ │ FEED   │  │
   │    │Python  │ │  Go    │ │:50070  │  │
   │    │Drain3  │ │Decision│ │  Go    │  │
   │    │Pattern │ │Keep/   │ │Vector  │  │
   │    │Mining  │ │Suppress│ │Search  │  │
   │    └────────┘ └────────┘ └────────┘  │
   │         │         │         │         │
   │         └─────────┴─────────┘         │
   │                   │                   │
   │                   ▼                   │
   │             ┌──────────┐              │
   │             │  REDIS   │              │
   │             │Templates │              │
   │             │  State   │              │
   │             └──────────┘              │
   │                   │                   │
   └──────────┬────────┴──────────┬────────┘
              │                   │
              ▼                   ▼
        ┌──────────┐        ┌──────────┐
        │ Loki-Raw │        │   Loki   │
        │  :3101   │        │(Processed│
        │ 7-day    │        │  :3100   │
        │ Raw Logs │        │ 30-day   │
        │Complete  │        │ Sampled  │
        │Unmodified│        │Enriched  │
        └──────────┘        └──────────┘
              │                   │
              ▼                   ▼
        ┌──────────┐        ┌──────────┐
        │Compliance│        │Analytics │
        │Debugging │        │Dashboards│
        │Audit     │        │Search    │
        └──────────┘        └──────────┘
```

## Components

### 1. Gateway Service (Go)
**Port:** 8001 (local) / 8080 (prod)  
**Purpose:** Primary ingestion point and pipeline orchestrator

**Features:**
- **Multi-Protocol Ingestion**: OTLP, Loki API, JSON endpoints
- **Dual Log Publishing**: All protocols send raw logs to Loki-Raw + processed logs to Loki
- **Pipeline Orchestration**: Raw → Normalize → Mine → Sample → Index
- **PII Redaction**: Automatic sensitive data masking
- **Dual Loki Management**: Separate raw (compliance) and processed (analytics) storage

[📖 Detailed Documentation](docs/_docs/reference/gateway-service.md)

### 2. Miner Service (Python)
**Port:** 50051 (gRPC)  
**Purpose:** Online log template discovery using Drain3

**Features:**
- Real-time pattern clustering
- Deterministic template ID generation
- Variable masking (numbers, IPs, UUIDs, etc.)
- Template persistence in Redis
- Configurable similarity thresholds

[📖 Detailed Documentation](docs/_docs/reference/component-services.md#miner-service-python)

### 3. Sampler Service (Go)
**Port:** 50060 (gRPC)  
**Purpose:** Intelligent keep/suppress decisions

**Features:**
- Multi-criteria sampling (severity, novelty, spikes, budget)
- Power-of-two logarithmic sampling
- Namespace-specific enforcement rules
- Shadow mode for testing
- Real-time counter tracking

[📖 Detailed Documentation](docs/_docs/reference/component-services.md#sampler-service-go)

### 4. IndexFeed Service (Go)
**Port:** 50070 (gRPC)  
**Purpose:** Vector embedding generation and semantic indexing

**Features:**
- Template-to-vector embedding conversion
- Qdrant vector storage
- Semantic similarity search
- Metadata filtering (service, env, severity)
- Batch processing optimization

[📖 Detailed Documentation](docs/_docs/reference/component-services.md#indexfeed-service-go)

### 5. Planner Service (Go)
**Port:** 50080 (gRPC)  
**Purpose:** Natural language query translation to LogQL

**Features:**
- Query embedding generation
- Template matching via semantic search
- LogQL query construction
- Multi-template aggregation
- Query explanation mode

[📖 Detailed Documentation](docs/_docs/reference/component-services.md#planner-service-go)

## Dual Loki Architecture

### Loki-Raw (Port 3101)
- **Retention:** 7 days
- **Content:** Complete, unmodified logs from ALL protocols
- **Labels:** `type="raw"`, `gateway="true"`, plus original labels
- **Protocols:** Loki API, JSON API, OTLP API
- **Use Cases:** Compliance, debugging, historical reconstruction, audit trails

### Loki (Processed) (Port 3100)
- **Retention:** 30 days
- **Content:** Sampled, enriched logs (60-90% reduction)
- **Labels:** `type="processed"`, `gateway="true"`, plus template metadata
- **Processing:** PII redaction, normalization, template mining, smart sampling
- **Use Cases:** Production queries, dashboards, semantic search, analytics

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.21+ (for Gateway, Sampler, IndexFeed, Planner)
- Python 3.10+ (for Miner)
- Poetry (Python dependency management)

### Setup

```bash
# Clone repository
git clone <repository-url>
cd ingestion-plane

# Start infrastructure (Redis, PostgreSQL, Qdrant, Loki, Grafana)
cd deploy
docker-compose up -d

# Wait for services to be healthy
docker-compose ps

# Install Python dependencies (Miner)
cd ../miner
poetry install

# Generate protobuf contracts
cd ../contracts
make gen-python
make gen-go

# Build Gateway
cd ../gateway
go build -o gateway cmd/main.go

# Run Services (in separate terminals)
cd ../miner && poetry run python main.py      # Terminal 1
cd ../sampler && go run main.go               # Terminal 2
cd ../indexfeed && go run main.go             # Terminal 3
cd ../gateway && ./gateway -config config-local.yaml  # Terminal 4
```

### Test Ingestion

```bash
# Test JSON API (sends to both raw and processed)
curl -X POST http://localhost:8001/v1/ingest/json \
  -H "Content-Type: application/json" \
  -d '{
    "records": [
      {
        "timestamp": "2024-01-01T00:00:00Z",
        "labels": {"service": "api", "env": "dev", "severity": "info"},
        "payload": "User 12345 logged in successfully",
        "format_hint": "text"
      }
    ]
  }'

# Test Loki API (sends to both raw and processed)
curl -X POST http://localhost:8001/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"service": "api", "env": "dev"},
        "values": [
          ["1704067200000000000", "User authentication failed"]
        ]
      }
    ]
  }'

# Verify raw logs (Loki-Raw)
curl -s 'http://localhost:3101/loki/api/v1/query?query={service="api"}' | jq

# Verify processed logs (Loki)
curl -s 'http://localhost:3100/loki/api/v1/query?query={service="api"}' | jq

# Check Grafana
open http://localhost:3000
# Login: admin / admin
# Explore → Select "Loki (Processed)" or "Loki (Raw)"
```

## Configuration

### Gateway (`gateway/config-local.yaml`)

```yaml
server:
  http:
    port: "8001"

# Processed logs (sampled)
loki:
  addr: "http://localhost:3100"
  labels:
    static:
      gateway: "true"
      type: "processed"

# Raw logs (unmodified)
loki_raw:
  addr: "http://localhost:3101"
  labels:
    static:
      gateway: "true"
      type: "raw"

# Service connections
miner:
  addr: "localhost:50051"
sampler:
  addr: "localhost:50060"
indexfeed:
  addr: "localhost:50070"
```

### Miner (`miner/drain3.ini`)

```ini
[DRAIN]
sim_th = 0.4        # Similarity threshold
depth = 4           # Tree depth
max_clusters = 1000 # Maximum templates
```

### Sampler (Gateway `config-local.yaml`)

```yaml
enforcement:
  debug: true   # Sample debug logs
  info: false   # Keep all info logs
  warn: false   # Keep all warn logs
  error: false  # Always keep errors
```

## Deployment

### Docker Compose (Development)

```bash
cd deploy
docker-compose up -d
```

Includes:
- Redis (6379)
- PostgreSQL (5432)
- Qdrant (6333, 6334)
- Loki (3100) - Processed
- Loki-Raw (3101) - Raw
- Grafana (3000)

### Kubernetes (Production)

See `deploy/k8s/` directory for Kubernetes manifests:
- StatefulSets for stateful services
- Deployments for stateless services
- Services for networking
- ConfigMaps for configuration
- PersistentVolumeClaims for storage

## Monitoring

### Prometheus Metrics

All services expose `/metrics` endpoints:
- Gateway: `http://localhost:8001/metrics`
- Services provide gRPC health checks

### Grafana Dashboards

Access Grafana at `http://localhost:3000`:
- **Ingestion Overview**: Throughput, latency, error rates
- **Sampling Decisions**: Keep/suppress ratios, reasons
- **Template Discovery**: New patterns, cluster growth
- **Loki Health**: Ingestion rates, buffer usage

### Key Metrics

**Gateway:**
- `gateway_ingest_requests_total{protocol,status}`
- `gateway_loki_enqueued_total{sink,severity}`
- `gateway_loki_dropped_total{sink,reason}`

**Miner:**
- `miner_templates_discovered_total`
- `miner_processing_latency_seconds`

**Sampler:**
- `sampler_decisions_total{action,reason}`
- `sampler_kept_logs_total{severity}`

## Performance

### Throughput
- **Gateway:** 50K+ logs/sec per instance
- **Miner:** 10K+ operations/sec
- **Sampler:** 100K+ decisions/sec
- **IndexFeed:** 5K+ embeddings/sec

### Latency (p99)
- **End-to-end:** < 100ms
- **Gateway ingestion:** < 10ms
- **Mining:** < 20ms
- **Sampling:** < 5ms

### Resource Usage (per instance)
- **Gateway:** 512MB-2GB RAM, 1-4 CPUs
- **Miner:** 1-4GB RAM, 1-2 CPUs
- **Sampler:** 512MB-1GB RAM, 1-2 CPUs
- **IndexFeed:** 1-2GB RAM, 1-2 CPUs

## Documentation

### Getting Started
- [System Architecture](docs/_docs/learn/architecture.md)
- [Getting Started Guide](docs/_docs/implement/getting-started.md)
- [User Guide](docs/_docs/implement/user-guide.md)

### Reference
- [Gateway Service](docs/_docs/reference/gateway-service.md)
- [Component Services](docs/_docs/reference/component-services.md)
- [API Reference](docs/_docs/reference/api-reference.md)
- [Data Contracts](docs/_docs/reference/data-contracts.md)

### Operations
- [Troubleshooting](docs/_docs/implement/troubleshooting.md)
- [Configuration Guide](docs/_docs/implement/user-guide.md)

## Development

### Project Structure

```
ingestion-plane/
├── gateway/           # Gateway service (Go)
│   ├── cmd/          # Main entry point
│   ├── pkg/          # Core packages
│   ├── internal/     # Internal packages
│   └── config*.yaml  # Configuration
├── miner/            # Miner service (Python)
│   ├── main.py       # Main service
│   └── drain3.ini    # Drain3 config
├── sampler/          # Sampler service (Go)
│   └── main.go
├── indexfeed/        # IndexFeed service (Go)
│   └── main.go
├── planner/          # Planner service (Go)
│   └── main.go
├── contracts/        # Protobuf contracts
│   ├── common/       # Shared types
│   ├── miner/        # Miner service
│   ├── sampler/      # Sampler service
│   └── indexfeed/    # IndexFeed service
├── deploy/           # Deployment configs
│   ├── docker-compose.yml
│   ├── grafana/      # Dashboards
│   └── loki/         # Loki configs
└── docs/             # Documentation (Jekyll)
```

### Building

```bash
# Gateway
cd gateway && make build

# Miner (Python)
cd miner && poetry install

# Sampler
cd sampler && go build

# IndexFeed
cd indexfeed && go build

# Planner
cd planner && go build
```

### Testing

```bash
# Gateway unit tests
cd gateway && make test

# Gateway integration tests
cd gateway && make test-integration

# Load testing
cd gateway && make test-load
```

## Troubleshooting

### Common Issues

**High Memory Usage (Gateway)**
- Reduce `loki.max_buffer_bytes`
- Increase `loki.flush_interval` frequency
- Scale horizontally

**Missing Logs**
- Check sampler enforcement rules
- Verify Loki connectivity
- Review `gateway_loki_dropped_total` metric

**Slow Queries**
- Add indexes in Qdrant
- Optimize LogQL queries
- Use template filters
- Enable query caching

**Template Explosion**
- Adjust Drain3 `sim_th` (increase for fewer clusters)
- Review masking patterns
- Increase `max_clusters` limit

See [Troubleshooting Guide](docs/_docs/implement/troubleshooting.md) for detailed solutions.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

### Code Style

- **Go:** `gofmt` and `golint`
- **Python:** `black` and `pylint`
- **Commits:** Conventional Commits format

## License

[MIT License](LICENSE)

## Contact

- **Issues:** GitHub Issues
- **Discussions:** GitHub Discussions
- **Documentation:** [Full Documentation](docs/)

## Acknowledgments

- **Drain3** algorithm for log parsing
- **Loki** for log storage and querying
- **Qdrant** for vector search
- **OpenTelemetry** for observability standards
