package thornado

import (
	"errors"
	"fmt"

	math "cosmossdk.io/math"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// NetworkMgr is going to manage the vaults
type NetworkMgr struct {
	k          keeper.Keeper
	txOutStore TxOutStore
	eventMgr   EventManager
}

// assetAmount represents an asset with a signed amount (can be positive or negative)
type assetAmount struct {
	Asset  common.Asset
	Amount math.Int
}

// newNetworkMgr create a new vault manager
func newNetworkMgr(k keeper.Keeper, txOutStore TxOutStore, eventMgr EventManager) *NetworkMgr {
	return &NetworkMgr{
		k:          k,
		txOutStore: txOutStore,
		eventMgr:   eventMgr,
	}
}

func (vm *NetworkMgr) processGenesisSetup(ctx cosmos.Context) error {
	if ctx.BlockHeight() != genesisBlockHeight {
		return nil
	}
	vaults, err := vm.k.GetAsgardVaults(ctx)
	if err != nil {
		return fmt.Errorf("fail to get vaults: %w", err)
	}
	if len(vaults) > 0 {
		ctx.Logger().Info("already have vault, no need to generate at genesis")
		return nil
	}
	active, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return fmt.Errorf("fail to get all active node accounts")
	}
	if len(active) == 0 {
		return errors.New("no active accounts,cannot proceed")
	}
	if len(active) == 1 {
		supportChains := common.Chains{
			common.Thornado,
			common.BTCChain,
		}
		pubSet := active[0].PubKeySet
		vault := NewVaultV2(0, ActiveVault, AsgardVault, pubSet.Secp256k1, supportChains.Strings(), vm.k.GetChainContracts(ctx, supportChains), pubSet.Ed25519)
		vault.Membership = common.PubKeys{pubSet.Secp256k1}.Strings()
		if err := vm.k.SetVault(ctx, vault); err != nil {
			return fmt.Errorf("fail to save vault: %w", err)
		}
	} else {
		// Trigger a keygen ceremony
		err := vm.TriggerKeygen(ctx, active)
		if err != nil {
			return fmt.Errorf("fail to trigger a keygen: %w", err)
		}
	}
	return nil
}

func (vm *NetworkMgr) BeginBlock(ctx cosmos.Context, mgr Manager) error {
	return vm.spawnDerivedAssets(ctx, mgr)
}

func (vm *NetworkMgr) suspendVirtualPool(ctx cosmos.Context, mgr Manager, derivedAsset common.Asset, suspendReasonErr error) {
}

func (vm *NetworkMgr) GetAvailableAnchorsAndDepths(
	ctx cosmos.Context,
	mgr Manager,
	asset common.Asset,
) ([]common.Asset, []cosmos.Uint, error) {
	return nil, nil, nil
}

func (vm *NetworkMgr) CalcAnchor(ctx cosmos.Context, mgr Manager, asset common.Asset) (cosmos.Uint, cosmos.Uint, cosmos.Uint) {
	return cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint()
}

func (vm *NetworkMgr) spawnDerivedAssets(ctx cosmos.Context, mgr Manager) error {
	return nil
}

func (vm *NetworkMgr) SpawnDerivedAsset(ctx cosmos.Context, asset common.Asset, mgr Manager) {
}

func (vm *NetworkMgr) fetchWeightedMeanSlip(ctx cosmos.Context, asset common.Asset, mgr Manager) (slip int64) {
	return 0
}

func (vm *NetworkMgr) calculateWeightedMeanSlip(ctx cosmos.Context, asset common.Asset, mgr Manager) int64 {
	return 0
}

// EndBlock move funds from retiring asgard vaults
func (vm *NetworkMgr) EndBlock(ctx cosmos.Context, mgr Manager) error {
	if ctx.BlockHeight() == genesisBlockHeight {
		return vm.processGenesisSetup(ctx)
	}
	controller := NewRouterUpgradeController(mgr)
	controller.Process(ctx)

	if err := vm.migrateFunds(ctx, mgr); err != nil {
		ctx.Logger().Error("fail to migrate funds", "error", err)
	}

	if err := vm.consolidateActiveBTCVaults(ctx, mgr); err != nil {
		ctx.Logger().Error("fail to schedule bitcoin vault consolidation", "error", err)
	}

	return nil
}

