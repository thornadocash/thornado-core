package utxo

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
	btctxscript "github.com/thornadocash/go-thornado/bifrost/txscript/txscript"

	btypes "github.com/thornadocash/go-thornado/bifrost/blockscanner/types"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/shared/utxo"
	"github.com/thornadocash/go-thornado/bifrost/thorclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

////////////////////////////////////////////////////////////////////////////////////////
// Address Checks
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) getAsgardAddress() ([]common.Address, error) {
	return utxo.GetAsgardAddressCached(&c.asgardCache, c.cfg.ChainID, c.bridge, constants.ThornadoBlockTime)
}

func (c *Client) isAsgardAddress(addressToCheck string) bool {
	asgards, err := c.getAsgardAddress()
	if err != nil {
		c.log.Err(err).Msg("fail to get asgard addresses")
		if len(asgards) == 0 {
			return false
		}
	}
	for _, addr := range asgards {
		if strings.EqualFold(addr.String(), addressToCheck) {
			return true
		}
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////////////
// Reorg Handling
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) processReorg(block *btcjson.GetBlockVerboseTxResult) ([]types.TxIn, error) {
	previousHeight := block.Height - 1
	prevBlockMeta, err := c.temporalStorage.GetBlockMeta(previousHeight)
	if err != nil {
		return nil, fmt.Errorf("fail to get block meta of height(%d): %w", previousHeight, err)
	}
	if prevBlockMeta == nil {
		return nil, nil
	}
	// the block's previous hash need to be the same as the block hash chain client recorded in block meta
	// blockMetas[PreviousHeight].BlockHash == Block.PreviousHash
	if strings.EqualFold(prevBlockMeta.BlockHash, block.PreviousHash) {
		return nil, nil
	}

	c.log.Info().
		Int64("currentHeight", block.Height).
		Str("previousHash", block.PreviousHash).
		Int64("blockMetaHeight", prevBlockMeta.Height).
		Str("blockMetaHash", prevBlockMeta.BlockHash).
		Msg("re-org detected")

	blockHeights, err := c.reConfirmTx(block.Height)
	if err != nil {
		c.log.Err(err).Msgf("fail to reprocess all txs")
	}
	var txIns []types.TxIn
	for _, height := range blockHeights {
		c.log.Info().Int64("height", height).Msg("rescanning block")
		var b *btcjson.GetBlockVerboseTxResult
		b, err = c.getBlock(height)
		if err != nil {
			c.log.Err(err).Int64("height", height).Msg("fail to get block from RPC")
			continue
		}
		var txIn types.TxIn
		txIn, err = c.extractTxs(b)
		if err != nil {
			c.log.Err(err).Msgf("fail to extract txIn from block")
			continue
		}
		if len(txIn.TxArray) == 0 {
			continue
		}
		txIns = append(txIns, txIn)
	}
	return txIns, nil
}

// reConfirmTx is triggered on detection of a re-org. It will walk backwards from the
// provided height until it finds a block with a matching hash, returning a slice all
// heights between the re-org height and the height of the common ancestor.
func (c *Client) reConfirmTx(height int64) ([]int64, error) {
	var rescanBlockHeights []int64

	// calculate the earliest look back height
	earliestHeight := height - c.cfg.BlockScanner.MaxReorgRescanBlocks
	if earliestHeight < 1 {
		earliestHeight = 1
	}

	// the current block is not yet in block meta, start from previous block
	for i := height - 1; i >= earliestHeight; i-- {
		blockMeta, err := c.temporalStorage.GetBlockMeta(i)
		if err != nil {
			return nil, fmt.Errorf("fail to get block meta %d from local storage: %w", i, err)
		}

		hash, err := c.rpc.GetBlockHash(blockMeta.Height)
		if err != nil {
			c.log.Err(err).Msgf("fail to get block verbose tx result: %d", blockMeta.Height)
		}
		if strings.EqualFold(blockMeta.BlockHash, hash) {
			break // we know about this block, everything prior is okay
		}

		c.log.Info().Int64("height", blockMeta.Height).Msg("re-confirming transactions")

		var errataTxs []types.ErrataTx
		for _, tx := range blockMeta.CustomerTransactions {
			// check if the tx still exists in chain
			if c.confirmTx(tx) {
				c.log.Info().Int64("height", blockMeta.Height).Str("txid", tx).Msg("transaction still exists")
				continue
			}

			// otherwise add it to the errata txs
			c.log.Info().Int64("height", blockMeta.Height).Str("txid", tx).Msg("errata tx")
			errataTxs = append(errataTxs, types.ErrataTx{
				TxID:  common.TxID(tx),
				Chain: c.cfg.ChainID,
			})

			blockMeta.RemoveCustomerTransaction(tx)
		}

		if len(errataTxs) > 0 {
			c.globalErrataQueue <- types.ErrataBlock{
				Height: blockMeta.Height,
				Txs:    errataTxs,
			}
		}

		rescanBlockHeights = append(rescanBlockHeights, blockMeta.Height)

		// update the stored block meta with the new block hash
		var r *btcjson.GetBlockVerboseResult
		r, err = c.rpc.GetBlockVerbose(hash)
		if err != nil {
			c.log.Err(err).Int64("height", blockMeta.Height).Msg("fail to get block verbose result")
		}
		blockMeta.PreviousHash = r.PreviousHash
		blockMeta.BlockHash = r.Hash
		if err = c.temporalStorage.SaveBlockMeta(blockMeta.Height, blockMeta); err != nil {
			c.log.Err(err).Int64("height", blockMeta.Height).Msg("fail to save block meta of height")
		}
	}
	return rescanBlockHeights, nil
}

func (c *Client) confirmTx(txid string) bool {
	// since daemons are run with the tx index enabled, this covers block and mempool
	_, err := c.rpc.GetRawTransaction(txid)
	if err != nil {
		c.log.Err(err).Str("txid", txid).Msg("fail to get tx")
	}
	return err == nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Mempool Cache
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) removeFromMemPoolCache(hash string) {
	if err := c.temporalStorage.UntrackMempoolTx(hash); err != nil {
		c.log.Err(err).Str("txid", hash).Msg("fail to remove from mempool cache")
	}
}

func (c *Client) tryAddToMemPoolCache(hash string) bool {
	added, err := c.temporalStorage.TrackMempoolTx(hash)
	if err != nil {
		c.log.Err(err).Str("txid", hash).Msg("fail to add to mempool cache")
	}
	return added
}

func (c *Client) canDeleteBlock(blockMeta *utxo.BlockMeta) bool {
	if blockMeta == nil {
		return true
	}
	for _, tx := range blockMeta.SelfTransactions {
		if result, err := c.rpc.GetMempoolEntry(tx); err == nil && result != nil {
			c.log.Info().Str("txid", tx).Msg("still in mempool, block cannot be deleted")
			return false
		}
	}
	return true
}

func (c *Client) updateNetworkInfo() {
	networkInfo, err := c.rpc.GetNetworkInfo()
	if err != nil {
		c.log.Err(err).Msg("fail to get network info")
		return
	}
	amt, err := btcutil.NewAmount(networkInfo.RelayFee)
	if err != nil {
		c.log.Err(err).Msg("fail to get minimum relay fee")
		return
	}
	c.minRelayFeeSats.Store(uint64(amt.ToUnit(btcutil.AmountSatoshi)))
}

func (c *Client) sendNetworkFee(height int64) error {
	hash, err := c.rpc.GetBlockHash(height)
	if err != nil {
		return fmt.Errorf("fail to get block hash: %w", err)
	}
	bs, err := c.rpc.GetBlockStats(hash)
	if err != nil {
		return fmt.Errorf("fail to get block stats: %w", err)
	}
	feeRate := uint64(bs.AverageFeeRate)

	if feeRate == 0 {
		return nil
	}

	c.networkFeeLock.Lock()
	defer c.networkFeeLock.Unlock()

	minRelayFeeSats := c.minRelayFeeSats.Load()
	if c.cfg.UTXO.EstimatedAverageTxSize*feeRate < minRelayFeeSats {
		feeRate = minRelayFeeSats / c.cfg.UTXO.EstimatedAverageTxSize
		if feeRate*c.cfg.UTXO.EstimatedAverageTxSize < minRelayFeeSats {
			feeRate++
		}
	}

	if feeRate < uint64(c.cfg.UTXO.MinSatsPerVByte) {
		feeRate = uint64(c.cfg.UTXO.MinSatsPerVByte)
	}

	// if gas cache blocks are set, use the max gas over that window
	if c.cfg.BlockScanner.GasCacheBlocks > 0 {
		c.feeRateCache = append(c.feeRateCache, feeRate)
		if len(c.feeRateCache) > c.cfg.BlockScanner.GasCacheBlocks {
			c.feeRateCache = c.feeRateCache[len(c.feeRateCache)-c.cfg.BlockScanner.GasCacheBlocks:]
		}
		for _, rate := range c.feeRateCache {
			if rate > feeRate {
				feeRate = rate
			}
		}
	}

	c.m.GetGauge(metrics.GasPrice(c.cfg.ChainID)).Set(float64(feeRate))

	// skip update if fee has not changed
	if c.lastFeeRate.Load() == feeRate {
		return nil
	}

	c.lastFeeRate.Store(feeRate)
	c.m.GetCounter(metrics.GasPriceChange(c.cfg.ChainID)).Inc()

	c.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           c.cfg.ChainID,
		Height:          height,
		TransactionSize: c.cfg.UTXO.EstimatedAverageTxSize,
		TransactionRate: feeRate,
	}

	c.log.Debug().Msg("send network fee to Thornado successfully")
	return nil
}

