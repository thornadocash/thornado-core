//go:build !testnet && !mocknet
// +build !testnet,!mocknet

package utxo

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	ttypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func (s *ZcashSuite) TestGetAddress(c *C) {
	pubkey := common.PubKey("thorpub1addwnpepqt7qug8vk9r3saw8n4r803ydj2g3dqwx0mvq5akhnze86fc536xcy2cr8a2")
	addr := s.client.GetAddress(pubkey)
	c.Assert(addr, Equals, "t1RMxNfN8Q7iwZw7UJB4CqzatxsBmGeYMe2")
}

func (s *ZcashSuite) TestConfirmationCountReady(c *C) {
	testCases := []struct {
		Height      int64
		Mempool     bool
		Required    int64
		AssertReady bool
	}{
		{
			Height:      4,
			Mempool:     true,
			Required:    9,
			AssertReady: true,
		},
		{
			Height:      1,
			Mempool:     false,
			Required:    0,
			AssertReady: true,
		},
		{
			Height:      1,
			Required:    1,
			AssertReady: true,
		},
		{
			Height:      2,
			Required:    1,
			AssertReady: false,
		},
		{
			Height:      1,
			Required:    5,
			AssertReady: false,
		},
	}

	s.client.currentBlockHeight.Store(2)

	// empty TxArray
	txIn := types.TxIn{
		Chain:    common.ZECChain,
		TxArray:  nil,
		Filtered: true,
		MemPool:  false,
	}
	c.Assert(s.client.ConfirmationCountReady(txIn), Equals, true)

	for _, tc := range testCases {
		pkey := ttypes.GetRandomPubKey()
		txIn := types.TxIn{
			Chain: common.ZECChain,
			TxArray: []*types.TxInItem{
				{
					BlockHeight: tc.Height,
					Tx:          "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
					Sender:      "t1KstPVzcNEK4ZeZeBh3GenuvfAJ6qfhVZP",
					To:          "t1RMxNfN8Q7iwZw7UJB4CqzatxsBmGeYMe2",
					Coins: common.Coins{
						common.NewCoin(common.ZECAsset, cosmos.NewUint(100_000)),
					},
					Memo:                "MEMO",
					ObservedVaultPubKey: pkey,
				},
			},
			Filtered:             true,
			MemPool:              tc.Mempool,
			ConfirmationRequired: tc.Required,
		}

		c.Assert(s.client.ConfirmationCountReady(txIn), Equals, tc.AssertReady, Commentf("%+v", tc))

	}
}

func (s *ZcashSuite) TestConfirmationCount(c *C) {
	testCases := []struct {
		Amount      uint64
		Mempool     bool
		Required    int64
		AssertCount int64
	}{
		{
			Amount:      100_000,
			Mempool:     true,
			Required:    9,
			AssertCount: 0,
		},
		{
			Amount:      100_000,
			Mempool:     false,
			Required:    0,
			AssertCount: 0,
		},
		{
			Amount:      100_000,
			Mempool:     false,
			Required:    5,
			AssertCount: 0,
		},
		{
			// 10 ZEC, big amount requires more confirmations.
			Amount:      1_000_000_000,
			Mempool:     false,
			Required:    0,
			AssertCount: 7,
		},
	}

	s.client.currentBlockHeight.Store(1)

	// empty TxArray
	txIn := types.TxIn{
		Chain:    common.ZECChain,
		TxArray:  nil,
		Filtered: true,
		MemPool:  false,
	}
	c.Assert(s.client.GetConfirmationCount(txIn), Equals, int64(0))

	for _, tc := range testCases {
		pkey := ttypes.GetRandomPubKey()
		txIn := types.TxIn{
			Chain: common.ZECChain,
			TxArray: []*types.TxInItem{
				{
					BlockHeight: 1,
					Tx:          "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
					Sender:      "t1KstPVzcNEK4ZeZeBh3GenuvfAJ6qfhVZP",
					To:          "t1RMxNfN8Q7iwZw7UJB4CqzatxsBmGeYMe2",
					Coins: common.Coins{
						common.NewCoin(common.ZECAsset, cosmos.NewUint(tc.Amount)),
					},
					Memo:                "MEMO",
					ObservedVaultPubKey: pkey,
				},
			},
			Filtered:             true,
			MemPool:              tc.Mempool,
			ConfirmationRequired: tc.Required,
		}

		c.Assert(s.client.GetConfirmationCount(txIn), Equals, tc.AssertCount, Commentf("%+v", tc))
	}
}
