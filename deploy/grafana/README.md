# Grafana Configuration

This directory contains Grafana provisioning configurations for the ingestion plane.

## Components

- **Grafana**: Web UI for visualizing logs and metrics (port 3000)
- **Loki**: Log aggregation system (port 3100)

## Default Credentials

- **Username**: `admin`
- **Password**: `admin`

## Access URLs

- Grafana UI: http://localhost:3000
- Loki API: http://localhost:3100

## Provisioned Datasources

- **Loki**: Automatically configured as the default datasource

## Usage

1. Start the services:
   ```bash
   cd deploy
   docker-compose up -d grafana loki
   ```

2. Access Grafana at http://localhost:3000

3. The Loki datasource is pre-configured and ready to use

4. Create dashboards or use LogQL queries in Explore view

## LogQL Examples

```logql
# View all logs
{job="ingestion-plane"}

# Filter by level
{job="ingestion-plane"} |= "ERROR"

# Filter by service
{job="ingestion-plane", service="gateway"}

# Count errors per minute
sum(rate({job="ingestion-plane"} |= "ERROR" [1m]))
```

## Directory Structure

```
grafana/
├── README.md
└── provisioning/
    ├── datasources/
    │   └── loki.yml          # Loki datasource configuration
    └── dashboards/
        └── dashboard.yml     # Dashboard provider configuration
```

