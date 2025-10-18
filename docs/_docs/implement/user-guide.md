---
layout: page
title: User Guide
permalink: /docs/implement/user-guide/
---

# User Guide

Complete guide to using the Ingestion Plane system effectively for log management and analysis.

## Basic Concepts

### Dual Loki Architecture

The system maintains two separate Loki instances:

**Loki-Raw (Port 3101):**
- Contains raw, unmodified logs
- 7-day retention
- Only for Loki Push API requests
- Use for: compliance, debugging, comparing with processed logs

**Loki (Processed) (Port 3100):**
- Contains sampled, enriched logs
- 30-day retention
- 60-90% volume reduction
- Includes template metadata
- Use for: production queries, dashboards, analysis

### Templates

Templates are normalized representations of log patterns discovered by the Miner service.

**Example:**
- **Original**: `User 12345 logged in from 192.168.1.100 at 2024-01-15T10:30:00Z`
- **Template**: `User <NUM> logged in from <IP> at <TS>`
- **Template ID**: `a1b2c3d4...` (deterministic hash)

### Data Flow

```
┌──────────────┐
│ Log Source   │
└──────┬───────┘
       │
       ▼
┌──────────────────────────────────────┐
│ Gateway                               │
│  - Loki API → Loki-Raw (raw)         │
│  - All → Normalize → Pipeline        │
└──────┬────────────────────────────────┘
       │
       ├─→ Miner → Template Discovery
       │
       ├─→ Sampler → Keep/Suppress Decision
       │
       ├─→ IndexFeed → Semantic Indexing
       │
       └─→ Loki (Processed) ← Only kept logs
```

### Sampling Decisions

The Sampler makes intelligent decisions about which logs to keep:

**Always Kept:**
- ERROR and FATAL severity logs
- Novel templates (< 24h old)
- Spike activity (unusual rate increases)
- First N observations (warmup period)

**Sampled:**
- DEBUG logs (aggressive sampling)
- INFO logs (moderate sampling if enforced)
- Repetitive patterns (power-of-two sampling)

**Suppressed:**
- Logs that don't match keep criteria
- Still counted in metrics
- Not sent to Loki (Processed)

### Keep Reasons

When a log is kept, it includes a `keep_reason`:

- `SEVERITY` - High severity (ERROR/FATAL)
- `NOVEL` - New template pattern
- `SPIKE` - Unusual activity detected
- `WARMUP` - Initial observations
- `LOG2` - Power-of-two sampling (1,2,4,8,16...)
- `STEADYK` - Regular steady-state sampling
- `BUDGET` - Within tenant budget

## Common Workflows

### 1. Sending Logs to the System

#### From Application Code

**Python with structlog:**
```python
import structlog
import requests

log = structlog.get_logger()

# Logs will be sent via your log shipper (Promtail/Vector)
log.info("user_login", user_id=12345, ip="192.168.1.1")
```

**Direct HTTP (Python):**
```python
import requests
from datetime import datetime

def send_log(message, service, severity="info", **labels):
    requests.post("http://gateway:8001/api/v1/logs", json={
        "records": [{
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "labels": {
                "service": service,
                "severity": severity,
                **labels
            },
            "payload": message
        }]
    })

send_log("User logged in", "api", severity="info", env="prod")
```

#### Using Promtail (Loki Agent)

**promtail-config.yaml:**
```yaml
server:
  http_listen_port: 9080

clients:
  - url: http://gateway:8001/loki/api/v1/push  # Goes to both raw and processed

scrape_configs:
  - job_name: system
    static_configs:
      - targets:
          - localhost
        labels:
          job: varlogs
          env: production
          __path__: /var/log/*.log
```

### 2. Querying Logs

#### Query Raw Logs (Grafana)

1. Open Grafana: http://localhost:3000
2. Go to **Explore**
3. Select **Loki (Raw)** datasource
4. Query:

```logql
{type="raw", service="api", env="production"}
| line_format "{% raw %}{{.ts}} [{{.severity}}] {{.line}}{% endraw %}"
```

**Use Cases:**
- Debugging gateway processing issues
- Compliance auditing
- Comparing with processed logs
- Historical reconstruction

