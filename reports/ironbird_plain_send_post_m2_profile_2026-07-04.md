# Post-M2 Ironbird Plain-Send Profile

Date: 2026-07-04 HST

This report records the first Ironbird plain-send rerun after the TreeDB M0-M2
optimization loop in `snissn/gomap`.

## Scope

The run compares full-stack goleveldb against full-stack TreeDB on the same
synthetic Cosmos SDK simapp plain `MsgSend` workload. "Full-stack" means both
the Cosmos app DB and the CometBFT node DB use the selected backend.

The primary throughput number is load-window TPS: successful app/CometBFT
transactions over the accepted sustained load window. Each accepted run met the
5 minute minimum load-window gate.

## Artifact Context

| Field | Value |
| --- | --- |
| Artifact root | `/mnt/fast4tb/ironbird-plain-send-post-m2-20260704T064921Z` |
| Ironbird branch | `codex/3479-post-m2-plain-send` |
| gomap | `ca4e48677afd944cd08911e2b85175c8ce9c55ac` |
| cosmos-db | `6ddcb75557e59bc4e6668ac7699cd52b63b3e402` |
| iavl | `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| cometbft-db | `b4f87847a725f92a046d927ce4a0f5b08b965995` |
| simapp source | `snissn/celestia-cosmos-sdk@28e5525fefe7aaa53d4726ef7a367242bacf9003` |

Workload shape:

| Field | Value |
| --- | ---: |
| Validators | 1 |
| Full nodes | 0 |
| Active wallets | 5,000 |
| Preseeded accounts | 100,000 |
| Blocks x tx/block | 400 x 500 |
| Intended transactions | 200,000 |
| Load-window target | 199,000 successful tx |
| Load-window minimum | 300s |
| Message | 1 `MsgSend` per tx |
| Raw tx audit | disabled |

The first goleveldb attempt is intentionally excluded from the comparison. It
used an older `cosmos-db` pin and failed to build because that pin called
`SyncCommandWAL` against a newer TreeDB adapter API. The accepted goleveldb
attempt rebuilt with `cosmos-db@6ddcb75557e5`.

## Accepted Results

| Backend | Accepted attempt | Backend verification | Load-window tx | Load-window seconds | Load-window TPS | Wall TPS |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| goleveldb | 2 | app `goleveldb`, node `goleveldb` | 199,497 | 338.03 | 590.18 | 309.96 |
| TreeDB | 1 | app `treedb`, node `treedb` | 199,498 | 403.03 | 495.00 | 340.72 |

TreeDB is 16.1% lower on load-window TPS for this post-M2 plain-send workload.
The wall-TPS number is not the primary comparison because the accepted goleveldb
run rebuilt the Docker image in the measured wall interval while the TreeDB run
used `-skip-build`.

## Runtime Breakdown

These buckets are diagnostic, not an additive partition of the 65s workload
delta. Commit, FinalizeBlock, and CheckTx are observed from different runtime
scopes and should be read as overlapping signals rather than rows that sum
cleanly to the total gap.

| Bucket | goleveldb | TreeDB | TreeDB delta |
| --- | ---: | ---: | ---: |
| Workload runtime | 338.03s | 403.03s | +65.00s |
| Observed ABCI | 225.07s | 266.75s | +41.68s |
| Commit | 36.47s | 56.35s | +19.88s |
| FinalizeBlock | 56.11s | 66.67s | +10.57s |
| CheckTx | 130.45s | 141.68s | +11.22s |
| Non-ABCI workload | 112.38s | 134.96s | +22.58s |
| Process CPU delta | 799.99s | 731.98s | -68.01s |

Commit is worse for TreeDB, but it is not the whole problem. TreeDB's average
commit is 61.25ms versus 37.63ms for goleveldb, and the theoretical max
runtime speedup if TreeDB commit became free is only 1.165x. That means a pure
commit optimization cannot explain or recover the entire plain-send gap unless
it also reduces downstream GC, memory, or scheduler effects.

## Resource Footprint

| Resource | goleveldb | TreeDB | TreeDB delta |
| --- | ---: | ---: | ---: |
| Validator max Docker memory | 2.07 GiB | 7.25 GiB | +5.18 GiB |
| Validator process RSS after | 2.03 GiB | 7.68 GiB | +5.65 GiB |
| Validator Docker block write | 33.2 GB | 16.8 GB | -16.4 GB |
| Data dir delta | 1.20 GiB | 5.96 GiB | +4.76 GiB |
| Application DB delta | 649 MiB | 2.00 GiB | +1.37 GiB |

This run still shows TreeDB doing less Docker block write than goleveldb, but
using far more resident memory and allocating far more heap over the profile
window.

## CPU Profile

goleveldb CPU profile:

| Area | Signal |
| --- | --- |
| LevelDB compaction | `leveldb.(*tableCompactionBuilder).run` 214.63s cumulative |
| LevelDB transaction/compaction | `leveldb.(*DB).compactionTransact` 221.27s cumulative |
| Snappy | `snappy.encodeBlock` 48.96s flat, 51.39s cumulative |
| Runtime GC | `runtime.gcDrain` 234.00s cumulative |
| SDK/crypto | secp256k1 and ante-handler work remain large shared costs |

TreeDB CPU profile:

| Area | Signal |
| --- | --- |
| Runtime GC | `runtime.gcDrain` 244.32s cumulative; `runtime.scanSpan` 159.91s cumulative |
| TreeDB zipper/apply | `zipper.(*Zipper).mergeLeaf` 53.30s cumulative; apply worker pool about 63s cumulative |
| TreeDB write path | `caching.(*Batch).writeRegularLocked` 16.65s cumulative; `commandWALPublicBatch.write` about 16.7s cumulative |
| TreeDB value log/compression | `valuelog.(*FramePreparer).prepareBlockFrameBody` 20.34s cumulative; `encodeBlockPayloadWithScratch` 20.49s cumulative |
| Sync/checkpoint | `Checkpoint` and pager sync are visible but no longer dominant |

The old "every write forces an expensive checkpoint" hypothesis is not the
dominant profile shape anymore. The visible TreeDB problem is allocation and
GC pressure around write/flush/commit work.

## Heap Profile

| Alloc-space metric | goleveldb | TreeDB |
| --- | ---: | ---: |
| Total alloc_space | 117,664 MB | 199,948 MB |
| Total alloc_objects | 1.368B | 1.335B |

The object counts are similar, but TreeDB allocates much more total space.

Top TreeDB alloc-space sites:

| Site | Flat alloc_space | Share |
| --- | ---: | ---: |
| `TreeDB/internal/memtable.getAppendOnlyEntries` | 59,746 MB | 29.88% |
| `TreeDB/zipper.(*ReadOnlyPrepareResult).cloneKey` | 11,668 MB | 5.84% |
| `TreeDB/caching.getEntrySlice` | 7,997 MB | 4.00% |
| `TreeDB/compress/zstd.(*fastBase).ensureHist` | 5,187 MB | 2.59% |
| `TreeDB/internal/valuelog.getWriterAppendBuf` | 4,181 MB | 2.09% |
| `RawKVBatchPayloadBuilder.appendRawKVPayloadSpace` | 3,005 MB | 1.50% |
| `commitlog.(*Reader).readSegmentPayload` | 2,383 MB | 1.19% |
| `commitlog.DecodeCommandFrame` | 2,336 MB | 1.17% |

Top goleveldb alloc-space sites are a mix of SDK/cachekv/IAVL allocation plus
LevelDB buffer, batch, table writer, and compaction allocation. goleveldb pays
the expected compaction/snappy costs; TreeDB avoids those but currently pays a
large memtable/reset and value-log/compression allocation tax.

## Interpretation

The post-M2 optimization loop removed the narrow command-WAL setup allocation
bugs found in focused TreeDB profiling, but did not make TreeDB faster than
goleveldb on this normal plain-send Ironbird workload.

The next high-confidence TreeDB target is not another commit fsync policy
change. It is reducing heap churn in the TreeDB write/flush/commit path:

- `getAppendOnlyEntries` is the single largest flat allocation site.
- `getEntrySlice`, append-only reset/replacement, and mutable memtable
  construction are large cumulative allocation sites.
- The high RSS aligns with the alloc-space profile and with TreeDB's slower
  load-window result despite lower process CPU seconds.

There is also an instrumentation caveat. The Cosmos module timing for TreeDB
reports `distribution` begin-blocker at about 1,957s total, much larger than
exclusive wall time. That should be treated as a useful anomaly to investigate,
not as exclusive elapsed time. It may reflect timer semantics, repeated
iterator work, or DB-sensitive reads hidden inside module execution.

## Next Optimization Issue

Create and execute the next tracker child under `snissn/gomap#3475`:

1. Reduce TreeDB memtable/reset allocation pressure in commit-heavy plain-send.
2. Preserve WAL/value-log durability and the no-slab persistent value-log model.
3. Add a focused microbenchmark or profile gate for repeated `WriteSync` /
   commit-like batch shapes.
4. Rerun the same Ironbird plain-send A/B after the fix lands on `main`.
