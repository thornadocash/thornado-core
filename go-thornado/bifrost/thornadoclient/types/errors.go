package types

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
)

// MissingSourceTxError means Thornado asked Bifrost to spend an inbound tx that
// no longer exists as a spendable source on the external chain.
type MissingSourceTxError struct {
	TxID  common.TxID
	Chain common.Chain
	Err   error
}

func (e MissingSourceTxError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("missing source tx %s on %s", e.TxID, e.Chain)
	}
	return fmt.Sprintf("missing source tx %s on %s: %v", e.TxID, e.Chain, e.Err)
}

func (e MissingSourceTxError) Unwrap() error {
	return e.Err
}
