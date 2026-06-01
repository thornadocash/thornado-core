package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestObservedTxOutValidateBasicAllowsBTCChildPathSource(t *testing.T) {
	SetupConfigForTest()

	pubKey := GetRandomPubKey()
	rootAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)
	childAddress, err := common.DeriveBTCTaprootAddress(pubKey, common.FirstDepositPathIndex)
	require.NoError(t, err)

	tx := common.NewTx(
		GetRandomTxHash(),
		childAddress,
		rootAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
	)
	observedTx := common.NewObservedTx(tx, 1, pubKey, 1)
	msg := NewMsgObservedTxOut(common.ObservedTxs{observedTx}, GetRandomBech32Addr())

	require.NoError(t, msg.ValidateBasic())
}

func TestObservedTxQuorumValidateBasicAllowsBTCChildPathSource(t *testing.T) {
	SetupConfigForTest()

	pubKey := GetRandomPubKey()
	rootAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)
	childAddress, err := common.DeriveBTCTaprootAddress(pubKey, common.FirstDepositPathIndex)
	require.NoError(t, err)

	tx := common.NewTx(
		GetRandomTxHash(),
		childAddress,
		rootAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
	)
	observedTx := common.NewObservedTx(tx, 1, pubKey, 1)
	msg := NewMsgObservedTxQuorum(&common.QuorumTx{ObsTx: observedTx}, GetRandomBech32Addr())

	require.NoError(t, msg.ValidateBasic())
}
