//go:build !mocknet
// +build !mocknet

package common

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
)

// newAddress in this file with build tags checks Mainnet(/Stagenet)-specific addresses.
func newAddress(address string) (Address, error) {
	var outputAddr interface{}

	outputAddr, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	switch outputAddr.(type) {
	case *btcutil.AddressPubKey:
		// AddressPubKey format is not supported by THORChain.
	default:
		if err == nil {
			return Address(address), nil
		}
	}

	return NoAddress, fmt.Errorf("address format not supported: %s", address)
}
