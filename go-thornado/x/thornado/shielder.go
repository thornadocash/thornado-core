package thornado

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thornadocash/go-thornado/go-wrappers/shielder"
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

func ComputeShielderMerkleRoot(commitments []string) (string, error) {
	buf, err := json.Marshal(commitments)
	if err != nil {
		return "", err
	}
	return shielder.MerkleRoot(string(buf))
}

func ComputeProtocolShielderCommitment(seed string, denominationSats uint64) (string, error) {
	receiptJSON, err := shielder.DeriveSplitReceipt(seed, denominationSats, seed)
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
