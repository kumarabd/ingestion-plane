# Ingestion Plane

Log ingestion pipeline with protobuf contracts.

## Setup

```bash
# Install dependencies
poetry install

# Activate virtual environment
poetry shell

# Generate protobuf stubs
make gen-python

# Run miner service
python miner/main.py
```