// sendNetworkFeeFromBlock will send network fee to Thornado based on the block result,
// for chains like Dogecoin which do not support the getblockstats RPC.
func (c *Client) sendNetworkFeeFromBlock(blockResult *btcjson.GetBlockVerboseTxResult) error {
	height := blockResult.Height
	var total float64 // total coinbase value, block reward + all transaction fees in the block
	var totalVSize int32
	for _, tx := range blockResult.Tx {
		if len(tx.Vin) == 1 && tx.Vin[0].IsCoinBase() {
			for _, opt := range tx.Vout {
				total += opt.Value
			}
		} else {
			totalVSize += tx.Vsize
		}
	}

	transactionSize := c.cfg.UTXO.EstimatedAverageTxSize
	var feeRateSats uint64

	// skip updating network fee if there are no utxos (except coinbase) in the block
	if totalVSize == 0 {
		return nil
	}
	amt, err := btcutil.NewAmount(total - c.cfg.ChainID.DefaultCoinbase())
	if err != nil {
		return fmt.Errorf("fail to parse total block fee amount, err: %w", err)
	}

	// average fee rate in sats/vbyte or default min relay fee
	feeRateSats = uint64(amt.ToUnit(btcutil.AmountSatoshi) / float64(totalVSize))
	if c.cfg.UTXO.DefaultMinRelayFeeSats > feeRateSats {
		feeRateSats = c.cfg.UTXO.DefaultMinRelayFeeSats
	}

	// round to prevent fee observation noise
	resolution := uint64(c.cfg.BlockScanner.GasPriceResolution)
	feeRateSats = ((feeRateSats / resolution) + 1) * resolution

	c.networkFeeLock.Lock()
	defer c.networkFeeLock.Unlock()

	// skip fee if less than 1 resolution away from the last
	lastFeeRate := c.lastFeeRate.Load()
	feeDelta := new(big.Int).Sub(big.NewInt(int64(feeRateSats)), big.NewInt(int64(lastFeeRate)))
	feeDelta.Abs(feeDelta)
	if lastFeeRate != 0 && feeDelta.Cmp(big.NewInt(c.cfg.BlockScanner.GasPriceResolution)) != 1 {
		return nil
	}

	c.log.Info().
		Int64("height", height).
		Uint64("lastFeeRate", lastFeeRate).
		Uint64("feeRateSats", feeRateSats).
		Msg("sendNetworkFee")

	c.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           c.cfg.ChainID,
		Height:          height,
		TransactionSize: transactionSize,
		TransactionRate: feeRateSats,
	}

	c.lastFeeRate.Store(feeRateSats)

	return nil
}

