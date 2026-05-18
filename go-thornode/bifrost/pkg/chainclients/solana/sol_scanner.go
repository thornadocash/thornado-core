package solana

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mr-tron/base58"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/syndtr/goleveldb/leveldb"
	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	shtypes "gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/solana/rpc"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/solana/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	sdk "gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"golang.org/x/sync/errgroup"
)

const (
	baseFeePerSigLamports = 5000 // "Base" fee per signature of a transaction

	transactionQueryLimit = 1000 // maximum number of transactions to query in a single request

	maxConcurrentTxQueries = 5 // maximum number of concurrent tx queries to the RPC node
)

////////////////////////////////////////////////////////////////////////////////////////
// SOLScanner
////////////////////////////////////////////////////////////////////////////////////////

type SOLScanner struct {
	cfg                config.BifrostBlockScannerConfiguration
	logger             zerolog.Logger
	db                 blockscanner.ScannerStorageSolana
	m                  *metrics.Metrics
	errCounter         *prometheus.CounterVec
	solRpc             *rpc.SolRPC
	pubKeyMgr          pubkeymanager.PubKeyValidator
	bridge             thorclient.ThorchainBridge
	solvencyReporter   shtypes.SolvencyReporter
	signerCacheManager *signercache.CacheManager

	globalNetworkFeeQueue chan common.NetworkFee
	globalTxsQueue        chan stypes.TxIn

	// fee rate
	feeRateCache []uint64
	lastFeeRate  uint64

	healthy *atomic.Bool

	// recent block hash (needed for tx construction / signing)
	// needs to be in consensus between validators
	recentBlockHash   string
	recentBlockHashMu sync.Mutex

	// network fee state
	netFeeSlotMultiple uint64
	netFeeInit         bool

	lastLog time.Time
	// counter for observed txs since last log
	txCount int

	stopChan chan struct{}

	// state for if we've successfully processed the initial height
	initialHeight     map[string]struct{}
	lastHeight        uint64
	lastHeightMu      sync.RWMutex
	vaultScanStatuses map[string]vaultScanStatus

	isChainPaused  bool
	lastMimirCheck time.Time
}

type vaultScanStatus struct {
	lastSig  string
	lastSlot uint64
}

// NewSOLScanner create a new instance of SOLScanner.
func NewSOLScanner(
	stopChan chan struct{},
	cfg config.BifrostBlockScannerConfiguration,
	storage blockscanner.ScannerStorageSolana,
	bridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
	rpc *rpc.SolRPC,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	solvencyReporter shtypes.SolvencyReporter,
	signerCacheManager *signercache.CacheManager,
) (*SOLScanner, error) {
	// check required arguments
	if storage == nil {
		return nil, errors.New("storage is nil")
	}
	if m == nil {
		return nil, errors.New("metrics manager is nil")
	}
	if pubkeyMgr == nil {
		return nil, errors.New("pubkey manager is nil")
	}

	return &SOLScanner{
		cfg:                cfg,
		logger:             log.Logger.With().Stringer("chain", cfg.ChainID).Logger(),
		errCounter:         m.GetCounterVec(metrics.BlockScanError(cfg.ChainID)),
		db:                 storage,
		m:                  m,
		solRpc:             rpc,
		pubKeyMgr:          pubkeyMgr,
		bridge:             bridge,
		solvencyReporter:   solvencyReporter,
		signerCacheManager: signerCacheManager,
		recentBlockHash:    "",
		lastFeeRate:        0,
		feeRateCache:       make([]uint64, 0),
		stopChan:           stopChan,
		healthy:            &atomic.Bool{},
		initialHeight:      make(map[string]struct{}),
		vaultScanStatuses:  map[string]vaultScanStatus{},
	}, nil
}

