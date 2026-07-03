# Ironbird DB-Signal Sprint

Date: 2026-07-01 UTC.

Branch: `codex/treedb-ironbird-report` in `snissn/ironbird`.

Status: complete local sprint. This is a local sprint artifact for finding an Ironbird workload regime where database choice matters. It does not open or imply upstream PRs.

## Goal

Find whether current or shaped Ironbird workloads expose meaningful LevelDB versus TreeDB storage/commit share, then decide whether the harness can demonstrate a real TreeDB uplift or whether it is in the wrong regime.

The key distinction is workload regime:

- Production sync has already shown a real TreeDB win outside Ironbird.
- Microbenchmarks have already shown TreeDB throughput wins outside Ironbird.
- The current Ironbird transaction workloads may still be dominated by transaction generation, ABCI execution, consensus pacing, empty-block commit overhead, or harness timing.

## New Instrumentation

The local report runner now captures validator-side storage signals around the load-test phase:

- Prometheus metrics before and after load, including ABCI method timing.
- Data directory and application DB byte sizes before and after load.
- Derived commit shares, average commit/finalize seconds, tx deltas, process CPU delta, and byte deltas.

Primary JSON fields:

- `metrics_before`
- `metrics_after`
- `data_sizes_before`
- `data_sizes_after`
- `storage_signal_summary`

Validation:

```sh
GOWORK=off go test ./cmd/local-report-runner
```

Result: pass.

## Artifacts

| Artifact | Purpose |
| --- | --- |
| `reports/artifacts/db-signal/smoke-goleveldb-metrics.json` | LevelDB one-transaction smoke with storage metrics. |
| `reports/artifacts/db-signal/smoke-treedb-metrics.json` | TreeDB one-transaction smoke with storage metrics. |
| `reports/artifacts/db-signal/simapp-goleveldb-msgarr-multisend-3x50x20x25-gas300m-metrics.json` | LevelDB high-gas `MsgArr(MsgMultiSend)` storage-fanout lane with storage metrics. |
| `reports/artifacts/db-signal/simapp-treedb-msgarr-multisend-3x50x20x25-gas300m-metrics.json` | TreeDB high-gas `MsgArr(MsgMultiSend)` storage-fanout lane with storage metrics. |
| `reports/artifacts/db-signal/simapp-goleveldb-dense-1x300x20x25-gas2b-metrics.json` | LevelDB dense-block synthetic storage-fanout lane. |
| `reports/artifacts/db-signal/simapp-treedb-dense-1x300x20x25-gas2b-metrics.json` | TreeDB dense-block synthetic storage-fanout lane. |
| `reports/artifacts/db-signal/simapp-goleveldb-dense-1x300x20x25-gas2b-repeat2-metrics.json` | LevelDB dense-block repeat. |
| `reports/artifacts/db-signal/simapp-treedb-dense-1x300x20x25-gas2b-repeat2-metrics.json` | TreeDB dense-block repeat. |
| `reports/artifacts/db-signal/simapp-goleveldb-largegenesis-50k-msgarr-multisend-metrics.json` | LevelDB 50k-wallet larger-state proxy. |
| `reports/artifacts/db-signal/simapp-treedb-largegenesis-50k-msgarr-multisend-metrics.json` | TreeDB 50k-wallet larger-state proxy. |
| `reports/artifacts/db-signal/commitbench-goleveldb-20blocks.json` | LevelDB 20-block commit-only microbenchmark. |
| `reports/artifacts/db-signal/commitbench-treedb-20blocks.json` | TreeDB 20-block commit-only microbenchmark. |

## Smoke Results

Both smokes ran one successful `MsgSend` according to raw CometBFT audit. Catalyst still reports these SDK txs as failed because of its SDK response decoder issue, so raw audit remains the source of truth.

| Backend | Raw successful tx | Runtime TPS | Commit seconds | Commit count | Avg commit | Finalize seconds | Data dir delta | App DB delta | Validator write max | Validator mem max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | 1 | 0.0310 | 0.2557s | 21 | 12.17ms | 0.0042s | 409,249 B | 178,261 B | 2.03 MB | 69.81 MiB |
| TreeDB | 1 | 0.0331 | 1.8859s | 20 | 94.30ms | 0.0046s | 959,933 B | 739,373 B | 3.54 MB | 358.2 MiB |

