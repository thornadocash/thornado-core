package btc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"

	"github.com/btcsuite/btcd/btcutil"
	btctxscript "github.com/btcsuite/btcd/txscript"

	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

////////////////////////////////////////////////////////////////////////////////////////
// UTXO Selection
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) getMaximumUtxosToSpend() int64 {
	utxosToSpend, err := c.bridge.GetConfigValue(constants.UTXO_MaxSpendCount.String())
	if err != nil {
		c.log.Err(err).Str("config", constants.UTXO_MaxSpendCount.String()).Msg("fail to get config")
	}
	if utxosToSpend <= 0 {
		utxosToSpend = c.cfg.UTXO.MaxUTXOsToSpend
	}
	return utxosToSpend
}

func (c *Client) getBTCConfigValue(key constants.ConfigName, fallback int64) int64 {
	value, err := c.bridge.GetConfigValue(key.String())
	if err != nil {
		c.log.Err(err).Str("config", key.String()).Msg("fail to get config")
	}
	if value <= 0 {
		return fallback
	}
	return value
}

// getAllUtxos will iterate unspend utxos for the given address and return the oldest
// set of utxos that can cover the amount.
func (c *Client) getUtxoToSpendAtPath(pubkey common.PubKey, pathIndex uint64, total btcutil.Amount, sweepDust bool) ([]btcjson.ListUnspentResult, error) {
	// get all unspent utxos
	addr, err := c.getVaultAddressAtPath(pubkey, pathIndex)
	if err != nil {
		return nil, fmt.Errorf("fail to get address from pubkey(%s): %w", pubkey, err)
	}
	utxos, err := c.rpc.ListUnspent(addr.String())
	if err != nil {
		return nil, fmt.Errorf("fail to get UTXOs: %w", err)
	}

	// spend UTXO older to younger
	sortUtxosForSpend(utxos)

	var result []btcjson.ListUnspentResult
	var toSpend btcutil.Amount
	minUTXOAmt := btcutil.Amount(c.cfg.ChainID.DustThreshold().Uint64()).ToBTC()
	utxosToSpend := c.getMaximumUtxosToSpend() // can be set by config
	minConfirmations := c.getBTCConfigValue(constants.BTC_ConfirmationsMin, c.cfg.UTXO.MinUTXOConfirmations)
	maxMempoolAncestors := c.getBTCConfigValue(constants.BTC_MaxMempoolAncestors, c.cfg.UTXO.MaxMempoolAncestors)

	for _, item := range utxos {
		if !c.isValidUTXO(item.ScriptPubKey) {
			c.log.Warn().Str("script", item.ScriptPubKey).Msgf("invalid utxo, unable to spend")
			continue
		}

		// analyze-ignore(float-comparison)
		if item.Confirmations < minConfirmations || item.Amount < minUTXOAmt {
			// For migration transactions, include confirmed sub-dust UTXOs to sweep all
			// vault funds. This allows resolving balance mismatches where the on-chain
			// UTXOs don't match the internal ledger, without requiring a consensus change.
			// analyze-ignore(float-comparison)
			if sweepDust && item.Amount < minUTXOAmt && item.Confirmations >= minConfirmations {
				// Allow confirmed sub-dust UTXOs through for migration sweeps
			} else {
				// use all UTXOs sent from base, regardless of confirmations or dust threshold
				isSelfTx := c.isSelfTransaction(item.TxID)

				// confirm sender of the UTXO is not base in case of lost block meta
				if !isSelfTx {
					isSelfTx = c.isFromBase(item.TxID)
				}
				if !isSelfTx {
					continue
				}
			}

			// For unconfirmed UTXOs (even self-transactions), check ancestor, descendant, and combined
			// chain count to avoid exceeding the chain's mempool limits. Set to 0 to disable.
			if item.Confirmations == 0 && maxMempoolAncestors > 0 {
				var entry *btcjson.GetMempoolEntryResult
				entry, err = c.rpc.GetMempoolEntry(item.TxID)
				if err != nil {
					// If we cannot get the mempool entry, the tx is likely confirmed.
					c.log.Debug().Err(err).Str("txid", item.TxID).Msg("failed to get mempool entry")
				}

				// Check the combined ancestor and descendant counts to avoid exceeding mempool
				// limits and receiving "-26: too-long-mempool-chain" errors on broadcast.
				if err == nil && entry.AncestorCount+entry.DescendantCount >= maxMempoolAncestors {
					c.log.Warn().
						Str("txid", item.TxID).
						Int64("ancestor_count", entry.AncestorCount).
						Int64("descendant_count", entry.DescendantCount).
						Int64("max_allowed", maxMempoolAncestors).
						Msg("skipping UTXO with too many ancestors/descendants to avoid mempool chain limit")
					continue
				}
			}
		}

		result = append(result, item)
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, fmt.Errorf("fail to convert to btcutil amount: %w", err)
		}
		toSpend += amt

		// in the scenario that there are too many unspent utxos available, make sure it
		// doesn't spend too much as too much UTXO will cause huge pressure on FROST, also
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

func (c *Client) getUtxoToSpend(pubkey common.PubKey, total btcutil.Amount, sweepDust bool) ([]btcjson.ListUnspentResult, error) {
	return c.getUtxoToSpendAtPath(pubkey, common.MainVaultPathIndex, total, sweepDust)
}

func internalTxType(txType string) bool {
	switch strings.ToLower(strings.TrimSpace(txType)) {
	case "sweep", "migrate", "consolidate":
		return true
	default:
		return false
	}
}

func sourceInputKey(txID string, vout uint32) string {
	return formatUtxoKey(strings.ToLower(txID), vout)
}

type sourceInputMapKey struct {
	txID string
	vout uint32
}

func sourceInputLookupKey(txID string, vout uint32) sourceInputMapKey {
	return sourceInputMapKey{
		txID: strings.ToLower(txID),
		vout: vout,
	}
}

func filterUtxosBySourceInputs(utxos []btcjson.ListUnspentResult, inputs []stypes.TxOutInput, total btcutil.Amount) ([]btcjson.ListUnspentResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("missing source_inputs for internal tx")
	}

	available := make(map[sourceInputMapKey]btcjson.ListUnspentResult, len(utxos))
	for _, item := range utxos {
		available[sourceInputLookupKey(item.TxID, item.Vout)] = item
	}

	seen := make(map[sourceInputMapKey]bool, len(inputs))
	result := make([]btcjson.ListUnspentResult, 0, len(inputs))
	var toSpend btcutil.Amount
	for _, input := range inputs {
		if input.TxID.IsEmpty() {
			return nil, fmt.Errorf("empty source input tx id")
		}
		key := sourceInputLookupKey(input.TxID.String(), input.Vout)
		if seen[key] {
			return nil, fmt.Errorf("duplicate source input %s", sourceInputKey(input.TxID.String(), input.Vout))
		}
		seen[key] = true
		item, ok := available[key]
		if !ok {
			return nil, fmt.Errorf("insufficient available UTXOs: missing source input %s", sourceInputKey(input.TxID.String(), input.Vout))
		}
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, fmt.Errorf("fail to convert source input amount to btcutil amount: %w", err)
		}
		if input.AmountSats > 0 && uint64(amt) != input.AmountSats {
			return nil, fmt.Errorf("source input %s amount mismatch: expected %d, got %d", sourceInputKey(input.TxID.String(), input.Vout), input.AmountSats, uint64(amt))
		}
		result = append(result, item)
		toSpend += amt
	}
	if toSpend < total {
		return nil, fmt.Errorf("insufficient available UTXOs: need %d, only have %d available from %d source inputs", total, toSpend, len(result))
	}
	return result, nil
}

