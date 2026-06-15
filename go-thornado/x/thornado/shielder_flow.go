package thornado

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/btcsuite/btcd/btcec"
	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type ShielderRedeemRequest struct {
	Proof  json.RawMessage
	Public json.RawMessage
}

type shielderRedeemPublicInputs struct {
	NullifierHash    string `json:"nullifier_hash"`
	MerkleRoot       string `json:"merkle_root"`
	DenominationSats uint64 `json:"denomination_sats"`
	Recipient        string `json:"recipient"`
	FeeSats          uint64 `json:"fee_sats"`
	RecipientPolicy  string `json:"recipient_policy,omitempty"`
	NodePubKey       string `json:"node_pub_key,omitempty"`
	BidID            string `json:"bid_id,omitempty"`
}

type shielderNoteCommitment struct {
	DenominationSats uint64 `json:"denomination_sats"`
	Commitment       string `json:"commitment"`
}

func CreateNodeSlotAuction(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, nodePubKey string, reserveSats uint64, expiryHeight int64) (types.NodeSlotAuction, error) {
	bond, err := validateNodeSlotAuctionCreate(ctx, k, seller, nodePubKey, expiryHeight)
	if err != nil {
		return types.NodeSlotAuction{}, err
	}
	auctionID := nodeSlotAuctionID(nodePubKey, bond.Slot, ctx.BlockHeight())
	auction := types.NodeSlotAuction{
		AuctionID:            auctionID,
		Seller:               seller,
		SellerOperatorPubKey: bond.OperatorPubKey,
		SellerNodePubKey:     bond.NodePubKey,
		Slot:                 bond.Slot,
		OriginalBondSats:     bond.BondSats,
		ReserveSats:          reserveSats,
		ExpiryHeight:         expiryHeight,
		Status:               types.NodeSlotAuctionOpen,
		CreatedHeight:        ctx.BlockHeight(),
		UpdatedHeight:        ctx.BlockHeight(),
	}
	return auction, k.SetNodeSlotAuction(ctx, auction)
}

func validateNodeSlotAuctionCreate(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, nodePubKey string, expiryHeight int64) (types.ShielderNodeBond, error) {
	if seller.Empty() {
		return types.ShielderNodeBond{}, fmt.Errorf("missing node slot auction seller")
	}
	if expiryHeight <= ctx.BlockHeight() {
		return types.ShielderNodeBond{}, fmt.Errorf("node slot auction expiry must be in the future")
	}
	duration := expiryHeight - ctx.BlockHeight()
	if minExpiry := getConfigDurationBlocks(ctx, k, constants.NodeSale_AuctionExpiryMinMinutes); minExpiry > 0 && duration < minExpiry {
		return types.ShielderNodeBond{}, fmt.Errorf("node slot auction expiry below minimum: %d/%d", duration, minExpiry)
	}
	if maxExpiry := getConfigDurationBlocks(ctx, k, constants.NodeSale_AuctionExpiryMaxMinutes); maxExpiry > 0 && duration > maxExpiry {
		return types.ShielderNodeBond{}, fmt.Errorf("node slot auction expiry above maximum: %d/%d", duration, maxExpiry)
	}
	bond, err := k.GetShielderNodeBond(ctx, nodePubKey)
	if err != nil {
		return types.ShielderNodeBond{}, err
	}
	if bond.NodePubKey == "" || bond.BondSats == 0 || !bond.FeeShareActive || bond.Sold {
		return types.ShielderNodeBond{}, fmt.Errorf("node has no active bonded slot")
	}
	nodeAccount, err := k.GetNodeAccount(ctx, bond.NodeAddress)
	if err != nil {
		return types.ShielderNodeBond{}, err
	}
	if nodeAccount.BondAddress.String() != seller.String() {
		return types.ShielderNodeBond{}, fmt.Errorf("node slot auction seller mismatch")
	}
	if nodeAccount.Status != NodeStandby {
		return types.ShielderNodeBond{}, fmt.Errorf("node slot must be standby before auction")
	}
	iter := k.GetNodeSlotAuctionIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var auction types.NodeSlotAuction
		if err := json.Unmarshal(iter.Value(), &auction); err != nil {
			return types.ShielderNodeBond{}, err
		}
		if auction.SellerNodePubKey != nodePubKey {
			continue
		}
		switch auction.Status {
		case types.NodeSlotAuctionOpen:
			if auction.ExpiryHeight > ctx.BlockHeight() {
				return types.ShielderNodeBond{}, fmt.Errorf("node slot auction already open")
			}
		case types.NodeSlotAuctionSelected:
			return types.ShielderNodeBond{}, fmt.Errorf("node slot auction already selected")
		}
	}
	return bond, nil
}

