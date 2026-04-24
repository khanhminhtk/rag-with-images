#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

echo "== Loaded ENV =="
echo "ENV_FILE=$ENV_FILE"
echo "RAG_HOST=$RAG_HOST"
echo "LLM_HOST=$LLM_HOST"
echo "MINIO_GRPC_HOST=$MINIO_GRPC_HOST"
echo "EMBEDDING_HOST=$EMBEDDING_HOST"
echo "PROCESS_FILE_HOST=$PROCESS_FILE_HOST"
echo "KAFKA_BOOTSTRAP=$KAFKA_BOOTSTRAP"

echo
echo "== gRPC reflection quick check =="
for host in "$RAG_HOST" "$LLM_HOST" "$MINIO_GRPC_HOST" "$EMBEDDING_HOST" "$PROCESS_FILE_HOST"; do
  if grpcurl -plaintext "$host" list >/dev/null 2>&1; then
    echo "[OK] $host"
  else
    echo "[WARN] cannot reach $host (service may not be started yet)"
  fi
done

echo
echo "Run tests manually (example):"
echo "./test_cases/rag_service_test_createcollection.sh"
echo "./test_cases/rag_service_test_insertpoints.sh"
echo "./test_cases/rag_service_test_searchpoints.sh"
