package pubkeymanager

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func Test(t *testing.T) { TestingT(t) }

type PubKeyMgrSuite struct{}

var _ = Suite(&PubKeyMgrSuite{})

func (s *PubKeyMgrSuite) TestPubkeyMgr(c *C) {
	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()
	pk3 := types.GetRandomPubKey()
	pk4 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)
	c.Check(pubkeyMgr.HasPubKey(pk1), Equals, false)
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	c.Check(pubkeyMgr.HasPubKey(pk1), Equals, true)
	c.Check(pubkeyMgr.pubkeys[0].PubKey.Equals(pk1), Equals, true)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, true)

	pubkeyMgr.AddPubKey(pk2, false, common.SigningAlgoSecp256k1)
	c.Check(pubkeyMgr.HasPubKey(pk2), Equals, true)
	c.Check(pubkeyMgr.pubkeys[1].PubKey.Equals(pk2), Equals, true)
	c.Check(pubkeyMgr.pubkeys[1].Signer, Equals, false)

	pks := pubkeyMgr.GetPubKeys()
	c.Assert(pks, HasLen, 2)

	pks = pubkeyMgr.GetSignPubKeys()
	c.Assert(pks, HasLen, 1)
	c.Check(pks[0].Equals(pk1), Equals, true)

	// remove a pubkey
	pubkeyMgr.RemovePubKey(pk2)
	c.Check(pubkeyMgr.HasPubKey(pk2), Equals, false)

	addr, err := pk1.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	ok, _ := pubkeyMgr.IsValidPoolAddress(addr.String(), common.ETHChain)
	c.Assert(ok, Equals, true)

	addr, err = pk3.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	pubkeyMgr.AddNodePubKey(pk4, common.SigningAlgoSecp256k1)
	c.Check(pubkeyMgr.GetNodePubKey(common.SigningAlgoSecp256k1).String(), Equals, pk4.String())
	ok, _ = pubkeyMgr.IsValidPoolAddress(addr.String(), common.ETHChain)
	c.Assert(ok, Equals, false)
}

func (s *PubKeyMgrSuite) TestFetchKeys(c *C) {
	nodePk := types.GetRandomPubKey()
	vaultPk := types.GetRandomPubKey()
	extraPk := types.GetRandomPubKey()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Logf("================>:%s", r.RequestURI)
		if r.RequestURI == "/thorchain/vaults/pubkeys" {
			var result openapi.VaultPubkeysResponse
			chain := common.ETHChain.String()
			router := "0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44"
			result.Asgard = append(result.Asgard, openapi.VaultInfo{
				PubKey: vaultPk.String(),
				Routers: []openapi.VaultRouter{
					{
						Chain:  &chain,
						Router: &router,
					},
				},
				Membership: []string{nodePk.String()},
			})
			buf, err := json.MarshalIndent(result, "", "	")
			c.Assert(err, IsNil)
			if _, err = w.Write(buf); err != nil {
				c.Error(err)
			}
		}
	}))

	cfg := config.BifrostClientConfiguration{
		ChainID:   "thorchain",
		ChainHost: server.URL[7:],
	}
	bridge, err := thorclient.NewThorchainBridge(cfg, nil, nil)
	c.Assert(err, IsNil)
	pubkeyMgr, err := NewPubKeyManager(bridge, nil)
	c.Assert(err, IsNil)
	hasCallbackFired := false
	callBack := func(pk common.PubKey) error {
		hasCallbackFired = true
		return nil
	}
	pubkeyMgr.RegisterCallback(callBack)

	// add extra old pubkey that should get pruned
	pubkeyMgr.AddPubKey(extraPk, false, common.SigningAlgoSecp256k1)
	c.Check(hasCallbackFired, Equals, true)

	// add a key that is the node account, ensure it is not pruned
	pubkeyMgr.pubkeys = append(pubkeyMgr.pubkeys, pubKeyInfo{
		PubKey:      nodePk,
		Signer:      true,
		NodeAccount: true,
		Algo:        common.SigningAlgoSecp256k1,
		Contracts:   map[common.Chain]common.Address{},
	})
	c.Check(len(pubkeyMgr.GetPubKeys()), Equals, 2)
	err = pubkeyMgr.Start()
	c.Assert(err, IsNil)

	// fetch without prune
	pubkeyMgr.fetchPubKeys(false)
	pubKeys := pubkeyMgr.GetPubKeys()
	c.Check(len(pubKeys), Equals, 3)
	c.Check(pubKeys[0].Equals(extraPk), Equals, true)
	c.Check(pubKeys[1].Equals(nodePk), Equals, true)
	c.Check(pubKeys[2].Equals(vaultPk), Equals, true)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, false)
	c.Check(pubkeyMgr.pubkeys[1].Signer, Equals, true)
	c.Check(pubkeyMgr.pubkeys[2].Signer, Equals, true)
	c.Check(pubkeyMgr.pubkeys[0].NodeAccount, Equals, false)
	c.Check(pubkeyMgr.pubkeys[1].NodeAccount, Equals, true)
	c.Check(pubkeyMgr.pubkeys[2].NodeAccount, Equals, false)

	// fetch with prune, add 2 extra keys to ensure all the prior ones are in prune range
	pubkeyMgr.AddPubKey(types.GetRandomPubKey(), false, common.SigningAlgoSecp256k1)
	pubkeyMgr.AddPubKey(types.GetRandomPubKey(), false, common.SigningAlgoSecp256k1)
	pubkeyMgr.fetchPubKeys(true)
	pubKeys = pubkeyMgr.GetPubKeys()
	c.Check(len(pubKeys), Equals, 4)
	c.Check(pubKeys[0].Equals(extraPk), Equals, false) // pruned
	c.Check(pubKeys[1].Equals(nodePk), Equals, true)
	c.Check(pubKeys[2].Equals(vaultPk), Equals, true)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, false)
	c.Check(pubkeyMgr.pubkeys[1].Signer, Equals, true)
	c.Check(pubkeyMgr.pubkeys[2].Signer, Equals, true)
	c.Check(pubkeyMgr.pubkeys[3].Signer, Equals, false)
	c.Check(pubkeyMgr.pubkeys[0].NodeAccount, Equals, false)
	c.Check(pubkeyMgr.pubkeys[1].NodeAccount, Equals, true)
	c.Check(pubkeyMgr.pubkeys[2].NodeAccount, Equals, false)
	c.Check(pubkeyMgr.pubkeys[3].NodeAccount, Equals, false)

	err = pubkeyMgr.Stop()
	c.Assert(err, IsNil)
}

