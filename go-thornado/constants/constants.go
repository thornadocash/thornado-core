// Package constants  contains all the constants used by thorchain
// by default all the settings in this is for mainnet
package constants

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blang/semver"
)

var (
	GitCommit       = "null"  // sha1 revision used to build the program
	BuildTime       = "null"  // when the executable was built
	Version         = "0.1.0" // software version
	int64Overrides  = map[ConstantName]int64{}
	boolOverrides   = map[ConstantName]bool{}
	stringOverrides = map[ConstantName]string{}
)

var SWVersion, _ = semver.Make(Version)

// max basis points
const MaxBasisPts = uint64(10_000)

// MaxMemoSize Maximum Memo Size
const MaxMemoSize = 250

// MaxDepositSaltSize limits MsgDeposit salt length in bytes.
const MaxDepositSaltSize = 64

// TODO: remove on hard fork.
// StreamingSwapMinBPFee multiplier. This is used to allow decimal points for
// streaming swap math
const StreamingSwapMinBPFeeMulti = int64(100)

// "width" of a volume bucket (15min)
const VolumeBucketSeconds = int64(900)

// used to preserve precision when determining the dollar price of rune.
const DollarMulti = 1e9

// Per-chain maximum gas for a single transaction on EVM chains.
const (
	MaxETHGas  = 50000000
	MaxAVAXGas = 100e8
	MaxBSCGas  = 50000000
	MaxBASEGas = 50000000
	MaxPOLGas  = 50000000

	// DefaultMaxEVMGas is a conservative default for EVM chains not explicitly listed above.
	DefaultMaxEVMGas = 50000000
)

// max amount of data that can be provided with OP_RETURN (bytes)
const MaxOpReturnDataSize = 80

// when using fake transactions to encode further memo information, support up
// to eight fake addresses (20 bytes each):
// 80 (op_return) + 8 * 20 (addresses) - 1 ('^' marker)
const MaxMemoSizeUtxoExtended = MaxOpReturnDataSize - 1 + 8*20

// The provided key must be comparable and should not be of type string or any other built-in type to avoid collisions between packages using context. Users of WithValue should define their own types for keys. To avoid allocating when assigning to an interface{}, context keys often have concrete type struct{}. Alternatively, exported context key variables' static type should be a pointer or interface.
type contextKey string

const (
	CtxMetricLabels   contextKey = "metricLabels"
	CtxObservedTx     contextKey = "observed-tx"
	CtxSimulationMode contextKey = "simulation-mode"
	CtxWASMQuery      contextKey = "wasm-query"
)

// Permitted characters in Mimirs
const MimirKeyRegex = `^[a-zA-Z0-9-]+$`

// Maximum length of a mimir (in bytes)
// If increasing this value, be sure to adjust test/regression/suites/mimir/mimir.yaml
const MaxMimirLength = 128

// ConstantVals implement ConstantValues interface
type ConstantVals struct {
	int64values  map[ConstantName]int64
	boolValues   map[ConstantName]bool
	stringValues map[ConstantName]string
}

// GetInt64Value get value in int64 type, if it doesn't exist then it will return the default value of int64, which is 0
func (cv *ConstantVals) GetInt64Value(name ConstantName) int64 {
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
func (cv *ConstantVals) GetBoolValue(name ConstantName) bool {
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
func (cv *ConstantVals) GetStringValue(name ConstantName) string {
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

func (cv *ConstantVals) String() string {
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

type ConstantValsByKeyname struct {
	Int64Values  map[string]int64  `json:"int_64_values"`
	BoolValues   map[string]bool   `json:"bool_values"`
	StringValues map[string]string `json:"string_values"`
}

func (cv ConstantVals) GetConstantValsByKeyname() ConstantValsByKeyname {
	result := ConstantValsByKeyname{}
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
func (cv ConstantVals) MarshalJSON() ([]byte, error) {
	result := cv.GetConstantValsByKeyname()
	return json.MarshalIndent(result, "", "	")
}
