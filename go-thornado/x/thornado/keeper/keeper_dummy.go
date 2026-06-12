package keeper

import (
	"errors"
	"fmt"
	"strings"

	"cosmossdk.io/log"
	upgradetypes "cosmossdk.io/x/upgrade/types"

	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	kvTypes "github.com/thornadocash/go-thornado/x/thornado/keeper/types"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

var kaboom = errors.New("Kaboom!!!")

type KVStoreDummy struct{}

func (k KVStoreDummy) Cdc() codec.BinaryCodec                  { return testutil.MakeTestEncodingConfig().Codec }
func (k KVStoreDummy) DeleteKey(_ cosmos.Context, _ []byte)    {}
func (k KVStoreDummy) CoinKeeper() bankkeeper.Keeper           { return bankkeeper.BaseKeeper{} }
func (k KVStoreDummy) AccountKeeper() authkeeper.AccountKeeper { return authkeeper.AccountKeeper{} }
func (k KVStoreDummy) Logger(ctx cosmos.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", ModuleName))
}

func (k KVStoreDummy) GetVersion() semver.Version { return semver.MustParse("9999999.0.0") }
func (k KVStoreDummy) GetVersionWithCtx(ctx cosmos.Context) (semver.Version, bool) {
	return semver.MustParse("9999999.0.0"), true
}
func (k KVStoreDummy) SetVersionWithCtx(ctx cosmos.Context, v semver.Version) {}
func (k KVStoreDummy) GetMinJoinLast(ctx cosmos.Context) (semver.Version, int64) {
	return k.GetMinJoinVersion(ctx), 0
}
func (k KVStoreDummy) SetMinJoinLast(ctx cosmos.Context) {}

func (k KVStoreDummy) ProposeUpgrade(ctx cosmos.Context, name string, upgrade types.UpgradeProposal) error {
	return kaboom
}

func (k KVStoreDummy) GetProposedUpgrade(ctx cosmos.Context, name string) (*types.UpgradeProposal, error) {
	return nil, kaboom
}

func (k KVStoreDummy) GetUpgradeVote(_ cosmos.Context, _ cosmos.AccAddress, _ string) (bool, error) {
	return false, kaboom
}

func (k KVStoreDummy) ApproveUpgrade(ctx cosmos.Context, addr cosmos.AccAddress, name string) {
	panic(kaboom)
}

func (k KVStoreDummy) RejectUpgrade(ctx cosmos.Context, addr cosmos.AccAddress, name string) {
	panic(kaboom)
}

func (k KVStoreDummy) GetUpgradePlan(ctx cosmos.Context) (upgradetypes.Plan, error) {
	return upgradetypes.Plan{}, nil
}

func (k KVStoreDummy) ScheduleUpgrade(ctx cosmos.Context, plan upgradetypes.Plan) error {
	return kaboom
}

func (k KVStoreDummy) GetUpgradeProposalIterator(_ cosmos.Context) cosmos.Iterator {
	return nil
}

func (k KVStoreDummy) GetUpgradeVoteIterator(_ cosmos.Context, _ string) cosmos.Iterator {
	return nil
}

func (k KVStoreDummy) RemoveExpiredUpgradeProposals(_ cosmos.Context) error {
	return nil
}

func (k KVStoreDummy) ClearUpgradePlan(ctx cosmos.Context) error { return nil }

func (k KVStoreDummy) GetKey(prefix kvTypes.DbPrefix, key string, other ...string) []byte {
	s := fmt.Sprintf("%s/1/%s", prefix, key)
	for _, o := range other {
		s += o
	}
	return []byte(s)
}

func (k KVStoreDummy) GetBalanceOfModule(ctx cosmos.Context, moduleName, denom string) cosmos.Uint {
	return cosmos.ZeroUint()
}

func (k KVStoreDummy) SendFromModuleToModule(ctx cosmos.Context, from, to string, coins common.Coins) error {
	return kaboom
}

func (k KVStoreDummy) SendCoins(ctx cosmos.Context, from, to cosmos.AccAddress, coins cosmos.Coins) error {
	return kaboom
}

func (k KVStoreDummy) SendFromAccountToModule(ctx cosmos.Context, from cosmos.AccAddress, to string, coins common.Coins) error {
	return kaboom
}

func (k KVStoreDummy) SendFromModuleToAccount(ctx cosmos.Context, from string, to cosmos.AccAddress, coins common.Coins) error {
	return kaboom
}

func (k KVStoreDummy) MintToModule(ctx cosmos.Context, module string, coin common.Coin) error {
	return kaboom
}

func (k KVStoreDummy) BurnFromModule(ctx cosmos.Context, module string, coin common.Coin) error {
	return kaboom
}

func (k KVStoreDummy) MintAndSendToAccount(ctx cosmos.Context, to cosmos.AccAddress, coin common.Coin) error {
	return kaboom
}

func (k KVStoreDummy) GetModuleAddress(module string) (common.Address, error) {
	if module == ReserveName {
		return "tthor1dheycdevq39qlkxs2a6wuuzyn4aqxhve3hhmlw", nil // Mocknet Reserve address
	}
	return "", kaboom
}

func (k KVStoreDummy) GetModuleAccAddress(module string) cosmos.AccAddress {
	return nil
}

func (k KVStoreDummy) GetAccount(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Account {
	return nil
}

func (k KVStoreDummy) GetBalance(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Coins {
	return nil
}

func (k KVStoreDummy) GetBalanceOf(ctx cosmos.Context, addr cosmos.AccAddress, asset common.Asset) cosmos.Coin {
	native, _ := common.NewCoin(asset, cosmos.ZeroUint()).Native()
	return native
}

func (k KVStoreDummy) HasCoins(ctx cosmos.Context, addr cosmos.AccAddress, coins cosmos.Coins) bool {
	return false
}

func (k KVStoreDummy) SetLastSignedHeight(_ cosmos.Context, _ int64) error { return kaboom }
func (k KVStoreDummy) GetLastSignedHeight(_ cosmos.Context) (int64, error) {
	return 0, kaboom
}

func (k KVStoreDummy) SetLastChainHeight(_ cosmos.Context, _ common.Chain, _ int64) error {
	return kaboom
}

func (k KVStoreDummy) ForceSetLastChainHeight(_ cosmos.Context, _ common.Chain, _ int64) {}

func (k KVStoreDummy) GetLastChainHeight(_ cosmos.Context, _ common.Chain) (int64, error) {
	return 0, kaboom
}

func (k KVStoreDummy) GetLastChainHeights(ctx cosmos.Context) (map[common.Chain]int64, error) {
	return nil, kaboom
}

func (k KVStoreDummy) TotalActiveNodes(_ cosmos.Context) (int, error) { return 0, kaboom }
func (k KVStoreDummy) ListNodesWithBond(_ cosmos.Context) (NodeAccounts, error) {
	return nil, kaboom
}

func (k KVStoreDummy) ListNodesByStatus(_ cosmos.Context, _ NodeStatus) (NodeAccounts, error) {
	return nil, kaboom
}

func (k KVStoreDummy) ListActiveNodes(_ cosmos.Context) (NodeAccounts, error) {
	return nil, kaboom
}

func (k KVStoreDummy) GetLowestActiveVersion(_ cosmos.Context) semver.Version {
	return semver.Version{
		Major: 0,
		Minor: 1,
		Patch: 0,
	}
}
func (k KVStoreDummy) GetMinJoinVersion(_ cosmos.Context) semver.Version { return semver.Version{} }
func (k KVStoreDummy) GetNodeAccount(_ cosmos.Context, _ cosmos.AccAddress) (NodeAccount, error) {
	return NodeAccount{}, kaboom
}

func (k KVStoreDummy) GetNodeAccountByPubKey(_ cosmos.Context, _ common.PubKey) (NodeAccount, error) {
	return NodeAccount{}, kaboom
}

func (k KVStoreDummy) SetNodeAccount(_ cosmos.Context, _ NodeAccount) error { return kaboom }
func (k KVStoreDummy) EnsureNodeKeysUnique(_ cosmos.Context, _ cosmos.AccAddress, _ string, _ common.PubKeySet) error {
	return kaboom
}
func (k KVStoreDummy) GetNodeAccountIterator(_ cosmos.Context) cosmos.Iterator { return nil }

func (k KVStoreDummy) GetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress) (int64, error) {
	return 0, kaboom
}
func (k KVStoreDummy) SetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) {}
func (k KVStoreDummy) ResetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress)        {}
func (k KVStoreDummy) IncNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) error {
	return kaboom
}

