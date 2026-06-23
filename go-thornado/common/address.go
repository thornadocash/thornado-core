package common

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/bech32"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

type Address string

const (
	NoAddress         = Address("")
	NoopAddress       = Address("noop")
	BondEscrowAddress = Address("bond_escrow")
)

var alphaNumRegex = regexp.MustCompile("^[:_A-Za-z0-9]*$")

// NewAddress creates a new Address. Supports Thornado bech32 account and Bitcoin addresses.
func NewAddress(address string) (Address, error) {
	if len(address) == 0 {
		return NoAddress, nil
	}
	if strings.EqualFold(address, BondEscrowAddress.String()) {
		return BondEscrowAddress, nil
	}
	if strings.EqualFold(address, NoopAddress.String()) {
		return NoopAddress, nil
	}

	if !alphaNumRegex.MatchString(address) {
		return NoAddress, fmt.Errorf("address format not supported: %s", address)
	}

	// Check bech32 addresses before network-specific Bitcoin formats.
	_, _, err := bech32.Decode(address)
	if err == nil {
		return Address(address), nil
	}

	// Network-specific (with build tags) address checking.
	return newAddress(address)
}

func (addr Address) IsChain(chain Chain) bool {
	switch chain {
	case BTCChain:
		prefix, _, err := bech32.Decode(addr.String())
		if err == nil && (prefix == "bc" || prefix == "tb") {
			return true
		}
		// Check mainnet other formats
		_, err = btcutil.DecodeAddress(addr.String(), &chaincfg.MainNetParams)
		if err == nil {
			return true
		}
		// Check testnet other formats
		_, err = btcutil.DecodeAddress(addr.String(), &chaincfg.TestNet3Params)
		if err == nil {
			return true
		}
		_, err = btcutil.DecodeAddress(addr.String(), &chaincfg.RegressionNetParams)
		if err == nil {
			return true
		}
		return false
	default:
		return false
	}
}

func (addr Address) GetChain() Chain {
	for _, chain := range []Chain{BTCChain} {
		if addr.IsChain(chain) {
			return chain
		}
	}
	var chain Chain
	return chain
}

func (addr Address) GetNetwork(chain Chain) ChainNetwork {
	currentNetwork := CurrentChainNetwork
	mainNetPredicate := func() ChainNetwork {
		if currentNetwork == MockNet {
			return MainNet
		}
		return currentNetwork
	}
	switch chain {
	case BTCChain:
		prefix, _, _ := bech32.Decode(addr.String())
		switch prefix {
		case "bc":
			return mainNetPredicate()
		case "bcrt", "tb":
			return MockNet
		default:
			_, err := btcutil.DecodeAddress(addr.String(), &chaincfg.MainNetParams)
			if err == nil {
				return mainNetPredicate()
			}
			_, err = btcutil.DecodeAddress(addr.String(), &chaincfg.TestNet3Params)
			if err == nil {
				return MockNet
			}
			_, err = btcutil.DecodeAddress(addr.String(), &chaincfg.RegressionNetParams)
			if err == nil {
				return MockNet
			}
		}
	}
	return currentNetwork
}

func (addr Address) AccAddress() (cosmos.AccAddress, error) {
	return cosmos.AccAddressFromBech32(addr.String())
}

func (addr Address) Equals(addr2 Address) bool {
	return strings.EqualFold(addr.String(), addr2.String())
}

func (addr Address) IsEmpty() bool {
	return strings.TrimSpace(addr.String()) == ""
}

func (addr Address) IsNoop() bool {
	return addr.Equals(NoopAddress)
}

func (addr Address) IsBondEscrow() bool {
	return addr.Equals(BondEscrowAddress)
}

func (addr Address) String() string {
	return string(addr)
}

func (addr Address) MappedAccAddress() (cosmos.AccAddress, error) {
	_, data, err := bech32.Decode(addr.String())
	if err != nil {
		return nil, err
	}
	encoded, err := bech32.Encode(sdk.GetConfig().GetBech32AccountAddrPrefix(), data)
	if err != nil {
		return nil, err
	}

	return cosmos.AccAddressFromBech32(encoded)
}
