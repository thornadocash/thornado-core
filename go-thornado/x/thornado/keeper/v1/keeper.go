package keeperv1

import (
	"fmt"
	"strings"

	storetypes "cosmossdk.io/core/store"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/keeper/types"
)

// NOTE: Always end a dbPrefix with a penalty ("/"). This is to ensure that there
// are no prefixes that contain another prefix. In the scenario where this is
// true, an iterator for a specific type, will get more than intended, and may
// include a different type. The penalty is used to protect us from this
// scenario.
// Also, use underscores between words and use lowercase characters only

const (
	prefixObservedTxIn            types.DbPrefix = "observed_tx_in/"
	prefixObservedTxOut           types.DbPrefix = "observed_tx_out/"
	prefixObservedLink            types.DbPrefix = "ob_link/"
	prefixTxOut                   types.DbPrefix = "txout/"
	prefixLastChainHeight         types.DbPrefix = "last_chain_height/"
	prefixLastSignedHeight        types.DbPrefix = "last_signed_height/"
	prefixLastObserveHeight       types.DbPrefix = "last_observe_height/"
	prefixNodeAccount             types.DbPrefix = "node_account/"
	prefixVault                   types.DbPrefix = "vault/"
	prefixVaultBaseIndex          types.DbPrefix = "vault_base_index/"
	prefixVaultBaseEDDSAIndex     types.DbPrefix = "vault_base_eddsa_index/"
	prefixNetwork                 types.DbPrefix = "network/"
	prefixObservingAddresses      types.DbPrefix = "observing_addresses/"
	prefixFrost                     types.DbPrefix = "frost/"
	prefixFrostKeysignFailure       types.DbPrefix = "frostKeysignFailure/"
	prefixKeygen                  types.DbPrefix = "keygen/"
	prefixErrataTx                types.DbPrefix = "errata/"
	prefixNodePenaltyPoints       types.DbPrefix = "penalty/"
	prefixNodeJail                types.DbPrefix = "jail/"
	prefixConfig                  types.DbPrefix = "config/"
	prefixMinJoinLast             types.DbPrefix = "minjoinlast/"
	prefixNodeConfig              types.DbPrefix = "nodeconfig/"
	prefixNodePauseChain          types.DbPrefix = "node_pause_chain/"
	prefixNetworkFee              types.DbPrefix = "network_fee/"
	prefixNetworkFeeVoter         types.DbPrefix = "network_fee_voter/"
	prefixFrostKeygenMetric         types.DbPrefix = "frost_keygen_metric/"
	prefixFrostKeysignMetric        types.DbPrefix = "frost_keysign_metric/"
	prefixFrostKeysignMetricLatest  types.DbPrefix = "latest_frost_keysign_metric/"
	prefixSolvencyVoter           types.DbPrefix = "solvency_voter/"
	prefixVersion                 types.DbPrefix = "version/"
	prefixUpgradeProposals        types.DbPrefix = "upgr_props/"
	prefixUpgradeVotes            types.DbPrefix = "upgr_votes/"
	prefixOraclePrice             types.DbPrefix = "oracle_price/"
	prefixDepositSession          types.DbPrefix = "deposit_session/"
	prefixShielderPowToken        types.DbPrefix = "deposit_pow/"
	prefixDepositPowTiming        types.DbPrefix = "deposit_pow_timing/"
	prefixDepositPowDifficulty    types.DbPrefix = "deposit_pow_difficulty/"
	prefixDepositAddress          types.DbPrefix = "deposit_address/"
	prefixVaultDepositPathIndex   types.DbPrefix = "vault_deposit_path_index/"
	prefixDepositRecord           types.DbPrefix = "deposit_record/"
	prefixShielderCommitment      types.DbPrefix = "shielder_commitment/"
	prefixShielderNotePubKey      types.DbPrefix = "shielder_note_pubkey/"
	prefixShielderDenomCommitment types.DbPrefix = "shielder_denom_commitment/"
	prefixShielderMerkleRoot      types.DbPrefix = "shielder_merkle_root/"
	prefixShielderRedeem          types.DbPrefix = "shielder_withdrawal/"
	prefixShielderNullifier       types.DbPrefix = "shielder_nullifier/"
	prefixShielderNodeBond        types.DbPrefix = "shielder_node_bond/"
	prefixShielderNodeBondSlot    types.DbPrefix = "shielder_node_bond_slot/"
	prefixFeePool                 types.DbPrefix = "fee_pool/"
	prefixShielderFeeNotePubKey   types.DbPrefix = "shielder_fee_note_pubkey/"
	prefixNodeSlotAuction         types.DbPrefix = "node_slot_auction/"
	prefixNodeSlotBid             types.DbPrefix = "node_slot_bid/"
	prefixNodeSlotAuctionBid      types.DbPrefix = "node_slot_auction_bid/"
)

