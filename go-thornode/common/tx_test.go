package common

import (
	"strings"

	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type TxSuite struct{}

var _ = Suite(&TxSuite{})

func (s TxSuite) TestTxID(c *C) {
	testCases := []struct {
		Id      string
		IsValid bool
		ToUpper bool
	}{
		{
			// Cosmos
			Id:      "A7DA8FF1B7C290616D68A276F30AC618315E6CCE982EB8F7A79339E163798F49",
			IsValid: true,
			ToUpper: true,
		}, {
			// Cosmos indexed
			Id:      "A7DA8FF1B7C290616D68A276F30AC618315E6CCE982EB8F7A79339E163798F49-1",
			IsValid: true,
			ToUpper: true,
		}, {
			// Cosmos indexed, big number
			Id:      "A7DA8FF1B7C290616D68A276F30AC618315E6CCE982EB8F7A79339E163798F49-97836",
			IsValid: true,
			ToUpper: true,
		}, {
			// EVM
			Id:      "0xb41cf456e942f3430681298c503def54b79a96e3373ef9d44ea314d7eae41952",
			IsValid: true,
			ToUpper: true,
		}, {
			// SOL (87 chars, base58 is case-sensitive so must NOT be uppercased)
			Id:      "28BRBs5NKV2BaGkKcCxX7PorxbLtiZyGmDP3zt9ChsJaNGS51Eqop8JGKSMh66R93YFXYfec7yp5mHfxnqqo1WtE",
			IsValid: true,
			ToUpper: false,
		}, {
			// SOL (88 chars)
			Id:      "5VERv8NMHYqQsM5Cg9J2kDfqyJzHmo6YNXRNh1JFhWaBa1ZNK5e2FtJCA7m1AimKs4Gbno3LnFruJHcxHBxhRFqR",
			IsValid: true,
			ToUpper: false,
		}, {
			// Bogus
			Id:      "bogus",
			IsValid: false,
		}, {
			// Cosmos indexed, invalid index char
			Id:      "A7DA8FF1B7C290616D68A276F30AC618315E6CCE982EB8F7A79339E163798F49-A",
			IsValid: false,
		}, {
			// Cosmos indexed, invalid index at pos 63
			Id:      "A7DA8FF1B7C290616D68A276F30AC618315E6CCE982EB8F7A79339E163798F4-12",
			IsValid: false,
		}, {
			// Cosmos indexed, multiple dashes
			Id:      "A7DA8FF1B7C290616D68A-76F30AC618315E6-CE982EB8F7A79339E163798F49-1",
			IsValid: false,
		},
	}

	for _, testCase := range testCases {
		tx, err := NewTxID(testCase.Id)

		if testCase.IsValid {
			c.Assert(err, IsNil)

			expectedId := testCase.Id
			if testCase.ToUpper {
				expectedId = strings.ToUpper(testCase.Id)
			}

			c.Check(tx.String(), Equals, expectedId)
			c.Check(tx.IsEmpty(), Equals, false)
			c.Check(tx.Equals(TxID(testCase.Id)), Equals, true)
			c.Check(func() { tx.Int64() }, Not(Panics), "Failed to convert")
		} else {
			c.Check(err, NotNil)
			c.Check(tx.String(), Equals, "")
			c.Check(tx.Int64(), Equals, int64(0))
		}
	}
}

func (s TxSuite) TestTx(c *C) {
	id, err := NewTxID("0xb41cf456e942f3430681298c503def54b79a96e3373ef9d44ea314d7eae41952")
	c.Assert(err, IsNil)
	tx := NewTx(
		id,
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Coins{NewCoin(ETHAsset, cosmos.NewUint(5*One))},
		Gas{NewCoin(ETHAsset, cosmos.NewUint(10000))},
		"hello memo",
	)
	c.Check(tx.ID.Equals(id), Equals, true)
	c.Check(tx.IsEmpty(), Equals, false)
	c.Check(tx.FromAddress.IsEmpty(), Equals, false)
	c.Check(tx.ToAddress.IsEmpty(), Equals, false)
	c.Assert(tx.Coins, HasLen, 1)
	c.Check(tx.Coins[0].Equals(NewCoin(ETHAsset, cosmos.NewUint(5*One))), Equals, true)
	c.Check(tx.Memo, Equals, "hello memo")
}

func (s TxSuite) TestEqualsExIgnoreGas(c *C) {
	id, err := NewTxID("0xb41cf456e942f3430681298c503def54b79a96e3373ef9d44ea314d7eae41952")
	c.Assert(err, IsNil)

	tx1 := NewTx(
		id,
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635b"),
		Coins{NewCoin(ETHAsset, cosmos.NewUint(5*One))},
		Gas{NewCoin(ETHAsset, cosmos.NewUint(22131))},
		"OUT:xyz",
	)

	// Same tx with different gas should match with EqualsExIgnoreGas but not EqualsEx
	tx2 := NewTx(
		id,
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635b"),
		Coins{NewCoin(ETHAsset, cosmos.NewUint(5*One))},
		Gas{NewCoin(ETHAsset, cosmos.NewUint(23061))},
		"OUT:xyz",
	)

	c.Check(tx1.EqualsEx(tx2), Equals, false)
	c.Check(tx1.EqualsExIgnoreGas(tx2), Equals, true)

	// Same gas should match both
	tx3 := NewTx(
		id,
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635b"),
		Coins{NewCoin(ETHAsset, cosmos.NewUint(5*One))},
		Gas{NewCoin(ETHAsset, cosmos.NewUint(22131))},
		"OUT:xyz",
	)
	c.Check(tx1.EqualsEx(tx3), Equals, true)
	c.Check(tx1.EqualsExIgnoreGas(tx3), Equals, true)

	// Different coins should fail both
	tx4 := NewTx(
		id,
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635b"),
		Coins{NewCoin(ETHAsset, cosmos.NewUint(6*One))},
		Gas{NewCoin(ETHAsset, cosmos.NewUint(22131))},
		"OUT:xyz",
	)
	c.Check(tx1.EqualsEx(tx4), Equals, false)
	c.Check(tx1.EqualsExIgnoreGas(tx4), Equals, false)

	// Different memo should fail both
	tx5 := NewTx(
		id,
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a"),
		Address("0x90f2b1ae50e6018230e90a33f98c7844a0ab635b"),
		Coins{NewCoin(ETHAsset, cosmos.NewUint(5*One))},
		Gas{NewCoin(ETHAsset, cosmos.NewUint(22131))},
		"OUT:abc",
	)
	c.Check(tx1.EqualsEx(tx5), Equals, false)
	c.Check(tx1.EqualsExIgnoreGas(tx5), Equals, false)
}
