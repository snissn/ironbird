# Ironbird TreeDB Empty-Batch Checkpoint Fix

Date: 2026-07-09 HST

## Conclusion

The dominant low-fanout TreeDB slowdown was a command-WAL migration bug, not
normal TreeDB commit cost and not a need to weaken durability.

An empty public `Batch.WriteSync()` was delegated to the cached layer. Because
command-WAL mode intentionally disables the cached redo journal, that cached
strict-sync fallback performed a full `Checkpoint()`. Celestia Core emits empty
synchronous batches in both its blockstore and transaction-index paths, so the
fallback turned hundreds of ordinary WAL barriers into backend checkpoints.

The fix gives an empty command-WAL batch a real durability-barrier path. It
first syncs pending value-log references, then syncs the command WAL under the
same publish barrier. Durable mode still reaches `fsync`; relaxed mode retains
its flush-to-kernel contract. `Checkpoint()` remains strict and unchanged.

The accepted fix result is:

- 223,995 successful transactions in 315.024 seconds.
- 711.04 load-window TPS.
- 15 blockstore checkpoints and 15 tx-index checkpoints over the debug-vars
  collection interval, down from 443 and 440 in the TreeDB baseline.
- Celestia `SaveTxInfo` fell from 66.90 ms/block to 5.07 ms/block.
- Consensus commit fell from 284.87 ms/block to 196.22 ms/block.

This removes the large measured TreeDB regression. Remaining TreeDB-owned
per-block costs are much smaller and should be evaluated with repeated paired
runs before another optimization sprint.

## Revisions

