# Ironbird Low-Fanout TreeDB Attribution Report

Date: 2026-07-07 UTC

This report closes the attribution matrix node for `snissn/gomap#3601` under
parent tracker `snissn/gomap#3598`.

The goal was to explain why full-stack TreeDB is slower than full-stack
goleveldb in Ironbird's low-fanout `plain-send` and `small-multisend`
workloads, using the new pipeline instrumentation from `snissn/ironbird#17`
and the new TreeDB public write-span counters from `snissn/gomap#3602`.

## Headline

TreeDB is still slower in this low-fanout matrix, but the slowdown is not
explained by per-block public `Checkpoint()` calls.

Accepted load-window results:

| Workload | goleveldb TPS | TreeDB TPS | TreeDB ratio | Window gap |
| --- | ---: | ---: | ---: | ---: |
| Plain send | 634.28 | 572.41 | 0.90x | +34.50s |
| Small multisend | 654.75 | 581.47 | 0.89x | +46.00s |

Primary attribution:

- TreeDB `Commit`/`WriteSync` is a real contributor. TreeDB average commit is
  about `18-19 ms/block` slower than goleveldb in both accepted rows.
- Public TreeDB `Checkpoint()` calls are zero for every observed store in both
  TreeDB rows, so the consumed Ironbird path is not forcing a full public
  checkpoint per block.
- TreeDB app-store `WriteSync` is about `25.6s` plain and `29.8s` small, close
  to the app commit work, but the total window gaps are larger than commit
  alone.
- `CheckTx` and `FinalizeBlock` are not worse for TreeDB in these rows.
  For `small-multisend`, observed ABCI time is almost equal while TreeDB's
  load window is much longer.
- Active-window CPU profiles show TreeDB using fewer CPU samples than goleveldb
  over the same 30s capture, while block/mutex profiles are dominated by
  CometBFT CAT mempool and gRPC broadcast paths. That points to missing
  non-ABCI scheduling, backpressure, or consensus/mempool attribution rather
  than a single obvious TreeDB CPU hotspot.
- TreeDB alloc-space is only about `4-5%` higher than goleveldb in these macro
  rows. TreeDB-owned allocation sites remain worth cleaning when CPU/GC-visible,
  but allocation alone does not currently explain the throughput gap.

## Consumed Code

| Repo | Evidence |
| --- | --- |
| `snissn/ironbird` | `snissn/ironbird#17`, merged as `09a0383e67e99c3905be93fabcec575664d8d245` |
| `snissn/gomap` | `snissn/gomap#3602`, merged as `626163f80649fecf2dc40d8429f921b931c420c6` |
| Ironbird matrix branch | `codex/3601-attribution-matrix` |
| Ironbird gomap pin | `v0.6.2-0.20260707153625-626163f80649` / `626163f80649fecf2dc40d8429f921b931c420c6` |

The Ironbird runner image suffix for this matrix is:

```text
gomap-626163f
```

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-626163f-20260707T153950Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-626163f-20260707T153950Z/summary.tsv
```

Command shape:

```sh
OUT_ROOT=/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-626163f-20260707T153950Z \
  RUNNER=/mnt/fast4tb/tmp/local-report-runner-attribution-626163f \
  REBUILD_RUNNER=false \
  SKIP_BUILD=true \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  STOP_CATALYST_AFTER_LOAD_WINDOW=true \
  ACTIVE_WINDOW_PROFILE_DURATION=30s \
  TMPDIR=/mnt/fast4tb/tmp \
  scripts/ironbird_lowfanout_attribution_sweep.sh
