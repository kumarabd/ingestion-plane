#!/bin/bash

# Script to generate test traffic to the gateway for log ingestion
# Run this script to create test log entries

echo "Generating test traffic to gateway..."

# Generate various types of requests with different severities
SEVERITIES=("info" "warn" "error" "debug")

for i in {1..10}; do
    echo "Request $i"
    
    # Use current timestamp in ISO 8601 format
    TS=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
    
    # Rotate through severities
    SEV=${SEVERITIES[$((i % 4))]}
    
    # Send logs in correct Loki push format (logproto.PushRequest JSON format)
    RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
      -H "Content-Type: application/json" \
      -X POST "http://localhost:8001/loki/api/v1/push" \
      --data-raw "{\"streams\": [{\"labels\": \"{job=\\\"test\\\", service=\\\"test-app\\\", env=\\\"dev\\\", severity=\\\"$SEV\\\"}\", \"entries\": [{\"ts\": \"$TS\", \"line\": \"Test log message $i with severity $SEV\"}]}]}")
    
    HTTP_STATUS=$(echo "$RESPONSE" | grep "HTTP_STATUS" | cut -d: -f2)
    
    if [ "$HTTP_STATUS" != "200" ]; then
        echo "  ❌ Failed with status: $HTTP_STATUS"
        echo "$RESPONSE" | grep -v "HTTP_STATUS"
    else
        echo "  ✅ Success (severity: $SEV)"
    fi
    
    sleep 0.5
done

echo ""
echo "========================================="
echo "Traffic generation complete!"
echo "========================================="
echo "📊 Sent 10 test log messages to gateway"
echo ""
echo "🔍 Verification commands:"
echo "  Gateway logs:  docker logs gateway-service 2>&1 | grep -i loki | tail -20"
echo "  Loki query:    curl -s 'http://localhost:3100/loki/api/v1/query?query={job=\"test\"}' | jq"
echo "  Grafana UI:    http://localhost:3000 (admin/admin)"
echo ""
