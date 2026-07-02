package thornado

import (
	"fmt"
	"sort"
	"strconv"
	"testing"

	"cosmossdk.io/log"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func testContext(height int64) cosmos.Context {
	return cosmos.Context{}.WithBlockHeight(height).WithLogger(log.NewNopLogger())
}

func (k *shielderFlowTestKeeper) SetVault(_ cosmos.Context, vault Vault) error {
	for i := range k.baseVaults {
		if k.baseVaults[i].PubKey.Equals(vault.PubKey) {
			k.baseVaults[i] = vault
			return nil
		}
	}
	k.baseVaults = append(k.baseVaults, vault)
	return nil
}

func (k *shielderFlowTestKeeper) GetVault(_ cosmos.Context, pk common.PubKey) (Vault, error) {
	for _, vault := range k.baseVaults {
		if vault.PubKey.Equals(pk) {
			return vault, nil
		}
	}
	return Vault{}, nil
}

func (k *shielderFlowTestKeeper) GetBaseVaults(_ cosmos.Context) (Vaults, error) {
	return append(Vaults{}, k.baseVaults...), nil
}

func (k *shielderFlowTestKeeper) SetTxOut(_ cosmos.Context, txOut *TxOut) error {
	k.txOutByHeight[txOut.Height] = *txOut
	k.txOuts = nil
	heights := make([]int64, 0, len(k.txOutByHeight))
	for height := range k.txOutByHeight {
		heights = append(heights, height)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	for _, height := range heights {
		k.txOuts = append(k.txOuts, k.txOutByHeight[height].TxArray...)
	}
	return nil
}

func (k *shielderFlowTestKeeper) GetTxOut(_ cosmos.Context, height int64) (*TxOut, error) {
	txOut, ok := k.txOutByHeight[height]
	if !ok {
		return NewTxOut(height), nil
	}
	return &txOut, nil
}

func (k *shielderFlowTestKeeper) GetTxOutIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	heights := make([]int64, 0, len(k.txOutByHeight))
	for height := range k.txOutByHeight {
		heights = append(heights, height)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	for _, height := range heights {
		txOut := k.txOutByHeight[height]
		value, _ := k.Cdc().Marshal(&txOut)
		iter.AddItem([]byte(strconv.FormatInt(height, 10)), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) SetSolvencyVoter(_ cosmos.Context, voter types.SolvencyVoter) {
	k.solvencyVoters[fmt.Sprintf("%s-%s", voter.Chain, voter.Id)] = voter
}

func (k *shielderFlowTestKeeper) GetSolvencyVoter(_ cosmos.Context, txID common.TxID, chain common.Chain) (types.SolvencyVoter, error) {
	return k.solvencyVoters[fmt.Sprintf("%s-%s", chain, txID)], nil
}

func (k *shielderFlowTestKeeper) GetSolvencyVoterIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	for id, voter := range k.solvencyVoters {
		value, _ := k.Cdc().Marshal(&voter)
		iter.AddItem([]byte(id), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) GetObservedTxOutVoterIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	keys := make([]string, 0, len(k.txOutVoters))
	for key := range k.txOutVoters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		voter := k.txOutVoters[key]
		value, _ := k.Cdc().Marshal(&voter)
		iter.AddItem([]byte(key), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) GetObservedTxInVoterIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	keys := make([]string, 0, len(k.txInVoters))
	for key := range k.txInVoters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		voter := k.txInVoters[key]
		value, _ := k.Cdc().Marshal(&voter)
		iter.AddItem([]byte(key), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) SetObservedTxInVoter(_ cosmos.Context, voter ObservedTxVoter) {
	k.txInVoters[voter.TxID.String()] = voter
}

func (k *shielderFlowTestKeeper) GetObservedTxInVoter(_ cosmos.Context, hash common.TxID) (ObservedTxVoter, error) {
	voter, ok := k.txInVoters[hash.String()]
	if !ok {
		return ObservedTxVoter{TxID: hash}, nil
	}
	return voter, nil
}

func (k *shielderFlowTestKeeper) SetObservedTxOutVoter(_ cosmos.Context, voter ObservedTxVoter) {
	k.txOutVoters[voter.TxID.String()] = voter
}

func (k *shielderFlowTestKeeper) GetObservedTxOutVoter(_ cosmos.Context, hash common.TxID) (ObservedTxVoter, error) {
	voter, ok := k.txOutVoters[hash.String()]
	if !ok {
		return ObservedTxVoter{TxID: hash}, nil
	}
	return voter, nil
}

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

func TestObservedOutboundAlreadyMatchedBTCSweepWithActualFee(t *testing.T) {
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

	txOut := &TxOut{
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    pubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_997_855)),
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_000))},
				GasRate:        13,
				InHash:         inHash,
				OutHash:        outHash,
				VaultPathIndex: pathIndex,
				TxType:         types.TxOutTypeSweep,
				SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
			},
		},
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
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0}}

	if !observedOutboundAlreadyMatchedTxOut(txOut, tx) {
		t.Fatal("expected already matched BTC sweep to remain matched after coin is updated to actual payout")
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

func TestObservedOutboundMatchesPrescriptiveSweepBySourceInput(t *testing.T) {
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
	firstIn, err := common.NewTxID("1111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	secondIn, err := common.NewTxID("2222222222222222222222222222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("3333333333333333333333333333333333333333333333333333333333333333")
	if err != nil {
		t.Fatal(err)
	}

	base := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      to,
		VaultPubKey:    pubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_987_000)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(13_000))},
		GasRate:        13,
		VaultPathIndex: pathIndex,
		TxType:         types.TxOutTypeSweep,
	}
	first := base
	first.InHash = firstIn
	first.SourceInputs = []types.TxOutInput{{TxId: firstIn, Vout: 0, AmountSats: 100_000_000}}
	second := base
	second.InHash = secondIn
	second.SourceInputs = []types.TxOutInput{{TxId: secondIn, Vout: 1, AmountSats: 100_000_000}}

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
	tx.Tx.SourceInputs = []common.TxInput{{TxID: secondIn, Vout: 1, AmountSats: 100_000_000}}

	if observedOutboundMatchesTxOut(tx, first) {
		t.Fatal("expected first same-address sweep to be rejected by source input")
	}
	if !observedOutboundMatchesTxOut(tx, second) {
		t.Fatal("expected second same-address sweep to match by source input")
	}
}

func TestTxForOutboundReplayMatchUsesFreshSourceInputs(t *testing.T) {
	pubKey := GetRandomPubKey()
	outHash, err := common.NewTxID("4444444444444444444444444444444444444444444444444444444444444444")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("5555555555555555555555555555555555555555555555555555555555555555")
	if err != nil {
		t.Fatal(err)
	}
	stored := common.NewObservedTx(
		common.NewTx(outHash, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}),
		157,
		pubKey,
		157,
	)
	fresh := stored
	fresh.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 1}}

	selected := txForOutboundReplayMatch(ObservedTxVoter{Tx: stored}, fresh)
	if len(selected.Tx.SourceInputs) != 1 || !selected.Tx.SourceInputs[0].TxID.Equals(inHash) {
		t.Fatal("expected fresh replay observation with source inputs to be selected")
	}
}