func (c *Client) getSourceUtxosToSpendAtPath(pubkey common.PubKey, pathIndex uint64, sourceInputs []stypes.TxOutInput, total btcutil.Amount) ([]btcjson.ListUnspentResult, error) {
	addr, err := c.getVaultAddressAtPath(pubkey, pathIndex)
	if err != nil {
		return nil, fmt.Errorf("fail to get address from pubkey(%s): %w", pubkey, err)
	}
	utxos, err := c.rpc.ListUnspent(addr.String())
	if err != nil {
		return nil, fmt.Errorf("fail to get UTXOs: %w", err)
	}
	filtered := utxos[:0]
	for _, item := range utxos {
		if !c.isValidUTXO(item.ScriptPubKey) {
			c.log.Warn().Str("script", item.ScriptPubKey).Msgf("invalid sweep source utxo, unable to spend")
			continue
		}
		filtered = append(filtered, item)
	}
	return filterUtxosBySourceInputs(filtered, sourceInputs, total)
}

func sameTxOutInputs(a, b []stypes.TxOutInput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].TxID.Equals(b[i].TxID) || a[i].Vout != b[i].Vout || a[i].AmountSats != b[i].AmountSats {
			return false
		}
	}
	return true
}

func formatUtxoKey(txID string, vout uint32) string {
	return fmt.Sprintf("%s-%d", txID, vout)
}

