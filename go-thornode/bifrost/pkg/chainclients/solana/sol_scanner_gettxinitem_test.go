package solana

import (
	"encoding/binary"

	"github.com/mr-tron/base58"
	"github.com/rs/zerolog/log"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/solana/rpc"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

const (
	testSystemProgram = "11111111111111111111111111111111"
	testMemoProgram   = "MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr"
	testSender        = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	testVault         = "6sbzC1eH4FTujJXWj51eQe25cYvr4xfXbJ1vAj7j2k5J"
	testNewAccount    = "HN7cABqLq46Es1jh92dQQisAq662SmxELLLsHHe4YWrH"
	testTxSig         = "5VERv8NMHYqQsM5Cg9J2kDfqyJzHmo6YNXRNh1JFhWaBa1ZNK5e2FtJCA7m1AimKs4Gbno3LnFruJHcxHBxhRFqR"
)

type GetTxInItemSuite struct{}

var _ = Suite(&GetTxInItemSuite{})

func makeTransferData(amount uint64) string {
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:4], 2)
	binary.LittleEndian.PutUint64(data[4:12], amount)
	return base58.Encode(data)
}

func makeCreateAccountData(lamports uint64) string {
	data := make([]byte, 52)
	// opcode 0 = CreateAccount (first 4 bytes already zero)
	binary.LittleEndian.PutUint64(data[4:12], lamports)
	return base58.Encode(data)
}

func makeMemoData(memo string) string {
	return base58.Encode([]byte(memo))
}

func newTestScanner() *SOLScanner {
	return &SOLScanner{
		cfg:    config.BifrostBlockScannerConfiguration{ChainID: common.SOLChain},
		logger: log.Logger.With().Str("module", "test").Logger(),
	}
}

// TestNormalTransfer verifies a standard single-transfer transaction is parsed correctly,
// reading the amount from instruction data.
func (s *GetTxInItemSuite) TestNormalTransfer(c *C) {
	scanner := newTestScanner()

	oneSol := uint64(1_000_000_000)
	fee := uint64(5000)

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          fee,
			PreBalances:  []uint64{10_000_000_000, 50_000_000_000, 1},
			PostBalances: []uint64{10_000_000_000 - oneSol - fee, 50_000_000_000 + oneSol, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram, testMemoProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 2, // System Program
						Accounts:       []int{0, 1},
						Data:           makeTransferData(oneSol),
					},
					{
						ProgramIdIndex: 3, // Memo Program
						Accounts:       []int{0},
						Data:           makeMemoData("=:ETH.ETH:0x1234567890abcdef"),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, NotNil)
	c.Assert(item.Sender, Equals, testSender)
	c.Assert(item.To, Equals, testVault)
	c.Assert(item.Memo, Equals, "=:ETH.ETH:0x1234567890abcdef")
	c.Assert(item.Coins[0].Amount.Equal(convertLamportsToTHORChain(oneSol)), Equals, true,
		Commentf("expected %s, got %s", convertLamportsToTHORChain(oneSol), item.Coins[0].Amount))
}

// TestBalanceDeltaAttack verifies that an attacker cannot inflate the observed amount
// by including additional instructions (e.g. CreateAccount) that debit their balance
// beyond the actual transfer amount.
func (s *GetTxInItemSuite) TestBalanceDeltaAttack(c *C) {
	scanner := newTestScanner()

	transferAmount := uint64(1_000_000_000)       // 1 SOL transferred to vault
	createAccountAmount := uint64(99_000_000_000) // 99 SOL used in CreateAccount
	fee := uint64(5000)

	// Sender's balance drops by 100 SOL + fee, but only 1 SOL goes to vault
	senderPreBalance := uint64(200_000_000_000)
	senderPostBalance := senderPreBalance - transferAmount - createAccountAmount - fee

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          fee,
			PreBalances:  []uint64{senderPreBalance, 50_000_000_000, 0, 1},
			PostBalances: []uint64{senderPostBalance, 50_000_000_000 + transferAmount, createAccountAmount, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testNewAccount, testSystemProgram, testMemoProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 3, // System Program
						Accounts:       []int{0, 1},
						Data:           makeTransferData(transferAmount),
					},
					{
						ProgramIdIndex: 3, // System Program (CreateAccount)
						Accounts:       []int{0, 2},
						Data:           makeCreateAccountData(createAccountAmount),
					},
					{
						ProgramIdIndex: 4, // Memo Program
						Accounts:       []int{0},
						Data:           makeMemoData("=:ETH.ETH:0x1234567890abcdef"),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, NotNil)

	// The amount should be 1 SOL (from instruction data), NOT 100 SOL (from balance delta)
	expectedAmount := convertLamportsToTHORChain(transferAmount)
	c.Assert(item.Coins[0].Amount.Equal(expectedAmount), Equals, true,
		Commentf("expected %s, got %s — balance delta attack should not inflate amount",
			expectedAmount, item.Coins[0].Amount))
}

