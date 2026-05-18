package tron

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"

	_ "embed"

	"cosmossdk.io/math"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	tcmetrics "gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/runners"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/tron/api"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/tron/rpc"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	tctss "gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/tss"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/tokenlist"
	tcconfig "gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
)

const (
	// TimestampValidity is the expiration duration for an outbound transaction. We use
	// the thornode block time of the outbound height as the timestamp to preserve
	// determinism, so the 20 minute expiration means the built transaction will be valid
	// to broadcast for the first 20 minutes of the 30 minute signing period.
	TimestampValidity = 20 * time.Minute

	ConfirmationBlocks int64 = 1
	FinalityBlocks     int64 = 19
)

//go:embed abi/trc20.json
var trc20ContractABI []byte

type TronClient struct {
	logger             zerolog.Logger
	config             tcconfig.BifrostChainConfiguration
	chainId            string
	blockScanner       *blockscanner.BlockScanner
	storage            *blockscanner.BlockScannerStorage
	signerCacheManager *signercache.CacheManager
	tssKeyManager      *tss.KeySign
	localKeyManager    *KeyManager
	tronScanner        *TronBlockScanner
	api                *api.TronApi
	rpc                *rpc.TronRpc
	abi                abi.ABI

	whitelist           map[string]tokenlist.ERC20Token
	bridge              thorclient.ThorchainBridge
	globalSolvencyQueue chan types.Solvency
	wg                  *sync.WaitGroup
	stopchan            chan struct{}
}

// NewTronClient creates a new instance of a Tron chain client
func NewTronClient(
	thorKeys *thorclient.Keys,
	config tcconfig.BifrostChainConfiguration,
	server *tctss.TssServer,
	bridge thorclient.ThorchainBridge,
	metrics *tcmetrics.Metrics,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	poolMgr thorclient.PoolManager,
) (*TronClient, error) {
	var err error

	logger := log.With().Str("module", config.ChainID.String()).Logger()

	if pubkeyMgr == nil {
		return nil, errors.New("pubkey manager is nil")
	}

	tokens := tokenlist.GetEVMTokenList(config.ChainID).Tokens

	whitelist := map[string]tokenlist.ERC20Token{}
	for _, token := range tokens {
		for _, address := range config.BlockScanner.WhitelistTokens {
			if strings.EqualFold(address, token.Address) {
				whitelist[address] = token
			}
		}
	}

	client := TronClient{
		logger:    logger,
		chainId:   config.ChainID.String(),
		config:    config,
		bridge:    bridge,
		wg:        &sync.WaitGroup{},
		stopchan:  make(chan struct{}),
		whitelist: whitelist,
		api:       api.NewTronApi(config.APIHost, config.BlockScanner.HTTPRequestTimeout),
		rpc:       rpc.NewTronRpc(config.RPCHost, config.BlockScanner.HTTPRequestTimeout),
	}

	client.tssKeyManager, err = tss.NewKeySign(server, bridge)
	if err != nil {
		logger.Err(err).Msg("failed to create tss signer")
		return nil, err
	}

	client.localKeyManager, err = NewLocalKeyManager(thorKeys)
	if err != nil {
		logger.Err(err).Msg("failed to create local key manager")
		return nil, err
	}

	var path string // if not set later, will in memory storage
	if len(client.config.BlockScanner.DBPath) > 0 {
		path = fmt.Sprintf(
			"%s/%s", config.BlockScanner.DBPath, config.BlockScanner.ChainID,
		)
	}

	client.storage, err = blockscanner.NewBlockScannerStorage(
		path,
		client.config.ScannerLevelDB,
	)
	if err != nil {
		logger.Err(err).Msg("failed to create scan storage")
		return nil, err
	}

	client.tronScanner, err = NewTronBlockScanner(
		config,
		client.bridge,
		pubkeyMgr,
		client.ReportSolvency,
	)
	if err != nil {
		logger.Err(err).Msg("failed to create tron block scanner")
		return nil, err
	}

	client.blockScanner, err = blockscanner.NewBlockScanner(
		client.config.BlockScanner,
		client.storage,
		metrics,
		client.bridge,
		client.tronScanner,
	)
	if err != nil {
		logger.Err(err).Msg("failed to create block scanner")
		return nil, err
	}

	client.signerCacheManager, err = signercache.NewSignerCacheManager(
		client.storage.GetInternalDb(),
	)
	if err != nil {
		logger.Err(err).Msg("failed to create signer cache manager")
		return nil, err
	}

	client.abi, err = abi.JSON(bytes.NewReader(trc20ContractABI))
	if err != nil {
		logger.Err(err).Msg("failed to parse ABI")
		return nil, err
	}

	return &client, nil
}

