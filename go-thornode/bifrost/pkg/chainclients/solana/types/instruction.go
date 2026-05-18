package types

import (
	"encoding/binary"
)

type CompiledInstruction struct {
	ProgramIDIndex int
	Accounts       []int
	Data           []byte
}

func NewCompiledInstruction(programIDIndex int, accounts []int, data []byte) CompiledInstruction {
	return CompiledInstruction{
		ProgramIDIndex: programIDIndex,
		Accounts:       accounts,
		Data:           data,
	}
}

// Create transfer instruction data
func serializeTransferInstructionData(amount uint64) []byte {
	var data []byte

	// Append transfer instruction
	transferInstruction := uint32(2)
	transferInstructionBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(transferInstructionBytes, transferInstruction)
	data = append(data, transferInstructionBytes...)

	// Append amount
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)

	return data
}

// NewCompiledTransferInstruction creates a CompiledIntstruction for a transfer instruction
func NewCompiledTransferInstruction(programIndex, fromIndex, toIndex int, amount uint64) CompiledInstruction {
	accounts := []int{fromIndex, toIndex}
	data := serializeTransferInstructionData(amount)
	return NewCompiledInstruction(programIndex, accounts, data)
}

type Instruction struct {
	ProgramID PublicKey
	Accounts  []AccountMeta
	Data      []byte
}
