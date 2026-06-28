package thornado

import (
	"fmt"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type btcSourceCandidate struct {
	input  types.TxOutInput
	height int64
}

const maxInt64 = int64(^uint64(0) >> 1)

func btcGasRateFromKeeper(ctx cosmos.Context, k keeper.Keeper) (int64, error) {
	networkFee, err := k.GetNetworkFee(ctx, common.BTCChain)
	if err != nil {
		return 0, fmt.Errorf("fail to get bitcoin network fee: %w", err)
	}
	if err := networkFee.Valid(); err != nil {
		return 0, fmt.Errorf("invalid bitcoin network fee: %w", err)
	}
	if networkFee.TransactionFeeRate > uint64(maxInt64) {
		return 0, fmt.Errorf("bitcoin gas rate exceeds int64: %d", networkFee.TransactionFeeRate)
	}
	return int64(networkFee.TransactionFeeRate), nil
}

func btcGasCoinFromNativeSats(sats uint64) common.Coin {
	coin := common.NewCoin(common.BTCAsset, common.BTCChain.NativeGasToThornado(cosmos.NewUint(sats)))
	coin.Decimals = common.BTCChain.GetGasAssetDecimal()
	return coin
}

func btcTxOutBatchHeight(ctx cosmos.Context, k keeper.Keeper, height int64) (int64, uint64) {
	windowBlocks := constants.MinutesToBlocks(
		k.GetConfigInt64(ctx, constants.Withdrawal_BatchWindowMinutes),
		k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds),
	)
	if windowBlocks <= 0 {
		return height, 0
	}

	origin := maxInt64
	iterator := k.GetTxOutIterator(ctx)
	if iterator == nil {
		return height, 0
	}
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iterator.Value(), &txOut); err != nil {
			continue
		}
		if txOut.Status == "" {
			continue
		}
		candidate := txOut.Height - (int64(txOut.Epoch)+1)*windowBlocks
		if candidate < origin {
			origin = candidate
		}
	}
	if origin == maxInt64 {
		origin = ctx.BlockHeight()
	}
	epoch := uint64((height - origin) / windowBlocks)
	closeHeight := origin + (int64(epoch)+1)*windowBlocks
	return closeHeight, epoch
}

func appendBTCExactTxOut(ctx cosmos.Context, k keeper.Keeper, height int64, item TxOutItem) error {
	item.TxType = item.GetTxType()
	if types.IsBatchableTxOutType(item.TxType) {
		batchHeight, epoch := btcTxOutBatchHeight(ctx, k, height)
		block, err := k.GetTxOut(ctx, batchHeight)
		if err != nil {
			return err
		}
		block.Epoch = epoch
		if block.Status == "" {
			block.Status = TxOutStatusPendingBatch
		}
		block.TxArray = append(block.TxArray, item)
		if err := refreshBTCExactTxOutBlock(ctx, k, block); err != nil {
			return err
		}
		return k.SetTxOut(ctx, block)
	}

	block, err := k.GetTxOut(ctx, height)
	if err != nil {
		return err
	}
	if block.Status == "" {
		block.Status = TxOutStatusPendingSign
	}
	block.TxArray = append(block.TxArray, item)
	if err := refreshBTCExactTxOutBlock(ctx, k, block); err != nil {
		return err
	}
	return k.SetTxOut(ctx, block)
}

func refreshBTCExactTxOut(ctx cosmos.Context, k keeper.Keeper, height int64) error {
	txOut, err := k.GetTxOut(ctx, height)
	if err != nil {
		return err
	}
	if txOut == nil || txOut.IsEmpty() {
		return nil
	}
	if err := refreshBTCExactTxOutBlock(ctx, k, txOut); err != nil {
		return err
	}
	return k.SetTxOut(ctx, txOut)
}

func refreshBTCExactTxOutBlock(ctx cosmos.Context, k keeper.Keeper, txOut *TxOut) error {
	handled := make([]bool, len(txOut.TxArray))
	for i := range txOut.TxArray {
		if handled[i] {
			continue
		}
		item := txOut.TxArray[i]
		if !btcTxOutItemNeedsExactRefresh(*txOut, item) {
			continue
		}

		group := []int{i}
		if btcBatchableTxOut(item) {
			for j := i + 1; j < len(txOut.TxArray); j++ {
				if !handled[j] && btcSameBatchSource(item, txOut.TxArray[j]) && txOut.TxArray[j].OutHash.IsEmpty() {
					group = append(group, j)
				}
			}
		}

		if err := refreshBTCExactTxOutGroup(ctx, k, txOut, group); err != nil {
			return err
		}
		for _, idx := range group {
			handled[idx] = true
		}
	}
	return nil
}

