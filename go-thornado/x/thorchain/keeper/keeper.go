package keeper

import (
	"cosmossdk.io/math"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/codec"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	kvTypes "github.com/thornadocash/go-thornado/x/thorchain/keeper/types"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
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
	RagnarokAccount(ctx cosmos.Context, addr cosmos.AccAddress)

	// passthrough funcs
	SendCoins(ctx cosmos.Context, from, to cosmos.AccAddress, coins cosmos.Coins) error

	InvariantRoutes() []common.InvariantRoute

	GetConstants() constants.ConstantValues
	GetConfigInt64(ctx cosmos.Context, key constants.ConstantName) int64
	DollarConfigInRune(ctx cosmos.Context, key constants.ConstantName) cosmos.Uint

	GetNativeTxFee(ctx cosmos.Context) cosmos.Uint
	GetTHORNameRegisterFee(ctx cosmos.Context) cosmos.Uint
	GetTHORNamePerBlockFee(ctx cosmos.Context) cosmos.Uint

	DeductNativeTxFeeFromAccount(ctx cosmos.Context, acctAddr cosmos.AccAddress) error

	// Keeper Interfaces
	KeeperConfig
	KeeperPool
	KeeperLastHeight
	KeeperLiquidityProvider
	KeeperNodeAccount
	KeeperUpgrade
	KeeperObserver
	KeeperObservedTx
	KeeperTxOut
	KeeperLiquidityFees
	KeeperOutboundFees
	KeeperSwapSlip
	KeeperVault
	KeeperReserveContributors
	KeeperNetwork
	KeeperTss
	KeeperTssKeysignFail
	KeeperKeygen
	KeeperRagnarok
	KeeperErrataTx
	KeeperBanVoter
	KeeperSwapQueue
	KeeperAdvSwapQueues
	KeeperMimir
	KeeperNetworkFee
	KeeperObservedNetworkFeeVoter
	KeeperOracle
	KeeperChainContract
	KeeperSolvencyVoter
	KeeperTHORName
	KeeperReferenceMemo
	KeeperHalt
	KeeperAnchors
	KeeperStreamingSwap
	KeeperSwapperClout
	KeeperTradeAccount
	KeeperSecuredAsset
	KeeperRUNEPool
	KeeperTCYClaimer
	KeeperTCYStaker
	KeeperVolume
	KeeperPOLReserve
	KeeperShielder
}

type KeeperConfig interface {
	GetConstants() constants.ConstantValues
	GetConfigInt64(ctx cosmos.Context, key constants.ConstantName) int64
}