func (vm *NetworkMgr) consolidateActiveBTCVaults(ctx cosmos.Context, mgr Manager) error {
	vaults, err := vm.k.GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}

	threshold, err := vm.k.GetMimir(ctx, "MaxUTXOsToSpend")
	if err != nil || threshold <= 0 {
		threshold = 15
	}
	if threshold < 2 {
		threshold = 2
	}

	for _, vault := range vaults {
		if vault.InboundTxCount < threshold {
			continue
		}
		if vaultHasPendingTxType(ctx, mgr.Keeper(), vault.PubKey, types.TxOutTypeConsolidate) {
			continue
		}
		coin := vault.GetCoin(common.BTCAsset)
		if coin.IsEmpty() || coin.Amount.IsZero() {
			continue
		}
		maxGasCoin, err := mgr.GasMgr().GetMaxGas(ctx, common.BTCChain)
		if err != nil {
			return fmt.Errorf("fail to get bitcoin consolidate max gas: %w", err)
		}
		if coin.Amount.LTE(maxGasCoin.Amount) {
			continue
		}
		rootAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
		if err != nil {
			return err
		}
		gasRate := int64(1)
		if nf, err := vm.k.GetNetworkFee(ctx, common.BTCChain); err == nil && nf.TransactionFeeRate > 0 {
			gasRate = int64(nf.TransactionFeeRate)
		}
		item := TxOutItem{
			Chain:            common.BTCChain,
			ToAddress:        rootAddr,
			VaultPubKey:      vault.PubKey,
			VaultPubKeyEddsa: vault.PubKeyEddsa,
			Coin:             common.NewCoin(common.BTCAsset, coin.Amount.Sub(maxGasCoin.Amount)),
			MaxGas:           common.Gas{maxGasCoin},
			GasRate:          gasRate,
			InHash:           common.BlankTxID,
			ModuleName:       AsgardName,
			VaultPathIndex:   common.MainVaultPathIndex,
			TxType:           types.TxOutTypeConsolidate,
		}
		if err := mgr.TxOutStore().UnSafeAddTxOutItem(ctx, mgr, item, ctx.BlockHeight()); err != nil {
			return fmt.Errorf("fail to add bitcoin consolidate txout: %w", err)
		}
		vault.InboundTxCount = 0
		if err := vm.k.SetVault(ctx, vault); err != nil {
			return fmt.Errorf("fail to reset vault inbound tx count: %w", err)
		}
	}
	return nil
}

func vaultHasPendingTxType(ctx cosmos.Context, k keeper.Keeper, pubkey common.PubKey, txType string) bool {
	signingPeriod := k.GetConstants().GetInt64Value(constants.SigningTransactionPeriod)
	startHeight := ctx.BlockHeight() - signingPeriod
	if startHeight < 1 {
		startHeight = 1
	}
	for height := startHeight; height <= ctx.BlockHeight()+signingPeriod; height++ {
		blockOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			continue
		}
		for _, item := range blockOut.TxArray {
			if item.OutHash.IsEmpty() && item.VaultPubKey.Equals(pubkey) && item.GetTxType() == txType {
				return true
			}
		}
	}
	return false
}

