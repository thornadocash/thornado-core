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
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	require.NoError(t, err)
	childAddress, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
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
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	require.NoError(t, err)
	childAddress, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
	require.NoError(t, err)

	tx := common.NewTx(
		GetRandomTxHash(),
		childAddress,
		rootAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
	)
	observedTx := common.NewObservedTx(tx, 1, pubKey, 1)
	msg := NewMsgObservedTxQuorum(&common.QuorumTx{
		ObsTx:        observedTx,
		Attestations: []*common.Attestation{{PubKey: []byte{1}}},
	}, GetRandomBech32Addr())

	require.NoError(t, msg.ValidateBasic())
}

func TestObservedTxValidateBasicAllowsInboundMempoolHeight(t *testing.T) {
	SetupConfigForTest()

	pubKey := GetRandomPubKey()
	rootAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)

	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomBTCAddress(),
		rootAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
	)
	observedTx := common.NewObservedTx(tx, 0, pubKey, 10)

	inboundMsg := NewMsgObservedTxIn(common.ObservedTxs{observedTx}, GetRandomBech32Addr())
	require.NoError(t, inboundMsg.ValidateBasic())

	quorumMsg := NewMsgObservedTxQuorum(&common.QuorumTx{
		ObsTx:        observedTx,
		Inbound:      true,
		Attestations: []*common.Attestation{{PubKey: []byte{1}}},
	}, GetRandomBech32Addr())
	require.NoError(t, quorumMsg.ValidateBasic())
}

func TestObservedTxValidateBasicAllowsOutboundMempoolHeight(t *testing.T) {
	SetupConfigForTest()

	pubKey := GetRandomPubKey()
	rootAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)

	tx := common.NewTx(
		GetRandomTxHash(),
		rootAddress,
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
	)
	observedTx := common.NewObservedTx(tx, 0, pubKey, 10)

	outboundMsg := NewMsgObservedTxOut(common.ObservedTxs{observedTx}, GetRandomBech32Addr())
	require.NoError(t, outboundMsg.ValidateBasic())

	quorumMsg := NewMsgObservedTxQuorum(&common.QuorumTx{
		ObsTx:        observedTx,
		Inbound:      false,
		Attestations: []*common.Attestation{{PubKey: []byte{1}}},
	}, GetRandomBech32Addr())
	require.NoError(t, quorumMsg.ValidateBasic())
}

func TestObservedTxValidateBasicRejectsMultipleCoinsOrGasCoins(t *testing.T) {
	SetupConfigForTest()

	pubKey := GetRandomPubKey()
	rootAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	require.NoError(t, err)
	childAddress, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
	require.NoError(t, err)

	tx := common.NewTx(
		GetRandomTxHash(),
		childAddress,
		rootAddress,
		common.Coins{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(1000)),
			common.NewCoin(common.BTCAsset, cosmos.NewUint(1)),
		},
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
	)
	observedTx := common.NewObservedTx(tx, 1, pubKey, 1)
	msg := NewMsgObservedTxOut(common.ObservedTxs{observedTx}, GetRandomBech32Addr())
	require.ErrorContains(t, msg.ValidateBasic(), "too many observed tx coins")

	tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000)))
	tx.Gas = common.Gas{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(1)),
	}
	observedTx = common.NewObservedTx(tx, 1, pubKey, 1)
	msg = NewMsgObservedTxOut(common.ObservedTxs{observedTx}, GetRandomBech32Addr())
	require.ErrorContains(t, msg.ValidateBasic(), "too many observed tx gas coins")
}
