# Ironbird Exact Non-ABCI Attribution Report: gomap 2182e84

Date: 2026-07-09 UTC / 2026-07-08 HST

This report closes `snissn/ironbird#31`, the report node under the exact
non-ABCI attribution graph `snissn/ironbird#28`. It uses the interval model
from `#29` and the bounded ABCI scrape-sample collection from `#30` to rerun
the low-fanout Ironbird matrix on the merged Ironbird head and the pinned gomap
`2182e84` dependency.

## Headline

The new instrumentation answers the immediate non-ABCI question: the previously
large non-ABCI number was a summed-duration residual, not an exact wall-clock
idle or harness bucket. With bounded interval union enabled, the accepted
windows have only `1.52s` to `2.53s` of exact residual outside the ABCI busy
union:

| Workload | Backend | Window TPS | Exact non-ABCI s | Exact non-ABCI % | Prior-style approx non-ABCI % |
| --- | --- | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 663.28 | 1.52 | 0.51% | 33.08% |
| Plain send | TreeDB | 594.56 | 2.53 | 0.75% | 42.39% |
| Small multisend | goleveldb | 676.03 | 1.52 | 0.43% | 34.72% |
| Small multisend | TreeDB | 593.75 | 2.53 | 0.63% | 42.30% |

TreeDB is still slower in this low-fanout matrix, but the measured gap is now
better bounded:

| Workload | goleveldb TPS | TreeDB TPS | TreeDB ratio | TreeDB delta |
| --- | ---: | ---: | ---: | ---: |
| Plain send | 663.28 | 594.56 | 0.90x | -10.36% |
| Small multisend | 676.03 | 593.75 | 0.88x | -12.17% |

The current best read is:

- There is not a large exact non-ABCI wall-time bucket left unexplained by the
  accepted-window interval model.
- The interval source is `bounded_sample`, derived from Prometheus counter
  deltas, so it is an upper-bound scrape-window model rather than event-level
  ABCI tracing.
- TreeDB commit is materially slower per block, but commit explains only part
  of the slower block cadence.
- Active-window CPU profiles do not show a single dominant TreeDB CPU hotspot.
  TreeDB uses fewer validator CPU seconds than goleveldb while still producing
  lower TPS, so the remaining gap looks more like synchronization, cadence,
  wait, or app/DB callback sequencing than raw CPU saturation.
- Block and mutex profiles are dominated by CometBFT/Cosmos local transaction
  intake stacks, especially `mempool/cat.(*TxPool).CheckTx`,
  `TryAddNewTx`, and `BroadcastTxSync`.

## Consumed Code

| Component | Evidence |
| --- | --- |
| `snissn/ironbird` head | `33b73e71045b6606937821e30f772167e8030ad8` |
| Ironbird branch | `codex/31-exact-nonabci-report` |
| `snissn/gomap` | `2182e84bd668f6ea610726717d90e09a86a17a32` |
| Ironbird gomap pin | `v0.6.2-0.20260708213404-2182e84bd668` |
| `snissn/cosmos-db` | `6ddcb75557e59bc4e6668ac7699cd52b63b3e402` |
| `snissn/iavl` | `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| `snissn/cometbft-db` | `b4f87847a725f92a046d927ce4a0f5b08b965995` |
| Docker image | `ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-2182e84` |

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-exact-nonabci-attribution-20260709T020911Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-exact-nonabci-attribution-20260709T020911Z/summary.tsv
```

Command:

```sh
RUN_ID=20260709T020911Z
OUT_ROOT=/mnt/fast4tb/ironbird-exact-nonabci-attribution-${RUN_ID}
RUNNER=/mnt/fast4tb/tmp/local-report-runner-exact-nonabci-${RUN_ID}

REBUILD_RUNNER=true \
  OUT_ROOT="$OUT_ROOT" \
  RUNNER="$RUNNER" \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  STOP_CATALYST_AFTER_LOAD_WINDOW=true \
  ACTIVE_WINDOW_PROFILE_DURATION=30s \
  TREEDB_POST_LOAD_DWELL=5m \
  BACKEND_ORDER_MODE=alternate \
  TMPDIR=/mnt/fast4tb/tmp \
  scripts/ironbird_lowfanout_attribution_sweep.sh
```

