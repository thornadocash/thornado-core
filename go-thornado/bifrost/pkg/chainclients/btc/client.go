package btc

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"sync"
	stdatomic "sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/btcutil"
	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"

	"github.com/thornadocash/go-thornado/bifrost/blockscanner"
	btypes "github.com/thornadocash/go-thornado/bifrost/blockscanner/types"
	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	p2pstorage "github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc/rpc"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/shared/runners"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/shared/signercache"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
)

////////////////////////////////////////////////////////////////////////////////////////
// Generate
////////////////////////////////////////////////////////////////////////////////////////

////////////////////////////////////////////////////////////////////////////////////////
// Client - Base
////////////////////////////////////////////////////////////////////////////////////////

// Client defines a generic UTXO client. Since there are differences in addresses, RPCs,
// and txscript between chains, chain additions should audit switches on chain type and
// extend where appropriate.
const btcErrataDelayBlocks int64 = 6

type Client struct {
	cfg config.BifrostChainConfiguration
	log zerolog.Logger
	m   *metrics.Metrics
	rpc *rpc.Client

	// ---------- signing ----------
	nodePubKey         common.PubKey
	nodePrivKey        *btcec.PrivateKey
	frostKeySigner     frost.ThornadoKeyManager
	signerCacheManager *signercache.CacheManager

	// ---------- sync ----------
	wg                 *sync.WaitGroup
	signerLock         *sync.Mutex
	vaultLocks         map[string]*sync.Mutex
	vaultPathLock      sync.RWMutex
	vaultPaths         map[string]map[uint64]struct{}
	vaultAddrCache     sync.Map
	vaultPathAddrs     sync.Map
	sourceScriptCache  sync.Map
	taprootPubKeyCache sync.Map
	networkFeeLock     sync.Mutex
	solvencyLock       sync.Mutex

	// ---------- scanner ----------
	blockScanner    *blockscanner.BlockScanner
	temporalStorage *TemporalStorage

	// ---------- control ----------
	globalErrataQueue      chan<- types.ErrataBlock
	globalSolvencyQueue    chan<- types.Solvency
	globalNetworkFeeQueue  chan<- common.NetworkFee
	stopchan               chan struct{}
	currentBlockHeight     stdatomic.Int64
	missingErrataFirstSeen sync.Map

	// ---------- thornado state ----------
	bridge    thornadoclient.ThornadoBridge
	baseCache stdatomic.Pointer[BaseCache]

	// ---------- fees / solvency ----------
	minRelayFeeSats         stdatomic.Uint64
	lastFeeRate             stdatomic.Uint64
	feeRateCache            []uint64
	lastSolvencyCheckHeight stdatomic.Int64

	// ---------- testing ----------
	disableVinZeroBatch bool

	// ---------- utility ----------
	// regexpRemoveTrailingZeros is used to remove trailing zeroes from utxo
	// fake addresses. Defined here, to compile just once
	regexpRemoveTrailingZeros *regexp.Regexp
}

// NewClient generates a new Client
func NewClient(
	thorKeys *thornadoclient.Keys,
	cfg config.BifrostChainConfiguration,
	bridge thornadoclient.ThornadoBridge,
	localState p2pstorage.LocalStateManager,
	m *metrics.Metrics,
	coordinator frost.SessionCoordinator,
) (*Client, error) {
	// verify the chain is supported
	supported := map[common.Chain]bool{
		common.BTCChain: true,
	}
	if !supported[cfg.ChainID] {
		return nil, fmt.Errorf("unsupported utxo chain: %s", cfg.ChainID)
	}

	logger := log.Logger.With().Stringer("chain", cfg.ChainID).Logger()
	if !cfg.BlockScanner.ScanMemPool {
		logger.Warn().Msg("BTC mempool scanning disabled in config; enabling for Thornado inbound pre-observation")
		cfg.BlockScanner.ScanMemPool = true
	}

	// create rpc client
	rpcClient, err := rpc.NewClient(
		cfg.RPCHost,
		cfg.UserName,
		cfg.Password,
		cfg.MaxRPCRetries,
		cfg.BlockScanner.HTTPRequestTimeout,
		cfg.ChainID,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("fail to create rpc client: %w", err)
	}

	thorPrivateKey, err := thorKeys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get Thornado private key: %w", err)
	}
	localParty, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(thorPrivateKey.PubKey().Bytes()))
	if err != nil {
		return nil, fmt.Errorf("fail to derive local FROST party key: %w", err)
	}
	frostKeysign, err := newVaultSigner(bridge, localState, logger, coordinator, localParty.String())
	if err != nil {
		return nil, fmt.Errorf("fail to create vault signer: %w", err)
	}
	nodePrivKey, _ := btcec.PrivKeyFromBytes(thorPrivateKey.Bytes())
	nodePubKey, err := bech32AccountPubKey(nodePrivKey)
	if err != nil {
		return nil, fmt.Errorf("fail to get node account public key: %w", err)
	}

	// create base client
	c := &Client{
		cfg:                       cfg,
		log:                       logger,
		m:                         m,
		rpc:                       rpcClient,
		nodePubKey:                nodePubKey,
		nodePrivKey:               nodePrivKey,
		frostKeySigner:            frostKeysign,
		wg:                        &sync.WaitGroup{},
		signerLock:                &sync.Mutex{},
		vaultLocks:                make(map[string]*sync.Mutex),
		vaultPaths:                make(map[string]map[uint64]struct{}),
		stopchan:                  make(chan struct{}),
		bridge:                    bridge,
		regexpRemoveTrailingZeros: regexp.MustCompile(`(?:00)+$`),
	}

	// import the node local address in the daemon wallet
	err = c.RegisterPublicKey(c.nodePubKey)
	if err != nil {
		return nil, fmt.Errorf("fail to register (%s): %w", c.nodePubKey, err)
	}

	var path string // fallback to in memory storage if unset
	if len(c.cfg.BlockScanner.DBPath) > 0 {
		path = fmt.Sprintf("%s/%s", c.cfg.BlockScanner.DBPath, c.cfg.BlockScanner.ChainID)
	}
	storage, err := blockscanner.NewBlockScannerStorage(path, c.cfg.ScannerLevelDB)
	if err != nil {
		return c, fmt.Errorf("fail to create blockscanner storage: %w", err)
	}

	c.blockScanner, err = blockscanner.NewBlockScanner(c.cfg.BlockScanner, storage, m, bridge, c)
	if err != nil {
		return c, fmt.Errorf("fail to create block scanner: %w", err)
	}

	c.temporalStorage, err = NewTemporalStorage(storage.GetInternalDb(), c.cfg.MempoolTxIDCacheSize)
	if err != nil {
		return c, fmt.Errorf("fail to create utxo storage: %w", err)
	}

	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	if err != nil {
		return nil, fmt.Errorf("fail to create signer cache manager, err: %w", err)
	}
	c.signerCacheManager = signerCacheManager
	c.updateNetworkInfo()

	return c, nil
}

