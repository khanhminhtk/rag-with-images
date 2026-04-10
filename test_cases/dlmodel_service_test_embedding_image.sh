#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

IMAGE_PATH="${IMAGE_PATH:-$ROOT_DIR/data/test/Screenshot_20260410_162759.png}"
PAYLOAD_JSON="$(mktemp /tmp/embed-image-request-XXXXXX.json)"
TMP_GO="$(mktemp "$ROOT_DIR/tmp_embed_image_payload_XXXXXX.go")"

cleanup() {
  rm -f "$PAYLOAD_JSON" "$TMP_GO"
}
trap cleanup EXIT

cat > "$TMP_GO" <<'GOEOF'
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"rag_imagetotext_texttoimage/internal/util"
)

type embedImageRequest struct {
	Images   []string `json:"images"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Channels int      `json:"channels"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run <this-file.go> <image_path>")
		os.Exit(1)
	}

	pixels, w, h, c, err := util.LoadImagePixels(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	req := embedImageRequest{
		Images:   []string{base64.StdEncoding.EncodeToString(pixels)},
		Width:    w,
		Height:   h,
		Channels: c,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(req); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
GOEOF

echo "== [1] Build EmbedImage request from image using util.LoadImagePixels =="
(cd "$ROOT_DIR" && GOCACHE=/tmp/go-build-cache go run "$TMP_GO" "$IMAGE_PATH" > "$PAYLOAD_JSON")
wc -c "$PAYLOAD_JSON"

echo "== [2] Call DeepLearningService.EmbedImage =="
grpcurl -plaintext -d @ "$EMBEDDING_HOST" DeepLearningService.EmbedImage < "$PAYLOAD_JSON"
