package keysign

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/blame"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/common"
)

func TestNewSignature(t *testing.T) {
	sig := NewSignature("msg1", "r_val", "s_val", "0")
	assert.Equal(t, "msg1", sig.Msg)
	assert.Equal(t, "r_val", sig.R)
	assert.Equal(t, "s_val", sig.S)
	assert.Equal(t, "0", sig.RecoveryID)
}

func TestNewResponse(t *testing.T) {
	sigs := []Signature{
		NewSignature("msg1", "r1", "s1", "0"),
		NewSignature("msg2", "r2", "s2", "1"),
	}
	b := blame.Blame{
		FailReason: "test reason",
	}
	resp := NewResponse(sigs, common.Success, b)
	assert.Equal(t, sigs, resp.Signatures)
	assert.Equal(t, common.Success, resp.Status)
	assert.Equal(t, b, resp.Blame)

	// Test with fail status
	resp2 := NewResponse(nil, common.Fail, blame.Blame{})
	assert.Nil(t, resp2.Signatures)
	assert.Equal(t, common.Fail, resp2.Status)
}
