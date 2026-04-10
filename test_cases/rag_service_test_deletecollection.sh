#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

COLLECTION="${COLLECTION:-demo_rag_grpcurl}"

grpcurl -plaintext -d "{
  \"name\": \"${COLLECTION}\"
}" "$RAG_HOST" RagService.DeleteCollection