func (s *SOLScanner) Start() {
	s.lastMimirCheck = time.Now().Add(-constants.ThorchainBlockTime)
	s.isChainPaused = false

	initialSlot, err := s.solRpc.GetSlot()
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to get initial slot")
		return
	}

	lastHeight, err := s.db.GetScanPos()
	// if err is ErrNotFound, then we set to 0.
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		s.logger.Error().Err(err).Msg("failed to get last scan pos")
		return
	}
	s.lastHeightMu.Lock()
	s.lastHeight = lastHeight
	s.lastHeightMu.Unlock()

	// initialize the network fee slot multiple to the closest multiple of NetFeeUpdateInterval beneath the initial slot
	s.netFeeSlotMultiple = initialSlot / s.cfg.Solana.FeeUpdateInterval

	// attempt to initialize the network fee. In simtest environment, we will not have enough tx history
	// so the initial netfee will be sent upon first tx(s) observation
	if err := s.checkRecentBlocksAndUpdateNetworkFee(s.netFeeSlotMultiple * s.cfg.Solana.FeeUpdateInterval); err != nil {
		s.logger.Error().Err(err).Msg("failed to check recent blocks and update network fee")
	}

	for {
		select {
		case <-s.stopChan:
			s.logger.Info().Msg("stopping SOL scanner")
			return
		default:
			s.scan()
		}
	}
}