Interpretation: the harness currently shows a large fixed TreeDB per-block commit cost even in a nearly idle smoke. This matters because many Ironbird runs include many empty or lightly loaded blocks while Catalyst waits for completion and collection.

## High-Gas Storage-Fanout Pair

Command shape:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-goleveldb -skip-build \
  -validators 1 -nodes 0 -wallets 5000 \
  -cosmos-blocks 3 -cosmos-txs 50 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 -cosmos-recipients 25 \
  -cosmos-max-gas 300000000 \
  -out reports/artifacts/db-signal/simapp-goleveldb-msgarr-multisend-3x50x20x25-gas300m-metrics.json
```

The TreeDB command is identical except `-scenario simapp-treedb` and the output path.

| Backend | Raw successful tx | Effective recipient ops | Runtime effective ops/s | Wall effective ops/s | Commit seconds | Finalize seconds | Commit share of ABCI | Commit share of commit+finalize | Avg commit | App DB delta | Validator write max | Validator mem max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | 150 | 75,000 | 1,710.43 | 835.35 | 0.2266s | 0.9011s | 15.1% | 20.1% | 7.81ms | 1,014,072 B | 90.7 MB | 371.4 MiB |
| TreeDB | 150 | 75,000 | 1,672.72 | 842.89 | 2.7577s | 1.0859s | 59.1% | 71.7% | 95.09ms | 3,197,302 B | 101 MB | 675.3 MiB |

Interpretation:

- The lane is storage-sensitive in the sense that TreeDB commit time is visible and material.
- It does not demonstrate TreeDB uplift. Runtime effective ops/s is slightly worse for TreeDB in this run, while wall effective ops/s is slightly better because total wall timing noise and setup/teardown dominate at this scale.
- The dominant difference is fixed per-block commit cost, not a clear transaction-driven storage throughput win.
- The run still has only 150 outer tx, so it is too small to make a durable throughput claim.

## Dense-Block Synthetic Pair

The dense-block probe packs twice as many outer txs into one requested tx block and raises block gas to 2B. This is intended to amortize the fixed empty/light-block commit tail and push more writes through the loaded section.

Command shape:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-goleveldb -skip-build \
  -validators 1 -nodes 0 -wallets 10000 \
  -cosmos-blocks 1 -cosmos-txs 300 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 -cosmos-recipients 25 \
  -cosmos-max-gas 2000000000 \
  -out reports/artifacts/db-signal/simapp-goleveldb-dense-1x300x20x25-gas2b-metrics.json
```

The TreeDB command is identical except `-scenario simapp-treedb` and the output path.

| Backend | Raw successful tx | Effective recipient ops | Runtime effective ops/s | Wall effective ops/s | Commit seconds | Finalize seconds | Commit share of ABCI | Commit share of commit+finalize | Avg commit | App DB delta | Validator write max | Validator mem max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | 300 | 150,000 | 4,671.79 | 1,489.00 | 0.4543s | 3.3902s | 8.9% | 11.8% | 15.67ms | 6,282,670 B | 161 MB | 1.402 GiB |
| TreeDB | 300 | 150,000 | 4,865.06 | 1,507.28 | 3.5228s | 2.3830s | 25.5% | 59.7% | 121.48ms | 5,496,282 B | 163 MB | 1.737 GiB |

TreeDB versus LevelDB in this single dense pair:

| Metric | Delta |
| --- | ---: |
| Runtime effective ops/s | +4.14% |
| Wall effective ops/s | +1.23% |
| App DB byte delta | -12.52% |
| Process CPU seconds delta | -7.15% |
| Validator memory max | +23.89% |
| Commit seconds | +675.43% |
| Finalize seconds | -29.71% |

Interpretation:

- This is the first local Ironbird DB-signal lane in this sprint that shows a positive TreeDB throughput delta.
- The lift is modest and should be treated as promising, not conclusive, until repeated.
- The mechanism is mixed: TreeDB is still much slower in commit, but faster enough in transaction/finalize work and app DB growth to win slightly on runtime effective ops/s.
- The workload is synthetic and should not be sold as production sync. It is useful because it finally reaches a regime where DB choice changes the result.

### Dense-Block Repeat

