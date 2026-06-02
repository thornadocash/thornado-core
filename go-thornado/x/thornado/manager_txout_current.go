package thornado

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	sdkmath "cosmossdk.io/math"
	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// TxOutStorage is going to manage all the outgoing tx
type TxOutStorage struct {
	keeper        keeper.Keeper
	constAccessor constants.ConfigValues
	eventMgr      EventManager
	gasManager    GasManager
}

// newTxOutStorage will create a new instance of TxOutStore.
func newTxOutStorage(keeper keeper.Keeper, constAccessor constants.ConfigValues, eventMgr EventManager, gasManager GasManager) *TxOutStorage {
	return &TxOutStorage{
		keeper:        keeper,
		eventMgr:      eventMgr,
		constAccessor: constAccessor,
		gasManager:    gasManager,
	}
}

func (tos *TxOutStorage) EndBlock(ctx cosmos.Context, mgr Manager) error {
	if err := tos.updateBatchStates(ctx); err != nil {
		return err
	}
	// update the max gas for all outbounds in this block. This can be useful
	// if an outbound transaction was scheduled into the future, and the gas
	// for that blockchain changes in that time span. This avoids the need to
	// reschedule the transaction to Base.
	txOut, err := tos.GetBlockOut(ctx)
	if err != nil {
		return err
	}

	maxGasCache := make(map[common.Chain]common.Coin)
	gasRateCache := make(map[common.Chain]int64)
	gasDetailsFailed := make(map[common.Chain]bool)

	for i, tx := range txOut.TxArray {
		voter, err := tos.keeper.GetObservedTxInVoter(ctx, tx.InHash)
		if err != nil {
			ctx.Logger().Error("fail to get observe tx in voter", "error", err)
			continue
		}

		// if the outbound height exists and is in the past, then no need to calculate new max gas
		if voter.OutboundHeight > 0 && voter.OutboundHeight < ctx.BlockHeight() {
			continue
		}

		// update max gas, take the larger of the current gas, or the last gas used

		maxGasCoin, okMaxGas := maxGasCache[tx.Chain]
		gasRate, okGasRate := gasRateCache[tx.Chain]
		// update cache if needed
		if !okMaxGas || !okGasRate {
			if gasDetailsFailed[tx.Chain] {
				continue
			}
			var err error
			maxGasCoin, gasRate, err = mgr.GasMgr().GetGasDetails(ctx, tx.Chain)
			if err != nil {
				ctx.Logger().Error("fail to get gas details", "chain", tx.Chain, "error", err)
				gasDetailsFailed[tx.Chain] = true
				continue
			}
			maxGasCache[tx.Chain] = maxGasCoin
			gasRateCache[tx.Chain] = gasRate
		}

		if len(tx.MaxGas) == 0 || (!maxGasCoin.IsEmpty() && !maxGasCoin.Amount.Equal(tx.MaxGas[0].Amount)) {
			// Update MaxGas in ObservedTxVoter action first; only update txOut if voter update succeeds
			// to maintain consistency between the txOut item and the voter action.
			if err := updateTxOutGas(ctx, tos.keeper, tx, common.Gas{maxGasCoin}); err != nil {
				ctx.Logger().Error("Failed to update MaxGas of action in ObservedTxVoter", "hash", tx.InHash, "error", err)
			} else {
				txOut.TxArray[i].MaxGas = common.Gas{maxGasCoin}
			}
		}
		if gasRate > 0 && txOut.TxArray[i].GasRate != gasRate {
			// Equals checks GasRate so update actions GasRate too (before updating in the queue item)
			// for future updates of MaxGas, which must match for matchActionItem in AddOutTx.
			// Only update txOut GasRate if voter update succeeds to prevent permanent desync.
			if err := updateTxOutGasRate(ctx, tos.keeper, tx, gasRate); err != nil {
				ctx.Logger().Error("Failed to update GasRate of action in ObservedTxVoter", "hash", tx.InHash, "error", err)
			} else {
				txOut.TxArray[i].GasRate = gasRate
			}
		}
	}

	if err := tos.keeper.SetTxOut(ctx, txOut); err != nil {
		return fmt.Errorf("fail to save tx out : %w", err)
	}
	return nil
}

func (tos *TxOutStorage) updateBatchStates(ctx cosmos.Context) error {
	signingPeriod := getConfigDurationBlocks(ctx, tos.keeper, constants.Keysign_PeriodMinutes)
	retryPeriod := getConfigDurationBlocks(ctx, tos.keeper, constants.Withdrawal_BatchWindowMinutes)
	iterator := tos.keeper.GetTxOutIterator(ctx)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var txOut TxOut
		if err := tos.keeper.Cdc().Unmarshal(iterator.Value(), &txOut); err != nil {
			return err
		}
		if txOut.IsEmpty() {
			continue
		}
		if !txOutUsesBatching(txOut) {
			if err := tos.keeper.SetTxOut(ctx, &txOut); err != nil {
				return err
			}
			continue
		}
		if txOut.Status == TxOutStatusPendingBatch && txOut.Height <= ctx.BlockHeight() {
			txOut.Status = TxOutStatusPendingSign
			txOut.SigningAttempt = 0
			txOut.RetryUntilHeight = 0
		}
		if txOut.Status == TxOutStatusPendingSign && txOutHasPendingItems(txOut) && signingPeriod > 0 && ctx.BlockHeight() >= txOut.Height+int64(txOut.SigningAttempt+1)*signingPeriod {
			txOut.Status = TxOutStatusPendingRetry
			txOut.RetryUntilHeight = ctx.BlockHeight() + retryPeriod
			txOut.SigningLeader = common.EmptyPubKey
		}
		if txOut.Status == TxOutStatusPendingRetry && txOut.RetryUntilHeight <= ctx.BlockHeight() {
			txOut.Status = TxOutStatusPendingSign
			txOut.SigningAttempt++
			txOut.RetryUntilHeight = 0
		}
		if txOut.Status == TxOutStatusPendingSign && txOutHasPendingItems(txOut) {
			txOut.SigningLeader = tos.selectSigningLeader(ctx, txOut, txOut.SigningAttempt)
		}
		if err := tos.keeper.SetTxOut(ctx, &txOut); err != nil {
			return err
		}
	}
	return nil
}

