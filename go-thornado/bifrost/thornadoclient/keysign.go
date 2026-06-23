package thornadoclient

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	btypes "github.com/thornadocash/go-thornado/bifrost/blockscanner/types"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var ErrNotFound = fmt.Errorf("not found")

type QueryKeysign struct {
	Keysign   types.TxOut `json:"keysign"`
	Signature string      `json:"signature"`
}

type queryTxOutQueue struct {
	Txouts []queryTxOut `json:"txouts"`
}

type queryTxOut struct {
	Height           string             `json:"height"`
	TxArray          []queryTxArrayItem `json:"tx_array"`
	Epoch            string             `json:"epoch,omitempty"`
	Status           string             `json:"status,omitempty"`
	SigningAttempt   string             `json:"signing_attempt,omitempty"`
	RetryUntilHeight string             `json:"retry_until_height,omitempty"`
}

type queryTxArrayItem struct {
	Chain                 common.Chain      `json:"chain,omitempty"`
	ToAddress             common.Address    `json:"to_address,omitempty"`
	VaultPubKey           common.PubKey     `json:"vault_pub_key,omitempty"`
	Coin                  queryCoin         `json:"coin"`
	MaxGas                []queryCoin       `json:"max_gas"`
	GasRate               json.RawMessage   `json:"gas_rate,omitempty"`
	InHash                common.TxID       `json:"in_hash,omitempty"`
	OutHash               common.TxID       `json:"out_hash,omitempty"`
	OutVout               json.RawMessage   `json:"out_vout,omitempty"`
	Aggregator            string            `json:"aggregator,omitempty"`
	AggregatorTargetAsset string            `json:"aggregator_target_asset,omitempty"`
	AggregatorTargetLimit *cosmos.Uint      `json:"aggregator_target_limit,omitempty"`
	VaultPubKeyEddsa      common.PubKey     `json:"vault_pub_key_eddsa,omitempty"`
	VaultPathIndex        json.RawMessage   `json:"vault_path_index,omitempty"`
	TxType                string            `json:"tx_type,omitempty"`
	SourceInputs          []queryTxOutInput `json:"source_inputs"`
}

type queryCoin struct {
	Asset    common.Asset    `json:"asset"`
	Amount   string          `json:"amount"`
	Decimals json.RawMessage `json:"decimals,omitempty"`
}

type queryTxOutInput struct {
	TxID       common.TxID     `json:"tx_id"`
	Vout       json.RawMessage `json:"vout,omitempty"`
	AmountSats json.RawMessage `json:"amount_sats,omitempty"`
}

func (q queryTxOut) txOut() (types.TxOut, bool) {
	height, err := strconv.ParseInt(q.Height, 10, 64)
	if err != nil || height <= 0 {
		return types.TxOut{}, false
	}
	epoch, _ := strconv.ParseUint(q.Epoch, 10, 64)
	signingAttempt, _ := strconv.ParseUint(q.SigningAttempt, 10, 64)
	retryUntilHeight, _ := strconv.ParseInt(q.RetryUntilHeight, 10, 64)
	return types.TxOut{
		Height:           height,
		TxArray:          q.txArray(),
		Epoch:            epoch,
		Status:           q.Status,
		SigningAttempt:   signingAttempt,
		RetryUntilHeight: retryUntilHeight,
	}, true
}

func (q queryTxOut) txArray() []types.TxArrayItem {
	items := make([]types.TxArrayItem, 0, len(q.TxArray))
	for _, item := range q.TxArray {
		items = append(items, item.txArrayItem())
	}
	return items
}

func (q queryTxArrayItem) txArrayItem() types.TxArrayItem {
	return types.TxArrayItem{
		Chain:                 q.Chain,
		ToAddress:             q.ToAddress,
		VaultPubKey:           q.VaultPubKey,
		Coin:                  q.Coin.coin(),
		MaxGas:                q.maxGas(),
		GasRate:               parseRawInt64(q.GasRate),
		InHash:                q.InHash,
		OutHash:               q.OutHash,
		OutVout:               uint32(parseRawUint64(q.OutVout)),
		Aggregator:            q.Aggregator,
		AggregatorTargetAsset: q.AggregatorTargetAsset,
		AggregatorTargetLimit: q.AggregatorTargetLimit,
		VaultPubKeyEddsa:      q.VaultPubKeyEddsa,
		VaultPathIndex:        parseRawUint64(q.VaultPathIndex),
		TxType:                q.TxType,
		SourceInputs:          q.sourceInputs(),
	}
}

func (q queryTxArrayItem) maxGas() common.Gas {
	gas := make(common.Gas, 0, len(q.MaxGas))
	for _, coin := range q.MaxGas {
		gas = append(gas, coin.coin())
	}
	return gas
}

func (q queryTxArrayItem) sourceInputs() []types.TxOutInput {
	inputs := make([]types.TxOutInput, 0, len(q.SourceInputs))
	for _, input := range q.SourceInputs {
		inputs = append(inputs, types.TxOutInput{
			TxID:       input.TxID,
			Vout:       uint32(parseRawUint64(input.Vout)),
			AmountSats: parseRawUint64(input.AmountSats),
		})
	}
	return inputs
}

