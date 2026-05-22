package thornado

import (
	"github.com/cometbft/cometbft/crypto"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

type nodeAddressValidatorAddressPair struct {
	nodeAddress      cosmos.AccAddress
	validatorAddress crypto.Address
}
