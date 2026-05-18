package solana

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/mr-tron/base58"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	cosmoscryptoed25519 "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/runners"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/solana/rpc"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/solana/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	tssp "gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/tss"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	mem "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
)

const solvencyDivisor = 10

// signerCacheExpiry is the window after a broadcast during which SignTx respects
// the cache unconditionally, without an RPC lookup. Past this window, SignTx
// verifies the recorded signature on chain and only re-signs if the RPC
// definitively reports it absent. Both conditions must hold — expiry alone never
// invalidates, because thorchain observation consensus can lag actual Solana
// inclusion and re-signing a landed-but-not-yet-observed tx would double-spend.
//
// 2h is well past Solana's ~60s blockhash validity (MAX_PROCESSING_AGE = 150 slots)
// and generous against prolonged scanner or consensus lag. It corresponds to 4x
// SigningTransactionPeriod (1200 thorchain blocks at 6s), so a stuck outbound gets
// at most a few rescheduled reschedules before this window allows re-sign.
const signerCacheExpiry = 2 * time.Hour

// broadcastLookupDelay is the time to wait after a broadcast error before
// querying the chain to see if the transaction landed anyway. Exposed as a
// variable so tests can override it.
var broadcastLookupDelay = 3 * time.Second

// SOLClient is a client for the Solana blockchain
type SOLClient struct {
	logger               zerolog.Logger
	cfg                  config.BifrostChainConfiguration
	localPubKey          string
	k                    *cosmoscryptoed25519.PrivKey
	solScanner           *SOLScanner
	bridge               thorclient.ThorchainBridge
	poolMgr              thorclient.PoolManager
	tssKeyManager        tss.ThorchainKeyManager
	wg                   *sync.WaitGroup
	stopchan             chan struct{}
	globalSolvencyQueue  chan stypes.Solvency
	signerCacheManager   *signercache.CacheManager
	rpcClient            *rpc.SolRPC
	solvencySlotMultiple int64
}