func SelectNodeSlotBid(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, auctionID, bidID string) (types.NodeSlotAuction, types.NodeSlotBid, error) {
	auction, bid, err := validateNodeSlotBidSelection(ctx, k, seller, auctionID, bidID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, err
	}
	auction.SelectedBidID = bid.BidID
	auction.Status = types.NodeSlotAuctionSettled
	auction.UpdatedHeight = ctx.BlockHeight()
	bid.Selected = true
	bid.Settled = true
	bid.UpdatedHeight = ctx.BlockHeight()
	if err := settleNodeSlotSale(ctx, k, auction, bid); err != nil {
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

func validateNodeSlotBidSelection(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, auctionID, bidID string) (types.NodeSlotAuction, types.NodeSlotBid, error) {
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
	if bid.AmountSats == 0 {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid deposit not matched")
	}
	if minBid := uint64(k.GetConfigInt64(ctx, constants.NodeSale_BidAmountMinSats)); bid.AmountSats < minBid {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid below minimum")
	}
	if bid.AmountSats < auction.ReserveSats {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("node slot bid below reserve")
	}
	if !bid.DepositID.IsEmpty() {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, fmt.Errorf("legacy node slot bid deposits are not supported")
	}
	return auction, bid, nil
}

func CreateNodeSlotBid(ctx cosmos.Context, k keeper.Keeper, bidder cosmos.AccAddress, auctionID, operatorPubKey, nodePubKey string) (types.NodeSlotBid, error) {
	if bidder.Empty() {
		return types.NodeSlotBid{}, fmt.Errorf("missing node slot bidder")
	}
	auction, err := k.GetNodeSlotAuction(ctx, strings.TrimSpace(auctionID))
	if err != nil {
		return types.NodeSlotBid{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionOpen {
		return types.NodeSlotBid{}, fmt.Errorf("node slot auction is not open")
	}
	if auction.ExpiryHeight <= ctx.BlockHeight() {
		return types.NodeSlotBid{}, fmt.Errorf("node slot auction expired")
	}
	operatorPubKey = strings.TrimSpace(operatorPubKey)
	nodePubKey = strings.TrimSpace(nodePubKey)
	operator, err := common.NewPubKey(operatorPubKey)
	if err != nil {
		return types.NodeSlotBid{}, fmt.Errorf("invalid bidder operator pubkey: %w", err)
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, nodePubKey); err != nil {
		return types.NodeSlotBid{}, fmt.Errorf("invalid bidder node pubkey: %w", err)
	}
	bidID := nodeSlotBidIDForNode(auction.AuctionID, bidder, nodePubKey)
	existing, err := k.GetNodeSlotBid(ctx, bidID)
	if err != nil {
		return types.NodeSlotBid{}, err
	}
	if existing.BidID != "" {
		return existing, nil
	}
	bid := types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auction.AuctionID,
		Bidder:         bidder,
		OperatorPubKey: operator,
		NodePubKey:     nodePubKey,
		DepositAddress: common.BondEscrowAddress,
		CreatedHeight:  ctx.BlockHeight(),
		UpdatedHeight:  ctx.BlockHeight(),
	}
	return bid, k.SetNodeSlotBid(ctx, bid)
}

func settleNodeSlotSale(ctx cosmos.Context, k keeper.Keeper, auction types.NodeSlotAuction, bid types.NodeSlotBid) error {
	if bid.AmountSats == 0 {
		return fmt.Errorf("node slot bid has no shielded amount")
	}
	sellerPayout := auction.OriginalBondSats
	if bid.AmountSats < sellerPayout {
		sellerPayout = bid.AmountSats
	}
	protocolBondSats := bid.AmountSats - sellerPayout
	if protocolBondDust, err := shielderDustRemainder(ctx, k, protocolBondSats); err != nil {
		return err
	} else if protocolBondDust > 0 {
		protocolBondSats -= protocolBondDust
		if err := addWithdrawalFee(ctx, k, protocolBondDust); err != nil {
			return err
		}
	}
	if err := transferNodeSlotSaleBond(ctx, k, auction, bid, bid.AmountSats); err != nil {
		return err
	}
	entitlementID, err := nodeSlotSaleEntitlementID(auction.AuctionID, bid.BidID)
	if err != nil {
		return err
	}
	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return err
	}
	deposit := types.DepositRecord{
		DepositID:          entitlementID,
		Owner:              auction.Seller,
		AmountSats:         sellerPayout,
		DepositAddress:     common.NoopAddress,
		VaultPubKey:        vault.PubKey,
		Settlement:         types.DepositSettlementOperatorSale,
		SellerPayoutSats:   sellerPayout,
		ProtocolBondSats:   protocolBondSats,
		NodeSlot:           auction.Slot,
		BondConfirmed:      true,
		MatchedHeight:      ctx.BlockHeight(),
		CreatedHeight:      ctx.BlockHeight(),
		Status:             types.DepositStatusSettled,
		OperatorPubKey:     auction.SellerOperatorPubKey,
		NodePubKey:         auction.SellerNodePubKey,
		AuctionID:          auction.AuctionID,
		RefundQueuedHeight: 0,
	}
	return k.SetDepositRecord(ctx, deposit)
}

func ShieldNodeSlotSaleEntitlement(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, auctionID, bidID, depositPubkey, signature string, sellerCommitments []string) (types.DepositRecord, error) {
	auction, bid, deposit, err := validateNodeSlotSaleEntitlementShield(ctx, k, seller, auctionID, bidID, sellerCommitments)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if auction.SelectedBidID != bid.BidID {
		return types.DepositRecord{}, fmt.Errorf("node slot auction selected bid mismatch")
	}
	if deposit.AmountSats == 0 {
		return types.DepositRecord{}, fmt.Errorf("node sale has no seller payout")
	}
	noteCommitments, err := parseShielderNoteCommitments(sellerCommitments, deposit.AmountSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	authorizedAmountSats := shielderNoteCommitmentTotal(noteCommitments)
	if err := VerifyShieldAuthorization(depositPubkey, signature, depositPubkey, authorizedAmountSats, sellerCommitments); err != nil {
		return types.DepositRecord{}, err
	}
	noteCommitments, floorRemainder, err := applyShielderNoteFloor(ctx, k, noteCommitments, deposit.AmountSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if floorRemainder > 0 {
		deposit.AmountSats -= floorRemainder
		deposit.SellerPayoutSats -= floorRemainder
		if err := addWithdrawalFee(ctx, k, floorRemainder); err != nil {
			return types.DepositRecord{}, err
		}
	}
	if shielderNoteCommitmentTotal(noteCommitments) != deposit.AmountSats {
		return types.DepositRecord{}, fmt.Errorf("node sale seller commitments do not match payout amount")
	}
	if err := insertShielderCommitments(ctx, k, noteCommitments); err != nil {
		return types.DepositRecord{}, err
	}
	deposit.Status = types.DepositStatusCommitted
	if err := k.SetDepositRecord(ctx, deposit); err != nil {
		return types.DepositRecord{}, err
	}
	return deposit, nil
}

func validateNodeSlotSaleEntitlementShield(ctx cosmos.Context, k keeper.Keeper, seller cosmos.AccAddress, auctionID, bidID string, sellerCommitments []string) (types.NodeSlotAuction, types.NodeSlotBid, types.DepositRecord, error) {
	auction, err := k.GetNodeSlotAuction(ctx, auctionID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionSettled || auction.SelectedBidID != strings.TrimSpace(bidID) {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, fmt.Errorf("node slot auction sale is not settled")
	}
	if !auction.Seller.Equals(seller) {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, fmt.Errorf("node slot auction seller mismatch")
	}
	bid, err := k.GetNodeSlotBid(ctx, bidID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, err
	}
	if bid.BidID == "" || bid.Settled {
		if bid.BidID == "" {
			return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, fmt.Errorf("node slot bid not found")
		}
	}
	entitlementID, err := nodeSlotSaleEntitlementID(auction.AuctionID, bid.BidID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, err
	}
	deposit, err := k.GetDepositRecord(ctx, entitlementID)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, err
	}
	if deposit.DepositID.IsEmpty() || deposit.Status != types.DepositStatusSettled || deposit.Settlement != types.DepositSettlementOperatorSale {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, fmt.Errorf("node sale entitlement is not shieldable")
	}
	notes, err := parseShielderNoteCommitments(sellerCommitments, deposit.AmountSats, false)
	if err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, err
	}
	if _, _, err := applyShielderNoteFloor(ctx, k, notes, deposit.AmountSats, false); err != nil {
		return types.NodeSlotAuction{}, types.NodeSlotBid{}, types.DepositRecord{}, err
	}
	return auction, bid, deposit, nil
}

func recordPendingShielderNodeBond(ctx cosmos.Context, k keeper.Keeper, session types.DepositSession, amountSats uint64) (uint64, error) {
	nodePubKey := strings.TrimSpace(session.NodePubKey)
	if nodePubKey == "" {
		return 0, nil
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, nodePubKey); err != nil {
		return 0, fmt.Errorf("invalid node pubkey: %w", err)
	}
	nodeAddress, err := session.OperatorPubKey.GetThorAddress()
	if err != nil {
		return 0, fmt.Errorf("invalid shielder operator pubkey address: %w", err)
	}
	bond, err := k.GetShielderNodeBond(ctx, nodePubKey)
	if err != nil {
		return 0, err
	}
	if bond.NodeAddress.Empty() {
		slot, err := k.AllocateShielderNodeBondSlot(ctx)
		if err != nil {
			return 0, err
		}
		requiredSats := shielderBondRequiredSats(ctx, k, slot)
		if amountSats < requiredSats {
			return 0, fmt.Errorf("node bond below required amount: %d/%d", amountSats, requiredSats)
		}
		bond = types.ShielderNodeBond{
			NodePubKey:     nodePubKey,
			OperatorPubKey: session.OperatorPubKey,
			NodeAddress:    nodeAddress,
			Slot:           slot,
			CreatedHeight:  ctx.BlockHeight(),
		}
	} else if !bond.NodeAddress.Equals(nodeAddress) {
		return 0, fmt.Errorf("node pubkey node address changed")
	} else if !bond.OperatorPubKey.Equals(session.OperatorPubKey) {
		return 0, fmt.Errorf("node bond operator pubkey mismatch")
	}
	bond.PendingSats += amountSats
	bond.UpdatedHeight = ctx.BlockHeight()
	if err := k.SetShielderNodeBond(ctx, bond); err != nil {
		return 0, err
	}
	return bond.Slot, nil
}

func confirmShielderNodeBond(ctx cosmos.Context, k keeper.Keeper, deposit types.DepositRecord) error {
	if !deposit.IsNodeBond() {
		return nil
	}
	bond, err := k.GetShielderNodeBond(ctx, deposit.NodePubKey)
	if err != nil {
		return err
	}
	if bond.NodeAddress.Empty() {
		return fmt.Errorf("shielder node bond not found")
	}
	if bond.PendingSats < deposit.AmountSats {
		return fmt.Errorf("shielder node pending bond underflow")
	}
	bond.PendingSats -= deposit.AmountSats
	pool, err := distributeFeePool(ctx, k)
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
		nodeAccount = NewNodeAccount(bond.NodeAddress, NodeWhiteListed, common.EmptyPubKeySet, deposit.NodePubKey, cosmos.ZeroUint(), bondAddress, ctx.BlockHeight())
	}
	nodeAccount.NodeConsPubKey = deposit.NodePubKey
	nodeAccount.BondAddress = bondAddress
	nodeAccount.Bond = cosmos.NewUint(bond.BondSats)
	if err := k.SetNodeAccount(ctx, nodeAccount); err != nil {
		return err
	}
	if err := k.SetShielderNodeBond(ctx, bond); err != nil {
		return err
	}
	if err := k.SetFeePool(ctx, pool); err != nil {
		return err
	}
	return nil
}

// ShieldUserDepositIntoPool consumes a matched transparent user deposit into
// fixed-denomination shielded pools without a separate shield transaction.
func ShieldUserDepositIntoPool(ctx cosmos.Context, k keeper.Keeper, deposit types.DepositRecord, commitments []string) (types.DepositRecord, error) {
	if deposit.DepositID.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("missing deposit id")
	}
	if deposit.IsNodeBond() || deposit.AuctionID != "" {
		return types.DepositRecord{}, fmt.Errorf("only user deposits shield into the privacy pool")
	}
	switch deposit.Status {
	case types.DepositStatusDepositMatched:
		deposit.Settlement = types.DepositSettlementUser
	case types.DepositStatusSettled:
		if deposit.Settlement != types.DepositSettlementUser || deposit.ShieldedSats != 0 {
			return types.DepositRecord{}, fmt.Errorf("duplicate deposit settlement")
		}
	case types.DepositStatusCommitted:
		return types.DepositRecord{}, fmt.Errorf("deposit already shielded")
	default:
		return types.DepositRecord{}, fmt.Errorf("deposit is not matched")
	}
	if len(commitments) == 0 {
		return types.DepositRecord{}, fmt.Errorf("missing shield note commitments")
	}
	if deposit.ShieldedSats > deposit.AmountSats {
		return types.DepositRecord{}, fmt.Errorf("deposit shielded amount exceeds deposit amount")
	}
	availableSats := deposit.AmountSats - deposit.ShieldedSats
	if availableSats == 0 {
		return types.DepositRecord{}, fmt.Errorf("deposit already fully shielded")
	}

	noteCommitments, err := parseShielderNoteCommitments(commitments, availableSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	noteCommitments, floorRemainder, err := applyShielderNoteFloor(ctx, k, noteCommitments, availableSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if floorRemainder > 0 {
		availableSats -= floorRemainder
		if err := addWithdrawalFee(ctx, k, floorRemainder); err != nil {
			return types.DepositRecord{}, err
		}
	}
	if shielderNoteCommitmentTotal(noteCommitments) != availableSats {
		return types.DepositRecord{}, fmt.Errorf("shield note denominations must match deposit amount")
	}
	if err := insertShielderCommitments(ctx, k, noteCommitments); err != nil {
		return types.DepositRecord{}, err
	}
	deposit.ShieldedSats += availableSats + floorRemainder
	deposit.Status = types.DepositStatusCommitted
	if err := k.SetDepositRecord(ctx, deposit); err != nil {
		return types.DepositRecord{}, err
	}
	return deposit, nil
}

func PostShielderShield(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, depositID common.TxID, commitments []string) (types.DepositRecord, error) {
	if owner.Empty() {
		return types.DepositRecord{}, fmt.Errorf("missing deposit owner")
	}
	if len(commitments) == 0 {
		return types.DepositRecord{}, fmt.Errorf("missing shielder commitments")
	}

	deposit, err := k.GetDepositRecord(ctx, depositID)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if deposit.DepositID.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("deposit not found")
	}
	if !deposit.Owner.Equals(owner) {
		return types.DepositRecord{}, fmt.Errorf("deposit owner mismatch")
	}
	switch deposit.Status {
	case types.DepositStatusDepositMatched:
		if deposit.IsNodeBond() {
			return types.DepositRecord{}, fmt.Errorf("node bonds activate via MsgBondFromNotes from shielded notes")
		}
		deposit.Settlement = types.DepositSettlementUser
		return ShieldUserDepositIntoPool(ctx, k, deposit, commitments)
	case types.DepositStatusSettled:
		if deposit.Settlement == types.DepositSettlementUser {
			return ShieldUserDepositIntoPool(ctx, k, deposit, commitments)
		}
		if deposit.Settlement == types.DepositSettlementOperatorBond {
			return types.DepositRecord{}, fmt.Errorf("node bonds activate via MsgBondFromNotes from shielded notes")
		}
		if deposit.Settlement == "" || deposit.ShieldedSats != 0 {
			return types.DepositRecord{}, fmt.Errorf("duplicate deposit settlement")
		}
	case types.DepositStatusCommitted:
		return types.DepositRecord{}, fmt.Errorf("deposit already shielded")
	default:
		return types.DepositRecord{}, fmt.Errorf("deposit is not matched")
	}
	if deposit.ShieldedSats > deposit.AmountSats {
		return types.DepositRecord{}, fmt.Errorf("deposit shielded amount exceeds deposit amount")
	}
	availableSats := deposit.AmountSats - deposit.ShieldedSats
	if availableSats == 0 {
		return types.DepositRecord{}, fmt.Errorf("deposit already fully shielded")
	}

	noteCommitments, err := parseShielderNoteCommitments(commitments, availableSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	noteCommitments, floorRemainder, err := applyShielderNoteFloor(ctx, k, noteCommitments, availableSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if floorRemainder > 0 {
		availableSats -= floorRemainder
		if err := addWithdrawalFee(ctx, k, floorRemainder); err != nil {
			return types.DepositRecord{}, err
		}
	}
	var total uint64
	for _, note := range noteCommitments {
		total += note.DenominationSats
	}
	if total == 0 {
		return types.DepositRecord{}, fmt.Errorf("missing shielder commitment amount")
	}
	if total != availableSats {
		return types.DepositRecord{}, fmt.Errorf("shielder commitment denominations must match deposit amount")
	}

	deposit.ShieldedSats += total + floorRemainder
	deposit.Status = types.DepositStatusCommitted
	if err := k.SetDepositRecord(ctx, deposit); err != nil {
		return types.DepositRecord{}, err
	}
	if err := insertShielderCommitments(ctx, k, noteCommitments); err != nil {
		return types.DepositRecord{}, err
	}
	return deposit, nil
}

func AccAddressFromCompressedSecp256k1(pubkeyHex string) (cosmos.AccAddress, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(pubkeyHex))
	if err != nil {
		return nil, fmt.Errorf("invalid secp256k1 pubkey")
	}
	if len(raw) == 32 {
		raw = append([]byte{0x02}, raw...)
	}
	if len(raw) != 33 {
		return nil, fmt.Errorf("invalid secp256k1 pubkey length")
	}
	if raw[0] != 0x02 && raw[0] != 0x03 {
		return nil, fmt.Errorf("invalid compressed secp256k1 pubkey prefix")
	}
	pubkey := &sdksecp256k1.PubKey{Key: raw}
	return cosmos.AccAddress(pubkey.Address()), nil
}

func VerifyShieldAuthorization(depositPubkey string, signatureHex string, depositID string, amountSats uint64, commitments []string) error {
	notes := make([]shielderNoteCommitment, 0, len(commitments))
	for _, raw := range commitments {
		var note shielderNoteCommitment
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &note); err != nil {
			return fmt.Errorf("invalid shielder commitment: %w", err)
		}
		note.Commitment = strings.TrimSpace(note.Commitment)
		notes = append(notes, note)
	}
	commitmentsJSON, err := json.Marshal(notes)
	if err != nil {
		return err
	}
	digest := hashLengthPrefixedParts([]string{
		"thornado-shielder-v1",
		"shield-authorization",
		strings.TrimSpace(depositPubkey),
		strings.TrimSpace(depositID),
		fmt.Sprintf("%d", amountSats),
		string(commitmentsJSON),
	})
	rawPubkey, err := secpPubkeyCandidates(strings.TrimSpace(depositPubkey))
	if err != nil {
		return fmt.Errorf("invalid deposit pubkey")
	}
	rawSig, err := hex.DecodeString(strings.TrimSpace(signatureHex))
	if err != nil {
		return fmt.Errorf("invalid shield authorization signature")
	}
	signature, err := btcec.ParseDERSignature(rawSig, btcec.S256())
	if err != nil {
		return fmt.Errorf("invalid shield authorization signature: %w", err)
	}
	halfOrder := new(big.Int).Rsh(btcec.S256().N, 1)
	if signature.S.Cmp(halfOrder) == 1 {
		return fmt.Errorf("high-S signature rejected")
	}
	for _, candidate := range rawPubkey {
		pubkey, err := btcec.ParsePubKey(candidate, btcec.S256())
		if err == nil && signature.Verify(digest, pubkey) {
			return nil
		}
	}
	return fmt.Errorf("shield authorization signature verification failed")
}

func hashLengthPrefixedParts(parts []string) []byte {
	hasher := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		hasher.Write(length[:])
		hasher.Write([]byte(part))
	}
	return hasher.Sum(nil)
}

func secpPubkeyCandidates(pubkeyHex string) ([][]byte, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(pubkeyHex))
	if err != nil {
		return nil, err
	}
	if len(raw) == 33 {
		return [][]byte{raw}, nil
	}
	if len(raw) == 32 {
		return [][]byte{append([]byte{0x02}, raw...), append([]byte{0x03}, raw...)}, nil
	}
	return nil, fmt.Errorf("invalid secp256k1 pubkey length")
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
			return nil, fmt.Errorf("shield commitments require denomination_sats")
		}
		result = append(result, shielderNoteCommitment{DenominationSats: depositAmountSats, Commitment: item})
	}
	return result, nil
}

