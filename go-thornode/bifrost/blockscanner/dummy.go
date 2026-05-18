package blockscanner

import "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"

type DummyFetcher struct {
	Tx  types.TxIn
	Err error
}

func (d DummyFetcher) FetchMemPool(height int64) (types.TxIn, error) {
	return d.Tx, d.Err
}

func (d DummyFetcher) FetchTxs(height, _ int64) (types.TxIn, error) {
	return d.Tx, d.Err
}

func (d DummyFetcher) GetHeight() (int64, error) {
	return 0, nil
}

func (d DummyFetcher) GetNetworkFee() (transactionSize, transactionFeeRate uint64) {
	return 0, 0
}
