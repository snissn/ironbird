# Ironbird Event-Cadence Diagnostic Report: gomap 2182e84

Date: 2026-07-09 UTC / 2026-07-08 HST

This report closes `snissn/ironbird#35`. The important conclusion is narrow:
the preserved accepted Ironbird rows can quantify the low-fanout TreeDB cadence
gap more directly, but they still do not contain exact event-level wall-clock
spans for the DB adapter, command-WAL, value-log, blockstore, tx-index, local
broadcast, or CAT mempool lock phases. This PR therefore adds a first-class
cadence diagnostic section to local-report-runner output and names those missing
sources instead of treating bounded scrape intervals as exact attribution.

## Consumed Evidence

Artifact root:

```text
/mnt/fast4tb/ironbird-exact-nonabci-attribution-20260709T020911Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-exact-nonabci-attribution-20260709T020911Z/summary.tsv
```

The matrix was run with `LOAD_WINDOW_MIN=5m`,
`LOAD_WINDOW_TARGET_FRACTION=0.995`, `ACTIVE_WINDOW_PROFILE_DURATION=30s`,
`TREEDB_POST_LOAD_DWELL=5m`, and `BACKEND_ORDER_MODE=alternate`.

Accepted rows:

| Workload | Backend | Attempt | Blocks | Tx/block | Window s | Successful tx | Runtime TPS | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 1 | 400 | 500 | 335.03 | 199,194 | 594.56 | 253.18 |
| Plain send | goleveldb | 1 | 400 | 500 | 300.02 | 199,000 | 663.28 | 443.69 |
| Small multisend | goleveldb | 2 | 480 | 500 | 353.52 | 238,992 | 676.03 | 468.47 |
| Small multisend | TreeDB | 2 | 480 | 500 | 402.52 | 238,998 | 593.75 | 278.19 |

Rejected-but-preserved rows:

| Workload | Backend | Attempt | Reason |
| --- | --- | ---: | --- |
| Small multisend | goleveldb | 1 | Completed 119,998 successful tx in 180.52s, below the 5-minute acceptance floor |
| Small multisend | TreeDB | 1 | Completed 119,999 successful tx in 185.02s, below the 5-minute acceptance floor |

## Cadence Decomposition

The new runner diagnostic computes this per validator:

```text
residual = max(0, avg_block_interval_seconds -
  (commit + finalize_block + prepare_proposal + process_proposal seconds per block))
```

`check_tx` is reported separately as transaction-intake pressure. It is not
subtracted from the block-stage residual because it is not an exclusive
block-stage wall-time bucket.

| Workload | Backend | TPS | Block interval ms | Commit ms/block | Finalize ms/block | Prepare ms/block | Process ms/block | ABCI block-stage ms/block | CheckTx equiv ms/block | Residual ms/block | Residual % |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 663.28 | 437.48 | 31.91 | 50.91 | 1.33 | 0.42 | 84.57 | 115.02 | 352.92 | 80.67% |
| Plain send | TreeDB | 594.56 | 549.35 | 52.02 | 50.18 | 1.57 | 0.39 | 104.16 | 120.27 | 445.19 | 81.04% |
| Small multisend | goleveldb | 676.03 | 430.67 | 28.17 | 50.31 | 1.62 | 0.54 | 80.64 | 120.57 | 350.02 | 81.27% |
| Small multisend | TreeDB | 593.75 | 523.11 | 46.49 | 50.34 | 1.90 | 0.44 | 99.17 | 125.00 | 423.93 | 81.04% |

TreeDB deltas:

| Workload | TPS ratio | Cadence penalty | Commit penalty | ABCI block-stage penalty | Residual penalty | Commit / cadence | Residual / cadence |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | 0.90x | +111.87 ms/block | +20.11 ms/block | +19.60 ms/block | +92.28 ms/block | 18.0% | 82.5% |
| Small multisend | 0.88x | +92.44 ms/block | +18.32 ms/block | +18.53 ms/block | +73.91 ms/block | 19.8% | 80.0% |

This is the clearest quantitative shape we currently have: TreeDB commit is
materially slower, but it is not the whole low-fanout throughput gap. Most of
the per-block cadence delta remains outside the measured ABCI block-stage
counters.

## Active-Window Profile Diagnostic

The accepted rows captured 30-second active-window CPU, allocs, block, mutex,
goroutine, heap, and trace artifacts under each row's `pprof/` directory.

