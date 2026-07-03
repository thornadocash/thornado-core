#!/usr/bin/env bash
# Final keygen/keysign/throughput assessment for the Rust bifrost cluster.
#
# Pulls each node's daemon.log instrumentation (DKG_TIMING, KEYSIGN_TIMING,
# party formed, signed-and-broadcast batches) plus chain-side state (halts,
# churn config, unsettled txouts, vault statuses) and prints a summary with
# min/p50/avg/max for every timing series, batch-size distribution, and
# settle throughput. Read-only: safe to run against a live cluster.
#
# Usage:
#   NODES="6:5.223.53.113 7:5.223.75.75 8:5.223.92.204 9:5.223.93.218" \
#   [SINCE="2026-07-03T04:00:00"] [API_NODE=5.223.75.75] \
#   ops/scripts/bifrost-metrics-report.sh
#
# SINCE filters log lines to the test window (ISO prefix match, UTC).

set -euo pipefail

NODES="${NODES:?set NODES=\"n:ip ...\"}"
SINCE="${SINCE:-}"
API_NODE="${API_NODE:-${NODES##* }}"
API_NODE="${API_NODE#*:}"
LOG_PATH="${LOG_PATH:-/root/rust-bifrost-live/daemon.log}"
SSH_OPTS=(-o ConnectTimeout=10 -o StrictHostKeyChecking=no)

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

for spec in $NODES; do
  n="${spec%%:*}"; ip="${spec#*:}"
  ssh "${SSH_OPTS[@]}" "root@$ip" "cat $LOG_PATH" 2>/dev/null \
    | sed -e 's/\x1b\[[0-9;]*m//g' >"$tmp/node$n.log" || true
done

ssh "${SSH_OPTS[@]}" "root@$API_NODE" '
  curl -s localhost:2377/thornado/config;
  echo "===TXOUT===";
  curl -s localhost:2377/thornado/txout;
  echo "===NODES===";
  curl -s localhost:2377/thornado/nodes;
  echo "===LASTBLOCK===";
  curl -s localhost:2377/thornado/lastblock
' >"$tmp/chain.json" 2>/dev/null || true

python3 - "$tmp" "$SINCE" <<'PYEOF'
import json
import os
import re
import statistics
import sys

tmpdir, since = sys.argv[1], sys.argv[2]

def stats(name, xs, unit="ms"):
    if not xs:
        print(f"  {name:<28} (no samples)")
        return
    xs = sorted(xs)
    p50 = xs[len(xs) // 2]
    print(
        f"  {name:<28} n={len(xs):<5} min={xs[0]:<7} p50={p50:<7} "
        f"avg={statistics.mean(xs):<9.1f} max={xs[-1]:<7} {unit}"
    )

def grab(line, key):
    m = re.search(rf"{key}=(\d+)", line)
    return int(m.group(1)) if m else None

def ts_of(line):
    m = re.match(r"(\S+)", line)
    return m.group(1) if m else ""

dkg, keysign, per_input, party, batch_items = [], [], [], [], []
broadcasts, per_node = [], {}

for fn in sorted(os.listdir(tmpdir)):
    if not fn.endswith(".log"):
        continue
    node = fn[:-4]
    counts = {"dkg": 0, "keysign": 0, "broadcast": 0}
    with open(os.path.join(tmpdir, fn), errors="replace") as f:
        for line in f:
            if since and ts_of(line) < since:
                continue
            if "DKG_TIMING" in line:
                v = grab(line, "dkg_ms")
                if v is not None:
                    dkg.append(v)
                    counts["dkg"] += 1
            elif "KEYSIGN_TIMING" in line:
                v = grab(line, "keysign_ms")
                if v is not None:
                    keysign.append(v)
                    counts["keysign"] += 1
                v = grab(line, "per_input_ms")
                if v is not None:
                    per_input.append(v)
            elif "party formed" in line:
                v = grab(line, "party_ms")
                if v is not None:
                    party.append(v)
            elif "signed and broadcast outbound batch" in line:
                v = grab(line, "items")
                if v is not None:
                    batch_items.append(v)
                counts["broadcast"] += 1
                broadcasts.append(ts_of(line))
    per_node[node] = counts

print("== Rust bifrost timing metrics" + (f" (since {since} UTC)" if since else ""))
stats("keygen DKG (dkg_ms)", dkg)
stats("keysign total (keysign_ms)", keysign)
stats("keysign per input", per_input)
stats("party formation (party_ms)", party)
stats("batch size (items/tx)", batch_items, unit="items")

if broadcasts:
    broadcasts.sort()
    print(f"\n== Throughput")
    print(f"  outbound txs broadcast       {len(broadcasts)}")
    print(f"  items signed                 {sum(batch_items) if batch_items else len(broadcasts)}")
    first, last = broadcasts[0], broadcasts[-1]
    print(f"  window                       {first} .. {last}")
    def mins(a, b):
        try:
            from datetime import datetime
            fmt = "%Y-%m-%dT%H:%M:%S"
            return max(
                (datetime.strptime(b[:19], fmt) - datetime.strptime(a[:19], fmt)).total_seconds() / 60.0,
                1 / 60.0,
            )
        except ValueError:
            return None
    m = mins(first, last)
    if m and batch_items:
        print(f"  items/minute                 {sum(batch_items)/m:.2f} over {m:.1f} min")

print("\n== Per node")
for node, counts in sorted(per_node.items()):
    print(f"  {node:<8} dkg={counts['dkg']} keysign={counts['keysign']} broadcasts={counts['broadcast']}")

chain_path = os.path.join(tmpdir, "chain.json")
if os.path.exists(chain_path):
    raw = open(chain_path, errors="replace").read()
    parts = raw.split("===TXOUT===")
    cfg = {}
    try:
        cfg = json.loads(parts[0])
    except ValueError:
        pass
    print("\n== Chain state")
    for key in sorted(cfg):
        if any(s in key for s in ("HALT", "CHURN", "NODE_SETDESIRED")):
            print(f"  {key:<32} {cfg[key].get('value')}")
    if len(parts) > 1:
        rest = parts[1].split("===NODES===")
        try:
            txo = json.loads(rest[0])
            unsettled = []
            for blk in txo.get("txouts", []):
                for it in blk.get("tx_array", []):
                    if not it.get("out_hash"):
                        unsettled.append((blk["height"], it.get("tx_type")))
            print(f"  unsettled txout items            {len(unsettled)}")
            for h, t in unsettled[:10]:
                print(f"    height={h} type={t}")
        except ValueError:
            pass
        if len(rest) > 1:
            nodes_part = rest[1].split("===LASTBLOCK===")
            try:
                nodes = json.loads(nodes_part[0])
                by_status = {}
                for na in nodes:
                    by_status.setdefault(na.get("status"), 0)
                    by_status[na.get("status")] += 1
                print(f"  nodes by status                  {by_status}")
            except ValueError:
                pass
            if len(nodes_part) > 1:
                try:
                    lb = json.loads(nodes_part[1])
                    print(f"  heights                          {lb[0]}")
                except (ValueError, IndexError):
                    pass
PYEOF
