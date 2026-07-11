#!/usr/bin/env python3
"""Summarize the accepted Ironbird durable-write M4 pair matrix."""

from __future__ import annotations

import argparse
import itertools
import json
import math
import statistics
from pathlib import Path
from typing import Any, Iterable


STAGES = (
    "consensus commit",
    "commit blockstore",
    "state app commit",
    "state save",
    "state save tx info",
    "tx index block total",
)
PRIMARY_STAGES = (
    "consensus commit",
    "commit blockstore",
    "state app commit",
    "state save",
    "tx index block total",
)
STORES = ("application.db", "blockstore.db", "state.db", "tx_index.db")


def metric_delta(metrics: dict[str, Any], name: str) -> float:
    value = metrics.get(name, {})
    if isinstance(value, dict):
        value = value.get("delta", 0)
    return float(value or 0)


def metric_value(metrics: dict[str, Any], name: str, field: str) -> float:
    value = metrics.get(name, {})
    if isinstance(value, dict):
        value = value.get(field, 0)
    return float(value or 0)


def quantile(values: list[float], fraction: float) -> float:
    ordered = sorted(values)
    if not ordered:
        return 0.0
    position = (len(ordered) - 1) * fraction
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return ordered[lower]
    return ordered[lower] + (ordered[upper] - ordered[lower]) * (position - lower)


def describe(values: Iterable[float]) -> dict[str, float | int]:
    samples = list(values)
    mean = statistics.fmean(samples)
    stdev = statistics.stdev(samples) if len(samples) > 1 else 0.0
    return {
        "n": len(samples),
        "mean": mean,
        "median": statistics.median(samples),
        "min": min(samples),
        "max": max(samples),
        "stdev": stdev,
        "rsd_pct": (stdev / mean * 100.0) if mean else 0.0,
    }


def paired_effect(baseline: list[float], candidate: list[float]) -> dict[str, Any]:
    ratios = [after / before for before, after in zip(baseline, candidate, strict=True)]
    changes = [(ratio - 1.0) * 100.0 for ratio in ratios]
    geomean = math.exp(statistics.fmean(math.log(ratio) for ratio in ratios))
    bootstrap = []
    for indexes in itertools.product(range(len(ratios)), repeat=len(ratios)):
        bootstrap.append(
            (math.exp(statistics.fmean(math.log(ratios[index]) for index in indexes)) - 1.0)
            * 100.0
        )
    return {
        "per_pair_pct": changes,
        "positive_pairs": sum(change > 0 for change in changes),
        "median_pct": statistics.median(changes),
        "geomean_pct": (geomean - 1.0) * 100.0,
        "mean_pct": statistics.fmean(changes),
        "min_pct": min(changes),
        "max_pct": max(changes),
        "geomean_bootstrap_95_pct": [
            quantile(bootstrap, 0.025),
            quantile(bootstrap, 0.975),
        ],
    }


def validator_resource(result: dict[str, Any]) -> dict[str, Any]:
    rows = [row for row in result.get("resource_summary", []) if "validator" in row.get("name", "")]
    return rows[0] if rows else {}


