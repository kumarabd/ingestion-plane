---
layout: page
title: API Reference
permalink: /docs/reference/api-reference/
---

# API Reference

## Base URL

All API endpoints are relative to the base URL:
```
https://log-analyzer.example.com/api/v1
```

## API Flow

{% mermaid %}
sequenceDiagram
    participant Client
    participant API
    participant Index
    participant Loki
    
    Client->>API: POST /search
    API->>Index: Query templates
    Index-->>API: Return matching templates
    API->>API: Generate LogQL
    API->>Loki: Execute LogQL query
    Loki-->>API: Return log results
    API-->>Client: Return search results
{% endmermaid %}

## Authentication

The API uses API key authentication. Include your API key in the request header:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
     https://log-analyzer.example.com/api/v1/search
```

## Search API

### Search Templates

Search for log templates using natural language queries.

**Endpoint:** `POST /search`

**Request:**
```json
{
  "query": "authentication failures",
  "filters": {
    "service": ["auth-service"],
    "environment": ["production"],
    "severity": ["error", "fatal"],
    "time_range": {
      "start": "2024-01-15T10:00:00Z",
      "end": "2024-01-15T12:00:00Z"
    }
  },
  "limit": 10,
  "include_explanations": true
}
```

**Response:**
```json
{
  "results": [
    {
      "template_id": "abc123",
      "template_text": "Authentication failed for user <string>",
      "score": 0.95,
      "service": "auth-service",
      "support_count": 45,
      "first_seen": "2024-01-15T11:30:00Z",
      "last_seen": "2024-01-15T11:45:00Z",
      "explanation": {
        "matched_tokens": ["authentication", "failed"],
        "semantic_similarity": 0.95
      }
    }
  ],
  "query_plan": {
    "generated_logql": "{service=\"auth-service\"} |= \"Authentication failed\"",
    "explanation": "Searching for authentication failures in auth-service"
  },
  "metadata": {
    "total_results": 1,
    "search_time_ms": 25
  }
}
```

### Get Template Details

Retrieve detailed information about a specific template.

**Endpoint:** `GET /templates/{template_id}`

**Response:**
```json
{
  "template_id": "abc123",
  "template_text": "Authentication failed for user <string>",
  "regex_text": "Authentication failed for user .+",
  "service": "auth-service",
  "first_seen": "2024-01-15T11:30:00Z",
  "last_seen": "2024-01-15T11:45:00Z",
  "support_count": 45,
  "statistics": {
    "total_occurrences": 45,
    "rate_per_minute": 3.0,
    "peak_rate": 8.0
  },
  "exemplars": [
    "Authentication failed for user john.doe",
    "Authentication failed for user admin"
  ]
}
```

## Template Management API

### List Templates

Get a list of templates with optional filtering.

**Endpoint:** `GET /templates`

**Query Parameters:**
- `service`: Filter by service name
- `environment`: Filter by environment
- `severity`: Filter by severity level
- `limit`: Maximum number of results (default: 100)
- `offset`: Number of results to skip (default: 0)

**Response:**
```json
{
  "templates": [
    {
      "template_id": "abc123",
      "template_text": "Authentication failed for user <string>",
      "service": "auth-service",
      "support_count": 45,
      "last_seen": "2024-01-15T11:45:00Z"
    }
  ],
  "pagination": {
    "total": 150,
    "limit": 100,
    "offset": 0,
    "has_more": true
  }
}
```

### Get Template Statistics

Get statistical information about templates.

**Endpoint:** `GET /templates/stats`

**Response:**
```json
{
  "total_templates": 1250,
  "templates_by_service": {
    "auth-service": 45,
    "api-gateway": 32,
    "user-service": 28
  },
  "templates_by_severity": {
    "error": 120,
    "warn": 85,
    "info": 1045
  },
  "recent_activity": {
    "new_templates_24h": 12,
    "updated_templates_24h": 8
  }
}
```

## Sampling API

### Get Sampling Metrics

Retrieve sampling performance metrics.

**Endpoint:** `GET /sampling/metrics`

**Response:**
```json
{
  "total_logs_processed": 1000000,
  "logs_kept": 150000,
  "logs_suppressed": 850000,
  "sampling_rate": 0.15,
  "metrics_by_reason": {
    "always_keep_errors": 50000,
    "novel_templates": 25000,
    "spike_detection": 15000,
    "logarithmic_sampling": 60000
  },
  "performance": {
    "avg_processing_time_ms": 2.5,
    "p95_processing_time_ms": 8.0
  }
}
```

### Update Sampling Policy

Update sampling policies for a tenant.

**Endpoint:** `PUT /sampling/policies/{tenant}`

**Request:**
```json
{
  "rules": [
    {
      "name": "always_keep_errors",
      "condition": "severity in ['error', 'fatal']",
      "action": "keep",
      "priority": 1
    }
  ],
  "budgets": {
    "max_qps": 10000,
    "burst_limit": 15000
  }
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Sampling policy updated successfully",
  "policy_id": "tenant-default-v2"
}
```

## Health and Monitoring API

### Health Check

Check the health status of the system.

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T12:00:00Z",
  "components": {
    "template_miner": "healthy",
    "sampler": "healthy",
    "semantic_indexer": "healthy",
    "redis": "healthy"
  },
  "metrics": {
    "uptime_seconds": 86400,
    "memory_usage_percent": 65.2,
    "cpu_usage_percent": 23.8
  }
}
```

