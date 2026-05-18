package xrp

import (
	"crypto/sha512"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tssp "gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/tss"

	sdkmath "cosmossdk.io/math"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/runners"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/xrp/keymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/xrp/keymanager/secp256k1"

	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	memo "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"

	binarycodec "github.com/Peersyst/xrpl-go/binary-codec"
	"github.com/Peersyst/xrpl-go/xrpl/hash"
	"github.com/Peersyst/xrpl-go/xrpl/queries/account"
	qcommon "github.com/Peersyst/xrpl-go/xrpl/queries/common"
	requests "github.com/Peersyst/xrpl-go/xrpl/queries/transactions"
	"github.com/Peersyst/xrpl-go/xrpl/rpc"
	transactions "github.com/Peersyst/xrpl-go/xrpl/transaction"
	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// Client is a structure to sign and broadcast tx to XRP chain used by signer mostly
type Client struct {
	logger              zerolog.Logger
	cfg                 config.BifrostChainConfiguration
	accts               *XrpMetaDataStore
	tssKeyManager       *tss.KeySign
	localKeyManager     *keymanager.KeyManager
	thorchainBridge     thorclient.ThorchainBridge
	storage             *blockscanner.BlockScannerStorage
	blockScanner        *blockscanner.BlockScanner
	signerCacheManager  *signercache.CacheManager
	xrpScanner          *XrpBlockScanner
	globalSolvencyQueue chan stypes.Solvency
	wg                  *sync.WaitGroup
	stopchan            chan struct{}
	rpcClient           *rpc.Client
	networkID           uint32
}

// NewClient creates a new instance of an XRP-based chain client
func NewClient(
	thorKeys *thorclient.Keys,
	cfg config.BifrostChainConfiguration,
	server *tssp.TssServer,
	thorchainBridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
) (*Client, error) {
	logger := log.With().Str("module", cfg.ChainID.String()).Logger()

	tssKm, err := tss.NewKeySign(server, thorchainBridge)
	if err != nil {
		return nil, fmt.Errorf("fail to create tss signer: %w", err)
	}

	priv, err := thorKeys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get private key: %w", err)
	}

	if thorchainBridge == nil {
		return nil, errors.New("thorchain bridge is nil")
	}

	localKm, err := keymanager.NewKeyManager(priv)
	if err != nil {
		return nil, fmt.Errorf("fail to create key manager: %w", err)
	}

	networkID := uint64(1)
	if cfg.ChainNetwork != "" {
		networkID, err = strconv.ParseUint(cfg.ChainNetwork, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("error parsing chain network as uint, %w", err)
		}
	}

	rpcConfig, err := rpc.NewClientConfig(cfg.RPCHost)
	if err != nil {
		return nil, fmt.Errorf("unable to create rpc config for client, %w", err)
	}
	rpcClient := rpc.NewClient(rpcConfig)

	c := &Client{
		logger:          logger,
		cfg:             cfg,
		accts:           NewXrpMetaDataStore(),
		tssKeyManager:   tssKm,
		localKeyManager: localKm,
		thorchainBridge: thorchainBridge,
		wg:              &sync.WaitGroup{},
		stopchan:        make(chan struct{}),
		rpcClient:       rpcClient,
		networkID:       uint32(networkID),
	}

	var path string // if not set later, will in memory storage
	if len(c.cfg.BlockScanner.DBPath) > 0 {
		path = fmt.Sprintf("%s/%s", c.cfg.BlockScanner.DBPath, c.cfg.BlockScanner.ChainID)
	}
	c.storage, err = blockscanner.NewBlockScannerStorage(path, c.cfg.ScannerLevelDB)
	if err != nil {
		return nil, fmt.Errorf("fail to create scan storage: %w", err)
	}

	c.xrpScanner, err = NewXrpBlockScanner(
		c.cfg.RPCHost,
		c.cfg.BlockScanner,
		c.storage,
		c.thorchainBridge,
		m,
		c.ReportSolvency,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cosmos scanner: %w", err)
	}

	c.blockScanner, err = blockscanner.NewBlockScanner(c.cfg.BlockScanner, c.storage, m, c.thorchainBridge, c.xrpScanner)
	if err != nil {
		return nil, fmt.Errorf("failed to create block scanner: %w", err)
	}

	signerCacheManager, err := signercache.NewSignerCacheManager(c.storage.GetInternalDb())
	if err != nil {
		return nil, fmt.Errorf("fail to create signer cache manager")
	}
	c.signerCacheManager = signerCacheManager

	return c, nil
}

