#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

COLLECTION="${COLLECTION:-demo_rag_grpcurl}"

grpcurl -plaintext -d "{
  \"collection_name\": \"${COLLECTION}\",
  \"points\": [
    {
      \"vectorObject\": [
        {\"name\": \"text_dense\", \"vector\": [0.91, 0.10, 0.10, 0.10]},
        {\"name\": \"image_dense\", \"vector\": [0.80, 0.20, 0.10, 0.10]}
      ],
      \"payload\": {
        \"doc_id\": \"doc-001\",
        \"text\": \"AI agent co the tu lap ke hoach\",
        \"modality\": \"text\",
        \"lang\": \"vi\",
        \"chunk_index\": \"1\",
        \"has_figure\": \"false\"
      }
    },
    {
      \"vectorObject\": [
        {\"name\": \"text_dense\", \"vector\": [0.10, 0.90, 0.10, 0.10]},
        {\"name\": \"image_dense\", \"vector\": [0.10, 0.85, 0.10, 0.10]}
      ],
      \"payload\": {
        \"doc_id\": \"doc-002\",
        \"text\": \"He thong retrieval dung Qdrant\",
        \"modality\": \"text\",
        \"lang\": \"vi\",
        \"chunk_index\": \"2\",
        \"has_figure\": \"true\"
      }
    },
    {
      \"vectorObject\": [
        {\"name\": \"text_dense\", \"vector\": [0.10, 0.10, 0.90, 0.10]},
        {\"name\": \"image_dense\", \"vector\": [0.10, 0.10, 0.85, 0.10]}
      ],
      \"payload\": {
        \"doc_id\": \"doc-003\",
        \"text\": \"BM25 ho tro lexical search\",
        \"modality\": \"text\",
        \"lang\": \"en\",
        \"chunk_index\": \"3\",
        \"has_figure\": \"false\"
      }
    }
  ]
}" "$RAG_HOST" RagService.InsertPoint
