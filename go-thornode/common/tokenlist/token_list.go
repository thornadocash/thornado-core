package tokenlist

import (
	"fmt"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
)

// ERC20Token is a struct to represent the token
type ERC20Token struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals int    `json:"decimals"`
}

// Asset returns the common.Asset representation of the token.
func (t ERC20Token) Asset(chain common.Chain) common.Asset {
	return common.Asset{
		Chain:  chain,
		Ticker: common.Ticker(t.Symbol),
		Symbol: common.Symbol(strings.ToUpper(fmt.Sprintf("%s-%s", t.Symbol, t.Address))),
	}
}

type EVMTokenList struct {
	Name   string       `json:"name"`
	Tokens []ERC20Token `json:"tokens"`
}

// GetEVMTokenList returns all available tokens for external asset matching for a
// particular EVM chain and version.
//
// NOTE: These tokens are NOT necessarily the same tokens that are whitelisted for each
// chain - whitelisting happens in each chain's bifrost chain client.
func GetEVMTokenList(chain common.Chain) EVMTokenList {
	switch chain {
	case common.ETHChain:
		return GetETHTokenList()
	case common.AVAXChain:
		return GetAVAXTokenList()
	case common.BSCChain:
		return GetBSCTokenList()
	case common.BASEChain:
		return GetBASETokenList()
	case common.TRONChain:
		return GetTRONTokenList()
	case common.POLChain:
		return GetPOLTokenList()

	default:
		return EVMTokenList{}
	}
}
