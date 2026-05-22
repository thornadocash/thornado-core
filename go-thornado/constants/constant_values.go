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
	Chain_BlocksPerYear ConfigName = iota
	Chain_PauseNodeBlocks

	BlockSign_DoublePenaltyPoints
	BlockSign_MissPenaltyPoints

	Churn_IntervalBlocks
	Churn_RetryIntervalBlocks

	Config_OperationalVotesMin

	BTC_ConfMultiplierBasisPoints
	BTC_ConfirmationsMin
	BTC_DefaultSatsPerVByte
	BTC_MaxConfirmations
	BTC_MaxMempoolAncestors
	BTC_MaxSatsPerVByte

	Deposit_AmountMinSats
	Deposit_PowDifficultyMin
	Deposit_PowExpiryBlocks
	Deposit_SessionExpiryBlocks
	Deposit_SweepRetryIntervalBlocks

	DoubleSign_MaxAgeBlocks

	Halt_ChainGlobal
	Halt_Churning
	Halt_SigningGlobal
	Halt_SolvencyCheck

	Keygen_FailJailBlocks
	Keygen_FailPenaltyPoints
	Keygen_RetryIntervalBlocks

	Keysign_FailJailBlocks
	Keysign_FailPenaltyPoints
	Keysign_PeriodBlocks

	Node_BadPenaltyPointsMin
	Node_BadRedline
	Node_BFTMin
	Node_BondSlotIncrementSats
	Node_BondStartAmountSats
	Node_PenaltyChurnOutThreshold
	Node_MissingBlocksChurnOut
	Node_MissingBlocksChurnOutMax
	Node_MissingBlocksTrackMax
	Node_SetDesired

	NodeSale_AuctionExpiryBlocksMax
	NodeSale_AuctionExpiryBlocksMin
	NodeSale_BidAmountMinSats

	Observation_DelayFlexibilityBlocks
	Observation_MissPenaltyPoints
	Observation_SubmitPenaltyPoints

	Shielder_FeeShareScale
	Shielder_NoteAmountMinSats

	Signer_Concurrency

	Slash_PauseThreshold
	Slash_PenaltyBasisPoints

	TxOut_MaxAttempts

	UTXO_MaxSpendCount

	Upgrade_ProposalCountMax

	Vault_MigrationIntervalBlocks
	Vault_MigrationRounds
	Vault_RetiredRecoveryAttemptsMax

	Withdrawal_FeeBasisPoints
	Withdrawal_FeeMinSats
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
