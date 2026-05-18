package utxo

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/utxo/zecutil"

	"github.com/eager7/dogutil"
	dogetxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/dogd-txscript"
	"gitlab.com/thorchain/thornode/v3/constants"

	"github.com/gcash/bchutil"
	bchtxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/bchd-txscript"

	"github.com/ltcsuite/ltcutil"
	ltctxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/ltcd-txscript"

	"github.com/btcsuite/btcutil"
	btctxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/txscript"

	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	mem "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

////////////////////////////////////////////////////////////////////////////////////////
// UTXO Selection
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) getMaximumUtxosToSpend() int64 {
	const mimirMaxUTXOsToSpend = `MaxUTXOsToSpend`
	utxosToSpend, err := c.bridge.GetMimir(mimirMaxUTXOsToSpend)
	if err != nil {
		c.log.Err(err).Msg("fail to get MaxUTXOsToSpend")
	}
	if utxosToSpend <= 0 {
		utxosToSpend = c.cfg.UTXO.MaxUTXOsToSpend
	}
	return utxosToSpend
}

// getAllUtxos will iterate unspend utxos for the given address and return the oldest
// set of utxos that can cover the amount.
func (c *Client) getUtxoToSpend(pubkey common.PubKey, total btcutil.Amount, sweepDust bool) ([]btcjson.ListUnspentResult, error) {
	// get all unspent utxos
	addr, err := pubkey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("fail to get address from pubkey(%s): %w", pubkey, err)
	}
	utxos, err := c.rpc.ListUnspent(addr.String())
	if err != nil {
		return nil, fmt.Errorf("fail to get UTXOs: %w", err)
	}

	// spend UTXO older to younger
	sort.SliceStable(utxos, func(i, j int) bool {
		if utxos[i].Confirmations > utxos[j].Confirmations {
			return true
		} else if utxos[i].Confirmations < utxos[j].Confirmations {
			return false
		}
		return utxos[i].TxID < utxos[j].TxID
	})

	var result []btcjson.ListUnspentResult
	var toSpend btcutil.Amount
	minUTXOAmt := btcutil.Amount(c.cfg.ChainID.DustThreshold().Uint64()).ToBTC()
	utxosToSpend := c.getMaximumUtxosToSpend() // can be set by mimir

	for _, item := range utxos {
		if !c.isValidUTXO(item.ScriptPubKey) {
			c.log.Warn().Str("script", item.ScriptPubKey).Msgf("invalid utxo, unable to spend")
			continue
		}

		// analyze-ignore(float-comparison)
		if item.Confirmations < c.cfg.UTXO.MinUTXOConfirmations || item.Amount < minUTXOAmt {
			// For migration transactions, include confirmed sub-dust UTXOs to sweep all
			// vault funds. This allows resolving balance mismatches where the on-chain
			// UTXOs don't match the internal ledger, without requiring a consensus change.
			// analyze-ignore(float-comparison)
			if sweepDust && item.Amount < minUTXOAmt && item.Confirmations >= c.cfg.UTXO.MinUTXOConfirmations {
				// Allow confirmed sub-dust UTXOs through for migration sweeps
			} else {
				// use all UTXOs sent from asgard, regardless of confirmations or dust threshold
				isSelfTx := c.isSelfTransaction(item.TxID)

				// confirm sender of the UTXO is not asgard in case of lost block meta
				if !isSelfTx {
					isSelfTx = c.isFromAsgard(item.TxID)
				}
				if !isSelfTx {
					continue
				}
			}

			// For unconfirmed UTXOs (even self-transactions), check ancestor, descendant, and combined
			// chain count to avoid exceeding the chain's mempool limits. Set to 0 to disable.
			if item.Confirmations == 0 && c.cfg.UTXO.MaxMempoolAncestors > 0 {
				var entry *btcjson.GetMempoolEntryResult
				entry, err = c.rpc.GetMempoolEntry(item.TxID)
				if err != nil {
					// If we cannot get the mempool entry, the tx is likely confirmed.
					c.log.Debug().Err(err).Str("txid", item.TxID).Msg("failed to get mempool entry")
				}

				// Check the combined ancestor and descendant counts to avoid exceeding mempool
				// limits and receiving "-26: too-long-mempool-chain" errors on broadcast.
				if err == nil && entry.AncestorCount+entry.DescendantCount >= c.cfg.UTXO.MaxMempoolAncestors {
					c.log.Warn().
						Str("txid", item.TxID).
						Int64("ancestor_count", entry.AncestorCount).
						Int64("descendant_count", entry.DescendantCount).
						Int64("max_allowed", c.cfg.UTXO.MaxMempoolAncestors).
						Msg("skipping UTXO with too many ancestors/descendants to avoid mempool chain limit")
					continue
				}
			}
		}

		if c.cfg.ChainID == common.ZECChain {
			id := formatUtxoKey(item.TxID, item.Vout)

			var found bool
			found, err = c.temporalStorage.HasSpentUtxo(id)
			if err != nil {
				c.log.Err(err).Msg("failed to check spent utxo")
				continue
			}

			if found {
				continue
			}
		}

		result = append(result, item)
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, fmt.Errorf("fail to convert to btcutil amount: %w", err)
		}
		toSpend += amt

		// in the scenario that there are too many unspent utxos available, make sure it
		// doesn't spend too much as too much UTXO will cause huge pressure on TSS, also
		// make sure it will spend at least maxUTXOsToSpend so the UTXOs will be
		// consolidated
		if int64(len(result)) >= utxosToSpend && toSpend >= total {
			break
		}
	}

	// If we couldn't collect enough UTXOs to cover the required amount, return an error
	// to avoid confusing downstream errors about negative balance
	if toSpend < total {
		return nil, fmt.Errorf("insufficient available UTXOs: need %d, only have %d available from %d UTXOs", total, toSpend, len(result))
	}

	return result, nil
}