func TestObservedOutboundBatchSingleObservationSettlesAllItems(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	txOut, sourceInput, sourceAddr := testQueuedBTCBatch(t, ctx, k, vaultPubKey)

	totalGas := testTxOutTotalBTCMaxGas(txOut)
	outHash := GetRandomTxHash()
	observedTx := common.NewTx(
		outHash,
		sourceAddr,
		txOut.TxArray[0].ToAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(700_000))),
		common.Gas{common.NewCoin(common.BTCAsset, totalGas)},
	)
	observedTx.SourceInputs = []common.TxInput{{TxID: sourceInput.TxId, Vout: sourceInput.Vout, AmountSats: sourceInput.AmountSats}}
	observed := common.NewObservedTx(observedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())

	if !markObservedOutboundTxOut(ctx, mgr, observed) {
		t.Fatal("expected one aggregate batch observation to settle all txout items")
	}
	stored := k.txOutByHeight[ctx.BlockHeight()]
	for i, item := range stored.TxArray {
		if !item.OutHash.Equals(outHash) || item.OutVout != uint32(i) {
			t.Fatalf("batch item %d was not settled by aggregate observation: %#v", i, item)
		}
	}
}

func TestObservedOutboundBatchRequiresPrescribedSourceInputs(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	txOut, sourceInput, sourceAddr := testQueuedBTCBatch(t, ctx, k, vaultPubKey)

	wrongInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 2_000_000)
	if wrongInput.TxId.Equals(sourceInput.TxId) {
		t.Fatal("test source inputs should differ")
	}
	observedTx := common.NewTx(
		GetRandomTxHash(),
		sourceAddr,
		txOut.TxArray[0].ToAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(700_000))),
		common.Gas{common.NewCoin(common.BTCAsset, testTxOutTotalBTCMaxGas(txOut))},
	)
	observedTx.SourceInputs = []common.TxInput{{TxID: wrongInput.TxId, Vout: wrongInput.Vout, AmountSats: wrongInput.AmountSats}}
	observed := common.NewObservedTx(observedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())

	if markObservedOutboundTxOut(ctx, mgr, observed) {
		t.Fatal("batch observation with wrong source inputs should not settle txout items")
	}
	stored := k.txOutByHeight[ctx.BlockHeight()]
	for i, item := range stored.TxArray {
		if !item.OutHash.IsEmpty() {
			t.Fatalf("batch item %d was incorrectly settled: %#v", i, item)
		}
	}
}

func TestObservedOutboundBatchOverMaxGasStillSettles(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	txOut, sourceInput, sourceAddr := testQueuedBTCBatch(t, ctx, k, vaultPubKey)

	totalGas := testTxOutTotalBTCMaxGas(txOut).Add(cosmos.NewUint(1))
	outHash := GetRandomTxHash()
	observedTx := common.NewTx(
		outHash,
		sourceAddr,
		txOut.TxArray[0].ToAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(700_000))),
		common.Gas{common.NewCoin(common.BTCAsset, totalGas)},
	)
	observedTx.SourceInputs = []common.TxInput{{TxID: sourceInput.TxId, Vout: sourceInput.Vout, AmountSats: sourceInput.AmountSats}}
	observed := common.NewObservedTx(observedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())

	if !markObservedOutboundTxOut(ctx, mgr, observed) {
		t.Fatal("expected signed batch with matching outputs and source inputs to settle despite over-max gas")
	}
	stored := k.txOutByHeight[ctx.BlockHeight()]
	for i, item := range stored.TxArray {
		if !item.OutHash.Equals(outHash) || item.OutVout != uint32(i) {
			t.Fatalf("batch item %d was not settled by over-max gas observation: %#v", i, item)
		}
	}
}

func TestObservedHistoricalOutboundBatchSettlesOutsideSigningWindow(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(500)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	k.configs[constants.Chain_BlockTimeSeconds] = 6
	k.configs[constants.Keysign_PeriodMinutes] = 5
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	oldCtx := testContext(100)
	txOut, sourceInput, sourceAddr := testQueuedBTCBatch(t, oldCtx, k, vaultPubKey)

	outHash := GetRandomTxHash()
	observedTx := common.NewTx(
		outHash,
		sourceAddr,
		txOut.TxArray[0].ToAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(700_000))),
		common.Gas{common.NewCoin(common.BTCAsset, testTxOutTotalBTCMaxGas(txOut))},
	)
	observedTx.SourceInputs = []common.TxInput{{TxID: sourceInput.TxId, Vout: sourceInput.Vout, AmountSats: sourceInput.AmountSats}}
	observed := common.NewObservedTx(observedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())

	matched, newlyMatched := markObservedOutboundTxOutStatus(ctx, mgr, observed)
	if !matched || !newlyMatched {
		t.Fatalf("expected historical batch to settle outside signing window, got matched=%v newly=%v", matched, newlyMatched)
	}
	stored := k.txOutByHeight[oldCtx.BlockHeight()]
	for i, item := range stored.TxArray {
		if !item.OutHash.Equals(outHash) || item.OutVout != uint32(i) {
			t.Fatalf("historical batch item %d was not settled: %#v", i, item)
		}
	}
}

func TestCommonOutboundRequiresPrescribedBTCSourceInputs(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	from, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to := GetRandomBTCAddress()
	inHash := GetRandomTxHash()
	outHash := GetRandomTxHash()
	sourceInput := types.TxOutInput{TxId: inHash, Vout: 0, AmountSats: 20_000_000}
	item := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      to,
		VaultPubKey:    vaultPubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(19_998_350)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_650))},
		InHash:         inHash,
		VaultPathIndex: common.MainVaultPathIndex,
		TxType:         types.TxOutTypeSweep,
		SourceInputs:   []types.TxOutInput{sourceInput},
	}
	if err := k.SetTxOut(ctx, &TxOut{Height: ctx.BlockHeight(), TxArray: []TxOutItem{item}}); err != nil {
		t.Fatal(err)
	}
	k.SetObservedTxInVoter(ctx, ObservedTxVoter{TxID: inHash, Actions: []TxOutItem{item}})

	wrongObservedTx := common.NewTx(
		outHash,
		from,
		to,
		common.NewCoins(item.Coin),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_650))},
	)
	wrongObservedTx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: sourceInput.AmountSats}}
	wrongObserved := common.NewObservedTx(wrongObservedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())
	if _, err := NewCommonOutboundTxHandler(mgr).handle(ctx, wrongObserved, inHash); err != nil {
		t.Fatal(err)
	}
	stored, err := k.GetTxOut(ctx, ctx.BlockHeight())
	if err != nil {
		t.Fatal(err)
	}
	if !stored.TxArray[0].OutHash.IsEmpty() {
		t.Fatalf("wrong source input completed txout: %#v", stored.TxArray[0])
	}

	rightObservedTx := wrongObservedTx
	rightObservedTx.SourceInputs = []common.TxInput{{TxID: sourceInput.TxId, Vout: sourceInput.Vout, AmountSats: sourceInput.AmountSats}}
	rightObserved := common.NewObservedTx(rightObservedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())
	if _, err := NewCommonOutboundTxHandler(mgr).handle(ctx, rightObserved, inHash); err != nil {
		t.Fatal(err)
	}
	stored, err = k.GetTxOut(ctx, ctx.BlockHeight())
	if err != nil {
		t.Fatal(err)
	}
	if !stored.TxArray[0].OutHash.Equals(outHash) {
		t.Fatalf("correct source input did not complete txout: %#v", stored.TxArray[0])
	}
}

