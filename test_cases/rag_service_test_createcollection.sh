#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

COLLECTION="${COLLECTION:-demo_rag_grpcurl}"

grpcurl -plaintext -d "{
  \"name\": \"${COLLECTION}\",
  \"vectors\": [
    {\"name\": \"text_dense\", \"size\": 4, \"distance\": \"cosine\"},
    {\"name\": \"image_dense\", \"size\": 4, \"distance\": \"cosine\"}
  ],
  \"shards\": 1,
  \"replication_factor\": 1,
  \"on_disk_payload\": true,
  \"optimizers_memmap\": true
}" "$RAG_HOST" RagService.CreateCollection
