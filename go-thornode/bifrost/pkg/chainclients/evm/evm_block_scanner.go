package evm

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	stdatomic "sync/atomic"

	_ "embed"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ecommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	etypes "github.com/ethereum/go-ethereum/core/types"
	ethclient "github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm/types"
	evmtypes "gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	. "gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/common/tokenlist"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/aggregators"
	memo "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
)

////////////////////////////////////////////////////////////////////////////////////////
// EVMScanner
////////////////////////////////////////////////////////////////////////////////////////

type EVMScanner struct {
	cfg                   config.BifrostBlockScannerConfiguration
	logger                zerolog.Logger
	db                    blockscanner.ScannerStorage
	m                     *metrics.Metrics
	errCounter            *prometheus.CounterVec
	gasPriceChanged       bool
	gasPrice              *big.Int
	lastReportedGasPrice  uint64
	ethClient             *ethclient.Client
	ethRpc                *evm.EthRPC
	blockMetaAccessor     evm.BlockMetaAccessor
	globalErrataQueue     chan<- stypes.ErrataBlock
	globalNetworkFeeQueue chan<- common.NetworkFee
	bridge                thorclient.ThorchainBridge
	pubkeyMgr             pubkeymanager.PubKeyValidator
	eipSigner             etypes.Signer
	currentBlockHeight    stdatomic.Int64
	gasCache              []*big.Int
	solvencyReporter      SolvencyReporter
	whitelistTokens       []tokenlist.ERC20Token
	whitelistContracts    []common.Address
	signerCacheManager    *signercache.CacheManager
	tokenManager          *evm.TokenManager

	vaultABI *abi.ABI
	erc20ABI *abi.ABI
}

