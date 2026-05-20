package common

import (
	"fmt"
	"math/big"
)

func ConvertDecimals(
	amount *big.Int,
	fromDecimals, toDecimals int,
) (*big.Int, error) {
	if fromDecimals < 0 || toDecimals < 0 {
		return nil, fmt.Errorf("decimals must be positive")
	}

	if fromDecimals == toDecimals {
		return new(big.Int).Set(amount), nil
	}

	exp := new(big.Int).Abs(big.NewInt(int64(fromDecimals - toDecimals)))
	diff := new(big.Int).Exp(big.NewInt(10), exp, nil)

	if fromDecimals > toDecimals {
		return new(big.Int).Div(amount, diff), nil
	} else {
		return new(big.Int).Mul(amount, diff), nil
	}
}
