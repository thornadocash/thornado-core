package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func testUserDepositPathIndex(t *testing.T, depositIndex uint64) uint64 {
	t.Helper()
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, depositIndex, common.DepositPathCommitmentRoot)
	if err != nil {
		t.Fatal(err)
	}
	return pathIndex
}

func TestObservedOutboundMatchesBTCSweepWithActualFee(t *testing.T) {
	pubKey := GetRandomPubKey()
	pathIndex := testUserDepositPathIndex(t, 0)
	from, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pe4qjnvwtqluzkmywrcxl3wz8xyacuxpels3s7tsnurrpw34x2jms3lusvd")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("6D708C41AD616354D2632D61985DD460E0E231D62F32222774AAD9B65758035A")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("BF25BFC8FFED823FBD369F1FED64128DFB5E94E9AA87723D9E688C00087A3DE1")
	if err != nil {
		t.Fatal(err)
	}

	item := types.TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      to,
		VaultPubKey:    pubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_987_000)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_000))},
		GasRate:        13,
		InHash:         inHash,
		VaultPathIndex: pathIndex,
		TxType:         types.TxOutTypeSweep,
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_997_855))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_145))},
		),
		157,
		pubKey,
		157,
	)

	if !observedOutboundMatchesTxOut(tx, item) {
		t.Fatal("expected observed BTC sweep to match txout item")
	}
}

func TestObservedOutboundRejectsBTCSweepFromWrongPath(t *testing.T) {
	pubKey := GetRandomPubKey()
	expectedPath := testUserDepositPathIndex(t, 0)
	wrongPath := testUserDepositPathIndex(t, 1)
	from, err := common.DeriveBTCTaprootAddress(pubKey, wrongPath)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pe4qjnvwtqluzkmywrcxl3wz8xyacuxpels3s7tsnurrpw34x2jms3lusvd")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("6D708C41AD616354D2632D61985DD460E0E231D62F32222774AAD9B65758035A")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("BF25BFC8FFED823FBD369F1FED64128DFB5E94E9AA87723D9E688C00087A3DE1")
	if err != nil {
		t.Fatal(err)
	}

	item := types.TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      to,
		VaultPubKey:    pubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_987_000)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_000))},
		GasRate:        13,
		InHash:         inHash,
		VaultPathIndex: expectedPath,
		TxType:         types.TxOutTypeSweep,
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_997_855))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_145))},
		),
		157,
		pubKey,
		157,
	)

	if observedOutboundMatchesTxOut(tx, item) {
		t.Fatal("expected observed BTC sweep from wrong path to be rejected")
	}
}

func TestObservedOutboundRejectsBTCSweepOverMaxGas(t *testing.T) {
	pubKey := GetRandomPubKey()
	pathIndex := testUserDepositPathIndex(t, 0)
	from, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pe4qjnvwtqluzkmywrcxl3wz8xyacuxpels3s7tsnurrpw34x2jms3lusvd")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("6D708C41AD616354D2632D61985DD460E0E231D62F32222774AAD9B65758035A")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("BF25BFC8FFED823FBD369F1FED64128DFB5E94E9AA87723D9E688C00087A3DE1")
	if err != nil {
		t.Fatal(err)
	}

	item := types.TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      to,
		VaultPubKey:    pubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_987_000)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_000))},
		GasRate:        13,
		InHash:         inHash,
		VaultPathIndex: pathIndex,
		TxType:         types.TxOutTypeSweep,
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_986_999))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_001))},
		),
		157,
		pubKey,
		157,
	)

	if observedOutboundMatchesTxOut(tx, item) {
		t.Fatal("expected observed BTC sweep over max gas to be rejected")
	}
}

func TestObservedOutboundRejectsAlreadyCompletedBTCSweep(t *testing.T) {
	pubKey := GetRandomPubKey()
	pathIndex := testUserDepositPathIndex(t, 0)
	from, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pe4qjnvwtqluzkmywrcxl3wz8xyacuxpels3s7tsnurrpw34x2jms3lusvd")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("6D708C41AD616354D2632D61985DD460E0E231D62F32222774AAD9B65758035A")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("BF25BFC8FFED823FBD369F1FED64128DFB5E94E9AA87723D9E688C00087A3DE1")
	if err != nil {
		t.Fatal(err)
	}

	item := types.TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      to,
		VaultPubKey:    pubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_987_000)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_000))},
		GasRate:        13,
		InHash:         inHash,
		OutHash:        outHash,
		VaultPathIndex: pathIndex,
		TxType:         types.TxOutTypeSweep,
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_997_855))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_145))},
		),
		157,
		pubKey,
		157,
	)

	if observedOutboundMatchesTxOut(tx, item) {
		t.Fatal("expected already completed BTC sweep txout to be rejected")
	}
}
