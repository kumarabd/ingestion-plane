---
layout: page
title: Gateway Service
permalink: /docs/reference/gateway-service/
---

# Gateway Service

The Gateway is the primary entry point for all log ingestion. It's written in Go and provides multi-protocol support, pipeline orchestration, and dual Loki sink management.

## Overview

**Language:** Go  
**Repository:** `/gateway`  
**Default Port:** 8001 (local), 8080 (production)  
**Protocol:** HTTP/2, gRPC client

## Responsibilities

1. **Multi-Protocol Ingestion**
   - OTLP (OpenTelemetry Protocol)
   - Loki Push API (`/loki/api/v1/push`)
   - JSON API (`/api/v1/logs`)
   - Auto-detection endpoint (`/v1/ingest`)

2. **Dual Log Path Management**
   - **Raw Path**: Forward Loki API requests to Loki-Raw (zero modifications)
   - **Processed Path**: Full pipeline processing for all sources

3. **Pipeline Orchestration**
   - Normalize and redact incoming logs
   - Call Miner service for template discovery
   - Call Sampler service for keep/suppress decisions
   - Call IndexFeed service for semantic indexing
   - Forward kept logs to Loki (Processed)

4. **Resource Management**
   - Buffering with backpressure
   - Rate limiting and quotas
   - Connection pooling
   - Graceful shutdown

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Gateway Service                       │
│                                                          │
│  ┌────────────────────────────────────────────────┐    │
│  │         HTTP Server (Gin Framework)            │    │
│  │  • OTLP Handler                                │    │
│  │  • Loki Handler                                │    │
│  │  • JSON Handler                                │    │
│  │  • Health/Metrics Endpoints                    │    │
│  └───────────────────┬────────────────────────────┘    │
│                      │                                   │
│                      ▼                                   │
│  ┌────────────────────────────────────────────────┐    │
│  │         Raw Queue (Buffered Channel)           │    │
│  │         Capacity: 2x MaxBatch                  │    │
│  └───────────────────┬────────────────────────────┘    │
│                      │                                   │
│                      ▼                                   │
│  ┌────────────────────────────────────────────────┐    │
│  │         Raw Worker (Goroutine)                 │    │
│  │  1. Loki API → EnqueuePushRequest → Loki-Raw  │    │
│  │  2. Normalize & Redact                         │    │
│  │  3. Send to Miner Channel                      │    │
│  └───────────────────┬────────────────────────────┘    │
│                      │                                   │
│                      ▼                                   │
│  ┌────────────────────────────────────────────────┐    │
│  │    Miner Batcher (4096 capacity channel)      │    │
│  └───────────────────┬────────────────────────────┘    │
│                      │                                   │
│                      ▼                                   │
│  ┌────────────────────────────────────────────────┐    │
│  │  Miner-to-Sampler Bridge (Goroutine)          │    │
│  │  • Receives MinedRecords                       │    │
│  │  • Converts to PipelineRecords                 │    │
│  │  • Sends to IndexFeed                          │    │
│  └───────────────────┬────────────────────────────┘    │
│                      │                                   │
│                      ▼                                   │
│  ┌────────────────────────────────────────────────┐    │
│  │   Sampler Batcher (4096 capacity channel)     │    │
│  └───────────────────┬────────────────────────────┘    │
│                      │                                   │
│                      ▼                                   │
│  ┌────────────────────────────────────────────────┐    │
│  │  Sampler-to-Loki Bridge (Goroutine)           │    │
│  │  • Receives Kept Records                       │    │
│  │  • Converts to LokiEntry                       │    │
│  │  • Enqueues to Loki Processed                  │    │
│  └────────────────────────────────────────────────┘    │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Key Components

### HTTP Handlers

#### OTLP Handler
```go
POST /v1/ingest/otlp
Content-Type: application/x-protobuf

// Parses OTLP ExportLogsServiceRequest
// Converts to RawLogBatch
// Enqueues for processing
```

#### Loki Handler
```go
POST /loki/api/v1/push
Content-Type: application/json
Content-Encoding: gzip (optional)

// Parses logproto.PushRequest
// Forwards to Loki-Raw via EnqueuePushRequest()
// Also converts and processes through pipeline
```

