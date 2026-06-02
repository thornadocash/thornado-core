package keeperv1

import (
	"errors"
	"fmt"
	"sort"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	kvTypes "github.com/thornadocash/go-thornado/x/thornado/keeper/types"
)

type VaultSecurity struct {
	Vault      Vault
	TotalBond  cosmos.Uint
	TotalValue cosmos.Uint
	Diff       cosmos.Int
}

func (k KVStore) setVault(ctx cosmos.Context, key []byte, record Vault) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getVault(ctx cosmos.Context, key []byte, record *Vault) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, record); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	return true, nil
}

// GetVaultIterator only iterates vaults.
func (k KVStore) GetVaultIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixVault)
}

// GetMostSecure with given list of vaults, find the vault that is most secure
func (k KVStore) GetMostSecure(ctx cosmos.Context, vaults Vaults, signingTransPeriod int64) Vault {
	vaults = k.SortBySecurity(ctx, vaults, signingTransPeriod)
	if len(vaults) == 0 {
		return Vault{}
	}
	return vaults[len(vaults)-1]
}

// GetMostSecureStrict given list of vaults, find the most secure.
// if the most secure vault's bond is less than securityBps * the vault's asset
// value in rune, it is considered insecure and no vault is returned
func (k KVStore) GetMostSecureStrict(ctx cosmos.Context, vaults Vaults, signingTransPeriod int64) Vault {
	vaultSecurity := k.getSortedVaultSecurity(ctx, vaults, signingTransPeriod)
	if len(vaults) == 0 {
		return Vault{}
	}

	return vaultSecurity[len(vaults)-1].Vault
}

// GetLeastSecure with given list of vaults, find the vault that is least secure
func (k KVStore) GetLeastSecure(ctx cosmos.Context, vaults Vaults, signingTransPeriod int64) Vault {
	vaults = k.SortBySecurity(ctx, vaults, signingTransPeriod)
	if len(vaults) == 0 {
		return Vault{}
	}
	return vaults[0]
}

// SortBySecurity sorts a list of vaults in an order by how close the total
// value of the vault is to the total bond of the members of that vault. Sorts
// by least secure to most secure.
func (k KVStore) SortBySecurity(ctx cosmos.Context, vaults Vaults, signingTransPeriod int64) Vaults {
	if len(vaults) <= 1 {
		return vaults
	}

	vaultSecurity := k.getSortedVaultSecurity(ctx, vaults, signingTransPeriod)
	final := make(Vaults, len(vaultSecurity))
	for i, v := range vaultSecurity {
		final[i] = v.Vault
	}

	return final
}

func (k KVStore) getSortedVaultSecurity(ctx cosmos.Context, vaults Vaults, signingTransPeriod int64) []VaultSecurity {
	vaultSecurity := make([]VaultSecurity, len(vaults))

	for i, vault := range vaults {
		// get total bond
		totalBond := cosmos.ZeroUint()
		for _, pk := range vault.GetMembership() {
			na, err := k.GetNodeAccountByPubKey(ctx, pk)
			if err != nil {
				ctx.Logger().Error("failed to get node account by pubkey", "error", err)
				continue
			}
			if na.Status == NodeActive {
				totalBond = totalBond.Add(na.Bond)
			}
		}

		// get total value
		totalValue := cosmos.ZeroUint()

		// add recent unsent txout items to totalValue
		h := ctx.BlockHeight() - signingTransPeriod
		if h < 1 {
			h = 1
		}
		for height := h; height <= ctx.BlockHeight(); height += 1 {
			txOut, err := k.GetTxOut(ctx, height)
			if err != nil {
				ctx.Logger().Error("unable to get txout", "error", err)
				continue
			}
			for _, item := range txOut.TxArray {
				if item.OutHash.IsEmpty() {
					toAddress, err := vault.GetAddress(item.Coin.Asset.GetChain())
					if err != nil {
						ctx.Logger().Error("failed to get address of chain", "error", err)
						continue
					}
					if item.VaultPubKey.Equals(vault.PubKey) {
						totalValue = common.SafeSub(totalValue, item.Coin.Amount)
					} else if item.ToAddress.Equals(toAddress) {
						totalValue = totalValue.Add(item.Coin.Amount)
					}
				}
			}
		}

		if totalValue.GT(totalBond) {
			diff := totalValue.Sub(totalBond).BigInt()
			vaultSecurity[i] = VaultSecurity{
				Vault:      vault,
				TotalBond:  totalBond,
				TotalValue: totalValue,
				Diff:       cosmos.NewIntFromBigInt(diff).MulRaw(-1),
			}
		} else {
			diff := totalBond.Sub(totalValue).BigInt()
			vaultSecurity[i] = VaultSecurity{
				Vault:      vault,
				TotalBond:  totalBond,
				TotalValue: totalValue,
				Diff:       cosmos.NewIntFromBigInt(diff),
			}
		}
	}

	// sort by how far total bond and total value are from each other
	sort.SliceStable(vaultSecurity, func(i, j int) bool {
		return vaultSecurity[i].Diff.LT(vaultSecurity[j].Diff)
	})

	return vaultSecurity
}

