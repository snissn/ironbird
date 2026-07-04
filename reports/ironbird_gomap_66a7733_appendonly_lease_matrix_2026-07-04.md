# Ironbird Matrix After TreeDB Command-WAL Value Lease

Date: 2026-07-04 HST

## Executive Summary

This report records the full Ironbird normal-workload rerun after merging
`snissn/gomap#3491`, the TreeDB command-WAL value lease optimization.

The focused gomap benchmark moved in the intended direction:

- hot `BatchWriteSync` command-WAL path: `56.83us` to `50.02us`, 11.99%
  faster, `59.18KiB/op` to `43.60KiB/op`, and `153` to `138 allocs/op`.
- checkpoint-flavored path: `98.91us` to `91.61us`, 7.38% faster,
  `72.14KiB/op` to `58.78KiB/op`, and `155` to `140 allocs/op`.

The full Ironbird matrix did not show a broad macro-throughput shift from this
gomap change. Current full-stack TreeDB remains slightly behind goleveldb on
plain send and small multisend, roughly tied on wall TPS for small multisend,
and materially ahead on moderate and high fanout:

| Workload | TreeDB load-window TPS vs goleveldb | TreeDB effective ops/s vs goleveldb | Result |
| --- | ---: | ---: | --- |
| Plain send | 0.96x | 0.96x | goleveldb slight win |
| Small multisend | 0.97x | 0.97x | near tie |
| Moderate multisend | 1.31x | 1.31x | TreeDB win |
| High fanout anchor | 2.19x | 2.19x | TreeDB large win |

The main performance conclusion is unchanged: this synthetic benchmark becomes
TreeDB-favorable as write fanout grows, but the normal low-fanout path is still
limited by broader Cosmos/CometBFT execution, allocation, and timing buckets.
Commit time is visible, but not large enough by itself to explain or fix the
remaining macro behavior. In the current run, making commit free would cap
TreeDB speedup at about 1.16x for plain send, 1.13x for moderate multisend,
and 1.03x for high fanout.

## Scope And Caveats

This report is fork-local to `snissn/ironbird` and `snissn/gomap`. It does not
open or claim readiness for public upstream PRs.

The runner pins gomap to the exact `#3491` merge commit, not the moving gomap
`main` branch. `snissn/gomap` main advanced after the merge while this report
was being assembled, so the fixed pseudo-version is intentional.

There is no locally available full accepted matrix that isolates only
post-`#3488` to post-`#3491`. The older full-matrix baseline used below is the
previous accepted full normal-workload matrix, but it also used different gomap
and `cosmos-db` pseudo-versions. Treat that comparison as prior-run context,
not a pure one-variable A/B. The cleaner same-`cosmos-db` comparison available
locally is plain-send-only, from the post-M2 report.

Ironbird does not currently export Go benchmark `B/op` or `allocs/op` for the
macro workload. Those counters are available from the focused gomap benchmark.
For Ironbird macro runs this report uses pprof `alloc_space` totals and top
allocation sites instead.

Ironbird also does not export an append-only direct-arena "used bytes" counter
in the macro result JSON. The focused gomap profile showed value-arena pool
get/put/drop-byte counters at zero for this command-WAL lease path; that is the
right directional signal for this PR, but the macro harness still needs a direct
TreeDB stats extraction if we want arena residency in future reports.

## Gomap PR State

| Item | Value |
| --- | --- |
| Tracker issue | `snissn/gomap#3490` |
| PR | `snissn/gomap#3491` |
| PR title | `TreeDB: lease command-WAL values into append-only memtables` |
| PR state | merged |
| Merge commit | `66a7733188b46b4d814bc7290ff33ffd29de79bf` |
| PR branch head before merge | `b7440550197a2c17ac86ed273c3323fb7a6d09ed` |
| CI state before merge | all required checks and CodeRabbit success |

Focused gomap validation:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp GOCACHE=/mnt/fast4tb/tmp/go-cache \
  GOMODCACHE=/mnt/fast4tb/tmp/go-mod-cache GOPATH=/mnt/fast4tb/tmp/go-path-cache \
  go test ./TreeDB ./TreeDB/caching ./TreeDB/internal/commitlog -count=1

