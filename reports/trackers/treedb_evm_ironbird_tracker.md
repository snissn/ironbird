# TreeDB EVM Ironbird Tracker

## Goal

Land first-class Ironbird benchmark coverage that can compare the Cosmos performance-blog EVM workload against a TreeDB-enabled Cosmos EVM app, with a self-contained report that distinguishes published blog targets, local Docker results, and production-scale results.

Done means `snissn/ironbird` has reproducible lanes for:

- Cosmos EVM baseline on the blog-style `MsgNativeTransferERC20` or gas-token-send workload.
- A TreeDB-enabled candidate running the same workload shape.
- A higher-throughput SDK storage-pressure lane while the exact EVM TreeDB app is blocked.
- Report artifacts with exact commands, refs, topology, throughput, inclusion counts, wall time, resource sampling, and pass/fail interpretation.

## Current Evidence

Local report branch: `codex/treedb-ironbird-report`.

Local report path:

- `reports/ironbird_treedb_reproduction_report_2026-07-01.md`

GitHub tracker:

- `https://github.com/snissn/ironbird/issues/1`

Key artifacts:

| Artifact | Result |
| --- | --- |
| `reports/artifacts/evm-blog-smoke-after-passphrase-fix.json` | EVM workload executes after funding derivation fix: 10 sent, 10 included, 10 successful. |
| `reports/artifacts/evm-blog-30x1000-passphrase-fix.json` | Original local EVM run: 30,000 attempted, 2,641 included, 2,639 successful, 528.2 runtime TPS. |
| `reports/artifacts/bottleneck/evm-1v1n-2x1000-highfee-config-rerun.json` | One validator plus one full node: 2,000 attempted, 728 included, high fees confirmed, no broadcast failures. |
| `reports/artifacts/bottleneck/evm-1v0n-10x1000-highfee-direct-validator.json` | One validator, no full nodes: 10,000 included, 9,996 successful, 1,000 runtime TPS. |
| `reports/artifacts/bottleneck/evm-1v0n-10x2000-highfee-direct-validator.json` | Offered 2,000 tx/s still reports 1,000 runtime TPS, with validator CPU near 977%, 1.332 GiB memory, 1.11 GB writes. |
| `reports/artifacts/bottleneck/evm-4v0n-5x1000-highfee-direct-validators.json` | Four local validators collapse inclusion: 14 of 5,000 included. |
| `reports/artifacts/bottleneck/evm-1v0n-5x1000-native-gas-transfer.json` | Native gas transfer diagnostic: 3,163 included, but Catalyst nonce/balance failures make the lane invalid as a fast path. |
| `reports/artifacts/storage-lanes/evm-write-1v0n-2x100-it100.json` | EVM `MsgWriteTo` interval diagnostic: 200 included, 0 successful, invalidated by Catalyst baseline generation dropping iteration/calldata-shaped fields. |
| `reports/artifacts/storage-lanes/evm-write-1v0n-2blocks-25-it100.json` | EVM `MsgWriteTo` block-cadence diagnostic: no useful collected txs, invalidated by result-window timing. |
| `reports/artifacts/storage-lanes/evm-contract-erc20-1v0n-2x100.json` | Deployed-contract ERC20 contrast: 200 included, 200 successful, 200 runtime TPS, but gas-light and not storage-limited. |
| `reports/artifacts/simapp-goleveldb-msgsend-25x100-fullaudit.json` | Plain SDK LevelDB: 2,500 raw-audited successful `MsgSend`, 19.253 TPS. |
| `reports/artifacts/simapp-treedb-msgsend-25x100-fullaudit.json` | Plain SDK TreeDB: 2,500 raw-audited successful `MsgSend`, 19.152 TPS. |
| `reports/artifacts/bottleneck/simapp-goleveldb-msgarr-25x100x100.json` | Packed SDK LevelDB: 250,000 contained messages, 750.77 runtime effective ops/s, 6.66 GB validator writes. |
| `reports/artifacts/bottleneck/simapp-treedb-msgarr-25x100x100.json` | Packed SDK TreeDB: 250,000 contained messages, 753.20 runtime effective ops/s, 5.73 GB validator writes. |
| `reports/artifacts/storage-lanes/simapp-goleveldb-msgarr-multisend-3x50x20x25-gas300m.json` | High-gas SDK storage-fanout LevelDB: 75,000 effective recipient operations, 1,698.48 runtime effective ops/s, 113 MB validator write max. |
| `reports/artifacts/storage-lanes/simapp-treedb-msgarr-multisend-3x50x20x25-gas300m.json` | High-gas SDK storage-fanout TreeDB: 75,000 effective recipient operations, 1,708.02 runtime effective ops/s, 126 MB validator write max. |

