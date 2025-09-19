---
layout: page
title: Getting Started
permalink: /docs/implement/getting-started/
---

# Getting Started with Log Analyzer

This guide will help you get started with the Log Analyzer system, from understanding the basics to setting up your first log analysis pipeline.

## Prerequisites

Before you begin, ensure you have:

- **Loki**: A running Loki instance for log storage
- **Log Sources**: Applications or services generating logs
- **Kubernetes**: For containerized deployment (optional)
- **Redis**: For state management and caching
- **Message Queue**: Kafka or similar for index-feed events

## Quick Start

### 1. Deploy Log Analyzer

Deploy the Log Analyzer components to your infrastructure:

```bash
# Clone the repository
git clone https://github.com/your-org/log-analyzer.git
cd log-analyzer

# Deploy using Helm (recommended)
helm install log-analyzer ./charts/log-analyzer \
  --set loki.url=http://loki:3100 \
  --set redis.url=redis://redis:6379 \
  --set kafka.brokers=kafka:9092
```

### 2. Configure Log Ingestion

Set up log collection from your applications:

```yaml
# promtail-config.yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://log-analyzer:3100/loki/api/v1/push

scrape_configs:
  - job_name: applications
    static_configs:
      - targets:
          - localhost
        labels:
          job: applications
          __path__: /var/log/apps/*.log
    pipeline_stages:
      - match:
          selector: '{job="applications"}'
          stages:
            - regex:
                expression: '^(?P<timestamp>\S+) (?P<level>\S+) (?P<message>.*)'
            - labels:
                level:
            - timestamp:
                source: timestamp
                format: RFC3339
```

### 3. Configure Sampling Policies

Create sampling policies for your environment:

```yaml
# sampling-policy.yaml
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

  budgets:
    max_qps: 10000
    burst_limit: 15000
```

### 4. Set Up PII Redaction

Configure PII redaction rules:

```yaml
# pii-rules.yaml
pii_rules:
  - name: "email_addresses"
    pattern: "\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}\\b"
    replacement: "<email>"
    severity: "high"
  
  - name: "credit_cards"
    pattern: "\\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\\b"
    replacement: "<credit_card>"
    severity: "critical"
  
  - name: "ip_addresses"
    pattern: "\\b(?:[0-9]{1,3}\\.){3}[0-9]{1,3}\\b"
    replacement: "<ip>"
    severity: "medium"
```

## Configuration

### Environment Variables

Configure the Log Analyzer using environment variables:

```bash
# Core settings
LOG_ANALYZER_LOKI_URL=http://loki:3100
LOG_ANALYZER_REDIS_URL=redis://redis:6379
LOG_ANALYZER_KAFKA_BROKERS=kafka:9092

# Sampling settings
LOG_ANALYZER_SAMPLING_POLICY_PATH=/config/sampling-policy.yaml
LOG_ANALYZER_PII_RULES_PATH=/config/pii-rules.yaml

# Performance settings
LOG_ANALYZER_MAX_QPS=10000
LOG_ANALYZER_BURST_LIMIT=15000
LOG_ANALYZER_WORKER_COUNT=4
```

### Kubernetes Deployment

Deploy using Kubernetes manifests:

```yaml
# log-analyzer-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: log-analyzer
spec:
  replicas: 3
  selector:
    matchLabels:
      app: log-analyzer
  template:
    metadata:
      labels:
        app: log-analyzer
    spec:
      containers:
      - name: log-analyzer
        image: log-analyzer:latest
        ports:
        - containerPort: 8080
        env:
        - name: LOG_ANALYZER_LOKI_URL
          value: "http://loki:3100"
        - name: LOG_ANALYZER_REDIS_URL
          value: "redis://redis:6379"
        - name: LOG_ANALYZER_KAFKA_BROKERS
          value: "kafka:9092"
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
```

## Verification

### 1. Check System Health

Verify that all components are running:

```bash
# Check Log Analyzer health
curl http://log-analyzer:8080/health

# Check template mining
curl http://log-analyzer:8080/api/v1/templates/stats

# Check sampling metrics
curl http://log-analyzer:8080/api/v1/sampling/metrics
```

### 2. Test Log Processing

Send test logs and verify processing:

```bash
# Send test log
echo '2024-01-15T10:30:00Z INFO User 12345 logged in from 192.168.1.100' | \
  curl -X POST -H "Content-Type: text/plain" \
  --data-binary @- http://log-analyzer:8080/api/v1/logs

# Check if template was created
curl http://log-analyzer:8080/api/v1/templates | jq '.templates[] | select(.template_text | contains("User"))'
```

### 3. Test Semantic Search

Verify semantic search functionality:

```bash
# Search for authentication-related templates
curl -X POST http://log-analyzer:8080/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "authentication failures",
    "filters": {
      "service": ["auth-service"]
    },
    "limit": 10
  }'
```

## Next Steps

Now that you have Log Analyzer running:

1. **Configure Monitoring**: Set up alerts and dashboards
2. **Tune Sampling**: Adjust policies based on your log patterns
3. **Integrate APIs**: Connect your applications to the search API
4. **Scale**: Add more instances as your log volume grows

## Troubleshooting

If you encounter issues:

1. **Check Logs**: Review Log Analyzer container logs
2. **Verify Connectivity**: Ensure all services can communicate
3. **Check Resources**: Monitor CPU and memory usage
4. **Review Configuration**: Validate YAML syntax and settings

For detailed troubleshooting, see the [Troubleshooting Guide](/docs/implement/troubleshooting/).

## Support

- **Documentation**: Check the [User Guide](/docs/implement/user-guide/) for detailed usage
- **API Reference**: See the [API Reference](/docs/reference/api-reference/) for technical details
- **Issues**: Report problems on [GitHub Issues]({{ site.repo }}/issues)