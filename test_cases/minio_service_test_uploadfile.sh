#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

BUCKET="${BUCKET:-$MINIO_BUCKET}"
OBJECT_KEY="${OBJECT_KEY:-demo/test-file.txt}"
EXPIRES="${EXPIRES:-900}"

printf '== Minio endpoint: %s ==\n' "$MINIO_GRPC_HOST"
grpcurl -plaintext "$MINIO_GRPC_HOST" list
grpcurl -plaintext "$MINIO_GRPC_HOST" describe MinioService

echo "== PresignUploadURL =="
grpcurl -plaintext -d "{
  \"bucket_name\": \"${BUCKET}\",
  \"object_key\": \"${OBJECT_KEY}\",
  \"expires_in_seconds\": ${EXPIRES}
}" "$MINIO_GRPC_HOST" MinioService.PresignUploadURL

echo "== DeleteFile =="
grpcurl -plaintext -d "{
  \"bucket_name\": \"${BUCKET}\",
  \"object_key\": \"${OBJECT_KEY}\"
}" "$MINIO_GRPC_HOST" MinioService.DeleteFile
