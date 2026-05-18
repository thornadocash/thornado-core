//go:build stagenet
// +build stagenet

package constants

func init() {
	int64Overrides = map[ConstantName]int64{
		ChurnInterval:              432000,
		OperationalVotesMin:        1,
		MinRunePoolDepth:           1_00000000,
		MinimumBondInRune:          200_000_00000000,
		PoolCycle:                  720,
		EmissionCurve:              8,
		NumberOfNewNodesPerChurn:   4,
		MintSynths:                 1,
		BurnSynths:                 1,
		MaxRuneSupply:              500_000_000_00000000,
		MultipleAffiliatesMaxCount: 5,
	}
	stringOverrides = map[ConstantName]string{
		DevFundAddress:       "sthor1gdq9qejfy33jctztqdrg5v4hvxnytmvjhduacc",
		MarketingFundAddress: "sthor1mtzxngd5lz2vjqpdvtu54dply7f26wews0uymq",
		OverSolvencyAddress:  "sthor1gdq9qejfy33jctztqdrg5v4hvxnytmvjhduacc",
	}
}