func txOutUsesBatching(txOut TxOut) bool {
	if len(txOut.TxArray) == 0 {
		return false
	}
	for _, item := range txOut.TxArray {
		if !types.IsBatchableTxOutType(item.TxType) {
			return false
		}
	}
	return true
}

func txOutHasPendingItems(txOut TxOut) bool {
	for _, item := range txOut.TxArray {
		if item.OutHash.IsEmpty() {
			return true
		}
	}
	return false
}

func (tos *TxOutStorage) selectSigningLeader(ctx cosmos.Context, txOut TxOut, attempt uint64) common.PubKey {
	if len(txOut.TxArray) == 0 {
		return common.EmptyPubKey
	}
	vaultPubKey := txOut.TxArray[0].VaultPubKey
	active, err := tos.keeper.ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to list active nodes for txout signing leader", "error", err)
		return common.EmptyPubKey
	}
	members := make([]string, 0, len(active))
	for _, node := range active {
		for _, membership := range node.SignerMembership {
			if strings.EqualFold(membership, vaultPubKey.String()) && !node.PubKeySet.Secp256k1.IsEmpty() {
				members = append(members, node.PubKeySet.Secp256k1.String())
				break
			}
		}
	}
	if len(members) == 0 {
		return common.EmptyPubKey
	}
	sort.Strings(members)
	digest := sha256.Sum256([]byte(fmt.Sprintf("txout:%d:%d", txOut.Epoch, txOut.Height)))
	offset := binary.BigEndian.Uint64(digest[:8])
	leader, err := common.NewPubKey(members[(offset+attempt)%uint64(len(members))])
	if err != nil {
		ctx.Logger().Error("fail to parse txout signing leader", "error", err)
		return common.EmptyPubKey
	}
	return leader
}

// GetBlockOut read the TxOut from kv store
func (tos *TxOutStorage) GetBlockOut(ctx cosmos.Context) (*TxOut, error) {
	return tos.keeper.GetTxOut(ctx, ctx.BlockHeight())
}

// GetOutboundItems read all the outbound item from kv store
func (tos *TxOutStorage) GetOutboundItems(ctx cosmos.Context) ([]TxOutItem, error) {
	block, err := tos.keeper.GetTxOut(ctx, ctx.BlockHeight())
	if block == nil {
		return nil, nil
	}
	return block.TxArray, err
}

// GetOutboundItemByToAddress read all the outbound items filter by the given to address
func (tos *TxOutStorage) GetOutboundItemByToAddress(ctx cosmos.Context, to common.Address) []TxOutItem {
	filterItems := make([]TxOutItem, 0)
	items, _ := tos.GetOutboundItems(ctx)
	for _, item := range items {
		if item.ToAddress.Equals(to) {
			filterItems = append(filterItems, item)
		}
	}
	return filterItems
}

// ClearOutboundItems remove all the tx out items , mostly used for test
func (tos *TxOutStorage) ClearOutboundItems(ctx cosmos.Context) {
	_ = tos.keeper.ClearTxOut(ctx, ctx.BlockHeight())
}

// When TryAddTxOutItem returns an error, there should be no state changes from it,
// including funds movements or fee events from prepareTxOutItem.
// So, use CacheContext to only commit state changes when cachedTryAddTxOutItem doesn't return an error.
func (tos *TxOutStorage) TryAddTxOutItem(ctx cosmos.Context, mgr Manager, toi TxOutItem, minOut cosmos.Uint) (bool, error) {
	if toi.ToAddress.IsNoop() {
		return true, nil
	}

	cacheCtx, commit := ctx.CacheContext()

	success, err := tos.cachedTryAddTxOutItem(cacheCtx, mgr, toi, minOut)
	if err == nil {
		commit()
	}
	return success, err
}

