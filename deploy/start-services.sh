#!/bin/bash
set -e

echo "🚀 Starting Ingestion Plane Services..."

# Start all services
docker-compose up -d

echo ""
echo "✅ Services started successfully!"
echo ""
echo "📊 Service URLs:"
echo "  - Redis:      localhost:6379"
echo "  - PostgreSQL: localhost:5432"
echo "  - Qdrant:     http://localhost:6333 (UI), localhost:6334 (gRPC)"
echo "  - Loki:       http://localhost:3100"
echo "  - Grafana:    http://localhost:3000"
echo ""
echo "🔐 Grafana Credentials:"
echo "  - Username: admin"
echo "  - Password: admin"
echo ""
echo "📝 Check service status:"
echo "  docker-compose ps"
echo ""
echo "📋 View logs:"
echo "  docker-compose logs -f [service-name]"
echo ""

