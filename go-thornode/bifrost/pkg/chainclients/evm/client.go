package evm

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ecommon "github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	ethclient "github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ethereum/go-ethereum"
	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/runners"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	tssp "gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/tss"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/aggregators"
	mem "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
)

////////////////////////////////////////////////////////////////////////////////////////
// EVMClient
////////////////////////////////////////////////////////////////////////////////////////

// EVMClient is a generic client for interacting with EVM chains.
type EVMClient struct {
	logger                  zerolog.Logger
	cfg                     config.BifrostChainConfiguration
	localPubKey             common.PubKey
	kw                      *evm.KeySignWrapper
	ethClient               *ethclient.Client
	evmScanner              *EVMScanner
	bridge                  thorclient.ThorchainBridge
	blockScanner            *blockscanner.BlockScanner
	vaultABI                *abi.ABI
	pubkeyMgr               pubkeymanager.PubKeyValidator
	poolMgr                 thorclient.PoolManager
	tssKeySigner            *tss.KeySign
	wg                      *sync.WaitGroup
	stopchan                chan struct{}
	globalSolvencyQueue     chan stypes.Solvency
	signerCacheManager      *signercache.CacheManager
	lastSolvencyCheckHeight int64

	// pendingLiabilities tracks signed but unconfirmed transaction amounts (value + maxGas)
	// keyed by vault address -> nonce -> amount in Wei.
	pendingLiabilities map[string]map[uint64]*big.Int
	liabilityMu        sync.Mutex

	// signingReady is set to true once pending liabilities have been reconstructed.
	// SignTx will reject requests until this is true to prevent overspending.
	signingReady atomic.Bool
}

// NewEVMClient creates a new EVMClient.
func NewEVMClient(
	thorKeys *thorclient.Keys,
	cfg config.BifrostChainConfiguration,
	server *tssp.TssServer,
	bridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	poolMgr thorclient.PoolManager,
) (*EVMClient, error) {
	// check required arguments
	if thorKeys == nil {
		return nil, fmt.Errorf("failed to create EVM client, thor keys empty")
	}
	if bridge == nil {
		return nil, errors.New("thorchain bridge is nil")
	}
	if pubkeyMgr == nil {
		return nil, errors.New("pubkey manager is nil")
	}
	if poolMgr == nil {
		return nil, errors.New("pool manager is nil")
	}

	// create keys
	tssKm, err := tss.NewKeySign(server, bridge)
	if err != nil {
		return nil, fmt.Errorf("failed to create tss signer: %w", err)
	}
	priv, err := thorKeys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}
	temp, err := codec.ToCmtPubKeyInterface(priv.PubKey())
	if err != nil {
		return nil, fmt.Errorf("failed to get tm pub key: %w", err)
	}
	pk, err := common.NewPubKeyFromCrypto(temp)
	if err != nil {
		return nil, fmt.Errorf("failed to get pub key: %w", err)
	}
	evmPrivateKey, err := evm.GetPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	clog := log.With().Str("module", "evm").Stringer("chain", cfg.ChainID).Logger()

	// create rpc client based on what authentication config is set
	var ethClient *ethclient.Client
	switch {
	case cfg.AuthorizationBearer != "":

		clog.Info().Msg("initializing evm client with bearer token")
		authFn := func(h http.Header) error {
			h.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AuthorizationBearer))
			return nil
		}
		var rpcClient *rpc.Client
		rpcClient, err = rpc.DialOptions(
			context.Background(),
			cfg.RPCHost,
			rpc.WithHTTPAuth(authFn),
		)
		if err != nil {
			return nil, err
		}
		ethClient = ethclient.NewClient(rpcClient)

	case cfg.UserName != "" && cfg.Password != "":
		clog.Info().Msg("initializing evm client with http basic auth")

		authFn := func(h http.Header) error {
			auth := base64.StdEncoding.EncodeToString([]byte(cfg.UserName + ":" + cfg.Password))
			h.Set("Authorization", fmt.Sprintf("Basic %s", auth))
			return nil
		}
		var rpcClient *rpc.Client
		rpcClient, err = rpc.DialOptions(
			context.Background(),
			cfg.RPCHost,
			rpc.WithHTTPAuth(authFn),
		)
		if err != nil {
			return nil, err
		}
		ethClient = ethclient.NewClient(rpcClient)

	default:
		ethClient, err = ethclient.Dial(cfg.RPCHost)
		if err != nil {
			return nil, fmt.Errorf("fail to dial ETH rpc host(%s): %w", cfg.RPCHost, err)
		}
	}

	rpcClient, err := evm.NewEthRPC(
		ethClient,
		cfg.BlockScanner.HTTPRequestTimeout,
		cfg.ChainID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("fail to create ETH rpc host(%s): %w", cfg.RPCHost, err)
	}

	// get chain id
	chainID, err := getChainID(ethClient, cfg.BlockScanner.HTTPRequestTimeout)
	if err != nil {
		return nil, err
	}
	if chainID.Uint64() == 0 {
		return nil, fmt.Errorf("chain id is: %d , invalid", chainID.Uint64())
	}

	// create keysign wrapper
	keysignWrapper, err := evm.NewKeySignWrapper(evmPrivateKey, pk, tssKm, chainID, cfg.ChainID.String())
	if err != nil {
		return nil, fmt.Errorf("fail to create %s key sign wrapper: %w", cfg.ChainID, err)
	}

	// load vault abi
	vaultABI, _, err := evm.GetContractABI(routerContractABI, erc20ContractABI)
	if err != nil {
		return nil, fmt.Errorf("fail to get contract abi: %w", err)
	}

	c := &EVMClient{
		logger:             clog,
		cfg:                cfg,
		ethClient:          ethClient,
		localPubKey:        pk,
		kw:                 keysignWrapper,
		bridge:             bridge,
		vaultABI:           vaultABI,
		pubkeyMgr:          pubkeyMgr,
		poolMgr:            poolMgr,
		tssKeySigner:       tssKm,
		wg:                 &sync.WaitGroup{},
		stopchan:           make(chan struct{}),
		pendingLiabilities: make(map[string]map[uint64]*big.Int),
	}

	// initialize storage
	var path string // if not set later, will in memory storage
	if len(c.cfg.BlockScanner.DBPath) > 0 {
		path = fmt.Sprintf("%s/%s", c.cfg.BlockScanner.DBPath, c.cfg.BlockScanner.ChainID)
	}
	storage, err := blockscanner.NewBlockScannerStorage(path, c.cfg.ScannerLevelDB)
	if err != nil {
		return c, fmt.Errorf("fail to create blockscanner storage: %w", err)
	}
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	if err != nil {
		return nil, fmt.Errorf("fail to create signer cache manager")
	}
	c.signerCacheManager = signerCacheManager

	// create block scanner
	c.evmScanner, err = NewEVMScanner(
		c.cfg.BlockScanner,
		storage,
		chainID,
		ethClient,
		rpcClient,
		c.bridge,
		m,
		pubkeyMgr,
		c.ReportSolvency,
		signerCacheManager,
	)
	if err != nil {
		return c, fmt.Errorf("fail to create evm block scanner: %w", err)
	}

	// initialize block scanner
	c.blockScanner, err = blockscanner.NewBlockScanner(
		c.cfg.BlockScanner, storage, m, c.bridge, c.evmScanner,
	)
	if err != nil {
		return c, fmt.Errorf("fail to create block scanner: %w", err)
	}

	// TODO: Is this necessary?
	localNodeAddress, err := c.localPubKey.GetAddress(cfg.ChainID)
	if err != nil {
		c.logger.Err(err).Stringer("chain", cfg.ChainID).Msg("failed to get local node address")
	}
	c.logger.Info().
		Stringer("chain", cfg.ChainID).
		Stringer("address", localNodeAddress).
		Msg("local node address")

	return c, nil
}

