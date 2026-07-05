package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ctlt "github.com/skip-mev/catalyst/chains/types"
)

func TestParsePrometheusMetricsKeepsSelectedSeries(t *testing.T) {
	text := `
# HELP cometbft_abci_connection_method_timing_seconds Timing for each ABCI method.
cometbft_abci_connection_method_timing_seconds_sum{chain_id="sgldb",method="commit",type="sync"} 1.25
cometbft_abci_connection_method_timing_seconds_count{chain_id="sgldb",method="commit",type="sync"} 5
cometbft_consensus_height{chain_id="sgldb"} 10
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
	if _, ok := metrics[`cometbft_consensus_height{chain_id="sgldb"}`]; ok {
		t.Fatalf("unselected consensus height metric was retained")
	}
	if _, ok := metrics["some_unrelated_metric"]; ok {
		t.Fatalf("unrelated metric was retained")
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

func TestSimappGomapPinOverride(t *testing.T) {
	t.Setenv("IRONBIRD_SIMAPP_GOMAP_VERSION", "v0.6.2-0.20260705113501-82affd8d5b0e")
	t.Setenv("IRONBIRD_SIMAPP_GOMAP_REF", "82affd8d5b0ecf73447203461dbddfaa939d2998")
	t.Setenv("IRONBIRD_SIMAPP_GOMAP_IMAGE_SLUG", "audit-revertboth")

	sc := simappFullStackScenario("simapp-treedb-all", "full stack TreeDB", "treedb", 1, 0, 100, preseedConfig{}, 2, 10, "MsgSend", "", 0, 0, 1000000, "")
	if !strings.Contains(sc.ReplaceCmd, "github.com/snissn/gomap=github.com/snissn/gomap@v0.6.2-0.20260705113501-82affd8d5b0e") {
		t.Fatalf("replace command missing gomap override: %s", sc.ReplaceCmd)
	}
	if sc.DependencyPins[1].Ref != "82affd8d5b0ecf73447203461dbddfaa939d2998" {
		t.Fatalf("gomap ref = %q, want override", sc.DependencyPins[1].Ref)
	}
	if !strings.HasSuffix(sc.ImageTag, "gomap-audit-revertboth") {
		t.Fatalf("image tag = %q, want gomap override slug", sc.ImageTag)
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
