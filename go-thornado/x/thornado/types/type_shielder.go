package types

import (
	"fmt"
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

const (
	DepositStatusAddressIssued   = "address_issued"
	DepositStatusDepositObserved = "deposit_observed"
	DepositStatusDepositMatched  = "deposit_matched"
	DepositStatusReturnQueued    = "return_queued"
	DepositStatusReturnComplete  = "return_complete"
	DepositStatusSettled         = "settled"
	DepositStatusCommitted       = "committed"
	DepositStatusKeysignQueued   = "keysign_queued"
	DepositStatusErrata          = "errata"
)

const (
	ShielderRedeemStatusAuthorized = "authorized"
	ShielderRedeemStatusSettled    = "settled"
)

const (
	DepositSettlementUser         = "user"
	DepositSettlementOperatorBond = "operator_bond"
	DepositSettlementOperatorSale = "operator_sale"
	DepositSettlementOperatorFee  = "operator_fee"
)

const (
	NodeSlotAuctionOpen     = "open"
	NodeSlotAuctionSelected = "selected"
	NodeSlotAuctionExpired  = "expired"
	NodeSlotAuctionSettled  = "settled"
)

const (
	ShielderRedeemPolicyUserBTC    = "user_btc"
	ShielderRedeemPolicyBondEscrow = "bond_escrow"
	ShielderRedeemPolicyBidDeposit = "bid_deposit"
)

type DepositSession struct {
	Owner                    cosmos.AccAddress `json:"owner"`
	PowToken                 string            `json:"pow_token"`
	DepositAddress           common.Address    `json:"deposit_address"`
	VaultPubKey              common.PubKey     `json:"vault_pub_key"`
	DepositPathIndex         uint64            `json:"deposit_path_index"`
	DepositPath              string            `json:"deposit_path,omitempty"`
	DepositPathType          string            `json:"deposit_path_type,omitempty"`
	DepositNonce             uint64            `json:"deposit_nonce,omitempty"`
	OperatorPubKey           common.PubKey     `json:"operator_pub_key,omitempty"`
	NodePubKey               string            `json:"node_pub_key,omitempty"`
	AuctionID                string            `json:"auction_id,omitempty"`
	CreatedHeight            int64             `json:"created_height"`
	ExpiresAtHeight          int64             `json:"expires_at_height,omitempty"`
	PurgeAtHeight            int64             `json:"purge_at_height,omitempty"`
	RefundEligibleHeight     int64             `json:"refund_eligible_height,omitempty"`
	PowDurationMs            uint64            `json:"pow_duration_ms,omitempty"`
	PowDifficulty            int64             `json:"pow_difficulty,omitempty"`
	Status                   string            `json:"status"`
	DepositID                common.TxID       `json:"deposit_id,omitempty"`
	InboundTxID              common.TxID       `json:"inbound_tx_id,omitempty"`
	BTCConfirmations         int64             `json:"btc_confirmations,omitempty"`
	BTCConfirmationsRequired int64             `json:"btc_confirmations_required,omitempty"`
	BTCObservedHeight        int64             `json:"btc_observed_height,omitempty"`
}

func (m DepositSession) Key() string {
	return m.Owner.String()
}

func (m DepositSession) Valid() error {
	if m.Owner.Empty() {
		return fmt.Errorf("missing deposit owner")
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing deposit pow token")
	}
	if m.DepositAddress.IsEmpty() {
		return fmt.Errorf("missing deposit address")
	}
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing deposit vault pubkey")
	}
	return nil
}

