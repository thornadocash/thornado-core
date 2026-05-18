package types

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MsgOutboundTxSuite struct{}

var _ = Suite(&MsgOutboundTxSuite{})

func (MsgOutboundTxSuite) TestMsgOutboundTx(c *C) {
	txID := GetRandomTxHash()
	inTxID := GetRandomTxHash()
	eth := GetRandomETHAddress()
	acc1 := GetRandomBech32Addr()
	tx := common.NewObservedTx(common.NewTx(
		txID,
		eth,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, cosmos.OneUint())},
		common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One))},
		"",
	), 12, GetRandomPubKey(), 12)
	m := NewMsgOutboundTx(tx, inTxID, acc1)
	EnsureMsgBasicCorrect(m, c)

	inputs := []struct {
		txID   common.TxID
		inTxID common.TxID
		sender common.Address
		signer cosmos.AccAddress
	}{
		{
			txID:   common.TxID(""),
			inTxID: inTxID,
			sender: eth,
			signer: acc1,
		},
		{
			txID:   txID,
			inTxID: common.TxID(""),
			sender: eth,
			signer: acc1,
		},
		{
			txID:   txID,
			inTxID: inTxID,
			sender: common.NoAddress,
			signer: acc1,
		},
		{
			txID:   txID,
			inTxID: inTxID,
			sender: eth,
			signer: cosmos.AccAddress{},
		},
	}

	for _, item := range inputs {
		tx = common.NewObservedTx(common.NewTx(
			item.txID,
			item.sender,
			GetRandomETHAddress(),
			common.Coins{common.NewCoin(common.ETHAsset, cosmos.OneUint())},
			common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One))},
			"",
		), 12, GetRandomPubKey(), 12)
		m = NewMsgOutboundTx(tx, item.inTxID, item.signer)
		err := m.ValidateBasic()
		c.Assert(err, NotNil, Commentf("%s", err.Error()))
	}
}
