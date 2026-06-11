package thornado

import (
	"encoding/json"
	"fmt"
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

func RejectLeakyShielderRedeemProof(ctx cosmos.Context, k keeper.Keeper, proofJSON []byte) error {
	var proof struct {
		Nullifier  string `json:"nullifier"`
		Secret     string `json:"secret"`
		Commitment string `json:"commitment"`
		Tornado    *struct {
			MerklePath *struct {
				PathElements []string `json:"path_elements"`
			} `json:"merkle_path"`
		} `json:"tornado"`
	}
	if err := json.Unmarshal(proofJSON, &proof); err != nil {
		return fmt.Errorf("invalid shielder proof json: %w", err)
	}
	if strings.TrimSpace(proof.Nullifier) != "" || strings.TrimSpace(proof.Secret) != "" || strings.TrimSpace(proof.Commitment) != "" {
		return fmt.Errorf("shielder proof carries private note material")
	}
	if proof.Tornado != nil {
		if proof.Tornado.MerklePath != nil && len(proof.Tornado.MerklePath.PathElements) > 0 {
			return fmt.Errorf("shielder proof carries merkle path")
		}
	}
	return nil
}

func ComputeShielderMerkleRoot(commitments []string) (string, error) {
	buf, err := json.Marshal(commitments)
	if err != nil {
		return "", err
	}
	return shielder.MerkleRoot(string(buf))
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
