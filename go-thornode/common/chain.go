package common

import (
	"errors"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/cosmos/cosmos-sdk/types"
	dogchaincfg "github.com/eager7/dogd/chaincfg"
	"github.com/hashicorp/go-multierror"
	ltcchaincfg "github.com/ltcsuite/ltcd/chaincfg"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

const (
	EmptyChain = Chain("")
	BSCChain   = Chain("BSC")
	ETHChain   = Chain("ETH")
	BTCChain   = Chain("BTC")
	LTCChain   = Chain("LTC")
	BCHChain   = Chain("BCH")
	DOGEChain  = Chain("DOGE")
	THORChain  = Chain("THOR")
	GAIAChain  = Chain("GAIA")
	NOBLEChain = Chain("NOBLE")
	AVAXChain  = Chain("AVAX")
	BASEChain  = Chain("BASE")
	TRONChain  = Chain("TRON")
	XRPChain   = Chain("XRP")
	SOLChain   = Chain("SOL")
	ZECChain   = Chain("ZEC")
	POLChain   = Chain("POL")
	SUIChain   = Chain("SUI")
	ADAChain   = Chain("ADA")

	SigningAlgoSecp256k1 = SigningAlgo("secp256k1")
	SigningAlgoEd25519   = SigningAlgo("ed25519")
)

var AllChains = [...]Chain{
	BSCChain,
	ETHChain,
	BTCChain,
	LTCChain,
	BCHChain,
	DOGEChain,
	THORChain,
	GAIAChain,
	NOBLEChain,
	AVAXChain,
	BASEChain,
	TRONChain,
	XRPChain,
	SOLChain,
	ZECChain,
	POLChain,
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
	return chain, nil
}

// Equals compare two chain to see whether they represent the same chain
func (c Chain) Equals(c2 Chain) bool {
	return strings.EqualFold(c.String(), c2.String())
}

func (c Chain) IsTHORChain() bool {
	return c.Equals(THORChain)
}

func (c Chain) IsBSCChain() bool {
	return c.Equals(BSCChain)
}

// GetEVMChains returns all "EVM" chains connected to THORChain
// "EVM" is defined, in thornode's context, as a chain that:
// - uses 0x as an address prefix
// - has a "Router" Smart Contract
func GetEVMChains() []Chain {
	return []Chain{ETHChain, AVAXChain, BSCChain, BASEChain, POLChain}
}

// GetUTXOChains returns all "UTXO" chains connected to THORChain.
func GetUTXOChains() []Chain {
	return []Chain{BTCChain, LTCChain, BCHChain, DOGEChain, ZECChain}
}

// IsEVM returns true if given chain is an EVM chain.
// See working definition of an "EVM" chain in the
// `GetEVMChains` function description
func (c Chain) IsEVM() bool {
	evmChains := GetEVMChains()
	for _, evm := range evmChains {
		if c.Equals(evm) {
			return true
		}
	}
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
	switch c {
	case SOLChain, SUIChain, ADAChain:
		return SigningAlgoEd25519
	default:
		return SigningAlgoSecp256k1
	}
}

// GetGasAsset chain's base asset
func (c Chain) GetGasAsset() Asset {
	switch c {
	case THORChain:
		return RuneNative
	case BSCChain:
		return BNBBEP20Asset
	case BTCChain:
		return BTCAsset
	case LTCChain:
		return LTCAsset
	case BCHChain:
		return BCHAsset
	case DOGEChain:
		return DOGEAsset
	case ETHChain:
		return ETHAsset
	case AVAXChain:
		return AVAXAsset
	case GAIAChain:
		return ATOMAsset
	case NOBLEChain:
		// Noble doesn't charge gas. `0uusdc` typically provided as fee
		return USDCAsset
	case BASEChain:
		return BaseETHAsset
	case TRONChain:
		return TRXAsset
	case XRPChain:
		return XRPAsset
	case SOLChain:
		return SOLAsset
	case ZECChain:
		return ZECAsset
	case POLChain:
		return POLAsset
	case SUIChain:
		return SUIAsset
	case ADAChain:
		return ADAAsset
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
	case GAIAChain:
		return "uatom", cosmos.NewUint(1e6)
	case NOBLEChain:
		return "uusdc", cosmos.NewUint(1e6)
	case XRPChain:
		return "drop", cosmos.NewUint(1e6)
	case TRONChain:
		return "sun", cosmos.NewUint(1e6)
	case BTCChain, BCHChain, LTCChain, DOGEChain, ZECChain:
		return "satsperbyte", cosmos.NewUint(1e8)
	case ETHChain, BSCChain, POLChain:
		return "gwei", cosmos.NewUint(1e9)
	case AVAXChain:
		return "nAVAX", cosmos.NewUint(1e9)
	case BASEChain:
		return "mwei", cosmos.NewUint(1e12)
	case SOLChain:
		return "lamport", cosmos.NewUint(1e9)
	case SUIChain:
		return "mist", cosmos.NewUint(1e9)
	case ADAChain:
		return "lovelace", cosmos.NewUint(1e6)
	default:
		return "", cosmos.OneUint() // Avoid any divide-by-zero.
	}
}

// NativeGasToThorchain converts native gas units to THORChain units (1e8).
func (c Chain) NativeGasToThorchain(native cosmos.Uint) cosmos.Uint {
	_, gasRateUnitsPerOne := c.GetGasUnits()
	return native.MulUint64(One).Quo(gasRateUnitsPerOne)
}

// ThorchainToNativeGas converts THORChain units (1e8) to native gas units.
func (c Chain) ThorchainToNativeGas(thorchain cosmos.Uint) cosmos.Uint {
	_, gasRateUnitsPerOne := c.GetGasUnits()
	return thorchain.Mul(gasRateUnitsPerOne).QuoUint64(One)
}

// GetGasAssetDecimal returns decimals for the gas asset of the given chain. Currently
// Gaia is 1e6 and all others are 1e8. If an external chain's gas asset is larger than
// 1e8, just return cosmos.DefaultCoinDecimals.
func (c Chain) GetGasAssetDecimal() int64 {
	switch c {
	case GAIAChain, NOBLEChain, TRONChain, ADAChain:
		return 6
	case XRPChain:
		return 6
	case SOLChain, SUIChain:
		return 9
	default:
		return cosmos.DefaultCoinDecimals
	}
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
		case GAIAChain:
			return "cosmos"
		case NOBLEChain:
			return "noble"
		case THORChain:
			// TODO update this to use mocknet address prefix
			return types.GetConfig().GetBech32AccountAddrPrefix()
		case BTCChain:
			return chaincfg.RegressionNetParams.Bech32HRPSegwit
		case LTCChain:
			return ltcchaincfg.RegressionNetParams.Bech32HRPSegwit
		case DOGEChain:
			return dogchaincfg.RegressionNetParams.Bech32HRPSegwit
		case ZECChain:
			return "tm"
		case ADAChain:
			return "addr_test1"
		}
	case MainNet, StageNet, ChainNet:
		switch c {
		case GAIAChain:
			return "cosmos"
		case NOBLEChain:
			return "noble"
		case THORChain:
			return types.GetConfig().GetBech32AccountAddrPrefix()
		case BTCChain:
			return chaincfg.MainNetParams.Bech32HRPSegwit
		case LTCChain:
			return ltcchaincfg.MainNetParams.Bech32HRPSegwit
		case DOGEChain:
			return dogchaincfg.MainNetParams.Bech32HRPSegwit
		case ZECChain:
			return "t1"
		case ADAChain:
			return "addr1"
		}
	}
	return ""
}

