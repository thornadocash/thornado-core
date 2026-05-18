package utxo

import (
	"fmt"
	"sync/atomic"
	"time"

	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/common"
)

// AsgardCache holds cached asgard addresses and the timestamp they were fetched.
type AsgardCache struct {
	Addresses []common.Address
	FetchedAt time.Time
}

func GetAsgardAddress(chain common.Chain, bridge thorclient.ThorchainBridge) ([]common.Address, error) {
	vaults, err := bridge.GetAsgardPubKeys()
	if err != nil {
		return nil, fmt.Errorf("fail to get asgards : %w", err)
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
	}
	return newAddresses, nil
}

// GetAsgardAddressCached returns asgard addresses from a per-client cache when fresh,
// otherwise refreshes the cache from THORChain and preserves last-known addresses when
// refresh fails or returns an empty set.
//
// When refresh fails and stale cache exists, cached addresses are returned together with
// the refresh error so callers can decide whether/how to surface it.
func GetAsgardAddressCached(cache *atomic.Pointer[AsgardCache], chain common.Chain, bridge thorclient.ThorchainBridge, ttl time.Duration) ([]common.Address, error) {
	cached := cache.Load()
	if cached != nil && time.Since(cached.FetchedAt) < ttl {
		return cached.Addresses, nil
	}

	newAddresses, err := GetAsgardAddress(chain, bridge)
	if err != nil {
		if cached != nil {
			return cached.Addresses, err
		}
		return nil, err
	}

	if len(newAddresses) > 0 {
		cache.Store(&AsgardCache{
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
