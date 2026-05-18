package observer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
)

// TestLogWriter is a custom writer that writes to testing.T.Log
type TestLogWriter struct {
	t *testing.T
}

// Write implements the io.Writer interface
func (w TestLogWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

func getTestLogger(t *testing.T) zerolog.Logger {
	// Create a writer that writes to testing.T.Log
	writer := TestLogWriter{t: t}

	// Set up a pretty console writer for readable test output
	consoleWriter := zerolog.ConsoleWriter{
		Out:     writer,
		NoColor: false, // Set to true if you don't want colors
	}

	// Create a logger with the test writer
	return zerolog.New(consoleWriter).
		Level(zerolog.DebugLevel). // Use debug level for tests
		With().
		Timestamp().
		Caller(). // Include caller information
		Logger()
}

// setupTestGossip sets up a test instance of AttestationGossip with mocked dependencies
func setupTestGossip(t *testing.T) (*AttestationGossip, *MockHost, *MockKeys, *MockGRPCClient, *MockThorchainBridge, *MockEventClient) {
	privKey := secp256k1.GenPrivKey()
	keys := &MockKeys{
		privKey: privKey,
	}

	pubkey2 := thorchain.GetRandomPubKey()
	pubkey3 := thorchain.GetRandomPubKey()

	// derive peer IDs
	peer1, err := conversion.GetPeerIDFromSecp256PubKey(privKey.PubKey().Bytes())
	require.NoError(t, err)
	pubkey2Bz, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeAccPub, pubkey2.String())
	require.NoError(t, err)
	peer2, err := conversion.GetPeerIDFromSecp256PubKey(pubkey2Bz.Bytes())
	require.NoError(t, err)
	pubkey3Bz, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeAccPub, pubkey3.String())
	require.NoError(t, err)
	peer3, err := conversion.GetPeerIDFromSecp256PubKey(pubkey3Bz.Bytes())
	require.NoError(t, err)

	peers := []peer.ID{peer1, peer2, peer3}
	host := NewMockHost(peers)

	pubKey, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey.PubKey())
	require.NoError(t, err)

	grpcClient := &MockGRPCClient{
		sendQuorumTxFunc: func(ctx context.Context, tx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
			return &ebifrost.SendQuorumTxResult{}, nil
		},
		sendQuorumNetworkFeeFunc: func(ctx context.Context, quorumNetworkFee *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
			return &ebifrost.SendQuorumNetworkFeeResult{}, nil
		},
		sendQuorumSolvencyFunc: func(ctx context.Context, quorumSolvency *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error) {
			return &ebifrost.SendQuorumSolvencyResult{}, nil
		},
		sendQuorumErrataFunc: func(ctx context.Context, quorumErrata *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error) {
			return &ebifrost.SendQuorumErrataTxResult{}, nil
		},
	}

	bridge := &MockThorchainBridge{
		getKeysignPartyFunc: func(pubKey common.PubKey) (common.PubKeys, error) {
			return common.PubKeys{pubKey, pubkey2, pubkey3}, nil
		},
	}

	eventClient := NewMockEventClient()

	// Create config
	cfg := config.BifrostAttestationGossipConfig{
		ObserveReconcileInterval:   1 * time.Second,
		LateObserveTimeout:         2 * time.Minute,
		NonQuorumTimeout:           10 * time.Hour,
		MinTimeBetweenAttestations: 10 * time.Second,
		AskPeers:                   2,
		AskPeersDelay:              1 * time.Second,
		MaxBatchSize:               100,
		BatchInterval:              100 * time.Millisecond,
	}

	batcher := NewAttestationBatcher(NewMockHost(peers), getTestLogger(t), nil, 1*time.Second, 100, 10*time.Second, 4)

	// Create the attestation gossip directly without calling NewAttestationGossip
	ag := &AttestationGossip{
		logger:               getTestLogger(t),
		host:                 host,
		privKey:              privKey,
		pubKey:               privKey.PubKey().Bytes(),
		grpcClient:           grpcClient,
		config:               cfg,
		bridge:               bridge,
		eventClient:          eventClient,
		cachedKeySignParties: make(map[common.PubKey]cachedKeySignParty),
		batcher:              batcher,

		observedTxs: make(map[txKey]*AttestationState[*common.ObservedTx]),
		networkFees: make(map[common.NetworkFee]*AttestationState[*common.NetworkFee]),
		solvencies:  make(map[common.TxID]*AttestationState[*common.Solvency]),
		errataTxs:   make(map[common.ErrataTx]*AttestationState[*common.ErrataTx]),

		observedTxsPool: NewAttestationStatePool[*common.ObservedTx](),
		networkFeesPool: NewAttestationStatePool[*common.NetworkFee](),
		solvenciesPool:  NewAttestationStatePool[*common.Solvency](),
		errataTxsPool:   NewAttestationStatePool[*common.ErrataTx](),

		peerMgr: newPeerManager(getTestLogger(t), 2),
	}

	// Set active validators
	ag.setActiveValidators([]common.PubKey{common.PubKey(pubKey), pubkey2, pubkey3})

	return ag, host, keys, grpcClient, bridge, eventClient
}