func (k KVStoreDummy) DecNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) error {
	return kaboom
}

func (k KVStoreDummy) GetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress) (Jail, error) {
	return Jail{}, kaboom
}

func (k KVStoreDummy) SetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress, height int64, reason string) error {
	return kaboom
}

func (k KVStoreDummy) ReleaseNodeAccountFromJail(ctx cosmos.Context, addr cosmos.AccAddress) error {
	return kaboom
}
func (k KVStoreDummy) GetObservingAddresses(_ cosmos.Context) ([]cosmos.AccAddress, error) {
	return nil, kaboom
}

func (k KVStoreDummy) AddObservingAddresses(_ cosmos.Context, _ []cosmos.AccAddress) error {
	return kaboom
}
func (k KVStoreDummy) ClearObservingAddresses(_ cosmos.Context)                      {}
func (k KVStoreDummy) SetObservedTxInVoter(_ cosmos.Context, _ ObservedTxVoter)      {}
func (k KVStoreDummy) GetObservedTxInVoterIterator(_ cosmos.Context) cosmos.Iterator { return nil }
func (k KVStoreDummy) GetObservedTxInVoter(_ cosmos.Context, _ common.TxID) (ObservedTxVoter, error) {
	return ObservedTxVoter{}, kaboom
}
func (k KVStoreDummy) SetObservedTxOutVoter(_ cosmos.Context, _ ObservedTxVoter)      {}
func (k KVStoreDummy) GetObservedTxOutVoterIterator(_ cosmos.Context) cosmos.Iterator { return nil }
func (k KVStoreDummy) GetObservedTxOutVoter(_ cosmos.Context, _ common.TxID) (ObservedTxVoter, error) {
	return ObservedTxVoter{}, kaboom
}
func (k KVStoreDummy) SetObservedLink(ctx cosmos.Context, _, _ common.TxID) {}
func (k KVStoreDummy) GetObservedLink(ctx cosmos.Context, inhash common.TxID) []common.TxID {
	return nil
}
func (k KVStoreDummy) SetFrostVoter(_ cosmos.Context, _ FrostVoter)             {}
func (k KVStoreDummy) GetFrostVoterIterator(_ cosmos.Context) cosmos.Iterator { return nil }
func (k KVStoreDummy) GetFrostVoter(_ cosmos.Context, _ string) (FrostVoter, error) {
	return FrostVoter{}, kaboom
}