#### JSON Handler
```go
POST /api/v1/logs
Content-Type: application/json

// Parses JSON log array
// Converts to RawLogBatch
// Enqueues for processing
```

### Ingest Handler

**Package:** `pkg/ingest`

**Responsibilities:**
- Format normalization (JSON, logfmt, plain text)
- Schema validation and limits enforcement
- PII redaction using configurable rules
- UTF-8 validation and sanitization
- Message truncation for oversized logs

**Configuration:**
```yaml
otlp:
  max_log_size: 1048576       # 1MB
  max_message_bytes: 1048576
  max_batch_size: 1000
  max_labels: 100
  max_fields: 200
  request_timeout: "30s"
  validate_utf8: true
  allowed_schemas: ["JSON", "LOGFMT", "TEXT"]
```

**Redaction Rules:**
```go
// Example patterns in redactor.go
- Email addresses → [REDACTED_EMAIL]
- Credit cards → [REDACTED_CC]
- SSN → [REDACTED_SSN]
- API keys → [REDACTED_API_KEY]
- IP addresses → [REDACTED_IP]
```

### Loki Sink Manager

**Package:** `pkg/sink/loki`

#### Two Enqueue Methods

**1. Enqueue (Processed Logs)**
```go
func (s *LokiSink) Enqueue(ctx context.Context, entries []LokiEntry)
```
- **Used by:** Sampler-to-Loki bridge
- **Purpose:** Send processed, enriched logs
- **Features:**
  - Adds static labels (gateway="true", type="processed")
  - Merges labels via streamKey()
  - Full buffering and batching
  - Severity-aware dropping under pressure

**2. EnqueuePushRequest (Raw Logs)**
```go
func (s *LokiSink) EnqueuePushRequest(ctx context.Context, req *logproto.PushRequest)
```
- **Used by:** Loki handler
- **Purpose:** Forward raw logs without modification
- **Features:**
  - Zero label additions
  - Preserves original labels exactly
  - Uses streamKeyFromLabels() without static labels
  - Same buffering/batching as processed

#### Buffering Strategy

```go
type LokiSink struct {
    streams     map[string]*streamBuffer  // Keyed by label combination
    usedBytes   int64                     // Current buffer usage
    usedEntries int64                     // Current entry count
    
    maxBatchBytes    int     // 1MB default
    maxBatchEntries  int     // 5000 default
    maxBufferBytes   int64   // 256MB default
    maxBufferEntries int64   // 1M default
    flushInterval    time.Duration  // 400ms default
}
```

**Flush Triggers:**
1. Buffer reaches maxBatchBytes
2. Buffer reaches maxBatchEntries  
3. FlushInterval timer expires
4. Shutdown signal received

**Drop Policy (Under Pressure):**
1. Drop DEBUG logs first (always)
2. Drop INFO logs if `protect_info=false`
3. Never drop WARN/ERROR/FATAL

### Pipeline Workers

#### Raw Worker
**Goroutine:** `runRawWorker()`

**Flow:**
1. Receive batch from rawQueue
2. If Loki API request → Forward to Loki-Raw immediately
3. Normalize and redact batch
4. Send normalized logs to minerInputCh
5. Emit to original emitter (shadow mode)

#### Miner-to-Sampler Bridge
**Goroutine:** `runMinerToSamplerBridge()`

**Flow:**
1. Receive MinedRecord from minerOutputCh
2. Convert to PipelineRecord with template_id
3. Send to IndexFeed for semantic indexing
4. Send to samplerInputCh

#### Sampler-to-Loki Bridge
**Goroutine:** `runSamplerToLokiBridge()`

**Flow:**
1. Receive kept PipelineRecord from samplerOutputKeptCh
2. Build enriched log line with metadata:
   - Original message
   - Template ID
   - Keep reason
   - Sampling decision details
3. Convert to LokiEntry
4. Enqueue to Loki (Processed)

### gRPC Clients

**Miner Client**
```yaml
miner:
  addr: "localhost:50051"
  timeout: "500ms"
  max_batch: 1000
  max_batch_wait: "50ms"
  max_retries: 3
  retry_base_delay: "50ms"
  shadow_only: false
```

**Sampler Client**
```yaml
sampler:
  addr: "localhost:50060"
  timeout: "500ms"
  max_batch: 1000
  max_batch_wait: "50ms"
```

