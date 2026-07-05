# Ironbird Matrix After TreeDB Low-Fanout Allocation Sprint

Date: 2026-07-05 UTC

## Executive Summary

This report is the M4 closeout for the `snissn/gomap` low-fanout allocation
sprint under parent tracker `snissn/gomap#3475` and final validation issue
`snissn/gomap#3498`.

The sprint landed two focused TreeDB allocation improvements after one measured
no-go:

| Milestone | State | Result |
| --- | --- | --- |
| M0 / `#3494` | PR `#3499` merged at `009af3d6887e2bafca51ede275bce1372f8ca390` | Added current-main repro/profile benchmarks for the low-fanout allocation path. |
| M1 / `#3495` | Closed with no PR | Measured no-go: the candidate append-only materialization allocation was not a steady TPS-window target. |
| M2 / `#3496` | PR `#3500` merged at `0e4ed74a1e0cd3b3ca803cd3cbf9a539446a8554` | Borrow stable hash-sorted batch values; intended pprof hot site moved from about `130 MB` to about `7 MB` alloc-space. |
| M3 / `#3497` | PR `#3501` merged at `cabb7a17fb8809aebcd33f6c007907cf0caddcc7` | Write command-WAL scan payloads in place; scan payload writer allocation dropped about `20001 KiB` to `2813 KiB`, and the pointer-forced benchmark improved from `1125908 ns/op, 282 allocs/op` to about `858861 ns/op, 215 allocs/op`. |
| M4 / `#3498` | This report | Full Ironbird matrix using gomap `cabb7a17fb88`. |

The full Ironbird result is mixed and should not be overclaimed. The focused
gomap work reduced real allocation sites, but the macro low-fanout workloads
still favor goleveldb in the accepted load window:

| Workload | TreeDB load-window TPS vs goleveldb | TreeDB effective ops/s vs goleveldb | Result |
| --- | ---: | ---: | --- |
| Plain send | 0.84x | 0.84x | goleveldb wins |
| Small multisend | 0.89x | 0.89x | goleveldb wins |
| Moderate multisend | 1.15x | 1.15x | TreeDB wins |
| High fanout anchor | 1.91x | 1.91x | TreeDB large win |

The main current conclusion is:

- The allocation sprint produced useful focused TreeDB improvements.
- It did not make low-fanout Ironbird plain/small throughput TreeDB-favorable.
- TreeDB remains favored as write fanout grows.
- Commit is measurable but not the whole limiter: making commit free would cap
  TreeDB speedup at about `1.15x` plain, `1.14x` small, `1.12x` moderate, and
  `1.03x` high fanout in this run.
- The next useful work is not another blind low-level allocation PR. It is
  better timed-window instrumentation that separates steady TPS-window DB work
  from setup/cleanup/replay/non-ABCI harness work, plus targeted investigation
  of the remaining TreeDB-specific allocation paths that still appear during
  accepted rows.

## Scope And Caveats

This report is fork-local to `snissn/ironbird` and `snissn/gomap`. It does not
open or claim readiness for public upstream PRs.

All accepted rows used a sustained load window. The sweep script reran rows that
were too short, including longer attempts for small and moderate multisend, so
the accepted rows are suitable for directional TPS comparison.

Ironbird macro profiles report pprof `alloc_space`; they do not report Go
benchmark `B/op` or `allocs/op`. Focused gomap PR evidence includes `B/op` and
`allocs/op`, but the macro tables below use pprof allocation totals and top
sites.

Ironbird does not currently export TreeDB append-only direct arena, reserve
pool, command-WAL payload, or value-log writer counters from the macro result
JSON. The report therefore uses focused gomap counters from the merged PRs and
macro pprof top sites. That counter gap should be fixed before claiming a more
precise macro allocation/counter win.