func formatUtxoKey(txID string, vout uint32) string {
	return fmt.Sprintf("%s-%d", txID, vout)
}

// vinsUnspent will return true if all the vins are unspent.
func (c *Client) vinsUnspent(tx stypes.TxOutItem, vins []*wire.TxIn) (bool, error) {
	// get all unspent utxos
	addr, err := tx.VaultPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return false, fmt.Errorf("fail to get address from pubkey(%s): %w", tx.VaultPubKey, err)
	}
	utxos, err := c.rpc.ListUnspent(addr.String())
	if err != nil {
		return false, fmt.Errorf("fail to get UTXOs: %w", err)
	}
	unspent := make(map[string]bool, len(utxos))
	for _, utxo := range utxos {
		unspent[formatUtxoKey(utxo.TxID, utxo.Vout)] = true
	}

	// return false if any vin is spent
	allUnspent := true
	for _, vin := range vins {
		key := formatUtxoKey(vin.PreviousOutPoint.Hash.String(), vin.PreviousOutPoint.Index)
		if c.cfg.ChainID == common.ZECChain {
			var found bool
			found, err = c.temporalStorage.HasSpentUtxo(key)
			if err != nil {
				return false, fmt.Errorf("fail to check spent utxo(%s): %w", key, err)
			}

			if found {
				c.log.Warn().
					Stringer("in_hash", tx.InHash).
					Str("txid", vin.PreviousOutPoint.Hash.String()).
					Uint32("vout", vin.PreviousOutPoint.Index).
					Msg("vin is marked spent in local cache")
				allUnspent = false
				continue
			}
		}

		if !unspent[key] {
			c.log.Warn().
				Stringer("in_hash", tx.InHash).
				Str("txid", vin.PreviousOutPoint.Hash.String()).
				Uint32("vout", vin.PreviousOutPoint.Index).
				Msg("vin is spent")
			allUnspent = false
		}
	}

	return allUnspent, nil
}

// isSelfTransaction check the block meta to see whether the transactions is broadcast
// by ourselves if the transaction is broadcast by ourselves, then we should be able to
// spend the UTXO even it is still in mempool as such we could daisy chain the outbound
// transaction
func (c *Client) isSelfTransaction(txID string) bool {
	bms, err := c.temporalStorage.GetBlockMetas()
	if err != nil {
		c.log.Err(err).Msg("fail to get block metas")
		return false
	}
	for _, item := range bms {
		for _, tx := range item.SelfTransactions {
			if strings.EqualFold(tx, txID) {
				c.log.Debug().Msgf("%s is self transaction", txID)
				return true
			}
		}
	}
	return false
}

