package observer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/evm"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
	types2 "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

type ObserverSuite struct {
	metrics  *metrics.Metrics
	thorKeys *thorclient.Keys
	bridge   thorclient.ThorchainBridge
	client   *evm.EVMClient
}

var _ = Suite(&ObserverSuite{})

////////////////////////////////////////////////////////////////////////////////////////
// Mock
////////////////////////////////////////////////////////////////////////////////////////

func (s *ObserverSuite) ResetMockClient(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.metrics)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	mockEvmRPC := httptest.NewServer(
		http.HandlerFunc(
			func(rw http.ResponseWriter, req *http.Request) {
				var body []byte
				body, err = io.ReadAll(req.Body)
				c.Assert(err, IsNil)
				type RPCRequest struct {
					JSONRPC string          `json:"jsonrpc"`
					ID      interface{}     `json:"id"`
					Method  string          `json:"method"`
					Params  json.RawMessage `json:"params"`
				}
				var rpcRequest RPCRequest
				err = json.Unmarshal(body, &rpcRequest)
				c.Assert(err, IsNil)
				if rpcRequest.Method == "eth_chainId" {
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0xf"}`))
					c.Assert(err, IsNil)
				}
				if rpcRequest.Method == "eth_blockNumber" {
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x7"}`))
					c.Assert(err, IsNil)
				}
			}))

	httpRequestTimeout, _ := time.ParseDuration("5s")

	s.client, err = evm.NewEVMClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			RPCHost:        mockEvmRPC.URL,
			SolvencyBlocks: 10,
			BlockScanner: config.BifrostBlockScannerConfiguration{
				HTTPRequestTimeout:  httpRequestTimeout,
				BlockScanProcessors: 1,
				ChainID:             common.AVAXChain,
				MaxHTTPRequestRetry: 10,
				EnforceBlockHeight:  true,
				GasCacheBlocks:      100,
			},
		},
		nil,
		s.bridge,
		s.metrics,
		pubkeyMgr,
		poolMgr,
	)

	c.Assert(err, IsNil)
	c.Assert(s.client, NotNil)
}

////////////////////////////////////////////////////////////////////////////////////////
// Setup
////////////////////////////////////////////////////////////////////////////////////////

func (s *ObserverSuite) SetUpSuite(c *C) {
	types2.SetupConfigForTest()

	server := httptest.NewServer(
		http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			switch {
			case strings.HasPrefix(req.RequestURI, thorclient.MimirEndpoint):
				buf, err := os.ReadFile("../../test/fixtures/endpoints/mimir/mimir.json")
				c.Assert(err, IsNil)
				_, err = rw.Write(buf)
				c.Assert(err, IsNil)
			case strings.HasPrefix(req.RequestURI, thorclient.ConstantsEndpoint):
				buf, err := os.ReadFile("../../test/fixtures/endpoints/constants/constants.json")
				c.Assert(err, IsNil)
				_, err = rw.Write(buf)
				c.Assert(err, IsNil)
			case strings.HasPrefix(req.RequestURI, "/thorchain/lastblock"):
				// NOTE: weird pattern in GetBlockHeight uses first thorchain height.
				_, err := rw.Write([]byte(`[
          {
            "chain": "NOOP",
            "lastobservedin": 0,
            "lastsignedout": 0,
            "thorchain": 0
          }
        ]`))
				c.Assert(err, IsNil)
			case strings.HasPrefix(req.RequestURI, "/"):
				_, err := rw.Write([]byte(`{
          "jsonrpc": "2.0",
          "id": 0,
          "result": {
            "height": "1",
            "hash": "E7FDA9DE4D0AD37D823813CB5BC0D6E69AB0D41BB666B65B965D12D24A3AE83C",
            "logs": [
              {
                "success": "true",
                "log": ""
              }
            ]
          }
        }`))
				c.Assert(err, IsNil)
			default:
				c.Errorf("invalid server query: %s", req.RequestURI)
			}
		}))

	s.metrics = getSharedTestMetrics()
	c.Assert(s.metrics, NotNil)

	var err error
	cfg := config.BifrostClientConfiguration{
		ChainID:      "thorchain",
		ChainHost:    server.Listener.Addr().String(),
		ChainRPC:     server.Listener.Addr().String(),
		SignerName:   "bob",
		SignerPasswd: "password",
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err = kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.thorKeys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)

	c.Assert(s.thorKeys, NotNil)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.metrics, s.thorKeys)
	c.Assert(s.bridge, NotNil)
	c.Assert(err, IsNil)
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	tmp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	_, err = common.NewPubKeyFromCrypto(tmp)
	c.Assert(err, IsNil)

	s.ResetMockClient(c)
}

