package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	ctlteth "github.com/skip-mev/catalyst/chains/ethereum/types"
	ctlt "github.com/skip-mev/catalyst/chains/types"
	"github.com/skip-mev/ironbird/activities/loadtest"
	"github.com/skip-mev/ironbird/activities/testnet"
	"github.com/skip-mev/ironbird/messages"
	coreutil "github.com/skip-mev/ironbird/petri/core/util"
	"github.com/skip-mev/ironbird/petri/cosmos/chain"
	"github.com/skip-mev/ironbird/petri/cosmos/node"
	"github.com/skip-mev/ironbird/types"
	"github.com/skip-mev/ironbird/util"
	"go.uber.org/zap"
)

const defaultMnemonic = "copper push brief egg scan entry inform record adjust fossil boss egg comic alien upon aspect dry avoid interest fury window hint race symptom"

const (
	profileBoundaryAcceptedWindow     = "accepted-window"
	profileBoundaryLoadWindowAdjacent = "load-window-adjacent"
	profileBoundaryWholeRun           = "whole-run"
)

type scenario struct {
	Name              string                 `json:"name"`
	Runner            string                 `json:"runner,omitempty"`
	IncludeInAll      bool                   `json:"-"`
	ChainName         string                 `json:"chain_name"`
	Description       string                 `json:"description"`
	Repo              string                 `json:"repo"`
	ChainSource       string                 `json:"chain_source"`
	ChainRef          string                 `json:"chain_ref"`
	Dockerfile        string                 `json:"dockerfile"`
	ReplaceCmd        string                 `json:"replace_cmd,omitempty"`
	BaseImage         string                 `json:"base_image"`
	ImageTag          string                 `json:"image_tag"`
	DependencyPins    []dependencyPin        `json:"dependency_pins,omitempty"`
	Preseed           preseedConfig          `json:"preseed,omitempty"`
	IsEVMChain        bool                   `json:"is_evm_chain"`
	NumValidators     uint64                 `json:"num_validators"`
	NumNodes          uint64                 `json:"num_nodes"`
	NumWallets        int                    `json:"num_wallets"`
	CatalystVersion   string                 `json:"catalyst_version,omitempty"`
	AppDBBackend      string                 `json:"app_db_backend,omitempty"`
	NodeDBBackend     string                 `json:"node_db_backend,omitempty"`
	Genesis           []chain.GenesisKV      `json:"genesis"`
	CustomAppConfig   map[string]interface{} `json:"custom_app_config,omitempty"`
	CustomConfig      map[string]interface{} `json:"custom_consensus_config,omitempty"`
	CustomClient      map[string]interface{} `json:"custom_client_config,omitempty"`
	AdditionalFlags   []string               `json:"additional_start_flags,omitempty"`
	AppCPUProfile     string                 `json:"app_cpu_profile,omitempty"`
	AppHeapProfile    string                 `json:"app_heap_profile,omitempty"`
	AppPprofListen    string                 `json:"app_pprof_listen,omitempty"`
	CelestiaSync      celestiaSyncConfig     `json:"celestia_sync,omitempty"`
	LoadTestSpec      ctlt.LoadTestSpec      `json:"load_test_spec"`
	GenesisBalance    *big.Int               `json:"-"`
	GenesisDelegation *big.Int               `json:"-"`
}

type dependencyPin struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Ref     string `json:"ref,omitempty"`
}

type preseedConfig struct {
	Profile             string `json:"profile,omitempty"`
	Accounts            int    `json:"accounts,omitempty"`
	ActiveWallets       int    `json:"active_wallets,omitempty"`
	GenesisAccounts     int    `json:"genesis_accounts,omitempty"`
	DeterministicSource string `json:"deterministic_source,omitempty"`
}

type runResult struct {
	Scenario            scenario               `json:"scenario"`
	StartedAt           time.Time              `json:"started_at"`
	FinishedAt          time.Time              `json:"finished_at"`
	WallSeconds         float64                `json:"wall_seconds"`
	LaunchSeconds       float64                `json:"launch_seconds,omitempty"`
	PhaseTimeline       []phaseSpan            `json:"phase_timeline,omitempty"`
	ImageBuildCommand   []string               `json:"image_build_command,omitempty"`
	ImageBuildLog       string                 `json:"image_build_log,omitempty"`
	ProviderName        string                 `json:"provider_name"`
	ContainerStats      []json.RawMessage      `json:"container_stats,omitempty"`
	ResourceSamples     []resourceSample       `json:"resource_samples,omitempty"`
	ResourceSummary     []resourceSummary      `json:"resource_summary,omitempty"`
	MetricsBefore       []metricSnapshot       `json:"metrics_before,omitempty"`
	MetricsAfter        []metricSnapshot       `json:"metrics_after,omitempty"`
	BackendVerification *backendVerification   `json:"backend_verification,omitempty"`
	DataSizesBefore     []dataPathSize         `json:"data_sizes_before,omitempty"`
	DataSizesAfter      []dataPathSize         `json:"data_sizes_after,omitempty"`
	StorageSignals      []storageSignal        `json:"storage_signal_summary,omitempty"`
	LoadWindow          *loadWindowObservation `json:"load_window,omitempty"`
	RuntimeBreakdown    []runtimeBreakdown     `json:"runtime_breakdown,omitempty"`
	ContainerLogs       map[string]string      `json:"container_logs,omitempty"`
	LoadTestResult      ctlt.LoadTestResult    `json:"load_test_result"`
	LoadTestConfig      string                 `json:"load_test_config,omitempty"`
	LoadTestLogs        string                 `json:"load_test_logs,omitempty"`
	LoadTestStopped     string                 `json:"load_test_stopped,omitempty"`
	LoadTestLogSummary  loadTestLogSummary     `json:"load_test_log_summary,omitempty"`
	CorrectedLoadTest   *correctedLoadTest     `json:"corrected_load_test,omitempty"`
	CommitBenchmark     *commitBenchmark       `json:"commit_benchmark,omitempty"`
	ProfileArtifacts    []profileArtifact      `json:"profile_artifacts,omitempty"`
	Derived             derivedMetrics         `json:"derived_metrics,omitempty"`
	RawTxAudit          []txAudit              `json:"raw_tx_audit,omitempty"`
	RawTxSummary        *txAuditSummary        `json:"raw_tx_summary,omitempty"`
	RawTxAuditSkipped   string                 `json:"raw_tx_audit_skipped,omitempty"`
	CelestiaSync        *celestiaSyncResult    `json:"celestia_sync,omitempty"`
	Error               string                 `json:"error,omitempty"`
}

type phaseSpan struct {
	Name    string    `json:"name"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Seconds float64   `json:"seconds"`
	Note    string    `json:"note,omitempty"`
}

type backendVerification struct {
	ExpectedAppDBBackend  string `json:"expected_app_db_backend,omitempty"`
	ExpectedNodeDBBackend string `json:"expected_node_db_backend,omitempty"`
	ObservedAppDBBackend  string `json:"observed_app_db_backend,omitempty"`
	ObservedNodeDBBackend string `json:"observed_node_db_backend,omitempty"`
	AppConfigPath         string `json:"app_config_path,omitempty"`
	NodeConfigPath        string `json:"node_config_path,omitempty"`
	Valid                 bool   `json:"valid"`
	Error                 string `json:"error,omitempty"`
}

type resourceSample struct {
	At    time.Time         `json:"at"`
	Stats []json.RawMessage `json:"stats"`
}

type resourceSummary struct {
	Name               string  `json:"name"`
	Samples            int     `json:"samples"`
	MaxCPUPerc         float64 `json:"max_cpu_percent,omitempty"`
	MaxMemUsageBytes   uint64  `json:"max_mem_usage_bytes,omitempty"`
	MaxMemUsage        string  `json:"max_mem_usage,omitempty"`
	MaxMemPerc         float64 `json:"max_mem_percent,omitempty"`
	LastNetIO          string  `json:"last_net_io,omitempty"`
	LastBlockIO        string  `json:"last_block_io,omitempty"`
	MaxBlockReadBytes  uint64  `json:"max_block_read_bytes,omitempty"`
	MaxBlockWriteBytes uint64  `json:"max_block_write_bytes,omitempty"`
}

type metricSnapshot struct {
	Name    string             `json:"name"`
	URL     string             `json:"url,omitempty"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
	Error   string             `json:"error,omitempty"`
}

type metricDeltaSnapshot struct {
	Name    string                      `json:"name"`
	URL     string                      `json:"url,omitempty"`
	Metrics map[string]metricDeltaValue `json:"metrics,omitempty"`
	Error   string                      `json:"error,omitempty"`
}

type metricDeltaValue struct {
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Delta  float64 `json:"delta"`
}

type dataPathSize struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes uint64 `json:"bytes,omitempty"`
	Error string `json:"error,omitempty"`
}

type storageSignal struct {
	Name                            string         `json:"name"`
	ABCIObservedSeconds             float64        `json:"abci_observed_seconds,omitempty"`
	ABCICommitSeconds               float64        `json:"abci_commit_seconds,omitempty"`
	ABCIFinalizeBlockSeconds        float64        `json:"abci_finalize_block_seconds,omitempty"`
	ABCICheckTxSeconds              float64        `json:"abci_check_tx_seconds,omitempty"`
	ABCIPrepareProposalSeconds      float64        `json:"abci_prepare_proposal_seconds,omitempty"`
	ABCIProcessProposalSeconds      float64        `json:"abci_process_proposal_seconds,omitempty"`
	ABCIQuerySeconds                float64        `json:"abci_query_seconds,omitempty"`
	ABCIFlushSeconds                float64        `json:"abci_flush_seconds,omitempty"`
	ABCIOtherSeconds                float64        `json:"abci_other_seconds,omitempty"`
	ABCICommitCount                 int            `json:"abci_commit_count,omitempty"`
	ABCIFinalizeBlockCount          int            `json:"abci_finalize_block_count,omitempty"`
	ABCICheckTxCount                int            `json:"abci_check_tx_count,omitempty"`
	ABCIPrepareProposalCount        int            `json:"abci_prepare_proposal_count,omitempty"`
	ABCIProcessProposalCount        int            `json:"abci_process_proposal_count,omitempty"`
	ABCIQueryCount                  int            `json:"abci_query_count,omitempty"`
	ABCIFlushCount                  int            `json:"abci_flush_count,omitempty"`
	AvgCommitSeconds                float64        `json:"avg_commit_seconds,omitempty"`
	AvgFinalizeBlockSeconds         float64        `json:"avg_finalize_block_seconds,omitempty"`
	CommitShareOfObservedABCI       float64        `json:"commit_share_of_observed_abci,omitempty"`
	CommitShareOfCommitPlusFinalize float64        `json:"commit_share_of_commit_plus_finalize,omitempty"`
	StateBlockProcessingSumRaw      float64        `json:"state_block_processing_sum_raw,omitempty"`
	StateBlockProcessingCount       int            `json:"state_block_processing_count,omitempty"`
	ConsensusBlockIntervalSeconds   float64        `json:"consensus_block_interval_seconds,omitempty"`
	ConsensusBlockIntervalCount     int            `json:"consensus_block_interval_count,omitempty"`
	ConsensusTotalTxsDelta          float64        `json:"consensus_total_txs_delta,omitempty"`
	MempoolSuccessfulTxsDelta       float64        `json:"mempool_successful_txs_delta,omitempty"`
	SDKTxCountDelta                 float64        `json:"sdk_tx_count_delta,omitempty"`
	SDKTxSuccessfulDelta            float64        `json:"sdk_tx_successful_delta,omitempty"`
	ProcessCPUSecondsDelta          float64        `json:"process_cpu_seconds_delta,omitempty"`
	DataDirBytesBefore              uint64         `json:"data_dir_bytes_before,omitempty"`
	DataDirBytesAfter               uint64         `json:"data_dir_bytes_after,omitempty"`
	DataDirBytesDelta               int64          `json:"data_dir_bytes_delta,omitempty"`
	ApplicationDBBytesBefore        uint64         `json:"application_db_bytes_before,omitempty"`
	ApplicationDBBytesAfter         uint64         `json:"application_db_bytes_after,omitempty"`
	ApplicationDBBytesDelta         int64          `json:"application_db_bytes_delta,omitempty"`
	ModuleTimings                   []moduleTiming `json:"module_timings,omitempty"`
}

type loadWindowObservation struct {
	TargetTransactions     int                   `json:"target_transactions,omitempty"`
	MinimumSeconds         float64               `json:"minimum_seconds,omitempty"`
	DurationSatisfied      bool                  `json:"duration_satisfied"`
	Reached                bool                  `json:"reached,omitempty"`
	StartedAt              time.Time             `json:"started_at,omitempty"`
	EndedAt                time.Time             `json:"ended_at,omitempty"`
	Seconds                float64               `json:"seconds,omitempty"`
	IncludedTransactions   int                   `json:"included_transactions,omitempty"`
	SuccessfulTransactions int                   `json:"successful_transactions,omitempty"`
	AppMetricCandidates    []string              `json:"app_metric_candidates,omitempty"`
	MetricsBefore          []metricSnapshot      `json:"metrics_before,omitempty"`
	MetricsAfter           []metricSnapshot      `json:"metrics_after,omitempty"`
	MetricDeltas           []metricDeltaSnapshot `json:"metric_deltas,omitempty"`
	StorageSignals         []storageSignal       `json:"storage_signal_summary,omitempty"`
	Error                  string                `json:"error,omitempty"`
}

type moduleTiming struct {
	Phase      string  `json:"phase"`
	Module     string  `json:"module"`
	Seconds    float64 `json:"seconds"`
	Count      int     `json:"count,omitempty"`
	AvgSeconds float64 `json:"avg_seconds,omitempty"`
}

type runtimeBreakdown struct {
	Name                            string  `json:"name"`
	WallSeconds                     float64 `json:"wall_seconds"`
	LaunchSeconds                   float64 `json:"launch_seconds"`
	WorkloadRuntimeSeconds          float64 `json:"workload_runtime_seconds"`
	PostLaunchNonWorkloadSeconds    float64 `json:"post_launch_non_workload_seconds"`
	ABCIObservedSeconds             float64 `json:"abci_observed_seconds"`
	ABCICommitSeconds               float64 `json:"abci_commit_seconds"`
	ABCIFinalizeBlockSeconds        float64 `json:"abci_finalize_block_seconds"`
	ABCICheckTxSeconds              float64 `json:"abci_check_tx_seconds"`
	ABCIProposalSeconds             float64 `json:"abci_proposal_seconds"`
	ABCIQuerySeconds                float64 `json:"abci_query_seconds"`
	ABCIFlushSeconds                float64 `json:"abci_flush_seconds"`
	ABCIOtherSeconds                float64 `json:"abci_other_seconds"`
	NonABCIWorkloadSeconds          float64 `json:"non_abci_workload_seconds"`
	CommitPctOfWorkload             float64 `json:"commit_pct_of_workload"`
	FinalizeBlockPctOfWorkload      float64 `json:"finalize_block_pct_of_workload"`
	CommitPlusFinalizePctOfWorkload float64 `json:"commit_plus_finalize_pct_of_workload"`
	ObservedABCIPctOfWorkload       float64 `json:"observed_abci_pct_of_workload"`
	NonABCIPctOfWorkload            float64 `json:"non_abci_pct_of_workload"`
	MaxRuntimeSpeedupIfCommitFree   float64 `json:"max_runtime_speedup_if_commit_free"`
	MaxRuntimeSpeedupIfCommitHalf   float64 `json:"max_runtime_speedup_if_commit_half"`
}

type loadTestLogSummary struct {
	SendingEvents            int               `json:"sending_events,omitempty"`
	SendingTxsTotal          int               `json:"sending_txs_total,omitempty"`
	BuiltBatchEvents         int               `json:"built_batch_events,omitempty"`
	FailedSendTotal          int               `json:"failed_send_total,omitempty"`
	FailedSendErrors         []logErrorCount   `json:"failed_send_errors,omitempty"`
	GoRoutinesCompletedTotal int               `json:"go_routines_completed_total,omitempty"`
	CollectorTxs             int               `json:"collector_txs,omitempty"`
	CollectorStartingBlock   uint64            `json:"collector_starting_block,omitempty"`
	CollectorEndingBlock     uint64            `json:"collector_ending_block,omitempty"`
	RunnerOverall            *runnerLogOverall `json:"runner_overall,omitempty"`
}

type logErrorCount struct {
	Error string `json:"error"`
	Count int    `json:"count"`
}

type runnerLogOverall struct {
	TotalTransactions         int     `json:"total_transactions,omitempty"`
	TotalIncludedTransactions int     `json:"total_included_transactions,omitempty"`
	SuccessfulTransactions    int     `json:"successful_transactions,omitempty"`
	FailedTransactions        int     `json:"failed_transactions,omitempty"`
	TPS                       float64 `json:"tps,omitempty"`
}

type correctedLoadTest struct {
	Source                       string   `json:"source,omitempty"`
	TotalTransactions            int      `json:"total_transactions,omitempty"`
	IncludedTransactions         int      `json:"included_transactions,omitempty"`
	SuccessfulTransactions       int      `json:"successful_transactions,omitempty"`
	FailedTransactions           int      `json:"failed_transactions,omitempty"`
	TotalGasUsed                 int64    `json:"total_gas_used,omitempty"`
	RuntimeSeconds               float64  `json:"runtime_seconds,omitempty"`
	TPS                          float64  `json:"tps,omitempty"`
	CatalystMismatch             bool     `json:"catalyst_mismatch,omitempty"`
	Notes                        []string `json:"notes,omitempty"`
	AppMetricsIncludedCandidates []string `json:"app_metrics_included_candidates,omitempty"`
}

type commitBenchmark struct {
	Blocks      uint64 `json:"blocks"`
	StartHeight uint64 `json:"start_height,omitempty"`
	EndHeight   uint64 `json:"end_height,omitempty"`
}

type profileArtifact struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Boundary        string `json:"timing_boundary,omitempty"`
	Path            string `json:"path,omitempty"`
	TopSummaryPath  string `json:"top_summary_path,omitempty"`
	TopSummaryError string `json:"top_summary_error,omitempty"`
	Error           string `json:"error,omitempty"`
}

type derivedMetrics struct {
	IntendedTransactions                int     `json:"intended_transactions,omitempty"`
	IncludedFraction                    float64 `json:"included_fraction,omitempty"`
	SuccessfulFraction                  float64 `json:"successful_fraction,omitempty"`
	RuntimeIncludedTPS                  float64 `json:"runtime_included_tps,omitempty"`
	WallIncludedTPS                     float64 `json:"wall_included_tps,omitempty"`
	LoadPhaseWallIncludedTPS            float64 `json:"load_phase_wall_included_tps,omitempty"`
	LoadWindowIncludedTPS               float64 `json:"load_window_included_tps,omitempty"`
	EffectiveOperations                 int     `json:"effective_operations,omitempty"`
	RuntimeEffectiveOperationsPerSec    float64 `json:"runtime_effective_operations_per_sec,omitempty"`
	WallEffectiveOperationsPerSec       float64 `json:"wall_effective_operations_per_sec,omitempty"`
	LoadPhaseEffectiveOperationsPerSec  float64 `json:"load_phase_effective_operations_per_sec,omitempty"`
	LoadWindowEffectiveOperationsPerSec float64 `json:"load_window_effective_operations_per_sec,omitempty"`
	EffectiveOperationNote              string  `json:"effective_operation_note,omitempty"`
}

type txAudit struct {
	Hash    string `json:"hash"`
	RPC     string `json:"rpc,omitempty"`
	Found   bool   `json:"found"`
	Height  int64  `json:"height,omitempty"`
	Code    uint32 `json:"code,omitempty"`
	GasUsed int64  `json:"gas_used,omitempty"`
	Error   string `json:"error,omitempty"`
}

type txAuditSummary struct {
	Queried      int     `json:"queried"`
	Found        int     `json:"found"`
	Successful   int     `json:"successful"`
	Failed       int     `json:"failed"`
	TotalGasUsed int64   `json:"total_gas_used"`
	TPS          float64 `json:"tps,omitempty"`
}

