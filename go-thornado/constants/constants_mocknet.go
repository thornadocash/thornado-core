//go:build mocknet
// +build mocknet

// For internal testing and mockneting
package constants

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ThornadoBlockTime = time.Second

// CamelToSnakeUpper converts a camelCase string to SNAKE_CASE.
// Examples: "Chain_BlockTimeSeconds" -> "CHAIN_BLOCK_TIME_SECONDS"
func CamelToSnakeUpper(s string) string {
	re := regexp.MustCompile(`([a-z0-9])([A-Z])|([A-Z]+)([A-Z][a-z])`)
	snake := re.ReplaceAllString(s, `${1}${3}_${2}${4}`)
	return strings.ToUpper(snake)
}

func init() {
	int64Overrides = map[ConfigName]int64{
		Chain_BlockTimeSeconds:        1,
		Node_SetDesired:               12,
		Churn_IntervalMinutes:         1,
		Churn_RetryIntervalMinutes:    1,
		Keygen_FailJailMinutes:        1,
		Keysign_FailJailMinutes:       1,
		Withdrawal_BatchWindowMinutes: 1,
		Node_MissingBlocksChurnOut:    100,
		Node_MissingBlocksChurnOutMax: 5,
		Config_OperationalVotesMin:    1, // For regtest single-signer Config changes without Admin
	}
	boolOverrides = map[ConfigName]bool{}
	stringOverrides = map[ConfigName]string{}

	v1Values := NewConfigValue()

	// allow overrides from environment variables in mocknet
	for k := range v1Values.int64values {
		env := CamelToSnakeUpper(k.String())
		if os.Getenv(env) != "" {
			int64Overrides[k], _ = strconv.ParseInt(os.Getenv(env), 10, 64)
		}
	}
	for k := range v1Values.boolValues {
		env := CamelToSnakeUpper(k.String())
		if os.Getenv(env) != "" {
			boolOverrides[k], _ = strconv.ParseBool(os.Getenv(env))
		}
	}
	for k := range v1Values.stringValues {
		env := CamelToSnakeUpper(k.String())
		if os.Getenv(env) != "" {
			stringOverrides[k] = os.Getenv(env)
		}
	}
}
