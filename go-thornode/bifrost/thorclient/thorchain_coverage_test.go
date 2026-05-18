package thorclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	stypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// CoverageSuite covers functions not tested in the existing suites.
type CoverageSuite struct {
	server                    *httptest.Server
	cfg                       config.BifrostClientConfiguration
	bridge                    *thorchainBridge
	authAccountFixture        string
	nodeAccountFixture        string
	nodesFixture              string
	inboundAddressesFixture   string
	vaultFixture              string
	poolsFixture              string
	thornameFixture           string
	mimirFixture              string
	referenceMemoFixture      string
	referenceMemoByHashResult string
}

var _ = Suite(&CoverageSuite{})

func (s *CoverageSuite) SetUpTest(c *C) {
	cfg, _, kb := SetupThorchainForTest(c)
	s.cfg = cfg

	// Defaults
	s.authAccountFixture = "../../test/fixtures/endpoints/auth/accounts/template.json"
	s.nodeAccountFixture = "../../test/fixtures/endpoints/nodeaccount/template.json"
	s.nodesFixture = "../../test/fixtures/endpoints/nodeaccount/nodes.json"
	s.inboundAddressesFixture = "../../test/fixtures/endpoints/inbound_addresses/inbound_addresses_with_fee.json"
	s.vaultFixture = "../../test/fixtures/endpoints/vaults/asgard.json"
	s.poolsFixture = "../../test/fixtures/endpoints/pools/pools.json"
	s.thornameFixture = "../../test/fixtures/endpoints/thorname/thorname.json"
	s.mimirFixture = "../../test/fixtures/endpoints/mimir/mimir.json"
	s.referenceMemoFixture = ""
	s.referenceMemoByHashResult = ""

	s.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasPrefix(req.RequestURI, AuthAccountEndpoint):
			httpTestHandler(c, rw, s.authAccountFixture)
		case strings.HasPrefix(req.RequestURI, NodeAccountsEndpoint):
			httpTestHandler(c, rw, s.nodesFixture)
		case strings.HasPrefix(req.RequestURI, NodeAccountEndpoint):
			httpTestHandler(c, rw, s.nodeAccountFixture)
		case strings.HasPrefix(req.RequestURI, LastBlockEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/lastblock/eth.json")
		case strings.HasPrefix(req.RequestURI, InboundAddressesEndpoint):
			httpTestHandler(c, rw, s.inboundAddressesFixture)
		case strings.HasPrefix(req.RequestURI, "/thorchain/vault/"):
			httpTestHandler(c, rw, s.vaultFixture)
		case strings.HasPrefix(req.RequestURI, AsgardVault):
			httpTestHandler(c, rw, s.vaultFixture)
		case strings.HasPrefix(req.RequestURI, "/thorchain/vaults") && strings.HasSuffix(req.RequestURI, "/signers"):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/tss/keysign_party.json")
		case strings.HasPrefix(req.RequestURI, PubKeysEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/vaults/pubKeys.json")
		case strings.HasPrefix(req.RequestURI, PoolsEndpoint):
			httpTestHandler(c, rw, s.poolsFixture)
		case strings.HasPrefix(req.RequestURI, "/thorchain/thorname/"):
			httpTestHandler(c, rw, s.thornameFixture)
		case strings.HasPrefix(req.RequestURI, MimirEndpoint):
			httpTestHandler(c, rw, s.mimirFixture)
		case strings.HasPrefix(req.RequestURI, ConstantsEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/constants/constants.json")
		case strings.HasPrefix(req.RequestURI, RagnarokEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/ragnarok/ragnarok.json")
		case strings.HasPrefix(req.RequestURI, StatusEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/status/status.json")
		case strings.HasPrefix(req.RequestURI, "/thorchain/memo/"):
			// Determine which fixture to use based on which is set
			if s.referenceMemoFixture != "" {
				_, err := rw.Write([]byte(s.referenceMemoFixture))
				c.Assert(err, IsNil)
			} else if s.referenceMemoByHashResult != "" {
				_, err := rw.Write([]byte(s.referenceMemoByHashResult))
				c.Assert(err, IsNil)
			}
		case strings.EqualFold(req.RequestURI, BroadcastTxsEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/txs/success.json")
		}
	}))
	s.cfg.ChainHost = s.server.Listener.Addr().String()
	s.cfg.ChainRPC = s.server.Listener.Addr().String()

	bridge, err := NewThorchainBridge(s.cfg, GetMetricForTest(c), NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd))
	c.Assert(err, IsNil)
	var ok bool
	s.bridge, ok = bridge.(*thorchainBridge)
	c.Assert(ok, Equals, true)
	s.bridge.httpClient.RetryMax = 1
}

func (s *CoverageSuite) TearDownTest(c *C) {
	s.server.Close()
}

// ---- MakeCodec ----

func (s *CoverageSuite) TestMakeCodec(c *C) {
	cdc := MakeCodec()
	c.Assert(cdc, NotNil)
}

// ---- GetConfig ----

func (s *CoverageSuite) TestGetConfig(c *C) {
	cfg := s.bridge.GetConfig()
	c.Assert(cfg.ChainID, Equals, s.cfg.ChainID)
	c.Assert(cfg.ChainHost, Equals, s.cfg.ChainHost)
}

// ---- HasNetworkFee ----

func (s *CoverageSuite) TestHasNetworkFee_Success(c *C) {
	has, err := s.bridge.HasNetworkFee(common.BTCChain)
	c.Assert(err, IsNil)
	c.Assert(has, Equals, true) // outbound_tx_size=250 > 0
}

func (s *CoverageSuite) TestHasNetworkFee_ZeroSize(c *C) {
	has, err := s.bridge.HasNetworkFee(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(has, Equals, false) // outbound_tx_size=0
}

func (s *CoverageSuite) TestHasNetworkFee_ChainNotFound(c *C) {
	has, err := s.bridge.HasNetworkFee(common.DOGEChain)
	c.Assert(err, NotNil)
	c.Assert(has, Equals, false)
	c.Assert(strings.Contains(err.Error(), "no inbound address found"), Equals, true)
}

// ---- GetNetworkFee ----

func (s *CoverageSuite) TestGetNetworkFee_Success(c *C) {
	txSize, feeRate, err := s.bridge.GetNetworkFee(common.BTCChain)
	c.Assert(err, IsNil)
	c.Assert(txSize, Equals, uint64(250))
	c.Assert(feeRate, Equals, uint64(25))
}

func (s *CoverageSuite) TestGetNetworkFee_ETH(c *C) {
	txSize, feeRate, err := s.bridge.GetNetworkFee(common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(txSize, Equals, uint64(80000))
	c.Assert(feeRate, Equals, uint64(30))
}

func (s *CoverageSuite) TestGetNetworkFee_ZeroFee(c *C) {
	txSize, feeRate, err := s.bridge.GetNetworkFee(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(txSize, Equals, uint64(0))
	c.Assert(feeRate, Equals, uint64(0))
}

// ---- GetVault ----

func (s *CoverageSuite) TestGetVault_Success(c *C) {
	// Use the asgard fixture which is a JSON array; the endpoint /thorchain/vault/{pk}
	// returns a single vault. Let's write a single-vault fixture inline.
	singleVault := s.server
	singleVault.Close()

	s.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.RequestURI, "/thorchain/vault/") {
			rw.Header().Set("Content-Type", "application/json")
			_, err := rw.Write([]byte(`{
				"block_height": 100,
				"pub_key": "tthorpub1addwnpepqfe6kxcaq2jl4zyzdfvmpmt68lz2uvnutj20yy8sskkzvm5n94cgsavqhnr",
				"coins": [],
				"type": "AsgardVault",
				"status": "ActiveVault",
				"membership": []
			}`))
			c.Assert(err, IsNil)
		}
	}))
	s.bridge.cfg.ChainHost = s.server.Listener.Addr().String()

	vault, err := s.bridge.GetVault("tthorpub1addwnpepqfe6kxcaq2jl4zyzdfvmpmt68lz2uvnutj20yy8sskkzvm5n94cgsavqhnr")
	c.Assert(err, IsNil)
	c.Assert(vault.PubKey.String(), Equals, "tthorpub1addwnpepqfe6kxcaq2jl4zyzdfvmpmt68lz2uvnutj20yy8sskkzvm5n94cgsavqhnr")
}