type celestiaSyncConfig struct {
	RunnerScript                   string `json:"runner_script,omitempty"`
	RunCommand                     string `json:"run_command,omitempty"`
	OutputDir                      string `json:"output_dir,omitempty"`
	ControlEnvFile                 string `json:"control_env_file,omitempty"`
	CandidateEnvFile               string `json:"candidate_env_file,omitempty"`
	MaxPairs                       int    `json:"max_pairs,omitempty"`
	PairRunMode                    string `json:"pair_run_mode,omitempty"`
	RunTimeoutSeconds              int    `json:"run_timeout_seconds,omitempty"`
	PostSyncDwellSeconds           int    `json:"post_sync_dwell_seconds,omitempty"`
	RequiredAcceptedSnapshotHeight string `json:"required_accepted_snapshot_height,omitempty"`
	TrustHeight                    string `json:"trust_height,omitempty"`
	TrustHash                      string `json:"trust_hash,omitempty"`
	StopAtLocalHeight              string `json:"stop_at_local_height,omitempty"`
	FreezeRemoteHeightAtStart      bool   `json:"freeze_remote_height_at_start,omitempty"`
	RunHomeBase                    string `json:"run_home_base,omitempty"`
	GoCacheRoot                    string `json:"go_cache_root,omitempty"`
	TempDir                        string `json:"temp_dir,omitempty"`
	CelestiaAppDir                 string `json:"celestia_app_dir,omitempty"`
	GomapDir                       string `json:"gomap_dir,omitempty"`
	CosmosDBDir                    string `json:"cosmos_db_dir,omitempty"`
	CometDBDir                     string `json:"comet_db_dir,omitempty"`
	CosmosStoreDir                 string `json:"cosmos_store_dir,omitempty"`
	CosmosLogDir                   string `json:"cosmos_log_dir,omitempty"`
	CosmosCoreDir                  string `json:"cosmos_core_dir,omitempty"`
	IAVLDir                        string `json:"iavl_dir,omitempty"`
	TreeDBOpenProfile              string `json:"treedb_open_profile,omitempty"`
	UseLocalTreeStack              bool   `json:"use_local_tree_stack,omitempty"`
	UseLocalCosmosStore            bool   `json:"use_local_cosmos_store,omitempty"`
	UseLocalIAVL                   bool   `json:"use_local_iavl,omitempty"`
	DryRun                         bool   `json:"dry_run,omitempty"`
}

type celestiaSyncResult struct {
	Config           celestiaSyncConfig           `json:"config"`
	Command          []string                     `json:"command,omitempty"`
	Environment      map[string]string            `json:"environment,omitempty"`
	OutputDir        string                       `json:"output_dir,omitempty"`
	ControlEnvFile   string                       `json:"control_env_file,omitempty"`
	CandidateEnvFile string                       `json:"candidate_env_file,omitempty"`
	ExitCode         int                          `json:"exit_code,omitempty"`
	CommandLog       string                       `json:"command_log,omitempty"`
	SummaryMarkdown  string                       `json:"summary_markdown,omitempty"`
	Decision         map[string]interface{}       `json:"decision,omitempty"`
	RunsCSV          []map[string]string          `json:"runs_csv,omitempty"`
	PairsCSV         []map[string]string          `json:"pairs_csv,omitempty"`
	Runs             []celestiaABRunSummary       `json:"runs,omitempty"`
	Artifacts        map[string]string            `json:"artifacts,omitempty"`
	RepoGit          map[string]map[string]string `json:"repo_git,omitempty"`
}

type celestiaABRunSummary struct {
	PairIndex                       int                    `json:"pair_index"`
	Variant                         string                 `json:"variant"`
	RunHome                         string                 `json:"run_home,omitempty"`
	SyncTimePath                    string                 `json:"sync_time_path,omitempty"`
	SyncTime                        map[string]string      `json:"sync_time,omitempty"`
	Build                           map[string]string      `json:"build,omitempty"`
	Status                          celestiaABStatus       `json:"status,omitempty"`
	Sync                            celestiaABSync         `json:"sync,omitempty"`
	Rewrite                         celestiaABRewrite      `json:"rewrite,omitempty"`
	Sizes                           celestiaABSizes        `json:"sizes,omitempty"`
	Metrics                         celestiaABMetrics      `json:"metrics,omitempty"`
	MaintenanceSummarySource        string                 `json:"maintenance_summary_source,omitempty"`
	MaintenanceSummaryIsLiveRuntime bool                   `json:"maintenance_summary_is_live_runtime,omitempty"`
	MaintenanceSummary              map[string]interface{} `json:"maintenance_summary,omitempty"`
	Path                            string                 `json:"path,omitempty"`
	Error                           string                 `json:"error,omitempty"`
}

type celestiaABStatus struct {
	Valid             bool   `json:"valid"`
	InvalidReason     string `json:"invalid_reason,omitempty"`
	RunExitCode       int    `json:"run_exit_code,omitempty"`
	Attempt           int    `json:"attempt,omitempty"`
	MaxAttempts       int    `json:"max_attempts,omitempty"`
	RunTimeoutSeconds int    `json:"run_timeout_seconds,omitempty"`
	SyncTimePresent   bool   `json:"sync_time_present,omitempty"`
}

type celestiaABSync struct {
	StartUTC                       string  `json:"start_utc,omitempty"`
	EndUTC                         string  `json:"end_utc,omitempty"`
	DBBackend                      string  `json:"db_backend,omitempty"`
	AppDBBackend                   string  `json:"app_db_backend,omitempty"`
	DurationSeconds                float64 `json:"duration_seconds,omitempty"`
	MaxRSSKB                       int64   `json:"max_rss_kb,omitempty"`
	MaxHWMKB                       int64   `json:"max_hwm_kb,omitempty"`
	FreezeRemoteHeightAtStart      int64   `json:"freeze_remote_height_at_start,omitempty"`
	TrustHeight                    int64   `json:"trust_height,omitempty"`
	TrustHash                      string  `json:"trust_hash,omitempty"`
	StopAtLocalHeight              int64   `json:"stop_at_local_height,omitempty"`
	FinalLocalHeight               int64   `json:"final_local_height,omitempty"`
	FinalRemoteHeight              int64   `json:"final_remote_height,omitempty"`
	FinalRemoteHeightActual        int64   `json:"final_remote_height_actual,omitempty"`
	BlocksSynced                   int64   `json:"blocks_synced,omitempty"`
	RemoteMinusStopHeight          *int64  `json:"remote_minus_stop_height,omitempty"`
	EndAppBytes                    int64   `json:"end_app_bytes,omitempty"`
	EndDataBytes                   int64   `json:"end_data_bytes,omitempty"`
	EndHomeBytes                   int64   `json:"end_home_bytes,omitempty"`
	RequiredAcceptedSnapshotHeight int64   `json:"required_accepted_snapshot_height,omitempty"`
	AcceptedSnapshotHeight         int64   `json:"accepted_snapshot_height,omitempty"`
	AcceptedSnapshotActualHeight   int64   `json:"accepted_snapshot_actual_height,omitempty"`
	AcceptedSnapshotRequiredHeight int64   `json:"accepted_snapshot_required_height,omitempty"`
	AcceptedSnapshotFormat         int64   `json:"accepted_snapshot_format,omitempty"`
	AcceptedSnapshotSource         string  `json:"accepted_snapshot_source,omitempty"`
	AcceptedSnapshotHash           string  `json:"accepted_snapshot_hash,omitempty"`
	AcceptedSnapshotObservedUTC    string  `json:"accepted_snapshot_observed_utc,omitempty"`
	AcceptedSnapshotMismatch       bool    `json:"accepted_snapshot_mismatch,omitempty"`
}

type celestiaABRewrite struct {
	Attempted bool    `json:"attempted,omitempty"`
	Seconds   float64 `json:"seconds,omitempty"`
	ExitCode  int     `json:"exit_code,omitempty"`
}

type celestiaABSizes struct {
	SyncAppBytes   int64 `json:"sync_app_bytes,omitempty"`
	DuSyncAppBytes int64 `json:"du_sync_app_bytes,omitempty"`
	SyncWALBytes   int64 `json:"sync_wal_bytes,omitempty"`
	PostAppBytes   int64 `json:"post_app_bytes,omitempty"`
	PostWALBytes   int64 `json:"post_wal_bytes,omitempty"`
}

type celestiaABMetrics struct {
	TSyncSeconds          float64 `json:"t_sync_seconds,omitempty"`
	TRewriteSeconds       float64 `json:"t_rewrite_seconds,omitempty"`
	TTotalSeconds         float64 `json:"t_total_seconds,omitempty"`
	SSyncAppBytes         int64   `json:"s_sync_app_bytes,omitempty"`
	SDuSyncAppBytes       int64   `json:"s_du_sync_app_bytes,omitempty"`
	SSyncWALBytes         int64   `json:"s_sync_wal_bytes,omitempty"`
	SPostAppBytes         int64   `json:"s_post_app_bytes,omitempty"`
	SPostWALBytes         int64   `json:"s_post_wal_bytes,omitempty"`
	SSyncAppBytesPerBlock float64 `json:"s_sync_app_bytes_per_block,omitempty"`
	SPostAppBytesPerBlock float64 `json:"s_post_app_bytes_per_block,omitempty"`
	MaxRSSKB              int64   `json:"max_rss_kb,omitempty"`
	BlocksSynced          int64   `json:"blocks_synced,omitempty"`
	TSyncSecondsPerBlock  float64 `json:"t_sync_seconds_per_block,omitempty"`
	TTotalSecondsPerBlock float64 `json:"t_total_seconds_per_block,omitempty"`
}

type reportArtifact struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Host        map[string]string `json:"host"`
	Git         map[string]string `json:"git"`
	Flags       map[string]string `json:"flags"`
	Results     []runResult       `json:"results"`
	Errors      map[string]string `json:"errors,omitempty"`
}

func main() {
	defaultCelestiaAppDir := defaultExistingPath("/home/mikers/dev/snissn/celestia-app-3128-mainnet", "/home/mikers/dev/snissn/celestia-app-native-iavl-harness")
	var (
		scenarioFlag                = flag.String("scenario", "all", "scenario to run: all, all-with-celestia, evm-blog, simapp-goleveldb, simapp-treedb, simapp-goleveldb-all, simapp-treedb-all, celestia-sync-ab")
		outPath                     = flag.String("out", "", "JSON artifact path")
		markdownPath                = flag.String("markdown-out", "", "Markdown summary artifact path; defaults to the JSON artifact path with .md extension")
		keepTestnets                = flag.Bool("keep-testnets", false, "leave docker testnets running instead of tearing them down")
		skipBuild                   = flag.Bool("skip-build", false, "skip docker image builds and reuse existing tags")
		commitBenchBlocks           = flag.Uint64("commit-benchmark-blocks", 0, "when >0, skip Catalyst and measure empty/light-block ABCI commit cost for this many blocks")
		appCPUProfileDir            = flag.String("app-cpuprofile-dir", "", "when set, pass --cpu-profile to app validators and copy validator 0 CPU profiles into this directory")
		appHeapProfileDir           = flag.String("app-heapprofile-dir", "", "when set, enable validator pprof and copy validator 0 heap profiles into this directory")
		appPprofProfileDir          = flag.String("app-pprof-profile-dir", "", "when set, enable validator pprof and copy validator 0 heap, allocs, block, mutex, and goroutine profiles into this directory")
		rawTxAudit                  = flag.Bool("raw-tx-audit", true, "when true, verify non-EVM tx inclusion with post-load CometBFT /tx queries; disable for clean app CPU profiles")
		loadWindowDrainTimeout      = flag.Duration("load-window-drain-timeout", 0, "optional extra time to keep the chain running after Catalyst exits so app metrics can reach the load-window target")
		loadWindowMinDuration       = flag.Duration("load-window-min-duration", 0, "minimum app-metric load-window duration required for final throughput evidence; short reached windows are annotated invalid")
		loadWindowTargetFraction    = flag.Float64("load-window-target-fraction", 1.0, "fraction of intended transactions required before the app-metric load-window target can be marked reached")
		stopCatalystAfterLoadWindow = flag.Bool("stop-catalyst-after-load-window", false, "stop the Catalyst task once app metrics reach the load-window target, avoiding Catalyst post-load tx lookup")
		cpuprofile                  = flag.String("cpuprofile", "", "write local runner CPU profile to file")
		memprofile                  = flag.String("memprofile", "", "write local runner heap profile to file")
		blockprofile                = flag.String("blockprofile", "", "write local runner block profile to file")
		mutexprofile                = flag.String("mutexprofile", "", "write local runner mutex profile to file")
		evmBlocks                   = flag.Int("evm-blocks", 0, "EVM load-test blocks; when >0 uses block-cadence mode instead of interval batches")
		evmBatches                  = flag.Int("evm-batches", 30, "EVM load-test batches")
		evmMsgs                     = flag.Int("evm-msgs", 1000, "EVM messages per batch")
		evmMsgType                  = flag.String("evm-msg-type", "MsgNativeTransferERC20", "EVM Catalyst message type")
		evmSendInterval             = flag.Duration("evm-send-interval", time.Second, "EVM send interval between batches")
		evmInitialWallets           = flag.Int("evm-initial-wallets", 0, "EVM initial wallets for Catalyst bootstrapped distribution; 0 uses even distribution")
		evmInitialContracts         = flag.Uint64("evm-initial-contracts", 5, "EVM Loader/ERC20 contracts to deploy for Catalyst contract workloads")
		evmIterations               = flag.Int("evm-iterations", 0, "EVM message iterations for Catalyst message types that support it")
		evmCalldataSize             = flag.Int("evm-calldata-size", 0, "EVM calldata size for Catalyst message types that support it")
		evmGasFeeCap                = flag.Int64("evm-gas-fee-cap", 100_000_000_000, "EVM max fee per gas for Catalyst dynamic-fee transactions")
		evmGasTipCap                = flag.Int64("evm-gas-tip-cap", 100_000_000_000, "EVM max priority fee per gas for Catalyst dynamic-fee transactions")
		cosmosMsg                   = flag.String("cosmos-msg", "MsgSend", "Cosmos message type: MsgSend or MsgMultiSend")
		cosmosContained             = flag.String("cosmos-contained-msg", "MsgSend", "Cosmos contained message type when -cosmos-msg=MsgArr")
		cosmosMsgsPerTx             = flag.Int("cosmos-msgs-per-tx", 10, "Cosmos messages per transaction when -cosmos-msg=MsgArr")
		cosmosBlocks                = flag.Int("cosmos-blocks", 50, "Cosmos load-test blocks")
		cosmosTxs                   = flag.Int("cosmos-txs", 200, "Cosmos transactions per block")
		cosmosMaxGas                = flag.Int64("cosmos-max-gas", 75_000_000, "Cosmos consensus block max gas")
		cosmosRecipients            = flag.Int("cosmos-recipients", 20, "Cosmos MsgMultiSend recipients")
		validators                  = flag.Uint64("validators", 1, "Docker validator count")
		nodes                       = flag.Uint64("nodes", 1, "Docker full node count")
		wallets                     = flag.Int("wallets", 5000, "wallets funded in genesis and exposed to Catalyst")
		preseedProfile              = flag.String("preseed-profile", "none", "preseed profile: none or accounts")
		preseedAccounts             = flag.Int("preseed-accounts", 0, "additional deterministic inactive accounts funded in genesis for simapp scenarios")
		catalyst                    = flag.String("catalyst-version", "", "Catalyst Docker tag; empty uses ghcr.io/skip-mev/catalyst:latest")
		celestiaABScript            = flag.String("celestia-ab-script", "/home/mikers/dev/snissn/gomap-human/scripts/run_celestia_ab.sh", "run_celestia A/B script used by -scenario celestia-sync-ab")
		celestiaRunCommand          = flag.String("celestia-run-cmd", filepath.Join(defaultCelestiaAppDir, "run_celestia.sh"), "Celestia sync launcher command used by run_celestia_ab.sh")
		celestiaOutDir              = flag.String("celestia-out-dir", filepath.Join("reports", "artifacts", "celestia-sync", time.Now().Format("20060102-150405")), "output directory for the Celestia A/B runner")
		celestiaControlEnv          = flag.String("celestia-control-env", "", "optional env file for the LevelDB/control Celestia run; generated when empty")
		celestiaCandidateEnv        = flag.String("celestia-candidate-env", "", "optional env file for the TreeDB/candidate Celestia run; generated when empty")
		celestiaMaxPairs            = flag.Int("celestia-max-pairs", 1, "Celestia A/B pairs to run")
		celestiaPairRunMode         = flag.String("celestia-pair-run-mode", "concurrent", "Celestia A/B pair run mode: sequential or concurrent")
		celestiaRunTimeout          = flag.Int("celestia-run-timeout-seconds", 1800, "per-variant Celestia sync timeout in seconds")
		celestiaDwell               = flag.Int("celestia-post-sync-dwell-seconds", 300, "post-sync dwell window in seconds")
		celestiaSnapshot            = flag.String("celestia-required-accepted-snapshot-height", "11758500", "accepted state-sync snapshot height required for strict Celestia runs; empty disables")
		celestiaTrustHeight         = flag.String("celestia-trust-height", "", "optional fixed Celestia trust height")
		celestiaTrustHash           = flag.String("celestia-trust-hash", "", "optional fixed Celestia trust hash")
		celestiaStopHeight          = flag.String("celestia-stop-at-local-height", "", "optional fixed local height stop target")
		celestiaFreezeRemote        = flag.Bool("celestia-freeze-remote-height-at-start", true, "freeze remote height at run start when no explicit stop height is supplied")
		celestiaRunHomeBase         = flag.String("celestia-run-home-base", defaultFastPath("ironbird-celestia-runs"), "base directory for generated Celestia homes")
		celestiaGoCacheRoot         = flag.String("celestia-go-cache-root", defaultFastPath("ironbird-go-cache"), "base directory for Celestia Go caches")
		celestiaTempDir             = flag.String("celestia-temp-dir", defaultFastPath("tmp"), "TMPDIR for Celestia builds and sync runs")
		celestiaAppDir              = flag.String("celestia-app-dir", defaultCelestiaAppDir, "Celestia app checkout used by the sync launcher")
		celestiaGomapDir            = flag.String("celestia-gomap-dir", defaultExistingPath("/mnt/fast4tb/worktrees/gomap-ironbird-wal-sync", "/home/mikers/dev/snissn/gomap"), "gomap checkout used for TreeDB in Celestia sync")
		celestiaCosmosDBDir         = flag.String("celestia-cosmos-db-dir", defaultExistingPath("/mnt/fast4tb/worktrees/cosmos-db-ironbird-wal-sync", "/home/mikers/dev/snissn/cosmos-db"), "cosmos-db checkout used for Celestia sync")
		celestiaCometDBDir          = flag.String("celestia-comet-db-dir", "/home/mikers/dev/snissn/cometbft-db", "cometbft-db checkout used for Celestia sync")
		celestiaCosmosStoreDir      = flag.String("celestia-cosmos-store-dir", "/home/mikers/dev/snissn/celestia-cosmos-sdk/store", "cosmossdk.io/store checkout used for Celestia sync")
		celestiaCosmosLogDir        = flag.String("celestia-cosmos-log-dir", "/home/mikers/dev/snissn/celestia-cosmos-sdk/log", "cosmossdk.io/log checkout used for Celestia sync")
		celestiaCosmosCoreDir       = flag.String("celestia-cosmos-core-dir", "/home/mikers/dev/snissn/celestia-cosmos-sdk/core", "cosmossdk.io/core checkout used for Celestia sync")
		celestiaIAVLDir             = flag.String("celestia-iavl-dir", "/home/mikers/dev/snissn/iavl", "IAVL checkout available to the Celestia sync launcher")
		celestiaTreeDBProfile       = flag.String("celestia-treedb-open-profile", "command_wal_durable", "TreeDB open profile for the Celestia TreeDB candidate")
		celestiaUseLocalTreeStack   = flag.Bool("celestia-use-local-tree-stack", true, "build Celestia against local gomap/cosmos-db/cometbft-db modules")
		celestiaUseLocalCosmosStore = flag.Bool("celestia-use-local-cosmos-store", true, "include local cosmossdk.io/store/log/core modules in the Celestia workspace")
		celestiaUseLocalIAVL        = flag.Bool("celestia-use-local-iavl", false, "replace github.com/cosmos/iavl with the local IAVL checkout")
		celestiaDryRun              = flag.Bool("celestia-dry-run", false, "for celestia-sync-ab, write env files and artifact metadata without executing the sync")
	)
	flag.Parse()
	if *loadWindowTargetFraction <= 0 || *loadWindowTargetFraction > 1 {
		fatalf("-load-window-target-fraction must be > 0 and <= 1, got %v", *loadWindowTargetFraction)
	}
	preseed := makePreseedConfig(*preseedProfile, *preseedAccounts, *wallets)
	celestiaConfig := celestiaSyncConfig{
		RunnerScript:                   *celestiaABScript,
		RunCommand:                     *celestiaRunCommand,
		OutputDir:                      *celestiaOutDir,
		ControlEnvFile:                 *celestiaControlEnv,
		CandidateEnvFile:               *celestiaCandidateEnv,
		MaxPairs:                       *celestiaMaxPairs,
		PairRunMode:                    *celestiaPairRunMode,
		RunTimeoutSeconds:              *celestiaRunTimeout,
		PostSyncDwellSeconds:           *celestiaDwell,
		RequiredAcceptedSnapshotHeight: *celestiaSnapshot,
		TrustHeight:                    *celestiaTrustHeight,
		TrustHash:                      *celestiaTrustHash,
		StopAtLocalHeight:              *celestiaStopHeight,
		FreezeRemoteHeightAtStart:      *celestiaFreezeRemote,
		RunHomeBase:                    *celestiaRunHomeBase,
		GoCacheRoot:                    *celestiaGoCacheRoot,
		TempDir:                        *celestiaTempDir,
		CelestiaAppDir:                 *celestiaAppDir,
		GomapDir:                       *celestiaGomapDir,
		CosmosDBDir:                    *celestiaCosmosDBDir,
		CometDBDir:                     *celestiaCometDBDir,
		CosmosStoreDir:                 *celestiaCosmosStoreDir,
		CosmosLogDir:                   *celestiaCosmosLogDir,
		CosmosCoreDir:                  *celestiaCosmosCoreDir,
		IAVLDir:                        *celestiaIAVLDir,
		TreeDBOpenProfile:              *celestiaTreeDBProfile,
		UseLocalTreeStack:              *celestiaUseLocalTreeStack,
		UseLocalCosmosStore:            *celestiaUseLocalCosmosStore,
		UseLocalIAVL:                   *celestiaUseLocalIAVL,
		DryRun:                         *celestiaDryRun,
	}
	celestiaConfig = normalizeCelestiaConfig(celestiaConfig)
	stopRuntimeProfiles := startRuntimeProfiles(*cpuprofile, *memprofile, *blockprofile, *mutexprofile)
	defer stopRuntimeProfiles()

	ctx := context.Background()
	if *outPath == "" {
		*outPath = filepath.Join("reports", "artifacts", fmt.Sprintf("ironbird-local-run-%s.json", time.Now().Format("20060102-150405")))
	}

	artifact := reportArtifact{
		GeneratedAt: time.Now(),
		Host:        hostMetadata(ctx),
		Git:         gitMetadata(ctx),
		Flags:       flagMetadata(),
		Errors:      map[string]string{},
	}

	allScenarios := []scenario{
		evmBlogScenario(*validators, *nodes, *wallets, *evmBlocks, *evmBatches, *evmMsgs, *evmMsgType, *evmSendInterval, *evmInitialWallets, *evmInitialContracts, *evmIterations, *evmCalldataSize, *evmGasFeeCap, *evmGasTipCap, *catalyst),
		simappScenario("simapp-goleveldb", "SDK simapp with goleveldb app state", "goleveldb", *validators, *nodes, *wallets, preseed, *cosmosBlocks, *cosmosTxs, *cosmosMsg, *cosmosContained, *cosmosMsgsPerTx, *cosmosRecipients, *cosmosMaxGas, *catalyst),
		simappScenario("simapp-treedb", "SDK simapp with TreeDB app state", "treedb", *validators, *nodes, *wallets, preseed, *cosmosBlocks, *cosmosTxs, *cosmosMsg, *cosmosContained, *cosmosMsgsPerTx, *cosmosRecipients, *cosmosMaxGas, *catalyst),
		simappFullStackScenario("simapp-goleveldb-all", "SDK simapp with goleveldb app state and CometBFT node DBs", "goleveldb", *validators, *nodes, *wallets, preseed, *cosmosBlocks, *cosmosTxs, *cosmosMsg, *cosmosContained, *cosmosMsgsPerTx, *cosmosRecipients, *cosmosMaxGas, *catalyst),
		simappFullStackScenario("simapp-treedb-all", "SDK simapp with TreeDB app state and CometBFT node DBs", "treedb", *validators, *nodes, *wallets, preseed, *cosmosBlocks, *cosmosTxs, *cosmosMsg, *cosmosContained, *cosmosMsgsPerTx, *cosmosRecipients, *cosmosMaxGas, *catalyst),
		celestiaSyncScenario(celestiaConfig),
	}
	selectedScenarios := selectScenarios(allScenarios, *scenarioFlag)
	var workerConfig types.WorkerConfig
	if needsWorkerConfig(selectedScenarios) {
		var err error
		workerConfig, err = types.ParseWorkerConfig("conf/worker.yaml")
		if err != nil {
			fatalf("parse worker config: %v", err)
		}
	}

	for _, sc := range selectedScenarios {
		if *appCPUProfileDir != "" {
			sc = withAppCPUProfile(sc)
		}
		if *appHeapProfileDir != "" || *appPprofProfileDir != "" {
			sc = withAppHeapProfile(sc)
		}
		var result runResult
		if sc.Runner == "celestia-sync-ab" {
			result = runCelestiaSyncScenario(ctx, sc)
		} else {
			result = runScenario(ctx, workerConfig, sc, *skipBuild, *keepTestnets, *commitBenchBlocks, *appCPUProfileDir, *appHeapProfileDir, *appPprofProfileDir, *rawTxAudit, *loadWindowDrainTimeout, *loadWindowMinDuration, *loadWindowTargetFraction, *stopCatalystAfterLoadWindow)
		}
		artifact.Results = append(artifact.Results, result)
		if result.Error != "" {
			artifact.Errors[result.Scenario.Name] = result.Error
		}
	}
	if len(artifact.Errors) == 0 {
		artifact.Errors = nil
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fatalf("create artifact dir: %v", err)
	}
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		fatalf("marshal artifact: %v", err)
	}
	if err := os.WriteFile(*outPath, append(body, '\n'), 0o644); err != nil {
		fatalf("write artifact: %v", err)
	}
	fmt.Printf("wrote %s\n", *outPath)
	if *markdownPath == "" {
		*markdownPath = markdownPathForJSON(*outPath)
	}
	if err := os.MkdirAll(filepath.Dir(*markdownPath), 0o755); err != nil {
		fatalf("create markdown artifact dir: %v", err)
	}
	if err := os.WriteFile(*markdownPath, []byte(renderReportMarkdown(artifact)), 0o644); err != nil {
		fatalf("write markdown artifact: %v", err)
	}
	fmt.Printf("wrote %s\n", *markdownPath)
}