The high-fanout timing JSON has a known boundary mismatch: the app/CometBFT
corrected accepted load window is larger than the runtime-breakdown window for
goleveldb. Throughput tables use the corrected accepted load-window metrics;
timing-bucket interpretation for high fanout should be treated as stage-profile
context, not exact wall accounting.

## Runner Pin

`cmd/local-report-runner/main.go` pins gomap to the merged M3 commit:

| Module | Version/ref |
| --- | --- |
| `github.com/snissn/gomap` | `v0.6.2-0.20260705085743-cabb7a17fb88` / `cabb7a17fb8809aebcd33f6c007907cf0caddcc7` |
| `github.com/cosmos/cosmos-db` | `v0.0.0-20260701184343-6ddcb75557e5` / `6ddcb75557e59bc4e6668ac7699cd52b63b3e402` |
| `github.com/cometbft/cometbft-db` | `v0.0.0-20260701074104-b4f87847a725` / `b4f87847a725f92a046d927ce4a0f5b08b965995` |
| `github.com/cosmos/iavl` | `v0.0.0-20260701072929-12a26715119b` / `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| Chain source | `https://github.com/snissn/celestia-cosmos-sdk` |
| Chain ref | `28e5525fefe7aaa53d4726ef7a367242bacf9003` |
| Docker image tag | `ironbird-report:snissn-sdk-28e5525f-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-cabb7a1` |

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z/summary.tsv
```

Profile summaries:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z/extract/profile_tops/
```

Timing/resource extracts:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z/extract/timing.tsv
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z/extract/resources.tsv
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z/extract/alloc_totals.tsv
```

Command:

```sh
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
OUT_ROOT=/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-${RUN_ID}

OUT_ROOT="$OUT_ROOT" \
  RUNNER=/mnt/fast4tb/tmp/local-report-runner-normal-cabb7a1 \
  REBUILD_RUNNER=true \
  SKIP_BUILD=false \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  STOP_CATALYST_AFTER_LOAD_WINDOW=true \
  TMPDIR=/mnt/fast4tb/tmp \
  scripts/ironbird_normal_workload_sweep.sh
