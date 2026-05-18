package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type BondMemo struct {
	MemoBase
	NodeAddress         cosmos.AccAddress
	BondProviderAddress cosmos.AccAddress
	NodeOperatorFee     int64
}

func (m BondMemo) GetAccAddress() cosmos.AccAddress { return m.NodeAddress }

func NewBondMemo(addr, additional cosmos.AccAddress, operatorFee int64) BondMemo {
	return BondMemo{
		MemoBase:            MemoBase{TxType: TxBond},
		NodeAddress:         addr,
		BondProviderAddress: additional,
		NodeOperatorFee:     operatorFee,
	}
}

func (p *parser) ParseBondMemo() (BondMemo, error) {
	addr := p.getAccAddress(1, true, nil)
	additional := p.getAccAddress(2, false, nil)
	operatorFee := p.getInt64(3, false, -1)
	return NewBondMemo(addr, additional, operatorFee), p.Error()
}