// scan is the main scanning loop for the SOLScanner. It will check if the chain is paused, get the current slot,
// and then scan for new transactions in the vaults. It will also update the network fee if necessary.min
func (s *SOLScanner) scan() {
	if time.Since(s.lastMimirCheck) >= constants.ThorchainBlockTime {
		s.isChainPaused = blockscanner.IsChainPaused(s.cfg, s.logger, s.bridge)
		s.lastMimirCheck = time.Now()
	}

	if s.isChainPaused {
		s.healthy.Store(false)
		time.Sleep(constants.ThorchainBlockTime)
		return
	}

	var vaultAddrs []string
	for _, vault := range s.pubKeyMgr.GetAlgoPubKeys(common.SigningAlgoEd25519, s.cfg.Solana.ScanInactiveVaults) {
		vaultAddrSol, err := vault.GetAddress(common.SOLChain)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to get vault address")
			continue
		}
		vaultAddrs = append(vaultAddrs, vaultAddrSol.String())
	}

	currentSlot, err := s.solRpc.GetSlot()
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to get current slot")
		time.Sleep(time.Second)
		return
	}

	if currentSlot > (s.netFeeSlotMultiple+1)*s.cfg.Solana.FeeUpdateInterval {
		var thorRate uint64
		_, thorRate, err = s.bridge.GetNetworkFee(common.SOLChain)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to get network fee")
		} else {
			s.lastFeeRate = thorRate
		}

		// we crossed the threshold for updating the network fee, so check the last N blocks
		//  up to the rounded slot for deterministic behavior across nodes.
		s.netFeeSlotMultiple = currentSlot / s.cfg.Solana.FeeUpdateInterval
		if err = s.checkRecentBlocksAndUpdateNetworkFee(s.netFeeSlotMultiple * s.cfg.Solana.FeeUpdateInterval); err != nil {
			s.logger.Error().Err(err).Msg("failed to check recent blocks and update network fee")
		}
	}

	s.lastHeightMu.RLock()
	lastSlot := s.lastHeight
	s.lastHeightMu.RUnlock()

	// skip if rpc slot is behind last scanned slot
	if currentSlot <= lastSlot {
		time.Sleep(s.cfg.Solana.ScanInterval)
		return
	}

	// coalesce scan to configured scan interval
	solanaBlockTime := time.Duration(common.SOLChain.ApproximateBlockMilliseconds()) * time.Millisecond
	lag := time.Duration(currentSlot-lastSlot) * solanaBlockTime
	if lag < s.cfg.Solana.ScanInterval {
		time.Sleep(s.cfg.Solana.ScanInterval)
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(vaultAddrs))

	healthy := true
	var mu sync.Mutex
	setUnhealthy := func() {
		mu.Lock()
		defer mu.Unlock()
		healthy = false
	}

	newLastSigs := make([]string, len(vaultAddrs))
	successfulQueries := make([]bool, len(vaultAddrs))
	countTxs := make([]int, len(vaultAddrs))
	vaultLastSigs := make([]string, len(vaultAddrs))
	for i, vaultAddr := range vaultAddrs {
		var lastSig string
		var vaultLastSlot uint64
		if vss, ok := s.vaultScanStatuses[vaultAddr]; ok {
			lastSig = vss.lastSig
			vaultLastSlot = vss.lastSlot
		} else {
			lastSig, vaultLastSlot, err = s.db.GetScanStatus(vaultAddr)
			// if err is ErrNotFound, then we set to "" and 0.
			if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
				s.logger.Error().Err(err).Str("vault", vaultAddr).Msg("failed to get vault scan status from db")
				setUnhealthy()
				wg.Done()
				continue
			}
			s.vaultScanStatuses[vaultAddr] = vaultScanStatus{
				lastSig:  lastSig,
				lastSlot: vaultLastSlot,
			}
		}
		vaultLastSigs[i] = lastSig

		var minContextSlot uint64
		var queryLastSig string
		if _, ok := s.initialHeight[vaultAddr]; ok || s.cfg.StartBlockHeight == 0 {
			minContextSlot = vaultLastSlot
			queryLastSig = lastSig
		} else {
			minContextSlot = uint64(s.cfg.StartBlockHeight)
			s.logger.Info().
				Str("vault", vaultAddr).
				Uint64("minContextSlot", minContextSlot).
				Msg("Initial height is set, querying all rpc node tx history")
			queryLastSig = ""
			// NOTE: this can observe txs that we already observed in the past,
			// because minContextSlot does not set a minimum bound for the query,
			// it only ensures that minContextSlot is in the history on the rpc node.
		}

		go s.scanVault(vaultAddr, i, queryLastSig, minContextSlot, setUnhealthy, countTxs, successfulQueries, newLastSigs, &wg)
	}
	wg.Wait()

	s.healthy.Store(healthy)

	var numTxs int

	for i, vaultAddr := range vaultAddrs {
		txCount := countTxs[i]

		// increment tx counts for this scan iteration and also since last log.
		numTxs += txCount
		s.txCount += txCount

		if successfulQueries[i] {
			// we are up to date as of the current slot, so minContextSlot
			// can be the current slot for the next call

			// since tx query was successful, we want to update last scanned slot for this vault. We may or may not have queried new txs.
			// If no txs were queried, we want to retain the last sig from the db.
			var lastSig string
			if txCount == 0 {
				lastSig = vaultLastSigs[i]
			} else {
				lastSig = newLastSigs[i]
			}

			// we have not scanned this vault before, so we mark it as such so that
			// the cfg.StartBlockHeight is only used for the first successful query.
			if _, ok := s.initialHeight[vaultAddr]; !ok {
				s.initialHeight[vaultAddr] = struct{}{}
			}

			s.vaultScanStatuses[vaultAddr] = vaultScanStatus{
				lastSig:  lastSig,
				lastSlot: currentSlot,
			}

			// update the db in a goroutine, we don't need to block here
			go func() {
				if err = s.db.SetScanStatus(vaultAddr, lastSig, currentSlot); err != nil {
					s.logger.Error().Err(err).Str("vault", vaultAddr).Msg("failed to set vault scan status")
				}
			}()
		}
	}

	if !s.netFeeInit && numTxs > 0 {
		// this should really only happen (and is only necessary) in test environments to initialize the network fee,
		// because we expect to have enough observed txs in the checkRecentBlocksAndUpdateNetworkFee above.
		s.sendNetworkFeeIfNecessary(currentSlot)
	}
	if err = s.solvencyReporter(int64(currentSlot)); err != nil {
		s.logger.Err(err).Msg("fail to send solvency to THORChain")
	}

	if (time.Since(s.lastLog) > (5 * time.Second)) || !healthy {
		s.lastLog = time.Now()
		s.logger.Info().
			Uint64("block height", currentSlot).
			Int("gap", 0).
			Int("txs", s.txCount).
			Bool("healthy", healthy).
			Msg("scan block")
		s.txCount = 0
	}

	if lastSlot > 0 {
		s.m.GetCounter(metrics.TotalBlockScanned).Add(float64(currentSlot - lastSlot))
	}

	s.lastHeightMu.Lock()
	s.lastHeight = currentSlot
	s.lastHeightMu.Unlock()

	// update the db in a goroutine, we don't need to block here
	go func() {
		if err := s.db.SetScanPos(currentSlot); err != nil {
			s.logger.Error().Err(err).Msg("failed to set scan position")
		}
	}()
}

