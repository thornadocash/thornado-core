package btc

import (
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
)

func observedSourceInputs(tx *btcjson.TxRawResult) []types.TxOutInput {
	return observedSourceInputsWithAmounts(tx, nil)
}

func observedSourceInputsWithAmounts(tx *btcjson.TxRawResult, vinTxs map[string]*btcjson.TxRawResult) []types.TxOutInput {
	if tx == nil || len(tx.Vin) == 0 {
		return nil
	}
	inputs := make([]types.TxOutInput, 0, len(tx.Vin))
	for _, vin := range tx.Vin {
		if vin.Txid == "" {
			continue
		}
		txID, err := common.NewTxID(vin.Txid)
		if err != nil {
			continue
		}
		amountSats := uint64(0)
		if vinTxs != nil {
			if vinTx := vinTxs[vin.Txid]; vinTx != nil && int(vin.Vout) < len(vinTx.Vout) {
				if amount, err := btcutil.NewAmount(vinTx.Vout[vin.Vout].Value); err == nil {
					amountSats = uint64(amount.ToUnit(btcutil.AmountSatoshi))
				}
			}
		}
		inputs = append(inputs, types.TxOutInput{
			TxID:       txID,
			Vout:       vin.Vout,
			AmountSats: amountSats,
		})
	}
	return inputs
}

func (c *Client) observedSourceInputsFromRPC(tx *btcjson.TxRawResult) []types.TxOutInput {
	if tx == nil || len(tx.Vin) == 0 {
		return nil
	}
	vinTxs := make(map[string]*btcjson.TxRawResult, len(tx.Vin))
	for _, vin := range tx.Vin {
		if vin.Txid == "" {
			continue
		}
		vinTx, err := c.rpc.GetRawTransactionVerbose(vin.Txid)
		if err != nil {
			c.log.Debug().Err(err).Str("txid", vin.Txid).Msg("fail to load source input amount")
			continue
		}
		vinTxs[vin.Txid] = vinTx
	}
	return observedSourceInputsWithAmounts(tx, vinTxs)
}
