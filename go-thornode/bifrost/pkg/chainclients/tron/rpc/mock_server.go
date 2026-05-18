package rpc

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"

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

		w.WriteHeader(http.StatusOK)
		data, _ := responses.ReadFile("test-tron/eth_call.json")
		_, _ = w.Write(data)
	}))
}
