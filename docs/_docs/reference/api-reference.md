---
layout: page
title: API Reference
permalink: /docs/reference/api-reference/
---

# API Reference

Complete API documentation for the Ingestion Plane Gateway service.

## Base URL

**Local Development:**
```
http://localhost:8001
```

**Production:**
```
https://ingestion-plane.example.com
```

## Ingestion APIs

### Auto-Detect Endpoint

Auto-detects protocol based on Content-Type header.

**Endpoint:** `POST /v1/ingest`

**Supported Content Types:**
- `application/json` → Routes to JSON handler
- `application/x-protobuf` → Routes to OTLP handler

---

### OTLP Ingestion

Ingest logs using OpenTelemetry Protocol (OTLP).

**Endpoint:** `POST /v1/ingest/otlp`

**Request:**
```http
POST /v1/ingest/otlp HTTP/1.1
Host: localhost:8001
Content-Type: application/x-protobuf

[OTLP ExportLogsServiceRequest protobuf binary]
```

**Response:**
```json
{
  "status": "success",
  "records_received": 100
}
```

**Features:**
- Full OTLP compliance
- Resource attributes preserved
- Scope and log attributes captured
- All logs go through processing pipeline

---

### Loki Push API

Ingest logs using Grafana Loki Push API format.

**Endpoint:** `POST /loki/api/v1/push`

**Special Behavior:**
- **Raw Logs**: Forwarded to Loki-Raw (port 3101) with ZERO modifications
- **Processed Logs**: Also go through full pipeline to Loki (port 3100)

**Request:**
```http
POST /loki/api/v1/push HTTP/1.1
Host: localhost:8001
Content-Type: application/json
Content-Encoding: gzip (optional)

{
  "streams": [
    {
      "stream": {
        "service": "api",
        "env": "production",
        "severity": "error"
      },
      "values": [
        ["1704110400000000000", "Error: Connection timeout to database"],
        ["1704110401000000000", "Error: Retry failed after 3 attempts"]
      ]
    }
  ]
}
```

**Response:**
```json
{
  "status": "success"
}
```

**Label Format:**
- Labels in `stream` object (not in the log line)
- Timestamp in nanoseconds since Unix epoch
- Multiple streams per request supported

---

### JSON Ingestion

Ingest logs using simple JSON format.

**Endpoint:** `POST /api/v1/logs`

**Request:**
```http
POST /api/v1/logs HTTP/1.1
Host: localhost:8001
Content-Type: application/json

{
  "records": [
    {
      "timestamp": "2024-01-01T12:00:00Z",
      "labels": {
        "service": "api",
        "env": "production",
        "severity": "info",
        "namespace": "default",
        "pod": "api-5d7c8f9b-xyz"
      },
      "payload": "User 12345 logged in from 192.168.1.100",
      "format_hint": "text"
    }
  ]
}
```

**Fields:**
- `timestamp` (optional): ISO 8601 format, defaults to current time
- `labels` (optional): Key-value pairs for metadata
- `payload` (required): The actual log message
- `format_hint` (optional): "json", "logfmt", or "text"

**Response:**
```json
{
  "status": "success",
  "records_received": 1
}
```

**Processing:**
- All logs go through normalization
- PII redaction applied
- Sent through Miner → Sampler → IndexFeed pipeline
- Kept logs sent to Loki (Processed) only

---

## Operational APIs

### Health Check

**Endpoint:** `GET /healthz`

**Response:**
```json
{
  "status": "ok",
  "time": "2024-01-01T12:00:00Z"
}
```

**Use Cases:**
- Kubernetes liveness probes
- Load balancer health checks
- Monitoring systems

---

### Prometheus Metrics

**Endpoint:** `GET /metrics`

**Response:**
```
# HELP gateway_ingest_requests_total Total ingestion requests
# TYPE gateway_ingest_requests_total counter
gateway_ingest_requests_total{protocol="loki",status="success"} 1234

# HELP gateway_loki_enqueued_total Logs enqueued to Loki
# TYPE gateway_loki_enqueued_total counter
gateway_loki_enqueued_total{sink="processed",severity="info"} 500
gateway_loki_enqueued_total{sink="raw",severity="info"} 1000

# HELP gateway_loki_dropped_total Logs dropped by Loki sink
# TYPE gateway_loki_dropped_total counter
gateway_loki_dropped_total{sink="processed",severity="debug",reason="buffer_full"} 10

# ... more metrics
```

**Key Metrics:**

**Ingestion:**
- `gateway_ingest_requests_total{protocol,status}`
- `gateway_ingest_records_total{protocol,severity}`
- `gateway_ingest_rejected_total{reason}`

**Loki:**
- `gateway_loki_enqueued_total{sink,severity}`
- `gateway_loki_dropped_total{sink,severity,reason}`
- `gateway_loki_buffer_bytes{sink,state}`
- `gateway_loki_flush_total{sink,status}`

