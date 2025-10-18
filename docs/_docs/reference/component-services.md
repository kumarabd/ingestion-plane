---
layout: page
title: Component Services
permalink: /docs/reference/component-services/
---

# Component Services

Detailed documentation for each microservice in the Ingestion Plane.

## Miner Service (Python)

**Repository:** `/miner`  
**Port:** 50051 (gRPC)  
**Language:** Python 3.10+  
**Algorithm:** Drain3

### Purpose

Online log template discovery using the Drain3 clustering algorithm. Identifies patterns in log streams and assigns deterministic template IDs.

### Key Features

- **Online Clustering**: Processes logs in real-time without batch processing
- **Drain3 Algorithm**: Depth-first tree traversal for efficient pattern matching
- **Template Persistence**: Stores templates in Redis for durability
- **Variable Masking**: Replaces numbers, IPs, UUIDs, etc. with placeholders
- **gRPC Interface**: High-performance streaming API

### Algorithm Overview

```python
# Drain3 Process
1. Parse log message into tokens
2. Apply masking rules (numbers → <NUM>, IPs → <IP>, etc.)
3. Calculate message length
4. Traverse prefix tree by length and first tokens
5. Find similar cluster (similarity > threshold)
6. If found: update cluster centroid
7. If not found: create new cluster
8. Return template_id (hash of canonical template)
```

### Configuration (drain3.ini)

```ini
[DRAIN]
sim_th = 0.4           # Similarity threshold (0-1)
depth = 4              # Tree depth for clustering
max_children = 100     # Max children per node
max_clusters = 1000    # Max total clusters

[MASKING]
masking = [
    {"regex_pattern": "\\d+", "mask_with": "<NUM>"},
    {"regex_pattern": "([0-9a-fA-F]{32,})", "mask_with": "<HEX>"},
    {"regex_pattern": "\\b(?:[0-9]{1,3}\\.){3}[0-9]{1,3}\\b", "mask_with": "<IP>"},
    {"regex_pattern": "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", "mask_with": "<UUID>"}
]

[SNAPSHOT]
snapshot_interval_minutes = 5
compress_state = true
```

### gRPC Contract

```protobuf
service MinerService {
  rpc MineLogs(stream MineLogsRequest) returns (stream MineLogsResponse);
}

message MineLogsRequest {
  string log_id = 1;
  string message = 2;
  map<string, string> labels = 3;
  google.protobuf.Timestamp timestamp = 4;
}

message MineLogsResponse {
  string log_id = 1;
  string template_id = 2;
  string template_text = 3;
  int32 cluster_id = 4;
  repeated string masked_tokens = 5;
}
```

### Redis Schema

```
# Template storage
template:{template_id} = {
    "template_text": "User <NUM> logged in from <IP>",
    "cluster_id": 42,
    "first_seen": "2024-01-01T00:00:00Z",
    "last_seen": "2024-01-01T12:00:00Z",
    "support_count": 1234,
    "service": "api",
    "logger": "auth"
}

# Cluster index
cluster:{service}:{logger}:{cluster_id} → template_id

# Service index
templates:by_service:{service} → set(template_id)
```

### Running

```bash
cd miner
poetry install
poetry run python main.py
```

### Metrics

- `miner_logs_processed_total`: Total logs processed
- `miner_templates_discovered_total`: New templates found
- `miner_clusters_active`: Current cluster count
- `miner_processing_latency_seconds`: Processing time

---

## Sampler Service (Go)

**Repository:** `/sampler`  
**Port:** 50060 (gRPC)  
**Language:** Go 1.21+

### Purpose

Makes intelligent keep/suppress decisions based on multiple criteria including severity, novelty, spikes, and budget constraints.

### Decision Logic (Priority Order)

```
1. HIGH SEVERITY → KEEP (error, fatal always kept)
2. NOVEL TEMPLATE → KEEP (< 24h since first seen)
3. INCIDENT CONTEXT → KEEP (has trace_id in active incident)
4. SPIKE DETECTED → KEEP (rate > p95 * spike_factor)
5. WARMUP PERIOD → KEEP (first N observations)
6. POWER OF TWO → KEEP (count is 1,2,4,8,16...)
7. STEADY STATE → KEEP (every kth message)
8. BUDGET GUARD → SUPPRESS (tenant over limit)
9. DEFAULT → SUPPRESS
```

### Configuration

