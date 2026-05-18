package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// ObservedTxs a list of ObservedTx
type ObservedTxs []ObservedTx

// NewObservedTx create a new instance of ObservedTx
func NewObservedTx(tx Tx, height int64, pk PubKey, finalisedHeight int64) ObservedTx {
	return ObservedTx{
		Tx:             tx,
		Status:         Status_incomplete,
		BlockHeight:    height,
		ObservedPubKey: pk,
		FinaliseHeight: finalisedHeight,
	}
}

// Valid check whether the observed tx represent valid information
func (m *ObservedTx) Valid() error {
	if err := m.Tx.Valid(); err != nil {
		return err
	}
	// Memo should not be empty, but it can't be checked here, because a
	// message failed validation will be rejected by THORNode.
	// Thus THORNode can't refund customer accordingly , which will result fund lost
	if m.BlockHeight <= 0 {
		return errors.New("block height can't be zero")
	}
	if m.ObservedPubKey.IsEmpty() {
		return errors.New("observed pool pubkey is empty")
	}
	if m.FinaliseHeight <= 0 {
		return errors.New("finalise block height can't be zero")
	}
	return nil
}

// IsEmpty check whether the Tx is empty
func (m *ObservedTx) IsEmpty() bool {
	return m.Tx.IsEmpty()
}

// Equals compare two ObservedTx
func (m ObservedTx) Equals(tx2 ObservedTx) bool {
	if !m.Tx.EqualsEx(tx2.Tx) {
		return false
	}
	if !m.ObservedPubKey.Equals(tx2.ObservedPubKey) {
		return false
	}
	if m.BlockHeight != tx2.BlockHeight {
		return false
	}
	if m.FinaliseHeight != tx2.FinaliseHeight {
		return false
	}
	if !strings.EqualFold(m.Aggregator, tx2.Aggregator) {
		return false
	}
	if !strings.EqualFold(m.AggregatorTarget, tx2.AggregatorTarget) {
		return false
	}
	emptyAmt := cosmos.ZeroUint()
	if m.AggregatorTargetLimit == nil {
		m.AggregatorTargetLimit = &emptyAmt
	}
	if tx2.AggregatorTargetLimit == nil {
		tx2.AggregatorTargetLimit = &emptyAmt
	}
	if !m.AggregatorTargetLimit.Equal(*tx2.AggregatorTargetLimit) {
		return false
	}
	return true
}

// IsFinal indicates whether ObserveTx is final.
func (m *ObservedTx) IsFinal() bool {
	return m.FinaliseHeight == m.BlockHeight
}

func (m *ObservedTx) GetOutHashes() TxIDs {
	txIDs := make(TxIDs, 0)
	for _, o := range m.OutHashes {
		txID, err := NewTxID(o)
		if err != nil {
			continue
		}
		txIDs = append(txIDs, txID)
	}
	return txIDs
}

// GetSigners return all the node address that had sign the tx
func (m *ObservedTx) GetSigners() []cosmos.AccAddress {
	addrs := make([]cosmos.AccAddress, 0)
	for _, a := range m.Signers {
		addr, err := cosmos.AccAddressFromBech32(a)
		if err != nil {
			continue
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

// String implement fmt.Stringer
func (m *ObservedTx) String() string {
	return m.Tx.String()
}

// HasSigned - check if given address has signed
func (m *ObservedTx) HasSigned(signer cosmos.AccAddress) bool {
	for _, sign := range m.GetSigners() {
		if sign.Equals(signer) {
			return true
		}
	}
	return false
}

// Sign add the given node account to signers list
// if the given signer is already in the list, it will return false, otherwise true
func (m *ObservedTx) Sign(signer cosmos.AccAddress) bool {
	if m.HasSigned(signer) {
		return false
	}
	m.Signers = append(m.Signers, signer.String())
	return true
}

// SetDone check the ObservedTx status, update it's status to done if the outbound tx had been processed
func (m *ObservedTx) SetDone(hash TxID, numOuts int) {
	// As an Asset->RUNE affiliate fee could also be RUNE,
	// allow multiple blank TxID OutHashes.
	// SetDone is still expected to only be called once (per ObservedTx) for each.
	if !hash.Equals(BlankTxID) {
		for _, done := range m.GetOutHashes() {
			if done.Equals(hash) {
				return
			}
		}
	}
	m.OutHashes = append(m.OutHashes, hash.String())
	if m.IsDone(numOuts) {
		m.Status = Status_done
	}
}

// IsDone will only return true when the number of out hashes is larger or equals the input number
func (m *ObservedTx) IsDone(numOuts int) bool {
	return len(m.OutHashes) >= numOuts
}

// MarshalJSON marshal Status to JSON in string form
func (x Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(x.String())
}

// UnmarshalJSON convert string form back to Status
func (x *Status) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if val, ok := Status_value[s]; ok {
		*x = Status(val)
		return nil
	}
	return fmt.Errorf("%s is not a valid status", s)
}

func (txs ObservedTxs) Contains(tx ObservedTx) bool {
	for _, item := range txs {
		if item.Equals(tx) {
			return true
		}
	}
	return false
}
