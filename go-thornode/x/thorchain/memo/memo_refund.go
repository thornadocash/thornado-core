package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
)

type RefundMemo struct {
	MemoBase
	TxID common.TxID
}

func (m RefundMemo) GetTxID() common.TxID { return m.TxID }

// String implement fmt.Stringer
func (m RefundMemo) String() string {
	return fmt.Sprintf("REFUND:%s", m.TxID.String())
}

// NewRefundMemo create a new RefundMemo
func NewRefundMemo(txID common.TxID) RefundMemo {
	return RefundMemo{
		MemoBase: MemoBase{TxType: TxRefund},
		TxID:     txID,
	}
}

func (p *parser) ParseRefundMemo() (RefundMemo, error) {
	txID := p.getTxID(1, true, common.BlankTxID)
	return NewRefundMemo(txID), p.Error()
}