// ---- GetAsgardPubKeys ----

func (s *CoverageSuite) TestGetAsgardPubKeys(c *C) {
	pks, err := s.bridge.GetAsgardPubKeys()
	c.Assert(err, IsNil)
	c.Assert(len(pks) > 0, Equals, true)
}

// ---- GetPools ----

func (s *CoverageSuite) TestGetPools(c *C) {
	pools, err := s.bridge.GetPools()
	c.Assert(err, IsNil)
	c.Assert(pools, NotNil)
	c.Assert(len(pools) > 0, Equals, true)
}

// ---- GetNodeAccounts ----

func (s *CoverageSuite) TestGetNodeAccounts(c *C) {
	nodes, err := s.bridge.GetNodeAccounts()
	c.Assert(err, IsNil)
	c.Assert(nodes, NotNil)
	c.Assert(len(nodes), Equals, 2)
}

// ---- FetchActiveNodes ----

func (s *CoverageSuite) TestFetchActiveNodes(c *C) {
	activeNodes, err := s.bridge.FetchActiveNodes()
	c.Assert(err, IsNil)
	// From nodes.json, only the first node has status "Active"
	c.Assert(len(activeNodes), Equals, 1)
}

// ---- FetchNodeStatus ----

func (s *CoverageSuite) TestFetchNodeStatus(c *C) {
	status, err := s.bridge.FetchNodeStatus()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, stypes.NodeStatus_Active)
}