// DustThreshold returns the min dust threshold for each chain
// The min dust threshold defines the lower end of the withdraw range of memoless savers txs
// The native coin value provided in a memoless tx defines a basis points amount of Withdraw or Add to a savers position as follows:
// Withdraw range: (dust_threshold + 1) -> (dust_threshold + 10_000)
// Add range: dust_threshold -> Inf
// NOTE: these should all be in 8 decimal places
func (c Chain) DustThreshold() cosmos.Uint {
	switch c {
	case BTCChain:
		return cosmos.NewUint(1_000)
	case LTCChain:
		return cosmos.NewUint(100_000)
	case BCHChain:
		return cosmos.NewUint(10_000)
	case ZECChain:
		return cosmos.NewUint(15_000)
	case DOGEChain:
		return cosmos.NewUint(100_000_000)
	case ETHChain, BASEChain:
		return cosmos.NewUint(1_000)
	case AVAXChain, POLChain:
		return cosmos.NewUint(100_000)
	case BSCChain:
		return cosmos.NewUint(10_000)
	case GAIAChain:
		return cosmos.NewUint(1_000_000)
	case NOBLEChain:
		return cosmos.NewUint(1_000_000)
	case XRPChain:
		// XRP's dust threshold is being set to 1 XRP. This is the base reserve requirement on XRP's ledger.
		// It is set to this value for two reasons:
		//    1. to prevent edge cases of outbound XRP to new addresses where this is the minimum that must be transferred
		//    2. to burn this amount on churns of each XRP vault, effectively leaving it behind as it cannot be transferred, but still transferring all other XRP
		// On churns, we can optionally delete the account to recover an additional .8 XRP, but would increases code complexity and will remove related ledger entries
		// Comparing to BTC, this dust threshold should be reasonable.
		return cosmos.NewUint(One) // 1 XRP
	case SOLChain:
		return cosmos.NewUint(100_000) // 0.001 SOL
	case TRONChain:
		return cosmos.NewUint(10_000_000)
	case SUIChain:
		return cosmos.NewUint(1_000_000)
	case ADAChain:
		return cosmos.NewUint(10_000_000)
	default:
		return cosmos.ZeroUint()
	}
}

