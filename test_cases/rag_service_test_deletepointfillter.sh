#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

COLLECTION="${COLLECTION:-demo_rag_grpcurl}"

grpcurl -plaintext -d "{
  \"collection_name\": \"${COLLECTION}\",
  \"filter\": {
    \"must\": [
      {\"key\": \"lang\", \"operator\": \"eq\", \"string_value\": \"en\"}
    ]
  }
}" "$RAG_HOST" RagService.DeletePointFilter
