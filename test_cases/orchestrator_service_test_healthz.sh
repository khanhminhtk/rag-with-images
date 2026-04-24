#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ORCHESTRATOR_HOST="${ORCHESTRATOR_HOST:-${SERVICE_HOST}:${ORCHESTRATOR_SERVICE_PORT:-8080}}"
BASE_URL="http://${ORCHESTRATOR_HOST}"

echo "== [1] Check orchestrator healthz =="
RAW="$(curl -sS -m 20 -w $'\n%{http_code}' "${BASE_URL}/healthz")"
HTTP_CODE="$(echo "$RAW" | tail -n1)"
BODY="$(echo "$RAW" | sed '$d')"

echo "HTTP ${HTTP_CODE}"
echo "$BODY" | jq .

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "healthz failed with HTTP ${HTTP_CODE}" >&2
  exit 1
fi

STATUS="$(echo "$BODY" | jq -r '.status // empty')"
if [[ "$STATUS" != "ok" ]]; then
  echo "unexpected health status: ${STATUS}" >&2
  exit 1
fi

echo "healthz passed."
