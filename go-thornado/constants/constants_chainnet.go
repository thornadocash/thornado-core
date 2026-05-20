//go:build chainnet
// +build chainnet

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
		DevFundAddress:       "cthor1jydyvc8wt84jcdl6qex0ps9czfmcaqurljevkt",
		MarketingFundAddress: "cthor15wayjcl0ecdnqzjx9a3f9zll6pzvphl6we8k6j",
		OverSolvencyAddress:  "cthor1jydyvc8wt84jcdl6qex0ps9czfmcaqurljevkt",
	}
}
