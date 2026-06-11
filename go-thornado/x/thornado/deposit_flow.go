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

func RegisterDepositPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken, operatorPubKey, nodePubKey string, powDurationMs uint64) (types.DepositSession, error) {
	return registerDepositPowToken(ctx, k, owner, powToken, operatorPubKey, nodePubKey, "", powDurationMs)
}

func RegisterNodeSlotBidDepositPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken, auctionID, operatorPubKey, nodePubKey string, powDurationMs uint64) (types.DepositSession, types.NodeSlotBid, error) {
	auctionID = strings.TrimSpace(auctionID)
	if auctionID == "" {
		return types.DepositSession{}, types.NodeSlotBid{}, fmt.Errorf("missing node slot auction id")
	}
	auction, err := k.GetNodeSlotAuction(ctx, auctionID)
	if err != nil {
		return types.DepositSession{}, types.NodeSlotBid{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionOpen {
		return types.DepositSession{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction is not open")
	}
	if auction.ExpiryHeight <= ctx.BlockHeight() {
		return types.DepositSession{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction expired")
	}
	session, err := registerDepositPowToken(ctx, k, owner, powToken, operatorPubKey, nodePubKey, auctionID, powDurationMs)
	if err != nil {
		return types.DepositSession{}, types.NodeSlotBid{}, err
	}
	bidID := nodeSlotBidID(auctionID, owner, session.DepositPathIndex)
	bid := types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auctionID,
		Bidder:         owner,
		OperatorPubKey: session.OperatorPubKey,
		NodePubKey:     session.NodePubKey,
		DepositAddress: session.DepositAddress,
		CreatedHeight:  ctx.BlockHeight(),
		UpdatedHeight:  ctx.BlockHeight(),
	}
	if err := k.SetNodeSlotBid(ctx, bid); err != nil {
		return types.DepositSession{}, types.NodeSlotBid{}, err
	}
	return session, bid, nil
}

func registerDepositPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken, operatorPubKey, nodePubKey, auctionID string, powDurationMs uint64) (types.DepositSession, error) {
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
	operatorPubKey = strings.TrimSpace(operatorPubKey)
	nodePubKey = strings.TrimSpace(nodePubKey)
	auctionID = strings.TrimSpace(auctionID)
	var operator common.PubKey
	if operatorPubKey != "" {
		var err error
		operator, err = common.NewPubKey(operatorPubKey)
		if err != nil {
			return types.DepositSession{}, fmt.Errorf("invalid operator pubkey: %w", err)
		}
	}
	if nodePubKey != "" {
		if operator.IsEmpty() {
			return types.DepositSession{}, fmt.Errorf("bond deposits require operator pubkey")
		}
		if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, nodePubKey); err != nil {
			return types.DepositSession{}, fmt.Errorf("invalid node pubkey: %w", err)
		}
		if auctionID == "" {
			return types.DepositSession{}, fmt.Errorf("node bonds activate via MsgBondFromNotes from shielded notes")
		}
	}
	if auctionID != "" && nodePubKey == "" {
		return types.DepositSession{}, fmt.Errorf("node slot auction bids require node pubkey")
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
	if nodePubKey != "" {
		pathType = common.VaultDepositPathNode
	}
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
		OperatorPubKey:   operator,
		NodePubKey:       nodePubKey,
		AuctionID:        auctionID,
		CreatedHeight:    ctx.BlockHeight(),
		ExpiresAtHeight:  expiresAtHeight,
		PurgeAtHeight:    purgeAtHeight,
		PowDurationMs:    powDurationMs,
		PowDifficulty: powDifficulty,
		Status:        types.DepositStatusAddressIssued,
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
		OperatorPubKey:  operator,
		NodePubKey:      nodePubKey,
		AuctionID:       auctionID,
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
		RefundEligibleHeight:     depositRefundEligibleHeight(ctx, k, mapping.ExpiresAtHeight),
		PowDurationMs:            mapping.PowDurationMs,
		PowDifficulty:            mapping.PowDifficulty,
		Status:                   types.DepositStatusDepositMatched,
		InboundTxID:              tx.Tx.ID,
		BTCConfirmations:         confirmationRequired,
		BTCConfirmationsRequired: confirmationRequired,
		BTCObservedHeight:        tx.BlockHeight,
	}
	if mapping.AuctionID != "" {
		bidID := nodeSlotBidID(mapping.AuctionID, mapping.Owner, mapping.PathIndex)
		bid, err := k.GetNodeSlotBid(ctx, bidID)
		if err != nil {
			return types.DepositRecord{}, err
		}
		if bid.BidID == "" {
			return types.DepositRecord{}, fmt.Errorf("node slot bid not found")
		}
		bid.DepositID = tx.Tx.ID
		bid.AmountSats = coin.Amount.Uint64()
		bid.UpdatedHeight = ctx.BlockHeight()
		if err := k.SetNodeSlotBid(ctx, bid); err != nil {
			return types.DepositRecord{}, err
		}
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
	if tx.FinaliseHeight > tx.BlockHeight {
		required = tx.FinaliseHeight - tx.BlockHeight
	}
	if required <= 0 {
		required = k.GetConfigInt64(ctx, constants.BTC_ConfirmationsMin)
	}
	if required <= 0 {
		required = 1
	}
	current := int64(0)
	if finalised {
		current = required
	}

	if session, err := k.GetDepositSession(ctx, mapping.Owner); err == nil && tx.Tx.ToAddress.Equals(session.DepositAddress) {
		session.InboundTxID = tx.Tx.ID
		session.BTCObservedHeight = tx.BlockHeight
		session.BTCConfirmations = current
		session.BTCConfirmationsRequired = required
		if err := k.SetDepositSession(ctx, session); err != nil {
			return err
		}
	}

	if deposit, err := k.GetDepositRecord(ctx, tx.Tx.ID); err == nil && !deposit.DepositID.IsEmpty() {
		deposit.InboundTxID = tx.Tx.ID
		deposit.BTCObservedHeight = tx.BlockHeight
		deposit.BTCConfirmations = current
		deposit.BTCConfirmationsRequired = required
		if err := k.SetDepositRecord(ctx, deposit); err != nil {
			return err
		}
	}
	return nil
}

func ProcessExpiredDepositAddressReturns(ctx cosmos.Context, mgr Manager) error {
	k := mgr.Keeper()
	iter := k.GetDepositRecordIterator(ctx)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var deposit types.DepositRecord
		if err := json.Unmarshal(iter.Value(), &deposit); err != nil {
			ctx.Logger().Error("fail to unmarshal deposit record", "error", err)
			continue
		}
		if deposit.Status != types.DepositStatusDepositMatched {
			continue
		}
		if deposit.Settlement != "" {
			continue
		}
		if deposit.RefundEligibleHeight <= 0 || deposit.RefundEligibleHeight > ctx.BlockHeight() {
			continue
		}
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
	if err := iter.Error(); err != nil {
		return err
	}
	return purgeExpiredDepositAddresses(ctx, mgr)
}

func queueExpiredDepositReturn(ctx cosmos.Context, mgr Manager, deposit types.DepositRecord) error {
	if deposit.DepositID.IsEmpty() {
		return fmt.Errorf("missing deposit id")
	}
	if deposit.ReturnAddress.IsEmpty() {
		return fmt.Errorf("missing deposit return address")
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
	maxGasCoin, err := mgr.GasMgr().GetMaxGas(ctx, common.BTCChain)
	if err != nil {
		return fmt.Errorf("fail to get bitcoin return max gas: %w", err)
	}
	gasRate := mgr.Keeper().GetConfigInt64(ctx, constants.BTC_DefaultSatsPerVByte)
	if nf, err := mgr.Keeper().GetNetworkFee(ctx, common.BTCChain); err == nil && nf.TransactionFeeRate > 0 {
		gasRate = int64(nf.TransactionFeeRate)
	}
	item := TxOutItem{
		Chain:      common.BTCChain,
		ToAddress:  deposit.ReturnAddress,
		Coin:       common.NewCoin(common.BTCAsset, amount),
		MaxGas:     common.Gas{maxGasCoin},
		GasRate:    gasRate,
		InHash:     deposit.DepositID,
		ModuleName: BaseName,
		TxType:     types.TxOutTypeRefund,
	}
	ctx.Logger().Info("queued expired deposit return",
		"deposit_id", deposit.DepositID.String(),
		"to", deposit.ReturnAddress.String(),
		"amount", amount.String(),
		"fee", feeSats,
	)
	ok, err := mgr.TxOutStore().TryAddTxOutItem(ctx, mgr, item, cosmos.ZeroUint())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("deposit return was not queued")
	}
	return addWithdrawalFee(ctx, mgr.Keeper(), feeSats)
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
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var mapping types.DepositAddress
		if err := json.Unmarshal(iter.Value(), &mapping); err != nil {
			ctx.Logger().Error("fail to unmarshal deposit address", "error", err)
			continue
		}
		if mapping.PurgeAtHeight <= 0 || mapping.PurgeAtHeight > ctx.BlockHeight() {
			continue
		}
		if err := k.DeleteDepositAddress(ctx, mapping.Address); err != nil {
			ctx.Logger().Error("fail to purge deposit address", "error", err, "address", mapping.Address.String())
		}
		if err := purgeRefundTxOutItemsForAddress(ctx, k, mapping.Address); err != nil {
			ctx.Logger().Error("fail to purge stale deposit refund txout", "error", err, "address", mapping.Address.String())
		}
	}
	return iter.Error()
}

func purgeRefundTxOutItemsForAddress(ctx cosmos.Context, k keeper.Keeper, address common.Address) error {
	iter := k.GetDepositRecordIterator(ctx)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var deposit types.DepositRecord
		if err := json.Unmarshal(iter.Value(), &deposit); err != nil {
			ctx.Logger().Error("fail to unmarshal deposit record", "error", err)
			continue
		}
		if deposit.DepositID.IsEmpty() || !deposit.DepositAddress.Equals(address) {
			continue
		}
		if err := purgeRefundTxOutItems(ctx, k, deposit.DepositID); err != nil {
			return err
		}
	}
	return iter.Error()
}

func purgeRefundTxOutItems(ctx cosmos.Context, k keeper.Keeper, depositID common.TxID) error {
	iter := k.GetTxOutIterator(ctx)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var txOut TxOut
		if err := json.Unmarshal(iter.Value(), &txOut); err != nil {
			ctx.Logger().Error("fail to unmarshal txout", "error", err)
			continue
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
			continue
		}
		txOut.TxArray = filtered
		if len(txOut.TxArray) == 0 {
			if err := k.ClearTxOut(ctx, txOut.Height); err != nil {
				return err
			}
			continue
		}
		if err := k.SetTxOut(ctx, &txOut); err != nil {
			return err
		}
	}
	return iter.Error()
}
