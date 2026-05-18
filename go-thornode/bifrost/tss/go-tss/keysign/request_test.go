package keysign

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/thorchain/thornode/v3/common"
)

func TestNewRequest(t *testing.T) {
	pk := "thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak"
	msgs := []string{"msg1", "msg2"}
	signers := []string{"signer1", "signer2"}
	blockHeight := int64(100)
	version := "0.14.0"

	r := NewRequest(pk, common.SigningAlgoSecp256k1, msgs, blockHeight, signers, version)
	assert.Equal(t, pk, r.PoolPubKey)
	assert.Equal(t, string(common.SigningAlgoSecp256k1), r.Algo)
	assert.Equal(t, msgs, r.Messages)
	assert.Equal(t, blockHeight, r.BlockHeight)
	assert.Equal(t, signers, r.SignerPubKeys)
	assert.Equal(t, version, r.Version)

	// Test with ed25519 algo
	r2 := NewRequest(pk, common.SigningAlgoEd25519, []string{"msg3"}, 200, []string{"s1"}, "0.15.0")
	assert.Equal(t, string(common.SigningAlgoEd25519), r2.Algo)
	assert.Equal(t, int64(200), r2.BlockHeight)
}