// NewSOLClient creates a new instance of a SOLClient
func NewSOLClient(
	thorKeys *thorclient.Keys,
	cfg config.BifrostChainConfiguration,
	server *tssp.TssServer,
	bridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	poolMgr thorclient.PoolManager,
) (*SOLClient, error) {
	// check required arguments
	if thorKeys == nil {
		return nil, errors.New("thor keys empty")
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

	// // create keys
	tssKm, err := tss.NewKeySign(server, bridge)
	if err != nil {
		return nil, fmt.Errorf("failed to create tss signer: %w", err)
	}
	priv, err := thorKeys.GetPrivateKeyEDDSA()
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	pub, ok := priv.PubKey().(*cosmoscryptoed25519.PubKey)
	if !ok {
		return nil, errors.New("failed to cast pubkey to ed25519")
	}
	formattedPub := base58.Encode(pub.Bytes())

	// create rpc clients
	rpcClient := rpc.NewSolRPC(cfg.RPCHost, cfg.BlockScanner.HTTPRequestTimeout)

	c := &SOLClient{
		logger:        log.With().Str("module", "sol").Stringer("chain", cfg.ChainID).Logger(),
		cfg:           cfg,
		localPubKey:   formattedPub,
		k:             priv,
		bridge:        bridge,
		poolMgr:       poolMgr,
		tssKeyManager: tssKm,
		wg:            &sync.WaitGroup{},
		stopchan:      make(chan struct{}),
		rpcClient:     rpcClient,
	}

	// initialize storage
	var path string // if not set later, will in memory storage
	if len(c.cfg.BlockScanner.DBPath) > 0 {
		path = fmt.Sprintf("%s/%s", c.cfg.BlockScanner.DBPath, c.cfg.BlockScanner.ChainID)
	}
	storage, err := blockscanner.NewBlockScannerStorageSolana(path, c.cfg.ScannerLevelDB)
	if err != nil {
		return c, fmt.Errorf("fail to create blockscanner storage: %w", err)
	}
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	if err != nil {
		return nil, fmt.Errorf("fail to create signer cache manager: %w", err)
	}
	c.signerCacheManager = signerCacheManager

	// create block scanner
	c.solScanner, err = NewSOLScanner(
		c.stopchan,
		c.cfg.BlockScanner,
		storage,
		bridge,
		m,
		rpcClient,
		pubkeyMgr,
		c.ReportSolvency,
		signerCacheManager,
	)
	if err != nil {
		return c, fmt.Errorf("fail to create solana scanner: %w", err)
	}

	return c, nil
}

// Start starts the chain client with the given queues.
func (c *SOLClient) Start(
	globalTxsQueue chan stypes.TxIn,
	globalErrataQueue chan stypes.ErrataBlock,
	globalSolvencyQueue chan stypes.Solvency,
	globalNetworkFeeQueue chan common.NetworkFee,
) {
	c.globalSolvencyQueue = globalSolvencyQueue
	c.solScanner.globalNetworkFeeQueue = globalNetworkFeeQueue
	c.solScanner.globalTxsQueue = globalTxsQueue
	c.tssKeyManager.Start()
	go c.solScanner.Start()
	c.wg.Add(1)
	go runners.SolvencyCheckRunner(c.GetChain(), c, c.bridge, c.stopchan, c.wg, constants.ThorchainBlockTime)
}

// Stop stops the chain client.
func (c *SOLClient) Stop() {
	c.tssKeyManager.Stop()
	close(c.stopchan)
	c.wg.Wait()
}

// IsBlockScannerHealthy returns true if the block scanner is healthy.
func (c *SOLClient) IsBlockScannerHealthy() bool {
	return c.solScanner.IsHealthy()
}

// --------------------------------- config ---------------------------------

// GetConfig returns the chain configuration.
func (c *SOLClient) GetConfig() config.BifrostChainConfiguration {
	return c.cfg
}

// GetChain returns the chain.
func (c *SOLClient) GetChain() common.Chain {
	return common.SOLChain
}

// --------------------------------- status ---------------------------------

// GetHeight returns the current height of the chain.
func (c *SOLClient) GetHeight() (int64, error) {
	return c.solScanner.GetHeight()
}

// GetBlockScannerHeight returns blockscanner height
func (c *SOLClient) GetBlockScannerHeight() (int64, error) {
	return c.solScanner.ScanHeight()
}

// RollbackBlockScanner rolls back the block scanner to the last observed block
func (c *SOLClient) RollbackBlockScanner() error {
	// rollback disabled for solana, as we scan by transaction signatures, not blocks heights.
	// a rollback to fetch all transactions for a vault could cause many duplicate observations and likewise the slashes for them.

	// if determined this is absolutely necessary, we could query all transactions for a vault address that exist on the node,
	// and then determine which transaction signature is the one immediately behind the desired rollback height, then use that
	// for queryLastSig passed to the SOLScanner.scanVault method.
	return nil
}

func (c *SOLClient) GetLatestTxForVault(vault string) (string, string, error) {
	lastObserved, err := c.signerCacheManager.GetLatestRecordedTx(stypes.InboundCacheKey(vault, c.GetChain().String()))
	if err != nil {
		return "", "", err
	}
	lastBroadCasted, err := c.signerCacheManager.GetLatestRecordedTx(stypes.BroadcastCacheKey(vault, c.GetChain().String()))
	return lastObserved, lastBroadCasted, err
}

// --------------------------------- addresses ---------------------------------

// GetAddress returns the address for the given public key.
func (c *SOLClient) GetAddress(poolPubKey common.PubKey) string {
	addr, err := poolPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		c.logger.Error().Err(err).Str("pool_pub_key", poolPubKey.String()).Msg("fail to get pool address")
		return ""
	}
	return addr.String()
}

// --------------------------------- balances ---------------------------------

// GetAccount returns the balance of the provided address.
func (c *SOLClient) GetAccount(pk common.PubKey, height *big.Int) (common.Account, error) {
	addr, err := pk.GetAddress(common.SOLChain)
	if err != nil {
		return common.Account{}, fmt.Errorf("fail to get address from pubkey(%s): %w", pk, err)
	}

	return c.GetAccountByAddress(addr.String(), height)
}

