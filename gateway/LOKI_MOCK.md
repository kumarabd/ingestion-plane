# Mock Mode for Loki Sink and Index-Feed Producer

Both the Loki sink and Index-Feed producer now support mock modes for local development that output data to stdout instead of sending to real services.

## Configuration

### Enable Mock Mode for Loki Sink

Set `mock_mode: true` in your Loki configuration:

```yaml
loki:
  addr: "http://loki:3100"
  mock_mode: true              # Enable mock mode
  flush_interval: "400ms"
  max_batch_bytes: 1000000
  max_batch_entries: 5000
  # ... other settings
```

### Enable Mock Mode for Index-Feed Producer

Set `mock_mode: true` in your Index-Feed configuration:

```yaml
indexfeed:
  enabled: true
  brokers: ["localhost:9092"]
  topic: "indexfeed"
  mock_mode: true              # Enable mock mode
  # ... other settings
```

### Disable Mock Mode (Production)

Set `mock_mode: false` or omit the setting for both services:

```yaml
loki:
  addr: "http://loki:3100"
  mock_mode: false             # Use real Loki instance
  # ... other settings

indexfeed:
  enabled: true
  brokers: ["localhost:9092"]
  topic: "indexfeed"
  mock_mode: false             # Use real Kafka instance
  # ... other settings
```

## Mock Output Format

### Loki Sink Mock Output

When mock mode is enabled, the Loki sink will:

1. **Decompress** the gzipped Loki push payload
2. **Pretty-print** the JSON to stdout with clear formatting
3. **Simulate** a successful HTTP response (200 OK)

### Index-Feed Producer Mock Output

When mock mode is enabled, the Index-Feed producer will:

1. **Pretty-print** each template event JSON to stdout with clear formatting
2. **Show** Kafka message key and headers for each event
3. **Simulate** successful Kafka production

### Example Outputs

#### Loki Sink Mock Output

```
================================================================================
LOKI MOCK OUTPUT - Logs would be sent to Loki
================================================================================
{
  "streams": [
    {
      "stream": {
        "service": "api",
        "env": "dev",
        "severity": "info",
        "namespace": "default",
        "pod": "api-pod-123",
        "gateway": "true"
      },
      "values": [
        [
          "1640995200000000000",
          "2021-12-31T12:00:00Z INFO API request processed | template_id=abc123 keep_reason=log2"
        ],
        [
          "1640995201000000000", 
          "2021-12-31T12:00:01Z DEBUG Database query executed | template_id=def456 keep_reason=log2"
        ]
      ]
    }
  ]
}
================================================================================
```

#### Index-Feed Producer Mock Output

```
================================================================================
INDEX-FEED MOCK OUTPUT - Template events would be sent to Kafka
================================================================================
Event 1:
Key: default:template_abc123
Headers: event-id=abc123def456, event-type=TEMPLATE_NEW, tenant=default
Payload:
{
  "type": "TEMPLATE_NEW",
  "templateId": "template_abc123",
  "template": "User {user_id} logged in from {ip_address}",
  "regex": "User \\d+ logged in from \\d+\\.\\d+\\.\\d+\\.\\d+",
  "labels": {
    "service": "api",
    "env": "dev",
    "severity": "info",
    "tenant": "default"
  },
  "stats": {
    "windows": {
      "count_10M": 0,
      "count_1H": 0,
      "count_24H": 0
    },
    "firstSeen": "2023-01-01T12:00:00Z",
    "lastSeen": "2023-01-01T12:00:00Z"
  },
  "examples": [
    {
      "message": "User 12345 logged in from 192.168.1.100"
    }
  ],
  "eventId": "abc123def456"
}
----------------------------------------
================================================================================
```

## Usage

### Local Development

Use the provided `config-local.yaml`:

```bash
# Start gateway with mock mode
./main -config config-local.yaml
```

### Production

Use the standard `config.yaml`:

```bash
# Start gateway with real Loki
./main -config config.yaml
```

## Benefits

- **No External Dependencies**: Develop and test without running Loki or Kafka instances
- **Easy Debugging**: See exactly what would be sent to both Loki and Kafka
- **Fast Iteration**: No network calls or external dependencies
- **Production Parity**: Uses the same batching, compression, and formatting logic for both services
- **Full Pipeline Testing**: Test the complete pipeline from logs to both outputs

## Notes

- **Loki Mock Mode**: Only affects the HTTP request - all other Loki sink functionality remains the same
- **Index-Feed Mock Mode**: Only affects the Kafka production - all other Index-Feed functionality remains the same
- **Batching & Metrics**: Batching, compression, retry logic, and metrics are still fully functional for both services
- **Output Format**: The output format matches exactly what would be sent to real Loki and Kafka instances
- **Metrics Collection**: Metrics are still collected normally (successful responses are simulated for both services)
- **Independent Control**: You can enable mock mode for one service while using the real instance for the other
