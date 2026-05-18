package types

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// NewMsgTssPool is a constructor function for MsgTssPool
func NewMsgTssPoolV2(
	pks []string,
	poolpk common.PubKey,
	secp256k1Signature,
	keysharesBackup []byte,
	keygenType KeygenType,
	height int64,
	bl []Blame,
	chains []string,
	signer cosmos.AccAddress,
	keygenTime int64,
	poolPubKeyEddsa common.PubKey,
	keysharesBackupEddsa []byte,
) (*MsgTssPool, error) {
	id, err := getTssID(pks, poolpk, height, bl)
	if err != nil {
		return nil, fmt.Errorf("fail to get tss id: %w", err)
	}
	return &MsgTssPool{
		ID:                   id,
		PubKeys:              pks,
		PoolPubKey:           poolpk,
		PoolPubKeyEddsa:      poolPubKeyEddsa,
		Height:               height,
		KeygenType:           keygenType,
		Blame:                bl,
		Chains:               chains,
		Signer:               signer,
		KeygenTime:           keygenTime,
		KeysharesBackup:      keysharesBackup,
		KeysharesBackupEddsa: keysharesBackupEddsa,
		Secp256K1Signature:   secp256k1Signature,
	}, nil
}