// TestNormalizeConfig tests the config normalization function
func TestNormalizeConfig(t *testing.T) {
	t.Run("uses defaults for empty config", func(t *testing.T) {
		config := config.BifrostAttestationGossipConfig{}
		normalizeConfig(&config)

		assert.Equal(t, defaultObserveReconcileInterval, config.ObserveReconcileInterval)
		assert.Equal(t, defaultLateObserveTimeout, config.LateObserveTimeout)
		assert.Equal(t, defaultNonQuorumTimeout, config.NonQuorumTimeout)
		assert.Equal(t, defaultMinTimeBetweenAttestations, config.MinTimeBetweenAttestations)
		assert.Equal(t, defaultAskPeers, config.AskPeers)
		assert.Equal(t, defaultAskPeersDelay, config.AskPeersDelay)
	})

	t.Run("respects provided values", func(t *testing.T) {
		config := config.BifrostAttestationGossipConfig{
			ObserveReconcileInterval:   30 * time.Second,
			LateObserveTimeout:         10 * time.Minute,
			NonQuorumTimeout:           1 * time.Hour,
			MinTimeBetweenAttestations: 60 * time.Second,
			AskPeers:                   5,
			AskPeersDelay:              30 * time.Second,
		}
		normalizeConfig(&config)

		assert.Equal(t, 30*time.Second, config.ObserveReconcileInterval)
		assert.Equal(t, 10*time.Minute, config.LateObserveTimeout)
		assert.Equal(t, 1*time.Hour, config.NonQuorumTimeout)
		assert.Equal(t, 60*time.Second, config.MinTimeBetweenAttestations)
		assert.Equal(t, 5, config.AskPeers)
		assert.Equal(t, 30*time.Second, config.AskPeersDelay)
	})
}

// TestActiveValidatorCount tests the active validator count functions
func TestActiveValidatorCount(t *testing.T) {
	ag, _, _, _, _, _ := setupTestGossip(t)

	// Test initial value
	count := ag.activeValidatorCount()
	assert.Equal(t, 3, count)

	privKey1 := secp256k1.GenPrivKey()
	privKey2 := secp256k1.GenPrivKey()
	privKey3 := secp256k1.GenPrivKey()

	pub1, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey1.PubKey())
	require.NoError(t, err)
	pub2, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey2.PubKey())
	require.NoError(t, err)

	// Test setting a new value
	ag.setActiveValidators([]common.PubKey{common.PubKey(pub1), common.PubKey(pub2)})
	count = ag.activeValidatorCount()
	assert.Equal(t, 2, count)

	peer1, err := conversion.GetPeerIDFromSecp256PubKey(privKey1.PubKey().Bytes())
	require.NoError(t, err)
	peer2, err := conversion.GetPeerIDFromSecp256PubKey(privKey2.PubKey().Bytes())
	require.NoError(t, err)
	peer3, err := conversion.GetPeerIDFromSecp256PubKey(privKey3.PubKey().Bytes())
	require.NoError(t, err)

	// Test isActiveValidator function
	assert.True(t, ag.isActiveValidator(peer1))
	assert.True(t, ag.isActiveValidator(peer2))
	assert.False(t, ag.isActiveValidator(peer3))
}

