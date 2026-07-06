#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s OUT_ROOT\n' "$0" >&2
  exit 2
fi

OUT_ROOT="$1"
summary="$OUT_ROOT/summary.tsv"
extract="$OUT_ROOT/extract"
profile_tops="$extract/profile_tops"

if [[ ! -f "$summary" ]]; then
  printf 'summary not found: %s\n' "$summary" >&2
  exit 1
fi

mkdir -p "$profile_tops"

fanout() {
  case "$1" in
    plain-send) printf '1' ;;
    small-multisend) printf '2' ;;
    moderate-multisend) printf '10' ;;
    high-fanout-anchor) printf '500' ;;
    *) printf '1' ;;
  esac
}

accepted="$extract/accepted_rows.tsv"
timing="$extract/timing.tsv"
resources="$extract/resources.tsv"
alloc_totals="$extract/alloc_totals.tsv"
pairwise="$extract/pairwise.tsv"

printf 'workload\tbackend\tattempt\tload_window_s\tsuccessful_tx\tload_tps\teffective_ops_s\twall_tps\tjson\n' > "$accepted"
printf 'workload\tbackend\tattempt\tload_window_s\tabci_observed_s\tcommit_s\tfinalize_s\tchecktx_s\tquery_s\tnon_abci_s\tavg_commit_ms\tprocess_cpu_delta_s\tcommit_pct_workload\tmax_speedup_commit_free\n' > "$timing"
printf 'workload\tbackend\tattempt\tvalidator_max_mem_bytes\tvalidator_max_mem\tdocker_block_write_bytes\tdocker_block_read_bytes\tdata_delta_bytes\tapplication_db_delta_bytes\tstate_db_delta_bytes\ttx_index_db_delta_bytes\tprocess_rss_after_bytes\n' > "$resources"
printf 'workload\tbackend\tattempt\talloc_space_total\n' > "$alloc_totals"

tail -n +2 "$summary" | while IFS=$'\t' read -r workload backend attempt accepted_flag load_window_s successful runtime_tps load_tps wall_tps json; do
  [[ "$accepted_flag" == "true" ]] || continue
  [[ -f "$json" ]] || { printf 'missing result json: %s\n' "$json" >&2; exit 1; }

  mult="$(fanout "$workload")"
  effective="$(awk -v tps="$load_tps" -v mult="$mult" 'BEGIN { printf "%.2f", tps * mult }')"
  printf '%s\t%s\t%s\t%s\t%s\t%.2f\t%s\t%.2f\t%s\n' \
    "$workload" "$backend" "$attempt" "$load_window_s" "$successful" "$load_tps" "$effective" "$wall_tps" "$json" >> "$accepted"

  jq -r --arg workload "$workload" --arg backend "$backend" --arg attempt "$attempt" '
    .results[0] as $r
    | ($r.runtime_breakdown[0] // {}) as $rb
    | [
        $workload,
        $backend,
        $attempt,
        ($rb.workload_runtime_seconds // $r.load_window.seconds // 0),
        ($rb.abci_observed_seconds // 0),
        ($rb.abci_commit_seconds // 0),
        ($rb.abci_finalize_block_seconds // 0),
        ($rb.abci_check_tx_seconds // 0),
        ($rb.abci_query_seconds // 0),
        ($rb.non_abci_workload_seconds // 0),
        (($rb.abci_commit_seconds // 0) / (($r.storage_signal_summary[0].abci_commit_count // 0) as $c | if $c == 0 then 1 else $c end) * 1000),
        ($r.storage_signal_summary[0].process_cpu_seconds_delta // 0),
        ($rb.commit_pct_of_workload // 0),
        ($rb.max_runtime_speedup_if_commit_free // 0)
      ]
    | @tsv
  ' "$json" >> "$timing"

  jq -r --arg workload "$workload" --arg backend "$backend" --arg attempt "$attempt" '
    def bytes_at($items; $path):
      (($items // [])[] | select(.path == $path) | .bytes) // 0;
    .results[0] as $r
    | (($r.resource_summary // [])[] | select(.name | test("validator-0$")) | .) as $rs
    | [
        $workload,
        $backend,
        $attempt,
        ($rs.max_mem_usage_bytes // 0),
        ($rs.max_mem_usage // ""),
        ($rs.max_block_write_bytes // 0),
        ($rs.max_block_read_bytes // 0),
        (bytes_at($r.data_sizes_after; "/simd/data") - bytes_at($r.data_sizes_before; "/simd/data")),
        (bytes_at($r.data_sizes_after; "/simd/data/application.db") - bytes_at($r.data_sizes_before; "/simd/data/application.db")),
        (bytes_at($r.data_sizes_after; "/simd/data/state.db") - bytes_at($r.data_sizes_before; "/simd/data/state.db")),
        (bytes_at($r.data_sizes_after; "/simd/data/tx_index.db") - bytes_at($r.data_sizes_before; "/simd/data/tx_index.db")),
        ($r.metrics_after[0].metrics.process_resident_memory_bytes // 0)
      ]
    | @tsv
  ' "$json" >> "$resources"

  heap="$(find "$(dirname "$json")/pprof" -maxdepth 1 -type f -name '*-heap.pprof' | head -1 || true)"
  cpu="$(find "$(dirname "$json")/pprof" -maxdepth 1 -type f -name '*-cpu.pprof' | head -1 || true)"
  if [[ -n "$heap" ]]; then
    heap_top="$profile_tops/${workload}-${backend}-a${attempt}-heap-alloc-top.txt"
    go tool pprof -top -unit=GB -sample_index=alloc_space "$heap" > "$heap_top"
    total="$(awk '/^Showing nodes accounting/ { sub(/^.* of /, ""); print; exit }' "$heap_top")"
    printf '%s\t%s\t%s\t%s\n' "$workload" "$backend" "$attempt" "$total" >> "$alloc_totals"
  fi
  if [[ -n "$cpu" ]]; then
    go tool pprof -top "$cpu" > "$profile_tops/${workload}-${backend}-a${attempt}-cpu-top.txt"
  fi
done

awk -F '\t' '
  NR == 1 { next }
  {
    workload=$1
    backend=$2
    load[workload,backend]=$6
    effective[workload,backend]=$7
    wall[workload,backend]=$8
    order[++n]=workload
  }
  END {
    print "workload\tgoleveldb_load_tps\ttreedb_load_tps\ttreedb_load_ratio\tgoleveldb_effective_ops_s\ttreedb_effective_ops_s\ttreedb_effective_ratio\tgoleveldb_wall_tps\ttreedb_wall_tps\ttreedb_wall_ratio"
    for (i = 1; i <= n; i++) {
      workload = order[i]
      if (seen[workload]++) {
        continue
      }
      if (!((workload,"goleveldb") in load) || !((workload,"treedb") in load)) {
        continue
      }
      gl = load[workload,"goleveldb"]
      tr = load[workload,"treedb"]
      ge = effective[workload,"goleveldb"]
      te = effective[workload,"treedb"]
      gw = wall[workload,"goleveldb"]
      tw = wall[workload,"treedb"]
      printf "%s\t%.2f\t%.2f\t%.2fx\t%.2f\t%.2f\t%.2fx\t%.2f\t%.2f\t%.2fx\n", workload, gl, tr, tr/gl, ge, te, te/ge, gw, tw, tw/gw
    }
  }
' "$accepted" > "$pairwise"

printf 'wrote extracts under %s\n' "$extract"
