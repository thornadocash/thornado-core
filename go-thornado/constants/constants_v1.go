package constants

// NewConstantValue get new instance of ConstantValue
func NewConstantValue() *ConstantVals {
	return &ConstantVals{
		int64values: map[ConstantName]int64{
			EmissionCurve:                       6,
			BlocksPerYear:                       5256000,
			MaxRuneSupply:                       -1, // max supply of rune. Default set to -1 to avoid consensus failure
			OutboundTransactionFee:              2_000000,
			NativeOutboundFeeUSD:                2_000000, // $0.02 fee on all swaps and withdrawals
			NativeTransactionFee:                2_000000,
			NativeTransactionFeeUSD:             2_000000,           // $0.02 fee on all on chain txs
			PoolCycle:                           43200,              // Make a pool available every 3 days
			StagedPoolCost:                      10_00000000,        // amount of rune to take from a staged pool on every pool cycle
			PendingLiquidityAgeLimit:            100800,             // age pending liquidity can be pending before its auto committed to the pool
			MinRunePoolDepth:                    10000_00000000,     // minimum rune pool depth to be an available pool
			MaxAvailablePools:                   100,                // maximum number of available pools
			MinimumNodesForBFT:                  4,                  // Minimum node count to keep network running. Below this, Ragnarök is performed.
			DesiredValidatorSet:                 100,                // desire validator set
			AsgardSize:                          40,                 // desired node operators in an asgard vault
			DerivedDepthBasisPts:                0,                  // Basis points to increase/decrease derived pool depth (10k == 1x)
			DerivedMinDepth:                     100,                // in basis points, min derived pool depth
			MaxAnchorSlip:                       1500,               // basis points of rune depth to trigger pausing a derived virtual pool
			MaxAnchorBlocks:                     300,                // max blocks to accumulate swap slips in anchor pools
			DynamicMaxAnchorSlipBlocks:          14400 * 14,         // number of blocks to sample in calculating the dynamic max anchor slip
			DynamicMaxAnchorTarget:              0,                  // target depth of derived virtual pool (in basis points)
			DynamicMaxAnchorCalcInterval:        14400,              // number of blocks to recalculate the dynamic max anchor
			FundMigrationInterval:               360,                // number of blocks THORNode will attempt to move funds from a retiring vault to an active one
			ChurnInterval:                       43200,              // How many blocks THORNode try to rotate validators
			ChurnRetryInterval:                  720,                // How many blocks until we retry a churn (only if we haven't had a successful churn in ChurnInterval blocks
			MissingBlockChurnOut:                0,                  // num of blocks a validator needs to NOT sign between churns
			MaxMissingBlockChurnOut:             0,                  // max number of nodes to be churned out due to not signing blocks
			MaxTrackMissingBlock:                700,                // maximum number of missing blocks to track for a block signer
			BadValidatorRedline:                 3,                  // redline multiplier to find a multitude of bad actors
			LackOfObservationPenalty:            2,                  // add two slash point for each block where a node does not observe
			SigningTransactionPeriod:            300,                // how many blocks before a request to sign a tx by yggdrasil pool, is counted as delinquent.
			DoubleSignMaxAge:                    24,                 // number of blocks to limit double signing a block
			PauseBond:                           0,                  // pauses the ability to bond
			PauseUnbond:                         0,                  // pauses the ability to unbond
			MinimumBondInRune:                   1_000_000_00000000, // 1 million rune
			MaxBondProviders:                    6,                  // maximum number of bond providers
			MaxOutboundAttempts:                 0,                  // maximum retries to reschedule a transaction
			SlashPenalty:                        15000,              // penalty paid (in basis points) for theft of assets
			PauseOnSlashThreshold:               100_00000000,       // number of rune to pause the network on the event a vault is slash for theft
			FailKeygenSlashPoints:               720,                // slash for 720 blocks , which equals 1 hour
			FailKeysignSlashPoints:              2,                  // slash for 2 blocks
			LiquidityLockUpBlocks:               0,                  // the number of blocks LP can withdraw after their liquidity
			ObserveSlashPoints:                  1,                  // the number of slashpoints for making an observation (redeems later if observation reaches consensus
			DoubleBlockSignSlashPoints:          1000,               // slash points for double block sign (3-4 days (over 43200 blocks) rewards lost from 5 minutes (50 blocks))
			MissBlockSignSlashPoints:            1,                  // slash points for not signing a block
			ObservationDelayFlexibility:         10,                 // number of blocks of flexibility for a validator to get their slash points taken off for making an observation
			JailTimeKeygen:                      720 * 6,            // blocks a node account is jailed for failing to keygen. DO NOT drop below tss timeout
			JailTimeKeysign:                     60,                 // blocks a node account is jailed for failing to keysign. DO NOT drop below tss timeout
			NodePauseChainBlocks:                720,                // number of blocks that a node can pause/resume a global chain halt
			NodeOperatorFee:                     500,                // Node operator fee
			EnableDerivedAssets:                 0,                  // enable/disable swapping of derived assets
			MinSwapsPerBlock:                    10,                 // process all swaps if queue is less than this number
			MaxSwapsPerBlock:                    100,                // max swaps to process per block
			EnableOrderBooks:                    0,                  // enable order books instead of swap queue
			EnableAdvSwapQueue:                  0,                  // enable advanced swap queue, value of 2 skips limit swaps and forces all swaps to be market trades
			AdvSwapQueueRapidSwapMax:            1,                  // maximum number of rapid swap iterations per block
			VirtualMultSynths:                   2,                  // pool depth multiplier for synthetic swaps
			VirtualMultSynthsBasisPoints:        10_000,             // pool depth multiplier for synthetic swaps (in basis points)
			MaxSynthPerPoolDepth:                1700,               // percentage (in basis points) of how many synths are allowed relative to pool depth of the related pool
			MaxSynthsForSaversYield:             0,                  // percentage (in basis points) synth per pool where synth yield reaches 0%
			MinSlashPointsForBadValidator:       100,                // The minimum slash point
			StreamingSwapPause:                  0,                  // pause streaming swaps from being processed or accepted
			StreamingSwapMinBPFee:               0,                  // min swap fee (in basis points) for a streaming swap trade
			StreamingSwapMaxLength:              14400,              // max number of blocks a streaming swap can trade for
			StreamingSwapMaxLengthNative:        14400 * 365,        // max number of blocks native streaming swaps can trade over
			StreamingLimitSwapMaxAge:            43200,              // max number of blocks a streaming limit swap can exist before completing (3 days)
			MinCR:                               10_000,             // Minimum collateralization ratio (basis pts)
			MaxCR:                               60_000,             // Maximum collateralization ratio (basis pts)
			LendingLever:                        3333,               // This controls (in basis points) how much lending is allowed relative to rune supply
			MinTxOutVolumeThreshold:             1000_00000000,      // total txout volume (in rune) a block needs to have to slow outbound transactions
			TxOutDelayRate:                      25_00000000,        // outbound rune per block rate for scheduled transactions (excluding native assets)
			TxOutDelayMax:                       17280,              // max number of blocks a transaction can be delayed
			MaxTxOutOffset:                      720,                // max blocks to offset a txout into a future block
			TNSRegisterFee:                      10_00000000,
			TNSRegisterFeeUSD:                   10_00000000, // registration fee for new THORName in USD
			TNSFeeOnSale:                        1000,        // fee for TNS sale in basis points
			TNSFeePerBlock:                      20,
			TNSFeePerBlockUSD:                   20,               // per block cost for TNS in USD
			PermittedSolvencyGap:                100,              // the setting is in basis points
			PermittedSolvencyGapUSD:             500_00000000,     // $500 USD in 1e8 notation
			ValidatorMaxRewardRatio:             1,                // the ratio to MinimumBondInRune at which validators stop receiving rewards proportional to their bond
			MaxNodeToChurnOutForLowVersion:      1,                // the maximum number of nodes to churn out for low version per churn
			ChurnOutForLowVersionBlocks:         21600,            // the blocks after the MinJoinVersion changes before nodes can be churned out for low version
			POLMaxNetworkDeposit:                0,                // Maximum amount of rune deposited into the pools
			POLMaxPoolMovement:                  100,              // Maximum amount of rune to enter/exit a pool per iteration - 1 equals one hundredth of a basis point of pool rune depth
			POLTargetSynthPerPoolDepth:          0,                // target synth per pool depth for POL (basis points)
			POLBuffer:                           0,                // buffer around the POL synth utilization (basis points added to/subtracted from POLTargetSynthPerPoolDepth basis points)
			RagnarokProcessNumOfLPPerIteration:  200,              // the number of LP to be processed per iteration during ragnarok pool
			SynthYieldBasisPoints:               5000,             // amount of the yield the capital earns the synth holder receives if synth per pool is 0%
			SynthYieldCycle:                     0,                // number of blocks when the network pays out rewards to yield bearing synths
			MinimumL1OutboundFeeUSD:             1000000,          // Minimum fee in USD to charge for LP swap, default to $0.01 , nodes need to vote it to a larger value
			MinimumPoolLiquidityFee:             0,                // Minimum liquidity fee made by the pool,active pool fail to meet this within a PoolCycle will be demoted
			ChurnMigrateRounds:                  5,                // Number of rounds to migrate vaults during churn
			AllowWideBlame:                      0,                // allow for a wide blame, only set in mocknet for regression testing tss keysign failures
			MaxAffiliateFeeBasisPoints:          10_000,           // Max allowed affiliate fee basis points
			TargetOutboundFeeSurplusRune:        100_000_00000000, // Target amount of RUNE for Outbound Fee Surplus: the sum of the diff between outbound cost to user and outbound cost to network
			MaxOutboundFeeMultiplierBasisPoints: 30_000,           // Maximum multiplier applied to base outbound fee charged to user, in basis points
			MinOutboundFeeMultiplierBasisPoints: 15_000,           // Minimum multiplier applied to base outbound fee charged to user, in basis points
			EnableUSDFees:                       0,                // enable USD fees
			PreferredAssetOutboundFeeMultiplier: 100,              // multiplier of the current preferred asset outbound fee, if rune balance > multiplier * outbound_fee, a preferred asset swap is triggered
			FeeUSDRoundSignificantDigits:        2,                // number of significant digits to round the RUNE value of USD denominated fees
			MigrationVaultSecurityBps:           0,                // vault bond must be greater than bps of funds value in rune to receive migrations
			CloutReset:                          720,              // number of blocks before clout spent gets reset
			CloutLimit:                          0,                // max clout allowed to spend
			KeygenRetryInterval:                 0,                // number of blocks to wait before retrying a keygen
			SaversStreamingSwapsInterval:        0,                // For Savers deposits and withdraws, the streaming swaps interval to use for the Native <> Synth swap
			RescheduleCoalesceBlocks:            0,                // number of blocks to coalesce rescheduled outbounds
			TradeAccountsEnabled:                0,                // enable/disable trade account
			TradeAccountsDepositEnabled:         1,
			EVMDisableContractWhitelist:         0,                  // enable/disable contract whitelist
			OperationalVotesMin:                 3,                  // Minimum node votes to set an Operational Mimir
			MemolessTxnTTL:                      3600,               // number of blocks before a memoless txn expires
			MemolessTxnRefCount:                 99_999,             // max number of reference ids per chain
			MemolessTxnCost:                     0,                  // additional cost in RUNE to register a memoless txn (operational mimir)
			MemolessTxnMaxUse:                   1,                  // maximum times a reference id can be utilized before refunding (operational mimir)
			L1SlipMinBps:                        0,                  // Minimum L1 asset swap fee in basis points
			TradeAccountsSlipMinBps:             0,                  // Minimum trade asset swap fee in basis points
			SecuredAssetSlipMinBps:              5,                  // Minimum secured asset swap fee in basis points
			SynthSlipMinBps:                     0,                  // Minimum synth asset swap fee in basis points
			DerivedSlipMinBps:                   0,                  // Minimum derived asset swap fee in basis points
			StableSlipMinBps:                    0,                  // Minimum swap fee in basis points for stable-to-stable swaps
			SlipMinBpsMax:                       100,                // Maximum slip min bps for all asset types
			RUNEPoolEnabled:                     0,                  // enable/disable RUNE Pool
			RUNEPoolDepositMaturityBlocks:       14400 * 90,         // blocks from last deposit to allow withdraw
			RUNEPoolMaxReserveBackstop:          5_000_000_00000000, // 5 million RUNE
			SaversEjectInterval:                 0,                  // number of blocks for savers check, disabled if zero
			SystemIncomeBurnRateBps:             1,                  // burn 1bps (0.01%) RUNE of all system income per ADR 17
			DevFundSystemIncomeBps:              500,                // allocate 500bps (5%) RUNE of all system income to dev fund per ADR 18
			MarketingFundSystemIncomeBps:        500,                // allocate 500bps (5%) RUNE of all system income to marketing fund per ADR 21
			PendulumAssetsBasisPoints:           10_000,             // Incentive curve adjustment lever to proportionally underestimate or overestimate Assets needing to be secured.
			PendulumUseEffectiveSecurity:        0,                  // If 1, use the effective security bond (the bond sacrificable to seize L1 Assets) as the securing bond for which to target double the value of the secured Assets. If 0, instead use the whole (rewards-receiving) total effective bond.
			PendulumUseVaultAssets:              0,                  // If 1. use the L1 Assets in the vaults (the Assets seizable by the lower-bond 2/3rds of nodes in each vault) as the Assets to be secured.  If 0, instead use only the L1 Assets in pools, ignoring the L1 Assets in for instance streaming swaps, oversolvencies, and Trade/Bridge Assets.
			TVLCapBasisPoints:                   0,                  // If 0, TVL Cap is set to the effective active bond. If non-zero, the value is interrupted as basis points relative to total active bond
			MultipleAffiliatesMaxCount:          5,                  // maximum number of nested affiliates
			BondSlashBan:                        5_000_00000000,     // 5000 RUNE - amount to slash bond of banned nodes
			BankSendEnabled:                     0,                  // enable/disable cosmos bank send messages
			RUNEPoolHaltDeposit:                 0,                  // enable/disable RUNEPool deposit (block height)
			RUNEPoolHaltWithdraw:                0,                  // enable/disable RUNEPool withdraw (block height)
			MinRuneForTCYStakeDistribution:      2_100_00000000,     // Set what is the minimum amount of rune need it on TCY fund in order to be distributed
			MinTCYForTCYStakeDistribution:       100000,             // Set what is the minimum amount of TCY need it on TCY fund in order to be distributed
			TCYStakeSystemIncomeBps:             1000,               // allocate 1000bps (10%) RUNE of all system income to TCY Fund
			TCYClaimingSwapHalt:                 1,                  // enable/disable claiming module rune to tcy swap
			TCYStakeDistributionHalt:            1,                  // enable/disable tcy stake distribution
			TCYStakingHalt:                      1,                  // enable/disable tcy staking
			TCYUnstakingHalt:                    1,                  // enable/disable tcy unstaking
			TCYClaimingHalt:                     1,                  // enable/disable tcy claiming
			ReserveMaxCap:                       0,                  // maximum reserve balance before EmissionCurve is overridden, 0 = disabled
			MaxDepositTxIDRetries:               100,                // maximum retries for deposit txid auto-increment to avoid collisions
			OverSolvencyToTreasuryBps:           0,                  // basis points (0-10000) of over-solvent liquidity to swap to RUNE and transfer to over-solvency sweep destination address
			OverSolvencyCheckInterval:           600,                // interval in blocks for over-solvency checks (approximately 30 days)
			MaxRetiredVaultRecoveryAttempts:     100,                // maximum retries for retired vault recovery refunds
			ModifyLimitSwapMaxIterations:        100,                // maximum iterations when searching for a user's swap in a ratio-grouped index
			POLReserveSystemIncomeBps:           0,                  // basis points of system income routed to POL reserve, 0 = disabled
			POLReserveMaxDeployment:             10_000_000_000,     // maximum RUNE deployed per pool per block (100 RUNE in 1e8 units)
		},
		boolValues: map[ConstantName]bool{
			StrictBondLiquidityRatio: true,
		},
		stringValues: map[ConstantName]string{
			DefaultPoolStatus:    "Staged",
			DevFundAddress:       "thor1sgnuptp32y2fht3258vlx68mq8wsk2uz4wrshu", // dev fund address for ADR 18
			MarketingFundAddress: "thor1usj8cqjmjea32csxn5fma96ffeulln40gyrahn", // marketing fund address for ADR 21
			OverSolvencyAddress:  "thor1lhufh0mwasa0lk9udppdegmvnkgqt08f0m9p5g", // over-solvency sweep destination address
			RequiredPriceFeeds:   "ATOM,AVAX,BCH,BNB,BTC,DOGE,ETH,LTC,RUNE,SOL,TRX,USDC,USDT,XRP,ZEC",
		},
	}
}
