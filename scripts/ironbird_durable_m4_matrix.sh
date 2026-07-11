#!/usr/bin/env bash
set -euo pipefail

if (($# != 0)); then
  printf 'usage: %s (configure with environment variables; no positional arguments)\n' "$0" >&2
  exit 2
fi

RUNNER="${RUNNER:-/mnt/fast4tb/bin/ironbird-local-report-runner-3658-1bc048e}"
OUT_ROOT="${OUT_ROOT:-/mnt/fast4tb/ironbird-durable-m4-validation-20260710}"
START_PAIR="${START_PAIR:-1}"
END_PAIR="${END_PAIR:-3}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-3}"
TMPDIR="${TMPDIR:-/mnt/fast4tb/tmp}"

BASELINE_VERSION="v0.6.2-0.20260709230517-9cd9c6874860"
BASELINE_REF="9cd9c6874860d2988002701bef042e50ba142cd0"
CANDIDATE_VERSION="v0.6.2-0.20260711063646-09a626cd8f10"
CANDIDATE_REF="09a626cd8f10fa161ef7f259d43b6567ea3e8abb"
BASELINE_IMAGE="ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-9cd9c68-comet-87379c9"
CANDIDATE_IMAGE="ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-09a626c-comet-87379c9"
REQUIRED_TXS=223875

mkdir -p "$OUT_ROOT/pairs" "$TMPDIR"

log() {
  printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

assert_preconditions() {
  [[ -x "$RUNNER" ]] || {
    log "runner is not executable: $RUNNER"
    return 1
  }
  docker image inspect "$BASELINE_IMAGE" >/dev/null
  docker image inspect "$CANDIDATE_IMAGE" >/dev/null

  local occupants
  occupants="$(docker ps --format '{{.Names}} {{.Image}}' | awk '$2 ~ /(simapp-v53|ironbird-report)/ {print}')"
  if [[ -n "$occupants" ]]; then
    log "benchmark-related containers already occupy the host; refusing to start a canonical row"
    return 1
  fi
}

pin_for_label() {
  case "$1" in
    baseline-treedb)
      printf '%s\t%s\t%s\t%s\n' treedb "$BASELINE_VERSION" "$BASELINE_REF" "$BASELINE_IMAGE"
      ;;
    candidate-treedb)
      printf '%s\t%s\t%s\t%s\n' treedb "$CANDIDATE_VERSION" "$CANDIDATE_REF" "$CANDIDATE_IMAGE"
      ;;
    goleveldb)
      printf '%s\t%s\t%s\t%s\n' goleveldb "$CANDIDATE_VERSION" "$CANDIDATE_REF" "$CANDIDATE_IMAGE"
      ;;
    *)
      log "unknown row label: $1"
      return 1
      ;;
  esac
}

order_for_pair() {
  case "$1" in
    1) printf '%s\n' 'baseline-treedb candidate-treedb goleveldb' ;;
    2) printf '%s\n' 'candidate-treedb goleveldb baseline-treedb' ;;
    3) printf '%s\n' 'goleveldb baseline-treedb candidate-treedb' ;;
    4) printf '%s\n' 'candidate-treedb baseline-treedb goleveldb' ;;
    5) printf '%s\n' 'goleveldb candidate-treedb baseline-treedb' ;;
    *)
      log "pair must be in [1,5]: $1"
      return 1
      ;;
  esac
}