// Start Tron chain client
func (c *TronClient) Start(
	globalTxsQueue chan types.TxIn,
	_ chan types.ErrataBlock,
	globalSolvencyQueue chan types.Solvency,
	globalNetworkFeeQueue chan common.NetworkFee,
) {
	c.globalSolvencyQueue = globalSolvencyQueue
	c.tronScanner.globalNetworkFeeQueue = globalNetworkFeeQueue
	c.blockScanner.Start(globalTxsQueue, globalNetworkFeeQueue)
	c.tssKeyManager.Start()

	c.wg.Add(1)
	go runners.SolvencyCheckRunner(
		c.GetChain(),
		c,
		c.bridge,
		c.stopchan,
		c.wg,
		time.Second,
	)
}

// Stop Tron chain client
func (c *TronClient) Stop() {
	c.tssKeyManager.Stop()
	c.blockScanner.Stop()
	close(c.stopchan)
	c.wg.Wait()
}

func (c *TronClient) IsBlockScannerHealthy() bool {
	return c.blockScanner.IsHealthy()
}

// GetChain returns the chain.
func (c *TronClient) GetChain() common.Chain {
	return c.config.ChainID
}

// GetConfig returns the chain client configuration
func (c *TronClient) GetConfig() tcconfig.BifrostChainConfiguration {
	return c.config
}

// GetHeight returns the current height of the chain.
func (c *TronClient) GetHeight() (int64, error) {
	return c.tronScanner.GetHeight()
}

// GetAddress returns the address for the given public key.
func (c *TronClient) GetAddress(pubKey common.PubKey) string {
	address, err := pubKey.GetAddress(c.GetChain())
	if err != nil {
		c.logger.Err(err).Msg("failed to get pool address")
		return ""
	}

	return address.String()
}

// GetAccount returns the account for the given public key.
func (c *TronClient) GetAccount(
	pubKey common.PubKey,
	height *big.Int,
) (common.Account, error) {
	address, err := pubKey.GetAddress(c.GetChain())
	if err != nil {
		c.logger.Err(err).
			Str("pubkey", pubKey.String()).
			Msg("failed to get pool address")
		return common.Account{}, err
	}
	return c.GetAccountByAddress(address.String(), height)
}

// GetAccountByAddress returns the account for the given address.
func (c *TronClient) GetAccountByAddress(
	address string,
	_ *big.Int,
) (common.Account, error) {
	balance, err := c.api.GetBalance(address)
	if err != nil {
		c.logger.Err(err).Msg("failed to get account")
		return common.Account{}, err
	}

	coins := common.Coins{{
		Asset:    common.TRXAsset,
		Amount:   math.NewUint(balance).Mul(math.NewUint(100)), // 1e6 -> 1e8
		Decimals: 6,
	}}

	account := common.Account{
		Coins: coins,
	}

	if len(c.whitelist) == 0 {
		return account, nil
	}

	for contract, token := range c.whitelist {
		balance, err := c.getTokenBalance(address, contract)
		if err != nil {
			c.logger.Err(err).Msg("failed to get token balance")
			return account, err
		}

		balance, err = common.ConvertDecimals(
			balance, token.Decimals, common.THORChainDecimals,
		)
		if err != nil {
			c.logger.Err(err).Msg("failed to convert token balance")
			return account, err
		}

		coin := common.NewCoin(
			token.Asset(c.config.ChainID),
			math.NewUintFromBigInt(balance),
		)
		coin.Decimals = int64(token.Decimals)

		account.Coins = account.Coins.Add(coin)
	}

	return account, nil
}

// GetBlockScannerHeight returns block scanner height for chain
func (c *TronClient) GetBlockScannerHeight() (int64, error) {
	return c.blockScanner.PreviousHeight(), nil
}

// RollbackBlockScanner rolls back the block scanner to the last observed block
func (c *TronClient) RollbackBlockScanner() error {
	return c.blockScanner.RollbackToLastObserved()
}

