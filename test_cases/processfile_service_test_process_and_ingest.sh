#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

REQUEST_TOPIC="${REQUEST_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_REQUEST_TOPIC:-orchestrator.training_file.process_and_ingest.request}}"
RESULT_TOPIC="${RESULT_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_RESULT_TOPIC:-orchestrator.training_file.process_and_ingest.result}}"
RESULT_TIMEOUT_MS="${RESULT_TIMEOUT_MS:-1800000}"

UUID="${UUID:-ai_sota_0022}"
LANG_VALUE="${LANG_VALUE:-vi}"
UPLINK_HOST="${UPLINK_HOST:-localhost}"
UPLINK_PORT="${UPLINK_PORT:-8000}"
UPLINK_PATH="${UPLINK_PATH:-/download/ai_sota_0022.pdf}"
URL_DOWNLOAD="${URL_DOWNLOAD:-http://${UPLINK_HOST}:${UPLINK_PORT}${UPLINK_PATH}}"
CORRELATION_ID="processfile-${UUID}-$(date +%s)"

echo "== [0] Verify processfile service is running =="
PROCESSFILE_PATTERN='processfile_service_test|/processfile_service|cmd/processfile_service/main.go'
if ! pgrep -f "$PROCESSFILE_PATTERN" >/dev/null 2>&1; then
  echo "processfile_service is not running." >&2
  echo "Expected process pattern: $PROCESSFILE_PATTERN" >&2
  echo "Current related processes:" >&2
  pgrep -af 'processfile|dlmodel|rag_service|llm_service' 2>/dev/null || true
  echo "In CI, this service is started by nohup in .gitlab-ci.yml test_e2e." >&2
  echo "For local run, start once before this test (do not start duplicate instances)." >&2
  exit 1
fi
echo "processfile_service is running."
echo

echo "== [1] Validate input file =="
if [[ "$URL_DOWNLOAD" == file://* ]]; then
  PDF_PATH="${URL_DOWNLOAD#file://}"
  [[ -f "$PDF_PATH" ]] || { echo "Input PDF not found: $PDF_PATH" >&2; exit 1; }
  ls -lh "$PDF_PATH"
else
  echo "URL_DOWNLOAD is remote URL: $URL_DOWNLOAD"
  echo "Tip: run uplink with: cd /home/minhtk/code/rag_imtotext_texttoim && python3 fastapi_uplink.py"
fi
echo

echo "== [2] Ensure Kafka topics =="
docker exec -i "$KAFKA_CONTAINER" kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --create --if-not-exists --topic "$REQUEST_TOPIC" --partitions 1 --replication-factor 1 >/dev/null
docker exec -i "$KAFKA_CONTAINER" kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --create --if-not-exists --topic "$RESULT_TOPIC" --partitions 1 --replication-factor 1 >/dev/null

echo "== [3] Publish request =="
docker exec -i "$KAFKA_CONTAINER" kafka-console-producer.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --topic "$REQUEST_TOPIC" <<JSON
{"uuid":"$UUID","url_download":"$URL_DOWNLOAD","lang":"$LANG_VALUE","timeout_seconds":900,"correlation_id":"$CORRELATION_ID"}
JSON

echo "Published correlation_id=$CORRELATION_ID"

echo "== [4] Wait result =="
RAW_RESULT=$(docker exec -i "$KAFKA_CONTAINER" kafka-console-consumer.sh \
  --bootstrap-server "$KAFKA_BOOTSTRAP" \
  --topic "$RESULT_TOPIC" \
  --from-beginning \
  --timeout-ms "$RESULT_TIMEOUT_MS" \
  --property print.key=true \
  --property key.separator='|' \
  2>/dev/null | grep "\"correlation_id\":\"${CORRELATION_ID}\"" | tail -n 1 || true)

if [[ -z "$RAW_RESULT" ]]; then
  echo "No result received for correlation_id=$CORRELATION_ID within ${RESULT_TIMEOUT_MS}ms" >&2
  exit 1
fi

RESULT_JSON="${RAW_RESULT#*|}"
echo "$RESULT_JSON" | jq .

SUCCESS="$(echo "$RESULT_JSON" | jq -r '.success // empty')"
if [[ "$SUCCESS" != "true" ]]; then
  MESSAGE="$(echo "$RESULT_JSON" | jq -r '.message // "unknown error"')"
  echo "Process and ingest failed: $MESSAGE" >&2
  exit 1
fi

echo "Process and ingest succeeded."