**Pipeline:**
- `gateway_miner_requests_total{status}`
- `gateway_sampler_decisions_total{action,reason}`

---

## Data Flow

### Loki Push API → Dual Path

```
POST /loki/api/v1/push
         │
         ├─→ EnqueuePushRequest() → Loki-Raw (zero modifications)
         │
         └─→ Pipeline Processing:
                Normalize → Miner → Sampler → Loki (Processed)
```

### JSON/OTLP → Single Path

```
POST /api/v1/logs or /v1/ingest/otlp
         │
         └─→ Pipeline Processing:
                Normalize → Miner → Sampler → Loki (Processed)
                                 ↓
                            IndexFeed → Qdrant
```

## Error Responses

### 400 Bad Request

```json
{
  "error": "invalid loki request format"
}
```

**Causes:**
- Malformed JSON
- Invalid timestamp format
- Missing required fields

### 503 Service Unavailable

```json
{
  "error": "service busy, please retry"
}
```

**Causes:**
- Internal queue full
- Backpressure from downstream services
- Rate limiting triggered

**Solution:** Retry with exponential backoff

### 413 Request Entity Too Large

```json
{
  "error": "Request body too large"
}
```

**Causes:**
- Request exceeds `max_message_bytes` (default 1MB)

**Solution:** Split into smaller batches

## Rate Limiting

Currently no explicit rate limiting. Backpressure is handled via:
- Bounded internal queues
- Sampler budget enforcement
- Loki sink buffer limits

## Examples

### Python Client

```python
import requests
import json
from datetime import datetime

def send_logs(logs):
    url = "http://localhost:8001/api/v1/logs"
    payload = {
        "records": [
            {
                "timestamp": datetime.utcnow().isoformat() + "Z",
                "labels": {
                    "service": log["service"],
                    "env": "production",
                    "severity": log["level"]
                },
                "payload": log["message"]
            }
            for log in logs
        ]
    }
    
    response = requests.post(url, json=payload)
    return response.json()

# Example usage
logs = [
    {"service": "api", "level": "info", "message": "User logged in"},
    {"service": "api", "level": "error", "message": "Database connection failed"}
]

result = send_logs(logs)
print(result)
```

### Go Client

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "time"
)

type LogRecord struct {
    Timestamp string            `json:"timestamp"`
    Labels    map[string]string `json:"labels"`
    Payload   string            `json:"payload"`
}

type LogBatch struct {
    Records []LogRecord `json:"records"`
}

func sendLogs(logs []LogRecord) error {
    batch := LogBatch{Records: logs}
    
    body, err := json.Marshal(batch)
    if err != nil {
        return err
    }
    
    resp, err := http.Post(
        "http://localhost:8001/api/v1/logs",
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    return nil
}

func main() {
    logs := []LogRecord{
        {
            Timestamp: time.Now().UTC().Format(time.RFC3339),
            Labels: map[string]string{
                "service": "api",
                "env": "production",
                "severity": "info",
            },
            Payload: "Request processed successfully",
        },
    }
    
    sendLogs(logs)
}
```

### cURL with Loki API

```bash
#!/bin/bash
# send-loki-logs.sh

TIMESTAMP=$(date +%s%N)  # Nanoseconds

curl -X POST http://localhost:8001/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d "{
    \"streams\": [
      {
        \"stream\": {
          \"service\": \"my-app\",
          \"env\": \"dev\",
          \"severity\": \"info\"
        },
        \"values\": [
          [\"$TIMESTAMP\", \"Application started successfully\"]
        ]
      }
    ]
  }"
```

## Best Practices

### Label Naming

Use consistent label keys:
- `service` - Service name (required)
- `env` - Environment (dev, staging, prod)
- `severity` - Log level (debug, info, warn, error, fatal)
- `namespace` - Kubernetes namespace
- `pod` - Pod name
- `host` - Hostname

### Batch Size

- **Small batches** (< 100): Good for real-time
- **Medium batches** (100-1000): Balanced
- **Large batches** (1000+): Maximum throughput

### Timestamp Format

**JSON API:** ISO 8601
```
2024-01-01T12:00:00Z
2024-01-01T12:00:00.123Z
2024-01-01T12:00:00+00:00
```

**Loki API:** Nanoseconds since Unix epoch
```
1704110400000000000
```

### Error Handling

Always implement retry logic:
```python
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.util.retry import Retry

retry_strategy = Retry(
    total=3,
    status_forcelist=[429, 500, 502, 503, 504],
    backoff_factor=1
)
adapter = HTTPAdapter(max_retries=retry_strategy)
http = requests.Session()
http.mount("http://", adapter)
```

## See Also

- [Gateway Service](gateway-service/) - Detailed Gateway documentation
- [Getting Started](../implement/getting-started/) - Setup guide
- [System Architecture](../learn/architecture/) - Architecture overview
- [Troubleshooting](../implement/troubleshooting/) - Common issues
