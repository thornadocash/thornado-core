package tcysmartcontract

import (
	"slices"

	"gitlab.com/thorchain/thornode/v3/common"
)

func IsTCYSmartContractAddress(address common.Address) bool {
	return slices.Contains(TCYSmartContractAddresses, address.String())
}

func GetTCYSmartContractAddresses() ([]common.Address, error) {
	var result []common.Address
	for _, addr := range TCYSmartContractAddresses {
		a, err := common.NewAddress(addr)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, nil
}
