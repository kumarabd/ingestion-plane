# Miner Service

Python gRPC service for log template mining with protobuf contracts.

## Setup

```bash
# Install dependencies
poetry install

# Activate virtual environment
poetry shell

# Run the service
poetry run python main.py
```

## Clean Imports

The service now uses clean imports without manual path manipulation:

```python
from contracts.miner.v1 import miner_pb2, miner_pb2_grpc
from contracts.ingest.v1 import ingest_pb2
```

## Dependencies

- Poetry for dependency management
- gRPC and protobuf for service communication
- Redis for state management
- Drain3 for log template mining

## Template Mining

This service uses [Drain3](https://github.com/logpai/Drain3) for log template mining. Drain3 is a robust streaming log template miner based on the Drain algorithm that can extract templates from log messages in real-time.

### Configuration

Drain3 configuration is managed through `drain3.ini` file with the following key settings:

- **Masking**: Automatic masking of dynamic values (IPs, numbers, UUIDs, emails)
- **Clustering**: Similarity threshold and depth settings for template clustering
- **Persistence**: State persistence for template learning across restarts

### Usage

The service automatically:
1. Processes incoming log messages
2. Extracts templates using Drain3
3. Caches templates in Redis for fast lookup
4. Provides template matching with confidence scores