func (vm *NetworkMgr) migrateFunds(ctx cosmos.Context, mgr Manager) error {
	const migrateInterval = int64(360)

	retiring, err := vm.k.GetAsgardVaultsByStatus(ctx, RetiringVault)
	if err != nil {
		return err
	}

	active, err := vm.k.GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}

	// if we have no active asgards to move funds to, don't move funds
	if len(active) == 0 {
		return nil
	}
	for _, av := range active {
		if av.Routers != nil {
			continue
		}
		av.Routers = vm.k.GetChainContracts(ctx, av.GetChains())
		if err = vm.k.SetVault(ctx, av); err != nil {
			ctx.Logger().Error("fail to update chain contract", "error", err)
		}
	}

	// if we have no retiring asgards to move funds from, don't do anything further
	if len(retiring) == 0 {
		return nil
	}

	vaultsAvailableCoins := map[common.PubKey]common.Coins{}
	for _, vault := range retiring {
		if vault.LenPendingTxBlockHeights(ctx.BlockHeight(), mgr.GetConstants().GetInt64Value(constants.SigningTransactionPeriod)) > 0 {
			ctx.Logger().Info("Skipping the migration of funds while transactions are still pending")
			// This refers to migrate TxOutItems only.
			return nil
		}

		// Copy the RetiringVault Coins for deduction.
		vaultsAvailableCoins[vault.PubKey] = common.NewCoins(vault.Coins...)
	}

	const migrationRounds = int64(2)

	signingTransactionPeriod := mgr.GetConstants().GetInt64Value(constants.SigningTransactionPeriod)
	startHeight := ctx.BlockHeight() - signingTransactionPeriod
	if startHeight < 1 {
		startHeight = 1
	}
	for height := startHeight; height <= ctx.BlockHeight()+signingTransactionPeriod; height++ {
		var blockOut *TxOut
		blockOut, err = mgr.Keeper().GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("fail to get block tx out", "error", err)
			continue
		}
		for _, toi := range blockOut.TxArray {
			// only still outstanding txout will be considered
			if !toi.OutHash.IsEmpty() {
				continue
			}
			availableCoins, ok := vaultsAvailableCoins[toi.VaultPubKey]
			if !ok {
				// This isn't one of the RetiringVaults.
				continue
			}
			// Deduct from the available Coins all pending outbounds and their MaxGas.
			for _, coin := range append(common.Coins{toi.Coin}, toi.MaxGas...) {
				availableCoins = availableCoins.SafeSub(coin)
			}
			// Having deducted from the Coins, ensure the map reflects the new amounts.
			vaultsAvailableCoins[toi.VaultPubKey] = availableCoins
		}
	}

	for _, vault := range retiring {
		if !vault.HasFunds() {
			vault.UpdateStatus(InactiveVault, ctx.BlockHeight())
			if err = vm.k.SetVault(ctx, vault); err != nil {
				ctx.Logger().Error("fail to set vault to inactive", "error", err)
			}
			continue
		}

		availableCoins, vaultsAvailableCoinOk := vaultsAvailableCoins[vault.PubKey]
		if !vaultsAvailableCoinOk {
			// This should never happen.
			ctx.Logger().Error("RetiringVault Coins not found in map", "vault_pubkey", vault.PubKey)
			continue
		}

		// move partial funds every 30 minutes
		if (ctx.BlockHeight()-vault.StatusSince)%migrateInterval == 0 {
			for _, coin := range availableCoins {
				// non-native rune assets are no migrated, therefore they are
				// burned in each churn
				if coin.IsNative() {
					continue
				}
				if coin.Amount.Equal(cosmos.ZeroUint()) {
					continue
				}

				targetVaults := active

				// Only prioritise migration to unreceived ActiveVaults for gas assets.
				if coin.Asset.IsGasAsset() {
					var filteredVaults Vaults
					for _, activeVault := range active {
						// Do not use HasAsset function so as to use zero-amount Coins to mark scheduled migrations,
						// without double-counting outbound item migration amounts.
						hasAsset := false
						for _, activeVaultCoin := range activeVault.Coins {
							if activeVaultCoin.Asset.Equals(coin.Asset) {
								hasAsset = true
								break
							}
						}
						// If there are vaults that has never received (or in this block had a migration scheduled for)
						// this Asset, prioritise them.
						if !hasAsset {
							filteredVaults = append(filteredVaults, activeVault)
						}
					}
					if len(filteredVaults) != 0 {
						targetVaults = filteredVaults
					}
				}

				// GetMostSecure also takes into account migration outbound items.
				target := vm.k.GetMostSecure(ctx, targetVaults, signingTransactionPeriod)
				// get address of asgard pubkey
				var addr common.Address
				addr, err = target.GetAddress(coin.Asset.GetChain())
				if err != nil {
					return err
				}

				// get index of target vault in active slice
				targetVaultIndex := -1
				for i, activeVault := range active {
					if target.PubKey.Equals(activeVault.PubKey) {
						targetVaultIndex = i
						break
					}
				}
				if targetVaultIndex == -1 {
					ctx.Logger().Error("fail to identify active vault", "pubkey", target.PubKey)
					continue
				}

				// figure the nth time, we've sent migration txs from this vault
				nth := (ctx.BlockHeight()-vault.StatusSince)/migrateInterval + 1

				// for the last migration round, only migrate the final amount
				// of non-gas assets. For the last migration round + 1, then
				// transfer all of the remaining gas assets. This was added
				// because of a rare condition where during the last migration
				// round one of the txns failed (ie stuck txn) but the other
				// did not (ie gas asset). This left the vault with some
				// non-gas asset but no gas asset to transfer them, hence
				// getting churn into a stuck position until someone donated
				// ETH to resolve it.
				// Here we await for all non-gas assets to have left the vault
				// before we transfer the remaining gas asset to stop this
				// scenario from happening
				if nth >= migrationRounds && vault.CoinLengthByChain(coin.Asset.GetChain()) > 1 && coin.Asset.IsGasAsset() {
					continue
				}

				// Default amount set to total remaining amount. Relies on the
				// signer, to successfully send these funds while respecting
				// gas requirements (so it'll actually send slightly less)
				amt := coin.Amount
				if nth < migrationRounds { // migrate partial funds prior to the final round
					// each round of migration, about the same amount is sent.  For example, if 5 rounds:
					// Round 1 = 1/5 ( 20% of current, 20% of start)
					// Round 2 = 1/4 ( 25% of current, 20% of start)
					// Round 3 = 1/3 ( 33% of current, 20% of start)
					// Round 4 = 1/2 ( 50% of current, 20% of start)
					// Round 5 = 1/1 (100% of current, 20% of start)
					amt = amt.QuoUint64(uint64(1 + migrationRounds - nth)) // as nth < migrationRounds, the denominator is never zero
				}
				amt = cosmos.RoundToDecimal(amt, coin.Decimals)

				chain := coin.Asset.GetChain()

				// minus gas costs for our transactions
				gasAsset := chain.GetGasAsset()
				if coin.Asset.Equals(gasAsset) {
					gasMgr := mgr.GasMgr()
					gas, err := gasMgr.GetMaxGas(ctx, coin.Asset.GetChain())
					if err != nil {
						ctx.Logger().Error("fail to get max gas: %w", err)
						return err
					}
					// if remainder is less than the gas amount, just send it all now
					if common.SafeSub(coin.Amount, amt).LTE(gas.Amount) {
						amt = coin.Amount
					}

					gasAmount := gas.Amount.MulUint64(uint64(vault.CoinLengthByChain(coin.Asset.GetChain())))

					// deduct estimated transaction fee from send amount
					amt = common.SafeSub(amt, gasAmount)

					// burn the remainder if amount after deducting gas is below dust threshold
					dustThreshold := chain.DustThreshold()

					if amt.LTE(dustThreshold) && nth > migrationRounds {
						// No migration should be attempted, but only burn dust if there are no pending outbounds.
						// (That is, truly only dust remaining in the vault for this Coin.)
						if !coin.Amount.Equal(vault.Coins.GetCoin(coin.Asset).Amount) {
							continue
						}

						ctx.Logger().Info("left coin is not enough to pay for gas, thus burn it", "coin", coin, "gas", gasAmount)
						vault.SubFunds(common.Coins{
							coin,
						})
						if err := vm.k.SetVault(ctx, vault); err != nil {
							return fmt.Errorf("fail to save vault: %w", err)
						}
						continue
					}

				}
				toi := TxOutItem{
					Chain:            chain,
					InHash:           common.BlankTxID,
					ToAddress:        addr,
					VaultPubKey:      vault.PubKey,
					VaultPubKeyEddsa: vault.PubKeyEddsa,
					Coin: common.Coin{
						Asset:  coin.Asset,
						Amount: amt,
					},
					Memo:   "",
					TxType: types.TxOutTypeMigrate,
				}
				ok, err := vm.txOutStore.TryAddTxOutItem(ctx, mgr, toi, cosmos.ZeroUint())
				if err != nil && !errors.Is(err, ErrNotEnoughToPayFee) {
					return err
				}
				if ok {
					// Migration scheduling having been successful, add a zero Amount of this Asset to the target ActiveVault
					// (which will not be set)
					// to prioritise target vaults without it for this block's migrations from other RetiringVaults.
					// There is no need to initially add outbound queue migration Assets,
					// since new migrations are skipped when there is a pending outbound (including migrations) from any RetiringVault.
					active[targetVaultIndex].AddFunds(common.NewCoins(common.NewCoin(coin.Asset, cosmos.ZeroUint())))

					vault.AppendPendingTxBlockHeights(ctx.BlockHeight(), mgr.GetConstants())
					if err := vm.k.SetVault(ctx, vault); err != nil {
						return fmt.Errorf("fail to save vault: %w", err)
					}
				}
			}
		}
	}
	return nil
}

