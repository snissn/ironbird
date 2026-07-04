# Ironbird Normal-Workload TreeDB vs goleveldb Sweep

Date: 2026-07-04 UTC

## Executive Summary

This sprint ran a full-stack Ironbird simapp matrix comparing goleveldb versus
TreeDB with both the Cosmos app DB and CometBFT node DBs set to the same backend.
Every accepted row met the sustained load-window gate of at least 5 minutes, and
each row verified the generated app and node DB backend config before reporting
results.

The result is workload-shaped rather than a blanket win:

- Plain `MsgSend`: goleveldb was slightly ahead on load-window TPS, 531.58 vs
  512.32.
- Small `MsgMultiSend` with 2 recipients: near tie, goleveldb 532.24 vs TreeDB
  519.00 load-window TPS.
- Moderate `MsgMultiSend` with 10 recipients: TreeDB pulled ahead, 465.63 vs
  377.82 load-window TPS, a 1.23x win.
- High fan-out anchor, 20 `MsgMultiSend` messages with 25 recipients each:
  TreeDB was much faster, 23.38 vs 9.90 load-window TPS, a 2.36x win.

The tradeoff is resource footprint. TreeDB generally used more memory and had a
larger final data directory on the normal workloads. In the high fan-out anchor,
TreeDB used fewer Docker block writes and had a smaller final `/simd/data`
delta, while still using more memory.

The primary throughput number in this report is **load-window TPS**, not Catalyst
runtime TPS. High fan-out goleveldb finished sending before the chain finished
including enough app-metric txs, so Catalyst runtime TPS overstated chain
throughput for that row. Load-window TPS is the app/CometBFT metric over the
accepted sustained window.

## Scope

This is a fork-local report for `snissn/ironbird`. It does not open or claim
readiness for public upstream Celestia, Cosmos SDK, CometBFT, cosmos-db, or
Ironbird PRs.

This is still a synthetic simapp benchmark. It is not a Celestia production
replay and should not be described as equivalent to Celestia production sync.

## Artifact And Run Context

Artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-stopwindow-20260704T011644Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-stopwindow-20260704T011644Z/summary.tsv
```

Profile summaries:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-stopwindow-20260704T011644Z/extract/profile_tops_long/
```

Runner commit:

```text
snissn/ironbird codex/normal-workload-sweep
97055ab0836e5d53a544f52413b1b8c8a94e5457
```

Host context:

| Field | Value |
| --- | --- |
| CPU | 11th Gen Intel Core i5-11400F @ 2.60GHz |
| CPU count | 12 |
| Memory | 31 GiB |
| OS | Linux 6.8.0-124-generic x86_64 |
| Go | go1.25.0 linux/amd64 |
| Docker | Docker 29.1.3 |
| Fast artifact disk | `/mnt/fast4tb`, 3.6T total, 2.7T free at run start |

Docker image tag:

```text
ironbird-report:snissn-sdk-28e5525f-fullstack-cosmosdb-f1d8b01-cometdb-b4f878-gomap-1afe86c
```

Dependency pins:

| Module | Version/ref |
| --- | --- |
| `github.com/cosmos/cosmos-db` | `v0.0.0-20260702024646-f1d8b016a90c` / `f1d8b016a90cc39abde5d362e4f6b54b96df5c73` |
| `github.com/snissn/gomap` | `v0.6.2-0.20260702024414-1afe86c1cbc0` / `1afe86c1cbc0acc7336fc0944c69ebfcd2f3dc8d` |
| `github.com/cosmos/iavl` | `v0.0.0-20260701072929-12a26715119b` / `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| `github.com/cometbft/cometbft-db` | `v0.0.0-20260701074104-b4f87847a725` / `b4f87847a725f92a046d927ce4a0f5b08b965995` |

Common chain shape:

| Field | Value |
| --- | --- |
| Validators | 1 |
| Nodes | 0 |
| Active wallets | 5,000 |
| Preseed profile | 100,000 genesis accounts |
| Raw tx audit | disabled with `-raw-tx-audit=false` |
| Load window | `LOAD_WINDOW_MIN=5m`, `LOAD_WINDOW_TARGET_FRACTION=0.995` |

Raw tx audit remains disabled because Catalyst's post-load `/tx` lookup path
still disagrees with this CometBFT response/query shape. Corrected counts use
app/CometBFT metrics.

## Reproduction Command

```sh
OUT_ROOT=/mnt/fast4tb/ironbird-normal-workload-sweep-stopwindow-$(date -u +%Y%m%dT%H%M%SZ) \
REBUILD_RUNNER=true \
LOAD_WINDOW_MIN=5m \
LOAD_WINDOW_TARGET_FRACTION=0.995 \
DRAIN_TIMEOUT=5m \
STOP_CATALYST_AFTER_LOAD_WINDOW=true \
TMPDIR=/mnt/fast4tb/tmp \
scripts/ironbird_normal_workload_sweep.sh
```

The script builds `/mnt/fast4tb/tmp/local-report-runner-normal` from the current
branch and writes one JSON result per workload/backend attempt.

## Workload Matrix

| Workload | Shape | Base blocks x txs | Max gas | Why it exists |
| --- | --- | ---: | ---: | --- |
| Plain send | 1 `MsgSend` per tx | 400 x 500 | 75,000,000 | closest normal Cosmos baseline |
| Small multisend | 1 `MsgMultiSend`, 2 recipients | 240 x 500 | 100,000,000 | mild state fan-out |
| Moderate multisend | 1 `MsgMultiSend`, 10 recipients | 160 x 500 | 150,000,000 | moderate state fan-out |
| High fan-out anchor | 1 `MsgArr` containing 20 `MsgMultiSend` x 25 recipients | 32 x 250 | 300,000,000 | DB-sensitive upper-bound comparison |

The script retries rows with doubled block counts until the accepted load window
is at least 5 minutes, up to five attempts.

## Metric Definitions

| Metric | Definition | Primary use |
| --- | --- | --- |
| Load-window TPS | App/CometBFT metric successful txs over the accepted sustained load window | Primary chain throughput comparison |
| Runtime TPS | Catalyst/corrected runtime TPS, or load-window fallback when Catalyst is stopped by the accepted-window condition | Secondary; can diverge when send and inclusion phases are decoupled |
| Wall TPS | Successful txs over full local runner wall time, including launch/setup/profile collection | End-to-end lower-bound comparison |
| Effective ops/s | Successful txs multiplied by workload fan-out, then divided by the same timing window | Useful only for fan-out workloads; not ordinary TPS |

## Backend Verification

Every accepted run verified both `config/app.toml` and `config/config.toml`.

| Workload | Backend | Observed app DB | Observed node DB | Valid |
| --- | --- | --- | --- | --- |
| Plain send | goleveldb | goleveldb | goleveldb | true |
| Plain send | TreeDB | treedb | treedb | true |
| Small multisend | goleveldb | goleveldb | goleveldb | true |
| Small multisend | TreeDB | treedb | treedb | true |
| Moderate multisend | goleveldb | goleveldb | goleveldb | true |
| Moderate multisend | TreeDB | treedb | treedb | true |
| High fan-out anchor | goleveldb | goleveldb | goleveldb | true |
| High fan-out anchor | TreeDB | treedb | treedb | true |

## Load-Window Acceptance

| Workload | Backend | Accepted attempt | Rejected short attempts | Accepted load-window seconds |
| --- | --- | ---: | --- | ---: |
| Plain send | goleveldb | 1 | none | 374.5 |
| Plain send | TreeDB | 1 | none | 388.5 |
| Small multisend | goleveldb | 2 | attempt 1: 222.0s | 449.0 |
| Small multisend | TreeDB | 2 | attempt 1: 219.5s | 460.5 |
| Moderate multisend | goleveldb | 2 | attempt 1: 193.0s | 422.0 |
| Moderate multisend | TreeDB | 2 | attempt 1: 166.0s | 342.5 |
| High fan-out anchor | goleveldb | 1 | none | 806.5 |
| High fan-out anchor | TreeDB | 1 | none | 340.5 |

All accepted rows met the 5-minute minimum.

## Throughput Results

| Workload | Backend | Attempt | Intended tx | Load target | Load-window tx | Load-window s | Load-window TPS | Runtime TPS | Wall TPS | Effective load-window ops/s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 1 | 200,000 | 199,000 | 199,092 | 374.5 | 531.58 | 533.99 | 350.93 |  |
| Plain send | TreeDB | 1 | 200,000 | 199,000 | 199,050 | 388.5 | 512.32 | 514.76 | 354.75 |  |
| Small multisend | goleveldb | 2 | 240,000 | 238,800 | 238,991 | 449.0 | 532.24 | 534.48 | 382.47 | 1,064.48 |
| Small multisend | TreeDB | 2 | 240,000 | 238,800 | 239,011 | 460.5 | 519.00 | 520.04 | 376.60 | 1,037.99 |
| Moderate multisend | goleveldb | 2 | 160,000 | 159,200 | 159,448 | 422.0 | 377.82 | 379.12 | 267.12 | 3,778.16 |
| Moderate multisend | TreeDB | 2 | 160,000 | 159,200 | 159,493 | 342.5 | 465.63 | 467.10 | 311.87 | 4,656.33 |
| High fan-out anchor | goleveldb | 1 | 8,000 | 7,960 | 7,982 | 806.5 | 9.90 | 15.13 | 8.14 | 4,948.37 |
| High fan-out anchor | TreeDB | 1 | 8,000 | 7,960 | 7,961 | 340.5 | 23.38 | 23.49 | 15.40 | 11,689.28 |

Pairwise ratios use TreeDB divided by goleveldb:

| Workload | Load-window TPS ratio | Wall TPS ratio | Effective load-window ops/s ratio | Category |
| --- | ---: | ---: | ---: | --- |
| Plain send | 0.96x | 1.01x | 0.96x | goleveldb slight load-window win |
| Small multisend | 0.98x | 0.98x | 0.98x | near tie |
| Moderate multisend | 1.23x | 1.17x | 1.23x | TreeDB win |
| High fan-out anchor | 2.36x | 1.89x | 2.36x | TreeDB large win |

## Resource Footprint

Docker max memory is sampled from Docker stats during the measured run. Process
RSS after is the app metric scraped after load and can differ from Docker's
sampled maximum.

| Workload | Backend | Docker max mem | Process RSS after | Docker block write | Docker block read | Data delta | application.db delta | state.db delta | tx_index.db delta |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2.13 GiB | 2.09 GiB | 32.04 GiB | 2.42 MiB | 1,232.7 MiB | 653.7 MiB | 56.8 MiB | 390.1 MiB |
| Plain send | TreeDB | 4.85 GiB | 7.38 GiB | 15.55 GiB | 0.24 MiB | 6,022.9 MiB | 2,026.1 MiB | 81.1 MiB | 3,393.9 MiB |
| Small multisend | goleveldb | 2.19 GiB | 1.99 GiB | 38.00 GiB | 3.41 MiB | 1,474.3 MiB | 630.3 MiB | 60.8 MiB | 610.0 MiB |
| Small multisend | TreeDB | 5.01 GiB | 7.62 GiB | 16.95 GiB | 0.25 MiB | 5,903.7 MiB | 1,858.6 MiB | 105.2 MiB | 3,289.8 MiB |
| Moderate multisend | goleveldb | 2.66 GiB | 2.42 GiB | 33.25 GiB | 0.77 MiB | 1,314.8 MiB | 390.9 MiB | 70.8 MiB | 650.1 MiB |
| Moderate multisend | TreeDB | 4.99 GiB | 6.50 GiB | 11.55 GiB | 0.21 MiB | 3,968.2 MiB | 1,263.0 MiB | 58.6 MiB | 2,172.1 MiB |
| High fan-out anchor | goleveldb | 3.36 GiB | 3.25 GiB | 25.80 GiB | 0.18 MiB | 978.2 MiB | 94.0 MiB | 107.5 MiB | 475.8 MiB |
| High fan-out anchor | TreeDB | 4.75 GiB | 4.66 GiB | 6.76 GiB | 0.04 MiB | 849.0 MiB | 170.8 MiB | 38.8 MiB | 336.4 MiB |

Resource takeaways:

- TreeDB reduced Docker block writes in every accepted row.
- TreeDB used substantially more memory in every accepted row.
- TreeDB's final data footprint was much larger for plain, small, and moderate
  workloads, mostly from `application.db` and `tx_index.db`.
- In the high fan-out anchor, TreeDB had a smaller total `/simd/data` delta and
  smaller `state.db` and `tx_index.db` deltas, despite a larger
  `application.db` delta.

## Timing Breakdown

The following timing table uses the load-window storage signal captured at the
accepted window. ABCI timers are useful indicators, but they are not a strict
wall-clock partition: some work is concurrent, some storage work is nested
inside CheckTx/FinalizeBlock/Commit, and some non-ABCI work is not directly
classified by this harness.

| Workload | Backend | Load-window s | ABCI observed s | Commit s | FinalizeBlock s | CheckTx s | Query s | Avg commit ms | Commit % of observed ABCI | Process CPU delta s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 374.5 | 263.1 | 39.2 | 71.5 | 149.9 | 0.0 | 38.4 | 14.9% | 891.8 |
| Plain send | TreeDB | 388.5 | 245.5 | 53.1 | 58.8 | 131.6 | 0.0 | 59.2 | 21.6% | 683.9 |
| Small multisend | goleveldb | 449.0 | 310.0 | 39.4 | 83.3 | 183.5 | 0.0 | 33.3 | 12.7% | 1,098.7 |
| Small multisend | TreeDB | 460.5 | 282.1 | 57.8 | 64.6 | 156.7 | 0.0 | 54.7 | 20.5% | 780.1 |
| Moderate multisend | goleveldb | 422.0 | 273.8 | 26.5 | 82.8 | 161.2 | 0.0 | 37.5 | 9.7% | 1,103.0 |
| Moderate multisend | TreeDB | 342.5 | 246.7 | 34.2 | 79.0 | 130.4 | 0.0 | 42.6 | 13.9% | 716.3 |
| High fan-out anchor | goleveldb | 806.5 | 699.0 | 8.0 | 72.1 | 398.9 | 216.0 | 23.4 | 1.1% | 2,212.6 |
| High fan-out anchor | TreeDB | 340.5 | 270.5 | 10.7 | 72.5 | 183.5 | 0.0 | 41.6 | 4.0% | 869.4 |

Commit is visible but not the main bottleneck in this matrix. TreeDB often has a
higher average commit time, but the larger wins come from lower CheckTx,
FinalizeBlock, CPU sample, allocation, and block-write pressure as fan-out
increases.

The high fan-out goleveldb row shows 216s of ABCI query time during the load
window. That did not appear in the matching TreeDB row. This may be a harness or
CometBFT query/index interaction and should not be reduced to "app execution"
without deeper instrumentation.

## Profile Highlights

| Workload | Backend | CPU samples | Heap alloc-space | Backend-specific profile notes |
| --- | --- | ---: | ---: | --- |
| Plain send | goleveldb | 903.9 s | 115.2 GB | LevelDB compaction cumulative about 262s; `leveldb.Batch.grow` 3.68 GB alloc-space |
| Plain send | TreeDB | 694.2 s | 190.3 GB | TreeDB zipper apply/write frames about 56-58s cumulative; `memtable.getAppendOnlyEntries` 57.19 GB alloc-space |
| Small multisend | goleveldb | 1,109.7 s | 147.3 GB | LevelDB compaction cumulative about 317s; `leveldb.Batch.grow` 5.58 GB alloc-space |
| Small multisend | TreeDB | 791.4 s | 220.1 GB | TreeDB zipper apply/write frames about 52-57s cumulative; `memtable.getAppendOnlyEntries` 66.82 GB alloc-space |
| Moderate multisend | goleveldb | 1,119.1 s | 195.5 GB | LevelDB compaction cumulative about 317s; `leveldb.Batch.grow` 22.71 GB alloc-space |
| Moderate multisend | TreeDB | 734.8 s | 212.4 GB | `memtable.getAppendOnlyEntries` 52.47 GB alloc-space; `Batch.SetWithRevision` 6.12 GB; `caching.getEntrySlice` 5.30 GB |
| High fan-out anchor | goleveldb | 2,222.8 s | 637.4 GB | LevelDB compaction cumulative about 562s; `leveldb.Batch.grow` about 84.0 GB; `leveldb/table.Reader.find` about 28.8 GB |
| High fan-out anchor | TreeDB | 887.9 s | 320.9 GB | `memtable.getAppendOnlyEntries` 24.38 GB; `Batch.SetWithRevision` 13.64 GB; `commitlog.DecodeCommandFrame` 5.83 GB |

Profile takeaways:

- goleveldb's clearest backend-specific cost is LevelDB compaction, especially
  as fan-out increases.
- TreeDB removes that compaction profile signature but has its own allocation
  pressure, especially `TreeDB/internal/memtable.getAppendOnlyEntries` and
  `TreeDB/caching.Batch.SetWithRevision`.
- TreeDB CPU samples were lower in every accepted row, even in rows where
  load-window TPS was a near tie or slight goleveldb win.
- TreeDB heap alloc-space was higher in the normal rows but lower in the high
  fan-out anchor.

## Category Wins

TreeDB wins:

- Moderate fan-out throughput: 1.23x load-window TPS and 1.17x wall TPS.
- High fan-out throughput: 2.36x load-window TPS and 1.89x wall TPS.
- Docker block writes in every accepted row, including 25.80 GiB goleveldb vs
  6.76 GiB TreeDB on the high fan-out anchor.
- CPU profile samples in every accepted row.
- High fan-out heap alloc-space and total data delta.

goleveldb wins or ties:

- Plain send load-window TPS: 531.58 goleveldb vs 512.32 TreeDB.
- Small multisend load-window TPS: 532.24 goleveldb vs 519.00 TreeDB, close
  enough to treat as a near tie.
- Memory footprint in every accepted row.
- Final data footprint on plain, small, and moderate workloads.

Inconclusive or under-measured:

- Non-ABCI time is not yet explained well enough. The harness can show ABCI
  method timers and a residual non-ABCI bucket, but it cannot attribute that
  bucket to mempool, RPC, scheduler, kernel I/O, profile collection, or DB
  internals.
- App DB versus CometBFT node DB costs are not separately timed. This matters
  because the scenario intentionally changes both.
- DB operation counts, DB latency histograms, compaction/checkpoint events,
  fsync/write latency, and per-directory write attribution are not yet captured.
- The high fan-out goleveldb query time needs deeper attribution before using it
  as an optimization target.

## Post-Sprint Review

This sweep improves the Ironbird evidence substantially versus the earlier
high-fanout-only report. It shows that TreeDB's advantage depends strongly on
workload shape:

- Simple tx envelopes do not reproduce the Celestia-style lift.
- Moderate fan-out begins to show a meaningful TreeDB throughput advantage.
- High fan-out produces a large TreeDB win and matches the expected direction
  from prior storage-heavy evidence.

The main learning is that "Cosmos TPS" is not one regime. Plain send is mostly
an app, mempool, and framework throughput test with relatively weak DB signal.
High fan-out creates enough storage pressure for LevelDB compaction and write
amplification to dominate visibly. Celestia production sync is likely closer to
the storage-sensitive side than to plain send, but this Ironbird run is not
itself production replay evidence.

The harness fixes from this sprint were necessary. Without stop-window and
sustained-window guardrails, short runs and Catalyst post-load behavior can
distort the result. The remaining issue is instrumentation depth: this harness
can classify workload-level outcomes, but it cannot yet fully explain every
runtime bucket.

## What Not To Claim

- Do not claim TreeDB universally improves Cosmos TPS.
- Do not claim this is equivalent to Celestia production sync or forked-mainnet
  replay.
- Do not quote effective ops/s as ordinary TPS.
- Do not use high fan-out alone as a normal workload result.
- Do not treat Catalyst runtime TPS as the headline when load-window TPS
  diverges.
- Do not claim public upstream readiness from this fork-local sprint.

## Recommended Next Steps

1. Add deeper instrumentation before another optimization loop: DB operation
   counts and latency by app DB, blockstore DB, state DB, and tx-index DB;
   compaction/checkpoint/WAL counters; and per-directory write attribution.
2. Fix or replace Catalyst raw tx audit for this CometBFT response shape so
   future reports can compare app metrics, CometBFT metrics, and direct tx
   lookup without disabling the audit.
3. Investigate TreeDB allocation hot spots shown here:
   `memtable.getAppendOnlyEntries`, `Batch.SetWithRevision`,
   `caching.getEntrySlice`, and `commitlog.DecodeCommandFrame`.
4. Investigate TreeDB normal-workload footprint, especially the large
   `application.db` and `tx_index.db` deltas.
5. Build a production-shaped state or replay lane before trying to make broad
   public claims from Ironbird. A useful next target is seeded production-like
   state plus a real or statistically production-shaped workload.

## Validation

Focused validation before the sweep:

```sh
GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./messages
bash -n scripts/ironbird_normal_workload_sweep.sh
git diff --check
GOWORK=off go build -o /mnt/fast4tb/tmp/local-report-runner-normal ./cmd/local-report-runner
```

The full accepted sweep completed successfully with all eight backend/workload
pairs accepted and no benchmark containers left running.
