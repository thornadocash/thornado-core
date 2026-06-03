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

func RejectLeakyShielderRedeemProof(ctx cosmos.Context, k keeper.Keeper, proofJSON []byte) error {
	var proof struct {
		Nullifier  string `json:"nullifier"`
		Secret     string `json:"secret"`
		Commitment string `json:"commitment"`
		Orchard    *struct {
			Actions []struct {
				CmxHex string `json:"cmx_hex"`
			} `json:"actions"`
		} `json:"orchard"`
	}
	if err := json.Unmarshal(proofJSON, &proof); err != nil {
		return fmt.Errorf("invalid shielder proof json: %w", err)
	}
	if strings.TrimSpace(proof.Nullifier) != "" || strings.TrimSpace(proof.Secret) != "" || strings.TrimSpace(proof.Commitment) != "" {
		return fmt.Errorf("shielder proof carries private note material")
	}
	if proof.Orchard == nil {
		return nil
	}
	for _, action := range proof.Orchard.Actions {
		cmx := strings.TrimSpace(action.CmxHex)
		if cmx == "" {
			continue
		}
		for _, candidate := range []string{cmx, strings.ToLower(cmx), strings.ToUpper(cmx)} {
			if k.ShielderCommitmentExists(ctx, candidate) {
				return fmt.Errorf("shielder proof carries spent commitment")
			}
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