## Current State

The local report runner directly calls Ironbird's testnet and load-test activity layer with the Docker provider. It can run:

- `evm-blog`: Cosmos EVM at `f90a5c79c0052e0f5cd670a367f24967d1120650`.
- `simapp-goleveldb`: `snissn/celestia-cosmos-sdk` at `28e5525fefe7aaa53d4726ef7a367242bacf9003`, `app-db-backend=goleveldb`.
- `simapp-treedb`: same SDK image and TreeDB dependency replacements, `app-db-backend=treedb`.

TreeDB is not yet tested under the exact EVM app and workload. The current best TreeDB comparison is the packed SDK `MsgArr` storage-pressure lane.

The current best storage-oriented lane is the high-gas SDK `MsgArr(MsgMultiSend)` proxy. It is not a true Celestia blob/DA workload; it is a local state-write fanout workload available through Catalyst's current SDK message set. It reaches roughly 1,700 effective recipient operations/sec with successful raw tx audit.

## Current Bottlenecks

| Bottleneck | Evidence | Current action |
| --- | --- | --- |
| Full-node EVM relay path | One validator plus one full node includes 728 of 2,000 attempted tx despite no broadcast failures and high fees. | Fix Catalyst/Ironbird full-node mempool-clear and transaction audit behavior; do not use full-node partial inclusion as a TreeDB comparison gate. |
| Direct-validator EVM CPU/processing ceiling | One validator/no full nodes includes all tx, but 10x2000 offered load still reports 1,000 runtime TPS with validator CPU near 977%. | Profile validator execution, mempool, consensus, commit, and DB write phases. |
| Local multi-validator direct topology | Four validators/no full nodes includes 14 of 5,000 attempted tx. | Treat as invalid until topology/RPC target behavior is fixed. |
| Native gas transfer generator | 3,163 of 5,000 included with insufficient-funds, replacement-underpriced, and nonce-too-low send failures. | Fix Catalyst nonce/balance prebuild behavior before using as a fast EVM lane. |
| EVM custom-write contract generator | `MsgWriteTo` interval mode included 200 tx but all failed; Catalyst baseline generation preserves only message type and count, dropping iteration/calldata fields. | Fix Catalyst baseline handling for `NumOfIterations`, `CalldataSize`, and gas limits before using `MsgWriteTo` or `MsgCallDataBlast` as storage/calldata lanes. |
| EVM block-cadence result window | `MsgWriteTo` block mode started but the collector trimmed a window with no landed txs. | Fix result-window timing before relying on block-cadence EVM probes. |
| SDK tx decoder | Catalyst reports SDK simapp tx as failed because the response shape includes `signers`. | Fix decoder; keep raw CometBFT audit as validation. |
| True DA workload missing | Current Catalyst SDK runner exposes `MsgSend`, `MsgMultiSend`, and `MsgArr`, not Celestia blob submission. | Treat `MsgArr(MsgMultiSend)` as a proxy only; add a true DA/blob lane if the runner/app surface supports it. |
| Exact EVM TreeDB app missing | Cosmos EVM baseline app is not wired to `snissn` TreeDB SDK/cosmos-db/IAVL forks. | Create or select a TreeDB-enabled Cosmos EVM fork before making exact EVM TreeDB claims. |