func (s *CoverageSuite) TestFetchNodeStatus_Unknown(c *C) {
	s.nodeAccountFixture = "../../test/fixtures/endpoints/nodeaccount/disabled.json"
	status, err := s.bridge.FetchNodeStatus()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, stypes.NodeStatus_Unknown)
}

// ---- GetErrataMsg ----

func (s *CoverageSuite) TestGetErrataMsg(c *C) {
	txID, err := common.NewTxID("0000000000000000000000000000000000000000000000000000000000000001")
	c.Assert(err, IsNil)
	msg := s.bridge.GetErrataMsg(txID, common.ETHChain)
	c.Assert(msg, NotNil)
}

// ---- GetSolvencyMsg ----

func (s *CoverageSuite) TestGetSolvencyMsg(c *C) {
	pk := stypes.GetRandomPubKey()
	coins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
	}
	msg := s.bridge.GetSolvencyMsg(1024, common.ETHChain, pk, coins)
	c.Assert(msg, NotNil)
}

func (s *CoverageSuite) TestGetSolvencyMsg_EmptyCoins(c *C) {
	pk := stypes.GetRandomPubKey()
	coins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
	}
	// coins with zero amount get filtered by NoneEmpty
	msg := s.bridge.GetSolvencyMsg(1024, common.ETHChain, pk, coins)
	c.Assert(msg, NotNil)
}

// ---- GetKeygenStdTx ----

func (s *CoverageSuite) TestGetKeygenStdTx(c *C) {
	poolPubKey := stypes.GetRandomPubKey()
	inputPks := common.PubKeys{stypes.GetRandomPubKey(), stypes.GetRandomPubKey()}
	chains := common.Chains{common.ETHChain, common.BTCChain}
	msg, err := s.bridge.GetKeygenStdTx(
		poolPubKey,
		[]byte("sig"),
		[]byte("backup"),
		nil,
		inputPks,
		stypes.KeygenType_AsgardKeygen,
		chains,
		100,
		12345,
		common.EmptyPubKey,
		nil,
	)
	c.Assert(err, IsNil)
	c.Assert(msg, NotNil)
}

// ---- PostKeysignFailure ----

func (s *CoverageSuite) TestPostKeysignFailure(c *C) {
	blame := stypes.Blame{
		FailReason: "test failure",
	}
	pk := stypes.GetRandomPubKey()
	coins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
	}
	txID, err := s.bridge.PostKeysignFailure(blame, 1024, "memo", coins, pk)
	c.Assert(err, IsNil)
	c.Assert(txID.IsEmpty(), Equals, false)
}

func (s *CoverageSuite) TestPostKeysignFailure_EmptyBlame(c *C) {
	blame := stypes.Blame{} // FailReason is empty
	pk := stypes.GetRandomPubKey()
	coins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
	}
	txID, err := s.bridge.PostKeysignFailure(blame, 1024, "memo", coins, pk)
	c.Assert(err, IsNil)
	c.Assert(txID.IsEmpty(), Equals, false)
}

// ---- GetMimirWithRef ----

func (s *CoverageSuite) TestGetMimirWithRef(c *C) {
	result, err := s.bridge.GetMimirWithRef("Halt%sChain", "ETH")
	c.Assert(err, IsNil)
	c.Assert(result, Equals, int64(10))
}

// ---- GetReferenceMemo ----

func (s *CoverageSuite) TestGetReferenceMemo(c *C) {
	s.referenceMemoFixture = `{"memo": "SWAP:ETH.ETH:0x1234"}`
	memo, err := s.bridge.GetReferenceMemo(common.ETHChain, "ref123")
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "SWAP:ETH.ETH:0x1234")
}

// ---- GetReferenceMemoByTxHash ----

