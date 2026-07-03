# Ironbird Preseeded-State TreeDB A/B

Date: 2026-07-01

This report records a local Ironbird sprint to test whether a larger, more production-shaped app state makes the existing simapp transaction workload more sensitive to the application DB backend. The runner used the already-built simapp Docker image:

`ironbird-report:snissn-sdk-28e5525f-cosmosdb-06039c0-gomap-v0.6.1`

Dependency provenance recorded in each artifact:

- `github.com/cosmos/cosmos-db` `v0.0.0-20260701185743-06039c0bbb3c`, ref `06039c0bbb3cdfd53df947e995d8760db9b30d4e`
- `github.com/snissn/gomap` `v0.6.1`, ref `3472a5bc6de75142c047a2c9cabe5a683c25e1e9`

## Runner Change

The local report runner now supports:

- `-preseed-profile accounts`
- `-preseed-accounts N`
- provenance fields for the pinned DB dependencies
- `launch_seconds`
- `data_sizes_before/after` collection for `/simd/config/genesis.json`

For simapp scenarios, the runner launches the chain with `active wallets + preseed accounts` in genesis, while Catalyst still runs against only the active workload wallet count. This lets inactive accounts enlarge genesis/app state without directly increasing the number of workload signers.

Validation:

```sh
GOWORK=off go test ./cmd/local-report-runner
```

Result: pass.

## Workload

Shared workload for the A/B runs:

- validators: `1`
- nodes: `0`
- active wallets: `5000`
- blocks: `3`
- transactions per block: `50`
- message: `MsgArr`
- contained message: `MsgMultiSend`
- contained messages per tx: `20`
- recipients per contained message: `25`
- max gas: `300000000`
- intended transactions: `150`
- effective operations: `150 * 20 * 25 = 75000`

The Catalyst top-level result still marks the transactions as failed because of the existing JSON decode issue, but raw transaction audit found all 150 transactions and all 150 succeeded in every A/B run below. The raw audit is the basis for TPS and success metrics.

## Artifacts

Smoke:

- `reports/artifacts/preseed-v061/simapp-treedb-preseed100-smoke.json`

100k preseed:

- `reports/artifacts/preseed-v061/simapp-goleveldb-preseed100k-msgarr-multisend-3x50x20x25-gas300m.json`
- `reports/artifacts/preseed-v061/simapp-treedb-preseed100k-msgarr-multisend-3x50x20x25-gas300m.json`

250k preseed:

- `reports/artifacts/preseed-v061/simapp-goleveldb-preseed250k-msgarr-multisend-3x50x20x25-gas300m.json`
- `reports/artifacts/preseed-v061/simapp-treedb-preseed250k-msgarr-multisend-3x50x20x25-gas300m.json`

## Results

| Preseed | Backend | Genesis accounts | Launch s | Wall s | Runtime TPS | Effective ops/s | Commit s | Finalize s | App DB before MB | App DB after MB | App DB delta MB | Max mem GiB | Max block write MB |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100k | goleveldb | 105000 | 165.372 | 231.955 | 3.3645 | 1682.27 | 2.5575 | 3.0058 | 0.005 | 63.805 | 63.799 | 2.011 | 299 |
| 100k | treedb | 105000 | 162.008 | 228.554 | 3.4724 | 1736.19 | 3.2583 | 3.1681 | 4.311 | 62.027 | 57.716 | 2.496 | 336 |
| 250k | goleveldb | 255000 | 364.839 | 431.467 | 3.4146 | 1707.28 | 6.6019 | 5.9867 | 0.005 | 151.150 | 151.145 | 4.124 | 708 |
| 250k | treedb | 255000 | 357.037 | 423.708 | 3.4380 | 1719.00 | 6.9820 | 6.2108 | 4.307 | 142.484 | 138.177 | 4.814 | 468 |

## Interpretation

The larger preseeded state did not expose a meaningful TreeDB throughput win in this Ironbird workload.

At 100k preseeded accounts, TreeDB was about 3.2% ahead on runtime effective ops/s. At 250k preseeded accounts, TreeDB was about 0.7% ahead. Those differences are small enough that I would treat them as noise or at most weak directionality, not as a compelling storage-engine result.

Commit time scaled with the number of preseeded accounts for both backends:

- 100k: LevelDB `2.56s`, TreeDB `3.26s`
- 250k: LevelDB `6.60s`, TreeDB `6.98s`

That suggests the enlarged genesis state does increase app-state commit work, but the workload is still not dominated by a TreeDB-favorable storage regime. The transaction execution path and SDK workload structure are still likely the main limiters.

There are two useful signals:

- TreeDB used less app DB space after the workload in both larger runs.
- TreeDB wrote less block data at 250k preseed (`468 MB` vs `708 MB`), but this did not translate into a large throughput win.

## Harness Limitation

The current preseed implementation is operationally useful for 100k and 250k accounts, but it is not a clean production replay model. Launch time is dominated by local deterministic wallet/account construction and JSON genesis injection:

- 100k preseed: about `162-165s`
- 250k preseed: about `357-365s`

This is separate from the transaction workload and should not be interpreted as database performance. It also means scaling to much larger states with this exact implementation will mostly benchmark the harness unless inactive preseed accounts are generated through a faster address-only path or loaded from a reusable state snapshot.

## Decision Gate

This sprint produced a valid preseeded-state Ironbird A/B, but it did not find the desired large TreeDB uplift.

Recommended next step: do not keep increasing synthetic account count in this same harness as the primary path. The better next experiment is a reusable production-shaped state snapshot or replay-style lane, where the initial state is loaded once and the measured phase is block execution against that state. If we keep this preseed lane, the next implementation improvement should make inactive accounts address-only and/or cache generated genesis/state so large-state A/B runs do not spend most of their time before chain startup.