```

The first row built the pinned Docker image before the final resumed command
used `SKIP_BUILD=true`. The sweep accepts only rows where the load window is
reached, satisfies the minimum duration, and has no runner error. The first
`small-multisend` attempts were intentionally rejected as too short and rerun
with more blocks.

## Accepted Matrix

| Workload | Backend | Attempt | Blocks | Tx/block | Load-window s | Successful tx | Load-window TPS | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 1 | 400 | 500 | 314.03 | 199,180 | 634.28 | 307.67 |
| Plain send | TreeDB | 1 | 400 | 500 | 348.52 | 199,498 | 572.41 | 397.93 |
| Small multisend | goleveldb | 2 | 480 | 500 | 365.02 | 238,999 | 654.75 | 457.27 |
| Small multisend | TreeDB | 2 | 480 | 500 | 411.02 | 238,998 | 581.47 | 419.47 |

Rejected-but-preserved rows:

| Workload | Backend | Attempt | Reason |
| --- | --- | ---: | --- |
| Small multisend | goleveldb | 1 | Load window below minimum duration |
| Small multisend | TreeDB | 1 | Load window below minimum duration |

## Pipeline Timing

| Workload | Backend | Window s | CheckTx s | Finalize s | Commit s | Commit count | Avg commit ms | Observed ABCI s | Commit/ABCI | Consensus interval s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 314.03 | 121.72 | 55.18 | 34.97 | 976 | 35.83 | 213.74 | 16.4% | 460.06 |
| Plain send | TreeDB | 348.52 | 109.93 | 46.58 | 47.19 | 878 | 53.75 | 205.42 | 23.0% | 492.19 |
| Small multisend | goleveldb | 365.02 | 142.07 | 60.24 | 33.53 | 1,150 | 29.16 | 238.31 | 14.1% | 510.47 |
| Small multisend | TreeDB | 411.02 | 133.08 | 54.57 | 49.39 | 1,024 | 48.23 | 239.59 | 20.6% | 552.62 |

Interpretation:

- Commit cost moved in the expected direction: TreeDB is slower per block.
- Plain commit explains about `12.2s` of a `34.5s` load-window gap.
- Small commit explains about `15.9s` of a `46.0s` load-window gap.
- In both rows, TreeDB has lower aggregate `CheckTx` and `FinalizeBlock` time.
- In `small-multisend`, observed ABCI total is effectively tied
  (`238.3s` goleveldb, `239.6s` TreeDB) while the TreeDB load window is
  `46.0s` longer. That is the strongest signal that the remaining gap sits in
  non-ABCI timing, queueing, consensus/mempool scheduling, load generation
  backpressure, or store work not captured by the current explicit ABCI
  buckets.

## TreeDB Store Counters

These are active load-window deltas from the new gomap public counters and
existing TreeDB debug counters.

### Plain Send TreeDB

| Store | Write calls | Write s | WriteSync calls | WriteSync s | Public checkpoint calls | Cache checkpoint runs | Cache checkpoint s | Cache max ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `application.db` | 24,074 | 5.77 | 879 | 25.58 | 0 | 15 | 3.74 | 403 |
| `blockstore.db` | 0 | 0.00 | 1,756 | 84.38 | 0 | 442 | 54.89 | 270 |
| `evidence.db` | 0 | 0.00 | 0 | 0.00 | 0 | 2 | 0.00 | 0 |
| `metadata.db` | 0 | 0.00 | 0 | 0.00 | 0 | 5 | 0.00 | 0 |
| `state.db` | 0 | 0.00 | 878 | 24.72 | 0 | 12 | 2.03 | 189 |
| `tx_index.db` | 0 | 0.00 | 1,753 | 205.38 | 0 | 441 | 177.56 | 1,419 |

Selected plain-send TreeDB counters:

| Store | Span ops | Span spans | Span fallbacks | Vlog append alloc MiB | Vlog decode alloc MiB | mmap fallback reads | mmap hits |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `application.db` | 9,688,159 | 511,244 | 205 | 248 | 363 | 160,301 | 213,964 |
| `blockstore.db` | 204,611 | 174,436 | 0 | 248 | 363 | 84,488 | 18,616 |
| `state.db` | 2,503 | 49 | 0 | 248 | 363 | 0 | 0 |
| `tx_index.db` | 6,006,756 | 1,457,676 | 0 | 248 | 363 | 1,006,293 | 520,530 |

### Small Multisend TreeDB

| Store | Write calls | Write s | WriteSync calls | WriteSync s | Public checkpoint calls | Cache checkpoint runs | Cache checkpoint s | Cache max ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `application.db` | 25,459 | 5.96 | 1,024 | 29.77 | 0 | 19 | 4.48 | 356 |
| `blockstore.db` | 0 | 0.00 | 2,048 | 101.67 | 0 | 523 | 67.98 | 404 |
| `evidence.db` | 0 | 0.00 | 0 | 0.00 | 0 | 7 | 0.00 | 0 |
| `metadata.db` | 0 | 0.00 | 0 | 0.00 | 0 | 10 | 0.00 | 0 |
| `state.db` | 0 | 0.00 | 1,024 | 29.22 | 0 | 15 | 2.68 | 214 |
| `tx_index.db` | 0 | 0.00 | 2,048 | 256.59 | 0 | 523 | 219.90 | 1,377 |

Selected small-multisend TreeDB counters:

| Store | Span ops | Span spans | Span fallbacks | Vlog append alloc MiB | Vlog decode alloc MiB | mmap fallback reads | mmap hits |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `application.db` | 7,644,273 | 446,687 | 246 | 320 | 538 | 177,201 | 335,032 |
| `blockstore.db` | 245,040 | 211,865 | 0 | 320 | 538 | 140,840 | 43,116 |
| `state.db` | 2,899 | 62 | 0 | 320 | 538 | 0 | 0 |
| `tx_index.db` | 8,848,832 | 1,409,339 | 0 | 320 | 538 | 1,068,162 | 747,835 |

Interpretation:

- `treedb.public.checkpoint.*` is zero in every store. The old concern that
  the adapter was doing a full public checkpoint on every block is not true for
  this consumed path.
- `application.db` `WriteSync` is close to the app-level commit bucket, so
  improving TreeDB's per-block durable write barrier is still worthwhile.
- Full-stack TreeDB also pays substantial `WriteSync` and cache checkpoint work
  in CometBFT stores, especially `tx_index.db` and `blockstore.db`. Those costs
  are not necessarily all on the critical path, but they mean the current
  `treedb-all` comparison is not an app-state-only comparison.
- The largest TreeDB store work in these rows is in `tx_index.db`, not
  `application.db`. A follow-up should isolate `app-only` TreeDB from
  `all-DB` TreeDB before using this matrix to prioritize app-state engine work.

## CPU, Allocation, Lock, And Resource Signals

### Profile Summary

All accepted rows have 30s active-window CPU, alloc-space, heap, block, mutex,
goroutine, and trace artifacts.

| Workload | Backend | CPU samples in 30s | CPU utilization in profile | Alloc-space total | Block delay total | Mutex delay total |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 58.51s | 195% | 13,764 MB | 3,804s | 2,133s |
| Plain send | TreeDB | 39.69s | 132% | 14,427 MB | 1.68h | 2,294s |
| Small multisend | goleveldb | 59.74s | 199% | 15,333 MB | 3,913s | 2,106s |
| Small multisend | TreeDB | 42.47s | 142% | 15,915 MB | 6,015s | 2,102s |

The CPU profile does not show one dominant TreeDB-owned flat hotspot. The top
flat samples are mostly Go runtime/GC, crypto, syscall, memory movement, and
compression. TreeDB-owned cumulative CPU sites are present but modest:

| Workload | Example TreeDB CPU sites |
| --- | --- |
| Plain send | `zipper.mergeLeaf` about `2.01s` cumulative; `zipper.writeRecursive` about `2.31s`; `caching.writeRegularLocked` about `1.35s`; value-log frame prep about `1.06s`; lz4 compression about `1.05s` |
| Small multisend | `zipper.mergeLeaf` about `2.17s`; `zipper.applySpanNativeWithPrepared` about `2.12s`; `caching.writeRegularLocked` about `1.14s`; value-log block encoding about `1.00s`; lz4 compression about `0.96s` |

Allocation totals are close at macro scale:

| Workload | goleveldb alloc-space | TreeDB alloc-space | TreeDB delta |
| --- | ---: | ---: | ---: |
| Plain send | 13,764 MB | 14,427 MB | +4.8% |
| Small multisend | 15,333 MB | 15,915 MB | +3.8% |

TreeDB-owned alloc-space sites remain visible:

| Workload | TreeDB-owned allocation sites |
| --- | --- |
| Plain send | `memtable.getAppendOnlyEntries` 507 MB; command-WAL payload append 440 MB; `DecodeCommandFrame` 274 MB; command-WAL segment read 263 MB; zstd/lz4-related paths visible below top tier |
| Small multisend | `memtable.getAppendOnlyEntries` 534 MB; command-WAL payload append 406 MB; command-WAL segment read 288 MB; `DecodeCommandFrame` 279 MB; `Batch.SetWithRevision` 227 MB; zstd history 203 MB |

Block and mutex profiles are dominated by the same harness-facing path on both
backends:

- `github.com/cometbft/cometbft/mempool/cat.(*TxPool).CheckTx`
- `github.com/cometbft/cometbft/mempool/cat.(*TxPool).TryAddNewTx`
- `BroadcastTxSync`
- Cosmos SDK gRPC broadcast middleware

This does not prove the mempool is the root cause of the TreeDB gap, because
the profiles aggregate many blocked goroutines. It does show that the current
instrumentation cannot cleanly divide the remaining gap into app DB time,
node DB time, mempool admission, consensus idle/block timing, and load
generator backpressure.

### Resource Footprint

| Workload | Backend | Validator max CPU | Validator max RSS | Validator block read | Validator block write | `/simd/data` | `application.db` | `tx_index.db` |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 407% | 2.09 GiB | 5.47 MB | 33.7 GB | 1.21 GiB | 0.64 GiB | 0.38 GiB |
| Plain send | TreeDB | 333% | 6.99 GiB | 157 MB | 16.4 GB | 6.00 GiB | 2.07 GiB | 3.35 GiB |
| Small multisend | goleveldb | 387% | 2.13 GiB | 4.27 MB | 41.1 GB | 1.31 GiB | 0.56 GiB | 0.51 GiB |
| Small multisend | TreeDB | 341% | 6.53 GiB | 305 MB | 19.0 GB | 5.74 GiB | 1.85 GiB | 3.17 GiB |

TreeDB's short-run footprint is much larger, especially `tx_index.db`, while
reported block writes are lower than goleveldb. That makes a simple disk write
bandwidth explanation unlikely for this low-fanout run. The footprint may still
be a dwell/compaction/retention issue rather than an immediate TPS limiter.

## What We Know Now

The current best explanation is a combination:

1. TreeDB has higher per-block durable write-sync cost than goleveldb in this
   low-fanout regime.
2. Full-stack TreeDB amplifies this through CometBFT stores, especially
   `tx_index.db` and `blockstore.db`, not only `application.db`.
3. The explicit ABCI buckets do not explain the whole slowdown. The strongest
   example is `small-multisend`, where observed ABCI time is effectively tied
   but TreeDB's accepted window is much longer.
4. CPU profiles do not show a single TreeDB-owned hot function large enough to
   explain the gap.
5. Allocation profiles show real TreeDB waste, but the macro total delta is
   only a few percent, so broad allocation cleanup should not be expected to
   move TPS unless it is tied to CPU/GC or write-sync timing.

## What This Rules Out

- The consumed Ironbird path is not calling public TreeDB `Checkpoint()` per
  block.
- The low-fanout slowdown is not simply `FinalizeBlock` being slower under
  TreeDB.
- The low-fanout slowdown is not obviously raw write bandwidth saturation,
  because goleveldb reports substantially more validator block writes.
- Another blind allocation sprint is unlikely to be the highest-value next
  step unless the candidate is proven CPU/GC-visible in the accepted window.

## Remaining Gap

We still do not have a complete accounting of the TreeDB slowdown.

The largest instrumentation gap is wall-clock attribution outside explicit ABCI
method totals. The next graph should split the accepted load window into:

- transaction generation and broadcast wait time,
- CAT mempool admission and precheck time,
- CheckTx execution time,
- block proposal/consensus interval and idle time,
- FinalizeBlock execution time,
- Commit execution time,
- per-store `WriteSync` critical path versus background/overlapped work,
- tx index and blockstore time,
- load generator drain/backpressure time.

That is the missing map required before picking another optimization sprint.

## Suggested Next Graph

Create an instrumentation graph rather than an optimization graph:

1. Add active-window wall-time spans for broadcast/mempool admission, block
   production, consensus interval, ABCI calls, tx indexing, blockstore writes,
   and app commit.
2. Add mode isolation rows:
   - goleveldb-all,
   - treedb-app-only if the harness can wire it cleanly,
   - treedb-node-only if useful,
   - treedb-all.
3. Correlate TreeDB per-store `WriteSync` spans with app `Commit` and node
   store activity so we can tell what is critical path and what is background
   or overlapped.
4. Only then prioritize TreeDB code changes. Current candidates to re-rank are:
   command-WAL payload build/decode/read, append-only entry materialization,
   zstd/lz4 buffer churn, value-log read/write buffer paths, and tx-index-heavy
   full-stack behavior.

The output of that graph should be a quantitative timing budget for the
accepted low-fanout window, not another collection of independent pprof top
lists.

## Validation

Local gates before the matrix:

```sh
GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./messages
bash -n scripts/ironbird_lowfanout_attribution_sweep.sh
GOWORK=off go build -o /mnt/fast4tb/tmp/local-report-runner-attribution-626163f ./cmd/local-report-runner
```

Matrix gate:

```sh
OUT_ROOT=/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-626163f-20260707T153950Z \
  RUNNER=/mnt/fast4tb/tmp/local-report-runner-attribution-626163f \
  REBUILD_RUNNER=false \
  SKIP_BUILD=true \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  STOP_CATALYST_AFTER_LOAD_WINDOW=true \
  ACTIVE_WINDOW_PROFILE_DURATION=30s \
  TMPDIR=/mnt/fast4tb/tmp \
  scripts/ironbird_lowfanout_attribution_sweep.sh
```

All four required rows reached accepted load-window status. The profile
artifact set includes CPU, alloc-space, heap, block, mutex, goroutine, and trace
captures for the accepted rows.