////////////////////////////////////////////////////////////////////////////////////////
// Setup
////////////////////////////////////////////////////////////////////////////////////////

func (s *ObserverSuite) TestProcess(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.metrics)
	c.Assert(err, IsNil)
	comm, err := p2p.NewCommunication(&p2p.Config{
		RendezvousString: "rendezvous",
		Port:             1234,
	}, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(comm, NotNil)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	err = comm.Start(priv.Bytes())
	c.Assert(err, IsNil)

	defer func() {
		stopErr := comm.Stop()
		c.Assert(stopErr, IsNil)
	}()

	c.Assert(comm.GetHost(), NotNil)
	ag, err := NewAttestationGossip(comm.GetHost(), s.thorKeys, "localhost:50051", s.bridge, s.metrics, config.BifrostAttestationGossipConfig{})
	c.Assert(err, IsNil)
	obs, err := NewObserver(
		pubkeyMgr,
		map[common.Chain]chainclients.ChainClient{common.AVAXChain: s.client},
		s.bridge,
		s.metrics,
		"",
		metrics.NewTssKeysignMetricMgr(),
		ag,
		"",
	)
	c.Assert(obs, NotNil)
	c.Assert(err, IsNil)
	ag.SetObserverHandleObservedTxCommitted(obs)
	err = obs.Start(context.Background())
	c.Assert(err, IsNil)
	time.Sleep(time.Second * 2)
	metric, err := s.metrics.GetCounterVec(metrics.ObserverError).GetMetricWithLabelValues("fail_to_send_to_thorchain", "1")
	c.Assert(err, IsNil)
	c.Check(int(testutil.ToFloat64(metric)), Equals, 0)

	err = obs.Stop()
	c.Assert(err, IsNil)
}

func (s *ObserverSuite) TestErrataTx(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.metrics)
	c.Assert(err, IsNil)
	comm, err := p2p.NewCommunication(&p2p.Config{
		RendezvousString: "rendezvous",
		Port:             1234,
	}, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(comm, NotNil)
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	err = comm.Start(priv.Bytes())
	c.Assert(err, IsNil)

	defer func() {
		stopErr := comm.Stop()
		c.Assert(stopErr, IsNil)
	}()

	c.Assert(comm.GetHost(), NotNil)

	ag, err := NewAttestationGossip(comm.GetHost(), s.thorKeys, "localhost:50051", s.bridge, s.metrics, config.BifrostAttestationGossipConfig{})
	c.Assert(err, IsNil)

	obs, err := NewObserver(pubkeyMgr, nil, s.bridge, s.metrics, "", metrics.NewTssKeysignMetricMgr(), ag, "")
	c.Assert(obs, NotNil)
	c.Assert(err, IsNil)
	ag.SetObserverHandleObservedTxCommitted(obs)
	c.Assert(obs.attestationGossip.AttestSolvency(context.Background(), common.Solvency{
		Height: 25,
		Chain:  common.ETHChain,
		Id:     thorchain.GetRandomTxHash(),
	}), NotNil)
	bech32Pub, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, priv.PubKey())
	c.Assert(err, IsNil)
	ag.setActiveValidators(common.PubKeys{common.PubKey(bech32Pub)})
	c.Assert(obs.attestationGossip.AttestSolvency(context.Background(), common.Solvency{
		Height: 25,
		Chain:  common.ETHChain,
		Id:     thorchain.GetRandomTxHash(),
	}), IsNil)
}

