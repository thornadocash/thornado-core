package tokenlist

import (
	"encoding/json"

	"gitlab.com/thorchain/thornode/v3/common/tokenlist/trontokens"
)

var tronTokenList EVMTokenList

func init() {
	err := json.Unmarshal(trontokens.TRONTokenListRaw, &tronTokenList)
	if err != nil {
		panic(err)
	}
}

func GetTRONTokenList() EVMTokenList {
	return tronTokenList
}
