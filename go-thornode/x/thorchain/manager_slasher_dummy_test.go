package thorchain

import (
	"github.com/cometbft/cometbft/crypto"
	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type DummySlasher struct {
	pts map[string]int64
}

func NewDummySlasher() *DummySlasher {
	return &DummySlasher{
		pts: make(map[string]int64),
	}
}

func (d DummySlasher) BeginBlock(ctx cosmos.Context, constAccessor constants.ConstantValues) {
}

func (d DummySlasher) HandleDoubleSign(ctx cosmos.Context, addr crypto.Address, infractionHeight int64, constAccessor constants.ConstantValues) error {
	return errKaboom
}

func (d DummySlasher) LackSigning(ctx cosmos.Context, mgr Manager) error {
	return errKaboom
}

func (d DummySlasher) SlashVault(ctx cosmos.Context, vaultPK common.PubKey, coins common.Coins, mgr Manager) error {
	return errKaboom
}

func (d DummySlasher) IncSlashPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress) {
	for _, addr := range addresses {
		found := false
		for k := range d.pts {
			if k == addr.String() {
				d.pts[k] += point
				found = true
				break
			}
		}
		if !found {
			d.pts[addr.String()] = point
		}
	}
}

func (d DummySlasher) DecSlashPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress) {
	for _, addr := range addresses {
		found := false
		for k := range d.pts {
			if k == addr.String() {
				d.pts[k] -= point
				found = true
				break
			}
		}
		if !found {
			d.pts[addr.String()] = -point
		}
	}
}