accepted_result() {
  local json="$1"
  local backend="$2"
  local version="$3"
  local ref="$4"
  local image="$5"

  jq -e \
    --arg backend "$backend" \
    --arg version "$version" \
    --arg ref "$ref" \
    --arg image "$image" \
    --argjson required "$REQUIRED_TXS" '
      (.results | length) == 1 and
      ((.results[0].error // "") == "") and
      .results[0].load_window.reached == true and
      .results[0].load_window.duration_satisfied == true and
      .results[0].load_window.included_transactions >= $required and
      .results[0].load_window.successful_transactions == .results[0].load_window.included_transactions and
      .results[0].backend_verification.valid == true and
      .results[0].backend_verification.observed_app_db_backend == $backend and
      .results[0].backend_verification.observed_node_db_backend == $backend and
      .results[0].backend_verification.observed_tx_indexer == "kv" and
      .results[0].scenario.image_tag == $image and
      any(.results[0].scenario.dependency_pins[];
        .module == "github.com/snissn/gomap" and .version == $version and .ref == $ref)
    ' "$json" >/dev/null
}

append_summary() {
  local pair="$1"
  local label="$2"
  local attempt="$3"
  local json="$4"
  local summary="$OUT_ROOT/pairs/summary.tsv"

  if [[ ! -f "$summary" ]]; then
    printf 'pair\tlabel\tattempt\taccepted\tseconds\tincluded\tsuccessful\ttps\twall_seconds\tjson\n' > "$summary"
  fi
  jq -r \
    --arg pair "$pair" \
    --arg label "$label" \
    --arg attempt "$attempt" \
    --arg json "$json" '
      [$pair, $label, $attempt, "true",
       (.results[0].load_window.seconds | tostring),
       (.results[0].load_window.included_transactions | tostring),
       (.results[0].load_window.successful_transactions | tostring),
       (.results[0].derived_metrics.load_window_included_tps | tostring),
       (.results[0].wall_seconds | tostring), $json] | @tsv
    ' "$json" >> "$summary"
}

run_row() {
  local pair="$1"
  local label="$2"
  local backend version ref image
  IFS=$'\t' read -r backend version ref image < <(pin_for_label "$label")

  local row_dir="$OUT_ROOT/pairs/pair-$pair/$label"
  if [[ -f "$row_dir/result.json" ]] && accepted_result "$row_dir/result.json" "$backend" "$version" "$ref" "$image"; then
    log "already accepted pair=$pair row=$label"
    return 0
  fi

  local attempt
  for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
    assert_preconditions
    if [[ -e "$row_dir" ]]; then
      mv "$row_dir" "${row_dir}-superseded-$(date -u +%Y%m%dT%H%M%SZ)"
    fi
    mkdir -p "$row_dir"
    log "starting pair=$pair row=$label attempt=$attempt"

    set +e
    TMPDIR="$TMPDIR" "$RUNNER" \
      -scenario "simapp-${backend}-all" \
      -skip-build \
      -simapp-gomap-version "$version" \
      -simapp-gomap-ref "$ref" \
      -validators 1 -nodes 0 -wallets 5000 \
      -preseed-profile accounts -preseed-accounts 100000 \
      -cosmos-txs 500 -cosmos-blocks 450 \
      -tx-indexer kv \
      -load-window-min-duration 5m \
      -load-window-target-fraction 0.995 \
      -load-window-drain-timeout 5m \
      -stop-catalyst-after-load-window \
      -app-debug-vars \
      -raw-tx-audit=false \
      -out "$row_dir/result.json" \
      >"$row_dir/runner.log" 2>&1
    local status=$?
    set -e

    if [[ $status -eq 0 ]] && [[ -f "$row_dir/result.json" ]] && \
        accepted_result "$row_dir/result.json" "$backend" "$version" "$ref" "$image"; then
      append_summary "$pair" "$label" "$attempt" "$row_dir/result.json"
      log "accepted pair=$pair row=$label attempt=$attempt"
      return 0
    fi

    local rejected="$OUT_ROOT/pairs/pair-$pair/${label}-rejected-attempt-$attempt"
    if [[ -e "$rejected" ]]; then
      rejected="${rejected}-$(date -u +%Y%m%dT%H%M%SZ)"
    fi
    mv "$row_dir" "$rejected"
    log "rejected pair=$pair row=$label attempt=$attempt status=$status artifact=$rejected"
  done

  log "failed pair=$pair row=$label after $MAX_ATTEMPTS attempts"
  return 1
}

if ((START_PAIR < 1 || END_PAIR > 5 || START_PAIR > END_PAIR)); then
  log "invalid pair range: $START_PAIR..$END_PAIR"
  exit 2
fi

for ((pair = START_PAIR; pair <= END_PAIR; pair++)); do
  order="$(order_for_pair "$pair")"
  log "pair=$pair order=$order"
  for label in $order; do
    run_row "$pair" "$label"
  done
done

log "completed canonical pair range $START_PAIR..$END_PAIR"