// (cached)TryAddTxOutItem add an outbound tx to block
// return bool indicate whether the transaction had been added successful or not
// return error indicate error
func (tos *TxOutStorage) cachedTryAddTxOutItem(ctx cosmos.Context, mgr Manager, toi TxOutItem, minOut cosmos.Uint) (bool, error) {
	outputs, _, err := tos.prepareTxOutItem(ctx, toi)
	if err != nil {
		return false, fmt.Errorf("failed to prepare tx out item: %w", err)
	}
	if len(outputs) == 0 {
		return false, ErrNotEnoughToPayFee
	}

	sumOut := cosmos.ZeroUint()
	for _, o := range outputs {
		sumOut = sumOut.Add(o.Coin.Amount)
	}
	if sumOut.LT(minOut) {
		return false, fmt.Errorf("outbound amount does not meet requirements (%d/%d)", sumOut.Uint64(), minOut.Uint64())
	}

	// calculate the single block height to send all of these txout items,
	// using the summed amount
	outboundHeight := ctx.BlockHeight()
	cloutApplied := cosmos.ZeroUint()
	if !toi.Chain.Equals(common.BTCChain) && !toi.InHash.IsEmpty() && !toi.InHash.Equals(common.BlankTxID) {
		voter, err := tos.keeper.GetObservedTxInVoter(ctx, toi.InHash)
		if err != nil {
			ctx.Logger().Error("fail to get observe tx in voter", "error", err)
			return false, fmt.Errorf("fail to get observe tx in voter,err:%w", err)
		}

		var targetHeight int64
		// Build a synthetic TxOutItem representing the whole batch so
		// CalcTxOutHeight sees the total Coin and MaxGas values.
		scheduled := outputs[0]
		scheduled.Coin.Amount = cosmos.ZeroUint()
		scheduled.MaxGas = common.Gas{}
		for _, output := range outputs {
			scheduled.Coin.Amount = scheduled.Coin.Amount.Add(output.Coin.Amount)
			scheduled.MaxGas = scheduled.MaxGas.Add(output.MaxGas...)
		}
		targetHeight, cloutApplied, err = tos.CalcTxOutHeight(ctx, mgr.GetVersion(), scheduled)
		if err != nil {
			ctx.Logger().Error("failed to calc target block height for txout item", "error", err)
		}

		// adjust delay to include streaming swap time since inbound consensus
		if voter.Height > 0 {
			targetHeight = (targetHeight - ctx.BlockHeight()) + voter.Height
		}

		if targetHeight > outboundHeight {
			outboundHeight = targetHeight
		}

		// While each outbound has its own security-appropriate outbound delay,
		// ensure the voter.OutboundHeight reflects the furthest-future scheduled outbound height
		// so as to serve as an estimate of when the entire transaction may be completed.
		if outboundHeight > voter.OutboundHeight {
			voter.OutboundHeight = outboundHeight
			tos.keeper.SetObservedTxInVoter(ctx, voter)
		}
	}

	// sum total output asset
	sumOutput := cosmos.ZeroUint()
	for _, output := range outputs {
		sumOutput = sumOutput.Add(output.Coin.Amount)
	}

	// add tx to block out
	totalCloutShare := cosmos.ZeroUint()
	for i, output := range outputs {
		cloutShare := cosmos.ZeroUint()
		if i < len(outputs)-1 {
			cloutShare = common.GetSafeShare(output.Coin.Amount, sumOutput, cloutApplied)
			totalCloutShare = totalCloutShare.Add(cloutShare)
		} else {
			cloutShare = common.SafeSub(cloutApplied, totalCloutShare) // remainder
		}
		output.CloutSpent = &cloutShare
		if err := tos.addToBlockOut(ctx, mgr, output, outboundHeight); err != nil {
			return false, err
		}
	}

	return true, nil
}

// UnSafeAddTxOutItem - blindly adds a tx out, skipping vault selection, transaction
// fee deduction, etc
func (tos *TxOutStorage) UnSafeAddTxOutItem(ctx cosmos.Context, mgr Manager, toi TxOutItem, height int64) error {
	if toi.ToAddress.IsNoop() {
		return nil
	}

	return tos.addToBlockOut(ctx, mgr, toi, height)
}

// selectFallbackVault selects a vault to assign an outbound to when no vault has sufficient
// available balance. This allows the outbound to enter the recovery flow (slasher will
// reschedule or trigger reverse swap) rather than silently dropping the funds.
// The vault with the highest balance of the required asset is selected, as long as it has
// minimum gas to attempt signing.
// Note: vaults passed here may have had pending outbounds deducted, so we fetch original
// balances from the keeper to make selection decisions based on actual vault holdings.
func (tos *TxOutStorage) selectFallbackVault(ctx cosmos.Context, toi TxOutItem, maxGasCoin common.Coin, vaults Vaults) Vault {
	var bestVault Vault
	bestAmount := cosmos.ZeroUint()

	for _, vault := range vaults {
		// Skip if vault is frozen for this chain
		if len(vault.Frozen) > 0 {
			chains, err := common.NewChains(vault.Frozen)
			if err != nil {
				ctx.Logger().Error("failed to convert chains", "error", err)
			}
			if chains.Has(toi.Chain) {
				continue
			}
		}

		// Get the original vault from keeper to check actual balances
		// (not the pending-outbound-deducted balances)
		originalVault, err := tos.keeper.GetVault(ctx, vault.PubKey)
		if err != nil {
			ctx.Logger().Error("failed to get vault from keeper", "error", err)
			continue
		}

		// Skip if vault address matches ToAddress (prevent self-sends)
		fromAddr, err := originalVault.GetAddress(toi.Chain)
		if err != nil || fromAddr.IsEmpty() || toi.ToAddress.Equals(fromAddr) {
			continue
		}

		// Must have minimum gas asset to attempt signing (use original balance)
		gasAsset := originalVault.GetCoin(toi.Chain.GetGasAsset())
		if gasAsset.IsEmpty() || gasAsset.Amount.LT(maxGasCoin.Amount) {
			continue
		}

		// Must have some amount of the required asset (use original balance)
		vaultCoinAmount := originalVault.GetCoin(toi.Coin.Asset).Amount
		if vaultCoinAmount.IsZero() {
			continue
		}

		// Select the vault with the highest balance of the required asset
		if vaultCoinAmount.GT(bestAmount) {
			bestAmount = vaultCoinAmount
			bestVault = originalVault
		}
	}

	return bestVault
}

