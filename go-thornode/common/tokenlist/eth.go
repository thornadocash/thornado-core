package tokenlist

import (
	"encoding/json"

	"gitlab.com/thorchain/thornode/v3/common/tokenlist/ethtokens"
)

var ethTokenList EVMTokenList

func init() {
	if err := json.Unmarshal(ethtokens.ETHTokenListRaw, &ethTokenList); err != nil {
		panic(err)
	}
}

func GetETHTokenList() EVMTokenList {
	return ethTokenList
}
