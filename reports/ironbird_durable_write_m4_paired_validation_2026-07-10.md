# Ironbird TreeDB Durable-Write M4 Paired Validation

<!-- markdownlint-disable MD013 -->

Date: 2026-07-10 HST

## Executive Conclusion And Nonclaims

The merged graph passes every required M4 and umbrella north-star gate. Across
five accepted alternating pairs, candidate TreeDB throughput improved by
2.816% at the paired median and 4.908% at the paired geometric mean; four of
five pairs were positive and the exact paired bootstrap 95% interval for the
geometric mean was +1.503% to +8.898%. All durability-stage, asynchronous
tx-index, synchronous-control-gap, correctness, memory, and storage gates also
pass.

The measured disposition is therefore **close the optimization graph after
this report PR is merged and linked**. No replacement blocker is required.
M4 does not itself earn throughput credit, and the result is not a claim that
the synthetic fixture predicts production TPS or that every child supplied an
independently separable share of the combined improvement.

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
| Ironbird matrix/analyzer | n/a | `db150823d44af85b3141674029b68d916c9a3d7b` final execution source; the first accepted baseline result was emitted at `f1e987652eac35ddc32c4ebf381f54e9766036f6` before the summary-ledger-only fix |
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

All canonical rows passed on their first measured attempt:

| Pair | Position | Row | Window (s) | Included / successful | TPS | Tx/block | Cadence (ms/block) |
| ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | baseline TreeDB | 331.528 | 223,998 / 223,998 | 675.653 | 190.796 | 282.392 |
| 1 | 2 | candidate TreeDB | 305.026 | 223,985 / 223,985 | 734.315 | 179.806 | 245.395 |
| 1 | 3 | candidate-graph LevelDB | 349.027 | 223,996 / 223,996 | 641.773 | 203.077 | 316.434 |
| 2 | 1 | candidate TreeDB | 306.529 | 223,993 / 223,993 | 730.740 | 181.397 | 248.806 |
| 2 | 2 | candidate-graph LevelDB | 325.033 | 223,995 / 223,995 | 689.145 | 210.718 | 305.770 |
| 2 | 3 | baseline TreeDB | 315.525 | 224,298 / 224,298 | 710.872 | 191.767 | 270.142 |
| 3 | 1 | candidate-graph LevelDB | 390.035 | 223,993 / 223,993 | 574.289 | 194.270 | 338.279 |
| 3 | 2 | baseline TreeDB | 321.541 | 223,895 / 223,895 | 696.319 | 191.353 | 275.292 |
| 3 | 3 | candidate TreeDB | 323.027 | 223,998 / 223,998 | 693.435 | 177.635 | 256.167 |
| 4 | 1 | candidate TreeDB | 309.528 | 224,150 / 224,150 | 724.168 | 183.747 | 253.919 |
| 4 | 2 | baseline TreeDB | 318.027 | 223,998 / 223,998 | 704.336 | 189.421 | 269.058 |
| 4 | 3 | candidate-graph LevelDB | 331.529 | 223,997 / 223,997 | 675.649 | 207.405 | 306.971 |
| 5 | 1 | candidate-graph LevelDB | 336.025 | 223,998 / 223,998 | 666.612 | 208.953 | 313.456 |
| 5 | 2 | candidate TreeDB | 302.555 | 224,040 / 224,040 | 740.494 | 178.474 | 241.079 |
| 5 | 3 | baseline TreeDB | 336.025 | 223,994 / 223,994 | 666.599 | 190.958 | 286.466 |

Canonical collection ran from 21:12:32 through 23:16:27 HST on July 10.
Every result has an empty error, valid app/node backend and `kv` indexer, and
the expected full gomap ref, pseudo-version, and immutable image tag.

Two setup-only pair-1 baseline starts overlapped an unrelated Go compile and
were stopped before any accepted window. Their logs remain under
`pair-1/baseline-treedb-rejected-attempt-{1,2-prewindow-contention}`; neither
produced a result or enters the ledger. The first valid baseline then passed
all predicates, but the shell summary append hit jq's reserved `$label` token.
The independently accepted JSON was retained without rerunning it, the
ledger-only bug was fixed with a regression test, and subsequent rows resumed
under the unchanged workload protocol.

The order permutations were:

1. baseline TreeDB, candidate TreeDB, LevelDB;
2. candidate TreeDB, LevelDB, baseline TreeDB;
3. LevelDB, baseline TreeDB, candidate TreeDB;
4. candidate TreeDB, baseline TreeDB, LevelDB;
5. LevelDB, candidate TreeDB, baseline TreeDB.

After three pairs, candidate TPS RSD was 3.147% and consensus-commit RSD was
3.110%, both above the 3% trigger. The three LevelDB controls also had 9.089%
RSD (641.773, 689.145, and 574.289 TPS), and pair 3 was mildly negative at
-0.414%. The required three-pair analysis was frozen at
`analysis-3-pairs.json` (SHA-256
`a63ec2f5874b191c3844f814a21cc046e937ec21fd82efd03f6022673a82b866`),
then the predetermined pair-4 and pair-5 permutations were run. The final
candidate TPS RSD fell to 2.541%; some stage/control RSDs remained above 3%,
which is reported as host/stage dispersion rather than hidden by more ad hoc
runs.

## Per-Row And Paired Statistics

| Metric | Baseline median (RSD) | Candidate median (RSD) | LevelDB median (RSD) |
| --- | ---: | ---: | ---: |
| TPS | 696.319 (2.739%) | 730.740 (2.541%) | 666.612 (6.999%) |
| transactions/block | 190.958 (0.465%) | 179.806 (1.351%) | 207.405 (3.209%) |
| cadence, ms/block | 275.292 (2.748%) | 248.806 (2.469%) | 313.456 (4.151%) |

| Paired candidate vs baseline | Per-pair change | Median | Geometric mean | Exact bootstrap 95% | Positive pairs |
| --- | --- | ---: | ---: | ---: | ---: |
| TPS | +8.682%, +2.795%, -0.414%, +2.816%, +11.085% | **+2.816%** | **+4.908%** | **+1.503% to +8.898%** | **4/5** |
| transactions/block | -5.760%, -5.408%, -7.169%, -2.995%, -6.538% | -5.760% | -5.585% | -6.664% to -4.198% | 0/5 |
| cadence | -13.101%, -7.898%, -6.947%, -5.628%, -15.843% | -7.898% | -9.969% | -13.583% to -6.613% | 0/5 |

The candidate reached the same transaction target with fewer transactions per
block but sufficiently faster cadence to improve accepted-window TPS. The
final analysis is `analysis.json`, SHA-256
`8d1ea2f939421077d540b5890a9f1006c96e288b782ea92ed644d9fcfddaa392`.

The confidence interval is an exact paired nonparametric bootstrap over all
`n^n` resamples of the pair ratios. With five pairs it is dispersion evidence,
not a high-powered population estimate.

## Stage Attribution And LevelDB Control

| Exact stage (ms/block) | Baseline median | Candidate median (RSD) | LevelDB median | Paired median / geomean change |
| --- | ---: | ---: | ---: | ---: |
| consensus commit | 201.923 | 177.108 (3.030%) | 185.632 | -10.446% / -12.718% |
| SaveBlock (`commit blockstore`) | 20.724 | 13.918 (3.361%) | 13.625 | -32.958% / -32.805% |
| state app commit | 40.768 | 33.032 (2.431%) | 33.650 | -17.094% / -18.400% |
| state save | 19.850 | 13.280 (3.605%) | 12.569 | -33.157% / -32.666% |
| SaveTxInfo | 5.219 | 5.412 (3.041%) | 6.909 | +1.399% / +2.651% |
| async tx-index block total | 48.664 | **34.184 (2.757%)** | 35.520 | **-29.385% / -31.608%** |

For the three synchronous TreeDB-owned comparison stages (SaveBlock, app
commit, and state save), candidate minus matching LevelDB was +0.177 ms/block
at the five-pair median and +4.214 ms/block at the worst pair. After also
offsetting candidate TreeDB's SaveTxInfo advantage, the net median was -1.563
ms/block and the worst pair was +3.008 ms/block. Both forms pass the <=10 ms
gate. The net worst pair was 3.008 ms/block, near but not strictly inside the
3 ms stretch threshold.

The median cadence remainder after subtracting the nested consensus-commit
parent was 73.092 ms/block for baseline and 70.510 ms/block for candidate. The
paired absolute residual changes were -7.027, -0.671, -2.859, +4.652, and
-2.919 ms/block. This remainder includes scheduling and uninstrumented work;
it is not added to the nested children or assigned to TreeDB.

