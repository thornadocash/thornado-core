package thorchain

import (
	cosmos "github.com/thornadocash/go-thornado/common/cosmos"
)

type OperatorRotateMemo struct {
	MemoBase
	OperatorAddress cosmos.AccAddress
}

func NewOperatorRotateMemo(operatorAddr cosmos.AccAddress) OperatorRotateMemo {
	return OperatorRotateMemo{
		MemoBase:        MemoBase{TxType: TxOperatorRotate},
		OperatorAddress: operatorAddr,
	}
}

func (p *parser) ParseOperatorRotate() (OperatorRotateMemo, error) {
	operatorAddr := p.getAccAddress(1, true, nil)
	return NewOperatorRotateMemo(operatorAddr), p.Error()
}
