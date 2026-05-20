package thorchain

import (
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

const (
	MimirRecallFund      = `MimirRecallFund`
	MimirUpgradeContract = `MimirUpgradeContract`

	MimirRecallFundTemplate      = `MimirRecallFund%s`
	MimirUpgradeContractTemplate = `MimirUpgradeContract%s`
)

type RouterUpgradeController struct {
	mgr Manager
}

// NewRouterUpgradeController create a new instance of RouterUpgradeController
func NewRouterUpgradeController(mgr Manager) *RouterUpgradeController {
	return &RouterUpgradeController{
		mgr: mgr,
	}
}

// getChainOldAndNewRouters returns the old a new router addresses
func (r *RouterUpgradeController) getChainOldAndNewRouters(chain common.Chain) (string, string, error) {
	return "", "", fmt.Errorf("router upgrades are disabled for BTC-only Thornado: %s", chain)
}

// getRouterChains gets the chains that have routers for the current version
func (r *RouterUpgradeController) getRouterChains(version semver.Version) ([]common.Chain, error) {
	return nil, nil
}

// upgradeContract updates a chain's router in the KVStore if needed
func (r *RouterUpgradeController) upgradeContract(ctx cosmos.Context, version semver.Version) error {
	chains, err := r.getRouterChains(version)
	if err != nil {
		return fmt.Errorf("fail to get router chains: %w", err)
	}

	// Iterate through all the chains with routers, see if any need their contracts updated
	for _, chain := range chains {
		mimirKey := fmt.Sprintf(MimirUpgradeContractTemplate, chain)
		mimirValue, err := r.mgr.Keeper().GetMimir(ctx, mimirKey)
		if err != nil {
			ctx.Logger().Error("fail to get router upgrade mimir", "chain", chain.String(), "error", err)
			continue
		}

		if mimirValue <= 0 {
			// mimir not set, skip
			continue
		}

		oldRouter, newRouter, err := r.getChainOldAndNewRouters(chain)
		if err != nil {
			ctx.Logger().Error("fail to get old and new router", "chain", chain.String(), "error", err)
			continue
		}

		currentChainContract, err := r.mgr.Keeper().GetChainContract(ctx, chain)
		if err != nil {
			ctx.Logger().Error("fail to get existing contract", "chain", chain.String(), "error", err)
			continue
		}

		// old router should be current router
		if !strings.EqualFold(currentChainContract.Router.String(), oldRouter) {
			ctx.Logger().Error("old router not current router", "chain", chain.String())
			continue
		}

		// new router should not be current router
		if strings.EqualFold(currentChainContract.Router.String(), newRouter) {
			ctx.Logger().Info("new router already set", "chain", chain.String())
			continue
		}

		// Update ChainContract
		// TODO: make this non-EVM agnostic (should not need to be an address)
		newRouterAddr, err := common.NewAddress(newRouter)
		if err != nil {
			ctx.Logger().Error("fail to parse new contract address", "chain", chain.String(), "addr", newRouter, "error", err)
			continue
		}
		newChainContract := ChainContract{
			Chain:  chain,
			Router: newRouterAddr,
		}
		r.mgr.Keeper().SetChainContract(ctx, newChainContract)

		// Unset upgrade router mimir
		err = r.mgr.Keeper().DeleteMimir(ctx, mimirKey)
		if err != nil {
			ctx.Logger().Debug("fail to unset router upgrade mimir", "chain", chain.String(), "error", err)
		}
	}

	return nil
}

// Process is the main entry of router upgrade controller
// refunds all USDT liquidity, and then upgrades contract
// all these steps are controlled by mimir
func (r *RouterUpgradeController) Process(ctx cosmos.Context) {
	version := r.mgr.GetVersion()

	if err := r.upgradeContract(ctx, version); err != nil {
		ctx.Logger().Error("fail to upgrade contract", "error", err)
	}
}