type DepositRecord struct {
	DepositID                common.TxID       `json:"deposit_id"`
	Owner                    cosmos.AccAddress `json:"owner"`
	AmountSats               uint64            `json:"amount_sats"`
	ShieldedSats             uint64            `json:"shielded_sats,omitempty"`
	DepositAddress           common.Address    `json:"deposit_address"`
	ReturnAddress            common.Address    `json:"return_address,omitempty"`
	VaultPubKey              common.PubKey     `json:"vault_pub_key"`
	DepositPathIndex         uint64            `json:"deposit_path_index"`
	DepositPath              string            `json:"deposit_path,omitempty"`
	DepositPathType          string            `json:"deposit_path_type,omitempty"`
	DepositNonce             uint64            `json:"deposit_nonce,omitempty"`
	OperatorPubKey           common.PubKey     `json:"operator_pub_key,omitempty"`
	NodePubKey               string            `json:"node_pub_key,omitempty"`
	AuctionID                string            `json:"auction_id,omitempty"`
	Settlement               string            `json:"settlement,omitempty"`
	SellerPayoutSats         uint64            `json:"seller_payout_sats,omitempty"`
	ProtocolBondSats         uint64            `json:"protocol_bond_sats,omitempty"`
	NodeSlot                 uint64            `json:"node_slot,omitempty"`
	BondConfirmed            bool              `json:"bond_confirmed,omitempty"`
	MatchedHeight            int64             `json:"matched_height"`
	CreatedHeight            int64             `json:"created_height,omitempty"`
	ExpiresAtHeight          int64             `json:"expires_at_height,omitempty"`
	PurgeAtHeight            int64             `json:"purge_at_height,omitempty"`
	RefundEligibleHeight     int64             `json:"refund_eligible_height,omitempty"`
	RefundQueuedHeight       int64             `json:"refund_queued_height,omitempty"`
	PowDurationMs            uint64            `json:"pow_duration_ms,omitempty"`
	PowDifficulty            int64             `json:"pow_difficulty,omitempty"`
	Status                   string            `json:"status"`
	InboundTxID              common.TxID       `json:"inbound_tx_id,omitempty"`
	BTCConfirmations         int64             `json:"btc_confirmations,omitempty"`
	BTCConfirmationsRequired int64             `json:"btc_confirmations_required,omitempty"`
	BTCObservedHeight        int64             `json:"btc_observed_height,omitempty"`
}

func (m DepositRecord) Key() string {
	return m.DepositID.String()
}

func (m DepositRecord) IsNodeBond() bool {
	return strings.TrimSpace(m.NodePubKey) != ""
}

func (m DepositRecord) Valid() error {
	if m.DepositID.IsEmpty() {
		return fmt.Errorf("missing deposit id")
	}
	if m.Owner.Empty() {
		return fmt.Errorf("missing deposit owner")
	}
	if m.AmountSats == 0 {
		return fmt.Errorf("missing deposit amount")
	}
	if m.ShieldedSats > m.AmountSats {
		return fmt.Errorf("shielded amount exceeds deposit amount")
	}
	if m.DepositAddress.IsEmpty() {
		return fmt.Errorf("missing deposit address")
	}
	if m.VaultPubKey.IsEmpty() && !m.DepositAddress.IsNoop() {
		return fmt.Errorf("missing deposit vault pubkey")
	}
	return nil
}

type DepositAddress struct {
	Address         common.Address    `json:"address"`
	VaultPubKey     common.PubKey     `json:"vault_pub_key"`
	PathIndex       uint64            `json:"path_index"`
	Path            string            `json:"path,omitempty"`
	PathType        string            `json:"path_type,omitempty"`
	DepositNonce    uint64            `json:"deposit_nonce,omitempty"`
	Owner           cosmos.AccAddress `json:"owner"`
	PowToken        string            `json:"pow_token"`
	OperatorPubKey  common.PubKey     `json:"operator_pub_key,omitempty"`
	NodePubKey      string            `json:"node_pub_key,omitempty"`
	AuctionID       string            `json:"auction_id,omitempty"`
	CreatedHeight   int64             `json:"created_height"`
	ExpiresAtHeight int64             `json:"expires_at_height,omitempty"`
	PurgeAtHeight   int64             `json:"purge_at_height,omitempty"`
	PowDurationMs   uint64            `json:"pow_duration_ms,omitempty"`
	PowDifficulty   int64             `json:"pow_difficulty,omitempty"`
}

type DepositPowTiming struct {
	PowToken          string            `json:"pow_token"`
	Owner             cosmos.AccAddress `json:"owner"`
	DurationMs        uint64            `json:"duration_ms"`
	Difficulty        int64             `json:"difficulty"`
	CreatedHeight     int64             `json:"created_height"`
	DepositID         common.TxID       `json:"deposit_id,omitempty"`
	DepositAmountSats uint64            `json:"deposit_amount_sats,omitempty"`
	MatchedHeight     int64             `json:"matched_height,omitempty"`
	Deposited         bool              `json:"deposited,omitempty"`
}

func (m DepositPowTiming) Key() string {
	return strings.TrimSpace(m.PowToken)
}

func (m DepositPowTiming) Valid() error {
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing deposit pow token")
	}
	if m.Owner.Empty() {
		return fmt.Errorf("missing deposit pow owner")
	}
	if m.Difficulty < 0 {
		return fmt.Errorf("invalid deposit pow difficulty")
	}
	return nil
}

