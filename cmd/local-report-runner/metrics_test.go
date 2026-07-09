package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ctlt "github.com/skip-mev/catalyst/chains/types"
)

func nearlyEqual(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

func TestParsePrometheusMetricsKeepsSelectedSeries(t *testing.T) {
	text := `
	# HELP cometbft_abci_connection_method_timing_seconds Timing for each ABCI method.
cometbft_abci_connection_method_timing_seconds_sum{chain_id="sgldb",method="commit",type="sync"} 1.25
cometbft_abci_connection_method_timing_seconds_count{chain_id="sgldb",method="commit",type="sync"} 5
cometbft_consensus_height{chain_id="sgldb"} 10
treedb_vlog_write_seconds_sum 2.5
go_gc_duration_seconds_sum 0.125
process_resident_memory_bytes 1234
blockstore_save_block_seconds_sum 3
tx_index_indexed_txs_total 4
sdk_store_commit_seconds_sum 5
app_custom_metric_total 6
some_unrelated_metric 99
tx_count 7
`
	metrics := parsePrometheusMetrics(text)
	if got := metrics[`cometbft_abci_connection_method_timing_seconds_sum{chain_id="sgldb",method="commit",type="sync"}`]; got != 1.25 {
		t.Fatalf("commit sum = %v, want 1.25", got)
	}
	if got := metrics["tx_count"]; got != 7 {
		t.Fatalf("tx_count = %v, want 7", got)
	}
	for name, want := range map[string]float64{
		`cometbft_consensus_height{chain_id="sgldb"}`: 10,
		"treedb_vlog_write_seconds_sum":               2.5,
		"go_gc_duration_seconds_sum":                  0.125,
		"process_resident_memory_bytes":               1234,
		"blockstore_save_block_seconds_sum":           3,
		"tx_index_indexed_txs_total":                  4,
		"sdk_store_commit_seconds_sum":                5,
		"app_custom_metric_total":                     6,
	} {
		if got := metrics[name]; got != want {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
	if _, ok := metrics["some_unrelated_metric"]; ok {
		t.Fatalf("unrelated metric was retained")
	}
}

func TestParsePrometheusMetricsDropsNonFiniteSeries(t *testing.T) {
	text := `
go_gc_duration_seconds_sum NaN
go_memstats_heap_alloc_bytes +Inf
go_threads 8
`
	metrics := parsePrometheusMetrics(text)
	if _, ok := metrics["go_gc_duration_seconds_sum"]; ok {
		t.Fatalf("NaN metric was retained")
	}
	if _, ok := metrics["go_memstats_heap_alloc_bytes"]; ok {
		t.Fatalf("Inf metric was retained")
	}
	if got := metrics["go_threads"]; got != 8 {
		t.Fatalf("go_threads = %v, want 8", got)
	}
	payload, err := json.Marshal(loadWindowObservation{MetricsBefore: []metricSnapshot{{Name: "validator-0", Metrics: metrics}}})
	if err != nil {
		t.Fatalf("marshal metrics with finite-only values: %v", err)
	}
	if !strings.Contains(string(payload), `"go_threads":8`) {
		t.Fatalf("payload missing finite metric: %s", payload)
	}
}

func TestMakePreseedConfigAccounts(t *testing.T) {
	cfg := makePreseedConfig("accounts", 100, 5000)
	if cfg.Profile != "accounts" {
		t.Fatalf("profile = %q, want accounts", cfg.Profile)
	}
	if cfg.Accounts != 100 {
		t.Fatalf("accounts = %d, want 100", cfg.Accounts)
	}
	if cfg.ActiveWallets != 5000 {
		t.Fatalf("active wallets = %d, want 5000", cfg.ActiveWallets)
	}
	if cfg.GenesisAccounts != 5100 {
		t.Fatalf("genesis accounts = %d, want 5100", cfg.GenesisAccounts)
	}
}

func TestLaunchGenesisAccountsFallsBackToWorkloadWallets(t *testing.T) {
	sc := scenario{NumWallets: 5000}
	if got := launchGenesisAccounts(sc); got != 5000 {
		t.Fatalf("launch accounts = %d, want 5000", got)
	}
}

func TestLaunchGenesisAccountsUsesPreseedTotal(t *testing.T) {
	sc := scenario{
		NumWallets: 5000,
		Preseed:    preseedConfig{GenesisAccounts: 15000},
	}
	if got := launchGenesisAccounts(sc); got != 15000 {
		t.Fatalf("launch accounts = %d, want 15000", got)
	}
}

func TestSimappFullStackScenarioSetsAppAndNodeBackends(t *testing.T) {
	sc := simappFullStackScenario("simapp-treedb-all", "full stack TreeDB", "treedb", 1, 0, 100, preseedConfig{}, 2, 10, "MsgSend", "", 0, 0, 1000000, "")
	if sc.AppDBBackend != "treedb" {
		t.Fatalf("app backend = %q, want treedb", sc.AppDBBackend)
	}
	if sc.NodeDBBackend != "treedb" {
		t.Fatalf("node backend = %q, want treedb", sc.NodeDBBackend)
	}
	if got := sc.CustomAppConfig["app-db-backend"]; got != "treedb" {
		t.Fatalf("app config backend = %v, want treedb", got)
	}
	if got := sc.CustomConfig["db_backend"]; got != "treedb" {
		t.Fatalf("node config backend = %v, want treedb", got)
	}
	if !strings.Contains(sc.ReplaceCmd, "github.com/cometbft/cometbft-db=github.com/snissn/cometbft-db@"+simappCometDBVersion) {
		t.Fatalf("replace command missing cometbft-db TreeDB fork: %s", sc.ReplaceCmd)
	}
	if strings.Contains(sc.ReplaceCmd, "go mod edit") || strings.Contains(sc.ReplaceCmd, "&&") {
		t.Fatalf("replace specs must not contain shell commands: %s", sc.ReplaceCmd)
	}
	if !strings.Contains(sc.ImageTag, "fullstack") {
		t.Fatalf("image tag = %q, want fullstack marker", sc.ImageTag)
	}
}

func TestSimappAppOnlyScenarioDoesNotSetNodeBackend(t *testing.T) {
	sc := simappScenario("simapp-treedb", "app TreeDB", "treedb", 1, 0, 100, preseedConfig{}, 2, 10, "MsgSend", "", 0, 0, 1000000, "")
	if sc.NodeDBBackend != "" {
		t.Fatalf("node backend = %q, want empty for app-only scenario", sc.NodeDBBackend)
	}
	if _, ok := sc.CustomConfig["db_backend"]; ok {
		t.Fatalf("app-only scenario unexpectedly sets db_backend")
	}
	if strings.Contains(sc.ReplaceCmd, "github.com/cometbft/cometbft-db") {
		t.Fatalf("app-only replace command unexpectedly includes cometbft-db: %s", sc.ReplaceCmd)
	}
}

func TestValidateBackendVerification(t *testing.T) {
	valid, problem := validateBackendVerification(backendVerification{
		ExpectedAppDBBackend:  "treedb",
		ExpectedNodeDBBackend: "treedb",
		ObservedAppDBBackend:  "treedb",
		ObservedNodeDBBackend: "treedb",
	})
	if !valid || problem != "" {
		t.Fatalf("valid verification = %t problem=%q, want valid", valid, problem)
	}
	valid, problem = validateBackendVerification(backendVerification{
		ExpectedAppDBBackend:  "treedb",
		ExpectedNodeDBBackend: "treedb",
		ObservedAppDBBackend:  "treedb",
		ObservedNodeDBBackend: "goleveldb",
	})
	if valid {
		t.Fatalf("verification unexpectedly valid")
	}
	if !strings.Contains(problem, `db_backend observed "goleveldb", want "treedb"`) {
		t.Fatalf("problem = %q, want node backend mismatch", problem)
	}
}

func TestSummarizeStorageSignalsComputesCommitShareAndSizeDelta(t *testing.T) {
	before := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:           1,
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`:         2,
			`cometbft_abci_connection_method_timing_seconds_sum{method="finalize_block"}`:   3,
			`cometbft_abci_connection_method_timing_seconds_count{method="finalize_block"}`: 2,
			`cometbft_abci_connection_method_timing_seconds_sum{method="query"}`:            0.5,
			`begin_blocker_sum{module="bank"}`:                                              0.25,
			`begin_blocker_count{module="bank"}`:                                            2,
			"tx_count":                                                                      10,
		},
	}}
	after := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:           3,
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`:         4,
			`cometbft_abci_connection_method_timing_seconds_sum{method="finalize_block"}`:   4,
			`cometbft_abci_connection_method_timing_seconds_count{method="finalize_block"}`: 4,
			`cometbft_abci_connection_method_timing_seconds_sum{method="query"}`:            0.75,
			`begin_blocker_sum{module="bank"}`:                                              0.75,
			`begin_blocker_count{module="bank"}`:                                            4,
			"tx_count":                                                                      15,
		},
	}}
	beforeSizes := []dataPathSize{{Name: "validator-0", Path: "/simd/data", Bytes: 100}}
	afterSizes := []dataPathSize{{Name: "validator-0", Path: "/simd/data", Bytes: 175}}

	signals := summarizeStorageSignals(before, after, beforeSizes, afterSizes)
	if len(signals) != 1 {
		t.Fatalf("signals len = %d, want 1", len(signals))
	}
	signal := signals[0]
	if signal.ABCICommitSeconds != 2 {
		t.Fatalf("commit seconds = %v, want 2", signal.ABCICommitSeconds)
	}
	if signal.ABCIFinalizeBlockSeconds != 1 {
		t.Fatalf("finalize seconds = %v, want 1", signal.ABCIFinalizeBlockSeconds)
	}
	if signal.CommitShareOfCommitPlusFinalize != 2.0/3.0 {
		t.Fatalf("commit/finalize share = %v, want %v", signal.CommitShareOfCommitPlusFinalize, 2.0/3.0)
	}
	if signal.SDKTxCountDelta != 5 {
		t.Fatalf("tx delta = %v, want 5", signal.SDKTxCountDelta)
	}
	if signal.DataDirBytesDelta != 75 {
		t.Fatalf("data delta = %v, want 75", signal.DataDirBytesDelta)
	}
	if signal.ABCIQuerySeconds != 0.25 {
		t.Fatalf("query seconds = %v, want 0.25", signal.ABCIQuerySeconds)
	}
	if len(signal.ModuleTimings) != 1 {
		t.Fatalf("module timings len = %d, want 1", len(signal.ModuleTimings))
	}
	if timing := signal.ModuleTimings[0]; timing.Phase != "begin_blocker" || timing.Module != "bank" || timing.Seconds != 0.5 || timing.Count != 2 || timing.AvgSeconds != 0.25 {
		t.Fatalf("module timing = %+v, want begin_blocker bank 0.5s count 2 avg 0.25", timing)
	}
}

func TestSummarizePipelineSignalsPromotesAcceptedWindowCounters(t *testing.T) {
	obs := loadWindowObservation{
		Seconds:                120,
		IncludedTransactions:   60000,
		SuccessfulTransactions: 59000,
		StorageSignals: []storageSignal{{
			Name:                          "validator-0",
			ABCICheckTxSeconds:            6,
			ABCICheckTxCount:              60000,
			ABCIFinalizeBlockSeconds:      18,
			ABCIFinalizeBlockCount:        60,
			ABCICommitSeconds:             3,
			ABCICommitCount:               60,
			ConsensusBlockIntervalSeconds: 120,
			ConsensusBlockIntervalCount:   60,
			ConsensusTotalTxsDelta:        60000,
			MempoolSuccessfulTxsDelta:     59000,
			SDKTxCountDelta:               60000,
			SDKTxSuccessfulDelta:          59000,
		}},
	}
	logs := loadTestLogSummary{
		SendingTxsTotal:        62000,
		FailedSendTotal:        2,
		CollectorStartingBlock: 100,
		CollectorEndingBlock:   159,
	}
	rows := summarizePipelineSignals(obs, logs)
	if len(rows) != 1 {
		t.Fatalf("pipeline rows len=%d want 1", len(rows))
	}
	row := rows[0]
	if row.SubmittedTransactions != 62000 || row.SubmittedMinusIncluded != 2000 {
		t.Fatalf("submitted/gap=%d/%d want 62000/2000", row.SubmittedTransactions, row.SubmittedMinusIncluded)
	}
	if row.CollectorBlockSpan != 60 {
		t.Fatalf("collector block span=%d want 60", row.CollectorBlockSpan)
	}
	if row.AvgBlockIntervalSeconds != 2 {
		t.Fatalf("avg block interval=%v want 2", row.AvgBlockIntervalSeconds)
	}
	if row.AvgTxsPerCommit != 1000 || row.AvgTxsPerConsensusBlock != 1000 {
		t.Fatalf("tx/block commit=%v consensus=%v want 1000/1000", row.AvgTxsPerCommit, row.AvgTxsPerConsensusBlock)
	}
	if row.AvgCheckTxSeconds != 0.0001 || row.AvgFinalizeBlockSeconds != 0.3 || row.AvgCommitSeconds != 0.05 {
		t.Fatalf("avg ABCI timings=%v/%v/%v want 0.0001/0.3/0.05", row.AvgCheckTxSeconds, row.AvgFinalizeBlockSeconds, row.AvgCommitSeconds)
	}
	if len(row.Notes) == 0 {
		t.Fatalf("expected send-gap note")
	}
}

