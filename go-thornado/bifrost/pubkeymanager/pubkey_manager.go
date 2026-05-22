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
	IsValidPoolAddress(addr string, chain common.Chain) (bool, common.ChainPoolInfo)
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
	GetContracts(chain common.Chain, includeInactive bool) []common.Address
	GetContract(chain common.Chain, pk common.PubKey) common.Address
}

// pubKeyInfo is a struct to store pubkey information  in memory
type pubKeyInfo struct {
	PubKey      common.PubKey
	Contracts   map[common.Chain]common.Address
	Signer      bool
	NodeAccount bool
	Algo        common.SigningAlgo
	Inactive    bool
}

// PubKeyManager manager an always up to date pubkeys , which implement PubKeyValidator interface
type PubKeyManager struct {
	bridge           thornadoclient.ThornadoBridge
	pubkeys          []pubKeyInfo
	rwMutex          *sync.RWMutex
	logger           zerolog.Logger
	errCounter       *prometheus.CounterVec
	m                *metrics.Metrics
	stopChan         chan struct{}
	callback         []OnNewPubKey
	pathCallback     []OnNewPubKeyPath
	depositAddresses map[string]common.ChainPoolInfo
}

// NewPubKeyManager create a new instance of PubKeyManager
func NewPubKeyManager(bridge thornadoclient.ThornadoBridge, m *metrics.Metrics) (*PubKeyManager, error) {
	return &PubKeyManager{
		logger:           log.With().Str("module", "public_key_mgr").Logger(),
		bridge:           bridge,
		errCounter:       m.GetCounterVec(metrics.PubKeyManagerError),
		m:                m,
		stopChan:         make(chan struct{}),
		rwMutex:          &sync.RWMutex{},
		callback:         []OnNewPubKey{},
		pathCallback:     []OnNewPubKeyPath{},
		depositAddresses: map[string]common.ChainPoolInfo{},
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

func (pkm *PubKeyManager) updateContractAddresses(pairs []thornadoclient.PubKeyContractAddressPair) {
	pkm.rwMutex.Lock()
	defer pkm.rwMutex.Unlock()
	for _, pair := range pairs {
		for idx, item := range pkm.pubkeys {
			if item.PubKey == pair.PubKey {
				pkm.pubkeys[idx].Contracts = pair.Contracts
			}
		}
	}
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
		if pk.Signer {
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
	pkm.rwMutex.Lock()
	defer pkm.rwMutex.Unlock()

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
			Contracts:   map[common.Chain]common.Address{},
			Inactive:    inactive,
		})
		if algo == common.SigningAlgoSecp256k1 {
			pkm.addDepositAddressLookaheadNoLock(pk)
		}
		pkm.fireCallback(pk)
	}
}

func (pkm *PubKeyManager) addDepositAddressLookaheadNoLock(pk common.PubKey) {
	for pathIndex := uint64(common.FirstDepositPathIndex); pathIndex <= common.DepositAddressLookahead; pathIndex++ {
		addr, err := common.DeriveBTCTaprootAddress(pk, pathIndex)
		if err != nil {
			pkm.logger.Error().Err(err).Str("pubkey", pk.String()).Uint64("path_index", pathIndex).Msg("fail to derive shielder deposit address")
			continue
		}
		pkm.depositAddresses[strings.ToLower(addr.String())] = common.ChainPoolInfo{
			Chain:       common.BTCChain,
			PubKey:      pk,
			PoolAddress: addr,
		}
		pkm.firePathCallback(pk, pathIndex)
	}
}

// AddNodePubKey add the given public key as a node public key to internal storage
func (pkm *PubKeyManager) AddNodePubKey(pk common.PubKey, algo common.SigningAlgo) {
	pkm.rwMutex.Lock()
	defer pkm.rwMutex.Unlock()

	for i, pubkey := range pkm.pubkeys {
		if pubkey.PubKey.Equals(pk) {
			pkm.pubkeys[i].Signer = true
			pkm.pubkeys[i].NodeAccount = true
			return
		}
	}

	if !pkm.hasPubKeyNoLock(pk) {
		pkm.pubkeys = append(pkm.pubkeys, pubKeyInfo{
			PubKey:      pk,
			Algo:        algo,
			Signer:      true,
			NodeAccount: true,
			Contracts:   map[common.Chain]common.Address{},
		})
		// a new pubkey get added , fire callback
		pkm.fireCallback(pk)
	}
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
	pkm.updateContractAddresses(addressPairs)

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

func matchAddress(addr string, chain common.Chain, key common.PubKey) (bool, common.ChainPoolInfo) {
	cpi, err := common.NewChainPoolInfo(chain, key)
	if err != nil {
		return false, common.NoChainPoolInfo
	}
	if strings.EqualFold(cpi.PoolAddress.String(), addr) {
		return true, cpi
	}
	return false, common.NoChainPoolInfo
}

// IsValidPoolAddress check whether the given address is a pool addr
func (pkm *PubKeyManager) IsValidPoolAddress(addr string, chain common.Chain) (bool, common.ChainPoolInfo) {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()

	if chain.Equals(common.BTCChain) {
		if cpi, ok := pkm.depositAddresses[strings.ToLower(addr)]; ok {
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
	return false, common.NoChainPoolInfo
}

// getPubkeys from Thornado
func (pkm *PubKeyManager) getPubkeys() ([]thornadoclient.PubKeyContractAddressPair, error) {
	return pkm.bridge.GetPubKeys()
}

// RegisterCallback register a call back that will be fired when a new key get added into the local memory storage
func (pkm *PubKeyManager) RegisterCallback(callback OnNewPubKey) {
	pkm.callback = append(pkm.callback, callback)
}

func (pkm *PubKeyManager) RegisterPathCallback(callback OnNewPubKeyPath) {
	pkm.pathCallback = append(pkm.pathCallback, callback)
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

// GetContracts return all the contracts for the requested chain.
// When includeInactive is false, only contracts from active/retiring vaults are returned.
func (pkm *PubKeyManager) GetContracts(chain common.Chain, includeInactive bool) []common.Address {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	var result []common.Address
	seen := map[common.Address]bool{} // avoid duplicates of router address
	for _, pk := range pkm.pubkeys {
		if !includeInactive && pk.Inactive {
			continue
		}
		if len(pk.Contracts) == 0 {
			continue
		}
		if addr, ok := pk.Contracts[chain]; ok {
			if seen[addr] {
				continue
			}
			seen[addr] = true
			result = append(result, addr)
		}
	}
	return result
}

// GetContract return the contract address that match the given chain and pubkey
func (pkm *PubKeyManager) GetContract(chain common.Chain, pubKey common.PubKey) common.Address {
	pkm.rwMutex.RLock()
	defer pkm.rwMutex.RUnlock()
	for _, pk := range pkm.pubkeys {
		if !pk.PubKey.Equals(pubKey) {
			continue
		}
		if len(pk.Contracts) == 0 {
			continue
		}
		return pk.Contracts[chain]
	}
	return common.NoAddress
}
