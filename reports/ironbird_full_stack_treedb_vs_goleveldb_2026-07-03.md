# Ironbird Full-Stack TreeDB vs goleveldb Report

Date: 2026-07-03

## Summary

This run compares Ironbird simapp with goleveldb used across the app and
CometBFT node databases against TreeDB used across the same stack. The important
change versus earlier app-only runs is that `config/app.toml` and
`config/config.toml` are both verified before load is reported:

| Scenario | App DB | Node DB | Backend verification |
| --- | --- | --- | --- |
| `simapp-goleveldb-all` | goleveldb | goleveldb | valid |
| `simapp-treedb-all` | treedb | treedb | valid |

The full-stack TreeDB scenario completed the same 5,000 transaction workload at
30.66 corrected TPS versus 10.52 corrected TPS for goleveldb, a 2.92x runtime
throughput improvement. Wall-clock time improved 1.71x. Docker block-write
accounting was also materially lower for TreeDB: 4.51 GB versus 18.2 GB.

The tradeoff in this run is resource footprint. TreeDB used a higher peak RSS
and a larger final data directory:

- Peak validator RSS: 5.888 GiB TreeDB versus 2.974 GiB goleveldb.
- `/simd/data` delta: 1,135.1 MiB TreeDB versus 639.8 MiB goleveldb.
- `/simd/data/application.db` delta: 114.6 MiB TreeDB versus 82.8 MiB goleveldb.

## Scope

This is a fork-local Ironbird report for `snissn/ironbird`. It does not open or
claim readiness for public upstream Celestia, Cosmos SDK, CometBFT, or Ironbird
PRs.

The benchmark uses the `snissn` dependency chain already prepared for TreeDB:

| Module | Version/ref |
| --- | --- |
| `github.com/cosmos/cosmos-db` | `v0.0.0-20260702024646-f1d8b016a90c` / `f1d8b016a90cc39abde5d362e4f6b54b96df5c73` |
| `github.com/snissn/gomap` | `v0.6.2-0.20260702024414-1afe86c1cbc0` / `1afe86c1cbc0acc7336fc0944c69ebfcd2f3dc8d` |
| `github.com/cosmos/iavl` | `v0.0.0-20260701072929-12a26715119b` / `12a26715119bb3ea55289ffd7b256161effc7b8b` |
| `github.com/cometbft/cometbft-db` | `v0.0.0-20260701074104-b4f87847a725` / `b4f87847a725f92a046d927ce4a0f5b08b965995` |

Docker image tag:

```text
ironbird-report:snissn-sdk-28e5525f-fullstack-cosmosdb-f1d8b01-cometdb-b4f878-gomap-1afe86c
```

## Workload

Both runs used the same local-report-runner workload:

```sh
TMPDIR=/mnt/fast4tb/tmp /mnt/fast4tb/tmp/local-report-runner-full-stack \
  -scenario <simapp-goleveldb-all|simapp-treedb-all> \
  -skip-build \
  -validators 1 -nodes 0 -wallets 5000 \
  -preseed-profile accounts -preseed-accounts 100000 \
  -cosmos-blocks 20 -cosmos-txs 250 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 -cosmos-recipients 25 \
  -cosmos-max-gas 300000000 \
  -load-window-min-duration=2m \
  -load-window-drain-timeout=15m \
  -raw-tx-audit=false \
  -app-cpuprofile-dir reports/artifacts/full-stack-benchmark-20260703T215205Z/pprof/<backend> \
  -app-heapprofile-dir reports/artifacts/full-stack-benchmark-20260703T215205Z/pprof/<backend> \
  -out reports/artifacts/full-stack-benchmark-20260703T215205Z/<backend>.json
```

Each successful transaction contains 20 `MsgMultiSend` messages with 25
recipients each, so the report also includes an "effective operations" view:

```text
successful transactions * 20 contained messages * 25 recipients
```

The saved artifacts are under:

```text
reports/artifacts/full-stack-benchmark-20260703T215205Z/
```

`reports/artifacts/` is intentionally ignored by git.

