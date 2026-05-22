package cmd

import (
	"bufio"
	"fmt"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	cosmoscryptoed25519 "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	bech32 "github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint SA1019 deprecated
	"github.com/spf13/cobra"

	"github.com/thornadocash/go-thornado/app"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/common/crypto/ed25519"
)

const (
	DefaultEd25519KeyName = `ed-thornado`
)

func GetEd25519Keys() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ed25519",
		Short: "Generate an ed25519 key",
		Long:  ``,
		Args:  cobra.ExactArgs(0),
		RunE:  ed25519Keys,
	}
	return cmd
}

func ed25519Keys(cmd *cobra.Command, args []string) error {
	kb, err := cosmos.GetKeybase(app.DefaultNodeHome)
	if err != nil {
		return fmt.Errorf("fail to get keybase: %w", err)
	}

	edKey := ed25519.SignerNameEDDSA(kb.SignerName)
	r, err := kb.Keybase.Key(edKey)
	if err != nil {
		buf := bufio.NewReader(cmd.InOrStdin())
		var mnemonic string
		mnemonic, err = input.GetString("Enter mnemonic", buf)
		if err != nil {
			return fmt.Errorf("fail to get mnemonic: %w", err)
		}

		r, err = kb.Keybase.NewAccount(edKey, mnemonic, kb.SignerPasswd, ed25519.HDPath, ed25519.Ed25519)
		if err != nil {
			return fmt.Errorf("fail to create new key: %w", err)
		}
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	pubKey := new(cosmoscryptoed25519.PubKey)
	if err = cdc.UnpackAny(r.PubKey, &pubKey); err != nil {
		return fmt.Errorf("fail to unpack pubkey: %w", err)
	}

	pkey := cmted25519.PubKey(pubKey.Bytes())
	tmp, err := cryptocodec.FromCmtPubKeyInterface(pkey)
	if err != nil {
		return fmt.Errorf("fail to get ED25519 key : %w", err)
	}
	// nolint
	pubBech32, err := bech32.MarshalPubKey(bech32.AccPK, tmp)
	if err != nil {
		return fmt.Errorf("fail generate bech32 account pub key")
	}
	fmt.Println(pubBech32)
	return nil
}