// GetAccountByAddress return account information
func (c *SOLClient) GetAccountByAddress(addr string, height *big.Int) (common.Account, error) {
	lamportsBalance, err := c.rpcClient.GetBalance(addr, "finalized", 0)
	if err != nil {
		return common.Account{}, fmt.Errorf("fail to get balance: %w", err)
	}

	// get amount
	amount := cosmos.NewUintFromBigInt(lamportsBalance)
	amount = amount.Quo(cosmos.NewUint(10)) // 1e9 -> 1e8

	coins := common.Coins{
		common.NewCoin(common.SOLAsset, amount),
	}

	return common.Account{
		Coins: coins,
	}, nil
}

// --------------------------------- solvency ---------------------------------

// ReportSolvency reports solvency once per configured solvency blocks.
func (c *SOLClient) ReportSolvency(height int64) error {
	if !c.ShouldReportSolvency(height) {
		return nil
	}

	// when block scanner is not healthy, only report from auto-unhalt SolvencyCheckRunner
	// (FetchTxs passes currentBlockHeight, while SolvencyCheckRunner passes chainHeight)
	if !c.IsBlockScannerHealthy() && height == int64(c.solScanner.lastHeight) { //nolint:staticcheck
		return nil
	}

	// fetch all asgard vaults
	asgardVaults, err := c.bridge.GetAsgards()
	if err != nil {
		return fmt.Errorf("fail to get asgards, err: %w", err)
	}

	currentFeeRate := cosmos.NewUint(3 * c.cfg.BlockScanner.MaxGasLimit * c.solScanner.lastFeeRate)

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
	for _, v := range chainVaults {
		if v.PubKeyEddsa.IsEmpty() {
			c.logger.Error().Msg("asgard vault pubkey is empty, skipping solvency check")
			continue
		}
		acct, err := c.GetAccount(v.PubKeyEddsa, nil)
		if err != nil {
			c.logger.Err(err).Msg("fail to get account balance")
			continue
		}
		heightNearestFloor := (height / solvencyDivisor) * solvencyDivisor
		msg := stypes.Solvency{
			Height: heightNearestFloor,
			Chain:  c.cfg.ChainID,
			PubKey: v.PubKey, // or v.PubKeyEddsa
			Coins:  acct.Coins,
		}
		processedVaults++

		if runners.IsVaultSolvent(c.cfg.ChainID, acct, v, currentFeeRate) {
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
	c.solvencySlotMultiple = height / c.cfg.SolvencyBlocks
	return nil
}

// ShouldReportSolvency returns true if the given height is a solvency report height.
func (c *SOLClient) ShouldReportSolvency(height int64) bool {
	if c.cfg.SolvencyBlocks <= 0 {
		return false
	}

	return height > (c.solvencySlotMultiple+1)*c.cfg.SolvencyBlocks
}

// --------------------------------- sign ---------------------------------

// SignTx returns the signed transaction.
func (c *SOLClient) SignTx(tx stypes.TxOutItem, height int64) ([]byte, []byte, *stypes.TxInItem, error) {
	if tx.Chain != common.SOLChain {
		return nil, nil, nil, errors.New("tx chain is not solana")
	}
	if tx.ToAddress.IsEmpty() {
		return nil, nil, nil, errors.New("to address is empty")
	}
	if tx.VaultPubKeyEddsa.IsEmpty() {
		return nil, nil, nil, errors.New("vault public key is empty")
	}

	if signed, expired := c.signerCacheManager.HasSignedWithExpiry(tx.CacheHash()); signed {
		if !expired {
			// Pre-expiry: the recorded broadcast's blockhash may still allow it to
			// land, so never re-sign during this window — doing so risks a second
			// broadcast that lands alongside the first and double-spends.
			c.logger.Info().Interface("tx", tx).Msg("transaction signed before, ignore")
			return nil, nil, nil, nil
		}

		// Expired: re-signing is only safe after chain-side verification confirms
		// the recorded broadcast is not on chain. Any other invalidation (expiry
		// alone, RPC error) would risk a double-spend when thorchain observation
		// consensus lags behind actual Solana inclusion.
		sig, ok := c.signerCacheManager.GetSignedTxHash(tx.CacheHash())
		if !ok {
			// No recorded broadcast hash to verify — respect the cache.
			c.logger.Info().Interface("tx", tx).Msg("transaction signed before, ignore")
			return nil, nil, nil, nil
		}
		result, lookupErr := c.rpcClient.GetTransactionConfirmed(sig)
		switch {
		case lookupErr != nil:
			c.logger.Warn().Err(lookupErr).Str("sig", sig).Interface("tx", tx).
				Msg("signature lookup failed, respecting signer cache")
			return nil, nil, nil, nil
		case result.Slot > 0:
			c.logger.Info().Str("sig", sig).Interface("tx", tx).
				Msg("cached broadcast confirmed on chain, respecting signer cache")
			return nil, nil, nil, nil
		default:
			c.logger.Warn().Str("sig", sig).Interface("tx", tx).
				Msg("expired cached broadcast not found on chain, clearing signer cache")
			c.signerCacheManager.RemoveSigned(sig)
		}
	}

	// GetMemo returns OriginalMemo for memoless outbounds
	// For truly memoless transactions (no Memo or OriginalMemo), default to TxOutbound
	memoForParsing := tx.GetMemo()

	solTx, err := c.CreateTx(tx, memoForParsing)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to create tx: %w", err)
	}

	checkPointBytes, err := solTx.Message.Serialize()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to serialize checkpoint bytes: %w", err)
	}

	signedTx, err := c.signTransaction(solTx, tx)
	if err != nil {
		// Forward blamed TSS failures to Thornode so EdDSA outbounds participate
		// in the same slash/freeze handling as the secp256k1 signer paths.
		var keysignError tss.KeysignError
		if errors.As(err, &keysignError) && len(keysignError.Blame.BlameNodes) > 0 {
			txID, errPostKeysignFail := c.bridge.PostKeysignFailure(keysignError.Blame, height, tx.GetMemo(), tx.Coins, tx.VaultPubKeyEddsa)
			if errPostKeysignFail != nil {
				return checkPointBytes, nil, nil, multierror.Append(err, errPostKeysignFail)
			}
			c.logger.Info().Str("tx_id", txID.String()).Msg("post keysign failure to THORChain")
		}
		return checkPointBytes, nil, nil, fmt.Errorf("fail to sign tx: %w", err)
	}

	signedBytes, err := signedTx.Serialize()
	if err != nil {
		return checkPointBytes, nil, nil, fmt.Errorf("fail to serialize signed tx: %w", err)
	}

	fromAddr, err := tx.VaultPubKeyEddsa.GetAddress(common.SOLChain)
	if err != nil {
		return checkPointBytes, nil, nil, fmt.Errorf("fail to get vault address: %w", err)
	}

	txIn := stypes.NewTxInItem(
		height,
		base58.Encode(signedTx.Signatures[0]),
		tx.Memo,
		fromAddr.String(),
		tx.ToAddress.String(),
		tx.Coins,
		common.Gas([]common.Coin{{Asset: common.SOLAsset, Amount: cosmos.NewUint(5000), Decimals: 9}}),
		tx.VaultPubKeyEddsa,
		"",
		"",
		nil,
	)

	return signedBytes, nil, txIn, nil
}

