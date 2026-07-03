# Ironbird TreeDB Reproduction And Bottleneck Report

Date: 2026-07-01 UTC.

Branch: `codex/treedb-ironbird-report` in `snissn/ironbird`.

This report uses Ironbird's Docker provider path to reproduce the Cosmos performance blog workload family, then pushes the local runner far enough to identify the current bottlenecks and create a higher-throughput TreeDB comparison lane.

## Executive Summary

The local Ironbird runner now has four useful benchmark lanes:

1. `evm-blog`: Cosmos EVM at the blog commit, using Catalyst `MsgNativeTransferERC20`.
2. `evm-blog` direct-validator mode: same EVM workload, but with zero full nodes so Catalyst sends to the validator RPC directly.
3. `simapp-goleveldb` versus `simapp-treedb`: `snissn/celestia-cosmos-sdk` simapp with `app-db-backend` switched between LevelDB and TreeDB.
4. `simapp` storage-fanout mode: `MsgArr(MsgMultiSend)` with raised block gas to push many account writes through each outer transaction.

Key current results:

| Question | Best current answer |
| --- | --- |
| Can Ironbird run the Cosmos blog EVM workload locally? | Yes. After the EVM account funding fix, the workload executes and reaches 528.2 runtime TPS through the original one-validator/one-full-node local topology. |
| What was the biggest EVM bottleneck? | The full-node RPC/txpool relay path. With one validator and one full node, a 2,000 tx attempt included only 728 tx despite no broadcast failures. Sending directly to the validator included all 10,000 tx at 1,000 runtime TPS. |
| Can the local EVM lane reach thousands of TPS? | It reaches 1,000 runtime TPS with full inclusion. A 2,000 offered-TPS run still reported 1,000 runtime TPS, with the validator near 977% CPU, 1.332 GiB memory, and 1.11 GB writes. The next ceiling is validator-side execution/processing, not fee config or Catalyst send CPU. |
| Do extra local validators help? | No in this local Docker setup. Four validators and no full nodes included only 14 of 5,000 attempted EVM transactions. |
| Did a different EVM workload help? | Native gas transfer was not a valid faster lane yet. It included 3,163 of 5,000 and hit Catalyst nonce/balance errors under load. |
| Did an EVM custom-write contract help? | Not yet. Catalyst exposes `MsgWriteTo`, but interval mode drops the requested iteration count during baseline generation, and the block-cadence probe ended before submitted txs landed in the collection window. A contract ERC20 lane executed 200/200 successfully at 200 TPS but is still gas-light and not a storage-limited signal. |
| Can a non-EVM storage-fanout lane reach thousands of effective operations/sec? | Yes. With `MsgArr(MsgMultiSend)`, 300M block gas, and raw CometBFT auditing, LevelDB reached 1,698.48 effective recipient ops/s and TreeDB reached 1,708.02 effective recipient ops/s. |
| Did TreeDB beat LevelDB? | Not on throughput. In the packed SDK `MsgArr` lane, TreeDB was effectively tied with LevelDB: 753.2 versus 750.8 effective contained messages/sec, within noise. TreeDB wrote less data at the Docker block-write sample high-water, but used more memory. |

The TreeDB result should be described narrowly: the fork is consumable by Ironbird, the runner can now push a storage-fanout SDK proxy above 1,700 effective operations/sec, and short TreeDB runs are roughly tied with LevelDB. This does not prove a durable throughput improvement. The exact Cosmos EVM plus TreeDB comparison remains blocked until a TreeDB-enabled Cosmos EVM app fork exists.

## Blog Target

