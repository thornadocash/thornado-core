package observer

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/common"
)

// TestNewAttestationState tests the creation of a new attestation state
func TestNewAttestationState(t *testing.T) {
	// Create a test tx
	tx := &common.Tx{
		ID:    "test-tx",
		Chain: common.Chain("BTC"),
	}

	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Check initial state
	assert.Equal(t, obsTx, state.Item)
	assert.NotZero(t, state.firstAttestationObserved)
	assert.Empty(t, state.attestations)
	assert.True(t, state.initialAttestationsSent.IsZero())
	assert.True(t, state.quorumAttestationsSent.IsZero())
	assert.True(t, state.lastAttestationsSent.IsZero())
}

// TestAddAttestation tests adding attestations to the state
func TestAddAttestation(t *testing.T) {
	// Create a test tx
	tx := &common.Tx{
		ID:    "test-tx",
		Chain: common.Chain("BTC"),
	}

	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Create a private key for testing
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey().Bytes()

	// Create a mock signature (real signature verification will likely fail in tests)
	signature := []byte("mock-signature")

	// Create an attestation with the mock signature
	attestation := &common.Attestation{
		PubKey:    pubKey,
		Signature: signature,
	}

	// Override the verifySignature function for testing
	origVerifySignature := verifySignature
	defer func() { verifySignature = origVerifySignature }()
	verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
		return nil // Always succeed in tests
	}

	// Add the attestation to the state
	err := state.AddAttestation(attestation)
	require.NoError(t, err)

	// Check that the attestation was added
	assert.Equal(t, 1, len(state.attestations))
	assert.Equal(t, pubKey, state.attestations[0].attestation.PubKey)
	assert.Equal(t, signature, state.attestations[0].attestation.Signature)
	assert.False(t, state.attestations[0].sent)

	// Try adding the same attestation again (should be ignored)
	err = state.AddAttestation(attestation)
	require.NoError(t, err)
	assert.Equal(t, 1, len(state.attestations), "Adding duplicate attestation should be ignored")

	// Create a different attestation with the same public key
	differentAttestation := &common.Attestation{
		PubKey:    pubKey,
		Signature: []byte("different-signature"),
	}

	// This should fail because we're using the same pubkey
	err = state.AddAttestation(differentAttestation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature already present for")
}

// TestUnsentAttestations tests retrieving unsent attestations
func TestUnsentAttestations(t *testing.T) {
	// Create a test tx
	tx := &common.Tx{
		ID:    "test-tx",
		Chain: common.Chain("BTC"),
	}

	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Add some attestations with varying sent states
	state.attestations = []attestationSentState{
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey1"),
				Signature: []byte("sig1"),
			},
			sent: true,
		},
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey2"),
				Signature: []byte("sig2"),
			},
			sent: false,
		},
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey3"),
				Signature: []byte("sig3"),
			},
			sent: true,
		},
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey4"),
				Signature: []byte("sig4"),
			},
			sent: false,
		},
	}

	// Get unsent attestations
	unsent := state.UnsentAttestations()

	// Check that only the unsent attestations were returned
	assert.Equal(t, 2, len(unsent))
	assert.Equal(t, []byte("pubkey2"), unsent[0].PubKey)
	assert.Equal(t, []byte("pubkey4"), unsent[1].PubKey)
}

// TestAttestationsCopy tests deep copying of attestations
func TestAttestationsCopy(t *testing.T) {
	// Create a test tx
	tx := &common.Tx{
		ID:    "test-tx",
		Chain: common.Chain("BTC"),
	}
	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Add some attestations
	state.attestations = []attestationSentState{
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey1"),
				Signature: []byte("sig1"),
			},
			sent: true,
		},
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey2"),
				Signature: []byte("sig2"),
			},
			sent: false,
		},
	}

	// Create a copy of the attestations
	copy := state.AttestationsCopy()

	// Check if we have the right number of attestations
	assert.Equal(t, len(state.attestations), len(copy))

	// Check if content matches
	for i, att := range copy {
		assert.Equal(t, state.attestations[i].attestation.PubKey, att.PubKey)
		assert.Equal(t, state.attestations[i].attestation.Signature, att.Signature)

		// Ensure it's a deep copy by modifying the original and checking the copy remains unchanged
		origPubKey := append([]byte(nil), state.attestations[i].attestation.PubKey...)
		if len(state.attestations[i].attestation.PubKey) > 0 {
			state.attestations[i].attestation.PubKey[0] = 0xFF
			assert.NotEqual(t, state.attestations[i].attestation.PubKey, origPubKey)
			assert.Equal(t, origPubKey, att.PubKey) // The copy should remain unchanged
		}
	}
}

