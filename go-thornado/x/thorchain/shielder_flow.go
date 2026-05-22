package thorchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/btcec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

const (
	BondStartAmountSats   uint64 = 100_000_000
	BondSlotIncrementSats uint64 = 100_000_000
	WithdrawalFeeBp       uint64 = 200
	feeShareScale         uint64 = 1_000_000_000_000
)

type ShielderWithdrawalRequest struct {
	Owner  cosmos.AccAddress
	Proof  json.RawMessage
	Public json.RawMessage
}

type shielderWithdrawalPublicInputs struct {
	NullifierHash    string `json:"nullifier_hash"`
	MerkleRoot       string `json:"merkle_root"`
	DenominationSats uint64 `json:"denomination_sats"`
	Recipient        string `json:"recipient"`
	FeeSats          uint64 `json:"fee_sats"`
}

type shielderNoteCommitment struct {
	DenominationSats uint64 `json:"denomination_sats"`
	Commitment       string `json:"commitment"`
}

func RegisterShielderPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken, operatorPubKey, validatorPubKey string) (types.ShielderSession, error) {
	return registerShielderPowToken(ctx, k, owner, powToken, operatorPubKey, validatorPubKey, "")
}

func RegisterNodeSlotBidPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken, auctionID, operatorPubKey, validatorPubKey string) (types.ShielderSession, types.NodeSlotBid, error) {
	auctionID = strings.TrimSpace(auctionID)
	if auctionID == "" {
		return types.ShielderSession{}, types.NodeSlotBid{}, fmt.Errorf("missing node slot auction id")
	}
	auction, err := k.GetNodeSlotAuction(ctx, auctionID)
	if err != nil {
		return types.ShielderSession{}, types.NodeSlotBid{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionOpen {
		return types.ShielderSession{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction is not open")
	}
	if auction.ExpiryHeight <= ctx.BlockHeight() {
		return types.ShielderSession{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction expired")
	}
	session, err := registerShielderPowToken(ctx, k, owner, powToken, operatorPubKey, validatorPubKey, auctionID)
	if err != nil {
		return types.ShielderSession{}, types.NodeSlotBid{}, err
	}
	bidID := nodeSlotBidID(auctionID, owner, session.DepositPathIndex)
	bid := types.NodeSlotBid{
		BidID:           bidID,
		AuctionID:       auctionID,
		Bidder:          owner,
		OperatorPubKey:  session.OperatorPubKey,
		ValidatorPubKey: session.ValidatorPubKey,
		CreatedHeight:   ctx.BlockHeight(),
		UpdatedHeight:   ctx.BlockHeight(),
	}
	if err := k.SetNodeSlotBid(ctx, bid); err != nil {
		return types.ShielderSession{}, types.NodeSlotBid{}, err
	}
	return session, bid, nil
}

func registerShielderPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken, operatorPubKey, validatorPubKey, auctionID string) (types.ShielderSession, error) {
	if owner.Empty() {
		return types.ShielderSession{}, fmt.Errorf("missing shielder owner")
	}
	powToken = strings.TrimSpace(powToken)
	if powToken == "" {
		return types.ShielderSession{}, fmt.Errorf("missing shielder pow token")
	}
	operatorPubKey = strings.TrimSpace(operatorPubKey)
	validatorPubKey = strings.TrimSpace(validatorPubKey)
	auctionID = strings.TrimSpace(auctionID)
	var operator common.PubKey
	if operatorPubKey != "" {
		var err error
		operator, err = common.NewPubKey(operatorPubKey)
		if err != nil {
			return types.ShielderSession{}, fmt.Errorf("invalid shielder operator pubkey: %w", err)
		}
	}
	if validatorPubKey != "" {
		if operator.IsEmpty() {
			return types.ShielderSession{}, fmt.Errorf("bond deposits require operator pubkey")
		}
		if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, validatorPubKey); err != nil {
			return types.ShielderSession{}, fmt.Errorf("invalid shielder validator pubkey: %w", err)
		}
	}
	if auctionID != "" && validatorPubKey == "" {
		return types.ShielderSession{}, fmt.Errorf("node slot auction bids require validator pubkey")
	}

	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.ShielderSession{}, err
	}
	pathIndex, err := k.AllocateVaultDepositPathIndex(ctx, vault.PubKey)
	if err != nil {
		return types.ShielderSession{}, err
	}
	address, err := vault.DeriveBTCAddress(pathIndex)
	if err != nil {
		return types.ShielderSession{}, err
	}

	session := types.ShielderSession{
		Owner:            owner,
		PowToken:         powToken,
		DepositAddress:   address,
		VaultPubKey:      vault.PubKey,
		DepositPathIndex: pathIndex,
		OperatorPubKey:   operator,
		ValidatorPubKey:  validatorPubKey,
		AuctionID:        auctionID,
		CreatedHeight:    ctx.BlockHeight(),
		Status:           types.ShielderStatusAddressIssued,
	}
	mapping := types.ShielderDepositAddress{
		Address:         address,
		VaultPubKey:     vault.PubKey,
		PathIndex:       pathIndex,
		Owner:           owner,
		PowToken:        powToken,
		OperatorPubKey:  operator,
		ValidatorPubKey: validatorPubKey,
		AuctionID:       auctionID,
		CreatedHeight:   ctx.BlockHeight(),
	}
	if err := k.SetShielderDepositAddress(ctx, mapping); err != nil {
		return types.ShielderSession{}, err
	}
	return session, k.SetShielderSession(ctx, session)
}

func MatchShielderDeposit(ctx cosmos.Context, mgr Manager, tx ObservedTx) (types.ShielderDeposit, error) {
	k := mgr.Keeper()
	if tx.Tx.ID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder deposit tx id")
	}
	if !tx.Tx.Chain.Equals(common.BTCChain) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit must be bitcoin")
	}
	coin := tx.Tx.Coins.GetCoin(common.BTCAsset)
	if coin.IsEmpty() || coin.Amount.IsZero() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder bitcoin deposit amount")
	}

	mapping, err := k.GetShielderDepositAddress(ctx, tx.Tx.ToAddress)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if mapping.VaultPubKey.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit address not registered")
	}
	session, err := k.GetShielderSession(ctx, mapping.Owner)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if session.DepositAddress.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder session not found")
	}
	if !tx.Tx.ToAddress.Equals(session.DepositAddress) || !mapping.VaultPubKey.Equals(session.VaultPubKey) || mapping.PathIndex != session.DepositPathIndex {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit mapping mismatch")
	}

	deposit := types.ShielderDeposit{
		DepositID:        tx.Tx.ID,
		Owner:            session.Owner,
		AmountSats:       coin.Amount.Uint64(),
		DepositAddress:   tx.Tx.ToAddress,
		VaultPubKey:      session.VaultPubKey,
		DepositPathIndex: session.DepositPathIndex,
		OperatorPubKey:   session.OperatorPubKey,
		ValidatorPubKey:  session.ValidatorPubKey,
		AuctionID:        session.AuctionID,
		MatchedHeight:    ctx.BlockHeight(),
		Status:           types.ShielderStatusDepositMatched,
	}
	if session.AuctionID != "" {
		bidID := nodeSlotBidID(session.AuctionID, session.Owner, session.DepositPathIndex)
		bid, err := k.GetNodeSlotBid(ctx, bidID)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		if bid.BidID == "" {
			return types.ShielderDeposit{}, fmt.Errorf("node slot bid not found")
		}
		bid.DepositID = tx.Tx.ID
		bid.AmountSats = coin.Amount.Uint64()
		bid.UpdatedHeight = ctx.BlockHeight()
		if err := k.SetNodeSlotBid(ctx, bid); err != nil {
			return types.ShielderDeposit{}, err
		}
	}
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}

	session.DepositID = tx.Tx.ID
	session.Status = types.ShielderStatusDepositMatched
	if err := k.SetShielderSession(ctx, session); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := queueVaultPathSweep(ctx, mgr, tx, session.VaultPubKey, session.DepositPathIndex); err != nil {
		return types.ShielderDeposit{}, err
	}

	return deposit, nil
}