git diff --check
```

Focused benchmark artifacts:

```text
/mnt/fast4tb/tmp/gomap-3491-reviewfix-20260704T193434Z/benchstat_hot.txt
/mnt/fast4tb/tmp/gomap-3491-reviewfix-20260704T193434Z/benchstat_checkpoint.txt
/mnt/fast4tb/tmp/gomap-3490-candidate-knownzero-20260704T192107Z/profile_final/
```

## Runner Pin

`cmd/local-report-runner/main.go` now pins gomap to the merged PR:

| Module | Version/ref |
| --- | --- |
| `github.com/cosmos/cosmos-db` | `v0.0.0-20260701184343-6ddcb75557e5` / `6ddcb75557e59bc4e6668ac7699cd52b63b3e402` |
| `github.com/snissn/gomap` | `v0.6.2-0.20260704195246-66a7733188b4` / `66a7733188b46b4d814bc7290ff33ffd29de79bf` |
| `github.com/cosmos/iavl` | `v0.0.0-20260701072929-12a26715119b` / `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| `github.com/cometbft/cometbft-db` | `v0.0.0-20260701074104-b4f87847a725` / `b4f87847a725f92a046d927ce4a0f5b08b965995` |

Docker image tags were updated from `gomap-ca4e486` to `gomap-66a7733`.

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-66a7733-20260704T195730Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-66a7733-20260704T195730Z/summary.tsv
```

Profile summaries:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-66a7733-20260704T195730Z/extract/profile_tops/
```

Command:

```sh
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
OUT_ROOT=/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-66a7733-${RUN_ID}

OUT_ROOT="$OUT_ROOT" \
RUNNER=/mnt/fast4tb/tmp/local-report-runner-normal-66a7733 \
REBUILD_RUNNER=true \
SKIP_BUILD=false \
LOAD_WINDOW_MIN=5m \
LOAD_WINDOW_TARGET_FRACTION=0.995 \
DRAIN_TIMEOUT=5m \
STOP_CATALYST_AFTER_LOAD_WINDOW=true \
TMPDIR=/mnt/fast4tb/tmp \
scripts/ironbird_normal_workload_sweep.sh
```

The script reran short rows with larger block counts until the accepted row met
the five-minute load-window gate. All accepted rows met that gate.

## Current Matrix

Primary throughput is load-window TPS: successful app/CometBFT transactions
over the accepted sustained window. Wall TPS includes local runner setup,
launch, image build/cache behavior, and profile collection.

| Workload | Backend | Attempt | Load-window s | Successful tx | Load-window TPS | Effective load-window ops/s | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 1 | 371.53 | 199,000 | 535.62 | 535.62 | 282.80 |
| Plain send | TreeDB | 1 | 386.53 | 199,497 | 516.13 | 516.13 | 271.50 |
| Small multisend | goleveldb | 2 | 455.53 | 238,957 | 524.57 | 1,049.14 | 372.48 |
| Small multisend | TreeDB | 2 | 468.03 | 238,998 | 510.65 | 1,021.30 | 371.15 |
| Moderate multisend | goleveldb | 2 | 431.03 | 159,231 | 369.42 | 3,694.24 | 264.55 |
| Moderate multisend | TreeDB | 2 | 328.03 | 159,251 | 485.48 | 4,854.78 | 319.61 |
| High fanout anchor | goleveldb | 1 | 758.03 | 7,984 | 10.53 | 5,266.31 | 8.58 |
| High fanout anchor | TreeDB | 2 | 691.55 | 15,932 | 23.04 | 11,518.99 | 18.36 |

Effective operations:

- plain send: same as successful transactions.
- small multisend: successful transactions times 2 recipients.
- moderate multisend: successful transactions times 10 recipients.
- high fanout anchor: successful transactions times 20 contained messages times
  25 recipients.

The high-fanout goleveldb row has a Catalyst raw-result mismatch: Catalyst's raw
summary said zero successful transactions, while app/CometBFT metrics corrected
that to 7,984 successful transactions. This report uses the corrected app
metrics, as the previous normal-workload report also did.