func TestSummarizeLoadTestLogsParsesCatalystBlockTiming(t *testing.T) {
	logs := strings.Join([]string{
		"[truncated task logs; showing tail]",
		`2026-07-08T21:45:28.650Z	INFO	completed sending transactions for block	{"block_number": 322, "txs_sent": 500, "expected_txs": 500}`,
		`2026-07-08T21:45:28.732Z	DEBUG	received new block event	{"height": 711}`,
		`2026-07-08T21:45:28.752Z	DEBUG	processing block	{"height": 711, "timestamp": "2026-07-08T21:45:27.841Z", "gas_limit": 75000000}`,
		`2026-07-08T21:45:28.752Z	INFO	starting to send transactions for block	{"block_number": 323, "expected_txs": 500}`,
		`2026-07-08T21:45:28.996Z	INFO	completed sending transactions for block	{"block_number": 323, "txs_sent": 500, "expected_txs": 500}`,
		`2026-07-08T21:45:29.002Z	INFO	processed block	{"height": 711}`,
		`2026-07-08T21:45:29.145Z	DEBUG	received new block event	{"height": 712}`,
		`2026-07-08T21:45:29.145Z	DEBUG	processing block	{"height": 712, "timestamp": "2026-07-08T21:45:28.287Z", "gas_limit": 75000000}`,
		`2026-07-08T21:45:29.145Z	INFO	starting to send transactions for block	{"block_number": 324, "expected_txs": 500}`,
		`2026-07-08T21:45:29.662Z	INFO	completed sending transactions for block	{"block_number": 324, "txs_sent": 500, "expected_txs": 500}`,
		`2026-07-08T21:45:29.662Z	INFO	processed block	{"height": 712}`,
	}, "\n")

	summary := summarizeLoadTestLogs(logs)
	timing := summary.CatalystTiming
	if timing == nil {
		t.Fatalf("expected Catalyst timing summary")
	}
	if !timing.LogTruncated {
		t.Fatalf("expected truncated log marker")
	}
	if timing.SendStartEvents != 2 || timing.SendCompleteEvents != 3 || timing.SendTxsSentTotal != 1500 || timing.SendMatchedTxsTotal != 1000 || timing.SendExpectedTxsTotal != 1000 {
		t.Fatalf("send counts = start %d complete %d sent %d matched %d expected %d, want 2/3/1500/1000/1000", timing.SendStartEvents, timing.SendCompleteEvents, timing.SendTxsSentTotal, timing.SendMatchedTxsTotal, timing.SendExpectedTxsTotal)
	}
	if timing.SendDurations == nil || timing.SendDurations.Count != 2 {
		t.Fatalf("send durations = %+v, want 2 entries", timing.SendDurations)
	}
	if !nearlyEqual(timing.SendDurations.TotalSeconds, 0.761) {
		t.Fatalf("send total = %v, want 0.761", timing.SendDurations.TotalSeconds)
	}
	if !nearlyEqual(timing.SendDurations.MaxSeconds, 0.517) {
		t.Fatalf("send max = %v, want 0.517", timing.SendDurations.MaxSeconds)
	}
	if !nearlyEqual(timing.SendTxsPerSecond, 1000/0.761) {
		t.Fatalf("send tx/s = %v, want %v", timing.SendTxsPerSecond, 1000/0.761)
	}
	if !strings.Contains(strings.Join(timing.Notes, "\n"), "completion events did not have retained start events") {
		t.Fatalf("expected unmatched completion note, got %v", timing.Notes)
	}
	if timing.BlockProcessingToProcessedDurations == nil || timing.BlockProcessingToProcessedDurations.Count != 2 {
		t.Fatalf("process durations = %+v, want 2 entries", timing.BlockProcessingToProcessedDurations)
	}
	if !nearlyEqual(timing.BlockProcessingToProcessedDurations.TotalSeconds, 0.767) {
		t.Fatalf("process total = %v, want 0.767", timing.BlockProcessingToProcessedDurations.TotalSeconds)
	}
	if timing.BlockEventToProcessingDurations == nil || !nearlyEqual(timing.BlockEventToProcessingDurations.MaxSeconds, 0.020) {
		t.Fatalf("event->processing = %+v, want max 0.020", timing.BlockEventToProcessingDurations)
	}
	if timing.ChainTimestampToProcessingDurations == nil || timing.ChainTimestampToProcessingDurations.Count != 2 {
		t.Fatalf("chain timestamp lags = %+v, want 2 entries", timing.ChainTimestampToProcessingDurations)
	}
}

func TestSummarizePipelineSignalsPromotesCatalystBlockTiming(t *testing.T) {
	obs := loadWindowObservation{
		Seconds:                10,
		IncludedTransactions:   1000,
		SuccessfulTransactions: 1000,
		StorageSignals: []storageSignal{{
			Name: "validator-0",
		}},
	}
	logs := loadTestLogSummary{
		CatalystTiming: &catalystLogTiming{
			SendTxsSentTotal:    1500,
			SendMatchedTxsTotal: 1000,
			SendTxsPerSecond:    1250,
			SendDurations: &durationStats{
				Count:        2,
				TotalSeconds: 0.8,
				AvgSeconds:   0.4,
				MaxSeconds:   0.5,
			},
			BlockProcessingToProcessedDurations: &durationStats{
				Count:      2,
				AvgSeconds: 0.45,
				MaxSeconds: 0.6,
			},
		},
	}

	rows := summarizePipelineSignals(obs, logs)
	if len(rows) != 1 {
		t.Fatalf("pipeline rows len=%d want 1", len(rows))
	}
	row := rows[0]
	if row.CatalystSendBlocks != 2 || row.CatalystSendTxs != 1000 || row.CatalystSendSeconds != 0.8 || row.AvgCatalystSendSeconds != 0.4 || row.MaxCatalystSendSeconds != 0.5 || row.CatalystSendTxsPerSecond != 1250 {
		t.Fatalf("send timing row = %+v", row)
	}
	if row.CatalystBlockProcessBlocks != 2 || row.AvgCatalystBlockProcessSeconds != 0.45 || row.MaxCatalystBlockProcessSeconds != 0.6 {
		t.Fatalf("block process timing row = %+v", row)
	}
}

func TestSummarizeConsensusStepTimings(t *testing.T) {
	deltas := map[string]float64{
		`cometbft_consensus_step_duration_seconds_sum{chain_id="chain",step="Propose"}`:   1.5,
		`cometbft_consensus_step_duration_seconds_count{chain_id="chain",step="Propose"}`: 3,
		`cometbft_consensus_step_duration_seconds_sum{chain_id="chain",step="Prevote"}`:   0.75,
		`cometbft_consensus_step_duration_seconds_count{chain_id="chain",step="Prevote"}`: 3,
	}
	rows := summarizeConsensusStepTimings(deltas)
	if len(rows) != 2 {
		t.Fatalf("rows len=%d want 2: %+v", len(rows), rows)
	}
	if rows[0].Name != "Propose" || rows[0].Class != "consensus_step" || rows[0].Seconds != 1.5 || rows[0].Count != 3 || rows[0].AvgSeconds != 0.5 {
		t.Fatalf("propose row = %+v, want 1.5s count 3 avg 0.5", rows[0])
	}
	if rows[1].Name != "Prevote" || rows[1].Seconds != 0.75 || rows[1].AvgSeconds != 0.25 {
		t.Fatalf("prevote row = %+v, want 0.75s avg 0.25", rows[1])
	}
}

func TestSummarizeCadenceDiagnosticsSeparatesBlockStagesFromResidual(t *testing.T) {
	obs := loadWindowObservation{
		Seconds:                10,
		IncludedTransactions:   1000,
		SuccessfulTransactions: 1000,
		StorageSignals: []storageSignal{{
			Name:                          "validator-0",
			ABCICommitSeconds:             1,
			ABCICommitCount:               10,
			ABCIFinalizeBlockSeconds:      2,
			ABCIFinalizeBlockCount:        10,
			ABCIPrepareProposalSeconds:    0.5,
			ABCIPrepareProposalCount:      10,
			ABCIProcessProposalSeconds:    0.25,
			ABCIProcessProposalCount:      10,
			ABCICheckTxSeconds:            3,
			ABCICheckTxCount:              1000,
			ConsensusBlockIntervalSeconds: 10,
			ConsensusBlockIntervalCount:   10,
			ConsensusTotalTxsDelta:        1000,
			ConsensusStepTimings: []cadenceStage{{
				Name:       "Propose",
				Class:      "consensus_step",
				Source:     "cometbft_consensus_step_duration_seconds",
				Provenance: "prometheus_counter_delta",
				Seconds:    0.8,
				Count:      10,
				AvgSeconds: 0.08,
			}},
		}},
		PipelineSignals: []pipelineSignal{{
			Name:                          "validator-0",
			AvgTxsPerConsensusBlock:       100,
			AvgBlockIntervalSeconds:       1,
			ConsensusBlockIntervalCount:   10,
			ConsensusBlockIntervalSeconds: 10,
		}},
	}

	rows := summarizeCadenceDiagnostics(obs)
	if len(rows) != 1 {
		t.Fatalf("cadence rows len=%d want 1", len(rows))
	}
	row := rows[0]
	if row.BlockCount != 10 || row.AvgBlockIntervalSeconds != 1 || row.AvgTxsPerBlock != 100 {
		t.Fatalf("block shape = count %d interval %v tx/block %v, want 10/1/100", row.BlockCount, row.AvgBlockIntervalSeconds, row.AvgTxsPerBlock)
	}
	if !nearlyEqual(row.ABCIBlockStageSecondsPerBlock, 0.375) {
		t.Fatalf("block-stage seconds/block = %v, want 0.375", row.ABCIBlockStageSecondsPerBlock)
	}
	if !nearlyEqual(row.CheckTxSecondsPerBlockEquivalent, 0.3) {
		t.Fatalf("checktx seconds/block = %v, want 0.3", row.CheckTxSecondsPerBlockEquivalent)
	}
	if !nearlyEqual(row.ConsensusStepSecondsPerBlock, 0.08) {
		t.Fatalf("consensus step seconds/block = %v, want 0.08", row.ConsensusStepSecondsPerBlock)
	}
	if !nearlyEqual(row.CadenceResidualAfterABCIBlockStagesSeconds, 0.625) {
		t.Fatalf("residual seconds/block = %v, want 0.625", row.CadenceResidualAfterABCIBlockStagesSeconds)
	}
	if !nearlyEqual(row.CadenceResidualPctOfBlockInterval, 62.5) {
		t.Fatalf("residual percent = %v, want 62.5", row.CadenceResidualPctOfBlockInterval)
	}
	if row.ExactEventSpanCoverage {
		t.Fatalf("expected exact event coverage to be false until event-level spans exist")
	}
	if len(row.MissingEventSpans) == 0 {
		t.Fatalf("expected missing event span owners to be named")
	}
	for _, stage := range row.Stages {
		if stage.Name == "check_tx" && stage.IncludedInResidual {
			t.Fatalf("check_tx must not be included in block-stage residual: %+v", stage)
		}
	}
}

