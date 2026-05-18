package types

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	mem "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
)

type TxIn struct {
	Chain                common.Chain `json:"chain"`
	TxArray              []*TxInItem  `json:"txArray"`
	Filtered             bool         `json:"filtered"`
	MemPool              bool         `json:"mem_pool"` // indicate whether this item is in the mempool or not
	ConfirmationRequired int64        `json:"confirmation_required"`

	// whether this originated from a "instant observation" - e.g. by a member of the signing party
	// immediately after signing, and also has incorrect gas, requiring a re-observation to correct.
	AllowFutureObservation bool `json:"allow_future_observation"`
}

type TxInItem struct {
	BlockHeight           int64         `json:"block_height"`
	Tx                    string        `json:"tx"`
	Memo                  string        `json:"memo"`
	Sender                string        `json:"sender"`
	To                    string        `json:"to"` // to address
	Coins                 common.Coins  `json:"coins"`
	Gas                   common.Gas    `json:"gas"`
	ObservedVaultPubKey   common.PubKey `json:"observed_vault_pub_key"`
	Aggregator            string        `json:"aggregator"`
	AggregatorTarget      string        `json:"aggregator_target"`
	AggregatorTargetLimit *cosmos.Uint  `json:"aggregator_target_limit"`
	CommittedUnFinalised  bool          `json:"committed_pre_final"`
}
type TxInStatus byte

func NewTxInItem(
	blockHeight int64,
	tx string,
	memo string,
	sender string,
	to string,
	coins common.Coins,
	gas common.Gas,
	observedVaultPubKey common.PubKey,
	aggregator string,
	aggregatorTarget string,
	aggregatorTargetLimit *cosmos.Uint,
) *TxInItem {
	return &TxInItem{
		BlockHeight:           blockHeight,
		Tx:                    tx,
		Memo:                  memo,
		Sender:                sender,
		To:                    to,
		Coins:                 coins,
		Gas:                   gas,
		ObservedVaultPubKey:   observedVaultPubKey,
		Aggregator:            aggregator,
		AggregatorTarget:      aggregatorTarget,
		AggregatorTargetLimit: aggregatorTargetLimit,
		CommittedUnFinalised:  false, // stateful parameter used internally in the observer
	}
}

const (
	Processing TxInStatus = iota
	Failed
)

func (t *TxInItem) Equals(other *TxInItem) bool {
	if t.BlockHeight != other.BlockHeight {
		return false
	}
	if t.Tx != other.Tx {
		return false
	}
	if t.Memo != other.Memo {
		return false
	}
	if t.Sender != other.Sender {
		return false
	}
	if t.To != other.To {
		return false
	}
	if !t.Coins.EqualsEx(other.Coins) {
		return false
	}
	if !t.Gas.Equals(other.Gas) {
		return false
	}
	if !t.ObservedVaultPubKey.Equals(other.ObservedVaultPubKey) {
		return false
	}
	return true
}

func (t *TxInItem) EqualsObservedTx(other common.ObservedTx) bool {
	// Do not compare block height, as we only keep one deck item for a tx pre/post finalization.
	// The final block height will be ConfirmationCount higher than the unfinalized tx.

	txId, err := common.NewTxID(t.Tx)
	if err != nil {
		return false
	}
	if !txId.Equals(other.Tx.ID) {
		return false
	}
	if t.Memo != other.Tx.Memo {
		return false
	}
	if t.Sender != other.Tx.FromAddress.String() {
		return false
	}
	if t.To != other.Tx.ToAddress.String() {
		return false
	}
	if !t.Coins.EqualsEx(other.Tx.Coins) {
		return false
	}
	if !t.Gas.Equals(other.Tx.Gas) {
		return false
	}
	if !t.ObservedVaultPubKey.Equals(other.ObservedPubKey) {
		return false
	}
	return true
}

func (t *TxInItem) Copy() *TxInItem {
	return &TxInItem{
		BlockHeight:           t.BlockHeight,
		Tx:                    t.Tx,
		Memo:                  t.Memo,
		Sender:                t.Sender,
		To:                    t.To,
		Coins:                 t.Coins.Copy(),
		Gas:                   common.Gas(common.Coins(t.Gas).Copy()),
		ObservedVaultPubKey:   t.ObservedVaultPubKey,
		Aggregator:            t.Aggregator,
		AggregatorTarget:      t.AggregatorTarget,
		AggregatorTargetLimit: t.AggregatorTargetLimit,
		CommittedUnFinalised:  t.CommittedUnFinalised,
	}
}

// IsEmpty return true only when every field in TxInItem is empty
func (t *TxInItem) IsEmpty() bool {
	return t.BlockHeight == 0 &&
		t.Tx == "" &&
		t.Memo == "" &&
		t.Sender == "" &&
		t.To == "" &&
		t.Coins.IsEmpty() &&
		t.Gas.IsEmpty() &&
		t.ObservedVaultPubKey.IsEmpty()
}

// CacheHash calculate the has used for signer cache
func (t *TxInItem) CacheHash(chain common.Chain, inboundHash string) string {
	str := fmt.Sprintf("%s|%s|%s|%s|%s", chain, t.To, t.Coins, t.Memo, inboundHash)
	return fmt.Sprintf("%X", sha256.Sum256([]byte(str)))
}

func (t *TxInItem) CacheVault(chain common.Chain) string {
	return InboundCacheKey(t.ObservedVaultPubKey.String(), chain.String())
}

// GetTotalTransactionValue return the total value of the requested asset
func (t TxIn) GetTotalTransactionValue(asset common.Asset, excludeFrom []common.Address) cosmos.Uint {
	total := cosmos.ZeroUint()
	for _, item := range t.TxArray {
		fromAsgard := false
		for _, fromAddress := range excludeFrom {
			if strings.EqualFold(fromAddress.String(), item.Sender) {
				fromAsgard = true
			}
		}
		if fromAsgard {
			continue
		}
		// skip confirmation counting if it is internal tx
		m, err := mem.ParseMemo(common.LatestVersion, item.Memo)
		if err == nil && m.IsInternal() {
			continue
		}
		c := item.Coins.GetCoin(asset)
		if c.IsEmpty() {
			continue
		}
		total = total.Add(c.Amount)
	}
	return total
}

// GetTotalGas return the total gas
func (t TxIn) GetTotalGas() cosmos.Uint {
	total := cosmos.ZeroUint()
	for _, item := range t.TxArray {
		if item.Gas == nil {
			continue
		}
		if err := item.Gas.Valid(); err != nil {
			continue
		}
		total = total.Add(item.Gas[0].Amount)
	}
	return total
}

func InboundCacheKey(vault, chain string) string {
	return fmt.Sprintf("inbound-%s-%s", vault, chain)
}
