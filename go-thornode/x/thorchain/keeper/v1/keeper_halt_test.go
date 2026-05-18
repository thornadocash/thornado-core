package keeperv1

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
)

type KeeperHaltSuite struct{}

var _ = Suite(&KeeperHaltSuite{})

func (s *KeeperHaltSuite) TestIsTradingHalt(c *C) {
	ctx, k := setupKeeperForTest(c)

	tx := common.Tx{Coins: common.Coins{common.Coin{Asset: common.BTCAsset}}}
	swapMsg := &MsgSwap{Tx: tx, TargetAsset: common.ETHAsset}
	addMsg := &MsgAddLiquidity{Asset: common.ETHAsset}
	withdrawMsg := &MsgWithdrawLiquidity{Asset: common.ETHAsset}

	// no halts
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	// eth ragnarok
	k.SetMimir(ctx, "RAGNAROK-ETH-ETH", 1)
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, true) // target asset
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	// synth to l1 bypasses ragnarok check for swaps
	swapMsg.Tx.Coins[0].Asset = common.ETHAsset.GetSyntheticAsset()
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	swapMsg.Tx.Coins[0].Asset = common.BTCAsset
	_ = k.DeleteMimir(ctx, "RAGNAROK-ETH-ETH")

	// btc ragnarok
	k.SetMimir(ctx, "RAGNAROK-BTC-BTC", 1)
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, true) // source asset
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	_ = k.DeleteMimir(ctx, "RAGNAROK-BTC-BTC")

	// btc chain trading halt
	k.SetMimir(ctx, "HaltBTCTrading", 1)
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, true) // source asset
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	_ = k.DeleteMimir(ctx, "HaltBTCTrading")

	// eth chain trading halt
	k.SetMimir(ctx, "HaltETHTrading", 1)
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, true) // target asset
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	_ = k.DeleteMimir(ctx, "HaltETHTrading")

	// global trading halt
	k.SetMimir(ctx, "HaltTrading", 1)
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltTrading")

	// TCY trading halt
	k.SetMimir(ctx, "HaltTCYTrading", 1)

	txTCY := common.Tx{Coins: common.Coins{common.Coin{Asset: common.RuneNative}}}
	swapTCYMsg := &MsgSwap{Tx: txTCY, TargetAsset: common.TCY}
	addTCYMsg := &MsgAddLiquidity{Asset: common.TCY}
	withdrawTCYMsg := &MsgWithdrawLiquidity{Asset: common.TCY}

	c.Check(k.IsTradingHalt(ctx, swapTCYMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, addTCYMsg), Equals, true)
	c.Check(k.IsTradingHalt(ctx, withdrawTCYMsg), Equals, false)

	txTCY = common.Tx{Coins: common.Coins{common.Coin{Asset: common.TCY}}}
	swapTCYMsg = &MsgSwap{Tx: txTCY, TargetAsset: common.RuneNative}
	c.Check(k.IsTradingHalt(ctx, swapTCYMsg), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltTCYTrading")

	// ETH trading halt from TCY
	k.SetMimir(ctx, "HaltETHTrading", 1)

	txTCY = common.Tx{Coins: common.Coins{common.Coin{Asset: common.TCY}}}
	swapTCYMsg = &MsgSwap{Tx: txTCY, TargetAsset: common.ETHAsset}
	c.Check(k.IsTradingHalt(ctx, swapTCYMsg), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltETHTrading")

	// ETH trading halt to TCY
	k.SetMimir(ctx, "HaltETHTrading", 1)

	txTCY = common.Tx{Coins: common.Coins{common.Coin{Asset: common.ETHAsset}}}
	swapTCYMsg = &MsgSwap{Tx: txTCY, TargetAsset: common.TCY}
	c.Check(k.IsTradingHalt(ctx, swapTCYMsg), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltETHTrading")

	// no halts
	c.Check(k.IsTradingHalt(ctx, swapMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, addMsg), Equals, false)
	c.Check(k.IsTradingHalt(ctx, withdrawMsg), Equals, false)

	// empty coins should not panic and should fall back to global trading halt check
	emptyCoinsSwap := &MsgSwap{Tx: common.Tx{Coins: common.Coins{}}, TargetAsset: common.ETHAsset}
	c.Check(k.IsTradingHalt(ctx, emptyCoinsSwap), Equals, false)
	k.SetMimir(ctx, "HaltTrading", 1)
	c.Check(k.IsTradingHalt(ctx, emptyCoinsSwap), Equals, true)
	_ = k.DeleteMimir(ctx, "HaltTrading")
}

func (s *KeeperHaltSuite) TestIsGlobalTradingHalted(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(10)

	// no halts
	c.Check(k.IsGlobalTradingHalted(ctx), Equals, false)

	// pending global trading halt
	k.SetMimir(ctx, "HaltTrading", 11)
	c.Check(k.IsGlobalTradingHalted(ctx), Equals, false)

	// current-block global trading halt
	k.SetMimir(ctx, "HaltTrading", 10)
	c.Check(k.IsGlobalTradingHalted(ctx), Equals, true)

	// current global trading halt
	k.SetMimir(ctx, "HaltTrading", 1)
	c.Check(k.IsGlobalTradingHalted(ctx), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltTrading")

	// no halts
	c.Check(k.IsGlobalTradingHalted(ctx), Equals, false)
}

func (s *KeeperHaltSuite) TestIsChainTradingHalted(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(10)

	// no halts
	c.Check(k.IsChainTradingHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainTradingHalted(ctx, common.ETHChain), Equals, false)

	// pending btc trading halt
	k.SetMimir(ctx, "HaltBTCTrading", 11)
	c.Check(k.IsChainTradingHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainTradingHalted(ctx, common.ETHChain), Equals, false)

	// current-block btc trading halt
	k.SetMimir(ctx, "HaltBTCTrading", 10)
	c.Check(k.IsChainTradingHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainTradingHalted(ctx, common.ETHChain), Equals, false)

	// current btc trading halt
	k.SetMimir(ctx, "HaltBTCTrading", 1)
	c.Check(k.IsChainTradingHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainTradingHalted(ctx, common.ETHChain), Equals, false)

	_ = k.DeleteMimir(ctx, "HaltBTCTrading")

	// current btc chain halt
	k.SetMimir(ctx, "HaltBTCChain", 1)
	c.Check(k.IsChainTradingHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainTradingHalted(ctx, common.ETHChain), Equals, false)

	_ = k.DeleteMimir(ctx, "HaltBTCChain")

	// no halts
	c.Check(k.IsChainTradingHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainTradingHalted(ctx, common.ETHChain), Equals, false)
}

func (s *KeeperHaltSuite) TestIsChainHalted(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(10)

	// no halts
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// pending global halt
	k.SetMimir(ctx, "HaltChainGlobal", 11)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// current-block global halt
	k.SetMimir(ctx, "HaltChainGlobal", 10)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, true)

	// current global halt
	k.SetMimir(ctx, "HaltChainGlobal", 1)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltChainGlobal")

	// expired node pause
	k.SetMimir(ctx, "NodePauseChainGlobal", 1)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// current-block node pause
	k.SetMimir(ctx, "NodePauseChainGlobal", 10)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, true)

	// current node pause
	k.SetMimir(ctx, "NodePauseChainGlobal", 11)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, true)

	_ = k.DeleteMimir(ctx, "NodePauseChainGlobal")

	// pending btc halt
	k.SetMimir(ctx, "HaltBTCChain", 11)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// current-block btc halt
	k.SetMimir(ctx, "HaltBTCChain", 10)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// current btc halt
	k.SetMimir(ctx, "HaltBTCChain", 1)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	_ = k.DeleteMimir(ctx, "HaltBTCChain")

	// pending btc solvency halt (though should never happen)
	k.SetMimir(ctx, "SolvencyHaltBTCChain", 11)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// current-block btc solvency halt
	k.SetMimir(ctx, "SolvencyHaltBTCChain", 10)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	// current btc solvency halt
	k.SetMimir(ctx, "SolvencyHaltBTCChain", 1)
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)

	_ = k.DeleteMimir(ctx, "SolvencyHaltBTCChain")

	// no halts
	c.Check(k.IsChainHalted(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsChainHalted(ctx, common.ETHChain), Equals, false)
}