type KeeperPool interface {
	GetPoolIterator(ctx cosmos.Context) cosmos.Iterator
	GetPool(ctx cosmos.Context, asset common.Asset) (Pool, error)
	GetPools(ctx cosmos.Context) (Pools, error)
	SetPool(ctx cosmos.Context, pool Pool) error
	PoolExist(ctx cosmos.Context, asset common.Asset) bool
	RemovePool(ctx cosmos.Context, asset common.Asset)
	SetPoolLUVI(ctx cosmos.Context, asset common.Asset, luvi cosmos.Uint)
	GetPoolLUVI(ctx cosmos.Context, asset common.Asset) (cosmos.Uint, error)
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

type KeeperSwapperClout interface {
	GetSwapperClout(ctx cosmos.Context, addr common.Address) (SwapperClout, error)
	SetSwapperClout(ctx cosmos.Context, record SwapperClout) error
	GetSwapperCloutIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperStreamingSwap interface {
	GetStreamingSwapIterator(ctx cosmos.Context) cosmos.Iterator
	SetStreamingSwap(ctx cosmos.Context, _ StreamingSwap)
	GetStreamingSwap(ctx cosmos.Context, _ common.TxID) (StreamingSwap, error)
	StreamingSwapExists(ctx cosmos.Context, _ common.TxID) bool
	RemoveStreamingSwap(ctx cosmos.Context, _ common.TxID)
}

type KeeperLiquidityProvider interface {
	GetLiquidityProviderIterator(ctx cosmos.Context, _ common.Asset) cosmos.Iterator
	GetLiquidityProvider(ctx cosmos.Context, asset common.Asset, addr common.Address) (LiquidityProvider, error)
	SetLiquidityProvider(ctx cosmos.Context, lp LiquidityProvider)
	RemoveLiquidityProvider(ctx cosmos.Context, lp LiquidityProvider)
	GetTotalSupply(ctx cosmos.Context, asset common.Asset) cosmos.Uint
}

type KeeperNodeAccount interface {
	TotalActiveValidators(ctx cosmos.Context) (int, error)
	ListValidatorsWithBond(ctx cosmos.Context) (NodeAccounts, error)
	ListValidatorsByStatus(ctx cosmos.Context, status NodeStatus) (NodeAccounts, error)
	ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error)
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
	SetBondProviders(ctx cosmos.Context, _ BondProviders) error
	GetBondProviders(ctx cosmos.Context, add cosmos.AccAddress) (BondProviders, error)
	DeductNativeTxFeeFromBond(ctx cosmos.Context, nodeAddr cosmos.AccAddress) error
	RemoveLowBondValidatorAccounts(ctx cosmos.Context) error
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
}

type KeeperLiquidityFees interface {
	AddToLiquidityFees(ctx cosmos.Context, asset common.Asset, fee cosmos.Uint) error
	GetTotalLiquidityFees(ctx cosmos.Context, height uint64) (cosmos.Uint, error)
	GetPoolLiquidityFees(ctx cosmos.Context, height uint64, asset common.Asset) (cosmos.Uint, error)
	GetRollingPoolLiquidityFee(ctx cosmos.Context, asset common.Asset) (uint64, error)
	ResetRollingPoolLiquidityFee(ctx cosmos.Context, asset common.Asset)
	SavePrevCyclePoolLiquidityFee(ctx cosmos.Context, asset common.Asset, fee uint64)
	GetPrevCyclePoolLiquidityFee(ctx cosmos.Context, asset common.Asset) (uint64, error)
	GetEffectivePoolLiquidityFee(ctx cosmos.Context, asset common.Asset) (uint64, error)
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

type KeeperSwapSlip interface {
	AddToSwapSlip(ctx cosmos.Context, asset common.Asset, amt cosmos.Int) error
	GetRollupCount(ctx cosmos.Context, asset common.Asset) (int64, error)
	RollupSwapSlip(ctx cosmos.Context, blockCount int64, _ common.Asset) (cosmos.Int, error)
	GetCurrentRollup(ctx cosmos.Context, asset common.Asset) (int64, error)
	SetCurrentRollup(ctx cosmos.Context, asset common.Asset, val int64)
	GetLongRollup(ctx cosmos.Context, asset common.Asset) (int64, error)
	SetLongRollup(ctx cosmos.Context, asset common.Asset, slip int64)
	GetPoolSwapSlip(ctx cosmos.Context, height int64, asset common.Asset) (cosmos.Int, error)
	DeletePoolSwapSlip(ctx cosmos.Context, height int64, asset common.Asset)
	GetSwapSlipSnapShot(ctx cosmos.Context, asset common.Asset, height int64) (int64, error)
	SetSwapSlipSnapShot(ctx cosmos.Context, asset common.Asset, height, currRollup int64)
	GetSwapSlipSnapShotIterator(ctx cosmos.Context, asset common.Asset) cosmos.Iterator
}

type KeeperTradeAccount interface {
	GetTradeAccount(ctx cosmos.Context, addr cosmos.AccAddress, asset common.Asset) (TradeAccount, error)
	SetTradeAccount(ctx cosmos.Context, record TradeAccount)
	RemoveTradeAccount(ctx cosmos.Context, record TradeAccount)
	GetTradeAccountIterator(ctx cosmos.Context) cosmos.Iterator
	GetTradeAccountIteratorWithAddress(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Iterator
	GetTradeUnit(ctx cosmos.Context, asset common.Asset) (TradeUnit, error)
	SetTradeUnit(ctx cosmos.Context, unit TradeUnit)
	GetTradeUnitIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperSecuredAsset interface {
	GetSecuredAsset(ctx cosmos.Context, asset common.Asset) (SecuredAsset, error)
	SetSecuredAsset(ctx cosmos.Context, unit SecuredAsset)
	GetSecuredAssetIterator(ctx cosmos.Context) cosmos.Iterator
}

type KeeperRUNEPool interface {
	GetRUNEPool(ctx cosmos.Context) (RUNEPool, error)
	SetRUNEPool(ctx cosmos.Context, pool RUNEPool)
	GetRUNEProviderIterator(ctx cosmos.Context) cosmos.Iterator
	GetRUNEProvider(ctx cosmos.Context, addr cosmos.AccAddress) (RUNEProvider, error)
	SetRUNEProvider(ctx cosmos.Context, rp RUNEProvider)
	RemoveRUNEProvider(ctx cosmos.Context, rp RUNEProvider)
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

type KeeperReserveContributors interface {
	AddPoolFeeToReserve(ctx cosmos.Context, fee cosmos.Uint) error
	AddBondFeeToReserve(ctx cosmos.Context, fee cosmos.Uint) error
}

// KeeperNetwork func to access network data in key value store
type KeeperNetwork interface {
	GetNetwork(ctx cosmos.Context) (Network, error)
	SetNetwork(ctx cosmos.Context, data Network) error
	GetPOL(ctx cosmos.Context) (ProtocolOwnedLiquidity, error)
	SetPOL(ctx cosmos.Context, data ProtocolOwnedLiquidity) error
}

// KeeperPOLReserve func to access POL reserve deposit data in key value store
type KeeperPOLReserve interface {
	GetPOLReserveDeposit(ctx cosmos.Context, asset common.Asset) (POLReserveDeposit, error)
	SetPOLReserveDeposit(ctx cosmos.Context, data POLReserveDeposit) error
	GetPOLReserveDepositIterator(ctx cosmos.Context) cosmos.Iterator
	IsPOLReserveEligiblePool(ctx cosmos.Context, asset common.Asset) bool
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

type KeeperRagnarok interface {
	RagnarokInProgress(_ cosmos.Context) bool
	GetRagnarokBlockHeight(_ cosmos.Context) (int64, error)
	SetRagnarokBlockHeight(_ cosmos.Context, _ int64)
	GetRagnarokNth(_ cosmos.Context) (int64, error)
	SetRagnarokNth(_ cosmos.Context, _ int64)
	GetRagnarokPending(_ cosmos.Context) (int64, error)
	SetRagnarokPending(_ cosmos.Context, _ int64)
	GetRagnarokWithdrawPosition(ctx cosmos.Context) (RagnarokWithdrawPosition, error)
	SetRagnarokWithdrawPosition(ctx cosmos.Context, position RagnarokWithdrawPosition)
	SetPoolRagnarokStart(ctx cosmos.Context, asset common.Asset)
	GetPoolRagnarokStart(ctx cosmos.Context, asset common.Asset) (int64, error)
	DeletePoolRagnarokStart(ctx cosmos.Context, asset common.Asset)
	IsRagnarok(ctx cosmos.Context, assets []common.Asset) bool
}

type KeeperErrataTx interface {
	SetErrataTxVoter(_ cosmos.Context, _ ErrataTxVoter)
	GetErrataTxVoterIterator(_ cosmos.Context) cosmos.Iterator
	GetErrataTxVoter(_ cosmos.Context, _ common.TxID, _ common.Chain) (ErrataTxVoter, error)
}

type KeeperSwapQueue interface {
	SetSwapQueueItem(ctx cosmos.Context, msg MsgSwap, i int) error
	GetSwapQueueIterator(ctx cosmos.Context) cosmos.Iterator
	GetSwapQueueItem(ctx cosmos.Context, txID common.TxID, i int) (MsgSwap, error)
	HasSwapQueueItem(ctx cosmos.Context, txID common.TxID, i int) bool
	RemoveSwapQueueItem(ctx cosmos.Context, txID common.TxID, i int)
}

type KeeperAdvSwapQueues interface {
	AdvSwapQueueEnabled(ctx cosmos.Context) bool
	SetAdvSwapQueueItem(ctx cosmos.Context, msg MsgSwap) error
	GetAdvSwapQueueItemIterator(ctx cosmos.Context) cosmos.Iterator
	GetAdvSwapQueueItem(ctx cosmos.Context, txID common.TxID, index int) (MsgSwap, error)
	HasAdvSwapQueueItem(ctx cosmos.Context, txID common.TxID, index int) bool
	RemoveAdvSwapQueueItem(ctx cosmos.Context, txID common.TxID, index int) error
	GetAdvSwapQueueIndexIterator(_ cosmos.Context, _ types.SwapType, _, _ common.Asset) cosmos.Iterator
	SetAdvSwapQueueIndex(_ cosmos.Context, _ MsgSwap) error
	GetAdvSwapQueueIndex(_ cosmos.Context, _ MsgSwap) ([]types.AdvSwapQueueIndexItem, error)
	HasAdvSwapQueueIndex(_ cosmos.Context, _ MsgSwap) (bool, error)
	RemoveAdvSwapQueueIndex(_ cosmos.Context, _ MsgSwap) error
	SetLimitSwapTTL(ctx cosmos.Context, blockHeight int64, txHashes []common.TxID) error
	GetLimitSwapTTL(ctx cosmos.Context, blockHeight int64) ([]common.TxID, error)
	RemoveLimitSwapTTL(ctx cosmos.Context, blockHeight int64)
	AddToLimitSwapTTL(ctx cosmos.Context, blockHeight int64, txHash common.TxID) error
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

type KeeperVolume interface {
	GetVolumeBucket(ctx cosmos.Context, pool common.Asset, index int64) (VolumeBucket, error)
	SetVolumeBucket(ctx cosmos.Context, bucket VolumeBucket) error
	GetVolumeBucketIterator(ctx cosmos.Context, pool common.Asset) cosmos.Iterator
	GetVolume(ctx cosmos.Context, pool common.Asset) (Volume, error)
	SetVolume(ctx cosmos.Context, volume Volume) error
}

type KeeperReferenceMemo interface {
	ReferenceMemoExists(ctx cosmos.Context, _ common.Asset, _ string) bool
	GetReferenceMemo(ctx cosmos.Context, _ common.Asset, _ string) (ReferenceMemo, error)
	GetReferenceMemoByTxnHash(ctx cosmos.Context, _ common.TxID) (ReferenceMemo, error)
	SetReferenceMemo(ctx cosmos.Context, _ ReferenceMemo)
	GetReferenceMemoIterator(ctx cosmos.Context) cosmos.Iterator
	DeleteReferenceMemo(ctx cosmos.Context, _ common.Asset, _ string) error
	GetLastReferenceNumber(ctx cosmos.Context, _ common.Asset) string
	SetLastReferenceNumber(ctx cosmos.Context, _ common.Asset, _ string)
}

// NewKeeper creates new instances of the thorchain Keeper
type KeeperTHORName interface {
	THORNameExists(ctx cosmos.Context, _ string) bool
	GetTHORName(ctx cosmos.Context, _ string) (THORName, error)
	SetTHORName(ctx cosmos.Context, name THORName)
	GetTHORNameIterator(ctx cosmos.Context) cosmos.Iterator
	DeleteTHORName(ctx cosmos.Context, _ string) error
	SetAffiliateCollector(_ cosmos.Context, _ AffiliateFeeCollector)
	GetAffiliateCollector(_ cosmos.Context, _ cosmos.AccAddress) (AffiliateFeeCollector, error)
	GetAffiliateCollectorIterator(_ cosmos.Context) cosmos.Iterator
	GetAffiliateCollectors(_ cosmos.Context) ([]AffiliateFeeCollector, error)
}

type KeeperHalt interface {
	IsTradingHalt(ctx cosmos.Context, msg cosmos.Msg) bool
	IsGlobalTradingHalted(ctx cosmos.Context) bool
	IsChainTradingHalted(ctx cosmos.Context, chain common.Chain) bool
	IsChainHalted(ctx cosmos.Context, chain common.Chain) bool
	IsLPPaused(ctx cosmos.Context, chain common.Chain) bool
	IsPoolDepositPaused(ctx cosmos.Context, asset common.Asset) bool
}

type KeeperAnchors interface {
	GetAnchors(ctx cosmos.Context, asset common.Asset) []common.Asset
	AnchorMedian(ctx cosmos.Context, assets []common.Asset) cosmos.Uint
	DollarsPerRune(ctx cosmos.Context) cosmos.Uint
	RunePerDollar(ctx cosmos.Context) cosmos.Uint
}

type KeeperTCYClaimer interface {
	SetTCYClaimer(ctx cosmos.Context, record TCYClaimer) error
	GetTCYClaimer(ctx cosmos.Context, l1Address common.Address, asset common.Asset) (TCYClaimer, error)
	GetTCYClaimerIteratorFromL1Address(ctx cosmos.Context, l1Address common.Address) cosmos.Iterator
	DeleteTCYClaimer(ctx cosmos.Context, l1Address common.Address, asset common.Asset)
	ListTCYClaimersFromL1Address(ctx cosmos.Context, l1Address common.Address) ([]TCYClaimer, error)
	GetTCYClaimerIterator(ctx cosmos.Context) cosmos.Iterator
	TCYClaimerExists(ctx cosmos.Context, l1Address common.Address, asset common.Asset) bool
	UpdateTCYClaimer(ctx cosmos.Context, l1Address common.Address, asset common.Asset, amount math.Uint) error
}

type KeeperTCYStaker interface {
	SetTCYStaker(ctx cosmos.Context, record TCYStaker) error
	GetTCYStaker(ctx cosmos.Context, address common.Address) (TCYStaker, error)
	DeleteTCYStaker(ctx cosmos.Context, address common.Address)
	ListTCYStakers(ctx cosmos.Context) ([]TCYStaker, error)
	TCYStakerExists(ctx cosmos.Context, address common.Address) bool
	UpdateTCYStaker(ctx cosmos.Context, address common.Address, amount math.Uint) error
}
