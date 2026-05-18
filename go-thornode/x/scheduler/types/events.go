package types

// event types
//
//nolint:gosec
const (
	EventScheduleMsg     = "schedule_add"
	EventExecuteMsg      = "schedule_execute"
	EventExecuteErrorMsg = "schedule_execute_error"

	AttributeAfter  = "after"
	AttributeHeight = "height"
	AttributeSender = "sender"
	AttributeMsg    = "msg"
	AttributeError  = "error"
)