func (s *PubKeyMgrSuite) TestGetAlgoPubKeys(c *C) {
	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()
	pk3 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	// Add keys with different algos and inactive states
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	pubkeyMgr.AddPubKey(pk2, false, common.SigningAlgoEd25519)
	pubkeyMgr.addPubKeyInternal(pk3, false, common.SigningAlgoSecp256k1, true) // inactive

	// Filter by secp256k1, exclude inactive
	pks := pubkeyMgr.GetAlgoPubKeys(common.SigningAlgoSecp256k1, false)
	c.Assert(pks, HasLen, 1)
	c.Check(pks[0].Equals(pk1), Equals, true)

	// Filter by secp256k1, include inactive
	pks = pubkeyMgr.GetAlgoPubKeys(common.SigningAlgoSecp256k1, true)
	c.Assert(pks, HasLen, 2)

	// Filter by ed25519
	pks = pubkeyMgr.GetAlgoPubKeys(common.SigningAlgoEd25519, false)
	c.Assert(pks, HasLen, 1)
	c.Check(pks[0].Equals(pk2), Equals, true)

	// No match
	pks = pubkeyMgr.GetAlgoPubKeys(common.SigningAlgo("unknown"), false)
	c.Assert(pks, HasLen, 0)
}

func (s *PubKeyMgrSuite) TestGetContracts(c *C) {
	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()
	pk3 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	router1 := common.Address("0xRouter1")
	router2 := common.Address("0xRouter2")

	// Add keys with contracts
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	pubkeyMgr.AddPubKey(pk2, false, common.SigningAlgoSecp256k1)
	pubkeyMgr.addPubKeyInternal(pk3, false, common.SigningAlgoSecp256k1, true) // inactive

	// Set contracts directly
	pubkeyMgr.rwMutex.Lock()
	pubkeyMgr.pubkeys[0].Contracts = map[common.Chain]common.Address{
		common.ETHChain: router1,
	}
	pubkeyMgr.pubkeys[1].Contracts = map[common.Chain]common.Address{
		common.ETHChain: router2,
	}
	pubkeyMgr.pubkeys[2].Contracts = map[common.Chain]common.Address{
		common.ETHChain: common.Address("0xRouter3"),
	}
	pubkeyMgr.rwMutex.Unlock()

	// Get contracts excluding inactive
	contracts := pubkeyMgr.GetContracts(common.ETHChain, false)
	c.Assert(contracts, HasLen, 2)

	// Get contracts including inactive
	contracts = pubkeyMgr.GetContracts(common.ETHChain, true)
	c.Assert(contracts, HasLen, 3)

	// Get contracts for a chain with no entries
	contracts = pubkeyMgr.GetContracts(common.BTCChain, false)
	c.Assert(contracts, HasLen, 0)

	// Test duplicate router address dedup
	pubkeyMgr.rwMutex.Lock()
	pubkeyMgr.pubkeys[1].Contracts[common.ETHChain] = router1 // same as pk1's router
	pubkeyMgr.rwMutex.Unlock()
	contracts = pubkeyMgr.GetContracts(common.ETHChain, false)
	c.Assert(contracts, HasLen, 1) // deduped

	// Test with no contracts
	pubkeyMgr.rwMutex.Lock()
	pubkeyMgr.pubkeys[0].Contracts = map[common.Chain]common.Address{}
	pubkeyMgr.pubkeys[1].Contracts = map[common.Chain]common.Address{}
	pubkeyMgr.rwMutex.Unlock()
	contracts = pubkeyMgr.GetContracts(common.ETHChain, false)
	c.Assert(contracts, HasLen, 0)
}

