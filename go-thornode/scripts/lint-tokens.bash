#!/bin/bash
set -euo pipefail

possibly_die() {
  if [[ ${CI_MERGE_REQUEST_TITLE-} == *"#check-lint-warning"* ]]; then
    echo "Merge request is marked unsafe. Please carefully review errors above."
  else
    exit 1
  fi
}

check_token_list() {
  local LIST="$1"

  echo "Linting $LIST"

  git show origin/develop:"$LIST" |
    jq -r '.tokens[] | .address | ascii_downcase' |
    sort -n >/tmp/orig_erc20_token_list.txt

  jq -r '.tokens[] | .address | ascii_downcase' <"$LIST" |
    sort -n >/tmp/modified_erc20_token_list.txt

  # shellcheck disable=SC2155
  local REMOVALS=$(comm -23 /tmp/orig_erc20_token_list.txt /tmp/modified_erc20_token_list.txt)

  # allow removals of dupes
  REMOVALS=$(echo "$REMOVALS" | grep -v -F -f /tmp/modified_erc20_token_list.txt || true)

  # Check for duplicates after filtering out legitimate removals
  local DUPES
  DUPES=$(cat /tmp/orig_erc20_token_list.txt /tmp/modified_erc20_token_list.txt | sort -n | uniq --count | awk '$1 > 2 {print}')
  # Filter out duplicates that are being legitimately removed
  DUPES=$(echo "$DUPES" | grep -v -F -f /tmp/modified_erc20_token_list.txt || true)
  if [ -n "$DUPES" ]; then
    printf "Tokens already in list:\n%s\n" "$DUPES"
    possibly_die
  fi

  if [ -n "$REMOVALS" ]; then
    printf "Tokens removed:\n%s\n" "$REMOVALS"
    possibly_die
  fi
}

check_token_list common/tokenlist/ethtokens/eth_mainnet_latest.json
check_token_list common/tokenlist/avaxtokens/avax_mainnet_latest.json
check_token_list common/tokenlist/bsctokens/bsc_mainnet_latest.json
check_token_list common/tokenlist/basetokens/base_mainnet_latest.json

# case sensitive
# check_token_list common/tokenlist/trontokens/tron_mainnet_latest.json

echo "OK"
