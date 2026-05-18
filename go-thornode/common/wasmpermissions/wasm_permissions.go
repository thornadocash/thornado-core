package wasmpermissions

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type WasmPermissions struct {
	Store       map[string]bool
	Instantiate map[string]bool
}

func (w WasmPermissions) CanStore(actor sdk.AccAddress) error {
	if w.Store[actor.String()] {
		return nil
	}
	return errors.New("unauthorized")
}

func (w WasmPermissions) CanInstantiate(actor sdk.AccAddress) error {
	if w.Instantiate[actor.String()] {
		return nil
	}
	return errors.New("unauthorized")
}

func GetWasmPermissions() WasmPermissions {
	return WasmPermissionsRaw
}