func TestSummarizeLoadWindowAccountingMarksApproximateResidual(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	result := runResult{
		PhaseTimeline: []phaseSpan{{
			Name:    "run_load_test",
			Started: base.Add(-time.Second),
			Ended:   base.Add(11 * time.Second),
			Seconds: 12,
		}},
	}
	obs := loadWindowObservation{
		StartedAt: base,
		EndedAt:   base.Add(10 * time.Second),
		Seconds:   10,
		StorageSignals: []storageSignal{{
			Name:                          "validator-0",
			ABCIObservedSeconds:           3,
			ABCICommitSeconds:             1,
			ABCIFinalizeBlockSeconds:      1.25,
			ABCICheckTxSeconds:            0.5,
			ProcessCPUSecondsDelta:        25,
			ConsensusBlockIntervalSeconds: 20,
			ConsensusBlockIntervalCount:   10,
		}},
		PipelineSignals: []pipelineSignal{{
			Name:                   "validator-0",
			SubmittedTransactions:  1050,
			IncludedTransactions:   1000,
			SuccessfulTransactions: 1000,
			SubmittedMinusIncluded: 50,
			FailedSendTotal:        2,
		}},
	}

	rows := summarizeLoadWindowAccounting(result, obs)
	if len(rows) != 1 {
		t.Fatalf("accounting rows len=%d want 1", len(rows))
	}
	row := rows[0]
	if row.ValidatorNonABCIApproxSeconds != 7 {
		t.Fatalf("approx non-ABCI = %v, want 7", row.ValidatorNonABCIApproxSeconds)
	}
	if row.ValidatorCoreEquivalent != 2.5 {
		t.Fatalf("core equivalent = %v, want 2.5", row.ValidatorCoreEquivalent)
	}
	if row.ConsensusBlockCadenceSeconds != 2 {
		t.Fatalf("block cadence = %v, want 2", row.ConsensusBlockCadenceSeconds)
	}
	if row.ABCIBusyUnionSeconds != nil || row.ABCIBusyUnionMissingReason == "" {
		t.Fatalf("busy union missing state = %v reason=%q", row.ABCIBusyUnionSeconds, row.ABCIBusyUnionMissingReason)
	}
	payload, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal accounting: %v", err)
	}
	for _, want := range []string{
		`"abci_busy_union_seconds":null`,
		`"validator_non_abci_wall_seconds":null`,
		`"loadgen_client_wait_seconds":null`,
		`"unaccounted_residual_formula":"max(0, load_window_seconds - abci_observed_sum_seconds)"`,
		`"mempool_backlog_summary":"submitted=1050 included=1000 successful=1000 send_gap=50 failed_send=2"`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload missing %s: %s", want, payload)
		}
	}
}

func TestLoadWindowIntervalUnionClipsAndMerges(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	windowStart := base.Add(10 * time.Second)
	windowEnd := base.Add(30 * time.Second)
	intervals := []loadWindowInterval{
		{
			Name:       "validator-0",
			Class:      "abci",
			Method:     "check_tx",
			Provenance: "exact_event",
			Started:    base.Add(8 * time.Second),
			Ended:      base.Add(16 * time.Second),
		},
		{
			Name:       "validator-0",
			Class:      "abci",
			Method:     "finalize_block",
			Provenance: "exact_event",
			Started:    base.Add(14 * time.Second),
			Ended:      base.Add(22 * time.Second),
		},
		{
			Name:       "validator-0",
			Class:      "abci",
			Method:     "commit",
			Provenance: "exact_event",
			Started:    base.Add(28 * time.Second),
			Ended:      base.Add(35 * time.Second),
		},
		{
			Name:       "validator-1",
			Class:      "abci",
			Method:     "commit",
			Provenance: "exact_event",
			Started:    base.Add(10 * time.Second),
			Ended:      base.Add(30 * time.Second),
		},
		{
			Name:       "validator-0",
			Class:      "loadgen",
			Provenance: "exact_event",
			Started:    base.Add(10 * time.Second),
			Ended:      base.Add(30 * time.Second),
		},
		{
			Class:      "abci",
			Method:     "query",
			Provenance: "exact_event",
			Started:    base.Add(10 * time.Second),
			Ended:      base.Add(30 * time.Second),
		},
	}

	unionSeconds, count, provenance := loadWindowIntervalClassUnion(intervals, "validator-0", "abci", windowStart, windowEnd)
	if unionSeconds != 14 {
		t.Fatalf("union seconds = %v, want 14", unionSeconds)
	}
	if count != 3 {
		t.Fatalf("interval count = %d, want 3", count)
	}
	if provenance != "exact_event" {
		t.Fatalf("provenance = %q, want exact_event", provenance)
	}

	attribution := summarizeLoadWindowIntervalAttributions(intervals, "validator-0", windowStart, windowEnd)
	if len(attribution) != 4 {
		t.Fatalf("attribution rows = %d, want 4: %+v", len(attribution), attribution)
	}

	aggregateUnionSeconds, aggregateCount, _ := loadWindowIntervalClassUnion(intervals, "", "abci", windowStart, windowEnd)
	if aggregateUnionSeconds != 20 || aggregateCount != 5 {
		t.Fatalf("aggregate union/count = %v/%d, want 20/5", aggregateUnionSeconds, aggregateCount)
	}
}

func TestSummarizeLoadWindowAccountingUsesABCIIntervals(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	result := runResult{
		PhaseTimeline: []phaseSpan{{
			Name:    "run_load_test",
			Started: base,
			Ended:   base.Add(10 * time.Second),
			Seconds: 10,
		}},
	}
	obs := loadWindowObservation{
		StartedAt: base,
		EndedAt:   base.Add(10 * time.Second),
		Seconds:   10,
		StorageSignals: []storageSignal{{
			Name:                     "validator-0",
			ABCIObservedSeconds:      8,
			ABCICommitSeconds:        2,
			ABCIFinalizeBlockSeconds: 4,
			ABCICheckTxSeconds:       2,
			ProcessCPUSecondsDelta:   15,
		}},
		WallClockIntervals: []loadWindowInterval{
			{
				Name:       "validator-0",
				Class:      "abci",
				Method:     "check_tx",
				Provenance: "exact_event",
				Started:    base,
				Ended:      base.Add(2 * time.Second),
			},
			{
				Name:       "validator-0",
				Class:      "abci",
				Method:     "finalize_block",
				Provenance: "exact_event",
				Started:    base.Add(1 * time.Second),
				Ended:      base.Add(4 * time.Second),
			},
			{
				Name:       "validator-0",
				Class:      "abci",
				Method:     "commit",
				Provenance: "exact_event",
				Started:    base.Add(6 * time.Second),
				Ended:      base.Add(8 * time.Second),
			},
		},
	}

	rows := summarizeLoadWindowAccounting(result, obs)
	if len(rows) != 1 {
		t.Fatalf("accounting rows len=%d want 1", len(rows))
	}
	row := rows[0]
	if row.ABCIBusyUnionSeconds == nil || *row.ABCIBusyUnionSeconds != 6 {
		t.Fatalf("ABCI busy union = %v, want 6", row.ABCIBusyUnionSeconds)
	}
	if row.ABCIOverlapSeconds == nil || *row.ABCIOverlapSeconds != 2 {
		t.Fatalf("ABCI overlap = %v, want 2", row.ABCIOverlapSeconds)
	}
	if row.ValidatorNonABCIWallSeconds == nil || *row.ValidatorNonABCIWallSeconds != 4 {
		t.Fatalf("exact non-ABCI wall = %v, want 4", row.ValidatorNonABCIWallSeconds)
	}
	if row.ValidatorNonABCIApproxSeconds != 2 {
		t.Fatalf("approx non-ABCI = %v, want 2", row.ValidatorNonABCIApproxSeconds)
	}
	if row.ABCIBusyUnionMissingReason != "" || row.ValidatorNonABCIWallMissingReason != "" {
		t.Fatalf("unexpected missing reasons: busy=%q wall=%q", row.ABCIBusyUnionMissingReason, row.ValidatorNonABCIWallMissingReason)
	}
	if row.UnaccountedResidualFormula != "max(0, load_window_seconds - abci_busy_union_seconds)" || row.UnaccountedResidualClassification != "interval_union_based" {
		t.Fatalf("residual formula/classification = %q/%q", row.UnaccountedResidualFormula, row.UnaccountedResidualClassification)
	}
	payload, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal accounting: %v", err)
	}
	for _, want := range []string{
		`"abci_busy_union_seconds":6`,
		`"abci_overlap_seconds":2`,
		`"validator_non_abci_wall_seconds":4`,
		`"abci_interval_count":3`,
		`"abci_interval_provenance":"exact_event"`,
		`"interval_attribution"`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload missing %s: %s", want, payload)
		}
	}
}

func TestAppendMetricDeltaWallClockIntervalsFromABCICounters(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	before := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`:       10,
			`cometbft_abci_connection_method_timing_seconds_sum{method="check_tx"}`:         0.10,
			`cometbft_abci_connection_method_timing_seconds_count{method="finalize_block"}`: 4,
			`cometbft_abci_connection_method_timing_seconds_sum{method="finalize_block"}`:   1.20,
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:           0.30,
		},
	}}
	after := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`:       13,
			`cometbft_abci_connection_method_timing_seconds_sum{method="check_tx"}`:         0.15,
			`cometbft_abci_connection_method_timing_seconds_count{method="finalize_block"}`: 4,
			`cometbft_abci_connection_method_timing_seconds_sum{method="finalize_block"}`:   1.20,
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:           0.35,
		},
	}}

	intervals := appendMetricDeltaWallClockIntervals(nil, base, before, base.Add(500*time.Millisecond), after)
	if len(intervals) != 2 {
		t.Fatalf("intervals len=%d want 2: %+v", len(intervals), intervals)
	}
	if intervals[0].Method != "check_tx" || intervals[1].Method != "commit" {
		t.Fatalf("methods = %q/%q, want check_tx/commit", intervals[0].Method, intervals[1].Method)
	}
	for _, interval := range intervals {
		if interval.Class != "abci" || interval.Provenance != "bounded_sample" {
			t.Fatalf("interval provenance = %+v, want ABCI bounded_sample", interval)
		}
		if interval.Started != base || interval.Ended != base.Add(500*time.Millisecond) || interval.Seconds != 0.5 {
			t.Fatalf("interval timing = %+v, want 500ms sample window", interval)
		}
	}
}

func TestMetricSamplePointsSeedFirstScrapeWithoutInterval(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	first := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 10,
		},
	}}

	var intervals []loadWindowInterval
	previous := seedMetricSamplePoints(base, first)
	if len(previous) != 1 {
		t.Fatalf("seeded samples len=%d want 1", len(previous))
	}
	intervals, previous = appendMetricDeltaWallClockIntervalsFromSamples(intervals, previous, base.Add(500*time.Millisecond), []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 10,
		},
	}})
	if len(intervals) != 0 {
		t.Fatalf("unchanged first-followup scrape emitted intervals: %+v", intervals)
	}
	if previous["validator-0"].at != base.Add(500*time.Millisecond) {
		t.Fatalf("previous sample time = %s, want followup scrape time", previous["validator-0"].at)
	}
}

func TestMetricSamplePointsPreserveLastGoodAcrossScrapeFailure(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	previous := seedMetricSamplePoints(base, []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`: 0,
		},
	}})

	var intervals []loadWindowInterval
	intervals, previous = appendMetricDeltaWallClockIntervalsFromSamples(intervals, previous, base.Add(500*time.Millisecond), []metricSnapshot{{
		Name:  "validator-0",
		Error: "connection refused",
	}})
	if len(intervals) != 0 {
		t.Fatalf("failed scrape emitted intervals: %+v", intervals)
	}
	if previous["validator-0"].at != base {
		t.Fatalf("failed scrape advanced previous sample to %s, want %s", previous["validator-0"].at, base)
	}

	intervals, previous = appendMetricDeltaWallClockIntervalsFromSamples(intervals, previous, base.Add(time.Second), []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`: 1,
		},
	}})
	if len(intervals) != 1 {
		t.Fatalf("intervals len=%d want 1: %+v", len(intervals), intervals)
	}
	if intervals[0].Started != base || intervals[0].Ended != base.Add(time.Second) || intervals[0].Seconds != 1 {
		t.Fatalf("interval timing = %+v, want last-good to success window", intervals[0])
	}
	if previous["validator-0"].at != base.Add(time.Second) {
		t.Fatalf("successful scrape did not advance previous sample: %s", previous["validator-0"].at)
	}
}

