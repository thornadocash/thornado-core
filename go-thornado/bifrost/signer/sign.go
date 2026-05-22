package signer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	sdktypes "github.com/cosmos/cosmos-sdk/types"

	"github.com/cenkalti/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"

	"github.com/thornadocash/go-thornado/app"
	"github.com/thornadocash/go-thornado/bifrost/blockscanner"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/observer"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients"
	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/bifrost/tss"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

// Signer will pull the tx out from thornado and then forward it to chain
type Signer struct {
	logger               zerolog.Logger
	cfg                  config.Bifrost
	wg                   *sync.WaitGroup
	thornadoBridge       thornadoclient.ThornadoBridge
	stopChan             chan struct{}
	blockScanner         *blockscanner.BlockScanner
	thornadoBlockScanner *ThornadoBlockScan
	chains               map[common.Chain]chainclients.ChainClient
	storage              SignerStorage
	m                    *metrics.Metrics
	errCounter           *prometheus.CounterVec
	tssKeygen            *tss.KeyGen
	pubkeyMgr            pubkeymanager.PubKeyValidator
	constantsProvider    *ConstantsProvider
	localPubKeyECDSA     common.PubKey
	localPubKeyEDDSA     common.PubKey
	tssKeysignMetricMgr  *metrics.TssKeysignMetricMgr
	observer             *observer.Observer
	pipeline             *pipeline
}

// NewSigner create a new instance of signer
func NewSigner(cfg config.Bifrost,
	thornadoBridge thornadoclient.ThornadoBridge,
	thorKeys *thornadoclient.Keys,
	localState storage.LocalStateManager,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	chains map[common.Chain]chainclients.ChainClient,
	m *metrics.Metrics,
	tssKeysignMetricMgr *metrics.TssKeysignMetricMgr,
	obs *observer.Observer,
) (*Signer, error) {
	storage, err := NewSignerStore(cfg.Signer.SignerDbPath, cfg.Signer.LevelDB, thornadoBridge.GetConfig().SignerPasswd)
	if err != nil {
		return nil, fmt.Errorf("fail to create thornado scan storage: %w", err)
	}
	if tssKeysignMetricMgr == nil {
		return nil, fmt.Errorf("fail to create signer , tss keysign metric manager is nil")
	}
	var na *ttypes.NodeAccount
	for i := 0; i < 300; i++ { // wait for 5 min before timing out
		var signerAddr sdktypes.AccAddress
		signerAddr, err = thorKeys.GetSignerInfo().GetAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to get address from thorKeys signer: %w", err)
		}
		na, err = thornadoBridge.GetNodeAccount(signerAddr.String())
		if err != nil {
			return nil, fmt.Errorf("fail to get node account from thornado,err:%w", err)
		}

		if !na.PubKeySet.Secp256k1.IsEmpty() {
			break
		}
		time.Sleep(constants.ThornadoBlockTime)
		log.Info().Msg("Waiting for node account to be registered...")
	}

	if na.PubKeySet.Secp256k1.IsEmpty() {
		return nil, fmt.Errorf("unable to find pubkey for this node account. exiting... ")
	}
	pubkeyMgr.AddNodePubKey(na.PubKeySet.Secp256k1, common.SigningAlgoSecp256k1)

	if !na.PubKeySet.Ed25519.IsEmpty() {
		pubkeyMgr.AddNodePubKey(na.PubKeySet.Ed25519, common.SigningAlgoEd25519)
	}

	cfg.Signer.BlockScanner.ChainID = common.Thornado // hard code to thornado

	// Create pubkey manager and add our private key
	thornadoBlockScanner, err := NewThornadoBlockScan(cfg.Signer.BlockScanner, storage, thornadoBridge, m, pubkeyMgr)
	if err != nil {
		return nil, fmt.Errorf("fail to create thornado block scan: %w", err)
	}

	blockScanner, err := blockscanner.NewBlockScanner(cfg.Signer.BlockScanner, storage, m, thornadoBridge, thornadoBlockScanner)
	if err != nil {
		return nil, fmt.Errorf("fail to create block scanner: %w", err)
	}

	kg, err := tss.NewTssKeyGen(thorKeys, localState, thornadoBridge)
	if err != nil {
		return nil, fmt.Errorf("fail to create Tss Key gen,err:%w", err)
	}
	constantProvider := NewConstantsProvider(thornadoBridge)
	return &Signer{
		logger:               log.With().Str("module", "signer").Logger(),
		cfg:                  cfg,
		wg:                   &sync.WaitGroup{},
		stopChan:             make(chan struct{}),
		blockScanner:         blockScanner,
		thornadoBlockScanner: thornadoBlockScanner,
		chains:               chains,
		m:                    m,
		storage:              storage,
		errCounter:           m.GetCounterVec(metrics.SignerError),
		pubkeyMgr:            pubkeyMgr,
		thornadoBridge:       thornadoBridge,
		tssKeygen:            kg,
		constantsProvider:    constantProvider,
		localPubKeyECDSA:     na.PubKeySet.Secp256k1,
		localPubKeyEDDSA:     na.PubKeySet.Ed25519,
		tssKeysignMetricMgr:  tssKeysignMetricMgr,
		observer:             obs,
	}, nil
}

