package thornado

import (
	"fmt"
	"math"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcd/btcutil"

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

// btcMaxBatchRecipients caps how many VOUT-bearing items may share one batch tx.
// The BTC observers (Go ignoreTx and the Rust port) ignore any tx with more than
// --max-value-outputs (default 10) outputs, so a batch of more than 9 recipients +
// 1 vault change builds a tx that confirms on Bitcoin but can never be observed or
// matched — the funds move while the txout items stay unsigned forever. Vin-only
// items (sweeps, pinned migrates) contribute inputs, not outputs, and are capped
// separately by btcMaxBatchVins.
const btcMaxBatchRecipients = 9

// btcMaxBatchVins caps how many source inputs one batch tx may spend. FROST signs
// ~250ms per input; 64 inputs stays well inside the keysign timeout.
const btcMaxBatchVins = 64

// btcVaultBatchTxOut returns the txout block collecting the unified epoch batch for
// the given vault: sweeps and pinned migrates ride as vins, outbounds as vouts, and
// at most one unpinned internal item absorbs the remainder. An open pending_batch
// block whose close height has not yet arrived is reused while it has room for the
// incoming item; otherwise a new block is opened one batch window ahead with the
// vault's next epoch sequence number.
func btcVaultBatchTxOut(ctx cosmos.Context, k keeper.Keeper, incoming TxOutItem) (*TxOut, error) {
	vault := incoming.VaultPubKey
	iterator := k.GetTxOutIterator(ctx)
	if iterator == nil {
		return nil, fmt.Errorf("fail to create txout iterator")
	}
	defer iterator.Close()
	var open *TxOut
	nextEpoch := uint64(0)
	for ; iterator.Valid(); iterator.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iterator.Value(), &txOut); err != nil {
			continue
		}
		batchVault, ok := txOutBatchVault(txOut)
		if !ok || !batchVault.Equals(vault) {
			continue
		}
		if txOut.Epoch+1 > nextEpoch {
			nextEpoch = txOut.Epoch + 1
		}
		if txOut.Status != TxOutStatusPendingBatch || txOut.Height <= ctx.BlockHeight() {
			continue
		}
		if !btcBatchHasRoom(txOut, incoming) {
			continue
		}
		if open == nil || txOut.Height < open.Height {
			openCopy := txOut
			open = &openCopy
		}
	}
	if open != nil {
		return open, nil
	}

	windowBlocks := constants.MinutesToBlocks(
		k.GetConfigInt64(ctx, constants.Withdrawal_BatchWindowMinutes),
		k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds),
	)
	if windowBlocks < 0 {
		windowBlocks = 0
	}
	height, err := nextEmptyTxOutHeight(ctx, k, ctx.BlockHeight()+windowBlocks)
	if err != nil {
		return nil, err
	}
	block, err := k.GetTxOut(ctx, height)
	if err != nil {
		return nil, err
	}
	block.Epoch = nextEpoch
	block.Status = TxOutStatusPendingBatch
	return block, nil
}

// txOutBatchVault returns the vault pubkey of a batch block: a block whose items are
// all epoch-batchable and belong to a single vault.
func txOutBatchVault(txOut TxOut) (common.PubKey, bool) {
	if len(txOut.TxArray) == 0 {
		return common.EmptyPubKey, false
	}
	vault := txOut.TxArray[0].VaultPubKey
	if vault.IsEmpty() {
		return common.EmptyPubKey, false
	}
	for _, item := range txOut.TxArray {
		if !btcEpochBatchItem(item) || !item.VaultPubKey.Equals(vault) {
			return common.EmptyPubKey, false
		}
	}
	return vault, true
}

// btcEpochBatchItem reports whether the item belongs in its vault's unified epoch
// batch: outbound and internal kinds spend from the vault root; sweeps contribute
// their pinned child-path deposit UTXO as an extra vin.
func btcEpochBatchItem(item TxOutItem) bool {
	if !item.Chain.Equals(common.BTCChain) {
		return false
	}
	switch types.NormalizeTxOutType(item.GetTxType()) {
	case types.TxOutTypeOut, types.TxOutTypeRefund, types.TxOutTypeMigrate, types.TxOutTypeConsolidate:
		return item.VaultPathIndex == common.MainVaultPathIndex
	case types.TxOutTypeSweep:
		return item.VaultPathIndex != common.MainVaultPathIndex
	default:
		return false
	}
}

// btcBatchVinOnlyItem reports whether the item contributes only inputs to its batch
// tx: deposit sweeps (child-path pin) and pinned migrates (specific root UTXO pin).
// Their value flows into the batch's remainder or change output; they get no vout.
func btcBatchVinOnlyItem(item TxOutItem) bool {
	if !btcEpochBatchItem(item) {
		return false
	}
	switch types.NormalizeTxOutType(item.GetTxType()) {
	case types.TxOutTypeSweep:
		return true
	case types.TxOutTypeMigrate:
		return btcPinnedSourceInputs(item)
	default:
		return false
	}
}

