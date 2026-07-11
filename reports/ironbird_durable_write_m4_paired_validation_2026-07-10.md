# Ironbird TreeDB Durable-Write M4 Paired Validation

Date: 2026-07-10 HST

## Executive Conclusion And Nonclaims

<!-- M4_DATA: final gate disposition, close/continue decision, and blocker URL if required. -->

This report attributes only differences measured by the accepted alternating
baseline/candidate pairs. The LevelDB rows are matching drift controls, not a
claim that one backend is universally faster. The profiled candidate row is
diagnostic and is excluded from canonical throughput statistics.

The M3 forced-pointer production branch was inactive in its production
diagnostic. Its diagnostic throughput is not included here and no M4
throughput change is attributed to M3. M4 itself changes no TreeDB runtime
code and receives zero direct TPS credit.

## Revisions And Consumed Dependency Proof

| Component | Baseline | Candidate / shared pin |
| --- | --- | --- |
| Ironbird execution harness | `1bc048ec1d3f17da88d0187e80b2e8467166a7ff` | same |
| Ironbird matrix/analyzer | n/a | `9009fe9bb5de7abac7a17f185f15a464420b6413` before data collection |
| Cosmos SDK simapp | `494824795d0b9eabf318aba755ee3320462df7ad` | same |
| cosmos-db | `6ddcb75557e59bc4e6668ac7699cd52b63b3e402` | same |
| TreeDB (`snissn/gomap`) | `9cd9c6874860d2988002701bef042e50ba142cd0` | `09a626cd8f10fa161ef7f259d43b6567ea3e8abb` |
| IAVL | `12a26715119bb3ea55289ffd7b256161effc7b8b` | same |
| cometbft-db | `b4f87847a725f92a046d927ce4a0f5b08b965995` | same |
| CometBFT | `87379c903cc82c03874b24a6e3f9045784ba4681` | same |

The candidate SHA resolves to Go pseudo-version
`v0.6.2-0.20260711063646-09a626cd8f10`; the baseline resolves to
`v0.6.2-0.20260709230517-9cd9c6874860`. The harness rejects a pseudo-version
whose 12-character suffix does not match its separately supplied full
40-character commit.

The two immutable image tags and IDs were:

| Role | Image tag | Image ID | Extracted `simd` SHA-256 |
| --- | --- | --- | --- |
| baseline | `ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-9cd9c68-comet-87379c9` | `sha256:8284008cb6ef8316159982fbcaa29d675c72f9e04e8cacda0773a3e2a25343f4` | `171f4d1966e6cb1e7deda330644dca4a839d408633bff4d10d4aaee5ace4e00c` |
| candidate | `ironbird-report:snissn-sdk-4948247-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-09a626c-comet-87379c9` | `sha256:d6ba490684e81aa9c4141dccef7287dbeddc6ce922a2d74a1360bb1aee9baf29` | `fda17e14a4b5601d4adb916bc42e8882f1b33d250145187981297436728d58af` |

Host-side `go version -m` on the extracted candidate binary reports Go
1.26.5 and the exact replacements for candidate gomap, cosmos-db, IAVL,
cometbft-db, and CometBFT. Every accepted JSON row separately records the
full dependency-pin array, build replacement command, ref-specific image tag,
and observed runtime backend. The LevelDB controls use the candidate-graph
binary while selecting `goleveldb` for both application and node DBs, so
TreeDB code is compiled but inactive there.

The clean runner binary is
`/mnt/fast4tb/bin/ironbird-local-report-runner-3658-1bc048e`, SHA-256
`c4ff86b43d0167c8c1dd56f30fd7fe24ff037d505d05e8b9f3f7c693df005cfc`.

## Preserved M0 Baseline

The complete 872 MiB M0 directory remains at
`/mnt/fast4tb/ironbird-durable-m0-baseline-20260710`. M4 does not modify or
reuse it as a writable output directory. A 44-file checksum inventory is at
`/mnt/fast4tb/ironbird-durable-m4-validation-20260710/m0-preserved-sha256.txt`;
the inventory SHA-256 is
`118d1ec1449f404d0c5c8f68d76fcedab37d151338482573cbee17184133bfed`.