```yaml
enforcement:
  debug: true    # Enforce sampling on DEBUG logs
  info: false    # Don't enforce on INFO
  warn: false    # Don't enforce on WARN  
  error: false   # Never enforce on ERROR (always keep)
  by_namespace:
    staging: true      # Enforce in staging
    production: false  # Don't enforce in production
```

### Keep Reasons

- `KEEP_REASON_SEVERITY`: High severity log (ERROR/FATAL)
- `KEEP_REASON_NOVEL`: New template (< 24h old)
- `KEEP_REASON_SPIKE`: Unusual rate increase detected
- `KEEP_REASON_WARMUP`: Initial observation period
- `KEEP_REASON_LOG2`: Power-of-two sampling
- `KEEP_REASON_STEADYK`: Regular sampling interval
- `KEEP_REASON_BUDGET`: Within budget limits

### gRPC Contract

```protobuf
service SamplerService {
  rpc Sample(stream SampleRequest) returns (stream SampleResponse);
}

message SampleRequest {
  string log_id = 1;
  string template_id = 2;
  string severity = 3;
  map<string, string> labels = 4;
  google.protobuf.Timestamp timestamp = 5;
}

message SampleResponse {
  string log_id = 1;
  SampleAction action = 2;  // KEEP or SUPPRESS
  KeepReason keep_reason = 3;
  float sample_rate = 4;
  string policy_version = 5;
  bool shadow = 6;  // If true, decision is not enforced
}
```

### Redis State

```
# Counters
counter:{template_id}:{severity} → count
counter:{template_id}:last_seen → timestamp

# Novelty tracking (TTL: 24h)
novelty:{template_id} → first_seen

# Spike detection
spike:baseline:{service}:{template_id} → ewma_value
spike:p95:{service}:{template_id} → p95_value

# Budget tracking
budget:{namespace}:current → current_qps
budget:{namespace}:limit → max_qps
```

### Running

```bash
cd sampler
go run main.go
```

### Metrics

- `sampler_decisions_total{action,reason}`: Decisions made
- `sampler_kept_logs_total{severity}`: Logs kept
- `sampler_suppressed_logs_total{severity}`: Logs suppressed
- `sampler_processing_latency_seconds`: Decision time

---

## IndexFeed Service (Go)

**Repository:** `/indexfeed`  
**Port:** 50070 (gRPC)  
**Language:** Go 1.21+

### Purpose

Converts log templates to vector embeddings and stores them in Qdrant for semantic search capabilities.

### Key Features

- **Embedding Generation**: Uses sentence transformers for text embeddings
- **Vector Storage**: Stores embeddings in Qdrant with metadata
- **Semantic Search**: Enables natural language queries over templates
- **Batch Processing**: Efficient batch embedding generation
- **Metadata Indexing**: Index by service, env, severity for filtering

### Architecture

```
Template → Clean & Normalize → Generate Embedding → Store in Qdrant
                                      ↓
                              Sentence Transformer Model
                              (e.g., all-MiniLM-L6-v2)
                                      ↓
                              384-dimensional vector
```

### gRPC Contract

```protobuf
service IndexFeedService {
  rpc IndexTemplate(IndexTemplateRequest) returns (IndexTemplateResponse);
  rpc SearchTemplates(SearchTemplatesRequest) returns (SearchTemplatesResponse);
}

message IndexTemplateRequest {
  string template_id = 1;
  string template_text = 2;
  string service = 3;
  string env = 4;
  string severity = 5;
  map<string, string> metadata = 6;
}

message SearchTemplatesRequest {
  string query = 1;
  int32 top_k = 2;
  float threshold = 3;
  map<string, string> filters = 4;  // service, env, severity
}

message SearchTemplatesResponse {
  repeated TemplateMatch matches = 1;
}

message TemplateMatch {
  string template_id = 1;
  string template_text = 2;
  float score = 3;
  map<string, string> metadata = 4;
}
```

### Qdrant Collection Schema

```json
{
  "name": "log_templates",
  "vectors": {
    "size": 384,
    "distance": "Cosine"
  },
  "payload_schema": {
    "template_id": "keyword",
    "template_text": "text",
    "service": "keyword",
    "env": "keyword",
    "severity": "keyword",
    "first_seen": "datetime",
    "last_seen": "datetime",
    "support_count": "integer"
  }
}
```

### Configuration