func (k KVStoreDummy) GetKeygenBlock(_ cosmos.Context, _ int64) (KeygenBlock, error) {
	return KeygenBlock{}, kaboom
}
func (k KVStoreDummy) SetKeygenBlock(_ cosmos.Context, _ KeygenBlock)          {}
func (k KVStoreDummy) GetKeygenBlockIterator(_ cosmos.Context) cosmos.Iterator { return nil }
func (k KVStoreDummy) GetTxOut(_ cosmos.Context, _ int64) (*TxOut, error)      { return nil, kaboom }
func (k KVStoreDummy) GetTxOutValue(_ cosmos.Context, _ int64) (cosmos.Uint, cosmos.Uint, error) {
	return cosmos.ZeroUint(), cosmos.ZeroUint(), kaboom
}

func (k KVStoreDummy) GetTOIsValue(_ cosmos.Context, _ ...TxOutItem) (cosmos.Uint, cosmos.Uint) {
	return cosmos.ZeroUint(), cosmos.ZeroUint()
}
func (k KVStoreDummy) SetTxOut(_ cosmos.Context, _ *TxOut) error                { return kaboom }
func (k KVStoreDummy) AppendTxOut(_ cosmos.Context, _ int64, _ TxOutItem) error { return kaboom }
func (k KVStoreDummy) ClearTxOut(_ cosmos.Context, _ int64) error               { return kaboom }
func (k KVStoreDummy) GetTxOutIterator(_ cosmos.Context) cosmos.Iterator        { return nil }
func (k KVStoreDummy) GetNextShielderNodeBondSlot(_ cosmos.Context) (uint64, error) {
	return 0, kaboom
}
func (k KVStoreDummy) SetNextShielderNodeBondSlot(_ cosmos.Context, _ uint64) error {
	return kaboom
}
func (k KVStoreDummy) AllocateShielderNodeBondSlot(_ cosmos.Context) (uint64, error) {
	return 0, kaboom
}
func (k KVStoreDummy) SetShielderNodeBond(_ cosmos.Context, _ types.ShielderNodeBond) error {
	return kaboom
}
func (k KVStoreDummy) GetShielderNodeBond(_ cosmos.Context, _ string) (types.ShielderNodeBond, error) {
	return types.ShielderNodeBond{}, kaboom
}
func (k KVStoreDummy) GetShielderNodeBondIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) SetFeePool(_ cosmos.Context, _ types.FeePool) error {
	return kaboom
}
func (k KVStoreDummy) GetFeePool(_ cosmos.Context) (types.FeePool, error) {
	return types.FeePool{}, kaboom
}
func (k KVStoreDummy) SetShielderFeeNotePubKey(_ cosmos.Context, _ string) error {
	return kaboom
}
func (k KVStoreDummy) ShielderFeeNotePubKeyUsed(_ cosmos.Context, _ string) bool {
	return false
}