// NewEVMScanner create a new instance of EVMScanner.
func NewEVMScanner(cfg config.BifrostBlockScannerConfiguration,
	storage blockscanner.ScannerStorage,
	chainID *big.Int,
	ethClient *ethclient.Client,
	ethRpc *evm.EthRPC,
	bridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	solvencyReporter SolvencyReporter,
	signerCacheManager *signercache.CacheManager,
) (*EVMScanner, error) {
	// check required arguments
	if storage == nil {
		return nil, errors.New("storage is nil")
	}
	if m == nil {
		return nil, errors.New("metrics manager is nil")
	}
	if ethClient == nil {
		return nil, errors.New("ETH RPC client is nil")
	}
	if pubkeyMgr == nil {
		return nil, errors.New("pubkey manager is nil")
	}

	// set storage prefixes
	prefixBlockMeta := fmt.Sprintf("%s-blockmeta-", strings.ToLower(cfg.ChainID.String()))
	prefixSignedMeta := fmt.Sprintf("%s-signedtx-", strings.ToLower(cfg.ChainID.String()))
	prefixTokenMeta := fmt.Sprintf("%s-tokenmeta-", strings.ToLower(cfg.ChainID.String()))

	// create block meta accessor
	blockMetaAccessor, err := evm.NewLevelDBBlockMetaAccessor(
		prefixBlockMeta, prefixSignedMeta, storage.GetInternalDb(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create block meta accessor: %w", err)
	}

	// load ABIs
	vaultABI, erc20ABI, err := evm.GetContractABI(routerContractABI, erc20ContractABI)
	if err != nil {
		return nil, fmt.Errorf("failed to create contract abi: %w", err)
	}

	// load token list
	allTokens := tokenlist.GetEVMTokenList(cfg.ChainID).Tokens
	var whitelistTokens []tokenlist.ERC20Token
	for _, addr := range cfg.WhitelistTokens {
		// find matching token in token list
		found := false
		for _, tok := range allTokens {
			if strings.EqualFold(addr, tok.Address) {
				whitelistTokens = append(whitelistTokens, tok)
				found = true
				break
			}
		}

		// all whitelisted tokens must be in the chain token list
		if !found {
			return nil, fmt.Errorf("whitelist token %s not found in token list", addr)
		}
	}

	// create token manager - storage is scoped to chain so assets should not collide
	tokenManager, err := evm.NewTokenManager(
		storage.GetInternalDb(),
		prefixTokenMeta,
		cfg.ChainID.GetGasAsset(),
		defaultDecimals,
		cfg.HTTPRequestTimeout,
		whitelistTokens,
		ethClient,
		routerContractABI,
		erc20ContractABI,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token helper: %w", err)
	}

	// store the token metadata for the chain gas asset
	err = tokenManager.SaveTokenMeta(
		cfg.ChainID.GetGasAsset().Symbol.String(), evm.NativeTokenAddr, defaultDecimals,
	)
	if err != nil {
		return nil, err
	}

	// load whitelist contracts for the chain
	whitelistContracts := []common.Address{}
	for _, agg := range aggregators.DexAggregators() {
		if agg.Chain.Equals(cfg.ChainID) {
			whitelistContracts = append(whitelistContracts, common.Address(agg.Address))
		}
	}

	return &EVMScanner{
		cfg:                  cfg,
		logger:               log.Logger.With().Stringer("chain", cfg.ChainID).Logger(),
		errCounter:           m.GetCounterVec(metrics.BlockScanError(cfg.ChainID)),
		ethRpc:               ethRpc,
		ethClient:            ethClient,
		db:                   storage,
		m:                    m,
		gasPrice:             big.NewInt(0),
		lastReportedGasPrice: 0,
		gasPriceChanged:      false,
		blockMetaAccessor:    blockMetaAccessor,
		bridge:               bridge,
		vaultABI:             vaultABI,
		erc20ABI:             erc20ABI,
		eipSigner:            etypes.NewPragueSigner(chainID),
		pubkeyMgr:            pubkeyMgr,
		gasCache:             make([]*big.Int, 0),
		solvencyReporter:     solvencyReporter,
		whitelistTokens:      whitelistTokens,
		whitelistContracts:   whitelistContracts,
		signerCacheManager:   signerCacheManager,
		tokenManager:         tokenManager,
	}, nil
}

// --------------------------------- exported ---------------------------------

// GetGasPrice returns the current gas price.
func (e *EVMScanner) GetGasPrice() *big.Int {
	if e.cfg.FixedGasRate > 0 {
		return big.NewInt(e.cfg.FixedGasRate)
	}
	return e.gasPrice
}

// GetNetworkFee returns current chain network fee according to Bifrost.
func (e *EVMScanner) GetNetworkFee() (transactionSize, transactionFeeRate uint64) {
	return e.cfg.MaxGasLimit, e.lastReportedGasPrice
}

// GetNonce returns the nonce (including pending) for the given address.
func (e *EVMScanner) GetNonce(addr string) (uint64, error) {
	return e.ethRpc.GetNonce(addr)
}

// GetNonceFinalized returns the nonce for the given address.
func (e *EVMScanner) GetNonceFinalized(addr string) (uint64, error) {
	return e.ethRpc.GetNonceFinalized(addr)
}

// FetchMemPool returns all transactions in the mempool.
func (e *EVMScanner) FetchMemPool(_ int64) (stypes.TxIn, error) {
	return stypes.TxIn{}, nil
}

// GetTokens returns all token meta data.
func (e *EVMScanner) GetTokens() ([]*evmtypes.TokenMeta, error) {
	return e.tokenManager.GetTokens()
}

// FetchTxs extracts all relevant transactions from the block at the provided height.
func (e *EVMScanner) FetchTxs(height, chainHeight int64) (stypes.TxIn, error) {
	// log height every 100 blocks
	if height%100 == 0 {
		e.logger.Info().Int64("height", height).Msg("fetching txs for height")
	}

	// process all transactions in the block
	e.updateCurrentBlockHeight(height)
	block, err := e.ethRpc.GetBlock(height)
	if err != nil {
		return stypes.TxIn{}, err
	}
	txIn, err := e.processBlock(block)
	if err != nil {
		e.logger.Error().Err(err).Int64("height", height).Msg("failed to search tx in block")
		return stypes.TxIn{}, fmt.Errorf("failed to process block: %d, err:%w", height, err)
	}

	// if reorgs are possible on this chain store block meta for handling
	if e.cfg.MaxReorgRescanBlocks > 0 {
		blockMeta := evmtypes.NewBlockMeta(block.Header(), txIn)
		if err = e.blockMetaAccessor.SaveBlockMeta(height, blockMeta); err != nil {
			e.logger.Err(err).Int64("height", height).Msg("fail to save block meta")
		}
		pruneHeight := height - e.cfg.MaxReorgRescanBlocks
		if pruneHeight > 0 {
			defer func() {
				if err = e.blockMetaAccessor.PruneBlockMeta(pruneHeight); err != nil {
					e.logger.Err(err).Int64("height", height).Msg("fail to prune block meta")
				}
			}()
		}
	}

	// skip reporting network fee and solvency if block more than flexibility blocks from tip
	if chainHeight-height > e.cfg.ObservationFlexibilityBlocks {
		return txIn, nil
	}

	// report network fee and solvency
	e.reportNetworkFee(height)
	if e.solvencyReporter != nil {
		if err = e.solvencyReporter(height); err != nil {
			e.logger.Err(err).Msg("failed to report Solvency info to THORNode")
		}
	}

	return txIn, nil
}

func (e *EVMScanner) getCurrentBlockHeight() int64 {
	return e.currentBlockHeight.Load()
}

// updateCurrentBlockHeight records the highest scanner height seen without
// allowing concurrent prefetches to move it backwards.
func (e *EVMScanner) updateCurrentBlockHeight(height int64) {
	for {
		currentHeight := e.currentBlockHeight.Load()
		if height <= currentHeight {
			return
		}
		if e.currentBlockHeight.CompareAndSwap(currentHeight, height) {
			return
		}
	}
}

// --------------------------------- extraction ---------------------------------

func (e *EVMScanner) processBlock(block *etypes.Block) (stypes.TxIn, error) {
	txIn := stypes.TxIn{
		Chain:    e.cfg.ChainID,
		TxArray:  nil,
		Filtered: false,
		MemPool:  false,
	}

	// skip empty blocks
	if block.Transactions().Len() == 0 {
		return txIn, nil
	}

	// process reorg if possible on this chain
	if e.cfg.MaxReorgRescanBlocks > 0 {
		reorgedTxIns, err := e.processReorg(block.Header())
		if err != nil {
			e.logger.Error().Err(err).Msgf("fail to process reorg for block %d", block.NumberU64())
			return txIn, err
		}
		if len(reorgedTxIns) > 0 {
			for _, item := range reorgedTxIns {
				if len(item.TxArray) == 0 {
					continue
				}
				txIn.TxArray = append(txIn.TxArray, item.TxArray...)
			}
		}
	}

	// collect all relevant transactions from the block and their gas prices
	txInBlock, gasPrices, err := e.getTxIn(block)
	if err != nil {
		return txIn, err
	}
	if len(txInBlock.TxArray) > 0 {
		txIn.TxArray = append(txIn.TxArray, txInBlock.TxArray...)
	}

	// update gas price from collected receipt effective gas prices
	e.updateGasPrice(gasPrices)

	return txIn, nil
}

func (e *EVMScanner) getTxInOptimized(method string, block *etypes.Block) (stypes.TxIn, []*big.Int, error) {
	// Use custom method name as it varies between implementation
	// It should be akin to: "fetch all transaction receipts within a block", e.g. getBlockReceipts
	// This has shown to be more efficient than getTransactionReceipt in a batch call
	txInbound := stypes.TxIn{
		Chain:    e.cfg.ChainID,
		Filtered: false,
		MemPool:  false,
	}

	// tx lookup for compatibility with shared evm functions
	txByHash := make(map[string]*etypes.Transaction)
	for _, tx := range block.Transactions() {
		if tx == nil {
			continue
		}

		// skip blob transactions
		if tx.Type() == etypes.BlobTxType {
			continue
		}

		// best effort remove the tx from the signed txs (ok if it does not exist)
		if err := e.blockMetaAccessor.RemoveSignedTxItem(tx.Hash().String()); err != nil {
			e.logger.Err(err).Str("tx hash", tx.Hash().String()).Msg("failed to remove signed tx item")
		}

		txByHash[tx.Hash().String()] = tx
	}

	var receipts []*etypes.Receipt
	blockNumStr := hexutil.EncodeBig(block.Number())
	err := e.ethClient.Client().Call(
		&receipts,
		method,
		blockNumStr,
	)
	if err != nil {
		e.logger.Error().Err(err).Msg("failed to fetch block receipts")
		return stypes.TxIn{}, nil, err
	}

	sem := make(chan struct{}, e.cfg.Concurrency)
	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	var gasPrices []*big.Int

	for _, receipt := range receipts {
		sem <- struct{}{}
		wg.Add(1)
		go func(receipt *etypes.Receipt) {
			defer wg.Done()
			defer func() { <-sem }()
			txForReceipt, ok := txByHash[receipt.TxHash.String()]
			if !ok {
				e.logger.Warn().
					Str("txHash", receipt.TxHash.String()).
					Uint64("blockNumber", block.NumberU64()).
					Msg("receipt tx not in block.Transactions or nil, ignoring...")
				return
			}

			// collect effective gas price from receipt for gas price tracking
			mu.Lock()
			gasPrices = append(gasPrices, receipt.EffectiveGasPrice)
			mu.Unlock()

			// tx without to address is not valid
			if txForReceipt.To() == nil {
				return
			}

			// extract the txInItem
			var txInItem *stypes.TxInItem
			txInItem, err = e.receiptToTxInItem(txForReceipt, receipt)
			if err != nil {
				e.logger.Error().Err(err).Msg("failed to convert receipt to txInItem")
				return
			}

			// skip invalid items
			if txInItem == nil {
				return
			}
			if len(txInItem.To) == 0 {
				return
			}
			if len([]byte(txInItem.Memo)) > constants.MaxMemoSize {
				return
			}

			// add the txInItem to the txInbound
			txInItem.BlockHeight = block.Number().Int64()
			mu.Lock()
			txInbound.TxArray = append(txInbound.TxArray, txInItem)
			mu.Unlock()
		}(receipt)
	}
	wg.Wait()

	if len(txInbound.TxArray) == 0 {
		e.logger.Debug().Uint64("block", block.NumberU64()).Msg("no tx need to be processed in this block")
		return stypes.TxIn{}, gasPrices, nil
	}
	return txInbound, gasPrices, nil
}

func (e *EVMScanner) getTxIn(block *etypes.Block) (stypes.TxIn, []*big.Int, error) {
	// CHANGEME: if an EVM chain supports some way of fetching all transaction receipts
	// within a block, register it here.
	switch e.cfg.ChainID {
	case common.BASEChain, common.BSCChain, common.POLChain:
		return e.getTxInOptimized("eth_getBlockReceipts", block)
	}

	txInbound := stypes.TxIn{
		Chain:    e.cfg.ChainID,
		Filtered: false,
		MemPool:  false,
	}
	var gasPrices []*big.Int

	// collect all relevant transactions from the block into batches
	batches := [][]*etypes.Transaction{}
	batch := []*etypes.Transaction{}
	for _, tx := range block.Transactions() {
		if tx == nil || tx.To() == nil {
			continue
		}

		// skip blob transactions
		if tx.Type() == etypes.BlobTxType {
			continue
		}

		// best effort remove the tx from the signed txs (ok if it does not exist)
		if err := e.blockMetaAccessor.RemoveSignedTxItem(tx.Hash().String()); err != nil {
			e.logger.Err(err).Str("tx hash", tx.Hash().String()).Msg("failed to remove signed tx item")
		}

		batch = append(batch, tx)
		if len(batch) >= e.cfg.TransactionBatchSize {
			batches = append(batches, batch)
			batch = []*etypes.Transaction{}
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}

	// process all batches
	for _, batch := range batches {
		// create the batch rpc request
		var rpcBatch []rpc.BatchElem
		for _, tx := range batch {
			rpcBatch = append(rpcBatch, rpc.BatchElem{
				Method: "eth_getTransactionReceipt",
				Args:   []interface{}{tx.Hash().String()},
				Result: &etypes.Receipt{},
			})
		}

		// send the batch rpc request
		err := e.ethClient.Client().BatchCall(rpcBatch)
		if err != nil {
			e.logger.Error().Int("size", len(batch)).Err(err).Msg("failed to batch fetch transaction receipts")
			return stypes.TxIn{}, nil, err
		}

		// process the batch rpc response
		for i, elem := range rpcBatch {
			if elem.Error != nil {
				if !errors.Is(err, ethereum.NotFound) {
					e.logger.Error().Err(elem.Error).Msg("failed to fetch transaction receipt")
				}
				continue
			}

			// get the receipt
			receipt, ok := elem.Result.(*etypes.Receipt)
			if !ok {
				e.logger.Error().Msg("failed to cast to transaction receipt")
				continue
			}

			// collect effective gas price from receipt for gas price tracking
			gasPrices = append(gasPrices, receipt.EffectiveGasPrice)

			// extract the txInItem
			var txInItem *stypes.TxInItem
			txInItem, err = e.receiptToTxInItem(batch[i], receipt)
			if err != nil {
				e.logger.Error().Err(err).Msg("failed to convert receipt to txInItem")
				continue
			}

			// skip invalid items
			if txInItem == nil {
				continue
			}
			if len(txInItem.To) == 0 {
				continue
			}
			if len([]byte(txInItem.Memo)) > constants.MaxMemoSize {
				continue
			}

			// add the txInItem to the txInbound
			txInItem.BlockHeight = block.Number().Int64()
			txInbound.TxArray = append(txInbound.TxArray, txInItem)
		}
	}

	if len(txInbound.TxArray) == 0 {
		e.logger.Debug().Uint64("block", block.NumberU64()).Msg("no tx need to be processed in this block")
		return stypes.TxIn{}, gasPrices, nil
	}
	return txInbound, gasPrices, nil
}

// TODO: This is only used by unit tests now, but covers receiptToTxInItem internally -
// refactor so this logic is only in test code.
func (e *EVMScanner) getTxInItem(tx *etypes.Transaction) (*stypes.TxInItem, error) {
	if tx == nil || tx.To() == nil {
		return nil, nil
	}

	receipt, err := e.ethRpc.GetReceipt(tx.Hash().Hex())
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get transaction receipt: %w", err)
	}

	return e.receiptToTxInItem(tx, receipt)
}

func (e *EVMScanner) receiptToTxInItem(tx *etypes.Transaction, receipt *etypes.Receipt) (*stypes.TxInItem, error) {
	if receipt.Status != 1 {
		e.logger.Debug().Stringer("txid", tx.Hash()).Uint64("status", receipt.Status).Msg("tx failed")

		// remove failed transactions from signer cache so they are retried
		if e.signerCacheManager != nil {
			e.signerCacheManager.RemoveSigned(tx.Hash().String())
		}

		return e.getTxInFromFailedTransaction(tx, receipt), nil
	}

	disableWhitelist, err := e.bridge.GetMimir(constants.EVMDisableContractWhitelist.String())
	if err != nil {
		e.logger.Err(err).Msgf("fail to get %s", constants.EVMDisableContractWhitelist.String())
		disableWhitelist = 0
	}

	if disableWhitelist == 1 {
		// parse tx without whitelist
		destination := tx.To()
		isToVault, _ := e.pubkeyMgr.IsValidPoolAddress(destination.String(), e.cfg.ChainID)

		// resolve sender upfront to check vault outbounds (approvals, cancels, etc.)
		sender, senderErr := e.eipSigner.Sender(tx)
		var isFromVault bool
		if senderErr == nil {
			isFromVault, _ = e.pubkeyMgr.IsValidPoolAddress(sender.String(), e.cfg.ChainID)
		}

		switch {
		case isToVault:
			// Tx to a vault
			return e.getTxInFromTransaction(tx, receipt)
		case e.isToValidContractAddress(destination, true):
			// Deposit directly to router
			return e.getTxInFromSmartContract(tx, receipt, 0)
		case isFromVault:
			// Tx from a vault (outbound, approval, cancel, etc.)
			return e.getTxInFromTransaction(tx, receipt)
		case evm.IsSmartContractCall(tx, receipt):
			// Tx to a different contract, attempt to parse with max allowable logs
			return e.getTxInFromSmartContract(tx, receipt, int64(e.cfg.MaxContractTxLogs))
		default:
			// ignore tx not involving any vault
			return nil, nil
		}
	} else {
		// parse tx with whitelist
		if e.isToValidContractAddress(tx.To(), true) {
			return e.getTxInFromSmartContract(tx, receipt, 0)
		}
		// check if tx is to or from a vault
		sender, err := e.eipSigner.Sender(tx)
		if err != nil {
			return nil, nil // ignore tx if we cannot determine sender
		}
		isFromVault, _ := e.pubkeyMgr.IsValidPoolAddress(sender.String(), e.cfg.ChainID)
		isToVault, _ := e.pubkeyMgr.IsValidPoolAddress(tx.To().String(), e.cfg.ChainID)
		if isFromVault || isToVault {
			return e.getTxInFromTransaction(tx, receipt)
		}
		// ignore tx not involving any vault
		return nil, nil
	}
}

// --------------------------------- reorg ---------------------------------

// processReorg compares the block's parent hash with the stored block hash. When a
// reorg is detected, it triggers a rescan of all cached blocks in the reorg window.
// The function returns observations from the rescanned blocks.
func (e *EVMScanner) processReorg(header *etypes.Header) ([]stypes.TxIn, error) {
	previousHeight := header.Number.Int64() - 1
	prevBlockMeta, err := e.blockMetaAccessor.GetBlockMeta(previousHeight)
	if err != nil {
		return nil, fmt.Errorf("fail to get block meta of height(%d) : %w", previousHeight, err)
	}

	// skip re-org processing if we did not store the previous block meta
	if prevBlockMeta == nil {
		return nil, nil
	}

	// no re-org if stored block hash at previous height is equal to current block parent
	if strings.EqualFold(prevBlockMeta.BlockHash, header.ParentHash.Hex()) {
		return nil, nil
	}
	e.logger.Info().
		Int64("height", previousHeight).
		Str("stored_hash", prevBlockMeta.BlockHash).
		Str("current_parent_hash", header.ParentHash.Hex()).
		Msg("reorg detected")

	// send erratas and determine the block heights to rescan
	heights, err := e.reprocessTxs()
	if err != nil {
		e.logger.Err(err).Msg("fail to reprocess all txs")
	}

	// rescan heights
	var txIns []stypes.TxIn
	for _, rescanHeight := range heights {
		e.logger.Info().Msgf("rescan block height: %d", rescanHeight)
		var block *etypes.Block
		block, err = e.ethRpc.GetBlock(rescanHeight)
		if err != nil {
			e.logger.Err(err).Int64("height", rescanHeight).Msg("fail to get block")
			continue
		}
		if block.Transactions().Len() == 0 {
			continue
		}
		var txIn stypes.TxIn
		txIn, _, err = e.getTxIn(block) // discard gas prices during reorg rescan
		if err != nil {
			e.logger.Err(err).Int64("height", rescanHeight).Msg("fail to extract txs from block")
			continue
		}
		if len(txIn.TxArray) > 0 {
			txIns = append(txIns, txIn)
		}
	}
	return txIns, nil
}

// reprocessTx is initiated when the chain client detects a reorg. It reads block
// metadata from local storage and processes all transactions, sending an RPC request to
// check each transaction's existence. If a transaction no longer exists, the chain
// client reports this to Thorchain.
//
// The []int64 return value represents the block heights to be rescanned.
func (e *EVMScanner) reprocessTxs() ([]int64, error) {
	blockMetas, err := e.blockMetaAccessor.GetBlockMetas()
	if err != nil {
		return nil, fmt.Errorf("fail to get block metas from local storage: %w", err)
	}
	var rescanBlockHeights []int64
	for _, blockMeta := range blockMetas {
		metaTxs := make([]types.TransactionMeta, 0)
		var errataTxs []stypes.ErrataTx
		for _, tx := range blockMeta.Transactions {
			if e.ethRpc.CheckTransaction(tx.Hash) {
				e.logger.Debug().Msgf("height: %d, tx: %s still exists", blockMeta.Height, tx.Hash)
				metaTxs = append(metaTxs, tx)
				continue
			}

			// send an errata if the transactino no longer exists on chain
			errataTxs = append(errataTxs, stypes.ErrataTx{
				TxID:  common.TxID(tx.Hash),
				Chain: e.cfg.ChainID,
			})
		}
		if len(errataTxs) > 0 {
			e.globalErrataQueue <- stypes.ErrataBlock{
				Height: blockMeta.Height,
				Txs:    errataTxs,
			}
		}

		// fetch the header header to determine if the hash has changed and requires rescan
		var header *etypes.Header
		header, err = e.ethRpc.GetHeader(blockMeta.Height)
		if err != nil {
			e.logger.Err(err).
				Int64("height", blockMeta.Height).
				Msg("fail to get block header to check for reorg")

			// err on the side of caution and rescan the block
			rescanBlockHeights = append(rescanBlockHeights, blockMeta.Height)
			continue
		}

		// if the block hash is different than previously recorded, rescan the block
		if !strings.EqualFold(blockMeta.BlockHash, header.Hash().Hex()) {
			rescanBlockHeights = append(rescanBlockHeights, blockMeta.Height)
		}

		// save the updated block meta
		blockMeta.PreviousHash = header.ParentHash.Hex()
		blockMeta.BlockHash = header.Hash().Hex()
		blockMeta.Transactions = metaTxs
		if err = e.blockMetaAccessor.SaveBlockMeta(blockMeta.Height, blockMeta); err != nil {
			e.logger.Err(err).Int64("height", blockMeta.Height).Msg("fail to save block meta")
		}
	}
	return rescanBlockHeights, nil
}

// --------------------------------- gas ---------------------------------

// updateGasPrice calculates and stores the current gas price to reported to thornode
func (e *EVMScanner) updateGasPrice(prices []*big.Int) {
	// skip empty blocks
	if len(prices) == 0 {
		return
	}

	// filter out nil gas prices (defensive check for EIP-1559 and other dynamic fee transactions)
	validPrices := make([]*big.Int, 0, len(prices))
	for _, price := range prices {
		if price != nil {
			validPrices = append(validPrices, price)
		}
	}
	if len(validPrices) == 0 {
		return
	}

	// find the median gas price in the block
	sort.Slice(validPrices, func(i, j int) bool { return validPrices[i].Cmp(validPrices[j]) == -1 })
	gasPrice := validPrices[len(validPrices)/2]

	// add to the cache
	e.gasCache = append(e.gasCache, gasPrice)
	if len(e.gasCache) > e.cfg.GasCacheBlocks {
		e.gasCache = e.gasCache[(len(e.gasCache) - e.cfg.GasCacheBlocks):]
	}

	// skip update unless cache is full
	if len(e.gasCache) < e.cfg.GasCacheBlocks {
		return
	}

	// compute the median of the median prices in the cache
	medians := []*big.Int{}
	for _, price := range e.gasCache {
		if price != nil {
			medians = append(medians, price)
		}
	}
	if len(medians) == 0 {
		return
	}
	sort.Slice(medians, func(i, j int) bool { return medians[i].Cmp(medians[j]) == -1 })
	median := medians[len(medians)/2]

	// round the price up to nearest configured resolution, converting gas rate units to wei
	_, gasRateUnitsPerOne := e.cfg.ChainID.GetGasUnits()
	resolution := cosmos.NewUint(uint64(e.cfg.GasPriceResolution)).MulUint64(common.WeiPerOne).Quo(gasRateUnitsPerOne).BigInt()
	median.Add(median, new(big.Int).Sub(resolution, big.NewInt(1)))
	median = median.Div(median, resolution)
	median = median.Mul(median, resolution)
	e.gasPrice = median

	// record metrics
	gasPriceFloat, _ := new(big.Float).SetInt64(e.gasPrice.Int64()).Float64()
	e.m.GetGauge(metrics.GasPrice(e.cfg.ChainID)).Set(gasPriceFloat)
	e.m.GetCounter(metrics.GasPriceChange(e.cfg.ChainID)).Inc()
}

// reportNetworkFee reports current network fee to thornode
func (e *EVMScanner) reportNetworkFee(height int64) {
	gasPrice := e.GetGasPrice()

	// skip posting if there is not yet a fee
	if gasPrice.Cmp(big.NewInt(0)) == 0 {
		return
	}

	// gas rate from wei to gas rate units
	_, gasRateUnitsPerOne := e.cfg.ChainID.GetGasUnits()
	gasRate := cosmos.NewUintFromBigInt(gasPrice).Mul(gasRateUnitsPerOne).QuoUint64(common.WeiPerOne).Uint64()
	if gasRate == 0 {
		gasRate = 1
	}

	// skip fee if less than 1 resolution away from the last
	var feeDelta uint64
	if gasRate >= e.lastReportedGasPrice {
		feeDelta = gasRate - e.lastReportedGasPrice
	} else {
		feeDelta = e.lastReportedGasPrice - gasRate
	}
	if e.lastReportedGasPrice != 0 && feeDelta <= uint64(e.cfg.GasPriceResolution) {
		skip := true

		// every 100 blocks send the fee if none is set
		if height%100 == 0 {
			hasNetworkFee, err := e.bridge.HasNetworkFee(e.cfg.ChainID)
			skip = err != nil || hasNetworkFee
		}

		if skip {
			return
		}
	}

	// post to thorchain
	e.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           e.cfg.ChainID,
		Height:          height,
		TransactionSize: e.cfg.MaxGasLimit,
		TransactionRate: gasRate,
	}
	e.lastReportedGasPrice = gasRate
}

