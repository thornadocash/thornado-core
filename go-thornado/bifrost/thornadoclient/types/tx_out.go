package types

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// TxOutItem represent the information of a tx bifrost need to process
type TxOutItem struct {
	Chain                 common.Chain   `json:"chain"`
	ToAddress             common.Address `json:"to"`
	VaultPubKey           common.PubKey  `json:"vault_pubkey"`
	Coins                 common.Coins   `json:"coins"`
	MaxGas                common.Gas     `json:"max_gas"`
	GasRate               int64          `json:"gas_rate"`
	InHash                common.TxID    `json:"in_hash"`
	OutHash               common.TxID    `json:"out_hash"`
	OutVout               uint32         `json:"out_vout,omitempty"`
	Aggregator            string         `json:"aggregator"`
	AggregatorTargetAsset string         `json:"aggregator_target_asset,omitempty"`
	AggregatorTargetLimit *cosmos.Uint   `json:"aggregator_target_limit,omitempty"`
	Checkpoint            []byte         `json:"-"`
	Height                int64          `json:"height"`
	VaultPubKeyEddsa      common.PubKey  `json:"vault_pub_key_eddsa,omitempty"`
	VaultPathIndex        uint64         `json:"vault_path_index,omitempty"`
	TxType                string         `json:"tx_type,omitempty"`
	SourceInputs          []TxOutInput   `json:"source_inputs"`
}

type TxOutInput struct {
	TxID       common.TxID `json:"tx_id"`
	Vout       uint32      `json:"vout,omitempty"`
	AmountSats uint64      `json:"amount_sats,omitempty"`
}

func sourceInputsString(inputs []TxOutInput) string {
	if len(inputs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		parts = append(parts, fmt.Sprintf("%s:%d:%d", input.TxID, input.Vout, input.AmountSats))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// Hash return a sha256 hash that can uniquely represent the TxOutItem
func (tx TxOutItem) Hash() string {
	str := fmt.Sprintf("%s|%s|%s|%s|%s|%s", tx.Chain, tx.ToAddress, tx.VaultPubKey, tx.Coins, tx.InHash, sourceInputsString(tx.SourceInputs))
	return fmt.Sprintf("%X", sha256.Sum256([]byte(str)))
}

// CacheHash return a hash that doesn't include VaultPubKey , thus this one can be used as cache key for txOutItem across different vaults
func (tx TxOutItem) CacheHash() string {
	str := fmt.Sprintf("%s|%s|%s|%s|%s", tx.Chain, tx.ToAddress, tx.Coins, tx.InHash, sourceInputsString(tx.SourceInputs))
	return fmt.Sprintf("%X", sha256.Sum256([]byte(str)))
}

func (tx TxOutItem) CacheVault(chain common.Chain) string {
	return BroadcastCacheKey(tx.VaultPubKey.String(), chain.String())
}

// Equals returns true when the TxOutItems are equal.
//
// NOTE: The height field should NOT be compared. This is necessary to pass through on
// the TxOutItem to the unstuck routine to determine the position within the signing
// period, but should not be used to determine equality for deduplication.
func (tx TxOutItem) Equals(tx2 TxOutItem) bool {
	if !tx.Chain.Equals(tx2.Chain) {
		return false
	}
	if !tx.VaultPubKey.Equals(tx2.VaultPubKey) {
		return false
	}
	if !tx.ToAddress.Equals(tx2.ToAddress) {
		return false
	}
	if !tx.Coins.EqualsEx(tx2.Coins) {
		return false
	}
	if !tx.InHash.Equals(tx2.InHash) {
		return false
	}
	if tx.GasRate != tx2.GasRate {
		return false
	}
	if !strings.EqualFold(tx.Aggregator, tx2.Aggregator) {
		return false
	}
	if !strings.EqualFold(tx.AggregatorTargetAsset, tx2.AggregatorTargetAsset) {
		return false
	}
	if tx.AggregatorTargetLimit == nil && tx2.AggregatorTargetLimit == nil {
		return true
	}
	if tx.AggregatorTargetLimit == nil && tx2.AggregatorTargetLimit != nil {
		return false
	}
	if tx.AggregatorTargetLimit != nil && tx2.AggregatorTargetLimit == nil {
		return false
	}
	if !tx.AggregatorTargetLimit.Equal(*tx2.AggregatorTargetLimit) {
		return false
	}
	if !tx.VaultPubKeyEddsa.Equals(tx2.VaultPubKeyEddsa) {
		return false
	}
	if tx.VaultPathIndex != tx2.VaultPathIndex {
		return false
	}
	if !sourceInputsEqual(tx.SourceInputs, tx2.SourceInputs) {
		return false
	}
	return true
}

func sourceInputsEqual(a, b []TxOutInput) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]TxOutInput(nil), a...)
	bb := append([]TxOutInput(nil), b...)
	sort.SliceStable(aa, func(i, j int) bool {
		if !aa[i].TxID.Equals(aa[j].TxID) {
			return aa[i].TxID.String() < aa[j].TxID.String()
		}
		return aa[i].Vout < aa[j].Vout
	})
	sort.SliceStable(bb, func(i, j int) bool {
		if !bb[i].TxID.Equals(bb[j].TxID) {
			return bb[i].TxID.String() < bb[j].TxID.String()
		}
		return bb[i].Vout < bb[j].Vout
	})
	for i := range aa {
		if !aa[i].TxID.Equals(bb[i].TxID) || aa[i].Vout != bb[i].Vout || aa[i].AmountSats != bb[i].AmountSats {
			return false
		}
	}
	return true
}

