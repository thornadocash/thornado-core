package signer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/observer"
	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

var (
	errNotDesignatedFrostSigner = errors.New("not designated FROST signer")
	errTxOutCompletionPending   = errors.New("txout completion pending")
	errFrostLeaderUnavailable   = errors.New("FROST party leader unavailable")
	errTxOutRemoved             = errors.New("txout removed from signer queue")
)

// Signer will pull the tx out from thornado and then forward it to chain
type Signer struct {
	logger                 zerolog.Logger
	cfg                    config.Bifrost
	wg                     *sync.WaitGroup
	thornadoBridge         thornadoclient.ThornadoBridge
	stopChan               chan struct{}
	blockScanner           *blockscanner.BlockScanner
	thornadoBlockScanner   *ThornadoBlockScan
	chains                 map[common.Chain]*btc.Client
	storage                SignerStorage
	m                      *metrics.Metrics
	errCounter             *prometheus.CounterVec
	frostKeygen            *frost.KeyGen
	pubkeyMgr              pubkeymanager.PubKeyValidator
	constantsProvider      *ConstantsProvider
	localState             storage.LocalStateManager
	localPubKeyECDSA       common.PubKey
	localPubKeyEDDSA       common.PubKey
	frostKeysignMetricMgr  *metrics.FrostKeysignMetricMgr
	observer               *observer.Observer
	pipeline               *pipeline
	debugSigningMu         sync.Mutex
	debugSigningOrder      []string
	debugSigningRecords    map[string]*DebugSigningPerformance
	debugSigningSeq        uint64
	missingErrataMu        sync.Mutex
	missingErrataSubmitted map[string]struct{}
}

// NewSigner create a new instance of signer
func NewSigner(cfg config.Bifrost,
	thornadoBridge thornadoclient.ThornadoBridge,
	thorKeys *thornadoclient.Keys,
	localState storage.LocalStateManager,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	chains map[common.Chain]*btc.Client,
	m *metrics.Metrics,
	frostKeysignMetricMgr *metrics.FrostKeysignMetricMgr,
	obs *observer.Observer,
	coordinator frost.SessionCoordinator,
) (*Signer, error) {
	storage, err := NewSignerStore(cfg.Signer.SignerDbPath, cfg.Signer.LevelDB, thornadoBridge.GetConfig().SignerPasswd)
	if err != nil {
		return nil, fmt.Errorf("fail to create thornado scan storage: %w", err)
	}
	if frostKeysignMetricMgr == nil {
		return nil, fmt.Errorf("fail to create signer , frost keysign metric manager is nil")
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

	cfg.Signer.BlockScanner.ChainID = common.BTCChain // hard code to thornado

	// Create pubkey manager and add our private key
	thornadoScanStorage := NewNamespacedScannerStorage(storage.db, "thornado-txout-")
	thornadoBlockScanner, err := NewThornadoBlockScan(cfg.Signer.BlockScanner, thornadoScanStorage, thornadoBridge, m, pubkeyMgr)
	if err != nil {
		return nil, fmt.Errorf("fail to create thornado block scan: %w", err)
	}

	blockScanner, err := blockscanner.NewBlockScanner(cfg.Signer.BlockScanner, thornadoScanStorage, m, thornadoBridge, thornadoBlockScanner)
	if err != nil {
		return nil, fmt.Errorf("fail to create block scanner: %w", err)
	}

	kg, err := frost.NewFrostKeyGen(thorKeys, localState, thornadoBridge, coordinator)
	if err != nil {
		return nil, fmt.Errorf("fail to create Frost Key gen,err:%w", err)
	}
	constantProvider := NewConstantsProvider(thornadoBridge)
	return &Signer{
		logger:                 log.With().Str("module", "signer").Logger(),
		cfg:                    cfg,
		wg:                     &sync.WaitGroup{},
		stopChan:               make(chan struct{}),
		blockScanner:           blockScanner,
		thornadoBlockScanner:   thornadoBlockScanner,
		chains:                 chains,
		m:                      m,
		storage:                storage,
		errCounter:             m.GetCounterVec(metrics.SignerError),
		pubkeyMgr:              pubkeyMgr,
		thornadoBridge:         thornadoBridge,
		frostKeygen:            kg,
		constantsProvider:      constantProvider,
		localState:             localState,
		localPubKeyECDSA:       na.PubKeySet.Secp256k1,
		localPubKeyEDDSA:       common.EmptyPubKey,
		frostKeysignMetricMgr:  frostKeysignMetricMgr,
		observer:               obs,
		debugSigningRecords:    make(map[string]*DebugSigningPerformance),
		missingErrataSubmitted: make(map[string]struct{}),
	}, nil
}

func (s *Signer) getChain(chainID common.Chain) (*btc.Client, error) {
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
	go s.pollKeygenBlocks()

	s.wg.Add(1)
	go s.signTransactions()

	s.blockScanner.Start(nil, nil)
	return nil
}

func (s *Signer) pollKeygenBlocks() {
	s.logger.Info().Msg("start to poll thornado keygen blocks")
	defer s.logger.Info().Msg("stop polling thornado keygen blocks")
	defer s.wg.Done()

	ticker := time.NewTicker(constants.ThornadoBlockTime)
	defer ticker.Stop()

	for {
		height, err := s.thornadoBlockScanner.GetHeight()
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to get thornado height for keygen polling")
		} else if height > 0 {
			if err := s.thornadoBlockScanner.processRecentKeygenBlocks(height); err != nil {
				s.logger.Error().Err(err).Int64("block", height).Msg("fail to poll thornado keygen blocks")
			}
		}

		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
		}
	}
}

func (s *Signer) shouldSign(tx types.TxOutItem) bool {
	return s.pubkeyMgr.HasPubKey(tx.VaultPubKey) || s.pubkeyMgr.HasPubKey(tx.VaultPubKeyEddsa)
}

func nodeHasSignerMembership(node *ttypes.QueryNodeResponse, vaultPubKey common.PubKey) bool {
	for _, membership := range node.SignerMembership {
		if strings.EqualFold(membership, vaultPubKey.String()) {
			return true
		}
	}
	return false
}

func (s *Signer) localFrostStateExists(vaultPubKey common.PubKey) bool {
	if s.localState == nil {
		return false
	}
	_, err := s.localState.GetLocalState(vaultPubKey.String())
	return err == nil
}

func (s *Signer) localFrostSigningParty(vaultPubKey common.PubKey) (common.PubKey, bool) {
	if s.localState == nil {
		return common.PubKey(""), false
	}
	state, err := s.localState.GetLocalState(vaultPubKey.String())
	if err != nil {
		return common.PubKey(""), false
	}
	if state.LocalPartyKey != "" {
		pk, err := common.NewPubKey(state.LocalPartyKey)
		if err == nil {
			return pk, true
		}
	}
	return s.localPubKeyECDSA, true
}

func (s *Signer) frostVaultMembers(vaultPubKey common.PubKey) ([]string, error) {
	vault, err := s.thornadoBridge.GetVault(vaultPubKey.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch FROST vault membership for %s: %w", vaultPubKey, err)
	}

	nodes, err := s.thornadoBridge.GetNodeAccounts()
	if err != nil {
		return nil, fmt.Errorf("fail to get node accounts for FROST signer selection: %w", err)
	}

	vaultMembers := make(map[string]struct{}, len(vault.Membership))
	for _, member := range vault.Membership {
		vaultMembers[member] = struct{}{}
	}

	members := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || !strings.EqualFold(node.Status, "active") {
			continue
		}
		pubKey := node.PubKeySet.Secp256k1.String()
		if pubKey == "" {
			continue
		}
		if len(vaultMembers) > 0 {
			if _, ok := vaultMembers[pubKey]; !ok {
				continue
			}
		}
		if !nodeHasSignerMembership(node, vaultPubKey) {
			continue
		}
		members = append(members, pubKey)
	}
	if len(members) == 0 {
		if len(vault.Membership) == 0 {
			return nil, fmt.Errorf("FROST vault %s has no membership", vaultPubKey)
		}
		return nil, fmt.Errorf("no active FROST signer members found for vault %s", vaultPubKey)
	}
	sort.Strings(members)
	return members, nil
}

