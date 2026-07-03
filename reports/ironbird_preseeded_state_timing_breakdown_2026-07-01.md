# Ironbird Preseeded-State Timing Breakdown

Date: 2026-07-01

This reruns the preseeded simapp LevelDB/TreeDB sweep with the local runner's
new phase timeline and runtime breakdown instrumentation.

All runs used:

- Docker image: `ironbird-report:snissn-sdk-28e5525f-cosmosdb-06039c0-gomap-v0.6.1`
- validators: `1`
- nodes: `0`
- active wallets: `5000`
- workload: `3` blocks x `50` tx/block
- message: `MsgArr`
- contained message: `MsgMultiSend`
- contained messages per tx: `20`
- recipients per contained message: `25`
- max gas: `300000000`
- intended txs: `150`
- effective operations: `150 * 20 * 25 = 75000`

Validation:

```sh
GOWORK=off go test ./cmd/local-report-runner
```

Result: pass.

## Artifacts

- `reports/artifacts/preseed-v061-breakdown/simapp-goleveldb-preseed100000-msgarr-multisend-3x50x20x25-gas300m-breakdown.json`
- `reports/artifacts/preseed-v061-breakdown/simapp-treedb-preseed100000-msgarr-multisend-3x50x20x25-gas300m-breakdown.json`
- `reports/artifacts/preseed-v061-breakdown/simapp-goleveldb-preseed250000-msgarr-multisend-3x50x20x25-gas300m-breakdown.json`
- `reports/artifacts/preseed-v061-breakdown/simapp-treedb-preseed250000-msgarr-multisend-3x50x20x25-gas300m-breakdown.json`

## Runtime Summary

The Catalyst top-level result still reports failed transactions because of the
known decode issue, but `raw_tx_summary` found all `150/150` transactions and
all succeeded in every run. Runtime TPS and effective ops/s below use the raw
audit-backed derived metrics.

| Preseed | Backend | Genesis accounts | Wall s | Launch s | Load phase s | Catalyst runtime s | Post-launch non-workload s | Runtime TPS | Runtime effective ops/s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100k | goleveldb | 105000 | 228.589 | 161.943 | 61.317 | 44.812 | 21.834 | 3.347 | 1673.66 |
| 100k | treedb | 105000 | 230.671 | 164.057 | 61.252 | 43.060 | 23.554 | 3.483 | 1741.74 |
| 250k | goleveldb | 255000 | 444.208 | 377.543 | 61.288 | 43.756 | 22.909 | 3.428 | 1714.06 |
| 250k | treedb | 255000 | 428.527 | 361.959 | 61.289 | 43.456 | 23.112 | 3.452 | 1725.90 |

## ABCI Runtime Breakdown

| Preseed | Backend | ABCI observed s | Commit s | Finalize s | CheckTx s | Proposal s | Query s | Non-ABCI workload s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100k | goleveldb | 5.829 | 2.509 | 2.924 | 0.246 | 0.073 | 0.077 | 38.983 |
| 100k | treedb | 6.710 | 3.448 | 2.881 | 0.218 | 0.054 | 0.109 | 36.350 |
| 250k | goleveldb | 13.648 | 6.724 | 6.540 | 0.204 | 0.057 | 0.123 | 30.107 |
| 250k | treedb | 15.436 | 8.737 | 6.327 | 0.243 | 0.054 | 0.075 | 28.019 |

| Preseed | Backend | Commit % runtime | Commit+finalize % runtime | Observed ABCI % runtime | Non-ABCI % runtime | Max speedup if commit free | Max speedup if commit half |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 100k | goleveldb | 5.60% | 12.12% | 13.01% | 86.99% | 1.059x | 1.029x |
| 100k | treedb | 8.01% | 14.70% | 15.58% | 84.42% | 1.087x | 1.042x |
| 250k | goleveldb | 15.37% | 30.31% | 31.19% | 68.81% | 1.182x | 1.083x |
| 250k | treedb | 20.11% | 34.67% | 35.52% | 64.48% | 1.252x | 1.112x |

## Storage And Resource Summary

| Preseed | Backend | App DB delta MB | Data dir delta MB | Process CPU s | Validator max mem GiB | Validator max block write MB |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 100k | goleveldb | 60.833 | 81.367 | 29.59 | 2.047 | 305 |
| 100k | treedb | 55.006 | 75.535 | 30.33 | 2.596 | 327 |
| 250k | goleveldb | 144.153 | 164.663 | 48.12 | 3.823 | 717 |
| 250k | treedb | 131.900 | 152.401 | 52.88 | 4.948 | 391 |

## Commit Counters

| Preseed | Backend | Commit count | Avg commit ms | Finalize count | Avg finalize ms |
| --- | --- | ---: | ---: | ---: | ---: |
| 100k | goleveldb | 30 | 83.65 | 30 | 97.46 |
| 100k | treedb | 29 | 118.89 | 29 | 99.36 |
| 250k | goleveldb | 26 | 258.63 | 26 | 251.52 |
| 250k | treedb | 25 | 349.49 | 25 | 253.06 |

## Interpretation

The more detailed timing confirms that increasing preseeded state makes the run
more storage-adjacent, but it still does not make this Ironbird lane strongly
TreeDB-favorable.

At 250k preseeded accounts, commit+finalize is now a meaningful fraction of the
measured Catalyst runtime: `30.31%` for LevelDB and `34.67%` for TreeDB. That is
high enough to justify caring about commit path instrumentation, but not high
enough to explain a large end-to-end lift by itself. Even making commit free
would cap the TreeDB 250k runtime speedup at about `1.25x`.

The A/B result is still small:

- 100k: TreeDB `1741.74` runtime effective ops/s vs LevelDB `1673.66`, about `+4.1%`.
- 250k: TreeDB `1725.90` runtime effective ops/s vs LevelDB `1714.06`, about `+0.7%`.

TreeDB still stores less data after the workload:

- 100k: TreeDB app DB delta `55.0 MB` vs LevelDB `60.8 MB`.
- 250k: TreeDB app DB delta `131.9 MB` vs LevelDB `144.2 MB`.

TreeDB also wrote much less validator block IO at 250k: `391 MB` vs LevelDB
`717 MB`. That is a real resource signal, but it did not convert into a large
runtime throughput win in this harness.

The main bottleneck shape remains:

- Large synthetic genesis setup dominates wall time and is not useful as DB
  throughput evidence.
- During the measured Catalyst runtime, non-ABCI work is still `64-69%` of the
  250k runs.
- Commit+finalize is visible and worth tracking, but this workload still has too
  much non-storage work to demonstrate the production-sync-sized TreeDB lift.

## Instrumentation Notes

The new artifacts include:

- `phase_timeline`: runner wall-clock phases.
- `runtime_breakdown`: explicit workload, ABCI, non-ABCI, and Amdahl categories.
- expanded `storage_signal_summary`: ABCI method timings and counts.
- `module_timings`: SDK begin/end blocker module metrics.

Use `runtime_breakdown` and CometBFT ABCI method timings as the authoritative
coarse timing categories. The SDK `module_timings` are captured for diagnosis,
but some exported module sums exceed the observed ABCI wall-time envelope, so
they should be treated as raw/directional until their units and aggregation
semantics are normalized.
