package thornado

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func RegisterDepositPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken string, powDurationMs uint64) (types.DepositSession, error) {
	if owner.Empty() {
		return types.DepositSession{}, fmt.Errorf("missing deposit owner")
	}
	powToken = strings.TrimSpace(powToken)
	if powToken == "" {
		return types.DepositSession{}, fmt.Errorf("missing deposit pow token")
	}
	powDifficulty := currentDepositPowDifficulty(ctx, k)
	if err := validateDepositPowToken(ctx, k, owner, powToken); err != nil {
		return types.DepositSession{}, err
	}
	if existing, err := k.GetDepositSessionByPowToken(ctx, powToken); err == nil && !existing.DepositAddress.IsEmpty() {
		expiry := getConfigDurationBlocks(ctx, k, constants.Deposit_PowExpiryMinutes)
		if expiry <= 0 || existing.CreatedHeight+expiry >= ctx.BlockHeight() {
			return types.DepositSession{}, fmt.Errorf("deposit pow token already used")
		}
	}

	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.DepositSession{}, err
	}
	pathType := common.VaultDepositPathUser
	depositNonce, pathIndex, err := k.AllocateVaultDepositPathIndex(ctx, vault.PubKey, pathType)
	if err != nil {
		return types.DepositSession{}, err
	}
	address, err := vault.DeriveBTCAddress(pathIndex)
	if err != nil {
		return types.DepositSession{}, err
	}
	path := common.VaultDepositPath(pathType, depositNonce, common.DepositPathCommitmentRoot)
	expiresAtHeight := nextDepositAddressExpiryHeight(ctx, k)
	purgeAtHeight := depositAddressPurgeHeight(ctx, k, ctx.BlockHeight())

	session := types.DepositSession{
		Owner:            owner,
		PowToken:         powToken,
		DepositAddress:   address,
		VaultPubKey:      vault.PubKey,
		DepositPathIndex: pathIndex,
		DepositPath:      path,
		DepositPathType:  string(pathType),
		DepositNonce:     depositNonce,
		CreatedHeight:    ctx.BlockHeight(),
		ExpiresAtHeight:  expiresAtHeight,
		PurgeAtHeight:    purgeAtHeight,
		PowDurationMs:    powDurationMs,
		PowDifficulty:    powDifficulty,
		Status:           types.DepositStatusAddressIssued,
	}
	mapping := types.DepositAddress{
		Address:         address,
		VaultPubKey:     vault.PubKey,
		PathIndex:       pathIndex,
		Path:            path,
		PathType:        string(pathType),
		DepositNonce:    depositNonce,
		Owner:           owner,
		PowToken:        powToken,
		CreatedHeight:   ctx.BlockHeight(),
		ExpiresAtHeight: expiresAtHeight,
		PurgeAtHeight:   purgeAtHeight,
		PowDurationMs:   powDurationMs,
		PowDifficulty:   powDifficulty,
	}
	if err := k.SetDepositAddress(ctx, mapping); err != nil {
		return types.DepositSession{}, err
	}
	if err := k.SetDepositPowTiming(ctx, types.DepositPowTiming{
		PowToken:      powToken,
		Owner:         owner,
		DurationMs:    powDurationMs,
		Difficulty:    powDifficulty,
		CreatedHeight: ctx.BlockHeight(),
	}); err != nil {
		return types.DepositSession{}, err
	}
	return session, k.SetDepositSession(ctx, session)
}