func (s *CoverageSuite) TestGetReferenceMemoByTxHash(c *C) {
	s.referenceMemoByHashResult = `{"memo": "SWAP:BTC.BTC:bc1q1234"}`
	s.referenceMemoFixture = "" // ensure first case doesn't match
	memo, err := s.bridge.GetReferenceMemoByTxHash("ABCD1234")
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "SWAP:BTC.BTC:bc1q1234")
}

// ---- GetInboundOutbound additional edge cases ----

func (s *CoverageSuite) TestGetInboundOutbound_Empty(c *C) {
	in, out, err := s.bridge.GetInboundOutbound(common.ObservedTxs{})
	c.Assert(err, IsNil)
	c.Assert(in, IsNil)
	c.Assert(out, IsNil)
}

func (s *CoverageSuite) TestGetInboundOutbound_InboundTx(c *C) {
	pk := stypes.GetRandomPubKey()
	vaultAddr, err := pk.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	tx := common.NewObservedTx(
		common.Tx{
			Coins: common.Coins{
				common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
			},
			Memo:        "SWAP:BTC.BTC",
			FromAddress: "0xd58610f89265a2fb637ac40edf59141ff873b266",
			ToAddress:   vaultAddr,
		},
		1,
		pk,
		1,
	)
	in, out, err := s.bridge.GetInboundOutbound(common.ObservedTxs{tx})
	c.Assert(err, IsNil)
	c.Assert(in, HasLen, 1)
	c.Assert(out, HasLen, 0)
}

func (s *CoverageSuite) TestGetInboundOutbound_CancelTransaction(c *C) {
	pk := stypes.GetRandomPubKey()
	vaultAddr, err := pk.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	// Cancel: to and from are the same vault address, with empty memo
	tx := common.NewObservedTx(
		common.Tx{
			Coins: common.Coins{
				common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
			},
			Memo:        "", // empty memo = cancel
			FromAddress: vaultAddr,
			ToAddress:   vaultAddr,
		},
		1,
		pk,
		1,
	)
	in, out, err := s.bridge.GetInboundOutbound(common.ObservedTxs{tx})
	c.Assert(err, IsNil)
	// Cancel tx: vaultToAddress=true, but isCancelTransaction=true, so it skips inbound;
	// vaultFromAddress=true, so it goes to outbound
	c.Assert(out, HasLen, 1)
	c.Assert(in, HasLen, 0)
}

func (s *CoverageSuite) TestGetInboundOutbound_DuplicateTx(c *C) {
	pk := stypes.GetRandomPubKey()
	vaultAddr, err := pk.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	tx := common.NewObservedTx(
		common.Tx{
			ID: "0000000000000000000000000000000000000000000000000000000000000001",
			Coins: common.Coins{
				common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
			},
			Memo:        "SWAP:BTC.BTC",
			FromAddress: "0xd58610f89265a2fb637ac40edf59141ff873b266",
			ToAddress:   vaultAddr,
		},
		1,
		pk,
		1,
	)
	// Send same tx twice
	in, out, err := s.bridge.GetInboundOutbound(common.ObservedTxs{tx, tx})
	c.Assert(err, IsNil)
	c.Assert(in, HasLen, 1) // second is duplicate, dropped
	c.Assert(out, HasLen, 0)
}

// ---- getThorChainURL with http prefix ----

func (s *CoverageSuite) TestGetThorChainURL_WithHTTPPrefix(c *C) {
	s.bridge.cfg.ChainHost = "http://localhost:1317"
	uri := s.bridge.getThorChainURL("thorchain/pools")
	c.Assert(uri, Equals, "http://localhost:1317/thorchain/pools")
}

// ---- NewThorchainBridge validation ----

func (s *CoverageSuite) TestNewThorchainBridge_EmptyChainID(c *C) {
	_, _, kb := SetupThorchainForTest(c)
	cfg := config.BifrostClientConfiguration{
		ChainID:    "",
		ChainHost:  "localhost",
		SignerName: "signer",
	}
	_, err := NewThorchainBridge(cfg, GetMetricForTest(c), NewKeysWithKeybase(kb, cfg.SignerName, "password"))
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "chain id is empty"), Equals, true)
}

func (s *CoverageSuite) TestNewThorchainBridge_EmptyChainHost(c *C) {
	_, _, kb := SetupThorchainForTest(c)
	cfg := config.BifrostClientConfiguration{
		ChainID:    "thorchain",
		ChainHost:  "",
		SignerName: "signer",
	}
	_, err := NewThorchainBridge(cfg, GetMetricForTest(c), NewKeysWithKeybase(kb, cfg.SignerName, "password"))
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "chain host is empty"), Equals, true)
}