## Pairwise Current Result

| Workload | goleveldb load TPS | TreeDB load TPS | TreeDB/load ratio | goleveldb effective ops/s | TreeDB effective ops/s | TreeDB/effective ratio | goleveldb wall TPS | TreeDB wall TPS | TreeDB/wall ratio |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | 535.62 | 516.13 | 0.96x | 535.6 | 516.1 | 0.96x | 282.80 | 271.50 | 0.96x |
| Small multisend | 524.57 | 510.65 | 0.97x | 1,049.1 | 1,021.3 | 0.97x | 372.48 | 371.15 | 1.00x |
| Moderate multisend | 369.42 | 485.48 | 1.31x | 3,694.2 | 4,854.8 | 1.31x | 264.55 | 319.61 | 1.21x |
| High fanout anchor | 10.53 | 23.04 | 2.19x | 5,266.3 | 11,519.0 | 2.19x | 8.58 | 18.36 | 2.14x |

## Comparison To Prior Accepted Full Matrix

Prior accepted full-matrix artifact:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-stopwindow-20260704T011644Z
```

Prior accepted full-matrix report:

```text
reports/ironbird_normal_workload_treedb_vs_goleveldb_2026-07-04.md
```

Dependency caveat: that run used
`github.com/snissn/gomap@1afe86c1cbc0acc7336fc0944c69ebfcd2f3dc8d` and
`github.com/cosmos/cosmos-db@f1d8b016a90cc39abde5d362e4f6b54b96df5c73`. The
current run uses gomap `66a7733...` and cosmos-db `6ddcb755...`.

| Workload | Backend | Prior load TPS | Current load TPS | Load delta | Prior wall TPS | Current wall TPS | Wall delta |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 531.58 | 535.62 | +0.8% | 350.93 | 282.80 | -19.4% |
| Plain send | TreeDB | 512.32 | 516.13 | +0.7% | 354.75 | 271.50 | -23.5% |
| Small multisend | goleveldb | 532.24 | 524.57 | -1.4% | 382.47 | 372.48 | -2.6% |
| Small multisend | TreeDB | 519.00 | 510.65 | -1.6% | 376.60 | 371.15 | -1.4% |
| Moderate multisend | goleveldb | 377.82 | 369.42 | -2.2% | 267.12 | 264.55 | -1.0% |
| Moderate multisend | TreeDB | 465.63 | 485.48 | +4.3% | 311.87 | 319.61 | +2.5% |
| High fanout anchor | goleveldb | 9.90 | 10.53 | +6.4% | 8.14 | 8.58 | +5.4% |
| High fanout anchor | TreeDB | 23.38 | 23.04 | -1.5% | 15.40 | 18.36 | +19.2% |

Do not overread wall TPS in the prior/current comparison. Wall time includes
runner and Docker build/cache behavior. Load-window TPS is the better benchmark
throughput comparison.

## Plain-Send Same-cosmos-db Context

The plain-send-only post-M2 report used `cosmos-db@6ddcb755...`, matching the
current runner's cosmos-db line, but it used pre-`#3488` gomap
`ca4e48677afd...`. That report measured:

| Backend | Older plain-send load TPS | Current plain-send load TPS |
| --- | ---: | ---: |
| goleveldb | 590.18 | 535.62 |
| TreeDB | 495.00 | 516.13 |

This is not a full matrix and should not be treated as a clean controlled
before/after either, but it is useful context: the current TreeDB plain-send row
is better than the prior plain-send-only TreeDB row, while the goleveldb row is
lower in the current matrix run.

## Focused Gomap Microbench

Hot command-WAL path:

| Metric | Baseline | Candidate | Delta |
| --- | ---: | ---: | ---: |
| sec/op | 56.83us | 50.02us | -11.99% |
| writes/s | 2.252M | 2.559M | +13.62% |
| B/op | 59.18KiB | 43.60KiB | -26.32% |
| allocs/op | 153 | 138 | -9.80% |

