# Observability Stack Installation Guide

This guide shows how to install Loki, Loki-raw, and Grafana in your Kubernetes cluster for the Ingestion Plane.

## Prerequisites

1. Kubernetes cluster (1.20+)
2. Helm 3.8+
3. kubectl configured to access your cluster

## Add Helm Repositories

```bash
# Add Grafana Helm repository
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
```

## Installation Steps

### 1. Create Namespace

```bash
kubectl create namespace ingestion-plane
```

**Note**: Each component will create its own ServiceAccount:
- `loki-processed` - For Loki (processed logs)
- `loki-raw` - For Loki-raw (raw logs)
- `grafana-sa` - For Grafana

### 2. Install Loki (Processed Logs)

Install Loki for processed/sampled logs with 30-day retention:

```bash
helm install loki grafana/loki \
  --namespace ingestion-plane \
  --values local-loki.yaml \
  --version 5.47.2
```

**Verify installation:**
```bash
kubectl get pods -n ingestion-plane -l app.kubernetes.io/name=loki
kubectl logs -n ingestion-plane -l app.kubernetes.io/name=loki --tail=50
```

**Service endpoints:**
- HTTP: `http://loki:3100` (direct access to Loki pod)
- Internal: `http://loki.logging.svc.cluster.local:3100`

### 3. Install Loki-Raw (Raw Logs)

Install a second Loki instance for raw/unfiltered logs with 7-day retention:

```bash
helm install loki-raw grafana/loki \
  --namespace ingestion-plane \
  --values local-loki-raw.yaml \
  --version 5.47.2
```

**Verify installation:**
```bash
kubectl get pods -n ingestion-plane -l app.kubernetes.io/name=loki | grep loki-raw
kubectl logs -n ingestion-plane -l app.kubernetes.io/instance=loki-raw --tail=50
```

**Service endpoints:**
- HTTP: `http://loki-raw:3100` (direct access to Loki pod)
- Internal: `http://loki-raw.logging.svc.cluster.local:3100`

### 4. Install Grafana

Install Grafana with datasources pre-configured for both Loki instances:

```bash
helm install grafana grafana/grafana \
  --namespace ingestion-plane \
  --values local-grafana.yaml \
  --version 7.3.7
```

**Verify installation:**
```bash
kubectl get pods -n ingestion-plane -l app.kubernetes.io/name=grafana
kubectl logs -n ingestion-plane -l app.kubernetes.io/name=grafana --tail=50
```

**Access Grafana:**
```bash
# Port forward to access Grafana UI
kubectl port-forward -n ingestion-plane svc/grafana 3000:80

# Open in browser
open http://localhost:3000

# Login credentials:
# Username: admin
# Password: admin
```

## Verify Complete Installation

```bash
# Check all observability pods
kubectl get pods -n ingestion-plane

# Expected output:
# NAME                               READY   STATUS    RESTARTS   AGE
# loki-0                             1/1     Running   0          2m
# loki-raw-0                         1/1     Running   0          2m
# grafana-xxxxx-xxxxx                1/1     Running   0          1m
# loki-gateway-xxxxx-xxxxx           1/1     Running   0          2m
# loki-raw-gateway-xxxxx-xxxxx       1/1     Running   0          2m
```

## Service Endpoints

| Service | Internal Endpoint | Port Forward Command |
|---------|-------------------|----------------------|
| Loki (Processed) | `http://loki:3100` | `kubectl port-forward -n logging svc/loki 3100:3100` |
| Loki (Raw) | `http://loki-raw:3100` | `kubectl port-forward -n logging svc/loki-raw 3101:3100` |
| Grafana | `http://grafana:80` | `kubectl port-forward -n logging svc/grafana 3000:80` |

## Testing Loki Endpoints

```bash
# Test Loki (processed)
kubectl port-forward -n logging svc/loki 3100:3100 &
curl http://localhost:3100/ready

# Test Loki-raw
kubectl port-forward -n logging svc/loki-raw 3101:3100 &
curl http://localhost:3101/ready

# Push test log to Loki
curl -X POST http://localhost:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"job": "test", "level": "info"},
        "values": [
          ["'$(date +%s)000000000'", "test log message"]
        ]
      }
    ]
  }'
```