func dbError(ctx cosmos.Context, wrapper string, err error) error {
	err = fmt.Errorf("KVStore Error: %s: %w", wrapper, err)
	ctx.Logger().Error("keeper error", "error", err)
	return err
}

// KVStore Keeper maintains the link to data storage and exposes getter/setter methods for the various parts of the state machine
type KVStore struct {
	cdc           codec.BinaryCodec
	coinKeeper    bankkeeper.Keeper
	accountKeeper authkeeper.AccountKeeper
	upgradeKeeper *upgradekeeper.Keeper
	storeService  storetypes.KVStoreService
	version       semver.Version
	constAccessor constants.ConfigValues
}

// NewKVStore creates new instances of the thornado Keeper
func NewKVStore(cdc codec.BinaryCodec, storeService storetypes.KVStoreService, coinKeeper bankkeeper.Keeper, accountKeeper authkeeper.AccountKeeper, upgradeKeeper *upgradekeeper.Keeper, version semver.Version) KVStore {
	return KVStore{
		coinKeeper:    coinKeeper,
		accountKeeper: accountKeeper,
		upgradeKeeper: upgradeKeeper,
		storeService:  storeService,
		cdc:           cdc,
		version:       version,
		constAccessor: constants.GetConfigValues(version),
	}
}

// NewKeeper creates new instances of the thornado Keeper
func NewKeeper(cdc codec.BinaryCodec, storeService storetypes.KVStoreService, coinKeeper bankkeeper.Keeper, accountKeeper authkeeper.AccountKeeper, upgradeKeeper *upgradekeeper.Keeper) keeper.Keeper {
	version := semver.MustParse("0.0.0")
	return NewKVStore(cdc, storeService, coinKeeper, accountKeeper, upgradeKeeper, version)
}

// Cdc return the amino codec
func (k KVStore) Cdc() codec.BinaryCodec {
	return k.cdc
}

// GetVersion return the current version
func (k KVStore) GetVersion() semver.Version {
	return k.version
}

func (k *KVStore) SetVersion(ver semver.Version) {
	k.version = ver
}

// GetKey return a key that can be used to store into key value store
func (k KVStore) GetKey(prefix types.DbPrefix, key string, other ...string) []byte {
	newKey := fmt.Sprintf("%s/%s", prefix, strings.ToUpper(key))

	// TODO: should this handle the penaltyes automatically?
	// ref: x/thornado/keeper/v1/keeper_last_height.go#GetLastObserveHeight
	for _, item := range other {
		newKey += strings.ToUpper(item)
	}

	return []byte(newKey)
}

// getIterator - get an iterator for given prefix
func (k KVStore) getIterator(ctx cosmos.Context, prefix types.DbPrefix) cosmos.Iterator {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return cosmos.KVStorePrefixIterator(store, []byte(prefix))
}

func (k KVStore) DeleteKey(ctx cosmos.Context, key []byte) {
	k.del(ctx, key)
}

