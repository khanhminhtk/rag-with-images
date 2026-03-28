#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

HOST="${HOST:-localhost}"
PORT="${PORT:-50051}"
ADDR="$HOST:$PORT"
COLLECTION="${COLLECTION:-test_e2e_grpc_$(date +%s)}"
SERVER_LOG="${SERVER_LOG:-/tmp/rag_service_e2e_server.log}"
GOCACHE_DIR="${GOCACHE_DIR:-/tmp/go-build}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

wait_for_server() {
  local retries=60
  local sleep_sec=1
  for ((i=1; i<=retries; i++)); do
    if grpcurl -plaintext "$ADDR" list RagService >/dev/null 2>&1; then
      return 0
    fi

    if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
      echo "RAG server process exited early. Log:" >&2
      cat "$SERVER_LOG" >&2 || true
      exit 1
    fi

    sleep "$sleep_sec"
  done

  echo "Timeout waiting for gRPC server at $ADDR" >&2
  cat "$SERVER_LOG" >&2 || true
  exit 1
}

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
}

require_cmd go
require_cmd grpcurl
require_cmd rg

trap cleanup EXIT

echo "[0/6] starting rag_service server on $ADDR"
(
  cd "$ROOT_DIR"
  GOCACHE="$GOCACHE_DIR" go run ./cmd/rag_service/main.go >"$SERVER_LOG" 2>&1
) &
SERVER_PID=$!

wait_for_server

echo "[1/6] create collection: $COLLECTION"
create_out="$(grpcurl -plaintext -d @ "$ADDR" RagService/CreateCollection <<JSON
{
  "name": "$COLLECTION",
  "vectors": [
    {"name": "text_dense", "size": "4", "distance": "cosine"},
    {"name": "image_dense", "size": "4", "distance": "cosine"}
  ],
  "shards": 1,
  "replicationFactor": 1,
  "onDiskPayload": true,
  "optimizersMemmap": true
}
JSON
)"
printf '%s\n' "$create_out"
printf '%s' "$create_out" | rg -q '"status":\s*true' || {
  echo "CreateCollection failed" >&2
  exit 1
}

echo "[2/6] insert points"
insert_out="$(grpcurl -plaintext -d @ "$ADDR" RagService/InsertPoint <<JSON
{
  "collectionName": "$COLLECTION",
  "points": [
    {
      "vectorObject": [
        {"name": "text_dense", "vector": [0.91, 0.12, 0.33, 0.77]}
      ],
      "payload": {
        "doc_id": "doc_1",
        "modality": "text",
        "unit_type": "paragraph",
        "text": "Transformer uses self-attention for sequence modeling.",
        "lang": "en",
        "page": "1",
        "chunk_index": "0",
        "token_count": "20",
        "has_table": "false",
        "has_figure": "false",
        "keywords": "transformer,attention",
        "created_at": "2026-03-28T12:00:00Z"
      }
    },
    {
      "vectorObject": [
        {"name": "text_dense", "vector": [0.54, 0.66, 0.13, 0.47]}
      ],
      "payload": {
        "doc_id": "doc_vi",
        "modality": "text",
        "unit_type": "paragraph",
        "text": "He thong mRAG ket hop OCR va truy xuat.",
        "lang": "vi",
        "page": "5",
        "chunk_index": "4",
        "token_count": "21",
        "has_table": "false",
        "has_figure": "false",
        "keywords": "mRAG,OCR,retrieval",
        "created_at": "2026-03-28T12:00:00Z"
      }
    }
  ]
}
JSON
)"
printf '%s\n' "$insert_out"
printf '%s' "$insert_out" | rg -q '"status":\s*true' || {
  echo "InsertPoint failed" >&2
  exit 1
}

echo "[3/6] search points"
search_out="$(grpcurl -plaintext -d @ "$ADDR" RagService/SearchPoint <<JSON
{
  "collectionName": "$COLLECTION",
  "vectorName": "text_dense",
  "vector": [0.1, 0.2, 0.3, 0.4],
  "queryText": "transformer",
  "limit": "5",
  "withPayload": true
}
JSON
)"
printf '%s\n' "$search_out"
printf '%s' "$search_out" | rg -q '"id":' || {
  echo "SearchPoint returned no results" >&2
  exit 1
}

echo "[4/6] delete by filter lang=vi"
delete_filter_out="$(grpcurl -plaintext -d @ "$ADDR" RagService/DeletePointFilter <<JSON
{
  "collectionName": "$COLLECTION",
  "filter": {
    "must": [
      {"key": "lang", "operator": "eq", "stringValue": "vi"}
    ]
  }
}
JSON
)"
printf '%s\n' "$delete_filter_out"
printf '%s' "$delete_filter_out" | rg -q '"status":\s*true' || {
  echo "DeletePointFilter failed" >&2
  exit 1
}

echo "[5/6] delete collection"
delete_out="$(grpcurl -plaintext -d @ "$ADDR" RagService/DeleteCollection <<JSON
{
  "name": "$COLLECTION"
}
JSON
)"
printf '%s\n' "$delete_out"
printf '%s' "$delete_out" | rg -q '"status":\s*true' || {
  echo "DeleteCollection failed" >&2
  exit 1
}

echo "[6/6] PASS: grpc e2e flow completed"
