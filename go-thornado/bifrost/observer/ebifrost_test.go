package observer

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"time"

	"cosmossdk.io/log"
	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/libp2p/go-libp2p-core/peer"
	zlog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients"
	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/thorclient"
	"github.com/thornadocash/go-thornado/bifrost/thorclient/types"
	"github.com/thornadocash/go-thornado/cmd"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/x/thorchain/ebifrost"
	stypes "github.com/thornadocash/go-thornado/x/thorchain/types"
)

// Mock ThorchainBridge that always returns active status and correct node count
type mockThorchainBridge struct {
	thorclient.ThorchainBridge
	activeNodes []common.PubKey
}

func (m *mockThorchainBridge) FetchNodeStatus() (stypes.NodeStatus, error) {
	return stypes.NodeStatus_Active, nil
}

func (m *mockThorchainBridge) FetchActiveNodes() ([]common.PubKey, error) {
	return m.activeNodes, nil
}

func (m *mockThorchainBridge) GetMimir(key string) (int64, error) {
	return 0, nil
}

// Create a local gRPC server
func createLocalGrpcServer(c *C, eb *ebifrost.EnshrinedBifrost, port int) (*grpc.Server, string) {
	addr := fmt.Sprintf("localhost:%d", port)
	lis, err := net.Listen("tcp", addr)
	c.Assert(err, IsNil)

	s := grpc.NewServer()
	ebifrost.RegisterLocalhostBifrostServer(s, eb)

	go func() {
		if err := s.Serve(lis); err != nil {
			c.Fatalf("Server exited with error: %v", err)
		}
	}()

	return s, addr
}

