package common

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/codec"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

// PubKey used in thorchain, it should be bech32 encoded string
// thus it will be something like
// tthorpub1addwnpepqt7qug8vk9r3saw8n4r803ydj2g3dqwx0mvq5akhnze86fc536xcycgtrnv
// tthorpub1addwnpepqdqvd4r84lq9m54m5kk9sf4k6kdgavvch723pcgadulxd6ey9u70k6zq8qe
type (
	PubKey  string
	PubKeys []PubKey
)

var (
	EmptyPubKey            PubKey
	EmptyPubKeySet         PubKeySet
	pubkeyToAddressCache   = make(map[string]Address)
	pubkeyToAddressCacheMu = &sync.RWMutex{}
)

////////////////////////////////////////////////////////////////////////////////////////
// PubKey
////////////////////////////////////////////////////////////////////////////////////////

// NewPubKey create a new instance of PubKey
// key is bech32 encoded string
func NewPubKey(key string) (PubKey, error) {
	if len(key) == 0 {
		return EmptyPubKey, nil
	}
	_, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeAccPub, key)
	if err != nil {
		return EmptyPubKey, fmt.Errorf("%s is not bech32 encoded pub key,err : %w", key, err)
	}
	return PubKey(key), nil
}

// NewPubKeyFromCrypto
func NewPubKeyFromCrypto(pk crypto.PubKey) (PubKey, error) {
	tmp, err := codec.FromCmtPubKeyInterface(pk)
	if err != nil {
		return EmptyPubKey, fmt.Errorf("fail to create PubKey from crypto.PubKey,err:%w", err)
	}
	s, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, tmp)
	if err != nil {
		return EmptyPubKey, fmt.Errorf("fail to create PubKey from crypto.PubKey,err:%w", err)
	}
	return PubKey(s), nil
}

// Equals check whether two are the same
func (p PubKey) Equals(pubKey1 PubKey) bool {
	return p == pubKey1
}

// IsEmpty to check whether it is empty
func (p PubKey) IsEmpty() bool {
	return len(p) == 0
}

// String stringer implementation
func (p PubKey) String() string {
	return string(p)
}

func (p PubKey) Secp256K1() (*btcec.PublicKey, error) {
	pk, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeAccPub, string(p))
	if err != nil {
		return nil, err
	}
	return btcec.ParsePubKey(pk.Bytes(), btcec.S256())
}

// GetAddress will return an address for the given chain
func (p PubKey) GetAddress(chain Chain) (Address, error) {
	if p.IsEmpty() {
		return NoAddress, nil
	}

	// cache pubkey to address, since this is expensive with many vaults in pubkey manager
	key := fmt.Sprintf("%s-%s", chain.String(), p.String())
	pubkeyToAddressCacheMu.RLock()
	if v, ok := pubkeyToAddressCache[key]; ok {
		pubkeyToAddressCacheMu.RUnlock()
		return v, nil
	}
	pubkeyToAddressCacheMu.RUnlock()

	chainNetwork := CurrentChainNetwork
	var addressString string
	switch chain {
	case Thornado:
		pk, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeAccPub, string(p))
		if err != nil {
			return NoAddress, err
		}
		str, err := ConvertAndEncode(chain.AddressPrefix(chainNetwork), pk.Address().Bytes())
		if err != nil {
			return NoAddress, fmt.Errorf("fail to bech32 encode the address, err: %w", err)
		}
		addressString = str
	case BTCChain:
		addr, err := DeriveBTCTaprootAddress(p, MainVaultPathIndex)
		if err != nil {
			return NoAddress, fmt.Errorf("fail to derive taproot address, err: %w", err)
		}
		addressString = addr.String()
	default:
		return NoAddress, nil
	}

	address, err := NewAddress(addressString)
	if err != nil {
		return address, fmt.Errorf("NewAddress, addressString %s, %w", addressString, err)
	}
	pubkeyToAddressCacheMu.Lock()
	pubkeyToAddressCache[key] = address
	pubkeyToAddressCacheMu.Unlock()
	return address, nil
}

