package thornadoclient

import (
	"encoding/json"
	"fmt"

	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// GetNodeAccount retrieves node account for this address from thornado
func (b *thornadoBridge) GetNodeAccount(thorAddr string) (*types.NodeAccount, error) {
	path := fmt.Sprintf("%s/%s", NodeAccountEndpoint, thorAddr)
	body, _, err := b.getWithPath(path)
	if err != nil {
		return &types.NodeAccount{}, fmt.Errorf("failed to get node account: %w", err)
	}
	var na types.NodeAccount
	if err = json.Unmarshal(body, &na); err != nil {
		return &types.NodeAccount{}, fmt.Errorf("failed to unmarshal node account: %w", err)
	}
	return &na, nil
}

// GetNodeAccounts retrieves all node accounts from thornado
// Returns QueryNodeResponse which includes PreflightStatus for each node
func (b *thornadoBridge) GetNodeAccounts() ([]*types.QueryNodeResponse, error) {
	path := NodeAccountsEndpoint
	body, _, err := b.getWithPath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get node accounts: %w", err)
	}
	var nodes []*types.QueryNodeResponse
	if err = json.Unmarshal(body, &nodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node accounts: %w", err)
	}
	return nodes, nil
}
