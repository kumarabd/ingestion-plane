#!/bin/bash
set -e

# Observability Stack Installation Script
# Installs Loki (processed), Loki-raw, and Grafana

NAMESPACE="${NAMESPACE:-logging}"
LOKI_VERSION="${LOKI_VERSION:-5.47.2}"
GRAFANA_VERSION="${GRAFANA_VERSION:-7.3.7}"

echo "================================================"
echo "Installing Observability Stack"
echo "================================================"
echo "Namespace: $NAMESPACE"
echo "Loki Version: $LOKI_VERSION"
echo "Grafana Version: $GRAFANA_VERSION"
echo ""

# Add Helm repositories
echo "→ Adding Grafana Helm repository..."
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

# Create namespace if it doesn't exist
echo ""
echo "→ Creating namespace: $NAMESPACE..."
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Install Loki (Processed Logs)
echo ""
echo "→ Installing Loki (Processed Logs)..."
helm upgrade --install loki grafana/loki \
  --namespace $NAMESPACE \
  --values local-loki.yaml \
  --version $LOKI_VERSION \
  --wait --timeout 5m

echo "✓ Loki installed successfully"

# Install Loki-Raw (Raw Logs)
echo ""
echo "→ Installing Loki-Raw (Raw Logs)..."
helm upgrade --install loki-raw grafana/loki \
  --namespace $NAMESPACE \
  --values local-loki-raw.yaml \
  --version $LOKI_VERSION \
  --wait --timeout 5m

echo "✓ Loki-Raw installed successfully"

# Install Grafana
echo ""
echo "→ Installing Grafana..."
helm upgrade --install grafana grafana/grafana \
  --namespace $NAMESPACE \
  --values local-grafana.yaml \
  --version $GRAFANA_VERSION \
  --wait --timeout 5m

echo "✓ Grafana installed successfully"

# Wait for all pods to be ready
echo ""
echo "→ Waiting for all pods to be ready..."
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=loki \
  -n $NAMESPACE \
  --timeout=5m || true

kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/instance=loki-raw \
  -n $NAMESPACE \
  --timeout=5m || true

kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=grafana \
  -n $NAMESPACE \
  --timeout=5m || true

echo ""
echo "================================================"
echo "✓ Observability Stack Installation Complete!"
echo "================================================"
echo ""
echo "Services installed:"
echo "  ✓ Loki (Processed) - 30 day retention"
echo "  ✓ Loki-Raw        - 7 day retention"
echo "  ✓ Grafana         - admin/admin"
echo ""
echo "Check status:"
echo "  kubectl get pods -n $NAMESPACE"
echo ""
echo "Access Grafana:"
echo "  kubectl port-forward -n $NAMESPACE svc/grafana 3000:80"
echo "  Then open: http://localhost:3000"
echo "  Login: admin / admin"
echo ""
echo "Access Loki (Processed):"
echo "  kubectl port-forward -n $NAMESPACE svc/loki 3100:3100"
echo "  Health: curl http://localhost:3100/ready"
echo ""
echo "Access Loki-Raw:"
echo "  kubectl port-forward -n $NAMESPACE svc/loki-raw 3101:3100"
echo "  Health: curl http://localhost:3101/ready"
echo ""
echo "Service Endpoints (within cluster):"
echo "  Loki (Processed): http://loki:3100"
echo "  Loki-Raw: http://loki-raw:3100"
echo "  Grafana: http://grafana:80"
echo ""

