package common

import (
	"errors"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/hashicorp/go-multierror"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

const (
	BTCChain = Chain("BTC")
	Thornado = Chain("THOR")

	SigningAlgoSecp256k1 = SigningAlgo("secp256k1")
	SigningAlgoEd25519   = SigningAlgo("ed25519")
)

var AllChains = [...]Chain{
	BTCChain,
	Thornado,
}

type SigningAlgo string

type Chain string

// Chains represent a slice of Chain
type Chains []Chain

// Valid validates chain format, should consist only of uppercase letters
func (c Chain) Valid() error {
	if len(c) < 3 {
		return errors.New("chain id len is less than 3")
	}
	if len(c) > 10 {
		return errors.New("chain id len is more than 10")
	}
	for _, ch := range string(c) {
		if ch < 'A' || ch > 'Z' {
			return errors.New("chain id can consist only of uppercase letters")
		}
	}
	return nil
}

// NewChain create a new Chain and default the siging_algo to Secp256k1
func NewChain(chainID string) (Chain, error) {
	chain := Chain(strings.ToUpper(chainID))
	if err := chain.Valid(); err != nil {
		return chain, err
	}
	if !chain.Equals(BTCChain) && !chain.Equals(Thornado) {
		return chain, errors.New("unsupported chain")
	}
	return chain, nil
}

// Equals compare two chain to see whether they represent the same chain
func (c Chain) Equals(c2 Chain) bool {
	return strings.EqualFold(c.String(), c2.String())
}

func (c Chain) IsThornado() bool {
	return c.Equals(Thornado)
}

func GetEVMChains() []Chain {
	return nil
}

func GetUTXOChains() []Chain {
	return []Chain{BTCChain}
}

func (c Chain) IsEVM() bool {
	return false
}

// IsUTXO returns true if given chain is a UTXO chain.
func (c Chain) IsUTXO() bool {
	utxoChains := GetUTXOChains()
	for _, utxo := range utxoChains {
		if c.Equals(utxo) {
			return true
		}
	}
	return false
}

// IsEmpty is to determinate whether the chain is empty
func (c Chain) IsEmpty() bool {
	return strings.TrimSpace(c.String()) == ""
}

// String implement fmt.Stringer
func (c Chain) String() string {
	// convert it to upper case again just in case someone created a ticker via Chain("rune")
	return strings.ToUpper(string(c))
}

// GetSigningAlgo get the signing algorithm for the given chain
func (c Chain) GetSigningAlgo() SigningAlgo {
	return SigningAlgoSecp256k1
}

// GetGasAsset chain's base asset
func (c Chain) GetGasAsset() Asset {
	switch c {
	case Thornado:
		return RuneNative
	case BTCChain:
		return BTCAsset
	default:
		return EmptyAsset
	}
}

// GetGasUnits returns the name of the gas unit for each chain
// as well as the number of gas units per 'One'.
// gasRateUnitsPerOne type is cosmos.Uint to avoid uint64 overflow through
// for example .Mul(gasRateUnitsPerOne).QuoUint64(common.One)
// rather than * gasRateUnitsPerOne / common.One .
func (c Chain) GetGasUnits() (gasRateUnits string, gasRateUnitsPerOne cosmos.Uint) {
	switch c {
	case BTCChain:
		return "satsperbyte", cosmos.NewUint(1e8)
	default:
		return "", cosmos.OneUint() // Avoid any divide-by-zero.
	}
}

// NativeGasToThornado converts native gas units to Thornado units (1e8).
func (c Chain) NativeGasToThornado(native cosmos.Uint) cosmos.Uint {
	_, gasRateUnitsPerOne := c.GetGasUnits()
	return native.MulUint64(One).Quo(gasRateUnitsPerOne)
}

// ThornadoToNativeGas converts Thornado units (1e8) to native gas units.
func (c Chain) ThornadoToNativeGas(thornado cosmos.Uint) cosmos.Uint {
	_, gasRateUnitsPerOne := c.GetGasUnits()
	return thornado.Mul(gasRateUnitsPerOne).QuoUint64(One)
}

// GetGasAssetDecimal returns decimals for the gas asset of the given chain. Currently
// Gaia is 1e6 and all others are 1e8. If an external chain's gas asset is larger than
// 1e8, just return cosmos.DefaultCoinDecimals.
func (c Chain) GetGasAssetDecimal() int64 {
	return cosmos.DefaultCoinDecimals
}

// IsValidAddress make sure the address is correct for the chain
// And this also make sure mocknet doesn't use mainnet address vice versa
func (c Chain) IsValidAddress(addr Address) bool {
	network := CurrentChainNetwork
	prefix := c.AddressPrefix(network)
	return strings.HasPrefix(addr.String(), prefix)
}

// AddressPrefix return the address prefix used by the given network (mocknet/mainnet)
func (c Chain) AddressPrefix(cn ChainNetwork) string {
	if c.IsEVM() {
		return "0x"
	}
	switch cn {
	case MockNet:
		switch c {
		case Thornado:
			// TODO update this to use mocknet address prefix
			return types.GetConfig().GetBech32AccountAddrPrefix()
		case BTCChain:
			return chaincfg.RegressionNetParams.Bech32HRPSegwit
		}
	case MainNet, StageNet, ChainNet:
		switch c {
		case Thornado:
			return types.GetConfig().GetBech32AccountAddrPrefix()
		case BTCChain:
			return chaincfg.MainNetParams.Bech32HRPSegwit
		}
	}
	return ""
}

// DustThreshold returns the min dust threshold for each chain
// NOTE: these should all be in 8 decimal places
func (c Chain) DustThreshold() cosmos.Uint {
	switch c {
	case BTCChain:
		return cosmos.NewUint(1_000)
	default:
		return cosmos.ZeroUint()
	}
}

// P2WPKHOutputValue returns the dust value for auxiliary P2WPKH outputs.
func (c Chain) P2WPKHOutputValue() int64 {
	switch c {
	case BTCChain:
		// https://github.com/bitcoin/bitcoin/blob/29.x/src/policy/policy.cpp#L28-L41
		return 294
	default:
		return 0 // unsupported chain
	}
}

// MaxMemoLength returns zero because Thornado does not accept transaction memos.
func (c Chain) MaxMemoLength() int {
	return 0
}

// DefaultCoinbase returns the default coinbase address for each chain, returns 0 if no
// coinbase emission is used. This is used used at the time of writing as a fallback
// value in Bifrost, and for inbound confirmation count estimates in the quote APIs.
func (c Chain) DefaultCoinbase() float64 {
	switch c {
	case BTCChain:
		return 3.125
	default:
		return 0
	}
}

func (c Chain) ApproximateBlockMilliseconds() int64 {
	switch c {
	case BTCChain:
		return 600_000
	case Thornado:
		return 6_000
	default:
		return 0
	}
}

func (c Chain) InboundNotes() string {
	switch c {
	case BTCChain:
		return "First output should be to inbound_address, second output should be change back to self. Do not include data outputs, send below the dust threshold, or use exotic spend scripts, locks, or address formats."
	case Thornado:
		return "Broadcast a MsgDeposit to the Thornado network with the appropriate memo. Do not use multi-in, multi-out transactions."
	default:
		return ""
	}
}

func (c Chain) OutboundNotes() string {
	return ""
}

func NewChains(raw []string) (Chains, error) {
	var returnErr error
	var chains Chains
	for _, c := range raw {
		chain, err := NewChain(c)
		if err == nil {
			chains = append(chains, chain)
		} else {
			returnErr = multierror.Append(returnErr, err)
		}
	}
	return chains, returnErr
}

// Has check whether chain c is in the list
func (chains Chains) Has(c Chain) bool {
	for _, ch := range chains {
		if ch.Equals(c) {
			return true
		}
	}
	return false
}

// Distinct return a distinct set of chains, no duplicates
func (chains Chains) Distinct() Chains {
	var newChains Chains
	for _, chain := range chains {
		if !newChains.Has(chain) {
			newChains = append(newChains, chain)
		}
	}
	return newChains
}

func (chains Chains) Strings() []string {
	strings := make([]string, len(chains))
	for i, c := range chains {
		strings[i] = c.String()
	}
	return strings
}
