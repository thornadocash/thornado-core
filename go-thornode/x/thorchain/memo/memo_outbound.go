package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
)

type OutboundMemo struct {
	MemoBase
	TxID common.TxID
}

func (m OutboundMemo) GetTxID() common.TxID { return m.TxID }
func (m OutboundMemo) String() string {
	return fmt.Sprintf("OUT:%s", m.TxID.String())
}

func NewOutboundMemo(txID common.TxID) OutboundMemo {
	return OutboundMemo{
		MemoBase: MemoBase{TxType: TxOutbound},
		TxID:     txID,
	}
}

func (p *parser) ParseOutboundMemo() (OutboundMemo, error) {
	txID := p.getTxID(1, true, common.BlankTxID)
	return NewOutboundMemo(txID), p.Error()
}
