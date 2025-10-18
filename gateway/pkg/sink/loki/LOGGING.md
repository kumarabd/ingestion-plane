# Loki Sink Logging

This document describes the logging added to the Loki sink for debugging and monitoring.

## Log Levels

### DEBUG Level
- **Enqueue Stage**: Entry count, buffer usage, active streams
  ```
  "Enqueueing entries to Loki sink" 
  entry_count=100 buffer_bytes_used=45000 buffer_entries_used=500 active_streams=3
  ```

- **Stream Creation**: New stream buffer creation
  ```
  "Created new stream buffer" 
  stream_key="env=prod,service=api,severity=info" labels={"env":"prod","service":"api","severity":"info"}
  ```

- **Batch Preparation**: Data being sent to Loki
  ```
  "Sending batch to Loki" 
  stream_key="..." labels={...} entry_count=1000 approx_bytes=95000
  ```

### INFO Level
- **Enqueue Summary**: Per-enqueue operation summary
  ```
  "Loki enqueue summary" 
  enqueued=100 dropped=0 total_buffer_bytes=45000 total_buffer_entries=500 total_streams=3
  ```

- **Successful Flush**: Successful batch delivery
  ```
  "Successfully sent batch to Loki" 
  status=204 latency=45ms entries_sent=1000 labels={"env":"prod","service":"api"}
  ```

### WARN Level
- **Dropped Entries**: Buffer pressure drops
  ```
  "Dropped log entry due to buffer pressure" 
  severity="debug" reason="buffer_full" labels={...}
  ```

### ERROR Level
- **Failed Flush**: Failed batch delivery after retries
  ```
  "Failed to send batch to Loki after retries" 
  status=500 latency=5s entries_dropped=1000 labels={...}
  ```

## Log Fields

### Common Fields
- `entry_count` - Number of entries in operation
- `labels` - Stream labels (service, env, severity, etc.)
- `stream_key` - Computed stream identifier

### Buffer Metrics
- `buffer_bytes_used` - Current buffer size in bytes
- `buffer_entries_used` - Current number of buffered entries
- `active_streams` - Number of active streams
- `total_buffer_bytes` - Total bytes across all streams
- `total_buffer_entries` - Total entries across all streams

### Performance Metrics
- `latency` - Operation duration
- `status` - HTTP response status code
- `approx_bytes` - Approximate payload size

### Counters
- `enqueued` - Successfully enqueued entries
- `dropped` - Dropped entries
- `entries_sent` - Successfully sent entries
- `entries_dropped` - Failed/dropped entries

## Monitoring Flow

### 1. Entry Reception
```
DEBUG: "Enqueueing entries to Loki sink" → Shows incoming traffic
INFO:  "Loki enqueue summary"           → Shows accept/drop decisions
```

### 2. Buffering
```
DEBUG: "Created new stream buffer"      → New stream detection
```

### 3. Flushing
```
DEBUG: "Sending batch to Loki"          → Pre-send information
INFO:  "Successfully sent batch"        → Success confirmation
ERROR: "Failed to send batch"           → Failure notification
```

### 4. Pressure Handling
```
WARN:  "Dropped log entry"              → Individual drops
```

## Example Log Sequence

```
DEBUG: Enqueueing entries to Loki sink entry_count=50 buffer_bytes_used=12000 ...
DEBUG: Created new stream buffer stream_key="env=prod,service=api,severity=info" ...
INFO:  Loki enqueue summary enqueued=50 dropped=0 ...
DEBUG: Sending batch to Loki stream_key="..." entry_count=50 approx_bytes=12000
INFO:  Successfully sent batch to Loki status=204 latency=23ms entries_sent=50
```

## Debugging Tips

1. **Enable DEBUG level** to see all operations
2. **Watch for WARN/ERROR** to identify issues
3. **Monitor buffer metrics** to detect pressure
4. **Track latency** for performance issues
5. **Check stream_key** for cardinality issues

## Integration with Metrics

All logged operations also emit metrics:
- `loki_enqueued` - Enqueued counter
- `loki_dropped` - Dropped counter  
- `loki_flush_success/fail` - Flush status
- `loki_http_{status}` - HTTP status codes
- Buffer gauges for sizing

Use logs for debugging, metrics for alerting.