The dense-block pair was repeated with the same command shape, writing to `*-repeat2-metrics.json`.

| Backend | Raw successful tx | Effective recipient ops | Runtime effective ops/s | Wall effective ops/s | Commit seconds | Finalize seconds | Commit share of ABCI | Commit share of commit+finalize | Avg commit | App DB delta | Validator write max | Validator mem max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB repeat | 300 | 150,000 | 4,755.14 | 1,470.55 | 0.4838s | 2.8777s | 9.4% | 14.4% | 16.68ms | 7,883,209 B | 160 MB | 1.349 GiB |
| TreeDB repeat | 300 | 150,000 | 4,813.15 | 1,480.54 | 3.3978s | 3.4881s | 30.8% | 49.3% | 117.17ms | 8,401,252 B | 163 MB | 1.750 GiB |

TreeDB versus LevelDB in the repeat:

| Metric | Delta |
| --- | ---: |
| Runtime effective ops/s | +1.22% |
| Wall effective ops/s | +0.68% |
| App DB byte delta | +6.57% |
| Process CPU seconds delta | +2.55% |
| Validator memory max | +29.73% |
| Commit seconds | +602.35% |
| Finalize seconds | +21.21% |

Repeat interpretation:

- The small TreeDB throughput edge repeats directionally: +4.14% first run, +1.22% second run on runtime effective ops/s.
- The secondary mechanism does not repeat. In the first dense pair TreeDB had lower app DB growth and lower process CPU; in the repeat it had higher app DB growth and higher process CPU.
- The stable signal is that TreeDB has much higher commit cost in this Ironbird/simapp setting, while dense blocks can make total throughput close enough that TreeDB slightly wins anyway.
- This is a valid synthetic DB-sensitive lane, but it is not yet a strong TreeDB-performance demonstration.

## Larger-State Proxy Pair

The larger-state proxy increases genesis wallet count to 50k while returning to the 3-block, 150-outer-tx storage-fanout shape. This is still not production sync or real transaction replay. It is a cheap local attempt to make the app state less toy-shaped.

Command shape:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-goleveldb -skip-build \
  -validators 1 -nodes 0 -wallets 50000 \
  -cosmos-blocks 3 -cosmos-txs 50 \
  -cosmos-msg MsgArr -cosmos-contained-msg MsgMultiSend \
  -cosmos-msgs-per-tx 20 -cosmos-recipients 25 \
  -cosmos-max-gas 300000000 \
  -out reports/artifacts/db-signal/simapp-goleveldb-largegenesis-50k-msgarr-multisend-metrics.json
```

The TreeDB command is identical except `-scenario simapp-treedb` and the output path.

| Backend | Raw successful tx | Effective recipient ops | Runtime effective ops/s | Wall effective ops/s | Commit seconds | Finalize seconds | Commit share of ABCI | Commit share of commit+finalize | Avg commit | App DB delta | Validator write max | Validator mem max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB 50k | 150 | 75,000 | 1,736.87 | 331.59 | 1.7438s | 2.0428s | 41.1% | 46.1% | 27.25ms | 30,614,439 B | 184 MB | 1.185 GiB |
| TreeDB 50k | 150 | 75,000 | 1,689.37 | 311.19 | 8.3356s | 2.1262s | 71.4% | 79.7% | 120.81ms | 31,799,860 B | 210 MB | 1.454 GiB |

TreeDB versus LevelDB in the 50k proxy:

| Metric | Delta |
| --- | ---: |
| Runtime effective ops/s | -2.73% |
| Wall effective ops/s | -6.15% |
| App DB byte delta | +3.87% |
| Process CPU seconds delta | +16.77% |
| Validator memory max | +22.70% |
| Commit seconds | +378.02% |
| Finalize seconds | +4.08% |

Interpretation:

- The cheap larger-state proxy does not reproduce the production-sync TreeDB lift.
- It increases commit share substantially for both backends, so it is more storage-visible than the first high-gas pair.
- TreeDB remains dominated by much higher commit time in this simapp/Ironbird path, and the larger state shape slightly hurts total throughput rather than helping it.
- The result suggests that merely increasing account/genesis size is not enough; production alignment likely needs a real exported-state/replay lane or at least module/key-distribution realism.

## Commit-Only Microbenchmark

The local runner now has a commit-only mode:

```sh
GOWORK=off TMPDIR=/mnt/fast4tb/tmp go run ./cmd/local-report-runner \
  -scenario simapp-goleveldb -skip-build \
  -validators 1 -nodes 0 -wallets 100 \
  -commit-benchmark-blocks 20 \
  -out reports/artifacts/db-signal/commitbench-goleveldb-20blocks.json
