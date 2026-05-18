package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type TxOutTestSuite struct{}

var _ = Suite(&TxOutTestSuite{})

func (TxOutTestSuite) TestTxOutItemHash(c *C) {
	item := TxOutItem{
		Chain:       "ETH",
		ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		VaultPubKey: "",
		Coins: common.Coins{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(194765912)),
		},
		Memo:   "REFUND:9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
		InHash: "9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
	}
	c.Check(item.Hash(), Equals, "D3F7241B1D046E5D0AC236366069947D135840A998675FCC69FF4F26CEFB1B5C")

	item = TxOutItem{
		Chain:       "ETH",
		ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		VaultPubKey: "",
		Memo:        "REFUND:9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
		InHash:      "9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
	}
	c.Check(item.Hash(), Equals, "F5FD1B7F57CB0CDFE5A39A896C8D7C8FA8B3C1C0177474E402990A0A3671FB0B")

	item = TxOutItem{
		Chain:       "ETH",
		ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		VaultPubKey: "thorpub1addwnpepqv7kdf473gc4jyls7hlx4rg",
		Memo:        "REFUND:9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
		InHash:      "9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
	}
	c.Check(item.Hash(), Equals, "7947BD11B6ADC5861B6FFCFA4346786E7C708B1F1A67454F68403A3A647D506B")
}

func (TxOutTestSuite) TestTxOutItemEqualsShouldIgnoreHeight(c *C) {
	item1 := TxOutItem{
		Chain:       "ETH",
		ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		VaultPubKey: "",
		Coins: common.Coins{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(194765912)),
		},
		Memo:   "REFUND:9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
		InHash: "9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
	}
	item2 := TxOutItem{
		Chain:       "ETH",
		ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		VaultPubKey: "",
		Coins: common.Coins{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(194765912)),
		},
		Memo:   "REFUND:9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
		InHash: "9999A5A08D8FCF942E1AAAA01AB1E521B699BA3A009FA0591C011DC1FFDC5E68",
	}
	c.Check(item1.Equals(item2), Equals, true)

	item1.Height = 1
	item2.Height = 2
	c.Check(item1.Equals(item2), Equals, true)
}

func (TxOutTestSuite) TestTxOutItemGetMemo(c *C) {
	// Normal memo
	item := TxOutItem{Memo: "SWAP:ETH.ETH:addr"}
	c.Check(item.GetMemo(), Equals, "SWAP:ETH.ETH:addr")

	// Empty memo with OriginalMemo (memoless outbound)
	item = TxOutItem{Memo: "", OriginalMemo: "SWAP:ETH.ETH:addr2"}
	c.Check(item.GetMemo(), Equals, "SWAP:ETH.ETH:addr2")

	// Both empty
	item = TxOutItem{}
	c.Check(item.GetMemo(), Equals, "")

	// Memo set, OriginalMemo also set - Memo takes precedence
	item = TxOutItem{Memo: "memo1", OriginalMemo: "memo2"}
	c.Check(item.GetMemo(), Equals, "memo1")
}

func (TxOutTestSuite) TestTxOutItemCacheHash(c *C) {
	item := TxOutItem{
		Chain:     "ETH",
		ToAddress: "0xaddr",
		Coins:     common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(100))},
		Memo:      "SWAP:BTC.BTC:addr",
		InHash:    "inhash123",
	}
	hash := item.CacheHash()
	c.Check(len(hash), Equals, 64)

	// CacheHash should not include VaultPubKey
	item2 := item
	item2.VaultPubKey = "thorpub1different"
	c.Check(item.CacheHash(), Equals, item2.CacheHash())

	// Different ToAddress produces different hash
	item3 := item
	item3.ToAddress = "0xother"
	c.Check(item.CacheHash() != item3.CacheHash(), Equals, true)
}

func (TxOutTestSuite) TestTxOutItemCacheVault(c *C) {
	item := TxOutItem{
		VaultPubKey: "thorpub1key",
	}
	result := item.CacheVault(common.ETHChain)
	c.Check(result, Equals, "broadcast-thorpub1key-ETH")
}

func (TxOutTestSuite) TestBroadcastCacheKey(c *C) {
	result := BroadcastCacheKey("vault1", "BTC")
	c.Check(result, Equals, "broadcast-vault1-BTC")

	result = BroadcastCacheKey("", "")
	c.Check(result, Equals, "broadcast--")
}

