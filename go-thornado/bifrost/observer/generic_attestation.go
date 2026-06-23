package observer

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/thornadocash/go-thornado/common"
)

// AttestationStatePool is a sync.Pool for reusing AttestationState objects to reduce memory allocations.
type AttestationStatePool[T AttestableItem] struct {
	*sync.Pool
}

// NewAttestationStatePool creates a new AttestationStatePool
func NewAttestationStatePool[T AttestableItem]() *AttestationStatePool[T] {
	return &AttestationStatePool[T]{
		Pool: &sync.Pool{
			New: func() interface{} {
				return &AttestationState[T]{
					attestations: make([]attestationSentState, 0, 10), // Preallocate with small capacity
				}
			},
		},
	}
}

func (p *AttestationStatePool[T]) NewAttestationState(initialItem T) *AttestationState[T] {
	pooled := p.Get()
	state, ok := pooled.(*AttestationState[T])
	if !ok || pooled == nil || state == nil {
		state = &AttestationState[T]{
			attestations: make([]attestationSentState, 0, 10),
		}
	}

	// reset / initialise fields
	state.Item = initialItem
	state.attestations = state.attestations[:0]
	state.firstAttestationObserved = time.Now()
	state.initialAttestationsSent = time.Time{}
	state.quorumAttestationsSent = time.Time{}
	state.lastAttestationsSent = time.Time{}
	state.lastCommittedAttestation = time.Time{}

	return state
}

func (p *AttestationStatePool[T]) PutAttestationState(state *AttestationState[T]) {
	state.Item = *new(T)                         // Clear item
	state.attestations = state.attestations[:0]  // Clear attestations
	state.firstAttestationObserved = time.Time{} // Reset timestamps
	state.initialAttestationsSent = time.Time{}
	state.quorumAttestationsSent = time.Time{}
	state.lastAttestationsSent = time.Time{}
	state.lastCommittedAttestation = time.Time{}
	// Return to pool
	p.Put(state)
}

// AttestableItem represents any data type that can be attested
type AttestableItem interface {
	// Marshal serializes the item for signature verification
	GetSignablePayload() ([]byte, error)

	// basic nil check, other validation if necessary
	IsValid() bool
}

// AttestMessage is an interface for messages containing an attestation
type AttestMessage interface {
	// GetAttestation returns the attestation from the message
	GetAttestation() *common.Attestation

	// GetSignablePayload returns the data that is signed for verification
	GetSignablePayload() ([]byte, error)
}

// Ensure all required types implement AttestableItem
var (
	_ AttestableItem = &attestableObservedTx{}
	_ AttestableItem = &common.NetworkFee{}
	_ AttestableItem = &common.Solvency{}
	_ AttestableItem = &common.ErrataTx{}
)

// ProcessAttestation processes an attestation message by checking for duplicates,
// verifying the signature against the payload, and adding it to the attestation state
func ProcessAttestation[T AttestMessage](attestations *[]attestationSentState, msg T) error {
	attestation := msg.GetAttestation()

	// Check for duplicates
	for _, item := range *attestations {
		if bytes.Equal(item.attestation.Signature, attestation.Signature) {
			// already have the signature, ignore
			return nil
		}
		if bytes.Equal(item.attestation.PubKey, attestation.PubKey) {
			// Unexpected: we should never have a different signature from the same pubkey for the same tx.
			return fmt.Errorf("signature already present for %s", attestation.PubKey)
		}
	}

	// Get the data to verify
	signBz, err := msg.GetSignablePayload()
	if err != nil {
		return fmt.Errorf("fail to get signable payload: %w", err)
	}

	// Verify the signature
	if err := verifySignature(signBz, attestation.Signature, attestation.PubKey); err != nil {
		return fmt.Errorf("signature verification failed for %s - %w", attestation.PubKey, err)
	}

	// Add the attestation
	*attestations = append(*attestations, attestationSentState{attestation: attestation, sent: false})
	return nil
}

// attestationSentState tracks an attestation and whether it has been sent
type attestationSentState struct {
	attestation *common.Attestation
	sent        bool
	committed   bool
}

// AttestationState is a generic attestation state for any attestable item
type AttestationState[T AttestableItem] struct {
	// The item being attested
	Item T

	// List of attestations that have been collected
	attestations []attestationSentState

	// Timing information
	firstAttestationObserved time.Time
	initialAttestationsSent  time.Time
	quorumAttestationsSent   time.Time
	lastAttestationsSent     time.Time
	lastCommittedAttestation time.Time

	mu sync.Mutex
}

// AddAttestation adds a new attestation to the state
func (s *AttestationState[T]) AddAttestation(attestation *common.Attestation) error {
	if !s.Item.IsValid() {
		return fmt.Errorf("item is not valid")
	}

	for _, item := range s.attestations {
		if bytes.Equal(item.attestation.Signature, attestation.Signature) {
			// already have the signature, ignore
			return nil
		}
		if bytes.Equal(item.attestation.PubKey, attestation.PubKey) {
			// Unexpected: we should never have a different signature from the same pubkey for the same item
			return fmt.Errorf("signature already present for %s", attestation.PubKey)
		}
	}

	// Get the marshaled data for signature verification
	signBz, err := s.Item.GetSignablePayload()
	if err != nil {
		return fmt.Errorf("fail to marshal item: %w", err)
	}

	// Verify the signature
	if err := verifySignature(signBz, attestation.Signature, attestation.PubKey); err != nil {
		return fmt.Errorf("signature verification failed for %x - %w", attestation.PubKey, err)
	}

	// Add the attestation
	s.attestations = append(s.attestations, attestationSentState{attestation: attestation, sent: false})
	return nil
}

