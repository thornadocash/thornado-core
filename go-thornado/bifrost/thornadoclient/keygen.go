package thornadoclient

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	btypes "github.com/thornadocash/go-thornado/bifrost/blockscanner/types"
	"github.com/thornadocash/go-thornado/common"
	openapi "github.com/thornadocash/go-thornado/openapi/gen"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// GetKeygenBlock retrieves keygen request for the given block height from thornado
func (b *thornadoBridge) GetKeygenBlock(blockHeight int64, pk string) (types.KeygenBlock, error) {
	path := fmt.Sprintf("%s/%d/%s", KeygenEndpoint, blockHeight, pk)
	body, status, err := b.getWithPath(path)
	if err != nil {
		if status == http.StatusNotFound {
			return types.KeygenBlock{}, btypes.ErrUnavailableBlock
		}
		return types.KeygenBlock{}, fmt.Errorf("failed to get keygen for a block height: %w", err)
	}
	var query openapi.KeygenResponse
	if err = json.Unmarshal(body, &query); err != nil {
		return types.KeygenBlock{}, fmt.Errorf("failed to unmarshal Keygen: %w", err)
	}

	if query.Signature == "" {
		return types.KeygenBlock{}, errors.New("invalid keygen signature: empty")
	}

	buf, err := json.Marshal(query.KeygenBlock)
	if err != nil {
		return types.KeygenBlock{}, fmt.Errorf("fail to marshal keygen block to json: %w", err)
	}

	pubKey, err := b.keys.GetSignerInfo().GetPubKey()
	if err != nil {
		return types.KeygenBlock{}, fmt.Errorf("fail to get signer pub key: %w", err)
	}
	s, err := base64.StdEncoding.DecodeString(query.Signature)
	if err != nil {
		return types.KeygenBlock{}, errors.New("invalid keygen signature: cannot decode signature")
	}
	if !pubKey.VerifySignature(buf, s) {
		return types.KeygenBlock{}, errors.New("invalid keygen signature: bad signature")
	}

	keygens := make([]types.Keygen, len(query.KeygenBlock.Keygens))
	for i := range query.KeygenBlock.Keygens {
		keygens[i] = types.Keygen{
			ID:      common.TxID(*query.KeygenBlock.Keygens[i].Id),
			Type:    types.KeygenType(types.KeygenType_value[*query.KeygenBlock.Keygens[i].Type]),
			Members: query.KeygenBlock.Keygens[i].Members,
		}
	}
	keygenBlock := types.KeygenBlock{
		Height:  *query.KeygenBlock.Height,
		Keygens: keygens,
	}

	return keygenBlock, nil
}
