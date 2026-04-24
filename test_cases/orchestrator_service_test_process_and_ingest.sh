#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ORCHESTRATOR_HOST="${ORCHESTRATOR_HOST:-${SERVICE_HOST}:${ORCHESTRATOR_SERVICE_PORT:-8080}}"
BASE_URL="http://${ORCHESTRATOR_HOST}"

UUID="${UUID:-ai_sota_0022}"
LANG_VALUE="${LANG_VALUE:-vi}"
UPLINK_HOST="${UPLINK_HOST:-localhost}"
UPLINK_PORT="${UPLINK_PORT:-8000}"
UPLINK_PATH="${UPLINK_PATH:-/download/ai_sota_0022.pdf}"
URL_DOWNLOAD="${URL_DOWNLOAD:-http://${UPLINK_HOST}:${UPLINK_PORT}${UPLINK_PATH}}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-900}"
CURL_MAX_TIME="${CURL_MAX_TIME:-1900}"

echo "== [1] Call process-and-ingest API =="
RAW="$(curl -sS -m "$CURL_MAX_TIME" -w $'\n%{http_code}' \
  -X POST "${BASE_URL}/api/v1/orchestrator/training-file/process-and-ingest" \
  -H "Content-Type: application/json" \
  -d "{
    \"uuid\": \"${UUID}\",
    \"url_download\": \"${URL_DOWNLOAD}\",
    \"lang\": \"${LANG_VALUE}\",
    \"timeout_seconds\": ${TIMEOUT_SECONDS}
  }")"

HTTP_CODE="$(echo "$RAW" | tail -n1)"
BODY="$(echo "$RAW" | sed '$d')"

echo "HTTP ${HTTP_CODE}"
echo "$BODY" | jq .

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "process-and-ingest API failed with HTTP ${HTTP_CODE}" >&2
  exit 1
fi

SUCCESS="$(echo "$BODY" | jq -r '.success // empty')"
RESP_UUID="$(echo "$BODY" | jq -r '.uuid // empty')"
VERIFIED="$(echo "$BODY" | jq -r '.verified // empty')"

if [[ "$SUCCESS" != "true" ]]; then
  echo "process-and-ingest success is not true" >&2
  exit 1
fi
if [[ "$RESP_UUID" != "$UUID" ]]; then
  echo "uuid mismatch: got=$RESP_UUID want=$UUID" >&2
  exit 1
fi
if [[ "$VERIFIED" != "true" ]]; then
  echo "verified is not true" >&2
  exit 1
fi

echo "process-and-ingest API passed."
