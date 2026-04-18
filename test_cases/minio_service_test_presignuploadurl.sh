#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

UPLOAD_HOST="${UPLOAD_HOST:-localhost}"
UPLOAD_PORT="${UPLOAD_PORT:-8000}"
UPLOAD_FILENAME="${UPLOAD_FILENAME:-_page_1_Figure_4.jpeg}"
URL_DOWNLOAD="${URL_DOWNLOAD:-http://${UPLOAD_HOST}:${UPLOAD_PORT}/download/${UPLOAD_FILENAME}}"
FOLDER_DOWNLOAD="${FOLDER_DOWNLOAD:-tmp/minio-upload-test}"
URL_PATH_NO_QUERY="${URL_DOWNLOAD%%\?*}"
OBJECT_FILENAME="${OBJECT_FILENAME:-$(basename "$URL_PATH_NO_QUERY")}"
OBJECT_KEY_DEFAULT="$(basename "$FOLDER_DOWNLOAD")/${OBJECT_FILENAME}"
OBJECT_KEY="${OBJECT_KEY:-$OBJECT_KEY_DEFAULT}"
EXPIRES="${EXPIRES:-900}"
OUTPUT_FILE="${OUTPUT_FILE:-/tmp/${OBJECT_FILENAME}}"

URL=$(grpcurl -plaintext -d "{
  \"bucket_name\": \"${MINIO_BUCKET}\",
  \"object_key\": \"${OBJECT_KEY}\",
  \"expires_in_seconds\": ${EXPIRES}
}" "$MINIO_GRPC_HOST" MinioService.PresignUploadURL | jq -r '.url')

if [[ -z "$URL" || "$URL" == "null" ]]; then
  echo "Failed to get presigned URL for object_key=${OBJECT_KEY}" >&2
  exit 1
fi

echo "$URL"
curl -L "$URL" -o "$OUTPUT_FILE"
echo "Downloaded to $OUTPUT_FILE"