func (k KVStoreDummy) SetNodeSlotAuction(_ cosmos.Context, _ types.NodeSlotAuction) error {
	return nil
}

func (k KVStoreDummy) GetNodeSlotAuction(_ cosmos.Context, _ string) (types.NodeSlotAuction, error) {
	return types.NodeSlotAuction{}, kaboom
}
func (k KVStoreDummy) GetNodeSlotAuctionIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}

func (k KVStoreDummy) SetNodeSlotBid(_ cosmos.Context, _ types.NodeSlotBid) error {
	return nil
}

func (k KVStoreDummy) GetNodeSlotBid(_ cosmos.Context, _ string) (types.NodeSlotBid, error) {
	return types.NodeSlotBid{}, kaboom
}
func (k KVStoreDummy) GetNodeSlotBidIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) GetChains(_ cosmos.Context) (common.Chains, error) { return nil, kaboom }
func (k KVStoreDummy) SetChains(_ cosmos.Context, _ common.Chains)       {}

func (k KVStoreDummy) GetVaultIterator(_ cosmos.Context) cosmos.Iterator  { return nil }
func (k KVStoreDummy) VaultExists(_ cosmos.Context, _ common.PubKey) bool { return false }
func (k KVStoreDummy) FindPubKeyOfAddress(_ cosmos.Context, _ common.Address, _ common.Chain) (common.PubKey, error) {
	return common.EmptyPubKey, kaboom
}
func (k KVStoreDummy) SetVault(_ cosmos.Context, _ Vault) error { return kaboom }
func (k KVStoreDummy) GetVault(_ cosmos.Context, _ common.PubKey) (Vault, error) {
	return Vault{}, kaboom
}
func (k KVStoreDummy) GetBaseVaults(_ cosmos.Context) (Vaults, error) { return nil, kaboom }
func (k KVStoreDummy) GetBaseVaultsByStatus(_ cosmos.Context, _ VaultStatus) (Vaults, error) {
	return nil, kaboom
}

func (k KVStoreDummy) RemoveFromBaseIndex(ctx cosmos.Context, pubkey common.PubKey) error {
	return kaboom
}

func (k KVStoreDummy) GetLeastSecure(_ cosmos.Context, _ Vaults, _ int64) Vault      { return Vault{} }
func (k KVStoreDummy) GetMostSecure(_ cosmos.Context, _ Vaults, _ int64) Vault       { return Vault{} }
func (k KVStoreDummy) GetMostSecureStrict(_ cosmos.Context, _ Vaults, _ int64) Vault { return Vault{} }
func (k KVStoreDummy) SortBySecurity(_ cosmos.Context, _ Vaults, _ int64) Vaults     { return nil }
func (k KVStoreDummy) GetPendingOutbounds(_ cosmos.Context, _ common.Asset) []TxOutItem {
	return nil
}
func (k KVStoreDummy) DeleteVault(_ cosmos.Context, _ common.PubKey) error { return kaboom }

func (k KVStoreDummy) GetNetwork(_ cosmos.Context) (Network, error) { return Network{}, kaboom }
func (k KVStoreDummy) SetNetwork(_ cosmos.Context, _ Network) error { return kaboom }

func (k KVStoreDummy) SetFrostKeysignFailVoter(_ cosmos.Context, frost FrostKeysignFailVoter) {
}

func (k KVStoreDummy) GetFrostKeysignFailVoterIterator(_ cosmos.Context) cosmos.Iterator {
	return nil
}