// TestMultipleTransfersRejected verifies that transactions with more than one
// System Program Transfer instruction are rejected.
func (s *GetTxInItemSuite) TestMultipleTransfersRejected(c *C) {
	scanner := newTestScanner()

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 50_000_000_000, 1},
			PostBalances: []uint64{8_000_000_000, 52_000_000_000, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 2,
						Accounts:       []int{0, 1},
						Data:           makeTransferData(1_000_000_000),
					},
					{
						ProgramIdIndex: 2,
						Accounts:       []int{0, 1},
						Data:           makeTransferData(1_000_000_000),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil)
}

// TestNoTransferInstruction verifies that a transaction with no Transfer instruction
// (e.g. only a memo) returns nil.
func (s *GetTxInItemSuite) TestNoTransferInstruction(c *C) {
	scanner := newTestScanner()

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 1},
			PostBalances: []uint64{10_000_000_000, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testMemoProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 1,
						Accounts:       []int{0},
						Data:           makeMemoData("some memo"),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil)
}

// TestZeroAmountTransfer verifies that a transfer of zero lamports returns nil.
func (s *GetTxInItemSuite) TestZeroAmountTransfer(c *C) {
	scanner := newTestScanner()

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 50_000_000_000, 1},
			PostBalances: []uint64{10_000_000_000, 50_000_000_000, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 2,
						Accounts:       []int{0, 1},
						Data:           makeTransferData(0),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil)
}

// TestSubDustTransferRejected verifies that transfers below the SOL dust threshold
// are ignored and not emitted as observations.
func (s *GetTxInItemSuite) TestSubDustTransferRejected(c *C) {
	scanner := newTestScanner()

	subDustLamports := uint64(999_999) // 99_999 in THORChain precision, below the 100_000 dust threshold

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 50_000_000_000, 1},
			PostBalances: []uint64{10_000_000_000 - subDustLamports - 5000, 50_000_000_000 + subDustLamports, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 2,
						Accounts:       []int{0, 1},
						Data:           makeTransferData(subDustLamports),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil)
}

// TestDustThresholdTransferAccepted verifies that a transfer exactly at the
// SOL dust threshold is still accepted.
func (s *GetTxInItemSuite) TestDustThresholdTransferAccepted(c *C) {
	scanner := newTestScanner()

	dustLamports := uint64(1_000_000) // 100_000 in THORChain precision, exactly the dust threshold

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 50_000_000_000, 1},
			PostBalances: []uint64{10_000_000_000 - dustLamports - 5000, 50_000_000_000 + dustLamports, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 2,
						Accounts:       []int{0, 1},
						Data:           makeTransferData(dustLamports),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, NotNil)
	c.Assert(item.Coins[0].Amount.Equal(common.SOLChain.DustThreshold()), Equals, true)
}

// TestShortInstructionData verifies that a Transfer instruction with data shorter
// than 12 bytes is rejected.
func (s *GetTxInItemSuite) TestShortInstructionData(c *C) {
	scanner := newTestScanner()

	// Only 4 bytes (opcode only, missing amount)
	shortData := make([]byte, 4)
	binary.LittleEndian.PutUint32(shortData[0:4], 2)

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 50_000_000_000, 1},
			PostBalances: []uint64{9_000_000_000, 51_000_000_000, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 2,
						Accounts:       []int{0, 1},
						Data:           base58.Encode(shortData),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil)
}

// TestAccountIndexOutOfRange verifies that instructions with account indices
// beyond the AccountKeys array are rejected.
func (s *GetTxInItemSuite) TestAccountIndexOutOfRange(c *C) {
	scanner := newTestScanner()

	txn := &rpc.TransactionResult{
		Slot: 100,
		Meta: rpc.RPCMeta{
			Fee:          5000,
			PreBalances:  []uint64{10_000_000_000, 1},
			PostBalances: []uint64{9_000_000_000, 1},
		},
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{
						ProgramIdIndex: 1,
						Accounts:       []int{0, 5}, // index 5 is out of range
						Data:           makeTransferData(1_000_000_000),
					},
				},
			},
		},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil)
}

// TestCalculateMedianDoesNotMutate verifies that calculateMedian does not
// modify the original slice order. Previously it sorted in place, which
// corrupted the chronological order of feeRateCache and caused truncation
// to keep the highest values instead of the most recent.
func (s *GetTxInItemSuite) TestCalculateMedianDoesNotMutate(c *C) {
	original := []uint64{500, 100, 300, 200, 400}
	expectedOrder := make([]uint64, len(original))
	copy(expectedOrder, original)

	median := calculateMedian(original)
	c.Assert(median, Equals, uint64(300))

	// The original slice must remain in its original order
	for i, v := range original {
		c.Assert(v, Equals, expectedOrder[i],
			Commentf("index %d: expected %d, got %d — calculateMedian mutated the input slice", i, expectedOrder[i], v))
	}
}