func applyShielderNoteFloor(ctx cosmos.Context, k keeper.Keeper, notes []shielderNoteCommitment, amountSats uint64, allowSpendableRemainder bool) ([]shielderNoteCommitment, uint64, error) {
	minNote := uint64(k.GetConfigInt64(ctx, constants.Shielder_NoteAmountMinSats))
	if minNote == 0 {
		return notes, 0, nil
	}
	filtered := make([]shielderNoteCommitment, 0, len(notes))
	var noteTotal, feeRemainder uint64
	for _, note := range notes {
		if note.DenominationSats < minNote {
			feeRemainder += note.DenominationSats
			continue
		}
		noteTotal += note.DenominationSats
		filtered = append(filtered, note)
	}
	if len(filtered) == 0 {
		return nil, 0, fmt.Errorf("shielder shield has no notes above minimum")
	}
	if noteTotal > amountSats {
		return nil, 0, fmt.Errorf("shielder commitment denominations exceed amount")
	}
	if noteTotal+feeRemainder > amountSats {
		return nil, 0, fmt.Errorf("shielder commitment denominations exceed amount")
	}
	if allowSpendableRemainder {
		return filtered, feeRemainder, nil
	}
	unallocated := amountSats - noteTotal - feeRemainder
	if unallocated >= minNote {
		return nil, 0, fmt.Errorf("shielder commitment denominations leave spendable remainder")
	}
	feeRemainder += unallocated
	return filtered, feeRemainder, nil
}

