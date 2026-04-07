#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   bash download_and_process_single_file.sh \
#     -pathfilesrc /absolute/or/relative/path/to/file.pdf \
#     -destdir /absolute/or/relative/output_dir \
#     -dev false
#
# Output layout (exactly one file per run):
#   -destdir/<filename_without_ext>/...
#
# Notes:
# - No hardcoded "processed" folder anymore.
# - One script execution processes exactly one source file.
# - Default marker output flag is --output_dir.
#   If your marker build expects --tput_dir:
#   MARKER_OUTPUT_FLAG=--tput_dir bash download_and_process_single_file.sh ...

MARKER_OUTPUT_FLAG="${MARKER_OUTPUT_FLAG:---output_dir}"

PATH_FILE_SRC=""
DEST_DIR=""
DEV_MODE="false"

usage() {
  cat <<USAGE
Usage:
  $0 -pathfilesrc <source_file> -destdir <destination_dir> -dev <true|false>

Arguments:
  -pathfilesrc   Source file path to process (required)
  -destdir       Destination root directory (required)
  -dev           true: skip all processing; false: run normally (required)

Example:
  $0 -pathfilesrc ./data/input/my_paper.pdf -destdir ./data/out -dev false
  # output will be in: ./data/out/my_paper/
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -pathfilesrc)
      [[ $# -ge 2 ]] || { echo "[ERROR] Missing value for -pathfilesrc"; usage; exit 1; }
      PATH_FILE_SRC="$2"
      shift 2
      ;;
    -destdir)
      [[ $# -ge 2 ]] || { echo "[ERROR] Missing value for -destdir"; usage; exit 1; }
      DEST_DIR="$2"
      shift 2
      ;;
    -dev)
      [[ $# -ge 2 ]] || { echo "[ERROR] Missing value for -dev"; usage; exit 1; }
      DEV_MODE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[ERROR] Unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$PATH_FILE_SRC" || -z "$DEST_DIR" || -z "$DEV_MODE" ]]; then
  echo "[ERROR] -pathfilesrc, -destdir, and -dev are required"
  usage
  exit 1
fi

case "$DEV_MODE" in
  true|false)
    ;;
  *)
    echo "[ERROR] -dev must be true or false"
    usage
    exit 1
    ;;
esac

if [[ "$DEV_MODE" == "true" ]]; then
  echo "[SKIP] DEV mode enabled (-dev=true): skipping all processing logic."
  exit 0
fi

if ! command -v marker_single >/dev/null 2>&1; then
  echo "[ERROR] marker_single command not found in PATH"
  exit 1
fi

case "$MARKER_OUTPUT_FLAG" in
  --output_dir|--tput_dir)
    ;;
  *)
    echo "[ERROR] MARKER_OUTPUT_FLAG must be --output_dir or --tput_dir"
    exit 1
    ;;
esac

if [[ ! -f "$PATH_FILE_SRC" ]]; then
  echo "[ERROR] Source file not found: $PATH_FILE_SRC"
  exit 1
fi

SRC_ABS="$(realpath "$PATH_FILE_SRC")"
DEST_ABS="$(realpath -m "$DEST_DIR")"

mkdir -p "$DEST_ABS"

FILE_BASENAME="$(basename "$SRC_ABS")"
FILE_NAME_NO_EXT="${FILE_BASENAME%.*}"
OUT_DIR="$DEST_ABS/$FILE_NAME_NO_EXT"

if [[ -d "$OUT_DIR" ]] && find "$OUT_DIR" -mindepth 1 -print -quit | grep -q .; then
  echo "[SKIP] Output already exists and is not empty: $OUT_DIR"
  echo "[INFO] Done (idempotent skip)."
  exit 0
fi

mkdir -p "$OUT_DIR"

echo "[RUN] marker_single"
echo "[INFO] Source: $SRC_ABS"
echo "[INFO] Dest  : $OUT_DIR"

if [[ "$MARKER_OUTPUT_FLAG" == "--output_dir" ]]; then
  marker_single "$SRC_ABS" --output_dir "$OUT_DIR"
else
  marker_single "$SRC_ABS" --tput_dir "$OUT_DIR"
fi

echo "[DONE] Processed successfully"
echo "[INFO] Output directory: $OUT_DIR"
