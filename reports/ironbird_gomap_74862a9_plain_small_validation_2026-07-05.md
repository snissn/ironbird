# Ironbird Plain/Small Validation After gomap 74862a9

Date: 2026-07-05 UTC

## Summary

This report validates the latest consumed `snissn/gomap` main commit after PR
`#3510`:

| Module | Version/ref |
| --- | --- |
| `github.com/snissn/gomap` | `v0.6.2-0.20260705225919-74862a95bb13` |
| gomap ref | `74862a95bb1308fe0c6b8ae6cbb394772d3b4f5a` |
| Docker image tag | `ironbird-report:snissn-sdk-28e5525f-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-74862a9` |

The accepted rows do not show a low-fanout macro throughput win for TreeDB:

| Workload | TreeDB load-window TPS vs goleveldb | TreeDB effective ops/s vs goleveldb | Result |
| --- | ---: | ---: | --- |
| Plain send | 0.84x | 0.84x | goleveldb wins |
| Small multisend | 0.87x | 0.87x | goleveldb wins |

This is still useful evidence. The current profiles show remaining TreeDB
allocation targets in accepted rows, while also showing that macro TPS is not
explained by TreeDB alone. The CPU profiles are dominated by Go GC/runtime,
Cosmos auth/signing, CometBFT JSON/event/indexing, and workload harness work.

## Reproduction

Artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-74862a9-20260705T230312Z
```

Accepted row summary:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-74862a9-20260705T230312Z/summary.tsv
```

Extracted tables and profile tops:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-74862a9-20260705T230312Z/extract/
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-74862a9-20260705T230312Z/extract/profile_tops/
```

Command:

```sh
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
OUT_ROOT=/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-74862a9-${RUN_ID}
mkdir -p "$OUT_ROOT"

GOWORK=off \
GONOSUMDB='github.com/snissn/*' \
GOPRIVATE='github.com/snissn/*' \
GOCACHE=/mnt/fast4tb/go-cache \
GOMODCACHE=/mnt/fast4tb/go/pkg/mod \
GOPATH=/mnt/fast4tb/go \
OUT_ROOT="$OUT_ROOT" \
RUNNER=/mnt/fast4tb/tmp/local-report-runner-normal-74862a9 \
REBUILD_RUNNER=true \
SKIP_BUILD=false \
WORKLOADS=plain-send,small-multisend \
LOAD_WINDOW_MIN=5m \
LOAD_WINDOW_TARGET_FRACTION=0.995 \
DRAIN_TIMEOUT=5m \
STOP_CATALYST_AFTER_LOAD_WINDOW=true \
TMPDIR=/mnt/fast4tb/tmp \
scripts/ironbird_normal_workload_sweep.sh

scripts/ironbird_extract_normal_workload.sh "$OUT_ROOT"
```

Preflight checks:

```sh
bash -n scripts/ironbird_normal_workload_sweep.sh
bash -n scripts/ironbird_extract_normal_workload.sh
GOWORK=off go list -m github.com/snissn/gomap@v0.6.2-0.20260705225919-74862a95bb13
GOWORK=off TMPDIR=/mnt/fast4tb/tmp GOCACHE=/mnt/fast4tb/go-cache \
  GOMODCACHE=/mnt/fast4tb/go/pkg/mod GOPATH=/mnt/fast4tb/go \
  go test ./cmd/local-report-runner ./activities/loadtest ./messages
