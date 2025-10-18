---
layout: page
title: System Architecture
permalink: /docs/learn/architecture/
---

# System Architecture

The Ingestion Plane is a sophisticated log processing system that performs intelligent log mining, sampling, and indexing. It consists of five core services working together to provide cost-effective, searchable log management.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           LOG SOURCES                                        │
│   Promtail / Vector / OTLP Collectors / Direct Loki Push API               │
└────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          GATEWAY SERVICE (Go)                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Multi-Protocol Ingestion:                                          │   │
│  │  • OTLP (OpenTelemetry)                                             │   │
│  │  • Loki Push API (/loki/api/v1/push)                                │   │
│  │  • JSON API (/api/v1/logs)                                          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                             │                                                │
│                             ▼                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Raw Log Forwarding (Loki Push API only)                           │   │
│  │  • Zero modifications                                               │   │
│  │  • Direct passthrough                                               │   │
│  └───────────────────────────┬─────────────────────────────────────────┘   │
│                               │                                              │
│                               ▼                                              │
│                         ┌──────────┐                                         │
│                         │ Loki-Raw │ ◄─── Raw, unprocessed logs            │
│                         └──────────┘                                         │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Processing Pipeline (All Sources):                                 │   │
│  │  1. Normalize & Redact (PII masking, format standardization)        │   │
│  │  2. Send to Miner (gRPC) → Template Discovery                       │   │
│  │  3. Send to Sampler (gRPC) → Keep/Suppress Decision                │   │
│  │  4. Send to IndexFeed (gRPC) → Semantic Indexing                    │   │
│  │  5. Send kept logs to Loki (Processed)                              │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└──────────────┬──────────────┬──────────────┬──────────────┬────────────────┘
               │              │              │              │
               ▼              ▼              ▼              ▼
         ┌─────────┐    ┌─────────┐    ┌──────────┐   ┌──────────┐
         │  MINER  │    │ SAMPLER │    │ INDEXFEED│   │   LOKI   │
         │(Python) │    │  (Go)   │    │   (Go)   │   │(Processed)│
         │         │    │         │    │          │   │          │
         │ Drain3  │    │Decision │    │Embedding │   │ Sampled  │
         │Template │    │ Engine  │    │Generator │   │   Logs   │
         │ Mining  │    │         │    │          │   │          │
         └─────────┘    └─────────┘    └──────────┘   └──────────┘
               │              │              │
               │              │              ▼
               │              │         ┌──────────┐
               │              │         │ Qdrant   │
               │              │         │  Vector  │
               │              │         │   Store  │
               │              │         └──────────┘
               │              │
               ▼              ▼
         ┌─────────────────────────┐
         │  REDIS (Shared State)   │
         │  • Template Catalog     │
         │  • Counters & Stats     │
         │  • Novelty Tracking     │
         │  • Spike Detection      │
         └─────────────────────────┘
```

## Data Flow Paths

### Path 1: Loki Push API → Raw Logs (Zero Processing)

```
Loki Client → Gateway (/loki/api/v1/push) → Parse Request → Loki-Raw
```

**Characteristics:**
- **Zero modifications**: Logs forwarded exactly as received
- **No label additions**: Original labels preserved
- **Fast path**: Minimal latency overhead
- **Use case**: Debugging, compliance, raw data retention

### Path 2: All Sources → Processed Logs (Full Pipeline)

```
Log Source → Gateway → Normalize → Miner → Sampler → Loki (Processed)
                         │           │         │
                         │           │         └→ IndexFeed → Qdrant
                         │           └→ Redis (Templates)
                         └→ Redis (State)
