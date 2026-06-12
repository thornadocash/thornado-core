package ebifrost

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	common "github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// ============================================================================
// Helpers
// ============================================================================

func newTestEBS() (*EnshrinedBifrost, codec.Codec) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:0",
	})
	return ebs, cdc
}

func createTestNetworkFee(chain common.Chain, height int64, attestations []*common.Attestation) *common.QuorumNetworkFee {
	return &common.QuorumNetworkFee{
		NetworkFee: &common.NetworkFee{
			Chain:           chain,
			Height:          height,
			TransactionSize: 250,
			TransactionRate: 10,
		},
		Attestations: attestations,
	}
}

func createTestSolvency(chain common.Chain, height int64, attestations []*common.Attestation) *common.QuorumSolvency {
	return &common.QuorumSolvency{
		Solvency: &common.Solvency{
			Chain:  chain,
			Height: height,
			PubKey: "thorpub1234",
			Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
		},
		Attestations: attestations,
	}
}

func createTestErrataTx(chain common.Chain, id string, attestations []*common.Attestation) *common.QuorumErrataTx {
	return &common.QuorumErrataTx{
		ErrataTx: &common.ErrataTx{
			Chain: chain,
			Id:    common.TxID(id),
		},
		Attestations: attestations,
	}
}

// ============================================================================
// NewEnshrinedBifrost
// ============================================================================

func TestNewEnshrinedBifrostDisabled(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  false,
		Address: "localhost:50051",
	})
	require.Nil(t, ebs)
}

// ============================================================================
// Start / Stop nil and double-start
// ============================================================================

func TestStartNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	require.NoError(t, ebs.Start())
}

func TestStopNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	ebs.Stop() // should not panic
}

func TestStopNotStarted(t *testing.T) {
	ebs, _ := newTestEBS()
	ebs.Stop() // should not panic when not started
}

func TestStartAlreadyStarted(t *testing.T) {
	ebs, _ := newTestEBS()
	// Mark as started without actually starting the gRPC server (to avoid goroutine panics)
	ebs.mu.Lock()
	ebs.started = true
	ebs.mu.Unlock()

	err := ebs.Start()
	require.ErrorIs(t, err, ErrAlreadyStarted)
}

func TestStartListenError(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "invalid-address-!@#$%",
	})
	err := ebs.Start()
	require.Error(t, err)
}

func TestStartPruneTimerBranch(t *testing.T) {
	// Test that startPruneTimer is called when CacheItemTTL > 0
	ebs, _ := newTestEBS()
	ebs.cfg.CacheItemTTL = 5 * time.Second

	// Call startPruneTimer directly to test the logic
	ebs.startPruneTimer()
	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)
	// Signal stop
	close(ebs.stopChan)
}

func TestStartPruneTimerSmallTTL(t *testing.T) {
	ebs, _ := newTestEBS()
	ebs.cfg.CacheItemTTL = 1 * time.Millisecond // Very small TTL forces min interval

	ebs.startPruneTimer()
	time.Sleep(50 * time.Millisecond)
	close(ebs.stopChan)
}

// ============================================================================
// SendQuorumNetworkFee
// ============================================================================

func TestSendQuorumNetworkFee(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")
	att2 := createTestAttestation("pk2", "sig2")

	// Add first
	nf1 := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att1})
	result, err := ebs.SendQuorumNetworkFee(context.Background(), nf1)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.networkFeeCache.items, 1)

	// Same network fee, new attestation
	nf2 := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att2})
	result, err = ebs.SendQuorumNetworkFee(context.Background(), nf2)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.networkFeeCache.items, 1)
	require.Len(t, ebs.networkFeeCache.items[0].Item.Attestations, 2)
}

// ============================================================================
// SendQuorumSolvency
// ============================================================================

func TestSendQuorumSolvency(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")
	att2 := createTestAttestation("pk2", "sig2")

	s1 := createTestSolvency(common.BTCChain, 100, []*common.Attestation{att1})
	result, err := ebs.SendQuorumSolvency(context.Background(), s1)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.solvencyCache.items, 1)

	// Same solvency, new attestation
	s2 := createTestSolvency(common.BTCChain, 100, []*common.Attestation{att2})
	result, err = ebs.SendQuorumSolvency(context.Background(), s2)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.solvencyCache.items, 1)
	require.Len(t, ebs.solvencyCache.items[0].Item.Attestations, 2)
}

// ============================================================================
// SendQuorumErrataTx
// ============================================================================