// TxArrayItem used to represent the tx out item coming from Thornado, there is little difference between TxArrayItem
// and TxOutItem defined above , only Coin <-> Coins field are different.
// TxArrayItem from Thornado has Coin , which only have a single coin
// TxOutItem used in bifrost need to support Coins for multisend
type TxArrayItem struct {
	Chain                 common.Chain   `json:"chain,omitempty"`
	ToAddress             common.Address `json:"to_address,omitempty"`
	VaultPubKey           common.PubKey  `json:"vault_pub_key,omitempty"`
	Coin                  common.Coin    `json:"coin"`
	MaxGas                common.Gas     `json:"max_gas"`
	GasRate               int64          `json:"gas_rate,omitempty"`
	InHash                common.TxID    `json:"in_hash,omitempty"`
	OutHash               common.TxID    `json:"out_hash,omitempty"`
	OutVout               uint32         `json:"out_vout,omitempty"`
	Aggregator            string         `json:"aggregator,omitempty"`
	AggregatorTargetAsset string         `json:"aggregator_target_asset,omitempty"`
	AggregatorTargetLimit *cosmos.Uint   `json:"aggregator_target_limit,omitempty"`
	VaultPubKeyEddsa      common.PubKey  `json:"vault_pub_key_eddsa,omitempty"`
	VaultPathIndex        uint64         `json:"vault_path_index,omitempty"`
	TxType                string         `json:"tx_type,omitempty"`
	SourceInputs          []TxOutInput   `json:"source_inputs"`
}

// TxOutItem convert the information to TxOutItem
func (tx TxArrayItem) TxOutItem(height int64) TxOutItem {
	return TxOutItem{
		Chain:                 tx.Chain,
		ToAddress:             tx.ToAddress,
		VaultPubKey:           tx.VaultPubKey,
		Coins:                 common.Coins{tx.Coin},
		MaxGas:                tx.MaxGas,
		GasRate:               tx.GasRate,
		InHash:                tx.InHash,
		OutHash:               tx.OutHash,
		OutVout:               tx.OutVout,
		Aggregator:            tx.Aggregator,
		AggregatorTargetAsset: tx.AggregatorTargetAsset,
		AggregatorTargetLimit: tx.AggregatorTargetLimit,
		Height:                height,
		VaultPubKeyEddsa:      tx.VaultPubKeyEddsa,
		VaultPathIndex:        tx.VaultPathIndex,
		TxType:                tx.TxType,
		SourceInputs:          tx.SourceInputs,
	}
}

// TxOut represent the tx out information , bifrost need to sign and process
type TxOut struct {
	Height           int64         `json:"height"`
	TxArray          []TxArrayItem `json:"tx_array"`
	Epoch            uint64        `json:"epoch,omitempty"`
	Status           string        `json:"status,omitempty"`
	SigningLeader    common.PubKey `json:"signing_leader,omitempty"`
	SigningAttempt   uint64        `json:"signing_attempt,omitempty"`
	RetryUntilHeight int64         `json:"retry_until_height,omitempty"`
}

func BroadcastCacheKey(vault, chain string) string {
	return fmt.Sprintf("broadcast-%s-%s", vault, chain)
}