type DepositPowDifficultyState struct {
	Difficulty         int64  `json:"difficulty"`
	LastRetargetHeight int64  `json:"last_retarget_height"`
	SampleCount        uint64 `json:"sample_count,omitempty"`
	WeightedP90Ms      uint64 `json:"weighted_p90_ms,omitempty"`
	UpdatedHeight      int64  `json:"updated_height,omitempty"`
}

func (m DepositAddress) Key() string {
	return m.Address.String()
}

type ShielderNodeBond struct {
	NodePubKey          string            `json:"node_pub_key"`
	OperatorPubKey      common.PubKey     `json:"operator_pub_key"`
	NodeAddress         cosmos.AccAddress `json:"node_address"`
	Slot                uint64            `json:"slot"`
	PendingSats         uint64            `json:"pending_sats,omitempty"`
	BondSats            uint64            `json:"bond_sats"`
	FeeDebtSats         uint64            `json:"fee_debt_sats,omitempty"`
	FeeShareActive      bool              `json:"fee_share_active,omitempty"`
	PendingFeeDepositID common.TxID       `json:"pending_fee_deposit_id,omitempty"`
	Sold                bool              `json:"sold,omitempty"`
	SoldAuctionID       string            `json:"sold_auction_id,omitempty"`
	CreatedHeight       int64             `json:"created_height"`
	UpdatedHeight       int64             `json:"updated_height"`
}

type NodeSlotAuction struct {
	AuctionID            string            `json:"auction_id"`
	Seller               cosmos.AccAddress `json:"seller"`
	SellerOperatorPubKey common.PubKey     `json:"seller_operator_pub_key"`
	SellerNodePubKey     string            `json:"seller_node_pub_key"`
	Slot                 uint64            `json:"slot"`
	OriginalBondSats     uint64            `json:"original_bond_sats"`
	ReserveSats          uint64            `json:"reserve_sats"`
	ExpiryHeight         int64             `json:"expiry_height"`
	SelectedBidID        string            `json:"selected_bid_id,omitempty"`
	Status               string            `json:"status"`
	CreatedHeight        int64             `json:"created_height"`
	UpdatedHeight        int64             `json:"updated_height"`
}

func (m NodeSlotAuction) Key() string {
	return strings.TrimSpace(m.AuctionID)
}

func (m NodeSlotAuction) Valid() error {
	if strings.TrimSpace(m.AuctionID) == "" {
		return fmt.Errorf("missing node slot auction id")
	}
	if m.Seller.Empty() {
		return fmt.Errorf("missing node slot auction seller")
	}
	if m.SellerOperatorPubKey.IsEmpty() {
		return fmt.Errorf("missing node slot auction seller operator pubkey")
	}
	if strings.TrimSpace(m.SellerNodePubKey) == "" {
		return fmt.Errorf("missing node slot auction seller node pubkey")
	}
	if m.OriginalBondSats == 0 {
		return fmt.Errorf("missing node slot auction original bond")
	}
	if m.ExpiryHeight <= 0 {
		return fmt.Errorf("missing node slot auction expiry")
	}
	if strings.TrimSpace(m.Status) == "" {
		return fmt.Errorf("missing node slot auction status")
	}
	return nil
}

type NodeSlotBid struct {
	BidID          string            `json:"bid_id"`
	AuctionID      string            `json:"auction_id"`
	Bidder         cosmos.AccAddress `json:"bidder"`
	OperatorPubKey common.PubKey     `json:"operator_pub_key"`
	NodePubKey     string            `json:"node_pub_key"`
	DepositAddress common.Address    `json:"deposit_address,omitempty"`
	DepositID      common.TxID       `json:"deposit_id,omitempty"`
	AmountSats     uint64            `json:"amount_sats,omitempty"`
	Selected       bool              `json:"selected,omitempty"`
	Settled        bool              `json:"settled,omitempty"`
	CreatedHeight  int64             `json:"created_height"`
	UpdatedHeight  int64             `json:"updated_height"`
}

func (m NodeSlotBid) Key() string {
	return strings.TrimSpace(m.BidID)
}