func shielderDustRemainder(ctx cosmos.Context, k keeper.Keeper, amountSats uint64) (uint64, error) {
	minNote := uint64(k.GetConfigInt64(ctx, constants.Shielder_NoteAmountMinSats))
	if minNote == 0 || amountSats == 0 || amountSats >= minNote {
		return 0, nil
	}
	return amountSats, nil
}

func shielderBondCommitment(deposit types.DepositRecord, denominationSats uint64, index int) (string, error) {
	raw := fmt.Sprintf("thornado:bond-commitment:v1|%s|%s|%s|%d|%d|%d|%s",
		deposit.DepositID.String(),
		deposit.OperatorPubKey.String(),
		deposit.NodePubKey,
		deposit.NodeSlot,
		denominationSats,
		index,
		deposit.VaultPubKey.String(),
	)
	sum := sha256.Sum256([]byte(raw))
	return ComputeProtocolShielderCommitment(strings.ToUpper(hex.EncodeToString(sum[:])), denominationSats)
}

func insertShielderCommitments(ctx cosmos.Context, k keeper.Keeper, notes []shielderNoteCommitment) error {
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
		if err := k.SetShielderCommitment(ctx, note.Commitment); err != nil {
			return err
		}
		if err := k.SetShielderDenominationCommitment(ctx, note.DenominationSats, note.Commitment); err != nil {
			return err
		}
		if err := k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
			Commitment:       note.Commitment,
			DenominationSats: note.DenominationSats,
		}); err != nil {
			return err
		}
		byDenomination[note.DenominationSats] = append(byDenomination[note.DenominationSats], note.Commitment)
	}
	return refreshShielderRoots(ctx, k, byDenomination)
}

func refreshShielderRoots(ctx cosmos.Context, k keeper.Keeper, byDenomination map[uint64][]string) error {
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

func shielderNoteCommitmentTotal(notes []shielderNoteCommitment) uint64 {
	var total uint64
	for _, note := range notes {
		total += note.DenominationSats
	}
	return total
}

func nodeSlotAuctionID(nodePubKey string, slot uint64, height int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-slot-auction:v1|%s|%d|%d", nodePubKey, slot, height)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func nodeSlotBidIDForNode(auctionID string, bidder cosmos.AccAddress, nodePubKey string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-slot-bid:v2|%s|%s|%s", auctionID, bidder.String(), strings.TrimSpace(nodePubKey))))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func nodeSlotSaleEntitlementID(auctionID, bidID string) (common.TxID, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-sale-entitlement:v1|%s|%s", strings.TrimSpace(auctionID), strings.TrimSpace(bidID))))
	return common.NewTxID(strings.ToUpper(hex.EncodeToString(sum[:])))
}

func nodeSlotSaleProtocolCommitment(auction types.NodeSlotAuction, bid types.NodeSlotBid, denominationSats uint64, index int) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:node-slot-sale-bond:v1|%s|%s|%s|%d|%d|%d",
		auction.AuctionID,
		bid.BidID,
		bid.NodePubKey,
		auction.Slot,
		denominationSats,
		index,
	)))
	return ComputeProtocolShielderCommitment(strings.ToUpper(hex.EncodeToString(sum[:])), denominationSats)
}