func btcTxOutItemNeedsExactRefresh(txOut TxOut, item TxOutItem) bool {
	if !item.OutHash.IsEmpty() || !item.Chain.Equals(common.BTCChain) {
		return false
	}
	return txOut.Status == TxOutStatusPendingBatch ||
		len(item.SourceInputs) == 0 ||
		item.MaxGas.IsEmpty() ||
		item.GasRate == 0
}

func refreshBTCExactTxOutGroup(ctx cosmos.Context, k keeper.Keeper, txOut *TxOut, group []int) error {
	if len(group) == 0 {
		return nil
	}
	first := txOut.TxArray[group[0]]
	vault, err := k.GetVault(ctx, first.VaultPubKey)
	if err != nil {
		return fmt.Errorf("fail to get bitcoin txout vault: %w", err)
	}
	if vault.PubKey.IsEmpty() {
		return fmt.Errorf("missing bitcoin txout vault: %s", first.VaultPubKey)
	}
	sourceAddr, err := common.DeriveBTCTaprootAddress(first.VaultPubKey, first.VaultPathIndex)
	if err != nil {
		return err
	}
	gasRate, err := btcGasRateFromKeeper(ctx, k)
	if err != nil {
		return err
	}

	outputAddrs := make([]common.Address, 0, len(group))
	totalOutput := cosmos.ZeroUint()
	for _, idx := range group {
		item := txOut.TxArray[idx]
		if !item.Coin.Asset.Equals(common.BTCAsset) {
			return fmt.Errorf("bitcoin txout item is not BTC: %s", item.Coin.Asset)
		}
		outputAddrs = append(outputAddrs, item.ToAddress)
		totalOutput = totalOutput.Add(item.Coin.Amount)
	}

	inputs := first.SourceInputs
	if len(inputs) == 0 || (btcBatchableTxOut(first) && txOut.Status == TxOutStatusPendingBatch) {
		inputs, err = selectBTCVaultSourceInputsForOutputs(ctx, k, vault, first.VaultPathIndex, sourceAddr, outputAddrs, totalOutput, gasRate, txOut.Height)
		if err != nil {
			return err
		}
	}
	if len(inputs) == 0 {
		return fmt.Errorf("missing bitcoin source inputs for txout height %d", txOut.Height)
	}

	maxGasCoin, err := btcExactGasCoin(first.VaultPubKey, first.VaultPathIndex, outputAddrs, inputs, gasRate)
	if err != nil {
		return err
	}
	if types.IsInternalTxOutType(first.TxType) {
		sourceAmount := btcSourceInputsAmount(inputs)
		if sourceAmount.LTE(maxGasCoin.Amount) {
			return fmt.Errorf("bitcoin internal txout source inputs cannot cover gas: source=%s max_gas=%s", sourceAmount, maxGasCoin.Amount)
		}
		txOut.TxArray[group[0]].Coin = common.NewCoin(common.BTCAsset, sourceAmount.Sub(maxGasCoin.Amount))
	}
	gasShares := btcSplitGasCoin(maxGasCoin, len(group))
	for i, idx := range group {
		txOut.TxArray[idx].SourceInputs = append([]types.TxOutInput(nil), inputs...)
		txOut.TxArray[idx].MaxGas = common.Gas{gasShares[i]}
		txOut.TxArray[idx].GasRate = gasRate
	}
	return nil
}

func btcSplitGasCoin(total common.Coin, parts int) []common.Coin {
	if parts <= 1 {
		return []common.Coin{total}
	}
	totalAmount := total.Amount.Uint64()
	base := totalAmount / uint64(parts)
	remainder := totalAmount % uint64(parts)
	shares := make([]common.Coin, parts)
	for i := 0; i < parts; i++ {
		amount := base
		if uint64(i) < remainder {
			amount++
		}
		coin := common.NewCoin(total.Asset, cosmos.NewUint(amount))
		coin.Decimals = total.Decimals
		shares[i] = coin
	}
	return shares
}

