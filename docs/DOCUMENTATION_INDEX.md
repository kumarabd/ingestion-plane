# Documentation Index

Complete guide to the Ingestion Plane documentation.

## Quick Links

- **Main README**: [/README.md](../README.md) - Project overview and quick start
- **Architecture**: [docs/learn/architecture.md](_docs/learn/architecture.md) - Detailed system architecture
- **Gateway Service**: [docs/reference/gateway-service.md](_docs/reference/gateway-service.md) - Gateway documentation
- **Component Services**: [docs/reference/component-services.md](_docs/reference/component-services.md) - Miner, Sampler, IndexFeed, Planner

## Documentation Structure

```
docs/
├── _docs/
│   ├── learn/                    # Conceptual Documentation
│   │   ├── overview.md          # High-level system overview
│   │   ├── architecture.md      # NEW: Detailed architecture with dual Loki
│   │   └── overview/
│   │       ├── problem-strategy.md
│   │       └── use-cases.md
│   │
│   ├── implement/               # Practical Guides
│   │   ├── getting-started.md   # Setup and installation
│   │   ├── user-guide.md        # Usage instructions
│   │   └── troubleshooting.md   # Common issues
│   │
│   └── reference/               # Technical Reference
│       ├── component-specs.md   # Existing component specs
│       ├── gateway-service.md   # NEW: Detailed Gateway docs
│       ├── component-services.md # NEW: Miner, Sampler, IndexFeed, Planner
│       ├── api-reference.md     # API endpoints
│       └── data-contracts.md    # Protobuf contracts
│
└── README.md                    # Documentation homepage
```

## What's New

### Architecture Documentation
**File**: `_docs/learn/architecture.md`

Comprehensive system architecture document covering:
- High-level architecture diagram
- Dual Loki setup (raw vs processed)
- Data flow paths (raw and processed)
- All five microservices (Gateway, Miner, Sampler, IndexFeed, Planner)
- State management with Redis
- Deployment and configuration
- Performance characteristics
- Security considerations
- Troubleshooting guide

### Gateway Service Documentation
**File**: `_docs/reference/gateway-service.md`

Detailed Gateway service documentation including:
- Multi-protocol ingestion (OTLP, Loki API, JSON)
- Internal architecture and components
- Dual Loki sink management
  - `Enqueue()` method for processed logs (with static labels)
  - `EnqueuePushRequest()` method for raw logs (zero modifications)
- Pipeline workers and bridges
- gRPC client configurations
- Complete configuration examples
- API reference
- Performance tuning
- Troubleshooting
- Development and deployment guides

### Component Services Documentation
**File**: `_docs/reference/component-services.md`

Documentation for all supporting services:

**Miner Service (Python/Drain3)**
- Algorithm overview
- Configuration (drain3.ini)
- Redis schema
- gRPC contract

**Sampler Service (Go)**
- Decision logic (9-step priority)
- Enforcement configuration
- Keep reasons
- Redis state management

**IndexFeed Service (Go)**
- Vector embedding generation
- Qdrant integration
- Search capabilities
- gRPC contract

**Planner Service (Go)**
- Query translation (NL → LogQL)
- Template matching
- Query execution flow

## Key Concepts

### Dual Loki Architecture

The system maintains two separate Loki instances:

1. **Loki-Raw (Port 3101)**
   - 7-day retention
   - Raw, unmodified logs
   - Loki Push API only
   - For compliance/debugging

2. **Loki (Processed) (Port 3100)**
   - 30-day retention
   - Sampled, enriched logs (60-90% reduction)
   - All sources
   - For production queries

### Data Paths

**Raw Path** (Loki API only):
```
Loki Client → Gateway → Parse → Loki-Raw (no modifications)
```

**Processed Path** (all sources):
```
Source → Gateway → Normalize → Miner → Sampler → Loki (Processed)
                                  │       │
                                  │       └→ IndexFeed → Qdrant
                                  └→ Redis (Templates, State)
```

## Component Overview

| Component | Language | Port | Purpose |
|-----------|----------|------|---------|
| **Gateway** | Go | 8001 | Multi-protocol ingestion, orchestration |
| **Miner** | Python | 50051 | Template discovery (Drain3) |
| **Sampler** | Go | 50060 | Keep/suppress decisions |
| **IndexFeed** | Go | 50070 | Vector embedding & search |
| **Planner** | Go | 50080 | NL → LogQL translation |

**Infrastructure:**
- Redis (6379) - Shared state
- PostgreSQL (5432) - Metadata
- Qdrant (6333) - Vectors
- Loki (3100) - Processed logs
- Loki-Raw (3101) - Raw logs
- Grafana (3000) - Visualization

## Getting Started

1. **Read the Overview**
   - Start with [README.md](../README.md)
   - Then [Architecture](_docs/learn/architecture.md)

2. **Setup Your Environment**
   - Follow [Getting Started](_docs/implement/getting-started.md)
   - Deploy with Docker Compose