// vinsUnspent will return true if all the vins are unspent.
func (c *Client) vinsUnspent(tx stypes.TxOutItem, vins []*wire.TxIn) (bool, error) {
	// get all unspent utxos
	addr, err := c.getVaultAddressAtPath(tx.VaultPubKey, tx.VaultPathIndex)
	if err != nil {
		return false, fmt.Errorf("fail to get address from pubkey(%s) at path %d: %w", tx.VaultPubKey, tx.VaultPathIndex, err)
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
	if !tx.MaxGas.IsEmpty() {
		gasAmt := tx.MaxGas.ToCoins().GetCoin(c.cfg.ChainID.GetGasAsset()).Amount
		amtToPay += gasAmt.Uint64()
	}
	return btcutil.Amount(amtToPay)
}

// getSourceScript retrieve pay to addr script from tx source
func (c *Client) getSourceScript(tx stypes.TxOutItem) ([]byte, error) {
	if c.cfg.ChainID.Equals(common.BTCChain) {
		return c.getSchnorrSourceScriptAtPath(tx.VaultPubKey, tx.VaultPathIndex)
	}
	sourceAddr, err := c.getVaultAddress(tx.VaultPubKey)
	if err != nil {
		return nil, fmt.Errorf("fail to get source address: %w", err)
	}

	var addr btcutil.Address
	addr, err = btcutil.DecodeAddress(sourceAddr.String(), c.getChainCfgBTC())
	if err != nil {
		return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
	}
	return btctxscript.PayToAddrScript(addr)
}

////////////////////////////////////////////////////////////////////////////////////////
// Build Transaction
////////////////////////////////////////////////////////////////////////////////////////

// estimateTxSize builds a dummy transaction with the given inputs and outputs and
// returns the exact virtual size (vbytes) according to BIP141.
// For non-segwit chains, it returns the actual serialized size.
func (c *Client) estimateTxSize(txes []btcjson.ListUnspentResult, customerScript, changeScript []byte) int64 {
	return c.estimateTxSizeWithOutputs(txes, [][]byte{customerScript}, changeScript)
}

func (c *Client) estimateTxSizeWithOutputs(txes []btcjson.ListUnspentResult, outputScripts [][]byte, changeScript []byte) int64 {
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

		txIn.Witness = make([][]byte, 2)
		txIn.Witness[0] = make([]byte, 72)
		txIn.Witness[1] = make([]byte, 33)

		tx.AddTxIn(txIn)
	}

	for _, outputScript := range outputScripts {
		tx.AddTxOut(wire.NewTxOut(0, outputScript))
	}

	// Add change output (will be added if balance > 0)
	tx.AddTxOut(wire.NewTxOut(0, changeScript))

	strippedSize := tx.SerializeSizeStripped()
	totalSize := tx.SerializeSize()
	return int64((strippedSize*3 + totalSize + 3) / 4)
}

