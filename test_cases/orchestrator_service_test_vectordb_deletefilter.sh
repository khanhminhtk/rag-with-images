#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ORCHESTRATOR_HOST="${ORCHESTRATOR_HOST:-${SERVICE_HOST}:${ORCHESTRATOR_SERVICE_PORT:-8080}}"
BASE_URL="http://${ORCHESTRATOR_HOST}"

COLLECTION_NAME="${COLLECTION_NAME:-ai_sota_0022}"
DOC_ID="${DOC_ID:-ai_sota_0022}"

echo "== [1] Delete points by filter via orchestrator API =="
RAW="$(curl -sS -m 60 -w $'\n%{http_code}' \
  -X POST "${BASE_URL}/api/v1/orchestrator/vectordb/points/delete-filter" \
  -H "Content-Type: application/json" \
  -d "{
    \"collection_name\": \"${COLLECTION_NAME}\",
    \"filter\": {
      \"must\": [
        {\"key\": \"doc_id\", \"operator\": \"eq\", \"string_value\": \"${DOC_ID}\"}
      ]
    }
  }")"

HTTP_CODE="$(echo "$RAW" | tail -n1)"
BODY="$(echo "$RAW" | sed '$d')"

echo "HTTP ${HTTP_CODE}"
echo "$BODY" | jq .

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "delete-filter API failed with HTTP ${HTTP_CODE}" >&2
  exit 1
fi

RESP_COLLECTION="$(echo "$BODY" | jq -r '.collection_name // empty')"
STATUS="$(echo "$BODY" | jq -r '.status // empty')"
if [[ "$RESP_COLLECTION" != "$COLLECTION_NAME" ]]; then
  echo "collection_name mismatch: got=$RESP_COLLECTION want=$COLLECTION_NAME" >&2
  exit 1
fi
if [[ "$STATUS" != "true" ]]; then
  echo "delete-filter status is not true" >&2
  exit 1
fi

echo "delete-filter API passed."