func (s *Signer) getChain(chainID common.Chain) (chainclients.ChainClient, error) {
	chain, ok := s.chains[chainID]
	if !ok {
		s.logger.Debug().Str("chain", chainID.String()).Msg("is not supported yet")
		return nil, errors.New("not supported")
	}
	return chain, nil
}

// Start signer process
func (s *Signer) Start() error {
	s.wg.Add(1)
	go s.processTxnOut(s.thornadoBlockScanner.GetTxOutMessages(), 1)

	s.wg.Add(1)
	go s.processKeygen(s.thornadoBlockScanner.GetKeygenMessages())

	s.wg.Add(1)
	go s.signTransactions()

	s.blockScanner.Start(nil, nil)
	return nil
}

func (s *Signer) shouldSign(tx types.TxOutItem) bool {
	return s.pubkeyMgr.HasPubKey(tx.VaultPubKey) || s.pubkeyMgr.HasPubKey(tx.VaultPubKeyEddsa)
}

// signTransactions - looks for work to do by getting a list of all unsigned
// transactions stored in the storage
func (s *Signer) signTransactions() {
	s.logger.Info().Msg("start to sign transactions")
	defer s.logger.Info().Msg("stop to sign transactions")
	defer s.wg.Done()
	for {
		select {
		case <-s.stopChan:
			return
		default:
			// When Thornado is catching up , bifrost might get stale data from thornado , thus it shall pause signing
			catchingUp, err := s.thornadoBridge.IsCatchingUp()
			if err != nil {
				s.logger.Error().Err(err).Msg("fail to get thornado sync status")
				time.Sleep(constants.ThornadoBlockTime)
				break // this will break select
			}
			if !catchingUp {
				s.processTransactions()
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func runWithContext(ctx context.Context, fn func() ([]byte, *types.TxInItem, error)) ([]byte, *types.TxInItem, error) {
	ch := make(chan error, 1)
	var checkpoint []byte
	var txIn *types.TxInItem
	go func() {
		var err error
		checkpoint, txIn, err = fn()
		ch <- err
	}()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case err := <-ch:
		return checkpoint, txIn, err
	}
}

func (s *Signer) processTransactions() {
	const signerConcurrency = int64(10)

	// if previously set to different concurrency, drain existing signings
	if s.pipeline != nil && s.pipeline.concurrency != signerConcurrency {
		s.pipeline.Wait()
		s.pipeline = nil
	}

	// if not set, or set to different concurrency, create new pipeline
	if s.pipeline == nil {
		var err error
		s.pipeline, err = newPipeline(signerConcurrency)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to create new pipeline")
			return
		}
	}

	// process transactions
	s.pipeline.SpawnSignings(s, s.thornadoBridge)
}

// processTxnOut processes outbound TxOuts and save them to storage
func (s *Signer) processTxnOut(ch <-chan types.TxOut, idx int) {
	s.logger.Info().Int("idx", idx).Msg("start to process tx out")
	defer s.logger.Info().Int("idx", idx).Msg("stop to process tx out")
	defer s.wg.Done()
	for {
		select {
		case <-s.stopChan:
			return
		case txOut, more := <-ch:
			if !more {
				return
			}
			s.logger.Info().Msgf("Received a TxOut Array of %v from the Thornado", txOut)
			items := make([]TxOutStoreItem, 0, len(txOut.TxArray))

			for i, tx := range txOut.TxArray {
				items = append(items, NewTxOutStoreItem(txOut.Height, tx.TxOutItem(txOut.Height), int64(i)))
			}
			if err := s.storage.Batch(items); err != nil {
				s.logger.Error().Err(err).Msg("fail to save tx out items to storage")
			}
		}
	}
}

func (s *Signer) processKeygen(ch <-chan ttypes.KeygenBlock) {
	s.logger.Info().Msg("start to process keygen")
	defer s.logger.Info().Msg("stop to process keygen")
	defer s.wg.Done()
	for {
		select {
		case <-s.stopChan:
			return
		case keygenBlock, more := <-ch:
			if !more {
				return
			}
			s.logger.Info().Interface("keygenBlock", keygenBlock).Msg("received a keygen block from thornado")
			s.processKeygenBlock(keygenBlock)
		}
	}
}

func (s *Signer) scheduleKeygenRetry(keygenBlock ttypes.KeygenBlock) bool {
	churnRetryInterval, err := s.thornadoBridge.GetMimir(constants.ChurnRetryInterval.String())
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get churn retry mimir")
		return false
	}
	if churnRetryInterval <= 0 {
		churnRetryInterval = constants.NewConstantValue().GetInt64Value(constants.ChurnRetryInterval)
	}
	keygenRetryInterval, err := s.thornadoBridge.GetMimir(constants.KeygenRetryInterval.String())
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get keygen retries mimir")
		return false
	}
	if keygenRetryInterval <= 0 {
		return false
	}

	// sanity check the retry interval is at least 1.5x the timeout
	retryIntervalDuration := time.Duration(keygenRetryInterval) * constants.ThornadoBlockTime
	if retryIntervalDuration <= s.cfg.Signer.KeygenTimeout*3/2 {
		s.logger.Error().
			Stringer("retryInterval", retryIntervalDuration).
			Stringer("keygenTimeout", s.cfg.Signer.KeygenTimeout).
			Msg("retry interval too short")
		return false
	}

	height, err := s.thornadoBridge.GetBlockHeight()
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get last chain height")
		return false
	}

	// target retry height is the next keygen retry interval over the keygen block height
	targetRetryHeight := (keygenRetryInterval - ((height - keygenBlock.Height) % keygenRetryInterval)) + height

	// skip trying close to churn retry
	if targetRetryHeight > keygenBlock.Height+churnRetryInterval-keygenRetryInterval {
		return false
	}

	go func() {
		// every block, try to start processing again
		for {
			time.Sleep(constants.ThornadoBlockTime)
			currentHeight, err := s.thornadoBridge.GetBlockHeight()
			if err != nil {
				s.logger.Error().Err(err).Msg("fail to get last chain height")
			}
			if currentHeight >= targetRetryHeight {
				s.logger.Info().
					Interface("keygenBlock", keygenBlock).
					Int64("currentHeight", currentHeight).
					Msg("retrying keygen")
				s.processKeygenBlock(keygenBlock)
				return
			}
		}
	}()

	s.logger.Info().
		Interface("keygenBlock", keygenBlock).
		Int64("retryHeight", targetRetryHeight).
		Msg("scheduled keygen retry")

	return true
}