func (k KVStoreDummy) GetFrostKeysignFailVoter(_ cosmos.Context, _ string) (FrostKeysignFailVoter, error) {
	return FrostKeysignFailVoter{}, kaboom
}

func (k KVStoreDummy) GetGas(_ cosmos.Context, _ common.Asset) ([]cosmos.Uint, error) {
	return nil, kaboom
}
func (k KVStoreDummy) SetGas(_ cosmos.Context, _ common.Asset, _ []cosmos.Uint) {}
func (k KVStoreDummy) GetGasIterator(ctx cosmos.Context) cosmos.Iterator        { return nil }

func (k KVStoreDummy) SetErrataTxVoter(_ cosmos.Context, _ ErrataTxVoter)        {}
func (k KVStoreDummy) GetErrataTxVoterIterator(_ cosmos.Context) cosmos.Iterator { return nil }
func (k KVStoreDummy) GetErrataTxVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (ErrataTxVoter, error) {
	return ErrataTxVoter{}, kaboom
}
func (k KVStoreDummy) GetConfig(_ cosmos.Context, key string) (int64, error) { return 0, kaboom }
func (k KVStoreDummy) GetConfigWithRef(_ cosmos.Context, template string, key ...any) (int64, error) {
	return 0, kaboom
}
func (k KVStoreDummy) SetConfig(_ cosmos.Context, key string, value int64) {}
func (k KVStoreDummy) GetNodeConfigs(ctx cosmos.Context, key string) (NodeConfigs, error) {
	return NodeConfigs{}, kaboom
}

func (k KVStoreDummy) SetNodeConfig(_ cosmos.Context, key string, value int64, acc cosmos.AccAddress) error {
	return kaboom
}
func (k KVStoreDummy) DeleteNodeConfigs(_ cosmos.Context, key string)           {}
func (k KVStoreDummy) PurgeOperationalNodeConfigs(_ cosmos.Context)             {}
func (k KVStoreDummy) DeleteConfig(_ cosmos.Context, key string) error          { return kaboom }
func (k KVStoreDummy) GetConfigIterator(ctx cosmos.Context) cosmos.Iterator     { return nil }
func (k KVStoreDummy) GetNodeConfigIterator(ctx cosmos.Context) cosmos.Iterator { return nil }
func (k KVStoreDummy) GetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress) int64 {
	return int64(-1)
}
func (k KVStoreDummy) SetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress) {}
func (k KVStoreDummy) IsOperationalConfig(key string) bool {
	key = strings.ToUpper(key)
	// Simplified representation.
	return strings.Contains(key, "HALT") || strings.Contains(key, "PAUSE")
}

func (k KVStoreDummy) GetNetworkFee(ctx cosmos.Context, chain common.Chain) (NetworkFee, error) {
	return NetworkFee{}, kaboom
}

func (k KVStoreDummy) SaveNetworkFee(ctx cosmos.Context, chain common.Chain, networkFee NetworkFee) error {
	return kaboom
}

func (k KVStoreDummy) GetNetworkFeeIterator(ctx cosmos.Context) cosmos.Iterator {
	return nil
}

func (k KVStoreDummy) SetObservedNetworkFeeVoter(ctx cosmos.Context, networkFeeVoter ObservedNetworkFeeVoter) {
}

func (k KVStoreDummy) GetObservedNetworkFeeVoterIterator(ctx cosmos.Context) cosmos.Iterator {
	return nil
}

func (k KVStoreDummy) GetObservedNetworkFeeVoter(ctx cosmos.Context, height int64, chain common.Chain, rate, size int64) (ObservedNetworkFeeVoter, error) {
	return ObservedNetworkFeeVoter{}, nil
}

func (k KVStoreDummy) SetLastObserveHeight(ctx cosmos.Context, chain common.Chain, address cosmos.AccAddress, height int64) error {
	return kaboom
}

func (k KVStoreDummy) ForceSetLastObserveHeight(ctx cosmos.Context, chain common.Chain, address cosmos.AccAddress, height int64) {
}

func (k KVStoreDummy) GetLastObserveHeight(ctx cosmos.Context, address cosmos.AccAddress) (map[common.Chain]int64, error) {
	return nil, kaboom
}

func (k KVStoreDummy) SetFrostKeygenMetric(_ cosmos.Context, metric *FrostKeygenMetric) {
}

