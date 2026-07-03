# Ironbird Large Throughput Report: TreeDB vs goleveldb

Generated: 2026-07-03

Branch: `codex/treedb-ironbird-report`

Ironbird commit: `40323ff205f639a5f10cd3f1b9a3c7ebc8bf98b2`

Artifact directory: `reports/artifacts/large-throughput-20260703T184435Z`

## Executive Summary

This run realigned the Ironbird TreeDB/goleveldb comparison away from short burst
TPS and toward sustained app-observed inclusion windows. The benchmark runner now
has an explicit `-load-window-min-duration` acceptance gate so a high-throughput
configuration that finishes in roughly one second is rejected as too short for
final evidence.

The sustained A/B run used a Cosmos SDK simapp workload with:

- 1 validator, 0 full nodes.
- 100,000 preseeded accounts and 5,000 active wallets.
- 5,000 submitted transactions.
- Each transaction containing 20 `MsgMultiSend` messages.
- Each `MsgMultiSend` using 25 recipients.
- 2,500,000 effective recipient operations.

The accepted sustained-window result:

- goleveldb: 5,000 app-successful txs in 400.55s, or 12.48 included tx/s.
- TreeDB: 5,000 app-successful txs in 336.06s, or 14.88 included tx/s.
- TreeDB was 1.19x faster on the sustained app-success window.
- In effective recipient operations, goleveldb reached 6,241 ops/s and TreeDB
  reached 7,439 ops/s.

This is a credible sustained-window comparison, not a one-second burst. It is
not yet a clean storage-engine ceiling benchmark. CPU profiles show the run is
still heavily affected by CometBFT/Catalyst result collection, tx indexing,
blockstore lookup, JSON/protobuf work, and Go runtime/GC. Direct app-store focus
accounts for only a small fraction of sampled CPU. The current evidence supports
a modest TreeDB win in this workload shape, plus a clear next harness-improvement
target before claiming a storage-limited TPS ceiling.

## Duration Validity Rule

The benchmark plan now requires enough load-window time to make throughput
meaningful. The runner accepts short runs for calibration, but final A/B evidence
must satisfy a sustained app-observed load window. For this run the target was at
least 2 minutes where supported by the runner flag.

Implemented runner changes:

- Added `-load-window-min-duration`.
- Added `minimum_seconds` and `duration_satisfied` fields to load-window
  observations.
- Annotated short reached windows with an error:
  `load window reached in ... below minimum duration ...; increase offered workload`.
- Only exports `load_window_*` headline throughput metrics when the window is
  reached and the duration requirement is satisfied.
- Added unit coverage:
  - `TestDeriveMetricsSkipsShortLoadWindow`
  - `TestLoadWindowAcceptedAllowsZeroMinimum`

Validation:

```sh
GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./activities/testnet ./petri/core/provider/docker ./petri/cosmos/chain ./messages
GOWORK=off go build -o /mnt/fast4tb/tmp/local-report-runner-clean ./cmd/local-report-runner
```

Note: the goleveldb sustained run was started before the minimum-duration flag
was added, so its JSON does not contain `minimum_seconds`. Its observed load
window was 400.55s, which manually satisfies the same 2-minute requirement.
The TreeDB sustained run used `-load-window-min-duration=2m` and recorded
`duration_satisfied=true`.

## Commands

goleveldb sustained run:

```sh
TMPDIR=/mnt/fast4tb/tmp /mnt/fast4tb/tmp/local-report-runner-clean \
  -scenario simapp-goleveldb \
  -skip-build \
  -validators 1 \
  -nodes 0 \
  -wallets 5000 \
  -preseed-profile accounts \
  -preseed-accounts 100000 \
  -cosmos-msg MsgArr \
  -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 \
  -cosmos-recipients 25 \
  -cosmos-blocks 20 \
  -cosmos-txs 250 \
  -cosmos-max-gas 5000000000 \
  -raw-tx-audit=false \
  -load-window-drain-timeout=10m \
  -app-cpuprofile-dir reports/artifacts/large-throughput-20260703T184435Z/pprof \
  -out reports/artifacts/large-throughput-20260703T184435Z/simapp-goleveldb-large-drain.json
```