// del - delete data from the kvstore
func (k KVStore) del(ctx cosmos.Context, key []byte) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if store.Has(key) {
		store.Delete(key)
	}
}

// has - kvstore has key
func (k KVStore) has(ctx cosmos.Context, key []byte) bool {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return store.Has(key)
}

func (k KVStore) setInt64(ctx cosmos.Context, key []byte, record int64) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	value := ProtoInt64{Value: record}
	buf := k.cdc.MustMarshal(&value)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getInt64(ctx cosmos.Context, key []byte, record *int64) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	value := ProtoInt64{}
	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, &value); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	*record = value.GetValue()
	return true, nil
}

func (k KVStore) setUint64(ctx cosmos.Context, key []byte, record uint64) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	value := ProtoUint64{Value: record}
	buf := k.cdc.MustMarshal(&value)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getUint64(ctx cosmos.Context, key []byte, record *uint64) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	value := ProtoUint64{Value: *record}
	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, &value); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	*record = value.GetValue()
	return true, nil
}

func (k KVStore) setAccAddresses(ctx cosmos.Context, key []byte, record []cosmos.AccAddress) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	value := ProtoAccAddresses{Value: record}
	buf := k.cdc.MustMarshal(&value)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getAccAddresses(ctx cosmos.Context, key []byte, record *[]cosmos.AccAddress) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	var value ProtoAccAddresses
	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, &value); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	*record = value.Value
	return true, nil
}

func (k KVStore) setString(ctx cosmos.Context, key, record string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	value := ProtoString{Value: record}
	buf := k.cdc.MustMarshal(&value)
	if buf == nil {
		store.Delete([]byte(key))
	} else {
		store.Set([]byte(key), buf)
	}
}

func (k KVStore) getString(ctx cosmos.Context, key string, record *string) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has([]byte(key)) {
		return false, nil
	}

	var value ProtoString
	bz := store.Get([]byte(key))
	if err := k.cdc.Unmarshal(bz, &value); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	*record = value.Value
	return true, nil
}

func (k KVStore) setStrings(ctx cosmos.Context, key []byte, record []string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if len(record) == 0 {
		store.Delete(key)
		return
	}
	value := ProtoStrings{Value: record}
	buf := k.cdc.MustMarshal(&value)
	store.Set(key, buf)
}

func (k KVStore) getStrings(ctx cosmos.Context, key []byte, record *[]string) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	var value ProtoStrings
	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, &value); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	*record = value.Value
	return true, nil
}

func (k KVStore) setUint(ctx cosmos.Context, key []byte, record cosmos.Uint) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	value := ProtoUint{Value: record}
	buf := k.cdc.MustMarshal(&value)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getUint(ctx cosmos.Context, key []byte, record *cosmos.Uint) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	var value ProtoUint
	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, &value); err != nil {
		return false, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	*record = value.Value
	return true, nil
}

func (k KVStore) GetBalanceOfModule(ctx cosmos.Context, moduleName, denom string) cosmos.Uint {
	addr := k.accountKeeper.GetModuleAddress(moduleName)
	coin := k.coinKeeper.GetBalance(ctx, addr, denom)
	return cosmos.NewUintFromBigInt(coin.Amount.BigInt())
}

// SendFromModuleToModule transfer asset from one module to another
func (k KVStore) SendFromModuleToModule(ctx cosmos.Context, from, to string, coins common.Coins) error {
	cosmosCoins := make(cosmos.Coins, len(coins))
	for i, c := range coins {
		cosmosCoins[i] = cosmos.NewCoin(c.Asset.Native(), cosmos.NewIntFromBigInt(c.Amount.BigInt()))
	}
	return k.coinKeeper.SendCoinsFromModuleToModule(ctx, from, to, cosmosCoins)
}

func (k KVStore) SendCoins(ctx cosmos.Context, from, to cosmos.AccAddress, coins cosmos.Coins) error {
	return k.coinKeeper.SendCoins(ctx, from, to, coins)
}

