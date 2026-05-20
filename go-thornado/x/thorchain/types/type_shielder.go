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
	ShielderStatusCommitted      = "committed"
	ShielderStatusKeysignQueued  = "keysign_queued"
)

type ShielderSession struct {
	Owner          cosmos.AccAddress `json:"owner"`
	PowToken       string            `json:"pow_token"`
	DepositAddress common.Address    `json:"deposit_address"`
	VaultPubKey    common.PubKey     `json:"vault_pub_key"`
	CreatedHeight  int64             `json:"created_height"`
	Status         string            `json:"status"`
	DepositID      common.TxID       `json:"deposit_id,omitempty"`
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
	DepositID      common.TxID       `json:"deposit_id"`
	Owner          cosmos.AccAddress `json:"owner"`
	AmountSats     uint64            `json:"amount_sats"`
	DepositAddress common.Address    `json:"deposit_address"`
	VaultPubKey    common.PubKey     `json:"vault_pub_key"`
	MatchedHeight  int64             `json:"matched_height"`
	Status         string            `json:"status"`
	Commitments    []string          `json:"commitments,omitempty"`
}

func (m ShielderDeposit) Key() string {
	return m.DepositID.String()
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
	if m.VaultPubKey.IsEmpty() {
		return fmt.Errorf("missing shielder deposit vault pubkey")
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