## North-Star Gates

| Gate | Current | Target | Required evidence | If the gate fails |
| --- | --- | --- | --- | --- |
| Exact workload parity | Baseline EVM only; TreeDB uses SDK simapp | Baseline EVM and TreeDB EVM use identical ERC20/gas-token workload | Same runner command except backend/ref; same wallets, batches, topology, and Catalyst mode | Keep tracker open and link the missing fork/app wiring blocker |
| Local direct EVM inclusion | 1,000 runtime TPS with full inclusion on one validator/no full nodes | Stable 1,500+ runtime TPS or explicit validator-side blocker | JSON artifact with inclusion, Catalyst config, logs, and resource samples | Profile and document the bottleneck before further tuning |
| Blog-style full-node topology | 528.2 runtime TPS peak with partial inclusion; 2x1000 includes 728/2000 | Stable selected local rate through non-validating RPC with explained inclusion percentage | Artifact with raw tx audit and txpool/mempool evidence | Do not compare DBs on a partial-inclusion lane |
| High-effective SDK lane | 1,698.48 LevelDB versus 1,708.02 TreeDB effective recipient ops/sec in the 300M gas `MsgArr(MsgMultiSend)` proxy | Repeatable thousands of effective ops/sec with quieter-host repeats and phase-specific counters | Raw CometBFT audit, effective-op calculation, resource samples, quiet-host repeat | Restrict claim to current measured storage pressure |
| TreeDB performance claim | No throughput win; TreeDB has -13.96% validator write max and +45.60% memory in packed lane | Positive throughput delta under identical workload, or narrowly scoped I/O claim | Baseline/candidate table with throughput, wall time, memory, disk, and raw counters | Do not claim throughput improvement |
| Resource high-water | Continuous Docker stats sample maxima from the local runner | Phase-specific process/cgroup counters for memory, disk writes, CPU, and network | Time-series artifact or summarized per-phase counters | Call values sample maxima, not exact OS high-water marks |
| Production-like reproduction | Not run locally | 5-10 validators plus non-validating RPC nodes or documented reason for deferral | Cloud or equivalent artifact and topology metadata | Mark report as local-only and keep production gate open |

## Scope

- Preserve the working EVM baseline and direct-validator high-throughput lane.
- Keep SDK simapp TreeDB as a secondary smoke/comparative lane until exact EVM TreeDB exists.
- Add benchmark/report evidence with exact refs and commands.
- Fix Catalyst/runner issues that block truthful measurement.
- Use only `snissn/*` repos unless public upstream PR authorization is explicitly given.

## Non-Goals And Boundary

- Do not claim TreeDB improves EVM throughput until the exact EVM workload has a before/after comparison.
- Do not claim TreeDB improves SDK throughput from the current packed lane; it is effectively tied with LevelDB.
- Do not treat local Docker throughput as equivalent to the Cosmos blog's production-scale 10-50 node results.
- Do not create public upstream PRs from this tracker without explicit maintainer/user authorization.
- Do not change public default app behavior just to make the benchmark pass.

## Execution Ordering And Blocking

1. Preserve the local report runner, EVM funding fix, config/log capture, raw audit, resource sampling, and report artifact structure in a focused PR.
2. Fix or work around Catalyst's SDK tx response decoder issue without relying only on log hash extraction.
3. Fix full-node relay/mempool-clear behavior or document why direct-validator mode is the only local full-inclusion EVM lane.
4. Profile the 1,000 TPS direct-validator EVM lane and identify whether the next ceiling is EVM execution, mempool, consensus, commit, or storage.
5. Fix Catalyst's EVM custom-workload baseline handling and block-cadence collection window.
6. Push high-gas `MsgArr(MsgMultiSend)` repeats under quieter host conditions.
7. Decide and implement the TreeDB-enabled EVM app path.
8. Run local baseline versus candidate with identical topology and commands.
9. Run or explicitly defer the production-scale topology gate.
10. Publish the self-contained report.

