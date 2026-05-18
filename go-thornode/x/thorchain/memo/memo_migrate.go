package thorchain

import (
	"fmt"
)

type MigrateMemo struct {
	MemoBase
	BlockHeight int64
}

func (m MigrateMemo) String() string {
	return fmt.Sprintf("MIGRATE:%d", m.BlockHeight)
}

func (m MigrateMemo) GetBlockHeight() int64 {
	return m.BlockHeight
}

func NewMigrateMemo(blockHeight int64) MigrateMemo {
	return MigrateMemo{
		MemoBase:    MemoBase{TxType: TxMigrate},
		BlockHeight: blockHeight,
	}
}

func (p *parser) ParseMigrateMemo() (MigrateMemo, error) {
	blockHeight := p.getInt64(1, true, 0)
	return NewMigrateMemo(blockHeight), p.Error()
}
