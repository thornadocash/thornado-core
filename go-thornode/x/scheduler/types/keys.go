package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "scheduler"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// QuerierRoute defines the module's query routing key
	QuerierRoute = ModuleName
)

var (
	ScheduleKey    = "schedule"
	SchedulePrefix = collections.NewPrefix(0)

	SenderIndexKey    = "sender_index"
	SenderIndexPrefix = collections.NewPrefix(1)
)