func TestSendQuorumErrataTx(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")
	att2 := createTestAttestation("pk2", "sig2")

	e1 := createTestErrataTx(common.BTCChain, "errata1", []*common.Attestation{att1})
	result, err := ebs.SendQuorumErrataTx(context.Background(), e1)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.errataCache.items, 1)

	// Different errata, new attestation
	e2 := createTestErrataTx(common.BTCChain, "errata2", []*common.Attestation{att2})
	result, err = ebs.SendQuorumErrataTx(context.Background(), e2)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.errataCache.items, 2)
}

func TestSendQuorumErrataTxRemovesMatchingTx(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")

	// Add a quorum tx with a specific chain/hash
	qtx := createTestQuorumTx(common.BTCChain, "hash123", true, []*common.Attestation{att1})
	_, err := ebs.SendQuorumTx(context.Background(), qtx)
	require.NoError(t, err)
	require.Len(t, ebs.quorumTxCache.items, 1)

	// Send an errata tx for the same chain/id
	errata := createTestErrataTx(common.BTCChain, "hash123", []*common.Attestation{att1})
	_, err = ebs.SendQuorumErrataTx(context.Background(), errata)
	require.NoError(t, err)

	// Quorum tx should be removed
	require.Len(t, ebs.quorumTxCache.items, 0)
	require.Len(t, ebs.errataCache.items, 1)
}

// Mark*AttestationsConfirmed (nil receiver)
// ============================================================================

func TestMarkQuorumTxAttestationsConfirmedNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	ebs.MarkQuorumTxAttestationsConfirmed(context.Background(), nil)
}

func TestMarkQuorumNetworkFeeAttestationsConfirmedNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	ebs.MarkQuorumNetworkFeeAttestationsConfirmed(context.Background(), nil)
}

func TestMarkQuorumSolvencyAttestationsConfirmedNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	ebs.MarkQuorumSolvencyAttestationsConfirmed(context.Background(), nil)
}

func TestMarkQuorumErrataTxAttestationsConfirmedNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	ebs.MarkQuorumErrataTxAttestationsConfirmed(context.Background(), nil)
}

// ============================================================================
// MarkQuorumNetworkFeeAttestationsConfirmed
// ============================================================================

func TestMarkQuorumNetworkFeeAttestationsConfirmed(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")
	att2 := createTestAttestation("pk2", "sig2")

	nf := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att1, att2})
	_, err := ebs.SendQuorumNetworkFee(context.Background(), nf)
	require.NoError(t, err)

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	// Confirm att1
	confirmNf := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att1})
	ebs.MarkQuorumNetworkFeeAttestationsConfirmed(ctx, confirmNf)

	require.Len(t, ebs.networkFeeCache.items, 1)
	require.Len(t, ebs.networkFeeCache.items[0].Item.Attestations, 1)

	// Confirm att2
	confirmNf2 := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att2})
	ebs.MarkQuorumNetworkFeeAttestationsConfirmed(ctx, confirmNf2)

	require.Len(t, ebs.networkFeeCache.items, 0)
}

func TestMarkQuorumNetworkFeeNotFound(t *testing.T) {
	ebs, _ := newTestEBS()

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	att := createTestAttestation("pk1", "sig1")
	nf := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att})
	ebs.MarkQuorumNetworkFeeAttestationsConfirmed(ctx, nf)
	// No panic, just logs
}

// ============================================================================
// MarkQuorumSolvencyAttestationsConfirmed
// ============================================================================

func TestMarkQuorumSolvencyAttestationsConfirmed(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")
	att2 := createTestAttestation("pk2", "sig2")

	s := createTestSolvency(common.BTCChain, 100, []*common.Attestation{att1, att2})
	_, err := ebs.SendQuorumSolvency(context.Background(), s)
	require.NoError(t, err)

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	// Confirm all
	confirmS := createTestSolvency(common.BTCChain, 100, []*common.Attestation{att1, att2})
	ebs.MarkQuorumSolvencyAttestationsConfirmed(ctx, confirmS)

	require.Len(t, ebs.solvencyCache.items, 0)
}

func TestMarkQuorumSolvencyNotFound(t *testing.T) {
	ebs, _ := newTestEBS()

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	att := createTestAttestation("pk1", "sig1")
	s := createTestSolvency(common.BTCChain, 100, []*common.Attestation{att})
	ebs.MarkQuorumSolvencyAttestationsConfirmed(ctx, s)
	// No panic
}

// ============================================================================
// MarkQuorumErrataTxAttestationsConfirmed
// ============================================================================

