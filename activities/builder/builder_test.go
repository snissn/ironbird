package builder

import (
	"strings"
	"testing"

	"github.com/skip-mev/ironbird/messages"
	"github.com/stretchr/testify/require"
)

func TestGenerateMultipleReplacesReturnsReplaceSpecs(t *testing.T) {
	specs := generateMultipleReplaces(messages.BuildDockerImageRequest{
		Repo:         "evm",
		CosmosSdkSha: "sdk-sha",
		CometBFTSha:  "comet-sha",
	})

	require.Contains(t, specs, "github.com/cosmos/cosmos-sdk=github.com/cosmos/cosmos-sdk@sdk-sha")
	require.Contains(t, specs, "github.com/cometbft/cometbft=github.com/cometbft/cometbft@comet-sha")
	require.NotContains(t, specs, "go mod edit")
	require.NotContains(t, specs, "&&")
	require.NotContains(t, specs, ";")
	for _, spec := range strings.Fields(specs) {
		require.Contains(t, spec, "=")
		require.Contains(t, spec, "@")
	}
}

func TestGenerateMultipleReplacesCometBFTRepoUsesRepoSHA(t *testing.T) {
	specs := generateMultipleReplaces(messages.BuildDockerImageRequest{
		Repo: "cometbft",
		SHA:  "comet-sha",
	})

	require.Equal(t, "github.com/cometbft/cometbft=github.com/cometbft/cometbft@comet-sha", specs)
}
