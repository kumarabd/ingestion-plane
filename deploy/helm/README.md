# Ingestion Plane Helm Chart

A comprehensive Helm chart for deploying the Ingestion Plane - a log processing and analytics platform.

## Overview

This Helm chart deploys the complete Ingestion Plane stack including:

- **Gateway**: HTTP/gRPC gateway for log ingestion
- **Miner**: Log pattern mining service using Drain3
- **Sampler**: Intelligent log sampling service
- **IndexFeed**: Log indexing and feeding service
- **Planner**: Query planning and optimization service
- **PostgreSQL**: Metadata and configuration storage (Bitnami)
- **Redis**: Caching and state management (CloudPirates)
- **Qdrant**: Vector database for semantic search

## Prerequisites

- Kubernetes 1.20+
- Helm 3.8+
- PV provisioner support in the underlying infrastructure (for persistence)

## Installing the Chart

### Add Required Helm Repositories

First, add the Bitnami and Qdrant Helm repositories:

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add qdrant https://qdrant.github.io/qdrant-helm
helm repo update
```

**Note**: The Redis chart is pulled from CloudPirates OCI registry (`oci://registry-1.docker.io/cloudpirates`) and doesn't require a separate repository addition.

### Install Dependencies

```bash
cd deploy/helm/ingestion-plane
helm dependency update
```

### Install the Chart

To install the chart with the release name `my-ingestion-plane`:

```bash
helm install my-ingestion-plane . -n ingestion-plane --create-namespace
```

### Install with Custom Values

```bash
helm install my-ingestion-plane . -n ingestion-plane \
  --create-namespace \
  --set gateway.replicaCount=3 \
  --set postgresql.auth.password=strongpassword
```

Or use a custom values file:

```bash
helm install my-ingestion-plane . -n ingestion-plane \
  --create-namespace \
  -f custom-values.yaml
```

## Upgrading the Chart

```bash
helm upgrade my-ingestion-plane . -n ingestion-plane
```

## Uninstalling the Chart

```bash
helm uninstall my-ingestion-plane -n ingestion-plane
```

## Configuration

The following table lists the configurable parameters and their default values.

### Global Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imageRegistry` | Global Docker image registry | `ghcr.io` |
| `global.imagePullSecrets` | Global Docker registry secret names | `[]` |

### Gateway Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `gateway.enabled` | Enable Gateway deployment | `true` |
| `gateway.replicaCount` | Number of Gateway replicas | `2` |
| `gateway.image.repository` | Gateway image repository | `kumarabd/ingestion-plane/gateway` |
| `gateway.image.tag` | Gateway image tag | `latest` |
| `gateway.service.type` | Gateway service type | `ClusterIP` |
| `gateway.service.httpPort` | Gateway HTTP port | `8080` |
| `gateway.service.grpcPort` | Gateway gRPC port | `9090` |
| `gateway.autoscaling.enabled` | Enable Gateway HPA | `false` |

### Miner Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `miner.enabled` | Enable Miner deployment | `true` |
| `miner.replicaCount` | Number of Miner replicas | `2` |
| `miner.image.repository` | Miner image repository | `kumarabd/ingestion-plane/miner` |
| `miner.service.grpcPort` | Miner gRPC port | `50051` |

### Sampler Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `sampler.enabled` | Enable Sampler deployment | `true` |
| `sampler.replicaCount` | Number of Sampler replicas | `2` |
| `sampler.service.grpcPort` | Sampler gRPC port | `50060` |

### IndexFeed Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `indexfeed.enabled` | Enable IndexFeed deployment | `true` |
| `indexfeed.replicaCount` | Number of IndexFeed replicas | `1` |
| `indexfeed.service.grpcPort` | IndexFeed gRPC port | `50070` |

### Planner Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `planner.enabled` | Enable Planner deployment | `true` |
| `planner.replicaCount` | Number of Planner replicas | `1` |
| `planner.service.grpcPort` | Planner gRPC port | `50080` |

### PostgreSQL Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `postgresql.enabled` | Enable PostgreSQL deployment | `true` |
| `postgresql.auth.username` | PostgreSQL username | `postgres` |
| `postgresql.auth.password` | PostgreSQL password | `postgres` |
| `postgresql.auth.database` | PostgreSQL database name | `ingestion_plane` |
| `postgresql.primary.persistence.size` | PostgreSQL PVC size | `10Gi` |

### Redis Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `redis.enabled` | Enable Redis deployment | `true` |
| `redis.architecture` | Redis architecture | `standalone` |
| `redis.auth.enabled` | Enable Redis authentication | `false` |
| `redis.master.persistence.size` | Redis PVC size | `8Gi` |

### Qdrant Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `qdrant.enabled` | Enable Qdrant deployment | `true` |
| `qdrant.persistence.size` | Qdrant PVC size | `20Gi` |
| `qdrant.service.httpPort` | Qdrant HTTP port | `6333` |
| `qdrant.service.grpcPort` | Qdrant gRPC port | `6334` |

### Ingress Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.className` | Ingress class name | `nginx` |
| `ingress.hosts` | Ingress hosts configuration | See values.yaml |

## Examples

### Production Setup with External Databases

```yaml
# production-values.yaml
gateway:
  replicaCount: 5
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 10

postgresql:
  enabled: false
  externalHost: my-postgres.example.com

redis:
  enabled: false
  externalHost: my-redis.example.com

qdrant:
  enabled: false
  externalHost: my-qdrant.example.com

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: logs.example.com
      paths:
        - path: /
          pathType: Prefix
          serviceName: gateway
          servicePort: 8080
  tls:
    - secretName: logs-tls
      hosts:
        - logs.example.com
```

### Development Setup

```yaml
# dev-values.yaml
gateway:
  replicaCount: 1
miner:
  replicaCount: 1
sampler:
  replicaCount: 1

postgresql:
  primary:
    persistence:
      size: 2Gi
redis:
  master:
    persistence:
      size: 1Gi
qdrant:
  persistence:
    size: 5Gi
```

## Verification

After installation, verify all pods are running:

```bash
kubectl get pods -n ingestion-plane
```

Check the service endpoints:

```bash
kubectl get svc -n ingestion-plane
```

Test the Gateway health endpoint:

```bash
kubectl port-forward -n ingestion-plane svc/my-ingestion-plane-gateway 8080:8080
curl http://localhost:8080/healthz
```

## Troubleshooting

### Chart dependency errors

If you encounter errors like "found in Chart.yaml, but missing in charts/ directory", you need to extract the chart dependencies:

```bash
cd deploy/helm
# Extract the dependency charts
cd charts
tar -xzf postgresql-13.2.24.tgz
tar -xzf redis-18.4.0.tgz
tar -xzf qdrant-0.8.4.tgz
cd ..
```

This happens because Helm expects the dependency charts to be extracted, not just present as .tgz files.

### Pods not starting

Check pod logs:
```bash
kubectl logs -n ingestion-plane <pod-name>
```

Check pod events:
```bash
kubectl describe pod -n ingestion-plane <pod-name>
```

### Database connection issues

Verify database credentials:
```bash
kubectl get secret -n ingestion-plane my-ingestion-plane-postgresql -o yaml
```

### Storage issues

Check PVC status:
```bash
kubectl get pvc -n ingestion-plane
```

## Support

For issues and questions:
- GitHub Issues: https://github.com/kumarabd/ingestion-plane/issues
- Documentation: https://github.com/kumarabd/ingestion-plane

## License

See the [LICENSE](../../../LICENSE) file for details.