func MatchCoreDeposit(ctx cosmos.Context, mgr Manager, tx ObservedTx) (types.DepositRecord, error) {
	k := mgr.Keeper()
	if tx.Tx.ID.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("missing deposit tx id")
	}
	if existing, err := k.GetDepositRecord(ctx, tx.Tx.ID); err == nil && !existing.DepositID.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("deposit tx already matched")
	}
	if !tx.Tx.Chain.Equals(common.BTCChain) {
		return types.DepositRecord{}, fmt.Errorf("deposit must be bitcoin")
	}
	coin := tx.Tx.Coins.GetCoin(common.BTCAsset)
	if coin.IsEmpty() || coin.Amount.IsZero() {
		return types.DepositRecord{}, fmt.Errorf("missing bitcoin deposit amount")
	}

	mapping, err := k.GetDepositAddress(ctx, tx.Tx.ToAddress)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if mapping.VaultPubKey.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("deposit address not registered")
	}
	if mapping.PurgeAtHeight > 0 && ctx.BlockHeight() >= mapping.PurgeAtHeight {
		return types.DepositRecord{}, fmt.Errorf("deposit address purged")
	}
	if minDeposit := uint64(k.GetConfigInt64(ctx, constants.Deposit_AmountMinSats)); coin.Amount.Uint64() < minDeposit {
		return types.DepositRecord{}, fmt.Errorf("deposit below dust threshold: %d/%d", coin.Amount.Uint64(), minDeposit)
	}
	confirmationRequired := k.GetConfigInt64(ctx, constants.BTC_ConfirmationsMin)
	if confirmationRequired <= 0 {
		confirmationRequired = 1
	}

	deposit := types.DepositRecord{
		DepositID:                tx.Tx.ID,
		Owner:                    mapping.Owner,
		AmountSats:               coin.Amount.Uint64(),
		DepositAddress:           tx.Tx.ToAddress,
		ReturnAddress:            tx.Tx.FromAddress,
		VaultPubKey:              mapping.VaultPubKey,
		DepositPathIndex:         mapping.PathIndex,
		DepositPath:              mapping.Path,
		DepositPathType:          mapping.PathType,
		DepositNonce:             mapping.DepositNonce,
		OperatorPubKey:           mapping.OperatorPubKey,
		NodePubKey:               mapping.NodePubKey,
		AuctionID:                mapping.AuctionID,
		MatchedHeight:            ctx.BlockHeight(),
		CreatedHeight:            mapping.CreatedHeight,
		ExpiresAtHeight:          mapping.ExpiresAtHeight,
		PurgeAtHeight:            mapping.PurgeAtHeight,
		PowDurationMs:            mapping.PowDurationMs,
		PowDifficulty:            mapping.PowDifficulty,
		Status:                   types.DepositStatusDepositMatched,
		InboundTxID:              tx.Tx.ID,
		SourceVout:               tx.Tx.SourceVout,
		BTCConfirmations:         confirmationRequired,
		BTCConfirmationsRequired: confirmationRequired,
		BTCObservedHeight:        tx.BlockHeight,
	}
	if depositMatchedAfterAddressExpiry(deposit) {
		deposit.RefundEligibleHeight = depositRefundEligibleHeight(ctx, k, mapping.ExpiresAtHeight)
	}
	if err := k.SetDepositRecord(ctx, deposit); err != nil {
		return types.DepositRecord{}, err
	}
	if timing, err := k.GetDepositPowTiming(ctx, mapping.PowToken); err == nil && timing.PowToken != "" {
		timing.DepositID = tx.Tx.ID
		timing.DepositAmountSats = coin.Amount.Uint64()
		timing.MatchedHeight = ctx.BlockHeight()
		timing.Deposited = true
		if err := k.SetDepositPowTiming(ctx, timing); err != nil {
			return types.DepositRecord{}, err
		}
	}

	if session, err := k.GetDepositSession(ctx, mapping.Owner); err == nil && tx.Tx.ToAddress.Equals(session.DepositAddress) {
		session.DepositID = tx.Tx.ID
		session.RefundEligibleHeight = deposit.RefundEligibleHeight
		session.InboundTxID = tx.Tx.ID
		session.SourceVout = tx.Tx.SourceVout
		session.BTCConfirmations = confirmationRequired
		session.BTCConfirmationsRequired = confirmationRequired
		session.BTCObservedHeight = tx.BlockHeight
		session.Status = types.DepositStatusDepositMatched
		if err := k.SetDepositSession(ctx, session); err != nil {
			return types.DepositRecord{}, err
		}
	}
	if err := queueVaultPathSweep(ctx, mgr, tx, mapping.VaultPubKey, mapping.PathIndex); err != nil {
		return types.DepositRecord{}, err
	}

	return deposit, nil
}

