#!/usr/bin/env bash

# This script wraps execution of trunk when run in CI.

set -euo pipefail

SCRIPT_DIR="$(dirname "$0")"
BASE_BRANCH="origin/develop"
FLAGS="-j8 --ci"
CHECK_EXISTING=false

if [ -n "${CI_MERGE_REQUEST_ID-}" ]; then
  # if go modules or trunk settings changed, also run with --all on merge requests
  if ! git diff --exit-code "$BASE_BRANCH" -- go.mod go.sum .trunk >/dev/null; then
    FLAGS="$FLAGS --all"
  # if there is a trunk-ignore comment change, also run with --all on merge requests
  elif git diff --unified=0 --no-prefix "$BASE_BRANCH" | sed '/^@@/d' | grep -q 'trunk-ignore'; then
    FLAGS="$FLAGS --all"
  # if this is a merge train, run with --all
  elif [ "${CI_MERGE_REQUEST_EVENT_TYPE-}" = "merge_train" ]; then
    FLAGS="$FLAGS --all"
  else
    FLAGS="$FLAGS --upstream $BASE_BRANCH --show-existing"
    CHECK_EXISTING=true
  fi
else
  FLAGS="$FLAGS --all"
fi

# run trunk
echo "Running: $SCRIPT_DIR/trunk check $FLAGS"
exec 3>&1
# trunk-ignore(shellcheck/SC2086): expanding $FLAGS as flags
OUT=$("$SCRIPT_DIR"/trunk check $FLAGS | tee /dev/fd/3)
exec 3>&-

# confirm that we did not introduce lint errors outside our changes on merge requests
if $CHECK_EXISTING; then
  if echo "$OUT" | grep -q "ISSUES"; then
    echo
    echo "Changes introduce external lint errors."
    exit 1
  fi
fi
