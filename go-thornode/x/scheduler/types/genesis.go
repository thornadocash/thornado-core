package types

import (
	"cosmossdk.io/errors"
)

// DefaultGenesis returns the default Capability genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Schedules: []Schedule{},
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	for _, schedule := range gs.GetSchedules() {
		if schedule.Height <= 0 {
			return errors.Wrapf(ErrInvalidGenesis, "invalid height: %d", schedule.Height)
		}
		for _, msg := range schedule.Msgs {
			if err := msg.ValidateBasic(); err != nil {
				return errors.Wrapf(ErrInvalidGenesis, "invalid message at height %d: %v", schedule.Height, err)
			}
		}
	}
	return nil
}