func (tos *TxOutStorage) DiscoverOutbounds(ctx cosmos.Context, transactionFeeAmount cosmos.Uint, maxGasCoin common.Coin, toi TxOutItem, vaults Vaults) ([]TxOutItem, cosmos.Uint) {
	var outputs []TxOutItem

	// Save original vaults before any filtering for potential fallback selection.
	// We need this because the sort logic below may filter out vaults with zero available balance.
	originalVaults := vaults

	// When there is more than one vault, sort the vaults by
	// (as an integer) how many vaults of that size
	// would be necessary to fulfill the outbound (smallest number first).
	// Having already been sorted by security, for a given vaults-necessary
	// the lowest security ones will still be ordered first.
	// The greater a vault's vaults-necessary, the less its security would be
	// decreased by taking part in the outbound;
	// also, outbounds from negligible-amount vaults (other than wasting gas) risk creating
	// duplicate txout items of which all but one would be stuck in the outbound queue.
	// Note that for vaults of equal (integer) vaults-necessary, any previous sort order remains.
	if len(vaults) > 1 {
		type VaultsNecessary struct {
			Vault    Vault
			Estimate uint64
		}

		vaultsNecessary := make([]VaultsNecessary, 0)

		for _, vault := range vaults {
			// Avoid a divide-by-zero by ignoring vaults with zero of the asset.
			if vault.GetCoin(toi.Coin.Asset).Amount.IsZero() {
				continue
			}

			// if vault is frozen, don't send more txns to sign, as they may be
			// delayed. Once a txn is skipped here, it will not be rescheduled again.
			if len(vault.Frozen) > 0 {
				chains, err := common.NewChains(vault.Frozen)
				if err != nil {
					ctx.Logger().Error("failed to convert chains", "error", err)
				}
				if chains.Has(maxGasCoin.Asset.GetChain()) {
					continue
				}
			}

			estimate := toi.Coin.Amount.Quo(vault.GetCoin(toi.Coin.Asset).Amount)
			var estimateU64 uint64
			if estimate.BigInt().IsUint64() {
				estimateU64 = estimate.Uint64()
			} else {
				estimateU64 = math.MaxUint64
			}
			vaultsNecessary = append(vaultsNecessary, VaultsNecessary{
				Vault:    vault,
				Estimate: estimateU64,
			})
		}

		// If more than one vault remains, sort by vaults-necessary ascending.
		if len(vaultsNecessary) > 1 {
			sort.SliceStable(vaultsNecessary, func(i, j int) bool {
				return vaultsNecessary[i].Estimate < vaultsNecessary[j].Estimate
			})
		}

		// Set 'vaults' to the sorted order.
		vaults = make(Vaults, len(vaultsNecessary))
		for i, v := range vaultsNecessary {
			vaults[i] = v.Vault
		}
	}

	for _, vault := range vaults {
		// Ensure BTCChain are not sending from and to the same address
		fromAddr, err := vault.GetAddress(toi.Chain)
		if err != nil || fromAddr.IsEmpty() || toi.ToAddress.Equals(fromAddr) {
			continue
		}

		vaultCoinAmount := vault.GetCoin(toi.Coin.Asset).Amount
		// if the asset in the vault is not enough to pay for the fee , then skip it
		if vaultCoinAmount.LTE(transactionFeeAmount) {
			continue
		}
		// if the vault doesn't have gas asset in it , or it doesn't have enough to pay for gas
		gasAsset := vault.GetCoin(toi.Chain.GetGasAsset())
		if gasAsset.IsEmpty() || gasAsset.Amount.LT(maxGasCoin.Amount) {
			continue
		}

		// If the outbound Asset is the gas Asset, assigning to the limit would go over the limit,
		// so reduce the available vaultCoinAmount by that MaxGas Amount.
		if toi.Coin.Asset.Equals(maxGasCoin.Asset) {
			vaultCoinAmount = common.SafeSub(vaultCoinAmount, maxGasCoin.Amount)
			if vaultCoinAmount.IsZero() {
				continue
			}
		}

		toi.VaultPubKey = vault.PubKey
		toi.VaultPubKeyEddsa = vault.PubKeyEddsa

		if toi.Coin.Amount.LTE(vaultCoinAmount) {
			outputs = append(outputs, toi)
			toi.Coin.Amount = cosmos.ZeroUint()
			break
		} else {
			remainingAmount := common.SafeSub(toi.Coin.Amount, vaultCoinAmount)
			toi.Coin.Amount = common.SafeSub(toi.Coin.Amount, remainingAmount)
			outputs = append(outputs, toi)
			toi.Coin.Amount = remainingAmount
		}
	}

	// If we still have remaining amount, handle it based on whether we have partial outputs.
	// This allows the outbound to enter the recovery flow (slasher will reschedule or trigger reverse swap)
	// rather than silently dropping the funds.
	if !toi.Coin.Amount.IsZero() {
		if len(outputs) == 0 {
			// No outputs were created - select a fallback vault.
			// Use originalVaults (not filtered by available balance) to find a vault that has the asset.
			fallbackVault := tos.selectFallbackVault(ctx, toi, maxGasCoin, originalVaults)
			if !fallbackVault.PubKey.IsEmpty() {
				ctx.Logger().Info("selecting fallback vault for outbound with insufficient available balance",
					"vault", fallbackVault.PubKey.String(),
					"asset", toi.Coin.Asset.String(),
					"amount", toi.Coin.Amount.String())
				toi.VaultPubKey = fallbackVault.PubKey
				toi.VaultPubKeyEddsa = fallbackVault.PubKeyEddsa
				outputs = append(outputs, toi)
				toi.Coin.Amount = cosmos.ZeroUint()
			}
		} else {
			// Partial outputs exist but there's a remainder that couldn't be allocated.
			// Add the remainder to the smallest output - this will cause that outbound to fail
			// during signing (insufficient vault balance) and be rescheduled through the normal
			// recovery flow until a vault has sufficient balance.
			smallestIdx := 0
			smallestAmount := outputs[0].Coin.Amount
			for i := 1; i < len(outputs); i++ {
				if outputs[i].Coin.Amount.LT(smallestAmount) {
					smallestIdx = i
					smallestAmount = outputs[i].Coin.Amount
				}
			}
			ctx.Logger().Info("adding remainder to smallest output for recovery flow",
				"vault", outputs[smallestIdx].VaultPubKey.String(),
				"asset", toi.Coin.Asset.String(),
				"original_amount", outputs[smallestIdx].Coin.Amount.String(),
				"remainder", toi.Coin.Amount.String())
			outputs[smallestIdx].Coin.Amount = outputs[smallestIdx].Coin.Amount.Add(toi.Coin.Amount)
			toi.Coin.Amount = cosmos.ZeroUint()
		}
	}

	return outputs, toi.Coin.Amount
}

