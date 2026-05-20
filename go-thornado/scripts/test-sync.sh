#!/bin/bash

set -euo pipefail

NET=$1

# only mainnet, stagenet, and chainnet are supported
if [ "${NET}" != "mainnet" ] && [ "${NET}" != "stagenet" ] && [ "${NET}" != "chainnet" ]; then
  echo "Unsupported network: ${NET}. Only 'mainnet', 'stagenet', and 'chainnet' are supported."
  exit 1
fi

if [ "${NET}" = "mainnet" ]; then
  CHAIN_ID=thorchain-1
elif [ "${NET}" = "chainnet" ]; then
  CHAIN_ID=thorchain-chainnet-1
  SNAPSHOTS_URL=https://chainnet-snapshots.thorchain.network
  EXTRA_ARGS="-e THOR_SEED_NODES_ENDPOINT=https://chainnet-thornode.thorchain.network/thorchain/nodes"
else
  CHAIN_ID=thorchain-stagenet-2
  SNAPSHOTS_URL=https://stagenet-snapshots.ninerealms.com
  EXTRA_ARGS="-e THOR_SEED_NODES_ENDPOINT=https://stagenet-thornode.ninerealms.com/thorchain/nodes"
fi

EXTRA_ARGS="${EXTRA_ARGS-}"
NET_DIR=tmp/sync-test/${NET}

# download snapshot
if [ ! -d "${NET_DIR}/data" ]; then
  mkdir -p "${NET_DIR}/data"

  if [ "${NET}" = "mainnet" ]; then
    # mainnet uses the Liquify snapshot API
    LIQUIFY_API="https://snapshot-api.liquify.com"
    SNAP_TYPE="mainnet-pruned"
    SNAPSHOTS_JSON=$(curl -sf "${LIQUIFY_API}/files/by-network/thorchain?type=${SNAP_TYPE}")
    FILE_ID=$(echo "${SNAPSHOTS_JSON}" | jq -r '.files | sort_by(-.block_height) | .[0] | .id')
    if [ "${FILE_ID}" = "null" ] || [ -z "${FILE_ID}" ]; then
      echo "ERROR: No snapshots found for type ${SNAP_TYPE}" >&2
      exit 1
    fi
    DOWNLOAD_JSON=$(curl -sf -X POST "${LIQUIFY_API}/files/${FILE_ID}/download-url")
    DOWNLOAD_URL=$(echo "${DOWNLOAD_JSON}" | jq -r '.urls | to_entries[0].value')
    SNAP_FILE=$(basename "${DOWNLOAD_URL%%\?*}")
    aria2c --split=16 --max-concurrent-downloads=16 --max-connection-per-server=16 \
      --continue --min-split-size=100M --check-certificate=true \
      --out="${NET_DIR}/${SNAP_FILE}" "${DOWNLOAD_URL}"
    tar xf "${NET_DIR}/${SNAP_FILE}" -C "${NET_DIR}/data"
  else
    # stagenet and chainnet use S3/minio-based snapshots
    LATEST_SNAPSHOT_KEY=$(
      docker run --rm --entrypoint sh minio/minio:latest -c "
      mc alias set minio ${SNAPSHOTS_URL} '' '' >/dev/null;
      mc ls minio/snapshots/thornode --json" | tail -n1 | jq -r .key
    )
    aria2c --split=16 --max-concurrent-downloads=16 --max-connection-per-server=16 \
      --continue --min-split-size=100M --out="${NET_DIR}/${LATEST_SNAPSHOT_KEY}" \
      "${SNAPSHOTS_URL}/snapshots/thornode/${LATEST_SNAPSHOT_KEY}"
    tar xf "${NET_DIR}/${LATEST_SNAPSHOT_KEY}" -C "${NET_DIR}/data"
  fi
fi

# build and start thornode
BUILDTAG=${NET} BRANCH=${NET} make docker-gitlab-build
# trunk-ignore(shellcheck/SC2086)
docker run --rm ${EXTRA_ARGS} \
  -v "$(pwd)/${NET_DIR}/data:/root/.thornado" \
  -e "CHAIN_ID=${CHAIN_ID}" \
  -e "NET=${NET}" \
  -p 1317:1317 \
  -p 27147:27147 \
  "registry.gitlab.com/thorchain/thornode:${NET}"
