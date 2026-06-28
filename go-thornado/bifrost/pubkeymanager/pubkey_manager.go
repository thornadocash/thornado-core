package pubkeymanager

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/constants"
)

// OnNewPubKey is a function that used as a callback , if somehow we need to do additional process when a new pubkey get added
type OnNewPubKey func(pk common.PubKey) error
type OnNewPubKeyPath func(pk common.PubKey, pathIndex uint64) error

// PubKeyValidator define the method that can be used to interact with public keys
type PubKeyValidator interface {
	IsValidVaultAddress(addr string, chain common.Chain) (bool, common.ChainVaultInfo)
	HasPubKey(pk common.PubKey) bool
	AddPubKey(pk common.PubKey, signer bool, algo common.SigningAlgo)
	AddNodePubKey(pk common.PubKey, algo common.SigningAlgo)
	RemovePubKey(pk common.PubKey)
	GetSignPubKeys() common.PubKeys
	GetNodePubKey(algo common.SigningAlgo) common.PubKey
	GetPubKeys() common.PubKeys
	GetAlgoPubKeys(algo common.SigningAlgo, includeInactive bool) common.PubKeys
	RegisterCallback(callback OnNewPubKey)
	RegisterPathCallback(callback OnNewPubKeyPath)
}

// pubKeyInfo is a struct to store pubkey information  in memory
type pubKeyInfo struct {
	PubKey      common.PubKey
	Signer      bool
	NodeAccount bool
	Algo        common.SigningAlgo
	Inactive    bool
}

// PubKeyManager manager an always up to date pubkeys , which implement PubKeyValidator interface
type PubKeyManager struct {
	bridge         thornadoclient.ThornadoBridge
	pubkeys        []pubKeyInfo
	rwMutex        *sync.RWMutex
	logger         zerolog.Logger
	errCounter     *prometheus.CounterVec
	m              *metrics.Metrics
	stopChan       chan struct{}
	callback       []OnNewPubKey
	pathCallback   []OnNewPubKeyPath
	vaultAddresses map[string]common.ChainVaultInfo
}

// NewPubKeyManager create a new instance of PubKeyManager
func NewPubKeyManager(bridge thornadoclient.ThornadoBridge, m *metrics.Metrics) (*PubKeyManager, error) {
	return &PubKeyManager{
		logger:         log.With().Str("module", "public_key_mgr").Logger(),
		bridge:         bridge,
		errCounter:     m.GetCounterVec(metrics.PubKeyManagerError),
		m:              m,
		stopChan:       make(chan struct{}),
		rwMutex:        &sync.RWMutex{},
		callback:       []OnNewPubKey{},
		pathCallback:   []OnNewPubKeyPath{},
		vaultAddresses: map[string]common.ChainVaultInfo{},
	}, nil
}

// Start to poll pubkeys from thornado
func (pkm *PubKeyManager) Start() error {
	pkm.fetchPubKeys(false)
	go pkm.updatePubKeys()
	return nil
}

// Stop pubkey manager
func (pkm *PubKeyManager) Stop() error {
	defer pkm.logger.Info().Msg("pubkey manager stopped")
	close(pkm.stopChan)
	return nil
}

// GetPubKeys return all the public keys managed by this PubKeyManager
func (pkm *PubKeyManager) GetPubKeys() common.PubKeys {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	pubkeys := make(common.PubKeys, len(pkm.pubkeys))
	for i, pk := range pkm.pubkeys {
		pubkeys[i] = pk.PubKey
	}
	return pubkeys
}

// GetAlgoPubKeys return all the public keys managed by this PubKeyManager
func (pkm *PubKeyManager) GetAlgoPubKeys(algo common.SigningAlgo, includeInactive bool) common.PubKeys {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	var pubkeys common.PubKeys
	for _, pk := range pkm.pubkeys {
		if pk.Algo != algo {
			continue
		}
		if !includeInactive && pk.Inactive {
			continue
		}
		pubkeys = append(pubkeys, pk.PubKey)
	}
	return pubkeys
}

