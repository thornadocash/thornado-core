package common

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	xrp "github.com/Peersyst/xrpl-go/address-codec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/bech32"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dogchaincfg "github.com/eager7/dogd/chaincfg"
	"github.com/eager7/dogutil"
	eth "github.com/ethereum/go-ethereum/common"
	bchchaincfg "github.com/gcash/bchd/chaincfg"
	"github.com/gcash/bchutil"
	ltcchaincfg "github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcutil"
	"github.com/mr-tron/base58"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/utxo/zecutil"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"

	adacommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

type Address string

const (
	NoAddress       = Address("")
	NoopAddress     = Address("noop")
	EVMNullAddress  = Address("0x0000000000000000000000000000000000000000")
	GaiaZeroAddress = Address("cosmos100000000000000000000000000000000708mjz")
	// TreasuryAddress is defined in address_treasury_*.go with build tags for network-specific addresses
)

var alphaNumRegex = regexp.MustCompile("^[:_A-Za-z0-9]*$")

// NewAddress create a new Address. Supports ETH/bech2/BTC/LTC/BCH/DOGE/XRP.
func NewAddress(address string) (Address, error) {
	if len(address) == 0 {
		return NoAddress, nil
	}

	if !alphaNumRegex.MatchString(address) {
		return NoAddress, fmt.Errorf("address format not supported: %s", address)
	}

	// Check is tron address
	if IsValidTRONAddress(address) {
		return Address(address), nil
	}

	if IsValidADAAddress(address) {
		return Address(address), nil
	}

	// Check is eth address
	if eth.IsHexAddress(address) {
		return Address(address), nil
	}

	// Check bech32 addresses, would succeed any string bech32 encoded (e.g. GAIA)
	_, _, err := bech32.Decode(address)
	if err == nil {
		return Address(address), nil
	}

	// Check is xrp address
	if IsValidXRPAddress(address) {
		return Address(address), nil
	}

	// Check is sol address
	if IsValidSOLAddress(address) {
		return Address(address), nil
	}

	if IsValidZECAddress(address) {
		return Address(address), nil
	}

	if IsValidSUIAddress(address) {
		return Address(address), nil
	}

	// Network-specific (with build tags) address checking.
	return newAddress(address)
}

func IsValidXRPAddress(address string) bool {
	// checks checksum and returns prefix (1 byte, 0x00) + account id (20 bytes)
	decoded, err := xrp.Base58CheckDecode(address)
	if err != nil {
		return false
	}

	return len(decoded) == 21 && decoded[0] == 0x00
}

func IsValidTRONAddress(address string) bool {
	if len(address) != 34 || address[:1] != "T" {
		return false
	}

	// prefix (1 byte, 0x41) + address (20 bytes) + checksum (4 bytes)
	decoded, err := base58.Decode(address)
	return err == nil && len(decoded) == 25 && decoded[0] == 0x41
}

func IsValidSOLAddress(address string) bool {
	decoded, err := base58.Decode(address)
	if err != nil {
		return false
	}

	return len(decoded) == 32
}

func IsValidZECAddress(address string) bool {
	// Zcash transparent addresses start with specific prefixes
	// Mainnet: t1 (P2PKH), t3 (P2SH), tex1
	// Testnet: tm (P2PKH), tn (P2SH), textest1
	for _, prefix := range []string{"t1", "t2", "t3", "tm", "tex1", "textest1"} {
		if strings.HasPrefix(address, prefix) {
			var network string
			switch prefix {
			case "t1", "t3", "tex1":
				network = "mainnet"
			case "t2", "tm", "textest1":
				network = "testnet3"
			}

			_, err := zecutil.DecodeAddress(address, network)
			return err == nil
		}
	}
	return false
}

func IsValidSUIAddress(address string) bool {
	if len(address) != 66 || address[:2] != "0x" {
		return false
	}
	_, err := hex.DecodeString(address[2:])
	return err == nil
}

