package common

import (
	"math/big"
	"sort"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// Gas coins
type Gas Coins

var (
	evmTransferFee = cosmos.NewUint(21000)
	evmGasPerByte  = cosmos.NewUint(68)
)

func GetEVMGasFee(chain Chain, gasPrice *big.Int, msgLen uint64) Gas {
	gasBytes := evmGasPerByte.MulUint64(msgLen)
	return Gas{
		{Asset: chain.GetGasAsset(), Amount: evmTransferFee.Add(gasBytes).Mul(cosmos.NewUintFromBigInt(gasPrice))},
	}
}

func MakeEVMGas(chain Chain, gasPrice *big.Int, gas uint64, layer1Fee *big.Int) Gas {
	unroundedGasAmt := cosmos.NewUint(gas).Mul(cosmos.NewUintFromBigInt(gasPrice))

	// If there's a separate layer1Fee (for instance for BASEChain), add it before rounding.
	if layer1Fee != nil {
		unroundedGasAmt = unroundedGasAmt.Add(cosmos.NewUintFromBigInt(layer1Fee))
	}

	roundedGasAmt := unroundedGasAmt.QuoUint64(1e10) // EVM's 1e18 / 1e10 -> THORChain's 1e8
	if unroundedGasAmt.GT(roundedGasAmt.MulUint64(1e10)) || roundedGasAmt.IsZero() {
		// Round gas amount up rather than down,
		// to increase rather than decrease solvency.
		roundedGasAmt = roundedGasAmt.Add(cosmos.OneUint())
	}

	return Gas{
		{Asset: chain.GetGasAsset(), Amount: roundedGasAmt},
	}
}

// Valid return nil when it is valid, otherwise return an error
func (g Gas) Valid() error {
	for _, coin := range g {
		if err := coin.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// IsEmpty return true as long as there is one coin in it that is not empty
func (g Gas) IsEmpty() bool {
	for _, coin := range g {
		if !coin.IsEmpty() {
			return false
		}
	}
	return true
}

// Coins Add for Gas.
func (gas Gas) Add(addCoins ...Coin) Gas {
	return Gas(Coins(gas).Add(addCoins...))
}

// Coins SafeSub for Gas.
func (gas Gas) SafeSub(subCoins ...Coin) Gas {
	return Gas(Coins(gas).SafeSub(subCoins...))
}

// Add combines two gas objects into one, adding amounts where needed
// or appending new coins.
// **WARNING**: dangerous, returns self-reference and also self-modifies
func (g Gas) Adds_deprecated(g2 Gas) Gas {
	var newGasCoins Gas
	for _, gc2 := range g2 {
		matched := false
		for i, gc1 := range g {
			if gc1.Asset.Equals(gc2.Asset) {
				g[i].Amount = g[i].Amount.Add(gc2.Amount)
				matched = true
			}
		}
		if !matched {
			newGasCoins = append(newGasCoins, gc2)
		}
	}

	return append(g, newGasCoins...)
}

// Equals Check if two lists of coins are equal to each other. Order does not matter
func (g Gas) Equals(gas2 Gas) bool {
	if len(g) != len(gas2) {
		return false
	}

	// sort both lists
	sort.Slice(g[:], func(i, j int) bool {
		return g[i].Asset.String() < g[j].Asset.String()
	})
	sort.Slice(gas2[:], func(i, j int) bool {
		return gas2[i].Asset.String() < gas2[j].Asset.String()
	})

	for i := range g {
		if !g[i].Equals(gas2[i]) {
			return false
		}
	}

	return true
}

// ToCoins convert the gas to Coins
func (g Gas) ToCoins() Coins {
	coins := make(Coins, len(g))
	for i := range g {
		coins[i] = NewCoin(g[i].Asset, g[i].Amount)
	}
	return coins
}

// NoneEmpty returns a new Gas which ignores any coin which is empty
// either Coin asset is empty or amount is empty
func (g Gas) NoneEmpty() Gas {
	newGas := Gas{}
	for _, item := range g {
		if item.IsEmpty() {
			continue
		}
		newGas = append(newGas, item)
	}
	return newGas
}
