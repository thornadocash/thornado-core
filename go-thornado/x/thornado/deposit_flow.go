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
		PowDurationMs:    powDurationMs,
		PowDifficulty:    powDifficulty,
		Status:           types.DepositStatusAddressIssued,
	}
	mapping := types.DepositAddress{
		Address:        address,
		VaultPubKey:    vault.PubKey,
		PathIndex:      pathIndex,
		Path:           path,
		PathType:       string(pathType),
		DepositNonce:   depositNonce,
		Owner:          owner,
		PowToken:       powToken,
		OperatorPubKey: operator,
		NodePubKey:     nodePubKey,
		AuctionID:      auctionID,
		CreatedHeight:  ctx.BlockHeight(),
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
	session, err := k.GetDepositSession(ctx, mapping.Owner)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if session.DepositAddress.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("deposit session not found")
	}
	if expiry := getConfigDurationBlocks(ctx, k, constants.Deposit_SessionExpiryMinutes); expiry > 0 && session.CreatedHeight+expiry < ctx.BlockHeight() {
		return types.DepositRecord{}, fmt.Errorf("deposit session expired")
	}
	if !tx.Tx.ToAddress.Equals(session.DepositAddress) || !mapping.VaultPubKey.Equals(session.VaultPubKey) || mapping.PathIndex != session.DepositPathIndex {
		return types.DepositRecord{}, fmt.Errorf("deposit mapping mismatch")
	}
	if minDeposit := uint64(k.GetConfigInt64(ctx, constants.Deposit_AmountMinSats)); coin.Amount.Uint64() < minDeposit {
		return types.DepositRecord{}, fmt.Errorf("deposit below dust threshold: %d/%d", coin.Amount.Uint64(), minDeposit)
	}

	deposit := types.DepositRecord{
		DepositID:        tx.Tx.ID,
		Owner:            session.Owner,
		AmountSats:       coin.Amount.Uint64(),
		DepositAddress:   tx.Tx.ToAddress,
		ReturnAddress:    tx.Tx.FromAddress,
		VaultPubKey:      session.VaultPubKey,
		DepositPathIndex: session.DepositPathIndex,
		DepositPath:      session.DepositPath,
		DepositPathType:  session.DepositPathType,
		DepositNonce:     session.DepositNonce,
		OperatorPubKey:   session.OperatorPubKey,
		NodePubKey:       session.NodePubKey,
		AuctionID:        session.AuctionID,
		MatchedHeight:    ctx.BlockHeight(),
		PowDurationMs:    session.PowDurationMs,
		PowDifficulty:    session.PowDifficulty,
		Status:           types.DepositStatusDepositMatched,
	}
	if session.AuctionID != "" {
		bidID := nodeSlotBidID(session.AuctionID, session.Owner, session.DepositPathIndex)
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
	if timing, err := k.GetDepositPowTiming(ctx, session.PowToken); err == nil && timing.PowToken != "" {
		timing.DepositID = tx.Tx.ID
		timing.DepositAmountSats = coin.Amount.Uint64()
		timing.MatchedHeight = ctx.BlockHeight()
		timing.Deposited = true
		if err := k.SetDepositPowTiming(ctx, timing); err != nil {
			return types.DepositRecord{}, err
		}
	}

	session.DepositID = tx.Tx.ID
	session.Status = types.DepositStatusDepositMatched
	if err := k.SetDepositSession(ctx, session); err != nil {
		return types.DepositRecord{}, err
	}
	if err := queueVaultPathSweep(ctx, mgr, tx, session.VaultPubKey, session.DepositPathIndex); err != nil {
		return types.DepositRecord{}, err
	}

	return deposit, nil
}

func ProcessForgottenDepositReturns(ctx cosmos.Context, mgr Manager) error {
	k := mgr.Keeper()
	days := k.GetConfigInt64(ctx, constants.Deposit_RefundIfForgottenDays)
	if days <= 0 {
		return nil
	}
	ageBlocks := constants.MinutesToBlocks(days*24*60, k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds))
	if ageBlocks <= 0 {
		return nil
	}

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
		if len(deposit.Commitments) != 0 || deposit.Settlement != "" {
			continue
		}
		if deposit.MatchedHeight <= 0 || deposit.MatchedHeight+ageBlocks > ctx.BlockHeight() {
			continue
		}
		if err := queueForgottenDepositReturn(ctx, mgr, deposit); err != nil {
			ctx.Logger().Error("fail to return forgotten deposit", "error", err, "deposit_id", deposit.DepositID.String())
			continue
		}
		deposit.Status = types.DepositStatusReturnQueued
		if err := k.SetDepositRecord(ctx, deposit); err != nil {
			ctx.Logger().Error("fail to mark forgotten deposit return queued", "error", err, "deposit_id", deposit.DepositID.String())
		}
	}
	return iter.Error()
}

func queueForgottenDepositReturn(ctx cosmos.Context, mgr Manager, deposit types.DepositRecord) error {
	if deposit.DepositID.IsEmpty() {
		return fmt.Errorf("missing deposit id")
	}
	if deposit.ReturnAddress.IsEmpty() {
		return fmt.Errorf("missing deposit return address")
	}
	amount := cosmos.NewUint(deposit.AmountSats)
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
		TxType:     types.TxOutTypeOut,
	}
	ctx.Logger().Info("queued forgotten deposit return",
		"deposit_id", deposit.DepositID.String(),
		"to", deposit.ReturnAddress.String(),
		"amount", amount.String(),
	)
	ok, err := mgr.TxOutStore().TryAddTxOutItem(ctx, mgr, item, cosmos.ZeroUint())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("deposit return was not queued")
	}
	return nil
}