```

Validation before the full matrix:

```sh
GOWORK=off GONOSUMDB=github.com/snissn/* GOPRIVATE=github.com/snissn/* \
  GOCACHE=/mnt/fast4tb/tmp/go-cache \
  GOMODCACHE=/mnt/fast4tb/tmp/go-mod-cache \
  GOPATH=/mnt/fast4tb/tmp/go-path-cache \
  go test ./cmd/local-report-runner ./activities/loadtest ./messages

GOWORK=off GONOSUMDB=github.com/snissn/* GOPRIVATE=github.com/snissn/* \
  GOCACHE=/mnt/fast4tb/tmp/go-cache \
  GOMODCACHE=/mnt/fast4tb/tmp/go-mod-cache \
  GOPATH=/mnt/fast4tb/tmp/go-path-cache \
  go build -o /mnt/fast4tb/tmp/local-report-runner-normal-cabb7a1 ./cmd/local-report-runner

bash -n scripts/ironbird_normal_workload_sweep.sh
git diff --check
```

## Current Matrix

Primary throughput is load-window TPS: successful app/CometBFT transactions
over the accepted sustained window. Effective ops/s multiplies transactions by
the workload's logical write fanout. Wall TPS includes local runner setup,
launch, image build/cache behavior, and profile collection.

| Workload | Backend | Attempt | Load-window s | Successful tx | Load-window TPS | Effective load-window ops/s | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 1 | 303.54 | 199,083 | 655.86 | 655.86 | 314.84 |
| Plain send | TreeDB | 1 | 361.52 | 199,496 | 551.82 | 551.82 | 306.19 |
| Small multisend | goleveldb | 2 | 371.02 | 238,996 | 644.15 | 1,288.31 | 457.49 |
| Small multisend | TreeDB | 2 | 415.52 | 238,998 | 575.18 | 1,150.35 | 418.66 |
| Moderate multisend | goleveldb | 2 | 362.02 | 159,283 | 439.98 | 4,399.82 | 305.74 |
| Moderate multisend | TreeDB | 3 | 631.02 | 318,496 | 504.73 | 5,047.31 | 408.47 |
| High fanout anchor | goleveldb | 1 | 617.02 | 7,992 | 12.94 | 6,472.22 | 10.41 |
| High fanout anchor | TreeDB | 1 | 322.03 | 7,962 | 24.72 | 12,362.20 | 16.63 |

## Pairwise Result

| Workload | goleveldb load TPS | TreeDB load TPS | TreeDB/load ratio | goleveldb effective ops/s | TreeDB effective ops/s | TreeDB/effective ratio | goleveldb wall TPS | TreeDB wall TPS | TreeDB/wall ratio |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | 655.86 | 551.82 | 0.84x | 655.86 | 551.82 | 0.84x | 314.84 | 306.19 | 0.97x |
| Small multisend | 644.15 | 575.18 | 0.89x | 1,288.31 | 1,150.35 | 0.89x | 457.49 | 418.66 | 0.92x |
| Moderate multisend | 439.98 | 504.73 | 1.15x | 4,399.82 | 5,047.31 | 1.15x | 305.74 | 408.47 | 1.34x |
| High fanout anchor | 12.94 | 24.72 | 1.91x | 6,472.22 | 12,362.20 | 1.91x | 10.41 | 16.63 | 1.60x |

## Comparison To Post-#3491 Baseline

Prior accepted full-matrix report:

```text
reports/ironbird_gomap_66a7733_appendonly_lease_matrix_2026-07-04.md
```

Prior accepted artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-66a7733-20260704T195730Z
```

| Workload | Post-#3491 TreeDB/load ratio | Current TreeDB/load ratio | Direction |
| --- | ---: | ---: | --- |
| Plain send | 0.96x | 0.84x | worse relative to goleveldb |
| Small multisend | 0.97x | 0.89x | worse relative to goleveldb |
| Moderate multisend | 1.31x | 1.15x | still TreeDB-favorable, smaller ratio |
| High fanout anchor | 2.19x | 1.91x | still TreeDB-favorable, smaller ratio |

The ratio movement is not a clean one-variable A/B. The goleveldb rows were also
faster in this run than the post-#3491 run, and wall time includes host and
Docker variability. The safe conclusion is narrower: the merged gomap allocation
sprint did not produce a visible low-fanout macro throughput win in Ironbird.

## Runtime Buckets

Commit time is real, but it is too small to be the only explanation for the
remaining macro behavior.

| Workload | Backend | Load/runtime window s | Commit s | Commit share | Commit-free speedup ceiling |
| --- | --- | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 303.54 | 33.65 | 11.09% | 1.12x |
| Plain send | TreeDB | 361.52 | 47.74 | 13.20% | 1.15x |
| Small multisend | goleveldb | 371.02 | 34.74 | 9.36% | 1.10x |
| Small multisend | TreeDB | 415.52 | 49.92 | 12.01% | 1.14x |
| Moderate multisend | goleveldb | 362.02 | 24.32 | 6.72% | 1.07x |
| Moderate multisend | TreeDB | 631.02 | 65.16 | 10.33% | 1.12x |
| High fanout anchor | goleveldb | 319.89 | 7.36 | 2.30% | 1.02x |
| High fanout anchor | TreeDB | 322.03 | 9.92 | 3.08% | 1.03x |

Low-fanout TreeDB also has a larger non-ABCI remainder in the current runtime
breakdown:

| Workload | Backend | ABCI observed s | Non-ABCI s | Non-ABCI share |
| --- | --- | ---: | ---: | ---: |
| Plain send | goleveldb | 195.68 | 107.86 | 35.54% |
| Plain send | TreeDB | 190.57 | 170.95 | 47.29% |
| Small multisend | goleveldb | 238.21 | 132.82 | 35.80% |
| Small multisend | TreeDB | 232.01 | 183.51 | 44.16% |
| Moderate multisend | goleveldb | 221.22 | 140.80 | 38.89% |
| Moderate multisend | TreeDB | 373.94 | 257.08 | 40.74% |

This is why the next instrumentation work should isolate the load window more
cleanly: current buckets show commit, finalize, CheckTx, pprof allocation, and a
large non-ABCI remainder, but they do not yet prove which exact subsystem owns
the low-fanout gap.

## Resource Profile

TreeDB wrote less Docker block data in every accepted pair, but it used much
more memory and generally left a larger on-disk data delta in low and moderate
fanout.

| Workload | Backend | Validator max memory | Docker block writes | Data delta | App DB delta | State DB delta | Tx-index DB delta |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2.081 GiB | 32.5 GB | 1.29 GB | 681 MB | 58 MB | 409 MB |
| Plain send | TreeDB | 4.625 GiB | 17.8 GB | 6.34 GB | 2.12 GB | 88 MB | 3.57 GB |
| Small multisend | goleveldb | 2.123 GiB | 40.2 GB | 1.43 GB | 635 MB | 66 MB | 547 MB |
| Small multisend | TreeDB | 4.718 GiB | 18.2 GB | 6.14 GB | 1.94 GB | 87 MB | 3.44 GB |
| Moderate multisend | goleveldb | 2.771 GiB | 36.8 GB | 1.35 GB | 381 MB | 74 MB | 688 MB |
| Moderate multisend | TreeDB | 8.244 GiB | 31.9 GB | 8.55 GB | 2.60 GB | 150 MB | 4.71 GB |
| High fanout anchor | goleveldb | 3.328 GiB | 28.7 GB | 1.01 GB | 68 MB | 126 MB | 499 MB |
| High fanout anchor | TreeDB | 7.932 GiB | 7.34 GB | 894 MB | 145 MB | 48 MB | 355 MB |

## Allocation Profile

Macro pprof `alloc_space` totals for accepted rows:

| Workload | goleveldb alloc_space | TreeDB alloc_space | TreeDB/goleveldb raw ratio | Notes |
| --- | ---: | ---: | ---: | --- |
| Plain send | 114.73 GB | 140.18 GB | 1.22x | TreeDB allocates more in the low-fanout case. |
| Small multisend | 145.47 GB | 158.36 GB | 1.09x | TreeDB allocates slightly more. |
| Moderate multisend | 198.93 GB | 338.25 GB | 1.70x raw | TreeDB accepted row processed about 2x transactions; per transaction TreeDB is lower. |
| High fanout anchor | 611,566 MB | 587,502 MB | 0.96x | TreeDB allocates slightly less in the high-fanout accepted row. |

Current low-fanout TreeDB top allocation sites still include clear TreeDB-local
work:

Plain send TreeDB:

- `zipper.(*ReadOnlyPrepareResult).cloneKey`: `11.48 GB`
- `caching.getEntrySlice`: `7.24 GB`
- `memtable.getAppendOnlyEntries`: `6.92 GB`
- `valuelog.getWriterAppendBuf`: `3.87 GB`
- `compress/zstd.ensureHist`: `3.16 GB`
- command-WAL payload read/decode/build paths: about `7.4 GB` combined
- shared Cosmos/CometBFT costs such as `jsonProperties`, cachekv btree
  allocation, `bytes.growSlice`, and `iavl.Node.clone` are also large.

Small multisend TreeDB:

- `caching.getEntrySlice`: `8.80 GB`
- `memtable.getAppendOnlyEntries`: `7.83 GB`
- `valuelog.getWriterAppendBuf`: `4.51 GB`
- `Batch.SetWithRevision`: `3.27 GB`
- command-WAL read/decode/build paths: about `8.1 GB` combined
- shared Cosmos/CometBFT costs remain material.

Moderate and high fanout change the profile:

- Moderate goleveldb is dominated by leveldb batch growth, table writer, and
  compaction allocation. TreeDB still has `memtable.getAppendOnlyEntries`,
  `Batch.SetWithRevision`, `cloneKey`, `getEntrySlice`, zstd, value-log, and
  command-WAL allocation, but it wins throughput despite those costs.
- High fanout TreeDB's largest allocation sites are largely block/RPC/value-log
  decode paths: `cometbft/rpc/core.loadRawBlock`, `BlockStore.LoadBlock`,
  `valuelog.getDecodeScratch`, `decodeRecordToScratch`, protobuf unmarshalling,
  and `bytes.growSlice`. This is less like the plain-send write-loop profile
  and more like a block load/decode workload.

CPU profiles support the same broad read:

- Low-fanout TreeDB still spends measurable time in `runtime.mallocgc`,
  `Zipper.mergeLeaf`, `caching.mergeCanonicalUnitRuns`, and
  `Batch.writeRegularLocked`.
- High-fanout TreeDB has much larger `runtime.mallocgc` and `runtime.growslice`
  shares, plus memtable/caching work, but goleveldb's own batch/table/compaction
  path becomes expensive enough that TreeDB wins.

## Interpretation

The current Ironbird benchmark has two distinct regimes:

1. Low fanout (`plain-send`, `small-multisend`): TreeDB's per-commit/per-write
   overhead and residual allocation pressure are not amortized enough. Goleveldb
   wins the accepted load window despite TreeDB using fewer Docker block writes.
2. Higher fanout (`moderate-multisend`, `high-fanout-anchor`): the write and
   storage shape becomes more favorable to TreeDB, and TreeDB wins by `1.15x` to
   `1.91x` effective ops/s.

The M0-M3 gomap sprint should be considered successful as a focused
implementation cleanup, not successful as a low-fanout macro-throughput win.
The fact that TreeDB still wins as fanout rises is useful: it says the full
stack can show TreeDB benefit, but the synthetic plain/small workloads still
mix DB overhead with Cosmos/CometBFT execution, non-ABCI time, profile overhead,
and load-generator/runtime effects.

## Recommended Next Work

1. Add timed-window TreeDB stats export to Ironbird result JSON.

   Export append-only direct arena usage, reserve grow calls/bytes, entry-pool
   retained estimates, command-WAL payload allocation/replay counters,
   value-log writer/decode buffer counters, checkpoint/sync counters, and DB
   open/close/cleanup counters at load-window start/end.

2. Split pprof/profile capture into load-window-only and closeout/replay windows.

   Several TreeDB allocation sites can be caused by setup, replay, close, or
   result collection. The M1 no-go showed why current whole-run profiles can
   mislead optimization priorities.

3. Investigate the remaining low-fanout TreeDB-local sites only after timed
   counters prove they occur in the TPS window.

   The current candidates are `cloneKey`, `getEntrySlice`,
   `getAppendOnlyEntries`, `valuelog.getWriterAppendBuf`, and command-WAL
   decode/payload paths.

4. Treat memory footprint as a first-class gate.

   TreeDB's accepted rows have materially higher RSS/high-water memory. Future
   optimizations should not trade small allocation wins for larger retained
   memory unless the report explicitly quantifies and accepts that tradeoff.

5. Keep the workload matrix.

   The fanout sweep is valuable because it shows where TreeDB begins winning.
   The next report should keep plain, small, moderate, and high fanout rather
   than optimizing only for the worst low-fanout row.

## M4 Decision

Do not close the broader `#3475` parent as "TreeDB low-fanout gap fixed". The
allocation sprint landed useful focused PRs, but the M4 matrix did not satisfy
the plain/small low-fanout north-star gate.

Close `#3498` as complete for final validation/reporting after this report PR
lands. Leave the parent tracker open, or replace it with a narrower follow-up
that owns timed-window instrumentation and steady-state TreeDB counter export.
