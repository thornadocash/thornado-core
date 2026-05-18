package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type TxInTestSuite struct{}

var _ = Suite(&TxInTestSuite{})

func (s *TxInTestSuite) TestNewTxInItem(c *C) {
	coins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))}
	gas := common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))}
	limit := cosmos.NewUint(500)
	item := NewTxInItem(
		100,
		"txhash123",
		"SWAP:BTC.BTC:addr",
		"sender1",
		"to1",
		coins,
		gas,
		"thorpub1key",
		"agg1",
		"aggTarget1",
		&limit,
	)
	c.Assert(item, NotNil)
	c.Check(item.BlockHeight, Equals, int64(100))
	c.Check(item.Tx, Equals, "txhash123")
	c.Check(item.Memo, Equals, "SWAP:BTC.BTC:addr")
	c.Check(item.Sender, Equals, "sender1")
	c.Check(item.To, Equals, "to1")
	c.Check(item.ObservedVaultPubKey, Equals, common.PubKey("thorpub1key"))
	c.Check(item.Aggregator, Equals, "agg1")
	c.Check(item.AggregatorTarget, Equals, "aggTarget1")
	c.Assert(item.AggregatorTargetLimit, NotNil)
	c.Check(item.AggregatorTargetLimit.Equal(cosmos.NewUint(500)), Equals, true)
	c.Check(item.CommittedUnFinalised, Equals, false)
}

func (s *TxInTestSuite) TestTxInItemEquals(c *C) {
	coins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))}
	gas := common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))}

	item1 := &TxInItem{
		BlockHeight:         100,
		Tx:                  "txhash",
		Memo:                "memo",
		Sender:              "sender",
		To:                  "to",
		Coins:               coins,
		Gas:                 gas,
		ObservedVaultPubKey: "thorpub1key",
	}
	item2 := &TxInItem{
		BlockHeight:         100,
		Tx:                  "txhash",
		Memo:                "memo",
		Sender:              "sender",
		To:                  "to",
		Coins:               coins,
		Gas:                 gas,
		ObservedVaultPubKey: "thorpub1key",
	}

	// Equal items
	c.Check(item1.Equals(item2), Equals, true)

	// Different BlockHeight
	item2.BlockHeight = 200
	c.Check(item1.Equals(item2), Equals, false)
	item2.BlockHeight = 100

	// Different Tx
	item2.Tx = "othertx"
	c.Check(item1.Equals(item2), Equals, false)
	item2.Tx = "txhash"

	// Different Memo
	item2.Memo = "othermemo"
	c.Check(item1.Equals(item2), Equals, false)
	item2.Memo = "memo"

	// Different Sender
	item2.Sender = "othersender"
	c.Check(item1.Equals(item2), Equals, false)
	item2.Sender = "sender"

	// Different To
	item2.To = "otherto"
	c.Check(item1.Equals(item2), Equals, false)
	item2.To = "to"

	// Different Coins
	item2.Coins = common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(200))}
	c.Check(item1.Equals(item2), Equals, false)
	item2.Coins = coins

	// Different Gas
	item2.Gas = common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(20))}
	c.Check(item1.Equals(item2), Equals, false)
	item2.Gas = gas

	// Different ObservedVaultPubKey
	item2.ObservedVaultPubKey = "thorpub1other"
	c.Check(item1.Equals(item2), Equals, false)
}

func (s *TxInTestSuite) TestTxInItemCopy(c *C) {
	limit := cosmos.NewUint(500)
	coins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))}
	gas := common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))}
	item := &TxInItem{
		BlockHeight:           100,
		Tx:                    "txhash",
		Memo:                  "memo",
		Sender:                "sender",
		To:                    "to",
		Coins:                 coins,
		Gas:                   gas,
		ObservedVaultPubKey:   "thorpub1key",
		Aggregator:            "agg",
		AggregatorTarget:      "target",
		AggregatorTargetLimit: &limit,
		CommittedUnFinalised:  true,
	}

	copied := item.Copy()
	c.Assert(copied, NotNil)
	c.Check(copied.BlockHeight, Equals, item.BlockHeight)
	c.Check(copied.Tx, Equals, item.Tx)
	c.Check(copied.Memo, Equals, item.Memo)
	c.Check(copied.Sender, Equals, item.Sender)
	c.Check(copied.To, Equals, item.To)
	c.Check(copied.ObservedVaultPubKey, Equals, item.ObservedVaultPubKey)
	c.Check(copied.Aggregator, Equals, item.Aggregator)
	c.Check(copied.AggregatorTarget, Equals, item.AggregatorTarget)
	c.Check(copied.AggregatorTargetLimit.Equal(*item.AggregatorTargetLimit), Equals, true)
	c.Check(copied.CommittedUnFinalised, Equals, true)

	// Verify it's a deep copy (modifying original coins doesn't affect copy)
	c.Check(copied.Equals(item), Equals, true)
}

