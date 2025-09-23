#!/bin/bash

# Test script for miner integration
echo "🧪 Testing Miner Integration"
echo "============================"

# Build the gateway
echo "🔨 Building gateway..."
go build -o gateway ./cmd/main.go

if [ $? -ne 0 ]; then
    echo "❌ Failed to build gateway"
    exit 1
fi

echo "✅ Gateway built successfully"

# Start the gateway with local config
echo "🚀 Starting gateway with local config..."
./gateway -config config-local.yaml &
GATEWAY_PID=$!

# Wait for gateway to start
sleep 3

# Test data
TEST_LOG='{
  "timestamp": 1703123456789000000,
  "labels": {
    "service": "test-service",
    "environment": "development",
    "level": "info"
  },
  "message": "This is a test log message for miner integration",
  "fields": {
    "user_id": "12345",
    "request_id": "req-abc-123"
  },
  "schema": "TEXT",
  "sanitized": false,
  "orig_len": 45
}'

echo "📤 Sending test log to gateway..."
curl -X POST \
  -H "Content-Type: application/json" \
  -d "$TEST_LOG" \
  http://localhost:8001/api/v1/logs

echo ""
echo "⏳ Waiting for miner to process the log..."
sleep 2

# Check if gateway is still running
if ps -p $GATEWAY_PID > /dev/null; then
    echo "✅ Gateway is running"
else
    echo "❌ Gateway stopped unexpectedly"
    exit 1
fi

# Stop the gateway
echo "🛑 Stopping gateway..."
kill $GATEWAY_PID
wait $GATEWAY_PID 2>/dev/null

echo "✅ Test completed!"
echo ""
echo "📝 Check the gateway output above for miner log processing messages."
echo "   Look for messages starting with '🏗️  MINER:'"
