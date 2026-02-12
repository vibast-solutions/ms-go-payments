#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT_DIR="$ROOT_DIR/app/types"

if ! command -v protoc >/dev/null 2>&1; then
  echo "Error: protoc is not installed or not on PATH." >&2
  exit 1
fi

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "Error: protoc-gen-go is not installed or not on PATH." >&2
  exit 1
fi

if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "Error: protoc-gen-go-grpc is not installed or not on PATH." >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

protoc \
  -I "$ROOT_DIR/proto" \
  --go_out="$OUT_DIR" --go_opt=paths=source_relative \
  --go-grpc_out="$OUT_DIR" --go-grpc_opt=paths=source_relative \
  "payments.proto"

echo "Generated protobuf/grpc files in $OUT_DIR"
