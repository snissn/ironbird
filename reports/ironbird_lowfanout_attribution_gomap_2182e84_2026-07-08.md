# Ironbird Low-Fanout Attribution Report: gomap 2182e84

Date: 2026-07-08 UTC

This report closes the attribution-matrix node for `snissn/gomap#3625` /
`snissn/ironbird#24`. It reruns the low-fanout Ironbird matrix after the gomap
lifecycle-counter stack landed, pins Ironbird to the merged gomap head, and
uses the new active-window profiling, TreeDB counter timeline, non-ABCI
residual, and post-load dwell artifacts to explain the remaining low-fanout
TreeDB gap.

## Headline

TreeDB is still slower than goleveldb on these low-fanout workloads:

| Workload | goleveldb TPS | TreeDB TPS | TreeDB ratio |
| --- | ---: | ---: | ---: |
| Plain send | 687.25 | 584.74 | 0.85x |
| Small multisend | 683.61 | 590.72 | 0.86x |

The current evidence says the gap is only partly TreeDB commit/write-barrier
cost:

- TreeDB `Commit` is slower per block: `52.7 ms` vs `33.6 ms` on plain-send,
  and `45.6 ms` vs `28.3 ms` on small-multisend.
- If TreeDB commit were free, the runtime model gives at most `1.16x`
  plain-send speedup and `1.13x` small-multisend speedup. Cutting commit in
  half gives only `1.07x` and `1.06x`.
- TreeDB uses fewer validator CPU seconds than goleveldb during the accepted
  load windows (`~1.44` core-equivalent vs `~2.62-2.65`), so the result does
  not look like a simple TreeDB CPU-saturation bottleneck.
- Active-window block and mutex profiles are dominated by CometBFT CAT mempool
  `TxPool.CheckTx` / `BroadcastTxSync` lock and channel waits. That is the best
  current explanation for why non-ABCI residual time remains large, but the
  harness still lacks exact interval-based non-ABCI wall attribution.

## Consumed Code

| Repo | Evidence |
| --- | --- |
| `snissn/gomap` | `2182e84bd668f6ea610726717d90e09a86a17a32` |
| Ironbird gomap pin | `v0.6.2-0.20260708213404-2182e84bd668` |
| `snissn/ironbird` base | `935fa0def15a4aa1d439119fc1c7a6908d61ef0c` |
| Ironbird report branch | `codex/24-attribution-report` |
| Docker image | `ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-2182e84` |

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-2182e84-20260708T213616Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-2182e84-20260708T213616Z/summary.tsv
```

Command shape:

```sh
OUT_ROOT=/mnt/fast4tb/ironbird-lowfanout-attribution-gomap-2182e84-20260708T213616Z \
  REBUILD_RUNNER=true \
  SKIP_BUILD=false \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  ACTIVE_WINDOW_PROFILE_DURATION=30s \
  TREEDB_POST_LOAD_DWELL=5m \
  BACKEND_ORDER_MODE=alternate \
  TMPDIR=/mnt/fast4tb/tmp \
  scripts/ironbird_lowfanout_attribution_sweep.sh
