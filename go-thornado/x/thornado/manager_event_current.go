package thornado

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

// EmitEventItem define the method all event need to implement
type EmitEventItem interface {
	Events() (cosmos.Events, error)
}

// EventMgr implement EventManager interface
type EventMgr struct{}

// newEventMgr create a new instance of EventMgr
func newEventMgr() *EventMgr {
	return &EventMgr{}
}

// EmitEvent to block
func (m *EventMgr) EmitEvent(ctx cosmos.Context, evt EmitEventItem) error {
	events, err := evt.Events()
	if err != nil {
		return fmt.Errorf("fail to get events: %w", err)
	}
	ctx.EventManager().EmitEvents(events)
	return nil
}

// EmitGasEvent emit gas events
func (m *EventMgr) EmitGasEvent(ctx cosmos.Context, gasEvent *EventGas) error {
	if gasEvent == nil {
		return nil
	}
	return m.EmitEvent(ctx, gasEvent)
}

// EmitSwapEvent emit swap event to block
func (m *EventMgr) EmitSwapEvent(ctx cosmos.Context, swap *EventSwap) error {
	// OutTxs is a temporary field that we used, as for now we need to keep backward compatibility so the
	// events change doesn't break midgard and smoke test, for double swap , we first swap the source asset to RUNE ,
	// and then from RUNE to target asset, so the first will be marked as success
	if !swap.OutTxs.IsEmpty() {
		outboundEvt := NewEventOutbound(swap.InTx.ID, swap.OutTxs)
		if err := m.EmitEvent(ctx, outboundEvt); err != nil {
			return fmt.Errorf("fail to emit an outbound event for double swap: %w", err)
		}
	}
	return m.EmitEvent(ctx, swap)
}

// EmitFeeEvent emit a fee event through event manager
func (m *EventMgr) EmitFeeEvent(ctx cosmos.Context, feeEvent *EventFee) error {
	if feeEvent.Fee.Coins.IsEmpty() && feeEvent.Fee.PoolDeduct.IsZero() {
		return nil
	}
	events, err := feeEvent.Events()
	if err != nil {
		return fmt.Errorf("fail to emit fee event: %w", err)
	}
	ctx.EventManager().EmitEvents(events)
	return nil
}
