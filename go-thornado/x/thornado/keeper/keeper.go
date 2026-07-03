package keeper

import (
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/codec"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	kvTypes "github.com/thornadocash/go-thornado/x/thornado/keeper/types"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type Keeper interface {
	Cdc() codec.BinaryCodec
	DeleteKey(ctx cosmos.Context, key []byte)
	GetVersion() semver.Version
	GetVersionWithCtx(ctx cosmos.Context) (semver.Version, bool)
	SetVersionWithCtx(ctx cosmos.Context, v semver.Version)
	GetMinJoinLast(ctx cosmos.Context) (semver.Version, int64)
	SetMinJoinLast(ctx cosmos.Context)
	GetKey(prefix kvTypes.DbPrefix, key string, other ...string) []byte
	GetBalanceOfModule(ctx cosmos.Context, moduleName, denom string) cosmos.Uint
	SendFromModuleToModule(ctx cosmos.Context, from, to string, coin common.Coins) error
	SendFromAccountToModule(ctx cosmos.Context, from cosmos.AccAddress, to string, coin common.Coins) error
	SendFromModuleToAccount(ctx cosmos.Context, from string, to cosmos.AccAddress, coin common.Coins) error
	MintToModule(ctx cosmos.Context, module string, coin common.Coin) error
	BurnFromModule(ctx cosmos.Context, module string, coin common.Coin) error
	MintAndSendToAccount(ctx cosmos.Context, to cosmos.AccAddress, coin common.Coin) error
	GetModuleAddress(module string) (common.Address, error)
	GetModuleAccAddress(module string) cosmos.AccAddress
	GetBalance(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Coins
	GetBalanceOf(ctx cosmos.Context, addr cosmos.AccAddress, asset common.Asset) cosmos.Coin
	HasCoins(ctx cosmos.Context, addr cosmos.AccAddress, coins cosmos.Coins) bool
	GetAccount(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Account

	// passthrough funcs
	SendCoins(ctx cosmos.Context, from, to cosmos.AccAddress, coins cosmos.Coins) error

	InvariantRoutes() []common.InvariantRoute

	GetConstants() constants.ConfigValues
	GetConfigInt64(ctx cosmos.Context, key constants.ConfigName) int64

	GetNativeTxFee(ctx cosmos.Context) cosmos.Uint

	DeductNativeTxFeeFromAccount(ctx cosmos.Context, acctAddr cosmos.AccAddress) error

	// Keeper Interfaces
	KeeperConfigDefaults
	KeeperLastHeight
	KeeperNodeAccount
	KeeperUpgrade
	KeeperObserver
	KeeperObservedTx
	KeeperTxOut
	KeeperOutboundFees
	KeeperVault
	KeeperNetwork
	KeeperFrost
	KeeperFrostKeysignFail
	KeeperKeygen
	KeeperErrataTx
	KeeperConfigStore
	KeeperNetworkFee
	KeeperObservedNetworkFeeVoter
	KeeperOracle
	KeeperSolvencyVoter
	KeeperHalt
	KeeperAnchors
	KeeperShielder
}

type KeeperConfigDefaults interface {
	GetConstants() constants.ConfigValues
	GetConfigInt64(ctx cosmos.Context, key constants.ConfigName) int64
}

type KeeperLastHeight interface {
	SetLastSignedHeight(ctx cosmos.Context, height int64) error
	GetLastSignedHeight(ctx cosmos.Context) (int64, error)
	SetLastChainHeight(ctx cosmos.Context, chain common.Chain, height int64) error
	ForceSetLastChainHeight(ctx cosmos.Context, chain common.Chain, height int64)
	GetLastChainHeight(ctx cosmos.Context, chain common.Chain) (int64, error)
	GetLastChainHeights(ctx cosmos.Context) (map[common.Chain]int64, error)
	SetLastObserveHeight(ctx cosmos.Context, chain common.Chain, address cosmos.AccAddress, height int64) error
	ForceSetLastObserveHeight(ctx cosmos.Context, chain common.Chain, address cosmos.AccAddress, height int64)
	GetLastObserveHeight(ctx cosmos.Context, address cosmos.AccAddress) (map[common.Chain]int64, error)
}

type KeeperNodeAccount interface {
	TotalActiveNodes(ctx cosmos.Context) (int, error)
	ListNodesWithBond(ctx cosmos.Context) (NodeAccounts, error)
	ListNodesByStatus(ctx cosmos.Context, status NodeStatus) (NodeAccounts, error)
	ListActiveNodes(ctx cosmos.Context) (NodeAccounts, error)
	GetLowestActiveVersion(ctx cosmos.Context) semver.Version
	GetMinJoinVersion(ctx cosmos.Context) semver.Version
	GetNodeAccount(ctx cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error)
	GetNodeAccountByPubKey(ctx cosmos.Context, pk common.PubKey) (NodeAccount, error)
	SetNodeAccount(ctx cosmos.Context, na NodeAccount) error
	EnsureNodeKeysUnique(ctx cosmos.Context, signer cosmos.AccAddress, consensusPubKey string, pubKeys common.PubKeySet) error
	GetNodeAccountIterator(ctx cosmos.Context) cosmos.Iterator
	GetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress) (int64, error)
	SetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64)
	IncNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) error
	DecNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) error
	ResetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress)
	GetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress) (Jail, error)
	SetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress, height int64, reason string) error
	ReleaseNodeAccountFromJail(ctx cosmos.Context, addr cosmos.AccAddress) error
	RemoveLowBondNodeAccounts(ctx cosmos.Context) error
}