func (s *ObserverSuite) TestAttestedTxWorkflow(c *C) {
	// Create enshrined bifrost servers for each validator
	ebs := make([]*ebifrost.EnshrinedBifrost, 4)
	grpcServers := make([]*grpc.Server, 4)
	grpcAddresses := make([]string, 4)

	// Initialize enshrined bifrost server for each validator
	for i := 0; i < 4; i++ {
		ebs[i] = ebifrost.NewEnshrinedBifrost(s.bridge.GetContext().Codec, log.NewNopLogger(), ebifrost.EBifrostConfig{
			Enable:  true,
			Address: fmt.Sprintf("localhost:%d", 50061+i),
		})
		grpcServers[i], grpcAddresses[i] = createLocalGrpcServer(c, ebs[i], 50061+i)
	}

	// Ensure we clean up all servers at the end
	defer func() {
		for _, server := range grpcServers {
			server.Stop()
		}
	}()

	logger := zlog.With().Logger()

	// Create 4 validator keys
	validatorKeys := make([]*thorclient.Keys, 4)
	validatorPubs := make([]common.PubKey, 4)
	for i := 0; i < 4; i++ {
		cfg := config.BifrostClientConfiguration{
			// ChainID: "thorchain",
			// ChainHost:    server.Listener.Addr().String(),
			// ChainRPC:     server.Listener.Addr().String(),
			SignerName:   "validator" + fmt.Sprint(i),
			SignerPasswd: "password",
		}

		registry := codectypes.NewInterfaceRegistry()
		cryptocodec.RegisterInterfaces(registry)
		cdc := codec.NewProtoCodec(registry)
		kb := cKeys.NewInMemory(cdc)
		_, _, err := kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.ThornadoHDPath, cfg.SignerPasswd, hd.Secp256k1)
		c.Assert(err, IsNil)
		keys := thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)

		validatorKeys[i] = keys
		priv, err := keys.GetPrivateKey()
		c.Assert(err, IsNil)
		validatorPubs[i], err = common.NewPubKeyFromCrypto(secp256k1.PubKey(priv.PubKey().Bytes()))
		c.Assert(err, IsNil)
	}

	// Create a mock thorchain bridge
	mockBridge := &mockThorchainBridge{
		activeNodes: validatorPubs, // 4 validators
	}

	ctx := context.Background()

	// Setup P2P network with 4 validators
	hosts := make([]*p2p.Communication, len(validatorKeys))
	attestationGossips := make([]*AttestationGossip, len(validatorKeys))
	observers := make([]*Observer, len(validatorKeys))

	basePort := 12340
	for i, keys := range validatorKeys {
		// Create P2P communication
		comm, err := p2p.NewCommunication(&p2p.Config{RendezvousString: "validator-test", Port: basePort + i}, nil, nil)
		c.Assert(err, IsNil)
		c.Assert(comm, NotNil)

		// Start communication
		privKey, err := keys.GetPrivateKey()
		c.Assert(err, IsNil)
		err = comm.Start(privKey.Bytes())
		c.Assert(err, IsNil)

		defer func() {
			stopErr := comm.Stop()
			c.Assert(stopErr, IsNil)
		}()

		// Create attestation gossip manually with the bifrost client
		ag, err := NewAttestationGossip(comm.GetHost(), keys, grpcAddresses[i], mockBridge, s.metrics, config.BifrostAttestationGossipConfig{
			ObserveReconcileInterval:   time.Second * 1,
			MinTimeBetweenAttestations: time.Second * 2,
			LateObserveTimeout:         time.Second * 4,
			AskPeers:                   3,
			AskPeersDelay:              time.Hour * 1, // don't ask peers during this test
			PeerTimeout:                time.Second * 20,
			MaxBatchSize:               100,
			BatchInterval:              time.Millisecond * 200,
			PeerConcurrentSends:        4,
			PeerConcurrentReceives:     5,
		})
		c.Assert(err, IsNil)

		// Create pubkey manager
		pubkeyMgr, err := pubkeymanager.NewPubKeyManager(mockBridge, s.metrics)
		c.Assert(err, IsNil)

		// Create observer
		chainClient := &mockChainClient{}
		obs, err := NewObserver(
			pubkeyMgr,
			map[common.Chain]chainclients.ChainClient{common.BTCChain: chainClient},
			mockBridge,
			s.metrics,
			"",
			metrics.NewTssKeysignMetricMgr(),
			ag,
			"",
		)
		c.Assert(err, IsNil)

		ag.SetObserverHandleObservedTxCommitted(obs)

		// Store for later use
		hosts[i] = comm
		attestationGossips[i] = ag
		observers[i] = obs

		logger.Info().Msgf("Validator %d: %s", i, comm.GetHost().ID())
	}

	// Wait for all comms to be started
	time.Sleep(time.Millisecond * 100)

	// Connect all peers to each other
	for i, host := range hosts {
		for j, peerHost := range hosts {
			if i == j {
				continue
			}

			ph := peerHost.GetHost()

			// Connect to peer using the host's Connect method
			peerInfo := peer.AddrInfo{
				ID:    ph.ID(),
				Addrs: ph.Addrs(),
			}

			logger.Info().Msgf("Dialing %s from %s", ph.ID(), host.GetHost().ID())

			var err error
			// Use more retries with exponential backoff to handle CI environment variability
			backoffDelay := time.Millisecond * 100
			for attempt := range 10 {
				err = host.GetHost().Connect(ctx, peerInfo)
				if err == nil {
					break
				}
				// Exponential backoff: 100ms, 200ms, 400ms, 800ms, 1s, 1s, 1s, 1s, 1s, 1s
				time.Sleep(backoffDelay)
				if attempt < 4 {
					backoffDelay *= 2
				} else {
					backoffDelay = time.Second
				}
			}
			c.Assert(err, IsNil)
		}
	}

	// Wait for all connections to be established
	time.Sleep(time.Millisecond * 100)

	logger.Info().Msg("All validators connected")

	// Set active validators for all attestation gossips using actual host peer IDs
	hostPeerIDs := make([]peer.ID, len(hosts))
	for i, host := range hosts {
		hostPeerIDs[i] = host.GetHost().ID()
	}

	// Set active validators for each attestation gossip instance
	for _, ag := range attestationGossips {
		ag.avMu.Lock()
		activePeers := make(map[peer.ID]bool)
		for _, peerID := range hostPeerIDs {
			activePeers[peerID] = true
		}
		ag.activeVals = activePeers
		ag.avMu.Unlock()
	}

	// Start all observers
	for _, obs := range observers {
		go func() {
			err := obs.Start(ctx)
			c.Assert(err, IsNil)
		}()
		defer func() {
			err := obs.Stop()
			c.Assert(err, IsNil)
		}()
	}

	// Wait for all observers to start
	time.Sleep(time.Millisecond * 100)

	logger.Info().Msg("All observers started")

	// Create a test transaction
	testTx := common.NewObservedTx(
		common.NewTx(
			common.TxID("0x1234567890"),
			common.Address("sender"),
			common.Address("receiver"),
			common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One))},
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1*common.One))},
			"SWAP:THOR.RUNE",
		),
		1,
		common.PubKey("pubkey"),
		10,
	)

	// First validator observes and attests the transaction
	err := attestationGossips[0].AttestObservedTx(ctx, &testTx, true)
	c.Assert(err, IsNil)

	logger.Info().Msg("First validator attested")

	// Wait for gossip to propagate
	time.Sleep(time.Millisecond * 100)

	// Check that enshrined bifrost has the tx cached
	for _, eb := range ebs {
		txs, _ := eb.ProposalInjectTxs(sdk.Context{}, 10000000)
		c.Assert(len(txs), Equals, 0) // No txs yet, need quorum
	}

	// Wait for the first validator to have the transaction ready
	pollForCondition(c, "all validators have the gossipped attestation of the first validator", func() bool {
		for _, atg := range attestationGossips {
			atg.mu.Lock()
			if len(atg.observedTxs) != 1 {
				atg.mu.Unlock()
				return false
			}
			ots, ok := atg.observedTxs[txKey{Chain: common.BTCChain, ID: testTx.Tx.ID, UniqueHash: testTx.Tx.Hash(1), Finalized: false, Inbound: true}]
			if !ok {
				atg.mu.Unlock()
				return false
			}
			ots.mu.Lock() // Should have the tx in the observed txs map
			if len(ots.attestations) != 1 {
				ots.mu.Unlock()
				atg.mu.Unlock()
				return false
			}
			ots.mu.Unlock()
			atg.mu.Unlock()
		}

		return true
	}, 10*time.Second, 100*time.Millisecond)

	// Have the rest of the validators attest
	for i := 1; i < len(validatorKeys); i++ {
		err := attestationGossips[i].AttestObservedTx(ctx, &testTx, true)
		c.Assert(err, IsNil)
	}

	logger.Info().Msg("Remaining validators attested")

	// Wait for all enshrined bifrosts to have the transaction ready
	pollForCondition(c, "all validators to have transaction ready", func() bool {
		for i, eb := range ebs {
			injectTxs, _ := eb.ProposalInjectTxs(sdk.Context{}, 10000000)
			if len(injectTxs) != 1 {
				logger.Info().Msgf("Validator %d: Has %d transactions ready (waiting for 1)", i, len(injectTxs))
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond)

	// for _, atg := range attestationGossips {
	// 	atg.mu.Lock()
	// 	logger.Info().Msgf("Observed txs: %+v", atg.observedTxs)
	// 	for _, ots := range atg.observedTxs {
	// 		ots.mu.Lock()
	// 		logger.Info().Msgf("Observed tx: %+v", ots)
	// 		for _, att := range ots.attestations {
	// 			logger.Info().Msgf("Attestation: %+v", att)
	// 		}
	// 		ots.mu.Unlock()
	// 	}
	// 	atg.mu.Unlock()
	// }

	// Now check that each validator's enshrined bifrost has the tx ready for injection
	// Each validator should have the transaction ready since they all participated in attestation
	var injectedTxQuorum *stypes.MsgObservedTxQuorum
	for i, eb := range ebs {
		injectTxs, _ := eb.ProposalInjectTxs(sdk.Context{}, 10000000)
		c.Assert(len(injectTxs), Equals, 1, Commentf("Validator %d: Expected 1 tx to be ready for injection", i)) // fails here
		if i == 0 {
			msgs, err := eb.GetInjectedMsgs(sdk.Context{}, injectTxs)
			c.Assert(err, IsNil)
			c.Assert(msgs, HasLen, 1)
			var ok bool
			injectedTxQuorum, ok = msgs[0].(*stypes.MsgObservedTxQuorum)
			c.Assert(ok, Equals, true)
			c.Assert(injectedTxQuorum.QuoTx.Attestations, HasLen, 4)
		}
	}

	for _, atg := range attestationGossips {
		atg.mu.Lock()
		c.Assert(len(atg.observedTxs), Equals, 1)
		ots, ok := atg.observedTxs[txKey{Chain: common.BTCChain, ID: testTx.Tx.ID, UniqueHash: testTx.Tx.Hash(1), Finalized: false, Inbound: true}]
		c.Assert(ok, Equals, true)                    // Should have the tx in the observed txs map
		ots.mu.Lock()                                 // Should have the tx in the observed txs map
		c.Assert(ots.attestations, HasLen, 4)         // Should have 4 attestations
		c.Assert(ots.UnsentAttestations(), HasLen, 0) // no attestations remain to be flushed
		ots.mu.Unlock()
		atg.mu.Unlock()
	}

	// Test marking confirmed on one validator's enshrined bifrost
	sdkCtx := sdk.Context{}.WithContext(ctx).WithBlockHeight(1)
	ebs[0].MarkQuorumTxAttestationsConfirmed(sdkCtx, injectedTxQuorum.QuoTx)

	// Check that tx was removed from that validator's injection queue
	injectTxsAfterMark, _ := ebs[0].ProposalInjectTxs(sdk.Context{}, 10000000)
	c.Assert(len(injectTxsAfterMark), Equals, 0, Commentf("Expected 0 txs after marking as confirmed"))

	// Other validators should still have the tx in their queue
	for i := 1; i < len(ebs); i++ {
		injectTxs, _ := ebs[i].ProposalInjectTxs(sdk.Context{}, 10000000)
		c.Assert(len(injectTxs), Equals, 1, Commentf("Validator %d: Should still have tx in queue", i))

		// Now clear from the other validators too
		ebs[i].MarkQuorumTxAttestationsConfirmed(sdkCtx, injectedTxQuorum.QuoTx)

		// Verify it's cleared
		injectTxsAfterMark, _ = ebs[i].ProposalInjectTxs(sdk.Context{}, 10000000)
		c.Assert(len(injectTxsAfterMark), Equals, 0, Commentf("Validator %d: Expected 0 txs after marking as confirmed", i))
	}

	time.Sleep(time.Second * 5)

	// attestation gossips should have removed the tx due to the timeout
	for _, atg := range attestationGossips {
		atg.mu.Lock()
		c.Assert(len(atg.observedTxs), Equals, 0)
		atg.mu.Unlock()
	}
}

// Helper function to poll for a condition with timeout
func pollForCondition(c *C, description string, condition func() bool, maxWait, pollInterval time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(pollInterval)
	}
	c.Fatalf("Timed out waiting for %s after %v", description, maxWait)
}

// Mock chain client for testing
type mockChainClient struct{}

func (m *mockChainClient) Start(chan types.TxIn, chan types.ErrataBlock, chan types.Solvency, chan common.NetworkFee) {
}

func (m *mockChainClient) Stop() {}

func (m *mockChainClient) GetChain() common.Chain {
	return common.BTCChain
}

func (m *mockChainClient) GetHeight() (int64, error) {
	return 1, nil
}

func (m *mockChainClient) GetConfirmationCount(txIn types.TxIn) int64 {
	return 0
}

func (m *mockChainClient) ConfirmationCountReady(txIn types.TxIn) bool {
	return true
}

func (m *mockChainClient) IsBlockScannerHealthy() bool {
	return true
}

func (m *mockChainClient) BroadcastTx(types.TxOutItem, []byte) (string, error) {
	return "", nil
}

func (m *mockChainClient) GetAccount(common.PubKey, *big.Int) (common.Account, error) {
	return common.Account{}, nil
}

func (m *mockChainClient) GetAccountByAddress(string, *big.Int) (common.Account, error) {
	return common.Account{}, nil
}

func (m *mockChainClient) GetAddress(common.PubKey) string {
	return ""
}

func (m *mockChainClient) GetBlockScannerHeight() (int64, error) {
	return 0, nil
}

func (m *mockChainClient) RollbackBlockScanner() error {
	return nil
}

func (m *mockChainClient) GetConfig() config.BifrostChainConfiguration {
	return config.BifrostChainConfiguration{}
}

func (m *mockChainClient) GetLatestTxForVault(string) (string, string, error) {
	return "", "", nil
}

func (m *mockChainClient) OnObservedTxIn(types.TxInItem, int64) {}

func (m *mockChainClient) SignTx(types.TxOutItem, int64) ([]byte, []byte, *types.TxInItem, error) {
	return nil, nil, nil, nil
}