The preserved M0 canonical result remains TreeDB 649.291 TPS mean with 6.476%
sample RSD and LevelDB 613.104 TPS mean with 2.737% RSD over five rows. Those
numbers explain why M4 reruns the exact baseline commit in each pair rather
than comparing a new candidate only to an old aggregate.

## Workload, Host, Acceptance, And Timing Boundary

Every canonical row used one validator, zero non-validator nodes, the `kv`
transaction indexer, 100,000 preseed accounts plus 5,000 active wallets
(105,000 genesis accounts), 500 plain `MsgSend` transactions per requested
block, and a 450-block / 225,000-transaction target. TreeDB or LevelDB was
selected consistently across the application and all CometBFT DBs.

Acceptance required all of the following:

- at least 223,875 included transactions (99.5% of 225,000);
- successful transactions equal included transactions;
- an app-metric load window of at least 300 seconds;
- no runner/result error;
- valid application/node backend and `kv` indexer verification;
- exact gomap full SHA, pseudo-version, and image tag consumption.

The throughput denominator is the app-metric accepted window from the first
accepted metric boundary through target inclusion. It excludes image build,
chain launch, genesis/preseed, and post-window cleanup. Catalyst is stopped at
the accepted boundary and raw transaction lookup/audit is disabled. Exact
CometBFT span deltas and TreeDB counter deltas use the same accepted-window
before/after scrapes.

The host was an Intel i5-11400F (12 logical CPUs), 31 GiB RAM, Linux
6.8.0-124, Docker 29.1.3, and Go 1.25.0 for the runner; the simapp image used
Go 1.26.5. Heavy DB state and artifacts lived on `/mnt/fast4tb`, an NVMe-backed
filesystem. Canonical rows began only when no other simapp/Ironbird benchmark
container was present. Two pre-window baseline starts overlapped an unrelated
Go compile, were stopped before measurement, and are retained as rejected
artifacts; neither appears in the statistics below.

## Canonical Row Order And Acceptance

<!-- M4_DATA: five-pair row table, rejection details, timing, host notes. -->

The order permutations were:

1. baseline TreeDB, candidate TreeDB, LevelDB;
2. candidate TreeDB, LevelDB, baseline TreeDB;
3. LevelDB, baseline TreeDB, candidate TreeDB;
4. candidate TreeDB, baseline TreeDB, LevelDB;
5. LevelDB, candidate TreeDB, baseline TreeDB.

<!-- M4_DATA: explain the three-pair extension trigger. -->

## Per-Row And Paired Statistics

<!-- M4_DATA: TPS, tx/block, cadence rows; mean/median/RSD; paired deltas and exact bootstrap CI. -->

The confidence interval is an exact paired nonparametric bootstrap over all
`n^n` resamples of the pair ratios. With five pairs it is dispersion evidence,
not a high-powered population estimate.

## Stage Attribution And LevelDB Control

<!-- M4_DATA: stage medians/RSD/paired effects and gross/net control gap. -->

`consensus commit` is a parent. `SaveBlock` and `ApplyVerifiedBlock` are
children. App commit is nested under the mempool-lock-held portion of state
block commit, while state save is another child of `ApplyVerifiedBlock`.
The async tx-index total can overlap consensus execution and is never added to
the synchronous stage total. The synchronous control gap is reported both as
the gross `SaveBlock + app commit + state save` delta and after the
`SaveTxInfo` offset.

## TreeDB Durability And Checkpoint Counters

<!-- M4_DATA: per-store WriteSync/wait/checkpoint/command-WAL tables and M1 reasons. -->

M1 separates write waits into `frontier_cutover`, `checkpoint_drain`, and
`maintenance`. The checkpoint gate counts only frontier-cutover plus
checkpoint-drain wall time; maintenance remains visible separately. Because a
monotonic max counter cannot be differenced into an exact interval maximum,
the after-value is reported as a conservative upper bound only when the
accepted window added reason samples.

## Candidate Profile, CPU, Allocation, GC, Memory, And Disk

<!-- M4_DATA: profile paths/tops, CPU, alloc, block, mutex, trace, GC, RSS/HWM, IO, storage. -->

The representative candidate profile uses the exact fixture and merged graph
but is labeled diagnostic because profile collection adds overhead. Its CPU,
allocs, heap, goroutine, block, mutex, and trace artifacts are not substituted
for any canonical row.