// btcBatchRemainderItem reports whether the item absorbs the batch remainder
// (union value minus fixed vouts minus gas): unpinned migrates and consolidates.
// At most one such item may share a batch.
func btcBatchRemainderItem(item TxOutItem) bool {
	if !btcEpochBatchItem(item) || btcBatchVinOnlyItem(item) {
		return false
	}
	return types.IsInternalTxOutType(item.TxType)
}

// btcBatchHasRoom reports whether an open pending_batch block can take the incoming
// item: vout-bearing items are capped at btcMaxBatchRecipients, vin-only items at
// btcMaxBatchVins, and only one remainder item may ride per batch.
func btcBatchHasRoom(txOut TxOut, incoming TxOutItem) bool {
	vinOnly, vouts, remainders := 0, 0, 0
	for _, item := range txOut.TxArray {
		switch {
		case btcBatchVinOnlyItem(item):
			vinOnly++
		case btcBatchRemainderItem(item):
			remainders++
			vouts++
		default:
			vouts++
		}
	}
	switch {
	case btcBatchVinOnlyItem(incoming):
		return vinOnly < btcMaxBatchVins
	case btcBatchRemainderItem(incoming):
		return remainders == 0 && vouts < btcMaxBatchRecipients
	default:
		return vouts < btcMaxBatchRecipients
	}
}

