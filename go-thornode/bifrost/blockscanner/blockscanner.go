package blockscanner

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	btypes "gitlab.com/thorchain/thornode/v3/bifrost/blockscanner/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
)

// BlockScannerFetcher define the methods a block scanner need to implement
type BlockScannerFetcher interface {
	// FetchMemPool scan the mempool
	FetchMemPool(height int64) (types.TxIn, error)
	// FetchTxs scan block with the given height
	FetchTxs(fetchHeight, chainHeight int64) (types.TxIn, error)
	// GetHeight return current block height
	GetHeight() (int64, error)
	// GetNetworkFee returns current network fee details
	GetNetworkFee() (transactionSize, transactionFeeRate uint64)
}

type Block struct {
	Height int64
	Txs    []string
}

// BlockScanner is used to discover block height
type BlockScanner struct {
	cfg                   config.BifrostBlockScannerConfiguration
	logger                zerolog.Logger
	wg                    *sync.WaitGroup
	scanChan              chan int64
	stopChan              chan struct{}
	rollbackChan          chan int64
	scannerStorage        ScannerStorage
	metrics               *metrics.Metrics
	previousBlock         int64
	globalTxsQueue        chan types.TxIn
	globalNetworkFeeQueue chan common.NetworkFee
	errorCounter          *prometheus.CounterVec
	thorchainBridge       thorclient.ThorchainBridge
	chainScanner          BlockScannerFetcher
	healthy               *atomic.Bool
}

// NewBlockScanner create a new instance of BlockScanner
func NewBlockScanner(cfg config.BifrostBlockScannerConfiguration, scannerStorage ScannerStorage, m *metrics.Metrics, thorchainBridge thorclient.ThorchainBridge, chainScanner BlockScannerFetcher) (*BlockScanner, error) {
	var err error
	if scannerStorage == nil {
		return nil, errors.New("scannerStorage is nil")
	}
	if m == nil {
		return nil, errors.New("metrics instance is nil")
	}
	if thorchainBridge == nil {
		return nil, errors.New("thorchain bridge is nil")
	}

	logger := log.Logger.With().Str("module", "blockscanner").Str("chain", cfg.ChainID.String()).Logger()
	scanner := &BlockScanner{
		cfg:             cfg,
		logger:          logger,
		wg:              &sync.WaitGroup{},
		stopChan:        make(chan struct{}),
		scanChan:        make(chan int64),
		rollbackChan:    make(chan int64),
		scannerStorage:  scannerStorage,
		metrics:         m,
		errorCounter:    m.GetCounterVec(metrics.CommonBlockScannerError),
		thorchainBridge: thorchainBridge,
		chainScanner:    chainScanner,
		healthy:         &atomic.Bool{},
	}

	scanner.previousBlock, err = scanner.GetStartHeight()
	logger.Info().Int64("block height", scanner.previousBlock).Msg("block scanner last fetch height")
	return scanner, err
}

// IsHealthy return if the block scanner is healthy or not
func (b *BlockScanner) IsHealthy() bool {
	return b.healthy.Load()
}

func (b *BlockScanner) PreviousHeight() int64 {
	return atomic.LoadInt64(&b.previousBlock)
}

