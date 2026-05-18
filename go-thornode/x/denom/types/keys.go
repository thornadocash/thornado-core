package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "denom"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// QuerierRoute defines the module's query routing key
	QuerierRoute = ModuleName
)

var (
	DenomAdminKey    = "admins"
	DenomAdminPrefix = collections.NewPrefix(0)
)