func TestMarkQuorumErrataTxAttestationsConfirmed(t *testing.T) {
	ebs, _ := newTestEBS()

	att1 := createTestAttestation("pk1", "sig1")

	e := createTestErrataTx(common.BTCChain, "err1", []*common.Attestation{att1})
	_, err := ebs.SendQuorumErrataTx(context.Background(), e)
	require.NoError(t, err)

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	confirmE := createTestErrataTx(common.BTCChain, "err1", []*common.Attestation{att1})
	ebs.MarkQuorumErrataTxAttestationsConfirmed(ctx, confirmE)

	require.Len(t, ebs.errataCache.items, 0)
}

func TestMarkQuorumErrataTxNotFound(t *testing.T) {
	ebs, _ := newTestEBS()

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	att := createTestAttestation("pk1", "sig1")
	e := createTestErrataTx(common.BTCChain, "err1", []*common.Attestation{att})
	ebs.MarkQuorumErrataTxAttestationsConfirmed(ctx, e)
	// No panic
}

// ============================================================================
// MarkQuorumTxAttestationsConfirmed - not found
// ============================================================================

func TestMarkQuorumTxNotFound(t *testing.T) {
	ebs, _ := newTestEBS()

	sdkCtx := createTestSDKContext(200)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	att := createTestAttestation("pk1", "sig1")
	tx := createTestQuorumTx(common.BTCChain, "nonexistent", true, []*common.Attestation{att})
	ebs.MarkQuorumTxAttestationsConfirmed(ctx, tx)
	// No panic, just logs
}

// ProposalInjectTxs nil receiver
// ============================================================================

func TestProposalInjectTxsNil(t *testing.T) {
	var ebs *EnshrinedBifrost
	result, totalBytes := ebs.ProposalInjectTxs(sdk.Context{}, 10000)
	require.Nil(t, result)
	require.Zero(t, totalBytes)
}

// ============================================================================
// ProposalInjectTxs with all cache types
// ============================================================================

func TestProposalInjectTxsAllCacheTypes(t *testing.T) {
	ebs, _ := newTestEBS()

	att := createTestAttestation("pk1", "sig1")

	// Add quorum tx
	qtx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{att})
	_, err := ebs.SendQuorumTx(context.Background(), qtx)
	require.NoError(t, err)

	// Add network fee
	nf := createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att})
	_, err = ebs.SendQuorumNetworkFee(context.Background(), nf)
	require.NoError(t, err)

	// Add solvency
	s := createTestSolvency(common.BTCChain, 100, []*common.Attestation{att})
	_, err = ebs.SendQuorumSolvency(context.Background(), s)
	require.NoError(t, err)

	// Add errata
	e := createTestErrataTx(common.BTCChain, "err1", []*common.Attestation{att})
	_, err = ebs.SendQuorumErrataTx(context.Background(), e)
	require.NoError(t, err)

	sdkCtx := createTestSDKContext(100)
	const maxBytes = 10000000
	result, totalBytes := ebs.ProposalInjectTxs(sdkCtx, maxBytes)

	// We should get at least 4 inject txs (quorum, nf, solvency, errata)
	// price feed may also inject
	require.GreaterOrEqual(t, len(result), 4)
	require.Greater(t, totalBytes, int64(0))
}

// ============================================================================
// removeSubscriber
// ============================================================================

func TestRemoveSubscriber(t *testing.T) {
	ebs, _ := newTestEBS()

	ch1 := make(chan *EventNotification, 1)
	ch2 := make(chan *EventNotification, 1)

	// Register subscribers
	ebs.subscribersMu.Lock()
	ebs.subscribers[EventQuorumTxCommitted] = []chan *EventNotification{ch1, ch2}
	ebs.subscribers[EventQuorumNetworkFeeCommitted] = []chan *EventNotification{ch1}
	ebs.subscribersMu.Unlock()

	// Remove ch1
	ebs.removeSubscriber(ch1)

	ebs.subscribersMu.Lock()
	// ch2 should still be subscribed to EventQuorumTxCommitted
	require.Len(t, ebs.subscribers[EventQuorumTxCommitted], 1)
	// EventQuorumNetworkFeeCommitted should be removed (no subscribers left)
	_, exists := ebs.subscribers[EventQuorumNetworkFeeCommitted]
	require.False(t, exists)
	ebs.subscribersMu.Unlock()
}

// ============================================================================
// broadcastEvent
// ============================================================================

