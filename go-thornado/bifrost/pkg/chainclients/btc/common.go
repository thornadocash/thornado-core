package btc

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
)

// BaseCache holds cached base addresses and the timestamp they were fetched.
type BaseCache struct {
	Addresses []common.Address
	FetchedAt time.Time
}

func GetBaseAddress(chain common.Chain, bridge thornadoclient.ThornadoBridge) ([]common.Address, error) {
	vaults, err := bridge.GetBasePubKeys()
	if err != nil {
		return nil, fmt.Errorf("fail to get baseVaults : %w", err)
	}

	newAddresses := make([]common.Address, 0)
	for _, v := range vaults {
		// we only care about secp256k1 keys
		if v.Algo != common.SigningAlgoSecp256k1 {
			continue
		}

		var addr common.Address
		addr, err = v.PubKey.GetAddress(chain)
		if err != nil {
			continue
		}
		newAddresses = append(newAddresses, addr)
		if chain.Equals(common.BTCChain) {
			for _, pathType := range []common.VaultDepositPathType{common.VaultDepositPathUser, common.VaultDepositPathNode} {
				pathIndexes, err := common.VaultDepositLookaheadPathIndexes(pathType)
				if err != nil {
					continue
				}
				for _, pathIndex := range pathIndexes {
					derived, err := common.DeriveBTCTaprootAddress(v.PubKey, pathIndex)
					if err != nil {
						continue
					}
					newAddresses = append(newAddresses, derived)
				}
			}
		}
	}
	return newAddresses, nil
}

// GetBaseAddressCached returns base addresses from a per-client cache when fresh,
// otherwise refreshes the cache from Thornado and preserves last-known addresses when
// refresh fails or returns an empty set.
//
// When refresh fails and stale cache exists, cached addresses are returned together with
// the refresh error so callers can decide whether/how to surface it.
func GetBaseAddressCached(cache *atomic.Pointer[BaseCache], chain common.Chain, bridge thornadoclient.ThornadoBridge, ttl time.Duration) ([]common.Address, error) {
	cached := cache.Load()
	if cached != nil && time.Since(cached.FetchedAt) < ttl {
		return cached.Addresses, nil
	}

	newAddresses, err := GetBaseAddress(chain, bridge)
	if err != nil {
		if cached != nil {
			return cached.Addresses, err
		}
		return nil, err
	}

	if len(newAddresses) > 0 {
		cache.Store(&BaseCache{
			Addresses: newAddresses,
			FetchedAt: time.Now(),
		})
		return newAddresses, nil
	}

	if cached != nil {
		return cached.Addresses, nil
	}

	return []common.Address{}, nil
}
