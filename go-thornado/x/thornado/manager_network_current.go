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
	vaults, err := vm.k.GetBaseVaults(ctx)
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
	minimumMembers := vm.k.GetConfigInt64(ctx, constants.Vault_BaseMembersMin)
	if minimumMembers > 0 && len(active) < int(minimumMembers) {
		ctx.Logger().Info("skip genesis base vault keygen, not enough active nodes", "active", len(active), "minimum", minimumMembers)
		return nil
	}
	if len(active) == 1 {
		supportChains := common.Chains{
			common.BTCChain,
			common.BTCChain,
		}
		pubSet := active[0].PubKeySet
		vault := NewVaultV2(0, ActiveVault, BaseVault, pubSet.Secp256k1, supportChains.Strings(), pubSet.Ed25519)
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

// EndBlock move funds from retiring base vaults
func (vm *NetworkMgr) EndBlock(ctx cosmos.Context, mgr Manager) error {
	if ctx.BlockHeight() == genesisBlockHeight {
		return vm.processGenesisSetup(ctx)
	}
	if err := vm.migrateFunds(ctx, mgr); err != nil {
		ctx.Logger().Error("fail to migrate funds", "error", err)
	}

	if err := vm.consolidateActiveBTCVaults(ctx, mgr); err != nil {
		ctx.Logger().Error("fail to schedule bitcoin vault consolidation", "error", err)
	}

	return nil
}

func (vm *NetworkMgr) consolidateActiveBTCVaults(ctx cosmos.Context, mgr Manager) error {
	vaults, err := vm.k.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}

	threshold := vm.k.GetConfigInt64(ctx, constants.UTXO_MaxSpendCount)
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
		consolidateAmount := common.SafeSub(coin.Amount, maxGasCoin.Amount.MulUint64(2))
		if consolidateAmount.IsZero() {
			continue
		}
		rootAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
		if err != nil {
			return err
		}
		gasRate := vm.k.GetConfigInt64(ctx, constants.BTC_DefaultSatsPerVByte)
		if nf, err := vm.k.GetNetworkFee(ctx, common.BTCChain); err == nil && nf.TransactionFeeRate > 0 {
			gasRate = int64(nf.TransactionFeeRate)
		}
		item := TxOutItem{
			Chain:            common.BTCChain,
			ToAddress:        rootAddr,
			VaultPubKey:      vault.PubKey,
			VaultPubKeyEddsa: vault.PubKeyEddsa,
			Coin:             common.NewCoin(common.BTCAsset, consolidateAmount),
			MaxGas:           common.Gas{maxGasCoin},
			GasRate:          gasRate,
			InHash:           common.BlankTxID,
			ModuleName:       BaseName,
			VaultPathIndex:   common.MainVaultPathIndex,
			TxType:           types.TxOutTypeConsolidate,
		}
		if err := vm.k.AppendTxOut(ctx, ctx.BlockHeight(), item); err != nil {
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
	signingPeriod := getConfigDurationBlocks(ctx, k, constants.Keysign_PeriodMinutes)
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
	migrateInterval := getConfigDurationBlocks(ctx, vm.k, constants.Vault_MigrationIntervalMinutes)

	retiring, err := vm.k.GetBaseVaultsByStatus(ctx, RetiringVault)
	if err != nil {
		return err
	}

	active, err := vm.k.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}

	// if we have no active baseVaults to move funds to, don't move funds
	if len(active) == 0 {
		return nil
	}
	// if we have no retiring baseVaults to move funds from, don't do anything further
	if len(retiring) == 0 {
		return nil
	}

	vaultsAvailableCoins := map[common.PubKey]common.Coins{}
	for _, vault := range retiring {
		if vault.LenPendingTxBlockHeights(ctx.BlockHeight(), getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes)) > 0 {
			ctx.Logger().Info("Skipping the migration of funds while transactions are still pending")
			// This refers to migrate TxOutItems only.
			return nil
		}

		// Copy the RetiringVault Coins for deduction.
		vaultsAvailableCoins[vault.PubKey] = common.NewCoins(vault.Coins...)
	}

	migrationRounds := vm.k.GetConfigInt64(ctx, constants.Vault_MigrationRounds)

	signingTransactionPeriod := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes)
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
				// Internal native assets are not migrated. BTC.BTC is an external
				// L1 asset in Thornado and must migrate between base vaults.
				if coin.Asset.IsNative() {
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
				// get address of base pubkey
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
				maxGas := common.Gas{}
				gasRate := vm.k.GetConfigInt64(ctx, constants.BTC_DefaultSatsPerVByte)
				if nf, err := vm.k.GetNetworkFee(ctx, chain); err == nil && nf.TransactionFeeRate > 0 {
					gasRate = int64(nf.TransactionFeeRate)
				}

				// minus gas costs for our transactions
				gasAsset := chain.GetGasAsset()
				if coin.Asset.Equals(gasAsset) {
					gasMgr := mgr.GasMgr()
					gas, err := gasMgr.GetMaxGas(ctx, coin.Asset.GetChain())
					if err != nil {
						ctx.Logger().Error("fail to get max gas: %w", err)
						return err
					}
					maxGas = common.Gas{gas}
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
					MaxGas:         maxGas,
					GasRate:        gasRate,
					VaultPathIndex: common.MainVaultPathIndex,
					TxType:         types.TxOutTypeMigrate,
				}
				ok := false
				if chain.Equals(common.BTCChain) {
					if err := vm.k.AppendTxOut(ctx, ctx.BlockHeight(), toi); err != nil {
						return fmt.Errorf("fail to add bitcoin migration txout: %w", err)
					}
					ok = true
				} else {
					ok, err = vm.txOutStore.TryAddTxOutItem(ctx, mgr, toi, cosmos.ZeroUint())
					if err != nil && !errors.Is(err, ErrNotEnoughToPayFee) {
						return err
					}
				}
				if ok {
					// Migration scheduling having been successful, add a zero Amount of this Asset to the target ActiveVault
					// (which will not be set)
					// to prioritise target vaults without it for this block's migrations from other RetiringVaults.
					// There is no need to initially add outbound queue migration Assets,
					// since new migrations are skipped when there is a pending outbound (including migrations) from any RetiringVault.
					active[targetVaultIndex].AddFunds(common.NewCoins(common.NewCoin(coin.Asset, cosmos.ZeroUint())))

					vault.AppendPendingTxBlockHeights(ctx.BlockHeight(), getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes))
					if err := vm.k.SetVault(ctx, vault); err != nil {
						return fmt.Errorf("fail to save vault: %w", err)
					}
				}
			}
		}
	}
	return nil
}

