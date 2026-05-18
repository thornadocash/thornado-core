package types

import (
	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultGenesis returns the default Capability genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Admins: []GenesisDenom{},
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	seenDenoms := map[string]bool{}

	for _, denom := range gs.GetAdmins() {
		if seenDenoms[denom.GetDenom()] {
			return errors.Wrapf(ErrInvalidGenesis, "duplicate denom: %s", denom.GetDenom())
		}
		seenDenoms[denom.GetDenom()] = true

		_, err := DeconstructDenom(denom.GetDenom())
		if err != nil {
			return err
		}

		if denom.Admin != "" {
			_, err = sdk.AccAddressFromBech32(denom.Admin)
			if err != nil {
				return errors.Wrapf(ErrInvalidAdmin, "Invalid admin address (%s)", err)
			}
		}
	}

	return nil
}