// GetPendingOutbounds selects txouts in the outbound and scheduled outbound queues (for deduction to leave only 'available' balances),
// as the amounts of both types of txout items are yet to be deducted from the vault balances
func (k KVStore) GetPendingOutbounds(ctx cosmos.Context, asset common.Asset) []TxOutItem {
	signingPeriod := constants.MinutesToBlocks(
		k.GetConfigInt64(ctx, constants.Keysign_PeriodMinutes),
		k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds),
	)
	startHeight := ctx.BlockHeight() - signingPeriod
	if startHeight < 1 {
		startHeight = 1
	}
	var outbounds []TxOutItem
	for height := startHeight; height <= ctx.BlockHeight()+signingPeriod; height++ {
		blockOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("fail to get block tx out", "error", err)
		}
		for _, txOutItem := range blockOut.TxArray {
			// only need to look at outbounds for the same asset
			if !txOutItem.Coin.Asset.Equals(asset) {
				continue
			}
			// only still outstanding txout will be considered
			if !txOutItem.OutHash.IsEmpty() {
				continue
			}
			outbounds = append(outbounds, txOutItem)
		}
	}
	return outbounds
}

// SetVault save the Vault object to store
func (k KVStore) SetVault(ctx cosmos.Context, vault Vault) error {
	if vault.IsBase() {
		if err := k.addBaseIndex(ctx, vault.PubKey); err != nil {
			return err
		}
		if !vault.PubKeyEddsa.IsEmpty() {
			if err := k.addBaseEDDSAIndex(ctx, vault.PubKeyEddsa, vault.PubKey); err != nil {
				return err
			}
		}
	}

	k.setVault(ctx, k.GetKey(prefixVault, vault.PubKey.String()), vault)
	return nil
}

// VaultExists check whether the given pubkey is associated with a vault
func (k KVStore) VaultExists(ctx cosmos.Context, pk common.PubKey) bool {
	eddsaPubKey, err := k.getBaseEDDSAIndex(ctx, pk)
	if err != nil {
		ctx.Logger().Error("fail to getBaseEDDSAIndex", err)
		return false
	}

	return k.has(ctx, k.GetKey(prefixVault, pk.String())) || !eddsaPubKey.IsEmpty()
}

// GetVault get Vault with the given pubkey from data store
func (k KVStore) GetVault(ctx cosmos.Context, pk common.PubKey) (Vault, error) {
	record := Vault{
		BlockHeight: ctx.BlockHeight(),
		PubKey:      pk,
	}
	ok, err := k.getVault(ctx, k.GetKey(prefixVault, pk.String()), &record)
	if !ok {
		// TODO: check for lookup by EDDSA pubkey
		ecdsaPubKey, eddsaErr := k.getBaseEDDSAIndex(ctx, pk)
		if eddsaErr != nil {
			return record, fmt.Errorf("unable to getBaseEDDSAIndex for %s: %w", pk, kvTypes.ErrVaultNotFound)
		}
		if !ecdsaPubKey.IsEmpty() {
			return k.GetVault(ctx, ecdsaPubKey)
		}
		return record, fmt.Errorf("vault with pubkey(%s) doesn't exist: %w", pk, kvTypes.ErrVaultNotFound)
	}
	if record.PubKey.IsEmpty() {
		record.PubKey = pk
	}
	// Maintains pre-sdk v0.50 behavior where the current block height is set if the vault's block height is 0
	if record.BlockHeight == 0 {
		record.BlockHeight = ctx.BlockHeight()
	}
	return record, err
}