func selectBTCVaultSourceInputsForOutputs(
	ctx cosmos.Context,
	k keeper.Keeper,
	vault Vault,
	vaultPathIndex uint64,
	sourceAddr common.Address,
	outputAddrs []common.Address,
	outputAmount cosmos.Uint,
	gasRate int64,
	ignoreTxOutHeight int64,
) ([]types.TxOutInput, error) {
	required := outputAmount
	var selected []types.TxOutInput
	for attempt := 0; attempt < 3; attempt++ {
		selected = btcSelectVaultSourceInputs(ctx, k, vault, sourceAddr, required, ignoreTxOutHeight)
		if len(selected) == 0 {
			return nil, fmt.Errorf("no bitcoin source inputs available for vault %s", vault.PubKey)
		}
		maxGasCoin, err := btcExactGasCoin(vault.PubKey, vaultPathIndex, outputAddrs, selected, gasRate)
		if err != nil {
			return nil, err
		}
		nextRequired := outputAmount.Add(maxGasCoin.Amount)
		if nextRequired.Equal(required) {
			return selected, nil
		}
		sourceTotal := btcSourceInputsAmount(selected)
		if sourceTotal.GTE(nextRequired) {
			return selected, nil
		}
		required = nextRequired
	}
	if btcSourceInputsAmount(selected).LT(required) {
		return nil, fmt.Errorf("insufficient bitcoin source inputs for vault %s: need %s, have %s", vault.PubKey, required, btcSourceInputsAmount(selected))
	}
	return selected, nil
}

func btcSelectVaultSourceInputs(ctx cosmos.Context, k keeper.Keeper, vault Vault, sourceAddr common.Address, required cosmos.Uint, ignoreTxOutHeight int64) []types.TxOutInput {
	candidates := btcVaultSourceCandidates(ctx, k, vault, sourceAddr, ignoreTxOutHeight)
	maxInputs := int(k.GetConfigInt64(ctx, constants.UTXO_MaxSpendCount))
	if maxInputs < 1 {
		maxInputs = 1
	}

	var total uint64
	inputs := make([]types.TxOutInput, 0, len(candidates))
	for _, candidate := range candidates {
		if len(inputs) >= maxInputs {
			break
		}
		inputs = append(inputs, candidate.input)
		total += candidate.input.AmountSats
		if cosmos.NewUint(total).GTE(required) {
			break
		}
	}
	if cosmos.NewUint(total).LT(required) {
		return nil
	}
	return inputs
}

