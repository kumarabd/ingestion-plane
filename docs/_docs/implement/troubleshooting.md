---
layout: page
title: Troubleshooting
permalink: /docs/implement/troubleshooting/
---

# Troubleshooting Guide

Common issues and solutions for the Ingestion Plane system.

## Service Startup Issues

### Gateway Won't Start

**Symptom:** Gateway exits immediately or fails to bind

**Check:**
```bash
# Port already in use?
lsof -i :8001

# Config file valid?
./gateway -config config-local.yaml --validate

# Dependencies running?
docker-compose ps
```

**Solutions:**
```bash
# Kill process on port
kill $(lsof -t -i:8001)

# Check config syntax
cat gateway/config-local.yaml | grep -A 5 "loki:"

# Restart infrastructure
cd deploy && docker-compose restart
```

### Miner Service Fails

**Symptom:** Python errors or connection refused

**Check:**
```bash
# Redis connection
redis-cli ping
# Expected: PONG

# Port available
lsof -i :50051

# Dependencies installed
cd miner && poetry show
```

**Solutions:**
```bash
# Reinstall dependencies
poetry install --no-cache

# Check drain3.ini syntax
cat drain3.ini

# Run with verbose logging
poetry run python main.py --log-level DEBUG
```

### Sampler/IndexFeed Won't Start

**Check:**
```bash
# Go dependencies
go mod download

# Redis connection
redis-cli -h localhost -p 6379 ping

# Port conflicts
lsof -i :50060  # Sampler
lsof -i :50070  # IndexFeed
```

**Solutions:**
```bash
# Rebuild
go build -o sampler main.go

# Check logs
./sampler 2>&1 | tee sampler.log
```

## Ingestion Issues

### Logs Not Reaching Gateway

**Symptom:** No logs appearing in either Loki instance

**Diagnostic:**
```bash
# Test Gateway directly
curl -X POST http://localhost:8001/api/v1/logs \
  -H "Content-Type: application/json" \
  -d '{
    "records": [{
      "timestamp": "2024-01-01T12:00:00Z",
      "labels": {"service": "test"},
      "payload": "test log"
    }]
  }'

# Expected: 200 OK

# Check metrics
curl http://localhost:8001/metrics | grep ingest_requests_total
```

**Solutions:**
- Verify Gateway is running: `curl http://localhost:8001/healthz`
- Check firewall rules
- Verify log shipper configuration
- Check Gateway logs for errors

### Logs in Raw But Not Processed

**Symptom:** Logs appear in Loki-Raw but not Loki (Processed)

**This is expected behavior** - logs are sampled! Check:

**1. Was it suppressed by Sampler?**
```bash
# Check sampling metrics
curl http://localhost:8001/metrics | grep sampler_suppressed_total

# Check enforcement rules
cat gateway/config-local.yaml | grep -A 10 enforcement
```

**2. Check severity:**
```logql
# In Grafana Loki (Raw)
{type="raw"} | json | severity="debug"
```
Debug logs are heavily sampled unless enforcement is disabled.

**3. Review keep/suppress ratio:**
```bash
# Gateway metrics
curl http://localhost:8001/metrics | grep sampler_decisions_total
```

**Solutions:**
- **To keep more logs**: Disable enforcement for that severity
  ```yaml
  enforcement:
    debug: false  # Keep all DEBUG logs
  ```
- **To see what's being suppressed**: Enable shadow mode
  ```yaml
  miner:
    shadow_only: true  # Don't actually drop
  ```

### Logs in Neither Loki

**Symptom:** Logs missing from both Raw and Processed Loki

**Check:**
```bash
# 1. Gateway received them?
curl http://localhost:8001/metrics | grep ingest_records_total

# 2. Loki connectivity?
curl http://localhost:3100/ready  # Processed
curl http://localhost:3101/ready  # Raw

# 3. Gateway Loki sink errors?
curl http://localhost:8001/metrics | grep loki_dropped_total

# 4. Gateway logs
# Look for: "Failed to send batch to Loki"
```

**Solutions:**
- Check Loki is healthy
- Verify network connectivity
- Check Gateway buffer settings (might be dropping under pressure)
- Review Gateway logs for errors