func (s *PubKeyMgrSuite) TestGetContract(c *C) {
	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()
	pk3 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	router1 := common.Address("0xRouter1")

	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	pubkeyMgr.AddPubKey(pk2, false, common.SigningAlgoSecp256k1)

	// Set contract for pk1
	pubkeyMgr.rwMutex.Lock()
	pubkeyMgr.pubkeys[0].Contracts = map[common.Chain]common.Address{
		common.ETHChain: router1,
	}
	pubkeyMgr.rwMutex.Unlock()

	// Get contract for existing chain and pubkey
	addr := pubkeyMgr.GetContract(common.ETHChain, pk1)
	c.Check(addr, Equals, router1)

	// Get contract for pubkey with empty contracts
	addr = pubkeyMgr.GetContract(common.ETHChain, pk2)
	c.Check(addr, Equals, common.NoAddress)

	// Get contract for non-existent pubkey
	addr = pubkeyMgr.GetContract(common.ETHChain, pk3)
	c.Check(addr, Equals, common.NoAddress)

	// Get contract for wrong chain
	addr = pubkeyMgr.GetContract(common.BTCChain, pk1)
	c.Check(addr, Equals, common.Address(""))
}

func (s *PubKeyMgrSuite) TestUpdateContractAddresses(c *C) {
	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	pubkeyMgr.AddPubKey(pk2, false, common.SigningAlgoSecp256k1)

	router1 := common.Address("0xRouter1")
	pairs := []thorclient.PubKeyContractAddressPair{
		{
			PubKey: pk1,
			Contracts: map[common.Chain]common.Address{
				common.ETHChain: router1,
			},
		},
	}

	pubkeyMgr.updateContractAddresses(pairs)

	// Verify contract was updated
	pubkeyMgr.rwMutex.RLock()
	c.Check(pubkeyMgr.pubkeys[0].Contracts[common.ETHChain], Equals, router1)
	// pk2 should not be affected
	c.Check(len(pubkeyMgr.pubkeys[1].Contracts), Equals, 0)
	pubkeyMgr.rwMutex.RUnlock()
}

func (s *PubKeyMgrSuite) TestAddPubKeyInternalUpdateExisting(c *C) {
	pk1 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	// Add as non-signer
	pubkeyMgr.AddPubKey(pk1, false, common.SigningAlgoSecp256k1)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, false)
	c.Check(pubkeyMgr.pubkeys[0].Inactive, Equals, false)

	// Update to signer - should update existing entry
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	c.Check(len(pubkeyMgr.pubkeys), Equals, 1)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, true)

	// Adding with signer=false should NOT revert signer to false
	pubkeyMgr.AddPubKey(pk1, false, common.SigningAlgoSecp256k1)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, true) // stays true

	// Test inactive flag update
	pubkeyMgr.addPubKeyInternal(pk1, false, common.SigningAlgoSecp256k1, true)
	c.Check(pubkeyMgr.pubkeys[0].Inactive, Equals, true)

	// Setting inactive back to false
	pubkeyMgr.addPubKeyInternal(pk1, false, common.SigningAlgoSecp256k1, false)
	c.Check(pubkeyMgr.pubkeys[0].Inactive, Equals, false)
}