```

## Accepted Rows

| Workload | Backend | Attempt | Load-window s | Successful tx | Load-window TPS | Effective ops/s | Wall TPS |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2 | 594.52 | 398,495 | 670.28 | 670.28 | 531.53 |
| Plain send | TreeDB | 1 | 354.52 | 199,499 | 562.73 | 562.73 | 310.50 |
| Small multisend | goleveldb | 2 | 365.02 | 238,995 | 654.74 | 1,309.48 | 463.14 |
| Small multisend | TreeDB | 2 | 418.52 | 238,923 | 570.87 | 1,141.74 | 417.58 |

Pairwise:

| Workload | goleveldb load TPS | TreeDB load TPS | TreeDB/load ratio | goleveldb effective ops/s | TreeDB effective ops/s | TreeDB/effective ratio | goleveldb wall TPS | TreeDB wall TPS | TreeDB/wall ratio |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | 670.28 | 562.73 | 0.84x | 670.28 | 562.73 | 0.84x | 531.53 | 310.50 | 0.58x |
| Small multisend | 654.74 | 570.87 | 0.87x | 1,309.48 | 1,141.74 | 0.87x | 463.14 | 417.58 | 0.90x |

## Comparison To cabb7a1 Low-Fanout Report

Prior report:

```text
reports/ironbird_gomap_cabb7a1_lowfanout_allocation_sprint_2026-07-05.md
```

Prior artifact root:

```text
/mnt/fast4tb/ironbird-normal-workload-sweep-gomap-cabb7a1-20260705T090215Z
```

| Workload | cabb7a1 TreeDB/load ratio | 74862a9 TreeDB/load ratio | Direction |
| --- | ---: | ---: | --- |
| Plain send | 0.84x | 0.84x | neutral relative ratio |
| Small multisend | 0.89x | 0.87x | slightly worse relative ratio |

The current run does not establish a macro throughput improvement from
`74862a9` on the two low-fanout workloads. It does confirm that remaining
TreeDB allocation work is visible in accepted rows.

## Runtime Buckets

| Workload | Backend | Load/runtime window s | ABCI observed s | Commit s | Finalize s | CheckTx s | Non-ABCI s | Avg commit ms | Commit share | Commit-free speedup ceiling |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 594.52 | 375.93 | 63.74 | 85.65 | 222.89 | 218.60 | 35.47 | 10.72% | 1.12x |
| Plain send | TreeDB | 354.52 | 194.68 | 47.01 | 42.19 | 103.70 | 159.84 | 53.97 | 13.26% | 1.15x |
| Small multisend | goleveldb | 365.02 | 229.15 | 33.50 | 53.08 | 139.91 | 135.87 | 29.76 | 9.18% | 1.10x |
| Small multisend | TreeDB | 418.52 | 232.79 | 49.70 | 49.80 | 130.86 | 185.74 | 48.53 | 11.87% | 1.13x |

Commit remains measurable, but the commit-free ceiling is too small to explain
the remaining macro throughput gap by itself.

## Resource And Allocation Totals

| Workload | Backend | Validator high-water memory | Docker block write | Data dir delta | App DB delta | State DB delta | Tx index DB delta | pprof alloc-space total |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain send | goleveldb | 2.179 GiB | 69.2 GB | 2.486 GB | 1.261 GB | 0.138 GB | 0.813 GB | 228.63 GB |
| Plain send | TreeDB | 4.629 GiB | 16.0 GB | 6.389 GB | 2.166 GB | 0.112 GB | 3.570 GB | 136.82 GB |
| Small multisend | goleveldb | 2.143 GiB | 40.4 GB | 1.397 GB | 0.601 GB | 0.065 GB | 0.549 GB | 145.97 GB |
| Small multisend | TreeDB | 7.225 GiB | 18.4 GB | 6.219 GB | 1.953 GB | 0.136 GB | 3.451 GB | 154.80 GB |

TreeDB writes less through Docker block I/O in these rows, but uses more memory
and creates a larger data directory. Plain-send TreeDB alloc-space is lower than
goleveldb, but small-multisend TreeDB alloc-space is higher.

## Profile Notes

Top TreeDB plain-send allocation sites:

| Site | Alloc-space |
| --- | ---: |
| `TreeDB/zipper.(*ReadOnlyPrepareResult).cloneKey` | 11.28 GB |
| `TreeDB/caching.getEntrySlice` | 7.62 GB |
| `TreeDB/internal/memtable.getAppendOnlyEntries` | 6.91 GB |
| `TreeDB/internal/valuelog.getWriterAppendBuf` | 3.50 GB |
| `TreeDB/internal/commitlog.(*RawKVBatchPayloadBuilder).appendRawKVPayloadSpace` | 2.77 GB |
| `TreeDB/internal/commitlog.(*Reader).readSegmentPayload` | 2.34 GB |
| `TreeDB/internal/commitlog.DecodeCommandFrame` | 2.31 GB |

Top TreeDB small-multisend allocation sites:

| Site | Alloc-space |
| --- | ---: |
| `TreeDB/caching.getEntrySlice` | 8.81 GB |
| `TreeDB/internal/memtable.getAppendOnlyEntries` | 7.82 GB |
| `TreeDB/internal/valuelog.getWriterAppendBuf` | 4.41 GB |
| `TreeDB/caching.(*Batch).SetWithRevision` | 3.24 GB |
| `TreeDB/internal/commitlog.(*Reader).readSegmentPayload` | 2.74 GB |
| `TreeDB/internal/commitlog.DecodeCommandFrame` | 2.68 GB |
| `TreeDB/internal/commitlog.(*RawKVBatchPayloadBuilder).appendRawKVPayloadSpace` | 2.62 GB |

The most direct next TreeDB allocation task is the plain-send
`ReadOnlyPrepareResult.cloneKey` path. It appears in the accepted plain row and
is absent from the accepted small row after `74862a9`, which makes it a scoped
plain-send target rather than a global explanation for both workloads.

The recurring cross-workload targets after that are:

- append-only entry slice materialization and arena reuse
- value-log writer append buffer allocation
- command-WAL payload build/read/decode allocation
- cache batch `SetWithRevision` allocation in small-multisend

CPU profiles are allocation/GC heavy but not TreeDB-only. For TreeDB plain-send,
the CPU profile includes about `161.97s` cumulative `runtime.gcDrain` and
`47.32s` cumulative `runtime.mallocgc` in a `551.03s` sample profile. For
TreeDB small-multisend, it includes about `180.45s` cumulative
`runtime.gcDrain` and `58.03s` cumulative `runtime.mallocgc` in a `641.84s`
sample profile. Cosmos signing/auth and CometBFT JSON/event/indexing remain
large macro contributors.

## Conclusion

This run is a validation checkpoint, not a success claim. `gomap@74862a9` does
not make TreeDB faster than goleveldb on the low-fanout Ironbird rows, but it
does preserve enough accepted-row profile signal to continue the allocation
sprint with better-scoped gomap work.

The next PR candidate should target the plain-send `cloneKey` allocation path
first, validate it with a focused gomap benchmark, then rerun the accepted
plain/small Ironbird pair before moving to the shared `getEntrySlice`,
`getAppendOnlyEntries`, value-log append buffer, and command-WAL payload sites.
