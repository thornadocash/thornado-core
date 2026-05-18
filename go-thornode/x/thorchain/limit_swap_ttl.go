package thorchain

import "gitlab.com/thorchain/thornode/v3/x/thorchain/types"

// getLimitSwapTTL returns the effective TTL for a limit swap.
// State.Interval is treated as custom TTL when in (0, maxAge], otherwise maxAge.
func getLimitSwapTTL(msg types.MsgSwap, maxAge int64) int64 {
	if maxAge <= 0 {
		return 0
	}
	if msg.State != nil && msg.State.Interval > 0 && msg.State.Interval <= uint64(maxAge) {
		return int64(msg.State.Interval)
	}
	return maxAge
}