```

The TreeDB command is identical except `-scenario simapp-treedb` and the output path.

This mode launches the chain, captures validator Prometheus metrics after startup, waits for 20 additional blocks with no Catalyst load, then captures metrics again.

| Backend | Measured blocks | Commit seconds | Commit count | Avg commit | Finalize seconds | Process CPU delta | App DB delta | Validator mem max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| LevelDB | 20 | 0.1579s | 20 | 7.90ms | 0.0042s | 0.98s | 164,078 B | 75.73 MiB |
| TreeDB | 20 | 2.0681s | 20 | 103.40ms | 0.0036s | 1.43s | 724,966 B | 319.9 MiB |

Interpretation:

- This isolates and reproduces the fixed per-block commit-cost difference without transaction load.
- TreeDB commit time is about 13.1x LevelDB in this microbenchmark.
- Finalize time is effectively negligible for both backends, so the signal is specifically ABCI `Commit`.
- This supports the hypothesis that short Ironbird/simapp runs are heavily affected by fixed per-block TreeDB work.

## Current Learning

Ironbird can now tell us that a workload is not the same as production sync. This is useful even though it is not yet the desired TreeDB win.

The current SDK lane is not wrong, but it appears to be in a mixed regime:

- It reaches thousands of effective recipient operations/sec, with the dense pair reaching about 4.7k-4.9k runtime effective ops/sec.
- It drives measurable app DB and block-write deltas.
- It still spends substantial time outside commit, especially in `finalize_block`.
- TreeDB's per-block commit overhead is large enough that empty/light blocks can dominate short local runs.
- Denser loaded blocks can overcome some of that fixed commit overhead and produce a repeatable but small positive TreeDB throughput delta.
- Larger toy-shaped account state does not by itself move the workload into the same regime as production sync.
- The commit-only microbenchmark reproduces the fixed cost directly: about 7.90ms/block for LevelDB versus 103.40ms/block for TreeDB in the 20-block run.

That explains why prior uninstrumented results could look like a tie: they were measuring a blend of Catalyst runtime, block pacing, tx execution, fixed commit overhead, and storage writes.

## Next Probes

Run these on a quiet host, or repeat immediately only after confirming no competing benchmark/test process is active.

1. True production-shaped lane:

- Seed from a real exported app state or a sanitized production-like genesis.
- Start both backends from the same state shape.
- Replay a bounded recent transaction window if the app and tx format are available.
- If replay is not available, run synthetic txs that match production module/key touch distribution.

This is the lane most likely to connect Ironbird back to the known production sync win, but it needs real state import/replay work rather than more local parameter tuning.

2. TreeDB commit-cost isolation:

- Run an empty/light-block commit micro-lane through Ironbird with the same metric capture.
- Compare TreeDB commit time by block count, not by tx count.
- Use that to decide whether the Ironbird simapp path is paying fixed commit overhead that production sync amortizes differently.

3. Dense-block scale-up:

- Repeat the 1-block dense lane with more contained writes if gas/runtime permits.
- Keep the objective narrow: see whether throughput delta grows with write density after fixed commit overhead is amortized.
- Stop if wall time grows but runtime effective ops/s does not separate.

## Current Conclusion

Partial conclusion: the new DB-signal instrumentation works, the current high-gas SDK workload exposes a large TreeDB per-block commit cost, the denser synthetic `MsgArr(MsgMultiSend)` lane shows a repeatable but small positive TreeDB throughput delta, and the cheap 50k-wallet larger-state proxy does not improve the TreeDB story.

The sprint outcome is useful but not the desired proof. Ironbird can now express DB-sensitive workloads, but the current local simapp lanes mostly reveal fixed TreeDB commit overhead and mixed execution/commit regimes. To connect Ironbird back to the known production sync win, the next useful investment is a real exported-state/replay lane or a much more faithful synthetic module/key distribution, not more small parameter churn on toy simapp traffic.