func (c *Client) getPaymentAmount(tx stypes.TxOutItem) btcutil.Amount {
	amtToPay := tx.Coins.GetCoin(c.cfg.ChainID.GetGasAsset()).Amount.Uint64()
	// ZEC gas is calculated differently, see getUtxoToSpend()
	if !tx.MaxGas.IsEmpty() && c.cfg.ChainID != common.ZECChain {
		gasAmt := tx.MaxGas.ToCoins().GetCoin(c.cfg.ChainID.GetGasAsset()).Amount
		amtToPay += gasAmt.Uint64()
	}
	return btcutil.Amount(amtToPay)
}

// getSourceScript retrieve pay to addr script from tx source
func (c *Client) getSourceScript(tx stypes.TxOutItem) ([]byte, error) {
	sourceAddr, err := tx.VaultPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("fail to get source address: %w", err)
	}

	switch c.cfg.ChainID {
	case common.DOGEChain:
		var addr dogutil.Address
		addr, err = dogutil.DecodeAddress(sourceAddr.String(), c.getChainCfgDOGE())
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return dogetxscript.PayToAddrScript(addr)
	case common.BCHChain:
		var addr bchutil.Address
		addr, err = bchutil.DecodeAddress(sourceAddr.String(), c.getChainCfgBCH())
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return bchtxscript.PayToAddrScript(addr)
	case common.LTCChain:
		var addr ltcutil.Address
		addr, err = ltctxscript.DecodeAddress(sourceAddr.String(), c.getChainCfgLTC())
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return ltctxscript.PayToAddrScript(addr)
	case common.BTCChain:
		var addr btcutil.Address
		addr, err = btcutil.DecodeAddress(sourceAddr.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return btctxscript.PayToAddrScript(addr)
	case common.ZECChain:
		params := c.getChainCfgZEC()
		var addr btcutil.Address
		addr, err := zecutil.DecodeAddress(sourceAddr.String(), params.Name)
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return zecutil.PayToAddrScript(addr)
	default:
		c.log.Fatal().Msg("unsupported chain")
		return nil, nil
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// Build Transaction
////////////////////////////////////////////////////////////////////////////////////////

// estimateTxSize builds a dummy transaction with the given inputs and outputs and
// returns the exact virtual size (vbytes) according to BIP141.
// For non-segwit chains, it returns the actual serialized size.
func (c *Client) estimateTxSize(txes []btcjson.ListUnspentResult, memoScripts [][]byte, customerScript, changeScript []byte) int64 {
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs with realistic witness/scriptSig data for size estimation
	for _, utxo := range txes {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			c.log.Error().Err(err).Msg("failed to parse txid for size estimation")
			continue
		}
		outpoint := wire.NewOutPoint(hash, utxo.Vout)
		txIn := wire.NewTxIn(outpoint, nil, nil)

		// Add realistic scriptSig/witness data for accurate size estimation
		if c.isSegwitChain() {
			// For segwit chains (BTC, LTC), inputs have empty scriptSig but witness data
			// Typical P2WPKH witness: [signature (71-73 bytes), pubkey (33 bytes)]
			txIn.Witness = make([][]byte, 2)
			txIn.Witness[0] = make([]byte, 72) // signature
			txIn.Witness[1] = make([]byte, 33) // pubkey
		} else {
			// For non-segwit chains (DOGE, BCH), inputs have scriptSig
			// Typical P2PKH scriptSig: [signature (71-73 bytes), pubkey (33 bytes)]
			// Script format: <sig> <pubkey>
			txIn.SignatureScript = make([]byte, 107) // ~72 + 33 + 2 bytes overhead
		}

		tx.AddTxIn(txIn)
	}

	// Add customer output
	tx.AddTxOut(wire.NewTxOut(0, customerScript))

	// Add change output (will be added if balance > 0)
	tx.AddTxOut(wire.NewTxOut(0, changeScript))

	// Add memo outputs
	if len(memoScripts) > 0 {
		// First script is OP_RETURN (value = 0)
		tx.AddTxOut(wire.NewTxOut(0, memoScripts[0]))

		// Additional scripts are P2WPKH/P2PKH outputs with dust value
		for _, script := range memoScripts[1:] {
			tx.AddTxOut(wire.NewTxOut(0, script)) // value doesn't affect size
		}
	}

	// Calculate size based on chain type
	if c.isSegwitChain() {
		// For segwit chains, calculate virtual size (weight/4)
		strippedSize := tx.SerializeSizeStripped()
		totalSize := tx.SerializeSize()
		// Virtual size = (base_size * 3 + total_size) / 4
		return int64((strippedSize*3 + totalSize + 3) / 4) // +3 for proper rounding
	}

	// For non-segwit chains, return actual serialized size
	return int64(tx.SerializeSize())
}

// isSegwitChain returns true if the chain supports segwit transactions
func (c *Client) isSegwitChain() bool {
	switch c.cfg.ChainID {
	case common.BTCChain, common.LTCChain:
		return true
	case common.DOGEChain, common.BCHChain, common.ZECChain:
		return false
	default:
		c.log.Fatal().Msgf("unsupported chain: %s", c.cfg.ChainID)
		return false
	}
}

func (c *Client) getGasCoin(tx stypes.TxOutItem, vSize int64) common.Coin {
	gasRate := tx.GasRate

	// if the gas rate is zero, try to get from last transaction fee
	if gasRate == 0 {
		fee, vBytes, err := c.temporalStorage.GetTransactionFee()
		if err != nil {
			c.log.Error().Err(err).Msg("fail to get previous transaction fee from local storage")
			return common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(vSize*gasRate)))
		}
		// analyze-ignore(float-comparison)
		if fee != 0.0 && vSize != 0 {
			var amt btcutil.Amount
			amt, err = btcutil.NewAmount(fee)
			if err != nil {
				c.log.Err(err).Msg("fail to convert amount from float64 to int64")
			} else {
				gasRate = int64(amt) / int64(vBytes) // sats per vbyte
			}
		}
	}

	// default to configured value
	if gasRate == 0 {
		gasRate = c.cfg.UTXO.DefaultSatsPerVByte
	}

	return common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(gasRate*vSize)))
}

