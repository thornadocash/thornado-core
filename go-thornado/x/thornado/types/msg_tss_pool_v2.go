package types

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// NewMsgTssPool is a constructor function for MsgTssPool
func NewMsgTssPoolV2(
	pks []string,
	vaultpk common.PubKey,
	secp256k1Signature,
	keysharesBackup []byte,
	keygenType KeygenType,
	height int64,
	bl []Blame,
	chains []string,
	signer cosmos.AccAddress,
	keygenTime int64,
	vaultPubKeyEddsa common.PubKey,
	keysharesBackupEddsa []byte,
	keysharesBackupFrost ...[]byte,
) (*MsgTssPool, error) {
	id, err := getTssID(pks, vaultpk, height, bl)
	if err != nil {
		return nil, fmt.Errorf("fail to get tss id: %w", err)
	}
	msg := &MsgTssPool{
		ID:                   id,
		PubKeys:              pks,
		VaultPubKey:          vaultpk,
		VaultPubKeyEddsa:     vaultPubKeyEddsa,
		Height:               height,
		KeygenType:           keygenType,
		Blame:                bl,
		Chains:               chains,
		Signer:               signer,
		KeygenTime:           keygenTime,
		KeysharesBackup:      keysharesBackup,
		KeysharesBackupEddsa: keysharesBackupEddsa,
		Secp256K1Signature:   secp256k1Signature,
	}
	if len(keysharesBackupFrost) > 0 {
		msg.KeysharesBackupFrost = keysharesBackupFrost[0]
	}
	return msg, nil
}
