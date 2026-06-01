#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

RESET=0
RESET_LOGS=0
BOOTSTRAP_REGTEST=1
WAIT_HEALTH=1
BTC_RPC_HOST_PORT="${BTC_RPC_HOST_PORT:-18443}"
BTC_P2P_HOST_PORT="${BTC_P2P_HOST_PORT:-18444}"

usage() {
  cat <<'EOF'
Usage: ops/scripts/deploy-localnet.sh [options]

Options:
  --reset                 Stop and remove localnet containers/volumes first.
  --reset-logs            With --reset, also remove ops/logs.
  --profiles LIST         Compose profiles, default from COMPOSE_PROFILES or mock.
  --btc-rpc-port PORT     Host RPC port for regtest bitcoind, default 18443.
  --btc-p2p-port PORT     Host P2P port for regtest bitcoind, default 18444.
  --skip-regtest          Do not create/fund regtest wallets.
  --skip-health           Do not wait for localnet health checks.
  -h, --help              Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --reset)
      RESET=1
      ;;
    --reset-logs)
      RESET=1
      RESET_LOGS=1
      ;;
    --profiles)
      COMPOSE_PROFILES="${2:?missing profile list}"
      shift
      ;;
    --btc-rpc-port)
      BTC_RPC_HOST_PORT="${2:?missing BTC RPC port}"
      shift
      ;;
    --btc-p2p-port)
      BTC_P2P_HOST_PORT="${2:?missing BTC P2P port}"
      shift
      ;;
    --skip-regtest)
      BOOTSTRAP_REGTEST=0
      ;;
    --skip-health)
      WAIT_HEALTH=0
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

port_busy() {
  python3 - "$1" <<'PY'
import socket
import sys

port = int(sys.argv[1])
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    sock.bind(("0.0.0.0", port))
except OSError:
    raise SystemExit(0)
raise SystemExit(1)
PY
}

next_free_port() {
	local port="$1"
	local reserved="${2:-}"
	while port_busy "${port}"; do
		port=$((port + 1))
	done
	while [[ -n "${reserved}" && "${port}" == "${reserved}" ]]; do
		port=$((port + 1))
		while port_busy "${port}"; do
			port=$((port + 1))
		done
	done
	echo "${port}"
}

if [[ "${RESET}" == "1" ]]; then
  if [[ "${RESET_LOGS}" == "1" ]]; then
    "${OPS_DIR}/scripts/localnet-reset.sh" --logs
  else
    "${OPS_DIR}/scripts/localnet-reset.sh"
  fi
fi

BTC_RPC_HOST_PORT="$(next_free_port "${BTC_RPC_HOST_PORT}")"
BTC_P2P_HOST_PORT="$(next_free_port "${BTC_P2P_HOST_PORT}" "${BTC_RPC_HOST_PORT}")"

override_file="$(mktemp "${TMPDIR:-/tmp}/thornado-localnet-ports.XXXXXX.yml")"
trap 'rm -f "${override_file}"' EXIT
cat > "${override_file}" <<EOF
services:
  bitcoind-regtest:
    ports: !reset
      - "${BTC_RPC_HOST_PORT}:18443"
      - "${BTC_P2P_HOST_PORT}:18444"
EOF

files=(-f "${COMPOSE_FILE}")
if profile_enabled mock; then
  files+=(-f "${MOCK_COMPOSE_FILE}")
fi
files+=(-f "${override_file}")

profile_args=()
IFS=',' read -r -a profile_parts <<< "${COMPOSE_PROFILES:-mock}"
for profile in "${profile_parts[@]}"; do
  profile="${profile#"${profile%%[![:space:]]*}"}"
  profile="${profile%"${profile##*[![:space:]]}"}"
  if [[ -n "${profile}" ]]; then
    profile_args+=(--profile "${profile}")
  fi
done

echo "Deploying Thornado localnet"
echo "profiles: ${COMPOSE_PROFILES:-mock}"
echo "bitcoind host ports: rpc=${BTC_RPC_HOST_PORT}, p2p=${BTC_P2P_HOST_PORT}"

docker compose --env-file "${ENV_EXAMPLE}" "${files[@]}" "${profile_args[@]}" up -d

if [[ "${WAIT_HEALTH}" == "1" ]]; then
  "${OPS_DIR}/scripts/wait-for-health.sh"
fi

if [[ "${BOOTSTRAP_REGTEST}" == "1" ]]; then
  "${OPS_DIR}/scripts/bootstrap-regtest.sh"
fi

echo "Localnet deployed."