func transferNodeSlotSaleBond(ctx cosmos.Context, k keeper.Keeper, auction types.NodeSlotAuction, bid types.NodeSlotBid, newBondSats uint64) error {
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, bid.NodePubKey); err != nil {
		return fmt.Errorf("invalid winning node pubkey: %w", err)
	}
	newNodeAddress, err := bid.OperatorPubKey.GetThorAddress()
	if err != nil {
		return fmt.Errorf("invalid winning operator pubkey address: %w", err)
	}
	oldBond, err := k.GetShielderNodeBond(ctx, auction.SellerNodePubKey)
	if err != nil {
		return err
	}
	if oldBond.NodePubKey == "" || oldBond.Slot != auction.Slot {
		return fmt.Errorf("seller bond does not match auction slot")
	}
	oldBond.Sold = true
	oldBond.SoldAuctionID = auction.AuctionID
	oldBond.FeeShareActive = false
	oldBond.BondSats = 0
	oldBond.PendingSats = 0
	oldBond.UpdatedHeight = ctx.BlockHeight()

	newBond := types.ShielderNodeBond{
		NodePubKey:     bid.NodePubKey,
		OperatorPubKey: bid.OperatorPubKey,
		NodeAddress:    newNodeAddress,
		Slot:           auction.Slot,
		BondSats:       newBondSats,
		CreatedHeight:  ctx.BlockHeight(),
		UpdatedHeight:  ctx.BlockHeight(),
	}
	pool, err := distributeFeePool(ctx, k)
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
		nodeAccount = NewNodeAccount(newNodeAddress, NodeStandby, common.EmptyPubKeySet, bid.NodePubKey, cosmos.ZeroUint(), bondAddress, ctx.BlockHeight())
	}
	nodeAccount.NodeConsPubKey = bid.NodePubKey
	nodeAccount.BondAddress = bondAddress
	nodeAccount.Bond = cosmos.NewUint(newBond.BondSats)
	nodeAccount.UpdateStatus(NodeStandby, ctx.BlockHeight())
	if err := k.SetShielderNodeBond(ctx, oldBond); err != nil {
		return err
	}
	if err := k.SetShielderNodeBond(ctx, newBond); err != nil {
		return err
	}
	if err := k.SetNodeAccount(ctx, nodeAccount); err != nil {
		return err
	}
	return k.SetFeePool(ctx, pool)
}

func AuthorizeShielderRedeem(ctx cosmos.Context, k keeper.Keeper, req ShielderRedeemRequest) (types.ShielderRedeem, error) {
	publicInputs, err := parseShielderRedeemPublicInputs(req.Public)
	if err != nil {
		return types.ShielderRedeem{}, err
	}
	policy := normalizeShielderRedeemPolicy(publicInputs.RecipientPolicy)
	if err := validateShielderRedeemPolicy(ctx, k, policy, publicInputs); err != nil {
		return types.ShielderRedeem{}, err
	}
	if err := RejectLeakyShielderRedeemProof(ctx, k, req.Proof); err != nil {
		return types.ShielderRedeem{}, err
	}
	if err := VerifyShielderRedeemJSON(req.Proof, req.Public); err != nil {
		return types.ShielderRedeem{}, err
	}
	if k.ShielderNullifierSpent(ctx, publicInputs.NullifierHash) {
		return types.ShielderRedeem{}, fmt.Errorf("shielder nullifier already spent")
	}
	if !k.ShielderMerkleRootExists(ctx, publicInputs.DenominationSats, publicInputs.MerkleRoot) {
		return types.ShielderRedeem{}, fmt.Errorf("unknown shielder merkle root")
	}

	recipient, err := shielderRedeemRecipient(publicInputs, policy)
	if err != nil {
		return types.ShielderRedeem{}, err
	}

	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.ShielderRedeem{}, err
	}

	withdrawalID := shielderRedeemID(publicInputs.NullifierHash, recipient.String(), policy)
	inHash, err := common.NewTxID(withdrawalID)
	if err != nil {
		return types.ShielderRedeem{}, err
	}
	withdrawal := types.ShielderRedeem{
		WithdrawalID:    withdrawalID,
		NullifierHash:   publicInputs.NullifierHash,
		MerkleRoot:      publicInputs.MerkleRoot,
		Recipient:       recipient,
		RecipientPolicy: policy,
		BidID:           strings.TrimSpace(publicInputs.BidID),
		NodePubKey:      strings.TrimSpace(publicInputs.NodePubKey),
		AmountSats:      publicInputs.DenominationSats,
		FeeSats:         publicInputs.FeeSats,
		InHash:          inHash,
		VaultPubKey:     vault.PubKey,
		RequestedHeight: ctx.BlockHeight(),
		Status:          types.ShielderRedeemStatusAuthorized,
	}
	if err := withdrawal.Valid(); err != nil {
		return types.ShielderRedeem{}, err
	}

	if err := k.SetShielderRedeem(ctx, withdrawal); err != nil {
		return types.ShielderRedeem{}, err
	}
	if err := k.SetShielderNullifierSpent(ctx, withdrawal.NullifierHash, withdrawal.WithdrawalID); err != nil {
		return types.ShielderRedeem{}, err
	}

	return withdrawal, nil
}

func BondFromShieldedNotes(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, nodePubKey, operatorPubKey string, req ShielderRedeemRequest) (types.ShielderNodeBond, error) {
	if owner.Empty() {
		return types.ShielderNodeBond{}, fmt.Errorf("missing bond owner")
	}
	nodePubKey = strings.TrimSpace(nodePubKey)
	if nodePubKey == "" {
		return types.ShielderNodeBond{}, fmt.Errorf("missing node pubkey")
	}
	operator, err := common.NewPubKey(operatorPubKey)
	if err != nil {
		return types.ShielderNodeBond{}, fmt.Errorf("invalid operator pubkey: %w", err)
	}
	operatorAddress, err := operator.GetThorAddress()
	if err != nil {
		return types.ShielderNodeBond{}, fmt.Errorf("invalid operator pubkey address: %w", err)
	}
	if !operatorAddress.Equals(owner) {
		return types.ShielderNodeBond{}, fmt.Errorf("bond owner mismatch")
	}
	publicInputs, err := parseShielderRedeemPublicInputs(req.Public)
	if err != nil {
		return types.ShielderNodeBond{}, err
	}
	if normalizeShielderRedeemPolicy(publicInputs.RecipientPolicy) != types.ShielderRedeemPolicyBondEscrow {
		return types.ShielderNodeBond{}, fmt.Errorf("bond notes require bond_escrow recipient policy")
	}
	if strings.TrimSpace(publicInputs.NodePubKey) != nodePubKey {
		return types.ShielderNodeBond{}, fmt.Errorf("bond node pubkey mismatch")
	}
	if publicInputs.FeeSats != 0 {
		return types.ShielderNodeBond{}, fmt.Errorf("bond notes must not pay withdrawal fee")
	}
	withdrawal, err := AuthorizeShielderRedeem(ctx, k, req)
	if err != nil {
		return types.ShielderNodeBond{}, err
	}
	return confirmBondFromNoteSpend(ctx, k, owner, nodePubKey, operator, withdrawal.AmountSats)
}

func confirmBondFromNoteSpend(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, nodePubKey string, operator common.PubKey, amountSats uint64) (types.ShielderNodeBond, error) {
	nodeSlot, err := recordPendingShielderNodeBond(ctx, k, types.DepositSession{
		Owner:          owner,
		OperatorPubKey: operator,
		NodePubKey:     nodePubKey,
	}, amountSats)
	if err != nil {
		return types.ShielderNodeBond{}, err
	}
	deposit := types.DepositRecord{
		Owner:          owner,
		AmountSats:     amountSats,
		OperatorPubKey: operator,
		NodePubKey:     nodePubKey,
		NodeSlot:       nodeSlot,
		Settlement:     types.DepositSettlementOperatorBond,
	}
	if err := confirmShielderNodeBond(ctx, k, deposit); err != nil {
		return types.ShielderNodeBond{}, err
	}
	bond, err := k.GetShielderNodeBond(ctx, nodePubKey)
	if err != nil {
		return types.ShielderNodeBond{}, err
	}
	return bond, nil
}