func (s *Signer) frostPartyLeader(item TxOutStoreItem, blockHeight, signingPeriod int64) (string, error) {
	tx := item.TxOutItem
	if !item.SigningLeader.IsEmpty() {
		return item.SigningLeader.String(), nil
	}
	members, err := s.frostVaultMembers(tx.VaultPubKey)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("txout:%d:%d", item.Epoch, item.Height)))
	offset := binary.BigEndian.Uint64(digest[:8])
	return members[offset%uint64(len(members))], nil
}

func (s *Signer) frostSignerRoles(item TxOutStoreItem, blockHeight, signingPeriod int64) (participate bool, broadcast bool, err error) {
	tx := item.TxOutItem
	if !item.SigningLeader.IsEmpty() {
		if !s.localFrostStateExists(tx.VaultPubKey) {
			s.logger.Debug().
				Stringer("in_hash", tx.InHash).
				Stringer("vault_pub_key", tx.VaultPubKey).
				Stringer("local_pub_key", s.localPubKeyECDSA).
				Msg("skipping FROST leader txout because local keyshare is missing")
			return false, false, nil
		}
		_, ok := s.localFrostSigningParty(tx.VaultPubKey)
		if !ok {
			return false, false, nil
		}
		participate = true
		broadcast = true
		return participate, broadcast, nil
	}

	if _, err := s.frostVaultMembers(tx.VaultPubKey); err != nil {
		return false, false, err
	}

	if !s.localFrostStateExists(tx.VaultPubKey) {
		s.logger.Debug().
			Stringer("in_hash", tx.InHash).
			Stringer("vault_pub_key", tx.VaultPubKey).
			Stringer("local_pub_key", s.localPubKeyECDSA).
			Msg("skipping FROST txout because local keyshare is missing")
		return false, false, nil
	}
	_, ok := s.localFrostSigningParty(tx.VaultPubKey)
	if !ok {
		return false, false, nil
	}
	participate = true
	broadcast = true
	return participate, broadcast, nil
}

func nextFrostSignerAttemptHeight(tx types.TxOutItem, blockHeight, signingPeriod int64) int64 {
	if signingPeriod <= 0 {
		return blockHeight + 1
	}
	if blockHeight <= tx.Height {
		return tx.Height + signingPeriod
	}
	attempt := (blockHeight - tx.Height) / signingPeriod
	return tx.Height + ((attempt + 1) * signingPeriod)
}

func unwrapKeysignError(err error) (frost.KeysignError, bool) {
	var ksErr frost.KeysignError
	if errors.As(err, &ksErr) {
		return ksErr, true
	}
	if err == nil {
		return frost.KeysignError{}, false
	}
	if strings.Contains(err.Error(), "fail to frost schnorr sign") ||
		strings.Contains(err.Error(), "fail to complete FROST keysign") ||
		strings.Contains(err.Error(), "join FROST party") {
		return frost.NewKeysignError(ttypes.Blame{FailReason: err.Error()}), true
	}
	return frost.KeysignError{}, false
}

func isFrostLeaderUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "leader not reachable") ||
		strings.Contains(msg, "leader is not reachable")
}

func (s *Signer) deferFrostKeysignRetry(item TxOutStoreItem) error {
	if ttypes.IsInternalTxOutType(item.TxOutItem.TxType) {
		return nil
	}
	blockHeight, err := s.thornadoBridge.GetBlockHeight()
	if err != nil {
		return err
	}
	signingTransactionPeriodMinutes, err := s.constantsProvider.GetInt64Value(blockHeight, constants.Keysign_PeriodMinutes)
	if err != nil {
		return err
	}
	blockTimeSeconds, err := s.constantsProvider.GetInt64Value(blockHeight, constants.Chain_BlockTimeSeconds)
	if err != nil || blockTimeSeconds <= 0 {
		blockTimeSeconds = constants.NewConfigValue().GetInt64Value(constants.Chain_BlockTimeSeconds)
	}
	signingTransactionPeriod := constants.MinutesToBlocks(signingTransactionPeriodMinutes, blockTimeSeconds)
	item.DeferredUntilHeight = nextFrostSignerAttemptHeight(item.TxOutItem, blockHeight, signingTransactionPeriod)
	if storeErr := s.storage.Set(item); storeErr != nil {
		return storeErr
	}
	s.logger.Debug().
		Int64("current_height", blockHeight).
		Int64("deferred_until_height", item.DeferredUntilHeight).
		Stringer("in_hash", item.TxOutItem.InHash).
		Msg("deferred FROST keysign retry to next signing period")
	return nil
}

func (s *Signer) deferFrostKeysignRetryWithNextLeader(item TxOutStoreItem) error {
	blockHeight, err := s.thornadoBridge.GetBlockHeight()
	if err != nil {
		return err
	}
	item.DeferredUntilHeight = blockHeight + 1
	if storeErr := s.storage.Set(item); storeErr != nil {
		return storeErr
	}
	s.logger.Debug().
		Int64("current_height", blockHeight).
		Int64("deferred_until_height", item.DeferredUntilHeight).
		Stringer("in_hash", item.TxOutItem.InHash).
		Msg("deferred FROST keysign retry")
	return nil
}

func (s *Signer) submitFrostLeaderUnavailable(item TxOutStoreItem, cause error) bool {
	leader := item.SigningLeader
	if leader.IsEmpty() {
		var err error
		leaderStr, err := s.frostPartyLeader(item, item.Height, 0)
		if err != nil {
			s.logger.Debug().Err(err).Msg("fail to resolve unavailable FROST leader")
			return false
		}
		leader, err = common.NewPubKey(leaderStr)
		if err != nil {
			s.logger.Debug().Err(err).Str("leader", leaderStr).Msg("fail to parse unavailable FROST leader")
			return false
		}
	}
	blame := ttypes.Blame{
		FailReason: "FROST party leader unavailable",
		BlameNodes: []ttypes.Node{{
			Pubkey: leader.String(),
		}},
	}
	if cause != nil {
		blame.FailReason = cause.Error()
	}
	txID, err := s.thornadoBridge.PostKeysignFailure(blame, item.Height, item.TxOutItem.Coins, item.TxOutItem.VaultPubKey)
	if err != nil {
		s.logger.Error().
			Err(err).
			Int64("height", item.Height).
			Stringer("leader", leader).
			Msg("fail to submit FROST leader unavailable vote")
		return false
	}
	s.logger.Warn().
		Int64("height", item.Height).
		Stringer("leader", leader).
		Stringer("tx_id", txID).
		Msg("submitted FROST leader unavailable vote")
	return true
}

func (s *Signer) txOutItemCompleted(item TxOutStoreItem) (bool, common.TxID, error) {
	txOut, err := s.thornadoBridge.GetKeysign(item.Height, item.TxOutItem.VaultPubKey.String())
	if err != nil {
		return false, common.TxID(""), err
	}

	if completed, outHash := completedTxOutItem(item, []types.TxOut{txOut}); completed {
		return true, outHash, nil
	}
	return false, common.TxID(""), nil
}

func (s *Signer) txOutItemCompletedInHistory(item TxOutStoreItem) (bool, common.TxID, error) {
	txOuts, err := s.thornadoBridge.GetAllTxOutKeysigns()
	if err != nil {
		return false, common.TxID(""), err
	}
	completed, outHash := completedTxOutItem(item, txOuts)
	return completed, outHash, nil
}