func (k KVStoreDummy) GetFrostKeygenMetric(_ cosmos.Context, key common.PubKey) (*FrostKeygenMetric, error) {
	return nil, kaboom
}

func (k KVStoreDummy) SetFrostKeysignMetric(_ cosmos.Context, metric *FrostKeysignMetric) {
}

func (k KVStoreDummy) GetFrostKeysignMetric(_ cosmos.Context, txID common.TxID) (*FrostKeysignMetric, error) {
	return nil, kaboom
}

func (k KVStoreDummy) GetLatestFrostKeysignMetric(_ cosmos.Context) (*FrostKeysignMetric, error) {
	return nil, kaboom
}
func (k KVStoreDummy) SetSolvencyVoter(_ cosmos.Context, _ SolvencyVoter) {}
func (k KVStoreDummy) GetSolvencyVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (SolvencyVoter, error) {
	return SolvencyVoter{}, kaboom
}

func (k KVStoreDummy) InvariantRoutes() []common.InvariantRoute {
	return nil
}

func (k KVStoreDummy) GetConstants() constants.ConfigValues {
	return constants.GetConfigValues(semver.MustParse("9999999.0.0"))
}

func (k KVStoreDummy) GetConfigInt64(ctx cosmos.Context, key constants.ConfigName) int64 {
	return -1
}

func (k KVStoreDummy) IsChainHalted(ctx cosmos.Context, chain common.Chain) bool { return false }

func (k KVStoreDummy) GetAnchors(ctx cosmos.Context, asset common.Asset) []common.Asset { return nil }
func (k KVStoreDummy) AnchorMedian(ctx cosmos.Context, assets []common.Asset) cosmos.Uint {
	return cosmos.ZeroUint()
}
func (k KVStoreDummy) GetNativeTxFee(ctx cosmos.Context) cosmos.Uint {
	return cosmos.ZeroUint()
}

func (k KVStoreDummy) GetOutboundTxFee(ctx cosmos.Context) cosmos.Uint {
	return cosmos.ZeroUint()
}

func (k KVStoreDummy) DeductNativeTxFeeFromAccount(ctx cosmos.Context, acctAddr cosmos.AccAddress) error {
	return nil
}

func (k KVStoreDummy) RemoveLowBondNodeAccounts(ctx cosmos.Context) error {
	return kaboom
}

func (k KVStoreDummy) SetPrice(_ cosmos.Context, _ OraclePrice) error {
	return nil
}

func (k KVStoreDummy) GetPrice(_ cosmos.Context, _ string) (OraclePrice, error) {
	return OraclePrice{}, nil
}
func (k KVStoreDummy) DelPrice(_ cosmos.Context, _ string) {}
func (k KVStoreDummy) GetPriceIterator(_ cosmos.Context) cosmos.Iterator {
	return nil
}