func SettleShielderDeposit(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, depositID common.TxID) (types.ShielderDeposit, error) {
	if owner.Empty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder settlement owner")
	}
	deposit, err := k.GetShielderDeposit(ctx, depositID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if deposit.DepositID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit not found")
	}
	if !deposit.Owner.Equals(owner) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit owner mismatch")
	}
	if deposit.Status != types.ShielderStatusDepositMatched {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit is not confirm-settleable")
	}
	if deposit.AuctionID != "" {
		return types.ShielderDeposit{}, fmt.Errorf("node sale bid deposits settle through auction selection")
	}
	if deposit.IsValidatorBond() {
		nodeSlot, err := recordPendingShielderValidatorBond(ctx, k, types.ShielderSession{
			Owner:           deposit.Owner,
			OperatorPubKey:  deposit.OperatorPubKey,
			ValidatorPubKey: deposit.ValidatorPubKey,
		}, deposit.AmountSats)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		deposit.NodeSlot = nodeSlot
		deposit.Settlement = types.ShielderSettlementOperatorBond
	} else {
		deposit.Settlement = types.ShielderSettlementUser
	}
	deposit.Status = types.ShielderStatusSettled
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	return deposit, nil
}

func CreateNodeSlotAuction(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, validatorPubKey string, reserveSats uint64, expiryHeight int64) (types.NodeSlotAuction, error) {
	if seller.Empty() {
		return types.NodeSlotAuction{}, fmt.Errorf("missing node slot auction seller")
	}
	if expiryHeight <= ctx.BlockHeight() {
		return types.NodeSlotAuction{}, fmt.Errorf("node slot auction expiry must be in the future")
	}
	bond, err := k.GetShielderValidatorBond(ctx, validatorPubKey)
	if err != nil {
		return types.NodeSlotAuction{}, err
	}
	if bond.ValidatorPubKey == "" || bond.BondSats == 0 {
		return types.NodeSlotAuction{}, fmt.Errorf("validator has no bonded slot")
	}
	nodeAccount, err := k.GetNodeAccount(ctx, bond.NodeAddress)
	if err != nil {
		return types.NodeSlotAuction{}, err
	}
	if nodeAccount.BondAddress.String() != seller.String() {
		return types.NodeSlotAuction{}, fmt.Errorf("node slot auction seller mismatch")
	}
	if nodeAccount.Status != NodeStandby {
		return types.NodeSlotAuction{}, fmt.Errorf("node slot must be standby before auction")
	}
	auctionID := nodeSlotAuctionID(validatorPubKey, bond.Slot, ctx.BlockHeight())
	auction := types.NodeSlotAuction{
		AuctionID:             auctionID,
		Seller:                seller,
		SellerOperatorPubKey:  bond.OperatorPubKey,
		SellerValidatorPubKey: bond.ValidatorPubKey,
		Slot:                  bond.Slot,
		OriginalBondSats:      bond.BondSats,
		ReserveSats:           reserveSats,
		ExpiryHeight:          expiryHeight,
		Status:                types.NodeSlotAuctionOpen,
		CreatedHeight:         ctx.BlockHeight(),
		UpdatedHeight:         ctx.BlockHeight(),
	}
	return auction, k.SetNodeSlotAuction(ctx, auction)
}