Checkpoint-flavored path:

| Metric | Baseline | Candidate | Delta |
| --- | ---: | ---: | ---: |
| sec/op | 98.91us | 91.61us | -7.38% |
| writes/s | 1.294M | 1.397M | +7.97% |
| B/op | 72.14KiB | 58.78KiB | -18.51% |
| allocs/op | 155 | 140 | -9.68% |

The focused profiled candidate showed:

| Counter | Value |
| --- | ---: |
| `treedb.cache.append_only.value_arena_pool_gets_total` | 0 |
| `treedb.cache.append_only.value_arena_pool_puts_total` | 0 |
| `treedb.cache.append_only.value_arena_pool_drop_bytes_total` | 0 |
| `treedb.cache.append_only.entry_pool_retained_bytes_estimate` | 134,964,368 |
| `treedb.cache.append_only.reserve.grow_bytes_total` | 477,439,072 |

Interpretation: the command-WAL optimization succeeded on the narrow path by
leasing WAL payload value buffers into append-only memtables and avoiding the
previous direct value-arena allocation path for these payloads. The macro run
does not export the same counter, so this report cannot assign a macro "direct
arena used bytes" value.

## Runtime Timing Buckets

| Workload | Backend | Workload s | Commit s | Finalize s | CheckTx s | Non-ABCI s | Commit pct | Max speedup if commit free |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 371.53 | 41.40 | 69.71 | 147.81 | 110.26 | 11.14% | 1.125x |
| Plain send | TreeDB | 386.53 | 54.13 | 60.61 | 129.68 | 140.16 | 14.00% | 1.163x |
| Small multisend | goleveldb | 455.53 | 41.04 | 86.29 | 188.25 | 136.17 | 9.01% | 1.099x |
| Small multisend | TreeDB | 468.03 | 54.33 | 68.04 | 158.58 | 184.29 | 11.61% | 1.131x |
| Moderate multisend | goleveldb | 431.03 | 26.70 | 88.92 | 175.76 | 136.48 | 6.20% | 1.066x |
| Moderate multisend | TreeDB | 328.03 | 37.54 | 76.97 | 120.35 | 90.39 | 11.44% | 1.129x |
| High fanout anchor | goleveldb | 758.03 | 8.24 | 63.96 | 363.11 | 0.00 | 1.67% | 1.017x |
| High fanout anchor | TreeDB | 691.55 | 18.89 | 143.70 | 379.08 | 0.00 | 2.73% | 1.028x |

Commit is not invisible: TreeDB commit time is higher than goleveldb in every
current row. But the speedup ceilings show that commit alone is no longer the
high-leverage macro target. For example, high fanout is already TreeDB's largest
win, and removing commit entirely would only add about 2.8% runtime headroom.

The non-ABCI bucket remains under-instrumented. It is useful as a residual, but
not as a root-cause category. Some rows report ABCI-observed time close to or
above the workload window because these counters are aggregated across
concurrent or repeated metric observations, not exclusive wall-clock spans.

## Memory, Disk, And Docker IO

| Workload | Backend | Validator max memory | Docker block write | `/simd/data` delta | `application.db` delta | `tx_index.db` delta |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2.092GiB | 34.7GB | 1.20GiB | 0.64GiB | 0.38GiB |
| Plain send | TreeDB | 6.768GiB | 16.4GB | 5.94GiB | 2.03GiB | 3.33GiB |
| Small multisend | goleveldb | 2.15GiB | 41.0GB | 1.30GiB | 0.56GiB | 0.51GiB |
| Small multisend | TreeDB | 7.351GiB | 18.4GB | 5.84GiB | 1.87GiB | 3.22GiB |
| Moderate multisend | goleveldb | 2.704GiB | 35.8GB | 1.31GiB | 0.38GiB | 0.67GiB |
| Moderate multisend | TreeDB | 4.672GiB | 12.6GB | 3.97GiB | 1.27GiB | 2.13GiB |
| High fanout anchor | goleveldb | 3.572GiB | 27.9GB | 0.97GiB | 0.09GiB | 0.47GiB |
| High fanout anchor | TreeDB | 8.171GiB | 14.7GB | 1.66GiB | 0.29GiB | 0.69GiB |

