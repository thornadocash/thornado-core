package constants

// NewConstantValue get new instance of ConstantValue
func NewConstantValue() *ConstantVals {
	return &ConstantVals{
		int64values: map[ConstantName]int64{
			BlocksPerYear:               5256000,
			MinimumNodesForBFT:          4,
			DesiredNodeSet:              100,   // desire node set
			ChurnInterval:               43200, // How many blocks Thornado try to rotate nodes
			ChurnRetryInterval:          720,   // How many blocks until we retry a churn (only if we haven't had a successful churn in ChurnInterval blocks
			MissingBlockChurnOut:        0,     // num of blocks a node needs to NOT sign between churns
			MaxMissingBlockChurnOut:     0,     // max number of nodes to be churned out due to not signing blocks
			MaxTrackMissingBlock:        700,   // maximum number of missing blocks to track for a block signer
			BadNodeRedline:              3,     // redline multiplier to find a multitude of bad actors
			LackOfObservationPenalty:    2,     // add two slash point for each block where a node does not observe
			SigningTransactionPeriod:    300,   // how many blocks before a request to sign a tx by yggdrasil pool, is counted as delinquent.
			DoubleSignMaxAge:            24,    // number of blocks to limit double signing a block
			SlashPenalty:                15000, // penalty paid (in basis points) for theft of assets
			PauseOnSlashThreshold:       0,
			FailKeygenSlashPoints:       720,     // slash for 720 blocks , which equals 1 hour
			FailKeysignSlashPoints:      2,       // slash for 2 blocks
			ObserveSlashPoints:          1,       // the number of slashpoints for making an observation (redeems later if observation reaches consensus
			DoubleBlockSignSlashPoints:  1000,    // slash points for double block sign (3-4 days (over 43200 blocks) rewards lost from 5 minutes (50 blocks))
			MissBlockSignSlashPoints:    1,       // slash points for not signing a block
			ObservationDelayFlexibility: 10,      // number of blocks of flexibility for a node to get their slash points taken off for making an observation
			JailTimeKeygen:              720 * 6, // blocks a node account is jailed for failing to keygen. DO NOT drop below tss timeout
			JailTimeKeysign:             60,      // blocks a node account is jailed for failing to keysign. DO NOT drop below tss timeout
			NodePauseChainBlocks:        720,     // number of blocks that a node can pause/resume a global chain halt
			MinPenaltyPointsForBadNode:  100,     // The minimum slash point
			KeygenRetryInterval:         0,       // number of blocks to wait before retrying a keygen
			OperationalVotesMin:         3,       // Minimum node votes to set an Operational Mimir
		},
		boolValues:   map[ConstantName]bool{},
		stringValues: map[ConstantName]string{},
	}
}
