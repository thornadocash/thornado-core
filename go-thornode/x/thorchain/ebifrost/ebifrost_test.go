package ebifrost

import (
	"context"
	"strconv"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	common "gitlab.com/thorchain/thornode/v3/common"
)

// Helper functions for test data

// createTestSDKContext creates a new SDK context with the given block height
func createTestSDKContext(height int64) sdk.Context {
	ctx := sdk.Context{}.WithBlockHeight(height)
	return ctx
}

// createTestAttestation creates a test Attestation instance
func createTestAttestation(pubKey, signature string) *common.Attestation {
	return &common.Attestation{
		PubKey:    []byte(pubKey),
		Signature: []byte(signature),
	}
}

// createTestTx creates a test Tx instance
func createTestTx(chain common.Chain, id string) common.ObservedTx {
	return common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID(id),
			Chain:       chain,
			Memo:        "test",
			Coins:       common.Coins{},
			Gas:         common.Gas{},
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
		},
	}
}

// createTestQuorumTx creates a test QuorumTx instance
func createTestQuorumTx(chain common.Chain, id string, inbound bool, attestations []*common.Attestation) *common.QuorumTx {
	return &common.QuorumTx{
		ObsTx:        createTestTx(chain, id),
		Attestations: attestations,
		Inbound:      inbound,
	}
}

// Mock transaction for testing
type MockTx struct {
	msgs []sdk.Msg
}

func (tx *MockTx) GetMsgs() []sdk.Msg {
	return tx.msgs
}

func (tx *MockTx) Marshal() ([]byte, error) {
	return []byte("mock_tx"), nil
}

// Mock SDK message
type MockMsg struct{}

func (msg MockMsg) Route() string                { return "mock" }
func (msg MockMsg) Type() string                 { return "mock" }
func (msg MockMsg) ValidateBasic() error         { return nil }
func (msg MockMsg) GetSignBytes() []byte         { return []byte{} }
func (msg MockMsg) GetSigners() []sdk.AccAddress { return []sdk.AccAddress{} }

// TestNewEnshrinedBifrost tests the creation of a new EnshrinedBifrost instance
func TestNewEnshrinedBifrost(t *testing.T) {
	// Create a simple registry and codec
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Verify the instance was created correctly
	require.NotNil(t, ebs)
	require.NotNil(t, ebs.s)
	require.NotNil(t, ebs.logger)
	require.NotNil(t, ebs.quorumTxCache.recentBlockItems)
	require.NotNil(t, ebs.networkFeeCache.recentBlockItems)
	require.NotNil(t, ebs.solvencyCache.recentBlockItems)
	require.NotNil(t, ebs.errataCache.recentBlockItems)
	require.Empty(t, ebs.quorumTxCache.items)
	require.Empty(t, ebs.networkFeeCache.items)
	require.Empty(t, ebs.solvencyCache.items)
	require.Empty(t, ebs.errataCache.items)
}

// TestSendQuorumTx tests the SendQuorumTx method
func TestSendQuorumTx(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create test data
	attestation1 := createTestAttestation("pubkey1", "sig1")
	attestation2 := createTestAttestation("pubkey2", "sig2")

	// Test case 1: New quorum tx
	tx1 := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation1})

	result, err := ebs.SendQuorumTx(context.Background(), tx1)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.quorumTxCache.items, 1)

	// Test case 2: Same tx with new attestation
	tx2 := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation2})

	result, err = ebs.SendQuorumTx(context.Background(), tx2)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.quorumTxCache.items, 1)                      // Still one tx
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 2) // But now with two attestations

	// Test case 3: Different tx
	tx3 := createTestQuorumTx(common.BTCChain, "tx2", false, []*common.Attestation{attestation1})

	result, err = ebs.SendQuorumTx(context.Background(), tx3)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, ebs.quorumTxCache.items, 2) // Now two txs
}