// NewObserveOnlyClient creates a BTC client for one-shot local observation commands.
// It intentionally skips FROST signer/session setup and scanner state.
func NewObserveOnlyClient(
	thorKeys *thornadoclient.Keys,
	cfg config.BifrostChainConfiguration,
	bridge thornadoclient.ThornadoBridge,
	m *metrics.Metrics,
) (*Client, error) {
	if cfg.ChainID != common.BTCChain {
		return nil, fmt.Errorf("unsupported observe-only chain: %s", cfg.ChainID)
	}
	logger := log.Logger.With().Stringer("chain", cfg.ChainID).Logger()
	rpcClient, err := rpc.NewClient(
		cfg.RPCHost,
		cfg.UserName,
		cfg.Password,
		cfg.MaxRPCRetries,
		cfg.BlockScanner.HTTPRequestTimeout,
		cfg.ChainID,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("fail to create rpc client: %w", err)
	}
	thorPrivateKey, err := thorKeys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get Thornado private key: %w", err)
	}
	nodePrivKey, _ := btcec.PrivKeyFromBytes(thorPrivateKey.Bytes())
	nodePubKey, err := bech32AccountPubKey(nodePrivKey)
	if err != nil {
		return nil, fmt.Errorf("fail to get node account public key: %w", err)
	}
	return &Client{
		cfg:                       cfg,
		log:                       logger,
		m:                         m,
		rpc:                       rpcClient,
		nodePubKey:                nodePubKey,
		nodePrivKey:               nodePrivKey,
		wg:                        &sync.WaitGroup{},
		signerLock:                &sync.Mutex{},
		vaultLocks:                make(map[string]*sync.Mutex),
		vaultPaths:                make(map[string]map[uint64]struct{}),
		stopchan:                  make(chan struct{}),
		bridge:                    bridge,
		regexpRemoveTrailingZeros: regexp.MustCompile(`(?:00)+$`),
	}, nil
}

// GetConfig returns the chain configuration.
func (c *Client) GetConfig() config.BifrostChainConfiguration {
	return c.cfg
}

// GetChain returns the chain ID.
func (c *Client) GetChain() common.Chain {
	return c.cfg.ChainID
}

// SetFrostPartyLeader pins the designated signer as the FROST party coordinator leader.
func (c *Client) SetFrostPartyLeader(leader string) {
	if setter, ok := c.frostKeySigner.(interface{ SetPartyLeader(string) }); ok {
		setter.SetPartyLeader(leader)
	}
}

// ClearFrostPartyLeader clears the pinned party leader after a multi-UTXO sign completes.
func (c *Client) ClearFrostPartyLeader() {
	if clearer, ok := c.frostKeySigner.(interface{ ClearPartyLeader() }); ok {
		clearer.ClearPartyLeader()
	}
}

// IsBlockScannerHealthy returns true if the block scanner is healthy.
func (c *Client) IsBlockScannerHealthy() bool {
	if c == nil || c.blockScanner == nil {
		return false
	}
	return c.blockScanner.IsHealthy()
}

// GetHeight returns current chain (not scanner) height.
func (c *Client) GetHeight() (int64, error) {
	return c.rpc.GetBlockCount()
}