func TestDirectBaseVaultInboundRefundPreemptsDepositMatch(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(200)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	rootAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.SetDepositAddress(ctx, types.DepositAddress{
		Owner:       cosmos.AccAddress("owner"),
		Address:     rootAddr,
		VaultPubKey: vaultPubKey,
		PathIndex:   common.MainVaultPathIndex,
	}); err != nil {
		t.Fatal(err)
	}
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		rootAddr,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{},
	)
	tx.SourceVout = 5
	observed := common.NewObservedTx(tx, 100, vaultPubKey, 100)
	voter := ObservedTxVoter{TxID: txID, Tx: observed}
	k.SetObservedTxInVoter(ctx, voter)

	if err := handleObservedTxInQuorum(ctx, mgr, nil, nil, nil, observed, voter, nil, true); err != nil {
		t.Fatal(err)
	}
	record := k.deposits[txID.String()]
	if record.Status != types.DepositStatusReturnQueued ||
		record.RefundEligibleHeight != ctx.BlockHeight() ||
		record.RefundQueuedHeight != ctx.BlockHeight() ||
		!record.SweepComplete {
		t.Fatalf("direct base-vault inbound was not queued for refund: %#v", record)
	}
	if string(record.Owner) != tx.FromAddress.String() {
		t.Fatalf("direct base-vault refund owner mismatch: %q/%q", string(record.Owner), tx.FromAddress.String())
	}
	if len(k.txOuts) != 1 || k.txOuts[0].GetTxType() != types.TxOutTypeRefund {
		t.Fatalf("expected one refund txout, got %#v", k.txOuts)
	}
	if len(k.txOuts[0].SourceInputs) != 1 ||
		!k.txOuts[0].SourceInputs[0].TxId.Equals(txID) ||
		k.txOuts[0].SourceInputs[0].Vout != 5 {
		t.Fatalf("refund did not spend direct root deposit source: %#v", k.txOuts[0].SourceInputs)
	}
}

func TestDirectBaseVaultInboundRefundWhenVaultRetiring(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(24052)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	mgr := newShielderFlowTestManager(k)
	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, RetiringVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	rootAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		rootAddr,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{},
	)
	tx.SourceVout = 0
	observed := common.NewObservedTx(tx, 67508, vaultPubKey, 67508)
	voter := ObservedTxVoter{TxID: txID, Tx: observed}
	k.SetObservedTxInVoter(ctx, voter)

	if err := handleObservedTxInQuorum(ctx, mgr, nil, nil, nil, observed, voter, nil, true); err != nil {
		t.Fatal(err)
	}
	record := k.deposits[txID.String()]
	if record.Status != types.DepositStatusReturnQueued || !record.SweepComplete {
		t.Fatalf("retiring base-vault inbound was not queued for refund: %#v", record)
	}
	if len(k.txOuts) != 1 || k.txOuts[0].GetTxType() != types.TxOutTypeRefund {
		t.Fatalf("expected one refund txout, got %#v", k.txOuts)
	}
	if !k.txOuts[0].InHash.Equals(txID) ||
		len(k.txOuts[0].SourceInputs) != 1 ||
		!k.txOuts[0].SourceInputs[0].TxId.Equals(txID) ||
		k.txOuts[0].SourceInputs[0].Vout != 0 {
		t.Fatalf("refund did not pin the direct root deposit source: %#v", k.txOuts[0])
	}
}

func TestBTCConsolidateObservedSubsetSettlesStaleTxOut(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	vaultPubKey := GetRandomPubKey()
	rootAddr, err := common.DeriveBTCTaprootAddress(vaultPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	inHash := GetRandomTxHash()
	outHash := GetRandomTxHash()
	sourceInputs := []types.TxOutInput{
		{TxId: GetRandomTxHash(), Vout: 0, AmountSats: 19_998_350},
		{TxId: GetRandomTxHash(), Vout: 1, AmountSats: 19_998_350},
		{TxId: GetRandomTxHash(), Vout: 2, AmountSats: 19_997_690},
	}
	staleCoin := cosmos.NewUint(sourceInputs[0].AmountSats + sourceInputs[1].AmountSats + sourceInputs[2].AmountSats - 20_000)
	item := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      rootAddr,
		VaultPubKey:    vaultPubKey,
		Coin:           common.NewCoin(common.BTCAsset, staleCoin),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000))},
		InHash:         inHash,
		VaultPathIndex: common.MainVaultPathIndex,
		TxType:         types.TxOutTypeConsolidate,
		SourceInputs:   sourceInputs,
	}
	if err := k.SetTxOut(ctx, &TxOut{Height: ctx.BlockHeight(), Status: TxOutStatusPendingSign, TxArray: []TxOutItem{item}}); err != nil {
		t.Fatal(err)
	}

	observedGas := uint64(10_019)
	observedAmount := cosmos.NewUint(sourceInputs[0].AmountSats + sourceInputs[1].AmountSats - observedGas)
	observedTx := common.NewTx(
		outHash,
		rootAddr,
		rootAddr,
		common.NewCoins(common.NewCoin(common.BTCAsset, observedAmount)),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(observedGas))},
	)
	observedTx.SourceInputs = []common.TxInput{
		{TxID: sourceInputs[0].TxId, Vout: sourceInputs[0].Vout, AmountSats: sourceInputs[0].AmountSats},
		{TxID: sourceInputs[1].TxId, Vout: sourceInputs[1].Vout, AmountSats: sourceInputs[1].AmountSats},
	}
	observed := common.NewObservedTx(observedTx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight())

	if !markObservedOutboundTxOut(ctx, mgr, observed) {
		t.Fatal("expected final consolidate observation with valid source-input subset to settle stale txout")
	}
	stored := k.txOutByHeight[ctx.BlockHeight()].TxArray[0]
	if !stored.OutHash.Equals(outHash) {
		t.Fatalf("expected out hash %s, got %s", outHash, stored.OutHash)
	}
	if got := len(stored.SourceInputs); got != 2 {
		t.Fatalf("expected stored source inputs to be corrected to observed inputs, got %d", got)
	}
	if !stored.Coin.Amount.Equal(observedAmount) {
		t.Fatalf("expected stored coin %s, got %s", observedAmount, stored.Coin.Amount)
	}
	if got := stored.MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount.Uint64(); got != observedGas {
		t.Fatalf("expected stored max gas %d, got %d", observedGas, got)
	}
}