func btcVaultSourceCandidates(ctx cosmos.Context, k keeper.Keeper, vault Vault, sourceAddr common.Address, ignoreTxOutHeight int64) []btcSourceCandidate {
	candidates := make(map[string]btcSourceCandidate)
	spent := make(map[string]struct{})
	usedOutVouts := make(map[string]map[uint32]struct{})

	for height := int64(1); height <= ctx.BlockHeight(); height++ {
		txOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("fail to get txout while collecting bitcoin source inputs", "height", height, "error", err)
			continue
		}
		for _, item := range txOut.TxArray {
			samePendingBatch := height == ignoreTxOutHeight &&
				item.OutHash.IsEmpty() &&
				item.Chain.Equals(common.BTCChain) &&
				item.VaultPubKey.Equals(vault.PubKey) &&
				item.VaultPathIndex == common.MainVaultPathIndex &&
				types.IsBatchableTxOutType(item.TxType)
			if item.Chain.Equals(common.BTCChain) && len(item.SourceInputs) > 0 && !samePendingBatch {
				for _, input := range item.SourceInputs {
					spent[btcSourceInputKey(input.TxId, input.Vout)] = struct{}{}
				}
			}
			if !item.OutHash.IsEmpty() {
				key := item.OutHash.String()
				if usedOutVouts[key] == nil {
					usedOutVouts[key] = make(map[uint32]struct{})
				}
				usedOutVouts[key][item.OutVout] = struct{}{}
				voter, err := k.GetObservedTxOutVoter(ctx, item.OutHash)
				if err == nil {
					markSpentBTCSourceInputs(spent, voter.Tx.Tx.SourceInputs)
					for _, observed := range voter.Txs {
						markSpentBTCSourceInputs(spent, observed.Tx.SourceInputs)
					}
				}
			}
			if item.OutHash.IsEmpty() ||
				!item.Chain.Equals(common.BTCChain) ||
				!item.VaultPubKey.Equals(vault.PubKey) ||
				!item.Coin.Asset.Equals(common.BTCAsset) ||
				!item.ToAddress.Equals(sourceAddr) ||
				item.Coin.Amount.IsZero() {
				continue
			}
			key := btcSourceInputKey(item.OutHash, item.OutVout)
			if _, ok := candidates[key]; ok {
				continue
			}
			candidates[key] = btcSourceCandidate{
				input: types.TxOutInput{
					TxId:       item.OutHash,
					Vout:       item.OutVout,
					AmountSats: item.Coin.Amount.Uint64(),
				},
				height: height,
			}
		}
	}

	outIter := k.GetObservedTxOutVoterIterator(ctx)
	if outIter != nil {
		defer outIter.Close()
		for ; outIter.Valid(); outIter.Next() {
			var voter ObservedTxVoter
			if err := k.Cdc().Unmarshal(outIter.Value(), &voter); err != nil {
				ctx.Logger().Error("fail to unmarshal observed txout while collecting bitcoin source inputs", "error", err)
				continue
			}
			for _, observed := range voter.Txs {
				tx := observed.Tx
				if !tx.Chain.Equals(common.BTCChain) ||
					!tx.FromAddress.Equals(sourceAddr) ||
					!observed.ObservedPubKey.Equals(vault.PubKey) {
					continue
				}
				markSpentBTCSourceInputs(spent, tx.SourceInputs)
				if len(tx.SourceInputs) == 0 || tx.ToAddress.Equals(sourceAddr) || tx.ID.IsEmpty() {
					continue
				}
				change := btcObservedOutboundChangeAmount(tx)
				if change == 0 {
					continue
				}
				vout := nextBTCChangeVout(usedOutVouts[tx.ID.String()])
				key := btcSourceInputKey(tx.ID, vout)
				if _, ok := candidates[key]; ok {
					continue
				}
				candidates[key] = btcSourceCandidate{
					input: types.TxOutInput{
						TxId:       tx.ID,
						Vout:       vout,
						AmountSats: change,
					},
					height: observed.BlockHeight,
				}
			}
		}
	}

	inIter := k.GetObservedTxInVoterIterator(ctx)
	if inIter != nil {
		defer inIter.Close()
		for ; inIter.Valid(); inIter.Next() {
			var voter ObservedTxVoter
			if err := k.Cdc().Unmarshal(inIter.Value(), &voter); err != nil {
				ctx.Logger().Error("fail to unmarshal observed txin while collecting bitcoin source inputs", "error", err)
				continue
			}
			tx := voter.Tx.Tx
			if !tx.Chain.Equals(common.BTCChain) ||
				!tx.ToAddress.Equals(sourceAddr) ||
				tx.ID.IsEmpty() {
				continue
			}
			coin := tx.Coins.GetCoin(common.BTCAsset)
			if coin.IsEmpty() || coin.Amount.IsZero() {
				continue
			}
			key := btcSourceInputKey(tx.ID, tx.SourceVout)
			if _, ok := candidates[key]; ok {
				continue
			}
			candidates[key] = btcSourceCandidate{
				input: types.TxOutInput{
					TxId:       tx.ID,
					Vout:       tx.SourceVout,
					AmountSats: coin.Amount.Uint64(),
				},
				height: voter.Height,
			}
		}
	}

	result := make([]btcSourceCandidate, 0, len(candidates))
	for key, candidate := range candidates {
		if _, ok := spent[key]; ok || candidate.input.AmountSats == 0 {
			continue
		}
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].height != result[j].height {
			return result[i].height < result[j].height
		}
		if result[i].input.AmountSats != result[j].input.AmountSats {
			return result[i].input.AmountSats > result[j].input.AmountSats
		}
		return btcSourceInputKey(result[i].input.TxId, result[i].input.Vout) < btcSourceInputKey(result[j].input.TxId, result[j].input.Vout)
	})
	return result
}

func btcExactGasCoin(vaultPubKey common.PubKey, vaultPathIndex uint64, outputAddrs []common.Address, inputs []types.TxOutInput, gasRate int64) (common.Coin, error) {
	if gasRate <= 0 {
		return common.NoCoin, fmt.Errorf("invalid bitcoin gas rate: %d", gasRate)
	}
	vSize, err := btcEstimatedVSize(vaultPubKey, vaultPathIndex, outputAddrs, inputs)
	if err != nil {
		return common.NoCoin, err
	}
	return btcGasCoinFromNativeSats(uint64(gasRate) * uint64(vSize)), nil
}