// RollbackToLastObserved rollback the block scanner to last observed height minus flex period
func (b *BlockScanner) RollbackToLastObserved() error {
	lastObservedHeight, err := b.thorchainBridge.GetLastObservedInHeight(b.cfg.ChainID)
	if err != nil {
		return fmt.Errorf("fail to get last observed height: %w", err)
	}
	if lastObservedHeight <= 0 {
		// no last observed height, no need to rollback
		return nil
	}

	maxConfirmations, err := b.thorchainBridge.GetMimirWithRef(constants.MimirTemplateMaxConfirmations, b.cfg.ChainID.String())
	if err != nil || maxConfirmations < 0 {
		maxConfirmations = 0
	}

	c, err := b.thorchainBridge.GetConstants()
	if err != nil {
		return fmt.Errorf("fail to get constants: %w", err)
	}

	obsDelayFlexConst := constants.ObservationDelayFlexibility.String()
	observerFlexWindowBlocksThor := c[obsDelayFlexConst]
	observerFlexWindowBlocksThorMimir, err := b.thorchainBridge.GetMimir(obsDelayFlexConst)
	if err == nil && observerFlexWindowBlocksThorMimir > 0 {
		observerFlexWindowBlocksThor = observerFlexWindowBlocksThorMimir
	}

	thorBlockTimeMs := c[constants.ThorchainBlockTime.String()] / int64(time.Millisecond)
	observerFlexWindowBlocksChain := observerFlexWindowBlocksThor * thorBlockTimeMs / b.cfg.ChainID.ApproximateBlockMilliseconds()
	if observerFlexWindowBlocksChain < 1 {
		observerFlexWindowBlocksChain = 1
	}

	rollbackHeight := lastObservedHeight - max(observerFlexWindowBlocksChain, maxConfirmations)

	if rollbackHeight < 0 {
		rollbackHeight = 0
	}

	b.rollbackChan <- rollbackHeight
	return nil
}

func (b *BlockScanner) rollback(height int64) error {
	if b.PreviousHeight() <= height {
		// height is already below rollback amount, proceed as normal
		return nil
	}
	if err := b.scannerStorage.SetScanPos(height); err != nil {
		return fmt.Errorf("fail to set scan pos: %w", err)
	}
	// set the previous block to height
	atomic.StoreInt64(&b.previousBlock, height)
	return nil
}

// GetMessages return the channel
func (b *BlockScanner) GetMessages() <-chan int64 {
	return b.scanChan
}

// Start block scanner
func (b *BlockScanner) Start(globalTxsQueue chan types.TxIn, globalNetworkFeeQueue chan common.NetworkFee) {
	b.globalTxsQueue = globalTxsQueue
	b.globalNetworkFeeQueue = globalNetworkFeeQueue
	// Only use persisted scan position if no explicit start height was configured.
	// When StartBlockHeight is set, it should always take precedence so operators
	// can force a rescan from a specific height.
	if b.cfg.StartBlockHeight <= 0 {
		currentPos, err := b.scannerStorage.GetScanPos()
		if err != nil {
			b.logger.Error().Err(err).Msgf("fail to get current block scan pos, %s will start from %d", b.cfg.ChainID, b.previousBlock)
		} else if currentPos > b.previousBlock {
			b.previousBlock = currentPos
		}
	}
	b.wg.Add(2)
	go b.scanBlocks()
	go b.scanMempool()
}

func (b *BlockScanner) scanMempool() {
	b.logger.Info().Msg("start to scan mempool")
	defer b.logger.Info().Msg("stop scan mempool")
	defer b.wg.Done()

	if !b.cfg.ScanMemPool {
		b.logger.Info().Msg("mempool scan is disabled")
		return
	}

	for {
		select {
		case <-b.stopChan:
			return
		default:
			// mempool scan will continue even the chain get halted , thus the network can still aware of outbound transaction
			// during chain halt
			preBlockHeight := atomic.LoadInt64(&b.previousBlock)
			currentBlock := preBlockHeight + 1
			txInMemPool, err := b.chainScanner.FetchMemPool(currentBlock)
			if err != nil {
				b.logger.Error().Err(err).Msg("fail to fetch MemPool")
			}
			if len(txInMemPool.TxArray) > 0 {
				select {
				case <-b.stopChan:
					return
				case b.globalTxsQueue <- txInMemPool:
				}
			} else {
				// backoff between mempool scans (some chain clients always return nothing)
				time.Sleep(constants.ThorchainBlockTime)
			}
		}
	}
}

