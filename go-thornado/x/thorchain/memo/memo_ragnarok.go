package thorchain

import (
	"fmt"
)

type RagnarokMemo struct {
	MemoBase
	BlockHeight int64
}

func (m RagnarokMemo) String() string {
	return fmt.Sprintf("RAGNAROK:%d", m.BlockHeight)
}

func (m RagnarokMemo) GetBlockHeight() int64 {
	return m.BlockHeight
}

func NewRagnarokMemo(blockHeight int64) RagnarokMemo {
	return RagnarokMemo{
		MemoBase:    MemoBase{TxType: TxRagnarok},
		BlockHeight: blockHeight,
	}
}

func (p *parser) ParseRagnarokMemo() (RagnarokMemo, error) {
	blockHeight := p.getInt64(1, true, 0)
	err := p.Error()
	return NewRagnarokMemo(blockHeight), err
}