func startRuntimeProfiles(cpuPath, heapPath, blockPath, mutexPath string) func() {
	if blockPath != "" {
		runtime.SetBlockProfileRate(1)
	}
	if mutexPath != "" {
		runtime.SetMutexProfileFraction(1)
	}
	if cpuPath != "" {
		f, err := os.Create(cpuPath)
		if err != nil {
			fatalf("create CPU profile: %v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			fatalf("start CPU profile: %v", err)
		}
		deferClose := f
		return func() {
			pprof.StopCPUProfile()
			_ = deferClose.Close()
			writeRuntimeProfiles(heapPath, blockPath, mutexPath)
		}
	}
	return func() {
		writeRuntimeProfiles(heapPath, blockPath, mutexPath)
	}
}

func writeRuntimeProfiles(heapPath, blockPath, mutexPath string) {
	if heapPath != "" {
		f, err := os.Create(heapPath)
		if err != nil {
			fatalf("create heap profile: %v", err)
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			_ = f.Close()
			fatalf("write heap profile: %v", err)
		}
		if err := f.Close(); err != nil {
			fatalf("close heap profile: %v", err)
		}
	}
	for _, item := range []struct {
		path string
		name string
	}{
		{path: blockPath, name: "block"},
		{path: mutexPath, name: "mutex"},
	} {
		if item.path == "" {
			continue
		}
		f, err := os.Create(item.path)
		if err != nil {
			fatalf("create %s profile: %v", item.name, err)
		}
		prof := pprof.Lookup(item.name)
		if prof == nil {
			_ = f.Close()
			fatalf("lookup %s profile: not found", item.name)
		}
		if err := prof.WriteTo(f, 0); err != nil {
			_ = f.Close()
			fatalf("write %s profile: %v", item.name, err)
		}
		if err := f.Close(); err != nil {
			fatalf("close %s profile: %v", item.name, err)
		}
	}
}

func withAppCPUProfile(sc scenario) scenario {
	if sc.BaseImage != "simapp" {
		return sc
	}
	name := fmt.Sprintf("%s-validator-0-cpu.pprof", sanitize(sc.Name))
	sc.AppCPUProfile = name
	sc.AdditionalFlags = append(sc.AdditionalFlags, "--cpu-profile", filepath.Join("/simd", name))
	return sc
}

func withAppHeapProfile(sc scenario) scenario {
	if sc.BaseImage != "simapp" {
		return sc
	}
	name := fmt.Sprintf("%s-validator-0-heap.pprof", sanitize(sc.Name))
	sc.AppHeapProfile = name
	sc.AppPprofListen = "localhost:6060"
	sc.CustomConfig = withRPCPprofListen(sc.CustomConfig, sc.AppPprofListen)
	return sc
}

func withRPCPprofListen(cfg map[string]interface{}, listen string) map[string]interface{} {
	out := cloneStringMap(cfg)
	rpc := cloneStringMap(asStringMap(out["rpc"]))
	rpc["pprof_laddr"] = listen
	out["rpc"] = rpc
	return out
}

func cloneStringMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func asStringMap(v interface{}) map[string]interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		return typed
	default:
		return nil
	}
}

func evmBlogScenario(validators, nodes uint64, wallets, blocks, batches, msgs int, msgType string, sendInterval time.Duration, initialWallets int, initialContracts uint64, iterations, calldataSize int, gasFeeCap, gasTipCap int64, catalyst string) scenario {
	loadMsg := ctlt.LoadTestMsg{Weight: 1, Type: ctlt.MsgType(msgType), NumMsgs: msgs}
	if iterations > 0 {
		loadMsg.NumOfIterations = iterations
	}
	if calldataSize > 0 {
		loadMsg.CalldataSize = calldataSize
	}
	spec := ctlt.LoadTestSpec{
		Name:           "eth_loadtest",
		Description:    "Ironbird local Docker reproduction of the Cosmos performance blog shape",
		Kind:           "eth",
		ChainID:        "262144",
		SendInterval:   sendInterval,
		NumBatches:     batches,
		InitialWallets: initialWallets,
		ChainCfg: ctlteth.ChainConfig{
			NumInitialContracts: initialContracts,
			TxOpts: ctlteth.TxOpts{
				GasFeeCap: big.NewInt(gasFeeCap),
				GasTipCap: big.NewInt(gasTipCap),
			},
		},
		Msgs: []ctlt.LoadTestMsg{loadMsg},
	}
	if blocks > 0 {
		spec.NumOfBlocks = blocks
		spec.SendInterval = 0
		spec.NumBatches = 0
	}
	return scenario{
		Name:            "evm-blog",
		IncludeInAll:    true,
		ChainName:       "evmblog",
		Description:     "Cosmos EVM blog-style native ERC20 transfer load",
		Repo:            "evm",
		ChainSource:     "https://github.com/cosmos/evm",
		ChainRef:        "f90a5c79c0052e0f5cd670a367f24967d1120650",
		Dockerfile:      "hack/evm.Dockerfile",
		BaseImage:       "evm",
		ImageTag:        "ironbird-report:evm-blog-f90a5c79",
		IsEVMChain:      true,
		NumValidators:   validators,
		NumNodes:        nodes,
		NumWallets:      wallets,
		CatalystVersion: catalyst,
		Genesis: []chain.GenesisKV{
			{Key: "app_state.staking.params.bond_denom", Value: "atest"},
			{Key: "app_state.gov.params.expedited_voting_period", Value: "120s"},
			{Key: "app_state.gov.params.voting_period", Value: "300s"},
			{Key: "app_state.gov.params.expedited_min_deposit.0.amount", Value: "1"},
			{Key: "app_state.gov.params.expedited_min_deposit.0.denom", Value: "atest"},
			{Key: "app_state.gov.params.min_deposit.0.amount", Value: "1"},
			{Key: "app_state.gov.params.min_deposit.0.denom", Value: "atest"},
			{Key: "app_state.evm.params.evm_denom", Value: "atest"},
			{Key: "app_state.mint.params.mint_denom", Value: "atest"},
			{Key: "app_state.bank.denom_metadata", Value: []map[string]interface{}{
				{
					"description": "The native staking token for evmd.",
					"denom_units": []map[string]interface{}{
						{"denom": "atest", "exponent": 0, "aliases": []string{"attotest"}},
						{"denom": "test", "exponent": 18, "aliases": []string{}},
					},
					"base":     "atest",
					"display":  "test",
					"name":     "Test Token",
					"symbol":   "TEST",
					"uri":      "",
					"uri_hash": "",
				},
			}},
			{Key: "app_state.erc20.native_precompiles", Value: []string{"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"}},
			{Key: "app_state.evm.params.active_static_precompiles", Value: []string{
				"0x0000000000000000000000000000000000000100",
				"0x0000000000000000000000000000000000000400",
				"0x0000000000000000000000000000000000000800",
				"0x0000000000000000000000000000000000000801",
				"0x0000000000000000000000000000000000000802",
				"0x0000000000000000000000000000000000000803",
				"0x0000000000000000000000000000000000000804",
				"0x0000000000000000000000000000000000000805",
			}},
			{Key: "app_state.erc20.token_pairs", Value: []map[string]interface{}{
				{
					"contract_owner": 1,
					"erc20_address":  "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
					"denom":          "atest",
					"enabled":        true,
				},
			}},
			{Key: "consensus.params.block.max_gas", Value: "550000000"},
			{Key: "consensus.params.block.max_bytes", Value: "104857600"},
			{Key: "app_state.feemarket.params.no_base_fee", Value: true},
		},
		CustomConfig: lowLatencyConsensusConfig(),
		CustomAppConfig: map[string]interface{}{
			"json-rpc": map[string]interface{}{
				"enable-profiling": true,
			},
		},
		CustomClient: map[string]interface{}{
			"chain-id": "262144",
		},
		LoadTestSpec: spec,
	}
}

func lowLatencyConsensusConfig() map[string]interface{} {
	return map[string]interface{}{
		"consensus": map[string]interface{}{
			"timeout_commit":             "0ms",
			"peer_gossip_sleep_duration": "0s",
		},
		"mempool": map[string]interface{}{
			"size": 1000000,
		},
		"p2p": map[string]interface{}{
			"flush_throttle_timeout": "0ms",
		},
	}
}

func makePreseedConfig(profile string, accounts, activeWallets int) preseedConfig {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		profile = "none"
	}
	if accounts < 0 {
		fatalf("preseed-accounts must be >= 0")
	}
	switch profile {
	case "none":
		if accounts != 0 {
			fatalf("preseed-accounts requires -preseed-profile accounts")
		}
		return preseedConfig{Profile: "none", ActiveWallets: activeWallets, GenesisAccounts: activeWallets}
	case "accounts":
		if accounts == 0 {
			fatalf("preseed-profile accounts requires -preseed-accounts > 0")
		}
		return preseedConfig{
			Profile:             "accounts",
			Accounts:            accounts,
			ActiveWallets:       activeWallets,
			GenesisAccounts:     activeWallets + accounts,
			DeterministicSource: "base mnemonic plus integer account passphrases",
		}
	default:
		fatalf("unknown preseed-profile %q", profile)
		return preseedConfig{}
	}
}

const (
	simappCosmosDBVersion = "v0.0.0-20260701184343-6ddcb75557e5"
	simappCosmosDBRef     = "6ddcb75557e59bc4e6668ac7699cd52b63b3e402"
	simappGomapVersion    = "v0.6.2-0.20260705085743-cabb7a17fb88"
	simappGomapRef        = "cabb7a17fb8809aebcd33f6c007907cf0caddcc7"
	simappIAVLVersion     = "v0.0.0-20260701072929-12a26715119b"
	simappIAVLRef         = "12a26715119bb3ea55289ffd7b256161effc7b8b"
	simappCometDBVersion  = "v0.0.0-20260701074104-b4f87847a725"
	simappCometDBRef      = "b4f87847a725f92a046d927ce4a0f5b08b965995"
)

func simappDependencyPins(includeCometDB bool) []dependencyPin {
	pins := []dependencyPin{
		{
			Module:  "github.com/cosmos/cosmos-db",
			Version: simappCosmosDBVersion,
			Ref:     simappCosmosDBRef,
		},
		{
			Module:  "github.com/snissn/gomap",
			Version: simappGomapVersion,
			Ref:     simappGomapRef,
		},
		{
			Module:  "github.com/cosmos/iavl",
			Version: simappIAVLVersion,
			Ref:     simappIAVLRef,
		},
	}
	if includeCometDB {
		pins = append(pins, dependencyPin{
			Module:  "github.com/cometbft/cometbft-db",
			Version: simappCometDBVersion,
			Ref:     simappCometDBRef,
		})
	}
	return pins
}

func simappReplaceCmd(includeCometDB bool) string {
	cmds := []string{
		"github.com/cosmos/cosmos-db=github.com/snissn/cosmos-db@" + simappCosmosDBVersion,
		"github.com/snissn/gomap=github.com/snissn/gomap@" + simappGomapVersion,
		"github.com/cosmos/iavl=github.com/snissn/iavl@" + simappIAVLVersion,
	}
	if includeCometDB {
		cmds = append(cmds, "github.com/cometbft/cometbft-db=github.com/snissn/cometbft-db@"+simappCometDBVersion)
	}
	return strings.Join(cmds, " ")
}

func simappScenario(name, desc, backend string, validators, nodes uint64, wallets int, preseed preseedConfig, blocks, txs int, cosmosMsg, containedMsg string, msgsPerTx, recipients int, maxGas int64, catalyst string) scenario {
	return simappScenarioWithBackends(name, desc, backend, "", false, validators, nodes, wallets, preseed, blocks, txs, cosmosMsg, containedMsg, msgsPerTx, recipients, maxGas, catalyst)
}

func simappFullStackScenario(name, desc, backend string, validators, nodes uint64, wallets int, preseed preseedConfig, blocks, txs int, cosmosMsg, containedMsg string, msgsPerTx, recipients int, maxGas int64, catalyst string) scenario {
	return simappScenarioWithBackends(name, desc, backend, backend, true, validators, nodes, wallets, preseed, blocks, txs, cosmosMsg, containedMsg, msgsPerTx, recipients, maxGas, catalyst)
}

func simappScenarioWithBackends(name, desc, appBackend, nodeBackend string, includeCometDB bool, validators, nodes uint64, wallets int, preseed preseedConfig, blocks, txs int, cosmosMsg, containedMsg string, msgsPerTx, recipients int, maxGas int64, catalyst string) scenario {
	appConfig := map[string]interface{}{}
	if appBackend != "" {
		appConfig["app-db-backend"] = appBackend
	}
	consensusConfig := lowLatencyConsensusConfig()
	if nodeBackend != "" {
		consensusConfig["db_backend"] = nodeBackend
	}
	loadMsgs := []ctlt.LoadTestMsg{{Weight: 1, Type: ctlt.MsgType(cosmosMsg)}}
	if cosmosMsg == "MsgMultiSend" {
		loadMsgs[0].NumOfRecipients = recipients
	}
	if cosmosMsg == "MsgArr" {
		loadMsgs[0].ContainedType = ctlt.MsgType(containedMsg)
		loadMsgs[0].NumMsgs = msgsPerTx
		loadMsgs[0].NumOfRecipients = recipients
	}
	return scenario{
		Name:            name,
		IncludeInAll:    true,
		ChainName:       shortChainName(name),
		Description:     desc,
		Repo:            "cosmos-sdk",
		ChainSource:     "https://github.com/snissn/celestia-cosmos-sdk",
		ChainRef:        "28e5525fefe7aaa53d4726ef7a367242bacf9003",
		Dockerfile:      "hack/simapp.Dockerfile",
		ReplaceCmd:      simappReplaceCmd(includeCometDB),
		BaseImage:       "simapp",
		ImageTag:        simappImageTag(includeCometDB),
		DependencyPins:  simappDependencyPins(includeCometDB),
		Preseed:         preseed,
		IsEVMChain:      false,
		NumValidators:   validators,
		NumNodes:        nodes,
		NumWallets:      wallets,
		CatalystVersion: catalyst,
		AppDBBackend:    appBackend,
		NodeDBBackend:   nodeBackend,
		Genesis: []chain.GenesisKV{
			{Key: "consensus.params.block.max_gas", Value: strconv.FormatInt(maxGas, 10)},
		},
		CustomAppConfig: appConfig,
		CustomConfig:    consensusConfig,
		LoadTestSpec: ctlt.LoadTestSpec{
			Name:        name,
			Description: desc,
			Kind:        "cosmos",
			ChainID:     name,
			NumOfBlocks: blocks,
			NumOfTxs:    txs,
			Msgs:        loadMsgs,
		},
	}
}

func simappImageTag(includeCometDB bool) string {
	if includeCometDB {
		return "ironbird-report:snissn-sdk-28e5525f-fullstack-cosmosdb-6ddcb75-cometdb-b4f878-gomap-cabb7a1"
	}
	return "ironbird-report:snissn-sdk-28e5525f-cosmosdb-6ddcb75-gomap-cabb7a1"
}

func celestiaSyncScenario(cfg celestiaSyncConfig) scenario {
	return scenario{
		Name:         "celestia-sync-ab",
		Runner:       "celestia-sync-ab",
		ChainName:    "celestia-mainnet",
		Description:  "Celestia mainnet production-shaped state-sync comparison: goleveldb control versus TreeDB candidate",
		Repo:         "celestia-app",
		ChainSource:  "https://github.com/snissn/celestia-app",
		ChainRef:     "local",
		AppDBBackend: "goleveldb-vs-treedb",
		CelestiaSync: cfg,
	}
}

