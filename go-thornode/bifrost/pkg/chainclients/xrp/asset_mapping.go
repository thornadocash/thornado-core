package xrp

import (
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"

	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"
)

type XrpAssetMapping struct {
	XrpKind        txtypes.CurrencyKind
	XrpCurrency    string
	XrpIssuer      string
	XrpDecimals    int64
	THORChainAsset common.Asset
}

// XrpAssetMappings maps an xrp denom to a THORChain symbol and provides the asset decimals
// CHANGEME: define assets that should be observed by THORChain here. This also acts a whitelist.
var XrpAssetMappings = []XrpAssetMapping{
	{
		XrpKind:        txtypes.XRP,
		XrpCurrency:    "",
		XrpIssuer:      "",
		XrpDecimals:    6,
		THORChainAsset: common.XRPAsset,
	},
}

func GetAssetByXrpCurrency(coin txtypes.CurrencyAmount) (XrpAssetMapping, bool) {
	for _, assetEntry := range XrpAssetMappings {
		if assetEntry.XrpKind == coin.Kind() {
			if assetEntry.XrpKind == txtypes.XRP {
				return assetEntry, true
			}
			if issuerKind, ok := coin.(txtypes.IssuedCurrencyAmount); ok &&
				strings.EqualFold(issuerKind.Issuer.String(), assetEntry.XrpIssuer) &&
				strings.EqualFold(issuerKind.Currency, assetEntry.XrpCurrency) {
				return assetEntry, true
			}
		}
	}
	return XrpAssetMapping{}, false
}

func GetAssetByThorchainAsset(asset common.Asset) (XrpAssetMapping, bool) {
	for _, assetEntry := range XrpAssetMappings {
		if asset.Equals(assetEntry.THORChainAsset) {
			return assetEntry, true
		}
	}
	return XrpAssetMapping{}, false
}
