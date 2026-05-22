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
	GetRuneBalanceOfModule(ctx cosmos.Context, moduleName string) cosmos.Uint
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

	GetConstants() constants.ConstantValues
	GetConfigInt64(ctx cosmos.Context, key constants.ConstantName) int64

	GetNativeTxFee(ctx cosmos.Context) cosmos.Uint

	DeductNativeTxFeeFromAccount(ctx cosmos.Context, acctAddr cosmos.AccAddress) error

	// Keeper Interfaces
	KeeperConfig
	KeeperLastHeight
	KeeperNodeAccount
	KeeperUpgrade
	KeeperObserver
	KeeperObservedTx
	KeeperTxOut
	KeeperOutboundFees
	KeeperVault
	KeeperNetwork
	KeeperTss
	KeeperTssKeysignFail
	KeeperKeygen
	KeeperErrataTx
	KeeperBanVoter
	KeeperMimir
	KeeperNetworkFee
	KeeperObservedNetworkFeeVoter
	KeeperOracle
	KeeperChainContract
	KeeperSolvencyVoter
	KeeperHalt
	KeeperAnchors
	KeeperShielder
}

type KeeperConfig interface {
	GetConstants() constants.ConstantValues
	GetConfigInt64(ctx cosmos.Context, key constants.ConstantName) int64
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
	EnsureNodeKeysUnique(ctx cosmos.Context, consensusPubKey string, pubKeys common.PubKeySet) error
	GetNodeAccountIterator(ctx cosmos.Context) cosmos.Iterator
	GetNodeAccountSlashPoints(_ cosmos.Context, _ cosmos.AccAddress) (int64, error)
	SetNodeAccountSlashPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64)
	IncNodeAccountSlashPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) error
	DecNodeAccountSlashPoints(_ cosmos.Context, _ cosmos.AccAddress, _ int64) error
	ResetNodeAccountSlashPoints(_ cosmos.Context, _ cosmos.AccAddress)
	GetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress) (Jail, error)
	SetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress, height int64, reason string) error
	ReleaseNodeAccountFromJail(ctx cosmos.Context, addr cosmos.AccAddress) error
	DeductNativeTxFeeFromBond(ctx cosmos.Context, nodeAddr cosmos.AccAddress) error
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
	SetShielderSession(ctx cosmos.Context, session types.ShielderSession) error
	GetShielderSession(ctx cosmos.Context, owner cosmos.AccAddress) (types.ShielderSession, error)
	GetShielderSessionByPowToken(ctx cosmos.Context, powToken string) (types.ShielderSession, error)
	SetShielderDepositAddress(ctx cosmos.Context, record types.ShielderDepositAddress) error
	GetShielderDepositAddress(ctx cosmos.Context, address common.Address) (types.ShielderDepositAddress, error)
	GetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey) (uint64, error)
	SetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, index uint64) error
	AllocateVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey) (uint64, error)
	SetShielderDeposit(ctx cosmos.Context, deposit types.ShielderDeposit) error
	GetShielderDeposit(ctx cosmos.Context, depositID common.TxID) (types.ShielderDeposit, error)
	SetShielderCommitment(ctx cosmos.Context, commitment string, depositID common.TxID) error
	ShielderCommitmentExists(ctx cosmos.Context, commitment string) bool
	SetShielderDenominationCommitment(ctx cosmos.Context, denominationSats uint64, commitment string, depositID common.TxID) error
	GetShielderDenominationCommitments(ctx cosmos.Context, denominationSats uint64) ([]string, error)
	SetShielderMerkleRoot(ctx cosmos.Context, denominationSats uint64, root string) error
	ShielderMerkleRootExists(ctx cosmos.Context, denominationSats uint64, root string) bool
	SetShielderWithdrawal(ctx cosmos.Context, withdrawal types.ShielderWithdrawal) error
	GetShielderWithdrawal(ctx cosmos.Context, withdrawalID string) (types.ShielderWithdrawal, error)
	SetShielderNullifierSpent(ctx cosmos.Context, nullifierHash string, withdrawalID string) error
	ShielderNullifierSpent(ctx cosmos.Context, nullifierHash string) bool
	GetNextShielderNodeBondSlot(ctx cosmos.Context) (uint64, error)
	SetNextShielderNodeBondSlot(ctx cosmos.Context, slot uint64) error
	AllocateShielderNodeBondSlot(ctx cosmos.Context) (uint64, error)
	SetShielderNodeBond(ctx cosmos.Context, bond types.ShielderNodeBond) error
	GetShielderNodeBond(ctx cosmos.Context, nodePubKey string) (types.ShielderNodeBond, error)
	SetShielderFeePool(ctx cosmos.Context, pool types.ShielderFeePool) error
	GetShielderFeePool(ctx cosmos.Context) (types.ShielderFeePool, error)
	SetShielderFeeNotePubKey(ctx cosmos.Context, pubKey common.PubKey, depositID common.TxID) error
	ShielderFeeNotePubKeyUsed(ctx cosmos.Context, pubKey common.PubKey) bool
	SetNodeSlotAuction(ctx cosmos.Context, auction types.NodeSlotAuction) error
	GetNodeSlotAuction(ctx cosmos.Context, auctionID string) (types.NodeSlotAuction, error)
	SetNodeSlotBid(ctx cosmos.Context, bid types.NodeSlotBid) error
	GetNodeSlotBid(ctx cosmos.Context, bidID string) (types.NodeSlotBid, error)
}

