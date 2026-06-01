package types

const (
	// ModuleName name of Thornado module
	ModuleName = "thornado"
	// DefaultCodespace is the same as ModuleName
	DefaultCodespace = ModuleName
	// ReserveName the module account name to keep reserve
	ReserveName = "reserve"
	// BaseName the module account name to keep base fund
	BaseName = "base"
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