func launchGenesisAccounts(sc scenario) int {
	if sc.Preseed.GenesisAccounts > 0 {
		return sc.Preseed.GenesisAccounts
	}
	return sc.NumWallets
}

func runScenario(ctx context.Context, config types.WorkerConfig, sc scenario, skipBuild, keep bool, commitBenchBlocks uint64, appCPUProfileDir, appHeapProfileDir, appPprofProfileDir string, rawTxAudit bool, loadWindowDrainTimeout, loadWindowMinDuration time.Duration, loadWindowTargetFraction float64, stopCatalystAfterLoadWindow bool) (result runResult) {
	result.Scenario = sc
	result.StartedAt = time.Now()
	result.ProviderName = fmt.Sprintf("ib%s%s", shortChainName(sc.Name), strings.ToLower(coreutil.RandomString(4)))
	defer func() {
		result.FinishedAt = time.Now()
		result.WallSeconds = result.FinishedAt.Sub(result.StartedAt).Seconds()
		if result.LoadTestResult.Overall.TotalTransactions != 0 || result.RawTxSummary != nil || result.CorrectedLoadTest != nil {
			result.Derived = deriveMetrics(sc, result)
		}
		result.RuntimeBreakdown = summarizeRuntimeBreakdown(result)
		if r := recover(); r != nil {
			result.Error = fmt.Sprint(r)
		}
	}()

	if !skipBuild {
		endPhase := startPhase(&result, "build_image", sc.ImageTag)
		cmd, logText, err := buildImage(ctx, sc)
		endPhase()
		result.ImageBuildCommand = cmd
		result.ImageBuildLog = trimLog(logText, 20000)
		if err != nil {
			result.Error = fmt.Sprintf("build image: %v", err)
			return result
		}
	}

	testnetActivity := &testnet.Activity{Chains: config.Chains, RegistryType: "local"}
	endPhase := startPhase(&result, "create_provider", result.ProviderName)
	createResp, err := testnetActivity.CreateProvider(ctx, messages.CreateProviderRequest{
		RunnerType: messages.Docker,
		Name:       result.ProviderName,
	})
	endPhase()
	if err != nil {
		result.Error = fmt.Sprintf("create provider: %v", err)
		return result
	}

	providerStateForLaunch := createResp.ProviderState
	providerStateForTeardown, err := util.CompressData(createResp.ProviderState)
	if err != nil {
		result.Error = fmt.Sprintf("compress provider state for teardown: %v", err)
		return result
	}
	defer func() {
		result.ContainerStats = dockerStats(ctx, result.ProviderName)
		result.ContainerLogs = dockerLogs(ctx, result.ProviderName, 16000)
		if keep {
			return
		}
		if len(providerStateForTeardown) == 0 {
			return
		}
		_, _ = testnetActivity.TeardownProvider(ctx, messages.TeardownProviderRequest{
			RunnerType:    messages.Docker,
			ProviderState: providerStateForTeardown,
		})
	}()

	launchStart := time.Now()
	endPhase = startPhase(&result, "launch_testnet", fmt.Sprintf("genesis_accounts=%d", launchGenesisAccounts(sc)))
	launchResp, err := testnetActivity.LaunchTestnet(ctx, messages.LaunchTestnetRequest{
		Name:                   sc.ChainName,
		Repo:                   sc.Repo,
		SHA:                    sc.ChainRef,
		IsEvmChain:             sc.IsEVMChain,
		Image:                  sc.ImageTag,
		BaseImage:              sc.BaseImage,
		GenesisModifications:   sc.Genesis,
		RunnerType:             messages.Docker,
		NumOfValidators:        sc.NumValidators,
		NumOfNodes:             sc.NumNodes,
		CustomAppConfig:        sc.CustomAppConfig,
		CustomConsensusConfig:  sc.CustomConfig,
		CustomClientConfig:     sc.CustomClient,
		AdditionalStartFlags:   sc.AdditionalFlags,
		SetPersistentPeers:     true,
		ProviderState:          providerStateForLaunch,
		NumWallets:             launchGenesisAccounts(sc),
		BaseMnemonic:           defaultMnemonic,
		ProviderSpecificConfig: nil,
	})
	endPhase()
	result.LaunchSeconds = time.Since(launchStart).Seconds()
	if len(launchResp.ProviderState) != 0 {
		providerStateForTeardown = launchResp.ProviderState
	}
	if err != nil {
		result.Error = fmt.Sprintf("launch testnet: %v", err)
		return result
	}
	providerStateForTeardown = launchResp.ProviderState

	if sc.BaseImage == "simapp" && (sc.AppDBBackend != "" || sc.NodeDBBackend != "") {
		endPhase = startPhase(&result, "verify_backend_config", "")
		verification := verifySimappBackends(ctx, launchResp.ProviderState, launchResp.ChainState, sc)
		result.BackendVerification = &verification
		endPhase()
		if !verification.Valid {
			result.Error = fmt.Sprintf("backend verification failed: %s", verification.Error)
			return result
		}
	}

	endPhase = startPhase(&result, "collect_before_metrics", "")
	result.MetricsBefore = scrapeMetrics(ctx, result.ProviderName)
	endPhase()
	endPhase = startPhase(&result, "collect_before_data_sizes", "")
	result.DataSizesBefore = collectDataSizes(ctx, result.ProviderName, sc)
	endPhase()

	sampler := startDockerStatsSampler(ctx, result.ProviderName, time.Second)
	defer func() {
		if sampler == nil {
			return
		}
		result.ResourceSamples = sampler.Stop()
		result.ResourceSummary = summarizeResourceSamples(result.ResourceSamples)
		sampler = nil
	}()
	if commitBenchBlocks > 0 {
		endPhase = startPhase(&result, "commit_benchmark", fmt.Sprintf("blocks=%d", commitBenchBlocks))
		bench, benchErr := runCommitBenchmark(ctx, launchResp.ProviderState, launchResp.ChainState, sc, commitBenchBlocks)
		endPhase()
		if sampler != nil {
			endPhase = startPhase(&result, "stop_resource_sampler", "")
			result.ResourceSamples = sampler.Stop()
			result.ResourceSummary = summarizeResourceSamples(result.ResourceSamples)
			sampler = nil
			endPhase()
		}
		endPhase = startPhase(&result, "collect_after_metrics", "")
		result.MetricsAfter = scrapeMetrics(ctx, result.ProviderName)
		endPhase()
		endPhase = startPhase(&result, "collect_after_data_sizes", "")
		result.DataSizesAfter = collectDataSizes(ctx, result.ProviderName, sc)
		endPhase()
		result.StorageSignals = summarizeStorageSignals(result.MetricsBefore, result.MetricsAfter, result.DataSizesBefore, result.DataSizesAfter)
		result.CommitBenchmark = bench
		endPhase = startPhase(&result, "collect_app_cpu_profile", "")
		result.ProfileArtifacts = append(result.ProfileArtifacts, collectAppProfiles(ctx, launchResp.ProviderState, launchResp.ChainState, sc, appCPUProfileDir, appHeapProfileDir, appPprofProfileDir)...)
		endPhase()
		if benchErr != nil {
			result.Error = fmt.Sprintf("run commit benchmark: %v", benchErr)
		}
		return result
	}
	var windowMonitor *loadWindowMonitor
	intendedTxs := intendedTransactions(sc)
	if !sc.IsEVMChain && intendedTxs > 0 {
		targetTxs := loadWindowTargetTransactions(intendedTxs, loadWindowTargetFraction)
		windowMonitor = startLoadWindowMonitor(ctx, result.ProviderName, result.MetricsBefore, targetTxs, loadWindowMinDuration, 500*time.Millisecond)
	}
	loadActivity := &loadtest.Activity{}
	if stopCatalystAfterLoadWindow && windowMonitor != nil {
		loadActivity.StopCondition = func(context.Context) (bool, string) {
			obs := windowMonitor.Last()
			if !obs.Reached {
				return false, ""
			}
			if obs.DurationSatisfied {
				return true, fmt.Sprintf("load window reached: %d successful tx in %.3fs", obs.SuccessfulTransactions, obs.Seconds)
			}
			return true, fmt.Sprintf("load window reached too quickly: %d successful tx in %.3fs below %.3fs minimum", obs.SuccessfulTransactions, obs.Seconds, obs.MinimumSeconds)
		}
	}
	endPhase = startPhase(&result, "run_load_test", fmt.Sprintf("wallets=%d", sc.NumWallets))
	loadResp, err := loadActivity.RunLoadTest(ctx, messages.RunLoadTestRequest{
		ChainState:      launchResp.ChainState,
		ProviderState:   launchResp.ProviderState,
		LoadTestSpec:    sc.LoadTestSpec,
		RunnerType:      messages.Docker,
		IsEvmChain:      sc.IsEVMChain,
		BaseMnemonic:    defaultMnemonic,
		NumWallets:      sc.NumWallets,
		CatalystVersion: sc.CatalystVersion,
	})
	endPhase()
	if windowMonitor != nil {
		var obs loadWindowObservation
		if loadWindowDrainTimeout > 0 {
			endPhase = startPhase(&result, "load_window_drain", fmt.Sprintf("timeout=%s", loadWindowDrainTimeout))
			obs = windowMonitor.Wait(loadWindowDrainTimeout)
			endPhase()
		} else {
			obs = windowMonitor.Stop()
		}
		result.LoadWindow = &obs
	}
	if sampler != nil {
		endPhase = startPhase(&result, "stop_resource_sampler", "")
		result.ResourceSamples = sampler.Stop()
		result.ResourceSummary = summarizeResourceSamples(result.ResourceSamples)
		sampler = nil
		endPhase()
	}
	endPhase = startPhase(&result, "collect_after_metrics", "")
	result.MetricsAfter = scrapeMetrics(ctx, result.ProviderName)
	endPhase()
	endPhase = startPhase(&result, "collect_after_data_sizes", "")
	result.DataSizesAfter = collectDataSizes(ctx, result.ProviderName, sc)
	endPhase()
	result.StorageSignals = summarizeStorageSignals(result.MetricsBefore, result.MetricsAfter, result.DataSizesBefore, result.DataSizesAfter)
	result.LoadTestResult = loadResp.Result
	result.LoadTestConfig = loadResp.LoadTestConfig
	result.LoadTestLogs = trimLog(loadResp.TaskLogs, 40000)
	result.LoadTestStopped = loadResp.StoppedReason
	result.LoadTestLogSummary = summarizeLoadTestLogs(loadResp.TaskLogs)
	if !sc.IsEVMChain && rawTxAudit {
		endPhase = startPhase(&result, "raw_tx_audit", "")
		result.RawTxAudit = auditRawTxs(ctx, result.ProviderName, sc.ChainName, loadResp.TaskLogs, loadResp.Result)
		result.RawTxSummary = summarizeTxAudit(result.RawTxAudit, loadResp.Result.Overall.Runtime)
		endPhase()
	} else if !sc.IsEVMChain {
		result.RawTxAuditSkipped = "disabled by -raw-tx-audit=false"
	}
	result.CorrectedLoadTest = summarizeCorrectedLoadTest(result)
	endPhase = startPhase(&result, "collect_app_cpu_profile", "")
	result.ProfileArtifacts = append(result.ProfileArtifacts, collectAppProfiles(ctx, launchResp.ProviderState, launchResp.ChainState, sc, appCPUProfileDir, appHeapProfileDir, appPprofProfileDir)...)
	endPhase()
	result.Derived = deriveMetrics(sc, result)
	if len(loadResp.ProviderState) != 0 {
		providerStateForTeardown = loadResp.ProviderState
	}
	if err != nil {
		var stopped *loadtest.StoppedByConditionError
		if errors.As(err, &stopped) && stopCatalystAfterLoadWindow && loadWindowAccepted(result.LoadWindow) {
			if result.LoadTestStopped == "" {
				result.LoadTestStopped = stopped.Reason
			}
			return result
		}
		result.Error = fmt.Sprintf("run load test: %v", err)
		return result
	}
	if loadResp.Result.Error != "" {
		result.Error = fmt.Sprintf("load test result error: %s", loadResp.Result.Error)
	}
	return result
}

func runCelestiaSyncScenario(ctx context.Context, sc scenario) (result runResult) {
	result.Scenario = sc
	result.StartedAt = time.Now()
	result.ProviderName = "external-celestia-sync"
	defer func() {
		result.FinishedAt = time.Now()
		result.WallSeconds = result.FinishedAt.Sub(result.StartedAt).Seconds()
		if r := recover(); r != nil {
			result.Error = fmt.Sprint(r)
		}
	}()

	cfg := sc.CelestiaSync
	endPhase := startPhase(&result, "prepare_celestia_sync", cfg.OutputDir)
	controlEnv, candidateEnv, err := prepareCelestiaEnvFiles(cfg)
	endPhase()
	if err != nil {
		result.Error = fmt.Sprintf("prepare celestia env files: %v", err)
		return result
	}

	syncResult := &celestiaSyncResult{
		Config:           cfg,
		OutputDir:        cfg.OutputDir,
		ControlEnvFile:   controlEnv,
		CandidateEnvFile: candidateEnv,
		Environment:      celestiaABEnvironment(cfg),
		RepoGit:          celestiaRepoMetadata(ctx, cfg),
	}
	result.CelestiaSync = syncResult

	command := []string{cfg.RunnerScript, cfg.OutputDir, cfg.RunCommand, controlEnv, candidateEnv}
	syncResult.Command = command
	if cfg.DryRun {
		endPhase = startPhase(&result, "celestia_sync_dry_run", "")
		endPhase()
		syncResult.CommandLog = "dry run: Celestia sync command was not executed"
		return result
	}

	endPhase = startPhase(&result, "run_celestia_sync_ab", fmt.Sprintf("pairs=%d", cfg.MaxPairs))
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = append(os.Environ(), envMapToList(syncResult.Environment)...)
	if cfg.RunnerScript != "" {
		cmd.Dir = filepath.Dir(cfg.RunnerScript)
	}
	out, runErr := cmd.CombinedOutput()
	endPhase()
	syncResult.CommandLog = trimLog(string(out), 60000)
	syncResult.ExitCode = commandExitCode(runErr)
	collectCelestiaArtifacts(syncResult)
	if runErr != nil {
		result.Error = fmt.Sprintf("run celestia sync A/B: %v", runErr)
	}
	return result
}

func prepareCelestiaEnvFiles(cfg celestiaSyncConfig) (string, string, error) {
	if cfg.OutputDir == "" {
		return "", "", fmt.Errorf("celestia output dir is required")
	}
	if cfg.TrustHeight == "" && cfg.TrustHash != "" || cfg.TrustHeight != "" && cfg.TrustHash == "" {
		return "", "", fmt.Errorf("celestia trust height and trust hash must be set together")
	}
	envDir := filepath.Join(cfg.OutputDir, "env")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		return "", "", err
	}
	controlEnv := cfg.ControlEnvFile
	if controlEnv == "" {
		controlEnv = filepath.Join(envDir, "control-goleveldb.env")
		if err := os.WriteFile(controlEnv, []byte(celestiaVariantEnv(cfg, "goleveldb")), 0o644); err != nil {
			return "", "", err
		}
	}
	candidateEnv := cfg.CandidateEnvFile
	if candidateEnv == "" {
		candidateEnv = filepath.Join(envDir, "candidate-treedb.env")
		if err := os.WriteFile(candidateEnv, []byte(celestiaVariantEnv(cfg, "treedb")), 0o644); err != nil {
			return "", "", err
		}
	}
	return controlEnv, candidateEnv, nil
}

func celestiaVariantEnv(cfg celestiaSyncConfig, backend string) string {
	env := map[string]string{
		"RUN_CELESTIA_REPO":                cfg.CelestiaAppDir,
		"DB_BACKEND":                       backend,
		"APP_DB_BACKEND":                   backend,
		"HOME":                             cfg.RunHomeBase,
		"CELESTIA_RUN_HOME_BASE":           cfg.RunHomeBase,
		"TMPDIR":                           cfg.TempDir,
		"USE_LOCAL_TREE_STACK":             boolEnv(cfg.UseLocalTreeStack),
		"USE_LOCAL_COSMOS_STORE":           boolEnv(cfg.UseLocalCosmosStore),
		"USE_LOCAL_IAVL":                   boolEnv(cfg.UseLocalIAVL),
		"LOCAL_GOMAP_DIR":                  cfg.GomapDir,
		"LOCAL_COSMOS_DB_DIR":              cfg.CosmosDBDir,
		"LOCAL_COMET_DB_DIR":               cfg.CometDBDir,
		"LOCAL_COSMOS_STORE_DIR":           cfg.CosmosStoreDir,
		"LOCAL_COSMOS_LOG_DIR":             cfg.CosmosLogDir,
		"LOCAL_COSMOS_CORE_DIR":            cfg.CosmosCoreDir,
		"LOCAL_IAVL_DIR":                   cfg.IAVLDir,
		"POST_SYNC_DWELL_SECONDS":          strconv.Itoa(cfg.PostSyncDwellSeconds),
		"FREEZE_REMOTE_HEIGHT_AT_START":    boolEnv(cfg.FreezeRemoteHeightAtStart),
		"CAPTURE_HEAP_ON_MAX_RSS":          "0",
		"CAPTURE_PPROF_ON_STUCK":           "0",
		"CAPTURE_FULL_SMAPS_ON_MAX_RSS":    "0",
		"CAPTURE_DEBUG_VARS_ON_MAX_RSS":    "0",
		"TREEDB_FORCE_CHECKPOINT_ON_WRITE": "0",
		"RUN_HOME_GLOB":                    filepath.Join(cfg.RunHomeBase, ".celestia-app-mainnet-"+backend+"-*"),
		"CELESTIA_APPD_BIN":                filepath.Join(cfg.CelestiaAppDir, "build", "celestia-appd-"+backend),
	}
	if backend == "treedb" {
		env["P2P_PORT"] = "37656"
		env["RPC_PORT"] = "37657"
		env["RPC_GRPC_PORT"] = "37658"
		env["PRIV_VALIDATOR_GRPC_PORT"] = "37659"
		env["PROMETHEUS_PORT"] = "37660"
		env["API_PORT"] = "37317"
		env["APP_GRPC_PORT"] = "39091"
		env["PPROF_LADDR"] = "localhost:6162"
		env["AB_LIVE_DEBUG_VARS_URL"] = "http://127.0.0.1:6162/debug/vars"
	} else {
		env["P2P_PORT"] = "36656"
		env["RPC_PORT"] = "36657"
		env["RPC_GRPC_PORT"] = "36658"
		env["PRIV_VALIDATOR_GRPC_PORT"] = "36659"
		env["PROMETHEUS_PORT"] = "36660"
		env["API_PORT"] = "36317"
		env["APP_GRPC_PORT"] = "39090"
		env["PPROF_LADDR"] = "localhost:6062"
		env["AB_LIVE_DEBUG_VARS_URL"] = "http://127.0.0.1:6062/debug/vars"
	}
	if cfg.GoCacheRoot != "" {
		env["GOCACHE"] = filepath.Join(cfg.GoCacheRoot, "build-cache")
		env["GOMODCACHE"] = filepath.Join(cfg.GoCacheRoot, "mod-cache")
		env["GOPATH"] = filepath.Join(cfg.GoCacheRoot, "gopath")
	}
	if cfg.RequiredAcceptedSnapshotHeight != "" {
		env["REQUIRED_ACCEPTED_SNAPSHOT_HEIGHT"] = cfg.RequiredAcceptedSnapshotHeight
	}
	if cfg.TrustHeight != "" {
		env["TRUST_HEIGHT"] = cfg.TrustHeight
		env["TRUST_HASH"] = cfg.TrustHash
	}
	if cfg.StopAtLocalHeight != "" {
		env["STOP_AT_LOCAL_HEIGHT"] = cfg.StopAtLocalHeight
	}
	if backend == "treedb" {
		env["TREEDB_OPEN_PROFILE"] = cfg.TreeDBOpenProfile
		env["TREEDB_ENABLE_LEAF_GENERATION_PACK_MAINTENANCE"] = "1"
	}
	return shellEnvFile(env)
}