// getPendingLiability returns the sum of all pending (signed but unconfirmed) liabilities
// for the given vault address, and prunes entries for nonces that have already been confirmed.
func (c *EVMClient) getPendingLiability(vaultAddr string, confirmedNonce uint64) *big.Int {
	c.liabilityMu.Lock()
	defer c.liabilityMu.Unlock()

	total := big.NewInt(0)
	vaultLiabilities, exists := c.pendingLiabilities[vaultAddr]
	if !exists {
		return total
	}

	// Prune confirmed nonces and sum pending liabilities
	for nonce, amount := range vaultLiabilities {
		if nonce < confirmedNonce {
			delete(vaultLiabilities, nonce)
			continue
		}
		total.Add(total, amount)
	}
	return total
}

// recordLiability records the liability (value + estimated fee) for a signed transaction.
func (c *EVMClient) recordLiability(vaultAddr string, nonce uint64, amount *big.Int) {
	c.liabilityMu.Lock()
	defer c.liabilityMu.Unlock()

	if c.pendingLiabilities[vaultAddr] == nil {
		c.pendingLiabilities[vaultAddr] = make(map[uint64]*big.Int)
	}
	c.pendingLiabilities[vaultAddr][nonce] = amount
}

// reconstructLiabilities scans the pending block on startup to reconstruct
// in-memory liability tracking for transactions that were signed before a restart.
// It sets signingReady to true once complete, allowing SignTx to proceed.
func (c *EVMClient) reconstructLiabilities() {
	defer c.wg.Done()

	// Get all vault addresses this node is a signer for
	vaultAddresses := make([]string, 0)
	for _, pk := range c.pubkeyMgr.GetAlgoPubKeys(c.cfg.ChainID.GetSigningAlgo(), true) {
		addr, err := pk.GetAddress(c.cfg.ChainID)
		if err != nil {
			c.logger.Warn().Err(err).Str("pubkey", pk.String()).Msg("failed to get vault address")
			continue
		}
		vaultAddresses = append(vaultAddresses, addr.String())
	}

	if len(vaultAddresses) == 0 {
		c.logger.Info().Msg("no vault addresses found, skipping liability reconstruction")
		c.signingReady.Store(true)
		return
	}

	// WaitGroup for local reconstruction routine
	var reconWg sync.WaitGroup
	sem := make(chan struct{}, c.cfg.EVM.ReconstructLiabilitiesConcurrency)
	c.logger.Info().
		Int("vault_count", len(vaultAddresses)).
		Int("concurrency", c.cfg.EVM.ReconstructLiabilitiesConcurrency).
		Msg("starting liability reconstruction for vaults")

	for _, vaultAddr := range vaultAddresses {
		sem <- struct{}{}
		reconWg.Add(1)
		go func(vAddr string) {
			c.logger.Info().Str("vault", vAddr).Msg("starting liability reconstruction for vault")
			defer func() { <-sem }()
			defer reconWg.Done()
			const maxRetries = 12 // ~60 seconds total (12 * 5s)
			retryCount := 0
			for {
				select {
				case <-c.stopchan:
					return
				default:
				}

				ready, err := c.reconstructVaultLiability(vAddr)
				if err != nil {
					c.logger.Warn().Err(err).Str("vault", vAddr).Msg("failed to reconstruct liability, retrying")
				} else if ready {
					return
				}

				retryCount++
				if retryCount >= maxRetries {
					c.logger.Warn().
						Str("vault", vAddr).
						Int("retries", retryCount).
						Msg("could not fully reconstruct liabilities, proceeding anyway")
					return
				}

				// Wait before retrying
				select {
				case <-c.stopchan:
					return
				case <-time.After(5 * time.Second):
				}
			}
		}(vaultAddr)
	}

	// Wait for all vaults to be ready
	reconWg.Wait()

	select {
	case <-c.stopchan:
		return
	default:
		c.logger.Info().Msg("liability reconstruction complete, client ready for signing")
		c.signingReady.Store(true)
	}
}

// reconstructVaultLiability checks a single vault's pending transactions and reconstructs liabilities.
// Returns (ready, error) - ready is true if the vault is safe to sign, false if we need to wait.
func (c *EVMClient) reconstructVaultLiability(vaultAddr string) (bool, error) {
	ctx, cancel := c.getTimeoutContext()
	defer cancel()

	addr := ecommon.HexToAddress(vaultAddr)

	// Get confirmed and pending nonces
	confirmedNonce, err := c.ethClient.NonceAt(ctx, addr, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get confirmed nonce: %w", err)
	}

	ctx2, cancel2 := c.getTimeoutContext()
	defer cancel2()
	pendingNonce, err := c.ethClient.PendingNonceAt(ctx2, addr)
	if err != nil {
		return false, fmt.Errorf("failed to get pending nonce: %w", err)
	}

	pendingCount := pendingNonce - confirmedNonce
	if pendingCount == 0 {
		// No pending transactions, vault is ready
		return true, nil
	}

	c.logger.Info().
		Str("vault", vaultAddr).
		Uint64("confirmedNonce", confirmedNonce).
		Uint64("pendingNonce", pendingNonce).
		Uint64("pendingCount", pendingCount).
		Msg("detected pending transactions on startup")

	// Fetch pending block to find our transactions
	// BlockByNumber with nil returns the pending block
	ctx3, cancel3 := c.getTimeoutContext()
	defer cancel3()
	pendingBlock, err := c.ethClient.BlockByNumber(ctx3, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get pending block: %w", err)
	}

	foundCount := uint64(0)
	for _, tx := range pendingBlock.Transactions() {
		// Get sender address using the scanner's signer
		sender, err := etypes.Sender(c.evmScanner.eipSigner, tx)
		if err != nil {
			continue
		}

		if !strings.EqualFold(sender.Hex(), vaultAddr) {
			continue
		}

		// This is our transaction - record the liability
		nonce := tx.Nonce()
		if nonce >= confirmedNonce && nonce < pendingNonce {
			// Calculate liability: value + max gas cost
			liability := new(big.Int).Set(tx.Value())
			maxGasCost := new(big.Int).Mul(tx.GasFeeCap(), big.NewInt(int64(tx.Gas())))
			liability.Add(liability, maxGasCost)

			c.recordLiability(vaultAddr, nonce, liability)
			foundCount++

			c.logger.Info().
				Str("vault", vaultAddr).
				Uint64("nonce", nonce).
				Str("liability", liability.String()).
				Msg("reconstructed liability from pending transaction")
		}
	}

	if foundCount < pendingCount {
		c.logger.Warn().
			Str("vault", vaultAddr).
			Uint64("expected", pendingCount).
			Uint64("found", foundCount).
			Msg("cannot see all pending transactions, waiting for confirmation")
		return false, nil
	}

	return true, nil
}

// Start starts the chain client with the given queues.
func (c *EVMClient) Start(
	globalTxsQueue chan stypes.TxIn,
	globalErrataQueue chan stypes.ErrataBlock,
	globalSolvencyQueue chan stypes.Solvency,
	globalNetworkFeeQueue chan common.NetworkFee,
) {
	c.evmScanner.globalErrataQueue = globalErrataQueue
	c.evmScanner.globalNetworkFeeQueue = globalNetworkFeeQueue
	c.globalSolvencyQueue = globalSolvencyQueue
	c.tssKeySigner.Start()
	c.blockScanner.Start(globalTxsQueue, globalNetworkFeeQueue)
	c.wg.Add(1)
	go c.unstuck()
	c.wg.Add(1)
	go runners.SolvencyCheckRunner(c.GetChain(), c, c.bridge, c.stopchan, c.wg, c.solvencyRunnerBackoff())
	c.wg.Add(1)
	go c.reconstructLiabilities()
}

