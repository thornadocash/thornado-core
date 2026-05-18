package gaia

import (
	"strings"
)

type CosmosAssetMapping struct {
	CosmosDenom     string
	CosmosDecimals  int
	THORChainSymbol string
}

func (c *CosmosBlockScanner) GetAssetByCosmosDenom(denom string) (CosmosAssetMapping, bool) {
	for _, asset := range c.cfg.WhitelistCosmosAssets {
		if strings.EqualFold(asset.Denom, denom) {
			return CosmosAssetMapping{
				CosmosDenom:     asset.Denom,
				CosmosDecimals:  asset.Decimals,
				THORChainSymbol: asset.THORChainSymbol,
			}, true
		}
	}
	return CosmosAssetMapping{}, false
}

func (c *CosmosBlockScanner) GetAssetByThorchainSymbol(symbol string) (CosmosAssetMapping, bool) {
	for _, asset := range c.cfg.WhitelistCosmosAssets {
		if strings.EqualFold(asset.THORChainSymbol, symbol) {
			return CosmosAssetMapping{
				CosmosDenom:     asset.Denom,
				CosmosDecimals:  asset.Decimals,
				THORChainSymbol: asset.THORChainSymbol,
			}, true
		}
	}
	return CosmosAssetMapping{}, false
}
