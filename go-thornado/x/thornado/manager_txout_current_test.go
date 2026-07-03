package thornado

import (
	"sort"
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

func TestTxOutEndBlockDoesNotMutatePrescribedPendingSignBTCGas(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	inHash := GetRandomTxHash()
	vaultPubKey := GetRandomPubKey()
	toAddress := GetRandomBTCAddress()
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 2_000_000)
	prescribedGas := common.NewCoin(common.BTCAsset, cosmos.NewUint(3_000))
	prescribedGas.Decimals = common.BTCChain.GetGasAssetDecimal()
	txOut := TxOut{
		Height: ctx.BlockHeight() - 3,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      toAddress,
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(1_997_000)),
				MaxGas:         common.Gas{prescribedGas},
				GasRate:        14,
				InHash:         inHash,
				ModuleName:     BaseName,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeRefund,
				SourceInputs:   []types.TxOutInput{sourceInput},
			},
		},
	}
	k.txOutByHeight[txOut.Height] = txOut
	k.txOutByHeight[ctx.BlockHeight()] = *NewTxOut(ctx.BlockHeight())
	k.networkFees[common.BTCChain] = NetworkFee{
		Chain:              common.BTCChain,
		TransactionSize:    221,
		TransactionFeeRate: 99,
	}
	mgr := newShielderFlowTestManager(k)
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	stored := k.txOutByHeight[txOut.Height].TxArray[0]
	if got := stored.GasRate; got != 14 {
		t.Fatalf("gas rate mutated: %d", got)
	}
	if got := stored.MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount.Uint64(); got != 3_000 {
		t.Fatalf("max gas mutated: %d", got)
	}
	if got := stored.Coin.Amount.Uint64(); got != 1_997_000 {
		t.Fatalf("coin mutated: %d", got)
	}
	if len(stored.SourceInputs) != 1 || !stored.SourceInputs[0].TxId.Equals(sourceInput.TxId) {
		t.Fatalf("source inputs mutated: %#v", stored.SourceInputs)
	}
}

func TestAppendBTCExactTxOutSeparatesBatchRefundAndMigration(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, RetiringVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 20_000_000)
	refundHash := GetRandomTxHash()
	refund := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      GetRandomBTCAddress(),
		VaultPubKey:    vaultPubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_800_000)),
		InHash:         refundHash,
		ModuleName:     BaseName,
		VaultPathIndex: common.MainVaultPathIndex,
		TxType:         types.TxOutTypeRefund,
		SourceInputs: []types.TxOutInput{{
			TxId:       refundHash,
			Vout:       0,
			AmountSats: 20_000_000,
		}},
	}
	if err := appendBTCExactTxOut(ctx, k, ctx.BlockHeight(), refund); err != nil {
		t.Fatal(err)
	}
	if len(k.txOutByHeight) != 1 {
		t.Fatalf("expected one refund batch txout, got %d", len(k.txOutByHeight))
	}
	var batchHeight int64
	for height, txOut := range k.txOutByHeight {
		batchHeight = height
		if txOut.Status != TxOutStatusPendingBatch || len(txOut.TxArray) != 1 || txOut.TxArray[0].TxType != types.TxOutTypeRefund {
			t.Fatalf("unexpected refund batch txout: %#v", txOut)
		}
	}

	migrate := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      GetRandomBTCAddress(),
		VaultPubKey:    vaultPubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_000_000)),
		InHash:         common.BlankTxID,
		VaultPathIndex: common.MainVaultPathIndex,
		TxType:         types.TxOutTypeMigrate,
		SourceInputs:   []types.TxOutInput{sourceInput},
	}
	if err := appendBTCExactTxOut(ctx, k, batchHeight, migrate); err != nil {
		t.Fatal(err)
	}
	if len(k.txOutByHeight) != 2 {
		t.Fatalf("expected refund batch and separate migration txout, got %d", len(k.txOutByHeight))
	}
	if got := k.txOutByHeight[batchHeight]; got.Status != TxOutStatusPendingBatch || len(got.TxArray) != 1 || got.TxArray[0].TxType != types.TxOutTypeRefund {
		t.Fatalf("refund batch was mutated: %#v", got)
	}
	migrationHeight := batchHeight + 1
	got := k.txOutByHeight[migrationHeight]
	if got.Status != TxOutStatusPendingSign || len(got.TxArray) != 1 || got.TxArray[0].TxType != types.TxOutTypeMigrate {
		t.Fatalf("migration was not separated into pending sign txout: %#v", got)
	}
}

