package types

import (
	"crypto/ed25519"
	"testing"

	. "gopkg.in/check.v1"
)

func TransactionTestPackage(t *testing.T) { TestingT(t) }

type TransactionTestSuite struct{}

var _ = Suite(&TransactionTestSuite{})

func (s *TransactionTestSuite) TestNewTransferTransaction(c *C) {
	from := MustPublicKeyFromString("9aE476sH92Vz7DMPyq5WLPkrKWivxeuTKEFKd2sZZcde")
	to := MustPublicKeyFromString("HEhDGuxaxGr9LuNtBdvbX2uggyAKoxYgHFaAiqxVu8UY")
	recentBlockHash := "9rAtxuhtKn8qagc3UtZFyhLrw5zkh6etv43TibaXuSKo"
	memo := "test memo"
	amount := uint64(1000000)

	tx := NewTransferTransaction(from, to, amount, memo, recentBlockHash)

	// Should have exactly 1 signature slot (NumRequireSignatures=1)
	c.Assert(len(tx.Signatures), Equals, 1)
	c.Assert(len(tx.Signatures[0]), Equals, 64)

	// Message should have 4 accounts: from, to, SystemProgramID, MemoProgramID
	c.Assert(len(tx.Message.Accounts), Equals, 4)
	c.Assert(tx.Message.Accounts[0], DeepEquals, from)
	c.Assert(tx.Message.Accounts[1], DeepEquals, to)
	c.Assert(tx.Message.Accounts[2], DeepEquals, SystemProgramID)
	c.Assert(tx.Message.Accounts[3], DeepEquals, MemoProgramID)

	// Header
	c.Assert(tx.Message.Header.NumRequireSignatures, Equals, uint8(1))
	c.Assert(tx.Message.Header.NumReadonlySignedAccounts, Equals, uint8(0))
	c.Assert(tx.Message.Header.NumReadonlyUnsignedAccounts, Equals, uint8(2))

	// Should have 2 instructions: transfer and memo
	c.Assert(len(tx.Message.Instructions), Equals, 2)

	// Transfer instruction (index 0): program index 2 (system program)
	c.Assert(tx.Message.Instructions[0].ProgramIDIndex, Equals, 2)
	c.Assert(tx.Message.Instructions[0].Accounts, DeepEquals, []int{0, 1})

	// Memo instruction (index 1): program index 3 (memo program)
	c.Assert(tx.Message.Instructions[1].ProgramIDIndex, Equals, 3)
	c.Assert(tx.Message.Instructions[1].Accounts, DeepEquals, []int{0})
	c.Assert(tx.Message.Instructions[1].Data, DeepEquals, []byte(memo))

	c.Assert(tx.Message.RecentBlockHash, Equals, recentBlockHash)
}

func (s *TransactionTestSuite) TestNewTransaction(c *C) {
	header := MessageHeader{
		NumRequireSignatures:        2,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 1,
	}
	msg := NewMessage(header, nil, "", nil)
	tx := NewTransaction(msg)

	c.Assert(len(tx.Signatures), Equals, 2)
	for _, sig := range tx.Signatures {
		c.Assert(len(sig), Equals, 64)
	}
}

func (s *TransactionTestSuite) TestTransactionSerialize(c *C) {
	from := MustPublicKeyFromString("9aE476sH92Vz7DMPyq5WLPkrKWivxeuTKEFKd2sZZcde")
	to := MustPublicKeyFromString("HEhDGuxaxGr9LuNtBdvbX2uggyAKoxYgHFaAiqxVu8UY")
	recentBlockHash := "9rAtxuhtKn8qagc3UtZFyhLrw5zkh6etv43TibaXuSKo"

	tx := NewTransferTransaction(from, to, 100, "memo", recentBlockHash)
	data, err := tx.Serialize()
	c.Assert(err, IsNil)
	c.Assert(len(data) > 0, Equals, true)
}

func (s *TransactionTestSuite) TestTransactionSerializeSignatureMismatch(c *C) {
	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 0,
	}
	msg := NewMessage(header, nil, "11111111111111111111111111111111", nil)
	tx := Transaction{
		Signatures: []Signature{make([]byte, 64), make([]byte, 64)}, // 2 sigs but header says 1
		Message:    msg,
	}
	_, err := tx.Serialize()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "invalid number of signatures")
}

func (s *TransactionTestSuite) TestTransactionSerializeInvalidBlockHash(c *C) {
	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 0,
	}
	msg := NewMessage(header, nil, "not-valid-base58!!!", nil)
	tx := NewTransaction(msg)
	_, err := tx.Serialize()
	c.Assert(err, NotNil)
}

func (s *TransactionTestSuite) TestAddSignatureAndVerify(c *C) {
	// Generate a real ed25519 keypair
	pub, priv, err := ed25519.GenerateKey(nil)
	c.Assert(err, IsNil)

	var pubKey PublicKey
	copy(pubKey[:], pub)

	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 0,
	}
	accounts := []PublicKey{pubKey}
	msg := NewMessage(header, accounts, "11111111111111111111111111111111", nil)
	tx := NewTransaction(msg)

	// Sign the message
	msgData, err := msg.Serialize()
	c.Assert(err, IsNil)
	sig := ed25519.Sign(priv, msgData)

	// AddSignature should succeed
	err = tx.AddSignature(sig)
	c.Assert(err, IsNil)
	c.Assert([]byte(tx.Signatures[0]), DeepEquals, sig)
}

func (s *TransactionTestSuite) TestAddSignatureNoMatch(c *C) {
	// Use one key for the account and sign with a different key
	pub1, _, err := ed25519.GenerateKey(nil)
	c.Assert(err, IsNil)
	_, priv2, err := ed25519.GenerateKey(nil)
	c.Assert(err, IsNil)

	var pubKey1 PublicKey
	copy(pubKey1[:], pub1)

	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 0,
	}
	accounts := []PublicKey{pubKey1}
	msg := NewMessage(header, accounts, "11111111111111111111111111111111", nil)
	tx := NewTransaction(msg)

	// Sign with different key
	msgData, err := msg.Serialize()
	c.Assert(err, IsNil)
	sig := ed25519.Sign(priv2, msgData)

	err = tx.AddSignature(sig)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "no matching signer")
}

func (s *TransactionTestSuite) TestVerifySignatureSerializeError(c *C) {
	// Create a transaction with an invalid message that fails to serialize
	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 0,
	}
	msg := Message{
		Header:          header,
		Accounts:        []PublicKey{{}},
		RecentBlockHash: "not-valid-base58!!!",
		Instructions:    nil,
	}
	tx := NewTransaction(msg)

	// verifySignature should return false when serialize fails
	result := tx.verifySignature(0, make([]byte, 64))
	c.Assert(result, Equals, false)
}