// GetSignPubKeys get all the public keys that local node is a signer
func (pkm *PubKeyManager) GetSignPubKeys() common.PubKeys {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	pubkeys := make(common.PubKeys, 0)
	for _, pk := range pkm.pubkeys {
		if pk.Signer && !pk.NodeAccount && pk.Algo == common.SigningAlgoSecp256k1 {
			pubkeys = append(pubkeys, pk.PubKey)
		}
	}
	return pubkeys
}

// GetNodePubKey get node account pub key
func (pkm *PubKeyManager) GetNodePubKey(algo common.SigningAlgo) common.PubKey {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	for _, pk := range pkm.pubkeys {
		if pk.NodeAccount && pk.Algo == algo {
			return pk.PubKey
		}
	}
	return common.EmptyPubKey
}

// HasPubKey return true if the given public key exist
func (pkm *PubKeyManager) HasPubKey(pk common.PubKey) bool {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	return pkm.hasPubKeyNoLock(pk)
}

// hasPubKeyNoLock internal used only
func (pkm *PubKeyManager) hasPubKeyNoLock(pk common.PubKey) bool {
	for _, pubkey := range pkm.pubkeys {
		if pk.Equals(pubkey.PubKey) {
			return true
		}
	}
	return false
}

// AddPubKey add the given public key to internal storage
func (pkm *PubKeyManager) AddPubKey(pk common.PubKey, signer bool, algo common.SigningAlgo) {
	pkm.addPubKeyInternal(pk, signer, algo, false)
}

// addPubKeyInternal add the given public key to internal storage with inactive flag
func (pkm *PubKeyManager) addPubKeyInternal(pk common.PubKey, signer bool, algo common.SigningAlgo, inactive bool) {
	var newSecpKey bool

	pkm.rwMutex.Lock()
	if pkm.hasPubKeyNoLock(pk) {
		// pubkey already exists, update the signer and inactive status
		for i, pubkey := range pkm.pubkeys {
			if pk.Equals(pubkey.PubKey) {
				if signer {
					pkm.pubkeys[i].Signer = signer
				}
				pkm.pubkeys[i].Inactive = inactive
			}
		}
	} else {
		// pubkey doesn't exist yet, append it...
		pkm.pubkeys = append(pkm.pubkeys, pubKeyInfo{
			Algo:        algo,
			PubKey:      pk,
			Signer:      signer,
			NodeAccount: false,
			Inactive:    inactive,
		})
		if algo == common.SigningAlgoSecp256k1 {
			newSecpKey = true
		}
	}
	pkm.rwMutex.Unlock()

	if newSecpKey {
		pkm.fireCallback(pk)
		pkm.addDepositAddressLookahead(pk)
	}
}

func (pkm *PubKeyManager) addDepositAddressLookahead(pk common.PubKey) {
	type depositAddress struct {
		pathIndex uint64
		address   string
		info      common.ChainVaultInfo
	}

	addrs := make([]depositAddress, 0, common.DepositAddressLookahead*2)
	for _, pathType := range []common.VaultDepositPathType{common.VaultDepositPathUser, common.VaultDepositPathNode} {
		pathIndexes, err := common.VaultDepositLookaheadPathIndexes(pathType)
		if err != nil {
			pkm.logger.Error().Err(err).Str("pubkey", pk.String()).Str("path_type", string(pathType)).Msg("fail to derive deposit path lookahead")
			continue
		}
		for _, pathIndex := range pathIndexes {
			addr, err := common.DeriveBTCTaprootAddress(pk, pathIndex)
			if err != nil {
				pkm.logger.Error().Err(err).Str("pubkey", pk.String()).Uint64("path_index", pathIndex).Msg("fail to derive shielder deposit address")
				continue
			}
			addrs = append(addrs, depositAddress{
				pathIndex: pathIndex,
				address:   strings.ToLower(addr.String()),
				info: common.ChainVaultInfo{
					Chain:        common.BTCChain,
					PubKey:       pk,
					VaultAddress: addr,
				},
			})
		}
	}

	pkm.rwMutex.Lock()
	for _, item := range addrs {
		pkm.vaultAddresses[item.address] = item.info
	}
	pkm.rwMutex.Unlock()

	for _, item := range addrs {
		pkm.firePathCallback(pk, item.pathIndex)
	}
}

