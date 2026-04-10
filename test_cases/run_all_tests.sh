#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

SCRIPTS=(
  rag_service_test_createcollection.sh
  rag_service_test_insertpoints.sh
  rag_service_test_searchpoints.sh
  rag_service_test_deletepointfillter.sh
  rag_service_test_deletecollection.sh
  minio_service_test_uploadfile.sh
  llm_service_test_text_to_text.sh
  dlmodel_service_test_embedding_text.sh
)

for s in "${SCRIPTS[@]}"; do
  echo "== Running: $s =="
  "$(cd "$(dirname "$0")" && pwd)/$s"
  echo

done

echo "All selected tests finished."
