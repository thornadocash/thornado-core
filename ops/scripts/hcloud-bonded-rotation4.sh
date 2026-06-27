#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ACTION="${1:-inspect}"
REMOTE_ROOT="${REMOTE_ROOT:-/root/thornado}"
KNOWN_HOSTS="${KNOWN_HOSTS:-/tmp/thornado-hcloud-known-hosts}"
RUN_TIMEOUT_SECONDS="${RUN_TIMEOUT_SECONDS:-7200}"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ROOT_DIR/.env"
  set +a
fi

server_json="$(hcloud server list -o json)"
SERVER_IP="${SERVER_IP:-$(jq -r '[.[] | select(.status == "running" and (.labels.project // "") == "thornado" and (.labels.purpose // "") == "e2e")][0].public_net.ipv4.ip // empty' <<<"$server_json")}"
SERVER_NAME="$(jq -r --arg ip "$SERVER_IP" '.[] | select(.public_net.ipv4.ip == $ip) | .name' <<<"$server_json" | head -n1)"

[[ -n "$SERVER_IP" ]] || {
  echo "no running Thornado e2e HCloud server found; set SERVER_IP" >&2
  exit 1
}

ssh_cmd=(ssh -o BatchMode=yes -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new "root@$SERVER_IP")

inspect_remote() {
  "${ssh_cmd[@]}" 'docker ps -a --format "{{.Names}} {{.Status}} {{.Ports}}" | grep thornado-real5 || true'
  "${ssh_cmd[@]}" 'docker exec thornado-real5-cluster-1 bash -lc "cat /var/lib/thornado/real5/meta/run-summary.json 2>/dev/null || true; curl -m 8 -fsS http://127.0.0.1:1317/thornado/nodes 2>/dev/null | jq -r '\''((if type == \"array\" then . else .nodes end)[]?) | [.status,.node_cons_pub_key,(.bond // .total_bond // \"\")] | @tsv'\'' || true"'
}

sync_remote() {
  "${ssh_cmd[@]}" "mkdir -p '$REMOTE_ROOT/ops/scripts' '$REMOTE_ROOT/ops/docker' '$REMOTE_ROOT/docs' '$REMOTE_ROOT/circuits/tornado' '$REMOTE_ROOT/go-thornado/x/thornado' '$REMOTE_ROOT/go-thornado/bifrost/observer' '$REMOTE_ROOT/go-thornado/bifrost/pkg/chainclients/btc' '$REMOTE_ROOT/go-thornado/bifrost/pubkeymanager' '$REMOTE_ROOT/go-thornado/bifrost/thornadoclient'"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/ops/scripts/real-4node-e2e.sh" \
    "$ROOT_DIR/ops/scripts/docker-real5-node5-churn.sh" \
    "$ROOT_DIR/ops/scripts/tty-expect.py" \
    "root@$SERVER_IP:$REMOTE_ROOT/ops/scripts/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/docs/hcloud-bonded-rotation4-runbook.md" \
    "root@$SERVER_IP:$REMOTE_ROOT/docs/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/ops/docker/Dockerfile.real" \
    "root@$SERVER_IP:$REMOTE_ROOT/ops/docker/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/circuits/tornado/package.json" \
    "$ROOT_DIR/circuits/tornado/package-lock.json" \
    "root@$SERVER_IP:$REMOTE_ROOT/circuits/tornado/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/go-thornado/x/thornado/manager_validator.go" \
    "$ROOT_DIR/go-thornado/x/thornado/manager_validator_current.go" \
    "$ROOT_DIR/go-thornado/x/thornado/manager_validator_test.go" \
    "$ROOT_DIR/go-thornado/x/thornado/manager_network_current.go" \
    "$ROOT_DIR/go-thornado/x/thornado/ante.go" \
    "$ROOT_DIR/go-thornado/x/thornado/handler_observed_tx_helpers.go" \
    "$ROOT_DIR/go-thornado/x/thornado/handler_observed_tx_helpers_test.go" \
    "$ROOT_DIR/go-thornado/x/thornado/shielder_flow_test.go" \
    "root@$SERVER_IP:$REMOTE_ROOT/go-thornado/x/thornado/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/go-thornado/bifrost/observer/attestation_gossip.go" \
    "$ROOT_DIR/go-thornado/bifrost/observer/attestation_gossip_test.go" \
    "root@$SERVER_IP:$REMOTE_ROOT/go-thornado/bifrost/observer/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/go-thornado/bifrost/pkg/chainclients/btc/client.go" \
    "$ROOT_DIR/go-thornado/bifrost/pkg/chainclients/btc/client_internal.go" \
    "$ROOT_DIR/go-thornado/bifrost/pkg/chainclients/btc/common.go" \
    "$ROOT_DIR/go-thornado/bifrost/pkg/chainclients/btc/common_test.go" \
    "$ROOT_DIR/go-thornado/bifrost/pkg/chainclients/btc/bitcoin_test.go" \
    "$ROOT_DIR/go-thornado/bifrost/pkg/chainclients/btc/signer_internal.go" \
    "root@$SERVER_IP:$REMOTE_ROOT/go-thornado/bifrost/pkg/chainclients/btc/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/go-thornado/bifrost/pubkeymanager/pubkey_manager.go" \
    "$ROOT_DIR/go-thornado/bifrost/pubkeymanager/pubkey_manager_test.go" \
    "root@$SERVER_IP:$REMOTE_ROOT/go-thornado/bifrost/pubkeymanager/"
  scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new \
    "$ROOT_DIR/go-thornado/bifrost/thornadoclient/thornado.go" \
    "root@$SERVER_IP:$REMOTE_ROOT/go-thornado/bifrost/thornadoclient/"
}

run_remote() {
  sync_remote
  "${ssh_cmd[@]}" "cd '$REMOTE_ROOT' && FLOW_MODE='${FLOW_MODE:-bonded_rotation4}' FLOW_LIMIT='${FLOW_LIMIT:-1}' PROJECT=thornado-real5 BUILD_ARG='${BUILD_ARG:---build}' ops/scripts/docker-real5-node5-churn.sh reset"
  wait_remote_result
}

wait_remote_result() {
  local start status message
  start="$(date +%s)"
  while true; do
    status="$("${ssh_cmd[@]}" 'docker exec thornado-real5-cluster-1 bash -lc "jq -r .status /var/lib/thornado/real5/meta/run-summary.json 2>/dev/null || true"' 2>/dev/null || true)"
    message="$("${ssh_cmd[@]}" 'docker exec thornado-real5-cluster-1 bash -lc "jq -r .message /var/lib/thornado/real5/meta/run-summary.json 2>/dev/null || true"' 2>/dev/null || true)"
    case "$status" in
      PASS)
        echo "PASS: $message"
        return 0
        ;;
      FAIL)
        echo "FAIL: $message" >&2
        "${ssh_cmd[@]}" 'docker logs --tail=160 thornado-real5-cluster-1 2>&1 | tail -160' >&2 || true
        return 1
        ;;
      RUNNING)
        echo "RUNNING: $message"
        ;;
      *)
        echo "waiting for run summary..."
        ;;
    esac
    if (( "$(date +%s)" - start >= RUN_TIMEOUT_SECONDS )); then
      echo "timed out waiting for bonded_rotation4 result" >&2
      "${ssh_cmd[@]}" 'docker logs --tail=160 thornado-real5-cluster-1 2>&1 | tail -160' >&2 || true
      return 1
    fi
    sleep 30
  done
}

case "$ACTION" in
  inspect)
    echo "server=${SERVER_NAME:-unknown} ip=$SERVER_IP"
    inspect_remote
    ;;
  sync)
    sync_remote
    echo "synced bonded_rotation4 flow to ${SERVER_NAME:-$SERVER_IP}"
    ;;
  run)
    run_remote
    ;;
  *)
    echo "usage: $0 [inspect|sync|run]" >&2
    exit 2
    ;;
esac