Block profile totals:

| Workload | Backend | Cumulative block delay | Top flat delay buckets |
| --- | --- | ---: | --- |
| Plain send | goleveldb | 3,967.39s | `sync.Mutex.Lock` 72.40%, `runtime.selectgo` 25.90%, `runtime.chanrecv2` 1.33% |
| Plain send | TreeDB | 5,894.40s | `runtime.selectgo` 46.93%, `sync.Mutex.Lock` 39.20%, `runtime.chanrecv2` 13.44% |
| Small multisend | goleveldb | 4,062.04s | `sync.Mutex.Lock` 72.47%, `runtime.selectgo` 25.81%, `runtime.chanrecv2` 1.31% |
| Small multisend | TreeDB | 5,450.05s | `runtime.selectgo` 47.64%, `sync.Mutex.Lock` 38.27%, `runtime.chanrecv2` 13.64% |

Mutex profile totals:

| Workload | Backend | Cumulative mutex wait | Dominant cumulative wait path |
| --- | --- | ---: | --- |
| Plain send | goleveldb | 2,196.72s | CAT `TxPool.CheckTx` / `TryAddNewTx` / local `BroadcastTxSync`: 90.08% |
| Plain send | TreeDB | 1,807.00s | CAT `TxPool.CheckTx` / `TryAddNewTx` / local `BroadcastTxSync`: 97.49% |
| Small multisend | goleveldb | 2,158.06s | CAT `TxPool.CheckTx` / `TryAddNewTx` / local `BroadcastTxSync`: 94.30% |
| Small multisend | TreeDB | 2,111.94s | CAT `TxPool.CheckTx` / `TryAddNewTx` / local `BroadcastTxSync`: 97.22% |

Interpretation:

- The active-window wait profiles are dominated by transaction-intake and local
  broadcast paths, not by a flat TreeDB CPU function.
- TreeDB rows have much more `runtime.selectgo` and `chanrecv2` delay in block
  profiles, which is consistent with wait/backpressure or sequencing rather
  than pure CPU saturation.
- The mutex-profile totals are not themselves a proof that CAT mempool is the
  root cause, because pprof delay profiles aggregate many goroutines. They do
  identify transaction intake as the highest-value place to add exact spans.

## Missing Event Sources

The runner now emits `cadence_diagnostics` with
`exact_event_span_coverage=false` and explicit missing span labels until these
sources exist:

| Missing source | Owner / likely implementation location | Why it matters |
| --- | --- | --- |
| DB adapter `WriteSync`, command-WAL sync, value-log append/sync, checkpoint/publish spans | `snissn/cosmos-db` TreeDB adapter plus `snissn/gomap` debug/metrics hooks | Needed to decide whether the commit penalty is DB sync, value-log write, checkpoint, publish, or adapter overhead |
| CAT mempool lock wait/hold and `preCheck` event spans | consumed `snissn/cometbft`/CometBFT CAT path | Block/mutex profiles point at `TxPool.CheckTx`, `TryAddNewTx`, and `BroadcastTxSync`, but sampled profiles do not give exact per-tx wall spans |
| Catalyst local `BroadcastTxSync` client wait | Catalyst/Ironbird load generator integration | Needed to split client-side backpressure from validator-side admission delay |
| Blockstore and tx-index write spans | consumed CometBFT node DB path | Needed to separate app commit from node DB write/index costs in full-stack backend comparisons |
| Exact block phase events for proposal, block assembly, state execution, and commit | CometBFT state/consensus instrumentation | Needed to convert the 74-92 ms/block residual delta into named exclusive phases |

## Overall Bottleneck Read

For low-fanout plain send and small multisend, TreeDB is slower primarily in
block cadence, not in total successful transaction accounting. Commit is a real
piece of that: about 18-20 ms/block slower than goleveldb. But the larger
unexplained piece is the 74-92 ms/block residual after measured ABCI block-stage
counters.

The best current diagnostic is therefore:

1. Do not keep treating commit as the whole problem.
2. Do not treat bounded scrape-window residuals as exact non-ABCI wall time.
3. Add exact spans around DB adapter commit internals and CAT/local-broadcast
   transaction admission before opening another optimization sprint.

This PR makes that diagnostic durable in future Ironbird JSON and markdown
reports so subsequent runs show the cadence residual and missing event-span
owners directly.