func RecordDepositObservation(ctx cosmos.Context, k keeper.Keeper, tx ObservedTx, finalised bool) error {
	if !tx.Tx.Chain.Equals(common.BTCChain) || tx.Tx.ID.IsEmpty() || tx.Tx.ToAddress.IsEmpty() {
		return nil
	}
	mapping, err := k.GetDepositAddress(ctx, tx.Tx.ToAddress)
	if err != nil || mapping.VaultPubKey.IsEmpty() {
		return nil
	}
	required := int64(0)
	if !finalised && tx.BlockHeight > 0 && tx.FinaliseHeight >= tx.BlockHeight {
		required = tx.FinaliseHeight - tx.BlockHeight + 1
	}
	if required <= 0 {
		required = k.GetConfigInt64(ctx, constants.BTC_ConfirmationsMin)
	}
	if required <= 0 {
		required = 1
	}

	if session, err := k.GetDepositSession(ctx, mapping.Owner); err == nil && tx.Tx.ToAddress.Equals(session.DepositAddress) {
		sessionRequired := required
		if session.BTCConfirmationsRequired > 0 {
			sessionRequired = session.BTCConfirmationsRequired
		}
		session.InboundTxID = tx.Tx.ID
		session.SourceVout = tx.Tx.SourceVout
		session.BTCObservedHeight = tx.BlockHeight
		session.BTCConfirmations = depositConfirmationProgress(finalised, tx.BlockHeight, sessionRequired)
		session.BTCConfirmationsRequired = sessionRequired
		if !finalised && session.Status == types.DepositStatusAddressIssued {
			session.Status = types.DepositStatusDepositObserved
		}
		if err := k.SetDepositSession(ctx, session); err != nil {
			return err
		}
	}

	if deposit, err := k.GetDepositRecord(ctx, tx.Tx.ID); err == nil && !deposit.DepositID.IsEmpty() {
		depositRequired := required
		if deposit.BTCConfirmationsRequired > 0 {
			depositRequired = deposit.BTCConfirmationsRequired
		}
		deposit.InboundTxID = tx.Tx.ID
		deposit.SourceVout = tx.Tx.SourceVout
		deposit.BTCObservedHeight = tx.BlockHeight
		deposit.BTCConfirmations = depositConfirmationProgress(finalised, tx.BlockHeight, depositRequired)
		deposit.BTCConfirmationsRequired = depositRequired
		if err := k.SetDepositRecord(ctx, deposit); err != nil {
			return err
		}
	}
	return nil
}

func depositConfirmationProgress(finalised bool, blockHeight, required int64) int64 {
	current := int64(0)
	switch {
	case finalised:
		current = required
	case blockHeight > 0:
		current = 1
	}
	if current > required {
		return required
	}
	return current
}

