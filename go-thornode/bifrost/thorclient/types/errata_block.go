package types

import "gitlab.com/thorchain/thornode/v3/common"

type ErrataBlock struct {
	Height int64
	Txs    []ErrataTx
}

type ErrataTx struct {
	TxID  common.TxID
	Chain common.Chain
}