func (c *Client) getBlock(height int64) (*btcjson.GetBlockVerboseTxResult, error) {
	hash, err := c.rpc.GetBlockHash(height)
	if err != nil {
		return &btcjson.GetBlockVerboseTxResult{}, err
	}
	return c.rpc.GetBlockVerboseTxs(hash)
}

func (c *Client) isValidUTXO(hexPubKey string) bool {
	buf, decErr := hex.DecodeString(hexPubKey)
	if decErr != nil {
		c.log.Err(decErr).Msgf("fail to decode hex string, %s", hexPubKey)
		return false
	}

	scriptType, addresses, requireSigs, err := btctxscript.ExtractPkScriptAddrs(buf, c.getChainCfgBTC())
	if err != nil {
		c.log.Err(err).Msg("fail to extract pub key script")
		return false
	}
	switch scriptType {
	case btctxscript.MultiSigTy:
		return false
	default:
		return len(addresses) == 1 && requireSigs == 1
	}
}

func (c *Client) isRBFEnabled(tx *btcjson.TxRawResult) bool {
	for _, vin := range tx.Vin {
		if vin.Sequence < (0xffffffff - 1) {
			return true
		}
	}
	return false
}

func (c *Client) getTxIn(tx *btcjson.TxRawResult, height int64, isMemPool bool, vinZeroTxs map[string]*btcjson.TxRawResult) (types.TxInItem, error) {
	if c.ignoreTx(tx, height) {
		c.log.Debug().Int64("height", height).Str("txid", tx.Hash).Msg("ignore tx not matching format")
		return types.TxInItem{}, nil
	}
	// RBF enabled transaction will not be observed until committed to block
	if c.isRBFEnabled(tx) && isMemPool {
		return types.TxInItem{}, nil
	}
	sender, err := c.getSender(tx, vinZeroTxs)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to get sender from tx: %w", err)
	}
	output, err := c.getOutput(sender, tx, false)
	if err != nil {
		if errors.Is(err, btypes.ErrFailOutputMatchCriteria) {
			c.log.Debug().Int64("height", height).Str("txid", tx.Hash).Msg("ignore tx not matching format")
			return types.TxInItem{}, nil
		}
		return types.TxInItem{}, fmt.Errorf("fail to get output from tx: %w", err)
	}

	addresses := c.getAddressesFromScriptPubKey(output.ScriptPubKey)
	toAddr := addresses[0]

	isInbound := c.isAsgardAddress(toAddr)
	if isInbound {
		// only inbound UTXO need to be validated against multi-sig
		if !c.isValidUTXO(output.ScriptPubKey.Hex) {
			return types.TxInItem{}, fmt.Errorf("invalid utxo")
		}
	}
	amount, err := btcutil.NewAmount(output.Value)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to parse float64: %w", err)
	}
	amt := uint64(amount.ToUnit(btcutil.AmountSatoshi))

	gas, err := c.getGas(tx, isInbound)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to get gas from tx: %w", err)
	}
	return types.TxInItem{
		BlockHeight: height,
		Tx:          tx.Txid,
		Sender:      sender,
		To:          toAddr,
		Coins: common.Coins{
			common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(amt)),
		},
		Memo: "",
		Gas:  gas,
	}, nil
}

