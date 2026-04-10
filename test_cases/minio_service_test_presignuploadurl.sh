#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

OBJECT_KEY="${OBJECT_KEY:-Screenshot_20260410_162759.png}"
EXPIRES="${EXPIRES:-900}"
OUTPUT_FILE="${OUTPUT_FILE:-/tmp/Screenshot_20260410_162759.png}"

URL=$(grpcurl -plaintext -d "{
  \"bucket_name\": \"${MINIO_BUCKET}\",
  \"object_key\": \"${OBJECT_KEY}\",
  \"expires_in_seconds\": ${EXPIRES}
}" "$MINIO_GRPC_HOST" MinioService.PresignUploadURL | jq -r '.url')

echo "$URL"
curl -L "$URL" -o "$OUTPUT_FILE"