TreeDB sustained run:

```sh
TMPDIR=/mnt/fast4tb/tmp /mnt/fast4tb/tmp/local-report-runner-clean \
  -scenario simapp-treedb \
  -skip-build \
  -validators 1 \
  -nodes 0 \
  -wallets 5000 \
  -preseed-profile accounts \
  -preseed-accounts 100000 \
  -cosmos-msg MsgArr \
  -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 \
  -cosmos-recipients 25 \
  -cosmos-blocks 20 \
  -cosmos-txs 250 \
  -cosmos-max-gas 5000000000 \
  -raw-tx-audit=false \
  -load-window-min-duration=2m \
  -load-window-drain-timeout=15m \
  -app-cpuprofile-dir reports/artifacts/large-throughput-20260703T184435Z/pprof \
  -out reports/artifacts/large-throughput-20260703T184435Z/simapp-treedb-large-drain-min2m.json
```

Both scenarios used the same SDK fork and dependency pins:

- `github.com/snissn/celestia-cosmos-sdk` at `28e5525fefe7aaa53d4726ef7a367242bacf9003`
- `github.com/snissn/cosmos-db` at `f1d8b016a90cc39abde5d362e4f6b54b96df5c73`
- `github.com/snissn/gomap` at `1afe86c1cbc0acc7336fc0944c69ebfcd2f3dc8d`
- `github.com/snissn/iavl` pseudo-version `v0.0.0-20260701072929-12a26715119b`

## Throughput Results

| Metric | goleveldb | TreeDB | Direction |
| --- | ---: | ---: | --- |
| Wall time | 586.69s | 520.55s | TreeDB 1.13x shorter |
| Load-window duration | 400.55s | 336.06s | TreeDB 1.19x shorter |
| Load-window included tx/s | 12.48 | 14.88 | TreeDB 1.19x higher |
| Effective operations | 2,500,000 | 2,500,000 | same |
| Load-window effective ops/s | 6,241 | 7,439 | TreeDB 1.19x higher |
| Load-generator runtime tx/s | 67.27 | 66.43 | roughly same |
| Load-generator runtime effective ops/s | 33,634 | 33,214 | roughly same |

The headline sustained metric should be the app-observed load-window throughput,
not the load-generator runtime. Catalyst's own runtime measures send-side work
and then its collector reports all transactions as failed because `/tx` lookup
collection diverges from app metrics in this run. The runner therefore corrects
final successful counts from app metrics.

Important count caveat:

- Both scenarios reached `sdk_tx_count_delta=5000` and
  `sdk_tx_successful_delta=5000`.
- goleveldb consensus/mempool deltas were 4,536.
- TreeDB consensus/mempool deltas were 4,407.
- Catalyst logs reported 5,000 failed transactions in both scenarios.

The report therefore uses app metrics for successful transaction count and uses
the sustained app-success window as the comparison interval.

## Runtime Categories

ABCI method timing is validator-observed accumulated method time. It is not a
strict wall-clock partition and can exceed the load generator's measured send
runtime because the validator continues processing and because method timings
are collected from app/CometBFT metrics.

| Metric | goleveldb | TreeDB | Direction |
| --- | ---: | ---: | --- |
| Observed ABCI time | 140.62s | 119.76s | TreeDB lower |
| Commit time | 6.29s | 9.17s | goleveldb lower |
| Commit count | 178 | 148 | TreeDB used fewer commits |
| Avg commit | 35.4ms | 61.9ms | goleveldb lower |
| FinalizeBlock time | 58.01s | 56.15s | similar |
| Avg FinalizeBlock | 324.1ms | 376.8ms | goleveldb lower |
| CheckTx time | 32.89s | 28.77s | TreeDB lower |
| Proposal time | 15.69s | 11.34s | TreeDB lower |
| Query time | 27.73s | 14.32s | TreeDB lower |

Commit is visible, but it is not large enough to explain the whole benchmark:

- goleveldb commit was 6.29s out of 400.55s load-window time.
- TreeDB commit was 9.17s out of 336.06s load-window time.
- Even making TreeDB commit free would save about 9s on a 336s sustained window,
  or roughly 2.7% of that window.
- The runner's runtime model estimated a theoretical 1.14x speedup if TreeDB
  commit were free, but that model is based on send-runtime categories and should
  be treated as directional, not as a wall-clock bound.

Conclusion: TreeDB commit is slower here and worth monitoring, but optimizing
commit alone is unlikely to produce an order-of-magnitude TPS improvement in
this Ironbird shape.

## Storage And Disk Footprint

| Metric | goleveldb | TreeDB | Direction |
| --- | ---: | ---: | --- |
| `application.db` delta | 74.37 MB | 98.73 MB | goleveldb smaller |
| Total `/simd/data` delta | 1.55 GB | 1.56 GB | similar |
| Validator max block read | 1.08 GB | 211 MB | TreeDB lower |
| Validator max block write | 5.78 GB | 3.98 GB | TreeDB lower |
| Validator max memory | 7.58 GiB | 8.27 GiB | goleveldb lower |
| Validator max CPU | 595% | 614% | similar |

TreeDB used more app DB bytes in this workload but materially less observed
validator block I/O. That is an interesting category win, but this run still has
large non-app-state data growth: `tx_index.db`, `state.db`, and blockstore data
are substantial. Whole-node storage effects are not isolated from app-state DB
effects.

## Profiling Findings

CPU profiles:

- goleveldb profile:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-goleveldb-validator-0-cpu.pprof`
- TreeDB profile:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-treedb-validator-0-cpu.pprof`

Profile summaries:

- goleveldb CPU profile duration: 409.13s, total samples 925.11s, 226% CPU.
- TreeDB CPU profile duration: 344.24s, total samples 896.92s, 261% CPU.
- Both profiles are dominated by Go runtime/GC and memory movement:
  `runtime.spanClass.sizeclass`, `runtime.memmove`, `runtime.tryDeferToSpanScan`,
  `runtime.scanSpan`, `runtime.gcDrain`, `runtime.mallocgc`, and related paths.

TreeDB direct app-store focus:

- Store/app focus accounted for 18.21s, or 2.03% of TreeDB samples.
- Similar goleveldb store/app focus accounted for 13.15s, or 1.42% of goleveldb
  samples.
- Visible TreeDB symbols were small, for example
  `TreeDB/caching.(*DB).flushCanonicalPointUnits...` at about 1.39s cumulative
  and `flushAllLocked` at about 1.15s cumulative.

CometBFT/goleveldb focus inside the TreeDB run:

- A focused profile over CometBFT DB and `syndtr/goleveldb` accounted for
  257.06s, or 28.66% of TreeDB samples.
- Major cumulative paths:
  - `cometbft/state/txindex.(*IndexerService).OnStart.func1`: 141.47s
  - `state/txindex/kv.(*TxIndex).AddBatch`: 141.45s
  - `cometbft-db.(*goLevelDBBatch).Set`: 128.13s
  - `syndtr/goleveldb/leveldb.(*Batch).appendRec`: 128.77s
  - `syndtr/goleveldb/leveldb.(*Batch).grow`: 126.40s
  - `runtime.memmove`: 99.13s
  - CometBFT RPC `/tx` lookup paths: about 40.56s cumulative
  - Blockstore load paths: about 29.96s cumulative

This means a TreeDB app-state run still spends a lot of CPU in goleveldb because
CometBFT tx index, blockstore, and Catalyst result collection use separate
CometBFT DB paths. That is a harness and whole-node measurement issue, not a
TreeDB app-state hotspot.

## Harness Findings

The largest harness issue is Catalyst collection:

- Catalyst sends transactions and then performs result collection using tx
  lookups.
- The collector reports all 5,000 transactions as failed in both scenarios, even
  though app metrics show all 5,000 were included successfully.
- The collector drives CometBFT `/tx` and blockstore lookup paths, which pollutes
  CPU and I/O profiles with tx-index and blockstore work.