// --------------------------------- parse transaction ---------------------------------

func (e *EVMScanner) getTxInFromTransaction(tx *etypes.Transaction, receipt *etypes.Receipt) (*stypes.TxInItem, error) {
	txInItem := &stypes.TxInItem{
		Tx: tx.Hash().Hex()[2:], // drop the "0x" prefix
	}

	sender, err := e.eipSigner.Sender(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}
	txInItem.Sender = strings.ToLower(sender.String())
	txInItem.To = strings.ToLower(tx.To().String())

	// on native transactions the memo is hex encoded in the data field
	data := tx.Data()
	if len(data) > 0 {
		var memo []byte
		memo, err = hex.DecodeString(string(data))
		if err != nil {
			txInItem.Memo = string(data)
		} else {
			txInItem.Memo = string(memo)
		}
	}

	nativeValue := e.tokenManager.ConvertAmount(evm.NativeTokenAddr, tx.Value())
	txInItem.Coins = append(txInItem.Coins, common.NewCoin(e.cfg.ChainID.GetGasAsset(), nativeValue))
	txGasPrice := receipt.EffectiveGasPrice
	if txGasPrice == nil {
		txGasPrice = big.NewInt(0)
	}
	txInItem.Gas = common.MakeEVMGas(e.cfg.ChainID, txGasPrice, receipt.GasUsed, receipt.L1Fee)
	txInItem.Gas[0].Asset = e.cfg.ChainID.GetGasAsset()

	// drop non-outbound transactions below dust threshold
	isOutbound, _ := e.pubkeyMgr.IsValidPoolAddress(sender.String(), e.cfg.ChainID)
	if !isOutbound &&
		len(txInItem.Coins) == 1 &&
		txInItem.Coins[0].Asset.Equals(e.cfg.ChainID.GetGasAsset()) &&
		txInItem.Coins[0].Amount.LT(e.cfg.ChainID.DustThreshold()) {

		e.logger.Debug().Str("hash", tx.Hash().Hex()).Msg("dropping tx below dust threshold")
		return nil, nil
	}

	if txInItem.Coins.IsEmpty() {
		if isOutbound {
			// outbound with no coins that is not a cancel is assumed to be router approval
			isCancel := txInItem.Sender == txInItem.To
			if !isCancel {
				txInItem.Memo = "" // clear the memo (otherwise contains call data)
			}

			// Outbounds with no coins are either a cancel from unstuck.go, or a router
			// token approval - both should be observed to take gas fees from the vault.
			observableAmount := e.cfg.ChainID.DustThreshold()
			txInItem.Coins = common.NewCoins(common.NewCoin(txInItem.Gas[0].Asset, observableAmount))

			// remove the outbound from signer cache so it can be re-attempted
			e.signerCacheManager.RemoveSigned(tx.Hash().Hex())
		} else {
			e.logger.Debug().Msgf("there is no coin in this tx, ignore, %+v", txInItem)
			return nil, nil
		}
	}

	return txInItem, nil
}

