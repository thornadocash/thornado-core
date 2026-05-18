package ethereum

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ecommon "github.com/ethereum/go-ethereum/common"
	ecore "github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/txpool"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/runners"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/utxo"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"

	tssp "gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/tss"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/aggregators"
	mem "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
)

const (
	ethBlockRewardAndFee = 3 * 1e18
)

// Client is a structure to sign and broadcast tx to Ethereum chain used by signer mostly
type Client struct {
	logger                  zerolog.Logger
	cfg                     config.BifrostChainConfiguration
	localPubKey             common.PubKey
	client                  *ethclient.Client
	chainID                 *big.Int
	kw                      *evm.KeySignWrapper
	ethScanner              *ETHScanner
	bridge                  thorclient.ThorchainBridge
	blockScanner            *blockscanner.BlockScanner
	vaultABI                *abi.ABI
	pubkeyMgr               pubkeymanager.PubKeyValidator
	poolMgr                 thorclient.PoolManager
	asgardCache             atomic.Pointer[utxo.AsgardCache]
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

// NewClient create new instance of Ethereum client
func NewClient(thorKeys *thorclient.Keys,
	cfg config.BifrostChainConfiguration,
	server *tssp.TssServer,
	bridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	poolMgr thorclient.PoolManager,
) (*Client, error) {
	if thorKeys == nil {
		return nil, fmt.Errorf("fail to create ETH client,thor keys is empty")
	}
	tssKm, err := tss.NewKeySign(server, bridge)
	if err != nil {
		return nil, fmt.Errorf("fail to create tss signer: %w", err)
	}

	priv, err := thorKeys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get private key: %w", err)
	}

	temp, err := codec.ToCmtPubKeyInterface(priv.PubKey())
	if err != nil {
		return nil, fmt.Errorf("fail to get tm pub key: %w", err)
	}
	pk, err := common.NewPubKeyFromCrypto(temp)
	if err != nil {
		return nil, fmt.Errorf("fail to get pub key: %w", err)
	}

	if bridge == nil {
		return nil, errors.New("THORChain bridge is nil")
	}
	if pubkeyMgr == nil {
		return nil, errors.New("pubkey manager is nil")
	}
	if poolMgr == nil {
		return nil, errors.New("pool manager is nil")
	}
	ethPrivateKey, err := evm.GetPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	ethClient, err := ethclient.Dial(cfg.RPCHost)
	if err != nil {
		return nil, fmt.Errorf("fail to dial ETH rpc host(%s): %w", cfg.RPCHost, err)
	}
	chainID, err := getChainID(ethClient, cfg.BlockScanner.HTTPRequestTimeout)
	if err != nil {
		return nil, err
	}

	keysignWrapper, err := evm.NewKeySignWrapper(ethPrivateKey, pk, tssKm, chainID, "ETH")
	if err != nil {
		return nil, fmt.Errorf("fail to create ETH key sign wrapper: %w", err)
	}
	vaultABI, _, err := evm.GetContractABI(routerContractABI, erc20ContractABI)
	if err != nil {
		return nil, fmt.Errorf("fail to get contract abi: %w", err)
	}
	c := &Client{
		logger:             log.With().Str("module", "ethereum").Logger(),
		cfg:                cfg,
		client:             ethClient,
		chainID:            chainID,
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

	c.logger.Info().Msgf("current chain id: %d", chainID.Uint64())
	if chainID.Uint64() == 0 {
		return nil, fmt.Errorf("chain id is: %d , invalid", chainID.Uint64())
	}
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
	c.ethScanner, err = NewETHScanner(c.cfg.BlockScanner, storage, chainID, c.client, c.bridge, m, pubkeyMgr, c.ReportSolvency, signerCacheManager)
	if err != nil {
		return c, fmt.Errorf("fail to create eth block scanner: %w", err)
	}

	c.blockScanner, err = blockscanner.NewBlockScanner(c.cfg.BlockScanner, storage, m, c.bridge, c.ethScanner)
	if err != nil {
		return c, fmt.Errorf("fail to create block scanner: %w", err)
	}
	localNodeETHAddress, err := c.localPubKey.GetAddress(common.ETHChain)
	if err != nil {
		c.logger.Err(err).Msg("fail to get local node's ETH address")
	}
	c.logger.Info().Msgf("local node ETH address %s", localNodeETHAddress)

	return c, nil
}

// getPendingLiability returns the sum of all pending (signed but unconfirmed) liabilities
// for the given vault address, and prunes entries for nonces that have already been confirmed.
func (c *Client) getPendingLiability(vaultAddr string, confirmedNonce uint64) *big.Int {
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
func (c *Client) recordLiability(vaultAddr string, nonce uint64, amount *big.Int) {
	c.liabilityMu.Lock()
	defer c.liabilityMu.Unlock()

	if c.pendingLiabilities[vaultAddr] == nil {
		c.pendingLiabilities[vaultAddr] = make(map[uint64]*big.Int)
	}
	c.pendingLiabilities[vaultAddr][nonce] = amount
}

// IsETH return true if the token address equals to ethToken address
func IsETH(token string) bool {
	return strings.EqualFold(token, ethToken)
}

// Start to monitor Ethereum block chain
func (c *Client) Start(globalTxsQueue chan stypes.TxIn, globalErrataQueue chan stypes.ErrataBlock, globalSolvencyQueue chan stypes.Solvency, globalNetworkFeeQueue chan common.NetworkFee) {
	c.ethScanner.globalErrataQueue = globalErrataQueue
	c.ethScanner.globalNetworkFeeQueue = globalNetworkFeeQueue
	c.globalSolvencyQueue = globalSolvencyQueue
	c.tssKeySigner.Start()
	c.blockScanner.Start(globalTxsQueue, globalNetworkFeeQueue)
	c.wg.Add(1)
	go c.unstuck()
	c.wg.Add(1)
	go runners.SolvencyCheckRunner(c.GetChain(), c, c.bridge, c.stopchan, c.wg, constants.ThorchainBlockTime)
	c.wg.Add(1)
	go c.reconstructLiabilities()
}

// reconstructLiabilities scans the pending block on startup to reconstruct
// in-memory liability tracking for transactions that were signed before a restart.
// It sets signingReady to true once complete, allowing SignTx to proceed.
func (c *Client) reconstructLiabilities() {
	defer c.wg.Done()

	// Get all vault addresses this node is a signer for
	vaultAddresses := make([]string, 0)
	for _, pk := range c.pubkeyMgr.GetAlgoPubKeys(common.SigningAlgoSecp256k1, true) {
		addr, err := pk.GetAddress(common.ETHChain)
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

	for _, vaultAddr := range vaultAddresses {
		reconWg.Add(1)
		sem <- struct{}{}
		go func(vAddr string) {
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
func (c *Client) reconstructVaultLiability(vaultAddr string) (bool, error) {
	ctx, cancel := c.getContext()
	defer cancel()

	addr := ecommon.HexToAddress(vaultAddr)

	// Get confirmed and pending nonces
	confirmedNonce, err := c.client.NonceAt(ctx, addr, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get confirmed nonce: %w", err)
	}

	ctx2, cancel2 := c.getContext()
	defer cancel2()
	pendingNonce, err := c.client.PendingNonceAt(ctx2, addr)
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
	ctx3, cancel3 := c.getContext()
	defer cancel3()
	pendingBlock, err := c.client.BlockByNumber(ctx3, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get pending block: %w", err)
	}

	foundCount := uint64(0)
	for _, tx := range pendingBlock.Transactions() {
		// Get sender address
		signer := etypes.LatestSignerForChainID(c.chainID)
		sender, err := etypes.Sender(signer, tx)
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

// Stop ETH client
func (c *Client) Stop() {
	c.tssKeySigner.Stop()
	c.blockScanner.Stop()
	c.client.Close()
	close(c.stopchan)
	c.wg.Wait()
}

func (c *Client) IsBlockScannerHealthy() bool {
	return c.blockScanner.IsHealthy()
}

// GetConfig return the configurations used by ETH chain
func (c *Client) GetConfig() config.BifrostChainConfiguration {
	return c.cfg
}

func (c *Client) getContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.cfg.BlockScanner.HTTPRequestTimeout)
}

// getChainID retrieve the chain id from ETH node, and determinate whether we are running on test net by checking the status
// when it failed to get chain id , it will assume LocalNet
func getChainID(client *ethclient.Client, timeout time.Duration) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get chain id ,err: %w", err)
	}
	return chainID, err
}

// GetChain get chain
func (c *Client) GetChain() common.Chain {
	return common.ETHChain
}

// GetHeight gets height from eth scanner
func (c *Client) GetHeight() (int64, error) {
	return c.ethScanner.GetHeight()
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
	lastObserved, err := c.signerCacheManager.GetLatestRecordedTx(stypes.InboundCacheKey(vault, c.GetChain().String()))
	if err != nil {
		return "", "", err
	}
	lastBroadCasted, err := c.signerCacheManager.GetLatestRecordedTx(stypes.BroadcastCacheKey(vault, c.GetChain().String()))
	return lastObserved, lastBroadCasted, err
}

// GetAddress return current signer address, it will be bech32 encoded address
func (c *Client) GetAddress(poolPubKey common.PubKey) string {
	addr, err := poolPubKey.GetAddress(common.ETHChain)
	if err != nil {
		c.logger.Error().Err(err).Str("pool_pub_key", poolPubKey.String()).Msg("fail to get pool address")
		return ""
	}
	return addr.String()
}

// GetGasFee gets gas fee
func (c *Client) GetGasFee(gas uint64) common.Gas {
	return common.GetEVMGasFee(common.ETHChain, c.GetGasPrice(), gas)
}

// GetGasPrice gets gas price from eth scanner
func (c *Client) GetGasPrice() *big.Int {
	gasPrice := c.ethScanner.GetGasPrice()
	return gasPrice
}

// estimateGas estimates gas for tx
func (c *Client) estimateGas(from string, tx *etypes.Transaction) (uint64, error) {
	ctx, cancel := c.getContext()
	defer cancel()
	return c.client.EstimateGas(ctx, ethereum.CallMsg{
		From:     ecommon.HexToAddress(from),
		To:       tx.To(),
		GasPrice: tx.GasPrice(),
		// Gas:      tx.Gas(),
		Value: tx.Value(),
		Data:  tx.Data(),
	})
}

// GetNonce returns the nonce (including pending) for the given address.
func (c *Client) GetNonce(addr string) (uint64, error) {
	ctx, cancel := c.getContext()
	defer cancel()
	nonce, err := c.client.PendingNonceAt(ctx, ecommon.HexToAddress(addr))
	if err != nil {
		return 0, fmt.Errorf("fail to get account nonce: %w", err)
	}
	return nonce, nil
}

// GetNonceFinalized returns the nonce for the given address.
func (c *Client) GetNonceFinalized(addr string) (uint64, error) {
	ctx, cancel := c.getContext()
	defer cancel()
	return c.client.NonceAt(ctx, ecommon.HexToAddress(addr), nil)
}

func getTokenAddressFromAsset(asset common.Asset) string {
	if asset.Equals(common.ETHAsset) {
		return ethToken
	}
	allParts := strings.Split(asset.Symbol.String(), "-")
	return allParts[len(allParts)-1]
}

func (c *Client) getSmartContractAddr(pubkey common.PubKey) common.Address {
	return c.pubkeyMgr.GetContract(common.ETHChain, pubkey)
}

func (c *Client) getSmartContractByAddress(addr common.Address) common.Address {
	for _, pk := range c.pubkeyMgr.GetAlgoPubKeys(common.SigningAlgoSecp256k1, true) {
		evmAddr, err := pk.GetAddress(common.ETHChain)
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
			return c.pubkeyMgr.GetContract(common.ETHChain, pk)
		}
	}
	return common.NoAddress
}

func (c *Client) convertSigningAmount(amt *big.Int, token string) *big.Int {
	// convert 1e8 to 1e18
	amt = c.convertThorchainAmountToWei(amt)
	if IsETH(token) {
		return amt
	}
	tm, err := c.ethScanner.getTokenMeta(token)
	if err != nil {
		c.logger.Err(err).Msgf("fail to get token meta for token: %s", token)
		return amt
	}

	if tm.Decimal == defaultDecimals {
		// when the smart contract is using 1e18 as decimals , that means is based on WEI
		// thus the input amt is correct amount to send out
		return amt
	}
	var value big.Int
	amt = amt.Mul(amt, value.Exp(big.NewInt(10), big.NewInt(int64(tm.Decimal)), nil))
	amt = amt.Div(amt, value.Exp(big.NewInt(10), big.NewInt(defaultDecimals), nil))
	return amt
}

func (c *Client) convertThorchainAmountToWei(amt *big.Int) *big.Int {
	return big.NewInt(0).Mul(amt, big.NewInt(common.One*100))
}

// SignTx sign the the given TxArrayItem
func (c *Client) SignTx(tx stypes.TxOutItem, height int64) ([]byte, []byte, *stypes.TxInItem, error) {
	// Wait for liability reconstruction to complete before signing
	if !c.signingReady.Load() {
		return nil, nil, nil, fmt.Errorf("client still initializing, cannot sign yet")
	}

	if !tx.Chain.Equals(common.ETHChain) {
		return nil, nil, nil, fmt.Errorf("chain %s is not support by ETH chain client", tx.Chain)
	}

	if c.signerCacheManager.HasSigned(tx.CacheHash()) {
		c.logger.Info().Msgf("transaction(%+v), signed before , ignore", tx)
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
	var memoType mem.TxType = mem.TxOutbound // default to outbound for memoless
	if memoForParsing != "" {
		memo, err := mem.ParseMemo(common.LatestVersion, memoForParsing)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to parse memo(%s):%w", memoForParsing, err)
		}
		if memo.IsInbound() {
			return nil, nil, nil, fmt.Errorf("inbound memo should not be used for outbound tx")
		}
		memoType = memo.GetType()
	}

	contractAddr := c.getSmartContractAddr(tx.VaultPubKey)
	if contractAddr.IsEmpty() {
		return nil, nil, nil, fmt.Errorf("can't sign tx , fail to get smart contract address")
	}

	value := big.NewInt(0)
	ethValue := big.NewInt(0)
	var tokenAddr string
	if len(tx.Coins) == 1 {
		coin := tx.Coins[0]
		tokenAddr = getTokenAddressFromAsset(coin.Asset)
		value = value.Add(value, coin.Amount.BigInt())
		value = c.convertSigningAmount(value, tokenAddr)
		if IsETH(tokenAddr) {
			ethValue = value
		}
	}

	fromAddr, err := tx.VaultPubKey.GetAddress(common.ETHChain)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to get ETH address for pub key(%s): %w", tx.VaultPubKey, err)
	}

	dest := ecommon.HexToAddress(tx.ToAddress.String())
	var data []byte

	switch memoType {
	case mem.TxOutbound, mem.TxRefund, mem.TxRagnarok:
		if tx.Aggregator == "" {
			data, err = c.vaultABI.Pack("transferOut", dest, ecommon.HexToAddress(tokenAddr), value, tx.Memo)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("fail to create data to call smart contract(transferOut): %w", err)
			}
		} else {
			if memoType == mem.TxRefund || memoType == mem.TxRagnarok {
				return nil, nil, nil, fmt.Errorf("%s can't use transferOutAndCall", memoType)
			}
			c.logger.Info().Msgf("aggregator target address: %s", tx.AggregatorTargetAsset)
			if ethValue.Uint64() == 0 {
				return nil, nil, nil, fmt.Errorf("transferOutAndCall can only be used when outbound asset is ETH")
			}
			targetLimit := tx.AggregatorTargetLimit
			if targetLimit == nil {
				zeroLimit := cosmos.ZeroUint()
				targetLimit = &zeroLimit
			}
			aggAddr := ecommon.HexToAddress(tx.Aggregator)
			targetAddr := ecommon.HexToAddress(tx.AggregatorTargetAsset)
			// when address can't be round trip , the tx out item will be dropped
			if !strings.EqualFold(aggAddr.String(), tx.Aggregator) {
				c.logger.Error().Msgf("aggregator address can't roundtrip , ignore tx (%s != %s)", tx.Aggregator, aggAddr.String())
				return nil, nil, nil, nil
			}
			if !strings.EqualFold(targetAddr.String(), tx.AggregatorTargetAsset) {
				c.logger.Error().Msgf("aggregator target asset address can't roundtrip , ignore tx (%s != %s)", tx.AggregatorTargetAsset, targetAddr.String())
				return nil, nil, nil, nil
			}
			data, err = c.vaultABI.Pack("transferOutAndCall", aggAddr, targetAddr, dest, targetLimit.BigInt(), tx.Memo)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("fail to create data to call smart contract(transferOutAndCall): %w", err)
			}
		}
	case mem.TxMigrate:
		if tx.Aggregator != "" || tx.AggregatorTargetAsset != "" {
			return nil, nil, nil, fmt.Errorf("migration can't use aggregator")
		}
		if IsETH(tokenAddr) {
			data, err = c.vaultABI.Pack("transferOut", dest, ecommon.HexToAddress(tokenAddr), value, tx.Memo)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("fail to create data to call smart contract(transferOut): %w", err)
			}
		} else {
			newSmartContractAddr := c.getSmartContractByAddress(tx.ToAddress)
			if newSmartContractAddr.IsEmpty() {
				return nil, nil, nil, fmt.Errorf("fail to get new smart contract address")
			}
			data, err = c.vaultABI.Pack("transferAllowance", ecommon.HexToAddress(newSmartContractAddr.String()), dest, ecommon.HexToAddress(tokenAddr), value, tx.Memo)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("fail to create data to call smart contract(transferAllowance): %w", err)
			}
		}
	}

	// the nonce is stored as the transaction checkpoint, if it is set deserialize it
	// so we only retry with the same nonce to avoid double spend
	var nonce uint64
	if tx.Checkpoint != nil {
		if err = json.Unmarshal(tx.Checkpoint, &nonce); err != nil {
			return nil, nil, nil, fmt.Errorf("fail to deserialize checkpoint: %w", err)
		}
	} else {
		nonce, err = c.GetNonce(fromAddr.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to fetch account(%s) nonce : %w", fromAddr, err)
		}

		// abort signing if the pending nonce is too far in the future
		var finalizedNonce uint64
		finalizedNonce, err = c.GetNonceFinalized(fromAddr.String())
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
	c.logger.Info().Uint64("nonce", nonce).Msg("account info")

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

		ctx, cancel := c.getContext()
		defer cancel()
		err = c.client.SendTransaction(ctx, broadcastTx)
		if err != nil && err.Error() != txpool.ErrAlreadyKnown.Error() && err.Error() != ecore.ErrNonceTooLow.Error() {
			c.logger.Err(err).Str("hash", broadcastTx.Hash().String()).Msg("fail to broadcast approval transaction")
			return nil, nonceBytes, nil, fmt.Errorf("fail to broadcast approval transaction: %w", err)
		}

		tokenAddr = getTokenAddressFromAsset(tx.Coins[0].Asset)
		contractAddr = c.getSmartContractAddr(tx.VaultPubKey)

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
	}

	// Convert TxOutItem gas rate units to Wei.
	_, gasRateUnitsPerOne := c.cfg.ChainID.GetGasUnits()
	toiGasRateWei := cosmos.NewUint(uint64(tx.GasRate)).MulUint64(common.WeiPerOne).Quo(gasRateUnitsPerOne).BigInt()

	gasRate := c.GetGasPrice()
	if c.cfg.BlockScanner.FixedGasRate > 0 || gasRate.Cmp(big.NewInt(0)) == 0 {
		// if chain gas is zero we are still filling our gas price buffer, use outbound rate
		gasRate = toiGasRateWei
	} else {
		// Thornode uses a gas rate 1.5x the reported network fee for the rate and computed
		// max gas to ensure the rate is sufficient when it is signed later. Since we now know
		// the more recent rate, we will use our current rate with a lower bound on 2/3 the
		// outbound rate (the original rate we reported to Thornode in the network fee).
		lowerBound := toiGasRateWei
		lowerBound.Mul(lowerBound, big.NewInt(2))
		lowerBound.Div(lowerBound, big.NewInt(3))

		// round current rate to avoid consensus trouble, same rounding implied in outbound
		gasRate.Div(gasRate, big.NewInt(common.One*100))
		if gasRate.Cmp(big.NewInt(0)) == 0 { // floor at 1 like in network fee reporting
			gasRate = big.NewInt(1)
		}
		gasRate.Mul(gasRate, big.NewInt(common.One*100))

		// if the gas rate is less than the lower bound, use the lower bound
		if gasRate.Cmp(lowerBound) < 0 {
			gasRate = lowerBound
		}
	}

	// tip cap at configured percentage of max fee
	tipCap := new(big.Int).Mul(gasRate, big.NewInt(int64(c.cfg.EVM.MaxGasTipPercentage)))
	tipCap.Div(tipCap, big.NewInt(100))

	c.logger.Info().
		Stringer("inHash", tx.InHash).
		Str("outboundRate", c.convertThorchainAmountToWei(big.NewInt(tx.GasRate)).String()).
		Str("currentRate", c.GetGasPrice().String()).
		Str("effectiveRate", gasRate.String()).
		Msg("gas rate")

	// outbound tx always send to smart contract address
	estimatedETHValue := big.NewInt(0)
	estimateData := data
	if ethValue.Uint64() > 0 {
		// Use a small fixed value to estimate gas to avoid "insufficient fund" or "gas required
		// exceeds allowance" errors that can occur with the real value.
		estimatedETHValue = estimatedETHValue.SetInt64(21000)
		// The V6 router's transferOut requires msg.value == amount, so for transferOut calls
		// (non-aggregator native transfers) we must repack the call data with the matching
		// small value. transferOutAndCall doesn't have this check, so it uses data as-is.
		if tx.Aggregator == "" {
			estimateData, err = c.vaultABI.Pack("transferOut", dest, ecommon.HexToAddress(tokenAddr), estimatedETHValue, tx.Memo)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("fail to pack gas estimation transferOut: %w", err)
			}
		}
	}

	var createdTx *etypes.Transaction
	if c.cfg.BlockScanner.FixedGasRate == 0 {
		to := ecommon.HexToAddress(contractAddr.String())
		createdTx = etypes.NewTx(&etypes.DynamicFeeTx{
			ChainID:   c.chainID,
			Nonce:     nonce,
			To:        &to,
			Value:     estimatedETHValue,
			GasFeeCap: gasRate, // maxFeePerGas
			GasTipCap: tipCap,  // maxPriorityFeePerGas
			Data:      estimateData,

			// gas is ignored in estimate gas call
			// Gas: c.cfg.BlockScanner.MaxGasLimit,
		})
	} else {
		createdTx = etypes.NewTransaction(nonce, ecommon.HexToAddress(contractAddr.String()), estimatedETHValue, c.cfg.BlockScanner.MaxGasLimit, gasRate, estimateData)
	}

	estimatedGas, err := c.estimateGas(fromAddr.String(), createdTx)
	if err != nil {
		// in an edge case that vault doesn't have enough fund to fulfill an outbound transaction , it will fail to estimate gas
		// the returned error is `execution reverted`
		// when this fail , chain client should skip the outbound and move on to the next. The network will reschedule the outbound
		// after 300 blocks
		c.logger.Err(err).Msgf("fail to estimate gas")
		return nil, nonceBytes, nil, nil
	}
	c.logger.Info().Msgf("memo:%s estimated gas unit: %d", tx.Memo, estimatedGas)

	scheduledMaxFee := big.NewInt(0)
	for _, coin := range tx.MaxGas {
		scheduledMaxFee.Add(scheduledMaxFee, c.convertThorchainAmountToWei(coin.Amount.BigInt()))
	}

	if tx.Aggregator != "" {
		var gasLimitForAggregator uint64
		gasLimitForAggregator, err = aggregators.FetchDexAggregatorGasLimit(
			c.cfg.ChainID, tx.Aggregator,
		)
		if err != nil {
			c.logger.Err(err).
				Str("aggregator", tx.Aggregator).
				Msg("fail to get aggregator gas limit, aborting to let thornode reschedule")
			return nil, nil, nil, nil
		}

		// if the estimate gas is over the max, abort and let thornode reschedule for now
		if estimatedGas > gasLimitForAggregator {
			c.logger.Warn().
				Stringer("in_hash", tx.InHash).
				Uint64("estimated_gas", estimatedGas).
				Uint64("aggregator_gas_limit", gasLimitForAggregator).
				Msg("aggregator gas limit exceeded, aborting to let thornode reschedule")
			return nil, nil, nil, nil
		}

		// set limit to aggregator gas limit
		estimatedGas = gasLimitForAggregator

		scheduledMaxFee = scheduledMaxFee.Mul(scheduledMaxFee, big.NewInt(c.cfg.EVM.AggregatorMaxGasMultiplier))
	} else if len(tx.Coins) > 0 && !tx.Coins[0].Asset.IsGasAsset() {
		scheduledMaxFee = scheduledMaxFee.Mul(scheduledMaxFee, big.NewInt(c.cfg.EVM.TokenMaxGasMultiplier))
	}

	var estimatedFee *big.Int
	if c.cfg.BlockScanner.FixedGasRate == 0 {
		// determine max gas units based on scheduled max gas (fee) and current rate
		maxGasUnits := new(big.Int).Div(scheduledMaxFee, gasRate).Uint64()

		// if estimated gas is more than the planned gas, abort and let thornode reschedule
		if estimatedGas > maxGasUnits {
			c.logger.Warn().
				Stringer("in_hash", tx.InHash).
				Stringer("rate", gasRate).
				Uint64("estimated_gas_units", estimatedGas).
				Uint64("max_gas_units", maxGasUnits).
				Str("scheduled_max_fee", scheduledMaxFee.String()).
				Msg("max gas exceeded, aborting to let thornode reschedule")
			return nil, nil, nil, nil
		}

		estimatedFee = big.NewInt(int64(estimatedGas))
		totalGasRate := big.NewInt(0).Add(gasRate, tipCap)
		estimatedFee.Mul(estimatedFee, totalGasRate)

		to := ecommon.HexToAddress(contractAddr.String())
		createdTx = etypes.NewTx(&etypes.DynamicFeeTx{
			ChainID:   c.chainID,
			Nonce:     nonce,
			To:        &to,
			Value:     ethValue,
			Gas:       maxGasUnits,
			GasFeeCap: gasRate,
			GasTipCap: tipCap,
			Data:      data,
		})
	} else {

		// if over max scheduled gas, abort and let thornode reschedule
		estimatedFee = big.NewInt(int64(estimatedGas) * gasRate.Int64())
		if scheduledMaxFee.Cmp(estimatedFee) < 0 {
			c.logger.Warn().
				Stringer("in_hash", tx.InHash).
				Stringer("rate", gasRate).
				Uint64("estimated_gas", estimatedGas).
				Str("estimated_fee", estimatedFee.String()).
				Str("scheduled_max_fee", scheduledMaxFee.String()).
				Msg("max gas exceeded, aborting to let thornode reschedule")
			return nil, nil, nil, nil
		}

		createdTx = etypes.NewTransaction(
			nonce, ecommon.HexToAddress(contractAddr.String()), ethValue, estimatedGas, gasRate, data,
		)
	}

	// before signing, confirm the vault has enough gas asset
	// use nil height to get the latest confirmed balance (not pending, to avoid masking attacks)
	gasBalance, err := c.GetBalance(fromAddr.String(), ethToken, nil, "")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to get gas asset balance: %w", err)
	}

	// Subtract pending liabilities (signed but unconfirmed transactions) from the balance
	confirmedNonce, _ := c.GetNonceFinalized(fromAddr.String())
	pendingLiability := c.getPendingLiability(fromAddr.String(), confirmedNonce)
	effectiveBalance := new(big.Int).Sub(gasBalance, pendingLiability)

	requiredBalance := new(big.Int).Add(ethValue, estimatedFee)
	if effectiveBalance.Cmp(requiredBalance) < 0 {
		return nil, nil, nil, fmt.Errorf("insufficient gas asset balance: effective %s (latest %s - liability %s) < required %s",
			effectiveBalance.String(), gasBalance.String(), pendingLiability.String(), requiredBalance.String())
	}

	// Declare rawTx at function level for proper scoping
	var rawTx []byte

	// Sign main transaction
	rawTx, err = c.sign(createdTx, tx.VaultPubKey, height, tx)
	if err != nil || len(rawTx) == 0 {
		return nil, nonceBytes, nil, fmt.Errorf("fail to sign message: %w", err)
	}

	// Record the liability for this signed transaction (value + estimated fee)
	c.recordLiability(fromAddr.String(), nonce, requiredBalance)

	// create the observation to be sent by the signer before broadcast
	chainHeight, err := c.GetHeight()
	if err != nil { // fall back to the scanner height, thornode voter does not use height
		chainHeight = c.ethScanner.getCurrentBlockHeight()
	}
	coin := tx.Coins[0]
	gas := common.MakeEVMGas(c.GetChain(), createdTx.GasPrice(), createdTx.Gas(), nil)
	// This is the maximum gas, using the gas limit for instant-observation
	// rather than the GasUsed which can only be gotten from the receipt when scanning.

	signedTx := &etypes.Transaction{}
	if err = signedTx.UnmarshalJSON(rawTx); err != nil {
		return nil, rawTx, nil, fmt.Errorf("fail to unmarshal signed tx: %w", err)
	}

	var txIn *stypes.TxInItem

	if err == nil {
		txIn = stypes.NewTxInItem(
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
	}

	return rawTx, nonceBytes, txIn, nil
}