type KeeperUpgrade interface {
	// mutative methods
	ProposeUpgrade(ctx cosmos.Context, name string, upgrade types.UpgradeProposal) error
	ApproveUpgrade(ctx cosmos.Context, addr cosmos.AccAddress, name string)
	RejectUpgrade(ctx cosmos.Context, addr cosmos.AccAddress, name string)
	RemoveExpiredUpgradeProposals(ctx cosmos.Context) error

	// query methods
	GetProposedUpgrade(ctx cosmos.Context, name string) (*types.UpgradeProposal, error)
	GetUpgradeVote(ctx cosmos.Context, addr cosmos.AccAddress, name string) (bool, error)
	GetUpgradeProposalIterator(ctx cosmos.Context) cosmos.Iterator
	GetUpgradeVoteIterator(ctx cosmos.Context, name string) cosmos.Iterator

	// x/upgrade module methods
	GetUpgradePlan(ctx cosmos.Context) (upgradetypes.Plan, error)
	ScheduleUpgrade(ctx cosmos.Context, plan upgradetypes.Plan) error
	ClearUpgradePlan(ctx cosmos.Context) error
}

type KeeperObserver interface {
	GetObservingAddresses(ctx cosmos.Context) ([]cosmos.AccAddress, error)
	AddObservingAddresses(ctx cosmos.Context, inAddresses []cosmos.AccAddress) error
	ClearObservingAddresses(ctx cosmos.Context)
}

type KeeperObservedTx interface {
	SetObservedTxInVoter(ctx cosmos.Context, tx ObservedTxVoter)
	GetObservedTxInVoterIterator(ctx cosmos.Context) cosmos.Iterator
	GetObservedTxInVoter(ctx cosmos.Context, hash common.TxID) (ObservedTxVoter, error)
	SetObservedTxOutVoter(ctx cosmos.Context, tx ObservedTxVoter)
	GetObservedTxOutVoterIterator(ctx cosmos.Context) cosmos.Iterator
	GetObservedTxOutVoter(ctx cosmos.Context, hash common.TxID) (ObservedTxVoter, error)
	SetObservedLink(ctx cosmos.Context, _, _ common.TxID)
	GetObservedLink(ctx cosmos.Context, inhash common.TxID) []common.TxID
}

type KeeperTxOut interface {
	SetTxOut(ctx cosmos.Context, blockOut *TxOut) error
	AppendTxOut(ctx cosmos.Context, height int64, item TxOutItem) error
	ClearTxOut(ctx cosmos.Context, height int64) error
	GetTxOutIterator(ctx cosmos.Context) cosmos.Iterator
	GetTxOut(ctx cosmos.Context, height int64) (*TxOut, error)
	GetTxOutValue(ctx cosmos.Context, height int64) (cosmos.Uint, cosmos.Uint, error)
	GetTOIsValue(ctx cosmos.Context, tois ...TxOutItem) (cosmos.Uint, cosmos.Uint)
}

