package types

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

const (
	ShielderStatusAddressIssued  = "address_issued"
	ShielderStatusDepositMatched = "deposit_matched"
	ShielderStatusSettled        = "settled"
	ShielderStatusCommitted      = "committed"
	ShielderStatusKeysignQueued  = "keysign_queued"
)

const (
	ShielderSettlementUser         = "user"
	ShielderSettlementOperatorBond = "operator_bond"
	ShielderSettlementOperatorSale = "operator_sale"
	ShielderSettlementOperatorFee  = "operator_fee"
)

const (
	NodeSlotAuctionOpen     = "open"
	NodeSlotAuctionSelected = "selected"
	NodeSlotAuctionExpired  = "expired"
	NodeSlotAuctionSettled  = "settled"
)

type ShielderSession struct {
	Owner            cosmos.AccAddress `json:"owner"`
	PowToken         string            `json:"pow_token"`
	DepositAddress   common.Address    `json:"deposit_address"`
	VaultPubKey      common.PubKey     `json:"vault_pub_key"`
	DepositPathIndex uint64            `json:"deposit_path_index"`
	OperatorPubKey   common.PubKey     `json:"operator_pub_key,omitempty"`
	NodePubKey       string            `json:"node_pub_key,omitempty"`
	AuctionID        string            `json:"auction_id,omitempty"`
	CreatedHeight    int64             `json:"created_height"`
	Status           string            `json:"status"`
	DepositID        common.TxID       `json:"deposit_id,omitempty"`
}

func (m ShielderSession) Key() string {
	return m.Owner.String()
}

func (m ShielderSession) Valid() error {
	if m.Owner.Empty() {
		return fmt.Errorf("missing shielder owner")
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing shielder pow token")
	}
	if m.DepositAddress.IsEmpty() {
		return fmt.Errorf("missing shielder deposit address")
	}
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing shielder vault pubkey")
	}
	return nil
}

type ShielderDeposit struct {
	DepositID        common.TxID       `json:"deposit_id"`
	Owner            cosmos.AccAddress `json:"owner"`
	AmountSats       uint64            `json:"amount_sats"`
	DepositAddress   common.Address    `json:"deposit_address"`
	VaultPubKey      common.PubKey     `json:"vault_pub_key"`
	DepositPathIndex uint64            `json:"deposit_path_index"`
	OperatorPubKey   common.PubKey     `json:"operator_pub_key,omitempty"`
	NodePubKey       string            `json:"node_pub_key,omitempty"`
	AuctionID        string            `json:"auction_id,omitempty"`
	Settlement       string            `json:"settlement,omitempty"`
	SellerPayoutSats uint64            `json:"seller_payout_sats,omitempty"`
	ProtocolBondSats uint64            `json:"protocol_bond_sats,omitempty"`
	NodeSlot         uint64            `json:"node_slot,omitempty"`
	BondConfirmed    bool              `json:"bond_confirmed,omitempty"`
	MatchedHeight    int64             `json:"matched_height"`
	Status           string            `json:"status"`
	Commitments      []string          `json:"commitments,omitempty"`
}

func (m ShielderDeposit) Key() string {
	return m.DepositID.String()
}

func (m ShielderDeposit) IsNodeBond() bool {
	return strings.TrimSpace(m.NodePubKey) != ""
}

func (m ShielderDeposit) Valid() error {
	if m.DepositID.IsEmpty() {
		return fmt.Errorf("missing shielder deposit id")
	}
	if m.Owner.Empty() {
		return fmt.Errorf("missing shielder deposit owner")
	}
	if m.AmountSats == 0 {
		return fmt.Errorf("missing shielder deposit amount")
	}
	if m.DepositAddress.IsEmpty() {
		return fmt.Errorf("missing shielder deposit address")
	}
	if m.VaultPubKey.IsEmpty() && !m.DepositAddress.IsNoop() {
		return fmt.Errorf("missing shielder deposit vault pubkey")
	}
	return nil
}

type ShielderDepositAddress struct {
	Address        common.Address    `json:"address"`
	VaultPubKey    common.PubKey     `json:"vault_pub_key"`
	PathIndex      uint64            `json:"path_index"`
	Owner          cosmos.AccAddress `json:"owner"`
	PowToken       string            `json:"pow_token"`
	OperatorPubKey common.PubKey     `json:"operator_pub_key,omitempty"`
	NodePubKey     string            `json:"node_pub_key,omitempty"`
	AuctionID      string            `json:"auction_id,omitempty"`
	CreatedHeight  int64             `json:"created_height"`
}

func (m ShielderDepositAddress) Key() string {
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

type ShielderFeePool struct {
	PendingSats        uint64 `json:"pending_sats,omitempty"`
	TotalSlots         uint64 `json:"total_slots"`
	FeePerSlotShare    uint64 `json:"fee_per_slot_share"`
	TotalCollectedSats uint64 `json:"total_collected_sats"`
	TotalClaimedSats   uint64 `json:"total_claimed_sats"`
}

func (m ShielderDepositAddress) Valid() error {
	if m.Address.IsEmpty() {
		return fmt.Errorf("missing shielder deposit address")
	}
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing shielder deposit address vault pubkey")
	}
	if m.PathIndex == common.MainVaultPathIndex {
		return fmt.Errorf("shielder deposit address path index cannot be zero")
	}
	if m.Owner.Empty() {
		return fmt.Errorf("missing shielder deposit address owner")
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing shielder deposit address pow token")
	}
	return nil
}

type ShielderWithdrawal struct {
	WithdrawalID    string            `json:"withdrawal_id"`
	Owner           cosmos.AccAddress `json:"owner"`
	NullifierHash   string            `json:"nullifier_hash"`
	MerkleRoot      string            `json:"merkle_root"`
	Recipient       common.Address    `json:"recipient"`
	AmountSats      uint64            `json:"amount_sats"`
	FeeSats         uint64            `json:"fee_sats"`
	InHash          common.TxID       `json:"in_hash"`
	VaultPubKey     common.PubKey     `json:"vault_pub_key"`
	RequestedHeight int64             `json:"requested_height"`
	Status          string            `json:"status"`
	Proof           json.RawMessage   `json:"proof,omitempty"`
	Public          json.RawMessage   `json:"public,omitempty"`
}

func (m ShielderWithdrawal) Key() string {
	return m.WithdrawalID
}

func (m ShielderWithdrawal) Valid() error {
	if strings.TrimSpace(m.WithdrawalID) == "" {
		return fmt.Errorf("missing shielder withdrawal id")
	}
	if m.Owner.Empty() {
		return fmt.Errorf("missing shielder withdrawal owner")
	}
	if strings.TrimSpace(m.NullifierHash) == "" {
		return fmt.Errorf("missing shielder nullifier hash")
	}
	if strings.TrimSpace(m.MerkleRoot) == "" {
		return fmt.Errorf("missing shielder merkle root")
	}
	if m.Recipient.IsEmpty() {
		return fmt.Errorf("missing shielder withdrawal recipient")
	}
	if m.AmountSats == 0 {
		return fmt.Errorf("missing shielder withdrawal amount")
	}
	if m.FeeSats >= m.AmountSats {
		return fmt.Errorf("shielder withdrawal fee exceeds amount")
	}
	if m.InHash.IsEmpty() {
		return fmt.Errorf("missing shielder withdrawal in hash")
	}
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing shielder withdrawal vault pubkey")
	}
	return nil
}