func (c *Client) getGasCoinZEC(tx *wire.MsgTx, memo string) common.Coin {
	// https://zips.z.cash/zip-0317
	// 8 bytes value + 1 byte length + OP_RETURN + OP_PUSHBYTES/DATA
	// -> 8 + 1 + 1 + 2 (using two bytes for OP_PUSHDATA2 for simplicity)
	var actionsMemo int
	if len(memo) > 0 {
		bytesOpReturn := 12 + len(memo)
		actionsMemo = (bytesOpReturn + 34 - 1) / 34
	}

	// assume 2 tx out: one to customer + one back to self
	amount := 5000*max(len(tx.TxIn), 2+actionsMemo) + ZecExtraFee

	return common.NewCoin(common.ZECAsset, cosmos.NewUint(uint64(amount)))
}

func (c *Client) buildTx(tx stypes.TxOutItem, sourceScript []byte) (*wire.MsgTx, map[string]int64, error) {
	// Parse memo early to determine if this is a migration (used for dust UTXO inclusion)
	isMigrate := false
	if memoStr := tx.GetMemo(); memoStr != "" {
		if parsedMemo, mErr := mem.ParseMemo(common.LatestVersion, memoStr); mErr == nil {
			isMigrate = parsedMemo.GetType() == mem.TxMigrate
		}
	}

	txes, err := c.getUtxoToSpend(tx.VaultPubKey, c.getPaymentAmount(tx), isMigrate)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to get unspent UTXO: %w", err)
	}
	redeemTx := wire.NewMsgTx(wire.TxVersion)
	totalAmt := int64(0)
	individualAmounts := make(map[string]int64, len(txes))
	for _, item := range txes {
		var txID *chainhash.Hash
		txID, err = chainhash.NewHashFromStr(item.TxID)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to parse txID(%s): %w", item.TxID, err)
		}
		// double check that the utxo is still valid
		outputPoint := wire.NewOutPoint(txID, item.Vout)
		sourceTxIn := wire.NewTxIn(outputPoint, nil, nil)
		redeemTx.AddTxIn(sourceTxIn)
		var amt btcutil.Amount
		amt, err = btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to parse amount(%f): %w", item.Amount, err)
		}
		individualAmounts[formatUtxoKey(txID.String(), item.Vout)] = int64(amt)
		totalAmt += int64(amt)
	}

	var buf []byte
	var nullDataScripts [][]byte
	switch c.cfg.ChainID {
	case common.DOGEChain:
		var outputAddr dogutil.Address
		outputAddr, err = dogutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgDOGE())
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = dogetxscript.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, dogetxscript.MaxDataCarrierSize, dogetxscript.NullDataScript, dogetxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	case common.BCHChain:
		var outputAddr bchutil.Address
		outputAddr, err = bchutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBCH())
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = bchtxscript.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, bchtxscript.MaxDataCarrierSize, bchtxscript.NullDataScript, bchtxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	case common.LTCChain:
		var outputAddr ltcutil.Address
		outputAddr, err = ltctxscript.DecodeAddress(tx.ToAddress.String(), c.getChainCfgLTC())
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = ltctxscript.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, ltctxscript.MaxDataCarrierSize, ltctxscript.NullDataScript, ltctxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	case common.BTCChain:
		var outputAddr btcutil.Address
		outputAddr, err = btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = btctxscript.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, btctxscript.MaxDataCarrierSize, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	case common.ZECChain:
		var outputAddr btcutil.Address
		outputAddr, err = zecutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgZEC().Name)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = zecutil.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, btctxscript.MaxDataCarrierSize, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	// For memoless outbounds, allow empty nullDataScripts
	if len(nullDataScripts) == 0 && len(tx.Memo) != 0 {
		return nil, nil, fmt.Errorf("no null data scripts generated, memo will not be included in the transaction")
	}

	// Parse the memo to be able to identify Migrate or Consolidate outbounds.
	// Use GetMemo() which returns OriginalMemo for memoless outbounds
	memoForParsing := tx.GetMemo()
	var memo mem.Memo
	if memoForParsing == "" {
		memo = mem.NewOutboundMemo(tx.InHash)
	} else {
		memo, err = mem.ParseMemo(common.LatestVersion, memoForParsing)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to parse memo: %w", err)
		}
	}

	totalSize := c.estimateTxSize(txes, nullDataScripts, buf, sourceScript)

	coinToCustomer := tx.Coins.GetCoin(c.cfg.ChainID.GetGasAsset())

	var gasCoin common.Coin
	switch c.cfg.ChainID {
	case common.ZECChain:
		gasCoin = c.getGasCoinZEC(redeemTx, memo.String())
	default:
		gasCoin = c.getGasCoin(tx, totalSize)
	}

	// maxFee in sats
	maxFeeSats := totalSize * c.cfg.UTXO.MaxSatsPerVByte
	gasAmtSats := gasCoin.Amount.Uint64()

	// make sure the transaction fee is not more than the max, otherwise it might reject the transaction
	if gasAmtSats > uint64(maxFeeSats) {
		diffSats := gasAmtSats - uint64(maxFeeSats) // in sats
		c.log.Info().Msgf("gas amount: %d is larger than maximum fee: %d, diff: %d", gasAmtSats, uint64(maxFeeSats), diffSats)
		gasAmtSats = uint64(maxFeeSats)
	} else {
		minRelayFeeSats := c.minRelayFeeSats.Load()
		if gasAmtSats < minRelayFeeSats {
			diffStats := minRelayFeeSats - gasAmtSats
			c.log.Info().Msgf("gas amount: %d is less than min relay fee: %d, diff remove from customer: %d", gasAmtSats, minRelayFeeSats, diffStats)
			gasAmtSats = minRelayFeeSats
		}
	}

	// if the total gas spend is more than max gas , then we have to take away some from the amount pay to customer
	if !tx.MaxGas.IsEmpty() {
		maxGasCoin := tx.MaxGas.ToCoins().GetCoin(c.cfg.ChainID.GetGasAsset())
		if gasAmtSats > maxGasCoin.Amount.Uint64() {
			c.log.Info().Msgf("max gas: %s, however estimated gas need %d", tx.MaxGas, gasAmtSats)
			gasAmtSats = maxGasCoin.Amount.Uint64()
		} else if gasAmtSats < maxGasCoin.Amount.Uint64() && memo.GetType() == mem.TxMigrate {
			// if the tx spend less gas then the estimated MaxGas , then the extra can be added to the coinToCustomer
			gap := maxGasCoin.Amount.Uint64() - gasAmtSats
			c.log.Info().Msgf("max gas is: %s, however only: %d is required, gap: %d goes to the vault migrated to", tx.MaxGas, gasAmtSats, gap)
			coinToCustomer.Amount = coinToCustomer.Amount.Add(cosmos.NewUint(gap))
		}
	} else if memo.GetType() == mem.TxConsolidate {
		gap := gasAmtSats
		c.log.Info().Msgf("consolidate tx, need gas: %d", gap)
		coinToCustomer.Amount = common.SafeSub(coinToCustomer.Amount, cosmos.NewUint(gap))
	}

	gasAmt := btcutil.Amount(gasAmtSats)
	if err = c.temporalStorage.UpsertTransactionFee(gasAmt.ToBTC(), int32(totalSize)); err != nil {
		c.log.Err(err).Msg("fail to save gas info to UTXO storage")
	}

	// pay to customer
	redeemTxOut := wire.NewTxOut(int64(coinToCustomer.Amount.Uint64()), buf)
	redeemTx.AddTxOut(redeemTxOut)

	// Calculate the total cost of P2WPKH outputs for extended memos
	p2wpkhOutputsCost := int64(0)
	if len(nullDataScripts) > 1 {
		// Each P2WPKH output (nullDataScripts[1:]) costs P2WPKHOutputValue()
		p2wpkhOutputsCost = int64(len(nullDataScripts)-1) * tx.Chain.P2WPKHOutputValue()
	}

	// balance to ourselves
	// add output to pay the balance back ourselves
	// Now properly account for P2WPKH outputs cost
	balance := totalAmt - redeemTxOut.Value - int64(gasAmt) - p2wpkhOutputsCost
	c.log.Info().Msgf("total: %d, to customer: %d, gas: %d, p2wpkh_outputs_cost: %d", totalAmt, redeemTxOut.Value, int64(gasAmt), p2wpkhOutputsCost)
	if balance < 0 {
		return nil, nil, fmt.Errorf("not enough balance to pay customer: %d", balance)
	}
	if balance > 0 {
		c.log.Info().Msgf("send %d back to self", balance)
		redeemTx.AddTxOut(wire.NewTxOut(balance, sourceScript))
	}

	// memo
	if len(tx.Memo) != 0 {
		redeemTx.AddTxOut(wire.NewTxOut(0, nullDataScripts[0]))
		for _, script := range nullDataScripts[1:] {
			redeemTx.AddTxOut(wire.NewTxOut(tx.Chain.P2WPKHOutputValue(), script))
		}
	}

	return redeemTx, individualAmounts, nil
}

