#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

COORDINATOR_HOST="${COORDINATOR_HOST:-5.223.93.218}"
WORKER_HOSTS="${WORKER_HOSTS:-5.223.51.101 5.223.55.114 5.223.55.174 5.223.92.204}"
REMOTE_ROOT="${REMOTE_ROOT:-/root/thornado}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
TAGS="${TAGS:-regtest mocknet}"
BINS="${BINS:-thornado bifrost}"
ARTIFACT_PORT="${ARTIFACT_PORT:-18080}"
BUILD_ID="${BUILD_ID:-$(date -u +%Y%m%d%H%M%S)}"
RUN_TESTS="${RUN_TESTS:-0}"
TEST_ARGS="${TEST_ARGS:-./x/thornado ./bifrost/pkg/chainclients/btc}"
SKIP_SOURCE_SYNC="${SKIP_SOURCE_SYNC:-1}"
INCLUDE_UNTRACKED="${INCLUDE_UNTRACKED:-0}"
KNOWN_HOSTS="${KNOWN_HOSTS:-/tmp/thornado-hcloud-known-hosts}"
RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260627104200}"

ssh_base=(ssh -o BatchMode=yes -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new)
scp_base=(scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new)
coord="root@$COORDINATOR_HOST"
artifact_name="thornado-binaries-${BUILD_ID}.tgz"
artifact_url="http://${COORDINATOR_HOST}:${ARTIFACT_PORT}/${artifact_name}"
local_tmp="${TMPDIR:-/tmp}/thornado-hcloud-deploy-${BUILD_ID}"

usage() {
  cat <<EOF
usage: $0 [deploy|deploy-restart|deploy-restart-all|build|publish|install|restart-thornado|restart-bifrost]

One deployment path:
  local source delta -> coordinator Linux build -> coordinator HTTP artifact -> worker pull -> hash verify -> atomic install

Environment:
  COORDINATOR_HOST   $COORDINATOR_HOST
  WORKER_HOSTS       $WORKER_HOSTS
  REMOTE_ROOT        $REMOTE_ROOT
  RUN_ROOT           $RUN_ROOT
  RUN_TESTS          $RUN_TESTS
  SOURCE_FILES       optional whitespace-separated file list to sync before build
  SKIP_SOURCE_SYNC   $SKIP_SOURCE_SYNC
  INCLUDE_UNTRACKED  $INCLUDE_UNTRACKED
EOF
}

remote_quote() {
  printf '%q' "$1"
}

ssh_bash() {
  local target="$1"
  local remote_cmd="bash -s --"
  shift
  while (( "$#" )); do
    remote_cmd+=" $(remote_quote "$1")"
    shift
  done
  "${ssh_base[@]}" "$target" "$remote_cmd"
}

collect_source_files() {
  if [[ -n "${SOURCE_FILES:-}" ]]; then
    printf '%s\n' $SOURCE_FILES
    return
  fi

  {
    git -C "$ROOT_DIR" diff --name-only -- go-thornado ops docs
    if [[ "$INCLUDE_UNTRACKED" == "1" ]]; then
      git -C "$ROOT_DIR" ls-files -o --exclude-standard -- go-thornado ops docs
    fi
  } | awk 'NF && $0 !~ /(^|\/)thornado-ui(\/|$)/' | sort -u
}