**IndexFeed Client**
```yaml
indexfeed:
  addr: "localhost:50070"
  timeout: "500ms"
  max_retries: 3
  retry_base_delay: "50ms"
```

## Configuration

### Complete Example (config-local.yaml)

```yaml
server:
  http:
    host: "0.0.0.0"
    port: "8001"
    read_timeout: "30s"
    write_timeout: "30s"
    idle_timeout: "60s"
    bounds:
      max_batch: 1000
      max_message_bytes: 65536
    pipeline:
      enqueue_timeout: "5s"

otlp:
  max_log_size: 1048576
  max_message_bytes: 1048576
  max_batch_size: 1000
  max_labels: 100
  max_fields: 200
  request_timeout: "30s"
  validate_utf8: true
  allowed_schemas: ["JSON", "LOGFMT", "TEXT"]

miner:
  addr: "localhost:50051"
  timeout: "500ms"
  max_batch: 1000
  max_batch_wait: "50ms"
  max_retries: 3
  retry_base_delay: "50ms"
  shadow_only: false

sampler:
  addr: "localhost:50060"
  timeout: "500ms"
  max_batch: 1000
  max_batch_wait: "50ms"

enforcement:
  debug: true
  info: false
  warn: false
  error: false
  by_namespace:
    staging: true

# Loki sink for processed logs
loki:
  addr: "http://localhost:3100"
  flush_interval: "400ms"
  max_batch_bytes: 1000000
  max_batch_entries: 5000
  max_buffer_bytes: 268435456
  max_buffer_entries: 1000000
  request_timeout: "5s"
  mock_mode: false
  retry:
    enabled: true
    initial_backoff: "200ms"
    max_backoff: "5s"
  drop_policy:
    debug_first: true
    protect_info: true
  labels:
    static:
      gateway: "true"
      type: "processed"

# Loki sink for raw logs
loki_raw:
  addr: "http://localhost:3101"
  flush_interval: "400ms"
  max_batch_bytes: 1000000
  max_batch_entries: 5000
  max_buffer_bytes: 268435456
  max_buffer_entries: 1000000
  request_timeout: "5s"
  mock_mode: false
  retry:
    enabled: true
    initial_backoff: "200ms"
    max_backoff: "5s"
  drop_policy:
    debug_first: true
    protect_info: false  # Less protective for raw
  labels:
    static:
      gateway: "true"
      type: "raw"

indexfeed:
  addr: "localhost:50070"
  timeout: "500ms"
  max_retries: 3
  retry_base_delay: "50ms"

metrics: {}
```

## API Reference

### Ingestion Endpoints

#### Auto-Detect Endpoint
```http
POST /v1/ingest
Content-Type: application/json | application/x-protobuf
```
Auto-detects protocol (OTLP vs JSON) based on content type.

#### OTLP Endpoint
```http
POST /v1/ingest/otlp
Content-Type: application/x-protobuf

[OTLP ExportLogsServiceRequest]
```

#### Loki Push API
```http
POST /loki/api/v1/push
Content-Type: application/json
Content-Encoding: gzip (optional)

{
  "streams": [
    {
      "stream": {"service": "api", "env": "prod"},
      "values": [
        ["1234567890000000000", "log line here"]
      ]
    }
  ]
}
```

#### JSON API
```http
POST /api/v1/logs
Content-Type: application/json

{
  "records": [
    {
      "timestamp": "2024-01-01T00:00:00Z",
      "labels": {"service": "api"},
      "payload": "log message"
    }
  ]
}
```

### Operational Endpoints

#### Health Check
```http
GET /healthz

Response: 200 OK
{
  "status": "ok",
  "time": "2024-01-01T00:00:00Z"
}
```

#### Metrics
```http
GET /metrics

Response: 200 OK
[Prometheus metrics format]
```

## Metrics

### Key Metrics Exported

**Ingestion:**
- `gateway_ingest_requests_total{protocol,status}`
- `gateway_ingest_records_total{protocol,severity}`
- `gateway_ingest_rejected_total{reason}`
- `gateway_ingest_latency_seconds{protocol}`

**Pipeline:**
- `gateway_pipeline_processed_total{stage}`
- `gateway_pipeline_errors_total{stage,error}`
- `gateway_pipeline_latency_seconds{stage}`