func ProcessExpiredDepositAddressReturns(ctx cosmos.Context, mgr Manager) error {
	k := mgr.Keeper()
	iter := k.GetDepositRecordIterator(ctx)
	deposits := make([]types.DepositRecord, 0)

	for ; iter.Valid(); iter.Next() {
		var deposit types.DepositRecord
		if err := json.Unmarshal(iter.Value(), &deposit); err != nil {
			ctx.Logger().Error("fail to unmarshal deposit record", "error", err)
			continue
		}
		deposits = append(deposits, deposit)
	}
	if err := iter.Error(); shouldLogIteratorError(err) {
		ctx.Logger().Error("deposit record iterator ended with error", "error", err)
	}
	if err := iter.Close(); shouldLogIteratorError(err) {
		ctx.Logger().Error("fail to close deposit record iterator", "error", err)
	}

	expiredDeposits := make([]types.DepositRecord, 0)

	for _, deposit := range deposits {
		if deposit.Status == types.DepositStatusErrata {
			if err := purgeTxOutItemsForDeposit(ctx, k, deposit.DepositID, types.TxOutTypeSweep, types.TxOutTypeRefund); err != nil {
				ctx.Logger().Error("fail to purge errata deposit txout items", "error", err, "deposit_id", deposit.DepositID)
			}
			continue
		}
		if deposit.Status == types.DepositStatusReturnQueued && !depositMatchedAfterAddressExpiry(deposit) {
			if depositRefundTxOutSigned(ctx, k, deposit.DepositID) {
				ctx.Logger().Error("active-address deposit refund already signed before cleanup",
					"deposit_id", deposit.DepositID.String(),
				)
				continue
			}
			if err := purgeRefundTxOutItems(ctx, k, deposit.DepositID); err != nil {
				ctx.Logger().Error("fail to purge active-address deposit refund txout", "error", err, "deposit_id", deposit.DepositID.String())
				continue
			}
			deposit.Status = types.DepositStatusDepositMatched
			deposit.RefundEligibleHeight = 0
			deposit.RefundQueuedHeight = 0
			if err := k.SetDepositRecord(ctx, deposit); err != nil {
				ctx.Logger().Error("fail to reset active-address deposit refund", "error", err, "deposit_id", deposit.DepositID.String())
			}
			continue
		}
		if deposit.Status == types.DepositStatusReturnQueued && !depositSweepCompleted(ctx, k, deposit) {
			if _, err := purgeRefundTxOutItemAtHeight(ctx, k, deposit.RefundQueuedHeight, deposit.DepositID); err != nil {
				ctx.Logger().Error("fail to purge premature deposit refund txout", "error", err, "deposit_id", deposit.DepositID.String())
				continue
			}
			deposit.Status = types.DepositStatusDepositMatched
			deposit.RefundQueuedHeight = 0
			if err := k.SetDepositRecord(ctx, deposit); err != nil {
				ctx.Logger().Error("fail to reset premature deposit refund", "error", err, "deposit_id", deposit.DepositID.String())
			}
		}
		if deposit.Status != types.DepositStatusDepositMatched {
			continue
		}
		if !depositSweepCompleted(ctx, k, deposit) {
			if purged, err := purgeLegacyRefundTxOutForDeposit(ctx, k, deposit); err != nil {
				ctx.Logger().Debug("legacy premature refund cleanup did not complete", "error", err, "deposit_id", deposit.DepositID.String())
			} else if purged {
				ctx.Logger().Info("purged legacy premature deposit refund txout", "deposit_id", deposit.DepositID.String())
			}
		}
		if deposit.Settlement != "" {
			continue
		}
		if !depositMatchedAfterAddressExpiry(deposit) {
			continue
		}
		if deposit.RefundEligibleHeight <= 0 || deposit.RefundEligibleHeight > ctx.BlockHeight() {
			continue
		}
		if !depositSweepCompleted(ctx, k, deposit) {
			ctx.Logger().Info("expired deposit refund waiting for completed sweep",
				"deposit_id", deposit.DepositID.String(),
				"path_index", deposit.DepositPathIndex,
			)
			continue
		}
		expiredDeposits = append(expiredDeposits, deposit)
	}

	for _, deposit := range expiredDeposits {
		if err := queueExpiredDepositReturn(ctx, mgr, deposit); err != nil {
			ctx.Logger().Error("fail to return expired deposit", "error", err, "deposit_id", deposit.DepositID.String())
			continue
		}
		deposit.Status = types.DepositStatusReturnQueued
		deposit.RefundQueuedHeight = ctx.BlockHeight()
		if err := k.SetDepositRecord(ctx, deposit); err != nil {
			ctx.Logger().Error("fail to mark expired deposit return queued", "error", err, "deposit_id", deposit.DepositID.String())
		}
	}
	return nil
}

func depositMatchedAfterAddressExpiry(deposit types.DepositRecord) bool {
	return deposit.ExpiresAtHeight > 0 &&
		deposit.MatchedHeight > 0 &&
		deposit.MatchedHeight >= deposit.ExpiresAtHeight
}

func depositRefundTxOutSigned(ctx cosmos.Context, k keeper.Keeper, depositID common.TxID) bool {
	if depositID.IsEmpty() {
		return false
	}
	iter := k.GetTxOutIterator(ctx)
	defer func() {
		if err := iter.Close(); shouldLogIteratorError(err) {
			ctx.Logger().Error("fail to close txout iterator", "error", err)
		}
	}()
	for ; iter.Valid(); iter.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iter.Value(), &txOut); err != nil {
			ctx.Logger().Error("fail to unmarshal txout", "error", err)
			continue
		}
		for _, item := range txOut.TxArray {
			if item.InHash.Equals(depositID) &&
				item.GetTxType() == types.TxOutTypeRefund &&
				!item.OutHash.IsEmpty() {
				return true
			}
		}
	}
	if err := iter.Error(); shouldLogIteratorError(err) {
		ctx.Logger().Error("txout iterator ended with error", "error", err)
	}
	return false
}

func depositSweepCompleted(ctx cosmos.Context, k keeper.Keeper, deposit types.DepositRecord) bool {
	if deposit.DepositID.IsEmpty() {
		return false
	}
	if deposit.SweepComplete {
		return true
	}
	if deposit.DepositPathIndex == common.MainVaultPathIndex {
		return true
	}
	if deposit.MatchedHeight <= 0 {
		return false
	}
	return txOutHasCompletedDepositSweepAtOrAfterMatchedHeight(ctx, k, deposit)
}

