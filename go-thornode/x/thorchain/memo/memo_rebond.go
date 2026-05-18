package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type RebondMemo struct {
	MemoBase
	NodeAddress            cosmos.AccAddress
	NewBondProviderAddress cosmos.AccAddress
	Amount                 cosmos.Uint
}

func (m RebondMemo) GetNodeAddress() cosmos.AccAddress {
	return m.NodeAddress
}

func (m RebondMemo) GetNewProviderAddress() cosmos.AccAddress {
	return m.NewBondProviderAddress
}

func (m RebondMemo) GetAmount() cosmos.Uint {
	return m.Amount
}

func NewRebondMemo(
	nodeAddress, newProvider cosmos.AccAddress,
	amount cosmos.Uint,
) RebondMemo {
	return RebondMemo{
		MemoBase:               MemoBase{TxType: TxRebond},
		NodeAddress:            nodeAddress,
		NewBondProviderAddress: newProvider,
		Amount:                 amount,
	}
}

func (p *parser) ParseRebondMemo() (RebondMemo, error) {
	nodeAddress := p.getAccAddress(1, true, nil)
	newProvider := p.getAccAddress(2, true, nil)
	amount := p.getUint(3, false, 0)
	return NewRebondMemo(nodeAddress, newProvider, amount), p.Error()
}