`consensus commit` is a parent. `SaveBlock` and `ApplyVerifiedBlock` are
children. App commit is nested under the mempool-lock-held portion of state
block commit, while state save is another child of `ApplyVerifiedBlock`.
The async tx-index total can overlap consensus execution and is never added to
the synchronous stage total. The synchronous control gap is reported both as
the gross `SaveBlock + app commit + state save` delta and after the
`SaveTxInfo` offset.

## TreeDB Durability And Checkpoint Counters

Medians across the five canonical rows:

| Store | WriteSync ms/call, base -> cand | Compatibility wait s, base -> cand | Candidate checkpoint runs / auto | Candidate checkpoint wall (s) | Candidate command-WAL append calls / total s | flush calls / total s | sync calls / total s | Post-frontier admissions |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| application.db | 19.984 -> **13.542** | 2.675 -> 1.337 | 15 / 11 | 3.100 | 32,860 / 0.686 | 31,627 / 0.112 | 1,258 / 8.504 | 265 |
| blockstore.db | 12.692 -> **9.439** | 0.547 -> 0.142 | 15 / 10 | 2.814 | 1,982 / 0.022 | 0 / 0 | 2,500 / 14.175 | 9 |
| state.db | 19.815 -> **13.245** | 0.288 -> 0.097 | 11 / 10 | 1.548 | 3,729 / 0.059 | 1,243 / 0.183 | 2,497 / 16.491 | 11 |
| tx_index.db | 20.573 -> **13.323** | 11.179 -> 0.932 | 15 / 10 | 19.151 | 1,982 / 2.110 | 0 / 0 | 2,502 / 17.894 | 76 |

The tx-index compatibility wait still includes maintenance. The reason-specific
candidate median was one `frontier_cutover` sample totaling 0.010906 s, zero
`checkpoint_drain` samples, and one separate `maintenance` sample totaling
0.893114 s. Across all five rows, the conservative checkpoint-specific max
upper bound was only 19.957 ms, far below 250 ms. Thus the required
frontier-plus-drain median is **0.010906 s**, while the longer background
checkpoint wall (19.151 s median) remains visible and is not relabeled as
foreground wait. Application, blockstore, and state likewise had zero median
checkpoint-drain samples; their remaining compatibility waits were primarily
maintenance.

M1 separates write waits into `frontier_cutover`, `checkpoint_drain`, and
`maintenance`. The checkpoint gate counts only frontier-cutover plus
checkpoint-drain wall time; maintenance remains visible separately. Because a
monotonic max counter cannot be differenced into an exact interval maximum,
the after-value is reported as a conservative upper bound only when the
accepted window added reason samples.

## Candidate Profile, CPU, Allocation, GC, Memory, And Disk

The diagnostic candidate row accepted 224,008/224,008 transactions over
356.032 s at 629.180 TPS with an empty error and exact candidate graph proof.
Profiling overhead is material, so this TPS is excluded from every paired
statistic.

All requested active-window artifacts are nonempty under `profile-candidate/pprof`:

| Artifact | Bytes | Summary |
| --- | ---: | --- |
| CPU | `simapp-treedb-all-validator-0-active-window-cpu.pprof`, 289,611 | 83.97 CPU-s sampled over 60 s; runtime scan/GC and secp256k1 dominate; no TreeDB function is a flat top-30 CPU owner |
| allocations | `...-allocs.pprof`, 225,014 | 19.56 GB sampled; TreeDB raw-KV payload construction 0.61 GB, segment reads 0.37 GB, command-frame decode 0.36 GB, initial memtable 0.31 GB |
| heap / goroutine | `...-heap.pprof`, 224,717; `...-goroutine.pprof`, 10,412 | retained-heap and goroutine snapshots |
| block | `...-block.pprof`, 29,597 | process-wide delay is dominated by mempool mutex/select/receive paths, not a TreeDB checkpoint-drain sample |
| mutex | `...-mutex.pprof`, 65,429 | process-wide delay is dominated by runtime/consensus mutex release attribution |
| trace | `...-trace.out`, 58,528,008 | Go 1.26 trace; matching Go 1.26.5 produced `trace-sched.pprof` (149,308) and `trace-sched.top.txt` (3,066) |

`cpu.top.txt`, `allocs.top.txt`, `heap.top.txt`, `goroutine.top.txt`,
`block.top.txt`, and `mutex.top.txt` sit beside the raw profiles. The host Go
1.25 trace reader was not used for attribution because it cannot decode Go
1.26 traces; the matching 1.26.5 toolchain produced the cited nonempty trace
summary.

