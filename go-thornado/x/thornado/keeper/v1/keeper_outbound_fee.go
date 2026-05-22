package keeperv1

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) GetOutboundTxFee(ctx cosmos.Context) cosmos.Uint {
	return cosmos.ZeroUint()
}
