#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNNER="${RUNNER:-/mnt/fast4tb/tmp/local-report-runner-lowfanout-gap}"
OUT_ROOT="${OUT_ROOT:-/mnt/fast4tb/ironbird-lowfanout-gap-$(date -u +%Y%m%dT%H%M%SZ)}"
LOAD_WINDOW_MIN="${LOAD_WINDOW_MIN:-5m}"
LOAD_WINDOW_TARGET_FRACTION="${LOAD_WINDOW_TARGET_FRACTION:-0.995}"
DRAIN_TIMEOUT="${DRAIN_TIMEOUT:-5m}"
STOP_CATALYST_AFTER_LOAD_WINDOW="${STOP_CATALYST_AFTER_LOAD_WINDOW:-true}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-5}"
VALIDATORS="${VALIDATORS:-1}"
NODES="${NODES:-0}"
WALLETS="${WALLETS:-5000}"
PRESEED_ACCOUNTS="${PRESEED_ACCOUNTS:-100000}"
SKIP_BUILD="${SKIP_BUILD:-false}"
TMPDIR="${TMPDIR:-/mnt/fast4tb/tmp}"
ACTIVE_WINDOW_PROFILE_DURATION="${ACTIVE_WINDOW_PROFILE_DURATION:-30s}"

mkdir -p "$OUT_ROOT" "$(dirname "$RUNNER")" "$TMPDIR"

if [[ ! -x "$RUNNER" || "${REBUILD_RUNNER:-false}" == "true" ]]; then
  (cd "$ROOT" && GOWORK=off go build -o "$RUNNER" ./cmd/local-report-runner)
fi

summary="$OUT_ROOT/summary.tsv"
if [[ ! -f "$summary" ]]; then
  printf 'workload\tscenario\tmode\tapp_backend\tnode_backend\tattempt\taccepted\tblocks\ttxs_per_block\tload_window_seconds\tsuccessful\truntime_tps\tload_window_tps\twall_tps\tphase_overlap_rows\tload_phase_in_window_seconds\tabci_commit_seconds\tabci_commit_count\ttreedb_write_sync_count\ttreedb_checkpoint_count\tjson\n' > "$summary"
fi

log() {
  printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

json_bool() {
  jq -r "$1 // false" "$2"
}

json_num() {
  jq -r "$1 // 0" "$2"
}

scenario_mode() {
  case "$1" in
    simapp-goleveldb|simapp-treedb) printf 'app-only' ;;
    simapp-goleveldb-all|simapp-treedb-all) printf 'all-db' ;;
    *) printf 'unknown' ;;
  esac
}

scenario_app_backend() {
  case "$1" in
    simapp-goleveldb|simapp-goleveldb-all) printf 'goleveldb' ;;
    simapp-treedb|simapp-treedb-all) printf 'treedb' ;;
    *) printf 'unknown' ;;
  esac
}

scenario_node_backend() {
  case "$1" in
    simapp-goleveldb-all) printf 'goleveldb' ;;
    simapp-treedb-all) printf 'treedb' ;;
    simapp-goleveldb|simapp-treedb) printf 'default' ;;
    *) printf 'unknown' ;;
  esac
}