// TestUnsentCount tests counting unsent attestations
func TestUnsentCount(t *testing.T) {
	tests := []struct {
		name          string
		attestations  []attestationSentState
		expectedCount int
	}{
		{
			name:          "Empty attestations",
			attestations:  []attestationSentState{},
			expectedCount: 0,
		},
		{
			name: "All sent",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: true},
			},
			expectedCount: 0,
		},
		{
			name: "All unsent",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: false},
				{attestation: &common.Attestation{}, sent: false},
				{attestation: &common.Attestation{}, sent: false},
			},
			expectedCount: 3,
		},
		{
			name: "Mixed",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: false},
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: false},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test tx
			tx := &common.Tx{
				ID:    "test-tx",
				Chain: common.Chain("BTC"),
			}
			obsTx := &common.ObservedTx{
				Tx: *tx,
			}

			// Create a new attestation state
			state := AttestationState[*common.ObservedTx]{
				Item:                     obsTx,
				firstAttestationObserved: time.Now(),
			}
			state.attestations = tt.attestations

			// Count unsent attestations
			count := state.UnsentCount()

			// Check the count
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

// TestAttestationCount tests counting all attestations
func TestAttestationCount(t *testing.T) {
	// Create a test tx
	tx := &common.Tx{
		ID:    "test-tx",
		Chain: common.Chain("BTC"),
	}
	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Check initial count
	assert.Equal(t, 0, state.AttestationCount())

	// Add some attestations
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{}, sent: true},
		{attestation: &common.Attestation{}, sent: false},
		{attestation: &common.Attestation{}, sent: true},
	}

	// Check count after adding attestations
	assert.Equal(t, 3, state.AttestationCount())
}

// TestShouldSendLate tests determining if late attestations should be sent
func TestShouldSendLate(t *testing.T) {
	minTimeBetweenAttestations := 5 * time.Minute

	tests := []struct {
		name               string
		attestations       []attestationSentState
		firstObserved      time.Time
		initialAttsSent    time.Time
		lastAttsSent       time.Time
		expectedShouldSend bool
	}{
		{
			name: "No unsent attestations",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: true},
			},
			firstObserved:      time.Now().Add(-10 * time.Minute),
			initialAttsSent:    time.Time{},
			lastAttsSent:       time.Time{},
			expectedShouldSend: false,
		},
		{
			name: "Has unsent, no previous sends, not enough time passed",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: false},
			},
			firstObserved:      time.Now().Add(-2 * time.Minute),
			initialAttsSent:    time.Time{},
			lastAttsSent:       time.Time{},
			expectedShouldSend: false,
		},
		{
			name: "Has unsent, no previous sends, enough time passed",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: false},
			},
			firstObserved:      time.Now().Add(-10 * time.Minute),
			initialAttsSent:    time.Time{},
			lastAttsSent:       time.Time{},
			expectedShouldSend: true,
		},
		{
			name: "Has unsent, sent before, not enough time passed",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: false},
			},
			firstObserved:      time.Now().Add(-10 * time.Minute),
			initialAttsSent:    time.Now().Add(-10 * time.Minute),
			lastAttsSent:       time.Now().Add(-2 * time.Minute),
			expectedShouldSend: false,
		},
		{
			name: "Has unsent, sent before, enough time passed",
			attestations: []attestationSentState{
				{attestation: &common.Attestation{}, sent: true},
				{attestation: &common.Attestation{}, sent: false},
			},
			firstObserved:      time.Now().Add(-15 * time.Minute),
			initialAttsSent:    time.Now().Add(-15 * time.Minute),
			lastAttsSent:       time.Now().Add(-10 * time.Minute),
			expectedShouldSend: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test tx
			tx := &common.Tx{
				ID:    "test-tx",
				Chain: common.Chain("BTC"),
			}
			obsTx := &common.ObservedTx{
				Tx: *tx,
			}

			// Create a new attestation state
			state := AttestationState[*common.ObservedTx]{
				Item:                     obsTx,
				firstAttestationObserved: time.Now(),
			}
			state.attestations = tt.attestations
			state.firstAttestationObserved = tt.firstObserved
			state.initialAttestationsSent = tt.initialAttsSent
			state.lastAttestationsSent = tt.lastAttsSent

			// Check if should send late
			shouldSend := state.ShouldSendLate(minTimeBetweenAttestations)

			// Verify result
			assert.Equal(t, tt.expectedShouldSend, shouldSend)
		})
	}
}

