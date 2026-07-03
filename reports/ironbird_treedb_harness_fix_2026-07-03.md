# Ironbird TreeDB Harness Fix Report

Date: 2026-07-03 UTC.

Branch: `codex/treedb-ironbird-report` in `snissn/ironbird`.

Status: local branch-ready harness fix. This report is self-contained; large JSON
and pprof artifacts remain under `reports/artifacts/` and are intentionally not
tracked.

## Goal

Make Ironbird TreeDB versus goleveldb throughput runs credible enough to decide
whether the current benchmark shape exposes a real app-state database signal.

The harness previously mixed three different concerns:

- Catalyst result accounting, which can mark successful SDK transactions as
  failed.
- Per-transaction raw RPC audit, which is useful for correctness but pollutes
  performance windows.
- Whole-run timing, which includes post-load sleep and transaction result
  collection.

## Harness Changes

- Added captured Catalyst task logs to load-test responses so local audit and
  reporting code can inspect the actual emitted transaction hashes.
- Added Docker task log retrieval.
- Added `-raw-tx-audit`; the default keeps the existing correctness audit, while
  `-raw-tx-audit=false` allows cleaner performance runs.
- Added corrected SDK load accounting from app metrics, but only when Catalyst
  appears to have misclassified the run.
- Added a load-window monitor that starts immediately before the load test and
  stops when app metrics show the intended transaction target was reached.
- Made load-window derived rates use the load-window observation counts rather
  than later whole-run counts.
- Kept low-latency simapp consensus and mempool tuning for throughput sweeps.
- Added app CPU profile plumbing through validator start flags and profile
  artifact collection.
- Fixed local Docker/simapp usability issues: configurable Go builder image,
  safe local activity workflow ID fallback, and EVM account passphrase handling.
- Ignored `reports/artifacts/` so generated JSON, profiles, traces, and logs do
  not pollute the branch.

## Validation

Focused test suite:

```sh
GOWORK=off go test ./cmd/local-report-runner ./activities/loadtest ./activities/testnet ./petri/core/provider/docker ./petri/cosmos/chain ./messages
```

Result: pass.

Final smoke run:

```sh
TMPDIR=/mnt/fast4tb/tmp /mnt/fast4tb/tmp/local-report-runner-clean \
  -scenario simapp-goleveldb \
  -skip-build \
  -validators 1 \
  -nodes 0 \
  -wallets 10 \
  -cosmos-msg MsgSend \
  -cosmos-blocks 1 \
  -cosmos-txs 1 \
  -cosmos-max-gas 1000000 \
  -raw-tx-audit=false \
  -out reports/artifacts/harness-final-smoke-20260703T090303Z/simapp-goleveldb-smoke.json
```

Result: pass. Catalyst reported the single SDK transaction as failed, while app
metrics corrected it to 1 included and 1 successful transaction. The
load-window monitor reached the 1 transaction target in 2.072s.

## Clean Load-Window Evidence

Artifact directory:

`reports/artifacts/throughput-loadwindow-20260703T034149Z`

Scenario:

- 1 validator, 0 full nodes
- 5,000 active wallets
- 100,000 inactive preseeded accounts
- 8 load blocks
- 100 outer tx per block
- `MsgArr` with 20 contained `MsgMultiSend` messages
- 25 recipients per contained `MsgMultiSend`
- 800 successful tx total
- 400,000 effective recipient operations

| Metric | TreeDB | goleveldb | TreeDB / goleveldb |
| --- | ---: | ---: | ---: |
| Load window seconds | 36.533 | 36.540 | 1.000x |
| Included tx/s in load window | 21.898 | 21.894 | 1.000x |
| Effective ops/s in load window | 10,949.0 | 10,946.9 | 1.000x |
| Load-window ABCI observed seconds | 24.068 | 23.798 | 1.011x |
| Load-window commit seconds | 5.872 | 3.701 | 1.587x |
| Average commit ms | 51.96 | 33.34 | 1.559x |
| Load-window finalize seconds | 10.114 | 11.449 | 0.883x |
| Load-window check_tx seconds | 7.660 | 8.123 | 0.943x |
| App DB delta | 73.77 MiB | 65.67 MiB | 1.123x |
| Data dir delta | 169.09 MiB | 159.35 MiB | 1.061x |
| Validator max RSS | 2.808 GiB | 2.368 GiB | +0.440 GiB |
| Validator block read | 469 MB | 144 MB | 3.257x |
| Validator block write | 1.93 GB | 1.40 GB | 1.379x |

## Interpretation

The cleaned load-window result shows tied throughput in this workload. TreeDB
has a real commit-wall, RSS, and IO disadvantage here, but commit is not large
enough to explain or create a large TPS gap:

- Reducing TreeDB commit to goleveldb's commit time would save about 2.17s over
  a 36.53s load window, at most about 1.06x.
- Removing TreeDB commit entirely would cap the speedup near 1.19x.

This points to a harness/workload-shape problem rather than an obvious TreeDB
CPU bottleneck. The production Celestia sync result remains the stronger
evidence that TreeDB can help in a storage traversal/sync regime.

## Next Work

- Add a send-only or app-metrics-only Catalyst mode that skips fixed post-load
  sleep and per-transaction collection for performance runs.
- Capture load-window-only CPU, block, mutex, and trace profiles.
- Add production-shaped state beyond account count: validators, delegations,
  auth/account shapes, bank balances, IBC-like keys, and larger historical
  state.
- Add query-only and replay/sync-style workloads, because those are closer to
  the regime where Celestia sync shows TreeDB uplift.
- Add TreeDB-specific counters for command WAL append/sync, backend flush,
  checkpoint, mmap/page-fault behavior, value-log writes, and lock wait.
