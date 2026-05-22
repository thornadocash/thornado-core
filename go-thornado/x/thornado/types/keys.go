package types

const (
	// ModuleName name of Thornado module
	ModuleName = "thornado"
	// DefaultCodespace is the same as ModuleName
	DefaultCodespace = ModuleName
	// ReserveName the module account name to keep reserve
	ReserveName = "reserve"
	// AsgardName the module account name to keep asgard fund
	AsgardName = "asgard"
	// BondName the name of account used to store bond
	BondName = "bond"
	// LendingName
	LendingName = "lending"
	// TreasuryName the name of the account used for treasury governance
	TreasuryName = "treasury"

	// StoreKey to be used when creating the KVStore
	StoreKey = ModuleName

	RouterKey = ModuleName

	QuerierRoute = ModuleName
)