// paySaverYield - takes a pool asset and total rune collected in yield to the pool, then pays out savers their proportion of yield based on its size
func (vm *NetworkMgr) paySaverYield(ctx cosmos.Context, asset common.Asset, runeAmt cosmos.Uint) error {
	return nil
}

// TriggerKeygen generate a record to instruct signer kick off keygen process
func (vm *NetworkMgr) TriggerKeygen(ctx cosmos.Context, nas NodeAccounts) error {
	halt, err := vm.k.GetMimir(ctx, "HaltChurning")
	if halt > 0 && halt <= ctx.BlockHeight() && err == nil {
		ctx.Logger().Info("churn event skipped due to mimir has halted churning")
		return nil
	}
	var members []string
	for i := range nas {
		members = append(members, nas[i].PubKeySet.Secp256k1.String())
	}
	keygen, err := NewKeygen(ctx.BlockHeight(), members, AsgardKeygen)
	if err != nil {
		return fmt.Errorf("fail to create a new keygen: %w", err)
	}
	keygenBlock, err := vm.k.GetKeygenBlock(ctx, ctx.BlockHeight())
	if err != nil {
		return fmt.Errorf("fail to get keygen block from data store: %w", err)
	}

	if !keygenBlock.Contains(keygen) {
		keygenBlock.Keygens = append(keygenBlock.Keygens, keygen)
	}

	// check if we already have a an active vault with the same membership,
	// skip if we do
	active, err := vm.k.GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return fmt.Errorf("fail to get active vaults: %w", err)
	}
	for _, vault := range active {
		if vault.MembershipEquals(keygen.GetMembers()) {
			ctx.Logger().Info("skip keygen due to vault already existing")
			return nil
		}
	}

	vm.k.SetKeygenBlock(ctx, keygenBlock)
	// clear the init vault
	initVaults, err := vm.k.GetAsgardVaultsByStatus(ctx, InitVault)
	if err != nil {
		ctx.Logger().Error("fail to get init vault", "error", err)
		return nil
	}
	for _, v := range initVaults {
		if v.HasFunds() {
			continue
		}
		v.UpdateStatus(InactiveVault, ctx.BlockHeight())
		if err := vm.k.SetVault(ctx, v); err != nil {
			ctx.Logger().Error("fail to save vault", "error", err)
		}
	}
	return nil
}