// ---- PostNetworkFee when node not active ----

func (s *CoverageSuite) TestPostNetworkFee_NotActive(c *C) {
	// Use a dedicated server that always returns disabled node account
	disabledServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasPrefix(req.RequestURI, AuthAccountEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/auth/accounts/template.json")
		case strings.HasPrefix(req.RequestURI, NodeAccountEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/nodeaccount/disabled.json")
		case strings.HasPrefix(req.RequestURI, LastBlockEndpoint):
			httpTestHandler(c, rw, "../../test/fixtures/endpoints/lastblock/eth.json")
		}
	}))
	defer disabledServer.Close()

	s.bridge.cfg.ChainHost = disabledServer.Listener.Addr().String()
	txID, err := s.bridge.PostNetworkFee(1024, common.ETHChain, 100, 100)
	c.Assert(err, IsNil)
	c.Assert(txID, Equals, common.BlankTxID)
}

// ---- EnsureNodeWhitelisted with unknown status ----

func (s *CoverageSuite) TestEnsureNodeWhitelisted_UnknownStatus(c *C) {
	s.nodeAccountFixture = "../../test/fixtures/endpoints/nodeaccount/disabled.json"
	err := s.bridge.EnsureNodeWhitelisted()
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "node account status"), Equals, true)
}

// ---- PoolManager edge cases ----

func (s *CoverageSuite) TestPoolMgr_PoolNotFound(c *C) {
	poolMgr := NewPoolMgr(s.bridge)
	c.Assert(poolMgr, NotNil)

	// Use a non-existent asset
	nonExistentAsset, err := common.NewAsset("ETH.DOESNOTEXIST-0X0000000000000000000000000000000000000000")
	c.Assert(err, IsNil)
	_, err = poolMgr.GetValue(nonExistentAsset, common.ETHAsset, cosmos.NewUint(1000))
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "doesn't exist"), Equals, true)
}

func (s *CoverageSuite) TestPoolMgr_DestPoolNotFound(c *C) {
	poolMgr := NewPoolMgr(s.bridge)
	c.Assert(poolMgr, NotNil)

	// TKN pool exists but DOESNOTEXIST does not
	sourceAsset, err := common.NewAsset("ETH.TKN-0X3B7FA4DD21C6F9BA3CA375217EAD7CAB9D6BF483")
	c.Assert(err, IsNil)
	destAsset, err := common.NewAsset("ETH.DOESNOTEXIST-0X0000000000000000000000000000000000000000")
	c.Assert(err, IsNil)
	_, err = poolMgr.GetValue(sourceAsset, destAsset, cosmos.NewUint(1000))
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "doesn't exist"), Equals, true)
}

// ---- httpResponseCache caching test ----

func (s *CoverageSuite) TestGetCaching(c *C) {
	// First call should populate cache
	buf1, status1, err1 := s.bridge.getWithPath(PoolsEndpoint)
	c.Assert(err1, IsNil)
	c.Assert(status1, Equals, http.StatusOK)
	c.Assert(buf1, NotNil)

	// Second call (within cache window) should return same cached data
	buf2, status2, err2 := s.bridge.getWithPath(PoolsEndpoint)
	c.Assert(err2, IsNil)
	c.Assert(status2, Equals, http.StatusOK)
	c.Assert(string(buf1), Equals, string(buf2))
}

// ---- get with non-200 status ----

func (s *CoverageSuite) TestGet_Non200Status(c *C) {
	// When server returns non-200 with a body, get() returns the body, status, and error
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest) // 400 is not retried by retryablehttp
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	buf, status, err := s.bridge.getWithPath("/some/path")
	c.Assert(err, NotNil)
	c.Assert(status, Equals, http.StatusBadRequest)
	c.Assert(string(buf), Equals, "bad request")
}

// ---- GetAsgards error on non-200 ----

func (s *CoverageSuite) TestGetAsgards_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0 // disable retries
	_, err := s.bridge.GetAsgards()
	c.Assert(err, NotNil)
}

// ---- GetConstants error on non-200 ----

func (s *CoverageSuite) TestGetConstants_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusServiceUnavailable)
		_, _ = rw.Write([]byte("unavailable"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetConstants()
	c.Assert(err, NotNil)
}

// ---- RagnarokInProgress error on non-200 ----

func (s *CoverageSuite) TestRagnarokInProgress_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.RagnarokInProgress()
	c.Assert(err, NotNil)
}