// AddNodePubKey add the given public key as a node public key to internal storage
func (pkm *PubKeyManager) AddNodePubKey(pk common.PubKey, algo common.SigningAlgo) {
	var newKey bool
	var existingKey bool

	pkm.rwMutex.Lock()
	for i, pubkey := range pkm.pubkeys {
		if pubkey.PubKey.Equals(pk) {
			pkm.pubkeys[i].Signer = true
			pkm.pubkeys[i].NodeAccount = true
			existingKey = true
			break
		}
	}

	if !existingKey {
		pkm.pubkeys = append(pkm.pubkeys, pubKeyInfo{
			PubKey:      pk,
			Algo:        algo,
			Signer:      true,
			NodeAccount: true,
		})
		newKey = true
	}
	pkm.rwMutex.Unlock()

	if newKey {
		pkm.fireCallback(pk)
	}
	// The pubkey manager can start before the signer has loaded its node
	// pubkey. Refresh vault membership now so existing vaults are marked as
	// signer targets for this node.
	pkm.fetchPubKeys(false)
}

// RemovePubKey remove the given public key from internal storage
func (pkm *PubKeyManager) RemovePubKey(pk common.PubKey) {
	pkm.rwMutex.Lock()
	defer pkm.rwMutex.Unlock()
	pkm.removePubKeyInternal(pk)
}

// removePubKeyInternal is a func to be used internally , and it doesn't lock the access to pkm.pubkeys
// caller need to lock pkm.pubkeys
func (pkm *PubKeyManager) removePubKeyInternal(pk common.PubKey) {
	for i, pubkey := range pkm.pubkeys {
		if pk.Equals(pubkey.PubKey) {
			pkm.pubkeys[i] = pkm.pubkeys[len(pkm.pubkeys)-1] // Copy last element to index i.
			pkm.pubkeys[len(pkm.pubkeys)-1] = pubKeyInfo{}   // Erase last element (write zero value).
			pkm.pubkeys = pkm.pubkeys[:len(pkm.pubkeys)-1]   // Truncate slice.
			break
		}
	}
}

func (pkm *PubKeyManager) fetchPubKeys(prune bool) {
	addressPairs, err := pkm.getPubkeys()
	if err != nil {
		pkm.logger.Error().Err(err).Msg("fail to get pubkeys from Thornado")
		return
	}
	nodePubKey := pkm.GetNodePubKey(common.SigningAlgoSecp256k1)
	var pubkeys common.PubKeys
	for _, pk := range addressPairs {
		signer := false
		for _, member := range pk.Membership {
			if member.Equals(nodePubKey) {
				signer = true
				break
			}
		}
		pkm.addPubKeyInternal(pk.PubKey, signer, pk.Algo, pk.Inactive)
		pubkeys = append(pubkeys, pk.PubKey)
	}
	if prune {
		pkm.rwMutex.Lock()
		defer pkm.rwMutex.Unlock()
		// prune retired addresses
		for i, pk := range pkm.pubkeys {
			if pk.NodeAccount {
				// never remove our own pubkey
				continue
			}
			if i < (len(pkm.pubkeys) - 2) { // don't delete the more recent (last) pubkeys
				if !pubkeys.Contains(pk.PubKey) {
					pkm.removePubKeyInternal(pk.PubKey)
				}
			}
		}
	}
}

func (pkm *PubKeyManager) updatePubKeys() {
	pkm.logger.Info().Msg("start to update pub keys")
	defer pkm.logger.Info().Msg("stop to update pub keys")
	for i := 1; ; i++ {
		select {
		case <-pkm.stopChan:
			return
		case <-time.After(constants.ThornadoBlockTime):
			pkm.fetchPubKeys(i%100 == 0) // only prune every 100 blocks
		}
	}
}