// TestGetKeysignParty tests retrieving and caching the keysign party
func TestGetKeysignParty(t *testing.T) {
	ag, _, _, _, bridge, _ := setupTestGossip(t)

	t.Run("returns cached value if available", func(t *testing.T) {
		pubKey := common.PubKey("pubkey1")

		// First call should go to the bridge
		callCount := 0
		bridge.getKeysignPartyFunc = func(key common.PubKey) (common.PubKeys, error) {
			callCount++
			return common.PubKeys{pubKey, "pubkey2", "pubkey3"}, nil
		}

		result1, err := ag.getKeysignParty(pubKey)
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)
		assert.Len(t, result1, 3)

		// Second call should use cache
		result2, err := ag.getKeysignParty(pubKey)
		require.NoError(t, err)
		assert.Equal(t, 1, callCount) // Still 1, not incremented
		assert.Len(t, result2, 3)
	})

	t.Run("cache expires after TTL", func(t *testing.T) {
		pubKey := common.PubKey("expiring-key")

		callCount := 0
		bridge.getKeysignPartyFunc = func(key common.PubKey) (common.PubKeys, error) {
			callCount++
			return common.PubKeys{pubKey, "pubkey2", "pubkey3"}, nil
		}

		// First call - should hit the bridge
		_, err := ag.getKeysignParty(pubKey)
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)

		// Manually expire the cache entry by setting the time in the past
		ag.cachedKeySignMu.Lock()
		cached := ag.cachedKeySignParties[pubKey]
		cached.lastUpdated = time.Now().Add(-2 * cachedKeysignPartyTTL)
		ag.cachedKeySignParties[pubKey] = cached
		ag.cachedKeySignMu.Unlock()

		// Now simulate the pruning that would happen in the Start method
		ag.cachedKeySignMu.Lock()
		for pk, cached := range ag.cachedKeySignParties {
			if time.Since(cached.lastUpdated) > cachedKeysignPartyTTL {
				delete(ag.cachedKeySignParties, pk)
			}
		}
		ag.cachedKeySignMu.Unlock()

		// Next call should hit the bridge again
		_, err = ag.getKeysignParty(pubKey)
		require.NoError(t, err)
		assert.Equal(t, 2, callCount)
	})
}

