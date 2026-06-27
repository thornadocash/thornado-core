package signer

import (
	"sort"
	"strings"

	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/constants"
)

type DebugTxOut struct {
	Key                   string             `json:"key"`
	Chain                 string             `json:"chain"`
	Height                int64              `json:"height"`
	TxHeight              int64              `json:"tx_height"`
	Index                 int64              `json:"index"`
	Epoch                 uint64             `json:"epoch"`
	Status                TxStatus           `json:"status"`
	StatusLabel           string             `json:"status_label"`
	BatchStatus           string             `json:"batch_status"`
	SigningLeader         string             `json:"signing_leader,omitempty"`
	InHash                string             `json:"in_hash"`
	OutHash               string             `json:"out_hash,omitempty"`
	TxType                string             `json:"tx_type"`
	VaultPubKey           string             `json:"vault_pub_key"`
	ToAddress             string             `json:"to_address"`
	Coin                  string             `json:"coin"`
	GasRate               int64              `json:"gas_rate"`
	VaultPathIndex        uint64             `json:"vault_path_index"`
	SourceInputs          []types.TxOutInput `json:"source_inputs,omitempty"`
	CacheHash             string             `json:"cache_hash"`
	CacheVault            string             `json:"cache_vault"`
	Round7Retry           bool               `json:"round7_retry"`
	DeferredUntilHeight   int64              `json:"deferred_until_height,omitempty"`
	DeferredPast          bool               `json:"deferred_past"`
	CheckpointPresent     bool               `json:"checkpoint_present"`
	CheckpointBytes       int                `json:"checkpoint_bytes"`
	SignedTxPresent       bool               `json:"signed_tx_present"`
	SignedTxBytes         int                `json:"signed_tx_bytes"`
	ObservationPresent    bool               `json:"observation_present"`
	ObservationTx         string             `json:"observation_tx,omitempty"`
	ObservationHeight     int64              `json:"observation_height,omitempty"`
	IsFrostVault          bool               `json:"is_frost_vault"`
	LocalKeyshare         bool               `json:"local_keyshare"`
	LocalPartyKey         string             `json:"local_party_key,omitempty"`
	FrostRoles            *DebugFrostRoles   `json:"frost_roles,omitempty"`
	CurrentKeysignPresent *bool              `json:"current_keysign_present,omitempty"`
	CurrentKeysignStatus  string             `json:"current_keysign_status,omitempty"`
	Errors                []string           `json:"errors,omitempty"`
}

type DebugFrostRoles struct {
	Participate bool   `json:"participate"`
	Broadcast   bool   `json:"broadcast"`
	Leader      string `json:"leader,omitempty"`
	Period      int64  `json:"period_blocks,omitempty"`
	BlockHeight int64  `json:"block_height,omitempty"`
	Error       string `json:"error,omitempty"`
}

type DebugHealth struct {
	Stopped       bool           `json:"stopped"`
	LocalPubKey   string         `json:"local_pub_key"`
	TxOutCount    int            `json:"txout_count"`
	ByStatus      map[string]int `json:"by_status"`
	ByType        map[string]int `json:"by_type"`
	OldestHeight  int64          `json:"oldest_height,omitempty"`
	NewestHeight  int64          `json:"newest_height,omitempty"`
	PendingHashes []string       `json:"pending_hashes"`
}

type DebugLocalVault struct {
	PubKey            string   `json:"pub_key"`
	Status            string   `json:"status"`
	Membership        []string `json:"membership"`
	Chains            []string `json:"chains"`
	LocalKeyshare     bool     `json:"local_keyshare"`
	LocalPartyKey     string   `json:"local_party_key,omitempty"`
	SigningEngine     string   `json:"signing_engine,omitempty"`
	ParticipantKeys   []string `json:"participant_keys,omitempty"`
	Addresses         []string `json:"addresses,omitempty"`
	KeyshareReadError string   `json:"keyshare_read_error,omitempty"`
}

type chainTxOutDebugger interface {
	DebugTxOut(types.TxOutItem, int64) (interface{}, error)
}

func txStatusLabel(status TxStatus) string {
	switch status {
	case TxUnknown:
		return "unknown"
	case TxAvailable:
		return "available"
	case TxSpent:
		return "spent"
	default:
		return "legacy_or_unrecognized"
	}
}