func btcEstimatedVSize(vaultPubKey common.PubKey, vaultPathIndex uint64, outputAddrs []common.Address, inputs []types.TxOutInput) (int64, error) {
	if len(inputs) == 0 {
		return 0, fmt.Errorf("cannot estimate bitcoin tx size without source inputs")
	}
	tx := wire.NewMsgTx(wire.TxVersion)
	for _, input := range inputs {
		hash, err := chainhash.NewHashFromStr(input.TxId.String())
		if err != nil {
			return 0, fmt.Errorf("invalid bitcoin source input txid %s: %w", input.TxId, err)
		}
		txIn := wire.NewTxIn(wire.NewOutPoint(hash, input.Vout), nil, nil)
		txIn.Witness = [][]byte{
			make([]byte, 72),
			make([]byte, 33),
		}
		tx.AddTxIn(txIn)
	}
	for _, addr := range outputAddrs {
		script, err := btcOutputScript(addr)
		if err != nil {
			return 0, err
		}
		tx.AddTxOut(wire.NewTxOut(0, script))
	}
	changeScript, err := btcVaultOutputScript(vaultPubKey, vaultPathIndex)
	if err != nil {
		return 0, err
	}
	tx.AddTxOut(wire.NewTxOut(0, changeScript))

	strippedSize := tx.SerializeSizeStripped()
	totalSize := tx.SerializeSize()
	return int64((strippedSize*3 + totalSize + 3) / 4), nil
}

func btcOutputScript(addr common.Address) ([]byte, error) {
	net, err := common.BTCChainParams()
	if err != nil {
		return nil, err
	}
	decoded, err := btcutil.DecodeAddress(addr.String(), net)
	if err != nil {
		return nil, fmt.Errorf("fail to decode bitcoin address %s: %w", addr, err)
	}
	switch a := decoded.(type) {
	case *btcutil.AddressPubKeyHash:
		return make([]byte, 25), nil
	case *btcutil.AddressScriptHash:
		return make([]byte, 23), nil
	case *btcutil.AddressWitnessPubKeyHash:
		return make([]byte, 22), nil
	case *btcutil.AddressWitnessScriptHash:
		return make([]byte, 34), nil
	case *btcutil.AddressTaproot:
		return make([]byte, 34), nil
	case *btcutil.AddressPubKey:
		return make([]byte, len(a.ScriptAddress())+2), nil
	default:
		return nil, fmt.Errorf("unsupported bitcoin address type %T", decoded)
	}
}

func btcVaultOutputScript(pubKey common.PubKey, pathIndex uint64) ([]byte, error) {
	taprootKey, err := common.DeriveBTCTaprootPubKey(pubKey, pathIndex)
	if err != nil {
		return nil, err
	}
	return append([]byte{0x51, 0x20}, taprootKey...), nil
}

func btcSourceInputsAmount(inputs []types.TxOutInput) cosmos.Uint {
	total := cosmos.ZeroUint()
	for _, input := range inputs {
		total = total.Add(cosmos.NewUint(input.AmountSats))
	}
	return total
}

func markSpentBTCSourceInputs(spent map[string]struct{}, inputs []common.TxInput) {
	for _, input := range inputs {
		spent[btcSourceInputKey(input.TxID, input.Vout)] = struct{}{}
	}
}

func btcBatchableTxOut(item TxOutItem) bool {
	return item.Chain.Equals(common.BTCChain) &&
		item.VaultPathIndex == common.MainVaultPathIndex &&
		types.IsBatchableTxOutType(item.TxType)
}

func btcSameBatchSource(a, b TxOutItem) bool {
	return btcBatchableTxOut(a) &&
		btcBatchableTxOut(b) &&
		a.Chain.Equals(b.Chain) &&
		a.VaultPubKey.Equals(b.VaultPubKey) &&
		a.VaultPathIndex == b.VaultPathIndex
}

func btcTxOutInputsEqual(a, b []types.TxOutInput) bool {
	if len(a) != len(b) {
		return false
	}
	matched := make([]bool, len(b))
	for _, left := range a {
		found := false
		for i, right := range b {
			if matched[i] {
				continue
			}
			if left.TxId.Equals(right.TxId) && left.Vout == right.Vout && left.AmountSats == right.AmountSats {
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