type KeeperOutboundFees interface {
	AddToOutboundFeeWithheldRune(ctx cosmos.Context, outAsset common.Asset, withheld cosmos.Uint) error
	AddToOutboundFeeSpentRune(ctx cosmos.Context, outAsset common.Asset, spent cosmos.Uint) error
	GetOutboundFeeWithheldRune(ctx cosmos.Context, outAsset common.Asset) (cosmos.Uint, error)
	GetOutboundFeeWithheldRuneIterator(ctx cosmos.Context) cosmos.Iterator
	GetOutboundFeeSpentRune(ctx cosmos.Context, outAsset common.Asset) (cosmos.Uint, error)
	GetOutboundFeeSpentRuneIterator(ctx cosmos.Context) cosmos.Iterator
	GetOutboundTxFee(ctx cosmos.Context) cosmos.Uint
	GetSurplusForTargetMultiplier(ctx cosmos.Context, targetMultiplierBps cosmos.Uint) cosmos.Uint
}

type KeeperVault interface {
	GetVaultIterator(ctx cosmos.Context) cosmos.Iterator
	VaultExists(ctx cosmos.Context, pk common.PubKey) bool
	SetVault(ctx cosmos.Context, vault Vault) error
	GetVault(ctx cosmos.Context, pk common.PubKey) (Vault, error)
	HasValidVaultPools(ctx cosmos.Context) (bool, error)
	GetAsgardVaults(ctx cosmos.Context) (Vaults, error)
	GetAsgardVaultsByStatus(_ cosmos.Context, _ VaultStatus) (Vaults, error)
	GetLeastSecure(_ cosmos.Context, _ Vaults, _ int64) Vault
	GetMostSecure(_ cosmos.Context, _ Vaults, _ int64) Vault
	GetMostSecureStrict(_ cosmos.Context, _ Vaults, _ int64) Vault
	SortBySecurity(_ cosmos.Context, _ Vaults, _ int64) Vaults
	GetPendingOutbounds(_ cosmos.Context, _ common.Asset) []TxOutItem
	DeleteVault(ctx cosmos.Context, pk common.PubKey) error
	RemoveFromAsgardIndex(ctx cosmos.Context, pubkey common.PubKey) error
}

// KeeperNetwork func to access network data in key value store
type KeeperNetwork interface {
	GetNetwork(ctx cosmos.Context) (Network, error)
	SetNetwork(ctx cosmos.Context, data Network) error
}