func SelectNodeSlotBid(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, auctionID, bidID string) (types.NodeSlotAuction, types.NodeSlotBid, error) {
	auction, err := k.GetNodeSlotAuction(ctx, auctionID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionOpen {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction is not open")
	}
	if !auction.Seller.Equals(seller) {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction seller mismatch")
	}
	if auction.ExpiryHeight <= ctx.BlockHeight() {
		auction.Status = types.NodeSlotAuctionExpired
		auction.UpdatedHeight = ctx.BlockHeight()
		_ = k.SetNodeSlotAuction(ctx, auction)
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot auction expired")
	}
	bid, err := k.GetNodeSlotBid(ctx, bidID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	if bid.BidID == "" || bid.AuctionID != auction.AuctionID {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid not found for auction")
	}
	if bid.DepositID.IsEmpty() || bid.AmountSats == 0 {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid deposit not matched")
	}
	if bid.AmountSats < auction.ReserveSats {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid below reserve")
	}
	deposit, err := k.GetShielderDeposit(ctx, bid.DepositID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	if deposit.DepositID.IsEmpty() || deposit.Status != types.ShielderStatusDepositMatched {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid deposit is not settleable")
	}
	sellerPayout := auction.OriginalBondSats
	if bid.AmountSats < sellerPayout {
		sellerPayout = bid.AmountSats
	}
	deposit.Owner = seller
	deposit.Settlement = types.ShielderSettlementOperatorSale
	deposit.SellerPayoutSats = sellerPayout
	deposit.ProtocolBondSats = bid.AmountSats - sellerPayout
	deposit.Status = types.ShielderStatusSettled
	auction.SelectedBidID = bid.BidID
	auction.Status = types.NodeSlotAuctionSelected
	auction.UpdatedHeight = ctx.BlockHeight()
	bid.Selected = true
	bid.UpdatedHeight = ctx.BlockHeight()
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	if err := k.SetNodeSlotAuction(ctx, auction); err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	if err := k.SetNodeSlotBid(ctx, bid); err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	return auction, bid, nil
}

func SplitNodeSlotSale(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, auctionID, bidID string, sellerCommitments []string) (types.ShielderDeposit, error) {
	auction, err := k.GetNodeSlotAuction(ctx, auctionID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionSelected || auction.SelectedBidID != strings.TrimSpace(bidID) {
		return types.ShielderDeposit{}, fmt.Errorf("node slot auction bid not selected")
	}
	if !auction.Seller.Equals(seller) {
		return types.ShielderDeposit{}, fmt.Errorf("node slot auction seller mismatch")
	}
	bid, err := k.GetNodeSlotBid(ctx, bidID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if bid.BidID == "" || bid.Settled {
		return types.ShielderDeposit{}, fmt.Errorf("node slot bid is not settleable")
	}
	deposit, err := k.GetShielderDeposit(ctx, bid.DepositID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if deposit.DepositID.IsEmpty() || len(deposit.Commitments) > 0 {
		return types.ShielderDeposit{}, fmt.Errorf("node slot sale deposit is not splittable")
	}
	if deposit.Status != types.ShielderStatusSettled || deposit.Settlement != types.ShielderSettlementOperatorSale {
		return types.ShielderDeposit{}, fmt.Errorf("node slot sale deposit is not settled")
	}
	sellerNotes, err := parseShielderNoteCommitments(sellerCommitments, deposit.SellerPayoutSats, false)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	var sellerTotal uint64
	for _, note := range sellerNotes {
		sellerTotal += note.DenominationSats
	}
	if sellerTotal != deposit.SellerPayoutSats {
		return types.ShielderDeposit{}, fmt.Errorf("node slot seller commitments do not match payout amount")
	}
	allNotes := append([]shielderNoteCommitment{}, sellerNotes...)
	if deposit.ProtocolBondSats != 0 {
		allNotes = append(allNotes, shielderNoteCommitment{
			DenominationSats: deposit.ProtocolBondSats,
			Commitment:       nodeSlotSaleProtocolCommitment(auction, bid, deposit.ProtocolBondSats, 0),
		})
	}
	if err := insertShielderCommitments(ctx, k, deposit.DepositID, allNotes); err != nil {
		return types.ShielderDeposit{}, err
	}
	deposit.Commitments = shielderCommitmentStrings(allNotes)
	deposit.Status = types.ShielderStatusCommitted
	deposit.BondConfirmed = true
	if err := transferNodeSlotSaleBond(ctx, k, auction, bid, deposit.ProtocolBondSats); err != nil {
		return types.ShielderDeposit{}, err
	}
	auction.Status = types.NodeSlotAuctionSettled
	auction.UpdatedHeight = ctx.BlockHeight()
	bid.Settled = true
	bid.UpdatedHeight = ctx.BlockHeight()
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := k.SetNodeSlotAuction(ctx, auction); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := k.SetNodeSlotBid(ctx, bid); err != nil {
		return types.ShielderDeposit{}, err
	}
	return deposit, nil
}

func recordPendingShielderValidatorBond(ctx cosmos.Context, k keeper.Keeper, session types.ShielderSession, amountSats uint64) (uint64, error) {
	validatorPubKey := strings.TrimSpace(session.ValidatorPubKey)
	if validatorPubKey == "" {
		return 0, nil
	}
	consPubKey, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, validatorPubKey)
	if err != nil {
		return 0, fmt.Errorf("invalid shielder validator pubkey: %w", err)
	}
	nodeAddress := sdk.AccAddress(consPubKey.Address().Bytes())
	bond, err := k.GetShielderValidatorBond(ctx, validatorPubKey)
	if err != nil {
		return 0, err
	}
	if bond.NodeAddress.Empty() {
		slot, err := k.AllocateShielderNodeBondSlot(ctx)
		if err != nil {
			return 0, err
		}
		bond = types.ShielderValidatorBond{
			ValidatorPubKey: validatorPubKey,
			OperatorPubKey:  session.OperatorPubKey,
			NodeAddress:     nodeAddress,
			Slot:            slot,
			CreatedHeight:   ctx.BlockHeight(),
		}
	} else if !bond.NodeAddress.Equals(nodeAddress) {
		return 0, fmt.Errorf("validator pubkey node address changed")
	} else if !bond.OperatorPubKey.Equals(session.OperatorPubKey) {
		return 0, fmt.Errorf("validator bond operator pubkey mismatch")
	}
	bond.PendingSats += amountSats
	bond.UpdatedHeight = ctx.BlockHeight()
	if err := k.SetShielderValidatorBond(ctx, bond); err != nil {
		return 0, err
	}
	return bond.Slot, nil
}

func confirmShielderValidatorBond(ctx cosmos.Context, k keeper.Keeper, deposit types.ShielderDeposit) error {
	if !deposit.IsValidatorBond() {
		return nil
	}
	bond, err := k.GetShielderValidatorBond(ctx, deposit.ValidatorPubKey)
	if err != nil {
		return err
	}
	if bond.NodeAddress.Empty() {
		return fmt.Errorf("shielder validator bond not found")
	}
	if bond.PendingSats < deposit.AmountSats {
		return fmt.Errorf("shielder validator pending bond underflow")
	}
	bond.PendingSats -= deposit.AmountSats
	pool, err := distributeShielderFeePool(ctx, k)
	if err != nil {
		return err
	}
	bond.BondSats += deposit.AmountSats
	if !bond.FeeShareActive {
		bond.FeeShareActive = true
		bond.FeeDebtSats = pool.FeePerSlotShare
		pool.TotalSlots++
	}
	bond.UpdatedHeight = ctx.BlockHeight()

	nodeAccount, err := k.GetNodeAccount(ctx, bond.NodeAddress)
	if err != nil {
		return err
	}
	bondAddress, err := common.NewAddress(deposit.Owner.String())
	if err != nil {
		return fmt.Errorf("invalid shielder operator bond address: %w", err)
	}
	if nodeAccount.IsEmpty() {
		nodeAccount = NewNodeAccount(bond.NodeAddress, NodeWhiteListed, common.EmptyPubKeySet, deposit.ValidatorPubKey, cosmos.ZeroUint(), bondAddress, ctx.BlockHeight())
	}
	nodeAccount.ValidatorConsPubKey = deposit.ValidatorPubKey
	nodeAccount.BondAddress = bondAddress
	nodeAccount.Bond = cosmos.NewUint(bond.BondSats)
	if err := k.SetNodeAccount(ctx, nodeAccount); err != nil {
		return err
	}
	if err := k.SetShielderValidatorBond(ctx, bond); err != nil {
		return err
	}
	if err := k.SetShielderFeePool(ctx, pool); err != nil {
		return err
	}
	return nil
}

func PostShielderCommitments(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, depositID common.TxID, commitments []string) (types.ShielderDeposit, error) {
	if owner.Empty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder owner")
	}
	if len(commitments) == 0 {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder commitments")
	}

	deposit, err := k.GetShielderDeposit(ctx, depositID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if deposit.DepositID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit not found")
	}
	if !deposit.Owner.Equals(owner) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit owner mismatch")
	}
	if len(deposit.Commitments) > 0 {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit already split")
	}
	if deposit.Status != types.ShielderStatusSettled {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit is not settled")
	}

	noteCommitments, err := parseShielderNoteCommitments(commitments, deposit.AmountSats, deposit.Settlement == types.ShielderSettlementOperatorBond)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	seen := make(map[string]struct{}, len(noteCommitments))
	var total uint64
	for idx, note := range noteCommitments {
		if deposit.Settlement == types.ShielderSettlementOperatorBond {
			note.Commitment = shielderBondCommitment(deposit, note.DenominationSats, idx)
			noteCommitments[idx] = note
		}
		if note.Commitment == "" {
			return types.ShielderDeposit{}, fmt.Errorf("empty shielder commitment")
		}
		if _, ok := seen[note.Commitment]; ok {
			return types.ShielderDeposit{}, fmt.Errorf("duplicate shielder commitment")
		}
		if k.ShielderCommitmentExists(ctx, note.Commitment) {
			return types.ShielderDeposit{}, fmt.Errorf("shielder commitment already exists")
		}
		total += note.DenominationSats
		seen[note.Commitment] = struct{}{}
	}
	if total != deposit.AmountSats {
		return types.ShielderDeposit{}, fmt.Errorf("shielder commitment denominations do not match deposit amount")
	}

	deposit.Commitments = make([]string, 0, len(noteCommitments))
	for _, note := range noteCommitments {
		deposit.Commitments = append(deposit.Commitments, note.Commitment)
	}
	if deposit.Settlement == types.ShielderSettlementOperatorBond {
		if err := confirmShielderValidatorBond(ctx, k, deposit); err != nil {
			return types.ShielderDeposit{}, err
		}
		deposit.BondConfirmed = true
	}
	deposit.Status = types.ShielderStatusCommitted
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	byDenomination := make(map[uint64][]string)
	for _, note := range noteCommitments {
		if err := k.SetShielderCommitment(ctx, note.Commitment, depositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderDenominationCommitment(ctx, note.DenominationSats, note.Commitment, depositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		byDenomination[note.DenominationSats] = append(byDenomination[note.DenominationSats], note.Commitment)
	}
	for denomination := range byDenomination {
		leaves, err := k.GetShielderDenominationCommitments(ctx, denomination)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		root, err := ComputeShielderMerkleRoot(leaves)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderMerkleRoot(ctx, denomination, root); err != nil {
			return types.ShielderDeposit{}, err
		}
	}
	return deposit, nil
}

func parseShielderNoteCommitments(raw []string, depositAmountSats uint64, allowProtocolCommitments bool) ([]shielderNoteCommitment, error) {
	result := make([]shielderNoteCommitment, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("empty shielder commitment")
		}
		if strings.HasPrefix(item, "{") {
			var note shielderNoteCommitment
			if err := json.Unmarshal([]byte(item), &note); err != nil {
				return nil, fmt.Errorf("invalid shielder commitment: %w", err)
			}
			note.Commitment = strings.TrimSpace(note.Commitment)
			if note.DenominationSats == 0 {
				return nil, fmt.Errorf("missing shielder commitment denomination")
			}
			if note.Commitment == "" && !allowProtocolCommitments {
				return nil, fmt.Errorf("missing shielder commitment")
			}
			result = append(result, note)
			continue
		}
		if len(raw) != 1 {
			return nil, fmt.Errorf("split shielder commitments require denomination_sats")
		}
		result = append(result, shielderNoteCommitment{DenominationSats: depositAmountSats, Commitment: item})
	}
	return result, nil
}

func shielderBondCommitment(deposit types.ShielderDeposit, denominationSats uint64, index int) string {
	raw := fmt.Sprintf("thornado:bond-commitment:v1|%s|%s|%s|%d|%d|%d|%s",
		deposit.DepositID.String(),
		deposit.OperatorPubKey.String(),
		deposit.ValidatorPubKey,
		deposit.NodeSlot,
		denominationSats,
		index,
		deposit.VaultPubKey.String(),
	)
	sum := sha256.Sum256([]byte(raw))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func insertShielderCommitments(ctx cosmos.Context, k keeper.Keeper, depositID common.TxID, notes []shielderNoteCommitment) error {
	seen := make(map[string]struct{}, len(notes))
	byDenomination := make(map[uint64][]string)
	for _, note := range notes {
		note.Commitment = strings.TrimSpace(note.Commitment)
		if note.DenominationSats == 0 {
			return fmt.Errorf("missing shielder commitment denomination")
		}
		if note.Commitment == "" {
			return fmt.Errorf("empty shielder commitment")
		}
		if _, ok := seen[note.Commitment]; ok {
			return fmt.Errorf("duplicate shielder commitment")
		}
		if k.ShielderCommitmentExists(ctx, note.Commitment) {
			return fmt.Errorf("shielder commitment already exists")
		}
		seen[note.Commitment] = struct{}{}
		if err := k.SetShielderCommitment(ctx, note.Commitment, depositID); err != nil {
			return err
		}
		if err := k.SetShielderDenominationCommitment(ctx, note.DenominationSats, note.Commitment, depositID); err != nil {
			return err
		}
		byDenomination[note.DenominationSats] = append(byDenomination[note.DenominationSats], note.Commitment)
	}
	for denomination := range byDenomination {
		leaves, err := k.GetShielderDenominationCommitments(ctx, denomination)
		if err != nil {
			return err
		}
		root, err := ComputeShielderMerkleRoot(leaves)
		if err != nil {
			return err
		}
		if err := k.SetShielderMerkleRoot(ctx, denomination, root); err != nil {
			return err
		}
	}
	return nil
}

func shielderCommitmentStrings(notes []shielderNoteCommitment) []string {
	result := make([]string, 0, len(notes))
	for _, note := range notes {
		result = append(result, note.Commitment)
	}
	return result
}

func nodeSlotAuctionID(validatorPubKey string, slot uint64, height int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-slot-auction:v1|%s|%d|%d", validatorPubKey, slot, height)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func nodeSlotBidID(auctionID string, bidder cosmos.AccAddress, pathIndex uint64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-slot-bid:v1|%s|%s|%d", auctionID, bidder.String(), pathIndex)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func nodeSlotSaleProtocolCommitment(auction types.NodeSlotAuction, bid types.NodeSlotBid, denominationSats uint64, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-slot-sale-bond:v1|%s|%s|%s|%d|%d|%d",
		auction.AuctionID,
		bid.BidID,
		bid.ValidatorPubKey,
		auction.Slot,
		denominationSats,
		index,
	)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func transferNodeSlotSaleBond(ctx cosmos.Context, k keeper.Keeper, auction types.NodeSlotAuction, bid types.NodeSlotBid, extraBondSats uint64) error {
	consPubKey, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, bid.ValidatorPubKey)
	if err != nil {
		return fmt.Errorf("invalid winning validator pubkey: %w", err)
	}
	newNodeAddress := sdk.AccAddress(consPubKey.Address().Bytes())
	oldBond, err := k.GetShielderValidatorBond(ctx, auction.SellerValidatorPubKey)
	if err != nil {
		return err
	}
	if oldBond.ValidatorPubKey == "" || oldBond.Slot != auction.Slot {
		return fmt.Errorf("seller bond does not match auction slot")
	}
	oldBond.Sold = true
	oldBond.SoldAuctionID = auction.AuctionID
	oldBond.FeeShareActive = false
	oldBond.BondSats = 0
	oldBond.PendingSats = 0
	oldBond.UpdatedHeight = ctx.BlockHeight()

	newBond := types.ShielderValidatorBond{
		ValidatorPubKey: bid.ValidatorPubKey,
		OperatorPubKey:  bid.OperatorPubKey,
		NodeAddress:     newNodeAddress,
		Slot:            auction.Slot,
		BondSats:        auction.OriginalBondSats + extraBondSats,
		CreatedHeight:   ctx.BlockHeight(),
		UpdatedHeight:   ctx.BlockHeight(),
	}
	pool, err := distributeShielderFeePool(ctx, k)
	if err != nil {
		return err
	}
	newBond.FeeShareActive = true
	newBond.FeeDebtSats = pool.FeePerSlotShare

	bondAddress, err := common.NewAddress(bid.Bidder.String())
	if err != nil {
		return fmt.Errorf("invalid bidder bond address: %w", err)
	}
	nodeAccount, err := k.GetNodeAccount(ctx, newNodeAddress)
	if err != nil {
		return err
	}
	if nodeAccount.IsEmpty() {
		nodeAccount = NewNodeAccount(newNodeAddress, NodeStandby, common.EmptyPubKeySet, bid.ValidatorPubKey, cosmos.ZeroUint(), bondAddress, ctx.BlockHeight())
	}
	nodeAccount.ValidatorConsPubKey = bid.ValidatorPubKey
	nodeAccount.BondAddress = bondAddress
	nodeAccount.Bond = cosmos.NewUint(newBond.BondSats)
	nodeAccount.UpdateStatus(NodeStandby, ctx.BlockHeight())
	if err := k.SetShielderValidatorBond(ctx, oldBond); err != nil {
		return err
	}
	if err := k.SetShielderValidatorBond(ctx, newBond); err != nil {
		return err
	}
	if err := k.SetNodeAccount(ctx, nodeAccount); err != nil {
		return err
	}
	return k.SetShielderFeePool(ctx, pool)
}

func RequestShielderWithdrawal(ctx cosmos.Context, k keeper.Keeper, req ShielderWithdrawalRequest) (types.ShielderWithdrawal, error) {
	if req.Owner.Empty() {
		return types.ShielderWithdrawal{}, fmt.Errorf("missing shielder withdrawal owner")
	}
	if err := VerifyShielderWithdrawalJSON(req.Proof, req.Public); err != nil {
		return types.ShielderWithdrawal{}, err
	}

	publicInputs, err := parseShielderWithdrawalPublicInputs(req.Public)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if k.ShielderNullifierSpent(ctx, publicInputs.NullifierHash) {
		return types.ShielderWithdrawal{}, fmt.Errorf("shielder nullifier already spent")
	}
	if !k.ShielderMerkleRootExists(ctx, publicInputs.DenominationSats, publicInputs.MerkleRoot) {
		return types.ShielderWithdrawal{}, fmt.Errorf("unknown shielder merkle root")
	}

	recipient, err := common.NewAddress(publicInputs.Recipient)
	if err != nil {
		return types.ShielderWithdrawal{}, fmt.Errorf("invalid shielder withdrawal recipient: %w", err)
	}
	if !recipient.GetChain().Equals(common.BTCChain) {
		return types.ShielderWithdrawal{}, fmt.Errorf("shielder withdrawal recipient must be bitcoin")
	}

	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}

	withdrawalID := shielderWithdrawalID(publicInputs.NullifierHash, recipient.String())
	inHash, err := common.NewTxID(withdrawalID)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}
	feeSats := shielderWithdrawalFeeSats(publicInputs.DenominationSats)
	if publicInputs.FeeSats != 0 && publicInputs.FeeSats != feeSats {
		return types.ShielderWithdrawal{}, fmt.Errorf("invalid shielder withdrawal fee: %d/%d", publicInputs.FeeSats, feeSats)
	}

	withdrawal := types.ShielderWithdrawal{
		WithdrawalID:    withdrawalID,
		Owner:           req.Owner,
		NullifierHash:   publicInputs.NullifierHash,
		MerkleRoot:      publicInputs.MerkleRoot,
		Recipient:       recipient,
		AmountSats:      publicInputs.DenominationSats,
		FeeSats:         feeSats,
		InHash:          inHash,
		VaultPubKey:     vault.PubKey,
		RequestedHeight: ctx.BlockHeight(),
		Status:          types.ShielderStatusKeysignQueued,
		Proof:           append(json.RawMessage(nil), req.Proof...),
		Public:          append(json.RawMessage(nil), req.Public...),
	}
	if err := withdrawal.Valid(); err != nil {
		return types.ShielderWithdrawal{}, err
	}

	if err := k.SetShielderWithdrawal(ctx, withdrawal); err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if err := k.SetShielderNullifierSpent(ctx, withdrawal.NullifierHash, withdrawal.WithdrawalID); err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if err := addShielderWithdrawalFee(ctx, k, withdrawal.FeeSats); err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if err := queueShielderWithdrawalKeysign(ctx, k, withdrawal); err != nil {
		return types.ShielderWithdrawal{}, err
	}

	return withdrawal, nil
}

func shielderWithdrawalFeeSats(amountSats uint64) uint64 {
	return amountSats * WithdrawalFeeBp / 10_000
}

func addShielderWithdrawalFee(ctx cosmos.Context, k keeper.Keeper, amountSats uint64) error {
	if amountSats == 0 {
		return nil
	}
	pool, err := k.GetShielderFeePool(ctx)
	if err != nil {
		return err
	}
	pool.PendingSats += amountSats
	pool.TotalCollectedSats += amountSats
	return setDistributedShielderFeePool(ctx, k, pool)
}

func distributeShielderFeePool(ctx cosmos.Context, k keeper.Keeper) (types.ShielderFeePool, error) {
	pool, err := k.GetShielderFeePool(ctx)
	if err != nil {
		return pool, err
	}
	return pool, setDistributedShielderFeePool(ctx, k, pool)
}

func setDistributedShielderFeePool(ctx cosmos.Context, k keeper.Keeper, pool types.ShielderFeePool) error {
	if pool.PendingSats != 0 && pool.TotalSlots != 0 {
		increment := mulDivSats(pool.PendingSats, feeShareScale, pool.TotalSlots)
		if increment != 0 {
			distributed := mulDivSats(increment, pool.TotalSlots, feeShareScale)
			pool.FeePerSlotShare += increment
			pool.PendingSats -= distributed
		}
	}
	return k.SetShielderFeePool(ctx, pool)
}

func mulDivSats(a, b, c uint64) uint64 {
	if c == 0 || a == 0 || b == 0 {
		return 0
	}
	product := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	product.Div(product, new(big.Int).SetUint64(c))
	return product.Uint64()
}

func SettleShielderFees(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, validatorPubKey string, operatorSignature []byte) (types.ShielderDeposit, error) {
	if owner.Empty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder fee owner")
	}
	bond, err := k.GetShielderValidatorBond(ctx, validatorPubKey)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if bond.ValidatorPubKey == "" || !bond.FeeShareActive {
		return types.ShielderDeposit{}, fmt.Errorf("shielder validator has no confirmed bond")
	}
	if !bond.PendingFeeDepositID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder fee settlement already pending split")
	}
	pool, err := distributeShielderFeePool(ctx, k)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	accrued := pool.FeePerSlotShare
	if accrued <= bond.FeeDebtSats {
		return types.ShielderDeposit{}, fmt.Errorf("no shielder fees claimable")
	}
	claimSats := accrued - bond.FeeDebtSats
	authPayload := shielderFeeSettlePayload(validatorPubKey, owner, accrued, pool.FeePerSlotShare)
	if err := verifySecp256K1SignaturePayload(bond.OperatorPubKey, operatorSignature, authPayload); err != nil {
		return types.ShielderDeposit{}, fmt.Errorf("invalid shielder fee operator signature: %w", err)
	}
	depositID, err := shielderFeeDepositID(validatorPubKey, owner, accrued, ctx.BlockHeight())
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	deposit := types.ShielderDeposit{
		DepositID:      depositID,
		Owner:          owner,
		AmountSats:     claimSats,
		DepositAddress: common.NoopAddress,
		MatchedHeight:  ctx.BlockHeight(),
		Status:         types.ShielderStatusSettled,
		Settlement:     types.ShielderSettlementOperatorFee,
	}
	bond.FeeDebtSats = accrued
	bond.PendingFeeDepositID = depositID
	bond.UpdatedHeight = ctx.BlockHeight()
	pool.TotalClaimedSats += claimSats
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := k.SetShielderValidatorBond(ctx, bond); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := k.SetShielderFeePool(ctx, pool); err != nil {
		return types.ShielderDeposit{}, err
	}
	return deposit, nil
}

