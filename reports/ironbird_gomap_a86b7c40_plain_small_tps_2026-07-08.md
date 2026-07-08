# Ironbird Plain/Small TPS Evidence After gomap command-WAL Cleanup

Date: 2026-07-08 UTC

## Summary

This is a focused post-merge Ironbird TPS run pinned to
`github.com/snissn/gomap` commit
`a86b7c40dcf0580c3b54757dba1d8d566ae90223`.

The run covers the two low-fanout workloads requested:

- `plain-send`
- `small-multisend`

Both workloads compare the full-stack backend configuration:

- `simapp-goleveldb-all`: goleveldb app DB and goleveldb CometBFT node DB
- `simapp-treedb-all`: TreeDB app DB and TreeDB CometBFT node DB

Result: the new gomap pin does not make TreeDB faster than goleveldb on these
two low-fanout Ironbird workloads. TreeDB remains slower on accepted
load-window TPS:

| Workload | TreeDB / goleveldb load-window TPS | Result |
| --- | ---: | --- |
| Plain send | 0.82x | goleveldb faster |
| Small multisend | 0.92x | goleveldb faster, smaller gap |

Compared with the previous `cabb7a1` low-fanout report, plain-send moved
slightly worse in relative terms (`0.84x` to `0.82x`), while small-multisend
moved slightly better (`0.89x` to `0.92x`). This is not a broad macro TPS win
from the command-WAL cleanup, but it is also not evidence of a new severe
low-fanout regression.

## Runner Pin

| Module | Version/ref |
| --- | --- |
| `github.com/snissn/gomap` | `v0.6.2-0.20260708083527-a86b7c40dcf0` / `a86b7c40dcf0580c3b54757dba1d8d566ae90223` |
| `github.com/cosmos/cosmos-db` | `v0.0.0-20260701184343-6ddcb75557e5` / `6ddcb75557e59bc4e6668ac7699cd52b63b3e402` |
| `github.com/cometbft/cometbft-db` | `v0.0.0-20260701074104-b4f87847a725` / `b4f87847a725f92a046d927ce4a0f5b08b965995` |
| `github.com/cosmos/iavl` | `v0.0.0-20260701072929-12a26715119b` / `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| Chain source | `https://github.com/snissn/celestia-cosmos-sdk` |
| Chain ref | `494824795d0b9eabf318aba755ee3320462df7ad` |
| Docker image tag | `ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-a86b7c40` |

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-20260708T163749Z
```

Summary ledger:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-20260708T163749Z/summary.tsv
```

Command:

```sh
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
OUT_ROOT=/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-${RUN_ID}

OUT_ROOT="$OUT_ROOT" \
  RUNNER=/mnt/fast4tb/tmp/local-report-runner-normal-a86b7c40 \
  REBUILD_RUNNER=false \
  SKIP_BUILD=false \
  WORKLOADS="plain-send,small-multisend" \
  LOAD_WINDOW_MIN=5m \
  LOAD_WINDOW_TARGET_FRACTION=0.995 \
  DRAIN_TIMEOUT=5m \
  STOP_CATALYST_AFTER_LOAD_WINDOW=true \
  TMPDIR=/mnt/fast4tb/tmp \
  GOWORK=off \
  GONOSUMDB=github.com/snissn/* \
  GOPRIVATE=github.com/snissn/* \
  GOCACHE=/mnt/fast4tb/tmp/go-cache \
  GOMODCACHE=/mnt/fast4tb/tmp/go-mod-cache \
  GOPATH=/mnt/fast4tb/tmp/go-path-cache \
  scripts/ironbird_normal_workload_sweep.sh
```

Validation before the run:

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
  go build -o /mnt/fast4tb/tmp/local-report-runner-normal-a86b7c40 ./cmd/local-report-runner

