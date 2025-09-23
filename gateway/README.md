# Gateway Service

The Gateway service provides log ingestion capabilities via gRPC and HTTP endpoints.

## Features

- **gRPC Service**: `IngestService.Push(NormalizedLogBatch) -> Ack`
- **HTTP Endpoints**: `/v1/ingest` (batch), `/healthz`, `/metrics`
- **Shadow Mode Forwarding**: Forwards logs to Miner and Loki without dropping
- **Comprehensive Metrics**: Prometheus metrics for observability
- **Distributed Tracing**: OpenTelemetry tracing support

## Prerequisites

### 1. Install Protocol Buffers Compiler (protoc)

#### macOS (using Homebrew):
```bash
brew install protobuf
```

#### Ubuntu/Debian:
```bash
sudo apt-get update
sudo apt-get install protobuf-compiler
```

#### CentOS/RHEL:
```bash
sudo yum install protobuf-compiler
```

### 2. Install Go Dependencies

```bash
make install-deps
```

This will install:
- `protoc-gen-go` - Go protobuf plugin
- `protoc-gen-go-grpc` - Go gRPC plugin

## Building and Running

### 1. Generate Protobuf Code

```bash
make gen-proto
```

This generates Go code from the protobuf definitions in `../contracts/` directory.

### 2. Build the Gateway

```bash
make build
```

### 3. Run the Gateway

```bash
make run
```

Or run directly:
```bash
./bin/gateway
```

## Configuration

The gateway uses a YAML configuration file (`config.yaml`):

```yaml
server:
  http:
    host: "0.0.0.0"
    port: "8080"
  grpc:
    host: "0.0.0.0"
    port: "9090"

service:
  otlp:
    max_log_size: 1048576
    max_batch_size: 1000
    validate_utf8: true
  
  emitter:
    output_type: "stdout"
    batch_size: 100
    flush_interval: 5
```

## API Usage

### gRPC Client

```go
import "github.com/kumarabd/ingestion-plane/contracts/ingest/v1"

// Create gRPC client
conn, err := grpc.Dial("localhost:9090", grpc.WithInsecure())
client := ingestv1.NewIngestServiceClient(conn)

// Send batch
batch := &ingestv1.NormalizedLogBatch{
    Logs: []*ingestv1.NormalizedLog{
        {
            Timestamp: time.Now().UnixNano(),
            Message:   "Test log message",
            Schema:    ingestv1.SchemaType_SCHEMA_TYPE_TEXT,
        },
    },
}

ack, err := client.Push(context.Background(), batch)
```

### HTTP Client

```bash
# Health check
curl http://localhost:8080/healthz

# Metrics
curl http://localhost:8080/metrics

# Batch ingestion (OTLP format)
curl -X POST http://localhost:8080/v1/ingest \
  -H "Content-Type: application/x-protobuf" \
  --data-binary @batch.pb
```

## Development

### Available Make Targets

```bash
make help              # Show all available targets
make list-proto        # List all protobuf files
make validate          # Validate protobuf files
make gen-proto         # Generate protobuf code
make build             # Build the gateway
make run               # Build and run
make test              # Run tests
make clean             # Clean generated files
make install-deps      # Install Go protobuf plugins
```

### Project Structure

```
gateway/
├── cmd/main.go                    # Main entry point
├── pkg/
│   ├── ingest/                    # Log ingestion logic
│   │   ├── otlp.go               # OTLP handler
│   │   ├── grpc_service.go       # gRPC service implementation
│   │   └── emitter.go            # Log emission
│   ├── sink/                      # Log sinks (Loki, etc.)
│   │   └── loki/                  # Loki sink implementation
│   ├── indexfeed/                 # Index-Feed producer for template events
│   └── server/                    # HTTP and gRPC servers
├── internal/
│   ├── config/                    # Configuration management
│   └── metrics/                   # Metrics collection
├── generated/                     # Generated protobuf code
├── contracts/                     # Protobuf definitions (symlink to ../contracts)
└── config.yaml                    # Configuration file
```

### Protobuf Definitions

The protobuf definitions are located in the `../contracts/` directory:

- `contracts/ingest/v1/normalized_log.proto` - NormalizedLog message
- `contracts/ingest/v1/ingest_service.proto` - IngestService gRPC service

## Monitoring

### Metrics

The gateway exposes Prometheus metrics at `/metrics`:

- `grpc_push_latency_seconds` - gRPC Push method latency
- `grpc_batch_size` - Batch sizes
- `loki_sink_enqueued_total` - Total logs enqueued to Loki sink
- `loki_sink_flush_total` - Total Loki sink flushes
- `indexfeed_produced_total` - Total template events produced to Index-Feed
- `indexfeed_dropped_total` - Total dropped Index-Feed events

### Health Checks

- `/healthz` - Health check endpoint

### Tracing

The gateway supports OpenTelemetry tracing for distributed request tracking.

## Troubleshooting

### Common Issues

1. **protoc not found**: Install Protocol Buffers compiler
2. **Import errors**: Run `make gen-proto` to generate protobuf code
3. **Build failures**: Ensure all dependencies are installed with `make install-deps`

### Debugging

1. Check logs for detailed error messages
2. Verify configuration with `config.yaml`
3. Test endpoints individually:
   - Health: `curl http://localhost:8080/healthz`
   - Metrics: `curl http://localhost:8080/metrics`