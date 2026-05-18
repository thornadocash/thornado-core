package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/denom module sentinel errors
var (
	ErrInvalidGenesis = errors.Register(ModuleName, 1, "invalid genesis")
	ErrInvalidHeight  = errors.Register(ModuleName, 2, "invalid height")
)