func appendBTCExactTxOut(ctx cosmos.Context, k keeper.Keeper, height int64, item TxOutItem) error {
	item.TxType = item.GetTxType()
	if btcEpochBatchItem(item) {
		if item.VaultPubKey.IsEmpty() {
			return fmt.Errorf("fail to batch bitcoin txout: empty vault pubkey")
		}
		block, err := btcVaultBatchTxOut(ctx, k, item)
		if err != nil {
			return err
		}
		block.TxArray = append(block.TxArray, item)
		// a batch queued behind an unfinished predecessor may not have spendable
		// source inputs yet; the end-block refresh fills them in once the prior
		// batch completes, and promotion waits until every item has sources
		if err := refreshBTCExactTxOutBlock(ctx, k, block); err != nil {
			ctx.Logger().Info("deferring bitcoin batch source input selection", "height", block.Height, "error", err)
		}
		return k.SetTxOut(ctx, block)
	}

	var block *TxOut
	var err error
	for offset := int64(0); offset < 1000; offset++ {
		block, err = k.GetTxOut(ctx, height+offset)
		if err != nil {
			return err
		}
		if txOutCanAppendImmediateBTCItem(block) {
			height += offset
			break
		}
		block = nil
	}
	if block == nil {
		return fmt.Errorf("fail to find bitcoin immediate txout slot from height %d", height)
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

func txOutCanAppendImmediateBTCItem(txOut *TxOut) bool {
	return txOut == nil || txOut.IsEmpty() || txOut.Status == ""
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
		if txOut.Status == TxOutStatusPendingBatch && btcEpochBatchItem(item) {
			for j := i + 1; j < len(txOut.TxArray); j++ {
				other := txOut.TxArray[j]
				if !handled[j] && btcEpochBatchItem(other) &&
					other.VaultPubKey.Equals(item.VaultPubKey) &&
					other.OutHash.IsEmpty() {
					group = append(group, j)
				}
			}
		} else if btcBatchableTxOut(item) {
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
	// unified epoch semantics apply while the batch is still open; promoted
	// blocks (and pre-upgrade in-flight blocks) keep the legacy per-item math
	if txOut.Status == TxOutStatusPendingBatch && btcEpochBatchItem(first) {
		return refreshBTCEpochBatchGroup(ctx, k, txOut, group)
	}
	return refreshBTCLegacyTxOutGroup(ctx, k, txOut, group)
}

// refreshBTCEpochBatchGroup computes the unified epoch batch tx for one vault:
// vins = every vin-only item's pinned UTXO plus root UTXOs selected to cover the
// fixed vouts, vouts = one per outbound item plus at most one remainder internal
// item that absorbs (union - fixed - gas). All items carry the identical unioned
// SourceInputs, each stamped with the taproot path that controls it.
func refreshBTCEpochBatchGroup(ctx cosmos.Context, k keeper.Keeper, txOut *TxOut, group []int) error {
	first := txOut.TxArray[group[0]]
	vault, err := k.GetVault(ctx, first.VaultPubKey)
	if err != nil {
		return fmt.Errorf("fail to get bitcoin txout vault: %w", err)
	}
	if vault.PubKey.IsEmpty() {
		return fmt.Errorf("missing bitcoin txout vault: %s", first.VaultPubKey)
	}
	rootAddr, err := common.DeriveBTCTaprootAddress(vault.PubKey, common.MainVaultPathIndex)
	if err != nil {
		return err
	}
	gasRate, err := btcGasRateFromKeeper(ctx, k)
	if err != nil {
		return err
	}

	var vinOnly, vouts, remainders []int
	for _, idx := range group {
		item := txOut.TxArray[idx]
		if !item.Coin.Asset.Equals(common.BTCAsset) {
			return fmt.Errorf("bitcoin txout item is not BTC: %s", item.Coin.Asset)
		}
		switch {
		case btcBatchVinOnlyItem(item):
			vinOnly = append(vinOnly, idx)
		case btcBatchRemainderItem(item):
			remainders = append(remainders, idx)
		default:
			vouts = append(vouts, idx)
		}
	}
	if len(remainders) > 1 {
		return fmt.Errorf("bitcoin epoch batch at height %d has %d remainder items", txOut.Height, len(remainders))
	}

	// every pinned item contributes its pin to the union: sweeps and pinned
	// migrates as vin-only value, refunds because they must spend the exact
	// deposit UTXO they return (double-payout protection)
	pins := make([]types.TxOutInput, 0, len(group))
	seenPins := make(map[string]struct{})
	for _, idx := range group {
		item := txOut.TxArray[idx]
		isVinOnly := btcBatchVinOnlyItem(item)
		if !isVinOnly && !btcPinnedSourceInputs(item) {
			continue
		}
		itemPins := btcItemPinnedInputs(item)
		if len(itemPins) == 0 {
			if isVinOnly {
				return fmt.Errorf("bitcoin vin-only txout item %s has no pinned source input", item.InHash)
			}
			continue
		}
		for _, pin := range itemPins {
			key := btcSourceInputKey(pin.TxId, pin.Vout)
			if _, ok := seenPins[key]; ok {
				continue
			}
			seenPins[key] = struct{}{}
			pins = append(pins, pin)
		}
	}
	pinnedValue := btcSourceInputsAmount(pins)

	outputAddrs := make([]common.Address, 0, len(vouts)+1)
	fixedTotal := cosmos.ZeroUint()
	for _, idx := range vouts {
		item := txOut.TxArray[idx]
		outputAddrs = append(outputAddrs, item.ToAddress)
		fixedTotal = fixedTotal.Add(item.Coin.Amount)
	}
	remainderTarget := cosmos.ZeroUint()
	if len(remainders) == 1 {
		item := txOut.TxArray[remainders[0]]
		outputAddrs = append(outputAddrs, item.ToAddress)
		remainderTarget = item.Coin.Amount
	}

	// Root UTXOs are selected to cover the fixed vouts plus the remainder target
	// net of pinned value; gas converges over a few passes as the input count
	// changes the estimate.
	union := append([]types.TxOutInput(nil), pins...)
	var maxGasCoin common.Coin
	needed := fixedTotal.Add(remainderTarget)
	for attempt := 0; attempt < 3; attempt++ {
		maxGasCoin = btcGasCoinFromNativeSats(0)
		if len(union) > 0 {
			maxGasCoin, err = btcExactGasCoin(vault.PubKey, common.MainVaultPathIndex, outputAddrs, union, gasRate)
			if err != nil {
				return err
			}
		}
		required := needed.Add(maxGasCoin.Amount)
		if btcSourceInputsAmount(union).GTE(required) {
			break
		}
		shortfall := required.Sub(cosmos.MinUint(pinnedValue, required))
		rootInputs := btcSelectVaultSourceInputs(ctx, k, vault, rootAddr, shortfall, txOut.Height)
		if len(rootInputs) == 0 {
			if len(remainders) == 1 && pinnedValue.GT(fixedTotal.Add(maxGasCoin.Amount)) {
				// pins alone fund the batch; the remainder absorbs what is left
				union = append([]types.TxOutInput(nil), pins...)
				break
			}
			return fmt.Errorf("no bitcoin source inputs available for vault %s epoch batch", vault.PubKey)
		}
		union = append(append([]types.TxOutInput(nil), pins...), rootInputs...)
		if len(union) > btcMaxBatchVins {
			return fmt.Errorf("bitcoin epoch batch at height %d needs %d vins, cap is %d", txOut.Height, len(union), btcMaxBatchVins)
		}
	}
	if len(union) == 0 {
		return fmt.Errorf("missing bitcoin source inputs for txout height %d", txOut.Height)
	}
	maxGasCoin, err = btcExactGasCoin(vault.PubKey, common.MainVaultPathIndex, outputAddrs, union, gasRate)
	if err != nil {
		return err
	}
	unionValue := btcSourceInputsAmount(union)
	spendTotal := fixedTotal.Add(maxGasCoin.Amount)
	if unionValue.LT(spendTotal) {
		return fmt.Errorf("insufficient bitcoin epoch batch inputs for vault %s: need %s, have %s", vault.PubKey, spendTotal, unionValue)
	}
	if len(remainders) == 1 {
		remainderCoin := unionValue.Sub(spendTotal)
		if remainderCoin.LTE(common.BTCChain.DustThreshold()) {
			return fmt.Errorf("bitcoin epoch batch remainder %s is dust for vault %s", remainderCoin, vault.PubKey)
		}
		txOut.TxArray[remainders[0]].Coin = common.NewCoin(common.BTCAsset, remainderCoin)
	}
	for _, idx := range vinOnly {
		txOut.TxArray[idx].Coin = common.NewCoin(common.BTCAsset, cosmos.ZeroUint())
	}

	gasShares := btcSplitGasCoin(maxGasCoin, len(group))
	for i, idx := range group {
		txOut.TxArray[idx].SourceInputs = append([]types.TxOutInput(nil), union...)
		txOut.TxArray[idx].MaxGas = common.Gas{gasShares[i]}
		txOut.TxArray[idx].GasRate = gasRate
	}
	return nil
}

// btcItemPinnedInputs extracts the source inputs a vin-only item pins: entries whose
// TxId matches the item's InHash. On the creation shape (the item still carries only
// its own pin) the entries get stamped with the item's taproot path; once the union
// is in place the stamped PathIndex disambiguates entries sharing a funding txid.
func btcItemPinnedInputs(item TxOutItem) []types.TxOutInput {
	var all, byPath []types.TxOutInput
	for _, input := range item.SourceInputs {
		if !input.TxId.Equals(item.InHash) {
			continue
		}
		all = append(all, input)
		if input.PathIndex == item.VaultPathIndex {
			byPath = append(byPath, input)
		}
	}
	if len(byPath) > 0 {
		return byPath
	}
	for i := range all {
		all[i].PathIndex = item.VaultPathIndex
	}
	return all
}

// refreshBTCLegacyTxOutGroup keeps the pre-epoch behavior for BTC items that do not
// participate in the unified batch.
func refreshBTCLegacyTxOutGroup(ctx cosmos.Context, k keeper.Keeper, txOut *TxOut, group []int) error {
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
	if len(inputs) == 0 {
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
				btcEpochBatchItem(item)
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
				// vin-only items carry no vout of their own; registering their
				// placeholder OutVout would mask the batch's real change vout
				if !btcBatchVinOnlyItem(item) {
					usedOutVouts[key][item.OutVout] = struct{}{}
				}
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
		if result[i].input.AmountSats != result[j].input.AmountSats {
			return result[i].input.AmountSats > result[j].input.AmountSats
		}
		if result[i].height != result[j].height {
			return result[i].height < result[j].height
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
	var total uint64
	for _, input := range inputs {
		if math.MaxUint64-total < input.AmountSats {
			wideTotal := cosmos.NewUint(total)
			for _, input := range inputs {
				wideTotal = wideTotal.Add(cosmos.NewUint(input.AmountSats))
			}
			return wideTotal
		}
		total += input.AmountSats
	}
	return cosmos.NewUint(total)
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
		a.VaultPathIndex == b.VaultPathIndex &&
		btcPinnedSourceInputsCompatible(a, b)
}

func btcPinnedSourceInputs(item TxOutItem) bool {
	if item.InHash.IsEmpty() {
		return false
	}
	for _, input := range item.SourceInputs {
		if input.TxId.Equals(item.InHash) {
			return true
		}
	}
	return false
}

func btcPinnedSourceInputsCompatible(a, b TxOutItem) bool {
	aPinned := btcPinnedSourceInputs(a)
	bPinned := btcPinnedSourceInputs(b)
	if !aPinned && !bPinned {
		return true
	}
	return aPinned && bPinned && btcTxOutInputsEqual(a.SourceInputs, b.SourceInputs)
}

type btcTxOutInputMapKey struct {
	txID       common.TxID
	vout       uint32
	amountSats uint64
}

func btcTxOutInputKey(input types.TxOutInput) btcTxOutInputMapKey {
	return btcTxOutInputMapKey{
		txID:       input.TxId,
		vout:       input.Vout,
		amountSats: input.AmountSats,
	}
}

func btcTxOutInputsEqual(a, b []types.TxOutInput) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) > 8 {
		matched := make(map[btcTxOutInputMapKey]uint16, len(a))
		for _, input := range a {
			matched[btcTxOutInputKey(input)]++
		}
		for _, input := range b {
			key := btcTxOutInputKey(input)
			if matched[key] == 0 {
				return false
			}
			matched[key]--
		}
		return true
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