// scanVault scans a vault for new transactions. It will query the RPC node for the vault's
// address and get any transactions involving the address, starting from most recent until queryLastSig.
// If queryLastSig is empty, it will query all transactions for the vault address that exist on the node.
// minContextSlot is the minimum slot to query for transactions. It does not set a bound on the query,
// but does ensure that history exists on the RPC node until at least this slot.
func (s *SOLScanner) scanVault(
	vaultAddr string,
	i int,
	queryLastSig string,
	minContextSlot uint64,
	setUnhealthy func(),
	txCounts []int,
	successfulQueries []bool,
	newLastSigs []string,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	var totalTxs int
	var newestSig string
	before := ""
	for {
		res, err := s.solRpc.GetSignaturesForAddressUntil(vaultAddr, rpc.GetSignaturesForAddressUntilParams{
			Commitment:     "finalized",
			MinContextSlot: minContextSlot,
			Before:         before,
			Until:          queryLastSig,
			Limit:          transactionQueryLimit,
		})
		if err != nil {
			s.logger.Error().Err(err).Msgf("failed to get signatures for address %s", vaultAddr)
			setUnhealthy()
			return
		}

		if len(res) == 0 {
			if totalTxs == 0 {
				s.logger.Debug().Msgf("no new signatures for address %s", vaultAddr)
			}
			break
		}

		totalTxs += len(res)
		if newestSig == "" {
			newestSig = res[0].Signature
		}

		var eg errgroup.Group
		// limit the number of concurrent tx queries to the RPC node
		eg.SetLimit(maxConcurrentTxQueries)
		for _, sig := range res {
			eg.Go(func() error {
				tx, err := s.solRpc.GetTransaction(sig.Signature)
				if err != nil {
					setUnhealthy()
					return fmt.Errorf("failed to get transaction %s: %w", sig.Signature, err)
				}

				if tx.Meta.Err != nil {
					s.logger.Debug().Msgf("transaction %s failed: %s", sig.Signature, tx.Meta.Err)
					return nil
				}

				s.globalTxsQueue <- s.getTxIn(sig.Slot, []*rpc.TransactionResult{tx})
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			setUnhealthy()
			s.logger.Error().Err(err).Msgf("failed to process transactions")
			return
		}

		if len(res) < transactionQueryLimit {
			break
		}
		before = res[len(res)-1].Signature
	}

	successfulQueries[i] = true
	txCounts[i] = totalTxs
	if totalTxs > 0 {
		newLastSigs[i] = newestSig
	}
}

// Since we don't scan all txs in every block, in order to get a good perspective of the network fee
// we need to check the last N blocks and calculate the median fee rate.
func (s *SOLScanner) checkRecentBlocksAndUpdateNetworkFee(slot uint64) error {
	defer s.sendNetworkFeeIfNecessary(slot)

	numTxs := 0
	var lastSlot uint64

	var firstSlot uint64
	if slot > s.cfg.Solana.FeeSampleSlots {
		firstSlot = slot - s.cfg.Solana.FeeSampleSlots
	}

	blocks, err := s.solRpc.GetBlockHeights(firstSlot, slot)
	if err != nil {
		return fmt.Errorf("failed to get confirmed blocks: %w", err)
	}

	// reverse the blocks to get the most recent first
	slices.Reverse(blocks)

	setBlockHash := false
	for _, b := range blocks {
		block, err := s.solRpc.GetBlock(b)
		if err != nil {
			if strings.Contains(err.Error(), "Block status not yet available for slot") {
				// This can happen if the RPC is overwhelmed. give up for now.
				return fmt.Errorf("block status not yet available for slot %d", b)
			}
			s.logger.Error().Err(err).Msg("failed to get block to determine netfee")
			continue
		}
		if block == nil {
			s.logger.Debug().Msgf("block %d is nil", b)
			continue
		}
		if !setBlockHash {
			s.setRecentBlockHash(block.Blockhash)
			setBlockHash = true
		}
		if lastSlot == 0 {
			lastSlot = b
		}

		numTxsInBlock := s.calculateFeeRatesFromTransactions(block.Transactions)
		numTxs += numTxsInBlock

		if numTxs > s.cfg.GasCacheBlocks {
			// We have enough transactions to calculate the network fee
			s.logger.Info().Uint64("first_slot", b).Uint64("last_slot", lastSlot).Msg("found enough transactions to calculate network fee")
			break
		}
	}

	if numTxs < s.cfg.GasCacheBlocks {
		return fmt.Errorf("not enough transactions")
	}

	return nil
}

// GetHeight returns the current block height of Solana
func (s *SOLScanner) GetHeight() (int64, error) {
	height, err := s.solRpc.GetSlot()
	if err != nil {
		return -1, err
	}
	return int64(height), nil
}

// ScanHeight returns the current scanned height
func (s *SOLScanner) ScanHeight() (int64, error) {
	s.lastHeightMu.RLock()
	lastHeight := s.lastHeight
	s.lastHeightMu.RUnlock()

	if lastHeight > 0 {
		return int64(lastHeight), nil
	}

	// lastHeight not in memory, get from db

	lastSlot, err := s.db.GetScanPos()
	// if err is ErrNotFound, then we set to 0.
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return -1, err
	}
	return int64(lastSlot), nil
}

// ScanHeight returns the current scanned height
func (s *SOLScanner) IsHealthy() bool {
	return s.healthy.Load()
}

// GetrecentBlockHash returns the latest block hash
func (s *SOLScanner) getRecentBlockHash() string {
	s.recentBlockHashMu.Lock()
	defer s.recentBlockHashMu.Unlock()
	return s.recentBlockHash
}

// UpdaterecentBlockHash updates the local latest block hash to be used in tx signing
func (s *SOLScanner) setRecentBlockHash(hash string) {
	s.recentBlockHashMu.Lock()
	defer s.recentBlockHashMu.Unlock()
	s.recentBlockHash = hash
}

// GetNetworkFee returns current chain network fee according to Bifrost.
func (s *SOLScanner) GetNetworkFee() (transactionSize, transactionFeeRate uint64) {
	return 1, s.lastFeeRate
}

func (s *SOLScanner) sendNetworkFeeIfNecessary(height uint64) {
	if len(s.feeRateCache) == 0 {
		// Not enough txs yet
		return
	}

	if len(s.feeRateCache) < s.cfg.GasCacheBlocks && s.netFeeInit {
		// We are beyond the initial fee post, but not enough txs yet
		return
	}

	// Calculate the median fee rate from the gas cache

	medianFeeRate := calculateMedian(s.feeRateCache)

	// round medianFeeRate to the nearest resolution
	resolution := uint64(s.cfg.GasPriceResolution)
	roundedFeeRate := ((medianFeeRate + (resolution / 2)) / resolution) * resolution

	// truncate gas prices older than our max cached transactions
	if len(s.feeRateCache) > s.cfg.GasCacheBlocks {
		s.feeRateCache = s.feeRateCache[(len(s.feeRateCache) - s.cfg.GasCacheBlocks):]
	}

	if roundedFeeRate == 0 {
		s.logger.Debug().Msg("network fee is 0")
		return
	}

	s.netFeeInit = true

	// Update the fee rate if it has changed
	if roundedFeeRate == s.lastFeeRate {
		s.logger.Info().Uint64("fee", roundedFeeRate).Msg("network fee is already up to date")
		return
	}

	s.lastFeeRate = roundedFeeRate

	s.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           common.SOLChain,
		Height:          int64(height),
		TransactionSize: 1,
		TransactionRate: roundedFeeRate,
	}

	s.logger.Info().
		Uint64("fee", roundedFeeRate).
		Uint64("height", height).
		Msg("sent network fee to THORChain")
}