func (TxOutTestSuite) TestTxOutItemEqualsAllFields(c *C) {
	limit := cosmos.NewUint(500)
	base := TxOutItem{
		Chain:                 "ETH",
		ToAddress:             "0xaddr",
		VaultPubKey:           "thorpub1key",
		Coins:                 common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(100))},
		Memo:                  "SWAP:BTC.BTC:addr",
		InHash:                "inhash",
		GasRate:               10,
		Aggregator:            "agg1",
		AggregatorTargetAsset: "target1",
		AggregatorTargetLimit: &limit,
		VaultPubKeyEddsa:      "eddsapub1",
	}

	// Different chain
	other := base
	other.Chain = "BTC"
	c.Check(base.Equals(other), Equals, false)

	// Different VaultPubKey
	other = base
	other.VaultPubKey = "thorpub1other"
	c.Check(base.Equals(other), Equals, false)

	// Different ToAddress
	other = base
	other.ToAddress = "0xother"
	c.Check(base.Equals(other), Equals, false)

	// Different Coins
	other = base
	other.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(999))}
	c.Check(base.Equals(other), Equals, false)

	// Different InHash
	other = base
	other.InHash = "otherhash"
	c.Check(base.Equals(other), Equals, false)

	// Different Memo
	other = base
	other.Memo = "OTHER:memo"
	c.Check(base.Equals(other), Equals, false)

	// Different GasRate
	other = base
	other.GasRate = 99
	c.Check(base.Equals(other), Equals, false)

	// Different Aggregator
	other = base
	other.Aggregator = "otheragg"
	c.Check(base.Equals(other), Equals, false)

	// Different AggregatorTargetAsset
	other = base
	other.AggregatorTargetAsset = "othertarget"
	c.Check(base.Equals(other), Equals, false)

	// AggregatorTargetLimit: one nil, one not
	other = base
	other.AggregatorTargetLimit = nil
	c.Check(base.Equals(other), Equals, false)

	// AggregatorTargetLimit: both nil
	noLimit1 := base
	noLimit1.AggregatorTargetLimit = nil
	noLimit2 := base
	noLimit2.AggregatorTargetLimit = nil
	c.Check(noLimit1.Equals(noLimit2), Equals, true)

	// AggregatorTargetLimit: nil vs non-nil (reversed)
	other = base
	other.AggregatorTargetLimit = &limit
	noLimitBase := base
	noLimitBase.AggregatorTargetLimit = nil
	c.Check(noLimitBase.Equals(other), Equals, false)

	// AggregatorTargetLimit: different values
	otherLimit := cosmos.NewUint(999)
	other = base
	other.AggregatorTargetLimit = &otherLimit
	c.Check(base.Equals(other), Equals, false)

	// Different VaultPubKeyEddsa
	other = base
	other.VaultPubKeyEddsa = "eddsapub2"
	c.Check(base.Equals(other), Equals, false)
}

func (TxOutTestSuite) TestTxArrayItemToTxOutItem(c *C) {
	limit := cosmos.NewUint(500)
	arrayItem := TxArrayItem{
		Chain:                 "ETH",
		ToAddress:             "0xaddr",
		VaultPubKey:           "thorpub1key",
		Coin:                  common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
		Memo:                  "SWAP:BTC.BTC:addr",
		OriginalMemo:          "original_memo",
		MaxGas:                common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(10))},
		GasRate:               5,
		InHash:                "inhash",
		OutHash:               "outhash",
		Aggregator:            "agg1",
		AggregatorTargetAsset: "target1",
		AggregatorTargetLimit: &limit,
		VaultPubKeyEddsa:      "eddsapub1",
	}

	txOut := arrayItem.TxOutItem(42)
	c.Check(txOut.Chain, Equals, common.Chain("ETH"))
	c.Check(txOut.ToAddress, Equals, common.Address("0xaddr"))
	c.Check(txOut.VaultPubKey, Equals, common.PubKey("thorpub1key"))
	c.Assert(len(txOut.Coins), Equals, 1)
	c.Check(txOut.Coins[0].Asset.Equals(common.ETHAsset), Equals, true)
	c.Check(txOut.Coins[0].Amount.Equal(cosmos.NewUint(100)), Equals, true)
	c.Check(txOut.Memo, Equals, "SWAP:BTC.BTC:addr")
	c.Check(txOut.OriginalMemo, Equals, "original_memo")
	c.Check(txOut.GasRate, Equals, int64(5))
	c.Check(txOut.InHash, Equals, common.TxID("inhash"))
	c.Check(txOut.OutHash, Equals, common.TxID("outhash"))
	c.Check(txOut.Aggregator, Equals, "agg1")
	c.Check(txOut.AggregatorTargetAsset, Equals, "target1")
	c.Assert(txOut.AggregatorTargetLimit, NotNil)
	c.Check(txOut.AggregatorTargetLimit.Equal(cosmos.NewUint(500)), Equals, true)
	c.Check(txOut.Height, Equals, int64(42))
	c.Check(txOut.VaultPubKeyEddsa, Equals, common.PubKey("eddsapub1"))
}
