#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ORCHESTRATOR_HOST="${ORCHESTRATOR_HOST:-${SERVICE_HOST}:${ORCHESTRATOR_SERVICE_PORT:-8080}}"
BASE_URL="http://${ORCHESTRATOR_HOST}"

COLLECTION_NAME="${COLLECTION_NAME:-orchestrator_demo_collection}"

echo "== [1] Create vectordb collection via orchestrator API =="
RAW="$(curl -sS -m 60 -w $'\n%{http_code}' \
  -X POST "${BASE_URL}/api/v1/orchestrator/vectordb/collections" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"${COLLECTION_NAME}\"
  }")"

HTTP_CODE="$(echo "$RAW" | tail -n1)"
BODY="$(echo "$RAW" | sed '$d')"

echo "HTTP ${HTTP_CODE}"
echo "$BODY" | jq .

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "create collection API failed with HTTP ${HTTP_CODE}" >&2
  exit 1
fi

RESP_NAME="$(echo "$BODY" | jq -r '.name // empty')"
STATUS="$(echo "$BODY" | jq -r '.status // empty')"
if [[ "$RESP_NAME" != "$COLLECTION_NAME" ]]; then
  echo "collection name mismatch: got=$RESP_NAME want=$COLLECTION_NAME" >&2
  exit 1
fi
if [[ "$STATUS" != "true" ]]; then
  echo "create collection status is not true" >&2
  exit 1
fi

echo "create collection API passed."