#### Query Processed Logs (Grafana)

1. Select **Loki (Processed)** datasource
2. Query:

```logql
{type="processed", service="api", gateway="true"}
| json
| line_format "{% raw %}{{.message}} [Template: {{.template_id}}] [Reason: {{.keep_reason}}]{% endraw %}"
```

**Use Cases:**
- Production dashboards
- Incident investigation
- Pattern analysis
- Cost-optimized long-term storage

#### Advanced Filtering

**By severity:**
```logql
{type="processed", severity="error"}
```

**By template:**
```logql
{type="processed"} | json | template_id="abc123..."
```

**By keep reason:**
```logql
{type="processed"} | json | keep_reason="NOVEL"
```

**Time-based:**
```logql
{type="processed", service="api"} [5m]
```

### 3. Understanding Sampling

#### Check Sampling Decisions

**View Gateway metrics:**
```bash
curl http://localhost:8001/metrics | grep sampler_decisions

# Output:
# sampler_decisions_total{action="KEEP",reason="SEVERITY"} 150
# sampler_decisions_total{action="KEEP",reason="NOVEL"} 45
# sampler_decisions_total{action="SUPPRESS",reason="DEFAULT"} 800
```

**Calculate sampling rate:**
```
Kept: 150 + 45 = 195
Suppressed: 800
Total: 995
Sampling Rate: 195 / 995 = 19.6% (80.4% reduction)
```

#### Adjust Sampling

**To keep more logs** (development):
```yaml
# gateway/config-local.yaml
enforcement:
  debug: false  # Don't sample DEBUG
  info: false   # Don't sample INFO
```

**To sample more aggressively** (production):
```yaml
enforcement:
  debug: true   # Sample DEBUG heavily
  info: true    # Sample INFO moderately
  by_namespace:
    production: true  # Enforce in production
```

### 4. Monitoring Template Discovery

#### Check Template Catalog

**Redis CLI:**
```bash
docker exec -it redis-server redis-cli

# Count templates
DBSIZE

# List templates for a service
KEYS template:*

# Get template details
GET template:abc123def456...
```

**Output:**
```json
{
  "template_text": "User <NUM> logged in from <IP>",
  "cluster_id": 42,
  "first_seen": "2024-01-01T00:00:00Z",
  "last_seen": "2024-01-01T12:00:00Z",
  "support_count": 1234,
  "service": "api"
}
```

#### Monitor New Templates

**Check Miner logs:**
```bash
# In Miner terminal
# Look for: "New template discovered"
# Shows: template_id, template_text, cluster_id
```

**Check metrics:**
```bash
curl http://localhost:50051/metrics | grep templates_discovered_total
```

### 5. Working with Semantic Search (Future)

When the Planner service is integrated, you can query logs using natural language:

**Example Query:**
```
"Show me authentication failures in production"
```

**System Flow:**
1. Planner generates embedding from query
2. Searches IndexFeed for matching templates
3. Converts to LogQL query
4. Executes against Loki (Processed)
5. Returns results with explanations

## Configuration

### Environment-Specific Settings

#### Development

```yaml
# gateway/config-local.yaml
server:
  http:
    port: "8001"

loki:
  mock_mode: true  # Print to stdout

miner:
  shadow_only: true  # Don't drop logs

enforcement:
  debug: false  # Keep all logs
  info: false
```

#### Staging

```yaml
# gateway/config-staging.yaml
enforcement:
  debug: true   # Sample DEBUG
  info: false   # Keep INFO
  by_namespace:
    staging: true
```

#### Production

```yaml
# gateway/config.yaml
enforcement:
  debug: true
  info: true  # Moderate INFO sampling
  by_namespace:
    production: true

loki:
  max_buffer_bytes: 536870912  # 512MB (higher for prod)
```

### Service-Specific Overrides

**Per-namespace enforcement:**
```yaml
enforcement:
  by_namespace:
    critical-service: false  # Never sample
    low-priority: true       # Aggressive sampling
```

## Best Practices

### 1. Log Formatting

