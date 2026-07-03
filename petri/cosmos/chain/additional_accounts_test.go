package chain

import (
	"testing"

	petritypes "github.com/skip-mev/ironbird/petri/core/types"
	"github.com/stretchr/testify/require"
)

func TestAdditionalAccountPassphraseCosmos(t *testing.T) {
	walletCfg := petritypes.WalletConfig{SigningAlgorithm: "secp256k1"}

	require.Equal(t, "0", additionalAccountPassphrase(walletCfg, 0))
	require.Equal(t, "1", additionalAccountPassphrase(walletCfg, 1))
}

func TestAdditionalAccountPassphraseEVM(t *testing.T) {
	walletCfg := petritypes.WalletConfig{SigningAlgorithm: "eth_secp256k1"}

	require.Equal(t, "", additionalAccountPassphrase(walletCfg, 0))
	require.Equal(t, "1", additionalAccountPassphrase(walletCfg, 1))
}