## Branch And PR Policy

- Work should happen on topic branches in `snissn/ironbird`.
- Use only `snissn/*` repos unless upstream/public PR authorization is explicitly given.
- PRs should include focused tests, exact benchmark commands, JSON artifact paths, and a report delta.
- Performance claims require before/after evidence under identical workload, topology, hardware, and refs.
- Material regressions in throughput, wall time, memory, disk, or inclusion rate are blocking unless explicitly accepted as a scoped tradeoff.
- Request AI/code-review feedback only after the PR has coherent code, focused tests, updated report evidence, and current CI status.

## Milestones

### M0. Preserve Current Local Reproduction

- [x] Add the local report runner.
- [x] Keep the smoke test proving `MsgNativeTransferERC20` runs.
- [x] Keep the SDK LevelDB versus TreeDB raw-audit comparison as consumability evidence.
- [x] Capture Catalyst logs and generated load-test config.
- [x] Add raw transaction audit for SDK lanes.

Required tests:

- [x] `GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./activities/testnet ./petri/core/provider/docker`
- [x] `GOWORK=off go test ./petri/cosmos/chain -run 'TestAdditionalAccountPassphrase'`

### M1. Measurement Hygiene

- [x] Add continuous Docker stats sampling for memory, CPU, disk read/write, and network.
- [x] Make the report identify full-inclusion, partial-inclusion, and saturation runs separately.
- [x] Persist generated Catalyst YAML config in artifacts.
- [ ] Fix Catalyst's current SDK tx response decoder issue without relying only on log hash extraction.
- [ ] Add phase-specific process/cgroup counters for validator commit and DB write sections.

### M2. Bottleneck Fixes And Workload Shaping

- [x] Prove direct-validator EVM mode can reach 1,000 runtime TPS with full inclusion.
- [x] Prove higher offered EVM load does not exceed 1,000 runtime TPS on this host.
- [x] Show the local four-validator direct topology is currently invalid for high-rate EVM measurement.
- [x] Add packed SDK `MsgArr` workload support.
- [x] Add EVM block-cadence mode for custom contract probes.
- [x] Add EVM initial-contract control for deployed contract workloads.
- [x] Add Cosmos max-gas control for storage-fanout runs.
- [x] Probe SDK `MsgArr(MsgMultiSend)` storage-fanout at 75M and 300M block gas.
- [ ] Fix full-node relay/mempool-clear behavior.
- [ ] Fix native gas transfer nonce/balance behavior.
- [ ] Fix Catalyst interval baselines for `MsgWriteTo`, `MsgCallDataBlast`, iteration count, calldata size, and gas limits.
- [ ] Fix EVM block-cadence collection-window timing.
- [ ] Profile the direct-validator EVM 1,000 TPS ceiling.
- [ ] Repeat high-gas SDK storage-fanout lane under quieter host conditions and with phase-specific counters.

### M3. TreeDB EVM Candidate

- [ ] Select the repo/ref strategy for a TreeDB-enabled Cosmos EVM app.
- [ ] Add an Ironbird scenario that differs from the baseline only by backend/ref wiring.
- [ ] Prove the candidate can execute the same ERC20/gas-token workload.

### M4. Before/After Report

- [ ] Run baseline and TreeDB candidate under identical local Docker settings.
- [ ] Compare TPS, inclusion rate, wall time, gas, block utilization, resource high-water, and disk writes.
- [ ] Do not claim improvement unless the measured candidate beats baseline on the intended gate.

### M5. Production-Scale Follow-Through

- [ ] Run or explicitly defer the 5-10 validator / non-validating RPC topology.
- [ ] Update the report with topology, refs, commands, artifacts, and pass/fail interpretation.
