package thorchain

import (
	"encoding/json"
	"fmt"

	"github.com/thornadocash/go-thornado/go-wrappers/shielder"
)

type ShielderWithdrawalPayload struct {
	Proof  json.RawMessage `json:"proof"`
	Public json.RawMessage `json:"public"`
}

func VerifyShielderWithdrawalJSON(proofJSON, publicJSON []byte) error {
	if !json.Valid(proofJSON) {
		return fmt.Errorf("invalid shielder proof json")
	}
	if !json.Valid(publicJSON) {
		return fmt.Errorf("invalid shielder public input json")
	}
	return shielder.VerifyWithdrawal(string(proofJSON), string(publicJSON))
}

func VerifyShielderWithdrawal(withdrawal ShielderWithdrawalPayload) error {
	if len(withdrawal.Proof) == 0 {
		return fmt.Errorf("missing shielder proof")
	}
	if len(withdrawal.Public) == 0 {
		return fmt.Errorf("missing shielder public inputs")
	}
	return VerifyShielderWithdrawalJSON(withdrawal.Proof, withdrawal.Public)
}
