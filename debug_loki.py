#!/usr/bin/env python3
"""
Debug script to test Loki request parsing
"""

import json
import requests

# Test data
test_data = {
    "streams": [
        {
            "labels": "{foo=\"bar2\"}",
            "entries": [
                {
                    "ts": "2019-10-12T07:20:50.52Z",
                    "line": "test log message"
                }
            ]
        }
    ]
}

print("Sending test data:")
print(json.dumps(test_data, indent=2))

# Send request
response = requests.post(
    "http://localhost:8001/loki/api/v1/push",
    headers={"Content-Type": "application/json"},
    json=test_data
)

print(f"\nResponse status: {response.status_code}")
print(f"Response body: {response.text}")

# Also test with different format
test_data2 = {
    "streams": [
        {
            "labels": "{foo=\"bar2\"}",
            "entries": [
                {
                    "timestamp": "2019-10-12T07:20:50.52Z",
                    "line": "test log message 2"
                }
            ]
        }
    ]
}

print("\n\nSending test data 2:")
print(json.dumps(test_data2, indent=2))

response2 = requests.post(
    "http://localhost:8001/loki/api/v1/push",
    headers={"Content-Type": "application/json"},
    json=test_data2
)

print(f"\nResponse 2 status: {response2.status_code}")
print(f"Response 2 body: {response2.text}")