func (c *EVMClient) solvencyRunnerBackoff() time.Duration {
	// Use the chain block time for fast chains so the periodic runner does not skip
	// modulo-based solvency report heights. Fall back to THORChain block time.
	dur := time.Duration(c.cfg.ChainID.ApproximateBlockMilliseconds()) * time.Millisecond
	if dur <= 0 || dur > constants.ThorchainBlockTime {
		return constants.ThorchainBlockTime
	}
	return dur
}

// Stop stops the chain client.
func (c *EVMClient) Stop() {
	c.tssKeySigner.Stop()
	c.blockScanner.Stop()
	close(c.stopchan)
	c.wg.Wait()
}

// IsBlockScannerHealthy returns true if the block scanner is healthy.
func (c *EVMClient) IsBlockScannerHealthy() bool {
	return c.blockScanner.IsHealthy()
}

// --------------------------------- config ---------------------------------

// GetConfig returns the chain configuration.
func (c *EVMClient) GetConfig() config.BifrostChainConfiguration {
	return c.cfg
}

// GetChain returns the chain.
func (c *EVMClient) GetChain() common.Chain {
	return c.cfg.ChainID
}

// --------------------------------- status ---------------------------------

// GetHeight returns the current height of the chain.
func (c *EVMClient) GetHeight() (int64, error) {
	return c.evmScanner.GetHeight()
}

// GetBlockScannerHeight returns blockscanner height
func (c *EVMClient) GetBlockScannerHeight() (int64, error) {
	return c.blockScanner.PreviousHeight(), nil
}

// RollbackBlockScanner rolls back the block scanner to the last observed block
func (c *EVMClient) RollbackBlockScanner() error {
	return c.blockScanner.RollbackToLastObserved()
}

func (c *EVMClient) GetLatestTxForVault(vault string) (string, string, error) {
	lastObserved, err := c.signerCacheManager.GetLatestRecordedTx(stypes.InboundCacheKey(vault, c.GetChain().String()))
	if err != nil {
		return "", "", err
	}
	lastBroadCasted, err := c.signerCacheManager.GetLatestRecordedTx(stypes.BroadcastCacheKey(vault, c.GetChain().String()))
	return lastObserved, lastBroadCasted, err
}

// --------------------------------- addresses ---------------------------------

// GetAddress returns the address for the given public key.
func (c *EVMClient) GetAddress(poolPubKey common.PubKey) string {
	addr, err := poolPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		c.logger.Error().Err(err).Str("pool_pub_key", poolPubKey.String()).Msg("fail to get pool address")
		return ""
	}
	return addr.String()
}

// GetAccount returns the account for the given public key.
func (c *EVMClient) GetAccount(pk common.PubKey, height *big.Int) (common.Account, error) {
	addr := c.GetAddress(pk)
	nonce, err := c.evmScanner.GetNonce(addr)
	if err != nil {
		return common.Account{}, err
	}
	contractAddr := c.getSmartContractAddr(pk)
	coins, err := c.GetBalances(addr, height, contractAddr.String())
	if err != nil {
		return common.Account{}, err
	}
	account := common.NewAccount(int64(nonce), 0, coins, false)
	return account, nil
}

// GetAccountByAddress returns the account for the given address.
func (c *EVMClient) GetAccountByAddress(address string, height *big.Int) (common.Account, error) {
	nonce, err := c.evmScanner.GetNonce(address)
	if err != nil {
		return common.Account{}, err
	}
	contractAddr := c.getSmartContractByAddress(common.Address(address))
	coins, err := c.GetBalances(address, height, contractAddr.String())
	if err != nil {
		return common.Account{}, err
	}
	account := common.NewAccount(int64(nonce), 0, coins, false)
	return account, nil
}

func (c *EVMClient) getSmartContractAddr(pubkey common.PubKey) common.Address {
	return c.pubkeyMgr.GetContract(c.cfg.ChainID, pubkey)
}

func (c *EVMClient) getSmartContractByAddress(addr common.Address) common.Address {
	for _, pk := range c.pubkeyMgr.GetAlgoPubKeys(c.cfg.ChainID.GetSigningAlgo(), true) {
		evmAddr, err := pk.GetAddress(c.cfg.ChainID)
		if err != nil {
			c.logger.Warn().
				Err(err).
				Str("chain", common.ETHChain.String()).
				Str("address", addr.String()).
				Str("pubkey", pk.String()).
				Msg("fail to get address for pubkey")
			continue
		}
		if evmAddr.Equals(addr) {
			return c.pubkeyMgr.GetContract(c.cfg.ChainID, pk)
		}
	}
	return common.NoAddress
}

func (c *EVMClient) getTokenAddressFromAsset(asset common.Asset) string {
	if asset.Equals(c.cfg.ChainID.GetGasAsset()) {
		return evm.NativeTokenAddr
	}
	allParts := strings.Split(asset.Symbol.String(), "-")
	return allParts[len(allParts)-1]
}

// --------------------------------- balances ---------------------------------

// GetBalance returns the balance of the provided address.
// contractAddr is the router contract for the vault; for native tokens it is unused.
func (c *EVMClient) GetBalance(addr, token string, height *big.Int, contractAddr string) (*big.Int, error) {
	return c.evmScanner.tokenManager.GetBalance(addr, token, height, contractAddr)
}

// GetBalances returns the balances of the provided address.
// contractAddr is the router contract for the vault.
func (c *EVMClient) GetBalances(addr string, height *big.Int, contractAddr string) (common.Coins, error) {
	// for all the tokens the chain client has dealt with before
	tokens, err := c.evmScanner.GetTokens()
	if err != nil {
		return nil, fmt.Errorf("fail to get all the tokens: %w", err)
	}
	coins := common.Coins{}
	for _, token := range tokens {
		var balance *big.Int
		balance, err = c.GetBalance(addr, token.Address, height, contractAddr)
		if err != nil {
			c.logger.Err(err).Str("token", token.Address).Msg("fail to get balance for token")
			continue
		}
		asset := c.cfg.ChainID.GetGasAsset()
		if !strings.EqualFold(token.Address, evm.NativeTokenAddr) {
			asset, err = common.NewAsset(fmt.Sprintf("%s.%s-%s", c.GetChain(), token.Symbol, token.Address))
			if err != nil {
				return nil, err
			}
		}
		bal := c.evmScanner.tokenManager.ConvertAmount(token.Address, balance)
		coins = append(coins, common.NewCoin(asset, bal))
	}

	return coins.Distinct(), nil
}

// --------------------------------- gas ---------------------------------

// GetGasFee returns the gas fee based on the current gas price.
func (c *EVMClient) GetGasFee(gas uint64) common.Gas {
	return common.GetEVMGasFee(c.cfg.ChainID, c.GetGasPrice(), gas)
}

// GetGasPrice returns the current gas price.
func (c *EVMClient) GetGasPrice() *big.Int {
	gasPrice := c.evmScanner.GetGasPrice()
	return gasPrice
}

// --------------------------------- build transaction ---------------------------------