type KeeperShielder interface {
	SetDepositSession(ctx cosmos.Context, session types.DepositSession) error
	GetDepositSession(ctx cosmos.Context, owner cosmos.AccAddress) (types.DepositSession, error)
	GetDepositSessionByPowToken(ctx cosmos.Context, powToken string) (types.DepositSession, error)
	SetDepositPowTiming(ctx cosmos.Context, record types.DepositPowTiming) error
	GetDepositPowTiming(ctx cosmos.Context, powToken string) (types.DepositPowTiming, error)
	GetDepositPowTimingIterator(ctx cosmos.Context) cosmos.Iterator
	SetDepositPowDifficultyState(ctx cosmos.Context, state types.DepositPowDifficultyState) error
	GetDepositPowDifficultyState(ctx cosmos.Context) (types.DepositPowDifficultyState, error)
	SetDepositAddress(ctx cosmos.Context, record types.DepositAddress) error
	GetDepositAddress(ctx cosmos.Context, address common.Address) (types.DepositAddress, error)
	DeleteDepositAddress(ctx cosmos.Context, address common.Address) error
	GetDepositAddressIterator(ctx cosmos.Context) cosmos.Iterator
	GetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType) (uint64, error)
	SetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType, index uint64) error
	AllocateVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType) (uint64, uint64, error)
	SetDepositRecord(ctx cosmos.Context, deposit types.DepositRecord) error
	GetDepositRecord(ctx cosmos.Context, depositID common.TxID) (types.DepositRecord, error)
	GetDepositRecordIterator(ctx cosmos.Context) cosmos.Iterator
	GetDepositRecordIteratorAfter(ctx cosmos.Context, cursor string) cosmos.Iterator
	SetShielderCommitment(ctx cosmos.Context, commitment string) error
	ShielderCommitmentExists(ctx cosmos.Context, commitment string) bool
	SetShielderNoteRecord(ctx cosmos.Context, record types.StoredShielderNoteRecord) error
	GetShielderNoteRecordIterator(ctx cosmos.Context) cosmos.Iterator
	GetShielderNoteRecordIteratorAfter(ctx cosmos.Context, cursor string) cosmos.Iterator
	SetShielderDenominationLeaf(ctx cosmos.Context, denominationSats, index uint64, commitment string) error
	GetShielderDenominationCommitments(ctx cosmos.Context, denominationSats uint64) ([]string, error)
	SetShielderTreeState(ctx cosmos.Context, state types.StoredShielderTreeState) error
	GetShielderTreeState(ctx cosmos.Context, denominationSats uint64) (types.StoredShielderTreeState, bool, error)
	PurgeShielderPoolState(ctx cosmos.Context)
	SetShielderMerkleRoot(ctx cosmos.Context, denominationSats uint64, root string) error
	ShielderMerkleRootExists(ctx cosmos.Context, denominationSats uint64, root string) bool
	GetShielderMerkleRootIterator(ctx cosmos.Context) cosmos.Iterator
	SetShielderRedeem(ctx cosmos.Context, withdrawal types.ShielderRedeem) error
	GetShielderRedeem(ctx cosmos.Context, withdrawalID string) (types.ShielderRedeem, error)
	GetShielderRedeemByNullifier(ctx cosmos.Context, nullifierHash string) (types.ShielderRedeem, error)
	SetShielderRedeemOutHash(ctx cosmos.Context, outHash, withdrawalID string) error
	GetShielderRedeemByOutHash(ctx cosmos.Context, outHash string) (types.ShielderRedeem, bool, error)
	DeleteShielderRedeemOutHash(ctx cosmos.Context, outHash string)
	SetShielderNullifierSpent(ctx cosmos.Context, nullifierHash string, withdrawalID string) error
	ShielderNullifierSpent(ctx cosmos.Context, nullifierHash string) bool
	GetShielderNullifierIterator(ctx cosmos.Context) cosmos.Iterator
	GetShielderNullifierIteratorAfter(ctx cosmos.Context, cursor string) cosmos.Iterator
	GetNextShielderNodeBondSlot(ctx cosmos.Context) (uint64, error)
	SetNextShielderNodeBondSlot(ctx cosmos.Context, slot uint64) error
	AllocateShielderNodeBondSlot(ctx cosmos.Context) (uint64, error)
	SetShielderNodeBond(ctx cosmos.Context, bond types.ShielderNodeBond) error
	GetShielderNodeBond(ctx cosmos.Context, nodePubKey string) (types.ShielderNodeBond, error)
	GetShielderNodeBondIterator(ctx cosmos.Context) cosmos.Iterator
	SetShielderNodeBonder(ctx cosmos.Context, bonder types.ShielderNodeBonder) error
	GetShielderNodeBonder(ctx cosmos.Context, nodePubKey string, bonder cosmos.AccAddress) (types.ShielderNodeBonder, error)
	GetShielderNodeBonderIterator(ctx cosmos.Context) cosmos.Iterator
	DeleteShielderNodeBonder(ctx cosmos.Context, nodePubKey string, bonder cosmos.AccAddress) error
	SetFeePool(ctx cosmos.Context, pool types.FeePool) error
	GetFeePool(ctx cosmos.Context) (types.FeePool, error)
	SetShielderFeeNotePubKey(ctx cosmos.Context, pubKey string) error
	ShielderFeeNotePubKeyUsed(ctx cosmos.Context, pubKey string) bool
	SetNodeSlotAuction(ctx cosmos.Context, auction types.NodeSlotAuction) error
	GetNodeSlotAuction(ctx cosmos.Context, auctionID string) (types.NodeSlotAuction, error)
	GetNodeSlotAuctionIterator(ctx cosmos.Context) cosmos.Iterator
	SetNodeSlotBid(ctx cosmos.Context, bid types.NodeSlotBid) error
	GetNodeSlotBid(ctx cosmos.Context, bidID string) (types.NodeSlotBid, error)
	GetNodeSlotBidIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperOutboundFees interface {
	GetOutboundTxFee(ctx cosmos.Context) cosmos.Uint
}