// isSegwitChain returns true if the chain supports segwit transactions
func (c *Client) isSegwitChain() bool {
	return true
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
		gasRate = c.getBTCConfigValue(constants.BTC_DefaultSatsPerVByte, c.cfg.UTXO.DefaultSatsPerVByte)
	}

	return common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(gasRate*vSize)))
}

func (c *Client) capGasAmtSats(tx stypes.TxOutItem, totalSize int64, gasCoin common.Coin) uint64 {
	gasAmtSats := gasCoin.Amount.Uint64()

	maxFeeSats := totalSize * c.getBTCConfigValue(constants.BTC_MaxSatsPerVByte, c.cfg.UTXO.MaxSatsPerVByte)
	if gasAmtSats > uint64(maxFeeSats) {
		diffSats := gasAmtSats - uint64(maxFeeSats)
		c.log.Info().Msgf("gas amount: %d is larger than maximum fee: %d, diff: %d", gasAmtSats, uint64(maxFeeSats), diffSats)
		gasAmtSats = uint64(maxFeeSats)
	} else {
		minRelayFeeSats := c.minRelayFeeSats.Load()
		if gasAmtSats < minRelayFeeSats {
			diffSats := minRelayFeeSats - gasAmtSats
			c.log.Info().Msgf("gas amount: %d is less than min relay fee: %d, diff remove from customer: %d", gasAmtSats, minRelayFeeSats, diffSats)
			gasAmtSats = minRelayFeeSats
		}
	}

	if !tx.MaxGas.IsEmpty() {
		maxGasCoin := tx.MaxGas.ToCoins().GetCoin(c.cfg.ChainID.GetGasAsset())
		if gasAmtSats > maxGasCoin.Amount.Uint64() {
			c.log.Info().Msgf("max gas: %s, however estimated gas need %d", tx.MaxGas, gasAmtSats)
			gasAmtSats = maxGasCoin.Amount.Uint64()
		}
	}
	return gasAmtSats
}

func (c *Client) buildTx(tx stypes.TxOutItem, sourceScript []byte) (*wire.MsgTx, map[string]int64, error) {
	var txes []btcjson.ListUnspentResult
	var err error
	if len(tx.SourceInputs) > 0 {
		txes, err = c.getSourceUtxosToSpendAtPath(tx.VaultPubKey, tx.VaultPathIndex, tx.SourceInputs, c.getPaymentAmount(tx))
	} else {
		txes, err = c.getUtxoToSpendAtPath(tx.VaultPubKey, tx.VaultPathIndex, c.getPaymentAmount(tx), false)
	}
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

	outputAddr, err := btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
	if err != nil {
		return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
	}
	buf, err := btctxscript.PayToAddrScript(outputAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
	}

	totalSize := c.estimateTxSize(txes, buf, sourceScript)

	coinToCustomer := tx.Coins.GetCoin(c.cfg.ChainID.GetGasAsset())

	gasCoin := c.getGasCoin(tx, totalSize)
	gasAmtSats := c.capGasAmtSats(tx, totalSize, gasCoin)

	gasAmt := btcutil.Amount(gasAmtSats)
	if err = c.temporalStorage.UpsertTransactionFee(gasAmt.ToBTC(), int32(totalSize)); err != nil {
		c.log.Err(err).Msg("fail to save gas info to UTXO storage")
	}

	outputAmount, err := internalTxOutputAmount(tx.TxType, totalAmt, int64(gasAmt), int64(coinToCustomer.Amount.Uint64()))
	if err != nil {
		return nil, nil, err
	}

	// pay to customer
	redeemTxOut := wire.NewTxOut(outputAmount, buf)
	redeemTx.AddTxOut(redeemTxOut)

	// balance to ourselves
	// add output to pay the balance back ourselves
	balance := totalAmt - redeemTxOut.Value - int64(gasAmt)
	c.log.Info().Msgf("total: %d, to customer: %d, gas: %d", totalAmt, redeemTxOut.Value, int64(gasAmt))
	if balance < 0 {
		return nil, nil, fmt.Errorf("not enough balance to pay customer: %d", balance)
	}
	if balance > 0 {
		c.log.Info().Msgf("send %d back to self", balance)
		redeemTx.AddTxOut(wire.NewTxOut(balance, sourceScript))
	}

	return redeemTx, individualAmounts, nil
}