// getOutboundTxData generates the tx data and tx value of the outbound Router Contract call, and checks if the router contract has been updated
func (c *EVMClient) getOutboundTxData(txOutItem stypes.TxOutItem, memo mem.Memo, contractAddr common.Address) ([]byte, bool, *big.Int, error) {
	var data []byte
	var err error
	var tokenAddr string
	value := big.NewInt(0)
	evmValue := big.NewInt(0)
	hasRouterUpdated := false

	if len(txOutItem.Coins) == 1 {
		coin := txOutItem.Coins[0]
		tokenAddr = c.getTokenAddressFromAsset(coin.Asset)
		value = value.Add(value, coin.Amount.BigInt())
		value = c.evmScanner.tokenManager.ConvertSigningAmount(value, tokenAddr)
		if strings.EqualFold(tokenAddr, evm.NativeTokenAddr) {
			evmValue = value
		}
	}

	toAddr := ecommon.HexToAddress(txOutItem.ToAddress.String())

	switch memo.GetType() {
	case mem.TxOutbound, mem.TxRefund, mem.TxRagnarok:
		if txOutItem.Aggregator == "" {
			data, err = c.vaultABI.Pack("transferOut", toAddr, ecommon.HexToAddress(tokenAddr), value, txOutItem.Memo)
			if err != nil {
				return nil, hasRouterUpdated, nil, fmt.Errorf("fail to create data to call smart contract(transferOut): %w", err)
			}
		} else {
			memoType := memo.GetType()
			if memoType == mem.TxRefund || memoType == mem.TxRagnarok {
				return nil, hasRouterUpdated, nil, fmt.Errorf("%s can't use transferOutAndCall", memoType)
			}
			c.logger.Info().Msgf("aggregator target asset address: %s", txOutItem.AggregatorTargetAsset)
			if evmValue.Uint64() == 0 {
				return nil, hasRouterUpdated, nil, fmt.Errorf("transferOutAndCall can only be used when outbound asset is native")
			}
			targetLimit := txOutItem.AggregatorTargetLimit
			if targetLimit == nil {
				zeroLimit := cosmos.ZeroUint()
				targetLimit = &zeroLimit
			}
			aggAddr := ecommon.HexToAddress(txOutItem.Aggregator)
			targetAddr := ecommon.HexToAddress(txOutItem.AggregatorTargetAsset)
			// when address can't be round trip , the tx out item will be dropped
			if !strings.EqualFold(aggAddr.String(), txOutItem.Aggregator) {
				c.logger.Error().Msgf("aggregator address can't roundtrip , ignore tx (%s != %s)", txOutItem.Aggregator, aggAddr.String())
				return nil, hasRouterUpdated, nil, nil
			}
			if !strings.EqualFold(targetAddr.String(), txOutItem.AggregatorTargetAsset) {
				c.logger.Error().Msgf("aggregator target asset address can't roundtrip , ignore tx (%s != %s)", txOutItem.AggregatorTargetAsset, targetAddr.String())
				return nil, hasRouterUpdated, nil, nil
			}
			data, err = c.vaultABI.Pack("transferOutAndCall", aggAddr, targetAddr, toAddr, targetLimit.BigInt(), txOutItem.Memo)
			if err != nil {
				return nil, hasRouterUpdated, nil, fmt.Errorf("fail to create data to call smart contract(transferOutAndCall): %w", err)
			}
		}
	case mem.TxMigrate:
		if txOutItem.Aggregator != "" || txOutItem.AggregatorTargetAsset != "" {
			return nil, hasRouterUpdated, nil, fmt.Errorf("migration can't use aggregator")
		}
		if strings.EqualFold(tokenAddr, evm.NativeTokenAddr) {
			data, err = c.vaultABI.Pack("transferOut", toAddr, ecommon.HexToAddress(tokenAddr), value, txOutItem.Memo)
			if err != nil {
				return nil, hasRouterUpdated, nil, fmt.Errorf("fail to create data to call smart contract(transferOut): %w", err)
			}
		} else {
			newSmartContractAddr := c.getSmartContractByAddress(txOutItem.ToAddress)
			if newSmartContractAddr.IsEmpty() {
				return nil, hasRouterUpdated, nil, fmt.Errorf("fail to get new smart contract address")
			}
			data, err = c.vaultABI.Pack("transferAllowance", ecommon.HexToAddress(newSmartContractAddr.String()), toAddr, ecommon.HexToAddress(tokenAddr), value, txOutItem.Memo)
			if err != nil {
				return nil, hasRouterUpdated, nil, fmt.Errorf("fail to create data to call smart contract(transferAllowance): %w", err)
			}
		}
	}
	return data, hasRouterUpdated, evmValue, nil
}

