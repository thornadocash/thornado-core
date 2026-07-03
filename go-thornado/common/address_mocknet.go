//go:build mocknet
// +build mocknet

package common

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/btcutil"
)

// newAddress in this file with build tags checks Mocknet(/Testnet)-specific addresses.
func newAddress(address string) (Address, error) {
	var outputAddr interface{}

	outputAddr, err := btcutil.DecodeAddress(address, &chaincfg.TestNet3Params)
	switch outputAddr.(type) {
	case *btcutil.AddressPubKey:
		// AddressPubKey format is not supported by Thornado.
	default:
		if err == nil {
			return Address(address), nil
		}
	}

	outputAddr, err = btcutil.DecodeAddress(address, &chaincfg.RegressionNetParams)
	switch outputAddr.(type) {
	case *btcutil.AddressPubKey:
	default:
		if err == nil {
			return Address(address), nil
		}
	}

	return NoAddress, fmt.Errorf("address format not supported: %s", address)
}
