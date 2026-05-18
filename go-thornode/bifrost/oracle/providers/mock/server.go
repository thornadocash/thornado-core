package mock

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gorilla/websocket"
	"gitlab.com/thorchain/thornode/v3/common"

	_ "embed"
)

//go:embed data/*
var responses embed.FS

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func NewServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Websocket
		if strings.HasSuffix(r.URL.Path, "/ws") {
			handleWs(w, r)
			return
		}

		// REST
		filename := strings.ReplaceAll(r.URL.Path[1:], "/", "-")
		filename = strings.ToLower(filename)
		filename = strings.TrimSuffix(filename, "-")

		if filename == "lbank-v2-ticker-24hr.do" {
			symbol := r.URL.Query().Get("symbol")
			filename += "-" + symbol
		}

		filename = fmt.Sprintf("data/%s.json", filename)

		data, err := responses.ReadFile(filename)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		return
	}

	provider := parts[1]
	files := []string{}

	entries, err := responses.ReadDir("data")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), provider+"-ws-") {
			continue
		}

		files = append(files, entry.Name())
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		_, _, err = conn.ReadMessage()
		if err != nil {
			return
		}

		// send some bogus msg
		_ = conn.WriteMessage(websocket.TextMessage, []byte("bogus"))

		for _, filename := range files {
			data, err := responses.ReadFile("data/" + filename)
			if err != nil {
				fmt.Println(err)
				continue
			}

			switch provider {
			case common.ProviderDigifinex:
				var buf bytes.Buffer
				writer := zlib.NewWriter(&buf)

				_, err = writer.Write(data)
				if err != nil {
					return
				}

				err = writer.Close()
				if err != nil {
					return
				}

				_ = conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
			case common.ProviderHtx:
				var buf bytes.Buffer
				writer := gzip.NewWriter(&buf)

				_, err = writer.Write(data)
				if err != nil {
					return
				}

				err = writer.Close()
				if err != nil {
					return
				}

				_ = conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
			default:
				_ = conn.WriteMessage(websocket.TextMessage, data)
			}
		}
	}
}
