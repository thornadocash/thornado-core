package constants

// NewConfigValue get new instance of ConfigValue
func NewConfigValue() *ConfigVals {
	return &ConfigVals{
		int64values: map[ConfigName]int64{
			// Chain
			Chain_BlockTimeSeconds: 6,
			Chain_PauseNodeMinutes: 72,

			// BlockSign
			BlockSign_DoublePenaltyPoints: 1000,
			BlockSign_MissPenaltyPoints:   1,

			// Churn
			Churn_IntervalMinutes:      4320,
			Churn_RetryIntervalMinutes: 72,

			// Config
			Config_OperationalVotesMin: 3,

			// BTC
			BTC_ConfMultiplierBasisPoints: 10000,
			BTC_ConfirmationsMin:          1,
			BTC_DefaultSatsPerVByte:       25,
			BTC_MaxConfirmations:          0,
			BTC_MaxMempoolAncestors:       25,
			BTC_MaxSatsPerVByte:           9765,

			// Deposit
			Deposit_AmountMinSats:             546,
			Deposit_PowDifficultyMin:          20,
			Deposit_PowDifficultyMax:          64,
			Deposit_PowExpiryMinutes:          0,
			Deposit_PowRetargetStepMax:        1,
			Deposit_PowSamplesMin:             8,
			Deposit_PowTargetPercentile:       90,
			Deposit_PowTargetSeconds:          10,
			Deposit_SessionExpiryMinutes:      0,
			Deposit_SweepRetryIntervalMinutes: 72,
			Deposit_RefundIfForgottenDays:     30,

			// DoubleSign
			DoubleSign_MaxAgeMinutes: 3,

			// Halt
			Halt_ChainGlobal:   0,
			Halt_Churning:      0,
			Halt_SigningGlobal: 0,
			Halt_SolvencyCheck: 0,

			// Keygen
			Keygen_FailJailMinutes:      432,
			Keygen_FailPenaltyPoints:    720,
			Keygen_RetryIntervalMinutes: 0,

			// Keysign
			Keysign_FailJailMinutes:   6,
			Keysign_FailPenaltyPoints: 2,
			Keysign_PeriodMinutes:     30,

			// Node
			Node_BadPenaltyPointsMin:      100,
			Node_BadRedline:               3,
			Vault_BaseMembersMin:          4,
			Node_BondSlotIncrementSats:    100_000_000,
			Node_BondStartAmountSats:      100_000_000,
			Node_PenaltyChurnOutThreshold: 100,
			Node_MissingBlocksChurnOut:    0,
			Node_MissingBlocksChurnOutMax: 0,
			Node_MissingBlocksTrackMax:    700,
			Node_SetDesired:               100,

			// NodeSale
			NodeSale_AuctionExpiryMaxMinutes: 4320,
			NodeSale_AuctionExpiryMinMinutes: 1,
			NodeSale_BidAmountMinSats:        546,

			// Observation
			Observation_DelayFlexibilityMinutes: 1,
			Observation_MissPenaltyPoints:       2,
			Observation_SubmitPenaltyPoints:     1,

			// Shielder
			Shielder_FeeShareScale:     1_000_000_000_000,
			Shielder_NoteAmountMinSats: 100_000,

			// Signing
			Signer_Concurrency: 10,

			// Tx/vault operations
			TxOut_MaxAttempts:                0,
			UTXO_MaxSpendCount:               15,
			Upgrade_ProposalCountMax:         3,
			Vault_MigrationIntervalMinutes:   36,
			Vault_MigrationRounds:            2,
			Vault_RetiredRecoveryAttemptsMax: 100,

			// Withdrawal
			Withdrawal_FeeBasisPoints:     100,
			Withdrawal_BatchWindowMinutes: 10,
		},
		boolValues:   map[ConfigName]bool{},
		stringValues: map[ConfigName]string{},
	}
}