func TestAppendBTCExactTxOutCapsBatchRecipients(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 2_000_000_000)

	total := btcMaxBatchRecipients + 3
	for i := 0; i < total; i++ {
		item := TxOutItem{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vaultPubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000_000)),
			InHash:         GetRandomTxHash(),
			ModuleName:     BaseName,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeRefund,
		}
		if err := appendBTCExactTxOut(ctx, k, ctx.BlockHeight(), item); err != nil {
			t.Fatal(err)
		}
	}

	if len(k.txOutByHeight) != 2 {
		t.Fatalf("expected the batch to split into 2 txout blocks, got %d", len(k.txOutByHeight))
	}
	var sizes []int
	for _, txOut := range k.txOutByHeight {
		if txOut.Status != TxOutStatusPendingBatch {
			t.Fatalf("expected pending_batch block, got %#v", txOut.Status)
		}
		if len(txOut.TxArray) > btcMaxBatchRecipients {
			t.Fatalf("batch block exceeds recipient cap: %d", len(txOut.TxArray))
		}
		sizes = append(sizes, len(txOut.TxArray))
	}
	sort.Ints(sizes)
	if sizes[0] != 3 || sizes[1] != btcMaxBatchRecipients {
		t.Fatalf("unexpected batch split sizes: %v", sizes)
	}
}

func TestRepairMixedBTCPendingBatchThenEndBlockPromotes(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(120)
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, RetiringVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	refundHash := GetRandomTxHash()
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 20_000_000)
	mixedHeight := int64(100)
	k.txOutByHeight[mixedHeight] = TxOut{
		Height: mixedHeight,
		Epoch:  9,
		Status: TxOutStatusPendingBatch,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      GetRandomBTCAddress(),
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_800_000)),
				InHash:         refundHash,
				ModuleName:     BaseName,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeRefund,
				SourceInputs: []types.TxOutInput{{
					TxId:       refundHash,
					Vout:       0,
					AmountSats: 20_000_000,
				}},
			},
			{
				Chain:          common.BTCChain,
				ToAddress:      GetRandomBTCAddress(),
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_000_000)),
				InHash:         common.BlankTxID,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeMigrate,
				SourceInputs:   []types.TxOutInput{sourceInput},
			},
		},
	}
	mgr := newShielderFlowTestManager(k)
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := RepairMixedBTCPendingBatches(ctx, k); err != nil {
		t.Fatal(err)
	}
	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	refundTxOut := k.txOutByHeight[mixedHeight]
	if refundTxOut.Status != TxOutStatusPendingSign || len(refundTxOut.TxArray) != 1 || refundTxOut.TxArray[0].TxType != types.TxOutTypeRefund {
		t.Fatalf("refund batch was not promoted cleanly: %#v", refundTxOut)
	}
	migrationTxOut := k.txOutByHeight[mixedHeight+1]
	if migrationTxOut.Status != TxOutStatusPendingSign || len(migrationTxOut.TxArray) != 1 || migrationTxOut.TxArray[0].TxType != types.TxOutTypeMigrate {
		t.Fatalf("migration was not split into its own pending sign txout: %#v", migrationTxOut)
	}
}

func TestAppendBTCExactTxOutGroupsBatchesPerVault(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	vaultA := GetRandomPubKey()
	vaultB := GetRandomPubKey()
	addTestBTCVaultSourceInput(t, ctx, k, vaultA, 20_000_000)
	addTestBTCVaultSourceInput(t, ctx, k, vaultB, 20_000_000)

	newRefund := func(vault common.PubKey) TxOutItem {
		return TxOutItem{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vault,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000)),
			InHash:         GetRandomTxHash(),
			ModuleName:     BaseName,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeRefund,
		}
	}

	windowBlocks := constants.MinutesToBlocks(
		k.GetConfigInt64(ctx, constants.Withdrawal_BatchWindowMinutes),
		k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds),
	)
	closeHeight := ctx.BlockHeight() + windowBlocks

	if err := appendBTCExactTxOut(ctx, k, ctx.BlockHeight(), newRefund(vaultA)); err != nil {
		t.Fatal(err)
	}
	if err := appendBTCExactTxOut(ctx, k, ctx.BlockHeight(), newRefund(vaultA)); err != nil {
		t.Fatal(err)
	}
	if err := appendBTCExactTxOut(ctx, k, ctx.BlockHeight(), newRefund(vaultB)); err != nil {
		t.Fatal(err)
	}

	batchA := k.txOutByHeight[closeHeight]
	if batchA.Status != TxOutStatusPendingBatch || len(batchA.TxArray) != 2 || batchA.Epoch != 0 {
		t.Fatalf("vault A batch not grouped at close height: %#v", batchA)
	}
	for _, item := range batchA.TxArray {
		if !item.VaultPubKey.Equals(vaultA) {
			t.Fatalf("foreign vault item in vault A batch: %#v", item)
		}
	}
	batchB := k.txOutByHeight[closeHeight+1]
	if batchB.Status != TxOutStatusPendingBatch || len(batchB.TxArray) != 1 || batchB.Epoch != 0 || !batchB.TxArray[0].VaultPubKey.Equals(vaultB) {
		t.Fatalf("vault B batch not separated: %#v", batchB)
	}

	// once vault A's batch window has closed, a new item opens the next epoch
	laterHeight := closeHeight + 50
	laterCtx := ctx.WithBlockHeight(laterHeight)
	if err := appendBTCExactTxOut(laterCtx, k, laterCtx.BlockHeight(), newRefund(vaultA)); err != nil {
		t.Fatal(err)
	}
	nextBatchA := k.txOutByHeight[laterHeight+windowBlocks]
	if nextBatchA.Status != TxOutStatusPendingBatch || len(nextBatchA.TxArray) != 1 || nextBatchA.Epoch != 1 {
		t.Fatalf("vault A next batch epoch not incremented: %#v", nextBatchA)
	}
	if got := k.txOutByHeight[closeHeight]; len(got.TxArray) != 2 {
		t.Fatalf("closed vault A batch was mutated: %#v", got)
	}
}