func (s *Signer) processKeygenBlock(keygenBlock ttypes.KeygenBlock) {
	s.logger.Info().Interface("keygenBlock", keygenBlock).Msg("processing keygen block")

	// NOTE: in practice there is only one keygen in the keygen block
	for _, keygenReq := range keygenBlock.Keygens {
		keygenStart := time.Now()
		pubKey, blame, err := s.tssKeygen.GenerateNewKey(keygenBlock.Height, keygenReq.GetMembers(), common.Chains{common.BTCChain})
		if len(blame) > 0 {
			for _, b := range blame {
				s.logger.Error().
					Str("reason", b.FailReason).
					Interface("nodes", b.BlameNodes).
					Msg("keygen blame")
			}
		}
		keygenTime := time.Since(keygenStart).Milliseconds()

		if err != nil {
			s.errCounter.WithLabelValues("fail_to_keygen_pubkey", "").Inc()
			s.logger.Error().Err(err).Msg("fail to generate new pubkey")
		}

		// re-enqueue the keygen block to retry if we failed to generate a key
		if pubKey.Secp256k1.IsEmpty() {
			if s.scheduleKeygenRetry(keygenBlock) {
				return
			}
			s.logger.Error().Interface("keygenBlock", keygenBlock).Msg("done with keygen retries")
		}

		// generate a verification signature to ensure we can sign with the new key
		secp256k1Sig := s.secp256k1VerificationSignature(pubKey.Secp256k1)

		if err = s.sendKeygenToThornado(keygenBlock.Height, pubKey.Secp256k1, secp256k1Sig, blame, keygenReq.GetMembers(), keygenReq.Type, keygenTime, pubKey.Ed25519); err != nil {
			s.errCounter.WithLabelValues("fail_to_broadcast_keygen", "").Inc()
			s.logger.Error().Err(err).Msg("fail to broadcast keygen")
		}

		// monitor the new pubkey and any new members
		if !pubKey.Secp256k1.IsEmpty() {
			s.pubkeyMgr.AddPubKey(pubKey.Secp256k1, true, common.SigningAlgoSecp256k1)
		}
		if !pubKey.Ed25519.IsEmpty() {
			s.pubkeyMgr.AddPubKey(pubKey.Ed25519, true, common.SigningAlgoEd25519)
		}
		for _, pk := range keygenReq.GetMembers() {
			s.pubkeyMgr.AddPubKey(pk, false, common.SigningAlgoSecp256k1)
		}
	}
}

