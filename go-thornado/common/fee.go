package common

import "fmt"

// NewFee return a new instance of Fee
func NewFee(coins Coins) Fee {
	return Fee{
		Coins: coins,
	}
}

func (f Fee) String() string {
	return fmt.Sprintf("%s", f.Coins.String())
}