| Component | Revision | Purpose |
| --- | --- | --- |
| TreeDB baseline | `gomap` `2182e84bd668` | pre-fix comparison |
| TreeDB candidate | `gomap` `9cd9c6874860` | merged [`snissn/gomap` PR #3651](https://github.com/snissn/gomap/pull/3651), including value-log-before-WAL ordering |
| Celestia Core instrumentation | `snissn/cometbft` `87379c903cc8` | merged [`snissn/cometbft` PR #1](https://github.com/snissn/cometbft/pull/1) with commit, blockstore, state, and tx-index spans |
| Ironbird runner | branch `codex/exact-cadence-spans-next` | dependency pin, accepted-window collection, report rendering |

The accepted candidate image recorded the exact PR-head pseudo-version:

```text
v0.6.2-0.20260709223945-138e6dff7ef7
```

`snissn/gomap` PR #3651 was squash-merged as `9cd9c6874860` with pseudo-version
`v0.6.2-0.20260709230517-9cd9c6874860`. A post-merge tree comparison is empty,
so the accepted benchmarked source tree is byte-for-byte equivalent to the
merged commit now pinned by Ironbird.

The accepted run used Comet instrumentation head `7453f4166fa9`. Review then
changed only how tx-index collectors are obtained: merged commit
`87379c903cc8` routes them through the existing node metrics provider. The
default Prometheus provider used by the benchmark retains the same collectors
and measurements, and Ironbird now pins the merged commit.

## Workload And Acceptance

The accepted candidate used one validator, no non-validator nodes, TreeDB for
the Cosmos application DB and every CometBFT DB, the `kv` transaction indexer,
105,000 genesis accounts, 5,000 active wallets, 500 plain `MsgSend`
transactions per requested block, and a 450-block request ceiling.

The evidence gate required 99.5% of intended transactions and at least five
minutes of app-metric load-window time. Raw transaction auditing was disabled
so Catalyst could stop at the accepted app-metric boundary.

```sh
/mnt/fast4tb/bin/ironbird-local-report-runner \
  -scenario simapp-treedb-all \
  -validators 1 -nodes 0 -wallets 5000 \
  -preseed-profile accounts -preseed-accounts 100000 \
  -cosmos-txs 500 -cosmos-blocks 450 \
  -tx-indexer kv \
  -load-window-min-duration 5m \
  -load-window-target-fraction 0.995 \
  -stop-catalyst-after-load-window \
  -app-debug-vars -raw-tx-audit=false \
  -out /mnt/fast4tb/ironbird-empty-command-wal-fix-20260709T1242HST/treedb-kv-final-head/result.json
```

The host was checked before and during the accepted run. Old Petri/Ironbird
containers from prior experiments were removed before measurement; the load
window ran with only the tested validator and Catalyst containers in this
workload family.

## Accepted Results

The LevelDB and pre-fix TreeDB rows are the preserved plain-send rows from the
same exact-span instrumentation pass. The candidate is the accepted latest-head
run above.

| Metric | goleveldb baseline | TreeDB `2182e84` | TreeDB `138e6df` |
| --- | ---: | ---: | ---: |
| Successful transactions | 199,323 | 199,630 | 223,995 |
| Load-window seconds | 300.540 | 337.525 | 315.024 |
| Load-window TPS | 663.22 | 591.45 | 711.04 |
| Mean block interval | 466.76 ms | 547.32 ms | 387.57 ms |
| Consensus commit | 189.97 ms/block | 284.87 ms/block | 196.22 ms/block |
| Commit blockstore `SaveBlock` | 13.71 ms/block | 29.41 ms/block | 20.79 ms/block |
| State `ApplyVerifiedBlock` | 162.89 ms/block | 245.83 ms/block | 168.46 ms/block |
| Celestia `SaveTxInfo` | 6.81 ms/block | 66.90 ms/block | 5.07 ms/block |
| State block commit | 67.80 ms/block | 75.39 ms/block | 79.63 ms/block |
| ABCI app commit | 34.25 ms/block | 51.61 ms/block | 39.47 ms/block |
| State save | 12.59 ms/block | 27.65 ms/block | 19.75 ms/block |
| Async tx-index block total | 35.87 ms/block | 247.22 ms/block | 48.88 ms/block |

Relative to the preserved TreeDB baseline, the latest candidate improved
load-window TPS by 20.2%, reduced `SaveTxInfo` by 92.4%, reduced consensus
commit by 31.1%, and reduced asynchronous tx-index time by 80.2% per block.
The candidate also exceeded this single goleveldb row by 7.2% TPS, but that
cross-backend difference should be treated as directional because block
cadence and mempool scheduling vary between runs.

## Accepted-Window Attribution

The final candidate processed 1,170 measured blocks at 191.61 transactions per
block. The validator used 512.39 process-CPU seconds during the 315.024-second
window, or 1.63 core equivalents.

The current ABCI activity intervals are scrape-bounded, not event-exact. A
method counter change marks the entire scrape interval as active. Therefore the
311.998-second ABCI interval union is an upper bound on ABCI-busy wall time and
the 3.025-second interval-derived non-ABCI value is only a lower bound. The
independent 77.915-second residual from subtracting summed ABCI method durations
is also approximate because method execution can overlap. Neither value should
be presented as exact non-ABCI wall time.

| Accepted-window quantity | Value | Interpretation |
| --- | ---: | --- |
| Load window | 315.024 s | accepted app-metric boundary |
| Summed ABCI method durations | 237.109 s | concurrent duration counters; not wall-time union |
| Scrape-bounded ABCI interval union | 311.998 s | upper bound on ABCI-busy wall time |
| Interval-derived non-ABCI | 3.025 s | lower bound, not an exact measurement |
| Sum-derived non-ABCI residual | 77.915 s | approximate residual, not an exclusive bucket |
| Mean block interval | 387.57 ms | measured cadence |

The exact CometBFT stage spans do resolve the block-commit path. Rows below are
nested where indicated and must not be summed across parent/child boundaries.
The asynchronous tx index can overlap consensus execution.

| Stage | Mean per block | Relationship |
| --- | ---: | --- |
| Consensus finalize-commit | 196.22 ms | parent |
| Blockstore `SaveBlock` | 20.79 ms | consensus child |
| Consensus end-height WAL | 6.78 ms | consensus child |
| `ApplyVerifiedBlock` | 168.46 ms | consensus child and state parent |
| `FinalizeBlock` | 51.64 ms | apply child |
| Save finalize response | 8.53 ms | apply child |
| Celestia `SaveTxInfo` | 5.07 ms | apply child |
| State block commit | 79.63 ms | apply child |
| Mempool lock wait | 25.02 ms | block-commit child |
| Mempool lock held | 54.61 ms | block-commit child; includes the next two rows |
| ABCI app commit | 39.47 ms | lock-held child |
| Mempool update | 15.11 ms | lock-held child |
| State save | 19.75 ms | apply child |
| Event publication | 2.09 ms | apply child |
| Async tx-index total | 48.88 ms | overlapping asynchronous parent |
| Tx-index block write | 16.56 ms | async tx-index child |
| Tx-index transaction write | 29.67 ms | async tx-index child |

## TreeDB Counter Evidence

The following counters use each result's before/after TreeDB debug-vars
collection interval. Absolute checkpoint counts include normal automatic and
shutdown activity, so the meaningful signal is the collapse from hundreds of
checkpoints to low double digits despite the candidate processing more blocks.

| Store | Metric | TreeDB `2182e84` | TreeDB `138e6df` |
| --- | --- | ---: | ---: |
| blockstore | checkpoint runs | 443 | 15 |
| blockstore | checkpoint time | 55.48 s | 4.21 s |
| tx index | checkpoint runs | 440 | 15 |
| tx index | checkpoint time | 178.03 s | 42.70 s |
| application | checkpoint runs | 17 | 14 |
| application | checkpoint time | 4.27 s | 5.37 s |
| state | checkpoint runs | 16 | 10 |
| state | checkpoint time | 2.68 s | 1.38 s |

The pre-fix accepted load window made 1,730 blockstore `WriteSync` calls but
only 1,309 command-WAL appends. The approximately 421 empty synchronous batches
aligned with 436 load-window blockstore checkpoints, of which only nine were
normal automatic checkpoints. The tx-index path had the same shape.

After the fix, empty batches still exist, but they increase command-WAL sync
counters rather than checkpoint counters. In the final full debug-vars
interval, blockstore recorded 2,340 `WriteSync` calls, 1,848 command appends,
and 2,352 command-WAL syncs while performing only 15 checkpoints. Tx index
recorded 2,342 writes, 1,849 appends, 2,354 WAL syncs, and 15 checkpoints.
Ten checkpoints in each store were normal automatic checkpoints. The tx-index
checkpoint duration therefore remains a maintenance/dwell signal, but the low
run count proves it is no longer a per-block strict-sync fallback.

## Root Cause

The public command-WAL batch wrapper has two paths:

1. Dirty batch: append one command-WAL record and apply the cached batch.
2. Empty batch: no command record is required, but `WriteSync` must still act
   as a durability barrier for earlier command-WAL writes.

Before the fix, path 2 called the inner cached batch's `WriteSync`. That layer
cannot sync its own redo journal because command-WAL mode deliberately opens it
with the cached journal disabled. Its strict fallback is a full checkpoint.

`snissn/gomap` PR #3651 replaces only that empty-batch delegation with a backend command-WAL
barrier. The barrier holds command-WAL publication ordering, drains pending
value-log references first, and only then flushes or syncs the command WAL.
This is the correct public contract boundary:

- durable command-WAL mode calls the command journal's `Sync()`;
- relaxed command-WAL mode calls `Flush()`;
- value-log bytes referenced by earlier command records are durable before the
  command-WAL barrier returns;
- dirty batch behavior is unchanged;
- explicit `Checkpoint()` behavior is unchanged;
- no adapter, on-disk-format, or recovery semantic change is required.

## Validation

Local validation on the latest PR tree:

```sh
GOWORK=off go test ./TreeDB \
  -run 'TestPublicCommandWALDurableEmptyBatchWriteSyncOnlySyncsCommandWAL|TestPublicCommandWALDurableBatchWriteSync|TestPublicCommandWALRelaxedBatchWriteSync' \
  -count=1
GOWORK=off go test -race ./TreeDB \
  -run TestPublicCommandWALDurableEmptyBatchWriteSyncOnlySyncsCommandWAL \
  -count=1
GOWORK=off go test ./TreeDB/db ./TreeDB/caching ./TreeDB -count=1
GOWORK=off go vet ./TreeDB/db ./TreeDB/caching ./TreeDB
```

The focused tests, focused race tests, full `TreeDB/db`, `TreeDB/caching`, and
`TreeDB` package suites, and focused `go vet` pass. The public regression test
verifies one public sync call, one command-WAL sync, no command append, and no
checkpoint for an empty durable batch. Backend and caching tests additionally
verify that an empty external-reference set drains every pending value-log lane
before the command WAL is synced.

## Remaining Cost

The large checkpoint pathology is resolved. Against the preserved goleveldb
row, the latest TreeDB row still spends approximately:

- 7.1 ms/block more in synchronous blockstore `SaveBlock`;
- 5.2 ms/block more in ABCI app commit;
- 7.2 ms/block more in post-commit state save;
- 13.0 ms/block more in asynchronous tx indexing.

Those stages overlap differently with mempool and consensus work, and the
latest TreeDB row is not slower end to end. They are candidates for repeated
paired measurement, not evidence of another large bottleneck. The next
performance decision should therefore be based on multiple alternating
LevelDB/TreeDB pairs at this merged gomap revision rather than another broad
instrumentation sprint.

## Artifacts

```text
/mnt/fast4tb/ironbird-exact-save-tx-info-20260709T1120HST/goleveldb-kv/result.json
/mnt/fast4tb/ironbird-exact-save-tx-info-20260709T1120HST/treedb-kv/result.json
/mnt/fast4tb/ironbird-empty-command-wal-fix-20260709T1148HST/treedb-kv-accepted/result.json
/mnt/fast4tb/ironbird-empty-command-wal-fix-20260709T1209HST/treedb-kv-latest-head/result.json
/mnt/fast4tb/ironbird-empty-command-wal-fix-20260709T1242HST/treedb-kv-final-head/result.json
```