func (s *Signer) secp256k1VerificationSignature(pk common.PubKey) []byte {
	return nil
}

func (s *Signer) sendKeygenToThornado(height int64, poolPk common.PubKey, secp256k1Signature []byte, blame []ttypes.Blame, input common.PubKeys, keygenType ttypes.KeygenType, keygenTime int64, poolPubKeyEddsa common.PubKey) error {
	// collect supported chains in the configuration
	chains := common.Chains{
		common.Thornado,
	}
	for chain, chainCfg := range s.cfg.GetChains() {
		if !chainCfg.OptToRetire && !chainCfg.Disabled {
			chains = append(chains, chain)
		}
	}

	// make a best effort to add encrypted keyshares to the message
	var keyshares []byte
	var keysharesEddsa []byte
	var keysharesFrost []byte
	var err error
	if s.cfg.Signer.BackupKeyshares {
		if !poolPk.IsEmpty() {
			keysharePath := filepath.Join(app.DefaultNodeHome, fmt.Sprintf("localstate-%s.json", poolPk))
			frostRaw, isFrost, readErr := frostKeyshareRawFromLocalStatePath(keysharePath)
			switch {
			case readErr != nil:
				s.logger.Error().Err(readErr).Msg("fail to read keyshares")
			case isFrost:
				keysharesFrost, err = tss.EncryptRawKeyshares(frostRaw, os.Getenv("SIGNER_SEED_PHRASE"))
				if err != nil {
					s.logger.Error().Err(err).Msg("fail to encrypt frost keyshares")
				}
			default:
				keyshares, err = tss.EncryptKeyshares(keysharePath, os.Getenv("SIGNER_SEED_PHRASE"))
				if err != nil {
					s.logger.Error().Err(err).Msg("fail to encrypt secp256k1 keyshares")
				}
			}
		}
		if !poolPubKeyEddsa.IsEmpty() {
			keysharesEddsa, err = tss.EncryptKeyshares(
				filepath.Join(app.DefaultNodeHome, fmt.Sprintf("localstate-%s.json", poolPubKeyEddsa)),
				os.Getenv("SIGNER_SEED_PHRASE"),
			)
			if err != nil {
				s.logger.Error().Err(err).Msg("fail to encrypt eddsa keyshares")
			}
		}
	}

	var keygenMsg sdktypes.Msg
	if len(keysharesFrost) > 0 {
		if bridge, ok := s.thornadoBridge.(interface {
			GetKeygenStdTxWithFrost(common.PubKey, []byte, []byte, []ttypes.Blame, common.PubKeys, ttypes.KeygenType, common.Chains, int64, int64, common.PubKey, []byte, []byte) (sdktypes.Msg, error)
		}); ok {
			keygenMsg, err = bridge.GetKeygenStdTxWithFrost(poolPk, secp256k1Signature, keyshares, blame, input, keygenType, chains, height, keygenTime, poolPubKeyEddsa, keysharesEddsa, keysharesFrost)
		} else {
			keygenMsg, err = s.thornadoBridge.GetKeygenStdTx(poolPk, secp256k1Signature, keyshares, blame, input, keygenType, chains, height, keygenTime, poolPubKeyEddsa, keysharesEddsa)
		}
	} else {
		keygenMsg, err = s.thornadoBridge.GetKeygenStdTx(poolPk, secp256k1Signature, keyshares, blame, input, keygenType, chains, height, keygenTime, poolPubKeyEddsa, keysharesEddsa)
	}
	if err != nil {
		return fmt.Errorf("fail to get keygen id: %w", err)
	}
	strHeight := strconv.FormatInt(height, 10)

	bf := backoff.NewExponentialBackOff()
	bf.MaxElapsedTime = time.Minute
	return backoff.Retry(func() error {
		txID, err := s.thornadoBridge.Broadcast(keygenMsg)
		if err != nil {
			s.logger.Warn().Err(err).Msg("fail to send keygen tx to thornado")
			s.errCounter.WithLabelValues("fail_to_send_to_thornado", strHeight).Inc()
			return fmt.Errorf("fail to send the tx to thornado: %w", err)
		}
		s.logger.Info().Stringer("txid", txID).Int64("block", height).Msg("sent keygen tx to thornado")
		return nil
	}, bf)
}