// TestProposalInjectTxs tests the ProposalInjectTxs method
func TestProposalInjectTxs(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Add test txs
	attestation := createTestAttestation("pubkey1", "sig1")
	tx1 := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation})
	tx2 := createTestQuorumTx(common.BTCChain, "tx2", false, []*common.Attestation{attestation})

	ebs.quorumTxCache.items = []TimestampedItem[*common.QuorumTx]{
		{
			Item:      tx1,
			Timestamp: time.Now(),
		},
		{
			Item:      tx2,
			Timestamp: time.Now(),
		},
	}

	// Call the method
	sdkCtx := createTestSDKContext(100)

	const maxTxBytes = 10000000
	result, totalBytes := ebs.ProposalInjectTxs(sdkCtx, maxTxBytes)

	// Verify results
	require.Len(t, result, 2)
	// Verify the cumulative byte length is calculated and returned
	require.Greater(t, totalBytes, int64(0))

	// Test with a very small maxTxBytes to verify size filtering
	const smallMaxTxBytes = 1 // Too small to include any transactions
	limitedResult, limitedTotalBytes := ebs.ProposalInjectTxs(sdkCtx, smallMaxTxBytes)
	require.Len(t, limitedResult, 0, "No transactions should be included when maxTxBytes is too small")
	require.Zero(t, limitedTotalBytes)
}

// TestEnshrinedBifrostStartStop tests the Start and Stop methods
func TestEnshrinedBifrostStartStop(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Test start (using a high port to avoid conflicts)
	err := ebs.Start()
	require.NoError(t, err)

	// Wait a bit to allow the server to start
	time.Sleep(100 * time.Millisecond)

	// Test stop
	ebs.Stop()
}

// TestSendQuorumTxWithMultipleOverlappingAttestations tests sending a tx with multiple attestations
// where some are new and some already exist in both .txs and recentBlockTxs
func TestSendQuorumTxWithMultipleOverlappingAttestations(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create a varied set of attestations
	att1 := createTestAttestation("pubkey1", "sig1") // Will be in recentBlockTxs
	att2 := createTestAttestation("pubkey2", "sig2") // Will be in recentBlockTxs
	att3 := createTestAttestation("pubkey3", "sig3") // Will be in pending .txs
	att4 := createTestAttestation("pubkey4", "sig4") // Will be in pending .txs
	att5 := createTestAttestation("pubkey5", "sig5") // New attestation
	att6 := createTestAttestation("pubkey6", "sig6") // New attestation

	// Setup the initial state:

	// 1. Add attestations 1 and 2 to recentBlockTxs (as if already processed)
	committedTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{att1, att2})
	ebs.quorumTxCache.recentBlockItems[100] = []*common.QuorumTx{committedTx}

	// 2. Add attestations 3 and 4 to pending txs
	pendingTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{att3, att4})
	_, err := ebs.SendQuorumTx(context.Background(), pendingTx)
	require.NoError(t, err)

	// Verify the initial state
	require.Len(t, ebs.quorumTxCache.items, 1)
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 2)
	require.Len(t, ebs.quorumTxCache.recentBlockItems, 1)

	// Now send a tx with a mix of attestations: some already in recentBlockTxs,
	// some already in pending txs, and some new ones
	mixedTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{
		att1, att3, att5, att6, // 1 from recentBlockTxs, 1 from pending txs, 2 new
	})

	result, err := ebs.SendQuorumTx(context.Background(), mixedTx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check the resulting state:
	// 1. Only one tx should still be in .txs
	require.Len(t, ebs.quorumTxCache.items, 1)

	// 2. The tx should have attestations 3, 4, 5, and 6
	//    - 3 and 4 were already there
	//    - 5 and 6 are new
	//    - 1 was in recentBlockTxs so should be filtered out
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 4)

	// Verify all expected attestations are present
	found3, found4, found5, found6 := false, false, false, false
	for _, att := range ebs.quorumTxCache.items[0].Item.Attestations {
		switch string(att.PubKey) {
		case string(att3.PubKey):
			found3 = true
		case string(att4.PubKey):
			found4 = true
		case string(att5.PubKey):
			found5 = true
		case string(att6.PubKey):
			found6 = true
		}
	}

	require.True(t, found3, "attestation3 should be present")
	require.True(t, found4, "attestation4 should be present")
	require.True(t, found5, "attestation5 should be present")
	require.True(t, found6, "attestation6 should be present")
}

// TestReprocessingPrevention tests that already processed attestations are not reprocessed
func TestReprocessingPrevention(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create test data
	attestation := createTestAttestation("pubkey1", "sig1")
	tx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation})

	// Add the tx to recent block txs to simulate it was already processed
	ebs.quorumTxCache.recentBlockItems[100] = []*common.QuorumTx{tx}

	// Try to send the same tx again
	result, err := ebs.SendQuorumTx(context.Background(), tx)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, ebs.quorumTxCache.items) // No tx should be added, since it was already processed
}

