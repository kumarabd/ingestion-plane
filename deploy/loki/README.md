# Loki Configuration

This directory contains the Loki configuration for persistent storage.

## Persistence

Loki is configured to persist all data to `/loki` inside the container, which is mapped to a Docker volume (`loki_data`).

### Storage Directories

- **`/loki/chunks`** - Log data chunks
- **`/loki/boltdb-shipper-active`** - Active index files
- **`/loki/boltdb-shipper-cache`** - Index cache
- **`/loki/compactor`** - Compactor working directory
- **`/loki/wal`** - Write-Ahead Log for durability
- **`/loki/rules`** - Alerting rules

### Retention Policy

- **Retention Period**: 30 days (720 hours)
- **Compaction**: Runs every 10 minutes
- **Retention Enforcement**: Enabled with 2-hour delete delay

## Configuration Highlights

```yaml
# Storage backend
schema_config:
  configs:
    - store: boltdb-shipper    # Index storage
      object_store: filesystem  # Chunks storage

# Data persistence
common:
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules

# Retention
limits_config:
  retention_period: 720h  # 30 days
  
  # Out-of-order ingestion
  unordered_writes: true
  reject_old_samples_max_age: 168h  # Accept logs up to 7 days old
  creation_grace_period: 10m  # Accept logs up to 10 min in future

compactor:
  retention_enabled: true
  compaction_interval: 10m
```

## Out-of-Order Ingestion

Loki is configured to accept logs that arrive out of order:

- **`unordered_writes: true`** - Enables out-of-order writes
- **`reject_old_samples_max_age: 168h`** - Accepts logs up to 7 days old
- **`creation_grace_period: 10m`** - Accepts logs with timestamps up to 10 minutes in the future

This configuration is essential for distributed systems where logs may arrive out of sequence due to:
- Network delays
- Batch processing
- Multiple ingestion sources
- Clock skew between systems

## Verifying Persistence

Check data directories:
```bash
docker exec loki-server ls -la /loki/
```

Check volume:
```bash
docker volume inspect deploy_loki_data
```

## Backup

To backup Loki data:
```bash
# Stop Loki
docker-compose stop loki

# Backup the volume
docker run --rm -v deploy_loki_data:/loki -v $(pwd):/backup alpine \
  tar czf /backup/loki-backup-$(date +%Y%m%d).tar.gz /loki

# Start Loki
docker-compose start loki
```

## Restore

To restore Loki data:
```bash
# Stop Loki
docker-compose stop loki

# Restore from backup
docker run --rm -v deploy_loki_data:/loki -v $(pwd):/backup alpine \
  tar xzf /backup/loki-backup-YYYYMMDD.tar.gz -C /

# Start Loki
docker-compose start loki
```