func frostKeyshareRawFromLocalStatePath(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var state storage.KeygenLocalState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, false, err
	}
	if state.Engine() != storage.SigningEngineFrost {
		return nil, false, nil
	}
	if len(state.LocalData) == 0 {
		return nil, true, fmt.Errorf("frost local state has empty local data")
	}
	return state.LocalData, true, nil
}

// signAndBroadcast will sign the tx and broadcast it to the corresponding chain. On
// SignTx error for the chain client, if we receive checkpoint bytes we also return them
// with the error so they can be set on the TxOutStoreItem and re-used on a subsequent
// retry to avoid double spend. The second returned value is an optional observation
// that should be submitted to Thornado.
func (s *Signer) signAndBroadcast(item TxOutStoreItem) ([]byte, *types.TxInItem, error) {
	height := item.Height
	tx := item.TxOutItem

	// set the checkpoint on the tx out item if it was stored
	if item.Checkpoint != nil {
		tx.Checkpoint = item.Checkpoint
	}

	blockHeight, err := s.thornadoBridge.GetBlockHeight()
	if err != nil {
		s.logger.Error().Err(err).Msgf("fail to get block height")
		return nil, nil, err
	}
	signingTransactionPeriod, err := s.constantsProvider.GetInt64Value(blockHeight, constants.SigningTransactionPeriod)
	s.logger.Debug().Msgf("signing transaction period:%d", signingTransactionPeriod)
	if err != nil {
		s.logger.Error().Err(err).Msgf("fail to get constant value for(%s)", constants.SigningTransactionPeriod)
		return nil, nil, err
	}

	// if in round 7 retry, discard outbound if over the max outbound attempts
	inactiveVaultRound7Retry := false
	if item.Round7Retry {
		mimirKey := "MAXOUTBOUNDATTEMPTS"
		var maxOutboundAttemptsMimir int64
		maxOutboundAttemptsMimir, err = s.thornadoBridge.GetMimir(mimirKey)
		if err != nil {
			s.logger.Err(err).Msgf("fail to get %s", mimirKey)
			return nil, nil, err
		}
		attempt := (blockHeight - height) / signingTransactionPeriod
		if attempt > maxOutboundAttemptsMimir {
			s.logger.Warn().
				Int64("outbound_height", height).
				Int64("current_height", blockHeight).
				Int64("attempt", attempt).
				Msg("round 7 retry outbound tx has reached max outbound attempts")
			return nil, nil, nil
		}

		// determine if the round 7 retry is for an inactive vault
		var vault ttypes.Vault
		vault, err = s.thornadoBridge.GetVault(item.TxOutItem.VaultPubKey.String())
		if err != nil {
			log.Err(err).
				Stringer("vault_pubkey", item.TxOutItem.VaultPubKey).
				Msg("failed to get tx out item vault")
			return nil, nil, err
		}
		inactiveVaultRound7Retry = vault.Status == ttypes.VaultStatus_InactiveVault
	}

	// if not in round 7 retry or the round 7 retry is on an inactive vault, discard
	// outbound if within configured blocks of reschedule
	if !item.Round7Retry || inactiveVaultRound7Retry {
		if blockHeight-signingTransactionPeriod > height-s.cfg.Signer.RescheduleBufferBlocks {
			s.logger.Error().Msgf("tx was created at block height(%d), now it is (%d), it is older than (%d) blocks, skip it", height, blockHeight, signingTransactionPeriod)
			return nil, nil, nil
		}
	}

	chain, err := s.getChain(tx.Chain)
	if err != nil {
		s.logger.Error().Err(err).Msgf("not supported %s", tx.Chain.String())
		return nil, nil, err
	}
	mimirKey := "HALTSIGNING"
	haltSigningGlobalMimir, err := s.thornadoBridge.GetMimir(mimirKey)
	if err != nil {
		s.logger.Err(err).Msgf("fail to get %s", mimirKey)
		return nil, nil, err
	}
	if haltSigningGlobalMimir > 0 && haltSigningGlobalMimir <= blockHeight {
		s.logger.Info().Msg("signing has been halted globally")
		return nil, nil, nil
	}
	mimirKey = fmt.Sprintf(constants.MimirTemplateHaltSigning, tx.Chain)
	haltSigningMimir, err := s.thornadoBridge.GetMimir(mimirKey)
	if err != nil {
		s.logger.Err(err).Msgf("fail to get %s", mimirKey)
		return nil, nil, err
	}
	if haltSigningMimir > 0 && haltSigningMimir <= blockHeight {
		s.logger.Info().Msgf("signing for %s is halted", tx.Chain)
		return nil, nil, nil
	}
	if !s.shouldSign(tx) {
		s.logger.Info().Str("signer_address", chain.GetAddress(tx.VaultPubKey)).Msg("different pool address, ignore")
		return nil, nil, nil
	}

	if len(tx.ToAddress) == 0 {
		s.logger.Info().Msg("To address is empty, Thornado don't know where to send the fund , ignore")
		return nil, nil, nil // return nil and discard item
	}

	// don't sign if the block scanner is unhealthy. This is because the
	// network may not be able to detect the outbound transaction, and
	// therefore reschedule the transaction to another signer. In a disaster
	// scenario, the network could broadcast a transaction several times,
	// bleeding funds.
	if !chain.IsBlockScannerHealthy() {
		return nil, nil, fmt.Errorf("the block scanner for chain %s is unhealthy, not signing transactions due to it", chain.GetChain())
	}

	start := time.Now()
	defer func() {
		if h := s.m.GetHistograms(metrics.SignAndBroadcastDuration(chain.GetChain())); h != nil {
			h.Observe(time.Since(start).Seconds())
		}
	}()

	if !tx.OutHash.IsEmpty() {
		s.logger.Info().Str("OutHash", tx.OutHash.String()).Msg("tx had been sent out before")
		return nil, nil, nil // return nil and discard item
	}

	// We get the keysign object from thornado again to ensure it hasn't
	// been signed already, and we can skip. This helps us not get stuck on
	// a task that we'll never sign, because 2/3rds already has and will
	// never be available to sign again.
	txOut, err := s.thornadoBridge.GetKeysign(height, tx.VaultPubKey.String())
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get keysign items")
		return nil, nil, err
	}
	for _, txArray := range txOut.TxArray {
		if txArray.TxOutItem(item.TxOutItem.Height).Equals(tx) && !txArray.OutHash.IsEmpty() {
			// already been signed, we can skip it
			s.logger.Info().Str("tx_id", tx.OutHash.String()).Msgf("already signed. skipping...")
			return nil, nil, nil
		}
	}

	// If this chain provides a vault lock, hold it across sign and broadcast to avoid
	// concurrent UTXO selection or local double-spend on ambiguous broadcast errors.
	if locker, ok := chain.(interface{ GetVaultLock(string) *sync.Mutex }); ok {
		lock := locker.GetVaultLock(tx.VaultPubKey.String())
		lock.Lock()
		defer lock.Unlock()
	}

	// If SignedTx is set, we already signed and should only retry broadcast.
	var signedTx, checkpoint []byte
	var elapse time.Duration
	var observation *types.TxInItem
	if len(item.SignedTx) > 0 {
		s.logger.Info().Str("memo", tx.Memo).Msg("retrying broadcast of already signed tx")
		signedTx = item.SignedTx
		observation = item.Observation
	} else {
		startKeySign := time.Now()
		signedTx, checkpoint, observation, err = chain.SignTx(tx, height)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to sign tx")
			return checkpoint, nil, err
		}
		elapse = time.Since(startKeySign)

		// store immediately after a successful sign so the signed payload is saved even if broadcast crashes
		item.SignedTx = signedTx
		if storeErr := s.storage.Set(item); storeErr != nil {
			s.logger.Error().Err(storeErr).Msg("fail to persist signed tx before broadcast")
		}
	}

	// looks like the transaction is already signed
	if len(signedTx) == 0 {
		s.logger.Warn().Msgf("signed transaction is empty")
		return nil, nil, nil
	}

	// broadcast the transaction
	hash, err := chain.BroadcastTx(tx, signedTx)
	if err != nil {
		s.logger.Error().Err(err).Str("memo", tx.Memo).Msg("fail to broadcast tx to chain")

		// store the signed tx for the next retry
		item.SignedTx = signedTx
		item.Observation = observation
		if storeErr := s.storage.Set(item); storeErr != nil {
			s.logger.Error().Err(storeErr).Msg("fail to update tx out store item with signed tx")
		}

		return nil, observation, err
	}
	s.logger.Info().
		Str("chain", chain.GetChain().String()).
		Str("txid", hash).
		Str("memo", tx.Memo).
		Msg("broadcasted tx to chain")

	if s.isTssKeysign(tx.VaultPubKey) || s.isTssKeysign(tx.VaultPubKeyEddsa) {
		s.tssKeysignMetricMgr.SetTssKeysignMetric(hash, elapse.Milliseconds())
	}

	return nil, observation, nil
}