func matchAddress(addr string, chain common.Chain, key common.PubKey) (bool, common.ChainVaultInfo) {
	cpi, err := common.NewChainVaultInfo(chain, key)
	if err != nil {
		return false, common.NoChainVaultInfo
	}
	if strings.EqualFold(cpi.VaultAddress.String(), addr) {
		return true, cpi
	}
	return false, common.NoChainVaultInfo
}

// IsValidVaultAddress check whether the given address is a vault addr
func (pkm *PubKeyManager) IsValidVaultAddress(addr string, chain common.Chain) (bool, common.ChainVaultInfo) {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()

	if chain.Equals(common.BTCChain) {
		if cpi, ok := pkm.vaultAddresses[strings.ToLower(addr)]; ok {
			return true, cpi
		}
	}

	for _, pk := range pkm.pubkeys {
		// skip pubkeys with a different algo than the chain
		if chain.GetSigningAlgo() != pk.Algo {
			continue
		}

		ok, cpi := matchAddress(addr, chain, pk.PubKey)
		if ok {
			return ok, cpi
		}
	}
	return false, common.NoChainVaultInfo
}

// getPubkeys from Thornado
func (pkm *PubKeyManager) getPubkeys() ([]thornadoclient.PubKeyAddressPair, error) {
	return pkm.bridge.GetPubKeys()
}

// RegisterCallback register a call back that will be fired when a new key get added into the local memory storage
func (pkm *PubKeyManager) RegisterCallback(callback OnNewPubKey) {
	pkm.callback = append(pkm.callback, callback)
	for _, pk := range pkm.secpPubKeySnapshot() {
		if err := callback(pk); err != nil {
			pkm.logger.Err(err).Msg("fail to call callback")
		}
	}
}

func (pkm *PubKeyManager) RegisterPathCallback(callback OnNewPubKeyPath) {
	pkm.pathCallback = append(pkm.pathCallback, callback)
	for _, pk := range pkm.secpPubKeySnapshot() {
		pkm.addDepositAddressLookahead(pk)
		go pkm.fireDepositAddressLookaheadToCallback(pk, callback)
	}
}

func (pkm *PubKeyManager) secpPubKeySnapshot() common.PubKeys {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()

	pubkeys := make(common.PubKeys, 0)
	for _, pk := range pkm.pubkeys {
		if pk.NodeAccount || pk.Algo != common.SigningAlgoSecp256k1 {
			continue
		}
		pubkeys = append(pubkeys, pk.PubKey)
	}
	return pubkeys
}

func (pkm *PubKeyManager) fireDepositAddressLookaheadToCallback(pk common.PubKey, callback OnNewPubKeyPath) {
	for _, pathType := range []common.VaultDepositPathType{common.VaultDepositPathUser, common.VaultDepositPathNode} {
		pathIndexes, err := common.VaultDepositLookaheadPathIndexes(pathType)
		if err != nil {
			pkm.logger.Error().Err(err).Str("pubkey", pk.String()).Str("path_type", string(pathType)).Msg("fail to derive deposit path lookahead")
			continue
		}
		for _, pathIndex := range pathIndexes {
			if err := callback(pk, pathIndex); err != nil {
				pkm.logger.Err(err).Uint64("path_index", pathIndex).Msg("fail to call path callback")
			}
		}
	}
}

func (pkm *PubKeyManager) fireCallback(pk common.PubKey) {
	// fire callbacks in parallel and wait for all to complete
	wg := sync.WaitGroup{}
	for _, item := range pkm.callback {
		wg.Add(1)
		go func(item OnNewPubKey) {
			if err := item(pk); err != nil {
				pkm.logger.Err(err).Msg("fail to call callback")
			}
			wg.Done()
		}(item)
	}
	wg.Wait()
}

func (pkm *PubKeyManager) firePathCallback(pk common.PubKey, pathIndex uint64) {
	wg := sync.WaitGroup{}
	for _, item := range pkm.pathCallback {
		wg.Add(1)
		go func(item OnNewPubKeyPath) {
			if err := item(pk, pathIndex); err != nil {
				pkm.logger.Err(err).Uint64("path_index", pathIndex).Msg("fail to call path callback")
			}
			wg.Done()
		}(item)
	}
	wg.Wait()
}