// TestPartialOverlapWithRecentBlockTxs tests the case where a new tx contains both new and already
// processed attestations from recentBlockTxs
func TestPartialOverlapWithRecentBlockTxs(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create test data
	attestation1 := createTestAttestation("pubkey1", "sig1")
	attestation2 := createTestAttestation("pubkey2", "sig2")
	attestation3 := createTestAttestation("pubkey3", "sig3")

	// Create a tx that is already in recentBlockTxs with attestation1 and attestation2
	existingTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation1, attestation2})
	ebs.quorumTxCache.recentBlockItems[100] = []*common.QuorumTx{existingTx}

	// Now create a new tx with the same ID but with attestation2, attestation3
	// attestation2 is already processed, attestation3 is new
	newTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation2, attestation3})

	// Send the new tx
	result, err := ebs.SendQuorumTx(context.Background(), newTx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Only attestation3 should be added
	require.Len(t, ebs.quorumTxCache.items, 1)
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 1)
	require.Equal(t, attestation3.PubKey, ebs.quorumTxCache.items[0].Item.Attestations[0].PubKey)
	require.Equal(t, attestation3.Signature, ebs.quorumTxCache.items[0].Item.Attestations[0].Signature)
}

// TestPartialOverlapWithPendingTxs tests the case where a new tx contains attestations that
// partially overlap with attestations in a pending tx in the .txs slice
func TestPartialOverlapWithPendingTxs(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create test data
	attestation1 := createTestAttestation("pubkey1", "sig1")
	attestation2 := createTestAttestation("pubkey2", "sig2")
	attestation3 := createTestAttestation("pubkey3", "sig3")

	// Add a tx with attestation1 and attestation2
	tx1 := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation1, attestation2})
	_, err := ebs.SendQuorumTx(context.Background(), tx1)
	require.NoError(t, err)

	// Verify the initial state
	require.Len(t, ebs.quorumTxCache.items, 1)
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 2)

	// Now send a tx with the same ID but with attestation2 and attestation3
	tx2 := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation2, attestation3})
	_, err = ebs.SendQuorumTx(context.Background(), tx2)
	require.NoError(t, err)

	// The tx should now have all three attestations, with no duplicates
	require.Len(t, ebs.quorumTxCache.items, 1)
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 3)

	// Verify all attestations are present
	found1, found2, found3 := false, false, false
	for _, att := range ebs.quorumTxCache.items[0].Item.Attestations {
		if string(att.PubKey) == string(attestation1.PubKey) {
			found1 = true
		}
		if string(att.PubKey) == string(attestation2.PubKey) {
			found2 = true
		}
		if string(att.PubKey) == string(attestation3.PubKey) {
			found3 = true
		}
	}
	require.True(t, found1, "attestation1 should be present")
	require.True(t, found2, "attestation2 should be present")
	require.True(t, found3, "attestation3 should be present")
}

// TestMarkQuorumTxAttestationsConfirmed tests the MarkQuorumTxAttestationsConfirmed method
func TestMarkQuorumTxAttestationsConfirmed(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create test data
	attestation1 := createTestAttestation("pubkey1", "sig1")
	attestation2 := createTestAttestation("pubkey2", "sig2")
	tx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation1, attestation2})

	// Add the tx
	_, err := ebs.SendQuorumTx(context.Background(), tx)
	require.NoError(t, err)

	// Create context with block height
	sdkCtx := createTestSDKContext(100)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	// Mark one attestation as confirmed
	confirmTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation1})
	ebs.MarkQuorumTxAttestationsConfirmed(ctx, confirmTx)

	// Verify the tx still exists but with one attestation
	require.Len(t, ebs.quorumTxCache.items, 1)
	require.Len(t, ebs.quorumTxCache.items[0].Item.Attestations, 1)
	require.Equal(t, attestation2.PubKey, ebs.quorumTxCache.items[0].Item.Attestations[0].PubKey)

	// Verify tx was added to recentBlockTxs
	require.Len(t, ebs.quorumTxCache.recentBlockItems, 1)
	require.Contains(t, ebs.quorumTxCache.recentBlockItems, int64(100))
	require.Len(t, ebs.quorumTxCache.recentBlockItems[100], 1)

	// Mark the other attestation as confirmed
	confirmTx = createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{attestation2})
	ebs.MarkQuorumTxAttestationsConfirmed(ctx, confirmTx)

	// Verify the tx was removed since all attestations are confirmed
	require.Len(t, ebs.quorumTxCache.items, 0)
}

