#!/bin/bash

# Configuration
API_URL="http://localhost:8080/v1/downtime"
AUTHOR="admin"

usage() {
    echo "Usage: $0 <host_name> <duration_hours> [service_id] [comment]"
    echo "Example: $0 srv-db-01 2 \"\" \"Database migration\""
    exit 1
}

if [ "$#" -lt 2 ]; then
    usage
fi

HOST=$1
DURATION=$2
SERVICE=${3:-""}
COMMENT=${4:-"Maintenance scheduled via CLI"}

# Handle Date cross-platform (Linux vs macOS)
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS (BSD date)
    START_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    END_TIME=$(date -u -v+${DURATION}H +"%Y-%m-%dT%H:%M:%SZ")
else
    # Linux (GNU date)
    START_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    END_TIME=$(date -u -d "+$DURATION hours" +"%Y-%m-%dT%H:%M:%SZ")
fi

echo "Creating downtime for $HOST (Duration: ${DURATION}h)..."

# Send request
curl -s -X POST "$API_URL" \
     -H "Content-Type: application/json" \
     -d "{
           \"host_name\": \"$HOST\",
           \"service_id\": \"$SERVICE\",
           \"start_time\": \"$START_TIME\",
           \"end_time\": \"$END_TIME\",
           \"author\": \"$AUTHOR\",
           \"comment\": \"$COMMENT\"
         }" | jq .