// RotateVault update vault to Retiring and new vault to active
func (vm *NetworkMgr) RotateVault(ctx cosmos.Context, vault Vault) error {
	active, err := vm.k.GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}

	// find vaults the new vault conflicts with, mark them as inactive
	for _, asgard := range active {
		for _, member := range asgard.GetMembership() {
			if vault.Contains(member) {
				asgard.UpdateStatus(RetiringVault, ctx.BlockHeight())
				if err := vm.k.SetVault(ctx, asgard); err != nil {
					return err
				}

				ctx.EventManager().EmitEvent(
					cosmos.NewEvent(EventTypeInactiveVault,
						cosmos.NewAttribute("set asgard vault to inactive", asgard.PubKey.String())))
				break
			}
		}
	}

	// Update Node account membership
	for _, member := range vault.GetMembership() {
		na, err := vm.k.GetNodeAccountByPubKey(ctx, member)
		if err != nil {
			return err
		}
		na.TryAddSignerPubKey(vault.PubKey)
		if err := vm.k.SetNodeAccount(ctx, na); err != nil {
			return err
		}
	}

	vault.UpdateStatus(ActiveVault, ctx.BlockHeight())
	if err := vm.k.SetVault(ctx, vault); err != nil {
		return err
	}

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent(EventTypeActiveVault,
			cosmos.NewAttribute("add new asgard vault", vault.PubKey.String())))
	if err := vm.cleanupAsgardIndex(ctx); err != nil {
		ctx.Logger().Error("fail to clean up asgard index", "error", err)
	}
	return nil
}