// MemoToScripts converts a memo to UTXO scripts.
// Up to 80 bytes in a single OP_RETURN output; for longer memos, 79 bytes plus '^' marker in OP_RETURN,
// with remaining data in P2WPKH outputs (20 bytes each).
func MemoToScripts(memo string, maxDataCarrierSize int, nullDataScript, payToWitnessKeyHashScript func([]byte) ([]byte, error)) ([][]byte, error) {
	if len(memo) == 0 {
		return nil, nil
	}

	if len(memo) > constants.MaxMemoSize {
		return nil, fmt.Errorf("memo size %d exceeds maximum size of %d bytes", len(memo), constants.MaxMemoSize)
	}

	data := []byte(memo)

	// Calculate number of scripts: 1 OP_RETURN + ceil(remaining_data / 20) P2WPKH outputs
	remainingDataSize := len(data)
	if remainingDataSize > maxDataCarrierSize {
		remainingDataSize -= (maxDataCarrierSize - 1) // Reserve 1 byte for '^'
	} else {
		remainingDataSize = 0
	}
	numScripts := 1 + (remainingDataSize+19)/20 // 1 for OP_RETURN, plus P2WPKH outputs (20 bytes each)
	scripts := make([][]byte, 0, numScripts)

	// First chunk OP_RETURN: up to 80 bytes; if > 80 bytes, 79 bytes + '^' marker
	firstChunkSize := len(data)
	continuation := false
	if firstChunkSize > maxDataCarrierSize { // Reserve 1 byte for '^' if needed
		firstChunkSize = maxDataCarrierSize - 1
		continuation = true
	}
	firstChunk := make([]byte, 0, maxDataCarrierSize)
	firstChunk = append(firstChunk, data[:firstChunkSize]...)
	if continuation {
		firstChunk = append(firstChunk, '^')
	}
	script, err := nullDataScript(firstChunk)
	if err != nil {
		return nil, fmt.Errorf("fail to create OP_RETURN script: %w", err)
	}
	scripts = append(scripts, script)

	// Remaining data (if any) goes into P2WPKH outputs, 20 bytes each
	if continuation {
		remainingData := data[firstChunkSize:]
		for i := 0; len(remainingData) > 0; i++ {
			// Take up to 20 bytes for this P2WPKH output
			chunkSize := len(remainingData)
			if chunkSize > 20 {
				chunkSize = 20
			}
			hash := make([]byte, 20)
			copy(hash, remainingData[:chunkSize])
			// Remaining bytes (if < 20) are padded with zeros, signaling the end
			// (getMemo stops at a hash ending with "00")
			p2wpkhScript, err := payToWitnessKeyHashScript(hash)
			if err != nil {
				return nil, fmt.Errorf("fail to create P2WPKH script at index %d: %w", i, err)
			}
			scripts = append(scripts, p2wpkhScript)
			// Move to the next chunk
			remainingData = remainingData[chunkSize:]
		}
	}

	return scripts, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// UTXO Consolidation
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) consolidateVaultUTXOs(vault types.Vault, utxosToSpend int64) error {
	lock := c.GetVaultLock(vault.PubKey.String())

	// Hold the vault lock through broadcast to avoid local double-spend on failure/timeout.
	lock.Lock()
	defer lock.Unlock()

	// Probe UTXO availability for consolidation.
	// The amount used here doesn't matter , just to see whether there are more than 15 UTXO available or not
	utxos, err := c.getUtxoToSpend(vault.PubKey, 0, false)
	if err != nil {
		return fmt.Errorf("get utxos to spend: %w", err)
	}

	// Doesn't have enough UTXOs, no consolidation needed.
	if int64(len(utxos)) < utxosToSpend {
		return nil
	}

	txOutItem, err := c.buildConsolidateTxOutItem(vault, utxos)
	if err != nil {
		return err
	}

	height, err := c.bridge.GetBlockHeight()
	if err != nil {
		return fmt.Errorf("get THORChain block height: %w", err)
	}

	rawTx, _, _, err := c.SignTx(txOutItem, height)
	if err != nil {
		return fmt.Errorf("sign consolidate txout item: %w", err)
	}
	if len(rawTx) == 0 {
		c.log.Warn().Str("vault_pubkey", vault.PubKey.String()).Msg("signed consolidate transaction is empty, skipping broadcast")
		return nil
	}

	txID, err := c.BroadcastTx(txOutItem, rawTx)
	if err != nil {
		return fmt.Errorf("broadcast consolidate tx: %w", err)
	}
	c.log.Info().Str("vault_pubkey", vault.PubKey.String()).Msgf("broadcast consolidate tx successfully, hash:%s", txID)
	return nil
}