func txOutHasCompletedDepositSweepAtOrAfterMatchedHeight(ctx cosmos.Context, k keeper.Keeper, deposit types.DepositRecord) bool {
	if deposit.MatchedHeight <= 0 || deposit.MatchedHeight > ctx.BlockHeight() {
		return false
	}
	iter := k.GetTxOutIterator(ctx)
	defer func() {
		if err := iter.Close(); shouldLogIteratorError(err) {
			ctx.Logger().Error("fail to close txout iterator", "error", err)
		}
	}()

	for ; iter.Valid(); iter.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iter.Value(), &txOut); err != nil {
			ctx.Logger().Error("fail to unmarshal txout", "error", err)
			continue
		}
		if txOut.Height < deposit.MatchedHeight || txOut.Height > ctx.BlockHeight() {
			continue
		}
		if txOutHasCompletedDepositSweep(&txOut, deposit) {
			return true
		}
	}
	if err := iter.Error(); shouldLogIteratorError(err) {
		ctx.Logger().Error("txout iterator ended with error", "error", err)
	}
	return false
}

func txOutHasCompletedDepositSweep(txOut *TxOut, deposit types.DepositRecord) bool {
	if txOut == nil {
		return false
	}
	for _, item := range txOut.TxArray {
		if item.InHash.Equals(deposit.DepositID) &&
			item.GetTxType() == types.TxOutTypeSweep &&
			item.VaultPubKey.Equals(deposit.VaultPubKey) &&
			item.VaultPathIndex == deposit.DepositPathIndex &&
			!item.OutHash.IsEmpty() {
			return true
		}
	}
	return false
}

func markDepositSweepComplete(ctx cosmos.Context, mgr Manager, item TxOutItem, _ ObservedTx) {
	if item.TxType != types.TxOutTypeSweep || item.InHash.IsEmpty() {
		return
	}
	deposit, err := mgr.Keeper().GetDepositRecord(ctx, item.InHash)
	if err != nil {
		ctx.Logger().Error("fail to get swept deposit record", "error", err, "deposit_id", item.InHash.String())
		return
	}
	if deposit.DepositID.IsEmpty() ||
		!deposit.VaultPubKey.Equals(item.VaultPubKey) ||
		deposit.DepositPathIndex != item.VaultPathIndex {
		ctx.Logger().Error("observed sweep does not match deposit record",
			"deposit_id", item.InHash.String(),
			"item_pubkey", item.VaultPubKey.String(),
			"deposit_pubkey", deposit.VaultPubKey.String(),
			"item_path", item.VaultPathIndex,
			"deposit_path", deposit.DepositPathIndex,
		)
		return
	}
	deposit.SweepComplete = true
	if err := mgr.Keeper().SetDepositRecord(ctx, deposit); err != nil {
		ctx.Logger().Error("fail to mark deposit sweep complete", "error", err, "deposit_id", item.InHash.String())
	}
}

func queueExpiredDepositReturn(ctx cosmos.Context, mgr Manager, deposit types.DepositRecord) error {
	if deposit.DepositID.IsEmpty() {
		return fmt.Errorf("missing deposit id")
	}
	if deposit.ReturnAddress.IsEmpty() {
		return fmt.Errorf("missing deposit return address")
	}
	vault, _, err := currentBTCVaultAddress(ctx, mgr.Keeper())
	if err != nil {
		return fmt.Errorf("fail to get base vault for deposit return: %w", err)
	}
	feeSats := withdrawalFeeSats(ctx, mgr.Keeper(), deposit.AmountSats)
	if feeSats >= deposit.AmountSats {
		if err := addWithdrawalFee(ctx, mgr.Keeper(), deposit.AmountSats); err != nil {
			return err
		}
		ctx.Logger().Info("expired deposit return consumed by fee",
			"deposit_id", deposit.DepositID.String(),
			"amount", deposit.AmountSats,
			"fee", feeSats,
		)
		return nil
	}
	amount := cosmos.NewUint(deposit.AmountSats - feeSats)
	gasRate := mgr.Keeper().GetConfigInt64(ctx, constants.BTC_DefaultSatsPerVByte)
	if nf, err := mgr.Keeper().GetNetworkFee(ctx, common.BTCChain); err == nil && nf.TransactionFeeRate > 0 {
		gasRate = int64(nf.TransactionFeeRate)
	}
	item := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      deposit.ReturnAddress,
		VaultPubKey:    vault.PubKey,
		VaultPathIndex: common.MainVaultPathIndex,
		Coin:           common.NewCoin(common.BTCAsset, amount),
		GasRate:        gasRate,
		InHash:         deposit.DepositID,
		ModuleName:     BaseName,
		TxType:         types.TxOutTypeRefund,
	}
	ctx.Logger().Info("queued expired deposit return",
		"deposit_id", deposit.DepositID.String(),
		"to", deposit.ReturnAddress.String(),
		"amount", amount.String(),
		"fee", feeSats,
	)
	if err := mgr.Keeper().AppendTxOut(ctx, ctx.BlockHeight(), item); err != nil {
		return fmt.Errorf("fail to queue deposit return txout: %w", err)
	}
	return addWithdrawalFee(ctx, mgr.Keeper(), feeSats)
}