func SplitShielderFees(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, validatorPubKey string, operatorSignature []byte, commitments, feeNotePubKeys []string) (types.ShielderDeposit, error) {
	if owner.Empty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder fee owner")
	}
	if len(commitments) == 0 {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder fee commitments")
	}
	bond, err := k.GetShielderValidatorBond(ctx, validatorPubKey)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if bond.PendingFeeDepositID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder fee settlement not found")
	}
	deposit, err := k.GetShielderDeposit(ctx, bond.PendingFeeDepositID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if deposit.DepositID.IsEmpty() || deposit.Status != types.ShielderStatusSettled || deposit.Settlement != types.ShielderSettlementOperatorFee {
		return types.ShielderDeposit{}, fmt.Errorf("shielder fee deposit is not settled")
	}
	if !deposit.Owner.Equals(owner) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder fee owner mismatch")
	}
	if len(deposit.Commitments) > 0 {
		return types.ShielderDeposit{}, fmt.Errorf("shielder fee deposit already split")
	}
	noteCommitments, err := parseShielderNoteCommitments(commitments, deposit.AmountSats, false)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	notePubKeys, err := parseShielderFeeNotePubKeys(feeNotePubKeys, len(noteCommitments))
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	var total uint64
	for _, note := range noteCommitments {
		total += note.DenominationSats
	}
	if total != deposit.AmountSats {
		return types.ShielderDeposit{}, fmt.Errorf("shielder fee commitment denominations do not match claim amount")
	}
	authPayload := shielderFeeClaimPayload(validatorPubKey, owner, bond.FeeDebtSats, bond.FeeDebtSats, noteCommitments, notePubKeys)
	if err := verifySecp256K1SignaturePayload(bond.OperatorPubKey, operatorSignature, authPayload); err != nil {
		return types.ShielderDeposit{}, fmt.Errorf("invalid shielder fee operator signature: %w", err)
	}
	seen := make(map[string]struct{}, len(noteCommitments))
	byDenomination := make(map[uint64][]string)
	for idx, note := range noteCommitments {
		if note.Commitment == "" {
			return types.ShielderDeposit{}, fmt.Errorf("empty shielder fee commitment")
		}
		if _, ok := seen[note.Commitment]; ok {
			return types.ShielderDeposit{}, fmt.Errorf("duplicate shielder fee commitment")
		}
		if k.ShielderCommitmentExists(ctx, note.Commitment) {
			return types.ShielderDeposit{}, fmt.Errorf("shielder fee commitment already exists")
		}
		if k.ShielderFeeNotePubKeyUsed(ctx, notePubKeys[idx]) {
			return types.ShielderDeposit{}, fmt.Errorf("shielder fee note pubkey already used")
		}
		seen[note.Commitment] = struct{}{}
		deposit.Commitments = append(deposit.Commitments, note.Commitment)
		if err := k.SetShielderCommitment(ctx, note.Commitment, deposit.DepositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderDenominationCommitment(ctx, note.DenominationSats, note.Commitment, deposit.DepositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderFeeNotePubKey(ctx, notePubKeys[idx], deposit.DepositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		byDenomination[note.DenominationSats] = append(byDenomination[note.DenominationSats], note.Commitment)
	}
	for denomination := range byDenomination {
		leaves, err := k.GetShielderDenominationCommitments(ctx, denomination)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		root, err := ComputeShielderMerkleRoot(leaves)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderMerkleRoot(ctx, denomination, root); err != nil {
			return types.ShielderDeposit{}, err
		}
	}
	bond.PendingFeeDepositID = common.TxID("")
	bond.UpdatedHeight = ctx.BlockHeight()
	deposit.Status = types.ShielderStatusCommitted
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := k.SetShielderValidatorBond(ctx, bond); err != nil {
		return types.ShielderDeposit{}, err
	}
	return deposit, nil
}

func shielderFeeDepositID(validatorPubKey string, owner cosmos.AccAddress, accrued uint64, height int64) (common.TxID, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:fee-split:v1|%s|%s|%d|%d", validatorPubKey, owner.String(), accrued, height)))
	return common.NewTxID(strings.ToUpper(hex.EncodeToString(sum[:])))
}

func shielderFeeSettlePayload(validatorPubKey string, owner cosmos.AccAddress, accrued, feePerSlotShare uint64) []byte {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"thornado:fee-settle:v1",
		validatorPubKey,
		owner.String(),
		fmt.Sprintf("%d", accrued),
		fmt.Sprintf("%d", feePerSlotShare),
	}, "|")))
	return sum[:]
}

