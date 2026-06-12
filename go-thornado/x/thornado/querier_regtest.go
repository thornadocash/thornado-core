//go:build regtest
// +build regtest

package thornado

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func init() {
	initManager = func(ctx cosmos.Context, mgr *Mgrs) {
		_ = mgr.LoadManagerIfNecessary(ctx)
	}

}