func IsValidADAAddress(address string) bool {
	if !strings.HasPrefix(address, "addr1") && !strings.HasPrefix(address, "addr_test1") {
		return false
	}
	_, err := adacommon.NewAddress(address)
	return err == nil
}

// IsValidBCHAddress determinate whether the address is a valid new BCH address format
func (addr Address) IsValidBCHAddress() bool {
	// Check mainnet other formats
	bchAddr, err := bchutil.DecodeAddress(addr.String(), &bchchaincfg.MainNetParams)
	if err == nil {
		switch bchAddr.(type) {
		case *bchutil.LegacyAddressPubKeyHash, *bchutil.LegacyAddressScriptHash:
			return false
		}
		return true
	}
	bchAddr, err = bchutil.DecodeAddress(addr.String(), &bchchaincfg.TestNet3Params)
	if err == nil {
		switch bchAddr.(type) {
		case *bchutil.LegacyAddressPubKeyHash, *bchutil.LegacyAddressScriptHash:
			return false
		}
		return true
	}
	bchAddr, err = bchutil.DecodeAddress(addr.String(), &bchchaincfg.RegressionNetParams)
	if err == nil {
		switch bchAddr.(type) {
		case *bchutil.LegacyAddressPubKeyHash, *bchutil.LegacyAddressScriptHash:
			return false
		}
		return true
	}
	return false
}

// ConvertToNewBCHAddressFormat convert the given BCH to new address format
func ConvertToNewBCHAddressFormat(addr Address) (Address, error) {
	if !addr.IsChain(BCHChain) {
		return NoAddress, fmt.Errorf("address(%s) is not BCH chain", addr)
	}
	network := CurrentChainNetwork
	var param *bchchaincfg.Params
	switch network {
	case MockNet:
		param = &bchchaincfg.RegressionNetParams
	case MainNet, StageNet, ChainNet:
		param = &bchchaincfg.MainNetParams
	}
	bchAddr, err := bchutil.DecodeAddress(addr.String(), param)
	if err != nil {
		return NoAddress, fmt.Errorf("fail to decode address(%s), %w", addr, err)
	}
	return getBCHAddress(bchAddr, param)
}

func getBCHAddress(address bchutil.Address, cfg *bchchaincfg.Params) (Address, error) {
	switch address.(type) {
	case *bchutil.LegacyAddressPubKeyHash, *bchutil.AddressPubKeyHash:
		h, err := bchutil.NewAddressPubKeyHash(address.ScriptAddress(), cfg)
		if err != nil {
			return NoAddress, fmt.Errorf("fail to convert to new pubkey hash address: %w", err)
		}
		return NewAddress(h.String())
	case *bchutil.LegacyAddressScriptHash, *bchutil.AddressScriptHash:
		h, err := bchutil.NewAddressScriptHashFromHash(address.ScriptAddress(), cfg)
		if err != nil {
			return NoAddress, fmt.Errorf("fail to convert to new address script hash address: %w", err)
		}
		return NewAddress(h.String())
	}
	return NoAddress, fmt.Errorf("invalid address type")
}