// ---- GetThorchainVersion error on non-200 ----

func (s *CoverageSuite) TestGetThorchainVersion_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetThorchainVersion()
	c.Assert(err, NotNil)
}

// ---- GetMimir error on non-200 ----

func (s *CoverageSuite) TestGetMimir_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetMimir("SomeKey")
	c.Assert(err, NotNil)
}

// ---- GetPools error on non-200 ----

func (s *CoverageSuite) TestGetPools_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetPools()
	c.Assert(err, NotNil)
}

// ---- GetTHORName error on non-200 ----

func (s *CoverageSuite) TestGetTHORName_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetTHORName("test")
	c.Assert(err, NotNil)
}

// ---- GetContractAddress error on non-200 ----

func (s *CoverageSuite) TestGetContractAddress_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusServiceUnavailable)
		_, _ = rw.Write([]byte("unavailable"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetContractAddress()
	c.Assert(err, NotNil)
}

// ---- HasNetworkFee error on non-200 ----

func (s *CoverageSuite) TestHasNetworkFee_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.HasNetworkFee(common.ETHChain)
	c.Assert(err, NotNil)
}

// ---- GetVault error on non-200 ----

func (s *CoverageSuite) TestGetVault_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetVault("somepubkey")
	c.Assert(err, NotNil)
}

// ---- GetReferenceMemo error on non-200 ----

func (s *CoverageSuite) TestGetReferenceMemo_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetReferenceMemo(common.ETHChain, "ref123")
	c.Assert(err, NotNil)
}

// ---- GetReferenceMemoByTxHash error on non-200 ----

func (s *CoverageSuite) TestGetReferenceMemoByTxHash_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetReferenceMemoByTxHash("ABCD1234")
	c.Assert(err, NotNil)
}

// ---- GetContractAddress with multiple pubkeys ----

func (s *CoverageSuite) TestGetContractAddress_MultiplePubKeys(c *C) {
	pk1 := stypes.GetRandomPubKey()
	pk2 := stypes.GetRandomPubKey()
	multiPKServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		resp := []map[string]interface{}{
			{
				"chain":   "ETH",
				"pub_key": pk1.String(),
				"address": "0x1111",
				"router":  "0xRouter1",
				"halted":  false,
			},
			{
				"chain":   "BTC",
				"pub_key": pk1.String(),
				"address": "bc1q1111",
				"router":  "",
				"halted":  false,
			},
			{
				"chain":   "ETH",
				"pub_key": pk2.String(),
				"address": "0x2222",
				"router":  "0xRouter2",
				"halted":  false,
			},
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer multiPKServer.Close()

	s.bridge.cfg.ChainHost = multiPKServer.Listener.Addr().String()
	result, err := s.bridge.GetContractAddress()
	c.Assert(err, IsNil)
	c.Assert(len(result), Equals, 2) // pk1 and pk2
}

// ---- GetNodeAccount unmarshal error ----

func (s *CoverageSuite) TestGetNodeAccount_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`not json`))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetNodeAccount("tthor1test")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "failed to unmarshal"), Equals, true)
}

// ---- GetNodeAccounts unmarshal error ----

func (s *CoverageSuite) TestGetNodeAccounts_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`not json`))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetNodeAccounts()
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "failed to unmarshal"), Equals, true)
}

// ---- IsCatchingUp unmarshal error ----

func (s *CoverageSuite) TestIsCatchingUp_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`bad json`))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	s.bridge.cfg.ChainRPC = badServer.Listener.Addr().String()
	_, err := s.bridge.IsCatchingUp()
	c.Assert(err, NotNil)
}

// ---- GetLastObservedInHeight chain not found ----

func (s *CoverageSuite) TestGetLastObservedInHeight_ChainNotFound(c *C) {
	_, err := s.bridge.GetLastObservedInHeight(common.DOGEChain)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "fail to GetLastObservedInHeight"), Equals, true)
}

// ---- GetLastSignedOutHeight chain not found ----

func (s *CoverageSuite) TestGetLastSignedOutHeight_ChainNotFound(c *C) {
	_, err := s.bridge.GetLastSignedOutHeight(common.DOGEChain)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "fail to GetLastSignedOutHeight"), Equals, true)
}

// ---- Keys GetKeyringKeybase errors ----

func (s *CoverageSuite) TestGetKeyringKeybase_EmptySignerName(c *C) {
	_, _, err := GetKeyringKeybase("/tmp", "", "password")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "signer name is empty"), Equals, true)
}

