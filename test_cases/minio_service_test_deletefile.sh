#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

OBJECT_KEY="${OBJECT_KEY:-Screenshot_20260410_162759.png}"

grpcurl -plaintext -d "{
  \"bucket_name\": \"${MINIO_BUCKET}\",
  \"object_key\": \"${OBJECT_KEY}\"
}" "$MINIO_GRPC_HOST" MinioService.DeleteFile
