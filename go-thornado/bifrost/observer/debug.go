package observer

import (
	"fmt"
	"sort"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	clienttypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	thornadotypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type DebugObserverOnDeck struct {
	Busy  bool                      `json:"busy,omitempty"`
	Count int                       `json:"count"`
	Items []DebugObserverOnDeckItem `json:"items"`
}

type DebugObserverOnDeckItem struct {
	Chain                  string                `json:"chain"`
	Height                 int64                 `json:"height"`
	Mempool                bool                  `json:"mempool"`
	TxCount                int                   `json:"tx_count"`
	ConfirmationRequired   int64                 `json:"confirmation_required"`
	AllowFutureObservation bool                  `json:"allow_future_observation"`
	Filtered               bool                  `json:"filtered"`
	Txs                    []DebugObserverTxItem `json:"txs"`
}

type DebugObserverAddress struct {
	Chain                  string `json:"chain"`
	Address                string `json:"address"`
	LocalVault             bool   `json:"local_vault"`
	LocalVaultPubKey       string `json:"local_vault_pub_key,omitempty"`
	LocalVaultAddress      string `json:"local_vault_address,omitempty"`
	ThornadoDepositAddress bool   `json:"thornado_deposit_address,omitempty"`
	Error                  string `json:"error,omitempty"`
}

type DebugObserverTxItem struct {
	Tx                   string                   `json:"tx"`
	BlockHeight          int64                    `json:"block_height"`
	From                 string                   `json:"from"`
	To                   string                   `json:"to"`
	Coins                common.Coins             `json:"coins"`
	Gas                  common.Gas               `json:"gas"`
	ObservedVaultPubKey  string                   `json:"observed_vault_pubkey"`
	SourceVout           uint32                   `json:"source_vout"`
	SourceInputs         []clienttypes.TxOutInput `json:"source_inputs,omitempty"`
	CommittedUnFinalised bool                     `json:"committed_pre_final"`
}

type DebugAttestationPerformance struct {
	ActiveValidatorCount int                     `json:"active_validator_count"`
	LocalPeerID          string                  `json:"local_peer_id"`
	LocalAddress         string                  `json:"local_address"`
	LocalPubKey          string                  `json:"local_pub_key"`
	LocalIsActive        bool                    `json:"local_is_active"`
	ObservedTxs          []DebugAttestationState `json:"observed_txs"`
	NetworkFees          []DebugAttestationState `json:"network_fees"`
	Solvencies           []DebugAttestationState `json:"solvencies"`
	ErrataTxs            []DebugAttestationState `json:"errata_txs"`
}

func (o *Observer) DebugOnDeck() DebugObserverOnDeck {
	if !o.lock.TryLock() {
		return DebugObserverOnDeck{Busy: true}
	}
	defer o.lock.Unlock()

	res := DebugObserverOnDeck{
		Count: len(o.onDeck),
		Items: make([]DebugObserverOnDeckItem, 0, len(o.onDeck)),
	}
	for k, txIn := range o.onDeck {
		item := DebugObserverOnDeckItem{
			Chain:                  k.chain.String(),
			Height:                 k.height,
			Mempool:                k.mempool,
			TxCount:                len(txIn.TxArray),
			ConfirmationRequired:   txIn.ConfirmationRequired,
			AllowFutureObservation: txIn.AllowFutureObservation,
			Filtered:               txIn.Filtered,
			Txs:                    make([]DebugObserverTxItem, 0, len(txIn.TxArray)),
		}
		for _, tx := range txIn.TxArray {
			item.Txs = append(item.Txs, DebugObserverTxItem{
				Tx:                   tx.Tx,
				BlockHeight:          tx.BlockHeight,
				From:                 tx.Sender,
				To:                   tx.To,
				Coins:                tx.Coins,
				Gas:                  tx.Gas,
				ObservedVaultPubKey:  tx.ObservedVaultPubKey.String(),
				SourceVout:           tx.SourceVout,
				SourceInputs:         append([]clienttypes.TxOutInput(nil), tx.SourceInputs...),
				CommittedUnFinalised: tx.CommittedUnFinalised,
			})
		}
		res.Items = append(res.Items, item)
	}
	sort.Slice(res.Items, func(i, j int) bool {
		if res.Items[i].Chain != res.Items[j].Chain {
			return res.Items[i].Chain < res.Items[j].Chain
		}
		if res.Items[i].Height != res.Items[j].Height {
			return res.Items[i].Height < res.Items[j].Height
		}
		if res.Items[i].Mempool != res.Items[j].Mempool {
			return !res.Items[i].Mempool
		}
		if len(res.Items[i].Txs) == 0 || len(res.Items[j].Txs) == 0 {
			return len(res.Items[i].Txs) < len(res.Items[j].Txs)
		}
		return res.Items[i].Txs[0].Tx < res.Items[j].Txs[0].Tx
	})
	return res
}

func (o *Observer) DebugAddress(chain common.Chain, address string) DebugObserverAddress {
	res := DebugObserverAddress{
		Chain:   chain.String(),
		Address: address,
	}
	if ok, cpi := o.pubkeyMgr.IsValidVaultAddress(address, chain); ok {
		res.LocalVault = true
		res.LocalVaultPubKey = cpi.PubKey.String()
		res.LocalVaultAddress = cpi.VaultAddress.String()
	}
	if chain.Equals(common.BTCChain) {
		addr, err := common.NewAddress(address)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.ThornadoDepositAddress = o.thornadoBridge.IsVaultDepositAddress(addr)
	}
	return res
}

type DebugAttestationState struct {
	Type                   string          `json:"type"`
	ID                     string          `json:"id"`
	Chain                  string          `json:"chain,omitempty"`
	PubKey                 string          `json:"pub_key,omitempty"`
	Height                 int64           `json:"height,omitempty"`
	Inbound                bool            `json:"inbound,omitempty"`
	Finalized              bool            `json:"finalized,omitempty"`
	AllowFutureObservation bool            `json:"allow_future_observation,omitempty"`
	Count                  int             `json:"count"`
	CommittedCount         int             `json:"committed_count"`
	UncommittedCount       int             `json:"uncommitted_count"`
	UnsentCount            int             `json:"unsent_count"`
	QuorumTotal            int             `json:"quorum_total"`
	HasSuperMajority       bool            `json:"has_super_majority"`
	LocalAttestation       bool            `json:"local_attestation"`
	LocalCommitted         bool            `json:"local_committed"`
	Attesters              []DebugAttester `json:"attesters"`
	FirstObservedAt        time.Time       `json:"first_observed_at"`
	InitialSentAt          *time.Time      `json:"initial_sent_at,omitempty"`
	InitialSentMs          *int64          `json:"initial_sent_ms,omitempty"`
	QuorumSentAt           *time.Time      `json:"quorum_sent_at,omitempty"`
	QuorumSentMs           *int64          `json:"quorum_sent_ms,omitempty"`
	LastSentAt             *time.Time      `json:"last_sent_at,omitempty"`
	LastSentMs             *int64          `json:"last_sent_ms,omitempty"`
	LastCommittedAt        *time.Time      `json:"last_committed_at,omitempty"`
	LastCommittedMs        *int64          `json:"last_committed_ms,omitempty"`
	Error                  string          `json:"error,omitempty"`
}

type DebugAttester struct {
	Address   string `json:"address"`
	PubKey    string `json:"pub_key"`
	Sent      bool   `json:"sent"`
	Committed bool   `json:"committed"`
}

func (s *AttestationGossip) DebugPerformance() DebugAttestationPerformance {
	activeValidatorCount := s.activeValidatorCount()
	localPubKey, localAddress := debugPubKeyAddress(s.pubKey)
	res := DebugAttestationPerformance{
		ActiveValidatorCount: activeValidatorCount,
		LocalPeerID:          s.host.ID().String(),
		LocalAddress:         localAddress,
		LocalPubKey:          localPubKey,
		LocalIsActive:        s.isActiveValidator(s.host.ID()),
		ObservedTxs:          make([]DebugAttestationState, 0),
		NetworkFees:          make([]DebugAttestationState, 0),
		Solvencies:           make([]DebugAttestationState, 0),
		ErrataTxs:            make([]DebugAttestationState, 0),
	}

	type observedItem struct {
		key   txKey
		state *AttestationState[*attestableObservedTx]
	}
	type networkFeeItem struct {
		key   common.NetworkFee
		state *AttestationState[*common.NetworkFee]
	}
	type solvencyItem struct {
		key   common.TxID
		state *AttestationState[*common.Solvency]
	}
	type errataItem struct {
		key   common.ErrataTx
		state *AttestationState[*common.ErrataTx]
	}

	s.mu.Lock()
	observed := make([]observedItem, 0, len(s.observedTxs))
	for k, state := range s.observedTxs {
		observed = append(observed, observedItem{key: k, state: state})
	}
	networkFees := make([]networkFeeItem, 0, len(s.networkFees))
	for k, state := range s.networkFees {
		networkFees = append(networkFees, networkFeeItem{key: k, state: state})
	}
	solvencies := make([]solvencyItem, 0, len(s.solvencies))
	for k, state := range s.solvencies {
		solvencies = append(solvencies, solvencyItem{key: k, state: state})
	}
	errataTxs := make([]errataItem, 0, len(s.errataTxs))
	for k, state := range s.errataTxs {
		errataTxs = append(errataTxs, errataItem{key: k, state: state})
	}
	s.mu.Unlock()

	for _, item := range observed {
		quorumTotal := activeValidatorCount
		var quorumErr string
		if item.key.AllowFutureObservation {
			if party, err := s.getKeysignParty(item.key.ObservedPubKey); err != nil {
				quorumErr = err.Error()
			} else {
				quorumTotal = len(party)
			}
		}
		item.state.mu.Lock()
		var height int64
		localAttestation, localCommitted := item.state.AttestationStatus(s.pubKey)
		if item.state.Item != nil && item.state.Item.ObservedTx != nil {
			height = item.state.Item.ObservedTx.BlockHeight
		}
		res.ObservedTxs = append(res.ObservedTxs, debugAttestationState(
			"observed_tx",
			observedTxDebugID(item.key),
			item.key.Chain.String(),
			item.key.ObservedPubKey.String(),
			height,
			item.key.Inbound,
			item.key.Finalized,
			item.key.AllowFutureObservation,
			item.state,
			quorumTotal,
			quorumErr,
			localAttestation,
			localCommitted,
		))
		item.state.mu.Unlock()
	}
	for _, item := range networkFees {
		item.state.mu.Lock()
		res.NetworkFees = append(res.NetworkFees, debugAttestationState(
			"network_fee",
			fmt.Sprintf("%s:%d", item.key.Chain, item.key.Height),
			item.key.Chain.String(),
			"",
			item.key.Height,
			false,
			false,
			false,
			item.state,
			activeValidatorCount,
			"",
			false,
			false,
		))
		item.state.mu.Unlock()
	}
	for _, item := range solvencies {
		item.state.mu.Lock()
		pubKey := ""
		chain := ""
		var height int64
		if item.state.Item != nil {
			pubKey = item.state.Item.PubKey.String()
			chain = item.state.Item.Chain.String()
			height = item.state.Item.Height
		}
		res.Solvencies = append(res.Solvencies, debugAttestationState(
			"solvency",
			item.key.String(),
			chain,
			pubKey,
			height,
			false,
			false,
			false,
			item.state,
			activeValidatorCount,
			"",
			false,
			false,
		))
		item.state.mu.Unlock()
	}
	for _, item := range errataTxs {
		item.state.mu.Lock()
		res.ErrataTxs = append(res.ErrataTxs, debugAttestationState(
			"errata_tx",
			item.key.Id.String(),
			item.key.Chain.String(),
			"",
			0,
			false,
			false,
			false,
			item.state,
			activeValidatorCount,
			"",
			false,
			false,
		))
		item.state.mu.Unlock()
	}

	sortDebugAttestations(res.ObservedTxs)
	sortDebugAttestations(res.NetworkFees)
	sortDebugAttestations(res.Solvencies)
	sortDebugAttestations(res.ErrataTxs)
	return res
}

func debugAttestationState[T AttestableItem](
	typ, id, chain, pubKey string,
	height int64,
	inbound, finalized, allowFutureObservation bool,
	state *AttestationState[T],
	quorumTotal int,
	errText string,
	localAttestation bool,
	localCommitted bool,
) DebugAttestationState {
	firstObserved := state.firstAttestationObserved.UTC()
	initialSentAt, initialSentMs := debugAttestationTime(firstObserved, state.initialAttestationsSent)
	quorumSentAt, quorumSentMs := debugAttestationTime(firstObserved, state.quorumAttestationsSent)
	lastSentAt, lastSentMs := debugAttestationTime(firstObserved, state.lastAttestationsSent)
	lastCommittedAt, lastCommittedMs := debugAttestationTime(firstObserved, state.lastCommittedAttestation)
	count := state.AttestationCount()

	return DebugAttestationState{
		Type:                   typ,
		ID:                     id,
		Chain:                  chain,
		PubKey:                 pubKey,
		Height:                 height,
		Inbound:                inbound,
		Finalized:              finalized,
		AllowFutureObservation: allowFutureObservation,
		Count:                  count,
		CommittedCount:         state.CommittedCount(),
		UncommittedCount:       state.UncommittedCount(),
		UnsentCount:            state.UnsentCount(),
		QuorumTotal:            quorumTotal,
		HasSuperMajority:       quorumTotal > 0 && thornadotypes.HasSuperMajority(count, quorumTotal),
		LocalAttestation:       localAttestation,
		LocalCommitted:         localCommitted,
		Attesters:              debugAttesters(state),
		FirstObservedAt:        firstObserved,
		InitialSentAt:          initialSentAt,
		InitialSentMs:          initialSentMs,
		QuorumSentAt:           quorumSentAt,
		QuorumSentMs:           quorumSentMs,
		LastSentAt:             lastSentAt,
		LastSentMs:             lastSentMs,
		LastCommittedAt:        lastCommittedAt,
		LastCommittedMs:        lastCommittedMs,
		Error:                  errText,
	}
}

func debugAttesters[T AttestableItem](state *AttestationState[T]) []DebugAttester {
	attesters := make([]DebugAttester, 0, len(state.attestations))
	for _, item := range state.attestations {
		if item.attestation == nil {
			continue
		}
		bech32PubKey, address := debugPubKeyAddress(item.attestation.PubKey)
		attesters = append(attesters, DebugAttester{
			Address:   address,
			PubKey:    bech32PubKey,
			Sent:      item.sent,
			Committed: item.committed,
		})
	}
	sort.Slice(attesters, func(i, j int) bool {
		return attesters[i].Address < attesters[j].Address
	})
	return attesters
}

func debugPubKeyAddress(pubKeyBz []byte) (string, string) {
	pubKey := secp256k1.PubKey{Key: pubKeyBz}
	bech32PubKey := ""
	if bz, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, &pubKey); err == nil {
		bech32PubKey = bz
	}
	return bech32PubKey, cosmos.AccAddress(pubKey.Address()).String()
}

func debugAttestationTime(firstObserved, timestamp time.Time) (*time.Time, *int64) {
	if firstObserved.IsZero() || timestamp.IsZero() {
		return nil, nil
	}
	ts := timestamp.UTC()
	ms := ts.Sub(firstObserved).Milliseconds()
	return &ts, &ms
}

func observedTxDebugID(key txKey) string {
	if !key.ID.IsEmpty() {
		return key.ID.String()
	}
	return key.UniqueHash
}

func sortDebugAttestations(items []DebugAttestationState) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].FirstObservedAt.Equal(items[j].FirstObservedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].FirstObservedAt.Before(items[j].FirstObservedAt)
	})
}
