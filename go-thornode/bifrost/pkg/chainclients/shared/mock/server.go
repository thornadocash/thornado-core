package mock

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
)

func NewChainRpc(chain common.Chain) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data []byte

		if chain.IsUTXO() {
			w.Header().Set("Content-Type", "application/json")
			data = handleUtxo(chain, r)
		}

		_, err := w.Write(data)
		if err != nil {
			panic(err)
		}
	}))
}

func NewThornodeApi() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleThornodeApi(w, r)
	}))
}

func getJson(file string) []byte {
	file = strings.TrimPrefix(file, "/")
	file = "../../../../test/fixtures/" + file + ".json"
	content, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}

	return content
}

func writeText(text string, w http.ResponseWriter) {
	_, err := w.Write([]byte(text))
	if err != nil {
		panic(err)
	}
}

func writeJson(file string, w http.ResponseWriter) {
	_, err := w.Write(getJson(file))
	if err != nil {
		panic(err)
	}
}