func (vm *NetworkMgr) cleanupAsgardIndex(ctx cosmos.Context) error {
	asgards, err := vm.k.GetAsgardVaults(ctx)
	if err != nil {
		return fmt.Errorf("fail to get all asgards,err: %w", err)
	}
	for _, vault := range asgards {
		if vault.PubKey.IsEmpty() {
			continue
		}
		if !vault.IsAsgard() {
			continue
		}
		if vault.Status == InactiveVault {
			if err := vm.k.RemoveFromAsgardIndex(ctx, vault.PubKey); err != nil {
				ctx.Logger().Error("fail to remove inactive asgard from index", "error", err)
			}
		}
	}
	return nil
}

func (vm *NetworkMgr) withdrawSavers(ctx cosmos.Context, pool common.Asset, na NodeAccount, mgr Manager) (done bool, err error) {
	return true, nil
}

func (vm *NetworkMgr) withdrawLPs(ctx cosmos.Context, pool common.Asset, na NodeAccount, mgr Manager) (done bool) {
	return true
}

// withdrawLiquidity processes a bounded batch per iteration.
// once the all LP get processed, none-gas pool will be removed , gas pool will be set to Suspended
func (vm *NetworkMgr) withdrawLiquidity(ctx cosmos.Context, pool common.Asset, na NodeAccount, mgr Manager) error {
	return nil
}

// UpdateNetwork Update the network data to reflect changing in this block
func (vm *NetworkMgr) UpdateNetwork(ctx cosmos.Context, constAccessor constants.ConstantValues, gasManager GasManager, eventMgr EventManager) error {
	network, err := vm.k.GetNetwork(ctx)
	if err != nil {
		return fmt.Errorf("fail to get existing network data: %w", err)
	}
	return vm.k.SetNetwork(ctx, network)
}

// Pays out Rewards
func (vm *NetworkMgr) payPoolRewards(ctx cosmos.Context, poolRewards []cosmos.Uint, pools []common.Asset) error {
	return nil
}

// Calculate pool deficit based on the pool's accrued fees compared with total fees.
func (vm *NetworkMgr) calcPoolDeficit(lpDeficit, totalFees, poolFees cosmos.Uint) cosmos.Uint {
	return common.GetSafeShare(poolFees, totalFees, lpDeficit)
}

