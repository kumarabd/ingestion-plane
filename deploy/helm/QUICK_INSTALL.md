# Quick Install - Observability Stack

## One-Command Install

```bash
cd deploy/helm
./install-observability.sh
```

## Or Manual Installation

### Step 1: Add Helm Repository
```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
```

### Step 2: Create Namespace
```bash
kubectl create namespace ingestion-plane
```

### Step 3: Install Components
```bash
# Install Loki (Processed Logs - 30 day retention)
helm install loki grafana/loki \
  -n ingestion-plane \
  -f local-loki.yaml \
  --version 5.47.2

# Install Loki-Raw (Raw Logs - 7 day retention)
helm install loki-raw grafana/loki \
  -n ingestion-plane \
  -f local-loki-raw.yaml \
  --version 5.47.2

# Install Grafana
helm install grafana grafana/grafana \
  -n ingestion-plane \
  -f local-grafana.yaml \
  --version 7.3.7
```

## Access Services

```bash
# Grafana UI
kubectl port-forward -n logging svc/grafana 3000:80
# Open: http://localhost:3000 (admin/admin)

# Loki (Processed)
kubectl port-forward -n logging svc/loki 3100:3100
# API: http://localhost:3100

# Loki-Raw
kubectl port-forward -n logging svc/loki-raw 3101:3100
# API: http://localhost:3101
```

## Internal Service Endpoints (within cluster)

```
Loki (Processed): http://loki:3100
Loki-Raw:         http://loki-raw:3100
Grafana:          http://grafana:80
```

## Verify Installation

```bash
kubectl get pods -n logging

# Expected minimal setup:
# NAME                      READY   STATUS    RESTARTS   AGE
# loki-0                    1/1     Running   0          2m
# loki-raw-0                1/1     Running   0          2m
# grafana-xxxxx-xxxxx       1/1     Running   0          1m
```

## Uninstall

```bash
helm uninstall loki loki-raw grafana -n logging
```

