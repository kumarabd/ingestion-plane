#!/bin/bash

# Script to generate test requests to the Planner API
# Run this to create log entries and exercise /v1/search and /v1/plan endpoints.

PLANNER_HOST="http://localhost:8080"

echo "Generating test traffic to Planner at $PLANNER_HOST ..."

for i in {1..10}; do
    echo "=== Request $i ==="
    
    # Alternate between different queries
    case $((i % 3)) in
      0)
        QUERY="log message with characters"
        ;;
      1)
        QUERY="auth login success for user 12345"
        ;;
      2)
        QUERY="database connection timeout"
        ;;
    esac

    echo "-> Sending /v1/search with query: $QUERY"
    SEARCH_RESP=$(curl -s -X POST "$PLANNER_HOST/v1/search" \
      -H "Content-Type: application/json" \
      -d "{
        \"query\": \"$QUERY\",
        \"label_filter\": {\"service\": \"auth\", \"env\": \"prod\"},
        \"top_k\": 3
      }")

    echo "Search Response: $SEARCH_RESP"

    # Extract template_id(s) from the response (jq required)
    TEMPLATE_ID=$(echo "$SEARCH_RESP" | jq -r '.hits[0].template_id // empty')
    TEMPLATE_REGEX=$(echo "$SEARCH_RESP" | jq -r '.hits[0].regex // empty')
    TEMPLATE_TEXT=$(echo "$SEARCH_RESP" | jq -r '.hits[0].template // empty')

    if [[ -n "$TEMPLATE_ID" ]]; then
      echo "-> Sending /v1/plan with selected template_id: $TEMPLATE_ID"

      PLAN_RESP=$(curl -s -X POST "$PLANNER_HOST/v1/plan" \
        -H "Content-Type: application/json" \
        -d "{
          \"selected\": [
            {
              \"template_id\": \"$TEMPLATE_ID\",
              \"template\": \"$TEMPLATE_TEXT\",
              \"regex\": \"$TEMPLATE_REGEX\",
              \"labels\": {\"service\":\"auth\", \"env\":\"prod\"}
            }
          ]
        }")

      echo "Plan Response: $PLAN_RESP"
    else
      echo "No template hits to plan."
    fi

    echo ""
    sleep 1
done

echo "Traffic generation complete!"
echo "Check Planner logs with: docker logs planner-service"
echo "Check Gateway logs with: docker logs gateway-service"