func (m NodeSlotBid) Valid() error {
	if strings.TrimSpace(m.BidID) == "" {
		return fmt.Errorf("missing node slot bid id")
	}
	if strings.TrimSpace(m.AuctionID) == "" {
		return fmt.Errorf("missing node slot bid auction id")
	}
	if m.Bidder.Empty() {
		return fmt.Errorf("missing node slot bid bidder")
	}
	if m.OperatorPubKey.IsEmpty() {
		return fmt.Errorf("missing node slot bid operator pubkey")
	}
	if strings.TrimSpace(m.NodePubKey) == "" {
		return fmt.Errorf("missing node slot bid node pubkey")
	}
	return nil
}

func (m ShielderNodeBond) Key() string {
	return strings.TrimSpace(m.NodePubKey)
}

func (m ShielderNodeBond) Valid() error {
	if strings.TrimSpace(m.NodePubKey) == "" {
		return fmt.Errorf("missing shielder node pubkey")
	}
	if m.OperatorPubKey.IsEmpty() {
		return fmt.Errorf("missing shielder node operator pubkey")
	}
	if m.NodeAddress.Empty() {
		return fmt.Errorf("missing shielder node node address")
	}
	return nil
}

type FeePool struct {
	PendingSats        uint64 `json:"pending_sats,omitempty"`
	TotalSlots         uint64 `json:"total_slots"`
	FeePerSlotShare    uint64 `json:"fee_per_slot_share"`
	TotalCollectedSats uint64 `json:"total_collected_sats"`
	TotalClaimedSats   uint64 `json:"total_claimed_sats"`
}

func (m DepositAddress) Valid() error {
	if m.Address.IsEmpty() {
		return fmt.Errorf("missing deposit address")
	}
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing deposit address vault pubkey")
	}
	if m.PathIndex == common.MainVaultPathIndex {
		return fmt.Errorf("deposit address path index cannot be zero")
	}
	if m.Owner.Empty() {
		return fmt.Errorf("missing deposit address owner")
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing deposit address pow token")
	}
	return nil
}

type StoredShielderNoteRecord struct {
	Commitment       string `json:"commitment"`
	DenominationSats uint64 `json:"denomination_sats"`
}

func (m StoredShielderNoteRecord) Key() string {
	return strings.TrimSpace(m.Commitment)
}

func (m StoredShielderNoteRecord) Valid() error {
	if strings.TrimSpace(m.Commitment) == "" {
		return fmt.Errorf("missing shielder note commitment")
	}
	if m.DenominationSats == 0 {
		return fmt.Errorf("missing shielder note denomination")
	}
	return nil
}

type ShielderRedeem struct {
	WithdrawalID    string         `json:"withdrawal_id"`
	NullifierHash   string         `json:"nullifier_hash"`
	MerkleRoot      string         `json:"merkle_root"`
	Recipient       common.Address `json:"recipient"`
	RecipientPolicy string         `json:"recipient_policy,omitempty"`
	BidID           string         `json:"bid_id,omitempty"`
	NodePubKey      string         `json:"node_pub_key,omitempty"`
	AmountSats      uint64         `json:"amount_sats"`
	FeeSats         uint64         `json:"fee_sats"`
	InHash          common.TxID    `json:"in_hash"`
	VaultPubKey     common.PubKey  `json:"vault_pub_key"`
	RequestedHeight int64          `json:"requested_height"`
	Status          string         `json:"status"`
}

func (m ShielderRedeem) Key() string {
	return m.WithdrawalID
}

func (m ShielderRedeem) Valid() error {
	if strings.TrimSpace(m.WithdrawalID) == "" {
		return fmt.Errorf("missing shielder redeem id")
	}
	if strings.TrimSpace(m.NullifierHash) == "" {
		return fmt.Errorf("missing shielder nullifier hash")
	}
	if strings.TrimSpace(m.MerkleRoot) == "" {
		return fmt.Errorf("missing shielder merkle root")
	}
	if m.Recipient.IsEmpty() {
		return fmt.Errorf("missing shielder redeem recipient")
	}
	if !m.Recipient.IsBondEscrow() && !m.Recipient.GetChain().Equals(common.BTCChain) {
		return fmt.Errorf("shielder redeem recipient must be bitcoin or bond escrow")
	}
	if m.AmountSats == 0 {
		return fmt.Errorf("missing shielder redeem amount")
	}
	if m.FeeSats >= m.AmountSats {
		return fmt.Errorf("shielder redeem fee exceeds amount")
	}
	if m.InHash.IsEmpty() {
		return fmt.Errorf("missing shielder redeem in hash")
	}
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing shielder redeem vault pubkey")
	}
	return nil
}