**Use structured logging:**
```python
# Good
log.info("user_login", user_id=123, ip="1.2.3.4")

# Avoid
log.info("User 123 logged in from 1.2.3.4")  # Harder to parse
```

**Include consistent labels:**
```yaml
Required labels:
  - service: "api"
  - env: "production"
  - severity: "info"

Recommended:
  - namespace: "default"
  - pod: "api-xyz"
  - version: "v1.2.3"
```

### 2. Severity Levels

Use appropriate severity levels:

- **DEBUG**: Verbose debugging information (heavily sampled)
- **INFO**: General informational messages (moderately sampled)
- **WARN**: Warning messages, potential issues (lightly sampled)
- **ERROR**: Error events (always kept)
- **FATAL**: Critical failures (always kept)

### 3. Query Optimization

**Use label filters:**
```logql
# Good - uses index
{service="api", env="production"}

# Bad - scans all logs
{} |= "api"
```

**Narrow time ranges:**
```logql
# Good - specific time
{service="api"} [5m]

# Bad - scans days
{service="api"} [7d]
```

### 4. Monitoring

**Create Grafana dashboards** to monitor:
- Ingestion rate (logs/sec)
- Sampling rate (kept vs suppressed)
- Template discovery rate
- Error rates by service
- Keep reasons distribution

**Key Panels:**
```promql
# Ingestion rate
rate(gateway_ingest_records_total[5m])

# Sampling rate
rate(sampler_kept_logs_total[5m]) 
  / 
rate(sampler_decisions_total[5m])

# Template count
miner_templates_discovered_total
```

## Advanced Features

### Shadow Mode

Test sampling decisions without actually dropping logs:

```yaml
# gateway/config.yaml
miner:
  shadow_only: true  # Don't enforce drops
```

**Benefits:**
- See what would be sampled
- Verify sampling logic
- Test new enforcement rules
- Safe production testing

### Budget Enforcement

Control costs by limiting ingestion per namespace:

```yaml
# Implemented in Sampler
budget:
  namespaces:
    high-volume-service:
      max_qps: 1000  # Max 1000 logs/sec
      action: "sample_more"  # Increase sampling
```

### PII Redaction

Configure in Gateway to automatically redact sensitive data:

```yaml
# Configured in pkg/ingest/redactor.go
- Email addresses → [REDACTED_EMAIL]
- Credit cards → [REDACTED_CC]
- SSN → [REDACTED_SSN]
- API keys → [REDACTED_API_KEY]
- IP addresses → [REDACTED_IP] (optional)
```

## Tips & Tricks

### Compare Raw vs Processed

**Query both Loki instances:**

Raw:
```logql
{type="raw", service="api"} [5m]
```

Processed:
```logql
{type="processed", service="api"} [5m]
```

**Compare counts:**
- Raw count: Total logs received
- Processed count: Logs after sampling
- Reduction: (Raw - Processed) / Raw * 100%

### Find New Patterns

**Query for novel templates:**
```logql
{type="processed"} | json | keep_reason="NOVEL"
```

This shows recently discovered log patterns that might indicate new issues or features.

### Investigate Spikes

**Find spike-triggered logs:**
```logql
{type="processed"} | json | keep_reason="SPIKE"
```

Shows logs kept due to unusual rate increases - often indicates incidents.

### Debug Missing Logs

1. **Check raw Loki first:**
   ```logql
   {type="raw"} |= "search term"
   ```
   If not there → log never reached Gateway

2. **Check processed Loki:**
   ```logql
   {type="processed"} |= "search term"
   ```
   If raw but not processed → log was suppressed by Sampler

3. **Check sampling metrics:**
   ```bash
   curl http://localhost:8001/metrics | grep suppressed
   ```

4. **Adjust enforcement if needed**

## See Also

- [System Architecture](../learn/architecture/) - Understanding the system
- [Gateway Service](../reference/gateway-service/) - Gateway details
- [Component Services](../reference/component-services/) - Other services
- [API Reference](../reference/api-reference/) - API documentation
- [Troubleshooting](troubleshooting/) - Common issues
- [Getting Started](getting-started/) - Initial setup
