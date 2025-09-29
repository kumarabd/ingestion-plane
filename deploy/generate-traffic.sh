#!/bin/bash

# Script to generate test traffic to Nginx for log generation
# Run this script to create some log entries in Nginx

echo "Generating test traffic to Nginx..."

# Generate various types of requests
for i in {1..10}; do
    echo "Request $i"
    
    # Normal requests
    if ! curl -H "Content-Type: application/json" \
  -s -X POST "http://localhost:8001/loki/api/v1/push" \
  --data-raw "{\"streams\": [{ \"labels\": \"{foo=\\\"bar2\\\"}\", \"entries\": [{ \"ts\": \"2019-10-12T07:20:50.52Z\", \"line\": \"This is a log message with $i characters\" }] }]}" 2>&1; then
        echo "Error: curl command failed for request $i"
    fi
    sleep 1
done

echo "Traffic generation complete!"
echo "Check the logs with: docker logs nginx-app"
echo "Check gateway logs with: docker logs gateway-service"