func celestiaABEnvironment(cfg celestiaSyncConfig) map[string]string {
	maxPairs := cfg.MaxPairs
	if maxPairs <= 0 {
		maxPairs = 1
	}
	pairRunMode := cfg.PairRunMode
	if pairRunMode == "" {
		pairRunMode = "concurrent"
	}
	runTimeout := cfg.RunTimeoutSeconds
	if runTimeout < 0 {
		runTimeout = 0
	}
	env := map[string]string{
		"PAIR_RUN_MODE":                     pairRunMode,
		"MAX_PAIRS":                         strconv.Itoa(maxPairs),
		"MIN_PAIRS":                         "1",
		"CLEAR_WIN_PAIRS":                   "1",
		"CLEAR_LOSS_PAIRS":                  "1",
		"STOP_ON_CLEAR":                     "0",
		"RUN_TIMEOUT_SECONDS":               strconv.Itoa(runTimeout),
		"RUN_MAX_ATTEMPTS_PER_VARIANT":      "1",
		"RUN_RETRY_SLEEP_SECONDS":           "0",
		"SLEEP_BETWEEN_RUNS_SECONDS":        "5",
		"REWRITE_ENABLED":                   "0",
		"STOP_PAIR_ON_FIRST_INVALID":        "1",
		"AB_CAPTURE_LIGHT_VLOG_STATS":       "0",
		"AB_DISABLE_HEAVY_DIAGNOSTICS":      "1",
		"AB_CAPTURE_HEAP_ON_MAX_RSS":        "0",
		"AB_CAPTURE_PPROF_ON_STUCK":         "0",
		"AB_CAPTURE_FULL_SMAPS_ON_MAX_RSS":  "0",
		"AB_CAPTURE_DEBUG_VARS_ON_MAX_RSS":  "0",
		"AB_CAPTURE_LIVE_DEBUG_VARS":        "1",
		"PAIR_ALIGN_TRUST_FROM_FIRST":       "0",
		"PAIR_ALIGN_STOP_HEIGHT_FROM_FIRST": "0",
		"PAIR_ALIGN_STOP_MARGIN":            "0",
		"BLOCK_DRIFT_TOLERANCE":             "0",
		"SCORING_MODE":                      "per_block",
		"ALLOW_DRIFT_SCORING":               "0",
		"RUN_HOME_GLOB":                     filepath.Join(cfg.RunHomeBase, ".celestia-app-mainnet-*"),
	}
	if cfg.TempDir != "" {
		env["TMPDIR"] = cfg.TempDir
	}
	if cfg.TrustHeight != "" {
		env["PAIR_ALIGN_TRUST_FROM_FIRST"] = "0"
	}
	if cfg.StopAtLocalHeight != "" {
		env["PAIR_ALIGN_STOP_HEIGHT_FROM_FIRST"] = "0"
	}
	return env
}

func collectCelestiaArtifacts(result *celestiaSyncResult) {
	outDir := result.OutputDir
	if outDir == "" {
		return
	}
	artifacts := map[string]string{}
	for name, path := range map[string]string{
		"summary":   filepath.Join(outDir, "summary.md"),
		"decision":  filepath.Join(outDir, "decision.json"),
		"runs_csv":  filepath.Join(outDir, "runs.csv"),
		"pairs_csv": filepath.Join(outDir, "pairs.csv"),
		"runs_dir":  filepath.Join(outDir, "runs"),
		"meta":      filepath.Join(outDir, "meta.txt"),
	} {
		if _, err := os.Stat(path); err == nil {
			artifacts[name] = path
		}
	}
	if len(artifacts) > 0 {
		result.Artifacts = artifacts
	}
	if body, err := os.ReadFile(filepath.Join(outDir, "summary.md")); err == nil {
		result.SummaryMarkdown = string(body)
	}
	if body, err := os.ReadFile(filepath.Join(outDir, "decision.json")); err == nil {
		var decision map[string]interface{}
		if err := json.Unmarshal(body, &decision); err == nil {
			result.Decision = decision
		}
	}
	if rows, err := readCSVRecords(filepath.Join(outDir, "runs.csv")); err == nil {
		result.RunsCSV = rows
	}
	if rows, err := readCSVRecords(filepath.Join(outDir, "pairs.csv")); err == nil {
		result.PairsCSV = rows
	}
	result.Runs = readCelestiaRunSummaries(filepath.Join(outDir, "runs"))
}

func readCelestiaRunSummaries(runsDir string) []celestiaABRunSummary {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return nil
	}
	var out []celestiaABRunSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(runsDir, entry.Name(), "run.json")
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var run celestiaABRunSummary
		if err := json.Unmarshal(body, &run); err != nil {
			run = celestiaABRunSummary{Path: path, Error: err.Error()}
		}
		run.Path = path
		attachCelestiaSyncTime(&run)
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PairIndex == out[j].PairIndex {
			return out[i].Variant < out[j].Variant
		}
		return out[i].PairIndex < out[j].PairIndex
	})
	return out
}

func attachCelestiaSyncTime(run *celestiaABRunSummary) {
	if run.RunHome == "" {
		return
	}
	syncTimePath := filepath.Join(run.RunHome, "sync", "sync-time.log")
	values, err := readKeyValueFile(syncTimePath)
	if err != nil || len(values) == 0 {
		return
	}
	run.SyncTimePath = syncTimePath
	run.SyncTime = values
	enrichCelestiaSyncFromKV(&run.Sync, values)
}

func enrichCelestiaSyncFromKV(sync *celestiaABSync, values map[string]string) {
	setStringIfPresent := func(dst *string, key string) {
		if value := strings.TrimSpace(values[key]); value != "" {
			*dst = value
		}
	}
	setInt64IfPresent := func(dst *int64, key string) {
		value, ok := parseInt64Value(values[key])
		if ok {
			*dst = value
		}
	}

	setStringIfPresent(&sync.StartUTC, "start_utc")
	setStringIfPresent(&sync.EndUTC, "end_utc")
	setStringIfPresent(&sync.DBBackend, "db_backend")
	setStringIfPresent(&sync.AppDBBackend, "app_db_backend")
	setStringIfPresent(&sync.TrustHash, "trust_hash")
	setStringIfPresent(&sync.AcceptedSnapshotSource, "accepted_snapshot_source")
	setStringIfPresent(&sync.AcceptedSnapshotHash, "accepted_snapshot_hash")
	setStringIfPresent(&sync.AcceptedSnapshotObservedUTC, "accepted_snapshot_observed_utc")

	setInt64IfPresent(&sync.TrustHeight, "trust_height")
	setInt64IfPresent(&sync.StopAtLocalHeight, "stop_at_local_height")
	setInt64IfPresent(&sync.FinalLocalHeight, "final_local_height")
	setInt64IfPresent(&sync.FinalRemoteHeight, "final_remote_height")
	setInt64IfPresent(&sync.FinalRemoteHeightActual, "final_remote_height_actual")
	setInt64IfPresent(&sync.BlocksSynced, "blocks_synced")
	setInt64IfPresent(&sync.MaxRSSKB, "max_rss_kb")
	setInt64IfPresent(&sync.MaxHWMKB, "max_hwm_kb")
	setInt64IfPresent(&sync.EndAppBytes, "end_app_bytes")
	setInt64IfPresent(&sync.EndDataBytes, "end_data_bytes")
	setInt64IfPresent(&sync.EndHomeBytes, "end_home_bytes")
	setInt64IfPresent(&sync.RequiredAcceptedSnapshotHeight, "required_accepted_snapshot_height")
	setInt64IfPresent(&sync.AcceptedSnapshotHeight, "accepted_snapshot_height")
	setInt64IfPresent(&sync.AcceptedSnapshotActualHeight, "accepted_snapshot_actual_height")
	setInt64IfPresent(&sync.AcceptedSnapshotRequiredHeight, "accepted_snapshot_required_height")
	setInt64IfPresent(&sync.AcceptedSnapshotFormat, "accepted_snapshot_format")

	if value := strings.TrimSpace(values["accepted_snapshot_mismatch"]); value != "" {
		sync.AcceptedSnapshotMismatch = value == "1" || strings.EqualFold(value, "true")
	}
}

func readKeyValueFile(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values[key] = strings.TrimSpace(value)
	}
	return values, nil
}

func parseInt64Value(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "disabled") || strings.EqualFold(raw, "unset") {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func readCSVRecords(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, nil
	}
	header := rows[0]
	var out []map[string]string
	for _, row := range rows[1:] {
		record := map[string]string{}
		for i, key := range header {
			if i < len(row) {
				record[key] = row[i]
			} else {
				record[key] = ""
			}
		}
		out = append(out, record)
	}
	return out, nil
}

func celestiaRepoMetadata(ctx context.Context, cfg celestiaSyncConfig) map[string]map[string]string {
	repos := map[string]string{
		"celestia_app": cfg.CelestiaAppDir,
		"gomap":        cfg.GomapDir,
		"cosmos_db":    cfg.CosmosDBDir,
		"comet_db":     cfg.CometDBDir,
		"cosmos_store": cfg.CosmosStoreDir,
		"cosmos_log":   cfg.CosmosLogDir,
		"cosmos_core":  cfg.CosmosCoreDir,
		"iavl":         cfg.IAVLDir,
	}
	out := map[string]map[string]string{}
	for name, path := range repos {
		if path == "" {
			continue
		}
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			continue
		}
		out[name] = gitMetadataForDir(ctx, path)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func gitMetadataForDir(ctx context.Context, dir string) map[string]string {
	meta := map[string]string{"path": dir}
	for key, args := range map[string][]string{
		"branch":   {"branch", "--show-current"},
		"head":     {"rev-parse", "HEAD"},
		"describe": {"describe", "--always", "--dirty"},
		"status":   {"status", "--short"},
		"remote":   {"remote", "get-url", "origin"},
	} {
		meta[key] = gitCommandString(ctx, dir, args...)
	}
	return meta
}

func startPhase(result *runResult, name, note string) func() {
	start := time.Now()
	return func() {
		end := time.Now()
		result.PhaseTimeline = append(result.PhaseTimeline, phaseSpan{
			Name:    name,
			Started: start,
			Ended:   end,
			Seconds: end.Sub(start).Seconds(),
			Note:    note,
		})
	}
}

func phaseSeconds(result runResult, name string) float64 {
	for _, span := range result.PhaseTimeline {
		if span.Name == name {
			return span.Seconds
		}
	}
	return 0
}

type loadWindowMonitor struct {
	cancel context.CancelFunc
	done   <-chan loadWindowObservation
	mu     sync.Mutex
	last   loadWindowObservation
}

func (m *loadWindowMonitor) Last() loadWindowObservation {
	if m == nil {
		return loadWindowObservation{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

func (m *loadWindowMonitor) setLast(obs loadWindowObservation) {
	m.mu.Lock()
	m.last = obs
	m.mu.Unlock()
}

func (m *loadWindowMonitor) Stop() loadWindowObservation {
	m.cancel()
	return <-m.done
}

func (m *loadWindowMonitor) Wait(timeout time.Duration) loadWindowObservation {
	if timeout <= 0 {
		return m.Stop()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case obs := <-m.done:
		return obs
	case <-timer.C:
		m.cancel()
		obs := <-m.done
		if obs.Error == "" && !obs.Reached {
			obs.Error = fmt.Sprintf("load window target not reached before drain timeout %s", timeout)
		}
		return obs
	}
}

func startLoadWindowMonitor(ctx context.Context, providerName string, baseline []metricSnapshot, targetTransactions int, minDuration, interval time.Duration) *loadWindowMonitor {
	started := time.Now()
	if targetTransactions <= 0 {
		done := make(chan loadWindowObservation, 1)
		obs := loadWindowObservation{
			TargetTransactions: targetTransactions,
			MinimumSeconds:     minDuration.Seconds(),
			DurationSatisfied:  minDuration <= 0,
			StartedAt:          started,
			EndedAt:            time.Now(),
			MetricsBefore:      cloneMetricSnapshots(baseline),
			Error:              "target transaction count is not positive",
		}
		done <- obs
		monitor := &loadWindowMonitor{cancel: func() {}, done: done}
		monitor.setLast(obs)
		return monitor
	}
	monitorCtx, cancel := context.WithCancel(ctx)
	done := make(chan loadWindowObservation, 1)
	monitor := &loadWindowMonitor{cancel: cancel, done: done}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		last := loadWindowObservation{
			TargetTransactions: targetTransactions,
			MinimumSeconds:     minDuration.Seconds(),
			DurationSatisfied:  minDuration <= 0,
			StartedAt:          started,
			MetricsBefore:      cloneMetricSnapshots(baseline),
		}
		observe := func() loadWindowObservation {
			metrics := scrapeMetrics(monitorCtx, providerName)
			signals := summarizeStorageSignals(baseline, metrics, nil, nil)
			included, successful, candidates, ok := appMetricLoadCounts(signals)
			ended := time.Now()
			obs := loadWindowObservation{
				TargetTransactions:     targetTransactions,
				MinimumSeconds:         minDuration.Seconds(),
				DurationSatisfied:      minDuration <= 0 || ended.Sub(started) >= minDuration,
				StartedAt:              started,
				EndedAt:                ended,
				Seconds:                ended.Sub(started).Seconds(),
				IncludedTransactions:   included,
				SuccessfulTransactions: successful,
				AppMetricCandidates:    candidates,
				MetricsBefore:          cloneMetricSnapshots(baseline),
				MetricsAfter:           metrics,
				MetricDeltas:           metricDeltaSnapshots(baseline, metrics),
				StorageSignals:         signals,
			}
			if ok && successful >= targetTransactions {
				obs.Reached = true
				if !obs.DurationSatisfied {
					obs.Error = fmt.Sprintf("load window reached in %.3fs, below minimum duration %s; increase offered workload", obs.Seconds, minDuration)
				}
			}
			return obs
		}
		for {
			last = observe()
			monitor.setLast(last)
			if last.Reached {
				done <- last
				return
			}
			select {
			case <-ticker.C:
			case <-monitorCtx.Done():
				if last.Error == "" {
					last.Error = monitorCtx.Err().Error()
				}
				monitor.setLast(last)
				done <- last
				return
			}
		}
	}()
	return monitor
}

type appProfileRequest struct {
	Kind     string
	Endpoint string
	FileName string
	OutDir   string
	Boundary string
}

func (req appProfileRequest) artifact(scenarioName string) profileArtifact {
	return profileArtifact{
		Name:     scenarioName,
		Kind:     req.Kind,
		Boundary: req.Boundary,
	}
}

func collectAppProfiles(ctx context.Context, providerState, chainState []byte, sc scenario, cpuOutDir, heapOutDir, pprofOutDir string) []profileArtifact {
	requests := appProfileRequests(sc, cpuOutDir, heapOutDir, pprofOutDir)
	if len(requests) == 0 {
		return nil
	}
	artifacts := make([]profileArtifact, 0, len(requests))
	for _, req := range requests {
		artifacts = append(artifacts, req.artifact(sc.Name))
	}
	logger, _ := zap.NewDevelopment()
	decompressedProviderState, err := util.DecompressData(providerState)
	if err != nil {
		return failProfileArtifacts(artifacts, fmt.Sprintf("decompress provider state: %v", err))
	}
	decompressedChainState, err := util.DecompressData(chainState)
	if err != nil {
		return failProfileArtifacts(artifacts, fmt.Sprintf("decompress chain state: %v", err))
	}
	p, err := util.RestoreProvider(ctx, logger, messages.Docker, decompressedProviderState, util.ProviderOptions{})
	if err != nil {
		return failProfileArtifacts(artifacts, fmt.Sprintf("restore provider: %v", err))
	}
	walletConfig := testnet.CosmosWalletConfig
	if sc.IsEVMChain {
		walletConfig = testnet.EvmCosmosWalletConfig
	}
	ch, err := chain.RestoreChain(ctx, logger, p, decompressedChainState, node.RestoreNode, walletConfig)
	if err != nil {
		return failProfileArtifacts(artifacts, fmt.Sprintf("restore chain: %v", err))
	}
	validators := ch.GetValidators()
	if len(validators) == 0 {
		return failProfileArtifacts(artifacts, "no validators")
	}

	for i, req := range requests {
		switch req.Kind {
		case "validator_cpu":
			artifacts[i] = collectAppCPUProfile(ctx, validators[0], sc, req)
		default:
			artifacts[i] = collectAppPprofEndpointProfile(ctx, validators[0], sc, req)
		}
	}
	return attachPprofTopSummaries(ctx, artifacts)
}

func appProfileRequests(sc scenario, cpuOutDir, heapOutDir, pprofOutDir string) []appProfileRequest {
	var requests []appProfileRequest
	if heapOutDir != "" && sc.AppHeapProfile != "" {
		requests = append(requests, appProfileRequest{
			Kind:     "validator_heap",
			Endpoint: "heap?gc=1",
			FileName: sc.AppHeapProfile,
			OutDir:   heapOutDir,
			Boundary: profileBoundaryLoadWindowAdjacent,
		})
	}
	if pprofOutDir != "" && sc.AppPprofListen != "" {
		for _, profile := range []struct {
			kind     string
			endpoint string
			suffix   string
			boundary string
		}{
			{kind: "validator_heap", endpoint: "heap?gc=1", suffix: "heap", boundary: profileBoundaryLoadWindowAdjacent},
			{kind: "validator_allocs", endpoint: "allocs", suffix: "allocs", boundary: profileBoundaryWholeRun},
			{kind: "validator_block", endpoint: "block", suffix: "block", boundary: profileBoundaryWholeRun},
			{kind: "validator_mutex", endpoint: "mutex", suffix: "mutex", boundary: profileBoundaryWholeRun},
			{kind: "validator_goroutine", endpoint: "goroutine", suffix: "goroutine", boundary: profileBoundaryLoadWindowAdjacent},
		} {
			fileName := appPprofFileName(sc, profile.suffix)
			if profile.kind == "validator_heap" && heapOutDir == pprofOutDir && fileName == sc.AppHeapProfile {
				continue
			}
			requests = append(requests, appProfileRequest{
				Kind:     profile.kind,
				Endpoint: profile.endpoint,
				FileName: fileName,
				OutDir:   pprofOutDir,
				Boundary: profile.boundary,
			})
		}
	}
	if cpuOutDir != "" && sc.AppCPUProfile != "" {
		requests = append(requests, appProfileRequest{
			Kind:     "validator_cpu",
			FileName: sc.AppCPUProfile,
			OutDir:   cpuOutDir,
			Boundary: profileBoundaryWholeRun,
		})
	}
	return requests
}

func appPprofFileName(sc scenario, suffix string) string {
	return fmt.Sprintf("%s-validator-0-%s.pprof", sanitize(sc.Name), suffix)
}

type validatorConfigReader interface {
	ReadFile(context.Context, string) ([]byte, error)
}

func verifySimappBackends(ctx context.Context, providerState, chainState []byte, sc scenario) backendVerification {
	verification := backendVerification{
		ExpectedAppDBBackend:  sc.AppDBBackend,
		ExpectedNodeDBBackend: sc.NodeDBBackend,
		AppConfigPath:         "config/app.toml",
		NodeConfigPath:        "config/config.toml",
	}
	validator, err := restoreFirstValidator(ctx, providerState, chainState, sc)
	if err != nil {
		verification.Error = err.Error()
		return verification
	}
	appConfig, err := readValidatorToml(ctx, validator, verification.AppConfigPath)
	if err != nil {
		verification.Error = fmt.Sprintf("read app backend config: %v", err)
		return verification
	}
	nodeConfig, err := readValidatorToml(ctx, validator, verification.NodeConfigPath)
	if err != nil {
		verification.Error = fmt.Sprintf("read node backend config: %v", err)
		return verification
	}
	verification.ObservedAppDBBackend = tomlString(appConfig, "app-db-backend")
	verification.ObservedNodeDBBackend = tomlString(nodeConfig, "db_backend")
	verification.Valid, verification.Error = validateBackendVerification(verification)
	return verification
}

func restoreFirstValidator(ctx context.Context, providerState, chainState []byte, sc scenario) (validatorConfigReader, error) {
	logger, _ := zap.NewDevelopment()
	decompressedProviderState, err := util.DecompressData(providerState)
	if err != nil {
		return nil, fmt.Errorf("decompress provider state: %w", err)
	}
	decompressedChainState, err := util.DecompressData(chainState)
	if err != nil {
		return nil, fmt.Errorf("decompress chain state: %w", err)
	}
	p, err := util.RestoreProvider(ctx, logger, messages.Docker, decompressedProviderState, util.ProviderOptions{})
	if err != nil {
		return nil, fmt.Errorf("restore provider: %w", err)
	}
	walletConfig := testnet.CosmosWalletConfig
	if sc.IsEVMChain {
		walletConfig = testnet.EvmCosmosWalletConfig
	}
	ch, err := chain.RestoreChain(ctx, logger, p, decompressedChainState, node.RestoreNode, walletConfig)
	if err != nil {
		return nil, fmt.Errorf("restore chain: %w", err)
	}
	validators := ch.GetValidators()
	if len(validators) == 0 {
		return nil, errors.New("no validators")
	}
	return validators[0], nil
}

func readValidatorToml(ctx context.Context, reader validatorConfigReader, path string) (map[string]interface{}, error) {
	body, err := reader.ReadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := toml.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func tomlString(cfg map[string]interface{}, key string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func validateBackendVerification(verification backendVerification) (bool, string) {
	var problems []string
	if verification.ExpectedAppDBBackend != "" && verification.ObservedAppDBBackend != verification.ExpectedAppDBBackend {
		problems = append(problems, fmt.Sprintf("app-db-backend observed %q, want %q", verification.ObservedAppDBBackend, verification.ExpectedAppDBBackend))
	}
	if verification.ExpectedNodeDBBackend != "" && verification.ObservedNodeDBBackend != verification.ExpectedNodeDBBackend {
		problems = append(problems, fmt.Sprintf("db_backend observed %q, want %q", verification.ObservedNodeDBBackend, verification.ExpectedNodeDBBackend))
	}
	if len(problems) > 0 {
		return false, strings.Join(problems, "; ")
	}
	return true, ""
}

func failProfileArtifacts(artifacts []profileArtifact, message string) []profileArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	for i := range artifacts {
		artifacts[i].Error = message
	}
	return artifacts
}

func collectAppPprofEndpointProfile(ctx context.Context, validator interface {
	RunCommand(context.Context, []string) (string, string, int, error)
	ReadFile(context.Context, string) ([]byte, error)
}, sc scenario, req appProfileRequest) profileArtifact {
	artifact := req.artifact(sc.Name)
	if err := os.MkdirAll(req.OutDir, 0o755); err != nil {
		artifact.Error = fmt.Sprintf("create profile dir: %v", err)
		return artifact
	}
	remotePath := filepath.Join("/simd", req.FileName)
	endpointURL := "http://127.0.0.1:6060/debug/pprof/" + req.Endpoint
	cmd := []string{
		"sh",
		"-c",
		fmt.Sprintf("wget -q -O %s %s", shellQuote(remotePath), shellQuote(endpointURL)),
	}
	stdout, stderr, code, err := validator.RunCommand(ctx, cmd)
	if err != nil {
		artifact.Error = fmt.Sprintf("fetch %s profile: %v stdout=%q stderr=%q", req.Kind, err, trimLog(stdout, 2000), trimLog(stderr, 2000))
		return artifact
	}
	if code != 0 {
		artifact.Error = fmt.Sprintf("fetch %s profile exit %d stdout=%q stderr=%q", req.Kind, code, trimLog(stdout, 2000), trimLog(stderr, 2000))
		return artifact
	}
	body, err := validator.ReadFile(ctx, req.FileName)
	if err != nil {
		artifact.Error = fmt.Sprintf("read %s profile: %v", req.Kind, err)
		return artifact
	}
	localPath := filepath.Join(req.OutDir, req.FileName)
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		artifact.Error = fmt.Sprintf("write %s profile: %v", req.Kind, err)
		return artifact
	}
	artifact.Path = localPath
	return artifact
}

func collectAppCPUProfile(ctx context.Context, validator interface {
	Stop(context.Context) error
	ReadFile(context.Context, string) ([]byte, error)
}, sc scenario, req appProfileRequest) profileArtifact {
	artifact := req.artifact(sc.Name)
	if err := os.MkdirAll(req.OutDir, 0o755); err != nil {
		artifact.Error = fmt.Sprintf("create profile dir: %v", err)
		return artifact
	}
	if err := validator.Stop(ctx); err != nil {
		artifact.Error = fmt.Sprintf("stop validator: %v", err)
		return artifact
	}
	body, err := validator.ReadFile(ctx, req.FileName)
	if err != nil {
		artifact.Error = fmt.Sprintf("read validator profile: %v", err)
		return artifact
	}
	localPath := filepath.Join(req.OutDir, req.FileName)
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		artifact.Error = fmt.Sprintf("write validator profile: %v", err)
		return artifact
	}
	artifact.Path = localPath
	return artifact
}

func attachPprofTopSummaries(ctx context.Context, artifacts []profileArtifact) []profileArtifact {
	for i := range artifacts {
		if artifacts[i].Path == "" || artifacts[i].Error != "" {
			continue
		}
		summaryPath := artifacts[i].Path + ".top.txt"
		out, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", artifacts[i].Path).CombinedOutput()
		if err != nil {
			artifacts[i].TopSummaryError = fmt.Sprintf("go tool pprof -top: %v output=%q", err, trimLog(string(out), 2000))
			continue
		}
		if err := os.WriteFile(summaryPath, out, 0o644); err != nil {
			artifacts[i].TopSummaryError = fmt.Sprintf("write pprof top summary: %v", err)
			continue
		}
		artifacts[i].TopSummaryPath = summaryPath
	}
	return artifacts
}

func runCommitBenchmark(ctx context.Context, providerState, chainState []byte, sc scenario, blocks uint64) (*commitBenchmark, error) {
	if blocks == 0 {
		return nil, nil
	}
	logger, _ := zap.NewDevelopment()
	decompressedProviderState, err := util.DecompressData(providerState)
	if err != nil {
		return nil, fmt.Errorf("decompress provider state: %w", err)
	}
	decompressedChainState, err := util.DecompressData(chainState)
	if err != nil {
		return nil, fmt.Errorf("decompress chain state: %w", err)
	}
	p, err := util.RestoreProvider(ctx, logger, messages.Docker, decompressedProviderState, util.ProviderOptions{})
	if err != nil {
		return nil, fmt.Errorf("restore provider: %w", err)
	}
	walletConfig := testnet.CosmosWalletConfig
	if sc.IsEVMChain {
		walletConfig = testnet.EvmCosmosWalletConfig
	}
	ch, err := chain.RestoreChain(ctx, logger, p, decompressedChainState, node.RestoreNode, walletConfig)
	if err != nil {
		return nil, fmt.Errorf("restore chain: %w", err)
	}
	startHeight, err := ch.Height(ctx)
	if err != nil {
		return nil, fmt.Errorf("read start height: %w", err)
	}
	if err := ch.WaitForBlocks(ctx, blocks); err != nil {
		return nil, fmt.Errorf("wait for %d blocks from height %d: %w", blocks, startHeight, err)
	}
	endHeight, err := ch.Height(ctx)
	if err != nil {
		return nil, fmt.Errorf("read end height: %w", err)
	}
	return &commitBenchmark{
		Blocks:      blocks,
		StartHeight: startHeight,
		EndHeight:   endHeight,
	}, nil
}

func buildImage(ctx context.Context, sc scenario) ([]string, string, error) {
	args := []string{
		"build",
		"-f", sc.Dockerfile,
		"-t", sc.ImageTag,
		"--build-arg", "GIT_SHA=" + sanitize(sc.Name) + "-" + shortRef(sc.ChainRef),
		"--build-arg", "CHAIN_SRC=" + sc.ChainSource,
		"--build-arg", "CHAIN_TAG=" + sc.ChainRef,
	}
	if sc.BaseImage == "simapp" && strings.Contains(sc.ChainSource, "snissn/celestia-cosmos-sdk") {
		args = append(args, "--build-arg", "GO_IMAGE=golang:1.26-alpine")
	}
	if sc.ReplaceCmd != "" {
		args = append(args, "--build-arg", "REPLACE_CMD="+sc.ReplaceCmd)
	}
	args = append(args, "hack")
	cmd := exec.CommandContext(ctx, "docker", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return append([]string{"docker"}, args...), buf.String(), err
}

func selectScenarios(all []scenario, want string) []scenario {
	if want == "all" {
		var selected []scenario
		for _, sc := range all {
			if sc.IncludeInAll {
				selected = append(selected, sc)
			}
		}
		return selected
	}
	if want == "all-with-celestia" {
		return all
	}
	for _, sc := range all {
		if sc.Name == want {
			return []scenario{sc}
		}
	}
	fatalf("unknown scenario %q", want)
	return nil
}

func needsWorkerConfig(scenarios []scenario) bool {
	for _, sc := range scenarios {
		if sc.Runner == "celestia-sync-ab" {
			continue
		}
		return true
	}
	return false
}

func dockerStats(ctx context.Context, providerName string) []json.RawMessage {
	names := dockerContainerNames(ctx, providerName, false)
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, names...)
	statsOut, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil
	}
	var rows []json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(string(statsOut)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, json.RawMessage(line))
	}
	return rows
}

