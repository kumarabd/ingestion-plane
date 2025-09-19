---
layout: page
title: Data Contracts & APIs
permalink: /docs/reference/data-contracts/
---

# Data Contracts & APIs

## Index-Feed Event Schema

### Event Types

The Index-Feed system publishes three types of events to the `index.templates.events` topic:

#### TEMPLATE_NEW
Published when a new template is first discovered.

```json
{
  "event_type": "TEMPLATE_NEW",
  "template_id": "a1b2c3d4e5f6",
  "template_text": "User <int> logged in from <ip> at <ts>",
  "regex_text": "User \\d+ logged in from \\d+\\.\\d+\\.\\d+\\.\\d+ at \\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z",
  "service": "auth-service",
  "logger": "com.example.auth.AuthService",
  "file": "/var/log/auth.log",
  "first_seen": "2024-01-15T10:30:00Z",
  "last_seen": "2024-01-15T10:30:00Z",
  "support_count": 1,
  "token_signature": "WORD <int> WORD WORD <ip> WORD <ts>",
  "metadata": {
    "severity": "info",
    "environment": "production",
    "namespace": "default"
  }
}
```

#### TEMPLATE_UPDATE
Published when an existing template is updated or refined.

```json
{
  "event_type": "TEMPLATE_UPDATE",
  "template_id": "a1b2c3d4e5f6",
  "template_text": "User <int> logged in from <ip> at <ts>",
  "regex_text": "User \\d+ logged in from \\d+\\.\\d+\\.\\d+\\.\\d+ at \\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z",
  "service": "auth-service",
  "logger": "com.example.auth.AuthService",
  "file": "/var/log/auth.log",
  "first_seen": "2024-01-15T10:30:00Z",
  "last_seen": "2024-01-15T11:45:00Z",
  "support_count": 150,
  "token_signature": "WORD <int> WORD WORD <ip> WORD <ts>",
  "update_reason": "centroid_refinement",
  "metadata": {
    "severity": "info",
    "environment": "production",
    "namespace": "default",
    "variance_reduction": 0.15
  }
}
```

#### TEMPLATE_SPIKE
Published when a template shows unusual activity patterns.

```json
{
  "event_type": "TEMPLATE_SPIKE",
  "template_id": "a1b2c3d4e5f6",
  "template_text": "User <int> logged in from <ip> at <ts>",
  "regex_text": "User \\d+ logged in from \\d+\\.\\d+\\.\\d+\\.\\d+ at \\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z",
  "service": "auth-service",
  "logger": "com.example.auth.AuthService",
  "file": "/var/log/auth.log",
  "first_seen": "2024-01-15T10:30:00Z",
  "last_seen": "2024-01-15T12:00:00Z",
  "support_count": 1500,
  "spike_factor": 3.2,
  "baseline_rate": 50.0,
  "current_rate": 160.0,
  "window_duration": "10m",
  "metadata": {
    "severity": "info",
    "environment": "production",
    "namespace": "default",
    "alert_threshold": 2.0
  }
}
```

## Semantic Search API

### Search Endpoint

**POST** `/api/v1/search`

#### Request