// GetLatestTxForVault returns last observed and broadcasted tx for a particular vault and chain
func (c *TronClient) GetLatestTxForVault(vault string) (string, string, error) {
	lastObserved, err := c.signerCacheManager.GetLatestRecordedTx(
		types.InboundCacheKey(vault, c.GetChain().String()),
	)
	if err != nil {
		return "", "", err
	}
	lastBroadcasted, err := c.signerCacheManager.GetLatestRecordedTx(
		types.BroadcastCacheKey(vault, c.GetChain().String()),
	)
	return lastObserved, lastBroadcasted, err
}

// GetConfirmationCount returns the confirmation count for the given tx.
func (c *TronClient) GetConfirmationCount(_ types.TxIn) int64 {
	// https://developers.tron.network/docs/tron-protocol-transaction#transaction-lifecycle
	// We are scanning 19 blocks behind the actual tip, so returning 0 here
	return 0
}

func (c *TronClient) ConfirmationCountReady(_ types.TxIn) bool {
	return true
}

// OnObservedTxIn is called when a new observed tx is received.
func (c *TronClient) OnObservedTxIn(_ types.TxInItem, _ int64) {}

// SignTx returns the signed transaction.
func (c *TronClient) SignTx(
	txOutItem types.TxOutItem,
	_ int64,
) ([]byte, []byte, *types.TxInItem, error) {
	if c.signerCacheManager.HasSigned(txOutItem.CacheHash()) {
		c.logger.Info().
			Interface("tx", txOutItem).
			Msg("transaction already signed, ignoring...")
		return nil, nil, nil, nil
	}

	if len(c.tronScanner.refBlocks) == 0 {
		err := fmt.Errorf("no ref blocks found")
		c.logger.Err(err).Msg("")
		return nil, nil, nil, err
	}

	// TODO: Discuss checkpoint

	if len(txOutItem.Coins) != 1 {
		err := fmt.Errorf("multiple or no coins found")
		c.logger.Err(err).Msg("")
		return nil, nil, nil, err
	}

	coin := txOutItem.Coins[0]
	if coin.IsEmpty() {
		err := fmt.Errorf("coin is empty")
		c.logger.Err(err).Msg("")
		return nil, nil, nil, err
	}

	var err error
	var tronTx api.Transaction

	fromAddress := c.GetAddress(txOutItem.VaultPubKey)

	if coin.Asset == common.TRXAsset {
		// do trx transfer
		tronTx, err = c.api.CreateTransaction(
			fromAddress,
			txOutItem.ToAddress.String(),
			coin.Amount.Quo(math.NewUint(100)).Uint64(), // 1e8 -> 1e6
			txOutItem.Memo,
		)
		if err != nil {
			c.logger.Err(err).Msg("failed to create tx")
			return nil, nil, nil, err
		}
	} else {
		found := false
		var contract string
		var token tokenlist.ERC20Token

		for contract, token = range c.whitelist {
			symbol := strings.ToUpper(token.Symbol + "-" + contract)
			if coin.Asset.Symbol.String() == symbol {
				found = true
				break
			}
		}

		if !found {
			notFoundErr := fmt.Errorf("token not whitelisted")
			c.logger.Err(notFoundErr).Msg("")
			return nil, nil, nil, notFoundErr
		}

		amount := coin.Amount.BigInt()
		amount, err = common.ConvertDecimals(
			amount, common.THORChainDecimals, token.Decimals,
		)
		if err != nil {
			c.logger.Err(err).Msg("failed to convert token balance")
			return nil, nil, nil, err
		}

		tronTx, err = c.createTrc20Transaction(
			fromAddress,
			txOutItem.ToAddress.String(),
			contract,
			*amount,
			*txOutItem.MaxGas[0].Amount.BigInt(), // 1e8 -> 1e6
		)
		if err != nil {
			c.logger.Err(err).Msg("failed to create trc20 tx")
			return nil, nil, nil, err
		}
	}

	// Get the THORChain block timestamp for determinism - all nodes query the same
	// THORChain height and receive the same timestamp, preventing the race condition
	// where nodes at different scan heights would have different refBlocks lists.
	// Using time.Now() or scanner state would cause different transaction hashes
	// and prevent TSS signing consensus.
	thorTimestamp, err := c.bridge.GetBlockTimestamp(txOutItem.Height)
	if err != nil {
		c.logger.Err(err).Msg("failed to get THORChain block timestamp")
		return nil, nil, nil, err
	}

	// Look 2 minutes back to reduce race around the latest scanned ref block
	refTimestamp := thorTimestamp.Add(-2 * time.Minute)

	// Convert to milliseconds for TRON
	timestamp := thorTimestamp.UnixMilli()
	refTimestampMs := refTimestamp.UnixMilli()

	// Find the reference block closest to (but before) the THORChain timestamp
	// This ensures the ref block is deterministic while still being valid for TRON
	index := len(c.tronScanner.refBlocks) - 1
	for i := index; i >= 0; i-- {
		refBlock := c.tronScanner.refBlocks[i]
		if refBlock.Timestamp <= refTimestampMs {
			index = i
			break
		}
	}

	refBlock := c.tronScanner.refBlocks[index]

	refBlockHash := refBlock.Id[16:32]
	refBlockBytes := fmt.Sprintf("%016x", refBlock.Height)[12:16]

	tronTx.RawData.RefBlockBytes = refBlockBytes
	tronTx.RawData.RefBlockHash = refBlockHash
	tronTx.RawData.Timestamp = timestamp
	tronTx.RawData.Expiration = timestamp + TimestampValidity.Milliseconds()
	tronTx.RawData.Data = hex.EncodeToString([]byte(txOutItem.Memo))

	err = tronTx.Rehash()
	if err != nil {
		c.logger.Err(err).Msg("failed to rehash transaction")
		return nil, nil, nil, err
	}

	// ---------------------------------------------------------------------------

	hash, err := hex.DecodeString(tronTx.TxId)
	if err != nil {
		c.logger.Err(err).Msg("failed to decode tx hash")
		return nil, nil, nil, err
	}

	signature, err := c.sign(hash, txOutItem)
	if err != nil {
		c.logger.Err(err).Msg("failed to sign tx")
		return nil, nil, nil, err
	}

	tronTx.Signature = append(tronTx.Signature, hex.EncodeToString(signature))

	txBytes, err := json.Marshal(tronTx)
	if err != nil {
		c.logger.Err(err).Msg("failed to marshal tx")
		return nil, nil, nil, err
	}

	currentHeight, err := c.tronScanner.GetHeight()
	if err != nil {
		c.logger.Err(err).Msg("failed to get latest block")
		return nil, nil, nil, err
	}

	gasCoin := common.NewCoin(
		common.TRXAsset,
		cosmos.NewUint(uint64(txOutItem.GasRate)),
	)

	txIn := types.NewTxInItem(
		currentHeight,
		tronTx.TxId,
		txOutItem.Memo,
		fromAddress,
		txOutItem.ToAddress.String(),
		txOutItem.Coins,
		[]common.Coin{gasCoin},
		txOutItem.VaultPubKey,
		"",
		"",
		nil,
	)

	return txBytes, nil, txIn, nil
}