// TestExpiredAfterQuorum tests determining if the attestation state has expired
func TestExpiredAfterQuorum(t *testing.T) {
	lateObserveTimeout := 15 * time.Minute
	nonQuorumTimeout := 30 * time.Minute

	tests := []struct {
		name            string
		quorumAttsSent  time.Time
		lastAttsSent    time.Time
		expectedExpired bool
	}{
		{
			name:            "Quorum not reached",
			quorumAttsSent:  time.Time{},
			lastAttsSent:    time.Now().Add(-10 * time.Minute),
			expectedExpired: false,
		},
		{
			name:            "Quorum reached, not expired",
			quorumAttsSent:  time.Now().Add(-10 * time.Minute),
			lastAttsSent:    time.Now().Add(-10 * time.Minute),
			expectedExpired: false,
		},
		{
			name:            "Quorum reached, expired",
			quorumAttsSent:  time.Now().Add(-20 * time.Minute),
			lastAttsSent:    time.Now().Add(-10 * time.Minute),
			expectedExpired: true,
		},
		{
			name:            "Quorum not reached, but last sent expired",
			quorumAttsSent:  time.Time{},
			lastAttsSent:    time.Now().Add(-40 * time.Minute),
			expectedExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test tx
			tx := &common.Tx{
				ID:    "test-tx",
				Chain: common.Chain("BTC"),
			}

			obsTx := &common.ObservedTx{
				Tx: *tx,
			}

			// Create a new attestation state
			state := AttestationState[*common.ObservedTx]{
				Item:                     obsTx,
				firstAttestationObserved: time.Now(),
			}
			state.quorumAttestationsSent = tt.quorumAttsSent
			state.lastAttestationsSent = tt.lastAttsSent

			// Check if expired
			expired := state.ExpiredAfterQuorum(lateObserveTimeout, nonQuorumTimeout)

			// Verify result
			assert.Equal(t, tt.expectedExpired, expired)
		})
	}
}

// TestMarkAttestationsSent tests marking attestations as sent
func TestMarkAttestationsSent(t *testing.T) {
	// Create a test tx
	tx := &common.Tx{
		ID:    "test-tx",
		Chain: common.Chain("BTC"),
	}

	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Add some attestations
	state.attestations = []attestationSentState{
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey1"),
				Signature: []byte("sig1"),
			},
			sent: false,
		},
		{
			attestation: &common.Attestation{
				PubKey:    []byte("pubkey2"),
				Signature: []byte("sig2"),
			},
			sent: false,
		},
	}

	// Check initial values
	assert.True(t, state.initialAttestationsSent.IsZero())
	assert.True(t, state.quorumAttestationsSent.IsZero())
	assert.True(t, state.lastAttestationsSent.IsZero())

	// Mark as sent (not quorum)
	state.MarkAttestationsSent(false)

	// Check updated values
	assert.False(t, state.initialAttestationsSent.IsZero())
	assert.True(t, state.quorumAttestationsSent.IsZero()) // Should still be zero
	assert.False(t, state.lastAttestationsSent.IsZero())

	// Check that all attestations are marked as sent
	for _, att := range state.attestations {
		assert.True(t, att.sent)
	}

	// Add another unsent attestation
	state.attestations = append(state.attestations, attestationSentState{
		attestation: &common.Attestation{
			PubKey:    []byte("pubkey3"),
			Signature: []byte("sig3"),
		},
		sent: false,
	})

	// Reset timestamps to test quorum marking
	initialTime := state.initialAttestationsSent
	state.quorumAttestationsSent = time.Time{}
	state.lastAttestationsSent = time.Time{}

	// Mark as sent (with quorum)
	state.MarkAttestationsSent(true)

	// Check updated values
	assert.Equal(t, initialTime, state.initialAttestationsSent) // Should not change
	assert.False(t, state.quorumAttestationsSent.IsZero())      // Should be set now
	assert.False(t, state.lastAttestationsSent.IsZero())

	// Check that all attestations are marked as sent
	for _, att := range state.attestations {
		assert.True(t, att.sent)
	}
}

