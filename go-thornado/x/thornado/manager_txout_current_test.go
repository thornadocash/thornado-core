package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestTxOutEndBlockUpdatesGasWithoutInboundVoter(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	inHash := GetRandomTxHash()
	vaultPubKey := GetRandomPubKey()
	toAddress := GetRandomBTCAddress()
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 2_000_000)
	txOut := TxOut{
		Height: ctx.BlockHeight(),
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   toAddress,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000)),
				InHash:      inHash,
				ModuleName:  BaseName,
				TxType:      types.TxOutTypeRefund,
			},
		},
	}
	k.txOutByHeight[txOut.Height] = txOut
	mgr := newShielderFlowTestManager(k)
	mgr.gas = shielderFlowTestGasManager{
		maxGas: common.NewCoin(common.BTCAsset, cosmos.NewUint(3_094)),
	}
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	stored := k.txOutByHeight[txOut.Height]
	expectedGas, err := btcExactGasCoin(vaultPubKey, common.MainVaultPathIndex, []common.Address{toAddress}, []types.TxOutInput{sourceInput}, 14)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.TxArray[0].MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount.Uint64(); got != expectedGas.Amount.Uint64() {
		t.Fatalf("unexpected max gas: %d/%d", got, expectedGas.Amount.Uint64())
	}
	if got := stored.TxArray[0].GasRate; got != 14 {
		t.Fatalf("unexpected gas rate: %d", got)
	}
	if len(stored.TxArray[0].SourceInputs) != 1 || !stored.TxArray[0].SourceInputs[0].TxId.Equals(sourceInput.TxId) {
		t.Fatalf("unexpected source inputs: %#v", stored.TxArray[0].SourceInputs)
	}
	if _, ok := k.txInVoters[inHash.String()]; ok {
		t.Fatal("missing inbound voter should not be created")
	}
}

func TestRefreshBTCExactTxOutInternalSweepUsesSelectedInputLessGas(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	inHash := GetRandomTxHash()
	vaultPubKey := GetRandomPubKey()
	toAddress := GetRandomBTCAddress()
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 20_000_000)
	txOut := TxOut{
		Height: ctx.BlockHeight() - 5,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      toAddress,
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_990_000)),
				InHash:         inHash,
				ModuleName:     BaseName,
				VaultPathIndex: testUserDepositPathIndex(t, 0),
				TxType:         types.TxOutTypeSweep,
				SourceInputs:   []types.TxOutInput{sourceInput},
			},
		},
	}
	k.txOutByHeight[txOut.Height] = txOut
	k.txOutByHeight[ctx.BlockHeight()] = *NewTxOut(ctx.BlockHeight())
	mgr := newShielderFlowTestManager(k)
	mgr.gas = shielderFlowTestGasManager{
		maxGas: common.NewCoin(common.BTCAsset, cosmos.NewUint(3_094)),
	}
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	stored := k.txOutByHeight[txOut.Height]
	expectedGas, err := btcExactGasCoin(vaultPubKey, txOut.TxArray[0].VaultPathIndex, []common.Address{toAddress}, []types.TxOutInput{sourceInput}, 14)
	if err != nil {
		t.Fatal(err)
	}
	expectedAmount := cosmos.NewUint(sourceInput.AmountSats).Sub(expectedGas.Amount)
	if got := stored.TxArray[0].Coin.Amount; !got.Equal(expectedAmount) {
		t.Fatalf("unexpected sweep coin amount: %s/%s", got, expectedAmount)
	}
	if got := stored.TxArray[0].MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount.Uint64(); got != expectedGas.Amount.Uint64() {
		t.Fatalf("unexpected sweep max gas: %d/%d", got, expectedGas.Amount.Uint64())
	}
}