func (s *TxInTestSuite) TestTxInItemIsEmpty(c *C) {
	// Empty item
	item := &TxInItem{}
	c.Check(item.IsEmpty(), Equals, true)

	// Non-empty: has BlockHeight
	item.BlockHeight = 1
	c.Check(item.IsEmpty(), Equals, false)
	item.BlockHeight = 0

	// Non-empty: has Tx
	item.Tx = "tx"
	c.Check(item.IsEmpty(), Equals, false)
	item.Tx = ""

	// Non-empty: has Memo
	item.Memo = "memo"
	c.Check(item.IsEmpty(), Equals, false)
	item.Memo = ""

	// Non-empty: has Sender
	item.Sender = "sender"
	c.Check(item.IsEmpty(), Equals, false)
	item.Sender = ""

	// Non-empty: has To
	item.To = "to"
	c.Check(item.IsEmpty(), Equals, false)
	item.To = ""

	// Non-empty: has Coins
	item.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))}
	c.Check(item.IsEmpty(), Equals, false)
	item.Coins = nil

	// Non-empty: has Gas
	item.Gas = common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))}
	c.Check(item.IsEmpty(), Equals, false)
	item.Gas = nil

	// Non-empty: has ObservedVaultPubKey
	item.ObservedVaultPubKey = "thorpub1key"
	c.Check(item.IsEmpty(), Equals, false)
}

func (s *TxInTestSuite) TestTxInItemCacheHash(c *C) {
	item := &TxInItem{
		To:    "to_addr",
		Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
		Memo:  "SWAP:ETH.ETH:addr",
	}
	hash := item.CacheHash(common.BTCChain, "inboundhash123")
	c.Check(len(hash), Equals, 64) // SHA256 hex string
	c.Check(hash != "", Equals, true)

	// Same inputs produce same hash
	hash2 := item.CacheHash(common.BTCChain, "inboundhash123")
	c.Check(hash, Equals, hash2)

	// Different chain produces different hash
	hash3 := item.CacheHash(common.ETHChain, "inboundhash123")
	c.Check(hash != hash3, Equals, true)

	// Different inbound hash produces different hash
	hash4 := item.CacheHash(common.BTCChain, "differenthash")
	c.Check(hash != hash4, Equals, true)
}

func (s *TxInTestSuite) TestTxInItemCacheVault(c *C) {
	item := &TxInItem{
		ObservedVaultPubKey: "thorpub1key",
	}
	result := item.CacheVault(common.BTCChain)
	c.Check(result, Equals, "inbound-thorpub1key-BTC")
}

func (s *TxInTestSuite) TestInboundCacheKey(c *C) {
	result := InboundCacheKey("vault1", "BTC")
	c.Check(result, Equals, "inbound-vault1-BTC")

	result = InboundCacheKey("", "")
	c.Check(result, Equals, "inbound--")
}

func (s *TxInTestSuite) TestGetTotalTransactionValue(c *C) {
	txIn := TxIn{
		Chain: common.BTCChain,
		TxArray: []*TxInItem{
			{
				Sender: "sender1",
				Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
				Memo:   "SWAP:ETH.ETH:addr",
			},
			{
				Sender: "sender2",
				Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(200))},
				Memo:   "SWAP:ETH.ETH:addr",
			},
		},
	}

	// Total without excludeFrom
	total := txIn.GetTotalTransactionValue(common.BTCAsset, nil)
	c.Check(total.Equal(cosmos.NewUint(300)), Equals, true)

	// Exclude one sender
	total = txIn.GetTotalTransactionValue(common.BTCAsset, []common.Address{"sender1"})
	c.Check(total.Equal(cosmos.NewUint(200)), Equals, true)

	// Exclude all senders
	total = txIn.GetTotalTransactionValue(common.BTCAsset, []common.Address{"sender1", "sender2"})
	c.Check(total.Equal(cosmos.ZeroUint()), Equals, true)

	// Request asset not in coins
	total = txIn.GetTotalTransactionValue(common.ETHAsset, nil)
	c.Check(total.Equal(cosmos.ZeroUint()), Equals, true)

	// Internal memo (MIGRATE) is skipped
	txIn.TxArray = []*TxInItem{
		{
			Sender: "sender1",
			Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
			Memo:   "SWAP:ETH.ETH:addr",
		},
		{
			Sender: "sender2",
			Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(200))},
			Memo:   "MIGRATE:100",
		},
	}
	total = txIn.GetTotalTransactionValue(common.BTCAsset, nil)
	c.Check(total.Equal(cosmos.NewUint(100)), Equals, true)

	// Empty TxArray
	txIn.TxArray = nil
	total = txIn.GetTotalTransactionValue(common.BTCAsset, nil)
	c.Check(total.Equal(cosmos.ZeroUint()), Equals, true)
}

