# Ironbird sustained simapp allocation profile - 2026-07-03

## Scope

This rerun used the same large simapp workload for goleveldb and TreeDB, with
validator heap allocation capture added to `cmd/local-report-runner`. The goal
was to test whether the previously observed overhead was mostly allocation
pressure and, if so, whether the hot allocations were TreeDB-specific.

Artifacts:

- `reports/artifacts/alloc-throughput-20260703T204313Z/simapp-goleveldb-alloc.json`
- `reports/artifacts/alloc-throughput-20260703T204313Z/simapp-treedb-alloc.json`
- `reports/artifacts/alloc-throughput-20260703T204313Z/pprof/`

The workload:

- `validators=1`, `nodes=0`, `wallets=5000`
- preseeded app state: `100000` inactive accounts plus `5000` active wallets
- 20 blocks x 250 tx/block = 5000 tx
- each tx: `MsgArr` containing 20 `MsgMultiSend` messages
- each `MsgMultiSend`: 25 recipients
- effective operation count: 2,500,000 recipient operations
- `-load-window-min-duration=2m`
- `-load-window-drain-timeout=15m`
- `-raw-tx-audit=false`

Both runs reused image
`ironbird-report:snissn-sdk-28e5525f-cosmosdb-f1d8b01-gomap-1afe86c`.

## Validity

Both variants met the app-observed two-minute window requirement:

| Variant | App window reached | Window seconds | Successful app txs |
| --- | ---: | ---: | ---: |
| goleveldb | yes | 233.0 | 5000 |
| TreeDB | yes | 529.0 | 5000 |

The Catalyst JSON still reports zero successful txs and 5000 failed txs for
both variants, while app metrics report 5000 successful txs. For this report,
the accepted source is the app-metric load window, not Catalyst's ambiguous
success counter.

## Throughput

| Metric | goleveldb | TreeDB | Read |
| --- | ---: | ---: | --- |
| Wall seconds | 425.2 | 728.6 | TreeDB wall is slower |
| Launch seconds | 179.5 | 196.1 | similar setup cost |
| Catalyst load phase seconds | 101.4 | 102.1 | matched send duration |
| App load-window drain seconds | 131.6 | 426.9 | TreeDB needed much longer to observe all txs |
| Runtime included TPS | 76.1 | 75.7 | Catalyst-runtime view is essentially tied |
| Load-phase wall TPS | 49.3 | 49.0 | send phase is essentially tied |
| Accepted-window TPS | 21.5 | 9.5 | TreeDB is slower once inclusion/drain is counted |
| Accepted-window effective ops/s | 10728 | 4726 | same conclusion as TPS |

Interpretation: the runner can send the workload at the same rate to both
variants, but the TreeDB run takes much longer before app metrics show the full
5000 successful txs. That is the main end-to-end regression signal in this run.

## ABCI timing

| Metric | goleveldb | TreeDB |
| --- | ---: | ---: |
| Observed ABCI seconds | 104.4 | 145.8 |
| Commit seconds | 4.38 | 8.47 |
| Avg commit | 24.6 ms | 77.7 ms |
| FinalizeBlock seconds | 58.2 | 48.5 |
| Avg FinalizeBlock | 325 ms | 441 ms |
| CheckTx seconds | 25.7 | 32.1 |
| PrepareProposal seconds | 7.9 | 34.5 |
| Query seconds | 8.1 | 22.2 |

Commit is slower for TreeDB, but it is not large enough by itself to explain
the 529s accepted load window. TreeDB commit is 8.5s total, or about 1.6% of
the accepted TreeDB window. The larger issue is that a lot of the accepted
window is not explained by measured ABCI buckets, and the TreeDB run has much
larger post-load drain.

## Resource and storage signal

| Metric | goleveldb | TreeDB |
| --- | ---: | ---: |
| Validator max RSS sample | 7.249 GiB | 8.062 GiB |
| Validator max CPU sample | 602.5% | 682.5% |
| Validator max block read | 55.3 MB | 4.57 GB |
| Validator max block write | 2.87 GB | 11.8 GB |
| Data dir bytes delta | 1.00 GB | 1.64 GB |
| Application DB bytes delta | 74.0 MB | 94.0 MB |

The resource data says the next bottleneck is likely not "heap allocations
only". TreeDB has much higher validator block I/O and a longer post-load
inclusion window.

## Allocation profiles