func TestBTCInternalHistoricalOpenTxOutTakesPrecedenceOverCompletedDuplicate(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(1_000)
	k := newShielderFlowTestKeeper()
	k.configs[constants.Chain_BlockTimeSeconds] = 6
	k.configs[constants.Keysign_PeriodMinutes] = 5
	mgr := newShielderFlowTestManager(k)

	vaultPubKey := GetRandomPubKey()
	rootAddr, err := common.DeriveBTCTaprootAddress(vaultPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash := GetRandomTxHash()
	sourceInputs := []types.TxOutInput{
		{TxId: GetRandomTxHash(), Vout: 0, AmountSats: 19_998_350},
		{TxId: GetRandomTxHash(), Vout: 1, AmountSats: 19_998_350},
		{TxId: GetRandomTxHash(), Vout: 2, AmountSats: 19_997_690},
	}
	observedGas := uint64(10_019)
	observedAmount := cosmos.NewUint(sourceInputs[0].AmountSats + sourceInputs[1].AmountSats - observedGas)
	openItem := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      rootAddr,
		VaultPubKey:    vaultPubKey,
		Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(sourceInputs[0].AmountSats+sourceInputs[1].AmountSats+sourceInputs[2].AmountSats-20_000)),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000))},
		InHash:         GetRandomTxHash(),
		VaultPathIndex: common.MainVaultPathIndex,
		TxType:         types.TxOutTypeConsolidate,
		SourceInputs:   sourceInputs,
	}
	completedItem := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      rootAddr,
		VaultPubKey:    vaultPubKey,
		Coin:           common.NewCoin(common.BTCAsset, observedAmount),
		MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(observedGas))},
		OutHash:        outHash,
		VaultPathIndex: common.MainVaultPathIndex,
		TxType:         types.TxOutTypeConsolidate,
		SourceInputs:   sourceInputs[:2],
	}
	k.txOutByHeight[90] = TxOut{Height: 90, Status: TxOutStatusPendingSign, TxArray: []TxOutItem{openItem}}
	k.txOutByHeight[80] = TxOut{Height: 80, Status: TxOutStatusComplete, TxArray: []TxOutItem{completedItem}}

	observedTx := common.NewTx(
		outHash,
		rootAddr,
		rootAddr,
		common.NewCoins(common.NewCoin(common.BTCAsset, observedAmount)),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(observedGas))},
	)
	observedTx.SourceInputs = []common.TxInput{
		{TxID: sourceInputs[0].TxId, Vout: sourceInputs[0].Vout, AmountSats: sourceInputs[0].AmountSats},
		{TxID: sourceInputs[1].TxId, Vout: sourceInputs[1].Vout, AmountSats: sourceInputs[1].AmountSats},
	}
	observed := common.NewObservedTx(observedTx, 999, vaultPubKey, 999)

	matched, newlyMatched := markObservedOutboundTxOutStatus(ctx, mgr, observed)
	if !matched || !newlyMatched {
		t.Fatalf("expected open historical txout to be matched before completed duplicate, got matched=%v newly=%v", matched, newlyMatched)
	}
	if !k.txOutByHeight[90].TxArray[0].OutHash.Equals(outHash) {
		t.Fatalf("expected open txout to be settled, got %#v", k.txOutByHeight[90].TxArray[0])
	}
}

func testQueuedBTCBatch(t *testing.T, ctx cosmos.Context, k *shielderFlowTestKeeper, vaultPubKey common.PubKey) (*TxOut, types.TxOutInput, common.Address) {
	t.Helper()
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, vaultPubKey, 2_000_000)
	vault, err := k.GetVault(ctx, vaultPubKey)
	if err != nil {
		t.Fatal(err)
	}
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txOut := NewTxOut(ctx.BlockHeight())
	txOut.TxArray = []TxOutItem{
		{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vaultPubKey,
			VaultPathIndex: common.MainVaultPathIndex,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(400_000)),
			InHash:         GetRandomTxHash(),
			ModuleName:     BaseName,
			TxType:         types.TxOutTypeOut,
		},
		{
			Chain:          common.BTCChain,
			ToAddress:      GetRandomBTCAddress(),
			VaultPubKey:    vaultPubKey,
			VaultPathIndex: common.MainVaultPathIndex,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(300_000)),
			InHash:         GetRandomTxHash(),
			ModuleName:     BaseName,
			TxType:         types.TxOutTypeRefund,
		},
	}
	if err := refreshBTCExactTxOutBlock(ctx, k, txOut); err != nil {
		t.Fatal(err)
	}
	for _, item := range txOut.TxArray {
		if len(item.SourceInputs) != 1 || !item.SourceInputs[0].TxId.Equals(sourceInput.TxId) {
			t.Fatalf("unexpected batch source inputs: %#v", item.SourceInputs)
		}
	}
	if testTxOutTotalBTCMaxGas(txOut).IsZero() {
		t.Fatal("expected batch max gas")
	}
	if err := k.SetTxOut(ctx, txOut); err != nil {
		t.Fatal(err)
	}
	return txOut, sourceInput, sourceAddr
}

func testTxOutTotalBTCMaxGas(txOut *TxOut) cosmos.Uint {
	total := cosmos.ZeroUint()
	for _, item := range txOut.TxArray {
		total = total.Add(item.MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount)
	}
	return total
}

func TestBTCMigrationSourceInputsSelectWholeUTXOAtOrAboveTarget(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	networkMgr := &NetworkMgr{k: k}

	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, RetiringVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	hugeTx, err := common.NewTxID("9111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	tinyTx, err := common.NewTxID("9222222222222222222222222222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[10] = TxOut{
		Height: 10,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(90_000_000)),
				OutHash:     hugeTx,
			},
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000_000)),
				OutHash:     tinyTx,
			},
		},
	}

	inputs := networkMgr.btcMigrationSourceInputs(ctx, vault, sourceAddr, cosmos.NewUint(50_000_000))
	if len(inputs) != 1 {
		t.Fatalf("expected one selected UTXO, got %d", len(inputs))
	}
	if !inputs[0].TxId.Equals(hugeTx) || inputs[0].AmountSats != 90_000_000 {
		t.Fatalf("expected huge UTXO to be selected, got %#v", inputs[0])
	}
}

func TestBTCMigrationSourceInputsSelectLargestUTXORegardlessOfDiscoveryOrder(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	networkMgr := &NetworkMgr{k: k}

	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, RetiringVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	tinyTx, err := common.NewTxID("9322222222222222222222222222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	hugeTx, err := common.NewTxID("9311111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[10] = TxOut{
		Height: 10,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000_000)),
				OutHash:     tinyTx,
			},
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(90_000_000)),
				OutHash:     hugeTx,
			},
		},
	}

	inputs := networkMgr.btcMigrationSourceInputs(ctx, vault, sourceAddr, cosmos.NewUint(50_000_000))
	if len(inputs) != 1 {
		t.Fatalf("expected one selected UTXO, got %d", len(inputs))
	}
	if !inputs[0].TxId.Equals(hugeTx) || inputs[0].AmountSats != 90_000_000 {
		t.Fatalf("expected huge UTXO to be selected first, got %#v", inputs[0])
	}
}

func TestBTCMigrationSourceInputsSelectPriorMigrationDestinationUTXO(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	networkMgr := &NetworkMgr{k: k}

	oldPubKey := GetRandomPubKey()
	retiringPubKey := GetRandomPubKey()
	vault := NewVaultV2(20, RetiringVault, BaseVault, retiringPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	migrationTx, err := common.NewTxID("9411111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[10] = TxOut{
		Height: 10,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: oldPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(80_000_000)),
				OutHash:     migrationTx,
				OutVout:     0,
				TxType:      types.TxOutTypeMigrate,
			},
		},
	}

	inputs := networkMgr.btcMigrationSourceInputs(ctx, vault, sourceAddr, cosmos.NewUint(50_000_000))
	if len(inputs) != 1 {
		t.Fatalf("expected one selected migrated UTXO, got %d", len(inputs))
	}
	if !inputs[0].TxId.Equals(migrationTx) || inputs[0].Vout != 0 || inputs[0].AmountSats != 80_000_000 {
		t.Fatalf("expected prior migration destination UTXO to be selected, got %#v", inputs[0])
	}
}