func TestBroadcastEvent(t *testing.T) {
	ebs, _ := newTestEBS()

	ch := make(chan *EventNotification, 1)
	ebs.subscribersMu.Lock()
	ebs.subscribers[EventQuorumTxCommitted] = []chan *EventNotification{ch}
	ebs.subscribersMu.Unlock()

	ebs.broadcastEvent(EventQuorumTxCommitted, []byte("test"))

	select {
	case event := <-ch:
		require.Equal(t, EventQuorumTxCommitted, event.EventType)
		require.Equal(t, []byte("test"), event.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBroadcastEventFullChannel(t *testing.T) {
	ebs, _ := newTestEBS()

	// Channel of size 0 - will block
	ch := make(chan *EventNotification)
	ebs.subscribersMu.Lock()
	ebs.subscribers[EventQuorumTxCommitted] = []chan *EventNotification{ch}
	ebs.subscribersMu.Unlock()

	// Should not panic even with full/blocked channel
	ebs.broadcastEvent(EventQuorumTxCommitted, []byte("test"))
}

func TestBroadcastEventNoSubscribers(t *testing.T) {
	ebs, _ := newTestEBS()
	// No subscribers, should not panic
	ebs.broadcastEvent(EventQuorumTxCommitted, []byte("test"))
}

// ============================================================================
// SubscribeToEvents
// ============================================================================

func TestSubscribeToEventsUnknownEvent(t *testing.T) {
	ebs, _ := newTestEBS()

	req := &SubscribeRequest{
		EventTypes: []string{"unknown_event"},
	}
	err := ebs.SubscribeToEvents(req, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown event type")
}

// ============================================================================
// InjectCache direct tests
// ============================================================================

func TestInjectCacheAdd(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.Add("item1")
	cache.Add("item2")

	items := cache.Get()
	require.Len(t, items, 2)
	require.Equal(t, "item1", items[0])
	require.Equal(t, "item2", items[1])
}

func TestInjectCacheRemoveAt(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.Add("item0")
	cache.Add("item1")
	cache.Add("item2")

	// Remove middle
	cache.RemoveAt(1)
	items := cache.Get()
	require.Len(t, items, 2)
	require.Equal(t, "item0", items[0])
	require.Equal(t, "item2", items[1])

	// Remove out of bounds (no-op)
	cache.RemoveAt(-1)
	cache.RemoveAt(10)
	require.Len(t, cache.Get(), 2)
}

func TestInjectCacheAddToBlock(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.AddToBlock(100, "item1")
	cache.AddToBlock(100, "item2")
	cache.AddToBlock(101, "item3")

	require.Len(t, cache.recentBlockItems[100], 2)
	require.Len(t, cache.recentBlockItems[101], 1)
}

func TestInjectCacheCleanOldBlocks(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.AddToBlock(90, "old")
	cache.AddToBlock(95, "mid")
	cache.AddToBlock(100, "new")

	cache.CleanOldBlocks(100, 5)

	_, exists90 := cache.recentBlockItems[90]
	require.False(t, exists90)
	_, exists95 := cache.recentBlockItems[95]
	require.True(t, exists95)
	_, exists100 := cache.recentBlockItems[100]
	require.True(t, exists100)
}

func TestInjectCacheCheckRecentBlocks(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.AddToBlock(100, "hello")
	cache.AddToBlock(101, "world")

	found := cache.CheckRecentBlocks(func(s string) bool {
		return s == "world"
	})
	require.True(t, found)

	notFound := cache.CheckRecentBlocks(func(s string) bool {
		return s == "missing"
	})
	require.False(t, notFound)
}

func TestInjectCachePruneExpiredItems(t *testing.T) {
	cache := NewInjectCache[string]()

	// Add items with timestamps in the past
	cache.mu.Lock()
	cache.items = []TimestampedItem[string]{
		{Item: "old", Timestamp: time.Now().Add(-2 * time.Minute)},
		{Item: "new", Timestamp: time.Now()},
	}
	cache.mu.Unlock()

	pruned := cache.PruneExpiredItems(time.Minute)
	require.Len(t, pruned, 1)
	require.Equal(t, "old", pruned[0])

	remaining := cache.Get()
	require.Len(t, remaining, 1)
	require.Equal(t, "new", remaining[0])
}

func TestInjectCachePruneExpiredItemsAll(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.Add("item1")
	cache.Add("item2")

	// TTL of 0 should prune everything
	pruned := cache.PruneExpiredItems(0)
	require.Len(t, pruned, 2)
	require.Len(t, cache.Get(), 0)
}

func TestInjectCacheBroadcastEvent(t *testing.T) {
	cache := NewInjectCache[string]()

	var broadcastedType string
	var broadcastedPayload []byte
	broadcast := func(eventType string, payload []byte) {
		broadcastedType = eventType
		broadcastedPayload = payload
	}

	cache.BroadcastEvent(
		"test",
		func(s string) ([]byte, error) {
			return []byte(s), nil
		},
		broadcast,
		"test_event",
		log.NewNopLogger(),
	)

	require.Equal(t, "test_event", broadcastedType)
	require.Equal(t, []byte("test"), broadcastedPayload)
}

func TestInjectCacheBroadcastEventMarshalError(t *testing.T) {
	cache := NewInjectCache[string]()

	var called bool
	broadcast := func(eventType string, payload []byte) {
		called = true
	}

	cache.BroadcastEvent(
		"test",
		func(s string) ([]byte, error) {
			return nil, errors.New("marshal error")
		},
		broadcast,
		"test_event",
		log.NewNopLogger(),
	)

	require.False(t, called)
}

func TestInjectCacheProcessForProposal(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.Add("item1")
	cache.Add("item2")

	var logged []string
	result := cache.ProcessForProposal(
		func(s string) (sdk.Msg, error) {
			return nil, nil // msg not used for marshaling
		},
		func(msg sdk.Msg) ([]byte, error) {
			return []byte("txbytes"), nil
		},
		func(s string, logger log.Logger) {
			logged = append(logged, s)
		},
		log.NewNopLogger(),
	)

	require.Len(t, result, 2)
	require.Len(t, logged, 2)
}

func TestInjectCacheProcessForProposalMsgError(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.Add("item1")

	result := cache.ProcessForProposal(
		func(s string) (sdk.Msg, error) {
			return nil, errors.New("create msg error")
		},
		func(msg sdk.Msg) ([]byte, error) {
			return []byte("txbytes"), nil
		},
		func(s string, logger log.Logger) {},
		log.NewNopLogger(),
	)

	require.Len(t, result, 0)
}

func TestInjectCacheProcessForProposalTxError(t *testing.T) {
	cache := NewInjectCache[string]()
	cache.Add("item1")

	result := cache.ProcessForProposal(
		func(s string) (sdk.Msg, error) {
			return nil, nil
		},
		func(msg sdk.Msg) ([]byte, error) {
			return nil, errors.New("marshal tx error")
		},
		func(s string, logger log.Logger) {},
		log.NewNopLogger(),
	)

	require.Len(t, result, 0)
}

// ============================================================================
// Config tests
// ============================================================================

func TestDefaultEBifrostConfig(t *testing.T) {
	cfg := DefaultEBifrostConfig()
	require.True(t, cfg.Enable)
	require.Equal(t, "localhost:50051", cfg.Address)
	require.Equal(t, 30*time.Minute, cfg.CacheItemTTL)
}

func TestConfigTemplate(t *testing.T) {
	cfg := DefaultEBifrostConfig()
	tmpl := ConfigTemplate(cfg)
	require.Contains(t, tmpl, "enabled = true")
	require.Contains(t, tmpl, "localhost:50051")
}

func TestDefaultConfigTemplate(t *testing.T) {
	tmpl := DefaultConfigTemplate()
	require.Contains(t, tmpl, "[ebifrost]")
}

func TestAddModuleInitFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	AddModuleInitFlags(cmd)

	f := cmd.Flags().Lookup(flagEnabled)
	require.NotNil(t, f)

	f = cmd.Flags().Lookup(flagAddress)
	require.NotNil(t, f)
}

type mockAppOptions struct {
	data map[string]interface{}
}

func (m mockAppOptions) Get(key string) interface{} {
	return m.data[key]
}

func TestReadEBifrostConfig(t *testing.T) {
	// Default values
	opts := mockAppOptions{data: map[string]interface{}{}}
	cfg, err := ReadEBifrostConfig(opts)
	require.NoError(t, err)
	require.True(t, cfg.Enable)
	require.Equal(t, "localhost:50051", cfg.Address)

	// Custom values
	opts = mockAppOptions{data: map[string]interface{}{
		flagEnabled: false,
		flagAddress: "localhost:9090",
	}}
	cfg, err = ReadEBifrostConfig(opts)
	require.NoError(t, err)
	require.False(t, cfg.Enable)
	require.Equal(t, "localhost:9090", cfg.Address)
}

func TestReadEBifrostConfigEnableError(t *testing.T) {
	opts := mockAppOptions{data: map[string]interface{}{
		flagEnabled: "not-a-bool-value",
	}}
	_, err := ReadEBifrostConfig(opts)
	require.Error(t, err)
}

func TestReadEBifrostConfigAddressError(t *testing.T) {
	opts := mockAppOptions{data: map[string]interface{}{
		flagAddress: 12345, // not a string
	}}
	_, err := ReadEBifrostConfig(opts)
	require.Error(t, err)
}

// ============================================================================
// ante.go tests
// ============================================================================

// mockAnteSDKTx implements sdk.Tx for non-inject tx testing
type mockAnteSDKTx struct {
	msgs []sdk.Msg
}

func (m mockAnteSDKTx) GetMsgs() []sdk.Msg                    { return m.msgs }
func (m mockAnteSDKTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

func TestAnteHandleNonInjectTxWithAllowedMsg(t *testing.T) {
	decorator := NewInjectedTxDecorator()
	ctx := sdk.Context{}

	called := false
	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		called = true
		return ctx, nil
	}

	tx := mockAnteSDKTx{msgs: []sdk.Msg{}}
	_, err := decorator.AnteHandle(ctx, tx, false, next)
	require.NoError(t, err)
	require.True(t, called)
}

func TestAnteHandleNonInjectTxWithQuorumMsg(t *testing.T) {
	decorator := NewInjectedTxDecorator()
	ctx := sdk.Context{}

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		return ctx, nil
	}

	// Test with each quorum msg type
	quorumMsgs := []sdk.Msg{
		&types.MsgObservedTxQuorum{},
		&types.MsgNetworkFeeQuorum{},
		&types.MsgSolvencyQuorum{},
		&types.MsgErrataTxQuorum{},
	}

	for _, msg := range quorumMsgs {
		tx := mockAnteSDKTx{msgs: []sdk.Msg{msg}}
		_, err := decorator.AnteHandle(ctx, tx, false, next)
		require.Error(t, err)
	}
}

func TestAnteHandleInjectTxCheckTx(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	decorator := NewInjectedTxDecorator()

	// Create a real inject tx with an allowed msg
	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		return ctx, nil
	}

	// CheckTx should be rejected
	ctx := sdk.Context{}.WithIsCheckTx(true)
	_, err := decorator.AnteHandle(ctx, itx, false, next)
	require.Error(t, err)
}

func TestAnteHandleInjectTxSimulate(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	decorator := NewInjectedTxDecorator()

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		return ctx, nil
	}

	ctx := sdk.Context{}
	_, err := decorator.AnteHandle(ctx, itx, true, next)
	require.Error(t, err)
}

