package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type TronRpc struct {
	logger  zerolog.Logger
	http    *http.Client
	url     string
	timeout time.Duration
}

func NewTronRpc(url string, timeout time.Duration) *TronRpc {
	return &TronRpc{
		logger:  log.Logger.With().Str("module", "tron_rpc").Logger(),
		url:     url,
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

// public
// ----------------------------------------------------------------------------

func (rpc *TronRpc) EthCall(
	contract, data string,
) ([]byte, error) {
	if !strings.HasPrefix(data, "0x") {
		data = "0x" + data
	}

	return rpc.post("eth_call", map[string]any{
		"from":     contract,
		"to":       contract,
		"gas":      "0x0",
		"gasPrice": "0x0",
		"value":    "0x0",
		"data":     data,
	})
}

// private
// ----------------------------------------------------------------------------

func (rpc *TronRpc) getContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), rpc.timeout)
}

func (rpc *TronRpc) post(
	method string,
	params map[string]any,
) ([]byte, error) {
	ctx, cancel := rpc.getContext()
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  []any{params, "latest"},
		"id":      1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, "POST", rpc.url, bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := rpc.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}