// prepareTxOutItem will do some data validation which include the following
// 1. choose an appropriate vault(s) to send from (active base, then retiring base)
// 2. deduct transaction fee, keep in mind, only take transaction fee when active nodes are  more then minimumBFT
// return list of outbound transactions
func (tos *TxOutStorage) prepareTxOutItem(ctx cosmos.Context, toi TxOutItem) ([]TxOutItem, sdkmath.Uint, error) {
	var outputs []TxOutItem
	var remaining cosmos.Uint

	// Ensure the InHash is set
	if toi.InHash.IsEmpty() {
		toi.InHash = common.BlankTxID
	}

	if toi.ToAddress.IsEmpty() {
		return outputs, cosmos.ZeroUint(), fmt.Errorf("empty to address, can't send out")
	}
	if !toi.ToAddress.IsChain(toi.Chain) {
		return outputs, cosmos.ZeroUint(), fmt.Errorf("to address(%s), is not of chain(%s)", toi.ToAddress, toi.Chain)
	}

	transactionFeeAmount, err := tos.gasManager.GetAssetOutboundFee(ctx, toi.Coin.Asset, false)
	if err != nil {
		return nil, cosmos.ZeroUint(), fmt.Errorf("fail to get outbound fee: %w", err)
	}
	maxGasCoin, gasRate, err := tos.gasManager.GetGasDetails(ctx, toi.Chain)
	if err != nil {
		return nil, cosmos.ZeroUint(), fmt.Errorf("fail to get max gas details: %w", err)
	}

	// Here is the VaultPubKey selection.
	if !toi.VaultPubKey.IsEmpty() {
		// a vault is already manually selected, blindly go forth with that
		outputs = append(outputs, toi)
	} else {
		// No vault is already selected, discover one.
		// List all pending outbounds for the asset, this will be used
		// to deduct balances of vaults that have outstanding txs assigned
		pendingOutbounds := tos.keeper.GetPendingOutbounds(ctx, toi.Coin.Asset)

		signingTransactionPeriod := getConfigDurationBlocks(ctx, tos.keeper, constants.Keysign_PeriodMinutes)

		// ///////////// COLLECT ACTIVE BASE VAULTS ///////////////////
		var activeBaseVaults Vaults
		activeBaseVaults, err = tos.keeper.GetBaseVaultsByStatus(ctx, ActiveVault)
		if err != nil {
			return nil, cosmos.ZeroUint(), fmt.Errorf("fail to get active vaults: %w", err)
		}

		// All else being equal, prefer lower-security vaults for outbounds.
		activeBaseVaults = tos.keeper.SortBySecurity(ctx, activeBaseVaults, signingTransactionPeriod)

		for i := range activeBaseVaults {
			// having sorted by security, deduct the value of any assigned pending outbounds
			activeBaseVaults[i].DeductVaultPendingOutbounds(pendingOutbounds)
		}
		// //////////////////////////////////////////////////////////////

		// ///////////// COLLECT RETIRING BASE VAULTS /////////////////
		var retiringBaseVaults Vaults
		retiringBaseVaults, err = tos.keeper.GetBaseVaultsByStatus(ctx, RetiringVault)
		if err != nil {
			return nil, cosmos.ZeroUint(), fmt.Errorf("fail to get retiring vaults: %w", err)
		}

		// All else being equal, prefer lower-security vaults for outbounds.
		retiringBaseVaults = tos.keeper.SortBySecurity(ctx, retiringBaseVaults, signingTransactionPeriod)

		for i := range retiringBaseVaults {
			// having sorted by security, deduct the value of any assigned pending outbounds
			retiringBaseVaults[i].DeductVaultPendingOutbounds(pendingOutbounds)
		}
		// //////////////////////////////////////////////////////////////

		// iterate over discovered vaults and find vaults to send funds from

		// All else being equal, choose active BaseVaults over retiring BaseVaults.
		outputs, remaining = tos.DiscoverOutbounds(ctx, transactionFeeAmount, maxGasCoin, toi, append(activeBaseVaults, retiringBaseVaults...))

		// Check we found enough funds to satisfy the request, error if we didn't
		if !remaining.IsZero() {
			return nil, cosmos.ZeroUint(), fmt.Errorf("insufficient funds for outbound request: %s %s remaining", toi.ToAddress.String(), remaining.String())
		}
	}

	// get the lending address to avoid deducting the outbound fee
	lendAddr, err := tos.keeper.GetModuleAddress(LendingName)
	if err != nil {
		return nil, cosmos.ZeroUint(), fmt.Errorf("fail to get lending address: %w", err)
	}
	// Here is the deduction from each output of either the MaxGas cost or the full outbound fee, but not both.
	var finalOutput []TxOutItem
	var feeEvents []*EventFee
	finalRuneFee := cosmos.ZeroUint()
	for i := range outputs {
		if outputs[i].MaxGas.IsEmpty() {
			outputs[i].MaxGas = common.Gas{
				maxGasCoin,
			}
			// BTC-only Thornado doesn't need to have max gas
			if outputs[i].MaxGas.IsEmpty() && !outputs[i].Chain.Equals(common.BTCChain) {
				return nil, cosmos.ZeroUint(), fmt.Errorf("max gas cannot be empty: %s", outputs[i].MaxGas)
			}

			outputs[i].GasRate = gasRate
		}

		feeDeduction := true

		// BTCChain txouts by nature allow fee deduction, but InactiveVault outbounds
		// require either no deduction or gas cost deduction instead.
		if !outputs[i].Chain.Equals(common.BTCChain) {
			vault, err := tos.keeper.GetVault(ctx, outputs[i].VaultPubKey)
			if err != nil {
				// An error is assumed for an empty VaultPubKey (BTCChain outbound),
				// but here avoided by the earlier conditional.
				ctx.Logger().Error("fail to get vault", "error", err)
			}

			// Whether the vault is truly an InactiveVault or the GetVault could not succeed
			// (InactiveVault is the default VaultStatus, 0),
			// do not try to deduct an outbound fee and instead only try to deduct gas asset MaxGas.
			if vault.Status == InactiveVault {
				feeDeduction = false
			}

			if !feeDeduction && outputs[i].Coin.Asset.IsGasAsset() {
				gasAmt := outputs[i].MaxGas.ToCoins().GetCoin(outputs[i].Coin.Asset).Amount
				outputs[i].Coin.Amount = common.SafeSub(outputs[i].Coin.Amount, gasAmt)
			}
		}

		if feeDeduction && !toi.ToAddress.Equals(lendAddr) {
			if outputs[i].Coin.Asset.IsRune() {
				runeFee := transactionFeeAmount // Fee is the prescribed RUNE fee
				if runeFee.GT(outputs[i].Coin.Amount) {
					runeFee = outputs[i].Coin.Amount // Fee is the full amount
				}
				finalRuneFee = finalRuneFee.Add(runeFee)
				outputs[i].Coin.Amount = common.SafeSub(outputs[i].Coin.Amount, runeFee)
				fee := common.NewFee(common.Coins{common.NewCoin(outputs[i].Coin.Asset, runeFee)}, cosmos.ZeroUint())
				feeEvents = append(feeEvents, NewEventFee(outputs[i].InHash, fee, cosmos.ZeroUint()))
			} else {
				assetFee := transactionFeeAmount
				if outputs[i].Coin.Amount.LTE(assetFee) {
					assetFee = outputs[i].Coin.Amount // Fee is the full amount
				}

				coinAmountAfterFee := common.SafeSub(outputs[i].Coin.Amount, assetFee) // Calculate amount after fee deduction

				// Check dust threshold BEFORE making any state changes (Finding 1)
				// This prevents pool balance corruption when outputs are dropped
				if toi.Coin.Asset.IsGasAsset() && coinAmountAfterFee.LT(toi.Chain.DustThreshold()) {
					ctx.Logger().
						With("inbound", toi.InHash).
						With("amount", coinAmountAfterFee).
						With("fee", transactionFeeAmount).
						Error("dropping gas asset output below dust threshold")
					continue
				}

				// Check for zero coin BEFORE making any state changes (Finding 2)
				// This prevents pool balance corruption and fee event inconsistency
				if coinAmountAfterFee.IsZero() {
					ctx.Logger().Info("tx out item would have zero coin after fee, checking if withdrawal", "tx_out", outputs[i].String())
					continue
				}

				// Now safe to deduct fee since we've confirmed the output won't be dropped
				outputs[i].Coin.Amount = coinAmountAfterFee

				if outputs[i].Coin.Asset.IsSyntheticAsset() || outputs[i].Coin.Asset.IsDerivedAsset() {
					// burn the native asset which used to pay for fee, that's only required when sending Synthetic/Derived assets from base
					// (not for instance applicable for Trade/Secured Assets which are not (1-to-1) Cosmos-SDK coins transferred from the Pool Module)
					// Only burn if assetFee > 0 (synths have zero fee, so skip for them)
					if outputs[i].GetModuleName() == BaseName && !assetFee.IsZero() {
						// Finding 4: Return errors from synth/derived fee burning to prevent supply accounting errors
						if err := tos.keeper.SendFromModuleToModule(ctx,
							BaseName,
							ModuleName,
							common.NewCoins(common.NewCoin(outputs[i].Coin.Asset, assetFee))); err != nil {
							return nil, cosmos.ZeroUint(), fmt.Errorf("fail to move native asset fee from base to Module: %w", err)
						}
						if err := tos.keeper.BurnFromModule(ctx, ModuleName, common.NewCoin(outputs[i].Coin.Asset, assetFee)); err != nil {
							return nil, cosmos.ZeroUint(), fmt.Errorf("fail to burn native asset: %w", err)
						}
						burnEvt := NewEventMintBurn(BurnSupplyType, outputs[i].Coin.Asset.Native(), assetFee, "burn_native_fee")
						if err := tos.eventMgr.EmitEvent(ctx, burnEvt); err != nil {
							ctx.Logger().Error("fail to emit burn event", "error", err)
						}
					}
				}

				fee := common.NewFee(common.Coins{common.NewCoin(outputs[i].Coin.Asset, assetFee)}, cosmos.ZeroUint())
				feeEvents = append(feeEvents, NewEventFee(outputs[i].InHash, fee, cosmos.ZeroUint()))
			}
		}
		// After applying all fees, check if the coin is still empty
		// (this can happen for RUNE outputs where the fee equals the amount)
		if outputs[i].Coin.IsEmpty() {
			ctx.Logger().Info("tx out item has zero coin", "tx_out", outputs[i].String())

			continue
		}

		// If the outbound coin is synthetic, respecting decimals is unnecessary
		// and leaves unburnt synths in the Pool Module
		if !outputs[i].Coin.Asset.IsSyntheticAsset() {
			outputs[i].Coin.Amount = cosmos.RoundToDecimal(outputs[i].Coin.Amount, int64(common.CoinDecimals))
		}

		if !outputs[i].InHash.Equals(common.BlankTxID) {
			// increment out number of out tx for this in tx
			voter, err := tos.keeper.GetObservedTxInVoter(ctx, outputs[i].InHash)
			if err != nil {
				return nil, cosmos.ZeroUint(), fmt.Errorf("fail to get observed tx voter: %w", err)
			}
			voter.FinalisedHeight = ctx.BlockHeight()
			voter.Actions = append(voter.Actions, outputs[i])
			tos.keeper.SetObservedTxInVoter(ctx, voter)
		}

		finalOutput = append(finalOutput, outputs[i])
	}

	for _, feeEvent := range feeEvents {
		if err := tos.eventMgr.EmitFeeEvent(ctx, feeEvent); err != nil {
			ctx.Logger().Error("fail to emit fee event", "error", err)
		}
	}
	if !finalRuneFee.IsZero() {
		if toi.Coin.IsRune() {
			// If the source module is the Reserve, leave the fee in the Reserve without a transfer.
			if toi.ModuleName != ReserveName {
				sourceModule := toi.GetModuleName() // Ensure that non-"".
				coin := common.NewCoin(common.BTCAsset, finalRuneFee)
				// Finding 3: Return error instead of just logging to prevent fund accounting mismatch
				// If this transfer fails, the fee has been deducted from the outbound but Reserve never receives it
				if err := tos.keeper.SendFromModuleToModule(ctx, sourceModule, ReserveName, common.NewCoins(coin)); err != nil {
					return nil, cosmos.ZeroUint(), fmt.Errorf("fail to send fee to reserve: %w", err)
				}
			}
		} else {
			// GetModuleName() to ensure that non-"" (BaseName).
			sourceModule := toi.GetModuleName()

			// Layer 1 or Synth Asset is implicitly swapped in a pool
			// whether in vault or burnt from another network module,
			// but Derived Asset has no outbound fee taken
			// so that the emitted amount passed to the loan handler
			// and the amount transferred to the Lending module are the same.
			// (If a fee were taken, then being for a Derived Asset pool
			//  it would contribute to Lending breathing room
			//  rather than affecting Pool Module RUNE.)
			//
			// If the source module is the Reserve, leave the fee in the Reserve without a transfer.
			if !toi.Coin.Asset.IsDerivedAsset() && sourceModule != ReserveName {
				coin := common.NewCoin(common.BTCAsset, finalRuneFee)
				// Finding 3: Return error instead of just logging to prevent fund accounting mismatch
				// If this transfer fails, the fee has been deducted from pool balances but Reserve never receives it
				if err := tos.keeper.SendFromModuleToModule(ctx, sourceModule, ReserveName, common.NewCoins(coin)); err != nil {
					return nil, cosmos.ZeroUint(), fmt.Errorf("fail to send fee to reserve: %w", err)
				}
			}
		}
	}

	return finalOutput, finalRuneFee, nil
}