// TestAttestObservedTx tests attesting to an observed transaction
func TestAttestObservedTx(t *testing.T) {
	// Disable deadline application during tests
	originalApplyDeadline := p2p.ApplyDeadline.Load()
	p2p.ApplyDeadline.Store(false)
	defer func() { p2p.ApplyDeadline.Store(originalApplyDeadline) }()

	ag, host, _, _, _, _ := setupTestGossip(t)

	// Override verifySignature for testing
	origVerifySignature := verifySignature
	defer func() { verifySignature = origVerifySignature }()
	verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
		return nil // Always succeed in tests
	}

	t.Run("successfully attests and sends to peers", func(t *testing.T) {
		// Create a test tx
		tx := &common.ObservedTx{
			Tx: common.Tx{
				ID:    "tx1",
				Chain: common.BSCChain,
			},
		}

		var streamsMu sync.Mutex
		streams := make(map[peer.ID]*streamPair)

		// Mock stream creation
		host.streamFunc = func(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
			if p == host.ID() {
				return nil, errors.New("cannot create stream to self")
			}

			clientToServer := &bytes.Buffer{}
			serverToClient := &bytes.Buffer{}
			pair := &streamPair{
				clientToServer: clientToServer,
				serverToClient: serverToClient,
				peer:           p,
			}

			// Register the stream for later inspection
			streamsMu.Lock()
			streams[p] = pair
			streamsMu.Unlock()

			// Create a stream that writes to clientToServer and reads from serverToClient
			stream := &MockStream{
				reader: serverToClient,
				writer: clientToServer,
				peer:   p,
				mu:     new(sync.Mutex),
			}

			// Set up a goroutine to automatically respond with "done"
			go func() {
				time.Sleep(50 * time.Millisecond)
				// Write "done" using proper protocol format
				lengthBytes := make([]byte, p2p.LengthHeader)
				binary.LittleEndian.PutUint32(lengthBytes, 4) // "done" is 4 bytes
				stream.mu.Lock()
				defer stream.mu.Unlock()
				serverToClient.Write(lengthBytes)
				serverToClient.WriteString("done")
			}()

			return stream, nil
		}

		// Test the function
		err := ag.AttestObservedTx(context.Background(), tx, true)
		require.NoError(t, err)

		// Wait a bit for all operations to complete
		time.Sleep(100 * time.Millisecond)

		// Verify the tx was added to observedTxs
		ag.mu.Lock()
		defer ag.mu.Unlock()

		key := txKey{
			Chain:                  tx.Tx.Chain,
			ID:                     tx.Tx.ID,
			UniqueHash:             tx.Tx.Hash(0),
			AllowFutureObservation: false,
			Finalized:              tx.IsFinal(),
			Inbound:                true,
		}

		state, exists := ag.observedTxs[key]
		assert.True(t, exists, "Observed tx should be added to the map")

		if exists {
			assert.Equal(t, 1, state.AttestationCount(), "Should have one attestation")
		}
	})
}

// TestHandleObservedTxAttestation tests handling an observed transaction attestation
func TestHandleObservedTxAttestation(t *testing.T) {
	ag, _, _, grpcClient, bridge, _ := setupTestGossip(t)

	// Override verifySignature for testing
	origVerifySignature := verifySignature
	defer func() { verifySignature = origVerifySignature }()
	verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
		return nil // Always succeed in tests
	}

	t.Run("processes attestation and sends to thornode when quorum reached", func(t *testing.T) {
		// Create private keys and valid signatures for testing
		privKey1 := secp256k1.GenPrivKey()
		privKey2 := secp256k1.GenPrivKey()
		privKey3 := secp256k1.GenPrivKey()

		// Create a test tx
		tx := common.ObservedTx{
			Tx: common.Tx{
				ID:    "tx-quorum",
				Chain: common.BSCChain,
			},
			ObservedPubKey: "pubkey1", // Important for keysign party lookup
		}

		pub1, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey1.PubKey())
		require.NoError(t, err)
		pub2, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey2.PubKey())
		require.NoError(t, err)
		pub3, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey3.PubKey())
		require.NoError(t, err)

		// Set up keysign party mock
		bridge.getKeysignPartyFunc = func(pubKey common.PubKey) (common.PubKeys, error) {
			return common.PubKeys{"pubkey1", "pubkey2", "pubkey3", "pubkey4"}, nil
		}

		// Set up GRPC client mock to capture the sent tx
		var sentTxs []*common.QuorumTx
		var grpcMu sync.Mutex

		grpcClient.sendQuorumTxFunc = func(ctx context.Context, quorumTx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
			grpcMu.Lock()
			sentTxs = append(sentTxs, quorumTx)
			grpcMu.Unlock()
			return &ebifrost.SendQuorumTxResult{}, nil
		}

		// Make sure active validator count is set
		ag.setActiveValidators([]common.PubKey{common.PubKey(pub1), common.PubKey(pub2), common.PubKey(pub3)})

		// Create attestations to reach supermajority (3 of 4)
		ag.handleObservedTxAttestation(context.Background(), common.AttestTx{
			ObsTx:   tx,
			Inbound: true,
			Attestation: &common.Attestation{
				PubKey:    privKey1.PubKey().Bytes(),
				Signature: []byte("sig1"),
			},
		})

		ag.handleObservedTxAttestation(context.Background(), common.AttestTx{
			ObsTx:   tx,
			Inbound: true,
			Attestation: &common.Attestation{
				PubKey:    privKey2.PubKey().Bytes(),
				Signature: []byte("sig2"),
			},
		})

		ag.handleObservedTxAttestation(context.Background(), common.AttestTx{
			ObsTx:   tx,
			Inbound: true,
			Attestation: &common.Attestation{
				PubKey:    privKey3.PubKey().Bytes(),
				Signature: []byte("sig3"),
			},
		})

		// Wait a bit for processing to complete
		time.Sleep(200 * time.Millisecond)

		// Verify it was sent to thornode
		grpcMu.Lock()
		defer grpcMu.Unlock()

		require.Len(t, sentTxs, 2, "Should have sent txs to thornode")
		sentAttestations := 0
		for _, sentTx := range sentTxs {
			assert.Equal(t, tx, sentTx.ObsTx, "Sent tx should match original")
			assert.True(t, sentTx.Inbound, "Tx should be marked as inbound")
			sentAttestations += len(sentTx.Attestations)
		}
		assert.Equal(t, sentAttestations, 3, "Should include all attestations")

		// Verify attestations are marked as sent
		ag.mu.Lock()
		defer ag.mu.Unlock()

		key := txKey{
			Chain:                  tx.Tx.Chain,
			ID:                     tx.Tx.ID,
			UniqueHash:             tx.Tx.Hash(0),
			AllowFutureObservation: false,
			Finalized:              tx.IsFinal(),
			Inbound:                true,
		}

		state, exists := ag.observedTxs[key]
		assert.True(t, exists, "Tx should exist in observed txs map")

		if exists {
			assert.Equal(t, 3, state.AttestationCount(), "Should have 3 attestations")
		}
	})
}

