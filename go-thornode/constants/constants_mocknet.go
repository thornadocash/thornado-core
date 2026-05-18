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

var ThorchainBlockTime = time.Second

// CamelToSnakeUpper converts a camelCase string to SNAKE_CASE.
// Examples: "PoolCycle" -> "POOL_CYCLE", "L1SlipMinBps" -> "L1_SLIP_MIN_BPS"
func CamelToSnakeUpper(s string) string {
	re := regexp.MustCompile(`([a-z0-9])([A-Z])|([A-Z]+)([A-Z][a-z])`)
	snake := re.ReplaceAllString(s, `${1}${3}_${2}${4}`)
	return strings.ToUpper(snake)
}

func init() {
	int64Overrides = map[ConstantName]int64{
		// ArtificialRagnarokBlockHeight: 200,
		DesiredValidatorSet:                 12,
		ChurnInterval:                       60,
		ChurnRetryInterval:                  30,
		MinimumBondInRune:                   100_000_000, // 1 rune
		MemolessTxnTTL:                      100,
		MemolessTxnMaxUse:                   5, // higher limit for testing
		EnableMemolessOutbound:              1, // Enable memoless outbound for mocknet testing
		ValidatorMaxRewardRatio:             3,
		FundMigrationInterval:               15,
		LiquidityLockUpBlocks:               0,
		MaxRuneSupply:                       500_000_000_00000000,
		JailTimeKeygen:                      10,
		JailTimeKeysign:                     10,
		AsgardSize:                          6,
		StreamingSwapMinBPFee:               100, // TODO: remove on hard fork
		EnableAdvSwapQueue:                  1,
		AdvSwapQueueRapidSwapMax:            1, // For testing rapid swaps
		VirtualMultSynthsBasisPoints:        20_000,
		MinTxOutVolumeThreshold:             2000000_00000000,
		MissingBlockChurnOut:                100,
		MaxMissingBlockChurnOut:             5,
		TxOutDelayRate:                      2000000_00000000,
		MaxSynthPerPoolDepth:                3_500,
		MaxSynthsForSaversYield:             5000,
		AllowWideBlame:                      1,
		TargetOutboundFeeSurplusRune:        10_000_00000000,
		MaxOutboundFeeMultiplierBasisPoints: 30_000,
		MinOutboundFeeMultiplierBasisPoints: 10_00,
		OperationalVotesMin:                 1, // For regtest single-signer Mimir changes without Admin
		PreferredAssetOutboundFeeMultiplier: 100,
		TradeAccountsEnabled:                1,
		MaxAffiliateFeeBasisPoints:          10_000,
		RUNEPoolDepositMaturityBlocks:       0,
		RUNEPoolMaxReserveBackstop:          0,
		SaversEjectInterval:                 60,
		SystemIncomeBurnRateBps:             0,
		DevFundSystemIncomeBps:              0,
		MarketingFundSystemIncomeBps:        0,
		TCYStakeSystemIncomeBps:             0,
		MultipleAffiliatesMaxCount:          5,
		BankSendEnabled:                     1,
	}
	boolOverrides = map[ConstantName]bool{
		StrictBondLiquidityRatio: false,
	}
	stringOverrides = map[ConstantName]string{
		DefaultPoolStatus:    "Available",
		DevFundAddress:       "tthor1qk8c8sfrmfm0tkncs0zxeutc8v5mx3pjj07k4u", // addr_thor_pig in regtest
		MarketingFundAddress: "tthor1qk8c8sfrmfm0tkncs0zxeutc8v5mx3pjj07k4u", // same as dev fund in regtest
		OverSolvencyAddress:  "tthor1qk8c8sfrmfm0tkncs0zxeutc8v5mx3pjj07k4u", // same as dev fund in regtest
	}

	v1Values := NewConstantValue()

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