func dockerLogs(ctx context.Context, providerName string, limit int) map[string]string {
	names := dockerContainerNames(ctx, providerName, true)
	if len(names) == 0 {
		return nil
	}
	logs := make(map[string]string, len(names))
	for _, name := range names {
		out, err := exec.CommandContext(ctx, "docker", "logs", "--tail", "300", name).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil && text == "" {
			text = err.Error()
		}
		if text == "" {
			continue
		}
		logs[name] = trimLog(text, limit)
	}
	if len(logs) == 0 {
		return nil
	}
	return logs
}

func dockerContainerNames(ctx context.Context, providerName string, all bool) []string {
	args := []string{"ps"}
	if all {
		args = append(args, "-a")
	}
	args = append(args, "--filter", "label=petri-provider="+providerName, "--format", "{{.Names}}")
	namesOut, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(namesOut))
}

type dockerStatsSampler struct {
	cancel context.CancelFunc
	done   chan []resourceSample
}

func startDockerStatsSampler(parent context.Context, providerName string, interval time.Duration) *dockerStatsSampler {
	if interval <= 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan []resourceSample, 1)
	go func() {
		defer close(done)
		samples := make([]resourceSample, 0, 128)
		take := func() {
			stats := dockerStats(ctx, providerName)
			if len(stats) == 0 {
				return
			}
			samples = append(samples, resourceSample{
				At:    time.Now(),
				Stats: stats,
			})
		}
		take()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				take()
				done <- samples
				return
			case <-ticker.C:
				take()
			}
		}
	}()
	return &dockerStatsSampler{cancel: cancel, done: done}
}

func (s *dockerStatsSampler) Stop() []resourceSample {
	s.cancel()
	samples, ok := <-s.done
	if !ok {
		return nil
	}
	return samples
}

func summarizeResourceSamples(samples []resourceSample) []resourceSummary {
	if len(samples) == 0 {
		return nil
	}
	byName := map[string]*resourceSummary{}
	for _, sample := range samples {
		for _, raw := range sample.Stats {
			var row map[string]string
			if err := json.Unmarshal(raw, &row); err != nil {
				continue
			}
			name := row["Name"]
			if name == "" {
				name = row["Container"]
			}
			if name == "" {
				continue
			}
			summary := byName[name]
			if summary == nil {
				summary = &resourceSummary{Name: name}
				byName[name] = summary
			}
			summary.Samples++
			if cpu := percentValue(row["CPUPerc"]); cpu > summary.MaxCPUPerc {
				summary.MaxCPUPerc = cpu
			}
			if memPercent := percentValue(row["MemPerc"]); memPercent > summary.MaxMemPerc {
				summary.MaxMemPerc = memPercent
			}
			if usage, usageText := memoryUsage(row["MemUsage"]); usage > summary.MaxMemUsageBytes {
				summary.MaxMemUsageBytes = usage
				summary.MaxMemUsage = usageText
			}
			if row["NetIO"] != "" {
				summary.LastNetIO = row["NetIO"]
			}
			if row["BlockIO"] != "" {
				summary.LastBlockIO = row["BlockIO"]
				readBytes, writeBytes := ioPairBytes(row["BlockIO"])
				if readBytes > summary.MaxBlockReadBytes {
					summary.MaxBlockReadBytes = readBytes
				}
				if writeBytes > summary.MaxBlockWriteBytes {
					summary.MaxBlockWriteBytes = writeBytes
				}
			}
		}
	}
	out := make([]resourceSummary, 0, len(byName))
	for _, summary := range byName {
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func scrapeMetrics(ctx context.Context, providerName string) []metricSnapshot {
	names := dockerContainerNames(ctx, providerName, false)
	if len(names) == 0 {
		return nil
	}
	client := &http.Client{Timeout: 2 * time.Second}
	var out []metricSnapshot
	for _, name := range names {
		if strings.Contains(name, "catalyst") {
			continue
		}
		url := containerMetricsURL(ctx, name)
		if url == "" {
			continue
		}
		snapshot := metricSnapshot{Name: name, URL: url}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			snapshot.Error = err.Error()
			out = append(out, snapshot)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			snapshot.Error = err.Error()
			out = append(out, snapshot)
			continue
		}
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snapshot.Error = resp.Status
			out = append(out, snapshot)
			continue
		}
		snapshot.Metrics = parsePrometheusMetrics(buf.String())
		out = append(out, snapshot)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func containerMetricsURL(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name).Output()
	if err == nil {
		if ip := strings.TrimSpace(string(out)); ip != "" {
			return "http://" + ip + ":26660/metrics"
		}
	}
	out, err = exec.CommandContext(ctx, "docker", "inspect", "--format", `{{with index .NetworkSettings.Ports "26660/tcp"}}{{(index . 0).HostPort}}{{end}}`, name).Output()
	if err != nil {
		return ""
	}
	if port := strings.TrimSpace(string(out)); port != "" {
		return "http://127.0.0.1:" + port + "/metrics"
	}
	return ""
}

func parsePrometheusMetrics(text string) map[string]float64 {
	metrics := map[string]float64{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := prometheusMetricName(fields[0])
		if !interestingMetric(name) {
			continue
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		metrics[fields[0]] = value
	}
	return metrics
}

func prometheusMetricName(expr string) string {
	if idx := strings.IndexByte(expr, '{'); idx >= 0 {
		return expr[:idx]
	}
	return expr
}

func interestingMetric(name string) bool {
	if name == "" {
		return false
	}
	for _, prefix := range []string{
		"treedb_",
		"tree_db_",
		"cometbft_",
		"tendermint_",
		"process_",
		"go_",
		"runtime_",
		"mempool_",
		"consensus_",
		"abci_",
		"blockstore_",
		"block_store_",
		"tx_index_",
		"indexer_",
		"sdk_",
		"cosmos_",
		"app_",
		"tx_",
		"begin_blocker_",
		"end_blocker_",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func collectDataSizes(ctx context.Context, providerName string, sc scenario) []dataPathSize {
	if sc.BaseImage != "simapp" {
		return nil
	}
	names := dockerContainerNames(ctx, providerName, false)
	if len(names) == 0 {
		return nil
	}
	paths := []string{
		"/simd/config/genesis.json",
		"/simd/data",
		"/simd/data/application.db",
		"/simd/data/blockstore.db",
		"/simd/data/state.db",
		"/simd/data/tx_index.db",
	}
	var out []dataPathSize
	for _, name := range names {
		if strings.Contains(name, "catalyst") {
			continue
		}
		script := "for p in " + strings.Join(paths, " ") + "; do if [ -e \"$p\" ]; then du -sb \"$p\"; fi; done"
		raw, err := exec.CommandContext(ctx, "docker", "exec", name, "sh", "-c", script).CombinedOutput()
		if err != nil {
			out = append(out, dataPathSize{Name: name, Path: "/simd/data", Error: strings.TrimSpace(string(raw))})
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			bytes, err := strconv.ParseUint(fields[0], 10, 64)
			if err != nil {
				continue
			}
			out = append(out, dataPathSize{Name: name, Path: fields[1], Bytes: bytes})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func summarizeStorageSignals(before, after []metricSnapshot, beforeSizes, afterSizes []dataPathSize) []storageSignal {
	beforeByName := metricSnapshotsByName(before)
	var out []storageSignal
	for _, afterSnap := range after {
		beforeSnap := beforeByName[afterSnap.Name]
		if beforeSnap == nil || len(afterSnap.Metrics) == 0 {
			continue
		}
		deltas := metricDeltas(beforeSnap.Metrics, afterSnap.Metrics)
		signal := storageSignal{Name: afterSnap.Name}
		signal.ABCIObservedSeconds = sumMetricDeltas(deltas, "cometbft_abci_connection_method_timing_seconds_sum")
		signal.ABCICommitSeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "commit")
		signal.ABCIFinalizeBlockSeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "finalize_block")
		signal.ABCICheckTxSeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "check_tx")
		signal.ABCIPrepareProposalSeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "prepare_proposal")
		signal.ABCIProcessProposalSeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "process_proposal")
		signal.ABCIQuerySeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "query")
		signal.ABCIFlushSeconds = metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_sum", "method", "flush")
		signal.ABCICommitCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "commit"))
		signal.ABCIFinalizeBlockCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "finalize_block"))
		signal.ABCICheckTxCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "check_tx"))
		signal.ABCIPrepareProposalCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "prepare_proposal"))
		signal.ABCIProcessProposalCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "process_proposal"))
		signal.ABCIQueryCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "query"))
		signal.ABCIFlushCount = int(metricDeltaWithLabel(deltas, "cometbft_abci_connection_method_timing_seconds_count", "method", "flush"))
		signal.ABCIOtherSeconds = nonNegative(signal.ABCIObservedSeconds -
			signal.ABCICommitSeconds -
			signal.ABCIFinalizeBlockSeconds -
			signal.ABCICheckTxSeconds -
			signal.ABCIPrepareProposalSeconds -
			signal.ABCIProcessProposalSeconds -
			signal.ABCIQuerySeconds -
			signal.ABCIFlushSeconds)
		if signal.ABCICommitCount > 0 {
			signal.AvgCommitSeconds = signal.ABCICommitSeconds / float64(signal.ABCICommitCount)
		}
		if signal.ABCIFinalizeBlockCount > 0 {
			signal.AvgFinalizeBlockSeconds = signal.ABCIFinalizeBlockSeconds / float64(signal.ABCIFinalizeBlockCount)
		}
		if signal.ABCIObservedSeconds > 0 {
			signal.CommitShareOfObservedABCI = signal.ABCICommitSeconds / signal.ABCIObservedSeconds
		}
		if total := signal.ABCICommitSeconds + signal.ABCIFinalizeBlockSeconds; total > 0 {
			signal.CommitShareOfCommitPlusFinalize = signal.ABCICommitSeconds / total
		}
		signal.StateBlockProcessingSumRaw = metricDelta(deltas, "cometbft_state_block_processing_time_sum")
		signal.StateBlockProcessingCount = int(metricDelta(deltas, "cometbft_state_block_processing_time_count"))
		signal.ConsensusBlockIntervalSeconds = metricDelta(deltas, "cometbft_consensus_block_interval_seconds_sum")
		signal.ConsensusBlockIntervalCount = int(metricDelta(deltas, "cometbft_consensus_block_interval_seconds_count"))
		signal.ConsensusTotalTxsDelta = metricDelta(deltas, "cometbft_consensus_total_txs")
		signal.MempoolSuccessfulTxsDelta = metricDelta(deltas, "cometbft_mempool_successful_txs")
		signal.SDKTxCountDelta = metricDelta(deltas, "tx_count")
		signal.SDKTxSuccessfulDelta = metricDelta(deltas, "tx_successful")
		signal.ProcessCPUSecondsDelta = metricDelta(deltas, "process_cpu_seconds_total")
		fillSizeDelta(&signal, afterSnap.Name, beforeSizes, afterSizes)
		signal.ModuleTimings = summarizeModuleTimings(deltas)
		out = append(out, signal)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func summarizeModuleTimings(deltas map[string]float64) []moduleTiming {
	var out []moduleTiming
	for _, phase := range []string{"begin_blocker", "end_blocker"} {
		sumMetric := phase + "_sum"
		countMetric := phase + "_count"
		for key, seconds := range deltas {
			if prometheusMetricName(key) != sumMetric || seconds <= 0 {
				continue
			}
			module := labelValue(key, "module")
			if module == "" {
				module = "unknown"
			}
			count := int(metricDeltaWithLabel(deltas, countMetric, "module", module))
			timing := moduleTiming{
				Phase:   phase,
				Module:  module,
				Seconds: seconds,
				Count:   count,
			}
			if count > 0 {
				timing.AvgSeconds = seconds / float64(count)
			}
			out = append(out, timing)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seconds == out[j].Seconds {
			if out[i].Phase == out[j].Phase {
				return out[i].Module < out[j].Module
			}
			return out[i].Phase < out[j].Phase
		}
		return out[i].Seconds > out[j].Seconds
	})
	return out
}

func cloneMetricSnapshots(snapshots []metricSnapshot) []metricSnapshot {
	if len(snapshots) == 0 {
		return nil
	}
	out := make([]metricSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		copied := metricSnapshot{
			Name:  snapshot.Name,
			URL:   snapshot.URL,
			Error: snapshot.Error,
		}
		if len(snapshot.Metrics) > 0 {
			copied.Metrics = make(map[string]float64, len(snapshot.Metrics))
			for key, value := range snapshot.Metrics {
				copied.Metrics[key] = value
			}
		}
		out = append(out, copied)
	}
	return out
}

