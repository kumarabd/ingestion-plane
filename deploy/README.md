# Log Ingestion Pipeline with Docker Compose

This Docker Compose setup demonstrates a complete log ingestion pipeline using:
- **Nginx**: Generates structured JSON logs
- **Grafana Agent**: Scrapes and forwards logs
- **Gateway Service**: Our custom log ingestion service
- **Loki**: Optional log storage
- **Grafana**: Optional log visualization

## Architecture

```
Nginx Container → Grafana Agent → Gateway Service → Forwarder (Loki/Output)
     ↓              ↓                ↓
  JSON Logs    Log Scraping    Log Processing
```

## Quick Start

1. **Build and start all services:**
   ```bash
   docker-compose up -d --build
   ```

2. **Generate test traffic:**
   ```bash
   ./generate-traffic.sh
   ```

3. **View logs:**
   ```bash
   # Nginx logs
   docker logs nginx-app
   
   # Gateway service logs
   docker logs gateway-service
   
   # Grafana Agent logs
   docker logs grafana-agent
   ```

## Services

### Nginx (Port 80)
- Serves a simple web page
- Generates structured JSON logs
- Health check endpoint: `http://localhost/health`
- Test endpoint: `http://localhost/test`

### Gateway Service (Ports 8080, 9090)
- HTTP API: `http://localhost:8080`
- gRPC API: `localhost:9090`
- Metrics: `http://localhost:8080/metrics`
- Receives logs from Grafana Agent

### Grafana Agent (Port 12345)
- Scrapes Nginx log files
- Forwards logs to Gateway service
- Metrics: `http://localhost:12345/metrics`

### Loki (Port 3100) - Optional
- Log storage and querying
- API: `http://localhost:3100`

### Grafana (Port 3000) - Optional
- Log visualization
- Web UI: `http://localhost:3000`
- Default credentials: admin/admin

## Configuration Files

- `nginx.conf`: Nginx configuration with JSON log format
- `agent-config.yaml`: Grafana Agent configuration
- `loki-config.yaml`: Loki configuration
- `generate-traffic.sh`: Script to generate test traffic

## Log Flow

1. **Nginx** generates structured JSON logs to `/var/log/nginx/access.log`
2. **Grafana Agent** scrapes these log files
3. **Agent** processes logs with pipeline stages (parsing, labeling)
4. **Agent** forwards processed logs to Gateway service via HTTP
5. **Gateway** processes and forwards logs based on emitter configuration

## Testing the Pipeline

1. **Check if services are running:**
   ```bash
   docker-compose ps
   ```

2. **Generate traffic:**
   ```bash
   ./generate-traffic.sh
   ```

3. **Verify log flow:**
   ```bash
   # Check Nginx logs
   docker exec nginx-app tail -f /var/log/nginx/access.log
   
   # Check Gateway logs
   docker logs -f gateway-service
   
   # Check Agent logs
   docker logs -f grafana-agent
   ```

4. **Test Gateway API directly:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/logs \
     -H "Content-Type: application/json" \
     -d '{"records":[{"timestamp":1640995200000000000,"labels":{"service":"test"},"message":"Test log message"}]}'
   ```

## Monitoring

- **Gateway Metrics**: `http://localhost:8080/metrics`
- **Agent Metrics**: `http://localhost:12345/metrics`
- **Grafana Dashboard**: `http://localhost:3000`

## Troubleshooting

1. **Check container status:**
   ```bash
   docker-compose ps
   docker-compose logs [service-name]
   ```

2. **Verify network connectivity:**
   ```bash
   docker exec grafana-agent ping gateway
   ```

3. **Check log files:**
   ```bash
   docker exec nginx-app ls -la /var/log/nginx/
   docker exec nginx-app tail -f /var/log/nginx/access.log
   ```

4. **Restart services:**
   ```bash
   docker-compose restart [service-name]
   ```

## Customization

- Modify `nginx.conf` to change log format
- Update `agent-config.yaml` to change log processing pipeline
- Adjust `config.yaml` in gateway service for different emitter settings
- Add more services to `docker-compose.yaml` as needed
