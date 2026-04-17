#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

REQUEST_TOPIC="${REQUEST_TOPIC:-${MINIO_KAFKA_UPLOAD_TOPIC:-minio.upload.request}}"
RESULT_TOPIC="${RESULT_TOPIC:-${MINIO_KAFKA_RESULT_TOPIC:-minio.upload.result}}"
UPLOAD_HOST="${UPLOAD_HOST:-localhost}"
UPLOAD_PORT="${UPLOAD_PORT:-8000}"
UPLOAD_FILENAME="${UPLOAD_FILENAME:-_page_1_Figure_0.jpeg}"
URL_DOWNLOAD="${URL_DOWNLOAD:-http://${UPLOAD_HOST}:${UPLOAD_PORT}/download/${UPLOAD_FILENAME}}"
FOLDER_DOWNLOAD="${FOLDER_DOWNLOAD:-tmp/minio-upload-test}"
MESSAGE_KEY="minio-upload-$(date +%s)-$RANDOM"

echo "== [1] Ensure Kafka topics =="
docker exec -i "$KAFKA_CONTAINER" kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --create --if-not-exists --topic "$REQUEST_TOPIC" --partitions 1 --replication-factor 1 >/dev/null
docker exec -i "$KAFKA_CONTAINER" kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --create --if-not-exists --topic "$RESULT_TOPIC" --partitions 1 --replication-factor 1 >/dev/null

echo "== [2] Publish upload request =="
PAYLOAD=$(printf '{"folderDownload":"%s","urlDownload":"%s"}' "$FOLDER_DOWNLOAD" "$URL_DOWNLOAD")
printf '%s:%s\n' "$MESSAGE_KEY" "$PAYLOAD" | docker exec -i "$KAFKA_CONTAINER" kafka-console-producer.sh \
  --bootstrap-server "$KAFKA_BOOTSTRAP" \
  --topic "$REQUEST_TOPIC" \
  --property parse.key=true \
  --property key.separator=:

echo "Published key=$MESSAGE_KEY"
echo "URL_DOWNLOAD=$URL_DOWNLOAD"

echo "== [3] Wait upload result =="
RAW_RESULT=$(docker exec -i "$KAFKA_CONTAINER" kafka-console-consumer.sh \
  --bootstrap-server "$KAFKA_BOOTSTRAP" \
  --topic "$RESULT_TOPIC" \
  --from-beginning \
  --timeout-ms 15000 \
  --property print.key=true \
  --property key.separator='|' \
  2>/dev/null | grep "^${MESSAGE_KEY}|" | tail -n 1 || true)

if [[ -z "$RAW_RESULT" ]]; then
  echo "No result received for key=$MESSAGE_KEY on topic=$RESULT_TOPIC" >&2
  exit 1
fi

RESULT_JSON="${RAW_RESULT#*|}"
echo "$RESULT_JSON" | jq .

STATUS="$(echo "$RESULT_JSON" | jq -r '.status // empty')"
if [[ "$STATUS" != "success" ]]; then
  echo "Upload pipeline failed. status=$STATUS" >&2
  exit 1
fi

echo "Upload pipeline succeeded."