func completedTxOutItem(item TxOutStoreItem, txOuts []types.TxOut) (bool, common.TxID) {
	for _, txOut := range txOuts {
		if txOut.Height != item.Height {
			continue
		}
		for _, tx := range txOut.TxArray {
			txItem := tx.TxOutItem(txOut.Height)
			// EdDSA is filled from vault state after storage retrieval, while queried
			// txouts may omit it. It is not part of BTC FROST txout identity.
			txItem.VaultPubKeyEddsa = item.TxOutItem.VaultPubKeyEddsa
			if txOutCompletionMatch(item.TxOutItem, txItem) && !tx.OutHash.IsEmpty() {
				return true, tx.OutHash
			}
		}
	}
	return false, common.TxID("")
}

func txOutCompletionMatch(a, b types.TxOutItem) bool {
	if !a.Chain.Equals(b.Chain) {
		return false
	}
	if !a.VaultPubKey.Equals(b.VaultPubKey) {
		return false
	}
	if !a.ToAddress.Equals(b.ToAddress) {
		return false
	}
	if !a.InHash.Equals(b.InHash) {
		return false
	}
	if !a.VaultPubKeyEddsa.Equals(b.VaultPubKeyEddsa) {
		return false
	}
	if a.VaultPathIndex != b.VaultPathIndex {
		return false
	}
	if a.TxType != b.TxType {
		return false
	}
	if ttypes.IsInternalTxOutType(a.TxType) {
		return true
	}
	if types.SourceInputsEqual(a.SourceInputs, b.SourceInputs) {
		return true
	}
	if (a.TxType == ttypes.TxOutTypeMigrate || a.TxType == ttypes.TxOutTypeConsolidate) &&
		sourceInputOutpointsEqual(a.SourceInputs, b.SourceInputs) {
		return true
	}
	return false
}

func sourceInputOutpointsEqual(a, b []types.TxOutInput) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	aa := append([]types.TxOutInput(nil), a...)
	bb := append([]types.TxOutInput(nil), b...)
	sort.SliceStable(aa, func(i, j int) bool {
		if !aa[i].TxID.Equals(aa[j].TxID) {
			return aa[i].TxID.String() < aa[j].TxID.String()
		}
		return aa[i].Vout < aa[j].Vout
	})
	sort.SliceStable(bb, func(i, j int) bool {
		if !bb[i].TxID.Equals(bb[j].TxID) {
			return bb[i].TxID.String() < bb[j].TxID.String()
		}
		return bb[i].Vout < bb[j].Vout
	})
	for i := range aa {
		if !aa[i].TxID.Equals(bb[i].TxID) || aa[i].Vout != bb[i].Vout {
			return false
		}
	}
	return true
}

func (s *Signer) refreshTxOutBatchMetadata(item TxOutStoreItem) TxOutStoreItem {
	txOut, err := s.thornadoBridge.GetKeysign(item.Height, item.TxOutItem.VaultPubKey.String())
	if err != nil {
		s.logger.Debug().Err(err).Interface("tx", item.TxOutItem).Msg("fail to refresh txout batch metadata")
		return item
	}
	for _, tx := range txOut.TxArray {
		txItem := tx.TxOutItem(txOut.Height)
		txItem.VaultPubKeyEddsa = item.TxOutItem.VaultPubKeyEddsa
		if item.TxOutItem.Equals(txItem) {
			item.Epoch = txOut.Epoch
			item.BatchStatus = txOut.Status
			item.SigningLeader = txOut.SigningLeader
			if err := s.storage.Set(item); err != nil {
				s.logger.Error().Err(err).Msg("fail to update txout batch metadata")
			}
			return item
		}
	}
	return item
}

func txOutItemPresent(item TxOutStoreItem, txOut types.TxOut) bool {
	if txOut.Height != item.Height {
		return false
	}
	for _, tx := range txOut.TxArray {
		txItem := tx.TxOutItem(txOut.Height)
		txItem.VaultPubKeyEddsa = item.TxOutItem.VaultPubKeyEddsa
		if item.TxOutItem.Equals(txItem) {
			return true
		}
	}
	return false
}

func txOutItemPresentInList(item TxOutStoreItem, txOuts []types.TxOut) bool {
	for _, txOut := range txOuts {
		if txOutItemPresent(item, txOut) {
			return true
		}
	}
	return false
}

func currentTxOutItemForSigning(item TxOutStoreItem, txOut types.TxOut) (types.TxOutItem, bool) {
	if txOut.Height != item.Height {
		return types.TxOutItem{}, false
	}
	if item.Index >= 0 && int(item.Index) < len(txOut.TxArray) {
		txItem := txOut.TxArray[item.Index].TxOutItem(txOut.Height)
		txItem = normalizeCurrentTxOutItem(item.TxOutItem, txItem)
		if txOutSigningIdentityMatch(item.TxOutItem, txItem) {
			return txItem, true
		}
	}

	var matched types.TxOutItem
	matches := 0
	for _, tx := range txOut.TxArray {
		txItem := normalizeCurrentTxOutItem(item.TxOutItem, tx.TxOutItem(txOut.Height))
		if !txOutSigningIdentityMatch(item.TxOutItem, txItem) {
			continue
		}
		matched = txItem
		matches++
	}
	if matches != 1 {
		return types.TxOutItem{}, false
	}
	return matched, true
}

func normalizeCurrentTxOutItem(stored, current types.TxOutItem) types.TxOutItem {
	if current.VaultPubKeyEddsa.IsEmpty() {
		current.VaultPubKeyEddsa = stored.VaultPubKeyEddsa
	}
	return current
}

func txOutSigningIdentityMatch(a, b types.TxOutItem) bool {
	return a.Chain.Equals(b.Chain) &&
		a.VaultPubKey.Equals(b.VaultPubKey) &&
		a.ToAddress.Equals(b.ToAddress) &&
		a.InHash.Equals(b.InHash) &&
		a.VaultPubKeyEddsa.Equals(b.VaultPubKeyEddsa) &&
		a.VaultPathIndex == b.VaultPathIndex &&
		a.TxType == b.TxType
}

func unsignedLocalTxOut(item TxOutStoreItem) bool {
	return len(item.Checkpoint) == 0 &&
		len(item.SignedTx) == 0 &&
		item.Observation == nil
}

