#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

UPLOAD_HOST="${UPLOAD_HOST:-localhost}"
UPLOAD_PORT="${UPLOAD_PORT:-8000}"
UPLOAD_FILENAME="${UPLOAD_FILENAME:-_page_1_Figure_0.jpeg}"
URL_DOWNLOAD="${URL_DOWNLOAD:-http://${UPLOAD_HOST}:${UPLOAD_PORT}/download/${UPLOAD_FILENAME}}"
FOLDER_DOWNLOAD="${FOLDER_DOWNLOAD:-tmp/minio-upload-test}"
URL_PATH_NO_QUERY="${URL_DOWNLOAD%%\?*}"
OBJECT_FILENAME="${OBJECT_FILENAME:-$(basename "$URL_PATH_NO_QUERY")}"
OBJECT_KEY_DEFAULT="$(basename "$FOLDER_DOWNLOAD")/${OBJECT_FILENAME}"
OBJECT_KEY="${OBJECT_KEY:-$OBJECT_KEY_DEFAULT}"

grpcurl -plaintext -d "{
  \"bucket_name\": \"${MINIO_BUCKET}\",
  \"object_key\": \"${OBJECT_KEY}\"
}" "$MINIO_GRPC_HOST" MinioService.DeleteFile