func (s *Signer) isTssKeysign(pubKey common.PubKey) bool {
	return !s.localPubKeyECDSA.Equals(pubKey) && !s.localPubKeyEDDSA.Equals(pubKey)
}

// Stop the signer process
func (s *Signer) Stop() error {
	s.logger.Info().Msg("receive request to stop signer")
	defer s.logger.Info().Msg("signer stopped successfully")
	close(s.stopChan)
	s.wg.Wait()
	if err := s.m.Stop(); err != nil {
		s.logger.Error().Err(err).Msg("fail to stop metric server")
	}
	s.blockScanner.Stop()
	return s.storage.Close()
}

////////////////////////////////////////////////////////////////////////////////////////
// pipelineSigner Interface
////////////////////////////////////////////////////////////////////////////////////////

func (s *Signer) isStopped() bool {
	select {
	case <-s.stopChan:
		return true
	default:
		return false
	}
}

func (s *Signer) storageList() []TxOutStoreItem {
	return s.storage.List()
}

func (s *Signer) processTransaction(item TxOutStoreItem) {
	s.logger.Info().
		Str("chain", item.TxOutItem.Chain.String()).
		Int64("height", item.Height).
		Int("status", int(item.Status)).
		Interface("tx", item.TxOutItem).
		Msg("Signing transaction")

	// a single keysign should not take longer than 5 minutes , regardless TSS or local
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	checkpoint, obs, err := runWithContext(ctx, func() ([]byte, *types.TxInItem, error) {
		return s.signAndBroadcast(item)
	})
	if err != nil {
		// Always store the checkpoint of the built tx if we have one. This ensures the same
		// vault will never attempt multiple txs for an outbound, as further protection
		// against cases like https://gitlab.com/thornado/thornado/-/merge_requests/4344.
		// In tradeoff, we accept a higher likelihood of temporary stuck outbounds.
		item.Checkpoint = checkpoint

		// mark the txout on round 7 failure to block other txs for the chain / pubkey
		ksErr := tss.KeysignError{}
		if errors.As(err, &ksErr) && ksErr.IsRound7() {
			s.logger.Error().Err(err).Interface("tx", item.TxOutItem).Msg("round 7 signing error")
			item.Round7Retry = true
			if storeErr := s.storage.Set(item); storeErr != nil {
				s.logger.Error().Err(storeErr).Msg("fail to update tx out store item with round 7 retry")
			}
		}

		s.logger.Error().Interface("tx", item.TxOutItem).Err(err).Msg("fail to sign and broadcast tx out store item")
		cancel()
		return
		// The 'item' for loop should not be items[0],
		// because problems which return 'nil, nil' should be skipped over instead of blocking others.
		// When signAndBroadcast returns an error (such as from a keysign timeout),
		// a 'return' and not a 'continue' should be used so that nodes can all restart the list,
		// for when the keysign failure was from a loss of list synchrony.
		// Otherwise, out-of-sync lists would cycle one timeout at a time, maybe never resynchronising.
	}
	cancel()

	// if enabled and the observation is non-nil, instant observe the outbound
	if s.cfg.Signer.AutoObserve && obs != nil {
		s.observer.ObserveSigned(types.TxIn{
			Chain:                  item.TxOutItem.Chain,
			TxArray:                []*types.TxInItem{obs},
			MemPool:                true,
			Filtered:               true,
			ConfirmationRequired:   0,
			AllowFutureObservation: true,
		})
	}

	// We have a successful broadcast! Remove the item from our store
	if err = s.storage.Remove(item); err != nil {
		s.logger.Error().Err(err).Msg("fail to update tx out store item")
	}
}
