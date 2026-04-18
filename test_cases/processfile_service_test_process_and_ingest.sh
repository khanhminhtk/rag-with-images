#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

REQUEST_TOPIC="${REQUEST_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_REQUEST_TOPIC:-orchestrator.training_file.process_and_ingest.request}}"
RESULT_TOPIC="${RESULT_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_RESULT_TOPIC:-orchestrator.training_file.process_and_ingest.result}}"
RESULT_TIMEOUT_MS="${RESULT_TIMEOUT_MS:-1800000}"

UUID="${UUID:-test}"
LANG_VALUE="${LANG_VALUE:-vi}"
UPLINK_HOST="${UPLINK_HOST:-localhost}"
UPLINK_PORT="${UPLINK_PORT:-8000}"
UPLINK_PATH="${UPLINK_PATH:-/download/ai_sota_0008.pdf}"
URL_DOWNLOAD="${URL_DOWNLOAD:-http://${UPLINK_HOST}:${UPLINK_PORT}${UPLINK_PATH}}"
CORRELATION_ID="processfile-${UUID}-$(date +%s)"

echo "== [0] Start processfile service in another terminal =="
echo "cd $ROOT_DIR && go run cmd/processfile_service/main.go"
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