// TriggerKeygen generate a record to instruct signer kick off keygen process
func (vm *NetworkMgr) TriggerKeygen(ctx cosmos.Context, nas NodeAccounts) error {
	halt := vm.k.GetConfigInt64(ctx, constants.Halt_Churning)
	if halt > 0 && halt <= ctx.BlockHeight() {
		ctx.Logger().Info("churn event skipped due to config has halted churning")
		return nil
	}
	minimumMembers := vm.k.GetConfigInt64(ctx, constants.Vault_BaseMembersMin)
	if minimumMembers > 0 && len(nas) < int(minimumMembers) {
		ctx.Logger().Info("skip base vault keygen, not enough members", "members", len(nas), "minimum", minimumMembers)
		return nil
	}
	var members []string
	seenMembers := make(map[string]struct{}, len(nas))
	for i := range nas {
		if nas[i].PubKeySet.IsEmpty() {
			return fmt.Errorf("fail to trigger keygen: node %s has empty pubkey set", nas[i].NodeAddress)
		}
		member := nas[i].PubKeySet.Secp256k1.String()
		if _, ok := seenMembers[member]; ok {
			return fmt.Errorf("fail to trigger keygen: duplicate secp256k1 pubkey %s", member)
		}
		seenMembers[member] = struct{}{}
		members = append(members, member)
	}
	keygen, err := NewKeygen(ctx.BlockHeight(), members, BaseVaultKeygen)
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
	active, err := vm.k.GetBaseVaultsByStatus(ctx, ActiveVault)
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
	initVaults, err := vm.k.GetBaseVaultsByStatus(ctx, InitVault)
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
	active, err := vm.k.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}

	// find vaults the new vault conflicts with, mark them as inactive
	for _, base := range active {
		for _, member := range base.GetMembership() {
			if vault.Contains(member) {
				base.UpdateStatus(RetiringVault, ctx.BlockHeight())
				if err := vm.k.SetVault(ctx, base); err != nil {
					return err
				}

				ctx.EventManager().EmitEvent(
					cosmos.NewEvent(EventTypeInactiveVault,
						cosmos.NewAttribute("set base vault to inactive", base.PubKey.String())))
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
			cosmos.NewAttribute("add new base vault", vault.PubKey.String())))
	if err := vm.cleanupBaseIndex(ctx); err != nil {
		ctx.Logger().Error("fail to clean up base index", "error", err)
	}
	return nil
}

func (vm *NetworkMgr) cleanupBaseIndex(ctx cosmos.Context) error {
	baseVaults, err := vm.k.GetBaseVaults(ctx)
	if err != nil {
		return fmt.Errorf("fail to get all baseVaults,err: %w", err)
	}
	for _, vault := range baseVaults {
		if vault.PubKey.IsEmpty() {
			continue
		}
		if !vault.IsBase() {
			continue
		}
		if vault.Status == InactiveVault {
			if err := vm.k.RemoveFromBaseIndex(ctx, vault.PubKey); err != nil {
				ctx.Logger().Error("fail to remove inactive base from index", "error", err)
			}
		}
	}
	return nil
}

// UpdateNetwork Update the network data to reflect changing in this block
func (vm *NetworkMgr) UpdateNetwork(ctx cosmos.Context, constAccessor constants.ConfigValues, gasManager GasManager, eventMgr EventManager) error {
	network, err := vm.k.GetNetwork(ctx)
	if err != nil {
		return fmt.Errorf("fail to get existing network data: %w", err)
	}
	return vm.k.SetNetwork(ctx, network)
}

// calculateNetworkSolvency calculates the aggregate solvency across all active vaults
// Returns a list of assets with their solvency amounts (positive = over-solvent, negative = under-solvent)
func (vm *NetworkMgr) calculateNetworkSolvency(ctx cosmos.Context, mgr Manager) ([]assetAmount, error) {
	return nil, nil
}