// TestVerifySignature tests the signature verification function
func TestVerifySignature(t *testing.T) {
	// Create a key pair for testing
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()

	// Test valid signature
	message := []byte("test message")
	signature, err := privKey.Sign(message)
	require.NoError(t, err)

	err = verifySignature(message, signature, pubKey.Bytes())
	assert.NoError(t, err, "Valid signature should verify successfully")

	// Test invalid signature (wrong message)
	wrongMessage := []byte("wrong message")
	err = verifySignature(wrongMessage, signature, pubKey.Bytes())
	assert.Error(t, err, "Signature for wrong message should fail verification")

	// Test invalid signature (wrong pubkey)
	wrongKey := secp256k1.GenPrivKey().PubKey()
	err = verifySignature(message, signature, wrongKey.Bytes())
	assert.Error(t, err, "Signature with wrong pubkey should fail verification")

	// Test invalid signature (corrupted signature)
	corruptedSig := append([]byte(nil), signature...)
	if len(corruptedSig) > 0 {
		corruptedSig[0] ^= 0xFF // Flip bits in first byte
	}
	err = verifySignature(message, corruptedSig, pubKey.Bytes())
	assert.Error(t, err, "Corrupted signature should fail verification")
}

// TestFullWorkflow tests the complete attestation workflow
func TestFullWorkflow(t *testing.T) {
	// Create two private keys for testing
	privKey1 := secp256k1.GenPrivKey()
	pubKey1 := privKey1.PubKey().Bytes()

	privKey2 := secp256k1.GenPrivKey()
	pubKey2 := privKey2.PubKey().Bytes()

	// Create a test tx
	tx := &common.Tx{
		ID:    "workflow-tx",
		Chain: common.Chain("BTC"),
	}
	obsTx := &common.ObservedTx{
		Tx: *tx,
	}

	// Create a new attestation state
	state := AttestationState[*common.ObservedTx]{
		Item:                     obsTx,
		firstAttestationObserved: time.Now(),
	}

	// Override the verifySignature function for testing
	origVerifySignature := verifySignature
	defer func() { verifySignature = origVerifySignature }()
	verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
		return nil // Always succeed in tests
	}

	// Create and add first attestation
	att1 := &common.Attestation{
		PubKey:    pubKey1,
		Signature: []byte("sig1"),
	}

	err := state.AddAttestation(att1)
	require.NoError(t, err)

	// Check state after first attestation
	assert.Equal(t, 1, state.AttestationCount())
	assert.Equal(t, 1, state.UnsentCount())

	// Check if should send (not enough time passed)
	assert.False(t, state.ShouldSendLate(5*time.Minute))

	// Set observed time further back to simulate time passing
	state.firstAttestationObserved = time.Now().Add(-10 * time.Minute)

	// Now it should suggest sending
	assert.True(t, state.ShouldSendLate(5*time.Minute))

	// Mark as sent (not quorum)
	state.MarkAttestationsSent(false)

	// Check updated state
	assert.Equal(t, 1, state.AttestationCount())
	assert.Equal(t, 0, state.UnsentCount())
	assert.False(t, state.initialAttestationsSent.IsZero())
	assert.True(t, state.quorumAttestationsSent.IsZero())
	assert.False(t, state.lastAttestationsSent.IsZero())

	// Add a second attestation
	att2 := &common.Attestation{
		PubKey:    pubKey2,
		Signature: []byte("sig2"),
	}

	err = state.AddAttestation(att2)
	require.NoError(t, err)

	// Check state after second attestation
	assert.Equal(t, 2, state.AttestationCount())
	assert.Equal(t, 1, state.UnsentCount())

	// Shouldn't send again yet (not enough time passed)
	assert.False(t, state.ShouldSendLate(5*time.Minute))

	// Simulate time passing
	state.lastAttestationsSent = time.Now().Add(-7 * time.Minute)

	// Now it should suggest sending again
	assert.True(t, state.ShouldSendLate(5*time.Minute))

	// Mark as sent (with quorum)
	state.MarkAttestationsSent(true)

	// Check final state
	assert.Equal(t, 2, state.AttestationCount())
	assert.Equal(t, 0, state.UnsentCount())
	assert.False(t, state.initialAttestationsSent.IsZero())
	assert.False(t, state.quorumAttestationsSent.IsZero())
	assert.False(t, state.lastAttestationsSent.IsZero())

	// Not expired yet
	assert.False(t, state.ExpiredAfterQuorum(15*time.Minute, 24*time.Hour))

	// Simulate more time passing
	state.quorumAttestationsSent = time.Now().Add(-20 * time.Minute)

	// Now it should be expired
	assert.True(t, state.ExpiredAfterQuorum(15*time.Minute, 24*time.Hour))
}