func parseShielderFeeNotePubKeys(raw []string, expected int) ([]common.PubKey, error) {
	if len(raw) != expected {
		return nil, fmt.Errorf("shielder fee note pubkey count mismatch")
	}
	result := make([]common.PubKey, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		pubKey, err := common.NewPubKey(strings.TrimSpace(item))
		if err != nil {
			return nil, fmt.Errorf("invalid shielder fee note pubkey: %w", err)
		}
		if pubKey.IsEmpty() {
			return nil, fmt.Errorf("missing shielder fee note pubkey")
		}
		if _, ok := seen[pubKey.String()]; ok {
			return nil, fmt.Errorf("duplicate shielder fee note pubkey")
		}
		seen[pubKey.String()] = struct{}{}
		result = append(result, pubKey)
	}
	return result, nil
}

func shielderFeeClaimPayload(validatorPubKey string, owner cosmos.AccAddress, accrued, feePerSlotShare uint64, notes []shielderNoteCommitment, notePubKeys []common.PubKey) []byte {
	parts := []string{
		"thornado:fee-claim:v1",
		validatorPubKey,
		owner.String(),
		fmt.Sprintf("%d", accrued),
		fmt.Sprintf("%d", feePerSlotShare),
	}
	for idx, note := range notes {
		parts = append(parts, fmt.Sprintf("%d:%s:%s", note.DenominationSats, note.Commitment, notePubKeys[idx].String()))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return sum[:]
}

func verifySecp256K1SignaturePayload(pk common.PubKey, sig []byte, payload []byte) error {
	if len(sig) != 64 {
		return fmt.Errorf("invalid secp256k1 signature length")
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	halfOrder := new(big.Int).Rsh(btcec.S256().N, 1)
	if s.Cmp(halfOrder) == 1 {
		return fmt.Errorf("high-S signature rejected")
	}
	signature := &btcec.Signature{R: r, S: s}
	spk, err := pk.Secp256K1()
	if err != nil {
		return fmt.Errorf("fail to get secp256k1 pubkey: %w", err)
	}
	if !signature.Verify(payload, spk) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func parseShielderWithdrawalPublicInputs(raw json.RawMessage) (shielderWithdrawalPublicInputs, error) {
	var publicInputs shielderWithdrawalPublicInputs
	if err := json.Unmarshal(raw, &publicInputs); err != nil {
		return publicInputs, fmt.Errorf("invalid shielder public inputs: %w", err)
	}
	if strings.TrimSpace(publicInputs.NullifierHash) == "" {
		return publicInputs, fmt.Errorf("missing shielder nullifier hash")
	}
	if strings.TrimSpace(publicInputs.MerkleRoot) == "" {
		return publicInputs, fmt.Errorf("missing shielder merkle root")
	}
	if publicInputs.DenominationSats == 0 {
		return publicInputs, fmt.Errorf("missing shielder denomination")
	}
	if publicInputs.FeeSats >= publicInputs.DenominationSats {
		return publicInputs, fmt.Errorf("shielder fee exceeds denomination")
	}
	return publicInputs, nil
}

func queueShielderWithdrawalKeysign(ctx cosmos.Context, k keeper.Keeper, withdrawal types.ShielderWithdrawal) error {
	amount := withdrawal.AmountSats - withdrawal.FeeSats
	item := TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   withdrawal.Recipient,
		VaultPubKey: withdrawal.VaultPubKey,
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(amount)),
		Memo:        "SHIELDER:" + withdrawal.WithdrawalID,
		MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(withdrawal.FeeSats))},
		GasRate:     1,
		InHash:      withdrawal.InHash,
		ModuleName:  AsgardName,
		TxType:      types.TxOutTypeOut,
	}
	return k.AppendTxOut(ctx, ctx.BlockHeight(), item)
}