// TestHandleNetworkFeeAttestation tests handling a network fee attestation
func TestHandleNetworkFeeAttestation(t *testing.T) {
	ag, _, _, grpcClient, _, _ := setupTestGossip(t)

	// Override verifySignature for testing
	origVerifySignature := verifySignature
	defer func() { verifySignature = origVerifySignature }()
	verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
		return nil // Always succeed in tests
	}

	t.Run("processes attestation and sends to thornode when quorum reached", func(t *testing.T) {
		// Create private keys for testing
		privKey1 := secp256k1.GenPrivKey()
		privKey2 := secp256k1.GenPrivKey()
		privKey3 := secp256k1.GenPrivKey()

		// Create a test network fee
		networkFee := common.NetworkFee{
			Chain:  common.BSCChain,
			Height: 200,
		}

		pub1, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey1.PubKey())
		require.NoError(t, err)
		pub2, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey2.PubKey())
		require.NoError(t, err)
		pub3, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey3.PubKey())
		require.NoError(t, err)

		// Set up GRPC client mock with mutex for thread safety
		var sentFees []*common.QuorumNetworkFee
		var grpcMu sync.Mutex

		grpcClient.sendQuorumNetworkFeeFunc = func(ctx context.Context, quorumNetworkFee *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
			grpcMu.Lock()
			sentFees = append(sentFees, quorumNetworkFee)
			grpcMu.Unlock()
			return &ebifrost.SendQuorumNetworkFeeResult{}, nil
		}

		// Make sure active validator count is set
		ag.setActiveValidators([]common.PubKey{common.PubKey(pub1), common.PubKey(pub2), common.PubKey(pub3)})

		// Create attestations to reach supermajority (3 of 4)
		ag.handleNetworkFeeAttestation(context.Background(), common.AttestNetworkFee{
			NetworkFee: &networkFee,
			Attestation: &common.Attestation{
				PubKey:    privKey1.PubKey().Bytes(),
				Signature: []byte("sig1"),
			},
		})

		ag.handleNetworkFeeAttestation(context.Background(), common.AttestNetworkFee{
			NetworkFee: &networkFee,
			Attestation: &common.Attestation{
				PubKey:    privKey2.PubKey().Bytes(),
				Signature: []byte("sig2"),
			},
		})

		ag.handleNetworkFeeAttestation(context.Background(), common.AttestNetworkFee{
			NetworkFee: &networkFee,
			Attestation: &common.Attestation{
				PubKey:    privKey3.PubKey().Bytes(),
				Signature: []byte("sig3"),
			},
		})

		// Wait for processing to complete
		time.Sleep(200 * time.Millisecond)

		// Verify it was sent to thornode
		grpcMu.Lock()
		defer grpcMu.Unlock()

		require.Len(t, sentFees, 2, "Should have sent network fees to thornode")
		sentAttestations := 0
		for _, sentFee := range sentFees {
			assert.Equal(t, networkFee.Chain, sentFee.NetworkFee.Chain)
			assert.Equal(t, networkFee.Height, sentFee.NetworkFee.Height)
			sentAttestations += len(sentFee.Attestations)
		}
		assert.Equal(t, sentAttestations, 3, "Should include all attestations")

		// Verify attestations are in the state
		ag.mu.Lock()
		defer ag.mu.Unlock()

		state, exists := ag.networkFees[networkFee]
		assert.True(t, exists, "Network fee should exist in networkFees map")

		if exists {
			assert.Equal(t, 3, state.AttestationCount(), "Should have 3 attestations")
		}
	})
}