TreeDB consistently writes fewer Docker block bytes than goleveldb in this
matrix, but uses more memory and leaves a larger final data directory. That
fits the broader pattern: TreeDB can win when the workload becomes storage-write
sensitive enough, while low-fanout rows still expose memory/allocation and
application execution costs.

## Allocation Profile Summary

Ironbird macro profile totals are pprof `alloc_space`, not Go benchmark
`B/op`. Accepted rows:

| Workload | goleveldb alloc_space | TreeDB alloc_space | TreeDB/goleveldb |
| --- | ---: | ---: | ---: |
| Plain send | 118,688MB | 144,142MB | 1.21x |
| Small multisend | 150,714MB | 163,637MB | 1.09x |
| Moderate multisend | 207,717MB | 170,862MB | 0.82x |
| High fanout anchor | 646,930MB | 858,448MB | 1.33x |

Notable TreeDB allocation sites in the current matrix:

| Workload | Site | Flat alloc_space |
| --- | --- | ---: |
| Plain send | `TreeDB/zipper.(*ReadOnlyPrepareResult).cloneKey` | 11,009MB |
| Plain send | `TreeDB/caching.getEntrySlice` | 7,943MB |
| Plain send | `TreeDB/internal/memtable.getAppendOnlyEntries` | 7,188MB |
| Plain send | `TreeDB/internal/valuelog.getWriterAppendBuf` | 3,877MB |
| Moderate multisend | `TreeDB/internal/memtable.getAppendOnlyEntries` | 9,231MB |
| Moderate multisend | `TreeDB/caching.(*Batch).SetWithRevision` | 6,174MB |
| Moderate multisend | `TreeDB/caching.getEntrySlice` | 4,981MB |
| High fanout | `TreeDB/internal/valuelog.getDecodeScratch` | 32,671MB |
| High fanout | `TreeDB/caching.(*Batch).SetWithRevision` | 27,425MB |
| High fanout | `TreeDB/internal/valuelog.decodeRecordToScratch` | 22,888MB |
| High fanout | `TreeDB/internal/memtable.getAppendOnlyEntries` | 20,476MB |

The old catastrophic `getAppendOnlyEntries` profile from the plain-send-only
post-M2 report is materially reduced in this current plain-send row, but TreeDB
still has meaningful allocation pressure. The current next candidates are
workload-specific:

- plain send: `cloneKey`, `getEntrySlice`, and remaining memtable entry churn.
- high fanout: value-log decode scratch and `SetWithRevision` allocation.
- shared: Cosmos/CometBFT JSON, protobuf, block loading, and SDK cache layers
  remain large non-TreeDB allocation sources.

## Overall Interpretation

`#3491` is worth keeping. It is a measured cleanup of the command-WAL value
lifetime path, it reduces focused latency and allocation, and it passed review
and CI. It also removes a narrow allocation smell that could keep confusing
future profiling.

It is not enough by itself to move the full Ironbird normal workload matrix in
a decisive way. The macro evidence says:

- TreeDB has stable demonstrative wins in moderate and high-fanout synthetic
  workloads.
- TreeDB is still not categorically faster on low-fanout, normal transaction
  shapes.
- Commit remains a measurable TreeDB disadvantage, but its standalone ceiling is
  modest.
- The next useful profiling work should separate residual TreeDB allocation from
  SDK/CometBFT execution and indexing cost, especially in the non-ABCI residual
  and high-fanout value-log decode paths.

## Validation

Runner pin validation:

```sh
go mod download github.com/snissn/gomap@v0.6.2-0.20260704195246-66a7733188b4
go test ./cmd/local-report-runner ./activities/loadtest ./messages
bash -n scripts/ironbird_normal_workload_sweep.sh
go build -o /mnt/fast4tb/tmp/local-report-runner-normal-66a7733 ./cmd/local-report-runner
git diff --check
```

The full matrix completed successfully at:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-66a7733-20260704T195730Z
```