`BACKEND_ORDER_MODE=alternate` was used to avoid always running TreeDB after
goleveldb. TreeDB rows also captured a 5-minute post-load dwell snapshot.

## Matrix

Accepted rows:

| Workload | Backend | Attempt | Blocks | Tx/block | Window s | Successful tx | Runtime TPS | Window TPS | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 1 | 400 | 500 | 335.03 | 199,194 | 594.56 | 594.56 | 253.18 |
| Plain send | goleveldb | 1 | 400 | 500 | 300.02 | 199,000 | 663.28 | 663.28 | 443.69 |
| Small multisend | goleveldb | 2 | 480 | 500 | 353.52 | 238,992 | 676.03 | 676.03 | 468.47 |
| Small multisend | TreeDB | 2 | 480 | 500 | 402.52 | 238,998 | 593.75 | 593.75 | 278.19 |

Rejected-but-preserved rows:

| Workload | Backend | Attempt | Reason |
| --- | --- | ---: | --- |
| Small multisend | goleveldb | 1 | Completed `119,998` successful tx in `180.52s`, below the 5-minute acceptance floor |
| Small multisend | TreeDB | 1 | Completed `119,999` successful tx in `185.02s`, below the 5-minute acceptance floor |

## Exact Non-ABCI Accounting

The exact residual below is computed as
`max(0, load_window_seconds - abci_busy_union_seconds)`. The union is exact over
the bounded intervals collected from
`cometbft_abci_connection_method_timing_seconds` counter changes. It is not
event-level tracing, and the runner preserves the older approximate residual
for comparison.

| Workload | Backend | Window s | ABCI union s | Exact residual s | Approx residual s | Approx residual % | CheckTx s | Finalize s | Commit s | Validator CPU s | Core equiv |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 335.03 | 332.50 | 2.53 | 142.02 | 42.39% | 103.43 | 43.15 | 44.74 | 488.68 | 1.46 |
| Plain send | goleveldb | 300.02 | 298.50 | 1.52 | 99.24 | 33.08% | 115.71 | 51.22 | 32.10 | 758.69 | 2.53 |
| Small multisend | goleveldb | 353.52 | 352.00 | 1.52 | 122.73 | 34.72% | 138.30 | 57.71 | 32.31 | 926.33 | 2.62 |
| Small multisend | TreeDB | 402.52 | 400.00 | 2.53 | 170.28 | 42.30% | 129.50 | 52.15 | 48.17 | 594.21 | 1.48 |

Interpretation:

- The old approximate residual overstated the actionable "non-ABCI" question
  because summed ABCI durations do not describe wall-clock interval coverage.
- TreeDB does not spend more summed CheckTx or FinalizeBlock time in these
  accepted rows. Its summed CheckTx and FinalizeBlock times are lower than
  goleveldb in both workloads.
- TreeDB commit is higher by `12.64s` on plain send and `15.86s` on small
  multisend, but the total load-window gap is `35.00s` and `49.00s`,
  respectively. Commit is material, not complete.

## Block Cadence

| Workload | Backend | Commit count | Avg tx/block | Avg block interval ms | Avg CheckTx ms | Avg FinalizeBlock ms | Avg Commit ms |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 861 | 231.39 | 549.35 | 0.255 | 50.18 | 51.96 |
| Plain send | goleveldb | 1,007 | 197.32 | 437.48 | 0.290 | 50.86 | 31.88 |
| Small multisend | goleveldb | 1,148 | 208.36 | 430.67 | 0.288 | 50.31 | 28.14 |
| Small multisend | TreeDB | 1,037 | 230.69 | 523.11 | 0.267 | 50.29 | 46.45 |

The cadence gap is larger than the commit gap:

| Workload | TreeDB commit penalty | TreeDB cadence penalty | Commit penalty / cadence penalty |
| --- | ---: | ---: | ---: |
| Plain send | +20.08 ms/block | +111.87 ms/block | 17.9% |
| Small multisend | +18.30 ms/block | +92.44 ms/block | 19.8% |

This is the strongest quantitative reason not to focus only on commit. The next
instrumentation should split the remaining cadence difference into app commit,
FinalizeBlock, mempool admission, proposal/block assembly, and DB callback
spans.

## Resource And Footprint

| Workload | Backend | Max validator CPU % | Max validator RSS | Validator block writes | `/simd/data` bytes | `application.db` bytes | `tx_index.db` bytes |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 335.99 | 6.743 GiB | 16.1 GB | 6,413,487,872 | 2,185,678,847 | 3,582,214,289 |
| Plain send | goleveldb | 390.58 | 2.091 GiB | 34.0 GB | 1,434,180,094 | 732,354,505 | 497,696,984 |
| Small multisend | goleveldb | 398.02 | 2.117 GiB | 41.2 GB | 1,411,517,017 | 605,780,964 | 549,323,519 |
| Small multisend | TreeDB | 337.14 | 7.354 GiB | 19.2 GB | 6,114,254,326 | 1,983,915,692 | 3,351,026,070 |

TreeDB has a much larger short-run directory/RSS footprint but lower container
block write volume. That footprint remains an operational subject, especially
for dwell/maintenance policy, but the accepted-window TPS gap is currently more
strongly connected to block cadence and transaction-intake wait profiles than
to raw block-device write volume.

## Active-Window Profiles

All accepted rows captured 30-second active-window CPU, allocs, block, mutex,
goroutine, heap, and trace artifacts. Example paths are under:

```text
/mnt/fast4tb/ironbird-exact-nonabci-attribution-20260709T020911Z/<workload>/<backend>/<attempt>/pprof/
```

CPU profile totals:

| Workload | Backend | CPU samples | Approx profile CPU |
| --- | --- | ---: | ---: |
| Plain send | TreeDB | 42.41s | 141.36% |
| Plain send | goleveldb | 60.72s | 202.40% |
| Small multisend | goleveldb | 58.05s | 193.50% |
| Small multisend | TreeDB | 36.57s | 121.90% |

The top flat CPU samples are mostly Go runtime/GC/allocation, secp256k1,
syscalls, memmove, and compression. TreeDB-owned CPU appears, but not as a
single dominant flat hotspot in these samples.

Alloc-space profiles:

| Workload | Backend | Total alloc-space | Notable DB-owned sites |
| --- | --- | ---: | --- |
| Plain send | TreeDB | 15,579.61 MB | `memtable.getAppendOnlyEntries` 539.71 MB; command-WAL builder 460.37 MB; command-WAL read/decode 559.48 MB combined; zstd history 179.30 MB |
| Plain send | goleveldb | 14,975.92 MB | `leveldb.MakeBatch` 477.25 MB; `util.makeSlice` 395.92 MB; `BufferPool.Get` 362.98 MB; batch append/grow 682.14 MB combined |
| Small multisend | goleveldb | 15,633.71 MB | `leveldb.MakeBatch` 444.76 MB; batch append/grow 794.62 MB combined; `BufferPool.Get` 409.88 MB; `util.makeSlice` 373.05 MB |
| Small multisend | TreeDB | 14,534.70 MB | `memtable.getAppendOnlyEntries` 478.44 MB; command-WAL builder 398.41 MB; command-WAL read/decode 547.27 MB combined; `Batch.SetWithRevision` 190.89 MB |

These allocations are real, but the macro result does not currently support
"TreeDB allocation pressure is the primary TPS limiter." TreeDB alloc-space is
similar to or lower than goleveldb in this accepted matrix while TreeDB TPS is
lower.

Block profiles:

| Workload | Backend | Total delay | Dominant flat stacks |
| --- | --- | ---: | --- |
| Plain send | TreeDB | 5,894.40s | `runtime.selectgo` 46.93%; `sync.(*Mutex).Lock` 39.20%; `runtime.chanrecv2` 13.44% |
| Plain send | goleveldb | 3,967.39s | `sync.(*Mutex).Lock` 72.40%; `runtime.selectgo` 25.90%; `runtime.chanrecv2` 1.33% |
| Small multisend | goleveldb | 4,062.04s | `sync.(*Mutex).Lock` 72.47%; `runtime.selectgo` 25.81%; `runtime.chanrecv2` 1.31% |
| Small multisend | TreeDB | 5,450.05s | `runtime.selectgo` 47.64%; `sync.(*Mutex).Lock` 38.27%; `runtime.chanrecv2` 13.64% |

The cumulative block stacks are dominated by local broadcast and CometBFT CAT
mempool paths, especially `TxPool.CheckTx`, `TxPool.TryAddNewTx`,
`TxPool.preCheck`, and `BroadcastTxSync`.

Mutex profiles:

| Workload | Backend | Total mutex delay | Main cumulative owner |
| --- | --- | ---: | --- |
| Plain send | TreeDB | 1,807.00s | `TxPool.CheckTx` / `BroadcastTxSync` at 97.49% cumulative |
| Plain send | goleveldb | 2,196.72s | `TxPool.CheckTx` / `BroadcastTxSync` at 90.08% cumulative |
| Small multisend | goleveldb | 2,158.06s | `TxPool.CheckTx` / `BroadcastTxSync` at 94.30% cumulative |
| Small multisend | TreeDB | 2,111.94s | `TxPool.CheckTx` / `BroadcastTxSync` at 97.22% cumulative |

This does not prove the mempool is the only bottleneck, but it does show that
the active-window waiting profile is shared and transaction-intake-heavy rather
than TreeDB-flat-CPU-heavy.

## What This Resolves

1. The broad "non-ABCI" bucket is no longer the top unknown in its prior form.
   The exact residual over bounded intervals is tiny in all accepted rows.
2. The earlier approximate residual remains useful as a warning that summed
   counters are not a wall-clock decomposition, not as proof of a large
   non-ABCI idle/harness window.
3. TreeDB commit cost is quantified and real, but average commit penalty is
   only about one fifth of the block-cadence penalty.
4. The gap is likely inside ABCI-covered transaction/block processing cadence
   or in transaction-intake synchronization/backpressure, not in a large
   unmeasured outer harness phase.

## Recommended Next Graph

The next sprint should stop broad profiling loops and add event-level or scoped
span attribution where this report still cannot distinguish causes:

1. Add app/ABCI span labels or counters for CheckTx, FinalizeBlock, and Commit
   sub-stages, including DB read batch, DB write batch, adapter WriteSync,
   command-WAL sync, value-log append/sync, and checkpoint/publish steps.
2. Add CometBFT CAT mempool transaction-intake instrumentation: `CheckTx` lock
   wait, lock hold, preCheck duration, application callback duration, and local
   `BroadcastTxSync` wait.
3. Add block-cadence attribution: proposal assembly, consensus commit cadence,
   block execution, app commit, CometBFT state/blockstore/tx-index writes, and
   idle/wait between phases.
4. Preserve the accepted-window interval table, but mark bounded scrape
   intervals separately from event-level spans in the report so future rows do
   not confuse upper-bound interval coverage with true method wall-time.
5. Rerun the same accepted matrix after instrumentation with identical
   `LOAD_WINDOW_MIN=5m`, alternate backend ordering, active-window profiles,
   and TreeDB dwell snapshots.

Success criteria for that follow-up graph should be a quantitative cadence
decomposition that accounts for the `92-112 ms/block` TreeDB cadence penalty
within named sub-stages, or a clear statement of the next missing span needed
to close the accounting.

## Validation

Local validation for this report PR:

```sh
git diff --check
GOWORK=off go test ./cmd/local-report-runner -count=1
```
