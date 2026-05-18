package providers

import (
	"fmt"
	"math/big"
)

func strToFloat(s string) *big.Float {
	f, _ := new(big.Float).SetString(s)
	return f
}

func checkFloat(v *big.Float) error {
	if v == nil {
		return fmt.Errorf("value is nil")
	}

	if v.Sign() <= 0 {
		return fmt.Errorf("value is negative or zero")
	}

	return nil
}
