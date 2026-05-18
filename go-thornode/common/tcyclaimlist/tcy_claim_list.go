package tcyclaimlist

import (
	"encoding/json"
)

type TCYClaimsJSON struct {
	Asset    string `json:"asset"`
	Address  string `json:"address"`
	TCYClaim uint64 `json:"tcy_claim"`
}

var tcyClaims []TCYClaimsJSON

func init() {
	if err := json.Unmarshal(TCYClaimsListRaw, &tcyClaims); err != nil {
		panic(err)
	}
}

func GetTCYClaimsList() []TCYClaimsJSON {
	return tcyClaims
}