```

`BACKEND_ORDER_MODE=alternate` was used to avoid always running TreeDB after
goleveldb. TreeDB rows also captured a 5-minute post-load dwell snapshot.

## Accepted Matrix

| Workload | Backend | Attempt | Blocks | Tx/block | Window s | Successful tx | Window TPS | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 1 | 400 | 500 | 340.52 | 199,117 | 584.74 | 215.39 |
| Plain send | goleveldb | 2 | 800 | 500 | 579.52 | 398,277 | 687.25 | 551.20 |
| Small multisend | goleveldb | 2 | 480 | 500 | 349.52 | 238,936 | 683.61 | 482.62 |
| Small multisend | TreeDB | 2 | 480 | 500 | 405.02 | 239,253 | 590.72 | 278.76 |

Rejected-but-preserved rows:

| Workload | Backend | Attempt | Reason |
| --- | --- | ---: | --- |
| Plain send | goleveldb | 1 | Completed `199,998` tx in `289.54s`, below the 5-minute acceptance floor |
| Small multisend | goleveldb | 1 | Completed `119,998` tx in `175.02s`, below the 5-minute acceptance floor |
| Small multisend | TreeDB | 1 | Completed `120,000` tx in `194.52s`, below the 5-minute acceptance floor |

## Timing Attribution

| Workload | Backend | Window s | CheckTx s | Finalize s | Commit s | Observed ABCI s | Non-ABCI approx s | Non-ABCI approx % |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 340.52 | 102.13 | 42.48 | 45.61 | 191.85 | 148.67 | 43.66% |
| Plain send | goleveldb | 579.52 | 216.75 | 94.29 | 61.33 | 375.59 | 203.94 | 35.19% |
| Small multisend | goleveldb | 349.52 | 135.94 | 58.68 | 31.17 | 228.33 | 121.20 | 34.67% |
| Small multisend | TreeDB | 405.02 | 127.49 | 51.12 | 47.44 | 228.51 | 176.51 | 43.58% |

Per-block timing:

| Workload | Backend | Commit count | Avg commit ms | Finalize count | Avg finalize ms | CheckTx count | Avg CheckTx ms | Avg block interval ms | Avg tx/commit |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 866 | 52.67 | 865 | 49.11 | 405,440 | 0.252 | 559.70 | 229.93 |
| Plain send | goleveldb | 1,823 | 33.65 | 1,823 | 51.72 | 802,466 | 0.270 | 391.77 | 218.47 |
| Small multisend | goleveldb | 1,102 | 28.28 | 1,102 | 53.25 | 481,294 | 0.282 | 441.49 | 216.82 |
| Small multisend | TreeDB | 1,040 | 45.61 | 1,039 | 49.21 | 485,800 | 0.262 | 522.91 | 230.05 |

Interpretation:

- On small-multisend, observed ABCI totals are effectively equal
  (`228.33s` goleveldb vs `228.51s` TreeDB) while the TreeDB load window is
  `55.50s` longer. This is the clearest evidence that the remaining slowdown is
  not explained by explicit ABCI method totals alone.
- TreeDB commit is worse per block, but goleveldb performs more blocks in the
  accepted rows. Commit remains worth optimizing, yet the larger signal is
  slower TreeDB block cadence and larger non-ABCI residual.
- The non-ABCI number is a residual, not a precise busy-time union. The harness
  still reports the missing reason as: `ABCI method interval start/end samples
  are not exported by this harness`.

## Resource And Footprint

| Workload | Backend | Validator CPU s | Core-equivalent | Max CPU % | Max RSS | Block writes | `application.db` bytes | `/simd/data` bytes |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | TreeDB | 491.82 | 1.44 | 330.65 | 7.143 GiB | 16.5 GB | 2,248,646,206 | 6,485,776,565 |
| Plain send | goleveldb | 1,517.59 | 2.62 | 450.08 | 2.186 GiB | 70.2 GB | 1,343,986,788 | 2,598,085,650 |
| Small multisend | goleveldb | 926.05 | 2.65 | 448.90 | 2.091 GiB | 40.7 GB | 602,419,316 | 1,418,467,748 |
| Small multisend | TreeDB | 581.91 | 1.44 | 323.56 | 5.580 GiB | 18.0 GB | 1,917,885,541 | 6,176,538,350 |

TreeDB writes fewer block-device bytes in these rows, but has a much larger
directory footprint and RSS. The footprint is plausibly a dwell/compaction
policy difference rather than the immediate TPS bottleneck, because the CPU and
wait profiles point elsewhere.

## TreeDB Lifecycle Counters

Active load-window TreeDB counter deltas:

| Metric | Plain TreeDB | Small TreeDB |
| --- | ---: | ---: |
| Cache checkpoint runs | 16 | 20 |
| Cache checkpoint total | 3,728.70 ms | 4,359.98 ms |
| Cache checkpoint max | 399.93 ms | 359.42 ms |
| Checkpoint `flush_all` | 1.75 s | 2.10 s |
| Checkpoint command-WAL publish | 40.33 ms | 53.33 ms |
| Checkpoint reducer publish | 140.13 ms | 182.24 ms |
| Checkpoint cutover | 213.00 ms | 269.60 ms |
| Leaf value-log sync | 3.34 ms | 0.27 ms |
| Command-WAL checkpoint publish, piggybacked | 9 | 12 |
| Command-WAL checkpoint publish, separate | 5 | 6 |
| Flush apply entries | 10,845,204 | 9,481,395 |
| Flush apply bytes | 960.28 MB | 851.71 MB |
| Flush apply backend write | 18.31 s | 16.78 s |
| Flush apply backend batch write | 15.14 s | 13.83 s |
| Flush apply leaf-log append | 12.02 s | 8.75 s |
| Flush apply compression | 6.67 s | 5.49 s |
| Flush apply value-log sync | 31.27 ms | 49.50 ms |
| Vlog GC runs | 2 | 2 |
| Vlog GC deleted bytes | 0 | 0 |
| Live value-log bytes | 2.18 GB | 1.89 GB |
| Stale value-log bytes | 0 | 0 |
| Batch arena key/value copied | 541.13 MB | 809.73 MB |
| Batch arena ptr-value copied | 423.69 MB | 632.85 MB |
| Writer append buffer allocated | 264.24 MB | 322.96 MB |
| Writer append buffer dropped | 188.74 MB | 247.46 MB |

Post-load 5-minute dwell:

| Workload | Snapshot | Dwell s | Checkpoint runs | Checkpoint total ms | GC runs | GC deleted bytes | BG vacuum runs | BG vacuums | `/simd/data` bytes | `application.db` bytes |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | load_end | 0.00 | 16 | 3,728.70 | 2 | 0 | 11 | 2 | 6,485,776,565 | 2,248,646,206 |
| Plain send | post_dwell | 300.01 | 29 | 6,101.65 | 4 | 0 | 21 | 2 | 6,503,688,780 | 2,228,434,856 |
| Small multisend | load_end | 0.00 | 21 | 4,498.33 | 2 | 0 | 13 | 2 | 6,176,538,350 | 1,917,885,541 |
| Small multisend | post_dwell | 300.01 | 32 | 6,045.15 | 3 | 0 | 23 | 2 | 6,252,641,374 | 1,943,409,618 |

Interpretation:

- Command-WAL publish and value-log sync are not the dominant measured TreeDB
  lifecycle costs in these rows.
- Cache checkpoint total is only a few seconds across the load window. That is
  much smaller than the total TPS gap.
- Backend flush/apply, leaf-log append, and compression are larger internal
  TreeDB cost centers, but they are still not large enough by themselves to
  explain the whole low-fanout gap.
- The dwell snapshots show continued checkpoint/background maintenance, but no
  value-log GC deletion. That supports treating compaction/dwell as a footprint
  issue before treating it as the primary throughput bottleneck.

## Active-Window Profiles

CPU profile totals for 30-second active windows:

| Workload | Backend | CPU samples | Approx profile CPU |
| --- | --- | ---: | ---: |
| Plain send | TreeDB | 41.69s | 139% |
| Plain send | goleveldb | 66.78s | 223% |
| Small multisend | TreeDB | 40.61s | 135% |
| Small multisend | goleveldb | 61.86s | 206% |

TreeDB active CPU profiles do not show one dominant TreeDB-owned flat hotspot.
The top flat samples are Go runtime/GC, secp256k1 crypto, syscalls,
`runtime.memmove`, and lz4 compression. TreeDB-owned allocation sites remain
visible:

| Workload | TreeDB alloc-space examples |
| --- | --- |
| Plain send | `memtable.getAppendOnlyEntries` 535.73 MB; command-WAL payload builder 463.01 MB; command-WAL decode/read 575.39 MB combined; zstd history 179.30 MB |
| Small multisend | `memtable.getAppendOnlyEntries` 536.28 MB; command-WAL payload builder 407.98 MB; command-WAL decode/read 560.60 MB combined; `Batch.SetWithRevision` 229.39 MB; zstd history 210.91 MB |

Block and mutex profiles are more revealing for the end-to-end gap:

- Plain TreeDB block delay over the active profile was `5,695.92s`, dominated
  by `runtime.selectgo` (`45.37%`), `sync.(*Mutex).Lock` (`40.30%`), and
  `runtime.chanrecv2` (`13.89%`).
- Small TreeDB block delay was `5,751.75s`, with the same shape:
  `runtime.selectgo` (`45.09%`), `sync.(*Mutex).Lock` (`39.78%`), and
  `runtime.chanrecv2` (`14.70%`).
- The TreeDB mutex profiles attribute almost all sampled mutex delay to
  `sync.(*Mutex).Unlock`, with cumulative stacks dominated by CometBFT
  `mempool/cat.(*TxPool).CheckTx`, `TryAddNewTx`, and local
  `BroadcastTxSync`.

That means the current bottleneck evidence is no longer "TreeDB has an obvious
CPU hotspot." It is "TreeDB is slower in the full low-fanout system while the
hot wait stacks are in the CometBFT transaction intake path and the harness
does not yet split that non-ABCI residual into exact sub-buckets."

## Conclusions

1. The benchmark result is real for this matrix: TreeDB is `13-15%` slower than
   goleveldb by accepted load-window TPS on both low-fanout workloads.
2. TreeDB commit/write-barrier cost is a real, quantified contributor. It is
   not enough to explain the whole gap.
3. The larger unresolved gap is non-ABCI residual time and slower block cadence.
   Current profiles point at CometBFT CAT mempool / local broadcast lock and
   channel waits, but the harness still cannot say how much of that is client
   pacing, mempool admission, consensus cadence, DB callback latency, or
   scheduler interaction.
4. TreeDB allocation cleanup remains reasonable hygiene, especially around
   append-only entry materialization and command-WAL payload materialization,
   but this run does not justify treating those allocations as the primary TPS
   limiter.
5. The next useful sprint should instrument exact non-ABCI intervals: loadgen
   send/wait latency, mempool `CheckTx` lock hold/wait, block assembly cadence,
   consensus proposal/commit cadence, and DB callback spans around app commit
   and CometBFT store writes.
