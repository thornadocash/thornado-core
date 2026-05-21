#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CRATE_DIR="$ROOT_DIR/rust/libgoschnorr"
TARGET_DIR="$CRATE_DIR/target/release"

case "$(uname -s)" in
  Darwin) OS_DIR="darwin"; LIB_NAME="libgoschnorr.dylib" ;;
  Linux) OS_DIR="linux"; LIB_NAME="libgoschnorr.so" ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  arm64|aarch64) ARCH_DIR="arm64" ;;
  x86_64|amd64) ARCH_DIR="amd64" ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

OUT_DIR="${OUT_DIR:-$ROOT_DIR/go-wrappers/includes/$OS_DIR/$ARCH_DIR}"
mkdir -p "$OUT_DIR"

cargo build --release --manifest-path "$CRATE_DIR/Cargo.toml"
cp "$TARGET_DIR/$LIB_NAME" "$OUT_DIR/$LIB_NAME"

echo "$OUT_DIR/$LIB_NAME"