func (c *EVMClient) buildOutboundTx(txOutItem stypes.TxOutItem, memo mem.Memo, nonce uint64) (*etypes.Transaction, error) {
	contractAddr := c.getSmartContractAddr(txOutItem.VaultPubKey)
	if contractAddr.IsEmpty() {
		// we may be churning from a vault that does not have a contract
		// try getting the toAddress (new vault) contract instead
		if memo.GetType() == mem.TxMigrate {
			contractAddr = c.getSmartContractByAddress(txOutItem.ToAddress)
		}
		if contractAddr.IsEmpty() {
			return nil, fmt.Errorf("can't sign tx, fail to get smart contract address")
		}
	}

	fromAddr, err := txOutItem.VaultPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("fail to get EVM address for pub key(%s): %w", txOutItem.VaultPubKey, err)
	}

	txData, _, evmValue, err := c.getOutboundTxData(txOutItem, memo, contractAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get outbound tx data %w", err)
	}
	if evmValue == nil {
		evmValue = cosmos.ZeroUint().BigInt()
	}

	// Convert TxOutItem gas rate units to Wei.
	_, gasRateUnitsPerOne := c.cfg.ChainID.GetGasUnits()
	toiGasRateWei := cosmos.NewUint(uint64(txOutItem.GasRate)).MulUint64(common.WeiPerOne).Quo(gasRateUnitsPerOne).BigInt()

	gasRate := c.GetGasPrice()
	if c.cfg.BlockScanner.FixedGasRate > 0 || gasRate.Cmp(big.NewInt(0)) == 0 {
		// if chain gas is zero we are still filling our gas price buffer, use outbound rate
		c.logger.Info().
			Stringer("toiGasRateWei", toiGasRateWei).
			Stringer("gasRate", gasRate).
			Msg("using gas rate from tx out item")
		gasRate = toiGasRateWei
	} else {
		// Thornode uses a gas rate 1.5x the reported network fee for the rate and computed
		// max gas to ensure the rate is sufficient when it is signed later. Since we now know
		// the more recent rate, we will use our current rate with a lower bound on 2/3 the
		// outbound rate (the original rate we reported to Thornode in the network fee).
		lowerBound := toiGasRateWei
		lowerBound.Mul(lowerBound, big.NewInt(2))
		lowerBound.Div(lowerBound, big.NewInt(3))

		// if the gas rate is less than the lower bound, use the lower bound
		if gasRate.Cmp(lowerBound) < 0 {
			c.logger.Info().
				Stringer("gasRate", gasRate).
				Stringer("lowerBound", lowerBound).
				Msg("gas rate is below lower bound, using lower bound")
			gasRate = lowerBound
		}
	}

	c.logger.Info().
		Stringer("inHash", txOutItem.InHash).
		Str("outboundRate", convertThorchainAmountToWei(big.NewInt(txOutItem.GasRate)).String()).
		Str("currentRate", c.GetGasPrice().String()).
		Str("effectiveRate", gasRate.String()).
		Msg("gas rate")

	// outbound tx always send to smart contract address
	estimatedEVMValue := big.NewInt(0)
	estimateTxData := txData
	if evmValue.Uint64() > 0 {
		// Use a small fixed value to estimate gas to avoid "insufficient fund" or "gas required
		// exceeds allowance" errors that can occur with the real value.
		estimatedEVMValue = estimatedEVMValue.SetInt64(21000)
		// The V6 router's transferOut requires msg.value == amount, so for transferOut calls
		// (non-aggregator native transfers) we must repack the call data with the matching
		// small value. transferOutAndCall doesn't have this check, so it uses txData as-is.
		if txOutItem.Aggregator == "" {
			toAddr := ecommon.HexToAddress(txOutItem.ToAddress.String())
			tokenAddr := c.getTokenAddressFromAsset(txOutItem.Coins[0].Asset)
			estimateTxData, err = c.vaultABI.Pack("transferOut", toAddr, ecommon.HexToAddress(tokenAddr), estimatedEVMValue, txOutItem.Memo)
			if err != nil {
				return nil, fmt.Errorf("fail to pack gas estimation transferOut: %w", err)
			}
		}
	}
	createdTx := etypes.NewTransaction(nonce, ecommon.HexToAddress(contractAddr.String()), estimatedEVMValue, c.cfg.BlockScanner.MaxGasLimit, gasRate, estimateTxData)
	estimatedGas, err := c.evmScanner.ethRpc.EstimateGas(fromAddr.String(), createdTx)
	if err != nil {
		// in an edge case that vault doesn't have enough fund to fulfill an outbound transaction , it will fail to estimate gas
		// the returned error is `execution reverted`
		// when this fail , chain client should skip the outbound and move on to the next. The network will reschedule the outbound
		// after 300 blocks
		c.logger.Err(err).Msg("fail to estimate gas")
		return nil, nil
	}

	scheduledMaxFee := big.NewInt(0)
	for _, coin := range txOutItem.MaxGas {
		scheduledMaxFee.Add(scheduledMaxFee, convertThorchainAmountToWei(coin.Amount.BigInt()))
	}

	if txOutItem.Aggregator != "" {
		var gasLimitForAggregator uint64
		gasLimitForAggregator, err = aggregators.FetchDexAggregatorGasLimit(
			c.cfg.ChainID, txOutItem.Aggregator,
		)
		if err != nil {
			c.logger.Err(err).
				Str("aggregator", txOutItem.Aggregator).
				Msg("fail to get aggregator gas limit, aborting to let thornode reschdule")
			return nil, nil
		}

		// if the estimate gas is over the max, abort and let thornode reschedule for now
		if estimatedGas > gasLimitForAggregator {
			c.logger.Warn().
				Stringer("in_hash", txOutItem.InHash).
				Uint64("estimated_gas", estimatedGas).
				Uint64("aggregator_gas_limit", gasLimitForAggregator).
				Msg("swap out gas limit exceeded, aborting to let thornode reschedule")
			return nil, nil
		}

		// set limit to aggregator gas limit
		estimatedGas = gasLimitForAggregator

		scheduledMaxFee = scheduledMaxFee.Mul(scheduledMaxFee, big.NewInt(c.cfg.EVM.AggregatorMaxGasMultiplier))
	} else if len(txOutItem.Coins) > 0 && !txOutItem.Coins[0].Asset.IsGasAsset() {
		scheduledMaxFee = scheduledMaxFee.Mul(scheduledMaxFee, big.NewInt(c.cfg.EVM.TokenMaxGasMultiplier))
	}

	// L2 chains require a small amount of gas asset left for the L1 fee
	if c.cfg.EVM.ExtraL1GasFee > 0 {
		l1Fee := big.NewInt(c.cfg.EVM.ExtraL1GasFee)
		scheduledMaxFee = scheduledMaxFee.Sub(scheduledMaxFee, convertThorchainAmountToWei(l1Fee))
	}

	// determine max gas units based on scheduled max gas (fee) and current rate
	maxGasUnits := new(big.Int).Div(scheduledMaxFee, gasRate).Uint64()

	// if estimated gas is more than the planned gas, abort and let thornode reschedule
	if estimatedGas > maxGasUnits {
		c.logger.Warn().
			Stringer("in_hash", txOutItem.InHash).
			Stringer("rate", gasRate).
			Uint64("estimated_gas_units", estimatedGas).
			Uint64("max_gas_units", maxGasUnits).
			Str("scheduled_max_fee", scheduledMaxFee.String()).
			Msg("max gas exceeded, aborting to let thornode reschedule")
		return nil, nil
	}

	createdTx = etypes.NewTransaction(
		nonce, ecommon.HexToAddress(contractAddr.String()), evmValue, maxGasUnits, gasRate, txData,
	)

	return createdTx, nil
}

// --------------------------------- sign ---------------------------------

