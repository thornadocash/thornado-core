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

type BaseCacheTestSuite struct{}

var _ = Suite(&BaseCacheTestSuite{})

type mockBaseBridge struct {
	thornadoclient.ThornadoBridge
	basePubKeys []thornadoclient.PubKeyAddressPair
	depositAddr common.Address
	err         error
	calls       int
}

func (m *mockBaseBridge) GetBasePubKeys() ([]thornadoclient.PubKeyAddressPair, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.basePubKeys, nil
}

func (m *mockBaseBridge) IsVaultDepositAddress(address common.Address) bool {
	return !m.depositAddr.IsEmpty() && m.depositAddr.Equals(address)
}

func makeBasePubKeyPair(c *C) thornadoclient.PubKeyAddressPair {
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
func (s *BaseCacheTestSuite) TestGetBaseAddressCachedFreshHit(c *C) {
	chain := common.BTCChain
	pair := makeBasePubKeyPair(c)
	cachedAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}

	var cache atomic.Pointer[BaseCache]
	cache.Store(&BaseCache{
		Addresses: cachedAddresses,
		FetchedAt: time.Now(),
	})

	bridge := &mockBaseBridge{}
	addresses, err := GetBaseAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, DeepEquals, cachedAddresses)
	c.Assert(bridge.calls, Equals, 0)
}

// A cache miss should refresh addresses from the bridge and store them.
func (s *BaseCacheTestSuite) TestGetBaseAddressCachedRefreshSuccess(c *C) {
	chain := common.BTCChain
	pair := makeBasePubKeyPair(c)
	refreshedAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}
	firstDepositPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	firstDepositAddress, err := common.DeriveBTCTaprootAddress(pair.PubKey, firstDepositPath)
	c.Assert(err, IsNil)

	var cache atomic.Pointer[BaseCache]
	bridge := &mockBaseBridge{
		basePubKeys: []thornadoclient.PubKeyAddressPair{pair},
	}

	addresses, err := GetBaseAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses[0], Equals, refreshedAddresses[0])
	c.Assert(addresses[1], Equals, firstDepositAddress)
	c.Assert(addresses, HasLen, int(common.DepositAddressLookahead)*2+1)
	c.Assert(bridge.calls, Equals, 1)
	c.Assert(cache.Load(), NotNil)
	c.Assert(cache.Load().Addresses, DeepEquals, addresses)
}

// When refresh fails, stale cached addresses should still be returned.
func (s *BaseCacheTestSuite) TestGetBaseAddressCachedRefreshErrorWithStaleCache(c *C) {
	chain := common.BTCChain
	pair := makeBasePubKeyPair(c)
	staleAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}
	expectedErr := errors.New("bridge unavailable")

	var cache atomic.Pointer[BaseCache]
	cache.Store(&BaseCache{
		Addresses: staleAddresses,
		FetchedAt: time.Now().Add(-2 * time.Second),
	})

	bridge := &mockBaseBridge{err: expectedErr}
	addresses, err := GetBaseAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(errors.Is(err, expectedErr), Equals, true)
	c.Assert(addresses, DeepEquals, staleAddresses)
	c.Assert(bridge.calls, Equals, 1)
}

// A refresh failure without cached data should be returned to the caller.
func (s *BaseCacheTestSuite) TestGetBaseAddressCachedRefreshErrorWithoutCache(c *C) {
	expectedErr := errors.New("bridge unavailable")

	var cache atomic.Pointer[BaseCache]
	bridge := &mockBaseBridge{err: expectedErr}
	addresses, err := GetBaseAddressCached(&cache, common.BTCChain, bridge, time.Second)

	c.Assert(errors.Is(err, expectedErr), Equals, true)
	c.Assert(addresses, IsNil)
	c.Assert(bridge.calls, Equals, 1)
}

// An empty refresh result should keep returning stale cached addresses.
func (s *BaseCacheTestSuite) TestGetBaseAddressCachedEmptyRefreshWithStaleCache(c *C) {
	chain := common.BTCChain
	pair := makeBasePubKeyPair(c)
	staleAddresses := []common.Address{expectedAddress(c, pair.PubKey, chain)}

	var cache atomic.Pointer[BaseCache]
	cache.Store(&BaseCache{
		Addresses: staleAddresses,
		FetchedAt: time.Now().Add(-2 * time.Second),
	})

	bridge := &mockBaseBridge{}
	addresses, err := GetBaseAddressCached(&cache, chain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, DeepEquals, staleAddresses)
	c.Assert(bridge.calls, Equals, 1)
}

// An empty refresh without cached data should return an empty slice.
func (s *BaseCacheTestSuite) TestGetBaseAddressCachedEmptyRefreshWithoutCache(c *C) {
	var cache atomic.Pointer[BaseCache]
	bridge := &mockBaseBridge{}

	addresses, err := GetBaseAddressCached(&cache, common.BTCChain, bridge, time.Second)

	c.Assert(err, IsNil)
	c.Assert(addresses, HasLen, 0)
	c.Assert(bridge.calls, Equals, 1)
	c.Assert(cache.Load(), IsNil)
}