type KeeperVault interface {
	GetVaultIterator(ctx cosmos.Context) cosmos.Iterator
	VaultExists(ctx cosmos.Context, pk common.PubKey) bool
	SetVault(ctx cosmos.Context, vault Vault) error
	GetVault(ctx cosmos.Context, pk common.PubKey) (Vault, error)
	GetBaseVaults(ctx cosmos.Context) (Vaults, error)
	GetBaseVaultsByStatus(_ cosmos.Context, _ VaultStatus) (Vaults, error)
	GetLeastSecure(_ cosmos.Context, _ Vaults, _ int64) Vault
	GetMostSecure(_ cosmos.Context, _ Vaults, _ int64) Vault
	GetMostSecureStrict(_ cosmos.Context, _ Vaults, _ int64) Vault
	SortBySecurity(_ cosmos.Context, _ Vaults, _ int64) Vaults
	GetPendingOutbounds(_ cosmos.Context, _ common.Asset) []TxOutItem
	DeleteVault(ctx cosmos.Context, pk common.PubKey) error
	RemoveFromBaseIndex(ctx cosmos.Context, pubkey common.PubKey) error
}

// KeeperNetwork func to access network data in key value store
type KeeperNetwork interface {
	GetNetwork(ctx cosmos.Context) (Network, error)
	SetNetwork(ctx cosmos.Context, data Network) error
}

type KeeperFrost interface {
	SetFrostVoter(_ cosmos.Context, frost FrostVoter)
	GetFrostVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetFrostVoter(_ cosmos.Context, _ string) (FrostVoter, error)
	SetFrostKeygenMetric(_ cosmos.Context, metric *FrostKeygenMetric)
	GetFrostKeygenMetric(_ cosmos.Context, key common.PubKey) (*FrostKeygenMetric, error)
	SetFrostKeysignMetric(_ cosmos.Context, metric *FrostKeysignMetric)
	GetFrostKeysignMetric(_ cosmos.Context, txID common.TxID) (*FrostKeysignMetric, error)
	GetLatestFrostKeysignMetric(_ cosmos.Context) (*FrostKeysignMetric, error)
}