// SignTx returns the signed transaction.
func (c *EVMClient) SignTx(tx stypes.TxOutItem, height int64) ([]byte, []byte, *stypes.TxInItem, error) {
	// Wait for liability reconstruction to complete before signing
	if !c.signingReady.Load() {
		return nil, nil, nil, fmt.Errorf("client still initializing, cannot sign yet")
	}

	if !tx.Chain.Equals(c.cfg.ChainID) {
		return nil, nil, nil, fmt.Errorf("chain %s is not support by evm chain client", tx.Chain)
	}

	if c.signerCacheManager.HasSigned(tx.CacheHash()) {
		c.logger.Info().Interface("tx", tx).Msg("transaction signed before, ignore")
		return nil, nil, nil, nil
	}

	if tx.ToAddress.IsEmpty() {
		return nil, nil, nil, fmt.Errorf("to address is empty")
	}
	if tx.VaultPubKey.IsEmpty() {
		return nil, nil, nil, fmt.Errorf("vault public key is empty")
	}
	if len(tx.Coins) == 0 {
		return nil, nil, nil, fmt.Errorf("coins is empty")
	}

	// GetMemo returns OriginalMemo for memoless outbounds
	// For truly memoless transactions (no Memo or OriginalMemo), default to TxOutbound
	memoForParsing := tx.GetMemo()
	var memo mem.Memo
	if memoForParsing == "" {
		memo = mem.NewOutboundMemo(tx.InHash)
	} else {
		var err error
		memo, err = mem.ParseMemo(common.LatestVersion, memoForParsing)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to parse memo(%s):%w", memoForParsing, err)
		}
		if memo.IsInbound() {
			return nil, nil, nil, fmt.Errorf("inbound memo should not be used for outbound tx")
		}
	}

	// the nonce is stored as the transaction checkpoint, if it is set deserialize it
	// so we only retry with the same nonce to avoid double spend
	var nonce uint64
	var fromAddr common.Address
	var err error
	fromAddr, err = tx.VaultPubKey.GetAddress(c.cfg.ChainID)
	if tx.Checkpoint != nil {
		if err = json.Unmarshal(tx.Checkpoint, &nonce); err != nil {
			return nil, nil, nil, fmt.Errorf("fail to unmarshal checkpoint: %w", err)
		}
		c.logger.Warn().Stringer("in_hash", tx.InHash).Uint64("nonce", nonce).Msg("using checkpoint nonce")
	} else {
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to get %s address for pub key(%s): %w", c.GetChain().String(), tx.VaultPubKey, err)
		}
		nonce, err = c.evmScanner.GetNonce(fromAddr.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to fetch account(%s) nonce: %w", fromAddr, err)
		}

		// abort signing if the pending nonce is too far in the future
		var finalizedNonce uint64
		finalizedNonce, err = c.evmScanner.GetNonceFinalized(fromAddr.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to fetch account(%s) finalized nonce: %w", fromAddr, err)
		}
		if (nonce - finalizedNonce) > c.cfg.MaxPendingNonces {
			c.logger.Warn().
				Uint64("nonce", nonce).
				Uint64("finalizedNonce", finalizedNonce).
				Msg("pending nonce too far in future")
			return nil, nil, nil, fmt.Errorf("pending nonce too far in future")
		}
	}

	// serialize nonce for later
	nonceBytes, err := json.Marshal(nonce)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to marshal nonce: %w", err)
	}

	// Check if allowance approval is needed (wrapped with mimir check)
	approvalTx, err := c.checkAndApproveAllowance(tx, nonce)
	if err != nil {
		c.logger.Err(err).Msg("fail to check allowance")
		return nil, nil, nil, err
	}

	// If approval transaction was created, sign and broadcast it before building the outbound.
	// The V6 router uses transferFrom for ERC20 transfers, which requires the vault to have
	// approved the router. The approval must be on-chain before gas estimation for the outbound
	// can succeed, so we broadcast it first and let the outbound be rescheduled if needed.
	if approvalTx != nil {
		// Sign approval transaction
		var approvalRawTx []byte
		approvalRawTx, err = c.sign(approvalTx, tx.VaultPubKey, height, tx)
		if err != nil {
			c.logger.Err(err).Msg("fail to sign approval transaction")
			return nil, nonceBytes, nil, fmt.Errorf("fail to sign approval transaction: %w", err)
		}

		// Broadcast approval transaction
		broadcastTx := &etypes.Transaction{}
		if err = broadcastTx.UnmarshalJSON(approvalRawTx); err != nil {
			c.logger.Err(err).Msg("fail to unmarshal approval tx")
			return nil, nonceBytes, nil, fmt.Errorf("fail to unmarshal approval transaction: %w", err)
		}

		ctx, cancel := c.getTimeoutContext()
		defer cancel()
		err = c.evmScanner.ethClient.SendTransaction(ctx, broadcastTx)
		if !isAcceptableError(err) {
			c.logger.Err(err).Str("hash", broadcastTx.Hash().String()).Msg("fail to broadcast approval transaction")
			return nil, nonceBytes, nil, fmt.Errorf("fail to broadcast approval transaction: %w", err)
		}

		tokenAddr := c.getTokenAddressFromAsset(tx.Coins[0].Asset)
		contractAddr := c.getSmartContractAddr(tx.VaultPubKey)

		c.logger.Info().
			Str("approval_txid", broadcastTx.Hash().String()).
			Str("token", tokenAddr).
			Str("router", contractAddr.String()).
			Msg("successfully broadcast approval transaction")

		// Update nonce past the approval tx
		nonce++

		// Update the checkpoint nonce so the rescheduled outbound uses the correct nonce
		nonceBytes, err = json.Marshal(nonce)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to marshal nonce: %w", err)
		}

		// Wait for the approval to be mined before estimating gas for the outbound.
		// The transferFrom will revert if the approval isn't on-chain yet. This only
		// happens once per vault per token, so the one-time delay is acceptable.
		// If the approval is still not mined in time, the outbound will be rescheduled.
		wait := time.Duration(c.cfg.ChainID.ApproximateBlockMilliseconds()*3) * time.Millisecond
		c.logger.Info().Dur("wait", wait).Msg("waiting for approval transaction to be mined")
		time.Sleep(wait)
	}

	outboundTx, err := c.buildOutboundTx(tx, memo, nonce)
	if err != nil {
		c.logger.Err(err).Msg("fail to build outbound tx")
		return nil, nil, nil, err
	}

	// if transaction is nil, abort to allow thornode reschedule
	if outboundTx == nil {
		return nil, nil, nil, nil
	}

	// before signing, confirm the vault has enough gas asset
	gasBalance, err := c.GetBalance(fromAddr.String(), evm.NativeTokenAddr, nil, "")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to get gas asset balance: %w", err)
	}

	// Subtract pending liabilities (signed but unconfirmed transactions) from the balance
	confirmedNonce, _ := c.evmScanner.GetNonceFinalized(fromAddr.String())
	pendingLiability := c.getPendingLiability(fromAddr.String(), confirmedNonce)
	effectiveBalance := new(big.Int).Sub(gasBalance, pendingLiability)

	// Calculate required balance (value + estimated fee)
	evmValue := outboundTx.Value()
	estimatedFee := new(big.Int).Mul(big.NewInt(int64(outboundTx.Gas())), outboundTx.GasPrice())
	requiredBalance := new(big.Int).Add(evmValue, estimatedFee)

	if effectiveBalance.Cmp(requiredBalance) < 0 {
		return nil, nil, nil, fmt.Errorf("insufficient gas asset balance: effective %s (latest %s - liability %s) < required %s",
			effectiveBalance.String(), gasBalance.String(), pendingLiability.String(), requiredBalance.String())
	}

	// Declare rawTx at function level for proper scoping
	var rawTx []byte

	// Sign main transaction
	rawTx, err = c.sign(outboundTx, tx.VaultPubKey, height, tx)
	if err != nil || len(rawTx) == 0 {
		return nil, nonceBytes, nil, fmt.Errorf("fail to sign message: %w", err)
	}

	// Record the liability for this signed transaction (value + estimated fee)
	c.recordLiability(fromAddr.String(), nonce, requiredBalance)

	// create the observation to be sent by the signer before broadcast
	chainHeight, err := c.GetHeight()
	if err != nil { // fall back to the scanner height, thornode voter does not use height
		chainHeight = c.evmScanner.getCurrentBlockHeight()
	}

	coin := tx.Coins[0]
	gas := common.MakeEVMGas(c.GetChain(), outboundTx.GasPrice(), outboundTx.Gas(), nil)
	// This is the maximum gas, using the gas limit for instant-observation
	// rather than the GasUsed which can only be gotten from the receipt when scanning.

	signedTx := &etypes.Transaction{}
	if err = signedTx.UnmarshalJSON(rawTx); err != nil {
		return nil, rawTx, nil, fmt.Errorf("fail to unmarshal signed tx: %w", err)
	}

	txIn := stypes.NewTxInItem(
		chainHeight,
		signedTx.Hash().Hex()[2:],
		tx.Memo,
		fromAddr.String(),
		tx.ToAddress.String(),
		common.NewCoins(
			coin,
		),
		gas,
		tx.VaultPubKey,
		"",
		"",
		nil,
	)

	return rawTx, nonceBytes, txIn, nil
}