func (s *Signer) removeUnsignedTxOutMissingFromThornado(item TxOutStoreItem) bool {
	if !unsignedLocalTxOut(item) {
		return false
	}
	txOut, err := s.thornadoBridge.GetKeysign(item.Height, item.TxOutItem.VaultPubKey.String())
	if err != nil {
		s.logger.Debug().Err(err).Interface("tx", item.TxOutItem).Msg("fail to check txout presence before signing")
		return false
	}
	if txOutItemPresent(item, txOut) {
		return false
	}
	txOuts, err := s.thornadoBridge.GetAllTxOutKeysigns()
	if err != nil {
		s.logger.Debug().Err(err).Interface("tx", item.TxOutItem).Msg("fail to cross-check txout history before signing")
		return false
	}
	if txOutItemPresentInList(item, txOuts) {
		return false
	}
	s.logger.Info().
		Str("chain", item.TxOutItem.Chain.String()).
		Int64("height", item.Height).
		Int64("index", item.Index).
		Stringer("in_hash", item.TxOutItem.InHash).
		Str("tx_type", item.TxOutItem.TxType).
		Msg("removing unsigned txout no longer present in thornado state")
	if err := s.storage.Remove(item); err != nil {
		s.logger.Error().Err(err).Msg("fail to remove tx out store item no longer present in thornado state")
	}
	return true
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
			// When BTCChain is catching up , bifrost might get stale data from thornado , thus it shall pause signing
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

func runWithContext(ctx context.Context, fn func() ([]byte, *types.TxInItem, bool, error)) ([]byte, *types.TxInItem, bool, error) {
	ch := make(chan error, 1)
	var checkpoint []byte
	var txIn *types.TxInItem
	var recovered bool
	go func() {
		var err error
		checkpoint, txIn, recovered, err = fn()
		ch <- err
	}()
	select {
	case <-ctx.Done():
		return nil, nil, false, ctx.Err()
	case err := <-ch:
		return checkpoint, txIn, recovered, err
	}
}

func (s *Signer) processTransactions() {
	signerConcurrency, err := s.thornadoBridge.GetConfigValue(constants.Signer_Concurrency.String())
	if err != nil || signerConcurrency <= 0 {
		s.logger.Error().Err(err).Str("config", constants.Signer_Concurrency.String()).Msg("fail to get config")
		signerConcurrency = constants.NewConfigValue().GetInt64Value(constants.Signer_Concurrency)
	}

	// if previously set to different concurrency, drain existing signings
	if s.pipeline != nil && s.pipeline.concurrency != signerConcurrency {
		s.pipeline.Wait()
		s.pipeline = nil
	}

	// if not set, or set to different concurrency, create new pipeline
	if s.pipeline == nil {
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
			s.logger.Debug().
				Int64("height", txOut.Height).
				Uint64("epoch", txOut.Epoch).
				Str("status", txOut.Status).
				Int("items", len(txOut.TxArray)).
				Msg("received txout batch from thornado")
			items := txOutUnsignedStoreItems(s.storage, txOut)
			if err := s.storage.Batch(items); err != nil {
				s.logger.Error().Err(err).Msg("fail to save tx out items to storage")
			}
		}
	}
}

func txOutUnsignedStoreItems(storage SignerStorage, txOut types.TxOut) []TxOutStoreItem {
	items := make([]TxOutStoreItem, 0, len(txOut.TxArray))
	for i, tx := range txOut.TxArray {
		if !tx.OutHash.IsEmpty() {
			continue
		}
		item := NewTxOutStoreItem(txOut.Height, tx.TxOutItem(txOut.Height), int64(i))
		item.Epoch = txOut.Epoch
		item.BatchStatus = txOut.Status
		item.SigningLeader = txOut.SigningLeader
		if storage != nil {
			item = mergeStoredTxOutItem(storage, item)
		}
		items = append(items, item)
	}
	return items
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
	churnRetryIntervalMinutes, err := s.thornadoBridge.GetConfigValue(constants.Churn_RetryIntervalMinutes.String())
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get churn retry config")
		return false
	}
	if churnRetryIntervalMinutes <= 0 {
		churnRetryIntervalMinutes = constants.NewConfigValue().GetInt64Value(constants.Churn_RetryIntervalMinutes)
	}
	keygenRetryIntervalMinutes, err := s.thornadoBridge.GetConfigValue(constants.Keygen_RetryIntervalMinutes.String())
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get keygen retries config")
		return false
	}
	if keygenRetryIntervalMinutes <= 0 {
		return false
	}

	// sanity check the retry interval is at least 1.5x the timeout
	retryIntervalDuration := time.Duration(keygenRetryIntervalMinutes) * time.Minute
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
	blockTimeSeconds, err := s.thornadoBridge.GetConfigValue(constants.Chain_BlockTimeSeconds.String())
	if err != nil || blockTimeSeconds <= 0 {
		blockTimeSeconds = constants.NewConfigValue().GetInt64Value(constants.Chain_BlockTimeSeconds)
	}
	churnRetryInterval := constants.MinutesToBlocks(churnRetryIntervalMinutes, blockTimeSeconds)
	keygenRetryInterval := constants.MinutesToBlocks(keygenRetryIntervalMinutes, blockTimeSeconds)

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
		pubKey, blame, err := s.frostKeygen.GenerateNewKey(keygenBlock.Height, keygenReq.GetMembers(), common.Chains{common.BTCChain})
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
			if len(blame) == 0 {
				s.errCounter.WithLabelValues("fail_to_keygen_pubkey", "").Inc()
				s.logger.Error().Interface("keygenBlock", keygenBlock).Msg("skip keygen broadcast: empty vault pubkey without blame")
				continue
			}
		}

		// generate a verification signature to ensure we can sign with the new key
		secp256k1Sig := s.secp256k1VerificationSignature(pubKey.Secp256k1)

		if err = s.sendKeygenToThornado(keygenBlock.Height, pubKey.Secp256k1, secp256k1Sig, blame, keygenReq.GetMembers(), keygenReq.Type, keygenTime, common.EmptyPubKey); err != nil {
			s.errCounter.WithLabelValues("fail_to_broadcast_keygen", "").Inc()
			s.logger.Error().Err(err).Msg("fail to broadcast keygen")
		}

		// monitor the new pubkey and any new members
		if !pubKey.Secp256k1.IsEmpty() {
			s.pubkeyMgr.AddPubKey(pubKey.Secp256k1, true, common.SigningAlgoSecp256k1)
		}
		for _, pk := range keygenReq.GetMembers() {
			s.pubkeyMgr.AddPubKey(pk, false, common.SigningAlgoSecp256k1)
		}
	}
}

func (s *Signer) secp256k1VerificationSignature(pk common.PubKey) []byte {
	return nil
}