func TestBTCVaultSourceInputsSelectPriorMigrationDestinationUTXO(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()

	oldPubKey := GetRandomPubKey()
	activePubKey := GetRandomPubKey()
	vault := NewVaultV2(20, ActiveVault, BaseVault, activePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	migrationTx, err := common.NewTxID("9511111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[10] = TxOut{
		Height: 10,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: oldPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(80_000_000)),
				OutHash:     migrationTx,
				OutVout:     0,
				TxType:      types.TxOutTypeMigrate,
			},
		},
	}

	inputs := btcSelectVaultSourceInputs(ctx, k, vault, sourceAddr, cosmos.NewUint(50_000_000), 0)
	if len(inputs) != 1 {
		t.Fatalf("expected one selected migrated UTXO, got %d", len(inputs))
	}
	if !inputs[0].TxId.Equals(migrationTx) || inputs[0].Vout != 0 || inputs[0].AmountSats != 80_000_000 {
		t.Fatalf("expected prior migration destination UTXO to be selected, got %#v", inputs[0])
	}
}

func TestBTCMigrationSourceInputsCanDrainTinyRemainderAfterHugeReserved(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	networkMgr := &NetworkMgr{k: k}

	vaultPubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, RetiringVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	destAddr, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	hugeTx, err := common.NewTxID("A911111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	tinyTx, err := common.NewTxID("A922222222222222222222222222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[10] = TxOut{
		Height: 10,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(90_000_000)),
				OutHash:     hugeTx,
			},
			{
				Chain:       common.BTCChain,
				ToAddress:   sourceAddr,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000_000)),
				OutHash:     tinyTx,
			},
		},
	}
	k.txOutByHeight[20] = TxOut{
		Height: 20,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      destAddr,
				VaultPubKey:    vaultPubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(89_990_000)),
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeMigrate,
				SourceInputs:   []types.TxOutInput{{TxId: hugeTx, Vout: 0, AmountSats: 90_000_000}},
			},
		},
	}

	inputs := networkMgr.btcMigrationSourceInputs(ctx, vault, sourceAddr, cosmos.NewUint(5_000_000))
	if len(inputs) != 1 {
		t.Fatalf("expected one selected UTXO, got %d", len(inputs))
	}
	if !inputs[0].TxId.Equals(tinyTx) || inputs[0].AmountSats != 10_000_000 {
		t.Fatalf("expected tiny remainder UTXO to be selected, got %#v", inputs[0])
	}
}

func TestObservedOutboundRequiresTxOutMatchForBTCVaultSpend(t *testing.T) {
	pubKey := GetRandomPubKey()
	from, err := common.DeriveBTCTaprootAddress(pubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pzw3dft08ts0r00y7lhpx50w7wfvqvhxal5pssdl9pkmv8mm5fjpsn4735s")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("DF25BFC8FFED823FBD369F1FED64128DFB5E94E9AA87723D9E688C00087A3DE1")
	if err != nil {
		t.Fatal(err)
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_145))},
		),
		157,
		pubKey,
		157,
	)
	if !observedOutboundRequiresTxOutMatch(tx) {
		t.Fatal("expected BTC vault outbound to require an open txout match")
	}
}

func TestObservedOutboundRejectsAlreadyCompletedBTCBatch(t *testing.T) {
	pubKey := GetRandomPubKey()
	from, err := common.DeriveBTCTaprootAddress(pubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to1, err := common.NewAddress("bcrt1pzw3dft08ts0r00y7lhpx50w7wfvqvhxal5pssdl9pkmv8mm5fjpsn4735s")
	if err != nil {
		t.Fatal(err)
	}
	to2, err := common.NewAddress("bcrt1pv3te6d3yfsdq3yqrh6lh8a9uln3ydjppm5jysjd8ypq7x4mm33vskaecp3")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("EF25BFC8FFED823FBD369F1FED64128DFB5E94E9AA87723D9E688C00087A3DE1")
	if err != nil {
		t.Fatal(err)
	}
	inHash1, err := common.NewTxID("0000000000000000000000000000000000000000000000000000000000000101")
	if err != nil {
		t.Fatal(err)
	}
	inHash2, err := common.NewTxID("0000000000000000000000000000000000000000000000000000000000000102")
	if err != nil {
		t.Fatal(err)
	}
	txOut := &TxOut{
		Height: 157,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to1,
				VaultPubKey:    pubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000_000)),
				InHash:         inHash1,
				OutHash:        outHash,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeOut,
			},
			{
				Chain:          common.BTCChain,
				ToAddress:      to2,
				VaultPubKey:    pubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000)),
				InHash:         inHash2,
				OutHash:        outHash,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeOut,
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to1,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(108_900_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_145))},
		),
		157,
		pubKey,
		157,
	)
	if markObservedOutboundTxOutBatch(cosmos.Context{}, nil, txOut, tx) {
		t.Fatal("expected already completed BTC batch txout to be rejected")
	}
}

func TestObservedOutboundAlreadyMatchedSingleIsIdempotent(t *testing.T) {
	pubKey := GetRandomPubKey()
	from, err := common.DeriveBTCTaprootAddress(pubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pfrs56cns3k4nvt7wkym80kddmctklp2ajcce8vept6wyqt8p4n9syx5h94")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("7890D11818077C7A7C2556B7CDF37DB801DE67078F85672589B41FF93EBB6A18")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("9FEF0CDB5AF0F2B7AE16A6F6DEA7B4ADB8649D96864E73BDA8192FC379AA7776")
	if err != nil {
		t.Fatal(err)
	}
	txOut := &TxOut{
		Height: 337,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    pubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(990_000)),
				InHash:         inHash,
				OutHash:        outHash,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeOut,
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(990_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_310))},
		),
		646,
		pubKey,
		646,
	)

	if !observedOutboundAlreadyMatchedTxOut(txOut, tx) {
		t.Fatal("expected duplicate observed single outbound to be treated as matched")
	}
}

func TestAlreadyMatchedObservedOutboundSkipsOnlyAfterVoterDone(t *testing.T) {
	pubKey := GetRandomPubKey()
	from, err := common.DeriveBTCTaprootAddress(pubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pfrs56cns3k4nvt7wkym80kddmctklp2ajcce8vept6wyqt8p4n9syx5h94")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("9BF93BE40799B705E16E52174A091ECE00422B2B8E62BECD5A85B0EF3EB6704E")
	if err != nil {
		t.Fatal(err)
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(990_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_142))},
		),
		292,
		pubKey,
		292,
	)
	voter := ObservedTxVoter{
		TxID: outHash,
		Tx:   tx,
		Txs:  []common.ObservedTx{tx},
	}

	if shouldSkipAlreadyProcessedObservedOutbound(voter, true, tx) {
		t.Fatal("already matched txout with incomplete voter must still run accounting")
	}

	voter.SetDone()
	if !shouldSkipAlreadyProcessedObservedOutbound(voter, true, tx) {
		t.Fatal("done voter should skip duplicate accounting")
	}
}