Source: [Performance Testing in Cosmos](https://cosmos.network/blog/performance-testing-in-cosmos).

Relevant blog target:

- Workload family: ERC20 or gas-token sends.
- Tooling: Ironbird and Catalyst.
- Published scale: production-grade 10-50 node networks, persistent RPC traffic through non-validating nodes, and 32 vCPU / 32 GB RAM class machines.
- Published throughput: the post describes 2,000 tx/s load tests yielding roughly 1,500 tx/s sustained throughput, and chains running around 1,800 tx/s.

This local report uses the same Ironbird/Petri/Catalyst family and the same Cosmos EVM `MsgNativeTransferERC20` workload type, but it is not the same cloud topology. Local Docker results are a bottleneck map and regression target for this host, not a claim that the blog topology has been fully reproduced.

## Local Environment

Host metadata captured in the artifact set:

| Field | Value |
| --- | --- |
| CPU | 12 logical CPUs, 11th Gen Intel Core i5-11400F @ 2.60GHz |
| Memory | 31 GiB |
| Docker | Docker version 29.1.3 |
| Go | go1.25.0 linux/amd64 |
| Fast temp disk | `/mnt/fast4tb`, used via `TMPDIR=/mnt/fast4tb/tmp` |
| Ironbird HEAD | `40323ff205f639a5f10cd3f1b9a3c7ebc8bf98b2` plus local branch changes |

Noisy-host caveat: this machine had other benchmark and system activity during the broader investigation. Treat small LevelDB/TreeDB deltas as noise unless they are repeated under a quieter controlled run.

## Harness Changes

The stock repository expected the Temporal worker path. This machine did not have the full Temporal runner prerequisites, so this branch adds a local Docker report runner that calls the same activity layer directly.

Changed or added paths:

| Path | Purpose |
| --- | --- |
| `cmd/local-report-runner/main.go` | Local scenario runner for `evm-blog`, `simapp-goleveldb`, and `simapp-treedb`; adds direct-validator topology controls, EVM fee/message flags, EVM block-cadence and initial-contract controls, `MsgArr` workload flags, Cosmos max-gas control, raw transaction auditing, resource sampling, and derived metrics. |
| `activities/testnet/testnet.go` | Allows `LaunchTestnet` to run outside Temporal by using a local workflow ID fallback. |
| `activities/loadtest/loadtest.go` | Preserves Catalyst config/log output in the artifact before teardown. |
| `messages/loadtest.go` | Adds `LoadTestConfig` to the load-test response. |
| `petri/core/provider/docker/task.go` | Captures Docker task logs for local artifacts. |
| `hack/simapp.Dockerfile` | Supports selecting a Go image for the SDK fork build. |
| `petri/cosmos/chain/chain.go` | Fixes EVM additional account funding so Petri funds the same first wallet Catalyst derives. |
| `petri/cosmos/chain/additional_accounts_test.go` | Unit coverage for the EVM passphrase behavior. |

The EVM funding fix was required. Catalyst derives Ethereum wallet 0 with an empty BIP39 passphrase, while Petri was funding the `"0"` passphrase wallet as the first additional EVM account. Before the fix, the chain launched but Catalyst failed before sending transactions with insufficient funds. After the fix, a 10 transaction smoke run completed 10/10 successfully.

## Reproduction Commands

Full-node EVM relay bottleneck check:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario evm-blog -skip-build \
  -validators 1 -nodes 1 \
  -evm-batches 2 -evm-msgs 1000 \
  -evm-gas-fee-cap 100000000000 -evm-gas-tip-cap 100000000000 \
  -out reports/artifacts/bottleneck/evm-1v1n-2x1000-highfee-config-rerun.json
```

Direct-validator EVM high-throughput lane:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario evm-blog -skip-build \
  -validators 1 -nodes 0 \
  -evm-batches 10 -evm-msgs 1000 \
  -evm-gas-fee-cap 100000000000 -evm-gas-tip-cap 100000000000 \
  -out reports/artifacts/bottleneck/evm-1v0n-10x1000-highfee-direct-validator.json
```

Direct-validator EVM offered-rate probe:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario evm-blog -skip-build \
  -validators 1 -nodes 0 \
  -evm-batches 10 -evm-msgs 2000 \
  -evm-gas-fee-cap 100000000000 -evm-gas-tip-cap 100000000000 \
  -out reports/artifacts/bottleneck/evm-1v0n-10x2000-highfee-direct-validator.json
```

Packed SDK LevelDB lane:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-goleveldb -skip-build \
  -validators 1 -nodes 0 \
  -wallets 5000 -cosmos-blocks 25 -cosmos-txs 100 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgSend -cosmos-msgs-per-tx 100 \
  -out reports/artifacts/bottleneck/simapp-goleveldb-msgarr-25x100x100.json
```

Packed SDK TreeDB lane:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-treedb -skip-build \
  -validators 1 -nodes 0 \
  -wallets 5000 -cosmos-blocks 25 -cosmos-txs 100 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgSend -cosmos-msgs-per-tx 100 \
  -out reports/artifacts/bottleneck/simapp-treedb-msgarr-25x100x100.json
```

High-gas SDK storage-fanout LevelDB lane:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-goleveldb -skip-build \
  -validators 1 -nodes 0 \
  -wallets 5000 -cosmos-blocks 3 -cosmos-txs 50 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 -cosmos-multisend-recipients 25 \
  -cosmos-max-gas 300000000 \
  -out reports/artifacts/storage-lanes/simapp-goleveldb-msgarr-multisend-3x50x20x25-gas300m.json
```

High-gas SDK storage-fanout TreeDB lane:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-treedb -skip-build \
  -validators 1 -nodes 0 \
  -wallets 5000 -cosmos-blocks 3 -cosmos-txs 50 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 -cosmos-multisend-recipients 25 \
  -cosmos-max-gas 300000000 \
  -out reports/artifacts/storage-lanes/simapp-treedb-msgarr-multisend-3x50x20x25-gas300m.json
```

EVM custom-write contract probe:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario evm-blog -skip-build \
  -validators 1 -nodes 0 -wallets 1000 \
  -evm-msg-type MsgWriteTo -evm-batches 2 -evm-msgs 100 \
  -evm-iterations 100 -evm-initial-contracts 5 \
  -evm-gas-fee-cap 100000000000 -evm-gas-tip-cap 100000000000 \
  -out reports/artifacts/storage-lanes/evm-write-1v0n-2x100-it100.json
```

EVM contract ERC20 contrast probe:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario evm-blog -skip-build \
  -validators 1 -nodes 0 -wallets 1000 \
  -evm-msg-type MsgTransferERC0 -evm-batches 2 -evm-msgs 100 \
  -evm-initial-contracts 5 \
  -evm-gas-fee-cap 100000000000 -evm-gas-tip-cap 100000000000 \
  -out reports/artifacts/storage-lanes/evm-contract-erc20-1v0n-2x100.json
```

## Scenario Refs

| Scenario | Chain source | Commit / dependency |
| --- | --- | --- |
| `evm-blog` | `https://github.com/cosmos/evm` | `f90a5c79c0052e0f5cd670a367f24967d1120650` |
| `simapp-goleveldb` | `https://github.com/snissn/celestia-cosmos-sdk` | `28e5525fefe7aaa53d4726ef7a367242bacf9003` |
| `simapp-treedb` | `https://github.com/snissn/celestia-cosmos-sdk` | `28e5525fefe7aaa53d4726ef7a367242bacf9003` |
| TreeDB cosmos-db replace | `github.com/snissn/cosmos-db` | `v0.0.0-20260701072812-7b5cfd624186` |
| TreeDB IAVL replace | `github.com/snissn/iavl` | `v0.0.0-20260701072929-12a26715119b` |

The simapp image is the same for LevelDB and TreeDB. The scenario-level app config switches only `app-db-backend`.

## Results

### EVM Workload And Bottleneck Matrix

Runtime TPS is Catalyst's result over the load-test start/end window. Wall TPS includes chain startup, load-test setup, auditing, artifact capture, and teardown, so it is much lower and should not be compared to the blog's tx/s claims.

| Artifact | Topology | Attempted | Included | Successful | Runtime TPS | Wall time | Validator CPU max | Validator memory max | Validator write max | Interpretation |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `evm-blog-smoke-after-passphrase-fix.json` | 1v/1n | 10 | 10 | 10 | 10.0 | 43.47s | n/a | n/a | n/a | Funding fix smoke. |
| `evm-blog-30x1000-passphrase-fix.json` | 1v/1n | 30,000 | 2,641 | 2,639 | 528.2 | 122.31s | n/a | n/a | n/a | Original local reproduction lane, partial inclusion. |
| `bottleneck/evm-1v1n-1x1000-highfee.json` | 1v/1n | 1,000 | 1,000 | 1,000 | 500.0 | 65.09s | 394.10% | 287.2 MiB | 63.5 MB | One batch can fully include through the full node. |
| `bottleneck/evm-1v1n-2x1000-highfee-config-rerun.json` | 1v/1n | 2,000 | 728 | 728 | 728.0 | 62.56s | 374.37% | 386.3 MiB | 95.6 MB | Full-node relay/txpool path loses inclusion under sustained load. |
| `bottleneck/evm-1v0n-10x1000-highfee-direct-validator.json` | 1v/0n | 10,000 | 10,000 | 9,996 | 1,000.0 | 103.97s | 969.40% | 901.2 MiB | 453 MB | Direct validator path reaches 1,000 TPS with full inclusion. |
| `bottleneck/evm-1v0n-10x2000-highfee-direct-validator.json` | 1v/0n | 20,000 | 20,000 | 19,998 | 1,000.0 | 230.69s | 977.34% | 1.332 GiB | 1.11 GB | Offered 2,000 tx/s still caps at 1,000 runtime TPS. |
| `bottleneck/evm-4v0n-5x1000-highfee-direct-validators.json` | 4v/0n | 5,000 | 14 | 14 | 14.0 | 70.05s | 280.03% | 385.0 MiB | 16.6 MB | Local multi-validator direct mode is not a valid high-rate lane. |
| `bottleneck/evm-1v0n-5x1000-native-gas-transfer.json` | 1v/0n | 5,000 | 3,163 | 3,163 | 632.6 | 77.57s | 732.12% | 642.0 MiB | 169 MB | Native gas transfer hits Catalyst nonce/balance issues and is not the current fast lane. |

The 2,000 attempted full-node run captured the Catalyst config in the JSON artifact and confirms the high dynamic-fee settings were applied:

```yaml
gas_fee_cap: "100000000000"
gas_tip_cap: "100000000000"
```

That makes low fee bidding unlikely as the cause of partial inclusion. The stronger explanation is structural: the full-node RPC/txpool relay path declares the mempool clear while transactions are not found later. In contrast, direct validator RPC included every attempted transaction in the 10,000 and 20,000 tx probes.

### EVM Bottleneck Conclusions

- The biggest improvement came from changing topology, not database code: one full node plus one validator peaked at partial inclusion, while direct validator RPC achieved full inclusion at 1,000 runtime TPS.
- The EVM lane is not gas-limited in these runs. The 20,000 tx direct-validator run averaged 6.54% block gas utilization while validator CPU approached ten logical cores.
- The current local ceiling is validator-side processing and storage pressure. The 20,000 tx run wrote 1.11 GB from the validator container and reached 1.332 GiB memory.
- Catalyst's full-node completion check is too weak for this workload shape because it looks cleared before all submitted transactions are queryable.
- Catalyst's native gas transfer generator is not trustworthy under this offered rate yet because prebuilt pending nonce/balance logic produces insufficient funds, replacement-underpriced, and nonce-too-low failures.
- Four local validators do not currently help because this path collapses inclusion instead of increasing throughput.

### SDK Backend Comparison

Catalyst's SDK decoder currently marks these SDK simapp transactions as failed because the response shape includes a `signers` field it does not understand. The local runner therefore extracts raw transaction hashes from Catalyst logs and queries CometBFT `/tx` before teardown. The raw audit is the source of truth for the SDK tables.

#### Plain `MsgSend` Baseline

This earlier lane proves TreeDB is consumable by Ironbird but is too low-throughput to expose storage differences.

| Backend | Load | Raw queried | Raw found | Raw successful | Raw failed | Raw TPS | Wall time | Total gas |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | 25 x 100 `MsgSend` | 2,500 | 2,500 | 2,500 | 0 | 19.253 | 166.57s | 192,191,713 |
| TreeDB | 25 x 100 `MsgSend` | 2,500 | 2,500 | 2,500 | 0 | 19.152 | 172.36s | 192,164,613 |

TreeDB was -0.52% on raw TPS and +3.47% on wall time in this lane. That is not a win.

#### Packed `MsgArr` Lane

To push more work through the chain per outer transaction, the runner now supports Catalyst `MsgArr`: 100 contained `MsgSend` messages per outer SDK transaction. The raw transaction count remains 2,500, but the effective contained-message count is 250,000.

| Backend | Raw successful tx | Effective contained messages | Raw TPS | Runtime effective ops/s | Wall effective ops/s | Wall time | Validator CPU max | Validator memory max | Validator write max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | 2,500 | 250,000 | 7.5077 | 750.7727 | 607.4758 | 411.54s | 501.12% | 1.215 GiB | 6.66 GB |
| TreeDB | 2,500 | 250,000 | 7.5320 | 753.2007 | 608.3753 | 410.93s | 478.17% | 1.769 GiB | 5.73 GB |

TreeDB versus LevelDB in the packed lane:

| Metric | Delta |
| --- | ---: |
| Raw TPS | +0.32% |
| Runtime effective ops/s | +0.32% |
| Wall effective ops/s | +0.15% |
| Wall time | -0.15% |
| Validator CPU max | -4.58% |
| Validator block write max | -13.96% |
| Validator memory max | +45.60% |

Interpretation:

- This lane raises the effective operation rate from about 19 simple SDK tx/s to about 750 contained messages/sec, enough to create GB-scale write pressure.
- TreeDB and LevelDB throughput are effectively tied here. The observed +0.32% TreeDB runtime delta is noise, not evidence of a throughput advantage.
- TreeDB reduced validator Docker block-write max from 6.66 GB to 5.73 GB in this run, but used substantially more memory.
- This is a useful storage-pressure lane, but it still does not reach the user's desired thousands of effective ops/sec.

### Storage-Oriented Workload Search

The follow-up search tried both lanes the user suggested:

- EVM custom contract work: use Catalyst's loader-contract messages, especially `MsgWriteTo`, to create many Solidity storage writes per transaction.
- More general data-availability-shaped work: use SDK `MsgArr(MsgMultiSend)` as a local storage-fanout proxy that pushes many account state updates per outer transaction.

The non-EVM lane found the best current storage-oriented signal. It is not true Celestia blob or DA execution, because the available Catalyst SDK runner only exposes `MsgSend`, `MsgMultiSend`, and `MsgArr`. It is still useful because `MsgArr(MsgMultiSend)` fans out many account writes behind each transaction and can be compared under identical LevelDB and TreeDB app-db settings.

#### EVM Contract Probes

| Artifact | Workload | Attempted | Included | Successful | Runtime TPS | Wall time | Validator CPU max | Validator memory max | Validator write max | Interpretation |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `storage-lanes/evm-write-1v0n-2x100-it100.json` | `MsgWriteTo`, requested 100 writes/tx | 200 | 200 | 0 | 100.0 | 44.58s | 71.67% | 192.4 MiB | 22.6 MB | Invalid storage-write signal; interval baseline generation drops the requested iteration count and gas settings, so all executions failed. |
| `storage-lanes/evm-write-1v0n-2blocks-25-it100.json` | `MsgWriteTo`, block-cadence mode | 50 intended | 0 collected | 0 collected | n/a | 32.11s | 119.12% | 185.5 MiB | 12.9 MB | Invalid result window; block-cadence submission started, but the collector trimmed a window with no landed txs. |
| `storage-lanes/evm-contract-erc20-1v0n-2x100.json` | `MsgTransferERC0`, deployed contract ERC20 transfer | 200 | 200 | 200 | 200.0 | 42.34s | 39.39% | 189.4 MiB | 22.9 MB | Valid execution contrast, but gas-light and not storage-limited. |

`MsgWriteTo` is the right EVM direction, but it is not benchmark-ready through this runner yet. In Catalyst v0.0.0-beta.16, interval-mode baseline generation rebuilds each message spec with only `Type` and `NumMsgs`, dropping fields such as `NumOfIterations` and `CalldataSize`. That makes custom high-write or high-calldata EVM workloads execute with the wrong baseline/gas shape. The block-cadence path avoids that baseline mutation but needs a result-window fix before it can produce comparable inclusion metrics.

#### SDK Storage-Fanout Proxy

The storage-fanout proxy uses:

```text
effective operations = successful raw txs * MsgArr contained messages/tx * MsgMultiSend recipients/message
```

With 20 contained `MsgMultiSend` messages and 25 recipients per contained message, each successful outer transaction represents 500 recipient operations.

| Backend | Artifact | Block gas | Raw successful tx | Effective recipient operations | Raw TPS | Runtime effective ops/s | Wall effective ops/s | Wall time | Validator CPU max | Validator memory max | Validator write max |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | `storage-lanes/simapp-goleveldb-msgarr-multisend-3x10x20x25.json` | 75M | 30 | 15,000 | 0.7822 | 391.08 | 194.15 | 77.26s | 120.39% | 276.8 MiB | 19.7 MB |
| TreeDB | `storage-lanes/simapp-treedb-msgarr-multisend-3x10x20x25.json` | 75M | 30 | 15,000 | 0.7640 | 382.01 | 196.55 | 76.32s | 19.80% | 467.8 MiB | 27.9 MB |
| LevelDB | `storage-lanes/simapp-goleveldb-msgarr-multisend-5x15x20x25.json` | 75M | 75 | 37,500 | 1.3260 | 662.98 | 328.91 | 114.01s | 281.62% | 422.3 MiB | 73.5 MB |
| TreeDB | `storage-lanes/simapp-treedb-msgarr-multisend-5x15x20x25.json` | 75M | 75 | 37,500 | 1.3400 | 669.98 | 326.09 | 115.00s | 261.69% | 634.0 MiB | 85.7 MB |
| LevelDB | `storage-lanes/simapp-goleveldb-msgarr-multisend-3x50x20x25-gas300m.json` | 300M | 150 | 75,000 | 3.3970 | 1,698.48 | 575.71 | 130.27s | 222.84% | 721.5 MiB | 113 MB |
| TreeDB | `storage-lanes/simapp-treedb-msgarr-multisend-3x50x20x25-gas300m.json` | 300M | 150 | 75,000 | 3.4160 | 1,708.02 | 600.65 | 124.86s | 325.13% | 899.4 MiB | 126 MB |

Interpretation:

- Raising block gas from the default 75M to 300M is the first local change that moved the SDK lane above 1,000 effective operations/sec.
- The 300M pair is a better storage-aligned lane than the EVM transfer lanes because it reduces Catalyst/RPC submission as the dominant factor and forces many app-state writes per outer transaction.
- TreeDB was slightly ahead on the 300M short run: +0.56% runtime effective ops/s and +4.33% wall effective ops/s. That is not enough to claim a durable throughput advantage without quieter repeats.
- TreeDB used more memory and wrote more sampled Docker block bytes in the 300M run: 899.4 MiB versus 721.5 MiB, and 126 MB versus 113 MB. This is the opposite of the earlier packed `MsgSend` lane's sampled write-byte result, so storage-byte claims need repeated, phase-specific counters.
- CPU is still material. Validator CPU reached 222.84% for LevelDB and 325.13% for TreeDB in the 300M run, so this is more storage-oriented than the EVM lanes but not a pure disk-limited benchmark.
- Catalyst's SDK collector still falsely marks these SDK transactions failed because it cannot decode the current response shape. The raw CometBFT tx audit is therefore the source of truth for success counts and effective-op calculations.

## Current State Of The Bottleneck

The Amdahl's-law problem is real: once transaction submission is no longer the dominant bottleneck, the benchmark is dominated by app execution, mempool behavior, consensus topology, gas limits, and storage pressure. The direct-validator EVM path removes the first full-node relay bottleneck and exposes a new ceiling around 1,000 runtime TPS. The packed SDK path increases per-transaction state work and exposes write-volume differences. The higher-gas `MsgArr(MsgMultiSend)` variant now reaches about 1,700 effective recipient operations/sec, which is the best current local lane for storage-oriented LevelDB versus TreeDB comparison, but it is still not enough to prove a TreeDB throughput advantage.

Most useful next directions:

1. Fix the full-node relay/mempool-clear correctness issue so the blog-style topology can sustain full inclusion without bypassing full nodes.
2. Add phase-specific profiling for validator CPU, block execution, mempool, commit, and DB write phases during the direct-validator 1,000 TPS lane.
3. Fix Catalyst's EVM interval baseline handling for `NumOfIterations` and `CalldataSize`, then rerun `MsgWriteTo` and `MsgCallDataBlast` as custom EVM storage/calldata stressors.
4. Push `MsgArr(MsgMultiSend)` harder with larger block gas, more outer txs, more funded wallets, and quieter-host repeats to see whether effective ops/sec can move materially beyond 1,700 without becoming purely message-execution bound.
5. Fix Catalyst native gas transfer nonce/balance generation if gas-token sends are expected to be a fast EVM lane.
6. Create or select a TreeDB-enabled Cosmos EVM fork before making any exact EVM TreeDB claim.

## What This Proves

This report proves:

- Ironbird can build and launch the Cosmos EVM blog commit locally through Docker.
- The `MsgNativeTransferERC20` workload executes successfully after the EVM wallet funding fix.
- The original local one-validator/one-full-node lane reaches 528.2 runtime included TPS but suffers partial inclusion.
- Bypassing the full node reaches 1,000 runtime TPS with full inclusion on the same local host.
- The runner now records enough config, logs, raw audits, and resource samples to explain the main bottlenecks.
- `snissn/celestia-cosmos-sdk` can run through Ironbird with `app-db-backend=treedb`.
- A packed SDK workload can produce roughly 750 effective contained messages/sec and GB-scale validator writes for LevelDB versus TreeDB comparison.
- A higher-gas SDK storage-fanout proxy can produce roughly 1,700 effective recipient operations/sec with successful raw tx audit.
- The EVM `MsgWriteTo` direction is blocked by Catalyst baseline/result-window issues rather than by TreeDB or LevelDB.

This report does not prove:

- That this machine reproduces the blog's production-scale 1,500-1,800 TPS result.
- That TreeDB improves Cosmos EVM ERC20 throughput.
- That TreeDB improves SDK throughput.
- That `MsgArr(MsgMultiSend)` is equivalent to Celestia blob/DA execution.
- That the current EVM custom-write probe is a valid storage benchmark.
- That Docker stats sample maxima are exact kernel or cgroup high-water marks.
- That multi-validator local Docker performance reflects a production cloud network.

## Follow-Up Punch List

Tracker: [snissn/ironbird#1](https://github.com/snissn/ironbird/issues/1).

1. Land the local runner, EVM funding fix, config/log capture, raw audit, and resource sampling in a focused `snissn/ironbird` PR.
2. Fix Catalyst's SDK tx response decoding so raw audit is validation rather than the only truthful result path.
3. Fix the full-node EVM relay/mempool-clear issue and rerun the one-validator/one-full-node lane until inclusion is explained or full inclusion is achieved.
4. Profile the one-validator direct EVM lane at 1,000 TPS and identify whether CPU time is in EVM execution, mempool, consensus, commit, or storage.
5. Fix Catalyst's EVM interval baseline handling so `MsgWriteTo`, `MsgCallDataBlast`, `NumOfIterations`, and `CalldataSize` survive baseline generation.
6. Fix or disable the invalid native gas transfer lane until nonce/balance prebuild behavior is corrected.
7. Run quieter-host repeats for the packed SDK and high-gas `MsgArr(MsgMultiSend)` LevelDB/TreeDB lanes before making any storage-performance claim.
8. Build the TreeDB-enabled Cosmos EVM app path if exact blog-workload TreeDB comparison remains the target.
9. Only after local gates pass, run a production-like topology with non-validating RPC nodes and larger machines.

## Artifact Index

| Artifact | Purpose |
| --- | --- |
| `reports/artifacts/evm-blog-smoke-after-passphrase-fix.json` | EVM workload smoke after funding fix. |
| `reports/artifacts/evm-blog-30x1000-passphrase-fix.json` | Original local saturated blog-workload reproduction: 528.2 runtime TPS, partial inclusion. |
| `reports/artifacts/evm-blog-30x500-passphrase-fix.json` | Lower-rate one-validator/one-node check. |
| `reports/artifacts/bottleneck/evm-1v1n-1x1000-highfee.json` | Full-node relay can include one 1,000 tx batch. |
| `reports/artifacts/bottleneck/evm-1v1n-2x1000-highfee-config-rerun.json` | Full-node relay loses inclusion under two 1,000 tx batches; config proves high fees were applied. |
| `reports/artifacts/bottleneck/evm-1v0n-10x1000-highfee-direct-validator.json` | Direct-validator EVM lane: 10,000 included, 1,000 runtime TPS. |
| `reports/artifacts/bottleneck/evm-1v0n-10x2000-highfee-direct-validator.json` | Offered 2,000 tx/s probe: still 1,000 runtime TPS, validator CPU near saturation. |
| `reports/artifacts/bottleneck/evm-4v0n-5x1000-highfee-direct-validators.json` | Multi-validator local diagnostic: not a valid high-throughput lane. |
| `reports/artifacts/bottleneck/evm-1v0n-5x1000-native-gas-transfer.json` | Native gas transfer diagnostic: invalidated by nonce/balance failures. |
| `reports/artifacts/simapp-goleveldb-msgsend-25x100-fullaudit.json` | Plain SDK LevelDB baseline with raw tx audit. |
| `reports/artifacts/simapp-treedb-msgsend-25x100-fullaudit.json` | Plain SDK TreeDB baseline with raw tx audit. |
| `reports/artifacts/bottleneck/simapp-goleveldb-msgarr-25x100x100.json` | Packed SDK LevelDB storage-pressure lane. |
| `reports/artifacts/bottleneck/simapp-treedb-msgarr-25x100x100.json` | Packed SDK TreeDB storage-pressure lane. |
| `reports/artifacts/storage-lanes/evm-write-1v0n-2x100-it100.json` | EVM `MsgWriteTo` interval probe; invalidated by Catalyst baseline dropping iteration/calldata-shaped fields. |
| `reports/artifacts/storage-lanes/evm-write-1v0n-2blocks-25-it100.json` | EVM `MsgWriteTo` block-cadence probe; invalidated by empty collection window. |
| `reports/artifacts/storage-lanes/evm-contract-erc20-1v0n-2x100.json` | EVM deployed-contract ERC20 contrast probe: 200/200 successful, but not storage-heavy. |
| `reports/artifacts/storage-lanes/simapp-goleveldb-msgarr-multisend-3x10x20x25.json` | SDK `MsgArr(MsgMultiSend)` LevelDB smoke: 15,000 effective recipient operations. |
| `reports/artifacts/storage-lanes/simapp-treedb-msgarr-multisend-3x10x20x25.json` | SDK `MsgArr(MsgMultiSend)` TreeDB smoke: 15,000 effective recipient operations. |
| `reports/artifacts/storage-lanes/simapp-goleveldb-msgarr-multisend-5x15x20x25.json` | SDK `MsgArr(MsgMultiSend)` LevelDB default-gas scale run: 37,500 effective recipient operations. |
| `reports/artifacts/storage-lanes/simapp-treedb-msgarr-multisend-5x15x20x25.json` | SDK `MsgArr(MsgMultiSend)` TreeDB default-gas scale run: 37,500 effective recipient operations. |
| `reports/artifacts/storage-lanes/simapp-goleveldb-msgarr-multisend-3x50x20x25-gas300m.json` | SDK `MsgArr(MsgMultiSend)` LevelDB high-gas run: 75,000 effective recipient operations, 1,698.48 runtime effective ops/s. |
| `reports/artifacts/storage-lanes/simapp-treedb-msgarr-multisend-3x50x20x25-gas300m.json` | SDK `MsgArr(MsgMultiSend)` TreeDB high-gas run: 75,000 effective recipient operations, 1,708.02 runtime effective ops/s. |

## Validation

Focused validation run after the report and tracker updates:

```sh
git diff --check
GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./activities/testnet ./petri/core/provider/docker
GOWORK=off go test ./petri/cosmos/chain -run 'TestAdditionalAccountPassphrase'
```

All three commands passed. A separate trailing-whitespace check over the new report, tracker, local runner, and passphrase test also passed.