```yaml
indexfeed:
  addr: "localhost:50070"
  timeout: "500ms"
  max_retries: 3
  retry_base_delay: "50ms"
  
qdrant:
  url: "http://localhost:6333"
  collection: "log_templates"
  
embedding:
  model: "sentence-transformers/all-MiniLM-L6-v2"
  batch_size: 32
  device: "cpu"  # or "cuda"
```

### Running

```bash
cd indexfeed
go run main.go
```

### Metrics

- `indexfeed_templates_indexed_total`: Templates indexed
- `indexfeed_embeddings_generated_total`: Embeddings created
- `indexfeed_search_requests_total`: Search queries
- `indexfeed_processing_latency_seconds`: Indexing time

---

## Planner Service (Go)

**Repository:** `/planner`  
**Port:** 50080 (gRPC)  
**Language:** Go 1.21+

### Purpose

Translates natural language queries to LogQL queries by matching templates via semantic search and generating appropriate filters.

### Query Flow

```
1. Natural Language Query
   ↓
2. Generate Query Embedding (via IndexFeed)
   ↓
3. Search for Matching Templates
   ↓
4. Convert Templates to LogQL Patterns
   ↓
5. Combine with Time/Label Filters
   ↓
6. Execute Against Loki
   ↓
7. Assemble and Rank Results
```

### Example Query Translation

**Input Query:**
```
"Show me authentication failures in production"
```

**Processing:**
1. Generate embedding for query
2. Search IndexFeed → finds templates:
   - "User <NUM> authentication failed: invalid credentials"
   - "Failed to authenticate user <UUID> from <IP>"
3. Generate LogQL:
```logql
{env="production", service=~"auth.*"} 
|= "authentication" 
|= "failed"
| line_format "{% raw %}{{.template_id}} {{.line}}{% endraw %}"
```

### gRPC Contract

```protobuf
service PlannerService {
  rpc PlanQuery(PlanQueryRequest) returns (PlanQueryResponse);
  rpc ExecuteQuery(ExecuteQueryRequest) returns (stream ExecuteQueryResponse);
}

message PlanQueryRequest {
  string natural_language_query = 1;
  google.protobuf.Timestamp start_time = 2;
  google.protobuf.Timestamp end_time = 3;
  map<string, string> filters = 4;
  int32 limit = 5;
}

message PlanQueryResponse {
  string logql_query = 1;
  repeated string matched_templates = 2;
  repeated TemplateMatch template_matches = 3;
  string explanation = 4;
}
```

### Configuration

```yaml
planner:
  addr: "localhost:50080"
  timeout: "5s"
  
loki:
  url: "http://localhost:3100"
  max_query_time: "30s"
  
indexfeed:
  addr: "localhost:50070"
  search_top_k: 10
  similarity_threshold: 0.7
```

### Running

```bash
cd planner
go run main.go
```

### Metrics

- `planner_queries_total{type}`: Queries processed
- `planner_templates_matched_total`: Templates matched
- `planner_logql_generated_total`: LogQL queries generated
- `planner_query_latency_seconds`: End-to-end time

---

## Service Dependencies

```
Gateway
  ├─► Miner (gRPC client)
  ├─► Sampler (gRPC client)
  ├─► IndexFeed (gRPC client)
  ├─► Loki (HTTP client) - Processed
  ├─► Loki-Raw (HTTP client) - Raw
  └─► Redis (cache)

Miner
  └─► Redis (templates, state)

Sampler
  └─► Redis (counters, novelty, spikes)

IndexFeed
  ├─► Qdrant (vectors)
  └─► Redis (metadata)

Planner
  ├─► IndexFeed (gRPC client)
  └─► Loki (HTTP client)
```

## Inter-Service Communication

All services use:
- **gRPC** for service-to-service (Gateway → Miner/Sampler/IndexFeed)
- **HTTP** for external systems (Gateway → Loki, Planner → Loki)
- **Redis** for shared state
- **Protocol Buffers** for data contracts

## Health Checks

All services implement gRPC health check protocol:

```bash
# Check service health
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
```

## Logging

All services log to stdout in JSON format:

```json
{
  "timestamp": "2024-01-01T00:00:00Z",
  "level": "info",
  "service": "miner",
  "message": "Template discovered",
  "template_id": "abc123",
  "cluster_id": 42
}
```

## See Also

- [Gateway Service (Detailed)](gateway-service/)
- [System Architecture](../learn/architecture/)
- [Data Contracts](data-contracts/)
- [API Reference](api-reference/)

