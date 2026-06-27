package btc

import (
	"testing"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestConfirmationMultiplierDefaultsToFullBasisPoints(t *testing.T) {
	got, err := GetConfMulBasisPoint(common.BTCChain.String(), &mockBridge{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(cosmos.NewUint(constants.MaxBasisPts)) {
		t.Fatalf("confirmation multiplier = %s, want %d", got.String(), constants.MaxBasisPts)
	}
}

func TestGetConfirmationCountProtocolControlledTxRequiresOneConfirmation(t *testing.T) {
	pubkey := ttypes.GetRandomPubKey()
	sender, err := common.DeriveBTCTaprootAddress(pubkey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		cfg:        config.BifrostChainConfiguration{ChainID: common.BTCChain, MinConfirmations: 6},
		bridge:     &mockBridge{basePubKeys: []thornadoclient.PubKeyAddressPair{{PubKey: pubkey, Algo: common.SigningAlgoSecp256k1}}},
		vaultPaths: make(map[string]map[uint64]struct{}),
	}

	got := client.GetConfirmationCount(types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:         2,
				Tx:                  "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Sender:              sender.String(),
				To:                  "bc1q2gjc0rnhy4nrxvuklk6ptwkcs9kcr59mcl2q9j",
				Coins:               common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(12_345_600))},
				ObservedVaultPubKey: pubkey,
			},
		},
		Filtered: true,
		MemPool:  false,
	})
	if got != 1 {
		t.Fatalf("confirmation count = %d, want 1", got)
	}
}