func TestAnteHandleInjectTxDeliverTxAllowedMsg(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	decorator := NewInjectedTxDecorator()

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		t.Fatal("next should not be called for inject tx")
		return ctx, nil
	}

	// DeliverTx (not check, not recheck, not simulate)
	ctx := sdk.Context{}
	_, err := decorator.AnteHandle(ctx, itx, false, next)
	require.NoError(t, err)
}

func TestAnteHandleInjectTxMultipleMsgs(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	decorator := NewInjectedTxDecorator()

	msg1 := &types.MsgNetworkFeeQuorum{}
	msg2 := &types.MsgSolvencyQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg1, msg2})

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		return ctx, nil
	}

	ctx := sdk.Context{}
	_, err := decorator.AnteHandle(ctx, itx, false, next)
	require.Error(t, err) // Only 1 msg allowed
}

// ============================================================================
// tx.go tests
// ============================================================================

func TestNewInjectTxAndGetMsgs(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	msgs := itx.GetMsgs()
	require.Len(t, msgs, 1)
}

func TestWInjectTxAsAny(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	any := itx.AsAny()
	require.NotNil(t, any)
}

func TestTxEncoderInjectTx(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	encoder := TxEncoder(nil)
	bz, err := encoder(itx)
	require.NoError(t, err)
	require.NotEmpty(t, bz)
}