Heap profiles were captured from the validator pprof endpoint with `heap?gc=1`.
This is process-cumulative allocation since start, not a perfect
load-window-only profile. It is still useful because both validators were
freshly started for the run.

Generated pprof summaries:

- `simapp-goleveldb_alloc_space_top.txt`
- `simapp-goleveldb_alloc_space_cum.txt`
- `simapp-goleveldb_inuse_space_top.txt`
- `simapp-goleveldb_inuse_space_cum.txt`
- `simapp-treedb_alloc_space_top.txt`
- `simapp-treedb_alloc_space_cum.txt`
- `simapp-treedb_inuse_space_top.txt`
- `simapp-treedb_inuse_space_cum.txt`

### Cumulative allocation (`alloc_space`)

| Focus | goleveldb | TreeDB |
| --- | ---: | ---: |
| Total alloc_space | 698,380 MB | 850,811 MB |
| CometBFT txindex focus | 526,075 MB, 75.3% | 664,422 MB, 78.1% |
| RPC/http focus | 98,211 MB, 14.1% | 92,715 MB, 10.9% |
| SDK/baseapp focus | 36,552 MB, 5.2% | 39,423 MB, 4.6% |
| TreeDB focus | 1 MB | 2,726 MB, 0.32% |

Top flat allocation in both variants:

- goleveldb: `github.com/syndtr/goleveldb/leveldb.(*Batch).grow`, 522,531 MB, 74.8%
- TreeDB: `github.com/syndtr/goleveldb/leveldb.(*Batch).grow`, 661,915 MB, 77.8%

This is the key allocation finding: even in the TreeDB app-state run, most
cumulative allocation is LevelDB batch growth under CometBFT tx indexing, not
TreeDB app-state work.

### Live heap (`inuse_space`)

| Focus | goleveldb | TreeDB |
| --- | ---: | ---: |
| Total inuse_space | 4,118 MB | 2,010 MB |
| CometBFT txindex focus | 1,409 MB, 34.2% | 19 MB, 0.95% |
| TreeDB focus | 1 MB | 236 MB, 11.7% |

TreeDB's live heap profile is lower at capture time despite the higher Docker
RSS high-water sample. Its TreeDB-specific live heap is visible but not dominant.

## CPU profile caveat

The TreeDB CPU profile was non-empty:

- `simapp-treedb-validator-0-cpu.pprof`
- duration: 537.9s
- total samples: 879.5s

The goleveldb CPU profile was copied but empty, so this run should not be used
for a paired CPU comparison.

TreeDB CPU focused summaries:

| Focus | TreeDB CPU samples |
| --- | ---: |
| GC/runtime cumulative top (`runtime.gcDrain`) | 401.5s, 45.7% |
| LevelDB/cometbft-db focus | 200.8s, 22.8% |
| CometBFT txindex focus | 133.5s, 15.2% |
| TreeDB focus | 5.5s, 0.63% |

The non-empty CPU profile agrees with the allocation profiles: the prominent
CPU cost is GC plus CometBFT/LevelDB paths. TreeDB-specific CPU is small in
this workload.

## Takeaways

1. We now have allocation profiles for both variants.
2. The allocation bottleneck in this workload is mostly not TreeDB. It is
   CometBFT tx indexing backed by goleveldb batch growth.
3. TreeDB still loses the accepted-window throughput in this run, but the
   evidence points more toward block I/O, post-load inclusion/drain, and
   under-instrumented non-ABCI time than toward TreeDB allocator CPU.
4. Commit is slower in TreeDB, but total commit time is too small to be the
   primary end-to-end explanation.
5. The current benchmark is still noisy for "TreeDB app-state vs goleveldb
   app-state" because a large LevelDB-backed CometBFT subsystem dominates
   allocation in both variants.

## Suggested next run

Run one or both of these before spending more time optimizing TreeDB internals:

1. Disable or neutralize CometBFT tx indexing for the app-state comparison
   (`tx_index.indexer = "null"` if supported in this harness). This should
   remove the dominant LevelDB batch allocation and tell us whether app-state
   backend differences become visible.
2. Add validator block/mutex/trace or endpoint-based CPU capture for both
   variants. The TreeDB run has much higher block I/O and much longer drain,
   so blocking/I/O evidence is now more important than another heap-only rerun.

Also fix CPU capture to avoid zero-byte control profiles. A pprof HTTP CPU
capture (`/debug/pprof/profile?seconds=N`) may be more robust than relying on
the simapp `--cpu-profile` file being flushed on Docker stop.
