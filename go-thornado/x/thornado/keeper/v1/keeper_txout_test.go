package keeperv1

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type KeeperTxOutSuite struct{}

var _ = Suite(&KeeperTxOutSuite{})

func (s *KeeperTxOutSuite) TestSetTxOutMarksCompleteWhenAllItemsHaveOutHash(c *C) {
	ctx, k := setupKeeperForTest(c)
	txOut := &TxOut{
		Height: 18,
		Status: TxOutStatusPendingSign,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				ToAddress:   GetRandomBTCAddress(),
				VaultPubKey: GetRandomPubKey(),
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000)),
				MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
				GasRate:     10,
				InHash:      GetRandomTxHash(),
				OutHash:     GetRandomTxHash(),
				TxType:      types.TxOutTypeOut,
			},
		},
	}

	c.Assert(k.SetTxOut(ctx, txOut), IsNil)
	stored, err := k.GetTxOut(ctx, txOut.Height)
	c.Assert(err, IsNil)
	c.Assert(stored.Status, Equals, TxOutStatusComplete)
	c.Assert(stored.TxArray[0].OutHash.IsEmpty(), Equals, false)
}