// checkAndApproveAllowance checks if token allowance is sufficient and creates an approval transaction if needed
func (c *EVMClient) checkAndApproveAllowance(tx stypes.TxOutItem, nonce uint64) (*etypes.Transaction, error) {
	// Check if mimir is enabled for allowance checks
	allowanceCheckEnabled, err := c.bridge.GetMimir(fmt.Sprintf(constants.MimirTemplateEVMAllowanceCheck, c.cfg.ChainID))
	if err != nil {
		c.logger.Err(err).Msg("fail to get EVMAllowanceCheck mimir")
		return nil, nil // Continue without approval check if mimir lookup fails
	}

	// If mimir is not set or is 0, skip allowance check
	if allowanceCheckEnabled <= 0 {
		return nil, nil
	}

	// Only check allowance for single token transfers (not native asset transfers)
	if len(tx.Coins) == 0 {
		return nil, nil
	} else if len(tx.Coins) > 1 {
		return nil, errors.New("evm token tx cannot have more than 1 coin")
	}

	coin := tx.Coins[0]
	tokenAddr := c.getTokenAddressFromAsset(coin.Asset)

	// Skip native token transfers (ETH, BNB, etc.) - they don't need approval
	if strings.EqualFold(tokenAddr, evm.NativeTokenAddr) {
		return nil, nil
	}

	// Get vault and router addresses
	vaultAddr, err := tx.VaultPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		c.logger.Err(err).Msg("fail to get vault address")
		return nil, errors.New("fail to get vault address")
	}

	contractAddr := c.getSmartContractAddr(tx.VaultPubKey)
	if contractAddr.IsEmpty() {
		c.logger.Debug().Msg("no router contract found, skipping allowance check")
		return nil, errors.New("no router contract found for vault")
	}

	// Check current allowance: erc20.allowance(vault, router)
	// This checks how much the router is currently allowed to spend from the vault's token balance
	allowanceCallData, err := c.evmScanner.erc20ABI.Pack("allowance", ecommon.HexToAddress(vaultAddr.String()), ecommon.HexToAddress(contractAddr.String()))
	if err != nil {
		c.logger.Err(err).Msg("fail to pack allowance call data")
		return nil, errors.New("fail to pack allowance call data")
	}

	// Call allowance function on the token contract
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	tokenContractAddr := ecommon.HexToAddress(tokenAddr)
	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &tokenContractAddr,
		Data: allowanceCallData,
	}, nil)
	if err != nil {
		c.logger.Err(err).Str("token", tokenAddr).Msg("fail to call allowance function")
		return nil, errors.New("fail to call allowance function on token contract")
	}

	// Unpack allowance result
	allowanceResult, err := c.evmScanner.erc20ABI.Unpack("allowance", result)
	if err != nil {
		c.logger.Err(err).Msg("fail to unpack allowance result")
		return nil, errors.New("fail to unpack allowance result")
	}

	if len(allowanceResult) == 0 {
		c.logger.Debug().Msg("empty allowance result")
		return nil, errors.New("empty allowance result")
	}

	currentAllowance, ok := allowanceResult[0].(*big.Int)
	if !ok {
		c.logger.Error().Msg("invalid allowance result type")
		return nil, errors.New("invalid allowance result type")
	}

	// Convert coin amount to proper decimals for comparison
	// THORChain stores all amounts in 1e8 format, but ERC20 tokens can have 3-18 decimals
	// ConvertSigningAmount handles the decimal conversion based on the token's actual decimals
	transferAmount := coin.Amount.BigInt()
	transferAmount = c.evmScanner.tokenManager.ConvertSigningAmount(transferAmount, tokenAddr)

	// Check if current allowance is sufficient
	if currentAllowance.Cmp(transferAmount) >= 0 {
		// Allowance is sufficient, no approval needed
		c.logger.Debug().
			Str("token", tokenAddr).
			Str("currentAllowance", currentAllowance.String()).
			Str("requiredAmount", transferAmount.String()).
			Msg("allowance sufficient, no approval needed")
		return nil, nil
	}

	c.logger.Info().
		Str("token", tokenAddr).
		Str("vault", vaultAddr.String()).
		Str("router", contractAddr.String()).
		Str("currentAllowance", currentAllowance.String()).
		Str("requiredAmount", transferAmount.String()).
		Msg("insufficient allowance, creating approval transaction")

	// Create approval transaction for maximum uint256 value to minimize future approvals
	// erc20.approve(router, maxUint256): vault approves router to spend max amount
	maxUint256 := new(big.Int)
	maxUint256.Sub(maxUint256.Lsh(big.NewInt(1), 256), big.NewInt(1))

	approvalData, err := c.evmScanner.erc20ABI.Pack("approve", ecommon.HexToAddress(contractAddr.String()), maxUint256)
	if err != nil {
		return nil, fmt.Errorf("fail to pack approval data: %w", err)
	}

	// Use the same gas price calculation as the main transaction
	_, gasRateUnitsPerOne := c.cfg.ChainID.GetGasUnits()
	toiGasRateWei := cosmos.NewUint(uint64(tx.GasRate)).MulUint64(common.WeiPerOne).Quo(gasRateUnitsPerOne).BigInt()

	gasPrice := c.GetGasPrice()
	if c.cfg.BlockScanner.FixedGasRate > 0 || gasPrice.Cmp(big.NewInt(0)) == 0 {
		gasPrice = toiGasRateWei
	} else {
		// Apply same gas price logic as main transaction
		lowerBound := toiGasRateWei
		lowerBound.Mul(lowerBound, big.NewInt(2))
		lowerBound.Div(lowerBound, big.NewInt(3))

		gasPrice.Div(gasPrice, big.NewInt(common.One*100))
		if gasPrice.Cmp(big.NewInt(0)) == 0 {
			gasPrice = big.NewInt(1)
		}
		gasPrice.Mul(gasPrice, big.NewInt(common.One*100))

		if gasPrice.Cmp(lowerBound) < 0 {
			gasPrice = lowerBound
		}
	}

	// Avoid transaction hash collision between chains in mocknet (same chain id)
	gasPrice.Add(gasPrice, c.evmScanner.ApprovalTxRateDustWei())

	// Use the same gas limit as other non-aggregator transactions
	approvalGasLimit := c.cfg.BlockScanner.MaxGasLimit

	// Create approval transaction
	approvalTx := etypes.NewTransaction(
		nonce,
		ecommon.HexToAddress(tokenAddr),
		big.NewInt(0), // No ETH value for ERC20 approval
		approvalGasLimit,
		gasPrice,
		approvalData,
	)

	c.logger.Info().
		Str("token", tokenAddr).
		Str("vault", vaultAddr.String()).
		Str("router", contractAddr.String()).
		Uint64("nonce", nonce).
		Uint64("gasLimit", approvalGasLimit).
		Str("gasPrice", gasPrice.String()).
		Msg("created approval transaction")

	return approvalTx, nil
}

// sign is design to sign a given message with keysign party and keysign wrapper
func (c *EVMClient) sign(tx *etypes.Transaction, poolPubKey common.PubKey, height int64, txOutItem stypes.TxOutItem) ([]byte, error) {
	rawBytes, err := c.kw.Sign(tx, poolPubKey)
	if err == nil && rawBytes != nil {
		return rawBytes, nil
	}
	var keysignError tss.KeysignError
	if errors.As(err, &keysignError) {
		if len(keysignError.Blame.BlameNodes) == 0 {
			// TSS doesn't know which node to blame
			return nil, fmt.Errorf("fail to sign tx: %w", err)
		}
		// key sign error forward the keysign blame to thorchain
		txID, errPostKeysignFail := c.bridge.PostKeysignFailure(keysignError.Blame, height, txOutItem.Memo, txOutItem.Coins, txOutItem.VaultPubKey)
		if errPostKeysignFail != nil {
			return nil, multierror.Append(err, errPostKeysignFail)
		}
		c.logger.Info().Str("tx_id", txID.String()).Msg("post keysign failure to thorchain")
	}
	return nil, fmt.Errorf("fail to sign tx: %w", err)
}

// --------------------------------- broadcast ---------------------------------