// TestStart tests starting the attestation gossip service
func TestStart(t *testing.T) {
	ag, _, _, _, _, _ := setupTestGossip(t)

	t.Run("starts and can be cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Start in a goroutine
		done := make(chan struct{})
		go func() {
			ag.Start(ctx)
			close(done)
		}()

		// Let it run for a bit
		time.Sleep(100 * time.Millisecond)

		// Cancel and make sure it stops
		cancel()

		select {
		case <-done:
			// Good, it stopped
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not exit after context cancellation")
		}
	})
}

// TestPruningLogic tests pruning of old attestation states
func TestPruningLogic(t *testing.T) {
	ag, _, _, _, _, _ := setupTestGossip(t)

	// Add an observed tx that should be pruned
	pruneKey := txKey{
		Chain: common.BSCChain,
		ID:    "prune-me",
	}

	pruneTx := &common.Tx{
		ID:    "prune-me",
		Chain: common.BSCChain,
	}

	obsTx := &common.ObservedTx{
		Tx: *pruneTx,
	}

	pruneState := ag.observedTxsPool.NewAttestationState(obsTx)

	// Set quorum time in the distant past
	pruneState.quorumAttestationsSent = time.Now().Add(-24 * time.Hour)

	ag.mu.Lock()
	ag.observedTxs[pruneKey] = pruneState
	ag.mu.Unlock()

	// Add a network fee that should be pruned
	pruneFee := common.NetworkFee{
		Chain:  common.BSCChain,
		Height: 400,
	}

	pruneFeeState := ag.networkFeesPool.NewAttestationState(&pruneFee)

	// Set quorum time in the distant past
	pruneFeeState.quorumAttestationsSent = time.Now().Add(-24 * time.Hour)

	ag.mu.Lock()
	ag.networkFees[pruneFee] = pruneFeeState
	ag.mu.Unlock()

	// Run a simulated ticker iteration
	ag.mu.Lock()
	for hash, state := range ag.observedTxs {
		state.mu.Lock()
		if state.ExpiredAfterQuorum(ag.config.LateObserveTimeout, ag.config.NonQuorumTimeout) {
			delete(ag.observedTxs, hash)
		}
		state.mu.Unlock()
	}

	for hash, state := range ag.networkFees {
		state.mu.Lock()
		if state.ExpiredAfterQuorum(ag.config.LateObserveTimeout, ag.config.NonQuorumTimeout) {
			delete(ag.networkFees, hash)
		}
		state.mu.Unlock()
	}
	ag.mu.Unlock()

	// Verify they were pruned
	ag.mu.Lock()
	_, txExists := ag.observedTxs[pruneKey]
	_, feeExists := ag.networkFees[pruneFee]
	ag.mu.Unlock()

	assert.False(t, txExists, "Expired observed tx should be pruned")
	assert.False(t, feeExists, "Expired network fee should be pruned")
}