func (c *Client) getVinZeroTxs(block *btcjson.GetBlockVerboseTxResult) (map[string]*btcjson.TxRawResult, error) {
	vinZeroTxs := make(map[string]*btcjson.TxRawResult)
	start := time.Now()

	dustThreshold := c.cfg.ChainID.DustThreshold().Uint64()

	// create our batches
	batches := [][]string{}
	batch := []string{}
	var count, ignoreCount, failMemoSkipCount, skipDustCount int // just for debug logs
	for i := range block.Tx {
		if c.ignoreTx(&block.Tx[i], block.Height) {
			ignoreCount++
			continue
		}

		// skip if sum of vout value is under thorchain dust threshold
		voutSats, err := sumVoutSats(&block.Tx[i])
		if err != nil {
			c.log.Error().Err(err).Str("txid", block.Tx[i].Txid).Msg("fail to sum vout sats")
		} else if voutSats < dustThreshold {
			skipDustCount++
			continue
		}

		memo, err := c.getMemo(&block.Tx[i])
		if err != nil || len(memo) > constants.MaxMemoSize {
			failMemoSkipCount++
			continue
		}

		count++
		batch = append(batch, block.Tx[i].Vin[0].Txid)
		if len(batch) >= c.cfg.UTXO.TransactionBatchSize {
			batches = append(batches, batch)
			batch = []string{}
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}

	c.log.Debug().
		Int64("height", block.Height).
		Int("ignoreCount", ignoreCount).
		Int("failMemoSkipCount", failMemoSkipCount).
		Int("skipDustCount", skipDustCount).
		Int("count", count).
		Int("batchSize", c.cfg.UTXO.TransactionBatchSize).
		Int("batchCount", len(batches)).
		Msg("getVinZeroTxs")

	// get the vin zero txs one batch at a time
	retries := 0
	for i := 0; i < len(batches); i++ {
		results, errs, err := c.rpc.BatchGetRawTransactionVerbose(batches[i])

		// if there was no rpc error, check for any tx errors
		txErrCount := 0
		if err == nil {
			for _, txErr := range errs {
				if txErr != nil {
					err = txErr
				}
				txErrCount++
			}
		}

		// retry the batch a few times on any errors to avoid wasted work
		if err != nil {
			if retries >= 3 {
				return nil, err
			}

			c.log.Err(err).Int("txErrCount", txErrCount).Msgf("retrying block txs batch %d", i)
			time.Sleep(time.Second)
			retries++
			i-- // retry the same batch
			continue
		}

		// add transactions to block result
		for _, tx := range results {
			vinZeroTxs[tx.Txid] = tx
		}
	}

	c.log.Debug().
		Int64("height", block.Height).
		Dur("duration", time.Since(start)).
		Msg("getVinZeroTxs complete")

	return vinZeroTxs, nil
}

func (c *Client) extractTxs(block *btcjson.GetBlockVerboseTxResult) (types.TxIn, error) {
	txIn := types.TxIn{
		Chain:   c.GetChain(),
		MemPool: false,
	}

	var vinZeroTxs map[string]*btcjson.TxRawResult
	var err error
	if !c.disableVinZeroBatch {
		vinZeroTxs, err = c.getVinZeroTxs(block)
		if err != nil {
			c.log.Error().Err(err).Msg("fail to get txid to vin zero tx, getTxIn will fan out")
		}
	}

	var txItems []*types.TxInItem
	for idx, tx := range block.Tx {
		// mempool transaction get committed to block , thus remove it from mempool cache
		c.removeFromMemPoolCache(tx.Hash)
		var txInItem types.TxInItem
		txInItem, err = c.getTxIn(&block.Tx[idx], block.Height, false, vinZeroTxs)
		if err != nil {
			// expected since vouts below dust threshold are skipped for vinZeroTxs
			c.log.Debug().Str("txid", tx.Txid).Err(err).Msg("fail to get TxInItem")
			continue
		}
		if txInItem.IsEmpty() {
			continue
		}
		if txInItem.Coins.IsEmpty() {
			continue
		}
		if txInItem.Coins[0].Amount.LT(c.cfg.ChainID.DustThreshold()) {
			continue
		}
		var added bool
		added, err = c.temporalStorage.TrackObservedTx(txInItem.Tx)
		if err != nil {
			c.log.Err(err).Msgf("fail to determine whether hash(%s) had been observed before", txInItem.Tx)
		}
		if !added {
			c.log.Info().Msgf("tx: %s had been report before, ignore", txInItem.Tx)
			continue
		}
		txItems = append(txItems, &txInItem)
	}
	txIn.TxArray = txItems
	return txIn, nil
}

// ignoreTx checks if we can already ignore a tx according to preset rules
// Allow up to 10 Vouts with value and 2 OP_RETURN Vouts (for getMemo appending).
func (c *Client) ignoreTx(tx *btcjson.TxRawResult, height int64) bool {
	if len(tx.Vin) == 0 || len(tx.Vout) == 0 || len(tx.Vout) > 12 {
		return true
	}
	if tx.Vin[0].Txid == "" {
		return true
	}
	// LockTime <= current height doesn't affect spendability,
	// and most wallets for users doing Memoless Savers deposits automatically set LockTime to the current height.
	if tx.LockTime > uint32(height) {
		return true
	}
	countWithOutput := 0
	for _, vout := range tx.Vout {
		// analyze-ignore(float-comparison)
		if vout.Value > 0 {
			countWithOutput++
		}
	}

	// none of the output has any value
	if countWithOutput == 0 {
		return true
	}
	// there are more than ten outputs with value in them, not Thornado format
	if countWithOutput > 10 {
		return true
	}
	return false
}

// getOutput retrieve the correct output for both inbound
// outbound tx.
// logic is if sender is a vault then prefer the first Vout with value,
// else prefer the first Vout with value that's to a vault
// an exception need to be made for consolidate tx , because consolidate tx will be send from asgard back asgard itself
func (c *Client) getOutput(sender string, tx *btcjson.TxRawResult, consolidate bool) (btcjson.Vout, error) {
	isSenderAsgard := c.isAsgardAddress(sender)
	for _, vout := range tx.Vout {
		if strings.EqualFold(vout.ScriptPubKey.Type, "nulldata") {
			continue
		}
		// analyze-ignore(float-comparison)
		if vout.Value <= 0 {
			continue
		}
		addresses := c.getAddressesFromScriptPubKey(vout.ScriptPubKey)
		if len(addresses) != 1 {
			// If more than one address, ignore this Vout.
			// TODO check what we do if get multiple addresses
			continue
		}
		receiver := addresses[0]
		// To be observed, either the sender or receiver must be an observed Thornado vault;
		// if the sender is a vault then assume the first Vout is the output (and a later Vout could be change).
		// If the sender isn't a vault, then do do not for instance
		// return a change address Vout as the output if before the vault-inbound Vout.
		if !isSenderAsgard && !c.isAsgardAddress(receiver) {
			continue
		}

		if consolidate && receiver == sender {
			return vout, nil
		}
		if !consolidate && receiver != sender {
			return vout, nil
		}
	}
	return btcjson.Vout{}, btypes.ErrFailOutputMatchCriteria
}

// isFromAsgard returns true if the tx is from asgard and false if not or on error.
// Since this is used to determine UTXOs used for outbounds, the risk of false negative
// is only that vault members may not find consensus on the outbound, whereas aborting
// on the error would guarantee the member is not a part of consensus. Returning a false
// negative should never be done, as it could result in members using an unconfirmed or
// dust VIN not sent by asgard in an outbound, which can be gamed by a malicious party.
func (c *Client) isFromAsgard(txid string) bool {
	// lookup the txid
	tx, err := c.rpc.GetRawTransactionVerbose(txid)
	if err != nil {
		c.log.Error().Err(err).Str("txid", txid).Msg("fail to get tx")
		return false
	}

	// get the sender
	sender, err := c.getSender(tx, nil)
	if err != nil {
		c.log.Error().Err(err).Str("txid", txid).Msg("fail to get sender")
		return false
	}

	// check if the sender is an asgard address
	return c.isAsgardAddress(sender)
}

// getSender returns sender address for a btc tx, using vin:0
func (c *Client) getSender(tx *btcjson.TxRawResult, vinZeroTxs map[string]*btcjson.TxRawResult) (string, error) {
	if len(tx.Vin) == 0 {
		return "", fmt.Errorf("no vin available in tx")
	}

	var vout btcjson.Vout
	if vinZeroTxs != nil {
		vinTx, ok := vinZeroTxs[tx.Vin[0].Txid]
		if !ok {
			// if vouts are below dust this is expected, so skip log noise
			value, err := sumVoutSats(tx)
			if err != nil || value >= c.cfg.ChainID.DustThreshold().Uint64() {
				c.log.Debug().Str("txid", tx.Txid).Msg("vin zero tx not found")
			}
			return "", fmt.Errorf("missing vin zero tx")
		}
		vout = vinTx.Vout[tx.Vin[0].Vout]
	} else {
		vinTx, err := c.rpc.GetRawTransactionVerbose(tx.Vin[0].Txid)
		if err != nil {
			return "", fmt.Errorf("fail to query raw tx")
		}
		vout = vinTx.Vout[tx.Vin[0].Vout]
	}

	// Validate sender UTXO is single-sig to prevent multisig sender spoofing.
	// An attacker could create a bare multisig with [victim_pubkey, attacker_pubkey],
	// sign with their key, and have the transaction attributed to the victim
	// (since ExtractPkScriptAddrs returns addresses[0] for bare multisig).
	// P2SH/P2WSH-wrapped multisig is unaffected (returns single 3xxx/bc1q address).
	if !c.isValidUTXO(vout.ScriptPubKey.Hex) {
		return "", fmt.Errorf("sender utxo must be single-sig")
	}

	addresses := c.getAddressesFromScriptPubKey(vout.ScriptPubKey)
	if len(addresses) == 0 {
		return "", fmt.Errorf("no address available in vout")
	}
	address := addresses[0]

	return address, nil
}

func (c *Client) getAddressesFromScriptPubKey(scriptPubKey btcjson.ScriptPubKeyResult) []string {
	return c.getAddressesFromScriptPubKeyBTC(scriptPubKey)
}

// getMemo returns memo for a btc tx, using vout OP_RETURN
func (c *Client) getMemo(tx *btcjson.TxRawResult) (string, error) {
	var memo string

	for _, vOut := range tx.Vout {
		switch strings.ToLower(vOut.ScriptPubKey.Type) {
		case "witness_v0_keyhash", "pubkeyhash", "nulldata":
			// do nothing
		default:
			continue
		}

		buf, err := hex.DecodeString(vOut.ScriptPubKey.Hex)
		if err != nil {
			c.log.Err(err).Msg("fail to hex decode scriptPubKey")
			continue
		}

		asm, err := btctxscript.DisasmString(buf)

		if err != nil {
			c.log.Err(err).Msg("fail to disasm script pubkey")
			continue
		}
		fields := strings.Fields(asm)

		if len(fields) < 2 {
			// we need at least OP_RETURN + data, or 0 + address
			continue
		}

		if fields[0] == "OP_RETURN" {
			// skip "0" field to avoid log noise
			if fields[1] == "0" {
				continue
			}

			var decoded string
			decoded, err = c.decodeHexString(fields[1])
			if err != nil {
				// silently return no memo to reduce log noise
				return "", nil
			}
			memo += decoded
			continue
		}

		// don't inspect further non OP_RETURN outputs unless we found one
		if len(memo) < constants.MaxOpReturnDataSize {
			continue
		}

		// marker can be at position >= 79 for a single / multiple OP_RETURN_
		if strings.LastIndex(memo, "^") < constants.MaxOpReturnDataSize-1 {
			// no continuation marker found
			continue
		}

		var pubkey string

		switch len(fields) {
		case 2:
			// Pay-to-witness-public-key-hash (P2WPKH) script format
			// Format: <0> <20-byte-key-hash>
			if fields[0] != "0" {
				continue
			}

			pubkey = fields[1]
		case 5:
			// Pay-to-public-key-hash (P2PKH) script format
			// Format: OP_DUP OP_HASH160 <20-byte-key-hash> OP_EQUALVERIFY OP_CHECKSIG
			requiredOps := []string{
				"OP_DUP", "OP_HASH160", fields[2],
				"OP_EQUALVERIFY", "OP_CHECKSIG",
			}

			isValidScript := true
			for i := 0; i < 4; i++ {
				if fields[i] != requiredOps[i] {
					isValidScript = false
					break
				}
			}

			if !isValidScript {
				continue
			}

			pubkey = fields[2]
		default:
			continue
		}

		// process pubkey

		// pubkey hash is ripemd-160, which is 20 bytes, 40 chars in hex
		if len(pubkey) != 40 {
			continue
		}

		// remove trailing zeros, if found
		pubkey = c.regexpRemoveTrailingZeros.ReplaceAllString(pubkey, "")

		decoded, err := c.decodeHexString(pubkey)
		if err != nil {
			// silently return no memo to reduce log noise
			return "", nil
		}
		memo += decoded

		// if pubkey has been stripped (was ending with "00") stop processing
		if len(pubkey) != 40 {
			break
		}

		// if memo > max size, stop processing
		if len(memo) >= constants.MaxMemoSize {
			break
		}
	}

	if strings.LastIndex(memo, "^") >= constants.MaxOpReturnDataSize-1 {
		memo = strings.Replace(memo, "^", "", 1)
	}

	return memo, nil
}

// decodeHexString decodes a provided hex string and returns the result
// as string or "" if the decoding failed
func (c *Client) decodeHexString(hexString string) (string, error) {
	errMsg := "fail to decode data: " + hexString

	decoded, err := hex.DecodeString(hexString)
	if err != nil {
		c.log.Debug().Err(err).Msg(errMsg)
		return "", err
	}

	// check for non alphanumeric chars
	// only allow letters: a-z A-Z, numbers, space and the following symbols:
	// !"#$%&'()*+,-./:;<=>?@[\]^_`{|}~
	for i, b := range decoded {
		if b < 0x20 || b > 0x7E {
			err := fmt.Errorf("invalid hex value at position %d: 0x%X", i, b)
			c.log.Debug().Err(err).Msg(errMsg)
			return "", err
		}
	}

	return string(decoded), nil
}

// getGas returns gas for a tx (sum vin - sum vout)
func (c *Client) getGas(tx *btcjson.TxRawResult, isInbound bool) (common.Gas, error) {
	var sumVin uint64 = 0
	for _, vin := range tx.Vin {
		vinTx, err := c.rpc.GetRawTransactionVerbose(vin.Txid)
		if err != nil {
			return common.Gas{}, fmt.Errorf("fail to query raw tx from node")
		}

		amount, err := btcutil.NewAmount(vinTx.Vout[vin.Vout].Value)
		if err != nil {
			return nil, err
		}
		sumVin += uint64(amount.ToUnit(btcutil.AmountSatoshi))
	}
	var sumVout uint64 = 0
	for _, vout := range tx.Vout {
		// Ignore all values after the first OP_RETURN on outbounds to consider the values
		// in the extra p2wpkh outputs as part of the gas and avoid insolvency.
		if !isInbound && strings.ToLower(vout.ScriptPubKey.Type) == "nulldata" {
			break
		}

		amount, err := btcutil.NewAmount(vout.Value)
		if err != nil {
			return nil, err
		}
		sumVout += uint64(amount.ToUnit(btcutil.AmountSatoshi))
	}
	totalGas := sumVin - sumVout
	return common.Gas{
		common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(totalGas)),
	}, nil
}

