package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/denom module sentinel errors
var (
	ErrDenomExists     = errors.Register(ModuleName, 1, "denom already exists")
	ErrUnauthorized    = errors.Register(ModuleName, 2, "unauthorized account")
	ErrInvalidDenom    = errors.Register(ModuleName, 3, "invalid denom")
	ErrInvalidAdmin    = errors.Register(ModuleName, 4, "invalid admin")
	ErrInvalidGenesis  = errors.Register(ModuleName, 5, "invalid genesis")
	ErrInvalidMetadata = errors.Register(ModuleName, 6, "invalid metadata")
)