func TestTxEncoderNonInjectTxWithUpstream(t *testing.T) {
	upstream := func(tx sdk.Tx) ([]byte, error) {
		return []byte("upstream"), nil
	}

	encoder := TxEncoder(upstream)
	bz, err := encoder(mockAnteSDKTx{})
	require.NoError(t, err)
	require.Equal(t, []byte("upstream"), bz)
}

func TestTxEncoderNonInjectTxNoUpstream(t *testing.T) {
	encoder := TxEncoder(nil)
	_, err := encoder(mockAnteSDKTx{})
	require.Error(t, err)
}

func TestTxDecoderInjectTx(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})
	bz, err := itx.Tx.Marshal()
	require.NoError(t, err)

	decoder := TxDecoder(cdc, nil)
	tx, err := decoder(bz)
	require.NoError(t, err)
	require.NotNil(t, tx)

	msgs := tx.GetMsgs()
	require.Len(t, msgs, 1)
}

func TestTxDecoderUpstreamSucceeds(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	upstream := func(txBytes []byte) (sdk.Tx, error) {
		return mockAnteSDKTx{}, nil
	}

	decoder := TxDecoder(cdc, upstream)
	tx, err := decoder([]byte("anything"))
	require.NoError(t, err)
	require.NotNil(t, tx)
}