func TestBatchPromotionWaitsForPriorVaultBatch(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(120)
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 40_000_000)

	prescribedGas := common.NewCoin(common.BTCAsset, cosmos.NewUint(3_000))
	prescribedGas.Decimals = common.BTCChain.GetGasAssetDecimal()
	inFlightHash := GetRandomTxHash()
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Epoch:  0,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vaultPubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000)),
			MaxGas:         common.Gas{prescribedGas},
			GasRate:        14,
			InHash:         inFlightHash,
			ModuleName:     BaseName,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeRefund,
			SourceInputs: []types.TxOutInput{{
				TxId:       inFlightHash,
				Vout:       0,
				AmountSats: 1_003_000,
			}},
		}},
	}
	k.txOutByHeight[95] = TxOut{
		Height: 95,
		Epoch:  1,
		Status: TxOutStatusPendingBatch,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vaultPubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(2_000_000)),
			InHash:         GetRandomTxHash(),
			ModuleName:     BaseName,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeRefund,
		}},
	}
	mgr := newShielderFlowTestManager(k)
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	if got := k.txOutByHeight[95]; got.Status != TxOutStatusPendingBatch {
		t.Fatalf("next batch was promoted while prior batch still pending: %#v", got)
	}
	if got := k.txOutByHeight[90]; got.Status != TxOutStatusPendingSign {
		t.Fatalf("in-flight batch status changed unexpectedly: %#v", got)
	}

	// completing the prior batch releases the next one
	inFlight := k.txOutByHeight[90]
	inFlight.TxArray[0].OutHash = GetRandomTxHash()
	k.txOutByHeight[90] = inFlight

	if err := store.EndBlock(ctx.WithBlockHeight(121), mgr); err != nil {
		t.Fatal(err)
	}
	if got := k.txOutByHeight[90]; got.Status != TxOutStatusComplete {
		t.Fatalf("signed batch not marked complete: %#v", got)
	}
	if got := k.txOutByHeight[95]; got.Status != TxOutStatusPendingSign || got.SigningAttempt != 0 {
		t.Fatalf("next batch not promoted after prior batch completed: %#v", got)
	}
}

func TestRepairSplitsMixedVaultPendingBatch(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(120)
	k := newShielderFlowTestKeeper()
	vaultA := GetRandomPubKey()
	vaultB := GetRandomPubKey()
	addTestBTCVaultSourceInput(t, ctx, k, vaultA, 20_000_000)
	addTestBTCVaultSourceInput(t, ctx, k, vaultB, 20_000_000)

	mixedHeight := int64(100)
	k.txOutByHeight[mixedHeight] = TxOut{
		Height: mixedHeight,
		Epoch:  3,
		Status: TxOutStatusPendingBatch,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      GetRandomBTCAddress(),
				VaultPubKey:    vaultA,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000)),
				InHash:         GetRandomTxHash(),
				ModuleName:     BaseName,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeRefund,
			},
			{
				Chain:          common.BTCChain,
				ToAddress:      GetRandomBTCAddress(),
				VaultPubKey:    vaultB,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(2_000_000)),
				InHash:         GetRandomTxHash(),
				ModuleName:     BaseName,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeRefund,
			},
		},
	}
	mgr := newShielderFlowTestManager(k)
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := RepairMixedBTCPendingBatches(ctx, k); err != nil {
		t.Fatal(err)
	}
	splitA := k.txOutByHeight[mixedHeight]
	if splitA.Status != TxOutStatusPendingBatch || len(splitA.TxArray) != 1 || !splitA.TxArray[0].VaultPubKey.Equals(vaultA) {
		t.Fatalf("vault A group not retained after split: %#v", splitA)
	}
	splitB := k.txOutByHeight[mixedHeight+1]
	if splitB.Status != TxOutStatusPendingBatch || len(splitB.TxArray) != 1 || !splitB.TxArray[0].VaultPubKey.Equals(vaultB) || splitB.Epoch != 3 {
		t.Fatalf("vault B group not split into its own batch: %#v", splitB)
	}

	// separate vaults promote independently in the same pass
	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	if got := k.txOutByHeight[mixedHeight]; got.Status != TxOutStatusPendingSign {
		t.Fatalf("vault A batch not promoted: %#v", got)
	}
	if got := k.txOutByHeight[mixedHeight+1]; got.Status != TxOutStatusPendingSign {
		t.Fatalf("vault B batch not promoted: %#v", got)
	}
}