func (p PubKey) GetThorAddress() (cosmos.AccAddress, error) {
	addr, err := p.GetAddress(Thornado)
	if err != nil {
		return nil, err
	}
	return cosmos.AccAddressFromBech32(addr.String())
}

// MarshalJSON to Marshals to JSON using Bech32
func (p PubKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// UnmarshalJSON to Unmarshal from JSON assuming Bech32 encoding
func (p *PubKey) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	pk, err := NewPubKey(s)
	if err != nil {
		return err
	}
	*p = pk
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////
// PubKeys
////////////////////////////////////////////////////////////////////////////////////////

func (p PubKeys) Valid() error {
	for _, pk := range p {
		if _, err := NewPubKey(pk.String()); err != nil {
			return err
		}
	}
	return nil
}

func (p PubKeys) Contains(pk PubKey) bool {
	for _, pp := range p {
		if pp.Equals(pk) {
			return true
		}
	}
	return false
}

// Equals check whether two pub keys are identical
func (p PubKeys) Equals(newPks PubKeys) bool {
	if len(p) != len(newPks) {
		return false
	}

	source := make(PubKeys, len(p))
	dest := make(PubKeys, len(newPks))
	copy(source, p)
	copy(dest, newPks)

	// sort both lists
	sort.Slice(source[:], func(i, j int) bool {
		return source[i].String() < source[j].String()
	})
	sort.Slice(dest[:], func(i, j int) bool {
		return dest[i].String() < dest[j].String()
	})
	for i := range source {
		if !source[i].Equals(dest[i]) {
			return false
		}
	}
	return true
}

// String implement stringer interface
func (p PubKeys) String() string {
	strs := make([]string, len(p))
	for i := range p {
		strs[i] = p[i].String()
	}
	return strings.Join(strs, ", ")
}

func (p PubKeys) Strings() []string {
	allStrings := make([]string, len(p))
	for i, pk := range p {
		allStrings[i] = pk.String()
	}
	return allStrings
}

func (p PubKeys) Addresses() ([]cosmos.AccAddress, error) {
	var err error
	addrs := make([]cosmos.AccAddress, len(p))
	for i, pk := range p {
		addrs[i], err = pk.GetThorAddress()
		if err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

// ConvertAndEncode converts from a base64 encoded byte string to hex or base32 encoded byte string and then to bech32
func ConvertAndEncode(hrp string, data []byte) (string, error) {
	converted, err := bech32.ConvertBits(data, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("encoding bech32 failed,%w", err)
	}
	return bech32.Encode(hrp, converted)
}

////////////////////////////////////////////////////////////////////////////////////////
// PubKeySet
////////////////////////////////////////////////////////////////////////////////////////

// NewPubKeySet create a new instance of PubKeySet , which contains two keys
func NewPubKeySet(secp256k1, ed25519 PubKey) PubKeySet {
	return PubKeySet{
		Secp256k1: secp256k1,
		Ed25519:   ed25519,
	}
}

// IsEmpty will determinate whether PubKeySet is an empty
func (p PubKeySet) IsEmpty() bool {
	return p.Secp256k1.IsEmpty() || p.Ed25519.IsEmpty()
}

// Equals check whether two PubKeySet are the same
func (p PubKeySet) Equals(pks1 PubKeySet) bool {
	return p.Ed25519.Equals(pks1.Ed25519) && p.Secp256k1.Equals(pks1.Secp256k1)
}

func (p PubKeySet) Contains(pk PubKey) bool {
	return p.Ed25519.Equals(pk) || p.Secp256k1.Equals(pk)
}

// String implements fmt.Stringer
func (p PubKeySet) String() string {
	return fmt.Sprintf(`
	secp256k1: %s
	ed25519: %s
`, p.Secp256k1.String(), p.Ed25519.String())
}

// GetAddress
func (p PubKeySet) GetAddress(chain Chain) (Address, error) {
	switch chain.GetSigningAlgo() {
	case SigningAlgoSecp256k1:
		return p.Secp256k1.GetAddress(chain)
	case SigningAlgoEd25519:
		return p.Ed25519.GetAddress(chain)
	}
	return NoAddress, fmt.Errorf("unknown signing algorithm")
}
