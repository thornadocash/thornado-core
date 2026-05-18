package thorchain

import (
	"testing"

	"github.com/cometbft/cometbft/crypto/secp256k1"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func TestDeduplicateAttestations(t *testing.T) {
	att1 := &common.Attestation{PubKey: []byte("pubkey1"), Signature: []byte("sig1")}
	att2 := &common.Attestation{PubKey: []byte("pubkey2"), Signature: []byte("sig2")}
	att3 := &common.Attestation{PubKey: []byte("pubkey3"), Signature: []byte("sig3")}

	// Test empty input
	result := deduplicateAttestations(nil, 10)
	if len(result) != 0 {
		t.Fatalf("expected 0 attestations, got %d", len(result))
	}

	// Test no duplicates
	result = deduplicateAttestations([]*common.Attestation{att1, att2, att3}, 10)
	if len(result) != 3 {
		t.Fatalf("expected 3 attestations, got %d", len(result))
	}

	// Test deduplication of identical pubkeys
	dup1 := &common.Attestation{PubKey: []byte("pubkey1"), Signature: []byte("sig1_dup")}
	result = deduplicateAttestations([]*common.Attestation{att1, dup1, att2, att1, att3}, 10)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique attestations, got %d", len(result))
	}

	// Test maxCount cap
	result = deduplicateAttestations([]*common.Attestation{att1, att2, att3}, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 attestations (capped), got %d", len(result))
	}

	// Test maxCount cap with duplicates - should cap after dedup
	result = deduplicateAttestations([]*common.Attestation{att1, att1, att1, att2, att3}, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 attestations (deduped+capped), got %d", len(result))
	}

	// Test nil attestations are skipped
	result = deduplicateAttestations([]*common.Attestation{nil, att1, nil, att2}, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 attestations (nils skipped), got %d", len(result))
	}

	// Test all duplicates of one pubkey
	result = deduplicateAttestations([]*common.Attestation{att1, att1, att1, att1}, 10)
	if len(result) != 1 {
		t.Fatalf("expected 1 unique attestation, got %d", len(result))
	}
}

type HandlerQuorumHelpersSuite struct{}

var _ = Suite(&HandlerQuorumHelpersSuite{})

func (s *HandlerQuorumHelpersSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// makeActiveNode creates a NodeAccount with a real secp256k1 key pair and returns
// the node account and private key for signing attestations.
func makeActiveNode(c *C, privKey secp256k1.PrivKey) NodeAccount {
	pubKey := privKey.PubKey()
	nodeAddress := cosmos.AccAddress(pubKey.Address())
	commonPubKey, err := common.NewPubKeyFromCrypto(pubKey)
	c.Assert(err, IsNil)

	return NewNodeAccount(
		nodeAddress,
		NodeActive,
		common.PubKeySet{
			Secp256k1: commonPubKey,
			Ed25519:   commonPubKey,
		},
		GetRandomBech32ConsensusPubKey(),
		cosmos.NewUint(common.One*100_000),
		GetRandomTHORAddress(),
		1,
	)
}

// signAttestation creates a valid attestation by signing the given data with the private key.
func signAttestation(c *C, privKey secp256k1.PrivKey, signBz []byte) *common.Attestation {
	signature, err := privKey.Sign(signBz)
	c.Assert(err, IsNil)

	return &common.Attestation{
		PubKey:    privKey.PubKey().Bytes(),
		Signature: signature,
	}
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationNilAttestation(c *C) {
	_, err := verifyQuorumAttestation(NodeAccounts{}, []byte("data"), nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "attestation is nil")
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationEmptyPubKey(c *C) {
	att := &common.Attestation{PubKey: []byte{}, Signature: []byte("sig")}
	_, err := verifyQuorumAttestation(NodeAccounts{}, []byte("data"), att)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "pubkey is empty")
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationEmptySignature(c *C) {
	att := &common.Attestation{PubKey: []byte("pubkey"), Signature: []byte{}}
	_, err := verifyQuorumAttestation(NodeAccounts{}, []byte("data"), att)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "signature is empty")
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationSignerNotActive(c *C) {
	// Create a key that signs but is NOT in the active node accounts list
	signerPrivKey := secp256k1.GenPrivKey()
	signBz := []byte("some data to sign")
	att := signAttestation(c, signerPrivKey, signBz)

	// Create a different node as the only active one
	otherPrivKey := secp256k1.GenPrivKey()
	otherNode := makeActiveNode(c, otherPrivKey)

	_, err := verifyQuorumAttestation(NodeAccounts{otherNode}, signBz, att)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "signer is not an active node account:.*")
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationEmptyActiveNodes(c *C) {
	privKey := secp256k1.GenPrivKey()
	signBz := []byte("some data")
	att := signAttestation(c, privKey, signBz)

	_, err := verifyQuorumAttestation(NodeAccounts{}, signBz, att)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "signer is not an active node account:.*")
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationInvalidSignature(c *C) {
	// Create a valid active node
	privKey := secp256k1.GenPrivKey()
	node := makeActiveNode(c, privKey)

	// Sign different data than what we verify against
	att := signAttestation(c, privKey, []byte("original data"))

	_, err := verifyQuorumAttestation(NodeAccounts{node}, []byte("different data"), att)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "failed to verify signature:.*")
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationValid(c *C) {
	privKey := secp256k1.GenPrivKey()
	node := makeActiveNode(c, privKey)
	signBz := []byte("valid data to sign")
	att := signAttestation(c, privKey, signBz)

	addr, err := verifyQuorumAttestation(NodeAccounts{node}, signBz, att)
	c.Assert(err, IsNil)
	c.Assert(addr, NotNil)
	// The returned address should match the node's address
	c.Assert(addr.Equals(node.NodeAddress), Equals, true)
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationMultipleActiveNodes(c *C) {
	// Create multiple active nodes
	privKey1 := secp256k1.GenPrivKey()
	node1 := makeActiveNode(c, privKey1)

	privKey2 := secp256k1.GenPrivKey()
	node2 := makeActiveNode(c, privKey2)

	privKey3 := secp256k1.GenPrivKey()
	node3 := makeActiveNode(c, privKey3)

	activeNodes := NodeAccounts{node1, node2, node3}
	signBz := []byte("data for node2")

	// Sign with node2's key
	att := signAttestation(c, privKey2, signBz)

	addr, err := verifyQuorumAttestation(activeNodes, signBz, att)
	c.Assert(err, IsNil)
	c.Assert(addr.Equals(node2.NodeAddress), Equals, true)

	// Sign with node3's key
	att3 := signAttestation(c, privKey3, signBz)
	addr3, err := verifyQuorumAttestation(activeNodes, signBz, att3)
	c.Assert(err, IsNil)
	c.Assert(addr3.Equals(node3.NodeAddress), Equals, true)
}

func (s *HandlerQuorumHelpersSuite) TestVerifyQuorumAttestationInvalidPubKeyBytes(c *C) {
	// Create a valid active node
	privKey := secp256k1.GenPrivKey()
	node := makeActiveNode(c, privKey)

	// Use garbage pubkey bytes - Bech32ifyPubKey should still work (it just encodes bytes),
	// but it won't match any active node
	att := &common.Attestation{
		PubKey:    []byte("not-a-valid-secp256k1-pubkey-bytes"),
		Signature: []byte("fake-sig"),
	}

	_, err := verifyQuorumAttestation(NodeAccounts{node}, []byte("data"), att)
	c.Assert(err, NotNil)
}