// Note that this can have false positives, such as being unable to distinguish between ETH and AVAX.
func (addr Address) IsChain(chain Chain) bool {
	if chain.IsEVM() {
		addrStr := addr.String()
		return len(addrStr) == 42 && addrStr[:2] == "0x"
	}
	switch chain {
	case XRPChain:
		return IsValidXRPAddress(addr.String())
	case TRONChain:
		return IsValidTRONAddress(addr.String())
	case GAIAChain:
		// Note: Gaia does not use a special prefix for testnet
		prefix, _, _ := bech32.Decode(addr.String())
		return prefix == "cosmos"
	case NOBLEChain:
		prefix, _, _ := bech32.Decode(addr.String())
		return prefix == "noble"
	case THORChain:
		prefix, _, _ := bech32.Decode(addr.String())
		return prefix == "thor" || prefix == "tthor" || prefix == "sthor" || prefix == "cthor"
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
		return false
	case LTCChain:
		// Check bech32 (segwit v0) and bech32m (taproot) addresses
		prefix, _, _, err := bech32.DecodeGeneric(addr.String())
		if err == nil && (prefix == "ltc" || prefix == "tltc" || prefix == "rltc") {
			return true
		}
		// Check mainnet other formats
		_, err = ltcutil.DecodeAddress(addr.String(), &ltcchaincfg.MainNetParams)
		if err == nil {
			return true
		}
		// Check testnet other formats
		_, err = ltcutil.DecodeAddress(addr.String(), &ltcchaincfg.TestNet4Params)
		if err == nil {
			return true
		}
		return false
	case BCHChain:
		// Check mainnet other formats
		_, err := bchutil.DecodeAddress(addr.String(), &bchchaincfg.MainNetParams)
		if err == nil {
			return true
		}
		// Check testnet other formats
		_, err = bchutil.DecodeAddress(addr.String(), &bchchaincfg.TestNet3Params)
		if err == nil {
			return true
		}
		// Check mocknet / regression other formats
		_, err = bchutil.DecodeAddress(addr.String(), &bchchaincfg.RegressionNetParams)
		if err == nil {
			return true
		}
		return false
	case DOGEChain:
		// Check mainnet other formats
		_, err := dogutil.DecodeAddress(addr.String(), &dogchaincfg.MainNetParams)
		if err == nil {
			return true
		}
		// Check testnet other formats
		_, err = dogutil.DecodeAddress(addr.String(), &dogchaincfg.TestNet3Params)
		if err == nil {
			return true
		}
		// Check mocknet / regression other formats
		_, err = dogutil.DecodeAddress(addr.String(), &dogchaincfg.RegressionNetParams)
		if err == nil {
			return true
		}
		return false
	case SOLChain:
		return IsValidSOLAddress(addr.String())
	case ZECChain:
		return IsValidZECAddress(addr.String())
	case SUIChain:
		return IsValidSUIAddress(addr.String())
	case ADAChain:
		return IsValidADAAddress(addr.String())
	default:
		return true // if THORNode don't specifically check a chain yet, assume its ok.
	}
}

// Note that this will always return ETHChain for an AVAXChain address,
// so perhaps only use it when determining a network (e.g. mainnet/testnet).
func (addr Address) GetChain() Chain {
	for _, chain := range []Chain{ETHChain, THORChain, BTCChain, LTCChain, BCHChain, DOGEChain, GAIAChain, AVAXChain, XRPChain, ZECChain, ADAChain} {
		if addr.IsChain(chain) {
			return chain
		}
	}
	return EmptyChain
}