```

**Characteristics:**
- **Template discovery**: Patterns identified and cataloged
- **Smart sampling**: 60-90% reduction through intelligent filtering
- **Semantic indexing**: Templates converted to embeddings
- **Label enrichment**: Added metadata (gateway="true", type="processed")
- **Use case**: Production queries, cost optimization, semantic search

## Dual Loki Architecture

The system maintains two separate Loki instances with different purposes:

### Loki-Raw (Port 3101)

**Purpose:** Complete, unmodified log retention

**Configuration:**
- 7-day retention (shorter than processed)
- Higher ingestion limits (20 MB/s)
- No static label additions
- Labels: `type="raw"`

**Use Cases:**
- Regulatory compliance and auditing
- Full historical reconstruction
- Debugging gateway processing issues
- Comparison with processed logs

### Loki (Processed) (Port 3100)

**Purpose:** Sampled, enriched logs for production queries

**Configuration:**
- 30-day retention (longer than raw)
- Standard ingestion limits (10 MB/s)
- Static label additions (gateway="true", type="processed")
- Labels: `type="processed"`

**Use Cases:**
- Production log queries and dashboards
- Incident response and troubleshooting
- Cost-optimized long-term storage
- Integration with semantic search

## Core Services

### 1. Gateway Service (Go)

**Repository:** `/gateway`

**Responsibilities:**
- Multi-protocol log ingestion (OTLP, Loki API, JSON)
- Raw log forwarding to Loki-Raw (Loki API only)
- Log normalization and PII redaction
- Pipeline orchestration (Miner → Sampler → IndexFeed)
- Buffering and backpressure management
- Metrics and observability

**Key Features:**
- Asynchronous processing with bounded channels
- Protocol-specific handlers (OTLP, Loki, JSON)
- Configurable pipeline stages
- Redis caching for performance
- Dual Loki sink management

**Configuration:**
- HTTP server on port 8001 (local) / 8080 (prod)
- gRPC clients for Miner, Sampler, IndexFeed
- Two Loki sinks (raw and processed)
- Redis connection for caching

**APIs:**
- `POST /v1/ingest` - Auto-detect protocol
- `POST /loki/api/v1/push` - Loki Push API
- `POST /v1/ingest/otlp` - OTLP logs
- `POST /api/v1/logs` - JSON logs
- `GET /healthz` - Health check
- `GET /metrics` - Prometheus metrics

### 2. Miner Service (Python)

**Repository:** `/miner`

**Responsibilities:**
- Online log template discovery using Drain3 algorithm
- Template clustering and centroid management
- Deterministic template ID generation
- Template catalog maintenance in Redis
- Event emission for new/updated templates

**Algorithm: Drain3**
- Depth-first tree traversal for log grouping
- Tokenization and masking of variables
- Similarity threshold for cluster assignment
- Dynamic template merging and splitting
- Support count tracking

**Key Features:**
- Configurable similarity threshold
- Max cluster depth control
- Template persistence in Redis
- gRPC server for mining requests
- Batch processing support

**Configuration:**
- gRPC server on port 50051
- Redis for template storage
- Drain3 parameters (depth, similarity)
- Template TTL and eviction policies

**Outputs:**
- `template_id`: Unique hash of canonical template
- `template_text`: Human-readable pattern
- `cluster_id`: Internal clustering identifier
- `masked_tokens`: Tokenized representation

### 3. Sampler Service (Go)

**Repository:** `/sampler`

**Responsibilities:**
- Intelligent keep/suppress decisions
- Multi-criteria sampling logic
- Budget and quota enforcement
- Spike detection and novelty tracking
- Keep reason attribution

**Decision Criteria (Priority Order):**
1. **High Severity**: Always keep ERROR/FATAL logs
2. **Novel Templates**: Keep new patterns (< 24h old)
3. **Spike Detection**: Keep more during unusual activity
4. **Warmup Period**: Keep first N observations
5. **Logarithmic Sampling**: Power-of-two counts (1,2,4,8...)
6. **Steady-State**: Regular sampling for established patterns
7. **Budget Guard**: Backpressure when limits exceeded
8. **Suppress**: Default action for noise

**Key Features:**
- Configurable enforcement rules per severity
- Namespace-based policy overrides
- Real-time counter updates in Redis
- Shadow mode for testing
- Comprehensive metrics

**Configuration:**
- gRPC server on port 50060
- Redis for state management
- Enforcement rules (debug, info, warn, error)
- Per-namespace overrides

**Outputs:**
- `action`: KEEP or SUPPRESS
- `keep_reason`: Why the log was kept
- `sample_rate`: Applied sampling rate
- `policy_version`: Policy identifier

### 4. IndexFeed Service (Go)

**Repository:** `/indexfeed`

**Responsibilities:**
- Template embedding generation
- Vector storage in Qdrant
- Semantic search capabilities
- Template metadata indexing
- Query result ranking

**Key Features:**
- Text embedding using sentence transformers
- Vector similarity search
- Metadata filtering (service, env, severity)
- Batch embedding generation
- Template catalog integration

**Configuration:**
- gRPC server on port 50070
- Qdrant connection for vectors
- Embedding model configuration
- Search parameters (top-k, threshold)

**Event Processing:**
- `TEMPLATE_NEW`: Index new template
- `TEMPLATE_UPDATE`: Re-index updated template
- `TEMPLATE_SPIKE`: Priority indexing for spikes

### 5. Planner Service (Go)

**Repository:** `/planner`

**Responsibilities:**
- Natural language query parsing
- LogQL query generation
- Query optimization and planning
- Template-to-query translation
- Result assembly and ranking

**Key Features:**
- Semantic query interpretation
- Template matching via IndexFeed
- LogQL query construction
- Multi-template query aggregation
- Explain mode for transparency

**Query Flow:**
1. Parse natural language query
2. Generate embeddings
3. Search templates in IndexFeed
4. Convert templates to LogQL
5. Execute against Loki
6. Assemble and rank results

## State Management

### Redis Schema

```
# Templates (from Miner)
templates:{template_id} → {template_text, cluster_id, first_seen, last_seen}
templates:by_service:{service} → set(template_id)

# Counters (from Sampler)
counter:{template_id}:{severity} → count
counter:{template_id}:last_seen → timestamp

# Novelty Tracking
novelty:{template_id} → first_seen (TTL: 24h)

# Spike Detection
spike:baseline:{key} → ewma_value
spike:p95:{key} → p95_value