// TestSendLateAttestations tests sending late attestations
func TestSendLateAttestations(t *testing.T) {
	ag, _, _, grpcClient, _, _ := setupTestGossip(t)

	// Prepare a test tx with late attestations
	tx := &common.Tx{
		ID:    "late-tx",
		Chain: common.BSCChain,
	}

	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	lateKey := txKey{
		Chain: tx.Chain,
		ID:    tx.ID,
	}

	lateState := ag.observedTxsPool.NewAttestationState(obsTx)

	// Add two attestations
	lateState.attestations = append(lateState.attestations, attestationSentState{
		attestation: &common.Attestation{
			PubKey:    []byte("pubkey-late-1"),
			Signature: []byte("sig-late-1"),
		},
		sent: true, // First one was already sent
	})

	// Add a second attestation that hasn't been sent yet
	lateState.attestations = append(lateState.attestations, attestationSentState{
		attestation: &common.Attestation{
			PubKey:    []byte("pubkey-late-2"),
			Signature: []byte("sig-late-2"),
		},
		sent: false, // This one hasn't been sent
	})

	// Set initial attestation time in the past beyond minTimeBetweenAttestations
	lateState.initialAttestationsSent = time.Now().Add(-2 * ag.config.MinTimeBetweenAttestations)
	lateState.lastAttestationsSent = time.Now().Add(-2 * ag.config.MinTimeBetweenAttestations)

	ag.mu.Lock()
	ag.observedTxs[lateKey] = lateState
	ag.mu.Unlock()

	// Set up GRPC client mock
	var sentTx *common.QuorumTx
	grpcClient.sendQuorumTxFunc = func(ctx context.Context, quorumTx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
		sentTx = quorumTx
		return &ebifrost.SendQuorumTxResult{}, nil
	}

	// Run the check for late attestations
	ag.mu.Lock()
	for k, state := range ag.observedTxs {
		state.mu.Lock()
		if state.ShouldSendLate(ag.config.MinTimeBetweenAttestations) {
			// Send late attestations
			ag.sendObservedTxAttestationsToThornode(context.Background(), *obsTx, state, k.Inbound, false, false)
		}
		state.mu.Unlock()
	}
	ag.mu.Unlock()

	// Verify the unsent attestation was sent to thornode
	assert.NotNil(t, sentTx)
	assert.Equal(t, obsTx.Tx.ID, sentTx.ObsTx.Tx.ID)
	assert.Len(t, sentTx.Attestations, 1) // Only the unsent one
	assert.Equal(t, []byte("pubkey-late-2"), sentTx.Attestations[0].PubKey)

	// Verify the late attestation is now marked as sent
	ag.mu.Lock()
	state := ag.observedTxs[lateKey]
	ag.mu.Unlock()

	state.mu.Lock()
	found := false
	for _, att := range state.attestations {
		if bytes.Equal(att.attestation.PubKey, []byte("pubkey-late-2")) {
			found = true
			assert.True(t, att.sent)
		}
	}
	state.mu.Unlock()

	assert.True(t, found, "Late attestation should be found and marked as sent")
}