// SignTransaction takes a rpc.Transaction and signs with the pubkey
func (c *SOLClient) signTransaction(solTx types.Transaction, txOut stypes.TxOutItem) (types.Transaction, error) {
	message, err := solTx.Message.Serialize()
	if err != nil {
		return solTx, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	var sig []byte

	parsedPub, err := txOut.VaultPubKeyEddsa.GetAddress(c.GetChain())
	if err != nil {
		return types.Transaction{}, fmt.Errorf("failed to get address from pubkey: %w", err)
	}

	if parsedPub.String() == c.localPubKey {
		sig, err = c.k.Sign(message)
		if err != nil {
			return types.Transaction{}, fmt.Errorf("failed to sign local transaction: %w", err)
		}
	} else {
		sig, _, err = c.tssKeyManager.RemoteSign(message, common.SigningAlgoEd25519, txOut.VaultPubKeyEddsa.String())
		if err != nil {
			return types.Transaction{}, fmt.Errorf("failed to sign remote transaction: %w", err)
		}
	}

	vaultAccount := solTx.Message.Accounts[0]
	if !ed25519.Verify(vaultAccount.Bytes(), message, sig) {
		return types.Transaction{}, errors.New("signature verification failed")
	}

	if err := solTx.AddSignature(sig); err != nil {
		return types.Transaction{}, fmt.Errorf("failed to add signature: %w", err)
	}

	return solTx, nil
}

// CreateTx builds a solana transfer function from a TxOutItem using the transaction
// specification (https://solana.com/docs/core/transactions#manual-sol-transfer)
func (c *SOLClient) CreateTx(tx stypes.TxOutItem, memoForParsing string) (types.Transaction, error) {
	recentBlockhash := c.solScanner.getRecentBlockHash()
	if recentBlockhash == "" {
		return types.Transaction{}, errors.New("recent blockhash is empty")
	}

	// Validation - use memoForParsing (which may be OriginalMemo for memoless outbounds)
	// For truly memoless transactions, default to TxOutbound
	var memo mem.Memo
	if memoForParsing == "" {
		memo = mem.NewOutboundMemo(tx.InHash)
	} else {
		var err error
		memo, err = mem.ParseMemo(common.LatestVersion, memoForParsing)
		if err != nil {
			return types.Transaction{}, fmt.Errorf("fail to parse memo(%s):%w", memoForParsing, err)
		}
		if memo.IsInbound() {
			return types.Transaction{}, errors.New("inbound memo should not be used for outbound tx")
		}
	}
	if len(tx.Coins) != 1 {
		return types.Transaction{}, fmt.Errorf("only support one coin per transaction, received: %d", len(tx.Coins))
	}

	solTx := types.Transaction{}

	// Create account pubkeys
	vaultAddr, err := tx.VaultPubKeyEddsa.GetAddress(c.GetChain())
	if err != nil {
		return types.Transaction{}, fmt.Errorf("fail to get vault address: %w", err)
	}
	vaultPubKey, err := types.PublicKeyFromString(vaultAddr.String())
	if err != nil {
		return solTx, fmt.Errorf("fail to create vault public key: %w", err)
	}
	toPubkey, err := types.PublicKeyFromString(tx.ToAddress.String())
	if err != nil {
		return solTx, fmt.Errorf("fail to create to public key: %w", err)
	}

	accountKeys := []types.PublicKey{vaultPubKey, toPubkey, types.SystemProgramID, types.MemoProgramID}

	// Memo instruction
	// Program index is 3 (referencing accountKeys) for the memo program
	// Account index is 0 for the vault/signer
	memoInstruction := types.NewCompiledInstruction(3, []int{0}, []byte(tx.Memo))

	solValue := convertToLamports(tx.Coins[0].Amount)
	if solValue.Int64() == 0 {
		return solTx, errors.New("sol value is zero")
	}
	// Transfer instruction
	// Program index is 2 (referencing accountKeys) for the system program
	// Account index is 0 for the vault/signer
	// Account index is 1 for the recipient
	transferInstruction := types.NewCompiledTransferInstruction(2, 0, 1, solValue.Uint64())

	header := types.MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 2,
	}

	message := types.NewMessage(header, accountKeys, recentBlockhash, []types.CompiledInstruction{memoInstruction, transferInstruction})
	return types.NewTransaction(message), nil
}

