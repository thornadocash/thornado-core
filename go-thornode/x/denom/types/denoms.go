package types

import (
	"context"
	fmt "fmt"
	"strings"

	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

const ModuleDenomPrefix = "x/"

// GetTokenDenom constructs a denom string for tokens created by denom based on an id
// The denom constructed is x/{id}
func GetTokenDenom(id string) (string, error) {
	denom := ModuleDenomPrefix + id
	return denom, sdk.ValidateDenom(denom)
}

// DeconstructDenom takes a token denom string and verifies that it is a valid
// denom of the denom module, and is of the form `x/{id}`
func DeconstructDenom(denom string) (id string, err error) {
	err = sdk.ValidateDenom(denom)
	if err != nil {
		return "", err
	}

	id, found := strings.CutPrefix(denom, ModuleDenomPrefix)

	if !found {
		return "", errors.Wrapf(ErrInvalidDenom, "denom prefix is incorrect. Should be: %s", ModuleDenomPrefix)
	}

	return id, nil
}

// NewdenomDenomMintCoinsRestriction creates and returns a BankMintingRestrictionFn that only allows minting of
// valid denom denoms
func NewDenomMintCoinsRestriction() banktypes.MintingRestrictionFn {
	return func(_ context.Context, coinsToMint sdk.Coins) error {
		for _, coin := range coinsToMint {
			_, err := DeconstructDenom(coin.Denom)
			if err != nil {
				return fmt.Errorf("does not have permission to mint %s", coin.Denom)
			}
		}
		return nil
	}
}