// Start Xrp chain client
func (c *Client) Start(globalTxsQueue chan stypes.TxIn, globalErrataQueue chan stypes.ErrataBlock, globalSolvencyQueue chan stypes.Solvency, globalNetworkFeeQueue chan common.NetworkFee) {
	c.globalSolvencyQueue = globalSolvencyQueue
	c.xrpScanner.globalNetworkFeeQueue = globalNetworkFeeQueue
	c.tssKeyManager.Start()
	c.blockScanner.Start(globalTxsQueue, globalNetworkFeeQueue)
	c.wg.Add(1)
	go runners.SolvencyCheckRunner(c.GetChain(), c, c.thorchainBridge, c.stopchan, c.wg, constants.ThorchainBlockTime)
}

// Stop Xrp chain client
func (c *Client) Stop() {
	c.tssKeyManager.Stop()
	c.blockScanner.Stop()
	close(c.stopchan)
	c.wg.Wait()
}

// GetConfig return the configuration used by Xrp chain client
func (c *Client) GetConfig() config.BifrostChainConfiguration {
	return c.cfg
}

func (c *Client) IsBlockScannerHealthy() bool {
	return c.blockScanner.IsHealthy()
}

func (c *Client) GetChain() common.Chain {
	return c.cfg.ChainID
}

func (c *Client) GetHeight() (int64, error) {
	return c.xrpScanner.GetHeight()
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
	addr, err := poolPubKey.GetAddress(c.GetChain())
	if err != nil {
		c.logger.Err(err).Str("pool_pub_key", poolPubKey.String()).Msg("fail to get pool address")
		return ""
	}
	return addr.String()
}

func (c *Client) GetAccount(pkey common.PubKey, height *big.Int) (common.Account, error) {
	addr, err := pkey.GetAddress(c.GetChain())
	if err != nil {
		return common.Account{}, fmt.Errorf("failed to convert address (%s) from bech32: %w", pkey, err)
	}
	return c.GetAccountByAddress(addr.String(), height)
}

func (c *Client) GetAccountByAddress(address string, height *big.Int) (common.Account, error) {
	aiReq := account.InfoRequest{
		Account:     txtypes.Address(address),
		LedgerIndex: qcommon.Current, // Query current/non-closed/non-validated ledger
	}
	if height != nil && height.Cmp(big.NewInt(0)) > 0 {
		aiReq.LedgerIndex = qcommon.LedgerIndex(height.Int64())
	}
	aiResp, err := c.rpcClient.GetAccountInfo(&aiReq)
	if err != nil {
		return common.Account{}, err
	}

	balance := sdkmath.NewUint(aiResp.AccountData.Balance.Uint64())
	coins, err := fromXrpToThorchain(txtypes.XRPCurrencyAmount(balance.Uint64()))
	if err != nil {
		return common.Account{}, err
	}

	return common.Account{
		Sequence:      int64(aiResp.AccountData.Sequence),
		AccountNumber: 0,
		Coins:         common.NewCoins(coins),
	}, nil
}

func (c *Client) processOutboundTx(tx stypes.TxOutItem) (*transactions.Payment, error) {
	fromAddr, err := tx.VaultPubKey.GetAddress(c.GetChain())
	if err != nil {
		return nil, fmt.Errorf("failed to convert address (%s) to bech32: %w", tx.VaultPubKey.String(), err)
	}

	if len(tx.Coins) != 1 {
		return nil, fmt.Errorf("cannot send more than 1 set of coins, trying %d set coins", len(tx.Coins))
	}

	signingPubKey, err := tx.VaultPubKey.Secp256K1()
	if err != nil {
		return nil, err
	}

	coin, err := fromThorchainToXrp(tx.Coins[0])
	if err != nil {
		return nil, err
	}

	payment := transactions.Payment{
		BaseTx: transactions.BaseTx{
			Account:       txtypes.Address(fromAddr),
			SigningPubKey: hex.EncodeToString(signingPubKey.SerializeCompressed()),
		},
		Amount:      coin,
		Destination: txtypes.Address(tx.ToAddress.String()),
	}

	// Network id is required when > 1024 (i.e. mocknet/standalone) and must not be included for mainnet/testnet
	if c.networkID > 1024 {
		payment.BaseTx.NetworkID = c.networkID
	}

	return &payment, nil
}