func purgeRefundTxOutItemAtHeight(ctx cosmos.Context, k keeper.Keeper, height int64, depositID common.TxID) (bool, error) {
	if height <= 0 || depositID.IsEmpty() {
		return false, nil
	}
	var lastErr error
	for h := height - 100; h <= height+100; h++ {
		if h <= 0 {
			continue
		}
		changed, err := purgeRefundTxOutItemExactHeight(ctx, k, h, depositID)
		if err != nil {
			lastErr = err
			continue
		}
		if changed {
			return true, nil
		}
	}
	return false, lastErr
}

func purgeRefundTxOutItemExactHeight(ctx cosmos.Context, k keeper.Keeper, height int64, depositID common.TxID) (bool, error) {
	txOut, err := k.GetTxOut(ctx, height)
	if err != nil {
		return false, err
	}
	filtered := txOut.TxArray[:0]
	changed := false
	for _, item := range txOut.TxArray {
		if item.InHash.Equals(depositID) && item.GetTxType() == types.TxOutTypeRefund {
			changed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !changed {
		return false, nil
	}
	txOut.TxArray = filtered
	if len(txOut.TxArray) == 0 {
		return true, k.ClearTxOut(ctx, height)
	}
	return true, k.SetTxOut(ctx, txOut)
}

func purgeLegacyRefundTxOutForDeposit(ctx cosmos.Context, k keeper.Keeper, deposit types.DepositRecord) (bool, error) {
	if deposit.DepositID.IsEmpty() {
		return false, nil
	}
	start := deposit.RefundEligibleHeight - 200
	if start < deposit.MatchedHeight {
		start = deposit.MatchedHeight
	}
	if start < 1 {
		start = 1
	}
	var lastErr error
	for height := start; height <= ctx.BlockHeight(); height++ {
		changed, err := purgeRefundTxOutItemExactHeight(ctx, k, height, deposit.DepositID)
		if err != nil {
			lastErr = err
			continue
		}
		if changed {
			return true, nil
		}
	}
	return false, lastErr
}

func nextDepositAddressExpiryHeight(ctx cosmos.Context, k keeper.Keeper) int64 {
	return nextChurnHeightAfter(ctx, k, ctx.BlockHeight())
}

func depositAddressPurgeHeight(ctx cosmos.Context, k keeper.Keeper, createdHeight int64) int64 {
	days := k.GetConfigInt64(ctx, constants.Deposit_RefundIfForgottenDays)
	if days <= 0 {
		return 0
	}
	blocks := constants.MinutesToBlocks(days*24*60, k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds))
	if blocks <= 0 {
		return 0
	}
	return createdHeight + blocks
}

func depositRefundEligibleHeight(ctx cosmos.Context, k keeper.Keeper, expiresAtHeight int64) int64 {
	if expiresAtHeight > ctx.BlockHeight() {
		return nextChurnHeightAfter(ctx, k, expiresAtHeight)
	}
	return nextChurnHeightAfter(ctx, k, ctx.BlockHeight())
}

func nextChurnHeightAfter(ctx cosmos.Context, k keeper.Keeper, height int64) int64 {
	interval := getConfigDurationBlocks(ctx, k, constants.Churn_IntervalMinutes)
	if interval <= 0 {
		return 0
	}
	next := getLastChurnHeight(ctx, k) + interval
	for next <= height {
		next += interval
	}
	return next
}

func purgeExpiredDepositAddresses(ctx cosmos.Context, mgr Manager) error {
	k := mgr.Keeper()
	iter := k.GetDepositAddressIterator(ctx)
	expired := make([]types.DepositAddress, 0)

	for ; iter.Valid(); iter.Next() {
		var mapping types.DepositAddress
		if err := json.Unmarshal(iter.Value(), &mapping); err != nil {
			ctx.Logger().Error("fail to unmarshal deposit address", "error", err)
			continue
		}
		if mapping.PurgeAtHeight <= 0 || mapping.PurgeAtHeight > ctx.BlockHeight() {
			continue
		}
		expired = append(expired, mapping)
	}
	if err := iter.Error(); shouldLogIteratorError(err) {
		ctx.Logger().Error("deposit address iterator ended with error", "error", err)
	}
	if err := iter.Close(); shouldLogIteratorError(err) {
		ctx.Logger().Error("fail to close deposit address iterator", "error", err)
	}

	for _, mapping := range expired {
		if err := k.DeleteDepositAddress(ctx, mapping.Address); err != nil {
			ctx.Logger().Error("fail to purge deposit address", "error", err, "address", mapping.Address.String())
		}
		if err := purgeRefundTxOutItemsForAddress(ctx, k, mapping.Address); err != nil {
			ctx.Logger().Error("fail to purge stale deposit refund txout", "error", err, "address", mapping.Address.String())
		}
	}
	return nil
}

func purgeRefundTxOutItemsForAddress(ctx cosmos.Context, k keeper.Keeper, address common.Address) error {
	iter := k.GetDepositRecordIterator(ctx)
	depositIDs := make([]common.TxID, 0)

	for ; iter.Valid(); iter.Next() {
		var deposit types.DepositRecord
		if err := json.Unmarshal(iter.Value(), &deposit); err != nil {
			ctx.Logger().Error("fail to unmarshal deposit record", "error", err)
			continue
		}
		if deposit.DepositID.IsEmpty() || !deposit.DepositAddress.Equals(address) {
			continue
		}
		depositIDs = append(depositIDs, deposit.DepositID)
	}
	if err := iter.Error(); shouldLogIteratorError(err) {
		ctx.Logger().Error("deposit record iterator ended with error", "error", err)
	}
	if err := iter.Close(); shouldLogIteratorError(err) {
		ctx.Logger().Error("fail to close deposit record iterator", "error", err)
	}

	for _, depositID := range depositIDs {
		if err := purgeRefundTxOutItems(ctx, k, depositID); err != nil {
			return err
		}
	}
	return nil
}

func purgeRefundTxOutItems(ctx cosmos.Context, k keeper.Keeper, depositID common.TxID) error {
	return purgeTxOutItemsForDeposit(ctx, k, depositID, types.TxOutTypeRefund)
}

func purgeTxOutItemsForDeposit(ctx cosmos.Context, k keeper.Keeper, depositID common.TxID, txTypes ...string) error {
	typeSet := make(map[string]struct{}, len(txTypes))
	for _, txType := range txTypes {
		typeSet[txType] = struct{}{}
	}
	iter := k.GetTxOutIterator(ctx)
	updates := make([]TxOut, 0)
	clears := make([]int64, 0)

	for ; iter.Valid(); iter.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iter.Value(), &txOut); err != nil {
			ctx.Logger().Error("fail to unmarshal txout", "error", err)
			continue
		}
		filtered := txOut.TxArray[:0]
		changed := false
		for _, item := range txOut.TxArray {
			if item.InHash.Equals(depositID) {
				if _, ok := typeSet[item.GetTxType()]; ok {
					changed = true
					continue
				}
			}
			filtered = append(filtered, item)
		}
		if !changed {
			continue
		}
		txOut.TxArray = filtered
		if len(txOut.TxArray) == 0 {
			clears = append(clears, txOut.Height)
			continue
		}
		updates = append(updates, txOut)
	}
	if err := iter.Error(); shouldLogIteratorError(err) {
		ctx.Logger().Error("txout iterator ended with error", "error", err)
	}
	if err := iter.Close(); shouldLogIteratorError(err) {
		ctx.Logger().Error("fail to close txout iterator", "error", err)
	}

	for _, height := range clears {
		if err := k.ClearTxOut(ctx, height); err != nil {
			return err
		}
	}
	for _, txOut := range updates {
		if err := k.SetTxOut(ctx, &txOut); err != nil {
			return err
		}
	}
	return nil
}
