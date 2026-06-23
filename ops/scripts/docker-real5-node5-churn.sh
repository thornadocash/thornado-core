#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/ops/docker-compose.real5.yml"
PROJECT="${PROJECT:-thornado-real5}"
BUILD_ARG="${BUILD_ARG:---build}"

case "${1:-up}" in
  up)
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d "$BUILD_ARG"
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" ps
    ;;
  down)
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down --remove-orphans
    ;;
  reset)
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down -v --remove-orphans
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d "$BUILD_ARG"
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" ps
    ;;
  logs)
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" logs -f cluster
    ;;
  *)
    echo "usage: $0 [up|down|reset|logs]" >&2
    exit 2
    ;;
esac