func (s *Signer) sendKeygenToThornado(height int64, vaultPk common.PubKey, secp256k1Signature []byte, blame []ttypes.Blame, input common.PubKeys, keygenType ttypes.KeygenType, keygenTime int64, vaultPubKeyEddsa common.PubKey) error {
	// collect supported chains in the configuration
	chains := common.Chains{
		common.BTCChain,
	}
	for chain, chainCfg := range s.cfg.GetChains() {
		if !chainCfg.OptToRetire && !chainCfg.Disabled && !chains.Has(chain) {
			chains = append(chains, chain)
		}
	}

	// make a best effort to add encrypted keyshares to the message
	var keyshares []byte
	var keysharesEddsa []byte
	var keysharesFrost []byte
	var err error
	if s.cfg.Signer.BackupKeyshares {
		if !vaultPk.IsEmpty() {
			keysharePath := filepath.Join(app.DefaultNodeHome, fmt.Sprintf("localstate-%s.json", vaultPk))
			frostRaw, isFrost, readErr := frostKeyshareRawFromLocalStatePath(keysharePath)
			switch {
			case readErr != nil:
				s.logger.Error().Err(readErr).Msg("fail to read keyshares")
			case isFrost:
				keysharesFrost, err = frost.EncryptRawKeyshares(frostRaw, os.Getenv("SIGNER_SEED_PHRASE"))
				if err != nil {
					s.logger.Error().Err(err).Msg("fail to encrypt frost keyshares")
				}
			default:
				keyshares, err = frost.EncryptKeyshares(keysharePath, os.Getenv("SIGNER_SEED_PHRASE"))
				if err != nil {
					s.logger.Error().Err(err).Msg("fail to encrypt secp256k1 keyshares")
				}
			}
		}
		if !vaultPubKeyEddsa.IsEmpty() {
			keysharesEddsa, err = frost.EncryptKeyshares(
				filepath.Join(app.DefaultNodeHome, fmt.Sprintf("localstate-%s.json", vaultPubKeyEddsa)),
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
			keygenMsg, err = bridge.GetKeygenStdTxWithFrost(vaultPk, secp256k1Signature, keyshares, blame, input, keygenType, chains, height, keygenTime, vaultPubKeyEddsa, keysharesEddsa, keysharesFrost)
		} else {
			keygenMsg, err = s.thornadoBridge.GetKeygenStdTx(vaultPk, secp256k1Signature, keyshares, blame, input, keygenType, chains, height, keygenTime, vaultPubKeyEddsa, keysharesEddsa)
		}
	} else {
		keygenMsg, err = s.thornadoBridge.GetKeygenStdTx(vaultPk, secp256k1Signature, keyshares, blame, input, keygenType, chains, height, keygenTime, vaultPubKeyEddsa, keysharesEddsa)
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
// that should be submitted to BTCChain.
func (s *Signer) signAndBroadcast(ctx context.Context, item *TxOutStoreItem) (checkpointOut []byte, observationOut *types.TxInItem, recoveredObservation bool, retErr error) {
	height := item.Height
	tx := item.TxOutItem
	perfID := s.debugSigningStart(*item)
	defer func() {
		switch {
		case retErr == nil:
			return
		case errors.Is(retErr, errNotDesignatedFrostSigner):
			s.debugSigningFinish(perfID, "not_designated", retErr.Error())
		case errors.Is(retErr, errTxOutCompletionPending):
			s.debugSigningFinish(perfID, "txout_completion_pending", retErr.Error())
		case errors.Is(retErr, errFrostLeaderUnavailable):
			s.debugSigningFinish(perfID, "leader_unavailable", retErr.Error())
		default:
			s.debugSigningError(perfID, retErr)
		}
	}()

	// set the checkpoint on the tx out item if it was stored
	if item.Checkpoint != nil {
		tx.Checkpoint = item.Checkpoint
	}

	s.debugSigningEvent(perfID, "block_height_resolve_start", "")
	blockHeight, err := s.thornadoBridge.GetBlockHeight()
	if err != nil {
		s.logger.Error().Err(err).Msgf("fail to get block height")
		return nil, nil, false, err
	}
	s.debugSigningEvent(perfID, "block_height_resolved", fmt.Sprintf("%d", blockHeight))
	signingTransactionPeriodMinutes, err := s.constantsProvider.GetInt64Value(blockHeight, constants.Keysign_PeriodMinutes)
	if err != nil {
		s.logger.Error().Err(err).Msgf("fail to get constant value for(%s)", constants.Keysign_PeriodMinutes)
		return nil, nil, false, err
	}
	blockTimeSeconds, err := s.constantsProvider.GetInt64Value(blockHeight, constants.Chain_BlockTimeSeconds)
	if err != nil || blockTimeSeconds <= 0 {
		blockTimeSeconds = constants.NewConfigValue().GetInt64Value(constants.Chain_BlockTimeSeconds)
	}
	signingTransactionPeriod := constants.MinutesToBlocks(signingTransactionPeriodMinutes, blockTimeSeconds)
	s.logger.Debug().Msgf("signing transaction period:%d", signingTransactionPeriod)
	s.debugSigningEvent(perfID, "signing_period_resolved", fmt.Sprintf("%d", signingTransactionPeriod))

	s.debugSigningEvent(perfID, "chain_resolve_start", tx.Chain.String())
	chain, err := s.getChain(tx.Chain)
	if err != nil {
		s.logger.Error().Err(err).Msgf("not supported %s", tx.Chain.String())
		return nil, nil, false, err
	}
	s.debugSigningEvent(perfID, "chain_resolved", chain.GetChain().String())

	s.debugSigningEvent(perfID, "keysign_state_fetch_start", "")
	txOut, err := s.thornadoBridge.GetKeysign(height, tx.VaultPubKey.String())
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get keysign items")
		return nil, nil, false, err
	}
	s.debugSigningEvent(perfID, "keysign_state_fetch_done", fmt.Sprintf("items=%d", len(txOut.TxArray)))
	currentTx, foundCurrentTx := currentTxOutItemForSigning(*item, txOut)
	if !foundCurrentTx {
		s.logger.Info().
			Str("chain", tx.Chain.String()).
			Int64("height", item.Height).
			Int64("index", item.Index).
			Stringer("in_hash", tx.InHash).
			Str("tx_type", tx.TxType).
			Msg("txout is no longer present in current thornado signing state")
		s.debugSigningFinish(perfID, "txout_not_current", "")
		return nil, nil, false, errTxOutCompletionPending
	}
	if item.Checkpoint != nil {
		currentTx.Checkpoint = item.Checkpoint
	}
	item.TxOutItem = currentTx
	item.Epoch = txOut.Epoch
	item.BatchStatus = txOut.Status
	item.SigningLeader = txOut.SigningLeader
	tx = currentTx
	if err := s.storage.Set(*item); err != nil {
		s.logger.Error().Err(err).Msg("fail to persist refreshed txout before signing")
		s.debugSigningEvent(perfID, "keysign_state_refresh_persist_error", err.Error())
	} else {
		s.debugSigningEvent(perfID, "keysign_state_refreshed", fmt.Sprintf("source_inputs=%d max_gas=%d gas_rate=%d", len(tx.SourceInputs), len(tx.MaxGas), tx.GasRate))
	}

	if !s.shouldSign(tx) {
		s.logger.Info().Str("signer_address", chain.GetAddress(tx.VaultPubKey)).Msg("different vault address, ignore")
		return nil, nil, false, nil
	}
	{
		s.debugSigningEvent(perfID, "recover_observation_start", "")
		obs, recovered, recoverErr := chain.RecoverTxObservation(tx, blockHeight)
		if recoverErr != nil {
			s.logger.Debug().
				Err(recoverErr).
				Stringer("in_hash", tx.InHash).
				Stringer("chain", tx.Chain).
				Msg("failed to recover txout observation before signing")
			s.debugSigningEvent(perfID, "recover_observation_error", recoverErr.Error())
		} else if obs != nil {
			s.logger.Info().
				Str("txid", obs.Tx).
				Stringer("in_hash", tx.InHash).
				Stringer("chain", tx.Chain).
				Bool("recovered", recovered).
				Msg("recovered txout observation before signing")
			s.debugSigningFinish(perfID, "recovered_observation", obs.Tx)
			return nil, obs, recovered, nil
		} else {
			s.debugSigningEvent(perfID, "recover_observation_none", "")
		}
	}

	configKey := constants.Halt_SigningGlobal.String()
	haltSigningGlobalConfig, err := s.thornadoBridge.GetConfigValue(configKey)
	if err != nil {
		s.logger.Err(err).Msgf("fail to get %s", configKey)
		return nil, nil, false, err
	}
	if haltSigningGlobalConfig > 0 && haltSigningGlobalConfig <= blockHeight {
		s.logger.Info().Msg("signing has been halted globally")
		return nil, nil, false, nil
	}
	configKey = fmt.Sprintf(constants.ConfigTemplateHaltSigning, tx.Chain)
	haltSigningConfig, err := s.thornadoBridge.GetConfigValue(configKey)
	if err != nil {
		s.logger.Err(err).Msgf("fail to get %s", configKey)
		return nil, nil, false, err
	}
	if haltSigningConfig > 0 && haltSigningConfig <= blockHeight {
		s.logger.Info().Msgf("signing for %s is halted", tx.Chain)
		return nil, nil, false, nil
	}
	if missing, err := chain.SourceTxMissing(tx, blockHeight); err != nil {
		s.logger.Error().Err(err).Interface("tx", tx).Msg("fail to check sweep source tx")
	} else if missing {
		if s.submitMissingSourceErrata(tx.InHash, tx.Chain) {
			if storeErr := s.storage.Remove(*item); storeErr != nil {
				s.logger.Error().Err(storeErr).Msg("fail to remove errata'd tx out store item")
			}
			s.debugSigningFinish(perfID, "missing_source_errata_submitted", "")
			return nil, nil, false, errTxOutRemoved
		}
	}
	s.debugSigningEvent(perfID, "leader_resolve_start", "")
	resolvedPartyLeader, leaderErr := s.frostPartyLeader(*item, blockHeight, signingTransactionPeriod)
	if leaderErr != nil {
		s.debugSigningEvent(perfID, "leader_resolve_error", leaderErr.Error())
	}
	s.debugSigningEvent(perfID, "roles_resolve_start", "")
	participate, broadcast, err := s.frostSignerRoles(*item, blockHeight, signingTransactionPeriod)
	if err != nil {
		return nil, nil, false, err
	}
	s.debugSigningRoles(perfID, participate, broadcast, resolvedPartyLeader)
	if !participate {
		item.DeferredUntilHeight = nextFrostSignerAttemptHeight(tx, blockHeight, signingTransactionPeriod)
		if storeErr := s.storage.Set(*item); storeErr != nil {
			s.logger.Error().Err(storeErr).Msg("fail to defer non-participating FROST tx out store item")
		} else {
			s.logger.Debug().
				Int64("current_height", blockHeight).
				Int64("deferred_until_height", item.DeferredUntilHeight).
				Stringer("in_hash", tx.InHash).
				Msg("deferred non-participating FROST txout")
		}
		return nil, nil, false, errNotDesignatedFrostSigner
	}

	if len(tx.ToAddress) == 0 {
		s.logger.Info().Msg("To address is empty, BTCChain don't know where to send the fund , ignore")
		return nil, nil, false, nil // return nil and discard item
	}

	// don't sign if the block scanner is unhealthy. This is because the
	// network may not be able to detect the outbound transaction, and
	// therefore reschedule the transaction to another signer. In a disaster
	// scenario, the network could broadcast a transaction several times,
	// bleeding funds.
	if !chain.IsBlockScannerHealthy() {
		if ttypes.IsInternalTxOutType(tx.TxType) {
			s.logger.Warn().
				Stringer("chain", chain.GetChain()).
				Stringer("in_hash", tx.InHash).
				Str("tx_type", tx.TxType).
				Msg("signing internal txout while block scanner is unhealthy")
		} else if strings.EqualFold(os.Getenv("BIFROST_SIGNER_ALLOW_UNHEALTHY_SIGNING"), "true") {
			s.logger.Warn().
				Stringer("chain", chain.GetChain()).
				Stringer("in_hash", tx.InHash).
				Msg("signing while block scanner is unhealthy due to explicit override")
		} else {
			return nil, nil, false, fmt.Errorf("the block scanner for chain %s is unhealthy, not signing transactions due to it", chain.GetChain())
		}
	}

	start := time.Now()
	defer func() {
		if h := s.m.GetHistograms(metrics.SignAndBroadcastDuration(chain.GetChain())); h != nil {
			h.Observe(time.Since(start).Seconds())
		}
	}()

	if !tx.OutHash.IsEmpty() {
		s.logger.Info().Str("OutHash", tx.OutHash.String()).Msg("tx had been sent out before")
		s.debugSigningFinish(perfID, "already_broadcast", tx.OutHash.String())
		return nil, nil, false, nil // return nil and discard item
	}

	// We get the keysign object from thornado before signing to ensure it hasn't
	// been signed already, and we can skip. This helps us not get stuck on
	// a task that we'll never sign, because 2/3rds already has and will
	// never be available to sign again.
	for _, txArray := range txOut.TxArray {
		txItem := txArray.TxOutItem(item.TxOutItem.Height)
		txItem.VaultPubKeyEddsa = tx.VaultPubKeyEddsa
		if txOutCompletionMatch(tx, txItem) && !txArray.OutHash.IsEmpty() {
			// already been signed, we can skip it
			s.logger.Info().Str("tx_id", txArray.OutHash.String()).Msgf("already signed. skipping...")
			s.debugSigningFinish(perfID, "already_signed", txArray.OutHash.String())
			return nil, nil, false, nil
		}
	}

	// If this chain provides a vault lock, hold it across sign and broadcast to avoid
	// concurrent UTXO selection or local double-spend on ambiguous broadcast errors.
	{
		lock := chain.GetVaultLock(tx.VaultPubKey.String())
		s.debugSigningEvent(perfID, "vault_lock_acquire_start", tx.VaultPubKey.String())
		if !lock.TryLock() {
			if completed, outHash, completeErr := s.txOutItemCompleted(*item); completeErr != nil {
				s.logger.Debug().Err(completeErr).Msg("fail to recheck txout completion while vault lock is busy")
			} else if completed {
				s.logger.Info().
					Stringer("out_hash", outHash).
					Msg("txout completed while waiting for vault lock")
				s.debugSigningFinish(perfID, "completed_while_vault_lock_busy", outHash.String())
				return nil, nil, false, nil
			} else if completed, outHash, historyErr := s.txOutItemCompletedInHistory(*item); historyErr != nil {
				s.logger.Debug().Err(historyErr).Msg("fail to recheck txout history while vault lock is busy")
			} else if completed {
				s.logger.Info().
					Stringer("out_hash", outHash).
					Msg("historical txout completed while waiting for vault lock")
				s.debugSigningFinish(perfID, "historical_completed_while_vault_lock_busy", outHash.String())
				return nil, nil, false, nil
			}
			s.debugSigningFinish(perfID, "vault_lock_busy", tx.VaultPubKey.String())
			return nil, nil, false, errTxOutCompletionPending
		}
		s.debugSigningEvent(perfID, "vault_lock_acquired", tx.VaultPubKey.String())
		defer lock.Unlock()
	}

	// If SignedTx is set, we already signed and should only retry broadcast.
	var signedTx, checkpoint []byte
	var elapse time.Duration
	var observation *types.TxInItem
	var batchItems []types.TxOutItem
	if len(item.SignedTx) > 0 {
		s.logger.Info().Msg("retrying broadcast of already signed tx")
		s.debugSigningEvent(perfID, "broadcast_retry_start", fmt.Sprintf("bytes=%d", len(item.SignedTx)))
		signedTx = item.SignedTx
		observation = item.Observation
	} else {
		if leaderErr != nil {
			s.logger.Debug().Err(leaderErr).Msg("fail to resolve FROST party leader")
		} else if resolvedPartyLeader != "" {
			chain.SetFrostPartyLeader(resolvedPartyLeader)
			defer chain.ClearFrostPartyLeader()
		}
		startKeySign := time.Now()
		s.debugSigningEvent(perfID, "batch_collect_start", "")
		batchItems = txOutBatchItems(txOut, height, tx)
		s.debugSigningBatchItems(perfID, len(batchItems))
		s.debugSigningEvent(perfID, "batch_collect_done", fmt.Sprintf("items=%d", len(batchItems)))
		s.debugSigningEvent(perfID, "sign_start", fmt.Sprintf("batch_items=%d", len(batchItems)))
		if len(batchItems) > 0 {
			s.logger.Info().
				Int("items", len(batchItems)).
				Int64("height", height).
				Uint64("epoch", txOut.Epoch).
				Msg("signing BTC txout batch")
			signedTx, checkpoint, observation, err = chain.SignTxBatchContext(ctx, batchItems, blockHeight)
		} else {
			signedTx, checkpoint, observation, err = chain.SignTxContext(ctx, tx, blockHeight)
		}
		if err != nil {
			if errors.Is(err, frost.ErrLocalPartyNotSelected) || strings.Contains(err.Error(), frost.ErrLocalPartyNotSelected.Error()) {
				s.logger.Trace().Err(err).Msg("local FROST signer was not selected by party leader")
				return checkpoint, nil, false, errNotDesignatedFrostSigner
			}
			if strings.Contains(err.Error(), "party closed") {
				s.logger.Trace().Err(err).Msg("FROST party already closed")
				return checkpoint, nil, false, errNotDesignatedFrostSigner
			}
			if strings.Contains(err.Error(), "missing source input") {
				if completed, outHash, completeErr := s.txOutItemCompleted(*item); completeErr != nil {
					s.logger.Debug().Err(completeErr).Msg("fail to recheck txout completion after missing source input")
				} else if completed {
					s.logger.Info().
						Stringer("out_hash", outHash).
						Msg("txout completed while signing; skipping missing source input retry")
					return nil, nil, false, nil
				} else if completed, outHash, historyErr := s.txOutItemCompletedInHistory(*item); historyErr != nil {
					s.logger.Debug().Err(historyErr).Msg("fail to recheck txout history after missing source input")
				} else if completed {
					s.logger.Info().
						Stringer("out_hash", outHash).
						Msg("historical txout completed while signing; skipping missing source input retry")
					return nil, nil, false, nil
				}
				s.logger.Trace().Err(err).Msg("source input already spent; deferring until txout completion is visible")
				return checkpoint, nil, false, errTxOutCompletionPending
			}
			if isFrostLeaderUnavailable(err) {
				s.logger.Debug().Err(err).Msg("FROST party leader unavailable; rotating leader")
				return checkpoint, nil, false, fmt.Errorf("%w: %v", errFrostLeaderUnavailable, err)
			}
			s.logger.Error().Err(err).Msg("fail to sign tx")
			return checkpoint, nil, false, err
		}
		elapse = time.Since(startKeySign)
		s.debugSigningEvent(perfID, "signature_produced", fmt.Sprintf("signed_tx_bytes=%d checkpoint_bytes=%d", len(signedTx), len(checkpoint)))
	}

	// looks like the transaction is already signed
	if len(signedTx) == 0 {
		if observation != nil {
			s.logger.Debug().
				Str("txid", observation.Tx).
				Int64("height", observation.BlockHeight).
				Msg("signed transaction is empty; returning recovered observation")
			s.debugSigningFinish(perfID, "recovered_observation", observation.Tx)
			return nil, observation, true, nil
		}
		s.logger.Debug().Msg("signed transaction is empty")
		s.debugSigningFinish(perfID, "empty_signed_tx", "")
		return nil, nil, false, nil
	}

	// store immediately after a successful sign so the signed payload is saved even if broadcast crashes
	item.SignedTx = signedTx
	if storeErr := s.storage.Set(*item); storeErr != nil {
		s.logger.Error().Err(storeErr).Msg("fail to persist signed tx before broadcast")
		s.debugSigningEvent(perfID, "signed_tx_persist_error", storeErr.Error())
	} else {
		s.debugSigningEvent(perfID, "signed_tx_persisted", fmt.Sprintf("bytes=%d", len(signedTx)))
	}

	// broadcast the transaction
	s.debugSigningEvent(perfID, "broadcast_start", "")
	hash, err := chain.BroadcastTx(tx, signedTx)
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to broadcast tx to chain")

		// store the signed tx for the next retry
		item.SignedTx = signedTx
		item.Observation = observation
		if storeErr := s.storage.Set(*item); storeErr != nil {
			s.logger.Error().Err(storeErr).Msg("fail to update tx out store item with signed tx")
		}

		return nil, observation, false, err
	}
	s.debugSigningEvent(perfID, "broadcast_complete", hash)
	s.logger.Info().
		Str("chain", chain.GetChain().String()).
		Str("txid", hash).
		Msg("broadcasted tx to chain")

	if len(batchItems) > 0 {
		if err := chain.MarkTxBatchSigned(batchItems, hash); err != nil {
			s.logger.Error().Err(err).Str("txid", hash).Msg("fail to mark BTC txout batch as signed")
		} else {
			s.debugSigningEvent(perfID, "batch_marked_signed", fmt.Sprintf("items=%d", len(batchItems)))
		}
	}

	s.frostKeysignMetricMgr.SetFrostKeysignMetric(hash, elapse.Milliseconds())

	s.debugSigningFinish(perfID, "finished", hash)
	return nil, observation, false, nil
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

func (s *Signer) submitMissingSourceErrata(txID common.TxID, chain common.Chain) bool {
	key := missingSourceErrataKey(txID, chain)
	s.missingErrataMu.Lock()
	if _, ok := s.missingErrataSubmitted[key]; ok {
		s.missingErrataMu.Unlock()
		s.logger.Debug().
			Stringer("missing_source_tx", txID).
			Stringer("chain", chain).
			Msg("missing source tx errata already submitted")
		return true
	}
	s.missingErrataSubmitted[key] = struct{}{}
	s.missingErrataMu.Unlock()

	msg := s.thornadoBridge.GetErrataMsg(txID, chain)
	errataTxID, err := s.thornadoBridge.Broadcast(msg)
	if err != nil {
		s.missingErrataMu.Lock()
		delete(s.missingErrataSubmitted, key)
		s.missingErrataMu.Unlock()
		s.logger.Error().
			Err(err).
			Stringer("missing_source_tx", txID).
			Stringer("chain", chain).
			Msg("fail to submit missing source tx errata")
		return false
	}
	s.logger.Warn().
		Stringer("missing_source_tx", txID).
		Stringer("chain", chain).
		Stringer("errata_tx", errataTxID).
		Msg("submitted missing source tx errata")
	return true
}

func missingSourceErrataKey(txID common.TxID, chain common.Chain) string {
	return chain.String() + ":" + txID.String()
}

func isBatchableBaseOutbound(tx types.TxOutItem) bool {
	if !tx.Chain.Equals(common.BTCChain) || tx.VaultPathIndex != common.MainVaultPathIndex {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(tx.TxType)) {
	case ttypes.TxOutTypeOut, ttypes.TxOutTypeRefund, "":
		return true
	default:
		return false
	}
}

func sameBatchSource(a, b types.TxOutItem) bool {
	return a.Chain.Equals(b.Chain) &&
		a.VaultPubKey.Equals(b.VaultPubKey) &&
		a.VaultPathIndex == b.VaultPathIndex
}

func txOutBatchItems(txOut types.TxOut, height int64, representative types.TxOutItem) []types.TxOutItem {
	if txOut.Status != ttypes.TxOutStatusPendingSign || !isBatchableBaseOutbound(representative) {
		return nil
	}
	items := make([]types.TxOutItem, 0, len(txOut.TxArray))
	foundRepresentative := false
	for _, txArray := range txOut.TxArray {
		item := txArray.TxOutItem(height)
		if !item.OutHash.IsEmpty() || !isBatchableBaseOutbound(item) || !sameBatchSource(item, representative) {
			return nil
		}
		if item.Equals(representative) {
			foundRepresentative = true
		}
		items = append(items, item)
	}
	if !foundRepresentative || len(items) < 2 {
		return nil
	}
	return items
}

func (s *Signer) removeTxOutBatchItems(item TxOutStoreItem) {
	removed := 0
	for _, stored := range s.storage.List() {
		if stored.Height != item.Height {
			continue
		}
		if !sameBatchSource(stored.TxOutItem, item.TxOutItem) || !isBatchableBaseOutbound(stored.TxOutItem) {
			continue
		}
		if err := s.storage.Remove(stored); err != nil {
			s.logger.Error().
				Err(err).
				Int64("height", item.Height).
				Int64("index", stored.Index).
				Stringer("in_hash", stored.TxOutItem.InHash).
				Msg("fail to remove completed txout batch item")
			continue
		}
		removed++
	}
	if removed > 0 {
		return
	}
	if err := s.storage.Remove(item); err != nil {
		s.logger.Error().Err(err).Msg("fail to remove completed txout batch representative")
	}
}

func txOutBatchTerminalStatus(status string) bool {
	return status == ttypes.TxOutStatusComplete || status == ttypes.TxOutStatusCancelled
}

func (s *Signer) removeTerminalOrCompletedTxOut(item TxOutStoreItem) (TxOutStoreItem, bool) {
	item = s.refreshTxOutBatchMetadata(item)
	if s.removeUnsignedTxOutMissingFromThornado(item) {
		return item, true
	}
	if item.BatchStatus != "" && item.BatchStatus != ttypes.TxOutStatusPendingSign {
		if txOutBatchTerminalStatus(item.BatchStatus) {
			s.logger.Info().
				Str("chain", item.TxOutItem.Chain.String()).
				Int64("height", item.Height).
				Str("status", item.BatchStatus).
				Stringer("in_hash", item.TxOutItem.InHash).
				Msg("removing terminal txout batch item from signer storage")
			if err := s.storage.Remove(item); err != nil {
				s.logger.Error().Err(err).Msg("fail to remove terminal tx out store item")
			}
			return item, true
		}
		s.logger.Debug().
			Int64("height", item.Height).
			Uint64("epoch", item.Epoch).
			Str("status", item.BatchStatus).
			Msg("skipping txout batch until it is pending_sign")
		return item, true
	}
	if completed, outHash, err := s.txOutItemCompleted(item); err != nil {
		s.logger.Debug().Err(err).Interface("tx", item.TxOutItem).Msg("fail to check txout completion before signing")
	} else if completed {
		s.logger.Info().
			Str("chain", item.TxOutItem.Chain.String()).
			Stringer("in_hash", item.TxOutItem.InHash).
			Stringer("out_hash", outHash).
			Msg("removing completed txout from signer storage")
		if err := s.storage.Remove(item); err != nil {
			s.logger.Error().Err(err).Msg("fail to remove completed tx out store item")
		}
		return item, true
	}
	if completed, outHash, err := s.txOutItemCompletedInHistory(item); err != nil {
		s.logger.Debug().Err(err).Interface("tx", item.TxOutItem).Msg("fail to check historical txout completion before signing")
	} else if completed {
		s.logger.Info().
			Str("chain", item.TxOutItem.Chain.String()).
			Stringer("in_hash", item.TxOutItem.InHash).
			Stringer("out_hash", outHash).
			Msg("removing historically completed txout from signer storage")
		if err := s.storage.Remove(item); err != nil {
			s.logger.Error().Err(err).Msg("fail to remove historically completed tx out store item")
		}
		return item, true
	}
	return item, false
}

func deferredRecoveredObservationTxIn(item TxOutStoreItem) (types.TxIn, bool) {
	if item.Observation == nil {
		return types.TxIn{}, false
	}
	if len(item.SignedTx) > 0 || len(item.Checkpoint) > 0 {
		return types.TxIn{}, false
	}
	return types.TxIn{
		Chain:                  item.TxOutItem.Chain,
		TxArray:                []*types.TxInItem{item.Observation},
		MemPool:                false,
		Filtered:               true,
		ConfirmationRequired:   0,
		AllowFutureObservation: true,
	}, true
}

func (s *Signer) processTerminalOrCompletedTransaction(item TxOutStoreItem) (TxOutStoreItem, bool) {
	return s.removeTerminalOrCompletedTxOut(item)
}

func (s *Signer) processDeferredTransaction(item TxOutStoreItem) {
	s.processTerminalOrCompletedTransaction(item)
}

func (s *Signer) processTransaction(item TxOutStoreItem) {
	var handled bool
	item, handled = s.processTerminalOrCompletedTransaction(item)
	if handled {
		return
	}

	s.logger.Debug().
		Str("chain", item.TxOutItem.Chain.String()).
		Int64("height", item.Height).
		Int("status", int(item.Status)).
		Stringer("in_hash", item.TxOutItem.InHash).
		Str("tx_type", item.TxOutItem.TxType).
		Msg("Signing transaction")

	// a single FROST keysign should not take longer than 5 minutes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	checkpoint, obs, recoveredObs, err := s.signAndBroadcast(ctx, &item)
	if err != nil {
		if errors.Is(err, errNotDesignatedFrostSigner) {
			if deferErr := s.deferFrostKeysignRetry(item); deferErr != nil {
				s.logger.Debug().Err(deferErr).Msg("fail to defer non-designated FROST keysign retry")
			}
			cancel()
			return
		}
		var missingSource types.MissingSourceTxError
		if errors.As(err, &missingSource) {
			if s.submitMissingSourceErrata(missingSource.TxID, missingSource.Chain) {
				if storeErr := s.storage.Remove(item); storeErr != nil {
					s.logger.Error().Err(storeErr).Msg("fail to remove errata'd tx out store item")
				}
			}
			cancel()
			return
		}
		if errors.Is(err, errTxOutCompletionPending) {
			cancel()
			return
		}
		if errors.Is(err, errTxOutRemoved) {
			cancel()
			return
		}
		if errors.Is(err, errFrostLeaderUnavailable) {
			s.submitFrostLeaderUnavailable(item, err)
			if deferErr := s.deferFrostKeysignRetryWithNextLeader(item); deferErr != nil {
				s.logger.Debug().Err(deferErr).Msg("fail to defer FROST keysign retry to next leader")
			}
			cancel()
			return
		}

		// Always store the checkpoint of the built tx if we have one. This ensures the same
		// vault will never attempt multiple txs for an outbound, as further protection
		// against cases like https://gitlab.com/thornado/thornado/-/merge_requests/4344.
		// In tradeoff, we accept a higher likelihood of temporary stuck outbounds.
		item.Checkpoint = checkpoint

		// mark the txout on round 7 failure to block other txs for the chain / pubkey
		if ksErr, ok := unwrapKeysignError(err); ok {
			if ksErr.IsRound7() {
				s.logger.Error().Err(err).Interface("tx", item.TxOutItem).Msg("round 7 signing error")
				item.Round7Retry = true
				if storeErr := s.storage.Set(item); storeErr != nil {
					s.logger.Error().Err(storeErr).Msg("fail to update tx out store item with round 7 retry")
				}
			} else if deferErr := s.deferFrostKeysignRetry(item); deferErr != nil {
				s.logger.Debug().Err(deferErr).Msg("fail to defer FROST keysign retry")
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

	// Freshly signed outbounds are observed automatically. Recovered txouts are
	// left for explicit operator recovery through the manual observe command.
	if s.cfg.Signer.AutoObserve && obs != nil && !recoveredObs {
		s.logger.Info().
			Str("txid", obs.Tx).
			Int64("height", obs.BlockHeight).
			Bool("mempool", true).
			Bool("recovered", false).
			Msg("auto observing signed txout")
		s.observer.ObserveSigned(types.TxIn{
			Chain:                  item.TxOutItem.Chain,
			TxArray:                []*types.TxInItem{obs},
			MemPool:                true,
			Filtered:               true,
			ConfirmationRequired:   0,
			AllowFutureObservation: true,
		})
	}
	if recoveredObs {
		item.Observation = obs
		if ttypes.IsInternalTxOutType(item.TxOutItem.TxType) {
			if err = s.storage.Remove(item); err != nil {
				s.logger.Error().Err(err).Msg("fail to remove recovered internal FROST tx out store item")
			}
			return
		}
		if deferErr := s.deferFrostKeysignRetry(item); deferErr != nil {
			s.logger.Debug().Err(deferErr).Msg("fail to defer recovered FROST txout retry")
		}
		return
	}

	// We have a successful broadcast! Remove the item from our store
	if item.BatchStatus == ttypes.TxOutStatusPendingSign && isBatchableBaseOutbound(item.TxOutItem) {
		s.removeTxOutBatchItems(item)
	} else if err = s.storage.Remove(item); err != nil {
		s.logger.Error().Err(err).Msg("fail to update tx out store item")
	}
}
