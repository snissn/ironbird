package loadtest

import (
	"context"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	ethtypes "github.com/skip-mev/catalyst/chains/ethereum/types"
	"github.com/skip-mev/catalyst/chains/types"
	"github.com/skip-mev/ironbird/activities/loadtest/mocks"
	"github.com/skip-mev/ironbird/petri/core/provider"
	types2 "github.com/skip-mev/ironbird/petri/core/types"
	"github.com/skip-mev/ironbird/util"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

func TestGenerateSpec(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	chainID := "1234"
	spec := types.LoadTestSpec{
		Name:         "evmtest",
		Description:  "loadtesting stuff",
		Kind:         "eth",
		SendInterval: 500 * time.Millisecond,
		NumBatches:   20,
		Msgs: []types.LoadTestMsg{
			{
				Type:    "MsgNativeTransferERC20",
				NumMsgs: 1500,
			},
		},
		ChainCfg: &ethtypes.ChainConfig{
			TxOpts: ethtypes.TxOpts{
				GasFeeCap: big.NewInt(100),
				GasTipCap: big.NewInt(109009),
			},
		},
	}

	baseMnemonic := "this is a mnemonic"
	numWallets := 4
	chain := mocks.NewMocktheChain(gomock.NewController(t))
	chain.EXPECT().GetConfig().Times(1).Return(types2.ChainConfig{})

	nodes := []types2.NodeI{
		mocks.MockNode{IP: "127.0.0.1"},
		mocks.MockNode{IP: "127.0.0.2"},
		mocks.MockNode{IP: "127.0.0.3"},
		mocks.MockNode{IP: "127.0.0.4"},
	}
	chain.EXPECT().GetNodes().Times(1).Return(nodes)

	specBZ, err := generateLoadTestSpec(ctx, logger, chain, chainID, spec, baseMnemonic, numWallets)
	require.NoError(t, err)

	var gotLoadtestSpec types.LoadTestSpec
	err = yaml.Unmarshal(specBZ, &gotLoadtestSpec)
	require.NoError(t, err)

	spec.ChainID = chainID
	spec.ChainCfg.(*ethtypes.ChainConfig).NodesAddresses = gotLoadtestSpec.ChainCfg.(*ethtypes.ChainConfig).NodesAddresses
	spec.BaseMnemonic = baseMnemonic
	spec.NumWallets = numWallets
	require.Equal(t, gotLoadtestSpec, spec)
	require.Equal(t, len(nodes), len(gotLoadtestSpec.ChainCfg.(*ethtypes.ChainConfig).NodesAddresses))
	require.Equal(t, gotLoadtestSpec.BaseMnemonic, baseMnemonic)
	require.Equal(t, gotLoadtestSpec.NumWallets, numWallets)
}

func TestRedactLoadTestConfigRemovesBaseMnemonic(t *testing.T) {
	config := []byte("name: test\nbase_mnemonic: secret words\nnum_wallets: 10\n")

	redacted, err := redactLoadTestConfig(config)
	require.NoError(t, err)
	require.NotContains(t, redacted, "secret words")
	require.Contains(t, redacted, "base_mnemonic: '[REDACTED]'")
	require.Contains(t, redacted, "num_wallets: 10")
}

func TestStopLoadTestByConditionSerializesStateAndReturnsStopError(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	p := &fakeLoadTestProvider{state: []byte("provider-state")}
	task := &fakeLoadTestTask{logs: "catalyst log tail"}
	chainState := []byte("chain-state")
	serializeChainCalled := false

	res, err := stopLoadTestByCondition(ctx, logger, p, func(ctx context.Context, got provider.ProviderI) ([]byte, error) {
		serializeChainCalled = true
		require.Same(t, p, got)
		return chainState, nil
	}, task, "redacted-config", "accepted load window reached")

	var stopped *StoppedByConditionError
	require.ErrorAs(t, err, &stopped)
	require.Equal(t, "accepted load window reached", stopped.Reason)
	require.Equal(t, "accepted load window reached", res.StoppedReason)
	require.Equal(t, "catalyst log tail", res.TaskLogs)
	require.Equal(t, "redacted-config", res.LoadTestConfig)
	require.True(t, task.destroyed)
	require.True(t, p.serialized)
	require.True(t, serializeChainCalled)

	providerState, err := util.DecompressData(res.ProviderState)
	require.NoError(t, err)
	require.Equal(t, []byte("provider-state"), providerState)

	gotChainState, err := util.DecompressData(res.ChainState)
	require.NoError(t, err)
	require.Equal(t, chainState, gotChainState)
}

func TestTruncateTaskLogsKeepsShortLogs(t *testing.T) {
	logs := "short catalyst log"

	require.Equal(t, logs, truncateTaskLogs(logs, maxLoadTestTaskLogBytes))
}

func TestTruncateTaskLogsCapsLongLogsAtTail(t *testing.T) {
	logs := strings.Repeat("a", 128) + "important tail"

	truncated := truncateTaskLogs(logs, 64)

	require.Len(t, truncated, 64)
	require.Contains(t, truncated, "showing tail")
	require.True(t, strings.HasSuffix(truncated, "important tail"))
}

type fakeLoadTestProvider struct {
	state      []byte
	serialized bool
}

var _ provider.ProviderI = (*fakeLoadTestProvider)(nil)

func (p *fakeLoadTestProvider) CreateTask(context.Context, provider.TaskDefinition) (provider.TaskI, error) {
	return nil, nil
}

func (p *fakeLoadTestProvider) SerializeTask(context.Context, provider.TaskI) ([]byte, error) {
	return nil, nil
}

func (p *fakeLoadTestProvider) DeserializeTask(context.Context, []byte) (provider.TaskI, error) {
	return nil, nil
}

func (p *fakeLoadTestProvider) Teardown(context.Context) error {
	return nil
}

func (p *fakeLoadTestProvider) SerializeProvider(context.Context) ([]byte, error) {
	p.serialized = true
	return p.state, nil
}

func (p *fakeLoadTestProvider) GetType() string {
	return "fake"
}

func (p *fakeLoadTestProvider) GetName() string {
	return "fake"
}

type fakeLoadTestTask struct {
	logs      string
	destroyed bool
}

var _ provider.TaskI = (*fakeLoadTestTask)(nil)
var _ loggableTask = (*fakeLoadTestTask)(nil)

func (t *fakeLoadTestTask) Start(context.Context) error {
	return nil
}

func (t *fakeLoadTestTask) Stop(context.Context) error {
	return nil
}

func (t *fakeLoadTestTask) Destroy(context.Context) error {
	t.destroyed = true
	return nil
}

func (t *fakeLoadTestTask) GetDefinition() provider.TaskDefinition {
	return provider.TaskDefinition{}
}

func (t *fakeLoadTestTask) GetStatus(context.Context) (provider.TaskStatus, error) {
	return provider.TASK_RUNNING, nil
}

func (t *fakeLoadTestTask) Modify(context.Context, provider.TaskDefinition) error {
	return nil
}

func (t *fakeLoadTestTask) WriteFile(context.Context, string, []byte) error {
	return nil
}

func (t *fakeLoadTestTask) ReadFile(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (t *fakeLoadTestTask) DownloadDir(context.Context, string, string) error {
	return nil
}

func (t *fakeLoadTestTask) GetIP(context.Context) (string, error) {
	return "", nil
}

func (t *fakeLoadTestTask) GetPrivateIP(context.Context) (string, error) {
	return "", nil
}

func (t *fakeLoadTestTask) GetExternalAddress(context.Context, string) (string, error) {
	return "", nil
}

func (t *fakeLoadTestTask) DialContext() func(context.Context, string, string) (net.Conn, error) {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, nil
	}
}

func (t *fakeLoadTestTask) RunCommand(context.Context, []string) (string, string, int, error) {
	return "", "", 0, nil
}

func (t *fakeLoadTestTask) Logs(context.Context) (string, error) {
	return t.logs, nil
}