func (s *CoverageSuite) TestGetKeyringKeybase_EmptyPassword(c *C) {
	_, _, err := GetKeyringKeybase("/tmp", "signer", "")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "password is empty"), Equals, true)
}

// ---- GetKeyringKeybase with non-existent signer ----

func (s *CoverageSuite) TestGetKeyringKeybase_NonExistentSigner(c *C) {
	_, _, err := GetKeyringKeybase("/tmp/nonexistent", "nonexistent_signer", "password")
	c.Assert(err, NotNil)
}

// ---- keygen error paths ----

func (s *CoverageSuite) TestGetKeygenBlock_NotFound(c *C) {
	notFoundServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}))
	defer notFoundServer.Close()

	s.bridge.cfg.ChainHost = notFoundServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetKeygenBlock(100, "pk")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetKeygenBlock_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetKeygenBlock(100, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "failed to unmarshal"), Equals, true)
}

func (s *CoverageSuite) TestGetKeygenBlock_EmptySignature(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keygen_block": map[string]interface{}{
				"height":  100,
				"keygens": []interface{}{},
			},
			"signature": "",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	_, err := s.bridge.GetKeygenBlock(100, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "invalid keygen signature: empty"), Equals, true)
}

func (s *CoverageSuite) TestGetKeygenBlock_InvalidSignature(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keygen_block": map[string]interface{}{
				"height":  100,
				"keygens": []interface{}{},
			},
			"signature": "not-valid-base64!!!",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	_, err := s.bridge.GetKeygenBlock(100, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "cannot decode signature"), Equals, true)
}

func (s *CoverageSuite) TestGetKeygenBlock_BadSignature(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keygen_block": map[string]interface{}{
				"height":  100,
				"keygens": []interface{}{},
			},
			"signature": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	_, err := s.bridge.GetKeygenBlock(100, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "bad signature"), Equals, true)
}

// ---- keysign error paths ----

func (s *CoverageSuite) TestGetKeysign_NotFound(c *C) {
	notFoundServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}))
	defer notFoundServer.Close()

	s.bridge.cfg.ChainHost = notFoundServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetKeysign(100, "pk")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetKeysign_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetKeysign(100, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "failed to unmarshal"), Equals, true)
}

func (s *CoverageSuite) TestGetKeysign_EmptyTxArray(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keysign": map[string]interface{}{
				"height":   1718,
				"tx_array": []interface{}{},
			},
			"signature": "",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	keysign, err := s.bridge.GetKeysign(1718, "pk")
	c.Assert(err, IsNil)
	c.Assert(keysign.TxArray, HasLen, 0) // empty tx_array returns without signature check
}

func (s *CoverageSuite) TestGetKeysign_EmptySignature(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keysign": map[string]interface{}{
				"height": 1718,
				"tx_array": []map[string]interface{}{
					{
						"chain":         "ETH",
						"to_address":    "0x1234",
						"vault_pub_key": "tthorpub1addwnpepqfe6kxcaq2jl4zyzdfvmpmt68lz2uvnutj20yy8sskkzvm5n94cgsavqhnr",
					},
				},
			},
			"signature": "",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	_, err := s.bridge.GetKeysign(1718, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "invalid keysign signature: empty"), Equals, true)
}

func (s *CoverageSuite) TestGetKeysign_InvalidSignatureBase64(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keysign": map[string]interface{}{
				"height": 1718,
				"tx_array": []map[string]interface{}{
					{
						"chain":         "ETH",
						"to_address":    "0x1234",
						"vault_pub_key": "tthorpub1addwnpepqfe6kxcaq2jl4zyzdfvmpmt68lz2uvnutj20yy8sskkzvm5n94cgsavqhnr",
					},
				},
			},
			"signature": "not-valid-base64!!!",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	_, err := s.bridge.GetKeysign(1718, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "cannot decode signature"), Equals, true)
}

func (s *CoverageSuite) TestGetKeysign_BadSignature(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"keysign": map[string]interface{}{
				"height": 1718,
				"tx_array": []map[string]interface{}{
					{
						"chain":         "ETH",
						"to_address":    "0x1234",
						"vault_pub_key": "tthorpub1addwnpepqfe6kxcaq2jl4zyzdfvmpmt68lz2uvnutj20yy8sskkzvm5n94cgsavqhnr",
					},
				},
			},
			"signature": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		}
		buf, _ := json.Marshal(resp)
		_, _ = rw.Write(buf)
	}))
	defer server.Close()

	s.bridge.cfg.ChainHost = server.Listener.Addr().String()
	_, err := s.bridge.GetKeysign(1718, "pk")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "bad signature"), Equals, true)
}

