package types

import (
	"errors"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// ObservedTxVoters a list of observed tx voter
type ObservedTxVoters []ObservedTxVoter

// NewObservedTxVoter create a new instance of ObservedTxVoter
func NewObservedTxVoter(txID common.TxID, txs []common.ObservedTx) ObservedTxVoter {
	observedTxVoter := ObservedTxVoter{
		TxID: txID,
		Txs:  txs,
	}
	return observedTxVoter
}

// Valid check whether the tx is valid , if it is not , then an error will be returned
func (m *ObservedTxVoter) Valid() error {
	if m.TxID.IsEmpty() {
		return errors.New("cannot have an empty tx id")
	}

	// check all other normal tx
	for _, in := range m.Txs {
		if err := in.Valid(); err != nil {
			return err
		}
	}

	return nil
}

// Key is to get the txid
func (m *ObservedTxVoter) Key() common.TxID {
	return m.TxID
}

// String implement fmt.Stringer
func (m *ObservedTxVoter) String() string {
	return m.TxID.String()
}

// matchActionItem is to check the given outboundTx again the list of actions , return true of the outboundTx matched any of the actions
func (m *ObservedTxVoter) matchActionItem(outboundTx common.Tx) bool {
	for _, toi := range m.Actions {
		// note: Coins.Contains will match amount as well
		matchCoin := outboundTx.Coins.Contains(toi.Coin)
		if !matchCoin && toi.Coin.Asset.Equals(toi.Chain.GetGasAsset()) {
			asset := toi.Chain.GetGasAsset()
			intendToSpend := toi.Coin.Amount.Add(toi.MaxGas.ToCoins().GetCoin(asset).Amount)
			actualSpend := outboundTx.Coins.GetCoin(asset).Amount.Add(outboundTx.Gas.ToCoins().GetCoin(asset).Amount)
			if intendToSpend.Equal(actualSpend) {
				matchCoin = true
			}

		}
		// Use GetMemo() to compare memos for memoless outbounds where
		// toi.Memo is empty but toi.OriginalMemo contains the actual memo
		if strings.EqualFold(toi.GetMemo(), outboundTx.Memo) &&
			toi.ToAddress.Equals(outboundTx.ToAddress) &&
			toi.Chain.Equals(outboundTx.Chain) &&
			matchCoin {
			return true
		}
	}
	return false
}

// AddOutTx trying to add the outbound tx into OutTxs ,
// return value false indicate the given outbound tx doesn't match any of the
// actions items , node account should be slashed for a malicious tx
// true indicated the outbound tx matched an action item , and it has been
// added into internal OutTxs
func (m *ObservedTxVoter) AddOutTx(in common.Tx) bool {
	if !m.matchActionItem(in) {
		// no action item match the outbound tx
		return false
	}
	// As an Asset->RUNE affiliate fee could also be RUNE,
	// allow multiple OutTxs with blank TxIDs.
	// AddOutTxs is still expected to only be called once for each.
	if !in.ID.Equals(common.BlankTxID) {
		for _, t := range m.OutTxs {
			if in.ID.Equals(t.ID) {
				return true
			}
		}
	}
	m.OutTxs = append(m.OutTxs, in)
	for i := range m.Txs {
		m.Txs[i].SetDone(in.ID, len(m.Actions))
	}

	if !m.Tx.IsEmpty() {
		m.Tx.SetDone(in.ID, len(m.Actions))
	}

	return true
}

// IsDone check whether THORChain finished process the tx, all outbound tx had
// been sent and observed
func (m *ObservedTxVoter) IsDone() bool {
	return len(m.Actions) <= len(m.OutTxs)
}

// Add is trying to add the given observed tx into the voter , if the signer
// already sign , they will not add twice , it simply return false
func (m *ObservedTxVoter) Add(observedTx common.ObservedTx, signer cosmos.AccAddress) bool {
	// check if this signer has already signed, no take backs allowed
	votedIdx := -1
	for idx, transaction := range m.Txs {
		if !transaction.Equals(observedTx) {
			continue
		}
		votedIdx = idx
		// check whether the signer is already in the list
		for _, siggy := range transaction.GetSigners() {
			if siggy.Equals(signer) {
				return false
			}
		}
	}
	if votedIdx != -1 {
		return m.Txs[votedIdx].Sign(signer)
	}
	observedTx.Signers = []string{signer.String()}
	m.Txs = append(m.Txs, observedTx)
	return true
}

// HasConsensus is to check whether the tx with finalise = false in this ObservedTxVoter reach consensus
// if ObservedTxVoter HasFinalised , then this function will return true as well
func (m *ObservedTxVoter) HasConsensus(nodeAccounts NodeAccounts) bool {
	consensusTx := m.GetTx(nodeAccounts)
	return !consensusTx.IsEmpty()
}

// HasFinalised is to check whether the tx with finalise = true  reach super majority
func (m *ObservedTxVoter) HasFinalised(nodeAccounts NodeAccounts) bool {
	finalTx := m.GetTx(nodeAccounts)
	if finalTx.IsEmpty() {
		return false
	}
	return finalTx.IsFinal()
}

// GetTx return the tx that has super majority
func (m *ObservedTxVoter) GetTx(nodeAccounts NodeAccounts) *common.ObservedTx {
	if !m.Tx.IsEmpty() && m.Tx.IsFinal() {
		return &m.Tx
	}
	finalTx := m.getConsensusTx(nodeAccounts, true)
	if !finalTx.IsEmpty() {
		return &finalTx
	}
	discoverTx := m.getConsensusTx(nodeAccounts, false)
	if !discoverTx.IsEmpty() {
		return &discoverTx
	}
	return &m.Tx
}

func (m *ObservedTxVoter) getConsensusTx(accounts NodeAccounts, final bool) common.ObservedTx {
	for _, txFinal := range m.Txs {
		voters := make(map[string]bool)
		if txFinal.IsFinal() != final {
			continue
		}
		for _, txIn := range m.Txs {
			if txIn.IsFinal() != final {
				continue
			}
			if !txFinal.Tx.EqualsEx(txIn.Tx) {
				continue
			}
			for _, signer := range txIn.GetSigners() {
				_, exist := voters[signer.String()]
				if !exist && accounts.IsNodeKeys(signer) {
					voters[signer.String()] = true
				}
			}
		}
		if HasSuperMajority(len(voters), len(accounts)) {
			return txFinal
		}
	}
	return common.ObservedTx{}
}

// SetReverted set all the tx status to `Reverted` , only when a relevant errata tx had been processed
func (m *ObservedTxVoter) SetReverted() {
	m.setStatus(common.Status_reverted)
	m.Reverted = true
}

func (m *ObservedTxVoter) setStatus(toStatus common.Status) {
	for _, item := range m.Txs {
		item.Status = toStatus
	}
	if !m.Tx.IsEmpty() {
		m.Tx.Status = toStatus
	}
}

// SetDone set all the tx status to `done`
// usually the status will be set to done once the outbound tx get observed and processed
// there are some situation , it doesn't have outbound , those will need to set manually
func (m *ObservedTxVoter) SetDone() {
	m.setStatus(common.Status_done)
}

// Get consensus signers for slash point decrementation
func (m *ObservedTxVoter) GetConsensusSigners() []cosmos.AccAddress {
	if m.Tx.IsEmpty() {
		return nil
	}

	final := m.Tx.IsFinal()
	signersMap := make(map[string]bool)
	var signers []cosmos.AccAddress
	for _, tx := range m.Txs {
		// Only include signers for Txs matching the Tx's finality.
		if tx.IsFinal() != final {
			continue
		}
		if !tx.Tx.EqualsEx(m.Tx.Tx) {
			continue
		}

		for _, signer := range tx.GetSigners() {
			// Use a map to ensure only a single record of each signer.
			// However, do not iterate over the map to get the signers slice,
			// as the slice order would vary between nodes.
			if !signersMap[signer.String()] {
				signers = append(signers, signer)
				signersMap[signer.String()] = true
			}
		}
	}

	return signers
}
