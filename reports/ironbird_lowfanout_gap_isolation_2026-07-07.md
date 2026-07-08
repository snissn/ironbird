# Ironbird Low-Fanout TreeDB Gap Isolation Report

Date: 2026-07-07 HST

This report closes `snissn/gomap#3606` under parent tracker
`snissn/gomap#3603`.

The purpose of this graph was to answer a narrower question than the previous
low-fanout attribution report: is the observed low-fanout TreeDB slowdown an
app-state TreeDB engine problem, a full-stack "all DBs are TreeDB" problem, or
mostly benchmark/harness scheduling noise?

## Headline

This matrix does **not** reproduce a stable low-fanout TreeDB slowdown as an
app-state DB effect.

Accepted load-window results:

| Workload | Mode | goleveldb TPS | TreeDB TPS | TreeDB ratio | TreeDB delta |
| --- | --- | ---: | ---: | ---: | ---: |
| Plain send | app-only | 569.09 | 606.26 | 1.065x | +37.17 TPS |
| Plain send | all-DB | 589.27 | 563.92 | 0.957x | -25.35 TPS |
| Small multisend | app-only | 592.27 | 591.53 | 0.999x | -0.74 TPS |
| Small multisend | all-DB | 547.49 | 587.19 | 1.073x | +39.70 TPS |

The strongest conclusions are:

- TreeDB app-state alone is not the low-fanout culprit in this run. It wins
  plain-send app-only and is effectively tied for small-multisend app-only.
- Full-stack TreeDB is mixed, not uniformly slower. It loses plain-send all-DB
  by about `4.3%` and wins small-multisend all-DB by about `7.3%`.
- TreeDB commit/WAL/cache work is real and consistently more expensive than
  goleveldb commit in these rows, but it does not predict the TPS direction.
- Active-window CPU profiles do not show a dominant TreeDB-owned CPU hotspot;
  TreeDB rows often use fewer CPU samples than goleveldb rows.
- Allocation overhead exists in TreeDB paths, especially all-DB rows, but the
  total alloc-space deltas are small versus the overall app/harness allocation
  surface and are not enough to explain the TPS sign.
- Block and mutex profiles are dominated by CometBFT/Catalyst/mempool and
  consensus scheduling paths, not a clear TreeDB-specific lock.

The right next step is not another blind TreeDB optimization sprint from this
data. If we keep chasing the remaining mixed all-DB behavior, the next graph
should instrument CometBFT transaction pipeline timing and variance, while
promoting the TreeDB command-WAL/cache counters into first-class sweep columns.

## Consumed Code

The benchmark used the merged Ironbird instrumentation stack:

| Repo | Evidence |
| --- | --- |
| `snissn/ironbird` | `snissn/ironbird#19`, merged as `74b6f994363898673ef25c9bc9b94a97693ac88b` |
| `snissn/ironbird` | `snissn/ironbird#20`, merged as `51141497921b4c04dcf69d232ce014b9b3a2c02e` on `origin/main` |
| Sweep branch commit | `7df8fbe4e7d704146b801ab9a7c9c716502e3e8e`, ancestor of `origin/main` |
| Runner image suffix | `snissn-sdk-4948247-cosmosdb-6ddcb75-gomap-626163f` |

The runner branch commit was started as the PR branch head and is now contained
in `origin/main`.

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-lowfanout-gap-7df8fbe-20260707T200705Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-lowfanout-gap-7df8fbe-20260707T200705Z/summary.tsv
```

Command shape:

```sh
OUT_ROOT=/mnt/fast4tb/ironbird-lowfanout-gap-7df8fbe-20260707T200705Z \
  RUNNER=/mnt/fast4tb/tmp/local-report-runner-lowfanout-gap-7df8fbe \
  REBUILD_RUNNER=true \
  TMPDIR=/mnt/fast4tb/tmp \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  STOP_CATALYST_AFTER_LOAD_WINDOW=true \
  ACTIVE_WINDOW_PROFILE_DURATION=30s \
  MAX_ATTEMPTS=5 \
  VALIDATORS=1 \
  NODES=0 \
  WALLETS=5000 \
  PRESEED_ACCOUNTS=100000 \
  SKIP_BUILD=false \
  scripts/ironbird_lowfanout_gap_isolation_sweep.sh
