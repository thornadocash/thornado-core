#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OPS_DIR="${ROOT_DIR}/ops"
COMPOSE_FILE="${OPS_DIR}/docker-compose.localnet.yml"
MOCK_COMPOSE_FILE="${OPS_DIR}/docker-compose.mock.yml"
ENV_EXAMPLE="${OPS_DIR}/env.localnet.example"
ENV_FILE="${OPS_DIR}/env.localnet"

load_localnet_env() {
  local requested_profiles="${COMPOSE_PROFILES:-}"

  if [[ -f "${ENV_EXAMPLE}" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "${ENV_EXAMPLE}"
    set +a
  fi

  if [[ -f "${ENV_FILE}" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "${ENV_FILE}"
    set +a
  fi

  if [[ -n "${requested_profiles}" ]]; then
    COMPOSE_PROFILES="${requested_profiles}"
  fi

  COMPOSE_PROFILES="${COMPOSE_PROFILES:-mock}"
  BITCOIN_RPC_USER="${BITCOIN_RPC_USER:-thornado}"
  BITCOIN_RPC_PASSWORD="${BITCOIN_RPC_PASSWORD:-thornado}"

  if profile_enabled mock; then
    THORNADO_BOOTSTRAP_CMD="${THORNADO_BOOTSTRAP_CMD:-${OPS_DIR}/scripts/mock-e2e-hooks.sh bootstrap-thornado}"
    FROST_DKG_CMD="${FROST_DKG_CMD:-${OPS_DIR}/scripts/mock-e2e-hooks.sh frost-dkg}"
    FROST_DKG_STATUS_CMD="${FROST_DKG_STATUS_CMD:-${OPS_DIR}/scripts/mock-e2e-hooks.sh frost-status}"
    SEND_DEPOSIT_CMD="${SEND_DEPOSIT_CMD:-${OPS_DIR}/scripts/mock-e2e-hooks.sh send-deposit}"
    RUN_WITHDRAWAL_CMD="${RUN_WITHDRAWAL_CMD:-${OPS_DIR}/scripts/mock-e2e-hooks.sh run-withdrawal}"
    export THORNADO_BOOTSTRAP_CMD FROST_DKG_CMD FROST_DKG_STATUS_CMD SEND_DEPOSIT_CMD RUN_WITHDRAWAL_CMD
  fi
}

compose_cmd() {
  local files=(-f "${COMPOSE_FILE}")
  if profile_enabled mock; then
    files+=(-f "${MOCK_COMPOSE_FILE}")
  fi
  docker compose --env-file "${ENV_EXAMPLE}" "${files[@]}" "$@"
}

compose_with_profiles() {
  local args=()
  local files=(-f "${COMPOSE_FILE}")
  local profiles="${COMPOSE_PROFILES:-mock}"
  local profile

  if profile_enabled mock; then
    files+=(-f "${MOCK_COMPOSE_FILE}")
  fi

  IFS=',' read -r -a profile_parts <<< "${profiles}"
  for profile in "${profile_parts[@]}"; do
    profile="${profile#"${profile%%[![:space:]]*}"}"
    profile="${profile%"${profile##*[![:space:]]}"}"
    if [[ -n "${profile}" ]]; then
      args+=(--profile "${profile}")
    fi
  done

  docker compose --env-file "${ENV_EXAMPLE}" "${files[@]}" "${args[@]}" "$@"
}

profile_enabled() {
  local wanted="$1"
  local profiles="${COMPOSE_PROFILES:-mock}"
  local profile

  IFS=',' read -r -a profile_parts <<< "${profiles}"
  for profile in "${profile_parts[@]}"; do
    profile="${profile#"${profile%%[![:space:]]*}"}"
    profile="${profile%"${profile##*[![:space:]]}"}"
    if [[ "${profile}" == "${wanted}" ]]; then
      return 0
    fi
  done

  return 1
}

service_container_id() {
  local service="$1"
  compose_cmd ps -q "${service}" 2>/dev/null || true
}

require_service_running() {
  local service="$1"
  local cid
  cid="$(service_container_id "${service}")"

  if [[ -z "${cid}" ]]; then
    echo "Service is not running: ${service}" >&2
    echo "Start localnet first: ops/scripts/localnet-up.sh" >&2
    exit 1
  fi
}

exec_service() {
  local service="$1"
  shift
  require_service_running "${service}"
  compose_cmd exec -T "${service}" "$@"
}

bitcoin_cli() {
  exec_service bitcoind-regtest bitcoin-cli \
    -regtest \
    -rpcuser="${BITCOIN_RPC_USER}" \
    -rpcpassword="${BITCOIN_RPC_PASSWORD}" \
    "$@"
}

wait_for_container_healthy() {
  local service="$1"
  local timeout="${2:-120}"
  local start
  local cid
  local status

  start="$(date +%s)"
  require_service_running "${service}"
  cid="$(service_container_id "${service}")"

  while true; do
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${cid}" 2>/dev/null || true)"

    if [[ "${status}" == "healthy" || "${status}" == "running" ]]; then
      echo "${service} is ${status}"
      return 0
    fi

    if (( "$(date +%s)" - start >= timeout )); then
      echo "Timed out waiting for ${service}; last status: ${status:-unknown}" >&2
      return 1
    fi

    sleep 2
  done
}

wait_for_tcp() {
  local host="$1"
  local port="$2"
  local label="${3:-${host}:${port}}"
  local timeout="${4:-120}"
  local start

  start="$(date +%s)"
  while true; do
    if (echo > "/dev/tcp/${host}/${port}") >/dev/null 2>&1; then
      echo "${label} is reachable"
      return 0
    fi

    if (( "$(date +%s)" - start >= timeout )); then
      echo "Timed out waiting for ${label}" >&2
      return 1
    fi

    sleep 2
  done
}

run_hook_or_explain() {
  local env_name="$1"
  local description="$2"
  local required_contract="$3"
  local command="${!env_name:-}"

  if [[ -z "${command}" ]]; then
    echo "No command configured for: ${description}" >&2
    echo "Set ${env_name} in ops/env.localnet once the owning workstream exposes this contract." >&2
    echo "Required contract: ${required_contract}" >&2
    exit 2
  fi

  echo "Running ${description}: ${command}"
  bash -lc "${command}"
}

missing_build_contexts() {
  local missing=0
  local dir

  if profile_enabled mock; then
    return 0
  fi

  for dir in go-thornado; do
    if [[ ! -d "${ROOT_DIR}/${dir}" ]]; then
      echo "Missing build context: ${ROOT_DIR}/${dir}" >&2
      missing=1
    fi
  done
  return "${missing}"
}