func (s *ObserverSuite) TestObserverDeckStorage(c *C) {
	vault1 := types2.GetRandomPubKey()
	vault2 := types2.GetRandomPubKey()
	vault1Addr, err := vault1.GetAddress(common.BTCChain)
	c.Assert(err, IsNil)
	vault2Addr, err := vault2.GetAddress(common.BTCChain)
	c.Assert(err, IsNil)

	// Helper function to create a minimal Observer instance for testing
	setupObserver := func() (*Observer, string) {
		tempDir, err := os.MkdirTemp("", "observer-test")
		c.Assert(err, IsNil)

		pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.metrics)
		c.Assert(err, IsNil)
		pubkeyMgr.AddPubKey(vault1, false, common.SigningAlgoSecp256k1)
		pubkeyMgr.AddPubKey(vault2, false, common.SigningAlgoSecp256k1)

		ag, err := NewAttestationGossip(NewMockHost([]peer.ID{}), s.thorKeys, "localhost:50052", s.bridge, s.metrics, config.BifrostAttestationGossipConfig{})
		c.Assert(err, IsNil)
		obs, err := NewObserver(
			pubkeyMgr,
			map[common.Chain]chainclients.ChainClient{common.AVAXChain: s.client},
			s.bridge,
			s.metrics,
			tempDir,
			metrics.NewTssKeysignMetricMgr(),
			ag,
			"",
		)
		c.Assert(obs, NotNil)
		c.Assert(err, IsNil)

		return obs, tempDir
	}

	// Helper to check if TxIn exists in both memory and storage
	assertTxExists := func(observer *Observer, key txInKey, txID string) {
		// Check memory cache
		observer.lock.Lock()
		deckTx, exists := observer.onDeck[key]
		observer.lock.Unlock()
		c.Assert(exists, Equals, true, Commentf("Transaction not found in memory cache"))

		found := false
		for _, item := range deckTx.TxArray {
			if item.Tx == txID {
				found = true
				break
			}
		}
		c.Assert(found, Equals, true, Commentf("Transaction not found in memory cache array"))

		// Check storage
		storageTxs, err := observer.storage.GetOnDeckTxs()
		c.Assert(err, IsNil)
		found = false
		for _, tx := range storageTxs {
			for _, item := range tx.TxArray {
				if item.Tx == txID {
					found = true
					break
				}
			}
		}
		c.Assert(found, Equals, true, Commentf("Transaction not found in storage"))
	}

	// Helper to check if TxIn is removed from memory and storage after finalization
	assertTxFinalized := func(observer *Observer, key txInKey, txID string) {
		observer.lock.Lock()
		deckTx, exists := observer.onDeck[key]
		observer.lock.Unlock()

		if !exists {
			// If the entire deck is removed, that's acceptable
			storageTxs, err := observer.storage.GetOnDeckTxs()
			c.Assert(err, IsNil)

			found := false
			for _, tx := range storageTxs {
				for _, item := range tx.TxArray {
					if item.Tx == txID {
						found = true
						break
					}
				}
			}
			c.Assert(found, Equals, false, Commentf("Transaction should be removed from storage after finalization"))
			return
		}

		// If deck still exists, the specific tx should be removed from its array
		found := false
		for _, item := range deckTx.TxArray {
			if item.Tx == txID {
				found = true
				break
			}
		}
		c.Assert(found, Equals, false, Commentf("Transaction should be removed from memory cache array after finalization"))
	}

	// Test Case 1: tx observed in mempool, then non-mempool, then final
	{
		observer, tempDir := setupObserver()
		defer func() {
			observer.storage.Close()
			os.RemoveAll(tempDir)
		}()

		// Define test data
		chain := common.BTCChain
		blockHeight := int64(100)
		confirmRequired := int64(6)
		finalHeight := blockHeight + confirmRequired
		txID := types2.GetRandomTxHash()
		memo := "SWAP:ETH.ETH:0xe6a30f4f3bad978910e2cbb4d97581f5b5a0ade0"
		recipient := "recipient"
		coins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))}

		// 1. Observe transaction in mempool
		txInMempool := &types.TxIn{
			Chain:                chain,
			MemPool:              true,
			ConfirmationRequired: confirmRequired,
			TxArray: []*types.TxInItem{
				{
					BlockHeight: blockHeight,
					Tx:          txID.String(),
					Memo:        memo,
					Sender:      vault1Addr.String(),
					To:          recipient,
					Coins:       coins,
				},
			},
		}
		observer.processObservedTx(*txInMempool)

		// Verify transaction is in both memory and storage
		key := TxInKey(txInMempool)
		assertTxExists(observer, key, txID.String())

		// Simulate non-final observation in Thorchain
		observedTxMempool := common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: vault1Addr,
				ToAddress:   common.Address(recipient),
				Coins:       coins,
				Memo:        memo,
			},
			BlockHeight:    blockHeight, // Initial block height
			FinaliseHeight: finalHeight, // Final height is in the future
			ObservedPubKey: vault1,
		}

		observer.handleObservedTxCommitted(observedTxMempool)

		// Transaction should still exist but be marked as CommittedUnFinalised
		assertTxExists(observer, key, txID.String())

		// 2. Observe same transaction in a block (non-mempool)
		txInBlock := &types.TxIn{
			Chain:                chain,
			MemPool:              false,
			ConfirmationRequired: confirmRequired,
			TxArray: []*types.TxInItem{
				{
					BlockHeight: blockHeight,
					Tx:          txID.String(),
					Memo:        memo,
					Sender:      vault1Addr.String(),
					To:          recipient,
					Coins:       coins,
					// After first handleObservedTxCommitted, this should be marked as committed
					CommittedUnFinalised: true,
				},
			},
		}
		observer.processObservedTx(*txInBlock)

		// Should still be in both memory and storage
		assertTxExists(observer, key, txID.String())

		// Simulate non-final observation in Thorchain (still not enough confirmations)
		observedTxBlock := common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: vault1Addr,
				ToAddress:   common.Address(recipient),
				Coins:       coins,
				Memo:        memo,
			},
			BlockHeight:    blockHeight, // Still the same block height
			FinaliseHeight: finalHeight, // Final height is still in the future
			ObservedPubKey: vault1,
		}

		observer.handleObservedTxCommitted(observedTxBlock)

		// Should still exist (not finalized yet)
		assertTxExists(observer, key, txID.String())

		// 3. Transaction finalized (BlockHeight == FinaliseHeight)
		observedTxFinal := common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: vault1Addr,
				ToAddress:   common.Address(recipient),
				Coins:       coins,
				Memo:        memo,
			},
			BlockHeight:    finalHeight, // Block height now equals finalize height
			FinaliseHeight: finalHeight, // This makes it final (BlockHeight == FinaliseHeight)
			ObservedPubKey: vault1,
		}

		observer.handleObservedTxCommitted(observedTxFinal)

		// Verify transaction is removed after finalization
		assertTxFinalized(observer, key, txID.String())
	}

	// Test Case 2: tx observed non-mempool, then final
	{
		observer, tempDir := setupObserver()
		defer func() {
			observer.storage.Close()
			os.RemoveAll(tempDir)
		}()

		// Define test data
		chain := common.BTCChain
		blockHeight := int64(100)
		confirmRequired := int64(6)
		sender := "sender"
		finalHeight := blockHeight + confirmRequired
		txID := types2.GetRandomTxHash()
		memo := "test"

		// 1. Observe transaction directly in a block (non-mempool)
		txInBlock := &types.TxIn{
			Chain:                chain,
			MemPool:              false,
			ConfirmationRequired: confirmRequired,
			TxArray: []*types.TxInItem{
				{
					BlockHeight: blockHeight,
					Tx:          txID.String(),
					Memo:        memo,
					Sender:      sender,
					To:          vault1Addr.String(),
					Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				},
			},
		}
		observer.processObservedTx(*txInBlock)

		// Verify transaction is in both memory and storage
		key := TxInKey(txInBlock)
		assertTxExists(observer, key, txID.String())

		// Simulate non-final observation in Thorchain
		observedTxNonFinal := common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: common.Address(sender),
				ToAddress:   vault1Addr,
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        memo,
			},
			BlockHeight:    blockHeight, // Initial block height
			FinaliseHeight: finalHeight, // Final height is in the future
			ObservedPubKey: vault1,
		}

		observer.handleObservedTxCommitted(observedTxNonFinal)

		// Transaction should still exist but be marked as CommittedUnFinalised
		assertTxExists(observer, key, txID.String())

		// 2. Transaction finalized (BlockHeight == FinaliseHeight)
		observedTxFinal := common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: common.Address(sender),
				ToAddress:   vault1Addr,
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        memo,
			},
			BlockHeight:    finalHeight, // Block height now equals finalize height
			FinaliseHeight: finalHeight, // This makes it final (BlockHeight == FinaliseHeight)
			ObservedPubKey: vault1,
		}

		observer.handleObservedTxCommitted(observedTxFinal)

		// Verify transaction is removed after finalization
		assertTxFinalized(observer, key, txID.String())
	}

	// Test Case 3: tx observed final immediately
	{
		observer, tempDir := setupObserver()
		defer func() {
			observer.storage.Close()
			os.RemoveAll(tempDir)
		}()

		// Define test data
		chain := common.BTCChain
		blockHeight := int64(100)
		confirmRequired := int64(0) // No confirmations required
		finalHeight := blockHeight  // Final height equals block height
		txID := types2.GetRandomTxHash()
		memo := "MIGRATE:BTC.BTC:1000"

		// 1. Observe transaction that's already final
		txFinal := &types.TxIn{
			Chain:                chain,
			MemPool:              false,
			ConfirmationRequired: confirmRequired,
			TxArray: []*types.TxInItem{
				{
					BlockHeight: blockHeight,
					Tx:          txID.String(),
					Memo:        memo,
					Sender:      vault1Addr.String(),
					To:          vault2Addr.String(),
					Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				},
			},
		}
		observer.processObservedTx(*txFinal)

		// Verify transaction is in both memory and storage
		key := TxInKey(txFinal)
		assertTxExists(observer, key, txID.String())

		// Simulate final observation in Thorchain (already final since BlockHeight == FinaliseHeight)
		observedTx := common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: vault1Addr,
				ToAddress:   vault2Addr,
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        memo,
			},
			BlockHeight:    finalHeight, // Block height equals finalize height (already final)
			FinaliseHeight: finalHeight, // This makes it final (BlockHeight == FinaliseHeight)
			ObservedPubKey: vault1,
		}

		observer.handleObservedTxCommitted(observedTx)

		// Simulate final observation in Thorchain (already final since BlockHeight == FinaliseHeight)
		// observe from both vaults
		observedTx = common.ObservedTx{
			Tx: common.Tx{
				ID:          txID,
				Chain:       chain,
				FromAddress: vault1Addr,
				ToAddress:   vault2Addr,
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        memo,
			},
			BlockHeight:    finalHeight, // Block height equals finalize height (already final)
			FinaliseHeight: finalHeight, // This makes it final (BlockHeight == FinaliseHeight)
			ObservedPubKey: vault2,
		}

		observer.handleObservedTxCommitted(observedTx)

		// Verify transaction is removed immediately after finalization
		assertTxFinalized(observer, key, txID.String())
	}
}

