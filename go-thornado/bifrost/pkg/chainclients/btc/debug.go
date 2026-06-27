package btc

import (
	"fmt"
	"strings"

	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type DebugTxOutState struct {
	Chain                   string             `json:"chain"`
	InHash                  string             `json:"in_hash"`
	TxType                  string             `json:"tx_type"`
	CacheHash               string             `json:"cache_hash"`
	CacheVault              string             `json:"cache_vault"`
	AlreadySigned           bool               `json:"already_signed"`
	SignedTxHash            string             `json:"signed_tx_hash,omitempty"`
	LatestRecordedVaultTx   string             `json:"latest_recorded_vault_tx,omitempty"`
	RecoveredObservation    *stypes.TxInItem   `json:"recovered_observation,omitempty"`
	SweepSpendObservation   *stypes.TxInItem   `json:"sweep_spend_observation,omitempty"`
	SourceInputs            []DebugSourceInput `json:"source_inputs,omitempty"`
	SourceMissing           *bool              `json:"source_missing,omitempty"`
	MempoolSpendsSweepInput bool               `json:"mempool_spends_sweep_input,omitempty"`
	Errors                  []string           `json:"errors,omitempty"`
}

type DebugSourceInput struct {
	TxID          string `json:"tx_id"`
	Vout          uint32 `json:"vout"`
	AmountSats    uint64 `json:"amount_sats,omitempty"`
	Known         bool   `json:"known"`
	Confirmed     bool   `json:"confirmed"`
	Unspent       bool   `json:"unspent"`
	SpentByTxID   string `json:"spent_by_txid,omitempty"`
	SpentAtHeight int64  `json:"spent_at_height,omitempty"`
	Error         string `json:"error,omitempty"`
}

// DebugTxOut returns read-only BTC signing state for a Thornado outbound item.
func (c *Client) DebugTxOut(tx stypes.TxOutItem, thornadoHeight int64) (interface{}, error) {
	res := DebugTxOutState{
		Chain:         c.GetChain().String(),
		InHash:        tx.InHash.String(),
		TxType:        tx.TxType,
		CacheHash:     tx.CacheHash(),
		CacheVault:    tx.CacheVault(c.GetChain()),
		AlreadySigned: c.txAlreadySigned(tx),
		SourceInputs:  make([]DebugSourceInput, 0, len(tx.SourceInputs)),
	}
	if txid, ok := c.signerCacheManager.GetSignedTxHash(tx.CacheHash()); ok {
		res.SignedTxHash = txid
	}
	if latest, err := c.signerCacheManager.GetLatestRecordedTx(tx.CacheVault(c.GetChain())); err != nil {
		res.Errors = append(res.Errors, "latest recorded vault tx: "+err.Error())
	} else {
		res.LatestRecordedVaultTx = latest
	}
	if obs, err := c.recoverSignedTxObservation(tx); err != nil {
		res.Errors = append(res.Errors, "recover signed tx observation: "+err.Error())
	} else {
		res.RecoveredObservation = obs
	}
	if tx.TxType == ttypes.TxOutTypeSweep {
		if obs, err := c.recoverSpentSweepObservation(tx); err != nil {
			res.Errors = append(res.Errors, "recover spent sweep observation: "+err.Error())
		} else {
			res.SweepSpendObservation = obs
		}
		res.MempoolSpendsSweepInput = c.sweepSpendInMempool(tx)
		if missing, err := c.SourceTxMissing(tx, thornadoHeight); err != nil {
			res.Errors = append(res.Errors, "source tx missing: "+err.Error())
		} else {
			res.SourceMissing = &missing
		}
	}
	inputs := tx.SourceInputs
	if len(inputs) == 0 && tx.TxType == ttypes.TxOutTypeSweep && !tx.InHash.IsEmpty() {
		inputs = []stypes.TxOutInput{{TxID: tx.InHash, Vout: 0}}
	}
	for _, input := range inputs {
		res.SourceInputs = append(res.SourceInputs, c.debugSourceInput(input))
	}
	return res, nil
}

func (c *Client) debugSourceInput(input stypes.TxOutInput) DebugSourceInput {
	res := DebugSourceInput{
		TxID:       input.TxID.String(),
		Vout:       input.Vout,
		AmountSats: input.AmountSats,
	}
	raw, err := c.rpc.GetRawTransactionVerbose(input.TxID.String())
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if raw == nil {
		res.Error = "source tx not found"
		return res
	}
	res.Known = true
	res.Confirmed = raw.BlockHash != ""
	txOut, err := c.rpc.GetTxOut(input.TxID.String(), input.Vout, true)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Unspent = txOut != nil
	if res.Unspent {
		return res
	}
	spend, height, err := c.findSpendingTx(input)
	if err != nil {
		res.Error = fmt.Sprintf("find spending tx: %s", err)
		return res
	}
	if spend != nil {
		res.SpentByTxID = strings.ToUpper(spend.Txid)
		res.SpentAtHeight = height
	}
	return res
}