## Validation

Focused validation before the full run:

```sh
GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./activities/testnet ./petri/core/provider/docker ./petri/cosmos/chain ./messages
GOWORK=off go build -o /mnt/fast4tb/tmp/local-report-runner-full-stack ./cmd/local-report-runner
```

Both backend smoke checks launched successfully and verified their expected
backend configuration:

```sh
TMPDIR=/mnt/fast4tb/tmp /mnt/fast4tb/tmp/local-report-runner-full-stack \
  -scenario simapp-goleveldb-all -validators 1 -nodes 0 -wallets 10 \
  -commit-benchmark-blocks 1 -raw-tx-audit=false \
  -out reports/artifacts/full-stack-smoke/goleveldb-all-config-smoke.json

TMPDIR=/mnt/fast4tb/tmp /mnt/fast4tb/tmp/local-report-runner-full-stack \
  -scenario simapp-treedb-all -skip-build -validators 1 -nodes 0 -wallets 10 \
  -commit-benchmark-blocks 1 -raw-tx-audit=false \
  -out reports/artifacts/full-stack-smoke/treedb-all-config-smoke.json
```

## Results

| Metric | goleveldb all | TreeDB all | Ratio |
| --- | ---: | ---: | ---: |
| Corrected TPS | 10.52 | 30.66 | 2.92x TreeDB |
| Corrected runtime | 475.32 s | 163.06 s | 2.92x faster TreeDB |
| Load window | 469.53 s | 253.53 s | 1.85x faster TreeDB |
| Wall time | 739.24 s | 433.19 s | 1.71x faster TreeDB |
| Successful transactions | 5,000 | 5,000 | equal |
| Runtime effective ops/s | 5,259.60 | 15,331.77 | 2.92x TreeDB |
| Load-window effective ops/s | 5,324.45 | 9,860.84 | 1.85x TreeDB |
| Validator max CPU | 551.17% | 534.20% | similar |
| Validator peak RSS | 2.974 GiB | 5.888 GiB | 1.98x higher TreeDB |
| Validator last block I/O | 2.51 MB / 18.2 GB | 19 MB / 4.51 GB | 4.04x fewer writes TreeDB |
| `/simd/data` delta | 639.8 MiB | 1,135.1 MiB | 1.77x higher TreeDB |
| `application.db` delta | 82.8 MiB | 114.6 MiB | 1.38x higher TreeDB |
| CPU profile samples | 1,534.83 s | 729.41 s | 2.10x fewer TreeDB |
| Heap alloc-space profile | 452.36 GB | 303.24 GB | 1.49x fewer TreeDB |

The corrected load-test source is app/CometBFT metrics because Catalyst's raw
transaction audit still disagrees with the CometBFT response schema for this
scenario. The benchmark therefore ran with `-raw-tx-audit=false`, and both JSON
artifacts record:

```text
disabled by -raw-tx-audit=false
catalyst result differs from corrected counts
```

## Timing Signals

ABCI method timers are useful hot-path indicators, but they are not a literal
wall-clock partition. They can include concurrent work and RPC/query activity;
for TreeDB, the observed ABCI total exceeds workload runtime.

| ABCI metric | goleveldb all | TreeDB all |
| --- | ---: | ---: |
| Commit total | 9.80 s | 7.84 s |
| Commit count | 466 | 178 |
| Average commit | 21.03 ms | 44.02 ms |
| FinalizeBlock total | 45.95 s | 46.40 s |
| CheckTx total | 185.13 s | 148.41 s |
| Query total | 21.02 s | 129.04 s |
| Commit share of observed ABCI | 3.70% | 2.34% |

Commit is not the dominant optimization target in this run. TreeDB's average
commit is higher, but total commit time is lower and the theoretical runtime
speedup if commit were free is only about 5% for TreeDB in this workload. The
larger win came from avoiding goleveldb compaction and reducing total CPU,
allocation, and block-write pressure.

## Profile Highlights

The goleveldb CPU profile shows a large backend-specific compaction cost:

| goleveldb CPU cumulative frame | Cumulative time |
| --- | ---: |
| `leveldb.(*DB).compactionTransact` | 367.39 s |
| `leveldb.(*DB).tableCompaction` / `tableAutoCompaction` | 365.50 s |
| `leveldb.(*tableCompactionBuilder).run` | 365.41 s |
| `runtime.gcDrain` | 437.00 s |
| CometBFT JSON/RPC response path | about 242 to 288 s |
| `baseapp.(*BaseApp).runTx` | 212.19 s |

The TreeDB CPU profile shifts away from compaction. Remaining large costs are
mostly common harness/application costs, plus TreeDB read/write path work:

| TreeDB CPU cumulative frame | Cumulative time |
| --- | ---: |
| `runtime.gcDrain` | 195.60 s |
| CometBFT JSON/RPC response path | about 147 to 172 s |
| `baseapp.(*BaseApp).runTx` | 156.42 s |
| CometBFT consensus finalize/commit path | about 124 s |
| TreeDB/cometbft-db get/write/command-WAL frames | about 20 s each in the visible hot paths |

Alloc-space profiles show the same broad shape. Total allocation volume dropped
from 452.36 GB to 303.24 GB, but TreeDB still has optimization opportunities:

| TreeDB alloc-space frame | Allocated |
| --- | ---: |
| `TreeDB/internal/memtable.getAppendOnlyEntries` | 16.84 GB |
| `TreeDB/internal/valuelog.getDecodeScratch` | 14.57 GB |
| `TreeDB/internal/valuelog.decodeRecordToScratch` | 10.24 GB flat / 24.85 GB cumulative |
| `TreeDB/caching.(*Batch).SetWithRevision` | 8.45 GB |
| `TreeDB/internal/commitlog.(*Reader).readSegmentPayload` | 3.10 GB |
| `TreeDB/internal/commitlog.DecodeCommandFrame` | 3.10 GB |
| `TreeDB/caching.getEntrySlice` | 3.07 GB |

For goleveldb, the clearest backend-specific allocation cost is:

| goleveldb alloc-space frame | Allocated |
| --- | ---: |
| `leveldb.(*Batch).grow` | 57.84 GB flat / 61.94 GB cumulative via `appendRec` |
| `leveldb/table.(*Reader).find` | 21.10 GB |
| `leveldb/table.(*Writer).writeBlock` | 9.96 GB |

## Interpretation

This run achieves the immediate Ironbird goal: it finds a benchmark shape where
TreeDB, wired through both Cosmos app storage and CometBFT node databases,
shows a large throughput lift over goleveldb. The result is directionally
consistent with the earlier Celestia production sync evidence, where TreeDB was
materially faster than goleveldb under production-shaped state replay.

The earlier app-only Ironbird runs were incomplete for this question because
CometBFT node databases stayed on goleveldb. That made the comparison piecemeal
and kept goleveldb costs in blockstore/state/tx-index paths. The new scenarios
make the backend choice explicit and verify it from the generated node config.

The main caveats are:

- This is still a synthetic simapp workload, not a literal Celestia production
  replay or forked-live workload.
- The raw transaction audit remains disabled because Catalyst and the CometBFT
  response schema disagree; corrected counts come from app/CometBFT metrics.
- TreeDB's peak memory and final data footprint are higher in this run.
- ABCI timers should not be read as a strict wall-clock decomposition.

## Follow-Up Work

Recommended next work:

1. Fix Catalyst raw transaction auditing for this CometBFT response shape so
   future reports can use both raw audit and app metrics.
2. Investigate TreeDB allocation hot spots in `getAppendOnlyEntries`,
   value-log decode scratch, `Batch.SetWithRevision`, and commitlog decode.
3. Investigate TreeDB storage footprint in this workload, especially
   `state.db` and `tx_index.db`.
4. Repeat full-stack runs with more validators/nodes after the single-validator
   codepath remains stable.
5. Add a Celestia-shaped replay or state-seeded workload if Ironbird is kept as
   the public-facing comparison harness.
