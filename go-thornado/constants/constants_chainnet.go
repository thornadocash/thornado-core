//go:build chainnet
// +build chainnet

package constants

func init() {
	int64Overrides = map[ConstantName]int64{
		ChurnInterval:       432000,
		OperationalVotesMin: 1,
	}
	stringOverrides = map[ConstantName]string{}
}
