package constants

import "strings"

var configDescriptions = map[string]string{
	Chain_BlockTimeSeconds.String(): "Expected Thornado block time in seconds.",
	Chain_PauseNodeMinutes.String(): "Minutes a node pause request remains active.",

	BlockSign_DoublePenaltyPoints.String(): "Penalty points for double-signing a block.",
	BlockSign_MissPenaltyPoints.String():   "Penalty points for missing block signatures.",

	Churn_IntervalMinutes.String():      "Minutes between scheduled churn evaluations.",
	Churn_RetryIntervalMinutes.String(): "Minutes before retrying a failed churn action.",

	Config_OperationalVotesMin.String(): "Minimum node votes required for operational config changes.",

	BTC_ConfMultiplierBasisPoints.String(): "Basis point multiplier applied to BTC confirmation targets.",
	BTC_ConfirmationsMin.String():          "Minimum BTC confirmations required before accepting observations.",
	BTC_DefaultSatsPerVByte.String():       "Fallback BTC fee rate in sats per vbyte.",
	BTC_MaxConfirmations.String():          "Maximum BTC confirmations requested, or zero for uncapped.",
	BTC_MaxMempoolAncestors.String():       "Maximum allowed BTC mempool ancestor count.",
	BTC_MaxSatsPerVByte.String():           "Maximum BTC fee rate in sats per vbyte.",

	Deposit_AmountMinSats.String():             "Minimum BTC deposit amount in sats.",
	Deposit_PowDifficultyMin.String():          "Minimum proof-of-work difficulty for deposit address requests.",
	"Deposit_PowDifficultyCurrent":             "Current retargeted proof-of-work difficulty for deposit address requests.",
	Deposit_PowDifficultyMax.String():          "Maximum proof-of-work difficulty after retargeting.",
	Deposit_PowExpiryMinutes.String():          "Minutes before an unused deposit proof-of-work token expires.",
	Deposit_PowRetargetStepMax.String():        "Maximum proof-of-work difficulty bits adjusted per churn cycle.",
	Deposit_PowSamplesMin.String():             "Minimum confirmed deposit samples before proof-of-work retargeting.",
	Deposit_PowTargetPercentile.String():       "Confirmed-deposit solve percentile targeted by proof-of-work retargeting.",
	Deposit_PowTargetSeconds.String():          "Target proof-of-work solve time in seconds.",
	Deposit_SessionExpiryMinutes.String():      "Minutes before an issued deposit session expires.",
	Deposit_SweepRetryIntervalMinutes.String(): "Minutes between retry attempts for deposit sweeps.",
	Deposit_RefundIfForgottenDays.String():     "Days before an issued deposit address is purged from monitoring.",

	DoubleSign_MaxAgeMinutes.String(): "Maximum age in minutes for double-sign evidence.",

	Halt_ChainGlobal.String():   "Global halt flag for chain activity.",
	Halt_Churning.String():      "Halt flag for validator churn.",
	Halt_SigningGlobal.String(): "Global halt flag for signing outbound transactions.",
	Halt_SolvencyCheck.String(): "Halt flag for solvency checks.",

	Keygen_FailJailMinutes.String():      "Jail duration in minutes after keygen failure.",
	Keygen_FailPenaltyPoints.String():    "Penalty points for keygen failure.",
	Keygen_RetryIntervalMinutes.String(): "Minutes before retrying keygen.",

	Keysign_FailJailMinutes.String():   "Jail duration in minutes after keysign failure.",
	Keysign_FailPenaltyPoints.String(): "Penalty points for keysign failure.",
	Keysign_PeriodMinutes.String():     "Minutes allowed for a keysign attempt.",

	Node_BadPenaltyPointsMin.String():      "Minimum penalty points for bad node classification.",
	Node_BadRedline.String():               "Bad node count threshold before churn pressure increases.",
	Vault_BaseMembersMin.String():          "Minimum member count for the base vault signer set.",
	Node_BondSlotIncrementSats.String():    "Additional BTC bond required per permanent node slot.",
	Node_BondStartAmountSats.String():      "Starting BTC bond required for the first node slot.",
	Node_PenaltyChurnOutThreshold.String(): "Penalty points threshold for churning a node out.",
	Node_MissingBlocksChurnOut.String():    "Missing block threshold for churn-out eligibility.",
	Node_MissingBlocksChurnOutMax.String(): "Maximum missing block churn-outs per churn.",
	Node_MissingBlocksTrackMax.String():    "Maximum blocks tracked for missing block accounting.",
	Node_SetDesired.String():               "Desired active node count.",

	NodeSale_AuctionExpiryMaxMinutes.String(): "Maximum node slot auction duration in minutes.",
	NodeSale_AuctionExpiryMinMinutes.String(): "Minimum node slot auction duration in minutes.",
	NodeSale_BidAmountMinSats.String():        "Minimum BTC bid amount for node slot auctions.",

	Observation_DelayFlexibilityMinutes.String(): "Allowed delay flexibility in minutes for observations.",
	Observation_MissPenaltyPoints.String():       "Penalty points for missed observations.",
	Observation_SubmitPenaltyPoints.String():     "Penalty points for invalid observation submissions.",

	Shielder_FeeShareScale.String():     "Fixed-point scale for node fee share accounting.",
	Shielder_NoteAmountMinSats.String(): "Minimum shielded note amount before dust goes to fees.",

	Signer_Concurrency.String(): "Maximum concurrent signing operations.",

	TxOut_MaxAttempts.String(): "Maximum outbound transaction attempts, or zero for unlimited.",

	UTXO_MaxSpendCount.String(): "Maximum UTXOs to spend in one BTC transaction.",

	Upgrade_ProposalCountMax.String(): "Maximum active upgrade proposals.",

	Vault_MigrationIntervalMinutes.String():   "Minutes between vault migration rounds.",
	Vault_MigrationRounds.String():            "Number of migration rounds for retiring vaults.",
	Vault_RetiredRecoveryAttemptsMax.String(): "Maximum recovery attempts for retired vault funds.",

	Withdrawal_FeeBasisPoints.String():     "Withdrawal fee in basis points collected by core.",
	Withdrawal_FeeMinSats.String():         "Minimum withdrawal fee in sats collected by core.",
	Withdrawal_BatchWindowMinutes.String(): "Minutes to batch withdrawal outbounds before signing.",

	ConfigKeyNodePauseChainGlobal: "Global node-requested chain pause height.",
}

func ConfigGroup(key string) string {
	if i := strings.Index(key, "_"); i > 0 {
		return key[:i]
	}
	if strings.HasPrefix(key, "HaltSigning") {
		return "Halt"
	}
	if key == ConfigKeyNodePauseChainGlobal {
		return "Node"
	}
	return "Other"
}

func ConfigDescription(key string) string {
	if desc, ok := configDescriptions[key]; ok {
		return desc
	}
	if strings.HasPrefix(key, "HaltSigning") {
		return "Halt height for signing on the referenced chain."
	}
	return "Runtime configuration value."
}