func (s *ObserverSuite) TestPeerConcurrencyLimits(c *C) {
	// Create a logger
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.InfoLevel)

	// Test concurrent operations with different limits
	testConcurrentOperations := func(concurrencyLimit int) {
		// Create a peer manager with the specified limit
		peerMgr := newPeerManager(logger, concurrencyLimit)

		// Generate random peer ID for testing
		privKey := secp256k1.GenPrivKey()
		peerID, err := conversion.GetPeerIDFromSecp256PubKey(privKey.PubKey().Bytes())
		c.Assert(err, IsNil)

		var wg sync.WaitGroup
		activeCount := 0
		maxActive := 0
		var countMu sync.Mutex

		// Track success/failure counts
		successCount := 0
		failureCount := 0

		// Launch many more goroutines than the concurrency limit
		totalOps := concurrencyLimit * 4
		for i := 0; i < totalOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				sem, err := peerMgr.acquire(peerID)
				if err == nil {
					// Successfully acquired token
					countMu.Lock()
					activeCount++
					successCount++
					if activeCount > maxActive {
						maxActive = activeCount
					}
					countMu.Unlock()

					// Hold the token for a bit to ensure concurrency
					time.Sleep(50 * time.Millisecond)

					countMu.Lock()
					activeCount--
					countMu.Unlock()

					peerMgr.release(sem)
				} else {
					countMu.Lock()
					failureCount++
					countMu.Unlock()
				}
			}()
		}

		// Wait for all operations to complete
		wg.Wait()

		// Verify max concurrency was enforced
		c.Assert(maxActive <= concurrencyLimit, Equals, true,
			Commentf("Expected max %d concurrent operations, got %d", concurrencyLimit, maxActive))

		// Verify that operations both succeeded and failed
		c.Assert(successCount > 0, Equals, true,
			Commentf("Expected some operations to succeed"))
		c.Assert(failureCount > 0, Equals, true,
			Commentf("Expected some operations to fail due to concurrency limits"))

		// Verify that the sum matches our total operations
		c.Assert(successCount+failureCount, Equals, totalOps,
			Commentf("Total operations should match success + failure count"))
	}

	// Test with different concurrency limits for sending
	c.Log("Testing send concurrency limits")
	testConcurrentOperations(1) // Strict limit
	testConcurrentOperations(2) // Standard limit
	testConcurrentOperations(5) // More permissive limit

	// Test with concurrency limits for receiving
	c.Log("Testing receive concurrency limits")
	testConcurrentOperations(3) // Standard receive limit (typically higher than send)

	// Test integration with AttestationGossip
	c.Log("Testing integration with AttestationGossip")

	// Create p2p communication for testing
	comm, err := p2p.NewCommunication(&p2p.Config{
		RendezvousString: "test-rendezvous",
		Port:             1234,
	}, nil, nil)
	c.Assert(err, IsNil)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	err = comm.Start(priv.Bytes())
	c.Assert(err, IsNil)

	defer func() {
		stopErr := comm.Stop()
		c.Assert(stopErr, IsNil)
	}()

	// Create AttestationGossip with specific concurrency settings
	sendLimit := 2
	receiveLimit := 3

	ag, err := NewAttestationGossip(
		comm.GetHost(),
		s.thorKeys,
		"localhost:50051",
		s.bridge,
		s.metrics,
		config.BifrostAttestationGossipConfig{
			PeerConcurrentSends:    sendLimit,
			PeerConcurrentReceives: receiveLimit,
		},
	)
	c.Assert(err, IsNil)

	// Verify peer manager was correctly initialized with limits
	c.Assert(ag.peerMgr.limit, Equals, receiveLimit)
	c.Assert(ag.batcher.peerMgr.limit, Equals, sendLimit)

	// Create test peer
	fakePeerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	c.Assert(err, IsNil)

	// Test batch limits by concurrent acquire/release
	{
		var wg sync.WaitGroup
		activeCount := 0
		maxActive := 0
		var countMu sync.Mutex

		// Launch more goroutines than the concurrency limit
		totalOps := sendLimit * 3
		for i := 0; i < totalOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				sem, err := ag.batcher.peerMgr.acquire(fakePeerID)
				if err == nil {
					// Successfully acquired token
					countMu.Lock()
					activeCount++
					if activeCount > maxActive {
						maxActive = activeCount
					}
					countMu.Unlock()

					// Hold the token for a bit
					time.Sleep(50 * time.Millisecond)

					countMu.Lock()
					activeCount--
					countMu.Unlock()

					ag.batcher.peerMgr.release(sem)
				}
			}()
		}

		// Wait for all operations to complete
		wg.Wait()

		// Verify max concurrency was enforced
		c.Assert(maxActive <= sendLimit, Equals, true,
			Commentf("Batcher should respect concurrency limit of %d", sendLimit))
	}

	// Test receive limits by concurrent acquire/release
	{
		var wg sync.WaitGroup
		activeCount := 0
		maxActive := 0
		var countMu sync.Mutex

		// Launch more goroutines than the concurrency limit
		totalOps := receiveLimit * 3
		for i := 0; i < totalOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				sem, err := ag.peerMgr.acquire(fakePeerID)
				if err == nil {
					// Successfully acquired token
					countMu.Lock()
					activeCount++
					if activeCount > maxActive {
						maxActive = activeCount
					}
					countMu.Unlock()

					// Hold the token for a bit
					time.Sleep(50 * time.Millisecond)

					countMu.Lock()
					activeCount--
					countMu.Unlock()

					ag.peerMgr.release(sem)
				}
			}()
		}

		// Wait for all operations to complete
		wg.Wait()

		// Verify max concurrency was enforced
		c.Assert(maxActive <= receiveLimit, Equals, true,
			Commentf("AttestationGossip should respect concurrency limit of %d", receiveLimit))
	}

	// Test pruning behavior
	{
		peerMgr := ag.peerMgr

		// Create two different test peer IDs
		peerID1 := fakePeerID
		peerID2, err := peer.Decode("QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ")
		c.Assert(err, IsNil)

		// Acquire and release for both peers
		sem1, err := peerMgr.acquire(peerID1)
		c.Assert(err, IsNil)
		sem2, err := peerMgr.acquire(peerID2)
		c.Assert(err, IsNil)

		peerMgr.release(sem1)
		peerMgr.release(sem2)

		// Verify both peers have semaphores
		peerMgr.mu.Lock()
		initialSemCount := len(peerMgr.semaphores)
		c.Assert(initialSemCount >= 2, Equals, true)
		peerMgr.mu.Unlock()

		// Override the lastZero time to simulate that peerID1's semaphore has been unused for longer than the prune interval
		peerMgr.mu.Lock()
		peerMgr.semaphores[peerID1].lastZero = time.Now().Add(-2 * semaphorePruneInterval)
		peerMgr.mu.Unlock()

		// Run prune
		peerMgr.prune()

		// Verify peerID1's semaphore was pruned
		peerMgr.mu.Lock()
		c.Assert(len(peerMgr.semaphores) < initialSemCount, Equals, true)
		_, exists := peerMgr.semaphores[peerID1]
		c.Assert(exists, Equals, false)
		peerMgr.mu.Unlock()
	}
}
