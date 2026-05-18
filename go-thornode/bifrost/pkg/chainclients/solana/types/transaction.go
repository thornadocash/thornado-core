package types

import (
	"crypto/ed25519"
	"fmt"
)

////////////////////////////////////////////////////////////////////////////////////////
// transaction.go contains the structs and functions to serialize and deserialize a solana transaction
////////////////////////////////////////////////////////////////////////////////////////

type Signature []byte

type Transaction struct {
	Signatures []Signature
	Message    Message
}

func NewTransferTransaction(from, to PublicKey, amount uint64, memo, recentBlockHash string) Transaction {
	accountKeys := []PublicKey{from, to, SystemProgramID, MemoProgramID}

	// Memo instruction
	// Program index is 3 (referencing accountKeys) for the memo program
	// Account index is 0 for the vault/signer
	memoInstruction := NewCompiledInstruction(3, []int{0}, []byte(memo))

	// 1e8 --> 1e9
	solValue := amount * 10

	// Transfer instruction
	// Program index is 2 (referencing accountKeys) for the system program
	// Account index is 0 for the vault/signer
	// Account index is 1 for the recipient
	transferInstruction := NewCompiledTransferInstruction(2, 0, 1, solValue)

	header := MessageHeader{
		NumRequireSignatures:        1,
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: 2,
	}

	message := NewMessage(header, accountKeys, recentBlockHash, []CompiledInstruction{transferInstruction, memoInstruction})
	return NewTransaction(message)
}

func NewTransaction(message Message) Transaction {
	signatures := make([]Signature, 0, message.Header.NumRequireSignatures)
	for i := uint8(0); i < message.Header.NumRequireSignatures; i++ {
		signatures = append(signatures, make([]byte, 64))
	}
	return Transaction{
		Signatures: signatures,
		Message:    message,
	}
}

func (tx *Transaction) Serialize() ([]byte, error) {
	if len(tx.Signatures) != int(tx.Message.Header.NumRequireSignatures) {
		return nil, fmt.Errorf("invalid number of signatures")
	}

	signatureCount := UintToVarLenBytes(uint64(len(tx.Signatures)))
	messageData, err := tx.Message.Serialize()
	if err != nil {
		return nil, err
	}

	output := make([]byte, 0, len(signatureCount)+len(signatureCount)*64+len(messageData))
	output = append(output, signatureCount...)
	for _, sig := range tx.Signatures {
		output = append(output, sig...)
	}
	output = append(output, messageData...)

	return output, nil
}

func (tx *Transaction) AddSignature(sig []byte) error {
	for i := 0; i < int(tx.Message.Header.NumRequireSignatures); i++ {
		if tx.verifySignature(i, sig) {
			tx.Signatures[i] = sig
			return nil
		}
	}
	return fmt.Errorf("no matching signer")
}

func (tx *Transaction) verifySignature(acctIdx int, sig []byte) bool {
	data, err := tx.Message.Serialize()
	if err != nil {
		return false
	}
	a := tx.Message.Accounts[acctIdx]

	return ed25519.Verify(a.Bytes(), data, sig)
}
