package types

import "github.com/thornadocash/go-thornado/common"

type ErrataBlock struct {
	Height int64
	Txs    []ErrataTx
}

type ErrataTx struct {
	TxID  common.TxID
	Chain common.Chain
}
