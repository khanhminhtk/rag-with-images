#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

TEXT="${TEXT:-Xin chao, day la test embedding text}"

grpcurl -plaintext -d "{
  \"text\": \"${TEXT}\"
}" "$EMBEDDING_HOST" DeepLearningService.EmbedText
