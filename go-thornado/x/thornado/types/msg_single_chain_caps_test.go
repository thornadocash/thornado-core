package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	frostMessages "github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestSolvencyValidateBasicRejectsMultipleCoins(t *testing.T) {
	SetupConfigForTest()

	coins := common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(1000)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(1)),
	}
	msg, err := NewMsgSolvency(common.BTCChain, GetRandomPubKey(), coins, 1, GetRandomBech32Addr())
	require.NoError(t, err)
	require.ErrorContains(t, msg.ValidateBasic(), "too many solvency coins")
}

func TestFrostKeysignFailValidateBasicRejectsMultipleCoins(t *testing.T) {
	SetupConfigForTest()

	msg, err := NewMsgFrostKeysignFail(
		1,
		Blame{
			FailReason: "failed",
			Round:      frostMessages.KEYSIGN1aUnicast,
			BlameNodes: []Node{{Pubkey: GetRandomPubKey().String()}},
		},
		common.Coins{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(1000)),
			common.NewCoin(common.BTCAsset, cosmos.NewUint(1)),
		},
		GetRandomBech32Addr(),
		GetRandomPubKey(),
	)
	require.NoError(t, err)
	require.ErrorContains(t, msg.ValidateBasic(), "too many coins")
}
