package types

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/cmd"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var setupMocknetBech32Once sync.Once

func setupMocknetBech32() {
	setupMocknetBech32Once.Do(func() {
		cfg := sdk.GetConfig()
		cfg.SetBech32PrefixForAccount(cmd.Bech32PrefixAccAddr, cmd.Bech32PrefixAccPub)
		cfg.SetBech32PrefixForValidator(cmd.Bech32PrefixValAddr, cmd.Bech32PrefixValPub)
		cfg.SetBech32PrefixForConsensusNode(cmd.Bech32PrefixConsAddr, cmd.Bech32PrefixConsPub)
	})
}

func TestTxArrayItemJSONIncludesNilSourceInputs(t *testing.T) {
	item := TxArrayItem{
		Chain:       common.BTCChain,
		ToAddress:   common.Address("bcrt1ptest"),
		VaultPubKey: common.PubKey("tthorpub1test"),
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(990000)),
	}

	bz, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	jsonText := string(bz)
	if !strings.Contains(jsonText, `"source_inputs":null`) {
		t.Fatalf("expected source_inputs:null in %s", jsonText)
	}
}

func TestKeysignJSONRoundTripMatchesSignedPayload(t *testing.T) {
	setupMocknetBech32()

	const responseJSON = `{
  "keysign": {
    "height": 337,
    "tx_array": [
      {
        "chain": "BTC",
        "to_address": "bcrt1pfrs56cns3k4nvt7wkym80kddmctklp2ajcce8vept6wyqt8p4n9syx5h94",
        "vault_pub_key": "tthorpub1addwnpepq04e5l9z7ape6yu9zcda49u5s4puwcjmfjlffd4vcy85gx7s8fphqnu8rcy",
        "coin": {
          "asset": "BTC.BTC",
          "amount": "990000"
        },
        "max_gas": null,
        "gas_rate": 14,
        "in_hash": "9FEF0CDB5AF0F2B7AE16A6F6DEA7B4ADB8649D96864E73BDA8192FC379AA7776",
        "tx_type": "out",
        "source_inputs": null
      }
    ],
    "epoch": 5,
    "status": "pending_sign",
    "signing_leader": "tthorpub1addwnpepqt7324luf7csz5zyxsh9r27p27uhe2ekxlrumcqy7xldu44mxyqhq2d2fs6"
  },
  "signature": "zh7jx27ItC6QR99IemPp9lDCBbEey1WOTRmvU7PJZvE+w7FxwnXTbcOfNQaB9ab3dT2AmYr2ujTgy5cQdgHfFw=="
}`

	var envelope struct {
		Keysign json.RawMessage `json:"keysign"`
	}
	if err := json.Unmarshal([]byte(responseJSON), &envelope); err != nil {
		t.Fatal(err)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, envelope.Keysign); err != nil {
		t.Fatal(err)
	}

	var txOut TxOut
	if err := json.Unmarshal(envelope.Keysign, &txOut); err != nil {
		t.Fatal(err)
	}

	remarshaled, err := json.Marshal(txOut)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(compact.Bytes(), remarshaled) {
		t.Fatalf("keysign JSON changed across Bifrost round trip\nsigned:     %s\nremarshaled: %s", compact.String(), string(remarshaled))
	}
}

func TestTxOutItemJSONIncludesNilSourceInputs(t *testing.T) {
	item := TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   common.Address("bcrt1ptest"),
		VaultPubKey: common.PubKey("tthorpub1test"),
		Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(990000))},
	}

	bz, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	jsonText := string(bz)
	if !strings.Contains(jsonText, `"source_inputs":null`) {
		t.Fatalf("expected source_inputs:null in %s", jsonText)
	}
}
