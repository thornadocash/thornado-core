package constants

import (
	"fmt"

	"github.com/blang/semver"
)

// ConfigName the name we used to get constant values.
//
//go:generate stringer -type=ConfigName
type ConfigName int

const (
	Chain_BlockTimeSeconds ConfigName = iota
	Chain_PauseNodeMinutes

	BlockSign_DoublePenaltyPoints
	BlockSign_MissPenaltyPoints

	Churn_IntervalMinutes
	Churn_RetryIntervalMinutes

	Config_OperationalVotesMin

	BTC_ConfMultiplierBasisPoints
	BTC_ConfirmationsMin
	BTC_DefaultSatsPerVByte
	BTC_MaxConfirmations
	BTC_MaxMempoolAncestors
	BTC_MaxSatsPerVByte

	Deposit_AmountMinSats
	Deposit_PowDifficultyMin
	Deposit_PowDifficultyMax
	Deposit_PowExpiryMinutes
	Deposit_PowRetargetStepMax
	Deposit_PowSamplesMin
	Deposit_PowTargetPercentile
	Deposit_PowTargetSeconds
	Deposit_SessionExpiryMinutes
	Deposit_SweepRetryIntervalMinutes
	Deposit_RefundIfForgottenDays

	DoubleSign_MaxAgeMinutes

	Halt_ChainGlobal
	Halt_Churning
	Halt_SigningGlobal
	Halt_SolvencyCheck

	Keygen_FailJailMinutes
	Keygen_FailPenaltyPoints
	Keygen_RetryIntervalMinutes

	Keysign_FailJailMinutes
	Keysign_FailPenaltyPoints
	Keysign_PeriodMinutes

	Node_BadPenaltyPointsMin
	Node_BadRedline
	Vault_BaseMembersMin
	Node_BondSlotIncrementSats
	Node_BondStartAmountSats
	Node_PenaltyChurnOutThreshold
	Node_MissingBlocksChurnOut
	Node_MissingBlocksChurnOutMax
	Node_MissingBlocksTrackMax
	Node_SetDesired

	NodeSale_AuctionExpiryMaxMinutes
	NodeSale_AuctionExpiryMinMinutes
	NodeSale_BidAmountMinSats

	Observation_DelayFlexibilityMinutes
	Observation_MissPenaltyPoints
	Observation_SubmitPenaltyPoints

	Shielder_FeeShareScale
	Shielder_NoteAmountMinSats

	Signer_Concurrency

	TxOut_MaxAttempts

	UTXO_MaxSpendCount

	Upgrade_ProposalCountMax

	Vault_MigrationIntervalMinutes
	Vault_MigrationRounds
	Vault_RetiredRecoveryAttemptsMax

	Withdrawal_FeeBasisPoints
	Withdrawal_BatchWindowMinutes
)

// ConfigValues define methods used to get constant values
type ConfigValues interface {
	fmt.Stringer
	GetInt64Value(name ConfigName) int64
	GetBoolValue(name ConfigName) bool
	GetStringValue(name ConfigName) string
	GetConfigValsByKeyname() ConfigValsByKeyname
}

// GetConfigValues will return an  implementation of ConfigValues which provide ways to get constant values
// TODO hard fork remove unused version parameter
func GetConfigValues(_ semver.Version) ConfigValues {
	return NewConfigValue()
}