func (s *KeeperHaltSuite) TestIsLPPaused(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(10)

	// no pauses
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, false)

	// pending btc pause (acts like halt)
	k.SetMimir(ctx, "PauseLPBTC", 11)
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, false)

	// current-block btc pause
	k.SetMimir(ctx, "PauseLPBTC", 10)
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, false)

	// current btc pause (acts like halt)
	k.SetMimir(ctx, "PauseLPBTC", 1)
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, false)

	_ = k.DeleteMimir(ctx, "PauseLPBTC")

	// pending global pause (acts like halt)
	k.SetMimir(ctx, "PauseLP", 11)
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, false)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, false)

	// current-block global pause
	k.SetMimir(ctx, "PauseLP", 10)
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, true)

	// current global pause (acts like halt)
	k.SetMimir(ctx, "PauseLP", 1)
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, true)
	c.Check(k.IsLPPaused(ctx, common.ETHChain), Equals, true)

	_ = k.DeleteMimir(ctx, "PauseLP")

	// no pauses
	c.Check(k.IsLPPaused(ctx, common.BTCChain), Equals, false)
}

func (s *KeeperHaltSuite) TestIsTCYTradingHalted(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(10)

	// no halts
	c.Check(k.IsTCYTradingHalted(ctx), Equals, false)

	// pending TCY trading halt
	k.SetMimir(ctx, "HaltTCYTrading", 11)
	c.Check(k.IsTCYTradingHalted(ctx), Equals, false)

	// current-block TCY trading halt
	k.SetMimir(ctx, "HaltTCYTrading", 10)
	c.Check(k.IsTCYTradingHalted(ctx), Equals, true)

	// current TCY trading halt
	k.SetMimir(ctx, "HaltTCYTrading", 1)
	c.Check(k.IsTCYTradingHalted(ctx), Equals, true)

	_ = k.DeleteMimir(ctx, "HaltTCYTrading")

	// no halts
	c.Check(k.IsTCYTradingHalted(ctx), Equals, false)
}

func (s *KeeperHaltSuite) TestIsPoolDepositPaused(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(10)

	// deposits are not paused
	c.Check(k.IsPoolDepositPaused(ctx, common.BTCAsset), Equals, false)
	c.Check(k.IsPoolDepositPaused(ctx, common.ETHAsset), Equals, false)

	// BTC is paused but ETH is not
	// XXX Should be replaced with SetMimirWithRef() when available a la MR 3561
	k.SetMimir(ctx, "PauseLPDeposit-BTC-BTC", 1)
	c.Check(k.IsPoolDepositPaused(ctx, common.BTCAsset), Equals, true)
	c.Check(k.IsPoolDepositPaused(ctx, common.ETHAsset), Equals, false)

	// XXX Should be replaced with SetMimirWithRef() when available a la MR 3561
	_ = k.DeleteMimir(ctx, "PauseLPDeposit-BTC-BTC")

	// back to normal
	c.Check(k.IsPoolDepositPaused(ctx, common.BTCAsset), Equals, false)
	c.Check(k.IsPoolDepositPaused(ctx, common.ETHAsset), Equals, false)
}
