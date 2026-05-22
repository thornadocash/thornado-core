//go:build stagenet
// +build stagenet

package constants

func init() {
	int64Overrides = map[ConfigName]int64{
		Churn_IntervalBlocks:       432000,
		Config_OperationalVotesMin: 1,
	}
	stringOverrides = map[ConfigName]string{}
}