// Checks current mimir settings to determine if the current chain is paused
// either globally or specifically
func IsChainPaused(cfg config.BifrostBlockScannerConfiguration, logger zerolog.Logger, bridge thorclient.ThorchainBridge) bool {
	thorHeight, err := bridge.GetBlockHeight()
	if err != nil {
		logger.Error().Err(err).Msg("fail to get THORChain block height")
	}

	// Check if chain has been halted via mimir
	haltHeight, err := bridge.GetMimir(fmt.Sprintf("Halt%sChain", cfg.ChainID))
	if err != nil {
		logger.Error().Err(err).Msgf("fail to get mimir setting %s", fmt.Sprintf("Halt%sChain", cfg.ChainID))
	}
	if haltHeight > 0 && thorHeight >= haltHeight {
		return true
	}

	// Check if chain has been halted by auto solvency checks
	solvencyHaltHeight, err := bridge.GetMimir(fmt.Sprintf("SolvencyHalt%sChain", cfg.ChainID))
	if err != nil {
		logger.Error().Err(err).Msgf("fail to get mimir %s", fmt.Sprintf("SolvencyHalt%sChain", cfg.ChainID))
	}
	if solvencyHaltHeight > 0 && thorHeight >= solvencyHaltHeight {
		return true
	}

	// Check if all chains halted globally
	globalHaltHeight, err := bridge.GetMimir("HaltChainGlobal")
	if err != nil {
		logger.Error().Err(err).Msg("fail to get mimir setting HaltChainGlobal")
	}
	if globalHaltHeight > 0 && thorHeight >= globalHaltHeight {
		return true
	}

	// Check if a node temporarily paused all chains
	nodePauseHeight, err := bridge.GetMimir("NodePauseChainGlobal")
	if err != nil {
		logger.Error().Err(err).Msg("fail to get mimir setting NodePauseChainGlobal")
	}

	return (nodePauseHeight > 0 && thorHeight <= nodePauseHeight)
}