func (tos *TxOutStorage) addToBlockOut(ctx cosmos.Context, mgr Manager, item TxOutItem, outboundHeight int64) error {
	// if we're sending native assets, transfer them now and return
	if item.Chain.Equals(common.BTCChain) {
		return tos.nativeTxOut(ctx, mgr, item)
	}

	// The outbound queue should never receive an item with a nil pointer field.
	if item.AggregatorTargetLimit == nil {
		aggregatorTargetLimit := cosmos.ZeroUint()
		item.AggregatorTargetLimit = &aggregatorTargetLimit
	}
	if item.CloutSpent == nil {
		cloutSpent := cosmos.ZeroUint()
		item.CloutSpent = &cloutSpent
	}
	item.TxType = item.GetTxType()

	vault, err := tos.keeper.GetVault(ctx, item.VaultPubKey)
	if err != nil {
		ctx.Logger().Error("fail to get vault", "error", err)
	}
	labels := []metrics.Label{
		telemetry.NewLabel("vault_type", vault.Type.String()),
		telemetry.NewLabel("pubkey", item.VaultPubKey.String()),
		telemetry.NewLabel("memo_type", "disabled"),
	}
	telemetry.SetGaugeWithLabels([]string{"thornado", "vault", "out_txn"}, float32(1), labels)

	if err := tos.eventMgr.EmitEvent(ctx, NewEventScheduledOutbound(item)); err != nil {
		ctx.Logger().Error("fail to emit scheduled outbound event", "error", err)
	}

	return tos.keeper.AppendTxOut(ctx, outboundHeight, item)
}

