package thornadoclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado"
)

func TestVerifyAndDecodeKeysignUsesRawPayload(t *testing.T) {
	thornado.SetupConfigForTest()

	rawKeysign := json.RawMessage(`{
		"height": 337,
		"tx_array": [{
			"chain": "BTC",
			"to_address": "bcrt1pfrs56cns3k4nvt7wkym80kddmctklp2ajcce8vept6wyqt8p4n9syx5h94",
			"vault_pub_key": "tthorpub1addwnpepq04e5l9z7ape6yu9zcda49u5s4puwcjmfjlffd4vcy85gx7s8fphqnu8rcy",
			"coin": {"asset": "BTC.BTC", "amount": "990000"},
			"max_gas": null,
			"gas_rate": 14,
			"in_hash": "9FEF0CDB5AF0F2B7AE16A6F6DEA7B4ADB8649D96864E73BDA8192FC379AA7776",
			"tx_type": "out",
			"out_vout": 2,
			"source_inputs": [{
				"tx_id": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				"vout": 1,
				"amount_sats": 1000000
			}]
		}],
		"epoch": 5,
		"status": "pending_sign"
	}`)
	var compact bytes.Buffer
	if err := json.Compact(&compact, rawKeysign); err != nil {
		t.Fatal(err)
	}
	priv := secp256k1.GenPrivKey()
	sig, err := priv.Sign(compact.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	txOut, err := verifyAndDecodeKeysign(QueryKeysign{
		Keysign:   rawKeysign,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, priv.PubKey(), 337)
	if err != nil {
		t.Fatal(err)
	}
	if len(txOut.TxArray) != 1 || txOut.TxArray[0].OutVout != 2 {
		t.Fatalf("unexpected decoded keysign: %+v", txOut)
	}

	remarshaled, err := json.Marshal(txOut)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(compact.Bytes(), remarshaled) {
		t.Fatal("test no longer exercises raw payload verification")
	}
}

func TestQueryTxOutPreservesSigningLeader(t *testing.T) {
	leader := common.PubKey("tthorpub1addwnpepqt7324luf7csz5zyxsh9r27p27uhe2ekxlrumcqy7xldu44mxyqhq2d2fs6")
	var q queryTxOut
	if err := json.Unmarshal([]byte(`{
		"height": "18010",
		"tx_array": [],
		"epoch": "3",
		"status": "pending_sign",
		"signing_leader": "`+leader.String()+`",
		"signing_attempt": "2",
		"retry_until_height": "18040"
	}`), &q); err != nil {
		t.Fatal(err)
	}
	txOut, ok := q.txOut()
	if !ok {
		t.Fatal("failed to parse txout")
	}
	if !txOut.SigningLeader.Equals(leader) {
		t.Fatalf("signing leader was not preserved: %q", txOut.SigningLeader)
	}
	if txOut.SigningAttempt != 2 || txOut.RetryUntilHeight != 18040 {
		t.Fatalf("unexpected retry metadata: %+v", txOut)
	}
}