// ---- block height error paths ----

func (s *CoverageSuite) TestGetBlockHeight_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetBlockHeight()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetBlockHeight_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetBlockHeight()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetBlockHeight_EmptyResult(c *C) {
	emptyServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("[]"))
	}))
	defer emptyServer.Close()

	s.bridge.cfg.ChainHost = emptyServer.Listener.Addr().String()
	_, err := s.bridge.GetBlockHeight()
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "failed to GetThorchainHeight"), Equals, true)
}

func (s *CoverageSuite) TestGetLastObservedInHeight_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetLastObservedInHeight(common.ETHChain)
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetLastSignedOutHeight_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetLastSignedOutHeight(common.ETHChain)
	c.Assert(err, NotNil)
}

// ---- node_account error paths ----

func (s *CoverageSuite) TestGetNodeAccount_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetNodeAccount("tthor1test")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetNodeAccounts_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetNodeAccounts()
	c.Assert(err, NotNil)
}

// ---- thorchain.go error paths ----

func (s *CoverageSuite) TestGetAsgards_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetAsgards()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetVault_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetVault("pk")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetConstants_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetConstants()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestRagnarokInProgress_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.RagnarokInProgress()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetThorchainVersion_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetThorchainVersion()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetMimir_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetMimir("key")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetPools_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetPools()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetTHORName_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetTHORName("test")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetContractAddress_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetContractAddress()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetReferenceMemo_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetReferenceMemo(common.ETHChain, "ref")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetReferenceMemoByTxHash_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetReferenceMemoByTxHash("hash")
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestHasNetworkFee_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.HasNetworkFee(common.ETHChain)
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetNetworkFee_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, _, err := s.bridge.GetNetworkFee(common.ETHChain)
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetKeysignParty_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetKeysignParty(stypes.GetRandomPubKey())
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestFetchNodeStatus_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.FetchNodeStatus()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestFetchActiveNodes_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.FetchActiveNodes()
	c.Assert(err, NotNil)
}

// ---- PoolManager update pool error path ----

func (s *CoverageSuite) TestPoolMgr_UpdatePoolError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	poolMgr := NewPoolMgr(s.bridge)
	// getPool calls updatePool when lastCheck is old
	pool := poolMgr.getPool(common.BTCAsset)
	c.Assert(pool.IsEmpty(), Equals, true)
}

// ---- GetPubKeys/GetAsgardPubKeys error paths ----

func (s *CoverageSuite) TestGetPubKeys_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetPubKeys()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetAsgardPubKeys_Non200(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("bad gateway"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.GetAsgardPubKeys()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetPubKeys_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetPubKeys()
	c.Assert(err, NotNil)
}

func (s *CoverageSuite) TestGetAsgardPubKeys_UnmarshalError(c *C) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	}))
	defer badServer.Close()

	s.bridge.cfg.ChainHost = badServer.Listener.Addr().String()
	_, err := s.bridge.GetAsgardPubKeys()
	c.Assert(err, NotNil)
}

// ---- getAccountNumberAndSequenceNumber get error path ----

func (s *CoverageSuite) TestGetAccountNumberAndSequenceNumber_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, _, err := s.bridge.getAccountNumberAndSequenceNumber()
	c.Assert(err, NotNil)
}

// ---- IsCatchingUp get error ----

func (s *CoverageSuite) TestIsCatchingUp_GetError(c *C) {
	errServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	s.bridge.cfg.ChainHost = errServer.Listener.Addr().String()
	s.bridge.cfg.ChainRPC = errServer.Listener.Addr().String()
	s.bridge.httpClient.RetryMax = 0
	_, err := s.bridge.IsCatchingUp()
	c.Assert(err, NotNil)
}

// ---- GetInboundOutbound address mismatch ----

func (s *CoverageSuite) TestGetInboundOutbound_NeitherMatch(c *C) {
	pk := stypes.GetRandomPubKey()
	tx := common.NewObservedTx(
		common.Tx{
			Coins: common.Coins{
				common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
			},
			Memo:        "SWAP:BTC.BTC",
			FromAddress: "0xaaaa",
			ToAddress:   "0xbbbb",
		},
		1,
		pk,
		1,
	)
	in, out, err := s.bridge.GetInboundOutbound(common.ObservedTxs{tx})
	c.Assert(err, IsNil)
	// Neither to nor from matches vault, so tx is dropped
	c.Assert(in, HasLen, 0)
	c.Assert(out, HasLen, 0)
}