func (s *TxInTestSuite) TestGetTotalGas(c *C) {
	txIn := TxIn{
		Chain: common.BTCChain,
		TxArray: []*TxInItem{
			{
				Gas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))},
			},
			{
				Gas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(20))},
			},
		},
	}

	total := txIn.GetTotalGas()
	c.Check(total.Equal(cosmos.NewUint(30)), Equals, true)

	// Nil gas
	txIn.TxArray = []*TxInItem{
		{
			Gas: nil,
		},
		{
			Gas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(15))},
		},
	}
	total = txIn.GetTotalGas()
	c.Check(total.Equal(cosmos.NewUint(15)), Equals, true)

	// Invalid gas (coin with empty asset) is skipped
	txIn.TxArray = []*TxInItem{
		{
			Gas: common.Gas{common.NewCoin(common.EmptyAsset, cosmos.ZeroUint())},
		},
	}
	total = txIn.GetTotalGas()
	c.Check(total.Equal(cosmos.ZeroUint()), Equals, true)

	// Empty TxArray
	txIn.TxArray = nil
	total = txIn.GetTotalGas()
	c.Check(total.Equal(cosmos.ZeroUint()), Equals, true)
}

func (s *TxInTestSuite) TestTxInItemEqualsObservedTx(c *C) {
	txID := "CF524818D42B63D25BFF4CEB10A066B7170E00A6E36DE3F66A92B6847379B053"
	coins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))}
	gas := common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))}

	item := &TxInItem{
		BlockHeight:         100,
		Tx:                  txID,
		Memo:                "SWAP:ETH.ETH:addr",
		Sender:              "sender1",
		To:                  "to1",
		Coins:               coins,
		Gas:                 gas,
		ObservedVaultPubKey: "thorpub1key",
	}

	observedTx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID(txID),
			Memo:        "SWAP:ETH.ETH:addr",
			FromAddress: common.Address("sender1"),
			ToAddress:   common.Address("to1"),
			Coins:       coins,
			Gas:         gas,
		},
		ObservedPubKey: "thorpub1key",
	}

	// Equal items
	c.Check(item.EqualsObservedTx(observedTx), Equals, true)

	// Invalid tx ID
	item2 := item.Copy()
	item2.Tx = "invalid-tx-id"
	c.Check(item2.EqualsObservedTx(observedTx), Equals, false)

	// Different TxID
	item3 := item.Copy()
	item3.Tx = "AA524818D42B63D25BFF4CEB10A066B7170E00A6E36DE3F66A92B6847379B053"
	c.Check(item3.EqualsObservedTx(observedTx), Equals, false)

	// Different Memo
	item4 := item.Copy()
	item4.Memo = "different"
	c.Check(item4.EqualsObservedTx(observedTx), Equals, false)

	// Different Sender
	item5 := item.Copy()
	item5.Sender = "othersender"
	c.Check(item5.EqualsObservedTx(observedTx), Equals, false)

	// Different To
	item6 := item.Copy()
	item6.To = "otherto"
	c.Check(item6.EqualsObservedTx(observedTx), Equals, false)

	// Different Coins
	item7 := item.Copy()
	item7.Coins = common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(999))}
	c.Check(item7.EqualsObservedTx(observedTx), Equals, false)

	// Different Gas
	item8 := item.Copy()
	item8.Gas = common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(999))}
	c.Check(item8.EqualsObservedTx(observedTx), Equals, false)

	// Different ObservedVaultPubKey
	item9 := item.Copy()
	item9.ObservedVaultPubKey = "thorpub1other"
	c.Check(item9.EqualsObservedTx(observedTx), Equals, false)
}