// sign is design to sign a given message with keysign party and keysign wrapper
func (c *Client) sign(tx *etypes.Transaction, poolPubKey common.PubKey, height int64, txOutItem stypes.TxOutItem) ([]byte, error) {
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
		c.logger.Info().Str("tx_id", txID.String()).Msgf("post keysign failure to thorchain")
	}
	return nil, fmt.Errorf("fail to sign tx: %w", err)
}

// GetBalance call smart contract to find out the balance of the given address and token.
// For ERC20 tokens, it queries vaultAllowance on the vault's specific router contract.
func (c *Client) GetBalance(addr, token string, height *big.Int, contractAddr string) (*big.Int, error) {
	ctx, cancel := c.getContext()
	defer cancel()
	if IsETH(token) {
		if height != nil && height.Cmp(big.NewInt(-1)) == 0 {
			return c.client.PendingBalanceAt(ctx, ecommon.HexToAddress(addr))
		}
		return c.client.BalanceAt(ctx, ecommon.HexToAddress(addr), height)
	}
	if contractAddr == "" {
		return nil, fmt.Errorf("fail to get contract address for vault")
	}
	input, err := c.vaultABI.Pack("vaultAllowance", ecommon.HexToAddress(addr), ecommon.HexToAddress(token))
	if err != nil {
		return nil, fmt.Errorf("fail to create vaultAllowance data to call smart contract")
	}
	c.logger.Debug().Msgf("query contract:%s for balance", contractAddr)
	toAddr := ecommon.HexToAddress(contractAddr)
	res, err := c.client.CallContract(ctx, ethereum.CallMsg{
		From: ecommon.HexToAddress(addr),
		To:   &toAddr,
		Data: input,
	}, height)
	if err != nil {
		return nil, err
	}
	output, err := c.vaultABI.Unpack("vaultAllowance", res)
	if err != nil {
		return nil, err
	}
	value, ok := abi.ConvertType(output[0], new(*big.Int)).(**big.Int)
	if !ok {
		return *value, fmt.Errorf("dev error: unable to get big.Int")
	}
	return *value, nil
}