// SignTx sign the the given TxArrayItem
func (c *Client) SignTx(tx stypes.TxOutItem, thorchainHeight int64) (signedTx, checkpoint []byte, _ *stypes.TxInItem, err error) {
	defer func() {
		if err != nil {
			var keysignError tss.KeysignError
			if errors.As(err, &keysignError) {
				if len(keysignError.Blame.BlameNodes) == 0 {
					c.logger.Err(err).Msg("TSS doesn't know which node to blame")
					return
				}

				// key sign error forward the keysign blame to thorchain
				var txID common.TxID
				txID, err = c.thorchainBridge.PostKeysignFailure(keysignError.Blame, thorchainHeight, tx.Memo, tx.Coins, tx.VaultPubKey)
				if err != nil {
					c.logger.Err(err).Msg("fail to post keysign failure to THORChain")
					return
				}
				c.logger.Info().Str("tx_id", txID.String()).Msgf("post keysign failure to thorchain")
			}
			c.logger.Err(err).Msg("failed to sign tx")
			return
		}
	}()

	if c.signerCacheManager.HasSigned(tx.CacheHash()) {
		c.logger.Info().Interface("tx", tx).Msg("transaction already signed, ignoring...")
		return nil, nil, nil, nil
	}

	msg, err := c.processOutboundTx(tx)
	if err != nil {
		c.logger.Err(err).Msg("failed to process outbound tx")
		return nil, nil, nil, err
	}

	currentHeight, err := c.xrpScanner.GetHeight()
	if err != nil {
		c.logger.Err(err).Msg("fail to get current block height")
		return nil, nil, nil, err
	}

	// the metadata is stored as the transaction checkpoint, if it is set deserialize it
	// so we only retry with the same account number and sequence to avoid double spend
	meta := XrpMetadata{}
	if tx.Checkpoint != nil {
		if err = json.Unmarshal(tx.Checkpoint, &meta); err != nil {
			c.logger.Err(err).Msg("fail to unmarshal checkpoint")
			return nil, nil, nil, err
		}
	} else {
		// Check if we have XrpMetadata for the current block height before fetching it
		meta = c.accts.Get(tx.VaultPubKey)
		if currentHeight > meta.BlockHeight {
			var acc common.Account
			acc, err = c.GetAccount(tx.VaultPubKey, big.NewInt(0))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("fail to get account info: %w", err)
			}
			// Only update local sequence # if it is less than what is on chain
			// When local sequence # is larger than on chain , that could be there are transactions in mempool not commit yet
			if meta.SeqNumber <= acc.Sequence {
				meta = XrpMetadata{
					SeqNumber:   acc.Sequence,
					BlockHeight: currentHeight,
				}
				c.accts.Set(tx.VaultPubKey, meta)
			}
		}
	}

	// serialize the checkpoint for later
	checkpointBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to marshal checkpoint: %w", err)
	}

	// gas rate from gas rate units to THORChain decimals
	gasRate := c.cfg.ChainID.NativeGasToThorchain(cosmos.NewUint(uint64(tx.GasRate)))
	if gasRate.IsZero() {
		gasRate = cosmos.OneUint()
	}
	gasCoin := common.NewCoin(common.XRPAsset, cosmos.NewUint(uint64(tx.GasRate)))

	// TODO: Optionally
	// (to avoid overpaying for already-queued TxOutItems when changing units),
	// rather than assuming Transaction Size 1 here,
	// use the tx.MaxGas instead like GAIA?

	feeCurrency, err := fromThorchainToXrp(common.NewCoin(common.XRPAsset, gasRate))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to get fee: %w", err)
	}

	fee, ok := feeCurrency.(txtypes.XRPCurrencyAmount)
	if !ok {
		return nil, nil, nil, fmt.Errorf("fail to cast fee to xrp currency amount")
	}

	msg.Sequence = uint32(meta.SeqNumber)
	msg.Fee = fee
	// For memoless outbounds, skip adding memo to the transaction
	if tx.Memo != "" {
		msg.BaseTx.Memos = []txtypes.MemoWrapper{
			{
				Memo: txtypes.Memo{
					MemoData: hex.EncodeToString([]byte(tx.Memo)),
				},
			},
		}
	}

	txBytes, err := c.signMsg(msg, tx.VaultPubKey)
	if err != nil {
		return nil, checkpointBytes, nil, fmt.Errorf("failed to sign message: %w", err)
	}

	// create instant observation
	fromAddr, err := tx.VaultPubKey.GetAddress(c.GetChain())
	if err != nil {
		c.logger.Err(err).Msg("error retrieving from address for instant observation")
		return txBytes, nil, nil, nil
	}
	blobHex := hex.EncodeToString(txBytes)
	txID, err := hash.SignTxBlob(blobHex)
	if err != nil {
		c.logger.Err(err).Msg("error retrieving txid for instant observation")
		return txBytes, nil, nil, nil
	}
	txIn := stypes.NewTxInItem(
		currentHeight,
		txID,
		tx.Memo,
		fromAddr.String(),
		tx.ToAddress.String(),
		tx.Coins,
		common.Gas([]common.Coin{gasCoin}),
		tx.VaultPubKey,
		"",
		"",
		nil,
	)

	return txBytes, nil, txIn, nil
}