type KeeperFrostKeysignFail interface {
	SetFrostKeysignFailVoter(_ cosmos.Context, frost FrostKeysignFailVoter)
	GetFrostKeysignFailVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetFrostKeysignFailVoter(_ cosmos.Context, _ string) (FrostKeysignFailVoter, error)
}

type KeeperKeygen interface {
	SetKeygenBlock(ctx cosmos.Context, keygenBlock KeygenBlock)
	GetKeygenBlockIterator(ctx cosmos.Context) cosmos.Iterator
	GetKeygenBlock(ctx cosmos.Context, height int64) (KeygenBlock, error)
}

type KeeperErrataTx interface {
	SetErrataTxVoter(_ cosmos.Context, _ ErrataTxVoter)
	GetErrataTxVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetErrataTxVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (ErrataTxVoter, error)
}

type KeeperConfigStore interface {
	GetConfig(_ cosmos.Context, key string) (int64, error)
	GetConfigWithRef(_ cosmos.Context, template string, ref ...any) (int64, error)
	SetConfig(_ cosmos.Context, key string, value int64)
	GetNodeConfigs(ctx cosmos.Context, key string) (NodeConfigs, error)
	SetNodeConfig(_ cosmos.Context, key string, value int64, acc cosmos.AccAddress) error
	DeleteNodeConfigs(ctx cosmos.Context, key string)
	PurgeOperationalNodeConfigs(ctx cosmos.Context)
	GetStoreMigrateVotes(ctx cosmos.Context, key string) StoreMigrateVotes
	SetStoreMigrateVote(ctx cosmos.Context, key, value string, acc cosmos.AccAddress)
	DeleteStoreMigrateVotes(ctx cosmos.Context, key string)
	GetStoreMigrateApplied(ctx cosmos.Context, key string) (string, bool)
	SetStoreMigrateApplied(ctx cosmos.Context, key, value string)
	SetRawStoreValue(ctx cosmos.Context, key, value []byte) error
	GetRawStoreValue(ctx cosmos.Context, key []byte) ([]byte, bool)
	DeleteRawStoreValue(ctx cosmos.Context, key []byte)
	ValidateRawStoreValue(key, value []byte) error
	ValidateRawStoreKey(key []byte) error
	GetConfigIterator(ctx cosmos.Context) cosmos.Iterator
	GetNodeConfigIterator(ctx cosmos.Context) cosmos.Iterator
	DeleteConfig(_ cosmos.Context, key string) error
	GetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress) int64
	SetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress)
	IsOperationalConfig(key string) bool
}

type KeeperNetworkFee interface {
	GetNetworkFee(ctx cosmos.Context, chain common.Chain) (NetworkFee, error)
	SaveNetworkFee(ctx cosmos.Context, chain common.Chain, networkFee NetworkFee) error
	GetNetworkFeeIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperObservedNetworkFeeVoter interface {
	SetObservedNetworkFeeVoter(ctx cosmos.Context, networkFeeVoter ObservedNetworkFeeVoter)
	GetObservedNetworkFeeVoterIterator(ctx cosmos.Context) cosmos.Iterator
	GetObservedNetworkFeeVoter(ctx cosmos.Context, height int64, chain common.Chain, rate, size int64) (ObservedNetworkFeeVoter, error)
}

type KeeperSolvencyVoter interface {
	SetSolvencyVoter(_ cosmos.Context, _ SolvencyVoter)
	GetSolvencyVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (SolvencyVoter, error)
	GetSolvencyVoterIterator(_ cosmos.Context) cosmos.Iterator
}

type KeeperOracle interface {
	SetPrice(ctx cosmos.Context, oraclePrice OraclePrice) error
	GetPrice(ctx cosmos.Context, symbol string) (OraclePrice, error)
	DelPrice(ctx cosmos.Context, symbol string)
	GetPriceIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperHalt interface {
	IsChainHalted(ctx cosmos.Context, chain common.Chain) bool
}

type KeeperAnchors interface {
	GetAnchors(ctx cosmos.Context, asset common.Asset) []common.Asset
	AnchorMedian(ctx cosmos.Context, assets []common.Asset) cosmos.Uint
}