// isToValidContractAddress this method make sure the transaction to address is to
// THORChain router or a whitelist address
func (e *EVMScanner) isToValidContractAddress(addr *ecommon.Address, includeWhiteList bool) bool {
	if addr == nil {
		return false
	}
	// get the smart contract used by thornode
	contractAddresses := e.pubkeyMgr.GetContracts(e.cfg.ChainID, false)
	if includeWhiteList {
		contractAddresses = append(contractAddresses, e.whitelistContracts...)
	}

	// combine the whitelist smart contract address
	addrString := addr.String() // this hashes internally so do it once
	for _, item := range contractAddresses {
		if strings.EqualFold(item.String(), addrString) {
			return true
		}
	}
	return false
}

// getTxInFromSmartContract returns txInItem
func (e *EVMScanner) getTxInFromSmartContract(tx *etypes.Transaction, receipt *etypes.Receipt, maxLogs int64) (*stypes.TxInItem, error) {
	txInItem := &stypes.TxInItem{
		Tx: tx.Hash().Hex()[2:], // drop the "0x" prefix
	}
	sender, err := e.eipSigner.Sender(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}
	txInItem.Sender = strings.ToLower(sender.String())
	// 1 is Transaction success state
	if receipt.Status != 1 {
		e.logger.Debug().Stringer("txid", tx.Hash()).Uint64("status", receipt.Status).Msg("tx failed")
		return nil, nil
	}
	p := evm.NewSmartContractLogParser(e.isToValidContractAddress,
		e.tokenManager.GetAssetFromTokenAddress,
		e.tokenManager.GetTokenDecimalsForTHORChain,
		e.tokenManager.ConvertAmount,
		e.vaultABI,
		e.cfg.ChainID.GetGasAsset(),
		maxLogs,
	)

	// txInItem will be changed in p.getTxInItem function, so if the function return an
	// error txInItem should be abandoned
	isVaultTransfer, err := p.GetTxInItem(receipt.Logs, txInItem)
	if err != nil {
		return nil, fmt.Errorf("failed to parse logs, err: %w", err)
	}
	if isVaultTransfer {
		contractAddresses := e.pubkeyMgr.GetContracts(e.cfg.ChainID, false)
		isDirectlyToRouter := false
		for _, item := range contractAddresses {
			if strings.EqualFold(item.String(), tx.To().String()) {
				isDirectlyToRouter = true
				break
			}
		}
		if isDirectlyToRouter {
			// it is important to keep this part outside the above loop, as when we do router
			// upgrade, which might generate multiple deposit event, along with tx that has
			// native value in it
			nativeValue := cosmos.NewUintFromBigInt(tx.Value())
			if !nativeValue.IsZero() {
				nativeValue = e.tokenManager.ConvertAmount(evm.NativeTokenAddr, tx.Value())
				if txInItem.Coins.GetCoin(e.cfg.ChainID.GetGasAsset()).IsEmpty() && !nativeValue.IsZero() {
					txInItem.Coins = append(txInItem.Coins, common.NewCoin(e.cfg.ChainID.GetGasAsset(), nativeValue))
				}
			}
		}
	}

	// Validate that the deposit's target vault is associated with the router
	// that emitted the event. This prevents an attacker from depositing to an
	// old router targeting the current active vault (or vice versa), which would
	// cause bifrost to observe an inbound the vault cannot actually spend.
	// Skip this check for vault-initiated transactions (e.g. migrations), which
	// legitimately use the old router to send to a vault on a new router.
	// NOTE: We compare against the log-emitting address (the actual router that
	// produced the Deposit event), NOT tx.To(), because aggregator contracts
	// forward deposits to the router internally — tx.To() is the aggregator.
	if txInItem.To != "" {
		senderIsVault, _ := e.pubkeyMgr.IsValidPoolAddress(txInItem.Sender, e.cfg.ChainID)
		if !senderIsVault {
			ok, cpi := e.pubkeyMgr.IsValidPoolAddress(txInItem.To, e.cfg.ChainID)
			if ok {
				expectedRouter := e.pubkeyMgr.GetContract(e.cfg.ChainID, cpi.PubKey)
				if expectedRouter != common.NoAddress {
					depositRouter := evm.DepositEventRouter(receipt.Logs)
					if depositRouter != nil && !strings.EqualFold(expectedRouter.String(), depositRouter.String()) {
						e.logger.Warn().
							Str("tx", txInItem.Tx).
							Str("vault", txInItem.To).
							Str("sender", txInItem.Sender).
							Str("expected_router", expectedRouter.String()).
							Str("deposit_router", depositRouter.String()).
							Msg("deposit targets vault via wrong router, ignoring")
						return nil, nil
					}
				}
			}
		}
	}

	// under no circumstance EVM gas price will be less than 1 Gwei, unless it is in dev environment
	txGasPrice := receipt.EffectiveGasPrice
	if txGasPrice == nil {
		txGasPrice = big.NewInt(0)
	}
	txInItem.Gas = common.MakeEVMGas(e.cfg.ChainID, txGasPrice, receipt.GasUsed, receipt.L1Fee)
	if txInItem.Coins.IsEmpty() {
		return nil, nil
	}
	return txInItem, nil
}