// signMsg takes a payment msg and signs it using either private key or TSS.
func (c *Client) signMsg(
	payment *transactions.Payment,
	pubkey common.PubKey,
) ([]byte, error) {
	xrpPubKey, err := pubkey.Secp256K1()
	if err != nil {
		return nil, err
	}

	flatTx := payment.Flatten()
	encodedTx, err := binarycodec.EncodeForSigning(flatTx)
	if err != nil {
		return nil, err
	}

	signBytes, err := hex.DecodeString(encodedTx)
	if err != nil {
		return nil, err
	}

	var derSignature []byte
	if c.localKeyManager.PublicKeyHex == hex.EncodeToString(xrpPubKey.SerializeCompressed()) {
		derSignature, err = c.localKeyManager.Sign(signBytes)
		if err != nil {
			return nil, fmt.Errorf("unable to sign using localKeyManager: %w", err)
		}
	} else {
		hashedMsg := sha512.Sum512(signBytes)
		var signature []byte
		signature, _, err = c.tssKeyManager.RemoteSign(hashedMsg[:32], common.SigningAlgoSecp256k1, pubkey.String())
		if err != nil {
			c.logger.Err(err).Msg("xrp remote sign")
			return nil, fmt.Errorf("error, xrp remote sign: %w", err)
		}
		if signature == nil {
			c.logger.Error().Msg("xrp remote sign, signature is nil")
			return nil, fmt.Errorf("error, xrp remote sign, signature is nil")
		}
		if len(signature) < 64 {
			c.logger.Error().Msg("xrp remote sign, signature is <64 bytes")
			return nil, fmt.Errorf("error, xrp remote sign, signature is <64 bytes")
		}
		// Extract R and S from the signature
		// The signature is in the format R || S || V where V is the recovery ID
		r := new(big.Int).SetBytes(signature[:32])
		s := new(big.Int).SetBytes(signature[32:64])

		// Create an ECDSASignature struct for ASN.1 DER encoding
		sig := secp256k1.ECDSASignature{
			R: r,
			S: s,
		}

		// Encode the signature in DER format
		derSignature, err = asn1.Marshal(sig)
		if err != nil {
			return nil, fmt.Errorf("failed to DER encode signature: %w", err)
		}
	}
	flatTx["TxnSignature"] = hex.EncodeToString(derSignature) // use flatTx so we don't need to call autofill again

	// Ensure the signature is valid
	if !verifySignature(signBytes, derSignature, xrpPubKey.SerializeCompressed()) {
		return nil, fmt.Errorf("unable to verify signature with secpPubKey")
	}

	txHex, err := binarycodec.Encode(flatTx)
	if err != nil {
		return nil, err
	}

	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	return txBytes, nil
}

func verifySignature(signBytes, signature, compressedPubKey []byte) bool {
	// Hash the transaction data
	messageHashFull := sha512.Sum512(signBytes)
	messageHash := messageHashFull[:32]

	// Parse the DER signature
	var sig secp256k1.ECDSASignature
	_, err := asn1.Unmarshal(signature, &sig)
	if err != nil {
		return false
	}

	// Prepare signature in the format expected by VerifySignature
	// Ensure R and S are padded to 32 bytes
	rBytes := secp256k1.PaddedBytes(sig.R, 32)
	sBytes := secp256k1.PaddedBytes(sig.S, 32)

	// Verify the signature
	return ethcrypto.VerifySignature(
		compressedPubKey,
		messageHash,
		append(rBytes, sBytes...),
	)
}

func (c *Client) txNeedsBroadcast(txHash string) bool {
	var txResponse *requests.TxResponse

	// Request the transaction from the server
	res, err := c.rpcClient.Request(&requests.TxRequest{
		Transaction: txHash,
	})
	if err != nil {
		return true
	}

	err = res.GetResult(&txResponse)
	if err != nil {
		// unexpected error, log it
		c.logger.Info().AnErr("error", err).Msg("error get results of tx request")
		return true
	}

	return txResponse == nil || !txResponse.Validated
}

