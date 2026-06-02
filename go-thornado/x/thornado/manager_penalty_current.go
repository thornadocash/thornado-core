package thornado

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// PenaltyManagerImpl tracks node penalty points. Thornado does not financially penalize
// vault members for BTC vault accounting failures.
type PenaltyManagerImpl struct {
	keeper keeper.Keeper
}

func newPenaltyManager(keeper keeper.Keeper, _ EventManager) *PenaltyManagerImpl {
	return &PenaltyManagerImpl{keeper: keeper}
}

func (s *PenaltyManagerImpl) IncPenaltyPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress) {
	for _, addr := range addresses {
		if err := s.keeper.IncNodeAccountPenaltyPoints(ctx, addr, point); err != nil {
			ctx.Logger().Error("fail to increase node penalty points", "error", err)
		}
	}
}

func (s *PenaltyManagerImpl) DecPenaltyPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress) {
	for _, addr := range addresses {
		if err := s.keeper.DecNodeAccountPenaltyPoints(ctx, addr, point); err != nil {
			ctx.Logger().Error("fail to decrease node penalty points", "error", err)
		}
	}
}
