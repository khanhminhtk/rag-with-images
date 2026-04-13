#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

REQUEST_TOPIC="${REQUEST_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_REQUEST_TOPIC:-orchestrator.training_file.process_and_ingest.request}}"
RESULT_TOPIC="${RESULT_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_RESULT_TOPIC:-orchestrator.training_file.process_and_ingest.result}}"

UUID="${UUID:-ai_sota_0001}"
LANG_VALUE="${LANG_VALUE:-vi}"
UPLINK_HOST="${UPLINK_HOST:-localhost}"
UPLINK_PORT="${UPLINK_PORT:-8000}"
UPLINK_PATH="${UPLINK_PATH:-/download/ai_sota_0001.pdf}"
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
echo "\n== [4] Consume result (run in another terminal) =="
echo "docker exec -it $KAFKA_CONTAINER kafka-console-consumer.sh --bootstrap-server $KAFKA_BOOTSTRAP --topic $RESULT_TOPIC --from-beginning"