// ObserveTxIn resolves a Bitcoin transaction through this node's RPC and converts it
// to the same inbound observation shape used by the scanner.
func (c *Client) ObserveTxIn(txid string) (types.TxIn, error) {
	tx, err := c.rpc.GetRawTransactionVerbose(txid)
	if err != nil {
		return types.TxIn{}, fmt.Errorf("fail to get raw transaction %s: %w", txid, err)
	}

	height := c.currentBlockHeight.Load()
	mempool := tx.BlockHash == ""
	if !mempool {
		block, err := c.rpc.GetBlockVerbose(tx.BlockHash)
		if err != nil {
			return types.TxIn{}, fmt.Errorf("fail to get block for transaction %s: %w", txid, err)
		}
		height = block.Height
	}
	if height <= 0 {
		height, err = c.GetHeight()
		if err != nil {
			return types.TxIn{}, fmt.Errorf("fail to get chain height for transaction %s: %w", txid, err)
		}
	}

	txInItems, err := c.getTxIns(tx, height, mempool, nil)
	if err != nil {
		return types.TxIn{}, fmt.Errorf("fail to build tx observation %s: %w", txid, err)
	}
	txArray := make([]*types.TxInItem, 0, len(txInItems))
	for i := range txInItems {
		txInItem := txInItems[i]
		if txInItem.IsEmpty() || txInItem.Coins.IsEmpty() {
			continue
		}
		txArray = append(txArray, &txInItem)
	}
	if len(txArray) == 0 {
		return types.TxIn{}, fmt.Errorf("transaction %s is not an observable inbound", txid)
	}

	return types.TxIn{
		Chain:   c.GetChain(),
		TxArray: txArray,
		MemPool: mempool,
	}, nil
}

// GetNetworkFee returns current chain network fee according to Bifrost.
func (c *Client) GetNetworkFee() (transactionSize, transactionFeeRate uint64) {
	transactionSize = c.estimatedAverageTxSize()
	return transactionSize, c.lastFeeRate.Load()
}

// GetBlockScannerHeight returns blockscanner height
func (c *Client) GetBlockScannerHeight() (int64, error) {
	return c.blockScanner.PreviousHeight(), nil
}

// RollbackBlockScanner rolls back the block scanner to the last observed block
func (c *Client) RollbackBlockScanner() error {
	return c.blockScanner.RollbackToLastObserved()
}

func (c *Client) GetLatestTxForVault(vault string) (string, string, error) {
	lastObserved, err := c.signerCacheManager.GetLatestRecordedTx(types.InboundCacheKey(vault, c.GetChain().String()))
	if err != nil {
		return "", "", err
	}
	lastBroadCasted, err := c.signerCacheManager.GetLatestRecordedTx(types.BroadcastCacheKey(vault, c.GetChain().String()))
	return lastObserved, lastBroadCasted, err
}

// GetAddress returns chain address for the given public key.
func (c *Client) GetAddress(pubkey common.PubKey) string {
	return c.GetAddressAtPath(pubkey, common.MainVaultPathIndex)
}

func (c *Client) GetAddressAtPath(pubkey common.PubKey, pathIndex uint64) string {
	addr, err := c.getVaultAddressAtPath(pubkey, pathIndex)
	if err != nil {
		c.log.Error().Err(err).Str("pubkey", pubkey.String()).Msg("fail to get vault address")
		return ""
	}
	return addr.String()
}

func (c *Client) getVaultAddress(pubkey common.PubKey) (common.Address, error) {
	return c.getVaultAddressAtPath(pubkey, common.MainVaultPathIndex)
}