func (k KVStore) getBaseIndex(ctx cosmos.Context) (common.PubKeys, error) {
	record := make([]string, 0)
	_, err := k.getStrings(ctx, k.GetKey(prefixVaultBaseIndex, ""), &record)
	if err != nil {
		return nil, err
	}
	pks := make(common.PubKeys, len(record))
	for i, s := range record {
		pks[i], err = common.NewPubKey(s)
		if err != nil {
			return nil, err
		}
	}
	return pks, nil
}

func (k KVStore) addBaseIndex(ctx cosmos.Context, pubkey common.PubKey) error {
	pks, err := k.getBaseIndex(ctx)
	if err != nil {
		return err
	}
	for _, pk := range pks {
		if pk.Equals(pubkey) {
			return nil
		}
	}
	pks = append(pks, pubkey)
	k.setStrings(ctx, k.GetKey(prefixVaultBaseIndex, ""), pks.Strings())
	return nil
}

func (k KVStore) addBaseEDDSAIndex(ctx cosmos.Context, pubKeyEDDSA, pubkeyECDSA common.PubKey) error {
	k.setStrings(ctx, k.GetKey(prefixVaultBaseEDDSAIndex, pubKeyEDDSA.String()), []string{pubkeyECDSA.String()})
	return nil
}

func (k KVStore) getBaseEDDSAIndex(ctx cosmos.Context, pubKeyEDDSA common.PubKey) (common.PubKey, error) {
	record := make([]string, 0)
	exists, err := k.getStrings(ctx, k.GetKey(prefixVaultBaseEDDSAIndex, pubKeyEDDSA.String()), &record)
	if err != nil || !exists {
		return common.EmptyPubKey, err
	}
	return common.PubKey(record[0]), nil
}

func (k KVStore) RemoveFromBaseIndex(ctx cosmos.Context, pubkey common.PubKey) error {
	pks, err := k.getBaseIndex(ctx)
	if err != nil {
		return err
	}

	newPks := common.PubKeys{}
	for _, pk := range pks {
		if !pk.Equals(pubkey) {
			newPks = append(newPks, pk)
		}
	}

	k.setStrings(ctx, k.GetKey(prefixVaultBaseIndex, ""), newPks.Strings())
	return nil
}

// GetBaseVaults return all base vaults
func (k KVStore) GetBaseVaults(ctx cosmos.Context) (Vaults, error) {
	pks, err := k.getBaseIndex(ctx)
	if err != nil {
		return nil, err
	}

	var baseVaults Vaults
	for _, pk := range pks {
		var vault Vault
		vault, err = k.GetVault(ctx, pk)
		if err != nil {
			return nil, err
		}
		if vault.IsBase() {
			baseVaults = append(baseVaults, vault)
		}
	}

	return baseVaults, nil
}

// GetBaseVaultsByStatus get all the base vault that have the given status
func (k KVStore) GetBaseVaultsByStatus(ctx cosmos.Context, status VaultStatus) (Vaults, error) {
	all, err := k.GetBaseVaults(ctx)
	if err != nil {
		return nil, err
	}

	var baseVaults Vaults
	for _, vault := range all {
		if vault.Status == status {
			baseVaults = append(baseVaults, vault)
		}
	}

	return baseVaults, nil
}

// DeleteVault remove the given vault from data store
func (k KVStore) DeleteVault(ctx cosmos.Context, pubkey common.PubKey) error {
	vault, err := k.GetVault(ctx, pubkey)
	if err != nil {
		if errors.Is(err, kvTypes.ErrVaultNotFound) {
			return nil
		}
		return err
	}

	if vault.HasFunds() {
		return errors.New("unable to delete vault: it still contains funds")
	}

	if vault.IsBase() {
		if err = k.RemoveFromBaseIndex(ctx, pubkey); err != nil {
			ctx.Logger().Error("fail to remove vault from base index", "error", err)
		}
	}
	// delete the actual vault
	k.del(ctx, k.GetKey(prefixVault, vault.PubKey.String()))
	return nil
}