# Cache (from Gateway)
cache:{key} → {cached_value} (TTL: configurable)
```

## Deployment

### Docker Compose

The system is deployed using Docker Compose with the following services:

**Infrastructure:**
- Redis (port 6379) - Shared state
- PostgreSQL (port 5432) - Metadata storage
- Qdrant (ports 6333, 6334) - Vector store
- Loki (port 3100) - Processed logs
- Loki-Raw (port 3101) - Raw logs
- Grafana (port 3000) - Visualization

**Services:**
- Gateway (port 8001/8080)
- Miner (port 50051)
- Sampler (port 50060)
- IndexFeed (port 50070)

### Health Checks

All services provide health check endpoints:
- Gateway: `GET /healthz`
- gRPC services: gRPC health check protocol

### Monitoring

**Metrics:**
- Prometheus metrics on `/metrics` endpoints
- Grafana dashboards for visualization
- Loki queries for log analysis

**Key Metrics:**
- Ingestion rate (logs/sec)
- Sampling rate (kept vs suppressed)
- Template discovery rate
- Buffer utilization
- Processing latency
- Error rates

## Configuration

### Gateway Configuration

```yaml
server:
  http:
    host: "0.0.0.0"
    port: "8001"
    
miner:
  addr: "localhost:50051"
  timeout: "500ms"
  
sampler:
  addr: "localhost:50060"
  timeout: "500ms"
  
loki:  # Processed logs
  addr: "http://localhost:3100"
  flush_interval: "400ms"
  labels:
    static:
      gateway: "true"
      type: "processed"
      
loki_raw:  # Raw logs
  addr: "http://localhost:3101"
  flush_interval: "400ms"
  labels:
    static:
      gateway: "true"
      type: "raw"
      
indexfeed:
  addr: "localhost:50070"
  timeout: "500ms"
```

### Miner Configuration (drain3.ini)

```ini
[DRAIN]
sim_th = 0.4
depth = 4
max_children = 100
max_clusters = 1000

[MASKING]
masking = [
    {"regex_pattern": "\\d+", "mask_with": "<NUM>"},
    {"regex_pattern": "[0-9a-fA-F]{8,}", "mask_with": "<HEX>"}
]
```

### Sampler Configuration

```yaml
enforcement:
  debug: true   # Enforce sampling on debug logs
  info: false   # Don't enforce on info
  warn: false   # Don't enforce on warn
  error: false  # Always keep error logs
  by_namespace:
    staging: true
    production: false
```

## Performance Characteristics

### Throughput

- **Gateway**: 50K+ logs/sec per instance
- **Miner**: 10K+ mining operations/sec
- **Sampler**: 100K+ decisions/sec
- **IndexFeed**: 5K+ embeddings/sec

### Latency

- **End-to-end**: < 100ms (p99)
- **Gateway ingestion**: < 10ms
- **Mining**: < 20ms per log
- **Sampling**: < 5ms per log
- **Indexing**: < 50ms per template

### Resource Usage

- **Gateway**: 512MB-2GB RAM, 1-4 CPU cores
- **Miner**: 1-4GB RAM (template storage), 1-2 CPU cores
- **Sampler**: 512MB-1GB RAM, 1-2 CPU cores
- **IndexFeed**: 1-2GB RAM, 1-2 CPU cores
- **Redis**: 2-8GB RAM (depends on template count)

## Scaling Strategies

### Horizontal Scaling

- **Gateway**: Deploy multiple instances behind load balancer
- **Miner**: Shard by service name or hash
- **Sampler**: Stateless, scales linearly
- **IndexFeed**: Shard by template ID

### Vertical Scaling

- **Redis**: Increase memory for more templates
- **Qdrant**: Add more nodes for vectors
- **Loki**: Add more ingesters and queriers

### Cost Optimization

- **Sampling Rate**: Achieves 60-90% reduction
- **Short-term Raw**: 7-day raw logs
- **Long-term Processed**: 30-day sampled logs
- **Tiered Storage**: Move old logs to object storage

## Security Considerations

### PII Redaction

- Configurable redaction rules in Gateway
- Pattern-based masking (emails, IPs, etc.)
- Hash-based pseudonymization
- Audit trail for redactions

### Network Security

- TLS for all gRPC connections
- mTLS for service-to-service auth
- API authentication tokens
- Network policies for service isolation

### Data Privacy

- GDPR-compliant log retention
- Right to erasure support
- Data minimization via sampling
- Encrypted storage options

## Troubleshooting

### Common Issues

**High Backpressure:**
- Increase buffer sizes
- Scale up processing services
- Tune sampling rates

**Template Explosion:**
- Adjust Drain3 similarity threshold
- Increase max clusters
- Review masking patterns

**Missing Logs:**
- Check sampling decisions
- Verify enforcement rules
- Review budget limits
- Check Loki retention

**Slow Queries:**
- Add indexes in Qdrant
- Optimize LogQL queries
- Use template filters
- Enable query caching

## Next Steps

- [Component Specifications](../reference/component-specs/)
- [API Reference](../reference/api-reference/)
- [Getting Started Guide](../implement/getting-started/)
- [Troubleshooting Guide](../implement/troubleshooting/)