// scanBlocks
func (b *BlockScanner) scanBlocks() {
	b.logger.Debug().Msg("start to scan blocks")
	defer b.logger.Debug().Msg("stop scan blocks")
	defer b.wg.Done()

	lastMimirCheck := time.Now().Add(-constants.ThorchainBlockTime)
	isChainPaused := false

	type fetchTxsResult struct {
		txIn types.TxIn
		err  error
	}
	prefetch := map[int64]chan fetchTxsResult{}

	// start up to grab those blocks
	for {
		select {
		case <-b.stopChan:
			return
		case amount := <-b.rollbackChan:
			if err := b.rollback(amount); err != nil {
				b.logger.Error().Err(err).Msg("fail to rollback block scanner")
				b.errorCounter.WithLabelValues(b.cfg.ChainID.String()).Inc()
				time.Sleep(b.cfg.BlockHeightDiscoverBackoff)
				continue
			}
		default:
			preBlockHeight := atomic.LoadInt64(&b.previousBlock)
			currentBlock := preBlockHeight + 1
			// check if mimir has disabled this chain
			if time.Since(lastMimirCheck) >= constants.ThorchainBlockTime {
				isChainPaused = IsChainPaused(b.cfg, b.logger, b.thorchainBridge)
				lastMimirCheck = time.Now()
			}

			// Chain is paused, mark as unhealthy
			if isChainPaused {
				b.healthy.Store(false)
				time.Sleep(constants.ThorchainBlockTime)
				continue
			}

			chainHeight, err := b.chainScanner.GetHeight()
			if err != nil {
				b.logger.Error().Err(err).Msg("fail to get chain block height")
				time.Sleep(b.cfg.BlockHeightDiscoverBackoff)
				continue
			}
			if chainHeight < currentBlock {
				time.Sleep(b.cfg.BlockHeightDiscoverBackoff)
				continue
			}

			// get the prefetched result or call FetchTxs for the block
			var txIn types.TxIn
			if resultChan, ok := prefetch[currentBlock]; ok {
				// already prefetched, just wait for the result
				result := <-resultChan
				delete(prefetch, currentBlock)
				txIn, err = result.txIn, result.err
			} else {
				txIn, err = b.chainScanner.FetchTxs(currentBlock, chainHeight)
			}

			if err != nil {
				// don't log an error if its because the block doesn't exist yet
				if !errors.Is(err, btypes.ErrUnavailableBlock) {
					b.logger.Error().Err(err).Int64("block height", currentBlock).Msg("fail to get RPCBlock")
					b.healthy.Store(false)
				}
				time.Sleep(b.cfg.BlockHeightDiscoverBackoff)
				continue
			}

			// start prefetching next blocks if configured
			if b.cfg.PrefetchBlocks > 1 {
				for i := int64(1); i < b.cfg.PrefetchBlocks; i++ {
					prefetchHeight := currentBlock + i
					if _, ok := prefetch[prefetchHeight]; !ok && prefetchHeight <= chainHeight {
						resultChan := make(chan fetchTxsResult, 1)
						prefetch[prefetchHeight] = resultChan
						go func(height int64, resultChan chan fetchTxsResult) {
							fetchTxIn, fetchErr := b.chainScanner.FetchTxs(height, chainHeight)
							resultChan <- fetchTxsResult{txIn: fetchTxIn, err: fetchErr}
						}(prefetchHeight, resultChan)
					}
				}
			}

			ms := b.cfg.ChainID.ApproximateBlockMilliseconds()

			// determine how often we compare THORNode network fee to Bifrost network fee.
			// General goal is about once per hour.
			mod := ((60 * 60 * 1000) + ms - 1) / ms
			if currentBlock%mod == 0 {
				b.updateStaleNetworkFee(currentBlock)
			}

			// determine how often we print a info log line for scanner
			// progress. General goal is about once per minute
			mod = (60_000 + ms - 1) / ms
			// enable this one , so we could see how far it is behind
			if currentBlock%mod == 0 || !b.healthy.Load() {
				b.logger.Info().
					Int64("block height", currentBlock).
					Int("txs", len(txIn.TxArray)).
					Int64("gap", chainHeight-currentBlock).
					Bool("healthy", b.healthy.Load()).
					Msg("scan block")
			}
			atomic.AddInt64(&b.previousBlock, 1)

			// consider 3 blocks or the configured lag time behind tip as healthy
			lagDuration := time.Duration((chainHeight-currentBlock)*ms) * time.Millisecond
			if chainHeight-currentBlock <= 3 || lagDuration < b.cfg.MaxHealthyLag {
				b.healthy.Store(true)
			} else {
				b.healthy.Store(false)
			}
			b.logger.Debug().Msgf("the gap is %d , healthy: %+v", chainHeight-currentBlock, b.healthy.Load())

			b.metrics.GetCounter(metrics.TotalBlockScanned).Inc()
			if len(txIn.TxArray) > 0 {
				select {
				case <-b.stopChan:
					return
				case b.globalTxsQueue <- txIn:
				}
			}
			if err = b.scannerStorage.SetScanPos(b.previousBlock); err != nil {
				b.logger.Error().Err(err).Msg("fail to save block scan pos")
				// alert!!
				continue
			}
		}
	}
}

