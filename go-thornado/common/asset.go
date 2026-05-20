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
	EmptyAsset = Asset{Symbol: "", Ticker: "", Synth: false}
	// BTCAsset BTC
	BTCAsset = Asset{Chain: BTCChain, Symbol: "BTC", Ticker: "BTC", Synth: false}
	// RuneNative RUNE on thorchain
	RuneNative = Asset{Chain: THORChain, Symbol: "RUNE", Ticker: "RUNE", Synth: false}
	TCY        = Asset{Chain: THORChain, Symbol: "TCY", Ticker: "TCY", Synth: false}
	TOR        = Asset{Chain: THORChain, Symbol: "TOR", Ticker: "TOR", Synth: false}
	THORBTC    = Asset{Chain: THORChain, Symbol: "BTC", Ticker: "BTC", Synth: false}
	// Whitelisted assets
	RUJI = Asset{Chain: THORChain, Symbol: "RUJI", Ticker: "RUJI", Synth: false}
	NAMI = Asset{Chain: THORChain, Symbol: "NAMI", Ticker: "NAMI", Synth: false}
	LQDY = Asset{Chain: THORChain, Symbol: "LQDY", Ticker: "LQDY", Synth: false}
	AUTO = Asset{Chain: THORChain, Symbol: "AUTO", Ticker: "AUTO", Synth: false}
	XUSK = Asset{Chain: THORChain, Symbol: "XUSK", Ticker: "XUSK", Synth: false}
)

var _ sdk.CustomProtobufType = (*Asset)(nil)