**Loki:**
- `gateway_loki_enqueued_total{sink,severity}`
- `gateway_loki_dropped_total{sink,severity,reason}`
- `gateway_loki_buffer_bytes{sink,state}`
- `gateway_loki_buffer_entries{sink,state}`
- `gateway_loki_flush_total{sink,status}`
- `gateway_loki_flush_latency_seconds{sink}`

**gRPC Clients:**
- `gateway_grpc_requests_total{service,method,status}`
- `gateway_grpc_latency_seconds{service,method}`

## Performance Tuning

### Channel Sizing

```go
// Adjust based on throughput requirements
rawQueue:           make(chan queuedItem, config.Bounds.MaxBatch*2)
minerInputCh:       make(chan logtypes.NormalizedLog, 4096)
minerOutputCh:      make(chan types.MinedRecord, 4096)
samplerInputCh:     make(chan *sampler.PipelineRecord, 4096)
samplerOutputKeptCh: make(chan *sampler.PipelineRecord, 4096)
```

**Recommendations:**
- **Low traffic (<1K logs/sec):** Default settings
- **Medium traffic (1K-10K logs/sec):** 2x channel sizes
- **High traffic (>10K logs/sec):** 4x channel sizes + horizontal scaling

### Loki Buffer Tuning

```yaml
loki:
  flush_interval: "400ms"     # Lower = less latency, more requests
  max_batch_bytes: 1000000    # Increase for better compression
  max_batch_entries: 5000     # Increase for better batching
  max_buffer_bytes: 268435456 # Increase if seeing drops
```

### Connection Pooling

```go
// HTTP Client settings (loki.go)
Transport: &http.Transport{
    MaxIdleConns:        100,   // Increase for high throughput
    MaxIdleConnsPerHost: 10,    // Increase for multi-loki
    IdleConnTimeout:     90 * time.Second,
}
```

## Troubleshooting

### High Memory Usage

**Symptoms:** OOM kills, high RSS

**Causes:**
- Large buffer accumulation
- Slow Loki ingestion
- gRPC client backpressure

**Solutions:**
```yaml
# Reduce buffer sizes
loki:
  max_buffer_bytes: 134217728  # 128MB instead of 256MB
  max_buffer_entries: 500000   # 500K instead of 1M

# Faster flushing
loki:
  flush_interval: "200ms"  # 200ms instead of 400ms
```

### High Latency

**Symptoms:** Slow ingestion, timeouts

**Causes:**
- Blocking gRPC calls
- Full channels
- Slow Loki writes

**Solutions:**
- Increase channel capacities
- Enable shadow mode temporarily
- Scale up downstream services
- Add more Gateway instances

### Missing Logs

**Symptoms:** Logs not appearing in Loki

**Checks:**
1. Check sampling decisions (Sampler)
2. Verify Loki connectivity
3. Check buffer drop metrics
4. Review enforcement rules

**Debug:**
```bash
# Check Gateway logs
tail -f gateway.log | grep "Dropped log"

# Check metrics
curl localhost:8001/metrics | grep dropped

# Test with curl
curl -X POST localhost:8001/api/v1/logs \
  -H "Content-Type: application/json" \
  -d '{"records":[{"timestamp":"2024-01-01T00:00:00Z","labels":{"service":"test"},"payload":"test log"}]}'
```

## Development

### Building

```bash
cd gateway
make build
```

### Running Locally

```bash
# With local config
./gateway -config config-local.yaml

# With environment variables
export GATEWAY_PORT=8001
./gateway
```

### Testing

```bash
# Unit tests
make test

# Integration tests
make test-integration

# Load tests
make test-load
```

## Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gateway cmd/main.go

FROM alpine:latest
COPY --from=builder /app/gateway /gateway
COPY config.yaml /config.yaml
CMD ["/gateway", "-config", "/config.yaml"]
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      containers:
      - name: gateway
        image: gateway:latest
        ports:
        - containerPort: 8080
        env:
        - name: MINER_ADDR
          value: "miner:50051"
        - name: SAMPLER_ADDR
          value: "sampler:50060"
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
```

## See Also

- [System Architecture](../learn/architecture/)
- [Miner Service](miner-service/)
- [Sampler Service](sampler-service/)
- [API Reference](api-reference/)