func (q queryCoin) coin() common.Coin {
	amount, err := cosmos.ParseUint(q.Amount)
	if err != nil {
		amount = cosmos.ZeroUint()
	}
	return common.Coin{
		Asset:    q.Asset,
		Amount:   amount,
		Decimals: parseRawInt64(q.Decimals),
	}
}

func parseRawInt64(raw json.RawMessage) int64 {
	n, _ := strconv.ParseInt(rawString(raw), 10, 64)
	return n
}

func parseRawUint64(raw json.RawMessage) uint64 {
	n, _ := strconv.ParseUint(rawString(raw), 10, 64)
	return n
}

func rawString(raw json.RawMessage) string {
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

func hasUnsignedTxOutItem(txOut types.TxOut) bool {
	for _, item := range txOut.TxArray {
		if item.OutHash.IsEmpty() {
			return true
		}
	}
	return false
}

// GetKeysign retrieves txout from this block height from thornado
func (b *thornadoBridge) GetKeysign(blockHeight int64, pk string) (types.TxOut, error) {
	path := fmt.Sprintf("%s/%d/%s", KeysignEndpoint, blockHeight, pk)
	body, status, err := b.getWithPath(path)
	if err != nil {
		if status == http.StatusNotFound {
			return types.TxOut{}, btypes.ErrUnavailableBlock
		}
		return types.TxOut{}, fmt.Errorf("failed to get tx from a block height: %w", err)
	}
	var query QueryKeysign
	if err = json.Unmarshal(body, &query); err != nil {
		return types.TxOut{}, fmt.Errorf("failed to unmarshal TxOut: %w", err)
	}
	// there is no txout item , thus no need to validate signature either
	if len(query.Keysign.TxArray) == 0 {
		return query.Keysign, nil
	}
	if query.Signature == "" {
		return types.TxOut{}, errors.New("invalid keysign signature: empty")
	}
	buf, err := json.Marshal(query.Keysign)
	if err != nil {
		return types.TxOut{}, fmt.Errorf("fail to marshal keysign block to json: %w", err)
	}
	pubKey, err := b.keys.GetSignerInfo().GetPubKey()
	if err != nil {
		return types.TxOut{}, fmt.Errorf("fail to get signer pub key: %w", err)
	}
	s, err := base64.StdEncoding.DecodeString(query.Signature)
	if err != nil {
		return types.TxOut{}, errors.New("invalid keysign signature: cannot decode signature")
	}
	if !pubKey.VerifySignature(buf, s) {
		return types.TxOut{}, errors.New("invalid keysign signature: bad signature")
	}

	// ensure the block height received is the one requested. Without this
	// check, an attacker could use a replay attack to steal funds
	if query.Keysign.Height != blockHeight {
		return types.TxOut{}, fmt.Errorf("invalid keysign: block height mismatch (%d vs %d)", query.Keysign.Height, blockHeight)
	}

	return query.Keysign, nil
}

func (b *thornadoBridge) GetPendingTxOutKeysigns() ([]types.TxOut, error) {
	body, _, err := b.getWithPath(TxOutEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending txouts: %w", err)
	}
	var queue queryTxOutQueue
	if err = json.Unmarshal(body, &queue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pending txouts: %w", err)
	}

	seen := make(map[string]struct{})
	txOuts := make([]types.TxOut, 0, len(queue.Txouts))
	for _, txOut := range queue.Txouts {
		parsedTxOut, ok := txOut.txOut()
		if !ok {
			continue
		}
		for _, item := range parsedTxOut.TxArray {
			if item.VaultPubKey == "" {
				continue
			}
			key := fmt.Sprintf("%d/%s", parsedTxOut.Height, item.VaultPubKey)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keysign, err := b.GetKeysign(parsedTxOut.Height, item.VaultPubKey.String())
			if err != nil {
				return nil, fmt.Errorf("failed to get pending keysign %s: %w", key, err)
			}
			if len(keysign.TxArray) > 0 && hasUnsignedTxOutItem(keysign) {
				txOuts = append(txOuts, keysign)
			}
		}
	}
	return txOuts, nil
}

func (b *thornadoBridge) GetAllTxOutKeysigns() ([]types.TxOut, error) {
	body, _, err := b.getWithPath(TxOutEndpoint + "/all")
	if err != nil {
		return nil, fmt.Errorf("failed to get txout history: %w", err)
	}
	var queue queryTxOutQueue
	if err = json.Unmarshal(body, &queue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal txout history: %w", err)
	}
	txOuts := make([]types.TxOut, 0, len(queue.Txouts))
	for _, txOut := range queue.Txouts {
		parsedTxOut, ok := txOut.txOut()
		if ok {
			txOuts = append(txOuts, parsedTxOut)
		}
	}
	return txOuts, nil
}
