//go:build regtest
// +build regtest

package thornado

import (
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/common/cosmos"
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
