// Package constants  contains all the constants used by thornado
// by default all the settings in this is for mainnet
package constants

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blang/semver"
)

var (
	GitCommit       = "null"   // sha1 revision used to build the program
	BuildTime       = "null"   // when the executable was built
	Version         = "3.17.0" // software version
	int64Overrides  = map[ConfigName]int64{}
	boolOverrides   = map[ConfigName]bool{}
	stringOverrides = map[ConfigName]string{}
)

var SWVersion, _ = semver.Make(Version)

// max basis points
const MaxBasisPts = uint64(10_000)

func MinutesToBlocks(minutes, blockTimeSeconds int64) int64 {
	if minutes <= 0 {
		return 0
	}
	if blockTimeSeconds <= 0 {
		blockTimeSeconds = NewConfigValue().GetInt64Value(Chain_BlockTimeSeconds)
	}
	seconds := minutes * 60
	return (seconds + blockTimeSeconds - 1) / blockTimeSeconds
}

// The provided key must be comparable and should not be of type string or any other built-in type to avoid collisions between packages using context. Users of WithValue should define their own types for keys. To avoid allocating when assigning to an interface{}, context keys often have concrete type struct{}. Alternatively, exported context key variables' static type should be a pointer or interface.
type contextKey string

const (
	CtxMetricLabels   contextKey = "metricLabels"
	CtxObservedTx     contextKey = "observed-tx"
	CtxSimulationMode contextKey = "simulation-mode"
)

// Permitted characters in Configs
const ConfigKeyRegex = `^[a-zA-Z0-9_-]+$`

// Maximum length of a config (in bytes)
// If increasing this value, be sure to adjust test/regression/suites/config/config.yaml
const MaxConfigLength = 128

// ConfigVals implement ConfigValues interface
type ConfigVals struct {
	int64values  map[ConfigName]int64
	boolValues   map[ConfigName]bool
	stringValues map[ConfigName]string
}

// GetInt64Value get value in int64 type, if it doesn't exist then it will return the default value of int64, which is 0
func (cv *ConfigVals) GetInt64Value(name ConfigName) int64 {
	// check overrides first
	v, ok := int64Overrides[name]
	if ok {
		return v
	}

	v, ok = cv.int64values[name]
	if !ok {
		return 0
	}
	return v
}

// GetBoolValue retrieve a bool constant value from the map
func (cv *ConfigVals) GetBoolValue(name ConfigName) bool {
	v, ok := boolOverrides[name]
	if ok {
		return v
	}
	v, ok = cv.boolValues[name]
	if !ok {
		return false
	}
	return v
}

// GetStringValue retrieve a string const value from the map
func (cv *ConfigVals) GetStringValue(name ConfigName) string {
	v, ok := stringOverrides[name]
	if ok {
		return v
	}
	v, ok = cv.stringValues[name]
	if ok {
		return v
	}
	return ""
}

func (cv *ConfigVals) String() string {
	sb := strings.Builder{}
	// analyze-ignore(map-iteration)
	for k, v := range cv.int64values {
		if overrideValue, ok := int64Overrides[k]; ok {
			sb.WriteString(fmt.Sprintf("%s:%d\n", k, overrideValue))
			continue
		}
		sb.WriteString(fmt.Sprintf("%s:%d\n", k, v))
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.boolValues {
		if overrideValue, ok := boolOverrides[k]; ok {
			sb.WriteString(fmt.Sprintf("%s:%v\n", k, overrideValue))
			continue
		}
		sb.WriteString(fmt.Sprintf("%s:%v\n", k, v))
	}
	return sb.String()
}

type ConfigValsByKeyname struct {
	Int64Values  map[string]int64  `json:"int_64_values"`
	BoolValues   map[string]bool   `json:"bool_values"`
	StringValues map[string]string `json:"string_values"`
}

func (cv ConfigVals) GetConfigValsByKeyname() ConfigValsByKeyname {
	result := ConfigValsByKeyname{}
	result.Int64Values = make(map[string]int64)
	result.BoolValues = make(map[string]bool)
	result.StringValues = make(map[string]string)

	// analyze-ignore(map-iteration)
	for k, v := range cv.int64values {
		result.Int64Values[k.String()] = v
	}
	// analyze-ignore(map-iteration)
	for k, v := range int64Overrides {
		result.Int64Values[k.String()] = v
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.boolValues {
		result.BoolValues[k.String()] = v
	}
	// analyze-ignore(map-iteration)
	for k, v := range boolOverrides {
		result.BoolValues[k.String()] = v
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.stringValues {
		result.StringValues[k.String()] = v
	}
	// analyze-ignore(map-iteration)
	for k, v := range stringOverrides {
		result.StringValues[k.String()] = v
	}

	return result
}

// MarshalJSON marshal result to json format
func (cv ConfigVals) MarshalJSON() ([]byte, error) {
	result := cv.GetConfigValsByKeyname()
	return json.MarshalIndent(result, "", "	")
}