// getTxInFromFailedTransaction when a transaction failed due to out of gas, this method
// will check whether the transaction is an outbound. It fakes a txInItem if the failed
// transaction is an outbound, and reports it back to THORNode so the gas fee can be
// deducted from the vault balance to keep accounting in sync.
func (e *EVMScanner) getTxInFromFailedTransaction(tx *etypes.Transaction, receipt *etypes.Receipt) *stypes.TxInItem {
	if receipt.Status == 1 {
		e.logger.Info().Str("hash", tx.Hash().String()).Msg("success transaction should not get into getTxInFromFailedTransaction")
		return nil
	}
	fromAddr, err := e.eipSigner.Sender(tx)
	if err != nil {
		e.logger.Err(err).Msg("failed to get from address")
		return nil
	}
	ok, cif := e.pubkeyMgr.IsValidPoolAddress(fromAddr.String(), e.cfg.ChainID)
	if !ok || cif.IsEmpty() {
		return nil
	}
	txGasPrice := receipt.EffectiveGasPrice
	if txGasPrice == nil {
		txGasPrice = big.NewInt(0)
	}
	txHash := tx.Hash().Hex()[2:]
	return &stypes.TxInItem{
		Tx:     txHash,
		Memo:   memo.NewOutboundMemo(common.TxID(strings.ToUpper(txHash))).String(),
		Sender: strings.ToLower(fromAddr.String()),
		To:     strings.ToLower(tx.To().String()),
		Coins:  common.NewCoins(common.NewCoin(e.cfg.ChainID.GetGasAsset(), cosmos.NewUint(1))),
		Gas:    common.MakeEVMGas(e.cfg.ChainID, txGasPrice, receipt.GasUsed, receipt.L1Fee),
	}
}