func TestTxDecoderUpstreamFailsInjectSucceeds(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})
	bz, err := itx.Tx.Marshal()
	require.NoError(t, err)

	upstream := func(txBytes []byte) (sdk.Tx, error) {
		return nil, errors.New("upstream failed")
	}

	decoder := TxDecoder(cdc, upstream)
	tx, err := decoder(bz)
	require.NoError(t, err)
	require.NotNil(t, tx)
}

func TestTxDecoderBothFail(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	upstream := func(txBytes []byte) (sdk.Tx, error) {
		return nil, errors.New("upstream failed")
	}

	decoder := TxDecoder(cdc, upstream)
	_, err := decoder([]byte("garbage"))
	require.Error(t, err)
}

func TestJSONTxEncoderInjectTx(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	encoder := JSONTxEncoder(cdc, nil)
	bz, err := encoder(itx)
	require.NoError(t, err)
	require.NotEmpty(t, bz)
}

func TestJSONTxEncoderNonInjectTxWithUpstream(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	upstream := func(tx sdk.Tx) ([]byte, error) {
		return []byte("upstream_json"), nil
	}

	encoder := JSONTxEncoder(cdc, upstream)
	bz, err := encoder(mockAnteSDKTx{})
	require.NoError(t, err)
	require.Equal(t, []byte("upstream_json"), bz)
}

func TestJSONTxEncoderNonInjectTxNoUpstream(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	encoder := JSONTxEncoder(cdc, nil)
	_, err := encoder(mockAnteSDKTx{})
	require.Error(t, err)
}

func TestJSONTxDecoderUpstreamSucceeds(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	upstream := func(txBytes []byte) (sdk.Tx, error) {
		return mockAnteSDKTx{}, nil
	}

	decoder := JSONTxDecoder(cdc, upstream)
	tx, err := decoder([]byte("anything"))
	require.NoError(t, err)
	require.NotNil(t, tx)
}

func TestJSONTxDecoderUpstreamFailsInjectSucceeds(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})
	bz, err := cdc.MarshalJSON(itx.Tx)
	require.NoError(t, err)

	upstream := func(txBytes []byte) (sdk.Tx, error) {
		return nil, errors.New("upstream failed")
	}

	decoder := JSONTxDecoder(cdc, upstream)
	tx, err := decoder(bz)
	require.NoError(t, err)
	require.NotNil(t, tx)
}

func TestJSONTxDecoderBothFail(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	upstream := func(txBytes []byte) (sdk.Tx, error) {
		return nil, errors.New("upstream failed")
	}

	decoder := JSONTxDecoder(cdc, upstream)
	_, err := decoder([]byte("garbage"))
	require.Error(t, err)
}

func TestJSONTxDecoderNoUpstream(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})
	bz, err := cdc.MarshalJSON(itx.Tx)
	require.NoError(t, err)

	decoder := JSONTxDecoder(cdc, nil)
	tx, err := decoder(bz)
	require.NoError(t, err)
	require.NotNil(t, tx)
}

// ============================================================================
// post.go tests
// ============================================================================

func TestPostDecoratorNonInjectTx(t *testing.T) {
	ebs, _ := newTestEBS()
	decorator := NewEnshrineBifrostPostDecorator(ebs)

	ctx := sdk.Context{}.WithBlockHeight(100)
	called := false
	next := func(ctx sdk.Context, tx sdk.Tx, simulate, success bool) (sdk.Context, error) {
		called = true
		return ctx, nil
	}

	tx := mockAnteSDKTx{}
	_, err := decorator.PostHandle(ctx, tx, false, true, next)
	require.NoError(t, err)
	require.True(t, called)
}