func TestTxOutEndBlockSplitsMixedPendingSignAndAssignsLeader(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(160)
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	nodePubKey := GetRandomPubKey()
	nodeAddr := GetRandomBech32Addr()
	account := NewNodeAccount(nodeAddr, NodeActive, common.NewPubKeySet(nodePubKey), nodePubKey.String(), cosmos.NewUint(1), common.Address(nodeAddr.String()), 1)
	account.SignerMembership = []string{vaultPubKey.String()}
	k.nodeAccounts[nodeAddr.String()] = account

	refundHash := GetRandomTxHash()
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 20_000_000)
	mixedHeight := int64(140)
	k.txOutByHeight[mixedHeight] = TxOut{
		Height: mixedHeight,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      GetRandomBTCAddress(),
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_800_000)),
				InHash:         refundHash,
				ModuleName:     BaseName,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeRefund,
				SourceInputs: []types.TxOutInput{{
					TxId:       refundHash,
					AmountSats: 20_000_000,
				}},
			},
			{
				Chain:          common.BTCChain,
				ToAddress:      GetRandomBTCAddress(),
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_000_000)),
				InHash:         common.BlankTxID,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeConsolidate,
				SourceInputs:   []types.TxOutInput{sourceInput},
			},
		},
	}
	mgr := newShielderFlowTestManager(k)
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}

	batchTxOut := k.txOutByHeight[mixedHeight]
	if batchTxOut.Status != TxOutStatusPendingSign || len(batchTxOut.TxArray) != 1 || batchTxOut.TxArray[0].TxType != types.TxOutTypeRefund {
		t.Fatalf("batchable item was not preserved as pending sign: %#v", batchTxOut)
	}
	internalTxOut := k.txOutByHeight[mixedHeight+1]
	if internalTxOut.Status != TxOutStatusPendingSign || len(internalTxOut.TxArray) != 1 || internalTxOut.TxArray[0].TxType != types.TxOutTypeConsolidate {
		t.Fatalf("internal item was not split into pending sign txout: %#v", internalTxOut)
	}

	if err := store.EndBlock(ctx.WithBlockHeight(ctx.BlockHeight()+1), mgr); err != nil {
		t.Fatal(err)
	}
	if got := k.txOutByHeight[mixedHeight].SigningLeader; !got.Equals(nodePubKey) {
		t.Fatalf("batchable txout leader not assigned: %s", got)
	}
	if got := k.txOutByHeight[mixedHeight+1].SigningLeader; !got.Equals(nodePubKey) {
		t.Fatalf("internal txout leader not assigned: %s", got)
	}
}

func TestTxOutEndBlockAssignsLeaderForInternalPendingSign(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(200)
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	nodePubKey := GetRandomPubKey()
	nodeAddr := GetRandomBech32Addr()
	account := NewNodeAccount(nodeAddr, NodeActive, common.NewPubKeySet(nodePubKey), nodePubKey.String(), cosmos.NewUint(1), common.Address(nodeAddr.String()), 1)
	account.SignerMembership = []string{vaultPubKey.String()}
	k.nodeAccounts[nodeAddr.String()] = account

	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 20_000_000)
	height := int64(190)
	k.txOutByHeight[height] = TxOut{
		Height: height,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vaultPubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_000_000)),
			InHash:         common.BlankTxID,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeConsolidate,
			SourceInputs:   []types.TxOutInput{sourceInput},
		}},
	}
	mgr := newShielderFlowTestManager(k)
	store := newTxOutStorage(k, constants.NewConfigValue(), nil, &mgr.gas)

	if err := store.EndBlock(ctx, mgr); err != nil {
		t.Fatal(err)
	}
	if got := k.txOutByHeight[height].SigningLeader; !got.Equals(nodePubKey) {
		t.Fatalf("internal txout leader not assigned: %s", got)
	}
}