func (c *Client) buildConsolidateTxOutItem(vault types.Vault, utxos []btcjson.ListUnspentResult) (stypes.TxOutItem, error) {
	// Convert each UTXO amount to sats before summing to avoid float accumulation drift.
	total := btcutil.Amount(0)
	for _, item := range utxos {
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return stypes.TxOutItem{}, fmt.Errorf("convert utxo amount %f: %w", item.Amount, err)
		}
		total += amt
	}

	addr, err := vault.PubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return stypes.TxOutItem{}, fmt.Errorf("get address for pubkey %s: %w", vault.PubKey, err)
	}

	// THORChain usually pays 1.5 of the last observed fee rate.
	feeRate := math.Ceil(float64(c.lastFeeRate.Load()) * 3 / 2)

	return stypes.TxOutItem{
		Chain:            c.cfg.ChainID,
		ToAddress:        addr,
		VaultPubKey:      vault.PubKey,
		VaultPubKeyEddsa: vault.PubKeyEddsa,
		Coins: common.Coins{
			common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(total))),
		},
		Memo:    mem.NewConsolidateMemo().String(),
		MaxGas:  nil,
		GasRate: int64(feeRate),
	}, nil
}

// consolidateUTXOs only required when there is a new block
func (c *Client) consolidateUTXOs() {
	defer func() {
		c.wg.Done()
		c.consolidateInProgress.Store(false)
	}()

	nodeStatus, err := c.bridge.FetchNodeStatus()
	if err != nil {
		c.log.Err(err).Msg("fail to get node status")
		return
	}
	if nodeStatus != types.NodeStatus_Active {
		c.log.Info().Msgf("node is not active , doesn't need to consolidate utxos")
		return
	}
	vaults, err := c.bridge.GetAsgards()
	if err != nil {
		c.log.Err(err).Msg("fail to get current asgards")
		return
	}
	utxosToSpend := c.getMaximumUtxosToSpend()
	for _, vault := range vaults {
		if !vault.Contains(c.nodePubKey) {
			// Not part of this vault , don't need to consolidate UTXOs for this Vault
			continue
		}
		if err = c.consolidateVaultUTXOs(vault, utxosToSpend); err != nil {
			c.log.Err(err).Str("vault_pubkey", vault.PubKey.String()).Msg("fail to consolidate utxos for vault")
		}
	}
}