// GetBalances gets all the balances of the given address.
// contractAddr is the router contract for the vault.
func (c *Client) GetBalances(addr string, height *big.Int, contractAddr string) (common.Coins, error) {
	// for all the tokens , this chain client have deal with before
	tokens, err := c.ethScanner.GetTokens()
	if err != nil {
		return nil, fmt.Errorf("fail to get all the tokens: %w", err)
	}
	coins := common.Coins{}
	for _, token := range tokens {
		var balance *big.Int
		balance, err = c.GetBalance(addr, token.Address, height, contractAddr)
		if err != nil {
			c.logger.Err(err).Msgf("fail to get balance for token:%s", token.Address)
			continue
		}
		asset := common.ETHAsset
		if !IsETH(token.Address) {
			asset, err = common.NewAsset(fmt.Sprintf("ETH.%s-%s", token.Symbol, token.Address))
			if err != nil {
				return nil, err
			}
		}
		bal := c.ethScanner.convertAmount(token.Address, balance)
		coins = append(coins, common.NewCoin(asset, bal))
	}

	return coins.Distinct(), nil
}

// GetAccount gets account by address in eth client
func (c *Client) GetAccount(pk common.PubKey, height *big.Int) (common.Account, error) {
	addr := c.GetAddress(pk)
	nonce, err := c.GetNonce(addr)
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

// GetAccountByAddress return account information
func (c *Client) GetAccountByAddress(address string, height *big.Int) (common.Account, error) {
	nonce, err := c.GetNonce(address)
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

// BroadcastTx decodes tx using rlp and broadcasts too Ethereum chain
func (c *Client) BroadcastTx(txOutItem stypes.TxOutItem, hexTx []byte) (string, error) {
	tx := &etypes.Transaction{}
	if err := tx.UnmarshalJSON(hexTx); err != nil {
		return "", err
	}
	ctx, cancel := c.getContext()
	defer cancel()
	if err := c.client.SendTransaction(ctx, tx); err != nil && err.Error() != txpool.ErrAlreadyKnown.Error() && err.Error() != ecore.ErrNonceTooLow.Error() {
		return "", err
	}
	txID := tx.Hash().String()
	c.logger.Info().Msgf("broadcast tx with memo: %s to ETH chain , hash: %s", txOutItem.Memo, txID)

	if err := c.signerCacheManager.SetSigned(txOutItem.CacheHash(), txOutItem.CacheVault(c.GetChain()), txID); err != nil {
		c.logger.Err(err).Msgf("fail to mark tx out item (%+v) as signed", txOutItem)
	}

	blockHeight, err := c.bridge.GetBlockHeight()
	if err != nil {
		c.logger.Err(err).Msgf("fail to get current THORChain block height")
		// at this point , the tx already broadcast successfully , don't return an error
		// otherwise will cause the same tx to retry
	} else if err = c.AddSignedTxItem(txID, blockHeight, txOutItem.VaultPubKey.String(), &txOutItem); err != nil {
		c.logger.Err(err).Msgf("fail to add signed tx item,hash:%s", txID)
	}

	return txID, nil
}

// ConfirmationCountReady check whether the given txIn is ready to be send to THORChain
func (c *Client) ConfirmationCountReady(txIn stypes.TxIn) bool {
	if len(txIn.TxArray) == 0 {
		return true
	}
	// MemPool items doesn't need confirmation
	if txIn.MemPool {
		return true
	}
	blockHeight := txIn.TxArray[0].BlockHeight
	confirm := txIn.ConfirmationRequired
	c.logger.Info().
		Int64("height", txIn.TxArray[0].BlockHeight).
		Int64("required", confirm).
		Int("transactions", len(txIn.TxArray)).
		Msg("pending confirmations")

	// every tx in txIn already have at least 1 confirmation
	return (c.ethScanner.getCurrentBlockHeight() - blockHeight) >= confirm
}

func (c *Client) getBlockReward(height int64) (*big.Int, error) {
	return big.NewInt(ethBlockRewardAndFee), nil
}

func (c *Client) getTotalTransactionValue(txIn stypes.TxIn, excludeFrom []common.Address) cosmos.Uint {
	total := cosmos.ZeroUint()
	if len(txIn.TxArray) == 0 {
		return total
	}
	for _, item := range txIn.TxArray {
		fromAsgard := false
		for _, fromAddress := range excludeFrom {
			if strings.EqualFold(fromAddress.String(), item.Sender) {
				fromAsgard = true
				break
			}
		}
		if fromAsgard {
			continue
		}
		for _, coin := range item.Coins {
			if coin.IsEmpty() {
				continue
			}
			amount := coin.Amount
			if !coin.Asset.Equals(common.ETHAsset) {
				var err error
				amount, err = c.poolMgr.GetValue(coin.Asset, common.ETHAsset, coin.Amount)
				if err != nil {
					c.logger.Err(err).Msgf("fail to get value for %s", coin.Asset)
					continue
				}

			}
			total = total.Add(amount)
		}
	}
	return total
}

// getBlockRequiredConfirmation find out how many confirmation the given txIn need to have before it can be send to THORChain
func (c *Client) getBlockRequiredConfirmation(txIn stypes.TxIn, height int64) (int64, error) {
	asgards, err := c.getAsgardAddress()
	if err != nil {
		c.logger.Err(err).Msg("fail to get asgard addresses")
	}
	c.logger.Debug().Msgf("asgards: %+v", asgards)
	totalTxValue := c.getTotalTransactionValue(txIn, asgards)
	totalTxValueInWei := c.convertThorchainAmountToWei(totalTxValue.BigInt())
	confMul, err := utxo.GetConfMulBasisPoint(c.GetChain().String(), c.bridge)
	if err != nil {
		c.logger.Err(err).Msgf("failed to get conf multiplier mimir value for %s", c.GetChain().String())
	}
	totalFeeAndSubsidy, err := c.getBlockReward(height)
	confValue := common.GetUncappedShare(confMul, cosmos.NewUint(constants.MaxBasisPts), cosmos.NewUintFromBigInt(totalFeeAndSubsidy))
	if err != nil {
		return 0, fmt.Errorf("fail to get coinbase value: %w", err)
	}
	confirm := cosmos.NewUintFromBigInt(totalTxValueInWei).MulUint64(2).Quo(confValue).Uint64()
	confirm, err = utxo.MaxConfAdjustment(confirm, c.GetChain().String(), c.bridge)
	if err != nil {
		c.logger.Err(err).Msgf("fail to get max conf value adjustment for %s", c.GetChain().String())
	}
	c.logger.Info().Msgf("totalTxValue:%s,total fee and Subsidy:%d,confirmation:%d", totalTxValueInWei, totalFeeAndSubsidy, confirm)
	if confirm < 2 {
		// in ETH PoS (post merge) reorgs are harder to do but can occur. In
		// looking at 1k reorg blocks, 10 were reorg'ed at a height of 2, and
		// the rest were one (none were three or larger). While the odds of
		// getting reorg'ed are small (as it can only happen for very small
		// trades), the additional delay to swappers is also small (12 secs or
		// so). Thus, the determination by thorsec, 9R and devs were to set the
		// new min conf is 2.
		return 2, nil
	}
	return int64(confirm), nil
}

// GetConfirmationCount decide the given txIn how many confirmation it requires
func (c *Client) GetConfirmationCount(txIn stypes.TxIn) int64 {
	if len(txIn.TxArray) == 0 {
		return 0
	}
	// MemPool items doesn't need confirmation
	if txIn.MemPool {
		return 0
	}
	blockHeight := txIn.TxArray[0].BlockHeight
	confirm, err := c.getBlockRequiredConfirmation(txIn, blockHeight)
	c.logger.Debug().Msgf("confirmation required: %d", confirm)
	if err != nil {
		c.logger.Err(err).Msg("fail to get block confirmation ")
		return 0
	}
	return confirm
}

func (c *Client) getAsgardAddress() ([]common.Address, error) {
	return utxo.GetAsgardAddressCached(&c.asgardCache, common.ETHChain, c.bridge, constants.ThorchainBlockTime)
}

// OnObservedTxIn gets called from observer when we have a valid observation
func (c *Client) OnObservedTxIn(txIn stypes.TxInItem, blockHeight int64) {
	c.ethScanner.onObservedTxIn(txIn, blockHeight)
	m, err := mem.ParseMemo(common.LatestVersion, txIn.Memo)
	if err != nil {
		// Debug log only as ParseMemo error is expected for THORName inbounds.
		c.logger.Debug().Err(err).Msgf("fail to parse memo: %s", txIn.Memo)
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

func (c *Client) ReportSolvency(ethBlockHeight int64) error {
	if !c.ShouldReportSolvency(ethBlockHeight) {
		return nil
	}

	// when block scanner is not healthy, only report from auto-unhalt SolvencyCheckRunner
	// (FetchTxs passes currentBlockHeight, while SolvencyCheckRunner passes chainHeight)
	if !c.IsBlockScannerHealthy() && ethBlockHeight == c.ethScanner.getCurrentBlockHeight() {
		return nil
	}

	// fetch all asgard vaults
	asgardVaults, err := c.bridge.GetAsgards()
	if err != nil {
		return fmt.Errorf("fail to get asgards,err: %w", err)
	}

	// 3x MaxGas breathing room, from gas rate units gas price to THORChain (1e8) format
	_, gasRateUnitsPerOne := c.cfg.ChainID.GetGasUnits()
	currentGasFee := cosmos.NewUint(c.cfg.BlockScanner.MaxGasLimit).MulUint64(3).MulUint64(c.ethScanner.lastReportedGasPrice).MulUint64(common.One).Quo(gasRateUnitsPerOne)

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
		acct, err = c.GetAccount(chainVaults[i].PubKey, new(big.Int).SetInt64(ethBlockHeight))
		if err != nil {
			c.logger.Err(err).Msgf("fail to get account balance")
			continue
		}

		msg := stypes.Solvency{
			Height: ethBlockHeight,
			Chain:  common.ETHChain,
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
			c.logger.Info().Msgf("fail to send solvency info to THORChain, timeout")
		}
	}
	c.lastSolvencyCheckHeight = ethBlockHeight
	return nil
}

// ShouldReportSolvency with given block height , should chain client report Solvency to THORNode?
func (c *Client) ShouldReportSolvency(height int64) bool {
	return height%20 == 0
}

// checkAndApproveAllowance checks if token allowance is sufficient and creates an approval transaction if needed
func (c *Client) checkAndApproveAllowance(tx stypes.TxOutItem, nonce uint64) (*etypes.Transaction, error) {
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
		return nil, errors.New("ethereum token tx cannot have more than 1 coin")
	}

	coin := tx.Coins[0]
	tokenAddr := getTokenAddressFromAsset(coin.Asset)

	// Skip native token transfers (ETH) - they don't need approval
	if IsETH(tokenAddr) {
		return nil, nil
	}

	// Get vault and router addresses
	vaultAddr, err := tx.VaultPubKey.GetAddress(common.ETHChain)
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
	allowanceCallData, err := c.ethScanner.erc20ABI.Pack("allowance", ecommon.HexToAddress(vaultAddr.String()), ecommon.HexToAddress(contractAddr.String()))
	if err != nil {
		c.logger.Err(err).Msg("fail to pack allowance call data")
		return nil, errors.New("fail to pack allowance call data")
	}

	// Call allowance function on the token contract
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	tokenContractAddr := ecommon.HexToAddress(tokenAddr)
	result, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &tokenContractAddr,
		Data: allowanceCallData,
	}, nil)
	if err != nil {
		c.logger.Err(err).Str("token", tokenAddr).Msg("fail to call allowance function")
		return nil, errors.New("fail to call allowance function on token contract")
	}

	// Unpack allowance result
	allowanceResult, err := c.ethScanner.erc20ABI.Unpack("allowance", result)
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
	// convertSigningAmount handles the decimal conversion based on the token's actual decimals
	transferAmount := coin.Amount.BigInt()
	transferAmount = c.convertSigningAmount(transferAmount, tokenAddr)

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

	approvalData, err := c.ethScanner.erc20ABI.Pack("approve", ecommon.HexToAddress(contractAddr.String()), maxUint256)
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
