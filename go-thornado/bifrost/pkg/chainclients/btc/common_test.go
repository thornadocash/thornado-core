package btc

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/cometbft/cometbft/crypto/secp256k1"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
)

type AsgardCacheTestSuite struct{}

var _ = Suite(&AsgardCacheTestSuite{})

type mockAsgardBridge struct {
	thornadoclient.ThornadoBridge
	asgardPubKeys []thornadoclient.PubKeyAddressPair
	err           error
	calls         int
}

func (m *mockAsgardBridge) GetAsgardPubKeys() ([]thornadoclient.PubKeyAddressPair, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.asgardPubKeys, nil
}

func makeAsgardPubKeyPair(c *C) thornadoclient.PubKeyAddressPair {
	pubKey, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	c.Assert(err, IsNil)

	return thornadoclient.PubKeyAddressPair{
		PubKey: pubKey,
		Algo:   common.SigningAlgoSecp256k1,
	}
}

func expectedAddress(c *C, pubKey common.PubKey, chain common.Chain) common.Address {
	addr, err := pubKey.GetAddress(chain)
	c.Assert(err, IsNil)
	return addr
}

// Fresh cache entries should be returned without calling the bridge.
func (s *AsgardCacheTestSuite) TestGetAsgardAddressCachedFreshHit(c *C) {
	chain := common.BTCChain
	pair := makeAsgardPubKeyPair(c)
	cachedAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}

	var cache atomic.Pointer[AsgardCache]
	cache.Store(&AsgardCache{
		Addresses: cachedAddresses,
		FetchedAt: time.Now(),
	})

	bridge := &mockAsgardBridge{}
	addresses, err := GetAsgardAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, DeepEquals, cachedAddresses)
	c.Assert(bridge.calls, Equals, 0)
}

// A cache miss should refresh addresses from the bridge and store them.
func (s *AsgardCacheTestSuite) TestGetAsgardAddressCachedRefreshSuccess(c *C) {
	chain := common.BTCChain
	pair := makeAsgardPubKeyPair(c)
	refreshedAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}

	var cache atomic.Pointer[AsgardCache]
	bridge := &mockAsgardBridge{
		asgardPubKeys: []thornadoclient.PubKeyAddressPair{pair},
	}

	addresses, err := GetAsgardAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, DeepEquals, refreshedAddresses)
	c.Assert(bridge.calls, Equals, 1)
	c.Assert(cache.Load(), NotNil)
	c.Assert(cache.Load().Addresses, DeepEquals, refreshedAddresses)
}

// When refresh fails, stale cached addresses should still be returned.
func (s *AsgardCacheTestSuite) TestGetAsgardAddressCachedRefreshErrorWithStaleCache(c *C) {
	chain := common.BTCChain
	pair := makeAsgardPubKeyPair(c)
	staleAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}
	expectedErr := errors.New("bridge unavailable")

	var cache atomic.Pointer[AsgardCache]
	cache.Store(&AsgardCache{
		Addresses: staleAddresses,
		FetchedAt: time.Now().Add(-2 * time.Second),
	})

	bridge := &mockAsgardBridge{err: expectedErr}
	addresses, err := GetAsgardAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(errors.Is(err, expectedErr), Equals, true)
	c.Assert(addresses, DeepEquals, staleAddresses)
	c.Assert(bridge.calls, Equals, 1)
}

// A refresh failure without cached data should be returned to the caller.
func (s *AsgardCacheTestSuite) TestGetAsgardAddressCachedRefreshErrorWithoutCache(c *C) {
	expectedErr := errors.New("bridge unavailable")

	var cache atomic.Pointer[AsgardCache]
	bridge := &mockAsgardBridge{err: expectedErr}
	addresses, err := GetAsgardAddressCached(&cache, common.BTCChain, bridge, time.Second)

	c.Assert(errors.Is(err, expectedErr), Equals, true)
	c.Assert(addresses, IsNil)
	c.Assert(bridge.calls, Equals, 1)
}

// An empty refresh result should keep returning stale cached addresses.
func (s *AsgardCacheTestSuite) TestGetAsgardAddressCachedEmptyRefreshWithStaleCache(c *C) {
	chain := common.BTCChain
	pair := makeAsgardPubKeyPair(c)
	staleAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}

	var cache atomic.Pointer[AsgardCache]
	cache.Store(&AsgardCache{
		Addresses: staleAddresses,
		FetchedAt: time.Now().Add(-2 * time.Second),
	})

	bridge := &mockAsgardBridge{}
	addresses, err := GetAsgardAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, DeepEquals, staleAddresses)
	c.Assert(bridge.calls, Equals, 1)
}

// An empty refresh without cached data should return an empty slice.
func (s *AsgardCacheTestSuite) TestGetAsgardAddressCachedEmptyRefreshWithoutCache(c *C) {
	var cache atomic.Pointer[AsgardCache]
	bridge := &mockAsgardBridge{}

	addresses, err := GetAsgardAddressCached(&cache, common.BTCChain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, HasLen, 0)
	c.Assert(bridge.calls, Equals, 1)
	c.Assert(cache.Load(), IsNil)
}