// --------------------------------- extraction ---------------------------------

// Gets the TxIn struct for a block
func (s *SOLScanner) getTxIn(slot uint64, transactions []*rpc.TransactionResult) stypes.TxIn {
	txIn := stypes.TxIn{
		Chain:                s.cfg.ChainID,
		TxArray:              make([]*stypes.TxInItem, 0),
		MemPool:              false,
		ConfirmationRequired: 1, // Assuming 1 confirmation is required
	}

	for _, txn := range transactions {
		// Skip transactions with no signatures or no fee
		if len(txn.Transaction.Signatures) == 0 || txn.Meta.Fee == 0 {
			continue
		}

		txItem := s.getTxInItem(txn, int64(slot))
		if txItem == nil {
			continue
		}

		txIn.TxArray = append(txIn.TxArray, txItem)
	}

	return txIn
}

// Calculates the median of a slice of uint64.
// Makes a copy to avoid mutating the original slice.
func calculateMedian(arr []uint64) uint64 {
	sorted := make([]uint64, len(arr))
	copy(sorted, arr)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	arr = sorted
	n := len(arr)
	if n%2 == 0 {
		return (arr[n/2-1] + arr[n/2]) / 2
	}
	return arr[n/2]
}

// calculateFeeRatesFromTransactions extracts fee rates from transactions for network fee calculation.
// Unlike getTxIn, this does not validate transactions or filter for THORChain inbounds.
// It processes ALL transactions in the block to get an accurate network fee measurement.
func (s *SOLScanner) calculateFeeRatesFromTransactions(transactions []*rpc.TransactionResult) int {
	txFeeRates := []uint64{}

	for _, txn := range transactions {
		// skip transactions with no signatures or no fee
		if len(txn.Transaction.Signatures) == 0 || txn.Meta.Fee == 0 {
			continue
		}

		// skip failed transactions
		if txn.Meta.Err != nil {
			continue
		}

		// calculate the fee rate (lamports per signature)
		numSigs := uint64(len(txn.Transaction.Signatures))
		feeRate := txn.Meta.Fee / numSigs

		// rate may underestimate in cases like multi-sig transactions, floor to base fee
		if feeRate < baseFeePerSigLamports {
			feeRate = baseFeePerSigLamports
		}

		txFeeRates = append(txFeeRates, feeRate)
	}

	// update the fee rate cache
	switch {
	case len(txFeeRates) == 1:
		s.feeRateCache = append(s.feeRateCache, txFeeRates[0])
	case len(txFeeRates) > 1:
		medianFeeRate := calculateMedian(txFeeRates)
		s.feeRateCache = append(s.feeRateCache, medianFeeRate)
	case len(txFeeRates) == 0:
		// if no transactions in the block, just add the base fee
		s.feeRateCache = append(s.feeRateCache, baseFeePerSigLamports)
	}

	return len(txFeeRates)
}

