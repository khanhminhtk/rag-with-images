#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

SCRIPTS=(
  orchestrator_service_test_healthz.sh
  orchestrator_service_test_chat.sh
  orchestrator_service_test_vectordb_deletecollection.sh
  orchestrator_service_test_vectordb_createcollection.sh
  orchestrator_service_test_process_and_ingest.sh
  orchestrator_service_test_vectordb_deletefilter.sh
  orchestrator_service_test_vectordb_deletecollection.sh
)

for s in "${SCRIPTS[@]}"; do
  echo "== Running: $s =="
  "$(cd "$(dirname "$0")" && pwd)/$s"
  echo
done

echo "All orchestrator API tests finished."