## Performance Issues

### High Memory Usage (Gateway)

**Symptom:** Gateway using > 4GB RAM, OOM kills

**Diagnostic:**
```bash
# Check buffer usage
curl http://localhost:8001/metrics | grep loki_buffer

# Check channel depths (in logs)
# Look for: "buffer_bytes_used", "buffer_entries_used"
```

**Solutions:**

**1. Reduce buffer sizes:**
```yaml
loki:
  max_buffer_bytes: 134217728  # 128MB (down from 256MB)
  max_buffer_entries: 500000   # Down from 1M
```

**2. Faster flushing:**
```yaml
loki:
  flush_interval: "200ms"  # Down from 400ms
```

**3. Scale horizontally:**
```bash
# Deploy multiple Gateway instances
# Use load balancer for distribution
```

### High Latency

**Symptom:** Slow log ingestion, timeouts

**Diagnostic:**
```bash
# Check metrics
curl http://localhost:8001/metrics | grep latency

# Check channel backlogs (in logs)
# Look for: "channel full", "backpressure"
```

**Solutions:**

**1. Increase channel capacities:**
```go
// In http.go (requires rebuild)
minerInputCh: make(chan logtypes.NormalizedLog, 8192)  // Up from 4096
```

**2. Scale downstream services:**
```bash
# Run multiple Miner instances
# Run multiple Sampler instances
# Use load balancing
```

**3. Enable mock mode temporarily:**
```yaml
loki:
  mock_mode: true  # Skip Loki writes for testing
```

### Template Explosion

**Symptom:** Thousands of templates, high Redis memory

**Diagnostic:**
```bash
# Count templates in Redis
docker exec redis-server redis-cli DBSIZE

# Check Miner metrics
curl http://localhost:50051/metrics | grep clusters_active
```

**Solutions:**

**1. Adjust similarity threshold:**
```ini
# miner/drain3.ini
[DRAIN]
sim_th = 0.6  # Increase from 0.4 (fewer clusters)
```

**2. Increase masking:**
```ini
[MASKING]
masking = [
    # Add more aggressive masking patterns
    {"regex_pattern": "\\w{32,}", "mask_with": "<HASH>"}
]
```

**3. Set max clusters:**
```ini
[DRAIN]
max_clusters = 5000  # Limit total
```

**4. Clear old templates:**
```bash
# Redis CLI
docker exec -it redis-server redis-cli

# Delete old templates (careful!)
SCAN 0 MATCH template:* COUNT 100
# Then selectively DEL old ones
```

## Data Issues

### Missing Metrics

**Symptom:** Metrics endpoint returns empty or incomplete data

**Check:**
```bash
# Gateway metrics
curl http://localhost:8001/metrics

# Verify Prometheus format
curl http://localhost:8001/metrics | head -20
```

**Solutions:**
- Restart Gateway service
- Check metrics handler initialization
- Verify Prometheus scraping configuration

### Incorrect Template Assignment

**Symptom:** Similar logs getting different template IDs

**Diagnostic:**
- Check Miner logs for template assignments
- Review similarity threshold
- Verify masking rules are applied

**Solutions:**
```ini
# Adjust similarity
[DRAIN]
sim_th = 0.5  # More lenient matching

# Review masking
[MASKING]
# Ensure numbers, IPs, UUIDs are masked consistently
```

### PII Leakage

**Symptom:** Sensitive data appearing in logs

**Solutions:**

**1. Check redaction rules:**
```bash
# In gateway/pkg/ingest/redactor.go
# Verify patterns for:
# - Emails
# - Credit cards
# - API keys
# - SSN
```

**2. Add custom redaction:**
```go
// Add to redactor.go
{
    Name: "custom_secret",
    Pattern: regexp.MustCompile(`secret_\w+`),
    Replacement: "[REDACTED_SECRET]",
}
```

**3. Test redaction:**
```bash
# Send log with PII
curl -X POST http://localhost:8001/api/v1/logs \
  -d '{"records":[{"payload":"Email: test@example.com"}]}'

# Check in Loki - should show [REDACTED_EMAIL]
```

## Connectivity Issues