// UnsentAttestations returns all attestations that have not been sent
func (s *AttestationState[T]) UnsentAttestations() []*common.Attestation {
	unsent := make([]*common.Attestation, 0)
	for _, item := range s.attestations {
		if !item.sent {
			unsent = append(unsent, item.attestation)
		}
	}
	return unsent
}

// UncommittedAttestations returns all attestations that have not been confirmed
// in a chain block yet. Quorum submissions must include the full pending set so
// the state machine can verify quorum in one injected message.
func (s *AttestationState[T]) UncommittedAttestations() []*common.Attestation {
	atts := make([]*common.Attestation, 0, len(s.attestations))
	for _, item := range s.attestations {
		if !item.committed {
			atts = append(atts, item.attestation)
		}
	}
	return atts
}

// AttestationsCopy returns a deep copy of all attestations
func (s *AttestationState[T]) AttestationsCopy() []*common.Attestation {
	atts := make([]*common.Attestation, 0, len(s.attestations))
	for _, item := range s.attestations {
		if item.committed {
			// skip committed attestations
			continue
		}
		atts = append(atts, &common.Attestation{
			PubKey:    append([]byte(nil), item.attestation.PubKey...),
			Signature: append([]byte(nil), item.attestation.Signature...),
		})
	}
	return atts
}

// UnsentCount returns the number of attestations that have not been sent
func (s *AttestationState[T]) UnsentCount() int {
	count := 0
	for _, item := range s.attestations {
		if !item.sent {
			count++
		}
	}
	return count
}

// AttestationCount returns the total number of attestations
func (s *AttestationState[T]) AttestationCount() int {
	return len(s.attestations)
}

// ShouldSendLate determines if late attestations should be sent
func (s *AttestationState[T]) ShouldSendLate(minTimeBetweenAttestations time.Duration) bool {
	if s.UnsentCount() == 0 {
		// nothing to send
		return false
	}

	if !s.lastAttestationsSent.IsZero() && time.Since(s.lastAttestationsSent) > minTimeBetweenAttestations {
		// we have sent attestations before, and it's been long enough to send again
		return true
	}

	if s.initialAttestationsSent.IsZero() && time.Since(s.firstAttestationObserved) > minTimeBetweenAttestations {
		// we haven't sent any attestations yet, and it's been long enough to send the first batch
		return true
	}

	return false
}

// ExpiredAfterQuorum determines if the attestation state can be pruned
func (s *AttestationState[T]) ExpiredAfterQuorum(lateObserveTimeout, nonQuorumTimeout time.Duration) bool {
	if !s.lastAttestationsSent.IsZero() && time.Since(s.lastAttestationsSent) > nonQuorumTimeout {
		// we haven't received a new attestation in a long time, stop tracking this item
		return true
	}

	allAttestationsCommitted := true
	for _, item := range s.attestations {
		if !item.committed {
			allAttestationsCommitted = false
			break
		}
	}

	if allAttestationsCommitted && !s.lastCommittedAttestation.IsZero() && time.Since(s.lastCommittedAttestation) > lateObserveTimeout {
		// all attestations have been committed to the chain, and it's been too long since the last one
		return true
	}

	if s.quorumAttestationsSent.IsZero() {
		// we haven't reached quorum yet, so we can't expire
		return false
	}

	if time.Since(s.quorumAttestationsSent) > lateObserveTimeout {
		// we have reached quorum, but it's been too long. Stop tracking this item.
		return true
	}

	return false
}

func (s *AttestationState[T]) State() string {
	return fmt.Sprintf("sent: %d, total: %d post-quorum: %t", s.UnsentCount(), len(s.attestations), !s.quorumAttestationsSent.IsZero())
}

// MarkAttestationsSent marks all attestations as sent and updates timestamps
func (s *AttestationState[T]) MarkAttestationsSent(isQuorum bool) {
	timestamp := time.Now()

	s.lastAttestationsSent = timestamp
	if s.initialAttestationsSent.IsZero() {
		s.initialAttestationsSent = timestamp
	}
	if isQuorum && s.quorumAttestationsSent.IsZero() {
		s.quorumAttestationsSent = timestamp
	}

	// Mark attestations as sent
	for i := range s.attestations {
		s.attestations[i].sent = true
	}
}

func (s *AttestationState[T]) MarkAttestationsCommitted(commitedAtts []*common.Attestation) {
	sigSet := make(map[string]struct{}, len(commitedAtts))
	for _, c := range commitedAtts {
		sigSet[string(c.Signature)] = struct{}{}
	}
	for i := range s.attestations {
		if _, ok := sigSet[string(s.attestations[i].attestation.Signature)]; ok {
			s.attestations[i].committed = true
		}
	}

	s.lastCommittedAttestation = time.Now()
}

// verifySignature verifies that a signature is valid for a specific public key and data
var verifySignature = func(signBz []byte, signature []byte, attester []byte) error {
	pub := secp256k1.PubKey{Key: attester}
	if !pub.VerifySignature(signBz, signature) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}