// --------------------------------- sign ---------------------------------

func convertToLamports(amount cosmos.Uint) *big.Int {
	// thorchain amounts in 1e8
	// multiply by 10 to get 1e9 (lamports)
	return new(big.Int).Mul(amount.BigInt(), big.NewInt(10))
}

// --------------------------------- broadcast ---------------------------------

// BroadcastTx broadcasts the transaction and returns the transaction hash.
func (c *SOLClient) BroadcastTx(txOutItem stypes.TxOutItem, rawTx []byte) (string, error) {
	base64Tx := base64.StdEncoding.EncodeToString(rawTx)
	txSig, err := c.rpcClient.BroadcastTx(base64Tx)
	if err != nil {
		c.logger.Err(err).Str("memo", txOutItem.Memo).Msg("broadcast error, checking if tx landed on chain")

		// Wait for the transaction to reach confirmed status before querying. Without this,
		// the lookup races against block production and likely misses on the first attempt.
		time.Sleep(broadcastLookupDelay)

		// The RPC call may have errored after the transaction was already accepted and
		// included on-chain (e.g. network timeout on the response). Extract the signature
		// from the raw bytes and look it up on chain; if found, treat the broadcast as
		// successful so the signer cache is written and the transaction is not re-signed on
		// a future reschedule.
		if knownSig, ok := txSigFromRawTx(rawTx); ok {
			if result, lookupErr := c.rpcClient.GetTransactionConfirmed(knownSig); lookupErr == nil && result.Slot > 0 {
				c.logger.Warn().Str("memo", txOutItem.Memo).Str("txid", knownSig).
					Msg("tx found on chain despite broadcast error, updating signer cache")
				expiry := time.Now().Add(signerCacheExpiry).UnixMilli()
				if cacheErr := c.signerCacheManager.SetSignedWithExpiry(txOutItem.CacheHash(), txOutItem.CacheVault(c.GetChain()), knownSig, expiry); cacheErr != nil {
					c.logger.Err(cacheErr).Interface("txOutItem", txOutItem).Msg("fail to mark tx out item as signed")
				}
				return knownSig, nil
			}
		}
		return "", fmt.Errorf("failed to broadcast tx: %w", err)
	}
	c.logger.Info().Str("memo", txOutItem.Memo).Str("txid", txSig).Msg("broadcast tx")

	// Record the broadcast in the signer cache with its expiry. SignTx will respect
	// the cache unconditionally until the expiry passes; past expiry it verifies on
	// chain before re-signing, so a dropped broadcast eventually recovers without
	// risking a double-spend against a lagging observation.
	expiry := time.Now().Add(signerCacheExpiry).UnixMilli()
	if err := c.signerCacheManager.SetSignedWithExpiry(txOutItem.CacheHash(), txOutItem.CacheVault(c.GetChain()), txSig, expiry); err != nil {
		c.logger.Err(err).Interface("txOutItem", txOutItem).Msg("fail to mark tx out item as signed")
	}

	return txSig, nil
}

