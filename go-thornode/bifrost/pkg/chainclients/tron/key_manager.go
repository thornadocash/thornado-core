package tron

import (
	"crypto/ecdsa"
	"fmt"

	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/common"

	"github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/ethereum/go-ethereum/crypto"
)

type KeyManager struct {
	priv *ecdsa.PrivateKey
	pub  common.PubKey
}

func NewLocalKeyManager(
	keys *thorclient.Keys,
) (*KeyManager, error) {
	priv, err := keys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get private key: %w", err)
	}

	temp, err := codec.ToCmtPubKeyInterface(priv.PubKey())
	if err != nil {
		return nil, fmt.Errorf("fail to get tm pub key: %w", err)
	}

	pub, err := common.NewPubKeyFromCrypto(temp)
	if err != nil {
		return nil, fmt.Errorf("fail to get pub key: %w", err)
	}

	evmPrivateKey, err := evm.GetPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	return &KeyManager{
		priv: evmPrivateKey,
		pub:  pub,
	}, nil
}

func (m *KeyManager) Pubkey() common.PubKey {
	return m.pub
}

func (m *KeyManager) Sign(msg []byte) ([]byte, error) {
	return crypto.Sign(msg, m.priv)
}
