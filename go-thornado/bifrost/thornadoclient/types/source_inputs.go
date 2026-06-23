package types

import "github.com/thornadocash/go-thornado/common"

func SourceInputsEqual(a, b []TxOutInput) bool {
	return sourceInputsEqual(a, b)
}

func ToCommonTxInputs(inputs []TxOutInput) []common.TxInput {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]common.TxInput, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, common.TxInput{
			TxID:       input.TxID,
			Vout:       input.Vout,
			AmountSats: input.AmountSats,
		})
	}
	return out
}

func txOutInputsEqualObservedInputs(a []TxOutInput, b []common.TxInput) bool {
	if len(a) != len(b) {
		return false
	}
	matched := make([]bool, len(b))
	for _, left := range a {
		found := false
		for i, right := range b {
			if matched[i] {
				continue
			}
			if left.TxID.Equals(right.TxID) && left.Vout == right.Vout && (left.AmountSats == 0 || right.AmountSats == 0 || left.AmountSats == right.AmountSats) {
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