func metricDeltaSnapshots(before, after []metricSnapshot) []metricDeltaSnapshot {
	beforeByName := metricSnapshotsByName(before)
	afterByName := metricSnapshotsByName(after)
	nameSet := map[string]struct{}{}
	for _, snapshot := range before {
		nameSet[snapshot.Name] = struct{}{}
	}
	for _, snapshot := range after {
		nameSet[snapshot.Name] = struct{}{}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]metricDeltaSnapshot, 0, len(names))
	for _, name := range names {
		beforeSnap := beforeByName[name]
		afterSnap := afterByName[name]
		delta := metricDeltaSnapshot{Name: name}
		if beforeSnap != nil {
			delta.URL = beforeSnap.URL
			delta.Error = appendSnapshotError(delta.Error, "before", beforeSnap.Error)
		}
		if afterSnap != nil {
			if delta.URL == "" {
				delta.URL = afterSnap.URL
			}
			delta.Error = appendSnapshotError(delta.Error, "after", afterSnap.Error)
		}
		metricKeys := map[string]struct{}{}
		if beforeSnap != nil {
			for key := range beforeSnap.Metrics {
				metricKeys[key] = struct{}{}
			}
		}
		if afterSnap != nil {
			for key := range afterSnap.Metrics {
				metricKeys[key] = struct{}{}
			}
		}
		if metricSnapshotsComparable(beforeSnap, afterSnap) && len(metricKeys) > 0 {
			delta.Metrics = make(map[string]metricDeltaValue, len(metricKeys))
			for key := range metricKeys {
				var beforeValue, afterValue float64
				if beforeSnap != nil {
					beforeValue = beforeSnap.Metrics[key]
				}
				if afterSnap != nil {
					afterValue = afterSnap.Metrics[key]
				}
				delta.Metrics[key] = metricDeltaValue{
					Before: beforeValue,
					After:  afterValue,
					Delta:  afterValue - beforeValue,
				}
			}
		}
		if len(delta.Metrics) > 0 || delta.Error != "" {
			out = append(out, delta)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metricSnapshotsComparable(beforeSnap, afterSnap *metricSnapshot) bool {
	if beforeSnap == nil || afterSnap == nil {
		return false
	}
	if beforeSnap.Error != "" || afterSnap.Error != "" {
		return false
	}
	return len(beforeSnap.Metrics) > 0 && len(afterSnap.Metrics) > 0
}

func appendSnapshotError(current, phase, errText string) string {
	if errText == "" {
		return current
	}
	addition := phase + ": " + errText
	if current == "" {
		return addition
	}
	return current + "; " + addition
}

func metricSnapshotsByName(snapshots []metricSnapshot) map[string]*metricSnapshot {
	out := make(map[string]*metricSnapshot, len(snapshots))
	for i := range snapshots {
		out[snapshots[i].Name] = &snapshots[i]
	}
	return out
}

func metricDeltas(before, after map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for key, afterValue := range after {
		out[key] = afterValue - before[key]
	}
	return out
}

func sumMetricDeltas(deltas map[string]float64, name string) float64 {
	var total float64
	for key, value := range deltas {
		if prometheusMetricName(key) == name {
			total += value
		}
	}
	return total
}

func metricDelta(deltas map[string]float64, name string) float64 {
	for key, value := range deltas {
		if prometheusMetricName(key) == name {
			return value
		}
	}
	return 0
}

func metricDeltaWithLabel(deltas map[string]float64, name, label, value string) float64 {
	needle := label + `="` + value + `"`
	for key, delta := range deltas {
		if prometheusMetricName(key) == name && strings.Contains(key, needle) {
			return delta
		}
	}
	return 0
}

func labelValue(expr, label string) string {
	start := strings.IndexByte(expr, '{')
	end := strings.LastIndexByte(expr, '}')
	if start < 0 || end <= start {
		return ""
	}
	needle := label + `="`
	labels := expr[start+1 : end]
	idx := strings.Index(labels, needle)
	if idx < 0 {
		return ""
	}
	valueStart := idx + len(needle)
	valueEnd := strings.IndexByte(labels[valueStart:], '"')
	if valueEnd < 0 {
		return ""
	}
	return labels[valueStart : valueStart+valueEnd]
}

func nonNegative(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

func fillSizeDelta(signal *storageSignal, name string, beforeSizes, afterSizes []dataPathSize) {
	beforeByPath := pathSizesForName(beforeSizes, name)
	afterByPath := pathSizesForName(afterSizes, name)
	set := func(path string, before *uint64, after *uint64, delta *int64) {
		b, bok := beforeByPath[path]
		a, aok := afterByPath[path]
		if bok {
			*before = b
		}
		if aok {
			*after = a
		}
		if bok && aok {
			*delta = int64(a) - int64(b)
		}
	}
	set("/simd/data", &signal.DataDirBytesBefore, &signal.DataDirBytesAfter, &signal.DataDirBytesDelta)
	set("/simd/data/application.db", &signal.ApplicationDBBytesBefore, &signal.ApplicationDBBytesAfter, &signal.ApplicationDBBytesDelta)
}

func pathSizesForName(sizes []dataPathSize, name string) map[string]uint64 {
	out := map[string]uint64{}
	for _, size := range sizes {
		if size.Name == name && size.Error == "" {
			out[size.Path] = size.Bytes
		}
	}
	return out
}

func summarizeRuntimeBreakdown(result runResult) []runtimeBreakdown {
	if len(result.StorageSignals) == 0 {
		return nil
	}
	workloadRuntime := loadRuntimeSeconds(result)
	if workloadRuntime == 0 && result.RawTxSummary != nil && result.RawTxSummary.TPS > 0 {
		workloadRuntime = float64(result.RawTxSummary.Successful) / result.RawTxSummary.TPS
	}
	out := make([]runtimeBreakdown, 0, len(result.StorageSignals))
	for _, signal := range result.StorageSignals {
		breakdown := runtimeBreakdown{
			Name:                         signal.Name,
			WallSeconds:                  result.WallSeconds,
			LaunchSeconds:                result.LaunchSeconds,
			WorkloadRuntimeSeconds:       workloadRuntime,
			PostLaunchNonWorkloadSeconds: nonNegative(result.WallSeconds - result.LaunchSeconds - workloadRuntime),
			ABCIObservedSeconds:          signal.ABCIObservedSeconds,
			ABCICommitSeconds:            signal.ABCICommitSeconds,
			ABCIFinalizeBlockSeconds:     signal.ABCIFinalizeBlockSeconds,
			ABCICheckTxSeconds:           signal.ABCICheckTxSeconds,
			ABCIProposalSeconds:          signal.ABCIPrepareProposalSeconds + signal.ABCIProcessProposalSeconds,
			ABCIQuerySeconds:             signal.ABCIQuerySeconds,
			ABCIFlushSeconds:             signal.ABCIFlushSeconds,
			ABCIOtherSeconds:             signal.ABCIOtherSeconds,
		}
		if workloadRuntime > 0 {
			breakdown.NonABCIWorkloadSeconds = nonNegative(workloadRuntime - signal.ABCIObservedSeconds)
			breakdown.CommitPctOfWorkload = 100 * signal.ABCICommitSeconds / workloadRuntime
			breakdown.FinalizeBlockPctOfWorkload = 100 * signal.ABCIFinalizeBlockSeconds / workloadRuntime
			breakdown.CommitPlusFinalizePctOfWorkload = 100 * (signal.ABCICommitSeconds + signal.ABCIFinalizeBlockSeconds) / workloadRuntime
			breakdown.ObservedABCIPctOfWorkload = 100 * signal.ABCIObservedSeconds / workloadRuntime
			breakdown.NonABCIPctOfWorkload = 100 * breakdown.NonABCIWorkloadSeconds / workloadRuntime
			if workloadRuntime > signal.ABCICommitSeconds {
				breakdown.MaxRuntimeSpeedupIfCommitFree = workloadRuntime / (workloadRuntime - signal.ABCICommitSeconds)
			}
			if workloadRuntime > signal.ABCICommitSeconds/2 {
				breakdown.MaxRuntimeSpeedupIfCommitHalf = workloadRuntime / (workloadRuntime - signal.ABCICommitSeconds/2)
			}
		}
		out = append(out, breakdown)
	}
	return out
}

func summarizeLoadTestLogs(logs string) loadTestLogSummary {
	if logs == "" {
		return loadTestLogSummary{}
	}
	var summary loadTestLogSummary
	errorCounts := map[string]int{}
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "Sending txs") {
			summary.SendingEvents++
			summary.SendingTxsTotal += intField(line, "num_txs")
		}
		if strings.Contains(line, "built batch ") {
			summary.BuiltBatchEvents++
		}
		if strings.Contains(line, "failed to send tx") {
			summary.FailedSendTotal++
			errText := stringField(line, "error")
			if errText == "" {
				errText = "unknown"
			}
			errorCounts[normalizeLogError(errText)]++
		}
		if strings.Contains(line, "go routines have completed") {
			summary.GoRoutinesCompletedTotal = intField(line, "total_txs")
		}
		if strings.Contains(line, "Collecting metrics") {
			if n := intField(line, "num_txs"); n > 0 {
				summary.CollectorTxs = n
			}
		}
		if strings.Contains(line, "starting_block") {
			summary.CollectorStartingBlock = uint64(intField(line, "starting_block"))
			summary.CollectorEndingBlock = uint64(intField(line, "ending_block"))
		}
		if strings.Contains(line, "runner results") {
			summary.RunnerOverall = parseRunnerOverall(line)
		}
	}
	if len(errorCounts) > 0 {
		summary.FailedSendErrors = topLogErrors(errorCounts, 8)
	}
	return summary
}

func parseRunnerOverall(line string) *runnerLogOverall {
	out := &runnerLogOverall{
		TotalTransactions:         intField(line, "TotalTransactions"),
		TotalIncludedTransactions: intField(line, "TotalIncludedTransactions"),
		SuccessfulTransactions:    intField(line, "SuccessfulTransactions"),
		FailedTransactions:        intField(line, "FailedTransactions"),
		TPS:                       floatField(line, "TPS"),
	}
	if out.TotalTransactions == 0 && out.TotalIncludedTransactions == 0 && out.TPS == 0 {
		return nil
	}
	return out
}

func summarizeCorrectedLoadTest(result runResult) *correctedLoadTest {
	overall := result.LoadTestResult.Overall
	if overall.TotalTransactions == 0 && result.RawTxSummary == nil && len(result.StorageSignals) == 0 {
		return nil
	}
	out := &correctedLoadTest{
		Source:                 "catalyst",
		TotalTransactions:      overall.TotalTransactions,
		IncludedTransactions:   overall.TotalIncludedTransactions,
		SuccessfulTransactions: overall.SuccessfulTransactions,
		FailedTransactions:     overall.FailedTransactions,
		RuntimeSeconds:         overall.Runtime.Seconds(),
		TPS:                    overall.TPS,
	}
	if result.RawTxSummary != nil && result.RawTxSummary.Queried > 0 {
		out.Source = "raw_tx_audit"
		out.TotalTransactions = result.RawTxSummary.Queried
		out.IncludedTransactions = result.RawTxSummary.Found
		out.SuccessfulTransactions = result.RawTxSummary.Successful
		out.FailedTransactions = result.RawTxSummary.Failed
		out.TotalGasUsed = result.RawTxSummary.TotalGasUsed
		if result.RawTxSummary.TPS > 0 {
			out.TPS = result.RawTxSummary.TPS
		}
	} else if included, successful, candidates, ok := appMetricLoadCounts(result.StorageSignals); ok && shouldUseAppMetricLoadCounts(overall, included, successful) {
		out.Source = "app_metrics"
		out.IncludedTransactions = included
		out.SuccessfulTransactions = successful
		out.AppMetricsIncludedCandidates = candidates
		if out.TotalTransactions < out.IncludedTransactions {
			out.TotalTransactions = out.IncludedTransactions
		}
		if out.TotalTransactions < out.SuccessfulTransactions {
			out.TotalTransactions = out.SuccessfulTransactions
		}
		out.FailedTransactions = nonNegativeInt(out.TotalTransactions - out.SuccessfulTransactions)
	}
	if result.RawTxAuditSkipped != "" {
		out.Notes = append(out.Notes, result.RawTxAuditSkipped)
	}
	if out.RuntimeSeconds == 0 && result.LoadTestStopped != "" && loadWindowAccepted(result.LoadWindow) {
		if result.LoadWindow.IncludedTransactions > 0 {
			out.IncludedTransactions = result.LoadWindow.IncludedTransactions
		}
		if result.LoadWindow.SuccessfulTransactions > 0 {
			out.SuccessfulTransactions = result.LoadWindow.SuccessfulTransactions
		}
		windowTotal := out.IncludedTransactions
		if windowTotal < out.SuccessfulTransactions {
			windowTotal = out.SuccessfulTransactions
		}
		if windowTotal > 0 {
			out.TotalTransactions = windowTotal
		}
		out.FailedTransactions = nonNegativeInt(out.TotalTransactions - out.SuccessfulTransactions)
		out.RuntimeSeconds = result.LoadWindow.Seconds
		out.TPS = float64(out.SuccessfulTransactions) / out.RuntimeSeconds
		out.Notes = append(out.Notes, "runtime and counts use accepted app-metric load-window because Catalyst was stopped before result collection")
	}
	if out.RuntimeSeconds == 0 && out.TPS > 0 && out.SuccessfulTransactions > 0 {
		out.RuntimeSeconds = float64(out.SuccessfulTransactions) / out.TPS
	}
	if out.TPS == 0 && out.RuntimeSeconds > 0 && out.SuccessfulTransactions > 0 {
		out.TPS = float64(out.SuccessfulTransactions) / out.RuntimeSeconds
	}
	out.CatalystMismatch = overall.TotalIncludedTransactions != out.IncludedTransactions ||
		overall.SuccessfulTransactions != out.SuccessfulTransactions ||
		overall.FailedTransactions != out.FailedTransactions
	if out.CatalystMismatch {
		out.Notes = append(out.Notes, "catalyst result differs from corrected counts")
	}
	return out
}

func shouldUseAppMetricLoadCounts(overall ctlt.OverallStats, included, successful int) bool {
	if included <= 0 && successful <= 0 {
		return false
	}
	if overall.TotalTransactions == 0 {
		return true
	}
	if successful > overall.SuccessfulTransactions {
		return true
	}
	return overall.TotalIncludedTransactions == 0 && included > 0 && overall.FailedTransactions > 0
}

func appMetricLoadCounts(signals []storageSignal) (included, successful int, candidates []string, ok bool) {
	for _, signal := range signals {
		addIncluded := func(name string, value float64) {
			n := roundedMetricCount(value)
			if n <= 0 {
				return
			}
			if n > included {
				included = n
			}
			candidates = append(candidates, fmt.Sprintf("%s.%s=%d", signal.Name, name, n))
		}
		addSuccessful := func(name string, value float64) {
			n := roundedMetricCount(value)
			if n <= 0 {
				return
			}
			if n > successful {
				successful = n
			}
			candidates = append(candidates, fmt.Sprintf("%s.%s=%d", signal.Name, name, n))
		}
		addIncluded("sdk_tx_count_delta", signal.SDKTxCountDelta)
		addIncluded("consensus_total_txs_delta", signal.ConsensusTotalTxsDelta)
		addIncluded("mempool_successful_txs_delta", signal.MempoolSuccessfulTxsDelta)
		addSuccessful("sdk_tx_successful_delta", signal.SDKTxSuccessfulDelta)
		addSuccessful("mempool_successful_txs_delta", signal.MempoolSuccessfulTxsDelta)
	}
	if included < successful {
		included = successful
	}
	return included, successful, candidates, included > 0 || successful > 0
}

func roundedMetricCount(value float64) int {
	if value <= 0 {
		return 0
	}
	return int(value + 0.5)
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func topLogErrors(counts map[string]int, limit int) []logErrorCount {
	out := make([]logErrorCount, 0, len(counts))
	for errText, count := range counts {
		out = append(out, logErrorCount{Error: errText, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Error < out[j].Error
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func normalizeLogError(errText string) string {
	replacements := []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`http://[0-9.]+:[0-9]+`), "http://<rpc>"},
		{regexp.MustCompile(`ws://[0-9.]+:[0-9]+`), "ws://<rpc>"},
	}
	for _, repl := range replacements {
		errText = repl.re.ReplaceAllString(errText, repl.with)
	}
	if len(errText) > 300 {
		errText = errText[:300]
	}
	return errText
}

func intField(line, key string) int {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s*"?(-?[0-9]+)"?`)
	match := pattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(match[1])
	return n
}

func floatField(line, key string) float64 {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s*"?(-?[0-9]+(?:\.[0-9]+)?)"?`)
	match := pattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return 0
	}
	n, _ := strconv.ParseFloat(match[1], 64)
	return n
}

func stringField(line, key string) string {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s*"((?:[^"\\]|\\.)*)"`)
	match := pattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return ""
	}
	text, err := strconv.Unquote(`"` + match[1] + `"`)
	if err != nil {
		return match[1]
	}
	return text
}

func loadCounts(result runResult) (included, successful int, runtimeSeconds, tps float64) {
	overall := result.LoadTestResult.Overall
	included = overall.TotalIncludedTransactions
	successful = overall.SuccessfulTransactions
	runtimeSeconds = overall.Runtime.Seconds()
	tps = overall.TPS
	if result.CorrectedLoadTest != nil {
		included = result.CorrectedLoadTest.IncludedTransactions
		successful = result.CorrectedLoadTest.SuccessfulTransactions
		runtimeSeconds = result.CorrectedLoadTest.RuntimeSeconds
		tps = result.CorrectedLoadTest.TPS
		return included, successful, runtimeSeconds, tps
	}
	if result.RawTxSummary != nil {
		included = result.RawTxSummary.Found
		successful = result.RawTxSummary.Successful
		if result.RawTxSummary.TPS > 0 {
			tps = result.RawTxSummary.TPS
			if runtimeSeconds == 0 && successful > 0 {
				runtimeSeconds = float64(successful) / result.RawTxSummary.TPS
			}
		}
	}
	return included, successful, runtimeSeconds, tps
}

func loadRuntimeSeconds(result runResult) float64 {
	_, _, runtimeSeconds, _ := loadCounts(result)
	return runtimeSeconds
}

func deriveMetrics(sc scenario, result runResult) derivedMetrics {
	included, successful, runtimeSeconds, tps := loadCounts(result)
	loadWindowIncluded := included
	loadWindowSuccessful := successful
	if result.LoadWindow != nil {
		if result.LoadWindow.IncludedTransactions > 0 {
			loadWindowIncluded = result.LoadWindow.IncludedTransactions
		}
		if result.LoadWindow.SuccessfulTransactions > 0 {
			loadWindowSuccessful = result.LoadWindow.SuccessfulTransactions
		}
	}
	derived := derivedMetrics{
		IntendedTransactions: intendedTransactions(sc),
		RuntimeIncludedTPS:   tps,
	}
	if derived.IntendedTransactions > 0 {
		derived.IncludedFraction = float64(included) / float64(derived.IntendedTransactions)
		derived.SuccessfulFraction = float64(successful) / float64(derived.IntendedTransactions)
	}
	if result.WallSeconds > 0 {
		derived.WallIncludedTPS = float64(included) / result.WallSeconds
	}
	if loadPhaseWallSeconds := phaseSeconds(result, "run_load_test"); loadPhaseWallSeconds > 0 {
		derived.LoadPhaseWallIncludedTPS = float64(included) / loadPhaseWallSeconds
	}
	if loadWindowAccepted(result.LoadWindow) && result.LoadWindow.Seconds > 0 {
		derived.LoadWindowIncludedTPS = float64(loadWindowIncluded) / result.LoadWindow.Seconds
	}
	effectiveOps, note := effectiveOperations(sc, successful)
	if effectiveOps > 0 {
		derived.EffectiveOperations = effectiveOps
		derived.EffectiveOperationNote = note
		if runtimeSeconds > 0 {
			derived.RuntimeEffectiveOperationsPerSec = float64(effectiveOps) / runtimeSeconds
		}
		if result.WallSeconds > 0 {
			derived.WallEffectiveOperationsPerSec = float64(effectiveOps) / result.WallSeconds
		}
		if loadPhaseWallSeconds := phaseSeconds(result, "run_load_test"); loadPhaseWallSeconds > 0 {
			derived.LoadPhaseEffectiveOperationsPerSec = float64(effectiveOps) / loadPhaseWallSeconds
		}
		if loadWindowAccepted(result.LoadWindow) && result.LoadWindow.Seconds > 0 {
			loadWindowEffectiveOps, _ := effectiveOperations(sc, loadWindowSuccessful)
			if loadWindowEffectiveOps > 0 {
				derived.LoadWindowEffectiveOperationsPerSec = float64(loadWindowEffectiveOps) / result.LoadWindow.Seconds
			}
		}
	}
	return derived
}

func loadWindowAccepted(obs *loadWindowObservation) bool {
	if obs == nil || !obs.Reached {
		return false
	}
	return obs.MinimumSeconds <= 0 || obs.DurationSatisfied
}

func loadWindowTargetTransactions(intended int, fraction float64) int {
	if intended <= 0 {
		return intended
	}
	if fraction <= 0 || fraction > 1 {
		return intended
	}
	target := int(math.Ceil(float64(intended) * fraction))
	if target < 1 {
		return 1
	}
	if target > intended {
		return intended
	}
	return target
}

func intendedTransactions(sc scenario) int {
	if sc.IsEVMChain {
		totalPerBatch := 0
		for _, msg := range sc.LoadTestSpec.Msgs {
			totalPerBatch += max(1, msg.NumMsgs)
		}
		if sc.LoadTestSpec.NumOfBlocks > 0 {
			return sc.LoadTestSpec.NumOfBlocks * totalPerBatch
		}
		return sc.LoadTestSpec.NumBatches * totalPerBatch
	}
	if sc.LoadTestSpec.NumOfBlocks > 0 && sc.LoadTestSpec.NumOfTxs > 0 {
		return sc.LoadTestSpec.NumOfBlocks * sc.LoadTestSpec.NumOfTxs
	}
	return 0
}