func (addr Address) GetNetwork(chain Chain) ChainNetwork {
	currentNetwork := CurrentChainNetwork
	mainNetPredicate := func() ChainNetwork {
		if currentNetwork == MockNet {
			return MainNet
		}
		return currentNetwork
	}
	// EVM addresses don't have different prefixes per network
	if chain.IsEVM() {
		return currentNetwork
	}
	switch chain {
	case THORChain:
		prefix, _, _ := bech32.Decode(addr.String())
		if strings.EqualFold(prefix, "thor") {
			return mainNetPredicate()
		}
		if strings.EqualFold(prefix, "tthor") {
			return MockNet
		}
		if strings.EqualFold(prefix, "sthor") {
			return StageNet
		}
		if strings.EqualFold(prefix, "cthor") {
			return ChainNet
		}
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
	case LTCChain:
		// DecodeGeneric handles both bech32 (segwit v0) and bech32m (taproot)
		prefix, _, _, _ := bech32.DecodeGeneric(addr.String())
		switch prefix {
		case "ltc":
			return mainNetPredicate()
		case "rltc", "tltc":
			return MockNet
		default:
			_, err := ltcutil.DecodeAddress(addr.String(), &ltcchaincfg.MainNetParams)
			if err == nil {
				return mainNetPredicate()
			}
			_, err = ltcutil.DecodeAddress(addr.String(), &ltcchaincfg.TestNet4Params)
			if err == nil {
				return MockNet
			}
			_, err = ltcutil.DecodeAddress(addr.String(), &ltcchaincfg.RegressionNetParams)
			if err == nil {
				return MockNet
			}
		}
	case BCHChain:
		// Check mainnet other formats
		_, err := bchutil.DecodeAddress(addr.String(), &bchchaincfg.MainNetParams)
		if err == nil {
			return mainNetPredicate()
		}
		// Check testnet other formats
		_, err = bchutil.DecodeAddress(addr.String(), &bchchaincfg.TestNet3Params)
		if err == nil {
			return MockNet
		}
		// Check mocknet / regression other formats
		_, err = bchutil.DecodeAddress(addr.String(), &bchchaincfg.RegressionNetParams)
		if err == nil {
			return MockNet
		}
	case DOGEChain:
		// Check mainnet other formats
		_, err := dogutil.DecodeAddress(addr.String(), &dogchaincfg.MainNetParams)
		if err == nil {
			return mainNetPredicate()
		}
		// Check testnet other formats
		_, err = dogutil.DecodeAddress(addr.String(), &dogchaincfg.TestNet3Params)
		if err == nil {
			return MockNet
		}
		// Check mocknet / regression other formats
		_, err = dogutil.DecodeAddress(addr.String(), &dogchaincfg.RegressionNetParams)
		if err == nil {
			return MockNet
		}
	case SOLChain, SUIChain:
		return currentNetwork
	case ZECChain:
		// Determine network based on address prefix
		if !addr.IsChain(chain) {
			return currentNetwork
		}

		for _, prefix := range []string{"tex1", "t1", "t3", "textest1", "tm", "t2"} {
			if strings.HasPrefix(addr.String(), prefix) {
				switch prefix {
				case "t1", "t3", "tex1":
					return mainNetPredicate()
				case "t2", "tm", "textest1":
					return MockNet
				}
			}
		}
	case ADAChain:
		addrStr := addr.String()
		if strings.HasPrefix(addrStr, "addr1") {
			return mainNetPredicate()
		}

		if strings.HasPrefix(addrStr, "addr_test1") {
			return MockNet
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

func (addr Address) String() string {
	return string(addr)
}

func (addr Address) MappedAccAddress() (cosmos.AccAddress, error) {
	if addr.GetChain().IsEVM() {
		return cosmos.AccAddressFromHexUnsafe(
			strings.TrimPrefix(addr.String(), "0x"),
		)
	}

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

// ToTexAddress returns the tex address for a Zcash P2PKH address
func (addr Address) ToTexAddress() (Address, error) {
	addrStr := addr.String()

	if !addr.IsChain(ZECChain) {
		return NoAddress, fmt.Errorf("%s is no Zcash address", addrStr)
	}

	if addrStr[:2] != "t1" && addrStr[:2] != "tm" {
		return NoAddress, fmt.Errorf("%s is no P2PKH address", addrStr)
	}

	var network, prefix string
	switch CurrentChainNetwork {
	case MockNet:
		network = "testnet3"
		prefix = "textest"
	case MainNet, StageNet, ChainNet:
		network = "mainnet"
		prefix = "tex"
	}

	zecAddr, err := zecutil.DecodeAddress(addrStr, network)
	if err != nil {
		return NoAddress, err
	}

	var bz []byte
	bz, err = bech32.ConvertBits(zecAddr.ScriptAddress(), 8, 5, true)
	if err != nil {
		return NoAddress, err
	}

	var texAddr string
	texAddr, err = bech32.EncodeM(prefix, bz)
	if err != nil {
		return NoAddress, err
	}

	return Address(texAddr), nil
}