func TestRepairRevertedObservedOutboundReversesFalseErrata(t *testing.T) {
	ctx := testContext(500)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	pubKey := GetRandomPubKey()
	vault := NewVaultV2(100, ActiveVault, BaseVault, pubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	k.baseVaults = Vaults{vault}

	from, err := common.DeriveBTCTaprootAddress(pubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1q7jxc5gseayjdeysdvtd3xvj2wkzmza7rystenz")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("A9EB813E8C21F60CA0399018B004FC66E8C2E34FC4ECB4652329E178EC1C4E17")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("DCE9F4D87D2200220647AAD24BA476E39527064AB8049E7FCE0624E5BED0A615")
	if err != nil {
		t.Fatal(err)
	}
	coin := common.NewCoin(common.BTCAsset, cosmos.NewUint(29_700_000))
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(coin),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_094))},
		),
		4520,
		pubKey,
		4520,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}
	k.txOutByHeight[100] = TxOut{
		Height: 100,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    pubKey,
			Coin:           coin,
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(5_000))},
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeRefund,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	voter := ObservedTxVoter{TxID: outHash, Tx: tx, Txs: []common.ObservedTx{tx}, FinalisedHeight: 499}
	voter.SetReverted()
	k.SetObservedTxOutVoter(ctx, voter)

	if !repairRevertedObservedOutbound(ctx, mgr, &voter, tx) {
		t.Fatal("expected reverted outbound to be repaired")
	}

	storedVault, err := k.GetVault(ctx, pubKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedVault.GetCoin(common.BTCAsset).Amount.Uint64(); got != 70_300_000 {
		t.Fatalf("wrong repaired vault balance: %d", got)
	}
	storedVoter, err := k.GetObservedTxOutVoter(ctx, outHash)
	if err != nil {
		t.Fatal(err)
	}
	if storedVoter.Reverted {
		t.Fatal("expected reverted flag to be cleared")
	}
	if storedVoter.Tx.Status != common.Status_done {
		t.Fatalf("expected done status, got %s", storedVoter.Tx.Status)
	}
}

func TestObservedOutboundAlreadyMatchedBatchIsIdempotent(t *testing.T) {
	pubKey := GetRandomPubKey()
	from, err := common.DeriveBTCTaprootAddress(pubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pfrs56cns3k4nvt7wkym80kddmctklp2ajcce8vept6wyqt8p4n9syx5h94")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("79CAB02CDFF319D8FA0E5A466405DDFDDA9BA04C86F10576DB759983750EFA98")
	if err != nil {
		t.Fatal(err)
	}
	inHash1, err := common.NewTxID("A300BAA19B55FFFE53F1CEBD6AF4DF28D9A8D5FF26AA05C623C753F4E0B2EEB1")
	if err != nil {
		t.Fatal(err)
	}
	inHash2, err := common.NewTxID("6B159AE062CE13A92F1453C8BCB692FCA160E0D6048C847E9FCE33C8F02CCD0C")
	if err != nil {
		t.Fatal(err)
	}
	sourceTx, err := common.NewTxID("1B159AE062CE13A92F1453C8BCB692FCA160E0D6048C847E9FCE33C8F02CCD0C")
	if err != nil {
		t.Fatal(err)
	}
	sourceInputs := []types.TxOutInput{{TxId: sourceTx, Vout: 0, AmountSats: 11_000_000}}
	txOut := &TxOut{
		Height: 397,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    pubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(990_000)),
				InHash:         inHash1,
				OutHash:        outHash,
				OutVout:        0,
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_787))},
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeOut,
				SourceInputs:   sourceInputs,
			},
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    pubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000)),
				InHash:         inHash2,
				OutHash:        outHash,
				OutVout:        1,
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_787))},
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeOut,
				SourceInputs:   sourceInputs,
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(10_890_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_574))},
		),
		636,
		pubKey,
		636,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: sourceTx, Vout: 0, AmountSats: 11_000_000}}

	if !observedOutboundAlreadyMatchedTxOut(txOut, tx) {
		t.Fatal("expected duplicate observed batch outbound to be treated as matched")
	}
}

func TestBTCMigrationOutboundCreditsDerivedDestinationVault(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("A111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("B111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    sourcePubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
				InHash:         inHash,
				OutHash:        outHash,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeMigrate,
				SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{TxID: outHash}
	if !creditBTCMigrationDestination(ctx, mgr, &voter, tx) {
		t.Fatal("expected outbound migration to credit destination vault")
	}

	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	got := storedDest.GetCoin(common.BTCAsset).Amount.Uint64()
	if got != 99_992_500 {
		t.Fatalf("outbound migration credited destination vault %d", got)
	}
	if storedDest.InboundTxCount != 1 {
		t.Fatalf("destination inbound count = %d", storedDest.InboundTxCount)
	}
	if !voter.UpdatedVault {
		t.Fatal("outbound voter was not marked updated")
	}
}

func TestBTCMigrationInboundCreditsDestinationOnce(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)
	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	voter := ObservedTxVoter{}
	from, err := common.NewAddress("bcrt1p2szxj3dd5yvsx9sf7fy5dth67h4x30puf23hp08xllnl4kd7xwqq3qknse")
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.DeriveBTCTaprootAddress(destPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txID, err := common.NewTxID("A311111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   to,
				VaultPubKey: sourcePubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(99_994_720)),
				MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_135))},
				InHash:      common.BlankTxID,
				OutHash:     txID,
				TxType:      types.TxOutTypeMigrate,
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			txID,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_994_720))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_135))},
		),
		99,
		destPubKey,
		99,
	)

	if !creditBTCMigrationInboundDestination(ctx, mgr, &destVault, &voter, tx) {
		t.Fatal("expected final inbound migration to credit destination")
	}
	if got := destVault.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_994_720 {
		t.Fatalf("destination credited %d", got)
	}
	if destVault.InboundTxCount != 1 {
		t.Fatalf("inbound count = %d", destVault.InboundTxCount)
	}
	if !voter.UpdatedVault {
		t.Fatal("voter was not marked updated")
	}
	if creditBTCMigrationInboundDestination(ctx, mgr, &destVault, &voter, tx) {
		t.Fatal("duplicate inbound migration credited again")
	}
	if got := destVault.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_994_720 {
		t.Fatalf("duplicate destination credit changed amount to %d", got)
	}

	pendingVoter := ObservedTxVoter{}
	pendingVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	tx.FinaliseHeight = 128
	if !creditBTCMigrationInboundDestination(ctx, mgr, &pendingVault, &pendingVoter, tx) {
		t.Fatal("expected pending inbound migration to credit destination")
	}
	if got := pendingVault.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_994_720 {
		t.Fatalf("pending destination credited %d", got)
	}
	if pendingVault.InboundTxCount != 1 {
		t.Fatalf("pending inbound count = %d", pendingVault.InboundTxCount)
	}
	if !pendingVoter.UpdatedVault {
		t.Fatal("pending voter was not marked updated")
	}

	k.SetObservedTxOutVoter(ctx, ObservedTxVoter{TxID: txID, UpdatedVault: true})
	laterInboundVoter := ObservedTxVoter{}
	tx.FinaliseHeight = 99
	if creditBTCMigrationInboundDestination(ctx, mgr, &destVault, &laterInboundVoter, tx) {
		t.Fatal("inbound migration credited again after outbound destination credit")
	}
	if got := destVault.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_994_720 {
		t.Fatalf("outbound-credited inbound changed amount to %d", got)
	}
	if !laterInboundVoter.UpdatedVault {
		t.Fatal("later inbound voter was not marked updated")
	}
}

