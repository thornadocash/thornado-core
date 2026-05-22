package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// MaxAffiliateFeeBasisPoints basis points for withdrawals
const MaxAffiliateFeeBasisPoints = 1_000

var (
	_ sdk.Msg              = &MsgSwap{}
	_ sdk.HasValidateBasic = &MsgSwap{}
	_ sdk.LegacyMsg        = &MsgSwap{}
)

// NewMsgSwap is a constructor function for MsgSwap
func NewMsgSwap(tx common.Tx, target common.Asset, destination common.Address, tradeTarget cosmos.Uint, affAddr common.Address, affPts cosmos.Uint, agg, aggregatorTargetAddr string, aggregatorTargetLimit *cosmos.Uint, stype SwapType, quan, interval uint64, version SwapVersion, signer cosmos.AccAddress) *MsgSwap {
	return &MsgSwap{
		Tx:                      tx,
		TargetAsset:             target,
		Destination:             destination,
		TradeTarget:             tradeTarget,
		AffiliateAddress:        affAddr,
		AffiliateBasisPoints:    affPts,
		Signer:                  signer,
		Aggregator:              agg,
		AggregatorTargetAddress: aggregatorTargetAddr,
		AggregatorTargetLimit:   aggregatorTargetLimit,
		SwapType:                stype,
		StreamQuantity:          quan,
		StreamInterval:          interval,
		Version:                 version,
		State: &SwapState{
			Quantity:  quan,
			Interval:  interval,
			Deposit:   tx.Coins[0].Amount,
			Withdrawn: cosmos.ZeroUint(),
			In:        cosmos.ZeroUint(),
			Out:       cosmos.ZeroUint(),
		},
	}
}

func (m *MsgSwap) IsLegacyStreaming() bool {
	// TODO: delete this function when retiring V1 swap manager
	return m.StreamInterval > 0 && m.IsV1()
}

func (m *MsgSwap) IsV1() bool {
	return m.Version == SwapVersion_v1
}

func (m *MsgSwap) IsV2() bool {
	return m.Version == SwapVersion_v2
}

func (m *MsgSwap) IsMarketSwap() bool {
	return m.SwapType == SwapType_market
}

func (m *MsgSwap) IsLimitSwap() bool {
	return m.SwapType == SwapType_limit
}

func (m *MsgSwap) IsStreaming() bool {
	return m.State.Quantity > 1
}

func (m *MsgSwap) IsDone() bool {
	switch m.SwapType {
	case SwapType_market:
		return m.State.Count >= m.State.Quantity
	case SwapType_limit:
		return m.State.In.Equal(m.State.Deposit)
	default:
		return false
	}
}

func (m *MsgSwap) NextSize() (cosmos.Uint, cosmos.Uint) {
	// dev error: should never see a swap quantity of zero
	if m.State.Quantity == 0 {
		return cosmos.ZeroUint(), cosmos.ZeroUint()
	}

	// establish default swap size
	swapSize := m.State.Deposit.QuoUint64(m.State.Quantity)

	// Note: Adding "+1" during the division introduces a small but meaningful change in behavior.
	// Without the "+1", we're effectively rounding down when distributing sats across swaps,
	// while with it, we're rounding up, which leads to a more balanced distribution.
	//
	// For example, if we have 5 swaps and a total of 9 sats to distribute:
	//     9 / 5 = 1.8
	//
	// With "+1" rounding (i.e., ceil-like behavior), each swap receives 2 sats,
	// until we exhaust the total, resulting in: [2, 2, 2, 2, 1].
	// Without "+1" (i.e., floor behavior), each swap gets only 1 sat initially,
	// pushing the remainder to the end: [1, 1, 1, 1, 5].
	//
	// The latter case disproportionately loads the last swap, causing imbalance.
	// Although the difference may seem minor, the "+1" approach yields a more evenly
	// distributed expectation per swap, which is generally preferable in practice.
	remainder := m.State.Deposit.Mod(cosmos.NewUint(m.State.Quantity))
	if m.SuccessCount() < remainder.Uint64() {
		swapSize = swapSize.Add(cosmos.OneUint())
	}

	// sanity check, ensure we never exceed the deposit amount
	if m.State.Deposit.LT(m.State.In.Add(swapSize)) {
		// use remainder of `m.Depost - m.In` instead
		swapSize = common.SafeSub(m.State.Deposit, m.State.In)
	}

	// calculate trade target for this sub-swap
	remainingIn := common.SafeSub(m.State.Deposit, m.State.In) // remaining inbound
	remainingOut := common.SafeSub(m.TradeTarget, m.State.Out) // remaining outbound
	target := common.GetSafeShare(swapSize, remainingIn, remainingOut)

	return swapSize, target
}

func (m *MsgSwap) SuccessCount() uint64 {
	return m.State.Count - m.FailCount()
}

func (m *MsgSwap) FailCount() uint64 {
	return uint64(len(m.State.FailedSwaps))
}

func (m *MsgSwap) GetStreamingSwap() StreamingSwap {
	return NewStreamingSwap(
		m.Tx.ID,
		m.StreamQuantity,
		m.StreamInterval,
		m.TradeTarget,
		m.Tx.Coins[0].Amount,
	)
}

// ValidateBasic runs stateless checks on the message
func (m *MsgSwap) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if err := m.Tx.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if m.TargetAsset.IsEmpty() {
		return cosmos.ErrUnknownRequest("swap Target cannot be empty")
	}
	if len(m.Tx.Coins) > 1 {
		return cosmos.ErrUnknownRequest("not expecting multiple coins in a swap")
	}
	if m.Tx.Coins.IsEmpty() {
		return cosmos.ErrUnknownRequest("swap coin cannot be empty")
	}
	for _, coin := range m.Tx.Coins {
		if coin.Asset.Equals(m.TargetAsset) {
			return cosmos.ErrUnknownRequest("swap Source and Target cannot be the same.")
		}
	}
	if m.Destination.IsEmpty() {
		return cosmos.ErrUnknownRequest("swap Destination cannot be empty")
	}
	// TODO: remove this check on hardfork
	if m.AffiliateAddress.IsEmpty() && !m.AffiliateBasisPoints.IsZero() {
		return cosmos.ErrUnknownRequest("swap affiliate address is empty while affiliate basis points is non-zero")
	}
	if !m.AffiliateBasisPoints.IsZero() && m.AffiliateBasisPoints.GT(cosmos.NewUint(MaxAffiliateFeeBasisPoints)) {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("affiliate fee basis points can't be more than %d", MaxAffiliateFeeBasisPoints))
	}
	if !m.Destination.IsNoop() && !m.Destination.IsChain(m.TargetAsset.GetChain()) {
		return cosmos.ErrUnknownRequest("swap destination address is not the same chain as the target asset")
	}
	if !m.AffiliateAddress.IsEmpty() && !m.AffiliateAddress.IsChain(common.Thornado) {
		return cosmos.ErrUnknownRequest("swap affiliate address must be a THOR address")
	}
	if len(m.Aggregator) != 0 && len(m.AggregatorTargetAddress) == 0 {
		return cosmos.ErrUnknownRequest("aggregator target asset address is empty")
	}
	if len(m.AggregatorTargetAddress) > 0 && len(m.Aggregator) == 0 {
		return cosmos.ErrUnknownRequest("aggregator is empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgSwap) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func (m *MsgSwap) GetTotalAffiliateFee() cosmos.Uint {
	return common.GetSafeShare(
		m.AffiliateBasisPoints,
		cosmos.NewUint(10000),
		m.Tx.Coins[0].Amount,
	)
}