func FinalizeShielderRedeem(ctx cosmos.Context, k keeper.Keeper, authorization types.ShielderRedeem) (types.ShielderRedeem, error) {
	policy := normalizeShielderRedeemPolicy(authorization.RecipientPolicy)
	switch policy {
	case types.ShielderRedeemPolicyBondEscrow:
		return authorization, fmt.Errorf("bond escrow redeems finalize via MsgBondFromNotes")
	case types.ShielderRedeemPolicyBidDeposit:
		return FinalizeBidDepositFromNoteSpend(ctx, k, authorization)
	case types.ShielderRedeemPolicyUserBTC:
		return QueueAuthorizedWithdrawalTxOut(ctx, k, authorization)
	default:
		return types.ShielderRedeem{}, fmt.Errorf("unknown shielder redeem policy: %s", policy)
	}
}

func FinalizeBidDepositFromNoteSpend(ctx cosmos.Context, k keeper.Keeper, authorization types.ShielderRedeem) (types.ShielderRedeem, error) {
	if err := authorization.Valid(); err != nil {
		return types.ShielderRedeem{}, err
	}
	if normalizeShielderRedeemPolicy(authorization.RecipientPolicy) != types.ShielderRedeemPolicyBidDeposit {
		return types.ShielderRedeem{}, fmt.Errorf("shielder redeem is not a bid deposit")
	}
	if authorization.FeeSats != 0 {
		return types.ShielderRedeem{}, fmt.Errorf("bid deposits must not pay withdrawal fee")
	}
	bid, err := openNodeSlotBidForRedeem(ctx, k, authorization.BidID, authorization.Recipient)
	if err != nil {
		return types.ShielderRedeem{}, err
	}
	bid.AmountSats += authorization.AmountSats
	bid.UpdatedHeight = ctx.BlockHeight()
	if err := k.SetNodeSlotBid(ctx, bid); err != nil {
		return types.ShielderRedeem{}, err
	}
	authorization.Status = types.ShielderRedeemStatusSettled
	if err := k.SetShielderRedeem(ctx, authorization); err != nil {
		return types.ShielderRedeem{}, err
	}
	return authorization, nil
}

func openNodeSlotBidForRedeem(ctx cosmos.Context, k keeper.Keeper, bidID string, recipient common.Address) (types.NodeSlotBid, error) {
	if !recipient.IsBondEscrow() {
		return types.NodeSlotBid{}, fmt.Errorf("bid deposit redeem recipient must be bond_escrow")
	}
	bid, err := k.GetNodeSlotBid(ctx, strings.TrimSpace(bidID))
	if err != nil {
		return types.NodeSlotBid{}, err
	}
	if bid.BidID == "" || bid.Settled {
		return types.NodeSlotBid{}, fmt.Errorf("node slot bid is not open for deposit")
	}
	auction, err := k.GetNodeSlotAuction(ctx, bid.AuctionID)
	if err != nil {
		return types.NodeSlotBid{}, err
	}
	if auction.AuctionID == "" || auction.Status != types.NodeSlotAuctionOpen {
		return types.NodeSlotBid{}, fmt.Errorf("node slot auction is not open")
	}
	if auction.ExpiryHeight <= ctx.BlockHeight() {
		return types.NodeSlotBid{}, fmt.Errorf("node slot auction expired")
	}
	return bid, nil
}

func normalizeShielderRedeemPolicy(policy string) string {
	switch strings.TrimSpace(policy) {
	case "", types.ShielderRedeemPolicyUserBTC:
		return types.ShielderRedeemPolicyUserBTC
	case types.ShielderRedeemPolicyBondEscrow:
		return types.ShielderRedeemPolicyBondEscrow
	case types.ShielderRedeemPolicyBidDeposit:
		return types.ShielderRedeemPolicyBidDeposit
	default:
		return strings.TrimSpace(policy)
	}
}

func shielderRedeemRecipient(publicInputs shielderRedeemPublicInputs, policy string) (common.Address, error) {
	recipient, err := common.NewAddress(publicInputs.Recipient)
	if err != nil {
		return common.NoAddress, fmt.Errorf("invalid shielder redeem recipient: %w", err)
	}
	switch policy {
	case types.ShielderRedeemPolicyBondEscrow:
		if !recipient.IsBondEscrow() {
			return common.NoAddress, fmt.Errorf("bond redeem recipient must be bond_escrow")
		}
	case types.ShielderRedeemPolicyUserBTC:
		if !recipient.GetChain().Equals(common.BTCChain) {
			return common.NoAddress, fmt.Errorf("shielder redeem recipient must be bitcoin")
		}
	case types.ShielderRedeemPolicyBidDeposit:
		if !recipient.IsBondEscrow() {
			return common.NoAddress, fmt.Errorf("bid deposit redeem recipient must be bond_escrow")
		}
	default:
		return common.NoAddress, fmt.Errorf("unknown shielder redeem policy: %s", policy)
	}
	return recipient, nil
}

func validateShielderRedeemPolicy(ctx cosmos.Context, k keeper.Keeper, policy string, publicInputs shielderRedeemPublicInputs) error {
	switch policy {
	case types.ShielderRedeemPolicyBondEscrow:
		if publicInputs.FeeSats != 0 {
			return fmt.Errorf("bond escrow redeems must not pay withdrawal fee")
		}
		if strings.TrimSpace(publicInputs.NodePubKey) == "" {
			return fmt.Errorf("bond escrow redeems require node pubkey")
		}
		if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, publicInputs.NodePubKey); err != nil {
			return fmt.Errorf("invalid bond node pubkey: %w", err)
		}
	case types.ShielderRedeemPolicyBidDeposit:
		if publicInputs.FeeSats != 0 {
			return fmt.Errorf("bid deposit redeems must not pay withdrawal fee")
		}
		bidID := strings.TrimSpace(publicInputs.BidID)
		if bidID == "" {
			return fmt.Errorf("bid deposit redeems require bid id")
		}
		recipient, err := common.NewAddress(publicInputs.Recipient)
		if err != nil {
			return err
		}
		if _, err := openNodeSlotBidForRedeem(ctx, k, bidID, recipient); err != nil {
			return err
		}
	case types.ShielderRedeemPolicyUserBTC:
		if strings.TrimSpace(publicInputs.BidID) != "" || strings.TrimSpace(publicInputs.NodePubKey) != "" {
			return fmt.Errorf("user btc redeems must not bind bid or node pubkey")
		}
	default:
		return fmt.Errorf("unknown shielder redeem policy: %s", policy)
	}
	return nil
}

func withdrawalFeeSats(ctx cosmos.Context, k keeper.Keeper, amountSats uint64) uint64 {
	fee := withdrawalFeeSatsForBp(amountSats, withdrawalFeeBp(ctx, k))
	if minFee := uint64(k.GetConfigInt64(ctx, constants.Withdrawal_FeeMinSats)); fee < minFee {
		return minFee
	}
	return fee
}

func withdrawalFeeSatsForBp(amountSats, feeBp uint64) uint64 {
	return amountSats * feeBp / 10_000
}

func withdrawalFeeBp(ctx cosmos.Context, k keeper.Keeper) uint64 {
	return uint64(k.GetConfigInt64(ctx, constants.Withdrawal_FeeBasisPoints))
}

func shielderBondRequiredSats(ctx cosmos.Context, k keeper.Keeper, slot uint64) uint64 {
	start := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))
	increment := uint64(k.GetConfigInt64(ctx, constants.Node_BondSlotIncrementSats))
	return start + slot*increment
}

func validateDepositPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken string) error {
	difficulty := currentDepositPowDifficulty(ctx, k)
	if difficulty <= 0 {
		return nil
	}
	if difficulty > 256 {
		return fmt.Errorf("deposit pow difficulty too high")
	}
	sum := sha256.Sum256([]byte(owner.String() + ":" + powToken))
	var zeros int64
	for _, b := range sum {
		for bit := 7; bit >= 0; bit-- {
			if (b>>uint(bit))&1 == 1 {
				if zeros >= difficulty {
					return nil
				}
				return fmt.Errorf("deposit pow token below difficulty")
			}
			zeros++
			if zeros >= difficulty {
				return nil
			}
		}
	}
	return nil
}