```json
{
  "query": "TLS handshakes failing after deploy",
  "filters": {
    "service": ["auth-service", "api-gateway"],
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

#### Response

```json
{
  "results": [
    {
      "template_id": "b2c3d4e5f6g7",
      "template_text": "TLS handshake failed for <ip>:<int> - <string>",
      "score": 0.95,
      "service": "auth-service",
      "logger": "com.example.auth.TLSService",
      "file": "/var/log/auth.log",
      "support_count": 45,
      "first_seen": "2024-01-15T11:30:00Z",
      "last_seen": "2024-01-15T11:45:00Z",
      "explanation": {
        "matched_tokens": ["TLS", "handshake", "failed"],
        "semantic_similarity": 0.95,
        "context_match": 0.88
      }
    }
  ],
  "query_plan": {
    "generated_logql": "{service=\"auth-service\",environment=\"production\"} |= \"TLS handshake failed\" | json | line_format \"{\\{.message\\}}\"",
    "explanation": "Searching for TLS handshake failures in auth-service production logs",
    "estimated_matches": 45
  },
  "metadata": {
    "total_results": 1,
    "search_time_ms": 25,
    "index_version": "v1.2.3"
  }
}
```

### Template Details Endpoint

**GET** `/api/v1/templates/{template_id}`

#### Response

```json
{
  "template_id": "b2c3d4e5f6g7",
  "template_text": "TLS handshake failed for <ip>:<int> - <string>",
  "regex_text": "TLS handshake failed for \\d+\\.\\d+\\.\\d+\\.\\d+:\\d+ - .+",
  "service": "auth-service",
  "logger": "com.example.auth.TLSService",
  "file": "/var/log/auth.log",
  "first_seen": "2024-01-15T11:30:00Z",
  "last_seen": "2024-01-15T11:45:00Z",
  "support_count": 45,
  "token_signature": "TLS WORD WORD WORD <ip>:<int> - <string>",
  "statistics": {
    "total_occurrences": 45,
    "rate_per_minute": 3.0,
    "peak_rate": 8.0,
    "spike_count": 2
  },
  "exemplars": [
    "TLS handshake failed for 192.168.1.100:443 - certificate verification failed",
    "TLS handshake failed for 10.0.0.5:8443 - connection timeout"
  ],
  "metadata": {
    "severity": "error",
    "environment": "production",
    "namespace": "default"
  }
}
```

## LogQL Generation

### Query Planner Output

The Query Planner generates LogQL queries based on semantic search results:

#### Basic Pattern Matching

**Input:** Template with high support count
**Output:**
```logql
{service="auth-service",environment="production"} |= "TLS handshake failed" | json | line_format "{\\{.message\\}}"
```

#### Time-based Filtering

**Input:** Search with time range
**Output:**
```logql
{service="auth-service",environment="production"} |= "TLS handshake failed" | json | line_format "{\\{.message\\}}" | __error__ = ""
```

#### Aggregation Queries

**Input:** Request for grouped results
**Output:**
```logql
sum(rate({service="auth-service",environment="production"} |= "TLS handshake failed" | json | line_format "{\\{.message\\}}" [5m])) by (service, severity)
```

#### Complex Filtering

**Input:** Multiple template matches with filters
**Output:**
```logql
{service=~"auth-service|api-gateway",environment="production",severity=~"error|fatal"} |= "TLS handshake failed" or |= "connection timeout" | json | line_format "{\\{.message\\}}"
```

## Configuration Schema

### Sampling Policy

```yaml
sampling_policy:
  tenant: "default"
  rules:
    - name: "always_keep_errors"
      condition: "severity in ['error', 'fatal']"
      action: "keep"
      priority: 1
    
    - name: "novel_templates"
      condition: "template_age < 24h"
      action: "keep"
      priority: 2
    
    - name: "spike_detection"
      condition: "rate > p95 * 2.0"
      action: "keep"
      priority: 3
    
    - name: "logarithmic_sampling"
      condition: "count in [1,2,4,8,16,32,64,128,256,512,1024]"
      action: "keep"
      priority: 4
    
    - name: "steady_state"
      condition: "count % 100 == 0"
      action: "keep"
      priority: 5
    
    - name: "budget_guard"
      condition: "tenant_qps > limit"
      action: "sample"
      sample_rate: 0.1
      priority: 6

  budgets:
    max_qps: 10000
    burst_limit: 15000
    
  spike_detection:
    window_size: "10m"
    spike_factor: 2.0
    baseline_percentile: 95
```

### PII Redaction Rules

```yaml
pii_rules:
  - name: "email_addresses"
    pattern: "\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}\\b"
    replacement: "<email>"
    severity: "high"
  
  - name: "credit_cards"
    pattern: "\\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\\b"
    replacement: "<credit_card>"
    severity: "critical"
  
  - name: "ssn"
    pattern: "\\b\\d{3}-\\d{2}-\\d{4}\\b"
    replacement: "<ssn>"
    severity: "critical"
  
  - name: "ip_addresses"
    pattern: "\\b(?:[0-9]{1,3}\\.){3}[0-9]{1,3}\\b"
    replacement: "<ip>"
    severity: "medium"
```

## Error Handling

### API Error Responses

```json
{
  "error": {
    "code": "INVALID_QUERY",
    "message": "Query contains invalid characters",
    "details": {
      "field": "query",
      "value": "TLS handshake failed <script>",
      "suggestion": "Remove special characters and try again"
    },
    "request_id": "req_123456789",
    "timestamp": "2024-01-15T12:00:00Z"
  }
}
```

### Common Error Codes

- `INVALID_QUERY` - Malformed search query
- `RATE_LIMIT_EXCEEDED` - Too many requests
- `TEMPLATE_NOT_FOUND` - Template ID doesn't exist
- `INDEX_UNAVAILABLE` - Search index is temporarily unavailable
- `AUTHENTICATION_FAILED` - Invalid or missing authentication
- `AUTHORIZATION_FAILED` - Insufficient permissions
