//go:build chainnet
// +build chainnet

package constants

func init() {
	int64Overrides = map[ConfigName]int64{
		Churn_IntervalMinutes:      43200,
		Config_OperationalVotesMin: 1,
	}
	stringOverrides = map[ConfigName]string{}
}