// Create a TxInItem for a SOL transfer transaction. This function will only return a
// TxInItem if the transaction contains exactly one SOL transfer instruction to the vault.
// Otherwise, it will return nil.
func (s *SOLScanner) getTxInItem(txn *rpc.TransactionResult, blockHeight int64) *stypes.TxInItem {
	memo := ""
	var transferCount int
	var fromAccount, toAccount string
	var amountMoved uint64
	fee := txn.Meta.Fee

	instructions := txn.Transaction.GetInstructions()

	for _, instruction := range instructions {
		programID := txn.Transaction.GetAccountAtIndex(instruction.ProgramIdIndex)

		// Handle memo
		if types.MemoProgramID.EqualsString(programID) {
			memoBytes, err := base58.Decode(instruction.Data)
			if err == nil {
				memo = string(memoBytes)
			} else {
				s.logger.Error().Err(err).Str("memoData", instruction.Data).Msg("Failed to decode memo")
			}
			continue
		}

		// Skip non-SystemProgram instructions
		if !types.SystemProgramID.EqualsString(programID) {
			continue
		}

		// Decode instruction data
		data, err := base58.Decode(instruction.Data)
		if err != nil {
			s.logger.Error().Err(err).Str("data", instruction.Data).Msg("Failed to decode instruction data")
			continue
		}

		// Check for transfer instruction
		if len(data) >= 1 && data[0] == 2 { // `2` indicates a System Program transfer instruction
			transferCount++
			if transferCount > 1 {
				s.logger.Warn().
					Int("transferCount", transferCount).
					Str("txID", txn.Transaction.Signatures[0]).
					Msg("Rejected transaction with multiple transfer instructions")
				return nil
			}

			if len(instruction.Accounts) < 2 {
				s.logger.Debug().Msg("Instruction does not contain enough accounts for transfer")
				return nil
			}

			fromIdx := instruction.Accounts[0]
			toIdx := instruction.Accounts[1]
			// TODO: V0 transactions using Address Lookup Tables (ALTs) can have account
			// indices that reference addresses in Meta.LoadedAddresses rather than
			// Message.AccountKeys. These transactions are silently dropped here. To
			// support them, parse LoadedAddresses and extend the account resolution.
			if fromIdx >= len(txn.Transaction.Message.AccountKeys) || toIdx >= len(txn.Transaction.Message.AccountKeys) {
				s.logger.Debug().Msg("Account index out of range")
				return nil
			}
			fromAccount = txn.Transaction.Message.AccountKeys[fromIdx]
			toAccount = txn.Transaction.Message.AccountKeys[toIdx]

			if len(data) < 12 {
				s.logger.Debug().Msg("Transfer instruction data too short")
				return nil
			}
			amountMoved = binary.LittleEndian.Uint64(data[4:12])
		}
	}

	// Skip if no valid transfer was found or amount is zero
	if transferCount == 0 || amountMoved == 0 {
		s.logger.Debug().
			Str("txID", txn.Transaction.Signatures[0]).
			Msg("No valid transfer found or amount is zero")
		return nil
	}

	solAmount := convertLamportsToTHORChain(amountMoved)
	if solAmount.LT(s.cfg.ChainID.DustThreshold()) {
		s.logger.Debug().
			Str("txID", txn.Transaction.Signatures[0]).
			Str("amount", solAmount.String()).
			Str("dustThreshold", s.cfg.ChainID.DustThreshold().String()).
			Msg("Transfer amount below dust threshold")
		return nil
	}

	solCoin := common.NewCoin(common.SOLAsset, solAmount)
	gasCoin := common.NewCoin(common.SOLAsset, convertLamportsToTHORChain(fee))
	solCoin.Decimals = 9
	gasCoin.Decimals = 9

	return &stypes.TxInItem{
		BlockHeight: blockHeight,
		Tx:          txn.Transaction.Signatures[0],
		Memo:        memo,
		Sender:      fromAccount,
		To:          toAccount,
		Coins:       []common.Coin{solCoin},
		Gas:         common.Gas{gasCoin},
	}
}

// --------------------------------- helpers ---------------------------------

// Function to convert lamports to sdk.Uint
func convertLamportsToTHORChain(lamports uint64) sdk.Uint {
	lamportsBig := new(big.Int).SetUint64(lamports)
	return sdk.NewUintFromBigInt(lamportsBig.Quo(lamportsBig, big.NewInt(10)))
}
