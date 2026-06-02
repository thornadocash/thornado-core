package common

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/blang/semver"
	"github.com/gogo/protobuf/jsonpb"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	// EmptyAsset empty asset, not valid
	EmptyAsset = Asset{Symbol: "", Ticker: ""}
	// BTCAsset BTC
	BTCAsset = Asset{Chain: BTCChain, Symbol: "BTC", Ticker: "BTC"}
)

var _ sdk.CustomProtobufType = (*Asset)(nil)

// NewAsset parse the given input into Asset object
func NewAsset(input string) (Asset, error) {
	var err error
	var asset Asset
	var sym string
	var parts []string
	re := regexp.MustCompile("[.-]")

	match := re.FindString(input)

	switch match {
	case "-":
		parts = strings.SplitN(input, match, 2)
		asset.Secured = true
	case ".":
		parts = strings.SplitN(input, match, 2)
	case "":
		parts = []string{input}
	}
	if len(parts) == 1 {
		asset.Chain = BTCChain
		sym = parts[0]
	} else {
		asset.Chain, err = NewChain(parts[0])
		if err != nil {
			return EmptyAsset, err
		}
		sym = parts[1]
	}

	asset.Symbol, err = NewSymbol(sym)
	if err != nil {
		return EmptyAsset, err
	}

	parts = strings.SplitN(sym, "-", 2)
	asset.Ticker, err = NewTicker(parts[0])
	if err != nil {
		return EmptyAsset, err
	}

	return asset, nil
}

func NewAssetWithShortCodes(version semver.Version, input string) (Asset, error) {
	return NewAssetWithShortCodesV3_1_0(input)
}

func NewAssetWithShortCodesV3_1_0(input string) (Asset, error) {
	shorts := make(map[string]string)

	shorts[BTCAsset.ShortCode()] = BTCAsset.String()

	long, ok := shorts[input]
	if ok {
		input = long
	}

	return NewAsset(input)
}

func (a Asset) Valid() error {
	if err := a.Chain.Valid(); err != nil {
		return fmt.Errorf("invalid chain: %w", err)
	}
	if err := a.Symbol.Valid(); err != nil {
		return fmt.Errorf("invalid symbol: %w", err)
	}
	return nil
}

// Equals determinate whether two assets are equivalent
func (a Asset) Equals(a2 Asset) bool {
	return a.Chain.Equals(a2.Chain) && a.Symbol.Equals(a2.Symbol) && a.Ticker.Equals(a2.Ticker) && a.Secured == a2.Secured
}

func (a Asset) GetChain() Chain {
	return a.Chain
}

// Get layer1 asset version
func (a Asset) GetLayer1Asset() Asset {
	if !a.IsSecuredAsset() {
		return a
	}
	return Asset{
		Chain:   a.Chain,
		Symbol:  a.Symbol,
		Ticker:  a.Ticker,
		Secured: false,
	}
}

// Get secured asset of asset
func (a Asset) GetSecuredAsset() Asset {
	if a.IsSecuredAsset() {
		return a
	}
	return Asset{
		Chain:   a.Chain,
		Symbol:  a.Symbol,
		Ticker:  a.Ticker,
		Secured: true,
	}
}

func (a Asset) IsSecuredAsset() bool {
	return a.Secured
}

func (a Asset) IsVaultAsset() bool {
	return false
}

// Native returns the native denomination for the asset.
func (a Asset) Native() string {
	return strings.ToLower(a.String())
}

// IsEmpty will be true when any of the field is empty, chain,symbol or ticker
func (a Asset) IsEmpty() bool {
	return a.Chain.IsEmpty() || a.Symbol.IsEmpty() || a.Ticker.IsEmpty()
}

// String implement fmt.Stringer , return the string representation of Asset
func (a Asset) String() string {
	div := "."
	if a.Secured {
		div = "-"
	}
	return fmt.Sprintf("%s%s%s", a.Chain.String(), div, a.Symbol.String())
}

// ShortCode returns the short code for the asset.
func (a Asset) ShortCode() string {
	switch a.String() {
	case "BTC.BTC":
		return "b"
	default:
		return ""
	}
}

// IsGasAsset check whether asset is base asset used to pay for gas
func (a Asset) IsGasAsset() bool {
	gasAsset := a.GetChain().GetGasAsset()
	if gasAsset.IsEmpty() {
		return false
	}
	return a.Equals(gasAsset)
}

// IsWhitelisted is retained for compatibility; Thornado has no internal whitelist.
func (a Asset) IsWhitelisted() bool {
	return false
}

// IsNative returns false because Thornado has no internal chain asset.
func (a Asset) IsNative() bool {
	return false
}

func (a Asset) IsExternalL1Asset() bool {
	return !a.IsSecuredAsset() && !a.IsNative()
}

// MarshalJSON implement Marshaler interface
func (a Asset) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// UnmarshalJSON implement Unmarshaler interface
func (a *Asset) UnmarshalJSON(data []byte) error {
	var err error
	var assetStr string
	if err = json.Unmarshal(data, &assetStr); err != nil {
		return err
	}
	if assetStr == "." {
		*a = EmptyAsset
		return nil
	}
	*a, err = NewAsset(assetStr)
	return err
}

// MarshalJSONPB implement jsonpb.Marshaler
func (a Asset) MarshalJSONPB(*jsonpb.Marshaler) ([]byte, error) {
	return a.MarshalJSON()
}

// UnmarshalJSONPB implement jsonpb.Unmarshaler
func (a *Asset) UnmarshalJSONPB(unmarshal *jsonpb.Unmarshaler, content []byte) error {
	return a.UnmarshalJSON(content)
}

// Replace pool name "." with a "-" for Config key checking.
func (a Asset) ConfigString() string {
	return a.Chain.String() + "-" + a.Symbol.String()
}
