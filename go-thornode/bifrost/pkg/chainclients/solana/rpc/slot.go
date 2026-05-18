package rpc

import (
	"fmt"
)

// Response struct for getSignaturesForAddress
type GetSlotResponse struct {
	Jsonrpc string    `json:"jsonrpc"`
	Id      int       `json:"id"`
	Result  *uint64   `json:"result"`
	Error   *RPCError `json:"error"`
}

// GetSlot - makes a getSlot RPC call to the Solana RPC. Will only return a
// "finalized" slot.
func (s *SolRPC) GetSlot() (uint64, error) {
	params := []interface{}{
		map[string]interface{}{
			"commitment": "finalized",
		},
	}

	var response GetSlotResponse
	if err := s.post("getSlot", params, &response); err != nil {
		return 0, fmt.Errorf("failed to call getSlot: %w", err)
	}

	if response.Error != nil && response.Error.Code != 0 {
		return 0, fmt.Errorf("RPC Error %d: %s", response.Error.Code, response.Error.Message)
	}

	if response.Result == nil {
		return 0, fmt.Errorf("RPC Error: result is nil")
	}

	return *response.Result, nil
}
