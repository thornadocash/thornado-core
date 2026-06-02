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

// EmitFeeEvent emit a fee event through event manager
func (m *EventMgr) EmitFeeEvent(ctx cosmos.Context, feeEvent *EventFee) error {
	if feeEvent.Fee.Coins.IsEmpty() {
		return nil
	}
	events, err := feeEvent.Events()
	if err != nil {
		return fmt.Errorf("fail to emit fee event: %w", err)
	}
	ctx.EventManager().EmitEvents(events)
	return nil
}
