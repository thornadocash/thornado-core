package tokenlist

import (
	"encoding/json"

	"gitlab.com/thorchain/thornode/v3/common/tokenlist/poltokens"
)

var polTokenList EVMTokenList

func init() {
	if err := json.Unmarshal(poltokens.POLTokenListRaw, &polTokenList); err != nil {
		panic(err)
	}
}

func GetPOLTokenList() EVMTokenList {
	return polTokenList
}