run_one() {
  local workload="$1"
  local scenario="$2"
  local attempt="$3"
  local blocks="$4"
  local txs="$5"
  local msg="$6"
  local contained="$7"
  local msgs_per_tx="$8"
  local recipients="$9"
  local max_gas="${10}"

  local mode app_backend node_backend out_dir out_json profile_dir
  mode="$(scenario_mode "$scenario")"
  app_backend="$(scenario_app_backend "$scenario")"
  node_backend="$(scenario_node_backend "$scenario")"
  out_dir="$OUT_ROOT/$workload/$scenario/attempt-$attempt"
  out_json="$out_dir/result.json"
  profile_dir="$out_dir/pprof"
  mkdir -p "$profile_dir"

  local runner_args=(
    -scenario "$scenario" \
    -validators "$VALIDATORS" -nodes "$NODES" -wallets "$WALLETS" \
    -preseed-profile accounts -preseed-accounts "$PRESEED_ACCOUNTS" \
    -cosmos-blocks "$blocks" -cosmos-txs "$txs" \
    -cosmos-msg "$msg" -cosmos-contained-msg "$contained" \
    -cosmos-msgs-per-tx "$msgs_per_tx" -cosmos-recipients "$recipients" \
    -cosmos-max-gas "$max_gas" \
    -load-window-min-duration "$LOAD_WINDOW_MIN" \
    -load-window-target-fraction "$LOAD_WINDOW_TARGET_FRACTION" \
    -load-window-drain-timeout "$DRAIN_TIMEOUT" \
    -raw-tx-audit=false \
    -app-debug-vars \
    -app-active-window-profile-dir "$profile_dir" \
    -app-active-window-profile-duration "$ACTIVE_WINDOW_PROFILE_DURATION" \
    -out "$out_json"
  )
  if [[ "$SKIP_BUILD" == "true" ]]; then
    runner_args+=(-skip-build)
  fi
  if [[ "$STOP_CATALYST_AFTER_LOAD_WINDOW" == "true" ]]; then
    runner_args+=(-stop-catalyst-after-load-window)
  fi

  log "running workload=$workload scenario=$scenario attempt=$attempt blocks=$blocks txs=$txs log=$out_dir/runner.log"
  if ! TMPDIR="$TMPDIR" "$RUNNER" "${runner_args[@]}" >"$out_dir/runner.log" 2>&1; then
    log "runner failed for workload=$workload scenario=$scenario attempt=$attempt; tail follows"
    tail -120 "$out_dir/runner.log" >&2 || true
    return 1
  fi

  local reached duration_satisfied result_error seconds successful runtime_tps load_window_tps wall_tps accepted
  local phase_overlap_rows load_phase_in_window abci_commit_seconds abci_commit_count write_sync_count checkpoint_count
  reached="$(json_bool '.results[0].load_window.reached' "$out_json")"
  duration_satisfied="$(json_bool '.results[0].load_window.duration_satisfied' "$out_json")"
  result_error="$(jq -r 'if (.results[0].error == null) then "" else (.results[0].error | tostring) end' "$out_json")"
  seconds="$(json_num '.results[0].load_window.seconds' "$out_json")"
  successful="$(json_num '.results[0].corrected_load_test.successful_transactions' "$out_json")"
  runtime_tps="$(json_num '.results[0].derived_metrics.runtime_included_tps' "$out_json")"
  load_window_tps="$(json_num '.results[0].derived_metrics.load_window_included_tps' "$out_json")"
  wall_tps="$(json_num '.results[0].derived_metrics.wall_included_tps' "$out_json")"
  phase_overlap_rows="$(json_num '(.results[0].load_window.phase_overlaps // []) | length' "$out_json")"
  load_phase_in_window="$(json_num '[.results[0].load_window.phase_overlaps[]? | select(.name == "run_load_test") | .in_window_seconds] | first // 0' "$out_json")"
  abci_commit_seconds="$(json_num '[.results[0].load_window.storage_signal_summary[]?.abci_commit_seconds] | max // 0' "$out_json")"
  abci_commit_count="$(json_num '[.results[0].load_window.storage_signal_summary[]?.abci_commit_count] | max // 0' "$out_json")"
  write_sync_count="$(json_num '[.results[0].load_window.treedb_stat_deltas[]?.metrics["treedb.public.batch.write_sync.count_total"].delta] | add // 0' "$out_json")"
  checkpoint_count="$(json_num '[.results[0].load_window.treedb_stat_deltas[]?.metrics["treedb.public.checkpoint.count_total"].delta] | add // 0' "$out_json")"

  accepted=false
  if [[ "$reached" == "true" && "$duration_satisfied" == "true" && -z "$result_error" ]]; then
    accepted=true
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$workload" "$scenario" "$mode" "$app_backend" "$node_backend" "$attempt" "$accepted" "$blocks" "$txs" \
    "$seconds" "$successful" "$runtime_tps" "$load_window_tps" "$wall_tps" "$phase_overlap_rows" \
    "$load_phase_in_window" "$abci_commit_seconds" "$abci_commit_count" "$write_sync_count" "$checkpoint_count" "$out_json" >> "$summary"

  [[ "$accepted" == "true" ]]
}

run_until_accepted() {
  local workload="$1"
  local scenario="$2"
  local base_blocks="$3"
  local txs="$4"
  local msg="$5"
  local contained="$6"
  local msgs_per_tx="$7"
  local recipients="$8"
  local max_gas="$9"

  local attempt blocks
  for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
    blocks=$((base_blocks * (2 ** (attempt - 1))))
    if run_one "$workload" "$scenario" "$attempt" "$blocks" "$txs" "$msg" "$contained" "$msgs_per_tx" "$recipients" "$max_gas"; then
      log "accepted workload=$workload scenario=$scenario attempt=$attempt"
      return 0
    fi
    log "load window too short or not reached for workload=$workload scenario=$scenario attempt=$attempt; retrying with larger block count"
  done
  log "failed to produce accepted row for workload=$workload scenario=$scenario after $MAX_ATTEMPTS attempts"
  return 1
}

row_already_accepted() {
  local workload="$1"
  local scenario="$2"
  [[ -f "$summary" ]] || return 1
  awk -F '\t' -v workload="$workload" -v scenario="$scenario" \
    'NR > 1 && $1 == workload && $2 == scenario && $7 == "true" { found = 1 } END { exit found ? 0 : 1 }' "$summary"
}

# workload base_blocks txs msg contained msgs_per_tx recipients max_gas
matrix=(
  "plain-send 400 500 MsgSend MsgSend 1 1 75000000"
  "small-multisend 240 500 MsgMultiSend MsgSend 1 2 100000000"
)

scenarios=(
  "simapp-goleveldb"
  "simapp-treedb"
  "simapp-goleveldb-all"
  "simapp-treedb-all"
)

failures=0
for row in "${matrix[@]}"; do
  read -r workload blocks txs msg contained msgs_per_tx recipients max_gas <<<"$row"
  for scenario in "${scenarios[@]}"; do
    if row_already_accepted "$workload" "$scenario"; then
      log "skipping already accepted workload=$workload scenario=$scenario"
      continue
    fi
    if ! run_until_accepted "$workload" "$scenario" "$blocks" "$txs" "$msg" "$contained" "$msgs_per_tx" "$recipients" "$max_gas"; then
      failures=$((failures + 1))
    fi
  done
done

log "summary: $summary"
if ((failures > 0)); then
  log "$failures matrix rows failed the accepted load-window gate"
  exit 1
fi
