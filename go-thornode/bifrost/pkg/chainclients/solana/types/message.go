package types

import (
	"encoding/binary"

	"github.com/mr-tron/base58"
)

type MessageVersion string

const (
	MessageVersionLegacy = "legacy"
	MessageVersionV0     = "v0"
)

type AccountMeta struct {
	PubKey     PublicKey
	IsSigner   bool
	IsWritable bool
}

type MessageHeader struct {
	NumRequireSignatures        uint8
	NumReadonlySignedAccounts   uint8
	NumReadonlyUnsignedAccounts uint8
}

type Message struct {
	Version             MessageVersion
	Header              MessageHeader
	Accounts            []PublicKey
	RecentBlockHash     string
	Instructions        []CompiledInstruction
	AddressLookupTables []CompiledAddressLookupTable
}

type CompiledAddressLookupTable struct {
	AccountKey      PublicKey
	WritableIndexes []uint8
	ReadonlyIndexes []uint8
}

func NewMessage(header MessageHeader, accounts []PublicKey, recentBlockHash string, instructions []CompiledInstruction) Message {
	return Message{
		Header:          header,
		Accounts:        accounts,
		RecentBlockHash: recentBlockHash,
		Instructions:    instructions,
	}
}

func (m *Message) Serialize() ([]byte, error) {
	b := []byte{}

	b = append(b, m.Header.NumRequireSignatures)
	b = append(b, m.Header.NumReadonlySignedAccounts)
	b = append(b, m.Header.NumReadonlyUnsignedAccounts)

	b = append(b, UintToVarLenBytes(uint64(len(m.Accounts)))...)
	for _, key := range m.Accounts {
		b = append(b, key[:]...)
	}

	blockHash, err := base58.Decode(m.RecentBlockHash)
	if err != nil {
		return nil, err
	}
	b = append(b, blockHash...)

	b = append(b, UintToVarLenBytes(uint64(len(m.Instructions)))...)
	for _, instruction := range m.Instructions {
		b = append(b, byte(instruction.ProgramIDIndex))
		b = append(b, UintToVarLenBytes(uint64(len(instruction.Accounts)))...)
		for _, accountIdx := range instruction.Accounts {
			b = append(b, byte(accountIdx))
		}

		b = append(b, UintToVarLenBytes(uint64(len(instruction.Data)))...)
		b = append(b, instruction.Data...)
	}

	return b, nil
}

func UintToVarLenBytes(l uint64) []byte {
	if l == 0 {
		return []byte{0x0}
	}
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, l)
	return buf[:n]
}