func (s *PubKeyMgrSuite) TestAddNodePubKeyExisting(c *C) {
	pk1 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	// Add as regular pubkey first
	pubkeyMgr.AddPubKey(pk1, false, common.SigningAlgoSecp256k1)
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, false)
	c.Check(pubkeyMgr.pubkeys[0].NodeAccount, Equals, false)

	// Promote to node pubkey
	pubkeyMgr.AddNodePubKey(pk1, common.SigningAlgoSecp256k1)
	c.Check(len(pubkeyMgr.pubkeys), Equals, 1) // no duplicate
	c.Check(pubkeyMgr.pubkeys[0].Signer, Equals, true)
	c.Check(pubkeyMgr.pubkeys[0].NodeAccount, Equals, true)
}

func (s *PubKeyMgrSuite) TestGetNodePubKeyNotFound(c *C) {
	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	pk1 := types.GetRandomPubKey()
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)

	// Search for ed25519 algo when only secp256k1 exists
	result := pubkeyMgr.GetNodePubKey(common.SigningAlgoEd25519)
	c.Check(result, Equals, common.EmptyPubKey)

	// Non-node account should not be returned
	result = pubkeyMgr.GetNodePubKey(common.SigningAlgoSecp256k1)
	c.Check(result, Equals, common.EmptyPubKey) // not a node account
}

func (s *PubKeyMgrSuite) TestFireCallbackWithError(c *C) {
	pk1 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	// Register a callback that returns an error
	errCalled := false
	pubkeyMgr.RegisterCallback(func(pk common.PubKey) error {
		errCalled = true
		return errors.New("callback error")
	})

	// Register a second callback that succeeds
	successCalled := false
	pubkeyMgr.RegisterCallback(func(pk common.PubKey) error {
		successCalled = true
		return nil
	})

	// Add a pubkey, which should fire both callbacks
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)
	c.Check(errCalled, Equals, true)
	c.Check(successCalled, Equals, true)
}

func (s *PubKeyMgrSuite) TestMatchAddress(c *C) {
	pk := types.GetRandomPubKey()

	// Valid match
	addr, err := pk.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	ok, cpi := matchAddress(addr.String(), common.ETHChain, pk)
	c.Check(ok, Equals, true)
	c.Check(cpi.PoolAddress.String(), Equals, addr.String())

	// Non-matching address
	ok, cpi = matchAddress("0x0000000000000000000000000000000000000000", common.ETHChain, pk)
	c.Check(ok, Equals, false)
	c.Check(cpi, Equals, common.EmptyChainPoolInfo)

	// Invalid chain/key combo that causes NewChainPoolInfo error
	ok, cpi = matchAddress("invalid", common.EmptyChain, pk)
	c.Check(ok, Equals, false)
	c.Check(cpi, Equals, common.EmptyChainPoolInfo)
}

func (s *PubKeyMgrSuite) TestRemovePubKeyNonExistent(c *C) {
	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)

	// Remove a key that doesn't exist - should not panic
	pubkeyMgr.RemovePubKey(pk2)
	c.Check(len(pubkeyMgr.pubkeys), Equals, 1)
	c.Check(pubkeyMgr.HasPubKey(pk1), Equals, true)
}

func (s *PubKeyMgrSuite) TestIsValidPoolAddressAlgoFiltering(c *C) {
	pk1 := types.GetRandomPubKey()

	pubkeyMgr, err := NewPubKeyManager(nil, nil)
	c.Assert(err, IsNil)

	// Add key with ed25519 algo
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoEd25519)

	// ETH chain uses secp256k1, so ed25519 key should not match
	addr, err := pk1.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	ok, _ := pubkeyMgr.IsValidPoolAddress(addr.String(), common.ETHChain)
	c.Check(ok, Equals, false)
}

func (s *PubKeyMgrSuite) TestFetchPubKeysError(c *C) {
	// Server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	cfg := config.BifrostClientConfiguration{
		ChainID:   "thorchain",
		ChainHost: server.URL[7:],
	}
	bridge, err := thorclient.NewThorchainBridge(cfg, nil, nil)
	c.Assert(err, IsNil)

	pubkeyMgr, err := NewPubKeyManager(bridge, nil)
	c.Assert(err, IsNil)

	// Add an existing key to verify it's not affected by failed fetch
	pk1 := types.GetRandomPubKey()
	pubkeyMgr.AddPubKey(pk1, true, common.SigningAlgoSecp256k1)

	// fetchPubKeys should handle the error gracefully
	pubkeyMgr.fetchPubKeys(false)
	c.Check(pubkeyMgr.HasPubKey(pk1), Equals, true)
}
