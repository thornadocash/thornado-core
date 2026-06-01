package thornadoclient

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado"
	stypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestGetInboundOutboundAcceptsBTCDepositChildAddress(t *testing.T) {
	thornado.SetupConfigForTest()

	pubKey := stypes.GetRandomPubKey()
	baseAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)
	depositAddress, err := common.DeriveBTCTaprootAddress(pubKey, common.FirstDepositPathIndex)
	require.NoError(t, err)
	sender, err := common.NewAddress("bcrt1qejdlz8pqe8g7908j69mr6crlgfve67nm9u2hhr")
	require.NoError(t, err)

	bridge := thornadoBridge{logger: zerolog.Nop()}
	inbound, outbound, err := bridge.GetInboundOutbound(common.ObservedTxs{
		{
			Tx: common.Tx{
				ID:          "deposit",
				Chain:       common.BTCChain,
				FromAddress: sender,
				ToAddress:   depositAddress,
				Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
			},
			ObservedPubKey: pubKey,
		},
		{
			Tx: common.Tx{
				ID:          "outbound",
				Chain:       common.BTCChain,
				FromAddress: depositAddress,
				ToAddress:   sender,
				Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
			},
			ObservedPubKey: pubKey,
		},
		{
			Tx: common.Tx{
				ID:          "base",
				Chain:       common.BTCChain,
				FromAddress: sender,
				ToAddress:   baseAddress,
				Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
			},
			ObservedPubKey: pubKey,
		},
	})

	require.NoError(t, err)
	require.Len(t, inbound, 2)
	require.Len(t, outbound, 1)
	require.Equal(t, common.TxID("deposit"), inbound[0].Tx.ID)
	require.Equal(t, common.TxID("base"), inbound[1].Tx.ID)
	require.Equal(t, common.TxID("outbound"), outbound[0].Tx.ID)
}