### Gateway Can't Reach Miner/Sampler

**Symptom:** gRPC errors, "connection refused"

**Check:**
```bash
# Miner running?
lsof -i :50051

# Sampler running?
lsof -i :50060

# Network reachable?
nc -zv localhost 50051
nc -zv localhost 50060
```

**Solutions:**
- Verify services are running
- Check firewall rules
- Update addresses in `config-local.yaml`
- Check DNS resolution (if using hostnames)

### Gateway Can't Reach Loki

**Symptom:** "Failed to send batch to Loki" errors

**Check:**
```bash
# Loki reachable?
curl http://localhost:3100/ready
curl http://localhost:3101/ready

# Gateway config correct?
cat gateway/config-local.yaml | grep addr
```

**Solutions:**
```bash
# Restart Loki
docker-compose restart loki loki-raw

# Check Docker networking
docker network ls
docker network inspect deploy_default

# Verify URLs in config
loki:
  addr: "http://localhost:3100"  # Correct for local
  # OR
  addr: "http://loki:3100"  # Correct for Docker network
```

### Redis Connection Issues

**Symptom:** "connection refused to Redis"

**Check:**
```bash
# Redis running?
docker ps | grep redis

# Port accessible?
redis-cli ping
```

**Solutions:**
```bash
# Restart Redis
docker-compose restart redis

# Check logs
docker logs redis-server
```

## Debugging Tips

### Enable Debug Logging

**Gateway:**
```bash
# Set log level in code or use verbose mode
LOG_LEVEL=debug ./gateway -config config-local.yaml
```

**Miner:**
```bash
poetry run python main.py --log-level DEBUG
```

### Trace a Specific Log

1. **Send with unique ID:**
```bash
curl -X POST http://localhost:8001/api/v1/logs \
  -d '{
    "records": [{
      "labels": {"service": "test", "trace_id": "unique-123"},
      "payload": "Debug trace log"
    }]
  }'
```

2. **Follow in Grafana:**
```logql
{trace_id="unique-123"}
```

3. **Check each stage:**
- Gateway logs: "Enqueueing"
- Miner logs: "Template discovery"
- Sampler logs: "Decision made"
- Loki logs: Query result

### Monitor Queue Depths

Watch for backpressure:

```bash
# Gateway logs show channel depths
# Look for:
# - "rawQueue: 1500/2000" (getting full)
# - "minerInputCh: 3800/4096" (near capacity)
```

If queues are consistently full:
- Increase channel capacities
- Scale up downstream services
- Enable sampling earlier in pipeline

## Common Error Messages

### "service busy, please retry"

**Meaning:** Gateway queue is full (backpressure)

**Solution:**
- Retry with exponential backoff
- Reduce send rate
- Scale up Gateway instances

### "Template clustering failed"

**Meaning:** Miner couldn't cluster the log

**Solution:**
- Check Miner logs for details
- Verify log format is supported
- Review masking patterns

### "buffer_full" drops

**Meaning:** Loki sink buffer exceeded limits

**Solution:**
- Increase buffer sizes
- Faster flush intervals
- Check Loki ingestion capacity

### "retry_exhausted"

**Meaning:** Failed to send to Loki after retries

**Solution:**
- Check Loki health
- Verify network connectivity
- Review Loki ingestion limits

## Get Help

If you're still stuck:

1. **Check Logs:**
   - Gateway: stdout
   - Miner: stdout
   - Sampler: stdout
   - Docker services: `docker logs <container>`

2. **Check Metrics:**
   - `curl http://localhost:8001/metrics`
   - Look for error counters

3. **Review Configuration:**
   - Compare with examples in [Gateway Service](../reference/gateway-service/)

4. **GitHub Issues:**
   - Search existing issues
   - Create new issue with:
     - Error messages
     - Configuration (redact secrets!)
     - Steps to reproduce

## See Also

- [Getting Started](getting-started/) - Initial setup
- [User Guide](user-guide/) - Usage guide
- [System Architecture](../learn/architecture/) - Architecture overview
- [Gateway Service](../reference/gateway-service/) - Gateway docs
- [Component Services](../reference/component-services/) - Service docs