// BroadcastTx sends the transaction to Tron chain
func (c *TronClient) BroadcastTx(
	txOutItem types.TxOutItem,
	txBytes []byte,
) (string, error) {
	response, err := c.api.BroadcastTransaction(txBytes)
	if err != nil {
		c.logger.Err(err).Msg("failed to broadcast tx")
		return "", err
	}

	// treat dup transaction as success - the transaction was already accepted
	if !response.Result && response.Code == "DUP_TRANSACTION_ERROR" {
		var tx api.Transaction
		if unmarshalErr := json.Unmarshal(txBytes, &tx); unmarshalErr != nil {
			return "", fmt.Errorf("failed to unmarshal tx for dup txid: %w", unmarshalErr)
		}
		c.logger.Info().Str("txid", tx.TxId).Msg("dup transaction treated as success")
		response.Result = true
		response.TxId = tx.TxId
	}

	// check the response
	if !response.Result {
		err = fmt.Errorf("failed to broadcast tx: %s - %s", response.Code, response.Message)
		c.logger.Err(err).Msg("")
		return "", err
	}

	err = c.signerCacheManager.SetSigned(
		txOutItem.CacheHash(),
		txOutItem.CacheVault(c.GetChain()),
		response.TxId,
	)
	if err != nil {
		c.logger.Err(err).
			Interface("tx_out_item", txOutItem).
			Msg("failed to mark tx out item as signed")
	}

	return response.TxId, nil
}