func TestBTCMigrationInboundQuorumSettlesSourceVault(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txID, err := common.NewTxID("A411111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("B411111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        txID,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			txID,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		destPubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{TxID: txID, Tx: tx}
	if err := handleObservedTxInQuorum(ctx, mgr, nil, nil, nil, tx, voter, nil, true); err != nil {
		t.Fatal(err)
	}

	storedSource, err := k.GetVault(ctx, sourcePubKey)
	if err != nil {
		t.Fatal(err)
	}
	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if !storedSource.GetCoin(common.BTCAsset).Amount.IsZero() {
		t.Fatalf("expected source vault to drain, got %s", storedSource.GetCoin(common.BTCAsset).Amount)
	}
	if storedSource.Status != InactiveVault {
		t.Fatalf("expected source vault inactive, got %s", storedSource.Status)
	}
	if len(storedSource.PendingTxBlockHeights) != 0 {
		t.Fatalf("expected source pending cleared, got %v", storedSource.PendingTxBlockHeights)
	}
	if got := storedDest.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_992_500 {
		t.Fatalf("expected destination credit, got %d", got)
	}
}

func TestBTCMigrationDuplicateInboundRepairsSourceVault(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	destVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txID, err := common.NewTxID("A511111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("B511111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        txID,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			txID,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		destPubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{TxID: txID, Tx: tx, UpdatedVault: true, FinalisedHeight: 99}
	if err := handleObservedTxInQuorum(ctx, mgr, nil, nil, nil, tx, voter, nil, false); err != nil {
		t.Fatal(err)
	}

	storedSource, err := k.GetVault(ctx, sourcePubKey)
	if err != nil {
		t.Fatal(err)
	}
	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if !storedSource.GetCoin(common.BTCAsset).Amount.IsZero() {
		t.Fatalf("expected duplicate inbound repair to drain source, got %s", storedSource.GetCoin(common.BTCAsset).Amount)
	}
	if storedSource.Status != InactiveVault {
		t.Fatalf("expected source vault inactive, got %s", storedSource.Status)
	}
	if got := storedDest.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_992_500 {
		t.Fatalf("duplicate inbound repair changed destination balance to %d", got)
	}
}

func TestBTCMigrationInboundRepairPassesDuplicateAnte(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txID, err := common.NewTxID("A611111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("B611111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        txID,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			txID,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		destPubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}
	signer := cosmos.AccAddress("node2")
	tx.Signers = []string{signer.String()}
	voter := ObservedTxVoter{TxID: txID, Tx: tx, Txs: []common.ObservedTx{tx}, UpdatedVault: true, FinalisedHeight: 99}

	if err := reserveObservedTxAttestations(ctx, k, voter, tx, []cosmos.AccAddress{signer}, true); err != nil {
		t.Fatalf("expected duplicate inbound migration repair through ante: %v", err)
	}
}

func TestBTCMigrationCreditRejectsWrongSourceInputs(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("A211111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	expectedIn, err := common.NewTxID("B211111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	actualIn, err := common.NewTxID("B311111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    sourcePubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
				InHash:         expectedIn,
				OutHash:        outHash,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeMigrate,
				SourceInputs:   []types.TxOutInput{{TxId: expectedIn, Vout: 0, AmountSats: 100_000_000}},
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: actualIn, Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{TxID: outHash}
	if creditBTCMigrationDestination(ctx, mgr, &voter, tx) {
		t.Fatal("wrong-source migration should not credit destination")
	}

	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if !storedDest.GetCoin(common.BTCAsset).Amount.IsZero() {
		t.Fatalf("wrong-source migration credited destination vault: %#v", storedDest.Coins)
	}
}

func TestBTCMigrationSourceSettlementUsesSourceInputsAndInactivatesDrainedVault(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("C111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{
			{
				Chain:          common.BTCChain,
				ToAddress:      to,
				VaultPubKey:    sourcePubKey,
				Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
				MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
				InHash:         inHash,
				OutHash:        outHash,
				VaultPathIndex: common.MainVaultPathIndex,
				TxType:         types.TxOutTypeMigrate,
				SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
			},
		},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	if !settleBTCMigrationSourceVault(ctx, mgr, tx) {
		t.Fatal("expected migration source vault to settle")
	}
	storedSource, err := k.GetVault(ctx, sourcePubKey)
	if err != nil {
		t.Fatal(err)
	}
	if !storedSource.GetCoin(common.BTCAsset).Amount.IsZero() {
		t.Fatalf("expected source vault BTC to be drained, got %s", storedSource.GetCoin(common.BTCAsset).Amount)
	}
	if storedSource.Status != InactiveVault {
		t.Fatalf("expected drained retiring vault to become inactive, got %s", storedSource.Status)
	}
	if len(storedSource.PendingTxBlockHeights) != 0 {
		t.Fatalf("expected migration pending height to be cleared, got %v", storedSource.PendingTxBlockHeights)
	}
}

func TestBTCMigrationAccountingSkipsNonFinalObservation(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("C211111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D211111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		100,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{TxID: outHash}
	if creditBTCMigrationDestination(ctx, mgr, &voter, tx) {
		t.Fatal("non-final migration should not credit destination from outbound")
	}
	if settleBTCMigrationSourceVault(ctx, mgr, tx) {
		t.Fatal("non-final migration should not settle source vault")
	}
	storedSource, err := k.GetVault(ctx, sourcePubKey)
	if err != nil {
		t.Fatal(err)
	}
	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedSource.GetCoin(common.BTCAsset).Amount.Uint64(); got != 100_000_000 {
		t.Fatalf("non-final migration changed source balance: %d", got)
	}
	if got := storedDest.GetCoin(common.BTCAsset).Amount.Uint64(); got != 0 {
		t.Fatalf("non-final migration credited destination: %d", got)
	}
}

func TestBTCMigrationAccountingIsIdempotentAfterSourceSettled(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("C311111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D311111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{TxID: outHash}
	if !creditBTCMigrationDestination(ctx, mgr, &voter, tx) {
		t.Fatal("expected final migration to credit destination")
	}
	if !settleBTCMigrationSourceVault(ctx, mgr, tx) {
		t.Fatal("expected final migration to settle once")
	}
	if creditBTCMigrationDestination(ctx, mgr, &voter, tx) {
		t.Fatal("expected duplicate final migration credit to be ignored")
	}
	if settleBTCMigrationSourceVault(ctx, mgr, tx) {
		t.Fatal("expected duplicate final migration to be ignored")
	}
	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedDest.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_992_500 {
		t.Fatalf("duplicate final migration changed destination balance: %d", got)
	}
}

func TestBTCMigrationReplayRepairDoesNotFallThroughAfterSourceSettled(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, InactiveVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("C511111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D511111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	if repairBTCInternalOutboundReplay(ctx, mgr, tx) {
		t.Fatal("settled migration replay should not fall through to generic internal outbound repair")
	}
	storedSource, err := k.GetVault(ctx, sourcePubKey)
	if err != nil {
		t.Fatal(err)
	}
	if storedSource.OutboundTxCount != 0 {
		t.Fatalf("settled migration replay changed outbound count: %d", storedSource.OutboundTxCount)
	}
	if !storedSource.GetCoin(common.BTCAsset).Amount.IsZero() {
		t.Fatalf("settled migration replay changed source balance: %s", storedSource.GetCoin(common.BTCAsset).Amount)
	}
}

func TestBTCInternalOutboundReplayRepairIsIdempotent(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	k.baseVaults = Vaults{vault}

	from, err := common.DeriveBTCTaprootAddress(vaultPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to := GetRandomBTCAddress()
	outHash, err := common.NewTxID("C611111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D611111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    vaultPubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000_000)),
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeSweep,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 10_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000_000))),
			common.Gas{},
		),
		99,
		vaultPubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 10_000_000}}

	if !repairBTCInternalOutboundReplay(ctx, mgr, tx) {
		t.Fatal("expected first replay repair to update vault accounting")
	}
	if repairBTCInternalOutboundReplay(ctx, mgr, tx) {
		t.Fatal("duplicate replay repair should be ignored")
	}
	storedVault, err := k.GetVault(ctx, vaultPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedVault.GetCoin(common.BTCAsset).Amount.Uint64(); got != 90_000_000 {
		t.Fatalf("duplicate replay repair changed vault balance: %d", got)
	}
	if storedVault.OutboundTxCount != 1 {
		t.Fatalf("duplicate replay repair changed outbound count: %d", storedVault.OutboundTxCount)
	}
	voter, err := k.GetObservedTxOutVoter(ctx, outHash)
	if err != nil {
		t.Fatal(err)
	}
	if !observedTxOutVoterDone(voter) {
		t.Fatal("repair did not mark outbound voter done")
	}
}

func TestBTCMigrationAccountingRunsBeforeDuplicateOutboundSkip(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("C411111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D411111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		99,
		sourcePubKey,
		99,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}
	voter := ObservedTxVoter{TxID: outHash, Tx: tx, Txs: []common.ObservedTx{tx}}
	voter.SetDone()

	matched, newlyMatched := markObservedOutboundTxOutStatus(ctx, mgr, tx)
	if !matched || newlyMatched {
		t.Fatalf("expected already matched txout, got matched=%v newly=%v", matched, newlyMatched)
	}
	if !creditBTCMigrationDestination(ctx, mgr, &voter, tx) {
		t.Fatal("expected already matched outbound migration to credit destination before skip")
	}
	if !shouldSkipAlreadyProcessedObservedOutbound(voter, matched, tx) {
		t.Fatal("expected duplicate outbound path to skip after migration accounting")
	}

	storedDest, err := k.GetVault(ctx, destPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedDest.GetCoin(common.BTCAsset).Amount.Uint64(); got != 99_992_500 {
		t.Fatalf("already matched outbound migration credited destination before skip: %d", got)
	}
}

func TestCompletedBTCMigrationMatchesOutsideSigningWindow(t *testing.T) {
	ctx := testContext(1_000)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("C611111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("D611111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
			InHash:         inHash,
			OutHash:        outHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: inHash, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_992_500))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7_500))},
		),
		999,
		sourcePubKey,
		999,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: inHash, Vout: 0, AmountSats: 100_000_000}}

	matched, newlyMatched := markObservedOutboundTxOutStatus(ctx, mgr, tx)
	if !matched || newlyMatched {
		t.Fatalf("expected completed historical migration to match without reopening txout, got matched=%v newly=%v", matched, newlyMatched)
	}
}