func (k KVStoreDummy) SetDepositSession(_ cosmos.Context, _ types.DepositSession) error { return nil }
func (k KVStoreDummy) GetDepositSession(_ cosmos.Context, _ cosmos.AccAddress) (types.DepositSession, error) {
	return types.DepositSession{}, nil
}
func (k KVStoreDummy) GetDepositSessionByPowToken(_ cosmos.Context, _ string) (types.DepositSession, error) {
	return types.DepositSession{}, nil
}
func (k KVStoreDummy) SetDepositPowTiming(_ cosmos.Context, _ types.DepositPowTiming) error {
	return nil
}
func (k KVStoreDummy) GetDepositPowTiming(_ cosmos.Context, _ string) (types.DepositPowTiming, error) {
	return types.DepositPowTiming{}, nil
}
func (k KVStoreDummy) GetDepositPowTimingIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) SetDepositPowDifficultyState(_ cosmos.Context, _ types.DepositPowDifficultyState) error {
	return nil
}
func (k KVStoreDummy) GetDepositPowDifficultyState(_ cosmos.Context) (types.DepositPowDifficultyState, error) {
	return types.DepositPowDifficultyState{}, nil
}
func (k KVStoreDummy) SetDepositAddress(_ cosmos.Context, _ types.DepositAddress) error {
	return nil
}
func (k KVStoreDummy) GetDepositAddress(_ cosmos.Context, _ common.Address) (types.DepositAddress, error) {
	return types.DepositAddress{}, nil
}
func (k KVStoreDummy) DeleteDepositAddress(_ cosmos.Context, _ common.Address) error {
	return nil
}
func (k KVStoreDummy) GetDepositAddressIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) GetNextVaultDepositPathIndex(_ cosmos.Context, _ common.PubKey, _ common.VaultDepositPathType) (uint64, error) {
	return 0, nil
}
func (k KVStoreDummy) SetNextVaultDepositPathIndex(_ cosmos.Context, _ common.PubKey, _ common.VaultDepositPathType, _ uint64) error {
	return nil
}
func (k KVStoreDummy) AllocateVaultDepositPathIndex(_ cosmos.Context, _ common.PubKey, pathType common.VaultDepositPathType) (uint64, uint64, error) {
	pathIndex, err := common.VaultDepositPathIndex(pathType, 0, common.DepositPathCommitmentRoot)
	return 0, pathIndex, err
}
func (k KVStoreDummy) SetDepositRecord(_ cosmos.Context, _ types.DepositRecord) error {
	return nil
}
func (k KVStoreDummy) GetDepositRecord(_ cosmos.Context, _ common.TxID) (types.DepositRecord, error) {
	return types.DepositRecord{}, nil
}
func (k KVStoreDummy) GetDepositRecordIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) SetShielderCommitment(_ cosmos.Context, _ string) error {
	return nil
}
func (k KVStoreDummy) ShielderCommitmentExists(_ cosmos.Context, _ string) bool { return false }
func (k KVStoreDummy) SetShielderNoteRecord(_ cosmos.Context, _ types.StoredShielderNoteRecord) error {
	return nil
}
func (k KVStoreDummy) GetShielderNoteRecordIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) SetShielderDenominationCommitment(_ cosmos.Context, _ uint64, _ string) error {
	return nil
}
func (k KVStoreDummy) GetShielderDenominationCommitments(_ cosmos.Context, _ uint64) ([]string, error) {
	return nil, nil
}
func (k KVStoreDummy) SetShielderMerkleRoot(_ cosmos.Context, _ uint64, _ string) error {
	return nil
}
func (k KVStoreDummy) ShielderMerkleRootExists(_ cosmos.Context, _ uint64, _ string) bool {
	return false
}
func (k KVStoreDummy) GetShielderMerkleRootIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}
func (k KVStoreDummy) SetShielderRedeem(_ cosmos.Context, _ types.ShielderRedeem) error {
	return nil
}
func (k KVStoreDummy) GetShielderRedeem(_ cosmos.Context, _ string) (types.ShielderRedeem, error) {
	return types.ShielderRedeem{}, nil
}
func (k KVStoreDummy) GetShielderRedeemByNullifier(_ cosmos.Context, _ string) (types.ShielderRedeem, error) {
	return types.ShielderRedeem{}, nil
}
func (k KVStoreDummy) SetShielderNullifierSpent(_ cosmos.Context, _, _ string) error {
	return nil
}
func (k KVStoreDummy) ShielderNullifierSpent(_ cosmos.Context, _ string) bool { return false }
func (k KVStoreDummy) GetShielderNullifierIterator(_ cosmos.Context) cosmos.Iterator {
	return NewDummyIterator()
}

// a mock cosmos.Iterator implementation for testing purposes
type DummyIterator struct {
	cosmos.Iterator
	placeholder int
	keys        [][]byte
	values      [][]byte
	err         error
}

func NewDummyIterator() *DummyIterator {
	return &DummyIterator{
		keys:   make([][]byte, 0),
		values: make([][]byte, 0),
	}
}

func (iter *DummyIterator) AddItem(key, value []byte) {
	iter.keys = append(iter.keys, key)
	iter.values = append(iter.values, value)
}

func (iter *DummyIterator) Next() {
	iter.placeholder++
}

func (iter *DummyIterator) Valid() bool {
	return iter.placeholder < len(iter.keys)
}

func (iter *DummyIterator) Key() []byte {
	return iter.keys[iter.placeholder]
}

func (iter *DummyIterator) Value() []byte {
	return iter.values[iter.placeholder]
}

func (iter *DummyIterator) Close() error {
	iter.placeholder = 0
	return nil
}

func (iter *DummyIterator) Error() error {
	return iter.err
}

func (iter *DummyIterator) Domain() (start, end []byte) {
	return nil, nil
}