func TestPostDecoratorSimulate(t *testing.T) {
	ebs, _ := newTestEBS()
	decorator := NewEnshrineBifrostPostDecorator(ebs)

	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	ctx := sdk.Context{}.WithBlockHeight(100)
	called := false
	next := func(ctx sdk.Context, tx sdk.Tx, simulate, success bool) (sdk.Context, error) {
		called = true
		return ctx, nil
	}

	_, err := decorator.PostHandle(ctx, itx, true, true, next)
	require.NoError(t, err)
	require.True(t, called) // simulate skips processing, passes to next
}

func TestPostDecoratorFailure(t *testing.T) {
	ebs, _ := newTestEBS()
	decorator := NewEnshrineBifrostPostDecorator(ebs)

	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	msg := &types.MsgNetworkFeeQuorum{}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	ctx := sdk.Context{}.WithBlockHeight(100)
	called := false
	next := func(ctx sdk.Context, tx sdk.Tx, simulate, success bool) (sdk.Context, error) {
		called = true
		return ctx, nil
	}

	_, err := decorator.PostHandle(ctx, itx, false, false, next)
	require.NoError(t, err)
	require.True(t, called) // !success skips processing, passes to next
}

func TestPostDecoratorInjectTxProcessesMsgTypes(t *testing.T) {
	ebs, _ := newTestEBS()
	decorator := NewEnshrineBifrostPostDecorator(ebs)

	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	att := createTestAttestation("pk1", "sig1")

	// Test MsgObservedTxQuorum
	qtx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{att})
	_, err := ebs.SendQuorumTx(context.Background(), qtx)
	require.NoError(t, err)

	msg := &types.MsgObservedTxQuorum{QuoTx: qtx}
	itx := NewInjectTx(cdc, []sdk.Msg{msg})

	ctx := sdk.Context{}.WithBlockHeight(100)
	next := func(ctx sdk.Context, tx sdk.Tx, simulate, success bool) (sdk.Context, error) {
		return ctx, nil
	}

	_, err = decorator.PostHandle(ctx, itx, false, true, next)
	require.NoError(t, err)
}

// ============================================================================
// startPruneTimer - prune actually removes items
// ============================================================================

func TestStartPruneTimerPrunesItems(t *testing.T) {
	ebs, _ := newTestEBS()
	ttl := 100 * time.Millisecond
	ebs.cfg.CacheItemTTL = ttl

	// Add items to various caches with old timestamps
	att := createTestAttestation("pk1", "sig1")

	ebs.quorumTxCache.mu.Lock()
	ebs.quorumTxCache.items = []TimestampedItem[*common.QuorumTx]{
		{Item: createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{att}), Timestamp: time.Now().Add(-2 * ttl)},
	}
	ebs.quorumTxCache.mu.Unlock()

	ebs.networkFeeCache.mu.Lock()
	ebs.networkFeeCache.items = []TimestampedItem[*common.QuorumNetworkFee]{
		{Item: createTestNetworkFee(common.BTCChain, 100, []*common.Attestation{att}), Timestamp: time.Now().Add(-2 * ttl)},
	}
	ebs.networkFeeCache.mu.Unlock()

	ebs.solvencyCache.mu.Lock()
	ebs.solvencyCache.items = []TimestampedItem[*common.QuorumSolvency]{
		{Item: createTestSolvency(common.BTCChain, 100, []*common.Attestation{att}), Timestamp: time.Now().Add(-2 * ttl)},
	}
	ebs.solvencyCache.mu.Unlock()

	ebs.errataCache.mu.Lock()
	ebs.errataCache.items = []TimestampedItem[*common.QuorumErrataTx]{
		{Item: createTestErrataTx(common.BTCChain, "err1", []*common.Attestation{att}), Timestamp: time.Now().Add(-2 * ttl)},
	}
	ebs.errataCache.mu.Unlock()

	// Call startPruneTimer directly to avoid gRPC server issues
	ebs.startPruneTimer()

	// Use require.Eventually with lock-safe Get() to avoid racing with the
	// prune goroutine and to tolerate CI scheduling delays.
	require.Eventually(t, func() bool {
		return len(ebs.quorumTxCache.Get()) == 0 &&
			len(ebs.networkFeeCache.Get()) == 0 &&
			len(ebs.solvencyCache.Get()) == 0 &&
			len(ebs.errataCache.Get()) == 0
	}, 5*time.Second, 50*time.Millisecond, "caches should be pruned")

	// Signal stop after assertions so the goroutine isn't torn down mid-prune.
	close(ebs.stopChan)
}