## Correctness And Durability Evidence

The canonical scenarios use the default durable TreeDB command-WAL profile.
WAL sync, value-log ordering, automatic checkpointing, and strict public
`Checkpoint()` remain enabled; no relaxed, cached-only, WAL-off, threshold,
or disabled-checkpoint profile is used.

<!-- M4_DATA: cite exact M1/M2/M3 merged recovery/reopen/failpoint/race tests and local harness tests. -->

The M1 contract admits point writes only after a command-WAL frontier cut and
keeps post-cut frames in a fresh recovery-owned generation. Range-span,
hookless, cached-WAL, WAL-off, publication, backend durability, cleanup, and
maintenance paths retain their required gates. M2 proved the operation-to-
syscall ledger rather than treating logical sync counters as duplicate kernel
syncs. M3 safely coalesced a forced-pointer value-log sync, but the production
fixture did not exercise that branch and no production throughput credit is
claimed.

## Expected Versus Measured Outcome By Child

| Child | Expected direct effect | Measured / credited effect |
| --- | --- | --- |
| #3654 M0 measurement | 0 TPS / 0% | <!-- M4_DATA --> |
| #3655 M1 checkpoint coordination | realistic 0% to +1%; optimistic +2% to +4% if async backpressure mattered | <!-- M4_DATA --> |
| #3656 M2 durability/syscall audit | 0 TPS / 0% | <!-- M4_DATA --> |
| #3657 M3 small durable WriteSync | realistic +1.5% to +2.7%; optimistic +3.1% to +4.7% | measured no-go; forced-pointer branch inactive in production, 0 production TPS credited |
| #3658 M4 validation/report | 0 TPS / 0% | 0 direct TPS; reports only the combined merged graph |

Child maxima overlap and are not summed.

## Gate Disposition, Residual, And Close/Continue Decision

<!-- M4_DATA: all #3658 and #3652 gates, measured residual, blocker/close decision. -->

## Reproduction Commands And Artifact Paths

Canonical pair range:

```sh
cd /mnt/fast4tb/worktrees/ironbird-3658-m4
RUNNER=/mnt/fast4tb/bin/ironbird-local-report-runner-3658-1bc048e \
OUT_ROOT=/mnt/fast4tb/ironbird-durable-m4-validation-20260710 \
START_PAIR=1 END_PAIR=3 \
scripts/ironbird_durable_m4_matrix.sh

RUNNER=/mnt/fast4tb/bin/ironbird-local-report-runner-3658-1bc048e \
OUT_ROOT=/mnt/fast4tb/ironbird-durable-m4-validation-20260710 \
START_PAIR=4 END_PAIR=5 \
scripts/ironbird_durable_m4_matrix.sh
```

Analysis:

```sh
python3 scripts/analyze_ironbird_durable_m4.py \
  /mnt/fast4tb/ironbird-durable-m4-validation-20260710 \
  --pairs 5 \
  --output /mnt/fast4tb/ironbird-durable-m4-validation-20260710/analysis.json
```

Representative candidate profile:

```sh
/mnt/fast4tb/bin/ironbird-local-report-runner-3658-1bc048e \
  -scenario simapp-treedb-all -skip-build \
  -simapp-gomap-version v0.6.2-0.20260711063646-09a626cd8f10 \
  -simapp-gomap-ref 09a626cd8f10fa161ef7f259d43b6567ea3e8abb \
  -validators 1 -nodes 0 -wallets 5000 \
  -preseed-profile accounts -preseed-accounts 100000 \
  -cosmos-txs 500 -cosmos-blocks 450 -tx-indexer kv \
  -load-window-min-duration 5m -load-window-target-fraction 0.995 \
  -load-window-drain-timeout 5m -stop-catalyst-after-load-window \
  -app-debug-vars -raw-tx-audit=false \
  -app-active-window-profile-dir \
    /mnt/fast4tb/ironbird-durable-m4-validation-20260710/profile-candidate/pprof \
  -app-active-window-profile-duration 60s \
  -out /mnt/fast4tb/ironbird-durable-m4-validation-20260710/profile-candidate/result.json
```

Artifact root:

```text
/mnt/fast4tb/ironbird-durable-m4-validation-20260710
```