sync_source_delta() {
  [[ -z "${SOURCE_FILES:-}" && "$SKIP_SOURCE_SYNC" == "1" ]] && {
    echo "source sync skipped"
    return
  }

  source_files=()
  while IFS= read -r source_file; do
    source_files+=("$source_file")
  done < <(collect_source_files)
  if (( ${#source_files[@]} == 0 )); then
    echo "no local source delta"
    return
  fi

  mkdir -p "$local_tmp"
  COPYFILE_DISABLE=1 tar --no-xattrs -C "$ROOT_DIR" -czf "$local_tmp/source-delta.tgz" "${source_files[@]}"
  "${ssh_base[@]}" "$coord" "mkdir -p $(remote_quote "$REMOTE_ROOT")"
  "${scp_base[@]}" "$local_tmp/source-delta.tgz" "$coord:$REMOTE_ROOT/source-delta-${BUILD_ID}.tgz"
  "${ssh_base[@]}" "$coord" "cd $(remote_quote "$REMOTE_ROOT") && tar -xzf source-delta-${BUILD_ID}.tgz && rm -f source-delta-${BUILD_ID}.tgz"
  echo "synced ${#source_files[@]} source files to coordinator"
}

build_on_coordinator() {
  sync_source_delta
  ssh_bash "$coord" "$REMOTE_ROOT" "$GO_BIN" "$TAGS" "$BINS" "$RUN_TESTS" "$TEST_ARGS" "$artifact_name" <<'REMOTE'
set -euo pipefail
remote_root="$1"
go_bin="$2"
tags="$3"
bins="$4"
run_tests="$5"
test_args="$6"
artifact_name="$7"

export PATH="$(dirname "$go_bin"):$PATH"
cd "$remote_root/go-thornado"

if [[ "$run_tests" == "1" ]]; then
  # shellcheck disable=SC2086
  "$go_bin" test -tags "$tags" $test_args
fi

mkdir -p "$remote_root/build"
for bin in $bins; do
  "$go_bin" build -tags "$tags" -o "$remote_root/build/$bin.new" "./cmd/$bin"
  mv "$remote_root/build/$bin.new" "$remote_root/build/$bin"
done

cd "$remote_root/build"
sha256sum $bins > artifacts.sha256
tar -czf "$artifact_name" $bins artifacts.sha256
sha256sum "$artifact_name" > "$artifact_name.sha256"
cat artifacts.sha256
cat "$artifact_name.sha256"
REMOTE
}

start_artifact_server() {
  ssh_bash "$coord" "$REMOTE_ROOT" "$ARTIFACT_PORT" <<'REMOTE'
set -euo pipefail
remote_root="$1"
port="$2"
pid_file="$remote_root/build/artifact-http.pid"

if [[ -s "$pid_file" ]]; then
  old_pid="$(cat "$pid_file")"
  kill "$old_pid" 2>/dev/null || true
fi

cd "$remote_root/build"
nohup python3 -m http.server "$port" --bind 0.0.0.0 > artifact-http.log 2>&1 &
echo "$!" > "$pid_file"
REMOTE
  sleep 2
}

stop_artifact_server() {
  ssh_bash "$coord" "$REMOTE_ROOT" <<'REMOTE' >/dev/null 2>&1 || true
set -euo pipefail
pid_file="$1/build/artifact-http.pid"
if [[ -s "$pid_file" ]]; then
  pid="$(cat "$pid_file")"
  kill "$pid" 2>/dev/null || true
  rm -f "$pid_file"
fi
REMOTE
}

install_worker() {
  local host="$1"
  local expected_artifact_sha="$2"

  ssh_bash "root@$host" "$REMOTE_ROOT" "$BUILD_ID" "$artifact_name" "$artifact_url" "$expected_artifact_sha" "$BINS" <<'REMOTE'
set -euo pipefail
remote_root="$1"
build_id="$2"
artifact_name="$3"
artifact_url="$4"
expected_artifact_sha="$5"
bins="$6"
stage="$remote_root/build/deploy-$build_id"

mkdir -p "$stage" "$remote_root/build"
cd "$stage"
curl -fsS --retry 5 --retry-delay 1 --connect-timeout 5 -o "$artifact_name" "$artifact_url"
printf '%s  %s\n' "$expected_artifact_sha" "$artifact_name" | sha256sum -c -
tar -xzf "$artifact_name"
sha256sum -c artifacts.sha256

cd "$remote_root/build"
for bin in $bins; do
  test -x "$stage/$bin"
  if [[ -f "$bin" ]]; then
    cp -a "$bin" "$bin.prev-$build_id"
  fi
  install -m 0755 "$stage/$bin" "$bin.next-$build_id"
  mv "$bin.next-$build_id" "$bin"
done
sha256sum $bins
REMOTE
}

install_workers() {
  local artifact_sha failures=0 host pid
  artifact_sha="$("${ssh_base[@]}" "$coord" "cut -d' ' -f1 $(remote_quote "$REMOTE_ROOT/build/$artifact_name.sha256")")"

  start_artifact_server
  trap stop_artifact_server EXIT

  mkdir -p "$local_tmp"
  for host in $WORKER_HOSTS; do
    install_worker "$host" "$artifact_sha" >"$local_tmp/install-$host.log" 2>&1 &
    echo "$!" >"$local_tmp/install-$host.pid"
  done

  for host in $WORKER_HOSTS; do
    pid="$(cat "$local_tmp/install-$host.pid")"
    if wait "$pid"; then
      echo "installed $host"
      cat "$local_tmp/install-$host.log"
    else
      echo "install failed $host" >&2
      cat "$local_tmp/install-$host.log" >&2
      failures=$((failures + 1))
    fi
  done

  (( failures == 0 ))
}

restart_bifrost_worker() {
  local host="$1"
  local node="$2"

  ssh_bash "root@$host" "$REMOTE_ROOT" "$RUN_ROOT" "$node" "$BUILD_ID" <<'REMOTE'
set -euo pipefail
remote_root="$1"
run_root="$2"
node="$3"
build_id="$4"
env_file="$run_root/meta/bifrost-$node.restart.env"
log="$run_root/logs/bifrost-$node.restart-$build_id.log"

if [[ ! -s "$env_file" ]]; then
  pid="$(pgrep -f "$remote_root/build/bifrost --log-level" | head -n 1 || true)"
  if [[ -z "$pid" ]]; then
    echo "no bifrost process and no env file for node $node" >&2
    exit 1
  fi
  cp "/proc/$pid/environ" "$env_file"
fi

pkill -TERM -f "$remote_root/build/bifrost --log-level" 2>/dev/null || true
for _ in $(seq 1 20); do
  if ! pgrep -f "$remote_root/build/bifrost --log-level" >/dev/null; then
    break
  fi
  sleep 0.5
done
pkill -KILL -f "$remote_root/build/bifrost --log-level" 2>/dev/null || true

mkdir -p "$run_root/logs" "$run_root/meta"
nohup xargs -0 -a "$env_file" sh -c 'exec env "$@" /root/thornado/build/bifrost --log-level debug' sh >>"$log" 2>&1 &
echo "$!" >"$run_root/meta/bifrost-$node.wrapper.pid"
echo "$log" >"$run_root/meta/bifrost-$node.current-log"

info_base="$(tr '\0' '\n' <"$env_file" | awk -F= '$1=="FROST_INFO_BASE"{print $2; exit}')"
if [[ -z "$info_base" ]]; then
  info_base=10340
fi
port=$((info_base + node))
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:$port/ping" >/dev/null; then
    child="$(pgrep -f "$remote_root/build/bifrost --log-level" | head -n 1 || true)"
    sha256sum "$remote_root/build/bifrost"
    echo "bifrost-$node restarted pid=$child log=$log port=$port"
    exit 0
  fi
  if ! pgrep -f "$remote_root/build/bifrost --log-level" >/dev/null; then
    tail -n 80 "$log" >&2 || true
    exit 1
  fi
  sleep 1
done
tail -n 80 "$log" >&2 || true
exit 1
REMOTE
}

restart_bifrost_workers() {
  local failures=0 host pid node=0
  mkdir -p "$local_tmp"

  for host in $WORKER_HOSTS; do
    node=$((node + 1))
    restart_bifrost_worker "$host" "$node" >"$local_tmp/restart-bifrost-$host.log" 2>&1 &
    echo "$!" >"$local_tmp/restart-bifrost-$host.pid"
  done

  for host in $WORKER_HOSTS; do
    pid="$(cat "$local_tmp/restart-bifrost-$host.pid")"
    if wait "$pid"; then
      echo "restarted bifrost $host"
      cat "$local_tmp/restart-bifrost-$host.log"
    else
      echo "restart bifrost failed $host" >&2
      cat "$local_tmp/restart-bifrost-$host.log" >&2
      failures=$((failures + 1))
    fi
  done

  (( failures == 0 ))
}

restart_thornado_worker() {
  local host="$1"
  local node="$2"

  ssh_bash "root@$host" "$REMOTE_ROOT" "$RUN_ROOT" "$node" "$BUILD_ID" <<'REMOTE'
set -euo pipefail
remote_root="$1"
run_root="$2"
node="$3"
build_id="$4"
cmd_file="$run_root/meta/thornado-$node.restart.cmd"
env_file="$run_root/meta/thornado-$node.restart.env"
log="$run_root/logs/thornado-$node.restart-$build_id.log"
home="$run_root/node$node"

pid="$(pgrep -f "$remote_root/build/thornado start --home $home" | head -n 1 || true)"
if [[ -z "$pid" ]]; then
  pid="$(pgrep -f "$remote_root/build/thornado start" | head -n 1 || true)"
fi
if [[ -n "$pid" ]]; then
  mkdir -p "$run_root/logs" "$run_root/meta"
  cp "/proc/$pid/cmdline" "$cmd_file"
  cp "/proc/$pid/environ" "$env_file"
fi
if [[ ! -s "$cmd_file" || ! -s "$env_file" ]]; then
  echo "no thornado process/env for node $node" >&2
  exit 1
fi

rpc_port="$(tr '\0' '\n' <"$cmd_file" | awk '$0=="--rpc.laddr"{getline; n=split($0,a,":"); print a[n]; exit}')"
if [[ -z "$rpc_port" ]]; then
  rpc_port=$((33360 + node))
fi

if [[ -n "$pid" ]]; then
  kill -TERM "$pid" 2>/dev/null || true
  for _ in $(seq 1 40); do
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    sleep 0.5
  done
  kill -KILL "$pid" 2>/dev/null || true
fi

mapfile -d '' env_args <"$env_file"
mapfile -d '' cmd_args <"$cmd_file"
nohup env "${env_args[@]}" "${cmd_args[@]}" >>"$log" 2>&1 &
echo "$!" >"$run_root/meta/thornado-$node.wrapper.pid"
echo "$log" >"$run_root/meta/thornado-$node.current-log"

for _ in $(seq 1 90); do
  if curl -fsS "http://127.0.0.1:$rpc_port/status" >/dev/null; then
    child="$(pgrep -f "$remote_root/build/thornado start --home $home" | head -n 1 || true)"
    sha256sum "$remote_root/build/thornado"
    echo "thornado-$node restarted pid=$child log=$log rpc_port=$rpc_port"
    exit 0
  fi
  if ! pgrep -f "$remote_root/build/thornado start --home $home" >/dev/null; then
    tail -n 120 "$log" >&2 || true
    exit 1
  fi
  sleep 1
done
tail -n 120 "$log" >&2 || true
exit 1
REMOTE
}

restart_thornado_workers() {
  local failures=0 host node=0
  mkdir -p "$local_tmp"

  for host in $WORKER_HOSTS; do
    node=$((node + 1))
    if restart_thornado_worker "$host" "$node" >"$local_tmp/restart-thornado-$host.log" 2>&1; then
      echo "restarted thornado $host"
      cat "$local_tmp/restart-thornado-$host.log"
    else
      echo "restart thornado failed $host" >&2
      cat "$local_tmp/restart-thornado-$host.log" >&2
      failures=$((failures + 1))
      break
    fi
  done

  (( failures == 0 ))
}

action="${1:-deploy}"
case "$action" in
  deploy)
    build_on_coordinator
    install_workers
    ;;
  deploy-restart)
    build_on_coordinator
    install_workers
    restart_bifrost_workers
    ;;
  deploy-restart-all)
    build_on_coordinator
    install_workers
    restart_thornado_workers
    restart_bifrost_workers
    ;;
  build)
    build_on_coordinator
    ;;
  publish)
    start_artifact_server
    echo "$artifact_url"
    ;;
  install)
    install_workers
    ;;
  restart-bifrost)
    restart_bifrost_workers
    ;;
  restart-thornado)
    restart_thornado_workers
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
