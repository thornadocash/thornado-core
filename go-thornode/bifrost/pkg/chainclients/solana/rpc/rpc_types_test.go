package rpc

import (
	"encoding/json"
	"testing"
)

// TestRPCVersion_UnmarshalJSON tests the UnmarshalJSON method for RPCVersion.
// It verifies that both string and numeric JSON values are correctly unmarshaled
// into the RPCVersion.Value field as a string.
func TestRPCVersion_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "String input",
			input:    `"1.2.3"`,
			expected: "1.2.3",
			wantErr:  false,
		},
		{
			name:     "Numeric input",
			input:    `123`,
			expected: "123",
			wantErr:  false,
		},
		{
			name:     "Invalid JSON",
			input:    `invalid`,
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Empty string",
			input:    `""`,
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v RPCVersion
			err := json.Unmarshal([]byte(tt.input), &v)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && v.Value != tt.expected {
				t.Errorf("UnmarshalJSON() got = %v, want %v", v.Value, tt.expected)
			}
		})
	}
}
