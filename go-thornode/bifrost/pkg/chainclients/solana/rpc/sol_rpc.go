package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SolRPC is a struct that interacts with the Solana blockchain
type SolRPC struct {
	host    string
	timeout time.Duration
	logger  zerolog.Logger
}

func NewSolRPC(host string, timeout time.Duration) *SolRPC {
	return &SolRPC{
		host:    host,
		timeout: timeout,
		logger:  log.Logger.With().Str("module", "sol_rpc").Logger(),
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// Private Methods
////////////////////////////////////////////////////////////////////////////////////////

func (s *SolRPC) getContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.timeout)
}

// post - Make a POST request to the Solana RPC
func (s *SolRPC) post(method string, params, response interface{}) error {
	ctx, cancel := s.getContext()
	defer cancel()

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.host, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: s.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-200 response status: %d", resp.StatusCode)
	}

	// stream the response body to json unmarshal directly into the response struct
	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil // return nil error if response is successfully decoded
}

////////////////////////////////////////////////////////////////////////////////////////
// RPC Methods
////////////////////////////////////////////////////////////////////////////////////////

func (s *SolRPC) GetLatestBlockhash() (string, error) {
	params := []interface{}{}

	var result struct {
		Result struct {
			Value struct {
				Blockhash string `json:"blockhash"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := s.post("getLatestBlockhash", params, &result); err != nil {
		return "", fmt.Errorf("failed to call getLatestBlockhash: %w", err)
	}

	return result.Result.Value.Blockhash, nil
}

func (s *SolRPC) GetBalance(address, commitment string, minSlot uint64) (*big.Int, error) {
	additionalParams := map[string]interface{}{"commitment": commitment}
	if minSlot > 0 {
		additionalParams["minContextSlot"] = minSlot
	}
	params := []interface{}{
		address,
		additionalParams,
	}

	var result struct {
		Result struct {
			Value json.Number `json:"value"` // Use json.Number to handle the numeric value without precision loss
		} `json:"result"`
	}
	if err := s.post("getBalance", params, &result); err != nil {
		return nil, fmt.Errorf("failed to call getBalance: %w", err)
	}

	// Convert the json.Number to *big.Int
	balanceBigInt := new(big.Int)
	if _, ok := balanceBigInt.SetString(string(result.Result.Value), 10); !ok {
		return nil, fmt.Errorf("failed to convert balance to big.Int: %s", result.Result.Value)
	}

	return balanceBigInt, nil
}

func (s *SolRPC) BroadcastTx(rawTx string) (string, error) {
	params := []interface{}{rawTx, map[string]interface{}{"encoding": "base64"}}

	var response struct {
		Result string `json:"result"`
		Error  struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := s.post("sendTransaction", params, &response); err != nil {
		return "", fmt.Errorf("failed to sendTransaction: %w", err)
	}

	if response.Error.Message != "" {
		return "", fmt.Errorf("sendTransaction error: %s (code: %d)", response.Error.Message, response.Error.Code)
	}

	return response.Result, nil
}
