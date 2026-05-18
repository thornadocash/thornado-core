//go:build regtest
// +build regtest

package thorchain

import (
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func init() {
	initManager = func(ctx cosmos.Context, mgr *Mgrs) {
		_ = mgr.LoadManagerIfNecessary(ctx)
	}

	queryExport = func(ctx sdk.Context, mgr *Mgrs) ([]byte, error) {
		contentBz := ExportGenesis(ctx, mgr.Keeper())
		res, err := json.Marshal(contentBz)
		if err != nil {
			return nil, fmt.Errorf("fail to marshal response to json: %w", err)
		}
		return res, nil
	}
}
