package thornado

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/go-wrappers/shielder"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

type ShielderRedeemPayload struct {
	Proof  json.RawMessage `json:"proof"`
	Public json.RawMessage `json:"public"`
}

func VerifyShielderRedeemJSON(proofJSON, publicJSON []byte) error {
	if !json.Valid(proofJSON) {
		return fmt.Errorf("invalid shielder proof json")
	}
	if !json.Valid(publicJSON) {
		return fmt.Errorf("invalid shielder public input json")
	}
	if err := ValidateShielderRedeemPublicJSON(publicJSON); err != nil {
		return err
	}
	return shielder.VerifyWithdrawal(string(proofJSON), string(publicJSON))
}

func VerifyShielderRedeem(withdrawal ShielderRedeemPayload) error {
	if len(withdrawal.Proof) == 0 {
		return fmt.Errorf("missing shielder proof")
	}
	if len(withdrawal.Public) == 0 {
		return fmt.Errorf("missing shielder public inputs")
	}
	return VerifyShielderRedeemJSON(withdrawal.Proof, withdrawal.Public)
}

func ValidateShielderRedeemPublicJSON(publicJSON []byte) error {
	return shielder.ValidateRedeemPublicJSON(string(publicJSON))
}

type shielderRedeemGroth16 struct {
	PiA      []string   `json:"pi_a"`
	PiB      [][]string `json:"pi_b"`
	PiC      []string   `json:"pi_c"`
	Protocol string     `json:"protocol,omitempty"`
}

type shielderRedeemTornado struct {
	Protocol string                 `json:"protocol"`
	Groth16  *shielderRedeemGroth16 `json:"groth16,omitempty"`
}

type shielderRedeemProof struct {
	Nullifier  string                 `json:"nullifier,omitempty"`
	Secret     string                 `json:"secret,omitempty"`
	Commitment string                 `json:"commitment,omitempty"`
	MerkleRoot string                 `json:"merkle_root"`
	Tornado    *shielderRedeemTornado `json:"tornado,omitempty"`
}

// RejectLeakyShielderRedeemProof enforces a strict allowlist on the redeem proof
// JSON: only the exact fields the legitimate client emits are accepted. Any
// unenumerated key (leaf index, path indices, deposit identity, or an arbitrary
// key the client might leak) is rejected up front. Private note material and the
// merkle witness are additionally rejected even though they are known keys.
func RejectLeakyShielderRedeemProof(ctx cosmos.Context, k keeper.Keeper, proofJSON []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(proofJSON)))
	decoder.DisallowUnknownFields()
	var proof shielderRedeemProof
	err := decoder.Decode(&proof)
	if err != nil {
		return fmt.Errorf("shielder proof carries unexpected fields: %w", err)
	}
	if decoder.More() {
		return fmt.Errorf("shielder proof carries trailing data")
	}
	if strings.TrimSpace(proof.Nullifier) != "" || strings.TrimSpace(proof.Secret) != "" || strings.TrimSpace(proof.Commitment) != "" {
		return fmt.Errorf("shielder proof carries private note material")
	}
	return nil
}

func ComputeShielderMerkleRoot(commitments []string) (string, error) {
	buf, err := json.Marshal(commitments)
	if err != nil {
		return "", err
	}
	rootHex, err := shielder.MerkleRoot(string(buf))
	if err != nil {
		return "", err
	}
	root := strings.TrimPrefix(strings.TrimSpace(rootHex), "0x")
	raw, err := hex.DecodeString(root)
	if err != nil {
		return "", fmt.Errorf("invalid shielder merkle root: %s", rootHex)
	}
	for left, right := 0, len(raw)-1; left < right; left, right = left+1, right-1 {
		raw[left], raw[right] = raw[right], raw[left]
	}
	value := new(big.Int).SetBytes(raw)
	return value.String(), nil
}

func ComputeProtocolShielderCommitment(seed string, denominationSats uint64) (string, error) {
	receiptJSON, err := shielder.DeriveShieldReceipt(seed, denominationSats, seed)
	if err != nil {
		return "", err
	}
	var receipt struct {
		Notes []struct {
			Commitment string `json:"commitment"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
		return "", err
	}
	if len(receipt.Notes) == 0 {
		return "", fmt.Errorf("missing protocol shielder note")
	}
	return strings.ToUpper(strings.TrimSpace(receipt.Notes[0].Commitment)), nil
}
