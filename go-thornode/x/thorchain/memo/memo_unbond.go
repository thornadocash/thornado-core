package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type UnbondMemo struct {
	MemoBase
	NodeAddress         cosmos.AccAddress
	Amount              cosmos.Uint
	BondProviderAddress cosmos.AccAddress
}

func (m UnbondMemo) GetAccAddress() cosmos.AccAddress { return m.NodeAddress }
func (m UnbondMemo) GetAmount() cosmos.Uint           { return m.Amount }

func NewUnbondMemo(addr, additional cosmos.AccAddress, amt cosmos.Uint) UnbondMemo {
	return UnbondMemo{
		MemoBase:            MemoBase{TxType: TxUnbond},
		NodeAddress:         addr,
		Amount:              amt,
		BondProviderAddress: additional,
	}
}

func (p *parser) ParseUnbondMemo() (UnbondMemo, error) {
	addr := p.getAccAddress(1, true, nil)
	amt := p.getUint(2, true, 0)
	additional := p.getAccAddress(3, false, nil)
	return NewUnbondMemo(addr, additional, amt), p.Error()
}