func (c *TronClient) ShouldReportSolvency(height int64) bool {
	return height%10 == 0
}

func (c *TronClient) ReportSolvency(height int64) error {
	if !c.ShouldReportSolvency(height) {
		return nil
	}

	// when block scanner is not healthy, only report from auto-unhalt SolvencyCheckRunner
	// (FetchTxs passes PreviousHeight + 1 from scanBlocks, while SolvencyCheckRunner passes chainHeight)
	if !c.IsBlockScannerHealthy() && height == c.blockScanner.PreviousHeight()+1 {
		return nil
	}

	asgardVaults, err := c.bridge.GetAsgards()
	if err != nil {
		c.logger.Err(err).Msg("failed to get asgard vaults")
		return err
	}

	fee := math.NewUint(c.tronScanner.currentFee)

	// report insolvent asgard vaults,
	// or else all if the chain is halted and all are solvent
	chainVaults := asgardVaults[:0]
	for _, asgard := range asgardVaults {
		if asgard.HasFundsForChain(c.config.ChainID) {
			chainVaults = append(chainVaults, asgard)
		}
	}

	solventMsgs := make([]types.Solvency, 0, len(chainVaults))
	insolventMsgs := make([]types.Solvency, 0, len(chainVaults))
	processedVaults := 0
	for i, vault := range chainVaults {
		acc, err := c.GetAccount(vault.PubKey, big.NewInt(height))
		if err != nil {
			c.logger.Err(err).Msgf("failed to get account balance")
			continue
		}

		msg := types.Solvency{
			// Token balances reported from the RPC appear to be based on finalized state and
			// lagging the observations themselves. We report the finalized height with the
			// solvency message so the handler in thornode will disregard if there is a more
			// recent observation.
			Height: height - FinalityBlocks,
			Chain:  c.config.ChainID,
			PubKey: chainVaults[i].PubKey,
			Coins:  acc.Coins,
		}
		processedVaults++

		currentGasFee := c.config.ChainID.NativeGasToThorchain(fee)
		if runners.IsVaultSolvent(c.config.ChainID, acc, chainVaults[i], currentGasFee) {
			solventMsgs = append(solventMsgs, msg)
			continue
		}

		insolventMsgs = append(insolventMsgs, msg)
	}

	msgs := insolventMsgs
	solvent := false

	// Only if the block scanner is unhealthy (e.g. solvency-halted) and all chain-specific vaults are solvent,
	// report that all the chain-specific vaults are solvent.
	// If there are any insolvent vaults, report only them.
	// Not reporting both solvent and insolvent vaults is to avoid noise (spam):
	// Reporting both could halt-and-unhalt SolvencyHalt in the same THOR block
	// (resetting its height), plus making it harder to know at a glance from solvency reports which vaults were insolvent.
	allVaultsProcessed := len(chainVaults) > 0 && processedVaults == len(chainVaults)
	if !c.IsBlockScannerHealthy() && allVaultsProcessed && len(solventMsgs) == processedVaults {
		msgs = solventMsgs
		solvent = true
	}

	for _, msg := range msgs {
		c.logger.Info().
			Str("asgard", msg.PubKey.String()).
			Interface("coins", msg.Coins).
			Bool("solvent", solvent).
			Msg("reporting solvency")
		// send solvency to thorchain via global queue consumed by the observer
		select {
		case c.globalSolvencyQueue <- msg:
		case <-time.After(constants.ThorchainBlockTime):
			c.logger.Info().Msg("failed to send solvency info to THORChain")
		}
	}

	return nil
}