func (c *Client) broadcastTx(txBlob string) error {
	response, err := c.rpcClient.Submit(txBlob, true)
	if err != nil {
		return fmt.Errorf("broadcast msg failed, %w", err)
	}

	// Only add the transaction to signer cache when it is sure the transaction has been broadcast successfully.
	// So for other scenario , like transaction already in mempool , invalid account sequence # , the transaction can be rescheduled , and retried
	// If we get tefPAST_SEQ, tx is already in current ledger, most likely by another validator.
	if response.EngineResult != "tesSUCCESS" && response.EngineResult != "tefPAST_SEQ" {
		c.logger.Info().Interface("broadcastRes", response).Msg("XRP BroadcastTx failed")
		return fmt.Errorf("transaction failed to submit with engine result: %s", response.EngineResult)
	}
	c.logger.Info().Interface("broadcastRes", response).Msg("XRP BroadcastTx success")
	return nil
}

// BroadcastTx is to broadcast the tx to cosmos chain
func (c *Client) BroadcastTx(tx stypes.TxOutItem, txBytes []byte) (string, error) {
	txBlob := hex.EncodeToString(txBytes)
	txHash, err := hash.SignTxBlob(txBlob)
	if err != nil {
		c.logger.Error().Msg("error hashing txBlob on broadcast")
		return "", err
	}

	// if tx has already been broadcasted, don't try again, it will error with engine result: tefPAST_SEQ
	if c.txNeedsBroadcast(txHash) {
		if err = c.broadcastTx(txBlob); err != nil {
			return "", err
		}
	}

	c.accts.SeqInc(tx.VaultPubKey)
	if err = c.signerCacheManager.SetSigned(tx.CacheHash(), tx.CacheVault(c.GetChain()), txHash); err != nil {
		c.logger.Err(err).Msg("fail to set signer cache")
	}

	return txHash, nil
}

// ConfirmationCountReady xrp chain has almost instant finality, so doesn't need to wait for confirmation
func (c *Client) ConfirmationCountReady(txIn stypes.TxIn) bool {
	return true
}

// GetConfirmationCount determine how many confirmations are required
// NOTE: Xrp chains are instant finality, so confirmations are not needed.
// If the transaction was successful, we know it is included in a block and thus immutable.
func (c *Client) GetConfirmationCount(txIn stypes.TxIn) int64 {
	return 0
}

func (c *Client) ReportSolvency(blockHeight int64) error {
	if !c.ShouldReportSolvency(blockHeight) {
		return nil
	}

	// when block scanner is not healthy, only report from auto-unhalt SolvencyCheckRunner
	// (FetchTxs passes PreviousHeight + 1 from scanBlocks, while SolvencyCheckRunner passes chainHeight)
	if !c.IsBlockScannerHealthy() && blockHeight == c.blockScanner.PreviousHeight()+1 {
		return nil
	}

	// fetch all asgard vaults
	asgardVaults, err := c.thorchainBridge.GetAsgards()
	if err != nil {
		return fmt.Errorf("fail to get asgards,err: %w", err)
	}

	// 1x estimated gas cost breathing room (transaction size 1), from gas rate units gas price to THORChain (1e8) format
	currentGasFee := c.cfg.ChainID.NativeGasToThorchain(c.xrpScanner.lastFee)

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
		acct, err = c.GetAccount(chainVaults[i].PubKey, new(big.Int).SetInt64(blockHeight))
		if err != nil {
			c.logger.Err(err).Msgf("fail to get account balance")
			continue
		}

		msg := stypes.Solvency{
			Height: blockHeight,
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
			c.logger.Info().Msgf("fail to send solvency info to THORChain, timeout")
		}
	}
	return nil
}

func (c *Client) ShouldReportSolvency(height int64) bool {
	// Block time on XRP generally hovers between 3-5 seconds (15
	// blocks/min). Since the last fee is used as a buffer we also want to ensure that is
	// non-zero (enough blocks have been seen) before checking insolvency to avoid false
	// positives.
	return height%c.cfg.SolvencyBlocks == 0 && !c.xrpScanner.lastFee.IsZero()
}

// OnObservedTxIn update the signer cache (in case we haven't already)
func (c *Client) OnObservedTxIn(txIn stypes.TxInItem, blockHeight int64) {
	m, err := memo.ParseMemo(common.LatestVersion, txIn.Memo)
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
