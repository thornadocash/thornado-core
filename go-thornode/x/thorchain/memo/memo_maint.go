package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// MaintMemo is used to toggle a node's maintenance flag
type MaintMemo struct {
	MemoBase
	NodeAddress cosmos.AccAddress
}

// NewMaintMemo creates a new MaintMemo with the given node address
func NewMaintMemo(addr cosmos.AccAddress) MaintMemo {
	return MaintMemo{
		MemoBase:    MemoBase{TxType: TxMaint},
		NodeAddress: addr,
	}
}

// GetAccAddress returns the node address from the memo
func (m MaintMemo) GetAccAddress() cosmos.AccAddress { return m.NodeAddress }

// String returns the string representation of the memo
func (m MaintMemo) String() string {
	return fmt.Sprintf("maint:%s", m.NodeAddress.String())
}

// ParseMaintMemo parses the given memo string into a MaintMemo
func (p *parser) ParseMaintMemo() (MaintMemo, error) {
	addr := p.getAccAddress(1, true, nil)
	return NewMaintMemo(addr), p.Error()
}
