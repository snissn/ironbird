#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

export OUT_ROOT="$TMP_ROOT"
export M4_MATRIX_LIBRARY_ONLY=true
source "$ROOT/scripts/ironbird_durable_m4_matrix.sh"

mkdir -p "$OUT_ROOT/pairs/pair-1/baseline-treedb"
result="$OUT_ROOT/pairs/pair-1/baseline-treedb/result.json"
jq -n '{
  results: [{
    load_window: {seconds: 301, included_transactions: 223875, successful_transactions: 223875},
    derived_metrics: {load_window_included_tps: 743.77},
    wall_seconds: 400
  }]
}' > "$result"

append_summary 1 baseline-treedb 1 "$result"

summary="$OUT_ROOT/pairs/summary.tsv"
[[ "$(wc -l < "$summary")" -eq 2 ]]
awk -F '\t' '
  NR == 2 {
    if ($1 != "1" || $2 != "baseline-treedb" || $3 != "1" || $4 != "true" || $8 != "743.77") {
      exit 1
    }
    found = 1
  }
  END { exit found ? 0 : 1 }
' "$summary"
summary_has_row 1 baseline-treedb