func (c *Client) getCoinbaseValue(blockHeight int64) (int64, error) {
	// TODO: this is inefficient; investigate coinbase cache
	result, err := c.getBlock(blockHeight)
	if err != nil {
		return 0, fmt.Errorf("fail to get block verbose tx: %w", err)
	}
	for _, tx := range result.Tx {
		if len(tx.Vin) == 1 && tx.Vin[0].IsCoinBase() {
			total := float64(0)
			for _, opt := range tx.Vout {
				total += opt.Value
			}
			var amt btcutil.Amount
			amt, err = btcutil.NewAmount(total)
			if err != nil {
				return 0, fmt.Errorf("fail to parse amount: %w", err)
			}
			return int64(amt), nil
		}
	}
	return 0, fmt.Errorf("fail to get coinbase value")
}

// getBlockRequiredConfirmation find out how many confirmation the given txIn need to have before it can be send to Thornado
func (c *Client) getBlockRequiredConfirmation(txIn types.TxIn, height int64) (int64, error) {
	asgardAddresses, err := c.getAsgardAddress()
	if err != nil {
		c.log.Err(err).Msg("fail to get asgard addresses")
	}
	totalTxValue := txIn.GetTotalTransactionValue(c.cfg.ChainID.GetGasAsset(), asgardAddresses)
	totalFeeAndSubsidy, err := c.getCoinbaseValue(height)
	if err != nil {
		c.log.Err(err).Msgf("fail to get coinbase value")
	}
	confMul, err := utxo.GetConfMulBasisPoint(c.GetChain().String(), c.bridge)
	if err != nil {
		c.log.Err(err).Msgf("fail to get conf multiplier mimir value for %s", c.GetChain().String())
	}
	if totalFeeAndSubsidy == 0 {
		var cbValue btcutil.Amount
		cbValue, err = btcutil.NewAmount(c.cfg.ChainID.DefaultCoinbase())
		if err != nil {
			return 0, fmt.Errorf("fail to get default coinbase value: %w", err)
		}
		totalFeeAndSubsidy = int64(cbValue)
	}
	confValue := common.GetUncappedShare(confMul, cosmos.NewUint(constants.MaxBasisPts), cosmos.SafeUintFromInt64(totalFeeAndSubsidy))
	confirm := totalTxValue.Quo(confValue).Uint64()
	confirm, err = utxo.MaxConfAdjustment(confirm, c.GetChain().String(), c.bridge)
	if err != nil {
		c.log.Err(err).Msgf("fail to get max conf value adjustment for %s", c.GetChain().String())
	}
	if confirm < c.cfg.MinConfirmations {
		confirm = c.cfg.MinConfirmations
	}
	c.log.Info().Msgf("totalTxValue:%s, totalFeeAndSubsidy:%d, confirm:%d", totalTxValue, totalFeeAndSubsidy, confirm)

	return int64(confirm), nil
}