func (c *Client) getVaultAddressAtPath(pubkey common.PubKey, pathIndex uint64) (common.Address, error) {
	cacheKey := fmt.Sprintf("%s|%s|%d", c.cfg.ChainID, pubkey, pathIndex)
	if cached, ok := c.vaultAddrCache.Load(cacheKey); ok {
		return cached.(common.Address), nil
	}
	var (
		addr common.Address
		err  error
	)
	if c.cfg.ChainID.Equals(common.BTCChain) {
		addr, err = common.DeriveBTCTaprootAddress(pubkey, pathIndex)
	} else {
		addr, err = pubkey.GetAddress(c.cfg.ChainID)
	}
	if err != nil {
		return "", err
	}
	c.vaultAddrCache.Store(cacheKey, addr)
	return addr, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Control
////////////////////////////////////////////////////////////////////////////////////////

// Start starts the scanner, signer, and solvency check.
func (c *Client) Start(
	globalTxsQueue chan types.TxIn,
	globalErrataQueue chan types.ErrataBlock,
	globalSolvencyQueue chan types.Solvency,
	globalNetworkFeeQueue chan common.NetworkFee,
) {
	c.globalErrataQueue = globalErrataQueue
	c.globalSolvencyQueue = globalSolvencyQueue
	c.globalNetworkFeeQueue = globalNetworkFeeQueue
	c.frostKeySigner.Start()
	c.blockScanner.Start(globalTxsQueue, globalNetworkFeeQueue)
	c.wg.Add(1)
	go runners.SolvencyCheckRunner(
		c.GetChain(), c, c.bridge, c.stopchan, c.wg, constants.ThornadoBlockTime,
	)
}

// Stop stops the scanner, signer, and solvency check.
func (c *Client) Stop() {
	c.blockScanner.Stop()
	c.frostKeySigner.Stop()
	close(c.stopchan)
	c.wg.Wait()
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Accounts
////////////////////////////////////////////////////////////////////////////////////////

// RegisterPublicKey imports the provided public key in the chain daemon.
func (c *Client) RegisterPublicKey(pubkey common.PubKey) error {
	return c.RegisterPublicKeyAtPath(pubkey, common.MainVaultPathIndex)
}

func (c *Client) RegisterPublicKeyAtPath(pubkey common.PubKey, pathIndex uint64) error {
	addr, err := c.getVaultAddressAtPath(pubkey, pathIndex)
	if err != nil {
		return fmt.Errorf("fail to get address from pubkey(%s): %w", pubkey, err)
	}

	if !c.cfg.ChainID.Equals(common.BTCChain) {
		err = c.rpc.CreateWallet("")
		if err != nil {
			c.log.Info().Err(err).Msg("fail to create wallet")
			return err
		}
	}

	if c.cfg.ChainID.Equals(common.BTCChain) {
		unlock, lockErr := lockBTCWalletImport()
		if lockErr != nil {
			return lockErr
		}
		defer unlock()

		known, knownErr := c.rpc.AddressKnown(addr.String())
		if knownErr == nil && known {
			c.rememberVaultPath(pubkey, pathIndex)
			return nil
		}
		if knownErr != nil {
			c.log.Debug().Err(knownErr).Str("addr", addr.String()).Msg("fail to check imported bitcoin address")
		}
	}

	if c.cfg.ChainID.Equals(common.BTCChain) {
		err = c.rpc.ImportDescriptorAddress(addr.String())
		if err != nil {
			c.log.Warn().Err(err).Str("addr", addr.String()).Msg("descriptor import failed, falling back to legacy importaddress")
			err = c.rpc.ImportAddress(addr.String())
		}
	} else {
		err = c.rpc.ImportAddress(addr.String())
	}
	if err != nil {
		c.log.Error().Err(err).
			Str("pubkey", pubkey.String()).
			Str("addr", addr.String()).
			Msg("fail to import address")
		return err
	}
	c.rememberVaultPath(pubkey, pathIndex)
	return nil
}

func lockBTCWalletImport() (func(), error) {
	f, err := os.OpenFile("/tmp/thornado-btc-wallet-import.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("fail to open BTC wallet import lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("fail to lock BTC wallet import: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func (c *Client) rememberVaultPath(pubkey common.PubKey, pathIndex uint64) {
	c.vaultPathLock.Lock()
	key := pubkey.String()
	if c.vaultPaths[key] == nil {
		c.vaultPaths[key] = make(map[uint64]struct{})
	}
	c.vaultPaths[key][pathIndex] = struct{}{}
	c.vaultPathLock.Unlock()

	if addr, err := c.getVaultAddressAtPath(pubkey, pathIndex); err != nil {
		c.log.Debug().Err(err).Str("pubkey", key).Uint64("path_index", pathIndex).Msg("fail to cache vault path address")
	} else {
		c.vaultPathAddrs.Store(strings.ToLower(addr.String()), struct{}{})
	}
}

func (c *Client) registeredVaultPaths(pubkey common.PubKey) []uint64 {
	c.vaultPathLock.RLock()
	defer c.vaultPathLock.RUnlock()

	paths := c.vaultPaths[pubkey.String()]
	if len(paths) == 0 {
		return []uint64{common.MainVaultPathIndex}
	}
	result := make([]uint64, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	return result
}

// GetAccount returns the account details for the given public key.
func (c *Client) GetAccount(pubkey common.PubKey, height *big.Int) (common.Account, error) {
	return c.getAccountForPaths(pubkey, c.registeredVaultPathsWithMain(pubkey), false)
}

func (c *Client) getSolvencyAccount(pubkey common.PubKey) (common.Account, error) {
	return c.getAccountForPaths(pubkey, c.solvencyVaultPaths(pubkey), true)
}

func (c *Client) getAccountForPaths(pubkey common.PubKey, pathIndexes []uint64, includeMempool bool) (common.Account, error) {
	acct := common.Account{}
	if pubkey.IsEmpty() {
		return acct, errors.New("pubkey can't be empty")
	}

	total := 0.0
	for _, pathIndex := range pathIndexes {
		addr, err := c.getVaultAddressAtPath(pubkey, pathIndex)
		if err != nil {
			return acct, fmt.Errorf("fail to get address from pubkey(%s) path(%d): %w", pubkey, pathIndex, err)
		}
		utxos, err := c.rpc.ListUnspent(addr.String())
		if err != nil {
			return acct, fmt.Errorf("fail to get UTXOs: %w", err)
		}

		total += c.sumAccountUtxos(utxos, includeMempool)
	}
	totalAmt, err := btcutil.NewAmount(total)
	if err != nil {
		return acct, fmt.Errorf("fail to convert total amount: %w", err)
	}
	return common.NewAccount(0, 0,
		common.Coins{
			common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(totalAmt))),
		}), nil
}

func (c *Client) sumAccountUtxos(utxos []btcjson.ListUnspentResult, includeMempool bool) float64 {
	total := 0.0
	for _, item := range utxos {
		if !c.isValidUTXO(item.ScriptPubKey) {
			continue
		}
		if !includeMempool && item.Confirmations == 0 {
			// pending tx in mempool, only count sends from base
			if !c.isSelfTransaction(item.TxID) && !c.isFromBase(item.TxID) {
				continue
			}
		}
		total += item.Amount
	}
	return total
}

func (c *Client) solvencyVaultPaths(pubkey common.PubKey) []uint64 {
	seen := make(map[uint64]struct{})
	paths := make([]uint64, 0, int(common.DepositAddressLookahead)+1)
	add := func(pathIndex uint64) {
		if _, ok := seen[pathIndex]; ok {
			return
		}
		seen[pathIndex] = struct{}{}
		paths = append(paths, pathIndex)
	}
	for _, pathIndex := range c.registeredVaultPathsWithMain(pubkey) {
		add(pathIndex)
	}
	for _, pathType := range []common.VaultDepositPathType{common.VaultDepositPathUser, common.VaultDepositPathNode} {
		pathIndexes, err := common.VaultDepositLookaheadPathIndexes(pathType)
		if err != nil {
			c.log.Error().Err(err).Str("pubkey", pubkey.String()).Str("path_type", string(pathType)).Msg("fail to derive solvency vault deposit path lookahead")
			continue
		}
		for _, pathIndex := range pathIndexes {
			add(pathIndex)
		}
	}
	return paths
}

// GetAccountByAddress is unimplemented for UTXO chains.
func (c *Client) GetAccountByAddress(address string, height *big.Int) (common.Account, error) {
	return common.Account{}, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Observations
////////////////////////////////////////////////////////////////////////////////////////

// OnObservedTxIn is called by the observer when a transaction is observed.
func (c *Client) OnObservedTxIn(txIn types.TxInItem, blockHeight int64) {
	// sanity check the transaction has a valid hash
	_, err := chainhash.NewHashFromStr(txIn.Tx)
	if err != nil {
		c.log.Error().Err(err).Str("txID", txIn.Tx).Msg("fail to add spendable utxo to storage")
		return
	}

	observedStage := ObservedTxStageFinal
	if blockHeight <= 0 {
		observedStage = ObservedTxStageMempool
	}
	cacheKey := c.observedTxCacheKey(txIn)
	if _, _, err = c.temporalStorage.TrackObservedTxStage(cacheKey, observedStage); err != nil {
		c.log.Err(err).Msgf("fail to add hash (%s) to observed tx cache", cacheKey)
	}
	c.recordObservedTxBlockMeta(txIn, blockHeight)
}

func (c *Client) recordObservedTxBlockMeta(txIn types.TxInItem, blockHeight int64) {
	blockMeta, err := c.temporalStorage.GetBlockMeta(blockHeight)
	if err != nil {
		c.log.Err(err).Int64("height", blockHeight).Msgf("fail to get block meta")
		return
	}
	if blockMeta == nil {
		blockMeta = NewBlockMeta("", blockHeight, "")
	}
	if c.isBaseAddress(txIn.Sender) {
		c.log.Debug().Int64("height", blockHeight).Msgf("add hash %s as self transaction", txIn.Tx)
		blockMeta.AddSelfTransaction(txIn.Tx)
	} else {
		// add the transaction to block meta
		blockMeta.AddCustomerTransaction(txIn.Tx)
	}
	if err = c.temporalStorage.SaveBlockMeta(blockHeight, blockMeta); err != nil {
		c.log.Err(err).Int64("height", blockHeight).Msgf("fail to save block meta to storage")
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Fetch Block
////////////////////////////////////////////////////////////////////////////////////////

// FetchTxs retrieves txs for a block height. The first argument is the block height to
// fetch, the second argument is the current chain tip.
func (c *Client) FetchTxs(height, chainHeight int64) (types.TxIn, error) {
	txIn := types.TxIn{
		Chain:   c.cfg.ChainID,
		TxArray: nil,
	}

	block, err := c.getBlock(height)
	if err != nil {
		if rpcErr, ok := err.(*btcjson.RPCError); ok && rpcErr.Code == btcjson.ErrRPCInvalidParameter {
			// this means the tx had been broadcast to chain, it must be another signer finished quicker then us
			return txIn, btypes.ErrUnavailableBlock
		}
		return txIn, fmt.Errorf("fail to get block: %w", err)
	}
	// if somehow the block is not valid
	if block.Hash == "" && block.PreviousHash == "" {
		return txIn, fmt.Errorf("fail to get block: %w", err)
	}

	c.updateCurrentBlockHeight(chainHeight)
	reScannedTxs, err := c.processReorg(block)
	if err != nil {
		if errors.Is(err, btypes.ErrPendingErrataDelay) {
			return txIn, err
		}
		c.log.Err(err).Msg("fail to process re-org")
	}
	if len(reScannedTxs) > 0 {
		for _, item := range reScannedTxs {
			if len(item.TxArray) == 0 {
				continue
			}
			txIn.TxArray = append(txIn.TxArray, item.TxArray...)
		}
	}

	blockMeta, err := c.temporalStorage.GetBlockMeta(block.Height)
	if err != nil {
		return txIn, fmt.Errorf("fail to get block meta from storage: %w", err)
	}
	if blockMeta == nil {
		blockMeta = NewBlockMeta(block.PreviousHash, block.Height, block.Hash)
	} else {
		blockMeta.PreviousHash = block.PreviousHash
		blockMeta.BlockHash = block.Hash
	}

	if err = c.temporalStorage.SaveBlockMeta(block.Height, blockMeta); err != nil {
		return txIn, fmt.Errorf("fail to save block meta into storage: %w", err)
	}
	pruneHeight := height - int64(c.cfg.UTXO.BlockCacheCount)
	if pruneHeight > 0 {
		defer func() {
			if err = c.temporalStorage.PruneBlockMeta(pruneHeight, c.canDeleteBlock); err != nil {
				c.log.Err(err).Int64("height", pruneHeight).Msg("fail to prune block meta")
			}
		}()
	}

	var txInBlock types.TxIn
	txInBlock, err = c.extractTxs(block)
	if err != nil {
		return types.TxIn{}, fmt.Errorf("fail to extract txIn from block: %w", err)
	}
	if len(txInBlock.TxArray) > 0 {
		txIn.TxArray = append(txIn.TxArray, txInBlock.TxArray...)
	}

	c.updateNetworkInfo()

	// report network fee and solvency if within flexibility blocks of tip
	if chainHeight-height <= c.cfg.BlockScanner.ObservationFlexibilityBlocks {
		err = c.sendNetworkFee(height)
		if err != nil {
			c.log.Err(err).Msg("fail to send network fee")
		}
		// when block scanner is not healthy, only report from auto-unhalt SolvencyCheckRunner
		if c.IsBlockScannerHealthy() {
			if err = c.ReportSolvency(height); err != nil {
				c.log.Err(err).Msg("fail to report solvency info")
			}
		}
	}

	return txIn, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Fetch Mempool
////////////////////////////////////////////////////////////////////////////////////////

// FetchMemPool retrieves txs from mempool
func (c *Client) FetchMemPool(height int64) (types.TxIn, error) {
	hashes, err := c.rpc.GetRawMempool()
	if err != nil {
		return types.TxIn{}, fmt.Errorf("fail to get tx hashes from mempool: %w", err)
	}
	currentMempool := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		currentMempool[h] = struct{}{}
	}
	c.errataDroppedMempoolTxs(height, currentMempool)

	txIn := types.TxIn{
		Chain:   c.GetChain(),
		MemPool: true,
	}

	// shuffle to distribute observations when mempool is large
	rand.Shuffle(len(hashes), func(i, j int) {
		hashes[i], hashes[j] = hashes[j], hashes[i]
	})

	// create batches
	batches := [][]string{}
	batch := []string{}
	for _, h := range hashes {
		// skip transactions we have already processed
		if !c.tryAddToMemPoolCache(h) {
			c.log.Debug().Msgf("ignoring processed tx %s", h)
			continue
		}

		// only process up to the batch size at once
		batch = append(batch, h)
		if len(batch) >= c.cfg.UTXO.TransactionBatchSize {
			batches = append(batches, batch)
			batch = []string{}
		}

		// if we have enough batches, stop
		if len(batches) >= c.cfg.UTXO.MaxMempoolBatches {
			break
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}

	// clear mempool cache for batches i or later in case of error
	clearMemPoolCache := func(i int) {
		for j := i; j < len(batches); j++ {
			for _, h := range batches[j] {
				c.removeFromMemPoolCache(h)
			}
		}
	}

	var returnErr error
	errCount := 0
	for i, batch := range batches {
		// fetch the batch of results
		var results []*btcjson.TxRawResult
		var errs []error
		results, errs, err = c.rpc.BatchGetRawTransactionVerbose(batch)
		if err != nil { // clear mempool cache for unprocessed batches and return error
			clearMemPoolCache(i)
			returnErr = fmt.Errorf("fail to get raw transactions from mempool: %w", err)
			break
		}

		// process the batch results
		for i := range results {
			result := results[i]
			err = errs[i]
			// the transaction could have been removed, regardless safe to continue
			if err != nil {
				errCount++
				c.removeFromMemPoolCache(batch[i]) // remove from mempool cache so it will retry
				c.log.Err(err).Str("hash", batch[i]).Msg("fail to get raw transaction verbose")
				continue
			}

			// filter transactions
			var txInItems []types.TxInItem
			txInItems, err = c.getTxIns(result, height, true, nil)
			if err != nil {
				c.removeFromMemPoolCache(batch[i])
				c.log.Debug().Err(err).Msg("fail to get TxInItem")
				continue
			}
			if len(txInItems) == 0 {
				c.removeFromMemPoolCache(batch[i])
				continue
			}
			for j := range txInItems {
				txInItem := txInItems[j]
				if txInItem.IsEmpty() || txInItem.Coins.IsEmpty() {
					continue
				}
				txIn.TxArray = append(txIn.TxArray, &txInItem)
			}
		}
	}

	// log some info if we observed or had errors
	if len(txIn.TxArray) > 0 || errCount > 0 {
		logEvent := c.log.Debug()
		if errCount > 0 {
			logEvent = c.log.Warn()
		}
		logEvent.Int("batch", len(batch)).
			Int("txins", len(txIn.TxArray)).
			Int("errors", errCount).
			Msg("retrieved mempool batch")
	}

	return txIn, returnErr
}

func (c *Client) errataDroppedMempoolTxs(height int64, currentMempool map[string]struct{}) {
	tracked, err := c.temporalStorage.ListMempoolTxs()
	if err != nil {
		c.log.Err(err).Msg("fail to list tracked mempool txs")
		return
	}

	var dropped []types.ErrataTx
	for _, txid := range tracked {
		if _, ok := currentMempool[txid]; ok {
			continue
		}
		if c.confirmTx(txid) {
			c.clearMissingErrata(txid)
			// Keep mined txs in the mempool marker set until block extraction
			// consumes them. That lets the later final observation pass the
			// observed-tx duplicate cache after the pre-confirmation observe.
			continue
		}
		if !c.missingErrataReady(txid, height) {
			continue
		}
		c.removeFromMemPoolCache(txid)
		dropped = append(dropped, types.ErrataTx{
			TxID:  common.TxID(txid),
			Chain: c.cfg.ChainID,
		})
	}
	if len(dropped) == 0 || c.globalErrataQueue == nil {
		return
	}
	c.globalErrataQueue <- types.ErrataBlock{
		Height: height,
		Txs:    dropped,
	}
}

func (c *Client) missingErrataReady(txid string, height int64) bool {
	if txid == "" {
		return false
	}
	if height <= 0 {
		height = c.getCurrentBlockHeight()
	}
	if height <= 0 {
		chainHeight, err := c.GetHeight()
		if err != nil {
			c.log.Err(err).Str("txid", txid).Msg("fail to resolve BTC height for missing tx errata delay")
			return false
		}
		c.updateCurrentBlockHeight(chainHeight)
		height = chainHeight
	}
	return c.missingErrataReadySince(txid, height, height)
}

func (c *Client) missingErrataReadySince(txid string, firstMissingHeight, height int64) bool {
	if txid == "" {
		return false
	}
	if firstMissingHeight <= 0 {
		firstMissingHeight = height
	}
	if height <= 0 {
		height = c.getCurrentBlockHeight()
	}
	if height <= 0 {
		chainHeight, err := c.GetHeight()
		if err != nil {
			c.log.Err(err).Str("txid", txid).Msg("fail to resolve BTC height for missing tx errata delay")
			return false
		}
		c.updateCurrentBlockHeight(chainHeight)
		height = chainHeight
	}
	key := strings.ToUpper(txid)
	first, ok := c.missingErrataFirstSeen.Load(key)
	if !ok {
		c.missingErrataFirstSeen.Store(key, firstMissingHeight)
		if height-firstMissingHeight >= btcErrataDelayBlocks {
			return true
		}
		c.log.Info().
			Int64("height", height).
			Int64("first_missing_height", firstMissingHeight).
			Int64("delay_blocks", btcErrataDelayBlocks).
			Str("txid", txid).
			Msg("missing BTC tx first seen; delaying errata")
		return false
	}
	firstHeight, ok := first.(int64)
	if !ok {
		c.missingErrataFirstSeen.Store(key, firstMissingHeight)
		return false
	}
	if height-firstHeight < btcErrataDelayBlocks {
		c.log.Info().
			Int64("height", height).
			Int64("first_missing_height", firstHeight).
			Int64("delay_blocks", btcErrataDelayBlocks).
			Str("txid", txid).
			Msg("missing BTC tx still inside errata delay")
		return false
	}
	return true
}

func (c *Client) clearMissingErrata(txid string) {
	if txid == "" {
		return
	}
	c.missingErrataFirstSeen.Delete(strings.ToUpper(txid))
}

func (c *Client) errataMissingObservedTx(height int64, txid string) bool {
	if txid == "" || c.globalErrataQueue == nil {
		return false
	}
	c.clearMissingErrata(txid)
	c.removeFromMemPoolCache(txid)
	if err := c.temporalStorage.UntrackObservedTx(txid); err != nil {
		c.log.Err(err).Str("txid", txid).Msg("fail to remove missing observed tx from cache")
	}
	c.globalErrataQueue <- types.ErrataBlock{
		Height: height,
		Txs: []types.ErrataTx{
			{
				TxID:  common.TxID(txid),
				Chain: c.cfg.ChainID,
			},
		},
	}
	return true
}

func (c *Client) errataMissingObservedTxs(height int64, txIn types.TxIn) bool {
	if height <= 0 {
		height = c.getCurrentBlockHeight()
		if height <= 0 {
			if chainHeight, err := c.GetHeight(); err == nil {
				c.updateCurrentBlockHeight(chainHeight)
				height = chainHeight
			} else {
				c.log.Err(err).Msg("fail to refresh chain height for missing observed tx errata")
			}
		}
	}
	missing := false
	delayHeight := c.getCurrentBlockHeight()
	if delayHeight <= 0 {
		delayHeight = height
	}
	for _, tx := range txIn.TxArray {
		if tx == nil || tx.Tx == "" {
			continue
		}
		if c.confirmTx(tx.Tx) {
			c.clearMissingErrata(tx.Tx)
			continue
		}
		if !c.missingErrataReady(tx.Tx, delayHeight) {
			missing = true
			continue
		}
		c.log.Info().Int64("height", height).Str("txid", tx.Tx).Msg("observed tx disappeared; queue errata")
		if c.errataMissingObservedTx(height, tx.Tx) {
			missing = true
		}
	}
	return missing
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Confirmation Counting
////////////////////////////////////////////////////////////////////////////////////////

// GetConfirmationCount returns the number of blocks required before processing in
// Thornado.
func (c *Client) GetConfirmationCount(txIn types.TxIn) int64 {
	// if there are no txs, nothing will be reported
	if len(txIn.TxArray) == 0 {
		return 0
	}

	// Mempool transactions are observed immediately, but still report the
	// configured minimum confirmations to Thornado for user-facing progress.
	if txIn.MemPool {
		minConfirmations, err := c.bridge.GetConfigValue(constants.BTC_ConfirmationsMin.String())
		if err != nil || minConfirmations <= 0 {
			minConfirmations = int64(c.cfg.MinConfirmations)
		}
		if minConfirmations <= 0 {
			minConfirmations = 1
		}
		return minConfirmations
	}

	// get the block height and confirmation required
	height := txIn.TxArray[0].BlockHeight
	if c.protocolControlledTxIn(txIn) {
		c.log.Info().Int64("height", height).Msg("protocol-controlled bitcoin tx requires 1 confirmation")
		return 1
	}
	confirm, err := c.getBlockRequiredConfirmation(txIn, height)
	if err != nil {
		c.log.Err(err).Int64("height", height).Msg("fail to get required confirmation")
		return 0
	}
	if confirm > 1 {
		confirm--
	}

	c.log.Info().Int64("height", height).Msgf("confirmation required: %d", confirm)
	return confirm
}

// ConfirmationCountReady will be called by the observer before sending the txIn to
// Thornado. It will return true if the scanner height is greater than or equal to the
// observed block height + confirmation required - 1.
// https://medium.com/coinmonks/1confvalue-a-simple-pow-confirmation-rule-of-thumb-a8d9c6c483dd
func (c *Client) ConfirmationCountReady(txIn types.TxIn) bool {
	// if there are no txs, nothing will be reported
	if len(txIn.TxArray) == 0 {
		return true
	}

	if txIn.MemPool {
		height := int64(0)
		if len(txIn.TxArray) > 0 {
			height = txIn.TxArray[0].BlockHeight
		}
		if c.errataMissingObservedTxs(height, txIn) {
			return false
		}
		// Mempool transactions are ready for pre-confirmation observation.
		return true
	}

	// check if we have the necessary number of confirmations
	height := txIn.TxArray[0].BlockHeight
	if c.errataMissingObservedTxs(height, txIn) {
		return false
	}

	confirm := txIn.ConfirmationRequired
	if confirm <= 0 {
		return true
	}

	currentHeight := c.getCurrentBlockHeight()
	if currentHeight < height {
		if chainHeight, err := c.GetHeight(); err == nil {
			c.updateCurrentBlockHeight(chainHeight)
			currentHeight = c.getCurrentBlockHeight()
		} else {
			c.log.Err(err).Int64("height", height).Msg("fail to refresh chain height for confirmation count")
		}
	}

	confirmations := currentHeight - height + 1
	ready := confirmations >= confirm
	c.log.Info().
		Int64("height", height).
		Int64("chain_height", currentHeight).
		Int64("required", confirm).
		Int64("confirmations", confirmations).
		Bool("ready", ready).
		Msg("confirmation count check")

	return ready
}

func (c *Client) getCurrentBlockHeight() int64 {
	return c.currentBlockHeight.Load()
}

// updateCurrentBlockHeight records the highest scanner height seen without
// allowing concurrent prefetches to move it backwards.
func (c *Client) updateCurrentBlockHeight(height int64) {
	for {
		currentHeight := c.getCurrentBlockHeight()
		if height <= currentHeight {
			return
		}
		if c.currentBlockHeight.CompareAndSwap(currentHeight, height) {
			return
		}
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Solvency
////////////////////////////////////////////////////////////////////////////////////////

// ShouldReportSolvency returns true if solvency should be reported at the given height.
func (c *Client) ShouldReportSolvency(height int64) bool {
	return c.shouldReportSolvency(height)
}

func (c *Client) shouldReportSolvency(height int64) bool {
	lastHeight := c.lastSolvencyCheckHeight.Load()
	if height-lastHeight <= 1 {
		return false
	}

	return true
}

// ReportSolvency reports solvency for all base vaults.
func (c *Client) ReportSolvency(height int64) error {
	c.solvencyLock.Lock()
	defer c.solvencyLock.Unlock()

	if !c.shouldReportSolvency(height) {
		return nil
	}

	// fetch all base vaults
	baseVaults, err := c.bridge.GetBaseVaults()
	if err != nil {
		return fmt.Errorf("fail to get baseVaults: %w", err)
	}

	currentGasFee := cosmos.NewUint(3 * c.estimatedAverageTxSize() * c.lastFeeRate.Load())

	chainVaults := baseVaults[:0]
	for _, base := range baseVaults {
		if base.HasFundsForChain(c.cfg.ChainID) {
			chainVaults = append(chainVaults, base)
		}
	}

	msgs := make([]types.Solvency, 0, len(chainVaults))
	for i := range chainVaults {
		var acct common.Account
		acct, err = c.getSolvencyAccount(chainVaults[i].PubKey)
		if err != nil {
			c.log.Err(err).Msgf("fail to get account balance")
			continue
		}

		msg := types.Solvency{
			Height: height,
			Chain:  c.cfg.ChainID,
			PubKey: chainVaults[i].PubKey,
			Coins:  acct.Coins,
		}
		solvent := runners.IsVaultSolvent(c.cfg.ChainID, acct, chainVaults[i], currentGasFee)
		msgs = append(msgs, msg)

		c.log.Info().
			Stringer("base", msg.PubKey).
			Interface("coins", msg.Coins).
			Bool("solvent", solvent).
			Msg("reporting solvency")
	}

	for i := range msgs {
		// send solvency to thornado via global queue consumed by the observer
		select {
		case c.globalSolvencyQueue <- msgs[i]:
		case <-time.After(constants.ThornadoBlockTime):
			c.log.Warn().Msgf("timeout sending solvency to thornado")
		}
	}

	c.lastSolvencyCheckHeight.Store(height)
	return nil
}