func currentDepositPowDifficulty(ctx cosmos.Context, k keeper.Keeper) int64 {
	state, err := k.GetDepositPowDifficultyState(ctx)
	if err == nil && state.Difficulty > 0 {
		return state.Difficulty
	}
	return k.GetConfigInt64(ctx, constants.Deposit_PowDifficultyMin)
}

func RetargetDepositPowDifficulty(ctx cosmos.Context, k keeper.Keeper) error {
	state, err := k.GetDepositPowDifficultyState(ctx)
	if err != nil {
		return err
	}
	if state.Difficulty <= 0 {
		state.Difficulty = k.GetConfigInt64(ctx, constants.Deposit_PowDifficultyMin)
	}
	lastRetargetHeight := state.LastRetargetHeight

	samples, totalWeight, err := depositPowRetargetSamples(ctx, k, lastRetargetHeight)
	if err != nil {
		return err
	}
	minSamples := uint64(k.GetConfigInt64(ctx, constants.Deposit_PowSamplesMin))
	if uint64(len(samples)) < minSamples || totalWeight == 0 {
		state.LastRetargetHeight = ctx.BlockHeight()
		state.SampleCount = uint64(len(samples))
		state.UpdatedHeight = ctx.BlockHeight()
		return k.SetDepositPowDifficultyState(ctx, state)
	}

	percentile := k.GetConfigInt64(ctx, constants.Deposit_PowTargetPercentile)
	if percentile <= 0 || percentile > 100 {
		percentile = 90
	}
	targetMs := uint64(k.GetConfigInt64(ctx, constants.Deposit_PowTargetSeconds)) * 1000
	if targetMs == 0 {
		targetMs = 10_000
	}
	p90 := weightedPowPercentile(samples, totalWeight, uint64(percentile))
	nextDifficulty := retargetPowDifficulty(state.Difficulty, p90, targetMs, k.GetConfigInt64(ctx, constants.Deposit_PowRetargetStepMax))
	minDifficulty := k.GetConfigInt64(ctx, constants.Deposit_PowDifficultyMin)
	maxDifficulty := k.GetConfigInt64(ctx, constants.Deposit_PowDifficultyMax)
	if nextDifficulty < minDifficulty {
		nextDifficulty = minDifficulty
	}
	if maxDifficulty > 0 && nextDifficulty > maxDifficulty {
		nextDifficulty = maxDifficulty
	}

	state.Difficulty = nextDifficulty
	state.LastRetargetHeight = ctx.BlockHeight()
	state.SampleCount = uint64(len(samples))
	state.WeightedP90Ms = p90
	state.UpdatedHeight = ctx.BlockHeight()
	return k.SetDepositPowDifficultyState(ctx, state)
}

type depositPowRetargetSample struct {
	durationMs uint64
	weightSats uint64
}

func depositPowRetargetSamples(ctx cosmos.Context, k keeper.Keeper, sinceHeight int64) ([]depositPowRetargetSample, uint64, error) {
	iter := k.GetDepositPowTimingIterator(ctx)
	defer iter.Close()

	var samples []depositPowRetargetSample
	var totalWeight uint64
	for ; iter.Valid(); iter.Next() {
		var record types.DepositPowTiming
		if err := json.Unmarshal(iter.Value(), &record); err != nil {
			return nil, 0, err
		}
		if !record.Deposited || record.MatchedHeight <= sinceHeight || record.DurationMs == 0 || record.DepositAmountSats == 0 {
			continue
		}
		samples = append(samples, depositPowRetargetSample{
			durationMs: record.DurationMs,
			weightSats: record.DepositAmountSats,
		})
		totalWeight += record.DepositAmountSats
	}
	return samples, totalWeight, nil
}

func weightedPowPercentile(samples []depositPowRetargetSample, totalWeight, percentile uint64) uint64 {
	if len(samples) == 0 || totalWeight == 0 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].durationMs < samples[j].durationMs
	})
	targetWeight := (totalWeight*percentile + 99) / 100
	var seen uint64
	for _, sample := range samples {
		seen += sample.weightSats
		if seen >= targetWeight {
			return sample.durationMs
		}
	}
	return samples[len(samples)-1].durationMs
}

func retargetPowDifficulty(current int64, observedMs, targetMs uint64, maxStep int64) int64 {
	if current <= 0 || observedMs == 0 || targetMs == 0 {
		return current
	}
	if maxStep <= 0 {
		maxStep = 1
	}
	step := int64(0)
	if observedMs*2 < targetMs {
		for adjusted := observedMs; adjusted*2 < targetMs && step < maxStep; adjusted *= 2 {
			step++
		}
		return current + step
	}
	if observedMs > targetMs*2 {
		for adjusted := observedMs; adjusted > targetMs*2 && step < maxStep; adjusted /= 2 {
			step++
		}
		return current - step
	}
	return current
}

func addWithdrawalFee(ctx cosmos.Context, k keeper.Keeper, amountSats uint64) error {
	if amountSats == 0 {
		return nil
	}
	pool, err := k.GetFeePool(ctx)
	if err != nil {
		return err
	}
	pool.PendingSats += amountSats
	pool.TotalCollectedSats += amountSats
	return setDistributedFeePool(ctx, k, pool)
}

func distributeFeePool(ctx cosmos.Context, k keeper.Keeper) (types.FeePool, error) {
	pool, err := k.GetFeePool(ctx)
	if err != nil {
		return pool, err
	}
	if err := setDistributedFeePool(ctx, k, pool); err != nil {
		return pool, err
	}
	return k.GetFeePool(ctx)
}

func setDistributedFeePool(ctx cosmos.Context, k keeper.Keeper, pool types.FeePool) error {
	if pool.PendingSats != 0 && pool.TotalSlots != 0 {
		minNote := uint64(k.GetConfigInt64(ctx, constants.Shielder_NoteAmountMinSats))
		if minNote != 0 && pool.PendingSats < minNote*pool.TotalSlots {
			return k.SetFeePool(ctx, pool)
		}
		increment := pool.PendingSats / pool.TotalSlots
		if increment != 0 {
			distributed := increment * pool.TotalSlots
			pool.FeePerSlotShare += increment
			pool.PendingSats -= distributed
		}
	}
	return k.SetFeePool(ctx, pool)
}

