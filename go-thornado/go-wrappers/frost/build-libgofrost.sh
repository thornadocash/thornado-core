#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CRATE_DIR="$REPO_ROOT/crates/thornado-frost-ffi"
TARGET_DIR="$REPO_ROOT/target/release"

case "$(uname -s)" in
Darwin)
  OS_DIR="darwin"
  LIB_NAME="libgofrost.dylib"
  ;;
Linux)
  OS_DIR="linux"
  LIB_NAME="libgofrost.so"
  ;;
*)
  echo "unsupported OS: $(uname -s)" >&2
  exit 1
  ;;
esac

case "$(uname -m)" in
arm64 | aarch64) ARCH_DIR="arm64" ;;
x86_64 | amd64) ARCH_DIR="amd64" ;;
*)
  echo "unsupported arch: $(uname -m)" >&2
  exit 1
  ;;
esac

OUT_DIR="${OUT_DIR:-$SCRIPT_DIR/includes/$OS_DIR/$ARCH_DIR}"
mkdir -p "$OUT_DIR"

cargo build --release -p thornado-frost-ffi
cp "$TARGET_DIR/$LIB_NAME" "$OUT_DIR/$LIB_NAME"

echo "$OUT_DIR/$LIB_NAME"