func internalTxOutputAmount(txType string, totalAmt, gasAmt, coinAmt int64) (int64, error) {
	outputAmount := coinAmt
	// Explicit internal tx types are custody sweeps. Spend the full selected
	// balance so child/old vault addresses do not retain change.
	switch strings.ToLower(strings.TrimSpace(txType)) {
	case "consolidate", "migrate", "sweep":
		outputAmount = totalAmt - gasAmt
		if outputAmount <= 0 {
			return 0, fmt.Errorf("not enough balance to sweep vault path: %d", outputAmount)
		}
	}
	return outputAmount, nil
}

func batchableBaseVaultTx(tx stypes.TxOutItem) bool {
	if !tx.Chain.Equals(common.BTCChain) {
		return false
	}
	if tx.VaultPathIndex != common.MainVaultPathIndex {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(tx.TxType)) {
	case "out", "refund", "":
		return true
	default:
		return false
	}
}

func btcBatchMaxGas(txs []stypes.TxOutItem) common.Gas {
	maxGas := common.Gas{}
	for _, tx := range txs {
		maxGas = maxGas.Add(tx.MaxGas...)
	}
	return maxGas
}

func (c *Client) buildTxBatch(txs []stypes.TxOutItem, sourceScript []byte) (*wire.MsgTx, map[string]int64, int64, error) {
	if len(txs) == 0 {
		return nil, nil, 0, fmt.Errorf("empty tx batch")
	}
	first := txs[0]
	if !batchableBaseVaultTx(first) {
		return nil, nil, 0, fmt.Errorf("tx is not batchable")
	}

	outputScripts := make([][]byte, 0, len(txs))
	totalOutput := int64(0)
	gasRate := first.GasRate
	prescribedInputs := append([]stypes.TxOutInput(nil), first.SourceInputs...)
	batchMaxGas := btcBatchMaxGas(txs)
	for _, tx := range txs {
		if !batchableBaseVaultTx(tx) {
			return nil, nil, 0, fmt.Errorf("tx is not batchable: %s", tx.TxType)
		}
		if !tx.Chain.Equals(first.Chain) || !tx.VaultPubKey.Equals(first.VaultPubKey) || tx.VaultPathIndex != first.VaultPathIndex {
			return nil, nil, 0, fmt.Errorf("tx batch mixes source vaults")
		}
		if !sameTxOutInputs(prescribedInputs, tx.SourceInputs) {
			return nil, nil, 0, fmt.Errorf("tx batch mixes source inputs")
		}
		if tx.GasRate > gasRate {
			gasRate = tx.GasRate
		}
		outputAddr, err := btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, nil, 0, fmt.Errorf("fail to decode output address: %w", err)
		}
		if !strings.EqualFold(outputAddr.String(), tx.ToAddress.String()) {
			return nil, nil, 0, fmt.Errorf("output address %s cannot roundtrip", tx.ToAddress.String())
		}
		script, err := btctxscript.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		coin := tx.Coins.GetCoin(c.cfg.ChainID.GetGasAsset())
		if coin.IsEmpty() {
			return nil, nil, 0, fmt.Errorf("tx batch item has no gas asset coin")
		}
		outputScripts = append(outputScripts, script)
		totalOutput += int64(coin.Amount.Uint64())
	}

	selectAmount := btcutil.Amount(totalOutput)
	var txes []btcjson.ListUnspentResult
	var err error
	var totalSize int64
	var gasAmt btcutil.Amount
	feeTx := first
	feeTx.GasRate = gasRate
	if !batchMaxGas.IsEmpty() {
		feeTx.MaxGas = batchMaxGas
	}
	for attempt := 0; attempt < 2; attempt++ {
		if len(prescribedInputs) > 0 {
			txes, err = c.getSourceUtxosToSpendAtPath(first.VaultPubKey, first.VaultPathIndex, prescribedInputs, selectAmount)
		} else {
			txes, err = c.getUtxoToSpendAtPath(first.VaultPubKey, first.VaultPathIndex, selectAmount, false)
		}
		if err != nil {
			return nil, nil, 0, fmt.Errorf("fail to get unspent UTXO: %w", err)
		}
		totalSize = c.estimateTxSizeWithOutputs(txes, outputScripts, sourceScript)
		gasCoin := c.getGasCoin(feeTx, totalSize)
		gasAmtSats := c.capGasAmtSats(feeTx, totalSize, gasCoin)
		gasAmt = btcutil.Amount(gasAmtSats)
		needed := btcutil.Amount(totalOutput) + gasAmt
		if selectAmount >= needed {
			break
		}
		selectAmount = needed
	}

	redeemTx := wire.NewMsgTx(wire.TxVersion)
	totalInput := int64(0)
	individualAmounts := make(map[string]int64, len(txes))
	for _, item := range txes {
		txID, err := chainhash.NewHashFromStr(item.TxID)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("fail to parse txID(%s): %w", item.TxID, err)
		}
		outputPoint := wire.NewOutPoint(txID, item.Vout)
		redeemTx.AddTxIn(wire.NewTxIn(outputPoint, nil, nil))
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("fail to parse amount(%f): %w", item.Amount, err)
		}
		individualAmounts[formatUtxoKey(txID.String(), item.Vout)] = int64(amt)
		totalInput += int64(amt)
	}

	for i, script := range outputScripts {
		coin := txs[i].Coins.GetCoin(c.cfg.ChainID.GetGasAsset())
		redeemTx.AddTxOut(wire.NewTxOut(int64(coin.Amount.Uint64()), script))
	}

	if err = c.temporalStorage.UpsertTransactionFee(gasAmt.ToBTC(), int32(totalSize)); err != nil {
		c.log.Err(err).Msg("fail to save gas info to UTXO storage")
	}

	balance := totalInput - totalOutput - int64(gasAmt)
	c.log.Info().
		Int64("total", totalInput).
		Int64("to_customers", totalOutput).
		Int64("gas", int64(gasAmt)).
		Int("outputs", len(outputScripts)).
		Msg("built BTC outbound batch")
	if balance < 0 {
		return nil, nil, 0, fmt.Errorf("not enough balance to pay batch: %d", balance)
	}
	if balance > 0 {
		redeemTx.AddTxOut(wire.NewTxOut(balance, sourceScript))
	}

	return redeemTx, individualAmounts, totalOutput, nil
}

// sortUtxosForSpend orders UTXOs oldest-to-youngest for deterministic coin
// selection: most confirmations first, ties broken by ascending txid. Stable so
// equal keys preserve input order. Extracted for cross-language conformance
// testing (see sighash_conformance_test.go / the Rust bifrost-signer crate).
func sortUtxosForSpend(utxos []btcjson.ListUnspentResult) {
	sort.SliceStable(utxos, func(i, j int) bool {
		if utxos[i].Confirmations != utxos[j].Confirmations {
			return utxos[i].Confirmations > utxos[j].Confirmations
		}
		return utxos[i].TxID < utxos[j].TxID
	})
}