The runner mitigates this by:

- Allowing raw tx audit to be disabled with `-raw-tx-audit=false`.
- Correcting final transaction counts from app metrics.
- Waiting for an app-metric load window to reach the intended transaction count.
- Adding a drain period to avoid stopping before the validator catches up.
- Adding the new minimum-duration acceptance gate.

The remaining issue is that Catalyst still performs its own collector work
inside the load-test activity. The next harness fix should make the Cosmos load
test submit-only, or otherwise skip Catalyst's tx lookup collector and rely on
app metrics for inclusion and success.

## Interpretation

What TreeDB demonstrates here:

- A valid sustained app-success window win: 336s vs 401s for 5,000 app-successful
  transactions.
- Better load-window effective ops/s: 7,439 vs 6,241.
- Lower observed total validator block I/O.
- Lower observed ABCI time in several categories, especially query, proposal,
  and CheckTx.

What goleveldb demonstrates here:

- Faster average app commit in this workload: 35.4ms vs 61.9ms.
- Smaller app DB growth for this particular simapp MultiSend workload.
- Slightly lower memory high-water mark.

What this benchmark does not yet demonstrate:

- It does not show thousands of app-included tx/s.
- It does not isolate app-state DB as the dominant bottleneck.
- It does not reproduce the production Celestia sync regime where TreeDB has
  already shown much larger lift.
- It does not justify spending all optimization effort on TreeDB commit; commit
  is measurable but not dominant in the accepted sustained window.

## Recommended Next Work

1. Add a submit-only Cosmos load-test path.

   Bypass or patch Catalyst's result collector for this local runner. Use app
   metrics as the source of truth for inclusion, success, and sustained window
   timing. This should remove large `/tx`, tx-index, and blockstore lookup noise
   from the validator profile.

2. Add explicit CometBFT DB mode control.

   Decide whether a scenario measures only app-state DB or whole-node DB. If it
   measures app-state DB, reduce CometBFT tx-index/blockstore interference. If it
   measures whole-node DB, route CometBFT DB through the candidate database stack
   or report it as a separate dimension.

3. Continue using long-window gates.

   Keep `-load-window-min-duration` on final runs. For high-throughput regimes,
   scale transaction count so the accepted window remains at least 1-2 minutes.
   A reasonable policy is:

   - Calibration runs may be short.
   - Final A/B runs require `duration_satisfied=true`.
   - If a run reaches the target too quickly, increase `cosmos-txs`, blocks,
     wallets, or contained operations until the app-success window is long enough.

4. Profile after collector removal.

   Repeat the same A/B run after submit-only mode. If app-store focus rises from
   roughly 2% to a meaningful share, then TreeDB commit/write-path optimization
   becomes more actionable. If not, the limiting path is still SDK/module,
   CometBFT, or load-generation overhead.

5. Try a production-shaped app-state workload after harness cleanup.

   The simapp MultiSend workload is useful for high write amplification, but it
   is not Celestia production sync. A better next state shape is a seeded or
   imported production-like state followed by replay or synthetic activity on top
   of that state, with the same long-window acceptance rule.

## Artifact Index

- Raw goleveldb sustained result:
  `reports/artifacts/large-throughput-20260703T184435Z/simapp-goleveldb-large-drain.json`
- Raw TreeDB sustained result:
  `reports/artifacts/large-throughput-20260703T184435Z/simapp-treedb-large-drain-min2m.json`
- A/B comparison:
  `reports/artifacts/large-throughput-20260703T184435Z/large-ab-comparison.json`
- goleveldb CPU profile:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-goleveldb-validator-0-cpu.pprof`
- TreeDB CPU profile:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-treedb-validator-0-cpu.pprof`
- TreeDB CometBFT/goleveldb focused profile text:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-treedb_comet_leveldb_focus_cum.txt`
- TreeDB store focused profile text:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-treedb_store_focus_cum.txt`
- goleveldb store focused profile text:
  `reports/artifacts/large-throughput-20260703T184435Z/pprof/simapp-goleveldb_store_focus_cum.txt`
