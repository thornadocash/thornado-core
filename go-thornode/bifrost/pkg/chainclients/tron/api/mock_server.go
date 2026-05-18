package api

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	_ "embed"
)

//go:embed test-tron/*
var responses embed.FS

func NewMockServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		err := json.NewDecoder(r.Body).Decode(&params)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		path := strings.Replace(r.URL.Path, "/wallet/", "", 1)
		switch path {
		case "getnowblock":
			w.WriteHeader(http.StatusOK)
			data, _ := responses.ReadFile("test-tron/getblockbynum_55088560.json")
			_, _ = w.Write(data)
		case "getblockbynum":
			var height int64

			_, found := params["num"]
			if !found {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(nil)
				return
			}

			switch v := params["num"].(type) {
			case float64:
				height = int64(v)
			case int64:
				height = v
			}

			switch height {
			case 55088560, 69351239:
				w.WriteHeader(http.StatusOK)
				filename := fmt.Sprintf("test-tron/getblockbynum_%d.json", height)
				data, _ := responses.ReadFile(filename)
				_, _ = w.Write(data)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(nil)

		case "gettransactioninfobyid":
			value, found := params["value"]

			var short string
			if found {
				short = fmt.Sprintf("%s", value)[:8]
			}

			switch short {
			case "ec7c9584", "e1cd4454":
				filename := fmt.Sprintf("test-tron/gettransactioninfobyid_%s.json", short)
				data, _ := responses.ReadFile(filename)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(nil)

		case "getaccount":
			address, found := params["address"]

			if found && fmt.Sprintf("%s", address) == "TU6nEM4GTca2L5AuDTnY1qp1rkQ2t8NxvM" {
				w.WriteHeader(http.StatusOK)
				data, _ := responses.ReadFile("test-tron/getaccount.json")
				_, _ = w.Write(data)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(nil)

		case "createtransaction", "estimateenergy", "getchainparameters",
			"triggersmartcontract", "broadcasttransaction":
			w.WriteHeader(http.StatusOK)
			data, _ := responses.ReadFile(fmt.Sprintf("test-tron/%s.json", path))

			_, _ = w.Write(data)

		default:
			fmt.Println(r.URL.Path)
			http.NotFound(w, r)
		}
	}))
}