func (s *Signer) DebugTxOuts() []DebugTxOut {
	items := s.storage.List()
	out := make([]DebugTxOut, 0, len(items))
	for _, item := range items {
		out = append(out, s.debugTxOut(item, false))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Height == out[j].Height {
			return out[i].Index < out[j].Index
		}
		return out[i].Height < out[j].Height
	})
	return out
}

func (s *Signer) DebugTxOutByInHash(inHash string) (DebugTxOut, bool) {
	item, ok := s.DebugStoredTxOutByInHash(inHash)
	if !ok {
		return DebugTxOut{}, false
	}
	return s.debugTxOut(item, true), true
}

func (s *Signer) DebugStoredTxOutByInHash(inHash string) (TxOutStoreItem, bool) {
	target := strings.ToUpper(inHash)
	for _, item := range s.storage.List() {
		if strings.EqualFold(item.TxOutItem.InHash.String(), target) {
			return item, true
		}
	}
	return TxOutStoreItem{}, false
}

func (s *Signer) DebugChainTxOut(inHash string) (interface{}, bool, error) {
	item, ok := s.DebugStoredTxOutByInHash(inHash)
	if !ok {
		return nil, false, nil
	}
	chain, err := s.getChain(item.TxOutItem.Chain)
	if err != nil {
		return nil, true, err
	}
	debugger, ok := chain.(chainTxOutDebugger)
	if !ok {
		return map[string]string{"error": "chain does not expose txout debug"}, true, nil
	}
	height, err := s.thornadoBridge.GetBlockHeight()
	if err != nil {
		height = item.Height
	}
	res, err := debugger.DebugTxOut(item.TxOutItem, height)
	return res, true, err
}

func (s *Signer) DebugHealth() DebugHealth {
	items := s.storage.List()
	res := DebugHealth{
		Stopped:       s.isStopped(),
		LocalPubKey:   s.localPubKeyECDSA.String(),
		TxOutCount:    len(items),
		ByStatus:      make(map[string]int),
		ByType:        make(map[string]int),
		PendingHashes: make([]string, 0),
	}
	for _, item := range items {
		status := txStatusLabel(item.Status)
		txType := item.TxOutItem.TxType
		if txType == "" {
			txType = "unknown"
		}
		res.ByStatus[status]++
		res.ByType[txType]++
		if res.OldestHeight == 0 || item.Height < res.OldestHeight {
			res.OldestHeight = item.Height
		}
		if item.Height > res.NewestHeight {
			res.NewestHeight = item.Height
		}
		if item.Status != TxSpent {
			res.PendingHashes = append(res.PendingHashes, item.TxOutItem.InHash.String())
		}
	}
	sort.Strings(res.PendingHashes)
	return res
}

func (s *Signer) DebugLocalVaults() ([]DebugLocalVault, error) {
	vaults, err := s.thornadoBridge.GetBaseVaults()
	if err != nil {
		return nil, err
	}
	res := make([]DebugLocalVault, 0, len(vaults))
	for _, vault := range vaults {
		item := DebugLocalVault{
			PubKey:     vault.PubKey.String(),
			Status:     vault.Status.String(),
			Membership: sortedStrings(vault.Membership),
			Chains:     sortedStrings(vault.Chains),
		}
		for _, chain := range vault.Chains {
			if client, ok := s.chains[common.Chain(strings.ToUpper(chain))]; ok {
				item.Addresses = append(item.Addresses, client.GetAddress(vault.PubKey))
			}
		}
		state, stateErr := s.localState.GetLocalState(vault.PubKey.String())
		if stateErr != nil {
			item.KeyshareReadError = stateErr.Error()
		} else {
			item.LocalKeyshare = len(state.LocalData) > 0
			item.LocalPartyKey = state.LocalPartyKey
			item.SigningEngine = state.Engine()
			item.ParticipantKeys = append([]string(nil), state.ParticipantKeys...)
		}
		res = append(res, item)
	}
	return res, nil
}

