package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
)

func handleUtxo(chain common.Chain, r *http.Request) []byte {
	var isBatch bool

	body, _ := io.ReadAll(r.Body)
	if body[0] == '[' {
		isBatch = true
	} else {
		body = []byte("[" + string(body) + "]")
	}

	var reqs []struct {
		Method string `json:"method"`
		Params []any  `json:"params"`
		Id     int    `json:"id"`
	}

	err := json.Unmarshal(body, &reqs)
	if err != nil {
		panic(err)
	}

	results := make([]map[string]any, len(reqs))
	for i, req := range reqs {
		var file string

		switch req.Method {
		case "getblockhash":
			// Support multiple block heights - check for height-specific fixture
			var height int64
			if len(req.Params) > 0 {
				param, ok := req.Params[0].(float64)
				if !ok {
					panic("param is no float64")
				}
				height = int64(param)
				switch height {
				case 3203735:
					// pass
				default:
					height = 3203735
				}
			}
			file = fmt.Sprintf("getblockhash-%d", height)
		case "getrawtransaction":
			// Support specific transaction lookups by txid
			var short string
			if len(req.Params) > 0 {
				txid, ok := req.Params[0].(string)
				if !ok {
					panic("param is no string")
				}
				if len(txid) > 6 {
					short = txid[:6]
				}
				switch short {
				case "500468", "a7e7ee":
					// pass
				default:
					short = "500468"
				}
			}
			// Try first 6 chars of txid for fixture lookup
			file = fmt.Sprintf("getrawtransaction-%s", short)
		case "getaddressutxos":
			if len(req.Params) == 0 {
				panic("no params for getaddressutxos provided")
			}
			address, ok := req.Params[0].(string)
			if !(ok) {
				panic("param is no string")
			}
			address = address[:8]
			switch address {
			case "t1RMxNfN", "tmAbHS91":
			// pass
			default:
				return []byte(`{"id":"","result":[],"error":null}`)
			}
			file = fmt.Sprintf("getaddressutxos-%s", address)
		case "getblock":
			// Use height-specific verbose fixture for verbose=2 requests (full tx data)
			var identifier string
			verbosity := 0

			if len(req.Params) > 0 {
				var ok bool
				identifier, ok = req.Params[0].(string)
				if !ok {
					panic("param is no string")
				}
			}

			if len(req.Params) > 1 {
				param, ok := req.Params[1].(float64)
				if ok {
					verbosity = int(param)
				}
			}

			switch identifier {
			case "3203735":
				// pass
			default:
				identifier = "3203735"
			}

			file = fmt.Sprintf("getblock-%s-%d", identifier, verbosity)
		default:
			file = req.Method
		}

		file = strings.ToLower(chain.String()) + "/" + file

		data := getJson(file)
		err = json.Unmarshal(data, &results[i])
		if err != nil {
			panic("param is no string")
		}
		results[i]["id"] = req.Id
	}

	var resp []byte
	if isBatch {
		resp, err = json.Marshal(results)
	} else {
		resp, err = json.Marshal(results[0])
	}

	if err != nil {
		panic(err)
	}

	return resp
}
