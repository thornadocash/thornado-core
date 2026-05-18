package mock

import (
	"net/http"
	"strings"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func handleThornodeApi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var file string

	if strings.HasPrefix(r.RequestURI, "/thorchain/node/") {
		writeJson("nodeaccount/template", w)
		return
	}

	if strings.HasPrefix(r.RequestURI, "/auth/accounts/") {
		_, err := w.Write([]byte(`{"jsonrpc":"2.0","id":"","result":{"height":"0","result":{"value":{"account_number":"0","sequence":"0"}}}}`))
		if err != nil {
			panic(err)
		}
		return
	}

	switch r.RequestURI {
	case "/thorchain/lastblock":
		file = "lastblock/root"
	case "/thorchain/lastblock/zec":
		file = "lastblock/zec"
	case "/thorchain/mimir/key/MaxUTXOsToSpend",
		"/thorchain/mimir/key/MaxConfirmations-ZEC",
		"/thorchain/mimir/key/ConfMultiplierBasisPoints-ZEC":
		writeText("-1", w)
		return
	case "/thorchain/vaults/tthorpub1addwnpepqwznsrgk2t5vn2cszr6ku6zned6tqxknugzw3vhdcjza284d7djp5rql6vn/sign":
		writeText("[]", w)
	case "/thorchain/vaults":
		file = "tss/keysign_party"
	case "/thorchain/version":
		writeText(`{"current":"`+types.GetCurrentVersion().String()+`"}`, w)
	default:
		panic("not implemented")
	}

	writeJson(file, w)
}