func (s *Signer) debugTxOut(item TxOutStoreItem, deep bool) DebugTxOut {
	tx := item.TxOutItem
	blockHeight := item.Height
	if h, err := s.thornadoBridge.GetBlockHeight(); err == nil {
		blockHeight = h
	}
	res := DebugTxOut{
		Key:                 item.Key(),
		Chain:               tx.Chain.String(),
		Height:              item.Height,
		TxHeight:            tx.Height,
		Index:               item.Index,
		Epoch:               item.Epoch,
		Status:              item.Status,
		StatusLabel:         txStatusLabel(item.Status),
		BatchStatus:         item.BatchStatus,
		SigningLeader:       item.SigningLeader.String(),
		InHash:              tx.InHash.String(),
		OutHash:             tx.OutHash.String(),
		TxType:              tx.TxType,
		VaultPubKey:         tx.VaultPubKey.String(),
		ToAddress:           tx.ToAddress.String(),
		Coin:                tx.Coins.String(),
		GasRate:             tx.GasRate,
		VaultPathIndex:      tx.VaultPathIndex,
		SourceInputs:        append([]types.TxOutInput(nil), tx.SourceInputs...),
		CacheHash:           tx.CacheHash(),
		CacheVault:          tx.CacheVault(tx.Chain),
		Round7Retry:         item.Round7Retry,
		DeferredUntilHeight: item.DeferredUntilHeight,
		DeferredPast:        txOutDeferredPast(item, blockHeight),
		CheckpointPresent:   len(item.Checkpoint) > 0,
		CheckpointBytes:     len(item.Checkpoint),
		SignedTxPresent:     len(item.SignedTx) > 0,
		SignedTxBytes:       len(item.SignedTx),
		ObservationPresent:  item.Observation != nil,
		LocalKeyshare:       s.localFrostStateExists(tx.VaultPubKey),
	}
	if item.Observation != nil {
		res.ObservationTx = item.Observation.Tx
		res.ObservationHeight = item.Observation.BlockHeight
	}
	if party, ok := s.localFrostSigningParty(tx.VaultPubKey); ok {
		res.LocalPartyKey = party.String()
	}
	if chain, err := s.getChain(tx.Chain); err == nil {
		res.IsFrostVault = s.isFrostVaultTxOut(tx.VaultPubKey, chain)
		if deep {
			res.FrostRoles = s.debugFrostRoles(item, chain, blockHeight)
		}
	} else if deep {
		res.Errors = append(res.Errors, err.Error())
	}
	if deep {
		if txOut, err := s.thornadoBridge.GetKeysign(item.Height, tx.VaultPubKey.String()); err != nil {
			res.Errors = append(res.Errors, "get current keysign: "+err.Error())
		} else {
			present := false
			for _, txArray := range txOut.TxArray {
				candidate := txArray.TxOutItem(item.TxOutItem.Height)
				if txOutCompletionMatch(tx, candidate) {
					present = true
					break
				}
			}
			res.CurrentKeysignPresent = &present
			res.CurrentKeysignStatus = txOut.Status
		}
	}
	return res
}

func (s *Signer) debugFrostRoles(item TxOutStoreItem, chain chainclients.ChainClient, blockHeight int64) *DebugFrostRoles {
	periodMinutes, err := s.constantsProvider.GetInt64Value(blockHeight, constants.Keysign_PeriodMinutes)
	if err != nil {
		return &DebugFrostRoles{BlockHeight: blockHeight, Error: err.Error()}
	}
	blockTimeSeconds, err := s.constantsProvider.GetInt64Value(blockHeight, constants.Chain_BlockTimeSeconds)
	if err != nil || blockTimeSeconds <= 0 {
		blockTimeSeconds = constants.NewConfigValue().GetInt64Value(constants.Chain_BlockTimeSeconds)
	}
	period := constants.MinutesToBlocks(periodMinutes, blockTimeSeconds)
	participate, broadcast, roleErr := s.frostSignerRoles(item, chain, blockHeight, period)
	leader, leaderErr := s.frostPartyLeader(item, chain, blockHeight, period)
	res := &DebugFrostRoles{
		Participate: participate,
		Broadcast:   broadcast,
		Leader:      leader,
		Period:      period,
		BlockHeight: blockHeight,
	}
	if roleErr != nil {
		res.Error = roleErr.Error()
	} else if leaderErr != nil {
		res.Error = leaderErr.Error()
	}
	return res
}

func sortedStrings(values []string) []string {
	res := append([]string(nil), values...)
	sort.Strings(res)
	return res
}