func queueVaultPathSweep(ctx cosmos.Context, mgr Manager, tx ObservedTx, sourcePubKey common.PubKey, pathIndex uint64) error {
	if sourcePubKey.IsEmpty() {
		return fmt.Errorf("missing sweep vault pubkey")
	}
	coin := tx.Tx.Coins.GetCoin(common.BTCAsset)
	if coin.IsEmpty() || coin.Amount.IsZero() {
		return fmt.Errorf("missing sweep bitcoin amount")
	}
	currentVault, currentRoot, err := currentBTCVaultAddress(ctx, mgr.Keeper())
	if err != nil {
		return err
	}
	sourceAddr, err := common.DeriveBTCTaprootAddress(sourcePubKey, pathIndex)
	if err != nil {
		return err
	}
	if sourcePubKey.Equals(currentVault.PubKey) && pathIndex == common.MainVaultPathIndex && tx.Tx.ToAddress.Equals(currentRoot) {
		return nil
	}

	maxGasCoin, err := mgr.GasMgr().GetMaxGas(ctx, common.BTCChain)
	if err != nil {
		return fmt.Errorf("fail to get bitcoin sweep max gas: %w", err)
	}
	amount := coin.Amount
	if gas := maxGasCoin.Amount; !gas.IsZero() {
		if amount.LTE(gas) {
			return fmt.Errorf("sweep amount is not enough to pay gas")
		}
		amount = amount.Sub(gas)
	}
	gasRate := int64(1)
	if nf, err := mgr.Keeper().GetNetworkFee(ctx, common.BTCChain); err == nil && nf.TransactionFeeRate > 0 {
		gasRate = int64(nf.TransactionFeeRate)
	}

	txType := types.TxOutTypeSweep
	if pathIndex == common.MainVaultPathIndex {
		txType = types.TxOutTypeMigrate
	}
	item := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      currentRoot,
		VaultPubKey:    sourcePubKey,
		Coin:           common.NewCoin(common.BTCAsset, amount),
		MaxGas:         common.Gas{maxGasCoin},
		GasRate:        gasRate,
		InHash:         tx.Tx.ID,
		ModuleName:     AsgardName,
		VaultPathIndex: pathIndex,
		TxType:         txType,
	}
	ctx.Logger().Info("queued memoless bitcoin vault path sweep",
		"from", sourceAddr.String(),
		"to", currentRoot.String(),
		"vault_pub_key", sourcePubKey.String(),
		"path_index", pathIndex,
		"amount", amount.String(),
	)
	return mgr.Keeper().AppendTxOut(ctx, ctx.BlockHeight(), item)
}

func currentBTCVaultAddress(ctx cosmos.Context, k keeper.Keeper) (Vault, common.Address, error) {
	vaults, err := k.GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return Vault{}, common.NoAddress, err
	}
	if len(vaults) == 0 {
		return Vault{}, common.NoAddress, fmt.Errorf("no active shielder bitcoin vault")
	}
	for _, vault := range vaults {
		address, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
		if err == nil && !address.IsEmpty() {
			return vault, address, nil
		}
	}
	return Vault{}, common.NoAddress, fmt.Errorf("no active shielder bitcoin vault address")
}

func shielderWithdrawalID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