// Calculate the block rewards that bonders and liquidity providers should receive
func (vm *NetworkMgr) calcBlockRewards(
	ctx cosmos.Context,
	availablePoolsRune,
	vaultsLiquidityRune,
	effectiveSecurityBond,
	totalEffectiveBond,
	totalReserve,
	totalLiquidityFees cosmos.Uint,
	emissionCurve int64,
	blocksPerYear int64,
	devFundSystemIncomeBps int64,
	systemIncomeBurnRateBps int64,
	tcyStakeSystemIncomeBps int64,
	marketingFundSystemIncomeBps int64,
	polReserveSystemIncomeBps int64) (
	bondReward cosmos.Uint,
	totalPoolRewards cosmos.Uint,
	lpShare cosmos.Uint,
	devFundDeduct cosmos.Uint,
	systemIncomeBurnDeduct cosmos.Uint,
	tcyStakeDeduct cosmos.Uint,
	marketingFundDeduct cosmos.Uint,
	polReserveDeduct cosmos.Uint,
) {
	// Block Rewards will take the latest reserve, divide it by the emission
	// curve factor, then divide by blocks per year
	trD := cosmos.NewDec(int64(totalReserve.Uint64()))
	ecD := cosmos.NewDec(emissionCurve)
	bpyD := cosmos.NewDec(blocksPerYear)
	// Defensive check: ensure emission curve and blocks per year are positive
	if emissionCurve <= 0 || blocksPerYear <= 0 {
		ctx.Logger().Error("invalid emission curve or blocks per year", "emissionCurve", emissionCurve, "blocksPerYear", blocksPerYear)
		// Return zero rewards if config is invalid
		return cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint(), cosmos.ZeroUint()
	}
	blockRewardD := trD.Quo(ecD).Quo(bpyD)
	blockReward := cosmos.NewUint(uint64((blockRewardD).RoundInt64()))

	systemIncome := blockReward.Add(totalLiquidityFees) // Get total system income for block
	devFundSystemIncomeBpsUint := cosmos.SafeUintFromInt64(devFundSystemIncomeBps)
	systemIncomeBurnRateBpsUint := cosmos.SafeUintFromInt64(systemIncomeBurnRateBps)
	tcyStakeSystemIncomeBpsUint := cosmos.SafeUintFromInt64(tcyStakeSystemIncomeBps)
	marketingFundSystemIncomeBpsUint := cosmos.SafeUintFromInt64(marketingFundSystemIncomeBps)
	devFundDeduct = common.GetSafeShare(devFundSystemIncomeBpsUint, cosmos.NewUint(10_000), systemIncome)
	systemIncomeBurnDeduct = common.GetSafeShare(systemIncomeBurnRateBpsUint, cosmos.NewUint(10_000), systemIncome)
	tcyStakeDeduct = common.GetSafeShare(tcyStakeSystemIncomeBpsUint, cosmos.NewUint(10_000), systemIncome)
	marketingFundDeduct = common.GetSafeShare(marketingFundSystemIncomeBpsUint, cosmos.NewUint(10_000), systemIncome)
	polReserveSystemIncomeBpsUint := cosmos.SafeUintFromInt64(polReserveSystemIncomeBps)
	polReserveDeduct = common.GetSafeShare(polReserveSystemIncomeBpsUint, cosmos.NewUint(10_000), systemIncome)

	totalBps := devFundSystemIncomeBpsUint.Add(systemIncomeBurnRateBpsUint).Add(tcyStakeSystemIncomeBpsUint).Add(marketingFundSystemIncomeBpsUint).Add(polReserveSystemIncomeBpsUint)
	if totalBps.GT(cosmos.NewUint(10_000)) {
		ctx.Logger().Error("total system income BPS exceeds 10000, deductions will be clamped",
			"total_bps", totalBps.String(),
			"dev_fund_bps", devFundSystemIncomeBpsUint.String(),
			"burn_bps", systemIncomeBurnRateBpsUint.String(),
			"tcy_stake_bps", tcyStakeSystemIncomeBpsUint.String(),
			"marketing_fund_bps", marketingFundSystemIncomeBpsUint.String(),
			"pol_reserve_bps", polReserveSystemIncomeBpsUint.String(),
		)
	}

	assetsBps := cosmos.NewUint(10_000)
	useEffectiveSecurity := false
	useVaultAssets := false

	if !tcyStakeDeduct.IsZero() {
		systemIncome = common.SafeSub(systemIncome, tcyStakeDeduct)
	}

	if devFundDeduct.GT(systemIncome) {
		devFundDeduct = systemIncome
	}

	if !devFundDeduct.IsZero() {
		systemIncome = common.SafeSub(systemIncome, devFundDeduct)
	}

	if systemIncomeBurnDeduct.GT(systemIncome) {
		systemIncomeBurnDeduct = systemIncome
	}
	if !systemIncomeBurnDeduct.IsZero() {
		systemIncome = common.SafeSub(systemIncome, systemIncomeBurnDeduct)
	}

	if marketingFundDeduct.GT(systemIncome) {
		marketingFundDeduct = systemIncome
	}
	if !marketingFundDeduct.IsZero() {
		systemIncome = common.SafeSub(systemIncome, marketingFundDeduct)
	}

	if polReserveDeduct.GT(systemIncome) {
		polReserveDeduct = systemIncome
	}
	if !polReserveDeduct.IsZero() {
		systemIncome = common.SafeSub(systemIncome, polReserveDeduct)
	}

	lpSplit := vm.getPoolShare(availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, systemIncome, assetsBps, useEffectiveSecurity, useVaultAssets) // Get liquidity provider share
	bonderSplit := common.SafeSub(systemIncome, lpSplit)                                                                                                                          // Remainder to Bonders

	ctx.Logger().Info(
		"incentive pendulum",
		"total_effective_bond", totalEffectiveBond,
		"effective_security_bond", effectiveSecurityBond,
		"vaults_liquidity_rune", vaultsLiquidityRune,
		"available_pools_rune", availablePoolsRune,
		"block_reward", blockReward,
		"total_liquidity_fees", totalLiquidityFees,
		"dev_fund_reward", devFundDeduct,
		"income_burn", systemIncomeBurnDeduct,
		"marketing_fund_reward", marketingFundDeduct,
		"total_pendulum_rewards", systemIncome,
		"pendulum_assets_basis_points", assetsBps,
		"use_vault_assets", useVaultAssets,
		"use_effective_security", useEffectiveSecurity,
		"bond_rewards", bonderSplit,
		"pool_rewards", lpSplit,
		"tcy_stake_reward", tcyStakeDeduct,
		"pol_reserve_reward", polReserveDeduct,
		"system_income", systemIncome,
	)

	lpShare = common.GetSafeShare(lpSplit, systemIncome, cosmos.NewUint(10_000))

	return bonderSplit, lpSplit, lpShare, devFundDeduct, systemIncomeBurnDeduct, tcyStakeDeduct, marketingFundDeduct, polReserveDeduct
}

