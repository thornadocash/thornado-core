package types

import (
	"testing"

	. "gopkg.in/check.v1"
)

func MessageTestPackage(t *testing.T) { TestingT(t) }

type MessageTestSuite struct{}

var _ = Suite(&MessageTestSuite{})

func (s *MessageTestSuite) TestSerialize(c *C) {
	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 2,
	}

	senderAccount := MustPublicKeyFromString("9aE476sH92Vz7DMPyq5WLPkrKWivxeuTKEFKd2sZZcde")
	receiverAccount := MustPublicKeyFromString("HEhDGuxaxGr9LuNtBdvbX2uggyAKoxYgHFaAiqxVu8UY")
	accounts := []PublicKey{senderAccount, receiverAccount, SystemProgramID, MemoProgramID}

	transferInstruction := NewCompiledTransferInstruction(2, 0, 1, 1000000000)
	memoInstruction := NewCompiledInstruction(3, []int{0}, []byte("Hello World"))

	m := NewMessage(header, accounts, "9rAtxuhtKn8qagc3UtZFyhLrw5zkh6etv43TibaXuSKo", []CompiledInstruction{transferInstruction, memoInstruction})
	serialized, err := m.Serialize()
	c.Assert(err, IsNil)

	// Expected length of serialized message
	//
	// Header = 3 bytes
	//
	// Accounts Section:
	// - Length prefix for total accounts (4 accounts) = 1 byte
	// - Accounts Public Keys (4 * 32 bytes) = 128 bytes
	// Total Accounts Section = 1 + 128 = 129 bytes
	//
	// RecentBlockHash = 32 bytes
	//
	// Instructions Section:
	// - Length prefix for total instructions (2 instructions) = 1 byte
	//
	// - Transfer Instruction:
	//   - ProgramIDIndex = 1 byte
	//   - Accounts length prefix (for 2 accounts) = 1 byte
	//   - Accounts data (2 indices * 1 byte/index) = 2 bytes
	//   - Data length prefix (for 12 bytes of data) = 1 byte
	//   - Data (12 bytes, as observed in logs for transfer amount) = 12 bytes
	//   - Total Transfer Instruction = 1 + 1 + 2 + 1 + 12 = 17 bytes
	//
	// - Memo Instruction:
	//   - ProgramIDIndex = 1 byte
	//   - Accounts length prefix (for 1 account) = 1 byte
	//   - Accounts data (1 index * 1 byte/index) = 1 byte
	//   - Data length prefix (for 11 bytes of "Hello World") = 1 byte
	//   - Data (11 bytes, for "Hello World") = 11 bytes
	//   - Total Memo Instruction = 1 + 1 + 1 + 1 + 11 = 15 bytes
	//
	// Total Calculated Length = 3 (Header) + 129 (Accounts) + 32 (RecentBlockHash) + 1 (Instructions Array Length) + 17 (Transfer Instruction) + 15 (Memo Instruction) = 197 bytes
	//

	c.Assert(len(serialized), Equals, 197)
}

func (s *MessageTestSuite) TestSerializeInvalidBlockHash(c *C) {
	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 0,
	}
	m := NewMessage(header, nil, "not-valid-base58!!!", nil)
	_, err := m.Serialize()
	c.Assert(err, NotNil)
}

func (s *MessageTestSuite) TestUintToVarLenBytesZero(c *C) {
	result := UintToVarLenBytes(0)
	c.Assert(result, DeepEquals, []byte{0x0})
}

func (s *MessageTestSuite) TestUintToVarLenBytesNonZero(c *C) {
	result := UintToVarLenBytes(1)
	c.Assert(len(result) > 0, Equals, true)
	c.Assert(result[0], Equals, byte(1))

	result = UintToVarLenBytes(300)
	c.Assert(len(result) > 1, Equals, true) // 300 needs more than 1 byte in varint
}

func (s *MessageTestSuite) TestNewMessage(c *C) {
	header := MessageHeader{
		NumRequireSignatures:        2,
		NumReadonlySignedAccounts:   1,
		NumReadonlyUnsignedAccounts: 3,
	}
	accounts := []PublicKey{MustPublicKeyFromString("11111111111111111111111111111111")}
	m := NewMessage(header, accounts, "hash", []CompiledInstruction{{ProgramIDIndex: 0}})
	c.Assert(m.Header, DeepEquals, header)
	c.Assert(len(m.Accounts), Equals, 1)
	c.Assert(m.RecentBlockHash, Equals, "hash")
	c.Assert(len(m.Instructions), Equals, 1)
}