func ShieldShielderFees(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, nodePubKey string, operatorSignature []byte, commitments, feeNotePubKeys []string) (types.DepositRecord, error) {
	if owner.Empty() {
		return types.DepositRecord{}, fmt.Errorf("missing shielder fee owner")
	}
	if len(commitments) == 0 {
		return types.DepositRecord{}, fmt.Errorf("missing shielder fee commitments")
	}
	bond, err := k.GetShielderNodeBond(ctx, nodePubKey)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if bond.NodePubKey == "" || !bond.FeeShareActive {
		return types.DepositRecord{}, fmt.Errorf("shielder node has no confirmed bond")
	}
	if !bond.NodeAddress.Equals(owner) {
		return types.DepositRecord{}, fmt.Errorf("shielder fee owner mismatch")
	}
	if !bond.PendingFeeDepositID.IsEmpty() {
		return types.DepositRecord{}, fmt.Errorf("shielder fee settlement already pending shield")
	}
	pool, err := distributeFeePool(ctx, k)
	if err != nil {
		return types.DepositRecord{}, err
	}
	accrued := pool.FeePerSlotShare
	if accrued <= bond.FeeDebtSats {
		return types.DepositRecord{}, fmt.Errorf("no shielder fees claimable")
	}
	claimSats := accrued - bond.FeeDebtSats
	if pool.TotalClaimedSats > pool.TotalCollectedSats || claimSats > pool.TotalCollectedSats-pool.TotalClaimedSats {
		return types.DepositRecord{}, fmt.Errorf("shielder fee claim exceeds available fees")
	}
	if claimSats <= withdrawalFeeSats(ctx, k, claimSats) {
		return types.DepositRecord{}, fmt.Errorf("fee claim amount must be redeemable after withdrawal fee")
	}
	depositID, err := shielderFeeDepositID(nodePubKey, owner, accrued, ctx.BlockHeight())
	if err != nil {
		return types.DepositRecord{}, err
	}
	deposit := types.DepositRecord{
		DepositID:      depositID,
		Owner:          owner,
		AmountSats:     claimSats,
		DepositAddress: common.NoopAddress,
		MatchedHeight:  ctx.BlockHeight(),
		Status:         types.DepositStatusSettled,
		Settlement:     types.DepositSettlementOperatorFee,
	}
	noteCommitments, err := parseShielderNoteCommitments(commitments, deposit.AmountSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	noteCommitments, floorRemainder, err := applyShielderNoteFloor(ctx, k, noteCommitments, deposit.AmountSats, false)
	if err != nil {
		return types.DepositRecord{}, err
	}
	if floorRemainder > 0 {
		deposit.AmountSats -= floorRemainder
		claimSats = deposit.AmountSats
	}
	notePubKeys, err := parseShielderFeeNotePubKeys(feeNotePubKeys, len(noteCommitments))
	if err != nil {
		return types.DepositRecord{}, err
	}
	var total uint64
	for _, note := range noteCommitments {
		total += note.DenominationSats
	}
	if total != deposit.AmountSats {
		return types.DepositRecord{}, fmt.Errorf("shielder fee commitment denominations do not match claim amount")
	}
	authPayload := shielderFeeClaimPayload(nodePubKey, owner, accrued, pool.FeePerSlotShare, noteCommitments, notePubKeys)
	if err := verifySecp256K1SignaturePayload(bond.OperatorPubKey, operatorSignature, authPayload); err != nil {
		return types.DepositRecord{}, fmt.Errorf("invalid shielder fee operator signature: %w", err)
	}
	seen := make(map[string]struct{}, len(noteCommitments))
	byDenomination := make(map[uint64][]string)
	for idx, note := range noteCommitments {
		if note.Commitment == "" {
			return types.DepositRecord{}, fmt.Errorf("empty shielder fee commitment")
		}
		if _, ok := seen[note.Commitment]; ok {
			return types.DepositRecord{}, fmt.Errorf("duplicate shielder fee commitment")
		}
		if k.ShielderCommitmentExists(ctx, note.Commitment) {
			return types.DepositRecord{}, fmt.Errorf("shielder fee commitment already exists")
		}
		if k.ShielderFeeNotePubKeyUsed(ctx, notePubKeys[idx]) {
			return types.DepositRecord{}, fmt.Errorf("shielder fee note pubkey already used")
		}
		seen[note.Commitment] = struct{}{}
		if err := k.SetShielderCommitment(ctx, note.Commitment); err != nil {
			return types.DepositRecord{}, err
		}
		if err := k.SetShielderDenominationCommitment(ctx, note.DenominationSats, note.Commitment); err != nil {
			return types.DepositRecord{}, err
		}
		if err := k.SetShielderFeeNotePubKey(ctx, notePubKeys[idx]); err != nil {
			return types.DepositRecord{}, err
		}
		if err := k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
			Commitment:       note.Commitment,
			DenominationSats: note.DenominationSats,
		}); err != nil {
			return types.DepositRecord{}, err
		}
		byDenomination[note.DenominationSats] = append(byDenomination[note.DenominationSats], note.Commitment)
	}
	if err := refreshShielderRoots(ctx, k, byDenomination); err != nil {
		return types.DepositRecord{}, err
	}
	bond.FeeDebtSats = accrued
	bond.UpdatedHeight = ctx.BlockHeight()
	pool.TotalClaimedSats += claimSats
	deposit.Status = types.DepositStatusCommitted
	if err := k.SetDepositRecord(ctx, deposit); err != nil {
		return types.DepositRecord{}, err
	}
	if err := k.SetShielderNodeBond(ctx, bond); err != nil {
		return types.DepositRecord{}, err
	}
	if err := k.SetFeePool(ctx, pool); err != nil {
		return types.DepositRecord{}, err
	}
	if floorRemainder > 0 {
		if err := addWithdrawalFee(ctx, k, floorRemainder); err != nil {
			return types.DepositRecord{}, err
		}
	}
	return deposit, nil
}

func shielderFeeDepositID(nodePubKey string, owner cosmos.AccAddress, accrued uint64, height int64) (common.TxID, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("thornado:fee-shield:v1|%s|%s|%d|%d", nodePubKey, owner.String(), accrued, height)))
	return common.NewTxID(strings.ToUpper(hex.EncodeToString(sum[:])))
}

func parseShielderFeeNotePubKeys(raw []string, expected int) ([]string, error) {
	if len(raw) != expected {
		return nil, fmt.Errorf("shielder fee note pubkey count mismatch")
	}
	result := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		pubKey := strings.TrimSpace(item)
		if pubKey == "" {
			return nil, fmt.Errorf("missing shielder fee note pubkey")
		}
		if _, err := hex.DecodeString(pubKey); err != nil || len(pubKey) != 66 {
			return nil, fmt.Errorf("invalid shielder fee note pubkey")
		}
		if _, ok := seen[pubKey]; ok {
			return nil, fmt.Errorf("duplicate shielder fee note pubkey")
		}
		seen[pubKey] = struct{}{}
		result = append(result, pubKey)
	}
	return result, nil
}

func shielderFeeClaimPayload(nodePubKey string, owner cosmos.AccAddress, accrued, feePerSlotShare uint64, notes []shielderNoteCommitment, notePubKeys []string) []byte {
	parts := []string{
		"thornado:fee-claim:v1",
		nodePubKey,
		owner.String(),
		fmt.Sprintf("%d", accrued),
		fmt.Sprintf("%d", feePerSlotShare),
	}
	for idx, note := range notes {
		parts = append(parts, fmt.Sprintf("%d:%s:%s", note.DenominationSats, note.Commitment, strings.TrimSpace(notePubKeys[idx])))
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

func parseShielderRedeemPublicInputs(raw json.RawMessage) (shielderRedeemPublicInputs, error) {
	if err := ValidateShielderRedeemPublicJSON(raw); err != nil {
		return shielderRedeemPublicInputs{}, err
	}
	var publicInputs shielderRedeemPublicInputs
	if err := json.Unmarshal(raw, &publicInputs); err != nil {
		return publicInputs, fmt.Errorf("invalid shielder public inputs: %w", err)
	}
	return publicInputs, nil
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
	gasRate := mgr.Keeper().GetConfigInt64(ctx, constants.BTC_DefaultSatsPerVByte)
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
		ModuleName:     BaseName,
		VaultPathIndex: pathIndex,
		TxType:         txType,
	}
	ctx.Logger().Info("queued bitcoin vault path sweep",
		"from", sourceAddr.String(),
		"to", currentRoot.String(),
		"vault_pub_key", sourcePubKey.String(),
		"path_index", pathIndex,
		"amount", amount.String(),
	)
	return mgr.Keeper().AppendTxOut(ctx, ctx.BlockHeight(), item)
}

func currentBTCVaultAddress(ctx cosmos.Context, k keeper.Keeper) (Vault, common.Address, error) {
	vaults, err := k.GetBaseVaultsByStatus(ctx, ActiveVault)
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

func shielderRedeemID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
