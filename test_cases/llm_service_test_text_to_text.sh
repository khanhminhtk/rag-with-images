#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

MODEL="${MODEL:-${LLM_MODEL:-models/gemini-3-flash-preview}}"
TEMP="${TEMP:-${LLM_TEMPERATURE:-0.7}}"
PROMPT="${PROMPT:-Tom tat ngan ve AI agent}"

grpcurl -plaintext -d "{
  \"model\": \"${MODEL}\",
  \"temperature\": ${TEMP},
  \"prompt\": \"${PROMPT}\",
  \"history\": [
    {\"role\":\"user\",\"content\":\"Chao ban\"},
    {\"role\":\"assistant\",\"content\":\"Chao ban, minh co the giup gi?\"}
  ],
  \"structure_output\": {
    \"answer\":\"string\",
    \"lang\":\"string\"
  }
}" "$LLM_HOST" LlmService.GenerateTextToText