// TestComplexAttestationConfirmationScenario tests a more complex scenario with multiple
// attestations being confirmed at different times and across multiple transactions
func TestComplexAttestationConfirmationScenario(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Create test data
	attestation1 := createTestAttestation("pubkey1", "sig1")
	attestation2 := createTestAttestation("pubkey2", "sig2")
	attestation3 := createTestAttestation("pubkey3", "sig3")
	attestation4 := createTestAttestation("pubkey4", "sig4")

	// Create two different transactions
	tx1 := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{
		attestation1, attestation2, attestation3,
	})

	tx2 := createTestQuorumTx(common.BTCChain, "tx2", false, []*common.Attestation{
		attestation2, attestation4,
	})

	// Add both txs
	_, err := ebs.SendQuorumTx(context.Background(), tx1)
	require.NoError(t, err)
	_, err = ebs.SendQuorumTx(context.Background(), tx2)
	require.NoError(t, err)

	// Verify initial state
	require.Len(t, ebs.quorumTxCache.items, 2)

	// Create context with block height
	sdkCtx := createTestSDKContext(100)
	ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

	// Confirm attestation1 and attestation2 from tx1
	confirmTx := createTestQuorumTx(common.BTCChain, "tx1", true, []*common.Attestation{
		attestation1, attestation2,
	})
	ebs.MarkQuorumTxAttestationsConfirmed(ctx, confirmTx)

	// Verify tx1 still exists but with only attestation3
	require.Len(t, ebs.quorumTxCache.items, 2)

	// Find tx1 and verify it has only attestation3 left
	var foundTx1 *common.QuorumTx
	for _, item := range ebs.quorumTxCache.items {
		qtx := item.Item
		if string(qtx.ObsTx.Tx.ID) == "tx1" {
			foundTx1 = qtx
			break
		}
	}
	require.NotNil(t, foundTx1)
	require.Len(t, foundTx1.Attestations, 1)
	require.Equal(t, attestation3.PubKey, foundTx1.Attestations[0].PubKey)

	// Verify tx2 still has both attestations (attestation2 confirmation only affects tx1)
	var foundTx2 *common.QuorumTx
	for _, item := range ebs.quorumTxCache.items {
		qtx := item.Item
		if string(qtx.ObsTx.Tx.ID) == "tx2" {
			foundTx2 = qtx
			break
		}
	}
	require.NotNil(t, foundTx2)
	require.Len(t, foundTx2.Attestations, 2)

	// Now confirm attestation2 and attestation4 in tx2
	confirmTx = createTestQuorumTx(common.BTCChain, "tx2", false, []*common.Attestation{
		attestation2, attestation4,
	})
	sdkCtx = createTestSDKContext(101) // Different block height
	ctx = context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)
	ebs.MarkQuorumTxAttestationsConfirmed(ctx, confirmTx)

	// Verify tx2 is now gone
	require.Len(t, ebs.quorumTxCache.items, 1) // Only tx1 with attestation3 remains

	// Check that the recent block txs contain entries for both blocks
	require.Len(t, ebs.quorumTxCache.recentBlockItems, 2)
	require.Contains(t, ebs.quorumTxCache.recentBlockItems, int64(100))
	require.Contains(t, ebs.quorumTxCache.recentBlockItems, int64(101))
}

// TestRecentBlockTxsCleanup tests the cleanup of old block txs
func TestRecentBlockTxsCleanup(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	logger := log.NewNopLogger()

	ebs := NewEnshrinedBifrost(cdc, logger, EBifrostConfig{
		Enable:  true,
		Address: "localhost:50051",
	})

	// Add txs for multiple block heights
	for h := int64(1); h <= cachedBlocks+5; h++ {
		attestation := createTestAttestation("pubkey"+strconv.FormatInt(h, 10), "sig"+strconv.FormatInt(h, 10))
		tx := createTestQuorumTx(common.BTCChain, "tx"+strconv.FormatInt(h, 10), true, []*common.Attestation{attestation})

		sdkCtx := createTestSDKContext(h)
		ctx := context.WithValue(context.Background(), sdk.SdkContextKey, sdkCtx)

		ebs.MarkQuorumTxAttestationsConfirmed(ctx, tx)
	}

	// Check that old blocks were cleaned up
	require.LessOrEqual(t, len(ebs.quorumTxCache.recentBlockItems), cachedBlocks+1)

	// Highest blocks should still be there
	highBlock := int64(cachedBlocks + 5)
	require.Contains(t, ebs.quorumTxCache.recentBlockItems, highBlock)

	// Lowest blocks should be removed
	lowBlock := int64(1)
	require.NotContains(t, ebs.quorumTxCache.recentBlockItems, lowBlock)
}