```

The sweep preserves rejected rows. The first `small-multisend` attempt for
each scenario was rejected because the load window was too short, then rerun
with enough blocks to pass the acceptance gate.

## Accepted Matrix

| Workload | Scenario | Mode | Attempt | Blocks | Tx/block | Load-window s | Successful tx | Runtime TPS | Wall TPS |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | `simapp-goleveldb` | app-only | 1 | 400 | 500 | 350.53 | 199,482 | 569.09 | 296.81 |
| Plain send | `simapp-treedb` | app-only | 1 | 400 | 500 | 328.53 | 199,173 | 606.26 | 318.89 |
| Plain send | `simapp-goleveldb-all` | all-DB | 1 | 400 | 500 | 338.53 | 199,487 | 589.27 | 402.29 |
| Plain send | `simapp-treedb-all` | all-DB | 1 | 400 | 500 | 353.52 | 199,360 | 563.92 | 337.77 |
| Small multisend | `simapp-goleveldb` | app-only | 2 | 480 | 500 | 403.53 | 238,997 | 592.27 | 423.99 |
| Small multisend | `simapp-treedb` | app-only | 2 | 480 | 500 | 404.03 | 238,996 | 591.53 | 423.26 |
| Small multisend | `simapp-goleveldb-all` | all-DB | 2 | 480 | 500 | 436.53 | 238,992 | 547.49 | 400.68 |
| Small multisend | `simapp-treedb-all` | all-DB | 2 | 480 | 500 | 407.02 | 238,999 | 587.19 | 425.16 |

## Commit Timing

TreeDB commit time is consistently higher, but it does not determine the TPS
winner in this matrix.

| Workload | Mode | goleveldb commit s | TreeDB commit s | Commit delta | goleveldb avg ms | TreeDB avg ms |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Plain send | app-only | 39.23 | 57.72 | +18.49 | 37.90 | 50.76 |
| Plain send | all-DB | 37.25 | 49.92 | +12.67 | 35.89 | 53.33 |
| Small multisend | app-only | 37.59 | 62.76 | +25.17 | 31.15 | 47.22 |
| Small multisend | all-DB | 39.75 | 49.54 | +9.79 | 31.32 | 48.24 |

Interpretation:

- Plain-send app-only is the key sanity check: TreeDB pays `+18.49s` more
  commit time but still wins load-window TPS.
- Small-multisend all-DB is the same pattern: TreeDB pays `+9.79s` more commit
  time but wins by `+39.70 TPS`.
- Commit is a real optimization target, but this data does not support treating
  commit as the dominant low-fanout throughput limiter.

## Phase Overlap

The phase-overlap instrumentation shows that the accepted load window is
almost exactly the `run_load_test` phase. For example, plain-send TreeDB
app-only:

| Phase | Class | Phase s | In-window s | After s |
| --- | --- | ---: | ---: | ---: |
| `run_load_test` | `crosses_window_end` | 331.42 | 328.53 | 2.89 |
| `load_window_drain` | `after_window` | 0.00 | 0.00 | 0.00 |
| `collect_active_window_app_profiles` | `after_window` | 0.00 | 0.00 | 0.00 |
| `collect_after_metrics` | `after_window` | 0.03 | 0.00 | 0.03 |
| `collect_after_treedb_debug_vars` | `after_window` | 0.07 | 0.00 | 0.07 |

This rules out a simple accounting bug where substantial post-load collection
work was being counted as accepted-window runtime.

## TreeDB Counters

Counter caveat: the preserved run artifact's `treedb_write_sync_count` and
`treedb_checkpoint_count` columns are zero because the sweep script looked for
`treedb.public.*.count_total`, while the runner emitted `calls_total` keys. Do
not use those preserved summary columns. The preserved JSON contains the public
counters, and the sweep script now reads `calls_total` for future rows. It also
emits `NA` if the public counter keys are absent, while preserving numeric zero
for present counters that actually measure zero.

Recomputed public counter deltas from the preserved result JSON:

| Workload | Mode | Public WriteSync calls | Public Checkpoint calls | Public Write calls |
| --- | --- | ---: | ---: | ---: |
| Plain send | app-only | 1,137 | 0 | 28,894 |
| Plain send | all-DB | 5,613 | 0 | 25,255 |
| Small multisend | app-only | 1,329 | 0 | 30,922 |
| Small multisend | all-DB | 6,161 | 0 | 25,440 |

The public checkpoint conclusion is therefore still valid for this run:
`Checkpoint()` calls are measured zero. The public `WriteSync` summary column in
the old `summary.tsv` is not valid, and should be read from the recomputed
table above or from the raw result JSON.

Aggregate TreeDB debug counter deltas:

| Workload | Mode | WAL appends | WAL syncs | WAL append s | WAL sync s | Cache checkpoint runs | Cache checkpoint s | Wait-for-checkpoint s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | app-only | 30,031 | 1,151 | 0.72 | 10.36 | 15 | 3.46 | 2.47 |
| Plain send | all-DB | 31,896 | 6,593 | 2.86 | 52.45 | 903 | 213.00 | 8.01 |
| Small multisend | app-only | 32,251 | 1,345 | 0.66 | 12.65 | 30 | 4.25 | 2.65 |
| Small multisend | all-DB | 32,658 | 7,236 | 3.51 | 65.44 | 1,072 | 289.12 | 9.86 |

Interpretation:

- App-only TreeDB has modest command-WAL sync cost relative to total load
  window. It is visible, but not enough to explain an app-state throughput gap.
- All-DB mode substantially increases WAL sync count and cache checkpoint work.
  That is expected because CometBFT stores are also TreeDB-backed.
- The cache checkpoint totals are large in all-DB mode, but they do not map
  directly onto critical-path load-window time. They are still worth surfacing
  in future reports because they can explain footprint, background I/O, and
  contention effects.

## Active-Window Profiles

Each accepted row captured 30-second active-window CPU, alloc-space, heap,
block, mutex, goroutine, and trace artifacts under the row's `pprof/`
directory.

| Workload | Mode | Backend | CPU samples | CPU utilization | Alloc-space total | Engine flat alloc |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| Plain send | app-only | goleveldb | 63.61s | 212% | 13,734.88 MB | 2,547.6 MB leveldb |
| Plain send | app-only | TreeDB | 49.83s | 166% | 13,565.72 MB | 2,039.6 MB TreeDB |
| Plain send | all-DB | goleveldb | 62.88s | 210% | 14,227.86 MB | 2,574.6 MB leveldb |
| Plain send | all-DB | TreeDB | 47.37s | 158% | 14,374.11 MB | 3,045.6 MB TreeDB |
| Small multisend | app-only | goleveldb | 63.43s | 211% | 14,021.76 MB | 2,554.1 MB leveldb |
| Small multisend | app-only | TreeDB | 52.27s | 174% | 13,916.39 MB | 1,765.9 MB TreeDB |
| Small multisend | all-DB | goleveldb | 59.30s | 198% | 13,666.04 MB | 2,486.2 MB leveldb |
| Small multisend | all-DB | TreeDB | 36.08s | 120% | 14,162.99 MB | 3,041.0 MB TreeDB |

The profile evidence does not show "TreeDB is burning more CPU in the active
window" as the explanation:

- TreeDB has fewer CPU samples than goleveldb in all four pairings.
- The top flat CPU sites are mostly Go runtime/GC, crypto, syscall, memory
  movement, compression, and app/harness code.
- TreeDB-owned CPU is visible but not dominant in the 30-second captures.
- TreeDB allocation remains worth improving in all-DB mode, but app-only
  TreeDB alloc-space is slightly lower than app-only goleveldb in both
  workloads.
- Block profiles are dominated by `sync.(*Mutex).Lock`,
  `runtime.selectgo`, `runtime.chanrecv2`, CometBFT CAT mempool paths, and RPC
  broadcast paths. Mutex profiles are dominated by consensus-state paths. There
  is no clean TreeDB lock bottleneck in this capture.

## Decision

The gap is significantly mapped enough to stop the current TreeDB-engine
optimization loop:

1. The previously assumed low-fanout slowdown is not stable in this isolation
   matrix.
2. App-state TreeDB does not lose in the accepted app-only rows.
3. Full-stack TreeDB behavior is mixed and likely sensitive to CometBFT
   tx-index/blockstore/state-store scheduling, mempool pressure, and run
   variance.
4. Commit/WAL/cache work is a real TreeDB cost, especially in all-DB mode, but
   it is not sufficient to explain overall throughput direction.
5. CPU, alloc, block, and mutex profiles do not justify launching a new TreeDB
   hot-path optimization PR directly from this evidence.

Recommended follow-on work, if we continue:

1. Add CometBFT/Catalyst transaction pipeline attribution: broadcast submit
   latency, CheckTx admission, mempool wait, block inclusion delay, consensus
   interval, and tx-index write timing.
2. Promote command-WAL/cache counters into first-class sweep summary columns,
   or restore the `treedb.public.*` debugvar keys in the consumed image.
3. Run a replicate-only matrix for the mixed rows, especially plain-send
   all-DB, before treating the `4.3%` loss as a stable regression.
4. Only start a TreeDB optimization sprint when a candidate is visible in
   critical-path CPU, contention, or first-class timing counters, not just
   alloc-space.