def read_row(path: Path, pair: int, label: str) -> dict[str, Any]:
    document = json.loads(path.read_text())
    result = document["results"][0]
    window = result["load_window"]
    signal = window["storage_signal_summary"][0]
    stages = {
        row["name"]: float(row["avg_seconds"]) * 1000.0
        for row in signal["exact_commit_stage_timings"]
    }
    consensus_count = next(
        row["count"] for row in signal["exact_commit_stage_timings"] if row["name"] == "consensus commit"
    )

    app_metrics = window["metric_deltas"][0]["metrics"]
    resources = validator_resource(result)
    stores: dict[str, Any] = {}
    for delta_row in window.get("treedb_stat_deltas", []):
        store = delta_row.get("store")
        if store not in STORES:
            continue
        metrics = delta_row.get("metrics", {})
        write_sync_calls = metric_delta(metrics, "treedb.public.batch.write_sync.calls_total")
        write_sync_ns = metric_delta(metrics, "treedb.public.batch.write_sync.ns_total")
        wait_reasons = {}
        for reason in ("frontier_cutover", "checkpoint_drain", "maintenance"):
            prefix = f"treedb.cache.write.wait.{reason}"
            count = metric_delta(metrics, f"{prefix}.count_total")
            wait_reasons[reason] = {
                "count": count,
                "total_seconds": metric_delta(metrics, f"{prefix}.ns_total") / 1e9,
                # A monotonic max cannot be differenced into an exact window max.
                # When the window adds samples, the after value is a conservative
                # upper bound; with no added sample the accepted-window max is zero.
                "max_upper_bound_ms": (
                    metric_value(metrics, f"{prefix}.ns_max", "after") / 1e6 if count else 0.0
                ),
                "p50_upper_bound_ms": (
                    metric_value(metrics, f"{prefix}.p50_upper_ns", "after") / 1e6 if count else 0.0
                ),
                "p95_upper_bound_ms": (
                    metric_value(metrics, f"{prefix}.p95_upper_ns", "after") / 1e6 if count else 0.0
                ),
                "p99_upper_bound_ms": (
                    metric_value(metrics, f"{prefix}.p99_upper_ns", "after") / 1e6 if count else 0.0
                ),
            }
        stores[store] = {
            "write_sync_calls": write_sync_calls,
            "write_sync_total_seconds": write_sync_ns / 1e9,
            "write_sync_avg_ms": (write_sync_ns / write_sync_calls / 1e6) if write_sync_calls else 0.0,
            "checkpoint_wait_calls": metric_delta(
                metrics, "treedb.cache.write.wait_for_checkpoint.count_total"
            ),
            "checkpoint_wait_seconds": metric_delta(
                metrics, "treedb.cache.write.wait_for_checkpoint.ns_total"
            )
            / 1e9,
            "checkpoint_wait_max_upper_bound_ms": (
                metric_value(metrics, "treedb.cache.write.wait_for_checkpoint.ns_max", "after")
                / 1e6
                if metric_delta(metrics, "treedb.cache.write.wait_for_checkpoint.count_total")
                else 0.0
            ),
            "checkpoint_runs": metric_delta(metrics, "treedb.cache.checkpoint.runs"),
            "checkpoint_total_seconds": metric_delta(
                metrics, "treedb.cache.checkpoint.total_ms"
            )
            / 1000.0,
            "auto_checkpoint_count": metric_delta(metrics, "treedb.cache.auto_checkpoint.count"),
            "command_wal_append_calls": metric_delta(
                metrics, "treedb.command_wal.append.count_total"
            ),
            "command_wal_append_seconds": metric_delta(
                metrics, "treedb.command_wal.append.ns_total"
            )
            / 1e9,
            "command_wal_flush_calls": metric_delta(
                metrics, "treedb.command_wal.flush.count_total"
            ),
            "command_wal_flush_seconds": metric_delta(
                metrics, "treedb.command_wal.flush.ns_total"
            )
            / 1e9,
            "command_wal_sync_calls": metric_delta(
                metrics, "treedb.command_wal.sync.count_total"
            ),
            "command_wal_sync_seconds": metric_delta(
                metrics, "treedb.command_wal.sync.ns_total"
            )
            / 1e9,
            "post_frontier_admissions": metric_delta(
                metrics, "treedb.cache.write.post_frontier_admission.count_total"
            ),
            "write_wait_reasons": wait_reasons,
        }

    data_dir = next(
        (row["bytes"] for row in result.get("data_sizes_after", []) if row.get("path") == "/simd/data"),
        0,
    )
    return {
        "pair": pair,
        "label": label,
        "path": str(path),
        "load_window_seconds": float(window["seconds"]),
        "included": int(window["included_transactions"]),
        "successful": int(window["successful_transactions"]),
        "tps": float(result["derived_metrics"]["load_window_included_tps"]),
        "blocks": int(consensus_count),
        "transactions_per_block": float(signal["consensus_total_txs_delta"]) / consensus_count,
        "cadence_ms_per_block": float(window["seconds"]) / consensus_count * 1000.0,
        "stages_ms_per_block": {name: stages[name] for name in STAGES},
        "unexplained_after_consensus_ms_per_block":
            float(window["seconds"]) / consensus_count * 1000.0 - stages["consensus commit"],
        "cpu_seconds": metric_delta(app_metrics, "process_cpu_seconds_total"),
        "cpu_core_equivalents": metric_delta(app_metrics, "process_cpu_seconds_total")
            / float(window["seconds"]),
        "allocation_bytes": metric_delta(app_metrics, "go_memstats_alloc_bytes_total"),
        "mallocs": metric_delta(app_metrics, "go_memstats_mallocs_total"),
        "gc_runs": metric_delta(app_metrics, "runtime_total_gc_runs"),
        "gc_pause_ms": metric_delta(app_metrics, "runtime_total_gc_pause_ns") / 1e6,
        "resident_bytes_after": float(
            window["metrics_after"][0]["metrics"].get("process_resident_memory_bytes", 0)
        ),
        "max_container_memory_bytes": float(resources.get("max_mem_usage_bytes", 0)),
        "max_block_read_bytes": float(resources.get("max_block_read_bytes", 0)),
        "max_block_write_bytes": float(resources.get("max_block_write_bytes", 0)),
        "data_dir_bytes_after": float(data_dir),
        "data_dir_bytes_delta": float(signal.get("data_dir_bytes_delta", 0)),
        "stores": stores,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("--pairs", type=int, default=3)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    rows = []
    by_pair: dict[int, dict[str, dict[str, Any]]] = {}
    for pair in range(1, args.pairs + 1):
        by_pair[pair] = {}
        for label in ("baseline-treedb", "candidate-treedb", "goleveldb"):
            path = args.root / "pairs" / f"pair-{pair}" / label / "result.json"
            row = read_row(path, pair, label)
            rows.append(row)
            by_pair[pair][label] = row

    labels: dict[str, Any] = {}
    for label in ("baseline-treedb", "candidate-treedb", "goleveldb"):
        selected = [row for row in rows if row["label"] == label]
        labels[label] = {
            "tps": describe(row["tps"] for row in selected),
            "transactions_per_block": describe(row["transactions_per_block"] for row in selected),
            "cadence_ms_per_block": describe(row["cadence_ms_per_block"] for row in selected),
            "stages_ms_per_block": {
                stage: describe(row["stages_ms_per_block"][stage] for row in selected)
                for stage in STAGES
            },
            "cpu_core_equivalents": describe(row["cpu_core_equivalents"] for row in selected),
            "allocation_bytes": describe(row["allocation_bytes"] for row in selected),
            "gc_runs": describe(row["gc_runs"] for row in selected),
            "gc_pause_ms": describe(row["gc_pause_ms"] for row in selected),
            "max_container_memory_bytes": describe(
                row["max_container_memory_bytes"] for row in selected
            ),
            "data_dir_bytes_after": describe(row["data_dir_bytes_after"] for row in selected),
            "unexplained_after_consensus_ms_per_block": describe(
                row["unexplained_after_consensus_ms_per_block"] for row in selected
            ),
        }

    baseline = [by_pair[pair]["baseline-treedb"] for pair in by_pair]
    candidate = [by_pair[pair]["candidate-treedb"] for pair in by_pair]
    paired: dict[str, Any] = {
        "tps": paired_effect(
            [row["tps"] for row in baseline], [row["tps"] for row in candidate]
        ),
        "transactions_per_block": paired_effect(
            [row["transactions_per_block"] for row in baseline],
            [row["transactions_per_block"] for row in candidate],
        ),
        "cadence_ms_per_block": paired_effect(
            [row["cadence_ms_per_block"] for row in baseline],
            [row["cadence_ms_per_block"] for row in candidate],
        ),
        "stages_ms_per_block": {
            stage: paired_effect(
                [row["stages_ms_per_block"][stage] for row in baseline],
                [row["stages_ms_per_block"][stage] for row in candidate],
            )
            for stage in STAGES
        },
    }

    stage_gaps = []
    for pair, pair_rows in by_pair.items():
        tree_stages = pair_rows["candidate-treedb"]["stages_ms_per_block"]
        level_stages = pair_rows["goleveldb"]["stages_ms_per_block"]
        gross = sum(
            tree_stages[name] - level_stages[name]
            for name in ("commit blockstore", "state app commit", "state save")
        )
        save_tx_info = tree_stages["state save tx info"] - level_stages["state save tx info"]
        stage_gaps.append(
            {
                "pair": pair,
                "gross_three_stage_ms_per_block": gross,
                "save_tx_info_offset_ms_per_block": save_tx_info,
                "net_four_stage_ms_per_block": gross + save_tx_info,
            }
        )

    candidate_primary_rsd = {
        "tps": labels["candidate-treedb"]["tps"]["rsd_pct"],
        **{
            stage: labels["candidate-treedb"]["stages_ms_per_block"][stage]["rsd_pct"]
            for stage in PRIMARY_STAGES
        },
    }
    candidate_state_write_sync = [
        row["stores"]["state.db"]["write_sync_avg_ms"] for row in candidate
    ]
    candidate_tx_checkpoint_wait = [
        sum(
            row["stores"]["tx_index.db"]["write_wait_reasons"][reason]["total_seconds"]
            for reason in ("frontier_cutover", "checkpoint_drain")
        )
        for row in candidate
    ]
    candidate_tx_checkpoint_wait_max = [
        max(
            row["stores"]["tx_index.db"]["write_wait_reasons"][reason]["max_upper_bound_ms"]
            for reason in ("frontier_cutover", "checkpoint_drain")
        )
        for row in candidate
    ]
    candidate_app_write_sync = [
        row["stores"]["application.db"]["write_sync_avg_ms"] for row in candidate
    ]
    candidate_async_tx_index = [
        row["stages_ms_per_block"]["tx index block total"] for row in candidate
    ]
    net_stage_gaps = [row["net_four_stage_ms_per_block"] for row in stage_gaps]
    throughput = paired["tps"]
    gates = {
        "throughput_pass": throughput["median_pct"] >= 2.0
        and throughput["geomean_pct"] >= 2.0
        and throughput["positive_pairs"] > args.pairs / 2,
        "state_write_sync_pass": statistics.median(candidate_state_write_sync) <= 16.0,
        "tx_index_checkpoint_wait_pass": statistics.median(candidate_tx_checkpoint_wait) <= 3.0,
        "tx_index_checkpoint_wait_max_pass": max(candidate_tx_checkpoint_wait_max) <= 250.0,
        "application_write_sync_pass": statistics.median(candidate_app_write_sync) <= 17.0,
        "async_tx_index_pass": statistics.median(candidate_async_tx_index) <= 42.0,
        "synchronous_stage_gap_pass": statistics.median(net_stage_gaps) <= 10.0,
    }
    gates["all_numeric_pass"] = all(gates.values())

    output = {
        "pairs": args.pairs,
        "rows": rows,
        "labels": labels,
        "paired_baseline_to_candidate": paired,
        "candidate_vs_leveldb_synchronous_stage_gaps": stage_gaps,
        "candidate_state_write_sync_ms": candidate_state_write_sync,
        "candidate_application_write_sync_ms": candidate_app_write_sync,
        "candidate_tx_index_checkpoint_wait_seconds": candidate_tx_checkpoint_wait,
        "candidate_tx_index_checkpoint_wait_max_upper_bound_ms":
            candidate_tx_checkpoint_wait_max,
        "candidate_async_tx_index_ms_per_block": candidate_async_tx_index,
        "candidate_primary_rsd_pct": candidate_primary_rsd,
        "extend_to_five": max(candidate_primary_rsd.values()) > 3.0
        or (
            throughput["geomean_bootstrap_95_pct"][0] <= 0
            <= throughput["geomean_bootstrap_95_pct"][1]
        ),
        "gates": gates,
    }
    rendered = json.dumps(output, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(rendered)
    print(
        json.dumps(
            {
                "pairs": args.pairs,
                "throughput": throughput,
                "candidate_primary_rsd_pct": candidate_primary_rsd,
                "extend_to_five": output["extend_to_five"],
                "gates": gates,
            },
            indent=2,
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    main()
