#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

MODEL="${MODEL:-${LLM_MODEL:-models/gemini-3-flash-preview}}"
TEMP="${TEMP:-${LLM_TEMPERATURE:-0.7}}"
IMAGE_PATH="${IMAGE_PATH:-$ROOT_DIR/data/test/Screenshot_20260410_162759.png}"
PROMPT="${PROMPT:-Buc anh noi ve gi?}"

grpcurl -plaintext -d "{
  \"model\": \"${MODEL}\",
  \"temperature\": ${TEMP},
  \"image_path\": \"${IMAGE_PATH}\",
  \"prompt\": \"${PROMPT}\",
  \"history\": [
    {\"role\":\"user\",\"content\":\"Chao ban\"},
    {\"role\":\"assistant\",\"content\":\"Chao ban, minh co the giup gi?\"}
  ],
  \"structure_output\": {
    \"answer\":\"string\",
    \"lang\":\"string\"
  }
}" "$LLM_HOST" LlmService.GenerateTextToImage