func effectiveOperations(sc scenario, successfulTxs int) (int, string) {
	if successfulTxs == 0 || len(sc.LoadTestSpec.Msgs) == 0 {
		return 0, ""
	}
	msg := sc.LoadTestSpec.Msgs[0]
	switch msg.Type.String() {
	case "MsgWriteTo":
		iterations := max(1, msg.NumOfIterations)
		return successfulTxs * iterations, fmt.Sprintf("successful transactions * %d storage-write iterations", iterations)
	case "MsgCrossContractCall":
		iterations := max(1, msg.NumOfIterations)
		return successfulTxs * iterations, fmt.Sprintf("successful transactions * %d cross-contract storage iterations", iterations)
	case "MsgCallDataBlast":
		calldataSize := max(1, msg.CalldataSize)
		return successfulTxs * calldataSize, fmt.Sprintf("successful transactions * %d calldata bytes processed", calldataSize)
	case "MsgMultiSend":
		recipients := max(1, msg.NumOfRecipients)
		return successfulTxs * recipients, fmt.Sprintf("successful transactions * %d MsgMultiSend recipients", recipients)
	case "MsgArr":
		messages := max(1, msg.NumMsgs)
		if msg.ContainedType.String() == "MsgMultiSend" {
			recipients := max(1, msg.NumOfRecipients)
			return successfulTxs * messages * recipients, fmt.Sprintf("successful transactions * %d contained messages * %d MsgMultiSend recipients", messages, recipients)
		}
		return successfulTxs * messages, fmt.Sprintf("successful transactions * %d contained messages", messages)
	default:
		return 0, ""
	}
}

func percentValue(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func memoryUsage(s string) (uint64, string) {
	parts := strings.Split(s, "/")
	if len(parts) == 0 {
		return 0, ""
	}
	usage := strings.TrimSpace(parts[0])
	return parseDockerSize(usage), usage
}

func ioPairBytes(s string) (uint64, uint64) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseDockerSize(strings.TrimSpace(parts[0])), parseDockerSize(strings.TrimSpace(parts[1]))
}

func parseDockerSize(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([kmgtp]?i?b?)?$`)
	match := re.FindStringSubmatch(s)
	if len(match) != 3 {
		return 0
	}
	value, _ := strconv.ParseFloat(match[1], 64)
	unit := strings.ToLower(match[2])
	multiplier := float64(1)
	switch unit {
	case "kib":
		multiplier = 1024
	case "mib":
		multiplier = 1024 * 1024
	case "gib":
		multiplier = 1024 * 1024 * 1024
	case "tib":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "kb", "k":
		multiplier = 1000
	case "mb", "m":
		multiplier = 1000 * 1000
	case "gb", "g":
		multiplier = 1000 * 1000 * 1000
	case "tb", "t":
		multiplier = 1000 * 1000 * 1000 * 1000
	}
	return uint64(value * multiplier)
}

func auditRawTxs(ctx context.Context, providerName, chainName, logs string, result ctlt.LoadTestResult) []txAudit {
	hashes := txHashesFromLogs(logs)
	if len(hashes) == 0 {
		return nil
	}
	rpcs := candidateRPCs(ctx, providerName, chainName, result)
	if len(rpcs) == 0 {
		audits := make([]txAudit, 0, len(hashes))
		for _, hash := range hashes {
			audits = append(audits, txAudit{Hash: hash, Error: "no RPC addresses available"})
		}
		return audits
	}

	client := &http.Client{Timeout: 5 * time.Second}
	audits := make([]txAudit, 0, len(hashes))
	for _, hash := range hashes {
		var last txAudit
		for _, rpc := range rpcs {
			audit := queryRawTx(ctx, client, rpc, hash)
			if audit.Found {
				audits = append(audits, audit)
				last = txAudit{}
				break
			}
			last = audit
		}
		if last.Hash != "" {
			audits = append(audits, last)
		}
	}
	return audits
}

func txHashesFromLogs(logs string) []string {
	re := regexp.MustCompile(`\b[A-F0-9]{64}\b`)
	seen := map[string]bool{}
	var hashes []string
	for _, hash := range re.FindAllString(logs, -1) {
		if seen[hash] {
			continue
		}
		seen[hash] = true
		hashes = append(hashes, hash)
	}
	return hashes
}

func candidateRPCs(ctx context.Context, providerName, chainName string, result ctlt.LoadTestResult) []string {
	seen := map[string]bool{}
	var rpcs []string
	for rpc := range result.ByNode {
		if strings.HasPrefix(rpc, "http://") || strings.HasPrefix(rpc, "https://") {
			seen[rpc] = true
			rpcs = append(rpcs, rpc)
		}
	}
	for _, rpc := range dockerExternalRPCs(ctx, providerName, chainName) {
		if seen[rpc] {
			continue
		}
		seen[rpc] = true
		rpcs = append(rpcs, rpc)
	}
	sort.Strings(rpcs)
	return rpcs
}

func dockerExternalRPCs(ctx context.Context, providerName, chainName string) []string {
	prefix := providerName + "-" + chainName + "-"
	namesOut, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "name="+prefix, "--format", "{{.Names}}").Output()
	if err != nil {
		return nil
	}
	var rpcs []string
	for _, name := range strings.Fields(string(namesOut)) {
		if !strings.Contains(name, "-node-") && !strings.Contains(name, "-validator-") {
			continue
		}
		out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", `{{range $p, $v := .NetworkSettings.Ports}}{{if eq (printf "%s" $p) "26657/tcp"}}{{(index $v 0).HostPort}}{{end}}{{end}}`, name).Output()
		if err != nil {
			continue
		}
		port := strings.TrimSpace(string(out))
		if port == "" || port == "<no value>" {
			continue
		}
		rpcs = append(rpcs, "http://127.0.0.1:"+port)
	}
	return rpcs
}

func queryRawTx(ctx context.Context, client *http.Client, rpc, hash string) txAudit {
	for _, prefix := range []string{"0x", ""} {
		audit := queryRawTxOnce(ctx, client, rpc, hash, prefix)
		if audit.Found {
			return audit
		}
		if audit.Error != "" && !strings.Contains(audit.Error, "not found") {
			return audit
		}
	}
	return txAudit{Hash: hash, RPC: rpc, Error: "not found"}
}

func queryRawTxOnce(ctx context.Context, client *http.Client, rpc, hash, prefix string) txAudit {
	audit := txAudit{Hash: hash, RPC: rpc}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rpc, "/")+"/tx?hash="+prefix+hash, nil)
	if err != nil {
		audit.Error = err.Error()
		return audit
	}
	resp, err := client.Do(req)
	if err != nil {
		audit.Error = err.Error()
		return audit
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		audit.Error = err.Error()
		return audit
	}
	if errVal, ok := body["error"]; ok {
		audit.Error = fmt.Sprint(errVal)
		return audit
	}
	resultMap, ok := body["result"].(map[string]interface{})
	if !ok {
		audit.Error = "missing result"
		return audit
	}
	txResult, ok := resultMap["tx_result"].(map[string]interface{})
	if !ok {
		audit.Error = "missing tx_result"
		return audit
	}
	audit.Found = true
	audit.Height = int64FromAny(resultMap["height"])
	audit.Code = uint32(int64FromAny(txResult["code"]))
	audit.GasUsed = int64FromAny(txResult["gas_used"])
	return audit
}

func summarizeTxAudit(audits []txAudit, runtime time.Duration) *txAuditSummary {
	if len(audits) == 0 {
		return nil
	}
	summary := &txAuditSummary{Queried: len(audits)}
	for _, audit := range audits {
		if !audit.Found {
			continue
		}
		summary.Found++
		summary.TotalGasUsed += audit.GasUsed
		if audit.Code == 0 {
			summary.Successful++
		} else {
			summary.Failed++
		}
	}
	if runtime > 0 {
		summary.TPS = float64(summary.Successful) / runtime.Seconds()
	}
	return summary
}

func int64FromAny(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	default:
		return 0
	}
}

func markdownPathForJSON(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".md"
	}
	return strings.TrimSuffix(path, ext) + ".md"
}

func renderReportMarkdown(artifact reportArtifact) string {
	var b strings.Builder
	b.WriteString("# Ironbird Local Report\n\n")
	if !artifact.GeneratedAt.IsZero() {
		b.WriteString("- Generated: ")
		b.WriteString(artifact.GeneratedAt.UTC().Format(time.RFC3339))
		b.WriteByte('\n')
	}
	if branch := artifact.Git["branch"]; branch != "" {
		b.WriteString("- Branch: `")
		b.WriteString(mdCell(branch))
		b.WriteString("`\n")
	}
	if head := artifact.Git["head"]; head != "" {
		b.WriteString("- Head: `")
		b.WriteString(mdCell(head))
		b.WriteString("`\n")
	}
	b.WriteByte('\n')

	b.WriteString("## Scenario Summary\n\n")
	b.WriteString("| Scenario | Error | Load window | Included | Successful | Load-window TPS | Profiles |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, result := range artifact.Results {
		included, successful, _, _ := loadCounts(result)
		loadWindow := ""
		loadTPS := ""
		if result.LoadWindow != nil {
			loadWindow = fmt.Sprintf("%.3fs", result.LoadWindow.Seconds)
			if result.Derived.LoadWindowIncludedTPS > 0 {
				loadTPS = metricFloat(result.Derived.LoadWindowIncludedTPS)
			}
		}
		b.WriteString("| ")
		b.WriteString(mdCell(result.Scenario.Name))
		b.WriteString(" | ")
		b.WriteString(mdCell(result.Error))
		b.WriteString(" | ")
		b.WriteString(mdCell(loadWindow))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(included))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(successful))
		b.WriteString(" | ")
		b.WriteString(mdCell(loadTPS))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(len(result.ProfileArtifacts)))
		b.WriteString(" |\n")
	}
	b.WriteByte('\n')

	for _, result := range artifact.Results {
		writeRuntimeBreakdownMarkdown(&b, result)
		writeAcceptedWindowMarkdown(&b, result)
		writeProfileArtifactsMarkdown(&b, result)
	}
	return b.String()
}

func writeRuntimeBreakdownMarkdown(b *strings.Builder, result runResult) {
	if len(result.RuntimeBreakdown) == 0 {
		return
	}
	b.WriteString("## ")
	b.WriteString(mdCell(result.Scenario.Name))
	b.WriteString(" Runtime Breakdown\n\n")
	b.WriteString("| Node | Workload s | ABCI s | Commit s | Finalize s | Non-ABCI s | Non-ABCI % |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, row := range result.RuntimeBreakdown {
		b.WriteString("| ")
		b.WriteString(mdCell(row.Name))
		b.WriteString(" | ")
		b.WriteString(metricFloat(row.WorkloadRuntimeSeconds))
		b.WriteString(" | ")
		b.WriteString(metricFloat(row.ABCIObservedSeconds))
		b.WriteString(" | ")
		b.WriteString(metricFloat(row.ABCICommitSeconds))
		b.WriteString(" | ")
		b.WriteString(metricFloat(row.ABCIFinalizeBlockSeconds))
		b.WriteString(" | ")
		b.WriteString(metricFloat(row.NonABCIWorkloadSeconds))
		b.WriteString(" | ")
		b.WriteString(metricFloat(row.NonABCIPctOfWorkload))
		b.WriteString(" |\n")
	}
	b.WriteByte('\n')
}

func writeAcceptedWindowMarkdown(b *strings.Builder, result runResult) {
	if result.LoadWindow == nil {
		return
	}
	obs := result.LoadWindow
	b.WriteString("## ")
	b.WriteString(mdCell(result.Scenario.Name))
	b.WriteString(" Accepted Window\n\n")
	b.WriteString("| Boundary | Reached | Duration ok | Seconds | Included | Successful | Target |\n")
	b.WriteString("| --- | --- | --- | ---: | ---: | ---: | ---: |\n")
	b.WriteString("| ")
	b.WriteString(profileBoundaryAcceptedWindow)
	b.WriteString(" | ")
	b.WriteString(strconv.FormatBool(obs.Reached))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatBool(obs.DurationSatisfied))
	b.WriteString(" | ")
	b.WriteString(metricFloat(obs.Seconds))
	b.WriteString(" | ")
	b.WriteString(strconv.Itoa(obs.IncludedTransactions))
	b.WriteString(" | ")
	b.WriteString(strconv.Itoa(obs.SuccessfulTransactions))
	b.WriteString(" | ")
	b.WriteString(strconv.Itoa(obs.TargetTransactions))
	b.WriteString(" |\n\n")
	if obs.Error != "" {
		b.WriteString("- Window note: ")
		b.WriteString(mdCell(obs.Error))
		b.WriteString("\n\n")
	}

	rows := metricDeltaRows(obs.MetricDeltas)
	errors := metricDeltaErrors(obs.MetricDeltas)
	if len(rows) == 0 && len(errors) == 0 {
		b.WriteString("No accepted-window metric deltas were recorded.\n\n")
		return
	}
	if len(rows) > 0 {
		b.WriteString("### Accepted-Window Metric Deltas\n\n")
		b.WriteString("| Container | Metric | Before | After | Delta |\n")
		b.WriteString("| --- | --- | ---: | ---: | ---: |\n")
		for _, row := range rows {
			b.WriteString("| ")
			b.WriteString(mdCell(row.Container))
			b.WriteString(" | ")
			b.WriteString(mdCell(row.Metric))
			b.WriteString(" | ")
			b.WriteString(metricFloat(row.Before))
			b.WriteString(" | ")
			b.WriteString(metricFloat(row.After))
			b.WriteString(" | ")
			b.WriteString(metricFloat(row.Delta))
			b.WriteString(" |\n")
		}
		b.WriteByte('\n')
	}

	if len(errors) > 0 {
		b.WriteString("### Accepted-Window Metric Scrape Errors\n\n")
		b.WriteString("| Container | Error |\n")
		b.WriteString("| --- | --- |\n")
		for _, errRow := range errors {
			b.WriteString("| ")
			b.WriteString(mdCell(errRow.Container))
			b.WriteString(" | ")
			b.WriteString(mdCell(errRow.Error))
			b.WriteString(" |\n")
		}
		b.WriteByte('\n')
	}
}

func writeProfileArtifactsMarkdown(b *strings.Builder, result runResult) {
	if len(result.ProfileArtifacts) == 0 {
		return
	}
	b.WriteString("## ")
	b.WriteString(mdCell(result.Scenario.Name))
	b.WriteString(" Profile Manifest\n\n")
	b.WriteString("| Kind | Timing boundary | Profile path | Top summary | Status |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, artifact := range result.ProfileArtifacts {
		status := "ok"
		if artifact.Error != "" {
			status = artifact.Error
		} else if artifact.TopSummaryError != "" {
			status = artifact.TopSummaryError
		}
		b.WriteString("| ")
		b.WriteString(mdCell(artifact.Kind))
		b.WriteString(" | ")
		b.WriteString(mdCell(artifact.Boundary))
		b.WriteString(" | ")
		b.WriteString(mdCell(artifact.Path))
		b.WriteString(" | ")
		b.WriteString(mdCell(artifact.TopSummaryPath))
		b.WriteString(" | ")
		b.WriteString(mdCell(status))
		b.WriteString(" |\n")
	}
	b.WriteByte('\n')
}

type metricDeltaRow struct {
	Container string
	Metric    string
	Before    float64
	After     float64
	Delta     float64
}

func metricDeltaRows(snapshots []metricDeltaSnapshot) []metricDeltaRow {
	var rows []metricDeltaRow
	for _, snapshot := range snapshots {
		for metric, values := range snapshot.Metrics {
			rows = append(rows, metricDeltaRow{
				Container: snapshot.Name,
				Metric:    metric,
				Before:    values.Before,
				After:     values.After,
				Delta:     values.Delta,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Container == rows[j].Container {
			return rows[i].Metric < rows[j].Metric
		}
		return rows[i].Container < rows[j].Container
	})
	return rows
}

type metricDeltaErrorRow struct {
	Container string
	Error     string
}

func metricDeltaErrors(snapshots []metricDeltaSnapshot) []metricDeltaErrorRow {
	var rows []metricDeltaErrorRow
	for _, snapshot := range snapshots {
		if snapshot.Error != "" {
			rows = append(rows, metricDeltaErrorRow{Container: snapshot.Name, Error: snapshot.Error})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Container < rows[j].Container })
	return rows
}

func metricFloat(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprint(value)
	}
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'g', 8, 64)
}

func mdCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}

func hostMetadata(ctx context.Context) map[string]string {
	meta := map[string]string{
		"goos":       runtime.GOOS,
		"goarch":     runtime.GOARCH,
		"go_version": runtime.Version(),
	}
	for key, cmd := range map[string][]string{
		"uname":          {"uname", "-a"},
		"docker_version": {"docker", "--version"},
		"lscpu_model":    {"sh", "-c", "lscpu | sed -n 's/^Model name:[[:space:]]*//p' | head -1"},
		"cpu_count":      {"sh", "-c", "nproc"},
		"memory":         {"sh", "-c", "free -h | sed -n '2p'"},
		"root_disk":      {"sh", "-c", "df -h / | tail -1"},
		"fast4tb_disk":   {"sh", "-c", "df -h /mnt/fast4tb 2>/dev/null | tail -1 || true"},
	} {
		meta[key] = commandStringSlice(ctx, cmd)
	}
	return meta
}

func gitMetadata(ctx context.Context) map[string]string {
	meta := map[string]string{}
	for key, cmd := range map[string][]string{
		"branch": {"git", "branch", "--show-current"},
		"head":   {"git", "rev-parse", "HEAD"},
		"status": {"git", "status", "--short"},
	} {
		meta[key] = commandStringSlice(ctx, cmd)
	}
	return meta
}

func gitCommandString(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return err.Error()
	}
	return strings.TrimSpace(string(out))
}

func commandStringSlice(ctx context.Context, cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	return commandString(ctx, cmd[0], cmd[1:]...)
}

func commandString(ctx context.Context, name string, args ...string) string {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return err.Error()
	}
	return strings.TrimSpace(string(out))
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func flagMetadata() map[string]string {
	out := map[string]string{}
	flag.VisitAll(func(f *flag.Flag) {
		out[f.Name] = f.Value.String()
	})
	return out
}

func envMapToList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func shellEnvFile(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for key := range env {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(env[key]) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# generated by ironbird cmd/local-report-runner\n")
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(shellQuote(env[key]))
		b.WriteByte('\n')
	}
	return b.String()
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func boolEnv(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func defaultExistingPath(candidates ...string) string {
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[len(candidates)-1]
}

func defaultFastPath(name string) string {
	base := "/mnt/fast4tb/tmp"
	if st, err := os.Stat(base); err == nil && st.IsDir() {
		return filepath.Join(base, name)
	}
	return filepath.Join(os.TempDir(), name)
}

func normalizeCelestiaConfig(cfg celestiaSyncConfig) celestiaSyncConfig {
	if cfg.PairRunMode == "" {
		cfg.PairRunMode = "concurrent"
	}
	cfg.RunnerScript = absPathIfRelative(cfg.RunnerScript)
	cfg.OutputDir = absPathIfRelative(cfg.OutputDir)
	cfg.ControlEnvFile = absPathIfRelative(cfg.ControlEnvFile)
	cfg.CandidateEnvFile = absPathIfRelative(cfg.CandidateEnvFile)
	cfg.RunHomeBase = absPathIfRelative(cfg.RunHomeBase)
	cfg.GoCacheRoot = absPathIfRelative(cfg.GoCacheRoot)
	cfg.TempDir = absPathIfRelative(cfg.TempDir)
	cfg.CelestiaAppDir = absPathIfRelative(cfg.CelestiaAppDir)
	cfg.GomapDir = absPathIfRelative(cfg.GomapDir)
	cfg.CosmosDBDir = absPathIfRelative(cfg.CosmosDBDir)
	cfg.CometDBDir = absPathIfRelative(cfg.CometDBDir)
	cfg.CosmosStoreDir = absPathIfRelative(cfg.CosmosStoreDir)
	cfg.CosmosLogDir = absPathIfRelative(cfg.CosmosLogDir)
	cfg.CosmosCoreDir = absPathIfRelative(cfg.CosmosCoreDir)
	cfg.IAVLDir = absPathIfRelative(cfg.IAVLDir)
	return cfg
}

func absPathIfRelative(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortRef(ref string) string {
	if len(ref) <= 12 {
		return ref
	}
	return ref[:12]
}

func shortChainName(name string) string {
	switch name {
	case "evm-blog":
		return "evmblog"
	case "simapp-goleveldb":
		return "sgldb"
	case "simapp-treedb":
		return "streedb"
	case "simapp-goleveldb-all":
		return "sgldball"
	case "simapp-treedb-all":
		return "streedba"
	default:
		clean := sanitize(name)
		if len(clean) > 8 {
			return clean[:8]
		}
		return clean
	}
}

func trimLog(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[len(s)-limit:]
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