// txSigFromRawTx extracts the base58-encoded transaction signature from a
// serialized Solana transaction. The wire format is:
//
//	[compact-u16 sig count = 0x01][64-byte signature][message...]
//
// All THORChain outbounds have exactly one signature, so the count prefix is
// always the single byte 0x01 and the signature occupies bytes [1:65].
func txSigFromRawTx(rawTx []byte) (string, bool) {
	const sigOffset = 1 // 1-byte compact-u16 for count=1
	const sigLen = 64
	if len(rawTx) < sigOffset+sigLen {
		return "", false
	}
	return base58.Encode(rawTx[sigOffset : sigOffset+sigLen]), true
}

// --------------------------------- observe ---------------------------------

// OnObservedTxIn is called when a new observed tx is received.
func (c *SOLClient) OnObservedTxIn(txIn stypes.TxInItem, blockHeight int64) {
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
func (c *SOLClient) GetConfirmationCount(txIn stypes.TxIn) int64 {
	// Solana has instant finality since we query everything with commitment "finalized"
	return 0
}

// ConfirmationCountReady returns true if the confirmation count is ready.
func (c *SOLClient) ConfirmationCountReady(txIn stypes.TxIn) bool {
	// Solana has instant finality since we query everything with commitment "finalized"
	return true
}