## Update Installations

```bash
# Update Loki
helm upgrade loki grafana/loki \
  --namespace ingestion-plane \
  --values local-loki.yaml \
  --version 5.47.2

# Update Loki-raw
helm upgrade loki-raw grafana/loki \
  --namespace ingestion-plane \
  --values local-loki-raw.yaml \
  --version 5.47.2

# Update Grafana
helm upgrade grafana grafana/grafana \
  --namespace ingestion-plane \
  --values local-grafana.yaml \
  --version 7.3.7
```

## Uninstall

```bash
# Uninstall all observability components
helm uninstall loki -n ingestion-plane
helm uninstall loki-raw -n ingestion-plane
helm uninstall grafana -n ingestion-plane

# Delete PVCs if needed
kubectl delete pvc -n ingestion-plane -l app.kubernetes.io/name=loki
kubectl delete pvc -n ingestion-plane -l app.kubernetes.io/name=grafana
```

## Configuration Details

### Loki (Processed Logs)
- **Retention**: 30 days
- **Storage**: 10Gi
- **Ingestion Rate**: 10 MB/s
- **Use Case**: Sampled and processed logs from gateway

### Loki-Raw (Raw Logs)
- **Retention**: 7 days (shorter for high volume)
- **Storage**: 20Gi (larger for raw volume)
- **Ingestion Rate**: 20 MB/s (higher for raw traffic)
- **Use Case**: All raw logs before sampling

### Grafana
- **Datasources**: Both Loki instances pre-configured
- **Admin User**: admin / admin
- **Anonymous Access**: Enabled (Viewer role)
- **Storage**: 5Gi for dashboards and settings

## Integration with Ingestion Plane

Update your Ingestion Plane Gateway configuration to use these endpoints:

```yaml
# In gateway config.yaml or Helm values
loki:
  addr: "http://loki:3100"
  # ... other config

loki_raw:
  addr: "http://loki-raw:3100"
  # ... other config
```

## Troubleshooting

### Loki not starting
```bash
# Check logs
kubectl logs -n ingestion-plane -l app.kubernetes.io/name=loki --tail=100

# Check events
kubectl describe pod -n ingestion-plane -l app.kubernetes.io/name=loki
```

### PVC issues
```bash
# Check PVCs
kubectl get pvc -n ingestion-plane

# If using dynamic provisioning, ensure storage class is available
kubectl get storageclass
```

### Grafana datasource issues
```bash
# Check Grafana logs
kubectl logs -n ingestion-plane -l app.kubernetes.io/name=grafana

# Test Loki connectivity from Grafana pod
kubectl exec -n ingestion-plane -it <grafana-pod> -- wget -O- http://loki-gateway/ready
```

### Check ServiceAccounts
```bash
# List all service accounts
kubectl get serviceaccounts -n ingestion-plane

# Expected:
# NAME             SECRETS   AGE
# loki-processed   0         5m
# loki-raw         0         5m
# grafana-sa       0         5m
```

## Production Considerations

For production deployments:

1. **Change deployment mode** to distributed (not SingleBinary)
2. **Use object storage** (S3, GCS, Azure) instead of filesystem
3. **Enable authentication** (`auth_enabled: true`)
4. **Configure proper resource limits**
5. **Set up monitoring and alerting**
6. **Use ingress** for external access
7. **Enable RBAC** and proper security policies
8. **Configure TLS/SSL**
9. **Set up backup strategies**
10. **Tune retention** based on actual needs

## References

- [Loki Helm Chart](https://github.com/grafana/loki/tree/main/production/helm/loki)
- [Grafana Helm Chart](https://github.com/grafana/helm-charts/tree/main/charts/grafana)
- [Loki Documentation](https://grafana.com/docs/loki/latest/)
- [Grafana Documentation](https://grafana.com/docs/grafana/latest/)