3. **Configure Services**
   - Gateway: `gateway/config-local.yaml`
   - Miner: `miner/drain3.ini`
   - See [Configuration Guide](_docs/implement/user-guide.md)

4. **Test Ingestion**
   - Send test logs via APIs
   - Check Grafana dashboards
   - Query both Loki instances

5. **Deep Dive**
   - [Gateway Service](_docs/reference/gateway-service.md)
   - [Component Services](_docs/reference/component-services.md)

## Common Tasks

### Send Logs

```bash
# JSON API
curl -X POST http://localhost:8001/api/v1/logs \
  -H "Content-Type: application/json" \
  -d '{"records":[{...}]}'

# Loki API (goes to both raw and processed)
curl -X POST http://localhost:8001/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{"streams":[{...}]}'
```

### Query Logs

**Raw logs** (Grafana → Loki (Raw)):
```logql
{type="raw", service="api"}
```

**Processed logs** (Grafana → Loki (Processed)):
```logql
{type="processed", service="api", gateway="true"}
```

### Monitor System

```bash
# Check health
curl http://localhost:8001/healthz

# View metrics
curl http://localhost:8001/metrics

# Grafana dashboards
open http://localhost:3000
```

### Troubleshoot

1. Check [Troubleshooting Guide](_docs/implement/troubleshooting.md)
2. Review service logs
3. Check metrics for drops/errors
4. Verify configurations

## API Reference

### Ingestion APIs

- `POST /v1/ingest` - Auto-detect protocol
- `POST /v1/ingest/otlp` - OTLP logs
- `POST /loki/api/v1/push` - Loki Push API
- `POST /api/v1/logs` - JSON logs

### Operational APIs

- `GET /healthz` - Health check
- `GET /metrics` - Prometheus metrics

See [API Reference](_docs/reference/api-reference.md) for details.

## Configuration

### Gateway

Key configuration sections:
- `server` - HTTP server settings
- `otlp` - OTLP validation rules
- `miner` - Miner client config
- `sampler` - Sampler client config
- `loki` - Processed logs sink
- `loki_raw` - Raw logs sink
- `indexfeed` - IndexFeed client config

See [Gateway Service](_docs/reference/gateway-service.md#configuration) for complete examples.

### Services

Each service has its own configuration:
- **Miner**: `drain3.ini` for Drain3 parameters
- **Sampler**: Enforcement rules in Gateway config
- **IndexFeed**: Qdrant and embedding model settings
- **Planner**: Query translation parameters

## Performance

### Throughput (per instance)
- Gateway: 50K+ logs/sec
- Miner: 10K+ ops/sec
- Sampler: 100K+ decisions/sec
- IndexFeed: 5K+ embeddings/sec

### Latency (p99)
- End-to-end: < 100ms
- Gateway: < 10ms
- Mining: < 20ms
- Sampling: < 5ms

### Resource Usage
- Gateway: 512MB-2GB RAM
- Miner: 1-4GB RAM
- Sampler: 512MB-1GB RAM
- IndexFeed: 1-2GB RAM

See [Architecture](_docs/learn/architecture.md#performance-characteristics) for details.

## Contributing

When updating documentation:

1. **Architecture changes**: Update `_docs/learn/architecture.md`
2. **Gateway changes**: Update `_docs/reference/gateway-service.md`
3. **Service changes**: Update `_docs/reference/component-services.md`
4. **API changes**: Update `_docs/reference/api-reference.md`
5. **Navigation**: Update `_data/toc.yml`
6. **Main README**: Keep [README.md](../README.md) in sync

## Documentation Standards

### File Naming
- Use kebab-case: `gateway-service.md`
- Include service name: `component-services.md`
- Be descriptive: `architecture.md` not `arch.md`

### Structure
- Start with YAML frontmatter (layout, title, permalink)
- Include table of contents for long docs
- Use code blocks with language hints
- Add diagrams for complex flows
- Link to related documentation

### Content
- Write for your audience (conceptual vs technical)
- Include examples and use cases
- Provide configuration samples
- Add troubleshooting sections
- Keep up-to-date with code changes

## Support

- **Issues**: Report bugs via GitHub Issues
- **Questions**: Use GitHub Discussions
- **Documentation**: This documentation site
- **Examples**: See `examples/` directory

## Changelog

**2024-01-15**
- ✅ Created comprehensive architecture documentation
- ✅ Added detailed Gateway service documentation
- ✅ Documented all component services (Miner, Sampler, IndexFeed, Planner)
- ✅ Documented dual Loki setup (raw vs processed)
- ✅ Updated main README with current state
- ✅ Updated navigation in toc.yml
- ✅ Created this documentation index

**Previous**
- Existing component specs documentation
- API reference
- Getting started guides

---

**Last Updated:** January 2024  
**Maintainer:** Development Team  
**Status:** ✅ Complete and Current

