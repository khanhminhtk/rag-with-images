#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

REQUEST_TOPIC="${REQUEST_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_REQUEST_TOPIC:-orchestrator.training_file.process_and_ingest.request}}"
RESULT_TOPIC="${RESULT_TOPIC:-${PROCESS_FILE_SERVICE_KAFKA_PROCESS_FILE_RESULT_TOPIC:-orchestrator.training_file.process_and_ingest.result}}"

UUID="${UUID:-ai_sota_0001}"
COLLECTION_NAME="${COLLECTION_NAME:-ai_sota_0001_collection}"
LANG_VALUE="${LANG_VALUE:-vi}"
URL_DOWNLOAD="${URL_DOWNLOAD:-file:///home/minhtk/code/rag_imtotext_texttoim/tmp/ai_sota_0001.pdf}"
DOWNLOAD_ROOT_DIR="${DOWNLOAD_ROOT_DIR:-$ROOT_DIR/data/download}"
PROCESS_ROOT_DIR="${PROCESS_ROOT_DIR:-$ROOT_DIR/data/processed}"
UPLOAD_ROOT_DIR="${UPLOAD_ROOT_DIR:-$ROOT_DIR/data/upload}"
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
fi
echo

echo "== [2] Ensure Kafka topics =="
docker exec -i "$KAFKA_CONTAINER" kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --create --if-not-exists --topic "$REQUEST_TOPIC" --partitions 1 --replication-factor 1 >/dev/null
docker exec -i "$KAFKA_CONTAINER" kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --create --if-not-exists --topic "$RESULT_TOPIC" --partitions 1 --replication-factor 1 >/dev/null

echo "== [3] Publish request =="
docker exec -i "$KAFKA_CONTAINER" kafka-console-producer.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --topic "$REQUEST_TOPIC" <<JSON
{"uuid":"$UUID","url_download":"$URL_DOWNLOAD","collection_name":"$COLLECTION_NAME","lang":"$LANG_VALUE","timeout_seconds":900,"download_root_dir":"$DOWNLOAD_ROOT_DIR","process_root_dir":"$PROCESS_ROOT_DIR","upload_root_dir":"$UPLOAD_ROOT_DIR","correlation_id":"$CORRELATION_ID"}
JSON

echo "Published correlation_id=$CORRELATION_ID"
echo "\n== [4] Consume result (run in another terminal) =="
echo "docker exec -it $KAFKA_CONTAINER kafka-console-consumer.sh --bootstrap-server $KAFKA_BOOTSTRAP --topic $RESULT_TOPIC --from-beginning"
