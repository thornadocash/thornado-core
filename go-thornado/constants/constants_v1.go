package constants

// NewConfigValue get new instance of ConfigValue
func NewConfigValue() *ConfigVals {
	return &ConfigVals{
		int64values: map[ConfigName]int64{
			// Chain
			Chain_BlocksPerYear:   5256000,
			Chain_PauseNodeBlocks: 720,

			// BlockSign
			BlockSign_DoublePenaltyPoints: 1000,
			BlockSign_MissPenaltyPoints:   1,

			// Churn
			Churn_IntervalBlocks:      43200,
			Churn_RetryIntervalBlocks: 720,

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
			Deposit_AmountMinSats:            546,
			Deposit_PowDifficultyMin:         0,
			Deposit_PowExpiryBlocks:          0,
			Deposit_SessionExpiryBlocks:      0,
			Deposit_SweepRetryIntervalBlocks: 720,

			// DoubleSign
			DoubleSign_MaxAgeBlocks: 24,

			// Halt
			Halt_ChainGlobal:   0,
			Halt_Churning:      0,
			Halt_SigningGlobal: 0,
			Halt_SolvencyCheck: 0,

			// Keygen
			Keygen_FailJailBlocks:      720 * 6,
			Keygen_FailPenaltyPoints:   720,
			Keygen_RetryIntervalBlocks: 0,

			// Keysign
			Keysign_FailJailBlocks:    60,
			Keysign_FailPenaltyPoints: 2,
			Keysign_PeriodBlocks:      300,

			// Node
			Node_BadPenaltyPointsMin:      100,
			Node_BadRedline:               3,
			Node_BFTMin:                   4,
			Node_BondSlotIncrementSats:    100_000_000,
			Node_BondStartAmountSats:      100_000_000,
			Node_PenaltyChurnOutThreshold: 100,
			Node_MissingBlocksChurnOut:    0,
			Node_MissingBlocksChurnOutMax: 0,
			Node_MissingBlocksTrackMax:    700,
			Node_SetDesired:               100,

			// NodeSale
			NodeSale_AuctionExpiryBlocksMax: 43200,
			NodeSale_AuctionExpiryBlocksMin: 1,
			NodeSale_BidAmountMinSats:       546,

			// Observation
			Observation_DelayFlexibilityBlocks: 10,
			Observation_MissPenaltyPoints:      2,
			Observation_SubmitPenaltyPoints:    1,

			// Shielder
			Shielder_FeeShareScale:     1_000_000_000_000,
			Shielder_NoteAmountMinSats: 546,

			// Signing/slashing
			Signer_Concurrency:       10,
			Slash_PauseThreshold:     0,
			Slash_PenaltyBasisPoints: 15000,

			// Tx/vault operations
			TxOut_MaxAttempts:                0,
			UTXO_MaxSpendCount:               15,
			Upgrade_ProposalCountMax:         3,
			Vault_MigrationIntervalBlocks:    360,
			Vault_MigrationRounds:            2,
			Vault_RetiredRecoveryAttemptsMax: 100,

			// Withdrawal
			Withdrawal_FeeBasisPoints: 200,
			Withdrawal_FeeMinSats:     10_000,
		},
		boolValues:   map[ConfigName]bool{},
		stringValues: map[ConfigName]string{},
	}
}