### System Metrics

Get detailed system performance metrics.

**Endpoint:** `GET /metrics`

**Response:**
```json
{
  "system": {
    "uptime_seconds": 86400,
    "memory_usage_bytes": 2147483648,
    "cpu_usage_percent": 23.8,
    "disk_usage_percent": 45.2
  },
  "processing": {
    "logs_per_second": 1500,
    "templates_per_second": 25,
    "avg_processing_latency_ms": 2.5
  },
  "sampling": {
    "total_processed": 1000000,
    "kept": 150000,
    "suppressed": 850000,
    "sampling_rate": 0.15
  }
}
```

## Error Handling

### Error Response Format

All API errors follow this format:

```json
{
  "error": {
    "code": "INVALID_QUERY",
    "message": "Query contains invalid characters",
    "details": {
      "field": "query",
      "value": "invalid query",
      "suggestion": "Use alphanumeric characters only"
    },
    "request_id": "req_123456789",
    "timestamp": "2024-01-15T12:00:00Z"
  }
}
```

### Common Error Codes

| Code | Description |
|------|-------------|
| `INVALID_QUERY` | Malformed search query |
| `RATE_LIMIT_EXCEEDED` | Too many requests |
| `TEMPLATE_NOT_FOUND` | Template ID doesn't exist |
| `INDEX_UNAVAILABLE` | Search index is temporarily unavailable |
| `AUTHENTICATION_FAILED` | Invalid or missing authentication |
| `AUTHORIZATION_FAILED` | Insufficient permissions |
| `VALIDATION_ERROR` | Request validation failed |
| `INTERNAL_ERROR` | Internal server error |

### Rate Limiting

API requests are rate limited to prevent abuse:

- **Search API**: 100 requests per minute per API key
- **Template API**: 200 requests per minute per API key
- **Management API**: 50 requests per minute per API key

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642248000
```

## SDKs and Libraries

### Python SDK

```python
from log_analyzer import LogAnalyzerClient

client = LogAnalyzerClient(api_key="your-api-key")

# Search for templates
results = client.search(
    query="authentication failures",
    filters={"service": ["auth-service"]},
    limit=10
)

# Get template details
template = client.get_template("abc123")
```

### JavaScript SDK

```javascript
import { LogAnalyzerClient } from '@log-analyzer/sdk';

const client = new LogAnalyzerClient({
  apiKey: 'your-api-key',
  baseUrl: 'https://log-analyzer.example.com'
});

// Search for templates
const results = await client.search({
  query: 'authentication failures',
  filters: { service: ['auth-service'] },
  limit: 10
});
```

## Webhooks

### Template Events

Subscribe to template-related events:

**Endpoint:** `POST /webhooks/templates`

**Request:**
```json
{
  "url": "https://your-app.com/webhooks/templates",
  "events": ["template_new", "template_update", "template_spike"],
  "secret": "your-webhook-secret"
}
```

**Webhook Payload:**
```json
{
  "event_type": "template_new",
  "template_id": "abc123",
  "template_text": "Authentication failed for user <string>",
  "service": "auth-service",
  "timestamp": "2024-01-15T12:00:00Z"
}
```