func (tos *TxOutStorage) CalcTxOutHeight(ctx cosmos.Context, version semver.Version, toi TxOutItem) (int64, cosmos.Uint, error) {
	return ctx.BlockHeight(), cosmos.ZeroUint(), nil
}

func (tos *TxOutStorage) nativeTxOut(ctx cosmos.Context, mgr Manager, toi TxOutItem) error {
	addr, err := toi.ToAddress.AccAddress()
	if err != nil {
		return err
	}

	toi.ModuleName = toi.GetModuleName() // Ensure that non-"".

	// mint if we're sending from BTCChain module
	if toi.ModuleName == ModuleName {
		if err = tos.keeper.MintToModule(ctx, toi.ModuleName, toi.Coin); err != nil {
			return fmt.Errorf("fail to mint coins during txout: %w", err)
		}
		mintEvt := NewEventMintBurn(MintSupplyType, toi.Coin.Asset.Native(), toi.Coin.Amount, "native_tx_out")
		if err = tos.eventMgr.EmitEvent(ctx, mintEvt); err != nil {
			ctx.Logger().Error("fail to emit mint event", "error", err)
		}
	}

	reserveAddress, err := tos.keeper.GetModuleAddress(ReserveName)
	if err != nil {
		ctx.Logger().Error("fail to get from address", "err", err)
		return err
	}

	// send funds to/from modules
	var sdkErr error

	if reserveAddress.Equals(toi.ToAddress) {
		sdkErr = tos.keeper.SendFromModuleToModule(ctx, toi.ModuleName, ReserveName, common.NewCoins(toi.Coin))
	} else {
		sdkErr = tos.keeper.SendFromModuleToAccount(ctx, toi.ModuleName, addr, common.NewCoins(toi.Coin))
	}

	if sdkErr != nil {
		return sdkErr
	}

	from, err := tos.keeper.GetModuleAddress(toi.ModuleName)
	if err != nil {
		ctx.Logger().Error("fail to get from address", "err", err)
		return err
	}
	outboundTxFee := tos.keeper.GetOutboundTxFee(ctx)

	tx := common.NewTx(
		common.BlankTxID,
		from,
		toi.ToAddress,
		common.Coins{toi.Coin},
		common.Gas{common.NewCoin(common.BTCAsset, outboundTxFee)},
	)

	active, err := tos.keeper.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		ctx.Logger().Error("fail to get active vaults", "err", err)
		return err
	}

	if len(active) == 0 {
		return fmt.Errorf("dev error: no pubkey for native txn")
	}

	observedTx := ObservedTx{
		ObservedPubKey: active[0].PubKey,
		BlockHeight:    ctx.BlockHeight(),
		Tx:             tx,
		FinaliseHeight: ctx.BlockHeight(),
	}
	m, err := processOneTxIn(ctx, tos.keeper, observedTx, tos.keeper.GetModuleAccAddress(BaseName))
	if err != nil {
		ctx.Logger().Error("fail to process txOut", "error", err, "tx", tx.String())
		return err
	}

	handler := NewInternalHandler(mgr)

	_, err = handler(ctx, m)
	if err != nil {
		ctx.Logger().Error("TxOut Handler failed:", "error", err)
		return err
	}

	return nil
}