// getPoolShare calculates the pool share of the total rewards. The distribution is
// calculated such that the amount distributed to pools should equal the amount
// distributed to the security bond when security bond is 2x the value in pools.
//
// totalLiquidty: RUNE value in pools
// securityBond: RUNE value bonded by smallest 66% of nodes
// effectiveBond: total RUNE value bonded, with max per-node at 66th percentile
// totalRewards: total RUNE rewards to be distributed
func (vm *NetworkMgr) getPoolShare(
	pooledRune, vaultLiquidity, effectiveSecurityBond, totalEffectiveBond, totalRewards, assetsBps cosmos.Uint, useEffectiveSecurity, useVaultAssets bool,
) cosmos.Uint {
	securing := effectiveSecurityBond
	secured := vaultLiquidity

	if !useEffectiveSecurity {
		securing = totalEffectiveBond
	}
	if !useVaultAssets {
		secured = pooledRune
	}

	// Proportionally underestimate or overestimate the Assets (in terms of RUNE value) needing to be secured.
	secured = common.GetUncappedShare(assetsBps, cosmos.NewUint(constants.MaxBasisPts), secured)

	// no payments to liquidity providers when more liquidity than security
	if securing.LTE(secured) {
		return cosmos.ZeroUint()
	}

	// calculate the base node share rewards
	baseNodeShare := common.GetSafeShare(secured, securing, totalRewards)

	// base pool share is the remaining
	basePoolShare := common.SafeSub(totalRewards, baseNodeShare)

	// correct for share of node rewards not received by the security bond
	// and for that pools shouldn't receive rewards for vault liquidity not in pools
	adjustmentNodeShare := common.GetUncappedShare(totalEffectiveBond, effectiveSecurityBond, baseNodeShare)
	adjustmentPoolShare := common.GetSafeShare(pooledRune, vaultLiquidity, basePoolShare)

	if !useEffectiveSecurity {
		adjustmentNodeShare = baseNodeShare
	}
	if !useVaultAssets {
		adjustmentPoolShare = basePoolShare
	}

	adjustmentRewards := adjustmentPoolShare.Add(adjustmentNodeShare)

	// Derive the pool share according to the adjustment rewards,
	// totalRewards being the allocation to never be exceeded.
	return common.GetSafeShare(adjustmentPoolShare, adjustmentRewards, totalRewards)
}

func (vm *NetworkMgr) redeemSynthAssetToReserve(ctx cosmos.Context, p common.Asset) error {
	return nil
}

// calculateNetworkSolvency calculates the aggregate solvency across all active vaults
// Returns a list of assets with their solvency amounts (positive = over-solvent, negative = under-solvent)
func (vm *NetworkMgr) calculateNetworkSolvency(ctx cosmos.Context, mgr Manager) ([]assetAmount, error) {
	return nil, nil
}