func TestUsableMetricSnapshotsDropsFailedFreshBaselineSamples(t *testing.T) {
	fresh := []metricSnapshot{
		{
			Name: "validator-0",
			URL:  "http://new-0/metrics",
			Metrics: map[string]float64{
				`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 12,
			},
		},
		{
			Name:  "validator-1",
			URL:   "http://new-1/metrics",
			Error: "connection refused",
		},
	}

	baseline := usableMetricSnapshots(fresh)
	if len(baseline) != 1 {
		t.Fatalf("baseline len=%d want 1: %+v", len(baseline), baseline)
	}
	byName := metricSnapshotsByName(baseline)
	if got := byName["validator-0"].Metrics[`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`]; got != 12 {
		t.Fatalf("validator-0 baseline = %v, want fresh value 12", got)
	}
	if byName["validator-0"].URL != "http://new-0/metrics" {
		t.Fatalf("validator-0 URL = %q, want fresh URL", byName["validator-0"].URL)
	}
	if byName["validator-1"] != nil {
		t.Fatalf("failed validator-1 should not be used as a fresh load-window baseline: %+v", byName["validator-1"])
	}
}

func TestAppendMissingUsableMetricBaselineSnapshotsAddsOnlyUnknownNodes(t *testing.T) {
	baseline := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 10,
		},
	}}
	current := []metricSnapshot{
		{
			Name: "validator-0",
			Metrics: map[string]float64{
				`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 99,
			},
		},
		{
			Name: "validator-1",
			Metrics: map[string]float64{
				`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 20,
			},
		},
		{
			Name:  "validator-2",
			Error: "connection refused",
		},
	}

	merged := appendMissingUsableMetricBaselineSnapshots(baseline, current)
	if len(merged) != 2 {
		t.Fatalf("merged len=%d want 2: %+v", len(merged), merged)
	}
	byName := metricSnapshotsByName(merged)
	if got := byName["validator-0"].Metrics[`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`]; got != 10 {
		t.Fatalf("validator-0 baseline = %v, want preserved original value 10", got)
	}
	if got := byName["validator-1"].Metrics[`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`]; got != 20 {
		t.Fatalf("validator-1 baseline = %v, want first usable value 20", got)
	}
	if byName["validator-2"] != nil {
		t.Fatalf("failed validator-2 should not be added as a missing baseline: %+v", byName["validator-2"])
	}
}

func TestBoundedSampleABCIIntervalsFeedAccounting(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	before := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 0,
			`cometbft_abci_connection_method_timing_seconds_sum{method="check_tx"}`:   0,
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`:   0,
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:     0,
		},
	}}
	first := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 4,
			`cometbft_abci_connection_method_timing_seconds_sum{method="check_tx"}`:   0.2,
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`:   0,
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:     0,
		},
	}}
	second := []metricSnapshot{{
		Name: "validator-0",
		Metrics: map[string]float64{
			`cometbft_abci_connection_method_timing_seconds_count{method="check_tx"}`: 4,
			`cometbft_abci_connection_method_timing_seconds_sum{method="check_tx"}`:   0.2,
			`cometbft_abci_connection_method_timing_seconds_count{method="commit"}`:   1,
			`cometbft_abci_connection_method_timing_seconds_sum{method="commit"}`:     0.1,
		},
	}}

	intervals := appendMetricDeltaWallClockIntervals(nil, base, before, base.Add(500*time.Millisecond), first)
	intervals = appendMetricDeltaWallClockIntervals(intervals, base.Add(500*time.Millisecond), first, base.Add(time.Second), second)
	obs := loadWindowObservation{
		StartedAt: base,
		EndedAt:   base.Add(time.Second),
		Seconds:   1,
		StorageSignals: []storageSignal{{
			Name:                "validator-0",
			ABCIObservedSeconds: 0.3,
			ABCICheckTxSeconds:  0.2,
			ABCICommitSeconds:   0.1,
		}},
		WallClockIntervals: intervals,
	}
	rows := summarizeLoadWindowAccounting(runResult{}, obs)
	if len(rows) != 1 {
		t.Fatalf("accounting rows len=%d want 1", len(rows))
	}
	row := rows[0]
	if row.ABCIBusyUnionSeconds == nil || *row.ABCIBusyUnionSeconds != 1 {
		t.Fatalf("ABCI bounded union = %v, want 1", row.ABCIBusyUnionSeconds)
	}
	if row.ABCIIntervalCount != 2 || row.ABCIIntervalProvenance != "bounded_sample" {
		t.Fatalf("interval count/provenance = %d/%q, want 2/bounded_sample", row.ABCIIntervalCount, row.ABCIIntervalProvenance)
	}
	if row.ValidatorNonABCIWallSeconds == nil || *row.ValidatorNonABCIWallSeconds != 0 {
		t.Fatalf("bounded non-ABCI wall = %v, want 0", row.ValidatorNonABCIWallSeconds)
	}
	if row.UnaccountedResidualClassification != "interval_union_based" {
		t.Fatalf("residual classification = %q, want interval_union_based", row.UnaccountedResidualClassification)
	}
}

func TestMetricDeltaSnapshotsPreserveBeforeAfterDelta(t *testing.T) {
	before := []metricSnapshot{{
		Name: "validator-0",
		URL:  "http://127.0.0.1:26660/metrics",
		Metrics: map[string]float64{
			"treedb_vlog_write_seconds_sum":  1.5,
			"process_cpu_seconds_total":      10,
			`cometbft_mempool_size{lane=""}`: 3,
		},
	}}
	after := []metricSnapshot{{
		Name: "validator-0",
		URL:  "http://127.0.0.1:26660/metrics",
		Metrics: map[string]float64{
			"treedb_vlog_write_seconds_sum":  4,
			"process_cpu_seconds_total":      15.25,
			`cometbft_mempool_size{lane=""}`: 0,
			"treedb_checkpoint_runs_total":   2,
		},
	}}

	deltas := metricDeltaSnapshots(before, after)
	if len(deltas) != 1 {
		t.Fatalf("deltas len = %d, want 1", len(deltas))
	}
	if got := deltas[0].Metrics["treedb_vlog_write_seconds_sum"]; got.Before != 1.5 || got.After != 4 || got.Delta != 2.5 {
		t.Fatalf("treedb delta = %+v, want before 1.5 after 4 delta 2.5", got)
	}
	if got := deltas[0].Metrics["process_cpu_seconds_total"]; got.Delta != 5.25 {
		t.Fatalf("process CPU delta = %+v, want delta 5.25", got)
	}
	if _, ok := deltas[0].Metrics["treedb_checkpoint_runs_total"]; ok {
		t.Fatalf("one-sided after metric fabricated as delta: %+v", deltas[0].Metrics["treedb_checkpoint_runs_total"])
	}

	payload, err := json.Marshal(loadWindowObservation{
		MetricsBefore: before,
		MetricsAfter:  after,
		MetricDeltas:  deltas,
	})
	if err != nil {
		t.Fatalf("marshal load window: %v", err)
	}
	for _, want := range []string{
		`"metrics_before"`,
		`"metrics_after"`,
		`"metric_deltas"`,
		`"before":1.5`,
		`"after":4`,
		`"delta":2.5`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload missing %s: %s", want, payload)
		}
	}
}

func TestMetricDeltaSnapshotsPreserveScrapeErrorsWithoutFabricatingDeltas(t *testing.T) {
	before := []metricSnapshot{{
		Name: "validator-0",
		URL:  "http://127.0.0.1:26660/metrics",
		Metrics: map[string]float64{
			"treedb_vlog_write_seconds_sum": 10,
		},
	}}
	after := []metricSnapshot{{
		Name:  "validator-0",
		URL:   "http://127.0.0.1:26660/metrics",
		Error: "GET /metrics: connection refused",
	}}

	deltas := metricDeltaSnapshots(before, after)
	if len(deltas) != 1 {
		t.Fatalf("deltas len = %d, want 1", len(deltas))
	}
	if deltas[0].Error == "" {
		t.Fatalf("expected scrape error to be preserved")
	}
	if len(deltas[0].Metrics) != 0 {
		t.Fatalf("fabricated metrics after scrape error: %+v", deltas[0].Metrics)
	}
}

func TestRenderReportMarkdownIncludesMetricDeltasAndProfileManifest(t *testing.T) {
	profileStarted := time.Date(2026, 7, 6, 12, 1, 0, 0, time.UTC)
	profileFinished := time.Date(2026, 7, 6, 12, 1, 5, 250000000, time.UTC)
	loadStartOffset := 1.5
	loadEndOffset := 6.75
	loadOverlap := 5.25
	artifact := reportArtifact{
		GeneratedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		Git: map[string]string{
			"branch": "codex/3582-accepted-window-attribution",
			"head":   "abc123",
		},
		Results: []runResult{{
			Scenario: scenario{Name: "plain-send"},
			LoadWindow: &loadWindowObservation{
				Reached:                true,
				DurationSatisfied:      true,
				Seconds:                12.5,
				IncludedTransactions:   100,
				SuccessfulTransactions: 100,
				TargetTransactions:     100,
				PhaseOverlaps: []loadWindowPhaseOverlap{{
					Name:                "run_load_test",
					PhaseSeconds:        15,
					BeforeWindowSeconds: 2.5,
					InWindowSeconds:     12.5,
					Classification:      "crosses_window_start",
					Note:                "wallets=100",
				}},
				PipelineSignals: []pipelineSignal{{
					Name:                    "validator-0",
					SubmittedTransactions:   120,
					IncludedTransactions:    100,
					SuccessfulTransactions:  100,
					SubmittedMinusIncluded:  20,
					CollectorBlockSpan:      10,
					AvgTxsPerConsensusBlock: 10,
					AvgBlockIntervalSeconds: 1.25,
					AvgCheckTxSeconds:       0.001,
					AvgFinalizeBlockSeconds: 0.2,
					AvgCommitSeconds:        0.05,
				}},
				CadenceDiagnostics: []cadenceDiagnostic{{
					Name:                             "validator-0",
					BlockCount:                       10,
					AvgBlockIntervalSeconds:          1.25,
					AvgTxsPerBlock:                   10,
					ABCIBlockStageSecondsPerBlock:    0.25,
					CheckTxSecondsPerBlockEquivalent: 0.01,
					CadenceResidualAfterABCIBlockStagesSeconds: 1,
					CadenceResidualPctOfBlockInterval:          80,
					MissingEventSpans: []string{
						"DB adapter WriteSync / command-WAL / value-log / checkpoint spans are not exported as event-level load-window spans",
					},
					Stages: []cadenceStage{{
						Name:               "commit",
						Class:              "abci_block_stage",
						Source:             "cometbft_abci_connection_method_timing_seconds",
						Provenance:         "prometheus_counter_delta",
						Seconds:            0.5,
						Count:              10,
						AvgSeconds:         0.05,
						PerBlockSeconds:    0.05,
						IncludedInResidual: true,
					}},
				}},
				MetricDeltas: []metricDeltaSnapshot{{
					Name: "validator-0",
					Metrics: map[string]metricDeltaValue{
						"treedb_vlog_write_seconds_sum": {Before: 1, After: 3.25, Delta: 2.25},
					},
				}},
			},
			ProfileArtifacts: []profileArtifact{
				{
					Name:                                     "plain-send",
					Kind:                                     "validator_block",
					Boundary:                                 profileBoundaryWholeRun,
					Endpoint:                                 "block?seconds=5",
					RequestedSeconds:                         5,
					StartedAt:                                &profileStarted,
					FinishedAt:                               &profileFinished,
					CollectionSeconds:                        5.25,
					CollectionStartedLoadWindowOffsetSeconds: &loadStartOffset,
					CollectionFinishedLoadWindowOffsetSeconds: &loadEndOffset,
					CollectionLoadWindowOverlapSeconds:        &loadOverlap,
					Path:                                      "/tmp/plain-send-block.pprof",
					TopSummaryPath:                            "/tmp/plain-send-block.pprof.top.txt",
				},
				{
					Name:     "plain-send",
					Kind:     "validator_mutex",
					Boundary: profileBoundaryWholeRun,
					Error:    "fetch validator_mutex profile exit 1",
				},
			},
		}},
	}

	md := renderReportMarkdown(artifact)
	for _, want := range []string{
		"## plain-send Accepted Window",
		"### Accepted-Window Phase Overlap",
		"| run_load_test | crosses_window_start | 15 | 2.5 | 12.5 | 0 | wallets=100 |",
		"### Accepted-Window Transaction Pipeline Summary",
		"| validator-0 | 120 | 100 | 100 | 20 | 10 | 10 | 1.25 | 0.001 | 0.2 | 0.05 |",
		"### Accepted-Window Cadence Diagnostics",
		"| validator-0 | 10 | 1.25 | 10 | 0.25 | 0.01 | 0 | 1 | 80 | false | DB adapter WriteSync / command-WAL / value-log / checkpoint spans are not exported as event-level load-window spans |",
		"### Accepted-Window Cadence Diagnostic Stages",
		"| validator-0 | commit | abci_block_stage | cometbft_abci_connection_method_timing_seconds | prometheus_counter_delta | 0.5 | 10 | 0.05 | 0.05 | true |  |",
		"### Accepted-Window Metric Deltas",
		"treedb_vlog_write_seconds_sum",
		"| validator-0 | treedb_vlog_write_seconds_sum | 1 | 3.25 | 2.25 |",
		"## plain-send Profile Manifest",
		"validator_block",
		profileBoundaryWholeRun,
		"block?seconds=5",
		"2026-07-06T12:01:00Z",
		"2026-07-06T12:01:05.25Z",
		"5.25",
		"1.5",
		"6.75",
		"/tmp/plain-send-block.pprof.top.txt",
		"fetch validator_mutex profile exit 1",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRenderReportMarkdownIncludesMetricScrapeErrorsWithoutDeltas(t *testing.T) {
	artifact := reportArtifact{
		GeneratedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		Results: []runResult{{
			Scenario: scenario{Name: "plain-send"},
			LoadWindow: &loadWindowObservation{
				Reached:                true,
				DurationSatisfied:      true,
				Seconds:                12.5,
				IncludedTransactions:   100,
				SuccessfulTransactions: 100,
				TargetTransactions:     100,
				MetricDeltas: []metricDeltaSnapshot{{
					Name:  "validator-0",
					Error: "after: GET /metrics: connection refused",
				}},
			},
		}},
	}

	md := renderReportMarkdown(artifact)
	for _, want := range []string{
		"### Accepted-Window Metric Scrape Errors",
		"validator-0",
		"after: GET /metrics: connection refused",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "No accepted-window metric deltas were recorded.") {
		t.Fatalf("markdown hid scrape errors as no-delta message:\n%s", md)
	}
}

func TestParseTreeDBDebugVarsExtractsInstanceCounters(t *testing.T) {
	raw := []byte(`{
		"cmdline": ["simd"],
		"treedb": {
			"treedb.expvar.instances_count": 1,
			"instances": {
				"/simd/data/application.db#0x123": {
					"treedb.expvar.wal_dir": "/simd/data/application.db/wal",
					"treedb.expvar.is_current": true,
					"treedb.command_wal.writer.sync_ns_total": 2500000000,
					"treedb.cache.checkpoint.total_ms": 1250,
					"treedb.process.identity.wal_dir": "/simd/data/application.db/wal",
					"treedb.process.memory.pool_pressure_level": "nominal"
				}
			}
		}
	}`)
	snapshots, err := parseTreeDBDebugVars("validator-0", "http://127.0.0.1:6060/debug/vars", raw)
	if err != nil {
		t.Fatalf("parse debug vars: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snapshots))
	}
	got := snapshots[0]
	if got.Name != "validator-0" || got.Store != "application.db" || got.WALDir != "/simd/data/application.db/wal" {
		t.Fatalf("snapshot identity = %+v", got)
	}
	if got.Metrics["treedb.command_wal.writer.sync_ns_total"] != 2500000000 {
		t.Fatalf("sync ns metric = %v", got.Metrics["treedb.command_wal.writer.sync_ns_total"])
	}
	if got.Metrics["treedb.cache.checkpoint.total_ms"] != 1250 {
		t.Fatalf("checkpoint ms metric = %v", got.Metrics["treedb.cache.checkpoint.total_ms"])
	}
	if _, ok := got.Metrics["treedb.process.memory.pool_pressure_level"]; ok {
		t.Fatalf("string status value was included as numeric metric")
	}
}

func TestTreeDBStatsDeltasAndTimingSummary(t *testing.T) {
	before := []treeDBStatsSnapshot{{
		Name:     "validator-0",
		Instance: "app#1",
		Store:    "application.db",
		WALDir:   "/simd/data/application.db/wal",
		Metrics: map[string]float64{
			"treedb.command_wal.writer.sync_ns_total": 1_000_000_000,
			"treedb.cache.checkpoint.total_ms":        500,
			"treedb.process.memory.heap_inuse_bytes":  128,
		},
	}}
	after := []treeDBStatsSnapshot{{
		Name:     "validator-0",
		Instance: "app#1",
		Store:    "application.db",
		WALDir:   "/simd/data/application.db/wal",
		Metrics: map[string]float64{
			"treedb.command_wal.writer.sync_ns_total": 3_500_000_000,
			"treedb.cache.checkpoint.total_ms":        750,
			"treedb.process.memory.heap_inuse_bytes":  256,
		},
	}}
	deltas := treeDBStatsDeltaSnapshots(before, after)
	if len(deltas) != 1 {
		t.Fatalf("deltas len = %d, want 1", len(deltas))
	}
	if got := deltas[0].Metrics["treedb.command_wal.writer.sync_ns_total"].Delta; got != 2_500_000_000 {
		t.Fatalf("sync delta = %v", got)
	}
	rows := treeDBTimingSummaryRows(deltas)
	if len(rows) != 2 {
		t.Fatalf("timing rows = %+v, want command WAL and checkpoint", rows)
	}
	gotSeconds := map[string]float64{}
	for _, row := range rows {
		gotSeconds[row.Group] = row.Seconds
	}
	if gotSeconds["command WAL"] != 2.5 {
		t.Fatalf("command WAL seconds = %v, want 2.5", gotSeconds["command WAL"])
	}
	if gotSeconds["checkpoint"] != 0.25 {
		t.Fatalf("checkpoint seconds = %v, want 0.25", gotSeconds["checkpoint"])
	}
}

func TestRenderReportMarkdownIncludesTreeDBDeltasWithoutPrometheusRows(t *testing.T) {
	artifact := reportArtifact{
		GeneratedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		Results: []runResult{{
			Scenario: scenario{Name: "plain-send"},
			LoadWindow: &loadWindowObservation{
				Reached:                true,
				DurationSatisfied:      true,
				Seconds:                12.5,
				IncludedTransactions:   100,
				SuccessfulTransactions: 100,
				TargetTransactions:     100,
				TreeDBStatDeltas: []treeDBStatsDelta{{
					Name:     "validator-0",
					Instance: "app#1",
					Store:    "application.db",
					WALDir:   "/simd/data/application.db/wal",
					Metrics: map[string]metricDeltaValue{
						"treedb.command_wal.writer.sync_ns_total": {
							Before: 1_000_000_000,
							After:  3_000_000_000,
							Delta:  2_000_000_000,
						},
					},
				}},
			},
		}},
	}
	md := renderReportMarkdown(artifact)
	for _, want := range []string{
		"### Accepted-Window TreeDB Timing Counter Deltas",
		"| validator-0 | application.db | command WAL | 2 |",
		"### Accepted-Window TreeDB Store Counter Deltas",
		"treedb.command_wal.writer.sync_ns_total",
		"/simd/data/application.db/wal",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "No accepted-window metric deltas were recorded.") {
		t.Fatalf("markdown hid TreeDB deltas as no-delta message:\n%s", md)
	}
}

func TestRenderReportMarkdownIncludesAccountingTimelineAndDwell(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	artifact := reportArtifact{
		GeneratedAt: base,
		Results: []runResult{{
			Scenario: scenario{Name: "plain-send-treedb"},
			LoadWindow: &loadWindowObservation{
				Reached:                true,
				DurationSatisfied:      true,
				StartedAt:              base,
				EndedAt:                base.Add(10 * time.Second),
				Seconds:                10,
				IncludedTransactions:   1000,
				SuccessfulTransactions: 1000,
				TargetTransactions:     1000,
				Accounting: []loadWindowAccounting{{
					Name:                            "validator-0",
					LoadWindowSeconds:               10,
					LoadGeneratorWallSeconds:        11,
					ABCIObservedSumSeconds:          3,
					ABCIBusyUnionSeconds:            floatPtr(2.5),
					ABCIOverlapSeconds:              floatPtr(0.5),
					ABCIByMethodSeconds:             map[string]float64{"commit": 1, "finalize_block": 1.25, "check_tx": 0.5},
					ValidatorNonABCIWallSeconds:     floatPtr(7.5),
					ValidatorNonABCIApproxSeconds:   7,
					ValidatorNonABCIPctApprox:       70,
					ValidatorProcessCPUSecondsDelta: 25,
					ValidatorCoreEquivalent:         2.5,
					ConsensusBlockCadenceSeconds:    2,
					ABCIIntervalProvenance:          "exact_event",
					LoadgenClientWaitMissingReason:  "Catalyst wait unavailable",
					UnaccountedResidualFormula:      "max(0, load_window_seconds - abci_busy_union_seconds)",
				}},
				TreeDBStatsTimeline: []treeDBStatsTimelineSample{{
					Label:          "load_start_proxy",
					At:             base,
					ElapsedSeconds: 0,
					Stats:          []treeDBStatsSnapshot{{Name: "validator-0", Store: "application.db"}},
				}, {
					Label:          "load_end",
					At:             base.Add(10 * time.Second),
					ElapsedSeconds: 10,
					Stats:          []treeDBStatsSnapshot{{Name: "validator-0", Store: "application.db"}},
				}},
				MetricDeltas: []metricDeltaSnapshot{{
					Name:    "validator-0",
					Metrics: map[string]metricDeltaValue{"tx_count": {Before: 0, After: 1000, Delta: 1000}},
				}},
			},
			TreeDBDwellSnapshots: []treeDBDwellSnapshot{{
				Label:     "load_end",
				At:        base.Add(10 * time.Second),
				DataSizes: []dataPathSize{{Name: "validator-0", Path: "/simd/data", Bytes: 100}},
			}, {
				Label:          "post_dwell",
				At:             base.Add(5*time.Minute + 10*time.Second),
				ElapsedSeconds: 300,
				DataSizes:      []dataPathSize{{Name: "validator-0", Path: "/simd/data", Bytes: 90}},
				DataSizeDeltas: []dataPathSizeDelta{{Name: "validator-0", Path: "/simd/data", Before: 100, After: 90, Delta: -10}},
			}},
		}},
	}

	md := renderReportMarkdown(artifact)
	for _, want := range []string{
		"### Accepted-Window Non-ABCI Accounting",
		"| validator-0 | 10 | 11 | 3 | 2.5 | 0.5 | 1 | 1.25 | 0.5 | 7.5 | 7 | 70 | 25 | 2.5 | 2 | exact_event |",
		"### Accepted-Window TreeDB Stats Timeline",
		"| load_end | 2026-07-08T12:00:10Z | 10 | 1 |",
		"## plain-send-treedb TreeDB Post-Load Dwell",
		"### TreeDB Dwell Data Size Deltas",
		"| validator-0 | /simd/data | 100 | 90 | -10 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestAppProfileRequestsKeepsExtendedPprofOptIn(t *testing.T) {
	sc := withAppHeapProfile(scenario{Name: "plain-send", BaseImage: "simapp"})
	if got := appProfileRequests(sc, appProfileCaptureConfig{}); len(got) != 0 {
		t.Fatalf("profile requests without dirs len = %d, want 0", len(got))
	}

	requests := appProfileRequests(sc, appProfileCaptureConfig{PprofOutDir: "/tmp/profiles"})
	var kinds []string
	for _, req := range requests {
		kinds = append(kinds, req.Kind+":"+req.Boundary)
	}
	want := []string{
		"validator_heap:" + profileBoundaryLoadWindowAdjacent,
		"validator_allocs:" + profileBoundaryWholeRun,
		"validator_goroutine:" + profileBoundaryLoadWindowAdjacent,
	}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("profile requests = %v, want %v", kinds, want)
	}
}

func TestAppProfileCaptureConfigActiveWindowOptIn(t *testing.T) {
	var zero appProfileCaptureConfig
	if zero.needsPprof() {
		t.Fatalf("zero config unexpectedly needs pprof")
	}
	if zero.needsDiagnosticProfileFlags() {
		t.Fatalf("zero config unexpectedly needs diagnostic profile flags")
	}

	active := appProfileCaptureConfig{ActiveWindowOutDir: "/tmp/active"}
	if !active.needsPprof() {
		t.Fatalf("active-window config should enable app pprof")
	}
	if !active.needsDiagnosticProfileFlags() {
		t.Fatalf("active-window config should enable block/mutex sampling flags")
	}
	debugVars := appProfileCaptureConfig{DebugVars: true}
	if !debugVars.needsPprof() {
		t.Fatalf("debug-vars config should enable app pprof")
	}
	if debugVars.needsDiagnosticProfileFlags() {
		t.Fatalf("debug-vars config should not enable block/mutex sampling flags")
	}
}

func TestAppProfileRequestsAddsDiagnosticProfilesOptIn(t *testing.T) {
	sc := withAppHeapProfile(scenario{Name: "plain-send", BaseImage: "simapp"})
	requests := appProfileRequests(sc, appProfileCaptureConfig{
		DiagnosticOutDir:   "/tmp/diagnostics",
		DiagnosticDuration: 1500 * time.Millisecond,
	})

	var got []string
	for _, req := range requests {
		got = append(got, strings.Join([]string{
			req.Kind,
			req.Endpoint,
			req.FileName,
			req.OutDir,
			req.Boundary,
			req.Duration.String(),
		}, "|"))
	}
	want := []string{
		"validator_block|block?seconds=2|plain-send-validator-0-block.pprof|/tmp/diagnostics|" + profileBoundaryLoadWindowAdjacent + "|1.5s",
		"validator_mutex|mutex?seconds=2|plain-send-validator-0-mutex.pprof|/tmp/diagnostics|" + profileBoundaryLoadWindowAdjacent + "|1.5s",
		"validator_trace|trace?seconds=2|plain-send-validator-0-trace.out|/tmp/diagnostics|" + profileBoundaryLoadWindowAdjacent + "|1.5s",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("diagnostic profile requests = %v, want %v", got, want)
	}
}

func TestAppActiveWindowProfileRequestsAddsAllKindsOptIn(t *testing.T) {
	sc := withAppHeapProfile(scenario{Name: "plain-send", BaseImage: "simapp"})
	if got := appActiveWindowProfileRequests(sc, appProfileCaptureConfig{}); len(got) != 0 {
		t.Fatalf("active profile requests without dir len = %d, want 0", len(got))
	}

	requests := appActiveWindowProfileRequests(sc, appProfileCaptureConfig{
		ActiveWindowOutDir:   "/tmp/active",
		ActiveWindowDuration: 1500 * time.Millisecond,
	})

	var got []string
	for _, req := range requests {
		got = append(got, strings.Join([]string{
			req.Kind,
			req.Endpoint,
			req.FileName,
			req.OutDir,
			req.Boundary,
			req.Duration.String(),
		}, "|"))
	}
	want := []string{
		"validator_cpu|profile?seconds=2|plain-send-validator-0-active-window-cpu.pprof|/tmp/active|" + profileBoundaryActiveWindow + "|1.5s",
		"validator_heap|heap?gc=1|plain-send-validator-0-active-window-heap.pprof|/tmp/active|" + profileBoundaryActiveWindow + "|0s",
		"validator_allocs|allocs|plain-send-validator-0-active-window-allocs.pprof|/tmp/active|" + profileBoundaryActiveWindow + "|0s",
		"validator_goroutine|goroutine|plain-send-validator-0-active-window-goroutine.pprof|/tmp/active|" + profileBoundaryActiveWindow + "|0s",
		"validator_block|block?seconds=2|plain-send-validator-0-active-window-block.pprof|/tmp/active|" + profileBoundaryActiveWindow + "|1.5s",
		"validator_mutex|mutex?seconds=2|plain-send-validator-0-active-window-mutex.pprof|/tmp/active|" + profileBoundaryActiveWindow + "|1.5s",
		"validator_trace|trace?seconds=2|plain-send-validator-0-active-window-trace.out|/tmp/active|" + profileBoundaryActiveWindow + "|1.5s",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("active profile requests = %v, want %v", got, want)
	}
}

func TestWithAppDiagnosticProfileFlagsEnablesBlockAndMutexSampling(t *testing.T) {
	originalFlags := []string{"--existing", "value"}
	sc := withAppDiagnosticProfileFlags(scenario{
		Name:            "plain-send",
		BaseImage:       "simapp",
		AdditionalFlags: originalFlags,
	})
	got := strings.Join(sc.AdditionalFlags, " ")
	want := "--existing value --block-profile-rate 1 --mutex-profile-fraction 1"
	if got != want {
		t.Fatalf("additional flags = %q, want %q", got, want)
	}
	if strings.Join(originalFlags, " ") != "--existing value" {
		t.Fatalf("mutated original flags: %+v", originalFlags)
	}

	nonSimapp := withAppDiagnosticProfileFlags(scenario{Name: "evm", BaseImage: "evm"})
	if len(nonSimapp.AdditionalFlags) != 0 {
		t.Fatalf("non-simapp flags = %+v, want empty", nonSimapp.AdditionalFlags)
	}
}

func TestSplitAppProfileRequestsForRawTxAuditKeepsCPUAfterAudit(t *testing.T) {
	sc := withAppCPUProfile(withAppHeapProfile(scenario{Name: "plain-send", BaseImage: "simapp"}))
	requests := appProfileRequests(sc, appProfileCaptureConfig{CPUOutDir: "/tmp/cpu", HeapOutDir: "/tmp/profiles", PprofOutDir: "/tmp/profiles"})
	preAudit, postAudit := splitAppProfileRequestsForRawTxAudit(requests)

	for _, req := range preAudit {
		if req.Kind == "validator_cpu" {
			t.Fatalf("pre-audit request contains stopping CPU profile: %+v", req)
		}
	}
	var preKinds []string
	for _, req := range preAudit {
		preKinds = append(preKinds, req.Kind)
	}
	wantPre := []string{"validator_heap", "validator_allocs", "validator_goroutine"}
	if strings.Join(preKinds, ",") != strings.Join(wantPre, ",") {
		t.Fatalf("pre-audit profile kinds = %v, want %v", preKinds, wantPre)
	}
	if len(postAudit) != 1 || postAudit[0].Kind != "validator_cpu" {
		t.Fatalf("post-audit requests = %+v, want only validator_cpu", postAudit)
	}
}

func TestAnnotateProfileArtifactsWithLoadWindowTiming(t *testing.T) {
	loadStarted := time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC)
	loadEnded := loadStarted.Add(5 * time.Second)
	collectionStarted := loadStarted.Add(2 * time.Second)
	collectionFinished := loadStarted.Add(6 * time.Second)

	artifacts := annotateProfileArtifactsWithLoadWindowTiming([]profileArtifact{{
		Kind:       "validator_cpu",
		StartedAt:  &collectionStarted,
		FinishedAt: &collectionFinished,
	}}, &loadWindowObservation{
		StartedAt: loadStarted,
		EndedAt:   loadEnded,
	})

	if len(artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(artifacts))
	}
	if artifacts[0].CollectionStartedLoadWindowOffsetSeconds == nil || *artifacts[0].CollectionStartedLoadWindowOffsetSeconds != 2 {
		t.Fatalf("start offset = %v, want 2", artifacts[0].CollectionStartedLoadWindowOffsetSeconds)
	}
	if artifacts[0].CollectionFinishedLoadWindowOffsetSeconds == nil || *artifacts[0].CollectionFinishedLoadWindowOffsetSeconds != 6 {
		t.Fatalf("finish offset = %v, want 6", artifacts[0].CollectionFinishedLoadWindowOffsetSeconds)
	}
	if artifacts[0].CollectionLoadWindowOverlapSeconds == nil || *artifacts[0].CollectionLoadWindowOverlapSeconds != 3 {
		t.Fatalf("overlap = %v, want 3", artifacts[0].CollectionLoadWindowOverlapSeconds)
	}
}

type fakeProfileEndpointValidator struct {
	body []byte
}

func (v fakeProfileEndpointValidator) RunCommand(context.Context, []string) (string, string, int, error) {
	time.Sleep(time.Millisecond)
	return "", "", 0, nil
}

func (v fakeProfileEndpointValidator) ReadFile(context.Context, string) ([]byte, error) {
	return v.body, nil
}

func TestCollectAppPprofEndpointProfileRecordsFinishedMetadata(t *testing.T) {
	outDir := t.TempDir()
	artifact := collectAppPprofEndpointProfile(context.Background(), fakeProfileEndpointValidator{body: []byte("profile")}, scenario{Name: "plain-send"}, appProfileRequest{
		Kind:     "validator_heap",
		Endpoint: "heap?gc=1",
		FileName: "heap.pprof",
		OutDir:   outDir,
		Boundary: profileBoundaryActiveWindow,
	})

	if artifact.Error != "" {
		t.Fatalf("artifact error = %q", artifact.Error)
	}
	if artifact.StartedAt == nil {
		t.Fatalf("collection start was not recorded")
	}
	if artifact.FinishedAt == nil {
		t.Fatalf("collection finish was not recorded")
	}
	if artifact.CollectionSeconds <= 0 {
		t.Fatalf("collection seconds = %v, want > 0", artifact.CollectionSeconds)
	}
	if artifact.Path != filepath.Join(outDir, "heap.pprof") {
		t.Fatalf("path = %q", artifact.Path)
	}
}

func TestAttachPprofTopSummariesSkipsTrace(t *testing.T) {
	artifacts := attachPprofTopSummaries(context.Background(), []profileArtifact{{
		Kind: "validator_trace",
		Path: filepath.Join(t.TempDir(), "trace.out"),
	}})
	if len(artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(artifacts))
	}
	if artifacts[0].TopSummaryPath != "" || artifacts[0].TopSummaryError != "" {
		t.Fatalf("trace artifact got top summary fields: %+v", artifacts[0])
	}
}

func TestLabelValue(t *testing.T) {
	expr := `begin_blocker_sum{chain_id="streedb",module="distribution"}`
	if got := labelValue(expr, "module"); got != "distribution" {
		t.Fatalf("module label = %q, want distribution", got)
	}
}

func TestStartPhaseRecordsTimeline(t *testing.T) {
	var result runResult
	end := startPhase(&result, "test_phase", "note")
	end()
	if len(result.PhaseTimeline) != 1 {
		t.Fatalf("phase timeline len = %d, want 1", len(result.PhaseTimeline))
	}
	span := result.PhaseTimeline[0]
	if span.Name != "test_phase" || span.Note != "note" {
		t.Fatalf("span = %+v, want named test_phase with note", span)
	}
	if span.Seconds < 0 {
		t.Fatalf("span seconds = %v, want non-negative", span.Seconds)
	}
}

func TestSummarizeLoadWindowPhaseOverlaps(t *testing.T) {
	base := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	obs := loadWindowObservation{
		StartedAt: base.Add(10 * time.Second),
		EndedAt:   base.Add(30 * time.Second),
	}
	spans := []phaseSpan{
		{
			Name:    "before",
			Started: base,
			Ended:   base.Add(5 * time.Second),
		},
		{
			Name:    "load",
			Started: base.Add(8 * time.Second),
			Ended:   base.Add(24 * time.Second),
		},
		{
			Name:    "spans",
			Started: base.Add(2 * time.Second),
			Ended:   base.Add(40 * time.Second),
		},
		{
			Name:    "after",
			Started: base.Add(31 * time.Second),
			Ended:   base.Add(35 * time.Second),
		},
	}
	got := summarizeLoadWindowPhaseOverlaps(spans, obs)
	if len(got) != 4 {
		t.Fatalf("overlap rows = %d, want 4", len(got))
	}
	if got[0].Classification != "before_window" || got[0].BeforeWindowSeconds != 5 {
		t.Fatalf("before row = %+v, want before_window with 5s before", got[0])
	}
	if got[1].Classification != "crosses_window_start" || got[1].BeforeWindowSeconds != 2 || got[1].InWindowSeconds != 14 {
		t.Fatalf("load row = %+v, want 2s before and 14s inside", got[1])
	}
	if got[2].Classification != "spans_window" || got[2].BeforeWindowSeconds != 8 || got[2].InWindowSeconds != 20 || got[2].AfterWindowSeconds != 10 {
		t.Fatalf("spans row = %+v, want 8s before, 20s inside, 10s after", got[2])
	}
	if got[3].Classification != "after_window" || got[3].AfterWindowSeconds != 4 {
		t.Fatalf("after row = %+v, want after_window with 4s after", got[3])
	}
}

func TestSummarizeRuntimeBreakdown(t *testing.T) {
	result := runResult{
		WallSeconds:   120,
		LaunchSeconds: 50,
		LoadTestResult: ctlt.LoadTestResult{
			Overall: ctlt.OverallStats{Runtime: 40 * time.Second},
		},
		StorageSignals: []storageSignal{{
			Name:                     "validator-0",
			ABCIObservedSeconds:      12,
			ABCICommitSeconds:        4,
			ABCIFinalizeBlockSeconds: 6,
			ABCICheckTxSeconds:       1,
			ABCIOtherSeconds:         1,
		}},
	}
	breakdowns := summarizeRuntimeBreakdown(result)
	if len(breakdowns) != 1 {
		t.Fatalf("breakdown len = %d, want 1", len(breakdowns))
	}
	breakdown := breakdowns[0]
	if breakdown.PostLaunchNonWorkloadSeconds != 30 {
		t.Fatalf("post-launch non-workload = %v, want 30", breakdown.PostLaunchNonWorkloadSeconds)
	}
	if breakdown.NonABCIWorkloadSeconds != 28 {
		t.Fatalf("non-ABCI workload = %v, want 28", breakdown.NonABCIWorkloadSeconds)
	}
	if breakdown.CommitPctOfWorkload != 10 {
		t.Fatalf("commit pct = %v, want 10", breakdown.CommitPctOfWorkload)
	}
	if breakdown.CommitPlusFinalizePctOfWorkload != 25 {
		t.Fatalf("commit+finalize pct = %v, want 25", breakdown.CommitPlusFinalizePctOfWorkload)
	}
}

func TestCorrectedLoadTestPrefersRawAudit(t *testing.T) {
	result := runResult{
		LoadTestResult: ctlt.LoadTestResult{
			Overall: ctlt.OverallStats{
				TotalTransactions:         150,
				TotalIncludedTransactions: 0,
				SuccessfulTransactions:    0,
				FailedTransactions:        150,
				Runtime:                   10 * time.Second,
			},
		},
		RawTxSummary: &txAuditSummary{
			Queried:      150,
			Found:        150,
			Successful:   150,
			Failed:       0,
			TotalGasUsed: 12345,
		},
	}
	got := summarizeCorrectedLoadTest(result)
	if got == nil {
		t.Fatalf("corrected summary was nil")
	}
	if got.Source != "raw_tx_audit" {
		t.Fatalf("source = %q, want raw_tx_audit", got.Source)
	}
	if got.SuccessfulTransactions != 150 || got.FailedTransactions != 0 {
		t.Fatalf("corrected counts = success %d failed %d, want 150/0", got.SuccessfulTransactions, got.FailedTransactions)
	}
	if !got.CatalystMismatch {
		t.Fatalf("expected catalyst mismatch")
	}
	if got.TPS != 15 {
		t.Fatalf("tps = %v, want 15", got.TPS)
	}
}

func TestCorrectedLoadTestUsesAppMetricsWhenAuditSkipped(t *testing.T) {
	result := runResult{
		RawTxAuditSkipped: "disabled by test",
		LoadTestResult: ctlt.LoadTestResult{
			Overall: ctlt.OverallStats{
				TotalTransactions:         150,
				TotalIncludedTransactions: 0,
				SuccessfulTransactions:    0,
				FailedTransactions:        150,
				Runtime:                   30 * time.Second,
			},
		},
		StorageSignals: []storageSignal{{
			Name:                   "validator-0",
			ConsensusTotalTxsDelta: 150,
			SDKTxCountDelta:        150,
			SDKTxSuccessfulDelta:   150,
		}},
	}
	got := summarizeCorrectedLoadTest(result)
	if got == nil {
		t.Fatalf("corrected summary was nil")
	}
	if got.Source != "app_metrics" {
		t.Fatalf("source = %q, want app_metrics", got.Source)
	}
	if got.IncludedTransactions != 150 || got.SuccessfulTransactions != 150 || got.FailedTransactions != 0 {
		t.Fatalf("corrected counts = included %d success %d failed %d, want 150/150/0", got.IncludedTransactions, got.SuccessfulTransactions, got.FailedTransactions)
	}
	if got.TPS != 5 {
		t.Fatalf("tps = %v, want 5", got.TPS)
	}
	if !got.CatalystMismatch {
		t.Fatalf("expected catalyst mismatch")
	}
	if len(got.AppMetricsIncludedCandidates) == 0 {
		t.Fatalf("expected app metric candidates")
	}
}

func TestCorrectedLoadTestStoppedUsesAcceptedLoadWindowCounts(t *testing.T) {
	result := runResult{
		RawTxAuditSkipped: "disabled by test",
		LoadTestStopped:   "load window reached",
		StorageSignals: []storageSignal{{
			Name:                   "validator-0",
			ConsensusTotalTxsDelta: 125,
			SDKTxCountDelta:        125,
			SDKTxSuccessfulDelta:   125,
		}},
		LoadWindow: &loadWindowObservation{
			TargetTransactions:     100,
			MinimumSeconds:         10,
			DurationSatisfied:      true,
			Reached:                true,
			Seconds:                20,
			IncludedTransactions:   100,
			SuccessfulTransactions: 100,
		},
	}
	got := summarizeCorrectedLoadTest(result)
	if got == nil {
		t.Fatalf("corrected summary was nil")
	}
	if got.IncludedTransactions != 100 || got.SuccessfulTransactions != 100 || got.TotalTransactions != 100 {
		t.Fatalf("corrected counts = total %d included %d success %d, want 100/100/100", got.TotalTransactions, got.IncludedTransactions, got.SuccessfulTransactions)
	}
	if got.RuntimeSeconds != 20 {
		t.Fatalf("runtime seconds = %v, want 20", got.RuntimeSeconds)
	}
	if got.TPS != 5 {
		t.Fatalf("tps = %v, want 5", got.TPS)
	}
	if !got.CatalystMismatch {
		t.Fatalf("expected catalyst mismatch")
	}
}

func TestCorrectedLoadTestKeepsValidCatalystCounts(t *testing.T) {
	result := runResult{
		LoadTestResult: ctlt.LoadTestResult{
			Overall: ctlt.OverallStats{
				TotalTransactions:         150,
				TotalIncludedTransactions: 150,
				SuccessfulTransactions:    150,
				FailedTransactions:        0,
				Runtime:                   30 * time.Second,
				TPS:                       5,
			},
		},
		StorageSignals: []storageSignal{{
			Name:                   "validator-0",
			ConsensusTotalTxsDelta: 150,
			SDKTxCountDelta:        150,
			SDKTxSuccessfulDelta:   150,
		}},
	}
	got := summarizeCorrectedLoadTest(result)
	if got == nil {
		t.Fatalf("corrected summary was nil")
	}
	if got.Source != "catalyst" {
		t.Fatalf("source = %q, want catalyst", got.Source)
	}
	if got.CatalystMismatch {
		t.Fatalf("did not expect catalyst mismatch")
	}
	if got.TPS != 5 {
		t.Fatalf("tps = %v, want 5", got.TPS)
	}
}

func TestDeriveMetricsUsesCorrectedCountsAndLoadPhaseWall(t *testing.T) {
	sc := scenario{
		LoadTestSpec: ctlt.LoadTestSpec{
			NumOfBlocks: 3,
			NumOfTxs:    50,
			Msgs: []ctlt.LoadTestMsg{{
				Type:            ctlt.MsgType("MsgArr"),
				ContainedType:   ctlt.MsgType("MsgMultiSend"),
				NumMsgs:         20,
				NumOfRecipients: 25,
			}},
		},
	}
	result := runResult{
		WallSeconds: 100,
		PhaseTimeline: []phaseSpan{{
			Name:    "run_load_test",
			Seconds: 20,
		}},
		LoadTestResult: ctlt.LoadTestResult{
			Overall: ctlt.OverallStats{
				TotalTransactions:         150,
				TotalIncludedTransactions: 0,
				SuccessfulTransactions:    0,
				FailedTransactions:        150,
				Runtime:                   10 * time.Second,
			},
		},
		CorrectedLoadTest: &correctedLoadTest{
			Source:                 "app_metrics",
			TotalTransactions:      150,
			IncludedTransactions:   150,
			SuccessfulTransactions: 150,
			RuntimeSeconds:         10,
			TPS:                    15,
		},
		LoadWindow: &loadWindowObservation{
			Reached:                true,
			Seconds:                15,
			IncludedTransactions:   120,
			SuccessfulTransactions: 120,
		},
	}
	got := deriveMetrics(sc, result)
	if got.IntendedTransactions != 150 {
		t.Fatalf("intended txs = %d, want 150", got.IntendedTransactions)
	}
	if got.RuntimeIncludedTPS != 15 {
		t.Fatalf("runtime tps = %v, want 15", got.RuntimeIncludedTPS)
	}
	if got.WallIncludedTPS != 1.5 {
		t.Fatalf("wall included tps = %v, want 1.5", got.WallIncludedTPS)
	}
	if got.LoadPhaseWallIncludedTPS != 7.5 {
		t.Fatalf("load phase included tps = %v, want 7.5", got.LoadPhaseWallIncludedTPS)
	}
	if got.LoadWindowIncludedTPS != 8 {
		t.Fatalf("load window included tps = %v, want 8", got.LoadWindowIncludedTPS)
	}
	if got.EffectiveOperations != 75000 {
		t.Fatalf("effective ops = %d, want 75000", got.EffectiveOperations)
	}
	if got.RuntimeEffectiveOperationsPerSec != 7500 {
		t.Fatalf("runtime effective ops/s = %v, want 7500", got.RuntimeEffectiveOperationsPerSec)
	}
	if got.LoadPhaseEffectiveOperationsPerSec != 3750 {
		t.Fatalf("load phase effective ops/s = %v, want 3750", got.LoadPhaseEffectiveOperationsPerSec)
	}
	if got.LoadWindowEffectiveOperationsPerSec != 4000 {
		t.Fatalf("load window effective ops/s = %v, want 4000", got.LoadWindowEffectiveOperationsPerSec)
	}
}

func TestDeriveMetricsSkipsShortLoadWindow(t *testing.T) {
	sc := scenario{
		LoadTestSpec: ctlt.LoadTestSpec{
			NumOfBlocks: 1,
			NumOfTxs:    1000,
			Msgs: []ctlt.LoadTestMsg{{
				Type:    ctlt.MsgType("MsgSend"),
				NumMsgs: 1,
			}},
		},
	}
	result := runResult{
		WallSeconds: 5,
		CorrectedLoadTest: &correctedLoadTest{
			Source:                 "app_metrics",
			TotalTransactions:      1000,
			IncludedTransactions:   1000,
			SuccessfulTransactions: 1000,
			RuntimeSeconds:         1,
			TPS:                    1000,
		},
		LoadWindow: &loadWindowObservation{
			TargetTransactions:     1000,
			MinimumSeconds:         120,
			DurationSatisfied:      false,
			Reached:                true,
			Seconds:                1,
			IncludedTransactions:   1000,
			SuccessfulTransactions: 1000,
		},
	}
	got := deriveMetrics(sc, result)
	if got.LoadWindowIncludedTPS != 0 {
		t.Fatalf("short load-window included tps = %v, want 0", got.LoadWindowIncludedTPS)
	}
	if got.LoadWindowEffectiveOperationsPerSec != 0 {
		t.Fatalf("short load-window effective ops/s = %v, want 0", got.LoadWindowEffectiveOperationsPerSec)
	}
}

func TestLoadWindowAcceptedAllowsZeroMinimum(t *testing.T) {
	if !loadWindowAccepted(&loadWindowObservation{Reached: true}) {
		t.Fatalf("zero-minimum reached load window should be accepted")
	}
	if loadWindowAccepted(&loadWindowObservation{Reached: true, MinimumSeconds: 120}) {
		t.Fatalf("duration-unsatisfied load window should not be accepted")
	}
	if !loadWindowAccepted(&loadWindowObservation{Reached: true, MinimumSeconds: 120, DurationSatisfied: true}) {
		t.Fatalf("duration-satisfied load window should be accepted")
	}
}

func TestLoadWindowTargetTransactionsUsesCeilingAndClamps(t *testing.T) {
	tests := []struct {
		name     string
		intended int
		fraction float64
		want     int
	}{
		{name: "strict", intended: 50000, fraction: 1, want: 50000},
		{name: "fraction", intended: 50000, fraction: 0.995, want: 49750},
		{name: "ceiling", intended: 101, fraction: 0.995, want: 101},
		{name: "minimum one", intended: 1, fraction: 0.5, want: 1},
		{name: "zero intended", intended: 0, fraction: 0.995, want: 0},
		{name: "invalid low fraction", intended: 50000, fraction: 0, want: 50000},
		{name: "invalid high fraction", intended: 50000, fraction: 1.1, want: 50000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := loadWindowTargetTransactions(tt.intended, tt.fraction); got != tt.want {
				t.Fatalf("loadWindowTargetTransactions(%d, %v) = %d, want %d", tt.intended, tt.fraction, got, tt.want)
			}
		})
	}
}

func TestLoadWindowMonitorWaitReturnsReachedObservation(t *testing.T) {
	done := make(chan loadWindowObservation, 1)
	done <- loadWindowObservation{
		TargetTransactions:     10,
		Reached:                true,
		Seconds:                2,
		IncludedTransactions:   10,
		SuccessfulTransactions: 10,
	}
	monitor := &loadWindowMonitor{cancel: func() {}, done: done}
	got := monitor.Wait(time.Minute)
	if !got.Reached || got.SuccessfulTransactions != 10 {
		t.Fatalf("wait observation = %+v, want reached with 10 successes", got)
	}
}

func TestLoadWindowMonitorWaitAnnotatesTimeout(t *testing.T) {
	done := make(chan loadWindowObservation, 1)
	canceled := false
	monitor := &loadWindowMonitor{
		cancel: func() {
			canceled = true
			done <- loadWindowObservation{
				TargetTransactions:     10,
				IncludedTransactions:   3,
				SuccessfulTransactions: 3,
			}
		},
		done: done,
	}
	got := monitor.Wait(time.Nanosecond)
	if !canceled {
		t.Fatalf("monitor was not canceled on timeout")
	}
	if got.Reached {
		t.Fatalf("wait observation reached unexpectedly: %+v", got)
	}
	if !strings.Contains(got.Error, "load window target not reached") {
		t.Fatalf("timeout error = %q, want load-window target message", got.Error)
	}
}

func TestSimappScenarioUsesLowLatencyConfig(t *testing.T) {
	sc := simappScenario("simapp-treedb", "test", "treedb", 1, 0, 100, preseedConfig{Profile: "none", GenesisAccounts: 100}, 3, 50, "MsgSend", "MsgSend", 1, 1, 1000000, "")
	consensus, ok := sc.CustomConfig["consensus"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing consensus config: %#v", sc.CustomConfig)
	}
	if consensus["timeout_commit"] != "0ms" {
		t.Fatalf("timeout_commit = %#v, want 0ms", consensus["timeout_commit"])
	}
	mempool, ok := sc.CustomConfig["mempool"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing mempool config: %#v", sc.CustomConfig)
	}
	if mempool["size"] != 1000000 {
		t.Fatalf("mempool size = %#v, want 1000000", mempool["size"])
	}
}

func TestRuntimeBreakdownJSONIncludesZeroCategories(t *testing.T) {
	payload, err := json.Marshal(runtimeBreakdown{Name: "validator-0"})
	if err != nil {
		t.Fatalf("marshal runtime breakdown: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal runtime breakdown: %v", err)
	}
	for _, key := range []string{
		"wall_seconds",
		"workload_runtime_seconds",
		"abci_commit_seconds",
		"abci_other_seconds",
		"non_abci_workload_seconds",
		"max_runtime_speedup_if_commit_free",
	} {
		if _, ok := got[key]; !ok {
			t.Fatalf("runtime breakdown JSON missing %q in %s", key, payload)
		}
	}
}

func TestPrepareCelestiaEnvFilesGeneratesControlAndCandidate(t *testing.T) {
	tmp := t.TempDir()
	cfg := celestiaSyncConfig{
		OutputDir:                      tmp,
		RunHomeBase:                    filepath.Join(tmp, "homes"),
		GoCacheRoot:                    filepath.Join(tmp, "go-cache"),
		TempDir:                        filepath.Join(tmp, "tmp"),
		CelestiaAppDir:                 "/repo/celestia-app",
		GomapDir:                       "/repo/gomap",
		CosmosDBDir:                    "/repo/cosmos-db",
		CometDBDir:                     "/repo/cometbft-db",
		CosmosStoreDir:                 "/repo/store",
		CosmosLogDir:                   "/repo/log",
		CosmosCoreDir:                  "/repo/core",
		IAVLDir:                        "/repo/iavl",
		TreeDBOpenProfile:              "command_wal_durable",
		PostSyncDwellSeconds:           300,
		RequiredAcceptedSnapshotHeight: "11758500",
		TrustHeight:                    "11756500",
		TrustHash:                      "ABCDEF",
		StopAtLocalHeight:              "11760500",
		FreezeRemoteHeightAtStart:      true,
		UseLocalTreeStack:              true,
		UseLocalCosmosStore:            true,
	}
	control, candidate, err := prepareCelestiaEnvFiles(cfg)
	if err != nil {
		t.Fatalf("prepare env files: %v", err)
	}
	controlBody, err := os.ReadFile(control)
	if err != nil {
		t.Fatalf("read control env: %v", err)
	}
	candidateBody, err := os.ReadFile(candidate)
	if err != nil {
		t.Fatalf("read candidate env: %v", err)
	}
	if !strings.Contains(string(controlBody), "APP_DB_BACKEND='goleveldb'") {
		t.Fatalf("control env did not select goleveldb:\n%s", controlBody)
	}
	if strings.Contains(string(controlBody), "TREEDB_OPEN_PROFILE") {
		t.Fatalf("control env should not set TreeDB profile:\n%s", controlBody)
	}
	for _, want := range []string{
		"RUN_HOME_GLOB='" + filepath.Join(tmp, "homes", ".celestia-app-mainnet-goleveldb-*") + "'",
		"P2P_PORT='36656'",
		"RPC_PORT='36657'",
		"RPC_GRPC_PORT='36658'",
		"PRIV_VALIDATOR_GRPC_PORT='36659'",
		"PROMETHEUS_PORT='36660'",
		"API_PORT='36317'",
		"APP_GRPC_PORT='39090'",
		"PPROF_LADDR='localhost:6062'",
		"CELESTIA_APPD_BIN='" + filepath.Join("/repo/celestia-app", "build", "celestia-appd-goleveldb") + "'",
	} {
		if !strings.Contains(string(controlBody), want) {
			t.Fatalf("control env missing %q:\n%s", want, controlBody)
		}
	}
	for _, want := range []string{
		"APP_DB_BACKEND='treedb'",
		"TREEDB_OPEN_PROFILE='command_wal_durable'",
		"HOME='" + filepath.Join(tmp, "homes") + "'",
		"RUN_HOME_GLOB='" + filepath.Join(tmp, "homes", ".celestia-app-mainnet-treedb-*") + "'",
		"P2P_PORT='37656'",
		"RPC_PORT='37657'",
		"RPC_GRPC_PORT='37658'",
		"PRIV_VALIDATOR_GRPC_PORT='37659'",
		"PROMETHEUS_PORT='37660'",
		"API_PORT='37317'",
		"APP_GRPC_PORT='39091'",
		"PPROF_LADDR='localhost:6162'",
		"CELESTIA_APPD_BIN='" + filepath.Join("/repo/celestia-app", "build", "celestia-appd-treedb") + "'",
		"REQUIRED_ACCEPTED_SNAPSHOT_HEIGHT='11758500'",
		"STOP_AT_LOCAL_HEIGHT='11760500'",
		"TRUST_HEIGHT='11756500'",
	} {
		if !strings.Contains(string(candidateBody), want) {
			t.Fatalf("candidate env missing %q:\n%s", want, candidateBody)
		}
	}
	if got := celestiaABEnvironment(cfg)["STOP_PAIR_ON_FIRST_INVALID"]; got != "1" {
		t.Fatalf("STOP_PAIR_ON_FIRST_INVALID = %q, want 1", got)
	}
	if got := celestiaABEnvironment(cfg)["PAIR_RUN_MODE"]; got != "concurrent" {
		t.Fatalf("PAIR_RUN_MODE = %q, want concurrent", got)
	}
}

func TestCollectCelestiaArtifactsParsesSummaryDecisionAndRuns(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "summary.md"), []byte("# run_celestia A/B summary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "decision.json"), []byte(`{"reason":"max_pairs","stop":true,"comparable_pairs":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "runs.csv"), []byte("pair_index,variant,t_sync_seconds\n1,control,692\n1,candidate,307\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "pairs.csv"), []byte("pair_index,outcome,delta_t_sync_seconds\n1,win,-385\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(tmp, "runs", "01_candidate")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runHome := filepath.Join(tmp, "homes", "treedb")
	syncDir := filepath.Join(runHome, "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatal(err)
	}
	syncTime := strings.Join([]string{
		"start_utc=2026-07-02T07:31:45Z",
		"db_backend=treedb",
		"app_db_backend=treedb",
		"trust_height=11756500",
		"trust_hash=ABCDEF",
		"required_accepted_snapshot_height=11758500",
		"accepted_snapshot_height=11787000",
		"accepted_snapshot_source=statesync_chunk",
		"accepted_snapshot_format=3",
		"accepted_snapshot_hash=unknown",
		"accepted_snapshot_observed_utc=2026-07-02T07:31:22Z",
		"accepted_snapshot_mismatch=true",
		"accepted_snapshot_required_height=11758500",
		"accepted_snapshot_actual_height=11787000",
	}, "\n")
	if err := os.WriteFile(filepath.Join(syncDir, "sync-time.log"), []byte(syncTime), 0o644); err != nil {
		t.Fatal(err)
	}
	runJSON := `{
  "pair_index": 1,
  "variant": "candidate",
  "run_home": "` + runHome + `",
  "status": {"valid": true, "run_exit_code": 0, "sync_time_present": true},
  "sync": {
    "duration_seconds": 307,
    "max_rss_kb": 6403100,
    "max_hwm_kb": 6417236,
    "trust_height": 11756500,
    "stop_at_local_height": 11760500,
    "final_local_height": 11760500,
    "blocks_synced": 4000,
    "end_app_bytes": 2315880319,
    "end_home_bytes": 4000000000
  },
  "sizes": {"sync_app_bytes": 2315880319, "post_wal_bytes": 1024},
  "metrics": {
    "t_sync_seconds": 307,
    "t_total_seconds": 614,
    "s_sync_app_bytes": 2315880319,
    "max_rss_kb": 6403100,
    "blocks_synced": 4000,
    "t_sync_seconds_per_block": 0.07675
  },
  "maintenance_summary_source": "diagnostics_json",
  "maintenance_summary_is_live_runtime": true,
  "maintenance_summary": {"rewrite_queued_debt_exec_runs": 2}
}`
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte(runJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	result := &celestiaSyncResult{OutputDir: tmp}
	collectCelestiaArtifacts(result)
	if result.Decision["reason"] != "max_pairs" {
		t.Fatalf("decision reason = %v, want max_pairs", result.Decision["reason"])
	}
	if !strings.Contains(result.SummaryMarkdown, "run_celestia") {
		t.Fatalf("summary markdown not loaded: %q", result.SummaryMarkdown)
	}
	if len(result.RunsCSV) != 2 || result.RunsCSV[1]["variant"] != "candidate" {
		t.Fatalf("runs csv = %+v", result.RunsCSV)
	}
	if len(result.PairsCSV) != 1 || result.PairsCSV[0]["outcome"] != "win" {
		t.Fatalf("pairs csv = %+v", result.PairsCSV)
	}
	if len(result.Runs) != 1 {
		t.Fatalf("runs len = %d, want 1", len(result.Runs))
	}
	run := result.Runs[0]
	if run.Sync.DurationSeconds != 307 || run.Sync.MaxRSSKB != 6403100 || run.Metrics.TTotalSeconds != 614 {
		t.Fatalf("run summary = %+v", run)
	}
	if run.SyncTime["trust_hash"] != "ABCDEF" {
		t.Fatalf("sync-time trust_hash = %q, want ABCDEF", run.SyncTime["trust_hash"])
	}
	if run.Sync.DBBackend != "treedb" || run.Sync.AcceptedSnapshotHeight != 11787000 || !run.Sync.AcceptedSnapshotMismatch {
		t.Fatalf("sync-time fields were not attached: %+v", run.Sync)
	}
	if got := run.MaintenanceSummary["rewrite_queued_debt_exec_runs"]; got != float64(2) {
		t.Fatalf("maintenance summary rewrite runs = %v, want 2", got)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