The full profiled window used 603.11 process CPU-s (1.694 core-equivalent),
allocated 128.273 GB across 1.604 billion mallocs, completed 87 GC cycles with
71.180 ms aggregate pause, ended at 5.746 GiB RSS, and reached 5.588 GiB peak
container memory. Its final `/simd/data` footprint was 4.006 GiB and Docker
reported 7.49 GB block writes.

Canonical unprofiled resource medians provide the regression comparison:

| Metric | Baseline | Candidate | Change |
| --- | ---: | ---: | ---: |
| process CPU-s / core-equivalent | 520.53 / 1.613 | 531.44 / 1.742 | +2.10% total CPU; +8.02% concurrency |
| allocated bytes / mallocs | 124.976 GB / 1.584B | 127.227 GB / 1.592B | +1.80% / +0.53% |
| GC cycles / aggregate pause | 80 / 15.174 ms | 83 / 15.599 ms | +3.75% / +2.81% |
| ending RSS / peak container memory | 5.819 / 5.768 GiB | 5.767 / 5.867 GiB | -0.89% / +1.72% |
| final data directory | 3.789 GiB | 3.950 GiB | +4.26% |
| Docker block writes | 7.21 GB | 7.44 GB | +3.19% |
| consensus blocks | 1,173 | 1,243 | +5.97% |
| final data bytes / consensus block | 3.483 MB | 3.435 MB | -1.38% |
| Docker write bytes / consensus block | 6.173 MB | 5.933 MB | -3.88% |

The absolute final-footprint and per-transaction increases are real and are
not erased by normalization. They coincide with the candidate producing 5.97%
more consensus blocks for the same transaction target because its median
transactions/block fell by 5.760%. With the identical genesis and transaction
fixture, median final bytes/block improved 1.38% and Docker write bytes/block
improved 3.88%, so the systematic absolute increase follows additional block
metadata/work rather than worse per-block storage or write intensity. Together
with -0.89% RSS and only +1.72% peak memory, this supports the scoped
non-material disposition while preserving the absolute tradeoff. LevelDB is
structurally different (1.353 GiB data but 37.8 GB median block writes) and is
not used as the TreeDB footprint baseline.

The representative candidate profile uses the exact fixture and merged graph
but is labeled diagnostic because profile collection adds overhead. Its CPU,
allocs, heap, goroutine, block, mutex, and trace artifacts are not substituted
for any canonical row.

## Correctness And Durability Evidence

The canonical scenarios use the default durable TreeDB command-WAL profile.
WAL sync, value-log ordering, automatic checkpointing, and strict public
`Checkpoint()` remain enabled; no relaxed, cached-only, WAL-off, threshold,
or disabled-checkpoint profile is used.

The merged child chain and canonical evidence are:

- M0 #3654 / PR #3663 merged as `2f8e391ccb3aa3f44ab4b417cace5abab49d51c7`
  from reviewed head `eba6273ec6fe212b0d5f0382ec35b90d77b8385e`
  ([evidence](https://github.com/snissn/gomap/issues/3654#issuecomment-4940614381)).
- M1 #3655 / PR #3692 merged as `3e5543145e2f4fd0762b6bdeb1bbcbcb42599f6c` from reviewed head
  `1bce6208374fa3ffc0d5e2a7333a117b6173eb7f`
  ([evidence](https://github.com/snissn/gomap/issues/3655#issuecomment-4941305639)).
  Exact repeated/race coverage includes
  `TestCachingDB_CheckpointExternalCommandWALAdmitsAfterFrontierCut`,
  `TestCachingDB_CheckpointDrainRetainsWriterGateWithoutCommandWALCutover`,
  `TestPublicCommandWALAutoCheckpointOverlapAdmitsPostFrontierWrites`,
  `TestPublicCommandWALCheckpointPostFrontierAdmissionPropagatesPublishError`,
  `TestPublicCommandWALCheckpointPostFrontierGenerationSurvivesCrashReopen`,
  `TestPublicCommandWALCheckpointDefaultCutoverAdmitsPostFrontierWriteSync`,
  and the range-write/histogram publication regressions.
- M2 #3656 / PR #3705 merged as `851d8e89c1e38fd1af6edc682ebe1722852d4215`
  from final head `91113ab9f4763b5b0434a3435fc2149fd1c8a272`
  ([evidence](https://github.com/snissn/gomap/issues/3656#issuecomment-4942120718)).
  `GOWORK=off go test ./TreeDB/... -count=1`, focused race tests, Windows
  writer cross-compilation, forced-pointer syscall reconciliation, and crash
  points before external-value durability and after command durability all
  passed. The ledger proves one dirty inline batch has one command-WAL fsync
  and preserves the ordered value-log-before-command barrier.
- M3 #3657 / PR #3710 merged as the candidate
  `09a626cd8f10fa161ef7f259d43b6567ea3e8abb` from reviewed head
  `8bf44b75ddd0b4ba585061332ea8506506b7a4e4`
  ([bounded evidence](https://github.com/snissn/gomap/issues/3657#issuecomment-4942996659),
  [merge disposition](https://github.com/snissn/gomap/issues/3657#issuecomment-4943126081)).
  Exact guard/recovery coverage includes
  `TestCachingValueLogExternalRefSyncCoalescingGuards`,
  `TestCachingValueLogExternalRefFlusherSyncsRotatedSegments`,
  `TestFlushCommandWALBarrierOrdersExternalRefsBeforeCommandWAL`,
  `TestCrashRecovery_CommandWALDurableSyncedUncheckpointedFramesReplay`,
  `TestAppendRawKVCommandWALOrderedEntriesRejectsMalformedValueLogPointerBeforeDurability`,
  and their focused race run.

For M4 itself, focused runner tests, the matrix shell regression, and analyzer
reproduction pass at close. Broad Ironbird local/Docker packages also pass;
the only broad-suite failures are external DigitalOcean/workflow tests whose
Tailscale OAuth endpoint returned HTTP 401, unrelated to this harness.

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
| #3654 M0 measurement | 0 TPS / 0% | 0 direct TPS; supplied the preserved baseline, stage attribution, and noise trigger |
| #3655 M1 checkpoint coordination | realistic 0% to +1%; optimistic +2% to +4% if async backpressure mattered | checkpoint-specific tx-index wait fell to 0.010906 s median and <=19.957 ms conservative max; async tx-index fell 29.385% at the paired median; no separately isolated TPS credit |
| #3656 M2 durability/syscall audit | 0 TPS / 0% | 0 direct TPS; supplied the physical sync ledger and contract boundary consumed by M3/M4 |
| #3657 M3 small durable WriteSync | realistic +1.5% to +2.7%; optimistic +3.1% to +4.7% | measured no-go; forced-pointer branch inactive in production, 0 production TPS credited |
| #3658 M4 validation/report | 0 TPS / 0% | 0 direct TPS; reports only the combined merged graph |

Child maxima overlap and are not summed.

## Gate Disposition, Residual, And Close/Continue Decision

| Gate | Required | Measured | Disposition |
| --- | ---: | ---: | --- |
| paired TreeDB TPS | >=+2% median and geomean; majority positive | +2.816% median, +4.908% geomean, 4/5 positive, exact CI +1.503% to +8.898% | **PASS** |
| tx-index checkpoint wait | <=3.0 s; <=250 ms conservative max | 0.010906 s median; 19.957 ms worst upper bound | **PASS** |
| state.db WriteSync | <=16 ms/call | 13.245 ms median | **PASS** |
| application.db WriteSync | <=17 ms/call | 13.542 ms median | **PASS** |
| async tx-index | <=42 ms/block | 34.184 ms/block median | **PASS** |
| synchronous gap vs LevelDB | <=10 ms/block | gross median/worst +0.177/+4.214 ms; net -1.563/+3.008 ms | **PASS** |
| durability/recovery | no regression | merged recovery/reopen/failpoint/race evidence; default durable profile in every row | **PASS** |
| memory/storage | no material regression; explain tradeoff | RSS -0.89%, HWM +1.72%; data/write +4.26%/+3.19% absolute but -1.38%/-3.88% per block across +5.97% more blocks | **PASS, disclosed modest tradeoff** |

The remaining median cadence residual after consensus commit is 70.510
ms/block, but it is not a failed declared gate and the end-to-end confidence
interval excludes zero. Because the +2% throughput gate and every stage gate
pass, opening a replacement blocker would contradict #3658's conditional
failure action. The measured decision is to merge and link this report, then
close #3658 and umbrella #3652; no optimization work remains hidden in this
report.

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
