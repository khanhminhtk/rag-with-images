#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

COLLECTION="${COLLECTION:-demo_rag_grpcurl}"

grpcurl -plaintext -d "{
  \"collection_name\": \"${COLLECTION}\",
  \"vector_name\": \"text_dense\",
  \"vector\": [0.90, 0.10, 0.10, 0.10],
  \"limit\": 2,
  \"with_payload\": true
}" "$RAG_HOST" RagService.SearchPoint

grpcurl -plaintext -d "{
  \"collection_name\": \"${COLLECTION}\",
  \"vector_name\": \"image_dense\",
  \"vector\": [0.10, 0.85, 0.10, 0.10],
  \"limit\": 2,
  \"with_payload\": true
}" "$RAG_HOST" RagService.SearchPoint

grpcurl -plaintext -d "{
  \"collection_name\": \"${COLLECTION}\",
  \"vector_name\": \"bm25\",
  \"query_text\": \"retrieval qdrant\",
  \"limit\": 2,
  \"with_payload\": true
}" "$RAG_HOST" RagService.SearchPoint

grpcurl -plaintext -d "{
  \"collection_name\": \"${COLLECTION}\",
  \"vector_name\": \"text_dense\",
  \"vector\": [0.10, 0.90, 0.10, 0.10],
  \"limit\": 2,
  \"with_payload\": true,
  \"filter\": {
    \"must\": [
      {\"key\": \"lang\", \"operator\": \"eq\", \"string_value\": \"vi\"}
    ]
  }
}" "$RAG_HOST" RagService.SearchPoint