func TestOpenBTCOutboundMatchesOutsideSigningWindow(t *testing.T) {
	ctx := testContext(279)
	k := newShielderFlowTestKeeper()
	k.configs[constants.Chain_BlockTimeSeconds] = 6
	k.configs[constants.Keysign_PeriodMinutes] = 5
	mgr := newShielderFlowTestManager(k)

	vaultPubKey := GetRandomPubKey()
	from, err := common.DeriveBTCTaprootAddress(vaultPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1qw6z86akz2kj9jzxudkuyhlzuxmlv9hffzghe88")
	if err != nil {
		t.Fatal(err)
	}
	inHash, err := common.NewTxID("551E8052F468BDF756D661C13DB821872A1028E619C686761E6423328422D4AD")
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("AF40B488ABEAAF714696D1F2B5EBB03535DD7616A018405B829958062CB087EC")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[220] = TxOut{
		Height: 220,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    vaultPubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000)),
			GasRate:        14,
			InHash:         inHash,
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeOut,
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(6_902))},
		),
		360,
		vaultPubKey,
		360,
	)

	matched, newlyMatched := markObservedOutboundTxOutStatus(ctx, mgr, tx)
	if !matched || !newlyMatched {
		t.Fatalf("expected still-open txout outside signing window to match, got matched=%v newly=%v", matched, newlyMatched)
	}
	stored := k.txOutByHeight[220].TxArray[0]
	if !stored.OutHash.Equals(outHash) {
		t.Fatalf("expected txout out hash %s, got %s", outHash, stored.OutHash)
	}
	voter := ObservedTxVoter{TxID: outHash, Tx: tx, Txs: []common.ObservedTx{tx}}
	voter.SetDone()
	if !shouldSkipAlreadyProcessedObservedOutbound(voter, matched, tx) {
		t.Fatal("done voter with newly matched historical txout should skip duplicate accounting")
	}
}

func TestBTCMigrationOutboundMatchesHistoricalTxOut(t *testing.T) {
	ctx := testContext(1_000)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	sourcePubKey := GetRandomPubKey()
	destPubKey := GetRandomPubKey()
	sourceVault := NewVaultV2(10, RetiringVault, BaseVault, sourcePubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	sourceVault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	sourceVault.PendingTxBlockHeights = []int64{90}
	destVault := NewVaultV2(90, ActiveVault, BaseVault, destPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{sourceVault, destVault}

	from, err := common.DeriveBTCTaprootAddress(sourcePubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	to, err := destVault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	outHash, err := common.NewTxID("E411111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	sourceTx, err := common.NewTxID("E511111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	k.txOutByHeight[90] = TxOut{
		Height: 90,
		TxArray: []TxOutItem{{
			Chain:          common.BTCChain,
			ToAddress:      to,
			VaultPubKey:    sourcePubKey,
			Coin:           common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000_000)),
			MaxGas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000))},
			VaultPathIndex: common.MainVaultPathIndex,
			TxType:         types.TxOutTypeMigrate,
			SourceInputs:   []types.TxOutInput{{TxId: sourceTx, Vout: 0, AmountSats: 100_000_000}},
		}},
	}
	tx := common.NewObservedTx(
		common.NewTx(
			outHash,
			from,
			to,
			common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_500_000))),
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(500_000))},
		),
		500,
		sourcePubKey,
		500,
	)
	tx.Tx.SourceInputs = []common.TxInput{{TxID: sourceTx, Vout: 0, AmountSats: 100_000_000}}

	matched, newlyMatched := markObservedOutboundTxOutStatus(ctx, mgr, tx)
	if !matched || !newlyMatched {
		t.Fatalf("expected historical migration txout match, got matched=%v newly=%v", matched, newlyMatched)
	}
	stored := k.txOutByHeight[90].TxArray[0]
	if !stored.OutHash.Equals(outHash) {
		t.Fatalf("expected historical txout out hash %s, got %s", outHash, stored.OutHash)
	}
	if got := stored.Coin.Amount.Uint64(); got != 99_500_000 {
		t.Fatalf("expected observed migration amount to update txout coin, got %d", got)
	}
}