func (c *TronClient) sign(
	data []byte,
	txOutItem types.TxOutItem,
) ([]byte, error) {
	var err error
	var signature, recovery []byte

	pubkey := txOutItem.VaultPubKey

	if c.localKeyManager.Pubkey().Equals(pubkey) {
		signature, err = c.localKeyManager.Sign(data)
		if err != nil {
			c.logger.Err(err).Msg("unable to sign using localKeyManager")
			return nil, err
		}
	} else {
		signature, recovery, err = c.tssKeyManager.RemoteSign(data, common.SigningAlgoSecp256k1, pubkey.String())
		if err == nil && signature != nil {
			return append(signature, recovery...), nil
		}

		var keysignError tss.KeysignError
		if errors.As(err, &keysignError) {
			if len(keysignError.Blame.BlameNodes) == 0 {
				// TSS doesn't know which node to blame
				c.logger.Err(err).Msg("failed to sign tx")
				return nil, err
			}

			// forward the keysign blame to thorchain
			txId, errPostKeysignFail := c.bridge.PostKeysignFailure(
				keysignError.Blame,
				txOutItem.Height,
				txOutItem.Memo,
				txOutItem.Coins,
				txOutItem.VaultPubKey,
			)
			if errPostKeysignFail != nil {
				return nil, multierror.Append(err, errPostKeysignFail)
			}
			c.logger.Info().
				Str("tx_id", txId.String()).
				Msg("post keysign failure to THORChain")
		}

		c.logger.Err(err).Msg("failed to sign tx")
		return nil, err
	}

	sigPub, err := crypto.Ecrecover(data, signature)
	if err != nil {
		c.logger.Err(err).Msg("failed to get public key")
		return nil, err
	}

	secpPub, err := pubkey.Secp256K1()
	if err != nil {
		c.logger.Err(err).Msg("failed convert public key")
		return nil, err
	}

	ecdsaPub := crypto.FromECDSAPub(secpPub.ToECDSA())

	if !bytes.Equal(sigPub, ecdsaPub) {
		err := fmt.Errorf("unable to verify signature with secp pub key")
		c.logger.Err(err).Msg("")
		return nil, err
	}

	return signature, nil
}

func (c *TronClient) createTrc20Transaction(
	fromAddress, toAddress, contractAddress string,
	amount big.Int,
	feeLimit big.Int,
) (tx api.Transaction, err error) {
	toAddressHex, err := api.ConvertAddress(toAddress)
	if err != nil {
		c.logger.Err(err).Msg("failed to convert address")
		return api.Transaction{}, err
	}

	// convert to proper evm address, so go-ethereum is able to handle it
	toAddressHex = strings.Replace(toAddressHex, "41", "0x", 1)

	data, err := c.abi.Pack(
		"transfer", ethcommon.HexToAddress(toAddressHex), &amount,
	)
	if err != nil {
		c.logger.Err(err).Msg("failed to pack data")
		return tx, err
	}

	tx, err = c.api.TriggerSmartContract(
		fromAddress,
		contractAddress,
		"transfer(address,uint256)",
		hex.EncodeToString(data[4:]),
		feeLimit.Uint64(),
	)
	if err != nil {
		c.logger.Err(err).Msg("failed to create trc20 transfer tx")
		return tx, err
	}

	return tx, nil
}

func (c *TronClient) getTokenBalance(
	address, contract string,
) (*big.Int, error) {
	addresses := []string{address, contract}
	for i, address := range addresses {
		tmp, err := api.ConvertAddress(address)
		if err != nil {
			c.logger.Err(err).Msg("failed to convert address")
			return nil, err
		}

		// convert to proper evm address, so go-ethereum is able to handle it
		addresses[i] = strings.Replace(tmp, "41", "0x", 1)
	}

	data, err := c.abi.Pack(
		"balanceOf", ethcommon.HexToAddress(addresses[0]),
	)
	if err != nil {
		c.logger.Err(err).Msg("failed to pack data")
		return nil, err
	}

	data, err = c.rpc.EthCall(addresses[1], fmt.Sprintf("%x", data))
	if err != nil {
		c.logger.Err(err).Msg("failed to process EthCall")
		return nil, err
	}

	var response rpc.Response
	err = json.Unmarshal(data, &response)
	if err != nil {
		c.logger.Err(err).Msg("failed to unmarshal response")
		return nil, err
	}

	if response.Result == "" {
		err = fmt.Errorf("response result is empty")
		c.logger.Err(err).Msg("")
		return nil, err
	}

	// omit the first two bytes ("0x")
	result, err := hex.DecodeString(response.Result[2:])
	if err != nil {
		c.logger.Err(err).Msg("failed to decode result data")
		return nil, err
	}

	unpacked, err := c.abi.Unpack("balanceOf", result)
	if err != nil {
		c.logger.Err(err).Msg("failed to unpack result data")
		return nil, err
	}

	balance, ok := unpacked[0].(*big.Int)
	if !ok {
		err := fmt.Errorf("failed to convert balance")
		c.logger.Err(err).Msg("")
		return nil, err
	}

	return balance, nil
}