// P2WPKHOutputValue returns the value for P2WPKH outputs used for memo data
func (c Chain) P2WPKHOutputValue() int64 {
	switch c {
	case BTCChain:
		// https://github.com/bitcoin/bitcoin/blob/29.x/src/policy/policy.cpp#L28-L41
		return 294
	case LTCChain:
		// dust relay fee in 'lits' is 10x of the fees on btc (30k vs 3k)
		// https://github.com/litecoin-project/litecoin/blob/v0.21.4/src/policy/policy.h#L52
		// https://github.com/litecoin-project/litecoin/blob/v0.21.4/src/policy/policy.cpp#L17-L30
		return 2940
	case DOGEChain:
		// DOGE creates P2PKH scripts for memo overflow outputs and has a dustRelayFee of
		// 1 DOGE/kB (100,000,000 koinu/kB). P2PKH dust = (34 + 148) * 100,000,000 / 1000.
		return 18_200_000
	case BCHChain:
		// using bitcoin default for p2pkh txout
		return 546
	case ZECChain:
		// TODO: validate on node
		return 2730
	default:
		return 0 // unsupported chain
	}
}

// MaxMemoLength returns the max memo length for each chain.
func (c Chain) MaxMemoLength() int {
	switch c {
	case BTCChain, LTCChain, BCHChain, DOGEChain, ZECChain:
		return constants.MaxOpReturnDataSize
	default:
		// Default to the max memo size that we will process, regardless
		// of any higher memo size capable on other chains.
		return constants.MaxMemoSize
	}
}

// DefaultCoinbase returns the default coinbase address for each chain, returns 0 if no
// coinbase emission is used. This is used used at the time of writing as a fallback
// value in Bifrost, and for inbound confirmation count estimates in the quote APIs.
func (c Chain) DefaultCoinbase() float64 {
	switch c {
	case BTCChain:
		return 3.125
	case LTCChain:
		return 6.25
	case BCHChain:
		return 3.125
	case DOGEChain:
		return 10000
	case ZECChain:
		return 1.5625
	default:
		return 0
	}
}

func (c Chain) ApproximateBlockMilliseconds() int64 {
	switch c {
	case BTCChain:
		return 600_000
	case LTCChain:
		return 150_000
	case BCHChain:
		return 600_000
	case DOGEChain:
		return 60_000
	case ETHChain:
		return 12_000
	case AVAXChain:
		return 1_000 // ~1s
	case BSCChain:
		return 450
	case GAIAChain:
		return 6_000
	case THORChain:
		return 6_000
	case BASEChain:
		return 2_000
	case TRONChain:
		return 3_000
	case XRPChain:
		return 4_000 // approx 3-5 seconds
	case NOBLEChain:
		return 1_500
	case SOLChain:
		return 400
	case ZECChain:
		return 75_000
	case POLChain:
		return 2_000
	case SUIChain:
		return 250
	case ADAChain:
		return 20_000
	default:
		return 0
	}
}

func (c Chain) InboundNotes() string {
	switch c {
	case LTCChain:
		return "First output should be to inbound_address, second output should be change back to self, third output should be OP_RETURN, limited to 80 bytes. Do not send below the dust threshold. Do not use exotic spend scripts, locks or address formats. MWEB addresses are not supported; do not use ltcmweb1 addresses or peg-in transactions."
	case BTCChain, BCHChain, DOGEChain, ZECChain:
		return "First output should be to inbound_address, second output should be change back to self, third output should be OP_RETURN, limited to 80 bytes. Do not send below the dust threshold. Do not use exotic spend scripts, locks or address formats."
	case ETHChain, AVAXChain, BSCChain, BASEChain, POLChain:
		return "Base Asset: Send the inbound_address the asset with the memo encoded in hex in the data field. Tokens: First approve router to spend tokens from user: asset.approve(router, amount). Then call router.depositWithExpiry(inbound_address, asset, amount, memo, expiry). Asset is the token contract address. Amount should be in native asset decimals (eg 1e18 for most tokens). Do not swap to smart contract addresses."
	case GAIAChain, NOBLEChain:
		return "Transfer the inbound_address the asset with the memo. Do not use multi-in, multi-out transactions."
	case THORChain:
		return "Broadcast a MsgDeposit to the THORChain network with the appropriate memo. Do not use multi-in, multi-out transactions."
	case XRPChain:
		return "Transfer the inbound_address the asset with the memo. Only a single memo is supported and only MemoData is used."
	default:
		return ""
	}
}

func (c Chain) OutboundNotes() string {
	switch c {
	case XRPChain:
		return "Ensure XRP destination address does not have special flags like lsfRequireDestTag, lsfRequireAuth, lsfDisallowIncomingPayChan, etcetera."
	case LTCChain:
		return "Do not use MWEB (ltcmweb1) addresses as the destination; they are not supported."
	default:
		return ""
	}
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
