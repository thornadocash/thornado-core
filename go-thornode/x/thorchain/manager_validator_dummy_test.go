package thorchain

import (
	abci "github.com/cometbft/cometbft/abci/types"

	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type ValidatorDummyMgr struct{}

// NewValidatorDummyMgr create a new instance of ValidatorDummyMgr
func NewValidatorDummyMgr() *ValidatorDummyMgr {
	return &ValidatorDummyMgr{}
}

func (vm *ValidatorDummyMgr) BeginBlock(_ cosmos.Context, _ Manager, _ []string) error {
	return errKaboom
}

func (vm *ValidatorDummyMgr) EndBlock(_ cosmos.Context, _ Manager) []abci.ValidatorUpdate {
	return nil
}

func (vm *ValidatorDummyMgr) processRagnarok(_ cosmos.Context, _ Manager) error {
	return errKaboom
}

func (vm *ValidatorDummyMgr) NodeAccountPreflightCheck(ctx cosmos.Context, na NodeAccount, constAccessor constants.ConstantValues) (NodeStatus, error) {
	return NodeDisabled, errKaboom
}
