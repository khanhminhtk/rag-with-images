#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ORCHESTRATOR_HOST="${ORCHESTRATOR_HOST:-${SERVICE_HOST}:${ORCHESTRATOR_SERVICE_PORT:-8080}}"
BASE_URL="http://${ORCHESTRATOR_HOST}"

SESSION_ID="${SESSION_ID:-session_1234678}"
QUERY="${QUERY:-Ảnh này nó nói về gì?}"
IMAGE_PATH="${IMAGE_PATH:-http://0.0.0.0:8000/download/_page_1_Figure_0.jpeg}"
UUID="${UUID:-ai_sota_0022}"

echo "== [1] Call orchestrator chat API =="
RAW="$(curl -sS -m 180 -w $'\n%{http_code}' \
  -X POST "${BASE_URL}/api/v1/orchestrator/chat" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"${SESSION_ID}\",
    \"query\": \"${QUERY}\",
    \"image_path\": \"${IMAGE_PATH}\",
    \"Uuid\": \"${UUID}\"
  }")"

HTTP_CODE="$(echo "$RAW" | tail -n1)"
BODY="$(echo "$RAW" | sed '$d')"

echo "HTTP ${HTTP_CODE}"
echo "$BODY" | jq .

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "chat API failed with HTTP ${HTTP_CODE}" >&2
  exit 1
fi

ANSWER="$(echo "$BODY" | jq -r '.answer // empty')"
RESP_SESSION_ID="$(echo "$BODY" | jq -r '.session_id // empty')"
if [[ -z "$ANSWER" ]]; then
  echo "chat response has empty answer" >&2
  exit 1
fi
if [[ "$RESP_SESSION_ID" != "$SESSION_ID" ]]; then
  echo "session_id mismatch: got=$RESP_SESSION_ID want=$SESSION_ID" >&2
  exit 1
fi

echo "chat API passed."