// SendFromAccountToModule transfer fund from one account to a module
func (k KVStore) SendFromAccountToModule(ctx cosmos.Context, from cosmos.AccAddress, to string, coins common.Coins) error {
	cosmosCoins := make(cosmos.Coins, len(coins))
	for i, c := range coins {
		cosmosCoins[i] = cosmos.NewCoin(c.Asset.Native(), cosmos.NewIntFromBigInt(c.Amount.BigInt()))
	}
	return k.coinKeeper.SendCoinsFromAccountToModule(ctx, from, to, cosmosCoins)
}

// SendFromModuleToAccount transfer fund from module to an account
func (k KVStore) SendFromModuleToAccount(ctx cosmos.Context, from string, to cosmos.AccAddress, coins common.Coins) error {
	cosmosCoins := make(cosmos.Coins, len(coins))
	for i, c := range coins {
		cosmosCoins[i] = cosmos.NewCoin(c.Asset.Native(), cosmos.NewIntFromBigInt(c.Amount.BigInt()))
	}
	return k.coinKeeper.SendCoinsFromModuleToAccount(ctx, from, to, cosmosCoins)
}

func (k KVStore) BurnFromModule(ctx cosmos.Context, module string, coin common.Coin) error {
	coinToBurn, err := coin.Native()
	if err != nil {
		return fmt.Errorf("fail to parse coins: %w", err)
	}
	coinsToBurn := cosmos.Coins{coinToBurn}
	err = k.coinKeeper.BurnCoins(ctx, module, coinsToBurn)
	if err != nil {
		return fmt.Errorf("fail to burn assets: %w", err)
	}

	return nil
}

func (k KVStore) MintToModule(ctx cosmos.Context, module string, coin common.Coin) error {
	if coin.Amount.IsZero() {
		return nil
	}

	coinToMint, err := coin.Native()
	if err != nil {
		return fmt.Errorf("fail to parse coins: %w", err)
	}
	coinsToMint := cosmos.Coins{coinToMint}
	err = k.coinKeeper.MintCoins(ctx, module, coinsToMint)
	if err != nil {
		return fmt.Errorf("fail to mint assets: %w", err)
	}

	return nil
}

func (k KVStore) MintAndSendToAccount(ctx cosmos.Context, to cosmos.AccAddress, coin common.Coin) error {
	// Mint coins into the reserve
	if err := k.MintToModule(ctx, ModuleName, coin); err != nil {
		return err
	}
	return k.SendFromModuleToAccount(ctx, ModuleName, to, common.NewCoins(coin))
}

func (k KVStore) GetModuleAddress(module string) (common.Address, error) {
	return common.NewAddress(k.accountKeeper.GetModuleAddress(module).String())
}

func (k KVStore) GetModuleAccAddress(module string) cosmos.AccAddress {
	return k.accountKeeper.GetModuleAddress(module)
}

func (k KVStore) GetBalance(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Coins {
	return k.coinKeeper.GetAllBalances(ctx, addr)
}

func (k KVStore) GetBalanceOf(ctx cosmos.Context, addr cosmos.AccAddress, asset common.Asset) cosmos.Coin {
	return k.coinKeeper.GetBalance(ctx, addr, asset.Native())
}

func (k KVStore) HasCoins(ctx cosmos.Context, addr cosmos.AccAddress, coins cosmos.Coins) bool {
	balance := k.coinKeeper.GetAllBalances(ctx, addr)
	return balance.IsAllGTE(coins)
}

func (k KVStore) GetAccount(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Account {
	return k.accountKeeper.GetAccount(ctx, addr)
}

func (k KVStore) GetNativeTxFee(ctx cosmos.Context) cosmos.Uint {
	return cosmos.ZeroUint()
}

func (k KVStore) DeductNativeTxFeeFromAccount(ctx cosmos.Context, acctAddr cosmos.AccAddress) error {
	return nil
}