type KeeperTss interface {
	SetTssVoter(_ cosmos.Context, tss TssVoter)
	GetTssVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetTssVoter(_ cosmos.Context, _ string) (TssVoter, error)
	SetTssKeygenMetric(_ cosmos.Context, metric *TssKeygenMetric)
	GetTssKeygenMetric(_ cosmos.Context, key common.PubKey) (*TssKeygenMetric, error)
	SetTssKeysignMetric(_ cosmos.Context, metric *TssKeysignMetric)
	GetTssKeysignMetric(_ cosmos.Context, txID common.TxID) (*TssKeysignMetric, error)
	GetLatestTssKeysignMetric(_ cosmos.Context) (*TssKeysignMetric, error)
}

type KeeperTssKeysignFail interface {
	SetTssKeysignFailVoter(_ cosmos.Context, tss TssKeysignFailVoter)
	GetTssKeysignFailVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetTssKeysignFailVoter(_ cosmos.Context, _ string) (TssKeysignFailVoter, error)
}

type KeeperKeygen interface {
	SetKeygenBlock(ctx cosmos.Context, keygenBlock KeygenBlock)
	GetKeygenBlockIterator(ctx cosmos.Context) cosmos.Iterator
	GetKeygenBlock(ctx cosmos.Context, height int64) (KeygenBlock, error)
}

type KeeperBanVoter interface {
	SetBanVoter(_ cosmos.Context, _ BanVoter)
	GetBanVoter(_ cosmos.Context, _ cosmos.AccAddress) (BanVoter, error)
	GetBanVoterIterator(_ cosmos.Context) cosmos.Iterator
}

type KeeperErrataTx interface {
	SetErrataTxVoter(_ cosmos.Context, _ ErrataTxVoter)
	GetErrataTxVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetErrataTxVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (ErrataTxVoter, error)
}

type KeeperMimir interface {
	GetMimir(_ cosmos.Context, key string) (int64, error)
	GetMimirWithRef(_ cosmos.Context, template string, ref ...any) (int64, error)
	SetMimir(_ cosmos.Context, key string, value int64)
	GetNodeMimirs(ctx cosmos.Context, key string) (NodeMimirs, error)
	SetNodeMimir(_ cosmos.Context, key string, value int64, acc cosmos.AccAddress) error
	DeleteNodeMimirs(ctx cosmos.Context, key string)
	PurgeOperationalNodeMimirs(ctx cosmos.Context)
	GetMimirIterator(ctx cosmos.Context) cosmos.Iterator
	GetNodeMimirIterator(ctx cosmos.Context) cosmos.Iterator
	DeleteMimir(_ cosmos.Context, key string) error
	GetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress) int64
	SetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress)
	IsOperationalMimir(key string) bool
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

type KeeperChainContract interface {
	SetChainContract(ctx cosmos.Context, cc ChainContract)
	GetChainContract(ctx cosmos.Context, chain common.Chain) (ChainContract, error)
	GetChainContracts(ctx cosmos.Context, chains common.Chains) []ChainContract
	GetChainContractIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperSolvencyVoter interface {
	SetSolvencyVoter(_ cosmos.Context, _ SolvencyVoter)
	GetSolvencyVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (SolvencyVoter, error)
}

type KeeperOracle interface {
	SetPrice(ctx cosmos.Context, oraclePrice OraclePrice) error
	GetPrice(ctx cosmos.Context, symbol string) (OraclePrice, error)
	DelPrice(ctx cosmos.Context, symbol string)
	GetPriceIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperHalt interface {
	IsTradingHalt(ctx cosmos.Context, msg cosmos.Msg) bool
	IsGlobalTradingHalted(ctx cosmos.Context) bool
	IsChainTradingHalted(ctx cosmos.Context, chain common.Chain) bool
	IsChainHalted(ctx cosmos.Context, chain common.Chain) bool
}

type KeeperAnchors interface {
	GetAnchors(ctx cosmos.Context, asset common.Asset) []common.Asset
	AnchorMedian(ctx cosmos.Context, assets []common.Asset) cosmos.Uint
	DollarsPerRune(ctx cosmos.Context) cosmos.Uint
	RunePerDollar(ctx cosmos.Context) cosmos.Uint
}
