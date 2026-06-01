#!/usr/bin/env bash
set -uo pipefail

OPS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$(date +%Y%m%d-%H%M%S)"
RESULT_DIR="${OPS_DIR}/logs/protocol-tests/${RUN_ID}"
RESULT_FILE="${RESULT_DIR}/RESULTS.md"

mkdir -p "${RESULT_DIR}"

total=0
failed=0
blocked=0

write_header() {
  {
    echo "# Protocol Test Results"
    echo
    echo "Run: ${RUN_ID}"
    echo "Args: $*"
    echo
  } > "${RESULT_FILE}"
}

run_section() {
  local name="$1"
  shift
  local slug="$1"
  shift
  local log_file="${RESULT_DIR}/${slug}.log"
  local started
  local finished
  local status

  total=$((total + 1))
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  echo
  echo "== ${name} =="
  echo "command: $*"

  set +e
  "$@" >"${log_file}" 2>&1
  status=$?
  set -e

  finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  {
    echo "## ${name}"
    echo
    echo "- Command: \`$*\`"
    echo "- Started: ${started}"
    echo "- Finished: ${finished}"
    echo "- Exit code: ${status}"
    echo "- Log: \`${log_file}\`"
    echo
    echo "### Results"
  } >> "${RESULT_FILE}"

  if [[ "${status}" == "0" ]]; then
	echo "PASS: ${name}"
	echo "- PASS" >> "${RESULT_FILE}"
  elif [[ "${status}" == "2" ]]; then
	blocked=$((blocked + 1))
	echo "BLOCKED: ${name} (exit ${status})"
	echo "- BLOCKED" >> "${RESULT_FILE}"
	echo "- Last output:" >> "${RESULT_FILE}"
	echo '```text' >> "${RESULT_FILE}"
	tail -n 40 "${log_file}" >> "${RESULT_FILE}"
	echo '```' >> "${RESULT_FILE}"
  else
	failed=$((failed + 1))
	echo "FAIL: ${name} (exit ${status})"
    echo "- FAIL" >> "${RESULT_FILE}"
    echo "- Last output:" >> "${RESULT_FILE}"
    echo '```text' >> "${RESULT_FILE}"
    tail -n 40 "${log_file}" >> "${RESULT_FILE}"
    echo '```' >> "${RESULT_FILE}"
  fi
  echo >> "${RESULT_FILE}"
}

write_header "$@"

scenario_args=()
reset_requested=0
for arg in "$@"; do
  case "${arg}" in
    --reset|--reset-logs)
      reset_requested=1
      ;;
    *)
      scenario_args+=("${arg}")
      ;;
  esac
done

if [[ "${reset_requested}" == "1" ]]; then
  rm -f "${OPS_DIR}/logs/mock-state/state.env"
fi

if [[ "${COMPOSE_PROFILES:-mock}" == *mock* ]]; then
  : "${THORNADO_BOOTSTRAP_CMD:=${OPS_DIR}/scripts/mock-e2e-hooks.sh bootstrap-thornado}"
  : "${FROST_DKG_CMD:=${OPS_DIR}/scripts/mock-e2e-hooks.sh frost-dkg}"
  : "${FROST_DKG_STATUS_CMD:=${OPS_DIR}/scripts/mock-e2e-hooks.sh frost-status}"
  : "${SEND_DEPOSIT_CMD:=${OPS_DIR}/scripts/mock-e2e-hooks.sh send-deposit}"
  : "${RUN_WITHDRAWAL_CMD:=${OPS_DIR}/scripts/mock-e2e-hooks.sh run-withdrawal}"
  export THORNADO_BOOTSTRAP_CMD FROST_DKG_CMD FROST_DKG_STATUS_CMD SEND_DEPOSIT_CMD RUN_WITHDRAWAL_CMD
fi

run_section "Flow 1: Local FROST" "01-local-frost" \
  "${OPS_DIR}/scenarios/10-frost-local.sh"

run_section "Flow 2: Basic Docker Localnet" "02-basic-localnet" \
  "${OPS_DIR}/scenarios/00-basic-localnet.sh" "$@"

run_section "Flow 3: FROST DKG" "03-frost-dkg" \
  "${OPS_DIR}/scenarios/20-frost-dkg.sh" --skip-regtest "${scenario_args[@]+"${scenario_args[@]}"}"

run_section "Flow 4: BTC Deposit + Shielder Commitment" "04-btc-deposit" \
  "${OPS_DIR}/scenarios/30-btc-deposit.sh" --skip-regtest "${scenario_args[@]+"${scenario_args[@]}"}"

run_section "Flow 5: Withdrawal + BTC FROST Keysign" "05-withdrawal" \
  "${OPS_DIR}/scenarios/40-withdrawal.sh" --skip-regtest "${scenario_args[@]+"${scenario_args[@]}"}"

{
  echo "## Summary"
  echo
	echo "- Sections: ${total}"
	echo "- Passed: $((total - failed - blocked))"
	echo "- Blocked: ${blocked}"
	echo "- Failed: ${failed}"
} >> "${RESULT_FILE}"

echo
echo "Results written to: ${RESULT_FILE}"
echo "Passed: $((total - failed - blocked))/${total}"
echo "Blocked: ${blocked}/${total}"
echo "Failed: ${failed}/${total}"

if [[ "${failed}" != "0" || "${blocked}" != "0" ]]; then
	exit 1
fi
