package constants

import (
	"fmt"

	"github.com/blang/semver"
)

// ConstantName the name we used to get constant values.
//
//go:generate stringer -type=ConstantName
type ConstantName int

const (
	EmissionCurve ConstantName = iota
	MaxRuneSupply
	BlocksPerYear
	OutboundTransactionFee
	NativeTransactionFee
	PoolCycle
	MinRunePoolDepth
	MaxAvailablePools
	StagedPoolCost
	PendingLiquidityAgeLimit
	MinimumNodesForBFT
	DesiredValidatorSet
	AsgardSize
	DerivedDepthBasisPts
	DerivedMinDepth
	MaxAnchorSlip
	MaxAnchorBlocks
	DynamicMaxAnchorSlipBlocks
	DynamicMaxAnchorTarget
	DynamicMaxAnchorCalcInterval
	ChurnInterval
	ChurnRetryInterval
	MissingBlockChurnOut
	MaxMissingBlockChurnOut
	MaxTrackMissingBlock
	BadValidatorRedline
	LackOfObservationPenalty
	SigningTransactionPeriod
	DoubleSignMaxAge
	PauseBond
	PauseUnbond
	MinimumBondInRune
	FundMigrationInterval
	MaxOutboundAttempts
	SlashPenalty
	PauseOnSlashThreshold
	FailKeygenSlashPoints
	FailKeysignSlashPoints
	LiquidityLockUpBlocks
	ObserveSlashPoints
	DoubleBlockSignSlashPoints
	MissBlockSignSlashPoints
	ObservationDelayFlexibility
	JailTimeKeygen
	JailTimeKeysign
	NodePauseChainBlocks
	EnableDerivedAssets
	MinSwapsPerBlock
	MaxSwapsPerBlock
	EnableOrderBooks
	EnableAdvSwapQueue
	AdvSwapQueueRapidSwapMax
	MaxSynthPerPoolDepth
	MaxSynthsForSaversYield
	VirtualMultSynths
	VirtualMultSynthsBasisPoints
	MinSlashPointsForBadValidator
	MaxBondProviders
	MinTxOutVolumeThreshold
	TxOutDelayRate
	TxOutDelayMax
	MaxTxOutOffset
	TNSRegisterFee
	TNSFeeOnSale
	TNSFeePerBlock
	StreamingSwapPause
	StreamingSwapMinBPFee // TODO: remove on hard fork
	StreamingSwapMaxLength
	StreamingSwapMaxLengthNative
	StreamingLimitSwapMaxAge
	MinCR
	MaxCR
	LendingLever
	PermittedSolvencyGap
	PermittedSolvencyGapUSD
	NodeOperatorFee
	ValidatorMaxRewardRatio
	MaxNodeToChurnOutForLowVersion
	ChurnOutForLowVersionBlocks
	POLMaxNetworkDeposit
	POLMaxPoolMovement
	POLTargetSynthPerPoolDepth
	POLBuffer
	RagnarokProcessNumOfLPPerIteration
	SynthYieldBasisPoints
	SynthYieldCycle
	MinimumL1OutboundFeeUSD
	MinimumPoolLiquidityFee
	ChurnMigrateRounds
	AllowWideBlame
	MaxAffiliateFeeBasisPoints
	TargetOutboundFeeSurplusRune
	MaxOutboundFeeMultiplierBasisPoints
	MinOutboundFeeMultiplierBasisPoints
	NativeOutboundFeeUSD
	NativeTransactionFeeUSD
	TNSRegisterFeeUSD
	TNSFeePerBlockUSD
	EnableUSDFees
	PreferredAssetOutboundFeeMultiplier
	FeeUSDRoundSignificantDigits
	MigrationVaultSecurityBps
	CloutReset
	CloutLimit
	KeygenRetryInterval
	SaversStreamingSwapsInterval
	RescheduleCoalesceBlocks
	L1SlipMinBps
	SynthSlipMinBps
	TradeAccountsSlipMinBps
	DerivedSlipMinBps
	SlipMinBpsMax
	TradeAccountsEnabled
	TradeAccountsDepositEnabled
	SecuredAssetSlipMinBps
	StableSlipMinBps
	EVMDisableContractWhitelist
	OperationalVotesMin
	RUNEPoolEnabled
	RUNEPoolDepositMaturityBlocks
	RUNEPoolMaxReserveBackstop
	SaversEjectInterval
	SystemIncomeBurnRateBps
	DevFundSystemIncomeBps
	DevFundAddress
	PendulumAssetsBasisPoints
	PendulumUseEffectiveSecurity
	PendulumUseVaultAssets
	TVLCapBasisPoints
	MultipleAffiliatesMaxCount
	BondSlashBan
	BankSendEnabled
	RUNEPoolHaltDeposit
	RUNEPoolHaltWithdraw
	MinRuneForTCYStakeDistribution
	MinTCYForTCYStakeDistribution
	TCYStakeSystemIncomeBps
	TCYClaimingSwapHalt
	TCYStakeDistributionHalt
	TCYStakingHalt
	TCYUnstakingHalt
	TCYClaimingHalt
	HaltRebond
	HaltOperatorRotate
	HaltMemoless
	RequiredPriceFeeds
	HaltOracle
	OracleUpdateInterval
	ReserveMaxCap
	MarketingFundSystemIncomeBps
	MarketingFundAddress
	OverSolvencyToTreasuryBps
	OverSolvencyCheckInterval
	OverSolvencyAddress
	MaxDepositTxIDRetries
	MaxRetiredVaultRecoveryAttempts
	ModifyLimitSwapMaxIterations
	POLReserveSystemIncomeBps
	POLReserveMaxDeployment

	// These are the implicitly-0 Constants undisplayed in the API endpoint (no explicit value set).
	ArtificialRagnarokBlockHeight
	BondLockupPeriod
	BurnSynths
	DefaultPoolStatus
	EnableMemolessOutbound
	ManualSwapsToSynthDisabled
	MaximumLiquidityRune
	MintSynths
	NumberOfNewNodesPerChurn
	SignerConcurrency
	MemolessTxnTTL
	MemolessTxnRefCount
	MemolessTxnCost
	MemolessTxnMaxUse
	StrictBondLiquidityRatio
	SwapOutDexAggregationDisabled
	P2PGateDisabled
)

// ConstantValues define methods used to get constant values
type ConstantValues interface {
	fmt.Stringer
	GetInt64Value(name ConstantName) int64
	GetBoolValue(name ConstantName) bool
	GetStringValue(name ConstantName) string
	GetConstantValsByKeyname() ConstantValsByKeyname
}

// GetConstantValues will return an  implementation of ConstantValues which provide ways to get constant values
// TODO hard fork remove unused version parameter
func GetConstantValues(_ semver.Version) ConstantValues {
	return NewConstantValue()
}