// NewAsset parse the given input into Asset object
func NewAsset(input string) (Asset, error) {
	var err error
	var asset Asset
	var sym string
	var parts []string
	re := regexp.MustCompile("[~./-]")

	match := re.FindString(input)

	switch match {
	case "~":
		parts = strings.SplitN(input, match, 2)
		asset.Trade = true
	case "/":
		parts = strings.SplitN(input, match, 2)
		asset.Synth = true
	case "-":
		parts = strings.SplitN(input, match, 2)
		asset.Secured = true
	case ".":
		parts = strings.SplitN(input, match, 2)
	case "":
		parts = []string{input}
	}
	if len(parts) == 1 {
		asset.Chain = THORChain
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
	shorts[RuneNative.ShortCode()] = RuneNative.String()

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
	if (a.Synth && a.Trade) || (a.Trade && a.Secured) || (a.Secured && a.Synth) {
		return fmt.Errorf("assets can only be one of trade, synth or secured")
	}
	if a.Synth && a.Chain.IsTHORChain() {
		return fmt.Errorf("synth asset cannot have chain THOR: %s", a)
	}
	if a.Trade && a.Chain.IsTHORChain() {
		return fmt.Errorf("trade asset cannot have chain THOR: %s", a)
	}
	if a.Secured && a.Chain.IsTHORChain() {
		return fmt.Errorf("secured asset cannot have chain THOR: %s", a)
	}
	return nil
}

// Equals determinate whether two assets are equivalent
func (a Asset) Equals(a2 Asset) bool {
	return a.Chain.Equals(a2.Chain) && a.Symbol.Equals(a2.Symbol) && a.Ticker.Equals(a2.Ticker) && a.Synth == a2.Synth && a.Trade == a2.Trade && a.Secured == a2.Secured
}

func (a Asset) GetChain() Chain {
	if a.Synth || a.Trade || a.Secured {
		return THORChain
	}
	return a.Chain
}

// Get layer1 asset version
func (a Asset) GetLayer1Asset() Asset {
	if !a.IsSyntheticAsset() && !a.IsTradeAsset() && !a.IsSecuredAsset() {
		return a
	}
	return Asset{
		Chain:   a.Chain,
		Symbol:  a.Symbol,
		Ticker:  a.Ticker,
		Synth:   false,
		Trade:   false,
		Secured: false,
	}
}

// Get synthetic asset of asset
func (a Asset) GetSyntheticAsset() Asset {
	if a.IsSyntheticAsset() {
		return a
	}
	return Asset{
		Chain:  a.Chain,
		Symbol: a.Symbol,
		Ticker: a.Ticker,
		Synth:  true,
	}
}

// Get trade asset of asset
func (a Asset) GetTradeAsset() Asset {
	if a.IsTradeAsset() {
		return a
	}
	return Asset{
		Chain:  a.Chain,
		Symbol: a.Symbol,
		Ticker: a.Ticker,
		Trade:  true,
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

// Get derived asset of asset
func (a Asset) GetDerivedAsset() Asset {
	return Asset{
		Chain:  THORChain,
		Symbol: a.Symbol,
		Ticker: a.Ticker,
		Synth:  false,
	}
}

// Check if asset is a pegged asset
func (a Asset) IsSyntheticAsset() bool {
	return a.Synth
}

func (a Asset) IsTradeAsset() bool {
	return a.Trade
}

func (a Asset) IsSecuredAsset() bool {
	return a.Secured
}

func (a Asset) IsVaultAsset() bool {
	return a.IsSyntheticAsset()
}

// Check if asset is a derived asset
func (a Asset) IsDerivedAsset() bool {
	return !a.Synth && !a.Trade && !a.Secured && a.GetChain().IsTHORChain() && !a.IsRune() && !a.IsTCY() && !a.IsWhitelisted()
}

// Native return native asset, only relevant on THORChain
func (a Asset) Native() string {
	switch {
	case a.IsRune():
		return "rune"
	case a.Equals(TOR):
		return "tor"
	case a.Equals(TCY):
		return "tcy"
	case a.Equals(RUJI):
		return "x/ruji"
	case a.Equals(NAMI):
		return "thor.nami"
	case a.Equals(LQDY):
		return "thor.lqdy"
	case a.Equals(XUSK):
		return "thor.xusk"
	case a.Equals(AUTO):
		return "thor.auto"
	}

	return strings.ToLower(a.String())
}

// IsEmpty will be true when any of the field is empty, chain,symbol or ticker
func (a Asset) IsEmpty() bool {
	return a.Chain.IsEmpty() || a.Symbol.IsEmpty() || a.Ticker.IsEmpty()
}

// String implement fmt.Stringer , return the string representation of Asset
func (a Asset) String() string {
	div := "."
	if a.Synth {
		div = "/"
	}
	if a.Trade {
		div = "~"
	}
	if a.Secured {
		div = "-"
	}
	return fmt.Sprintf("%s%s%s", a.Chain.String(), div, a.Symbol.String())
}

// ShortCode returns the short code for the asset.
func (a Asset) ShortCode() string {
	switch a.String() {
	case "THOR.RUNE":
		return "r"
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

// IsRune is a helper function ,return true only when the asset represent RUNE
func (a Asset) IsRune() bool {
	return RuneAsset().Equals(a)
}

// IsTCY is a helper function, return true only when the asset represents TCY
func (a Asset) IsTCY() bool {
	return TCY.Equals(a)
}

// IsWhitelisted is a helper function, return true when the asset is whitelisted for a base layer pool
func (a Asset) IsWhitelisted() bool {
	whitelist := map[Asset]bool{
		RUJI: true,
		NAMI: true,
		LQDY: true,
		XUSK: true,
		AUTO: true,
	}
	return whitelist[a]
}

// IsNative is a helper function, returns true when the asset is a native
// asset to THORChain (ie rune, a synth, etc)
func (a Asset) IsNative() bool {
	return a.GetChain().IsTHORChain()
}

func (a Asset) IsExternalL1Asset() bool {
	return !a.IsSyntheticAsset() && !a.IsDerivedAsset() && !a.IsTradeAsset() && !a.IsSecuredAsset() && !a.IsNative()
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

// RuneAsset return RUNE Asset depends on different environment
func RuneAsset() Asset {
	return RuneNative
}

// Replace pool name "." with a "-" for Mimir key checking.
func (a Asset) MimirString() string {
	return a.Chain.String() + "-" + a.Symbol.String()
}