// BroadcastTx broadcasts the transaction and returns the transaction hash.
func (c *EVMClient) BroadcastTx(txOutItem stypes.TxOutItem, hexTx []byte) (string, error) {
	// decode the transaction
	tx := &etypes.Transaction{}
	if err := tx.UnmarshalJSON(hexTx); err != nil {
		return "", err
	}
	txID := tx.Hash().String()

	// get context with default timeout
	ctx, cancel := c.getTimeoutContext()
	defer cancel()

	// send the transaction
	if err := c.ethClient.SendTransaction(ctx, tx); !isAcceptableError(err) {
		c.logger.Error().Str("txid", txID).Err(err).Msg("failed to send transaction")
		return "", err
	}
	c.logger.Info().Str("memo", txOutItem.Memo).Str("txid", txID).Msg("broadcast tx")

	// update the signer cache
	if err := c.signerCacheManager.SetSigned(txOutItem.CacheHash(), txOutItem.CacheVault(c.GetChain()), txID); err != nil {
		c.logger.Err(err).Interface("txOutItem", txOutItem).Msg("fail to mark tx out item as signed")
	}

	blockHeight, err := c.bridge.GetBlockHeight()
	if err != nil {
		c.logger.Err(err).Msg("fail to get current THORChain block height")
		// at this point , the tx already broadcast successfully , don't return an error
		// otherwise will cause the same tx to retry
	} else if err = c.AddSignedTxItem(txID, blockHeight, txOutItem.VaultPubKey.String(), &txOutItem); err != nil {
		c.logger.Err(err).Str("hash", txID).Msg("fail to add signed tx item")
	}

	return txID, nil
}

// --------------------------------- observe ---------------------------------

// OnObservedTxIn is called when a new observed tx is received.
func (c *EVMClient) OnObservedTxIn(txIn stypes.TxInItem, blockHeight int64) {
	m, err := mem.ParseMemo(common.LatestVersion, txIn.Memo)
	if err != nil {
		// Debug log only as ParseMemo error is expected for THORName inbounds.
		c.logger.Debug().Err(err).Str("memo", txIn.Memo).Msg("fail to parse memo")
		return
	}
	if !m.IsOutbound() {
		return
	}
	if m.GetTxID().IsEmpty() {
		return
	}
	if err = c.signerCacheManager.SetSigned(txIn.CacheHash(c.GetChain(), m.GetTxID().String()), txIn.CacheVault(c.GetChain()), txIn.Tx); err != nil {
		c.logger.Err(err).Msg("fail to update signer cache")
	}
}

// GetConfirmationCount returns the confirmation count for the given tx.
func (c *EVMClient) GetConfirmationCount(txIn stypes.TxIn) int64 {
	switch c.cfg.ChainID {
	case common.AVAXChain: // instant finality
		return 0
	case common.BASEChain:
		return 12 // ~2 Ethereum blocks for parity with the 2 block minimum in eth client
	case common.BSCChain:
		return 3 // round up from 2.5 blocks required for finality
	case common.POLChain:
		return 15
	default:
		c.logger.Fatal().Msgf("unsupported chain: %s", c.cfg.ChainID)
		return 0
	}
}

// ConfirmationCountReady returns true if the confirmation count is ready.
func (c *EVMClient) ConfirmationCountReady(txIn stypes.TxIn) bool {
	switch c.cfg.ChainID {
	case common.AVAXChain: // instant finality
		return true
	case common.BSCChain, common.POLChain:
		if len(txIn.TxArray) == 0 {
			return true
		}
		blockHeight := txIn.TxArray[0].BlockHeight
		confirm := txIn.ConfirmationRequired
		c.logger.Info().Msgf("confirmation required: %d", confirm)
		return (c.evmScanner.getCurrentBlockHeight() - blockHeight) >= confirm
	case common.BASEChain:
		// block is already finalized(settled to l1)
		return true
	default:
		c.logger.Fatal().Msgf("unsupported chain: %s", c.cfg.ChainID)
		return false
	}
}

// --------------------------------- solvency ---------------------------------

// ReportSolvency reports solvency once per configured solvency blocks.
func (c *EVMClient) ReportSolvency(height int64) error {
	if !c.ShouldReportSolvency(height) {
		return nil
	}

	// when block scanner is not healthy, only report from auto-unhalt SolvencyCheckRunner
	// (FetchTxs passes currentBlockHeight, while SolvencyCheckRunner passes chainHeight)
	if !c.IsBlockScannerHealthy() && height == c.evmScanner.getCurrentBlockHeight() {
		return nil
	}

	// fetch all asgard vaults
	asgardVaults, err := c.bridge.GetAsgards()
	if err != nil {
		return fmt.Errorf("fail to get asgards, err: %w", err)
	}

	// 3x MaxGas breathing room, from gas rate units gas price to THORChain (1e8) format
	_, gasRateUnitsPerOne := c.cfg.ChainID.GetGasUnits()
	currentGasFee := cosmos.NewUint(c.cfg.BlockScanner.MaxGasLimit).MulUint64(3).MulUint64(c.evmScanner.lastReportedGasPrice).MulUint64(common.One).Quo(gasRateUnitsPerOne)

	// report insolvent asgard vaults,
	// or else all if the chain is halted and all are solvent
	chainVaults := asgardVaults[:0]
	for _, asgard := range asgardVaults {
		if asgard.HasFundsForChain(c.cfg.ChainID) {
			chainVaults = append(chainVaults, asgard)
		}
	}

	msgs := make([]stypes.Solvency, 0, len(chainVaults))
	solventMsgs := make([]stypes.Solvency, 0, len(chainVaults))
	processedVaults := 0
	for i := range chainVaults {
		var acct common.Account
		acct, err = c.GetAccount(chainVaults[i].PubKey, new(big.Int).SetInt64(height))
		if err != nil {
			c.logger.Err(err).Msg("fail to get account balance")
			continue
		}

		msg := stypes.Solvency{
			Height: height,
			Chain:  c.cfg.ChainID,
			PubKey: chainVaults[i].PubKey,
			Coins:  acct.Coins,
		}
		processedVaults++

		if runners.IsVaultSolvent(c.cfg.ChainID, acct, chainVaults[i], currentGasFee) {
			solventMsgs = append(solventMsgs, msg) // Solvent-vault message
			continue
		}
		msgs = append(msgs, msg) // Insolvent-vault message
	}

	// Only if the block scanner is unhealthy (e.g. solvency-halted) and all chain-specific vaults are solvent,
	// report that all the chain-specific vaults are solvent.
	// If there are any insolvent vaults, report only them.
	// Not reporting both solvent and insolvent vaults is to avoid noise (spam):
	// Reporting both could halt-and-unhalt SolvencyHalt in the same THOR block
	// (resetting its height), plus making it harder to know at a glance from solvency reports which vaults were insolvent.
	solvent := false
	allVaultsProcessed := len(chainVaults) > 0 && processedVaults == len(chainVaults)
	if !c.IsBlockScannerHealthy() && allVaultsProcessed && len(solventMsgs) == processedVaults {
		msgs = solventMsgs
		solvent = true
	}

	for i := range msgs {
		c.logger.Info().
			Stringer("asgard", msgs[i].PubKey).
			Interface("coins", msgs[i].Coins).
			Bool("solvent", solvent).
			Msg("reporting solvency")

		// send solvency to thorchain via global queue consumed by the observer
		select {
		case c.globalSolvencyQueue <- msgs[i]:
		case <-time.After(constants.ThorchainBlockTime):
			c.logger.Info().Msg("fail to send solvency info to thorchain, timeout")
		}
	}
	c.lastSolvencyCheckHeight = height
	return nil
}

// ShouldReportSolvency returns true if the given height is a solvency report height.
func (c *EVMClient) ShouldReportSolvency(height int64) bool {
	return height%c.cfg.SolvencyBlocks == 0
}

// --------------------------------- helpers ---------------------------------

func (c *EVMClient) getTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.cfg.BlockScanner.HTTPRequestTimeout)
}