// TestHandleQuorumTxCommited tests handling committed quorum transactions
func TestHandleQuorumTxCommited(t *testing.T) {
	ag, _, _, _, _, _ := setupTestGossip(t)

	t.Run("calls observer handler if our attestation is in tx", func(t *testing.T) {
		// Set up observer handler
		handlerCalled := false
		var handledTx common.ObservedTx
		ag.observerHandleObservedTxCommitted = func(tx common.ObservedTx) {
			handlerCalled = true
			handledTx = tx
		}

		// Create a test tx with our attestation
		tx := common.ObservedTx{
			Tx: common.Tx{
				ID:    "tx-committed",
				Chain: common.BSCChain,
			},
		}

		quorumTx := common.QuorumTx{
			ObsTx: tx,
			Attestations: []*common.Attestation{
				{
					PubKey:    ag.pubKey,
					Signature: []byte("sig"),
				},
			},
		}

		payload, err := quorumTx.Marshal()
		require.NoError(t, err)

		// Call the handler
		ag.handleQuorumTxCommitted(&ebifrost.EventNotification{
			Payload: payload,
		})

		// Verify handler was called
		assert.True(t, handlerCalled)
		assert.Equal(t, tx, handledTx)
	})

	t.Run("doesn't call handler if our attestation is not in tx", func(t *testing.T) {
		// Set up observer handler
		handlerCalled := false
		ag.observerHandleObservedTxCommitted = func(tx common.ObservedTx) {
			handlerCalled = true
		}

		// Create a test tx without our attestation
		tx := common.ObservedTx{
			Tx: common.Tx{
				ID:    "tx-committed-2",
				Chain: common.BSCChain,
			},
		}

		quorumTx := common.QuorumTx{
			ObsTx: tx,
			Attestations: []*common.Attestation{
				{
					PubKey:    []byte("other-pubkey"),
					Signature: []byte("sig"),
				},
			},
		}

		payload, err := quorumTx.Marshal()
		require.NoError(t, err)

		// Call the handler
		ag.handleQuorumTxCommitted(&ebifrost.EventNotification{
			Payload: payload,
		})

		// Verify handler was not called
		assert.False(t, handlerCalled)
	})
}

// TestConcurrentAttestationHandling tests handling attestations concurrently
func TestConcurrentAttestationHandling(t *testing.T) {
	ag, _, _, _, _, _ := setupTestGossip(t)

	// Override verifySignature for testing
	origVerifySignature := verifySignature
	defer func() { verifySignature = origVerifySignature }()
	verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
		return nil // Always succeed in tests
	}

	tx := &common.ObservedTx{Tx: common.Tx{Chain: "BTC", ID: "tx1"}}
	numAttestations := 10

	valPrivs := make([]*secp256k1.PrivKey, numAttestations)
	valKeys := make([]common.PubKey, numAttestations)
	for i := 0; i < numAttestations; i++ {
		valPrivs[i] = secp256k1.GenPrivKey()
		pub, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, valPrivs[i].PubKey())
		require.NoError(t, err)
		valKeys[i] = common.PubKey(pub)
	}

	signBz, err := tx.Tx.Marshal()
	require.NoError(t, err)

	ag.setActiveValidators(valKeys)

	var wg sync.WaitGroup
	wg.Add(numAttestations)

	// Create a channel that all goroutines will notify when they're ready
	startupChan := make(chan struct{})
	readyChan := make(chan struct{})

	for i := 0; i < numAttestations; i++ {
		obsTx := *tx
		go func(i int) {
			// Notify that this goroutine is ready
			startupChan <- struct{}{}

			// Wait for the signal to start
			<-readyChan

			defer wg.Done()

			sig, _ := valPrivs[i].Sign(signBz)

			// Create a unique attestation
			attestTx := common.AttestTx{
				ObsTx: obsTx,
				Attestation: &common.Attestation{
					PubKey:    valPrivs[i].PubKey().Bytes(),
					Signature: sig,
				},
			}
			ag.handleObservedTxAttestation(context.Background(), attestTx)
		}(i)
	}

	// Wait for all goroutines to be ready
	for i := 0; i < numAttestations; i++ {
		<-startupChan
	}

	// Signal all goroutines to proceed, forces contention (for max concurrency)
	close(readyChan)

	wg.Wait()

	// Verify that all attestations were processed correctly
	ag.mu.Lock()
	defer ag.mu.Unlock()
	key := txKey{
		Chain:      tx.Tx.Chain,
		ID:         tx.Tx.ID,
		UniqueHash: tx.Tx.Hash(0),
		Finalized:  true,
	}
	state, exists := ag.observedTxs[key]
	assert.True(t, exists)
	assert.Equal(t, numAttestations, state.AttestationCount())
}
