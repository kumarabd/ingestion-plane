# Loki Out-of-Order Fix

## Problem

You were experiencing **HTTP 400 errors** when sending logs to Loki:

```
{"level":"error","status":400,"entries_dropped":1,"message":"Failed to send batch to Loki after retries"}
```

## Root Cause

Loki, by default, rejects logs that arrive **out of order** within the same stream. This happens because:

1. **Default Loki behavior**: Expects timestamps in strictly ascending order per stream
2. **Distributed systems**: Logs from multiple sources can arrive out of sequence
3. **Batching delays**: Network/processing delays cause timestamp reordering
4. **Clock skew**: Different systems may have slightly different clocks

## Solution

Updated Loki configuration to **allow out-of-order writes**:

### Configuration Changes (`loki-config.yaml`)

```yaml
limits_config:
  # Enable out-of-order ingestion
  unordered_writes: true
  
  # Accept logs up to 7 days old (very generous)
  reject_old_samples: true
  reject_old_samples_max_age: 168h
  
  # Accept logs with timestamps up to 10 minutes in the future
  creation_grace_period: 10m
  
  # Increase ingestion limits
  ingestion_rate_mb: 10
  ingestion_burst_size_mb: 20
```

### What These Settings Do

| Setting | Value | Purpose |
|---------|-------|---------|
| `unordered_writes` | `true` | Enables out-of-order log ingestion |
| `reject_old_samples_max_age` | `168h` (7 days) | Accepts logs up to 7 days old |
| `creation_grace_period` | `10m` | Accepts logs with timestamps up to 10 min in future |
| `ingestion_rate_mb` | `10` | 10 MB/s per tenant ingestion rate |
| `ingestion_burst_size_mb` | `20` | 20 MB burst size |

## Benefits

✅ **No more 400 errors** - Logs arriving out of order are accepted
✅ **Handles clock skew** - Grace period for future timestamps
✅ **Late arrivals** - Accepts logs up to 7 days old
✅ **Better reliability** - No data loss due to timestamp ordering

## Trade-offs

⚠️ **Slightly increased resource usage** - Out-of-order writes require more memory
⚠️ **Query performance** - May be marginally slower for very large datasets

However, these trade-offs are minimal and worth it for data reliability.

## Verification

After applying this configuration:

1. Restart Loki:
   ```bash
   docker-compose restart loki
   ```

2. Check Loki is ready:
   ```bash
   curl http://localhost:3100/ready
   # Should return: ready
   ```

3. Test log ingestion:
   ```bash
   # Your gateway should now successfully send logs without 400 errors
   ```

## Monitoring

Watch for these log patterns to confirm it's working:

**Before (Errors):**
```
Failed to send batch to Loki after retries status=400
```

**After (Success):**
```
Successfully sent batch to Loki status=204 entries_sent=X
```

## Alternative Solutions (Not Recommended)

1. ❌ **Sort logs before sending** - Adds latency and complexity
2. ❌ **Use timestamps at ingestion time** - Loses original log timestamps
3. ✅ **Configure Loki for out-of-order** - Best solution (implemented)

## References

- [Loki Out-of-Order Writes](https://grafana.com/docs/loki/latest/configuration/#limits_config)
- [Loki Ingestion Limits](https://grafana.com/docs/loki/latest/best-practices/)