bash -n scripts/ironbird_normal_workload_sweep.sh
git diff --check
```

## Accepted Rows

Primary throughput is accepted load-window TPS: successful app/CometBFT
transactions over the sustained accepted window. Wall TPS includes local runner
setup, launch, Docker/image effects, and profile collection.

The script rejected rows that did not satisfy the `5m` minimum window. Rejected
attempts are preserved in `summary.tsv`; only accepted rows are used below.

| Workload | Backend | Attempt | Load-window s | Successful tx | Load-window TPS | Effective load-window ops/s | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2 | 576.02 | 398,487 | 691.79 | 691.79 | 545.77 |
| Plain send | TreeDB | 1 | 352.52 | 199,497 | 565.92 | 565.92 | 316.07 |
| Small multisend | goleveldb | 2 | 379.53 | 238,990 | 629.71 | 1,259.41 | 441.79 |
| Small multisend | TreeDB | 2 | 412.03 | 238,997 | 580.05 | 1,160.09 | 414.42 |

## Pairwise Result

| Workload | goleveldb load TPS | TreeDB load TPS | TreeDB/load ratio | goleveldb effective ops/s | TreeDB effective ops/s | TreeDB/effective ratio | goleveldb wall TPS | TreeDB wall TPS | TreeDB/wall ratio |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | 691.79 | 565.92 | 0.82x | 691.79 | 565.92 | 0.82x | 545.77 | 316.07 | 0.58x |
| Small multisend | 629.71 | 580.05 | 0.92x | 1,259.41 | 1,160.09 | 0.92x | 441.79 | 414.42 | 0.94x |

## Comparison To Previous Low-Fanout Report

Previous report:

```text
reports/ironbird_gomap_cabb7a1_lowfanout_allocation_sprint_2026-07-05.md
```

| Workload | Backend | Previous load TPS (`cabb7a1`) | Current load TPS (`a86b7c40`) | Change |
| --- | --- | ---: | ---: | ---: |
| Plain send | goleveldb | 655.86 | 691.79 | +5.48% |
| Plain send | TreeDB | 551.82 | 565.92 | +2.55% |
| Small multisend | goleveldb | 644.15 | 629.71 | -2.24% |
| Small multisend | TreeDB | 575.18 | 580.05 | +0.85% |

| Workload | Previous TreeDB/load ratio | Current TreeDB/load ratio | Direction |
| --- | ---: | ---: | --- |
| Plain send | 0.84x | 0.82x | slightly worse relative to goleveldb |
| Small multisend | 0.89x | 0.92x | slightly better relative to goleveldb |

The safe conclusion is narrow: the post-merge gomap pin did not create a
low-fanout Ironbird throughput win, and the relative movement is mixed.

## Runtime Buckets

Absolute total seconds are affected by accepted-attempt size: plain goleveldb
accepted on a larger retry than plain TreeDB. The average per-commit values and
percent-of-window values are the safer comparison points.

| Workload | Backend | Window s | ABCI s | ABCI % | Commit s | Avg commit ms | Commit % | Finalize s | CheckTx s | Non-ABCI s | Commit-free speedup ceiling |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 576.02 | 380.18 | 66.00% | 61.48 | 33.69 | 10.67% | 95.42 | 219.96 | 195.84 | 1.12x |
| Plain send | TreeDB | 352.52 | 205.34 | 58.25% | 48.57 | 54.95 | 13.78% | 46.71 | 108.21 | 147.18 | 1.16x |
| Small multisend | goleveldb | 379.53 | 259.19 | 68.29% | 34.04 | 28.92 | 8.97% | 68.41 | 153.97 | 120.33 | 1.10x |
| Small multisend | TreeDB | 412.03 | 250.34 | 60.76% | 50.60 | 47.96 | 12.28% | 60.11 | 137.13 | 161.69 | 1.14x |

Readout:

- TreeDB has higher average commit cost in both accepted comparisons:
  `54.95ms` vs `33.69ms` on plain-send, and `47.96ms` vs `28.92ms` on
  small-multisend.
- Commit is still not the whole gap. Even making TreeDB commit free only gives
  a modeled ceiling of `1.16x` plain and `1.14x` small.
- Non-ABCI time remains large, especially for TreeDB small-multisend
  (`39.24%` of the accepted window). That keeps the benchmark from being a pure
  DB engine comparison.

## Resource And Footprint Signals

| Workload | Backend | Max validator RSS | Data dir after | App DB after | Docker block-write high sample |
| --- | --- | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2.16 GiB | 2.33 GiB | 1.20 GiB | 65.66 GiB |
| Plain send | TreeDB | 6.41 GiB | 5.95 GiB | 2.02 GiB | 15.65 GiB |
| Small multisend | goleveldb | 2.17 GiB | 1.43 GiB | 0.58 GiB | 38.00 GiB |
| Small multisend | TreeDB | 6.77 GiB | 5.75 GiB | 1.83 GiB | 17.42 GiB |

TreeDB is materially larger in memory and on-disk footprint in this short-lived
Ironbird workload. The goleveldb rows show larger Docker block-write samples,
likely reflecting LevelDB write/compaction behavior, but this should be treated
as a container-level signal rather than a precise DB write-amplification
measurement.

## Profile Notes

Profiles were captured for every accepted row:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-20260708T163749Z/plain-send/goleveldb/attempt-2/pprof/
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-20260708T163749Z/plain-send/treedb/attempt-1/pprof/
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-20260708T163749Z/small-multisend/goleveldb/attempt-2/pprof/
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-a86b7c40-plain-small-20260708T163749Z/small-multisend/treedb/attempt-2/pprof/
```

Caveats:

- CPU profiles are whole-run validator profiles, not load-window-only profiles.
- Heap profiles are in-use heap captures near the end of the load window, not
  allocation-space totals.
- `treedb_stat_deltas` are still absent from the Ironbird macro JSON.

Observed top-profile themes:

- goleveldb CPU has prominent `snappy.encodeBlock`,
  `goleveldb.(*tableCompactionBuilder).run`, runtime scanning/GC, syscalls, and
  secp256k1 work.
- TreeDB CPU is dominated by runtime scanning/GC, secp256k1, syscalls, and
  visible TreeDB-adjacent compression work such as
  `lz4block.(*Compressor).CompressBlock`.
- TreeDB heap has large in-use value-log/cache/compression residents:
  `valuelog.getDecodeScratch` around `310-348MB`, zstd history buffers around
  `133MB`, leaf-page read cache around `94-99MB`, grouped frame cache around
  `71-82MB`, and writer append buffers around `72-80MB`.

## Interpretation

This run supports three practical conclusions:

1. The command-WAL cleanup pin is correctly consumed by Ironbird and runs
   through sustained accepted low-fanout windows.
2. The post-merge pin does not demonstrate a low-fanout throughput win for
   TreeDB over goleveldb in Ironbird.
3. The remaining TreeDB low-fanout gap still looks like a mix of higher
   per-commit/per-batch cost, larger memory/heap residency, and non-ABCI
   harness/app work rather than a single commit-only problem.

The next useful measurement improvement is load-window-only CPU/alloc profiling
plus TreeDB macro counters in the Ironbird result JSON. Without that, the
profile evidence is useful but still too coarse to rank the next TreeDB
optimization sprint with high confidence.