// updateStaleNetworkFee broadcasts a network fee observation if the local scanner fee
// does not match the fee published to THORNode. This can be called periodically to
// ensure fee changes find consensus despite raciness on the observation height.
func (b *BlockScanner) updateStaleNetworkFee(currentBlock int64) {
	// Only broadcast MsgNetworkFee if the chain isn't THORChain
	// and the scanner is healthy.
	if b.cfg.ChainID.Equals(common.THORChain) || !b.healthy.Load() {
		return
	}

	transactionSize, transactionFeeRate := b.chainScanner.GetNetworkFee()
	thorTransactionSize, thorTransactionFeeRate, err := b.thorchainBridge.GetNetworkFee(b.cfg.ChainID)
	if err != nil {
		b.logger.Error().Err(err).Msg("fail to get thornode network fee")
		return
	}
	// Do not broadcast a regularly-timed network fee if the THORNode network fee is already consistent with the scanner's.
	if thorTransactionSize == transactionSize && thorTransactionFeeRate == transactionFeeRate {
		b.logger.Info().
			Int64("height", currentBlock).
			Uint64("size", transactionSize).
			Uint64("rate", transactionFeeRate).
			Msg("thornode network fee is consistent with scanner, no need to broadcast")
		return
	}

	b.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           b.cfg.ChainID,
		Height:          currentBlock,
		TransactionSize: transactionSize,
		TransactionRate: transactionFeeRate,
	}

	b.logger.Info().
		Int64("height", currentBlock).
		Uint64("size", transactionSize).
		Uint64("rate", transactionFeeRate).
		Msg("sent timed network fee to THORChain")
}

// GetStartHeight determines the height to start scanning:
//  1. Use the config start height if set.
//  2. If last consensus inbound height (lastblock) is available:
//     a) Use local scanner storage height if available, up to the max lag from lastblock.
//     b) Otherwise, use lastblock.
//  3. Otherwise, use local scanner storage height if available.
//  4. Otherwise, use the last height from the chain itself.
func (b *BlockScanner) GetStartHeight() (int64, error) {
	// get scanner storage height
	currentPos, _ := b.scannerStorage.GetScanPos() // ignore error

	clog := b.logger.With().Stringer("chain", b.cfg.ChainID).Logger()

	// 1. Use the config start height if set.
	if b.cfg.StartBlockHeight > 0 {
		clog.Info().
			Int64("start_height", b.cfg.StartBlockHeight).
			Msg("using configured start block height")
		return b.cfg.StartBlockHeight, nil
	}

	// wait for thorchain to be caught up first
	if err := b.thorchainBridge.WaitToCatchUp(); err != nil {
		clog.Info().Err(err).Msg("waiting for thorchain to catch up")
		return 0, err
	}

	if b.thorchainBridge != nil && b.cfg.ChainID != common.THORChain {
		height, _ := b.thorchainBridge.GetLastObservedInHeight(b.cfg.ChainID)
		if height > 0 {

			// 2.a) Use local scanner storage height if available, up to the max lag from lastblock.
			if currentPos > 0 {
				// calculate the max lag
				maxLagBlocks := b.cfg.MaxResumeBlockLag.Milliseconds() / b.cfg.ChainID.ApproximateBlockMilliseconds()

				// return the position up to the max block lag behind the consensus height
				if height <= currentPos+maxLagBlocks {
					clog.Info().
						Int64("start_height", currentPos).
						Int64("last_observed_height", height).
						Msg("using local scanner storage height")

					return currentPos, nil
				} else {
					clog.Info().
						Int64("start_height", height-maxLagBlocks).
						Int64("last_observed_height", height).
						Msg("using last observed height with max lag")
					return height - maxLagBlocks, nil
				}
			}

			// 2.b) Otherwise, use lastblock.
			clog.Info().
				Int64("start_height", height).
				Msg("using last observed height")
			return height, nil
		}
	}

	//  3. Otherwise, use local scanner storage height if available.
	if currentPos > 0 {
		clog.Info().
			Int64("start_height", currentPos).
			Msg("using local scanner storage height")
		return currentPos, nil
	}

	//  4. Otherwise, use the last height from the chain itself.
	height, err := b.chainScanner.GetHeight()
	if err != nil {
		clog.Error().Err(err).Msg("failed to get chain height")
		return 0, err
	}
	clog.Info().
		Int64("start_height", height).
		Msg("using chain height as start height")
	return b.chainScanner.GetHeight()
}

func (b *BlockScanner) Stop() {
	b.logger.Debug().Msg("receive stop request")
	defer b.logger.Debug().Msg("common block scanner stopped")
	close(b.stopChan)
	b.wg.Wait()
}
