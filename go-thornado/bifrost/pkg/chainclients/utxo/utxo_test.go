package utxo

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	. "gopkg.in/check.v1"
)

const (
	bob      = "bob"
	password = "password"
)

var m *metrics.Metrics

func TestPackage(t *testing.T) { TestingT(t) }

func TestUpdateCurrentBlockHeightMonotonic(t *testing.T) {
	client := &Client{}

	client.updateCurrentBlockHeight(100)
	client.updateCurrentBlockHeight(90)
	if got := client.getCurrentBlockHeight(); got != 100 {
		t.Fatalf("expected current block height to remain 100, got %d", got)
	}

	client.updateCurrentBlockHeight(110)
	if got := client.getCurrentBlockHeight(); got != 110 {
		t.Fatalf("expected current block height to advance to 110, got %d", got)
	}
}

func GetMetricForTest(c *C, chain common.Chain) *metrics.Metrics {
	if m != nil {
		return m
	}
	var err error
	m, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
		Enabled:      false,
		ListenPort:   9000,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		Chains:       common.Chains{common.BTCChain},
	})
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	return m
}

func httpTestHandler(c *C, rw http.ResponseWriter, fixture string) {
	content, err := os.ReadFile(fixture)
	if err != nil {
		c.Fatal(err)
	}
	rw.Header().Set("Content-Type", "application/json")
	if _, err = rw.Write(content); err != nil {
		c.Fatal(err)
	}
}

// assertSenderUTXOValidation verifies that Client.getSender rejects bare
// multisig sender UTXOs. This guards against the sender-spoofing attack where
// an attacker crafts a bare multisig [victim_pubkey, attacker_pubkey] and signs
// with their own key, causing getSender to attribute the tx to the victim
// (first pubkey's address). P2SH/P2WSH-wrapped multisig is not affected since
// those scriptPubKeys return a single 3xxx/bc1q address.
func assertSenderUTXOValidation(c *C, client *Client) {
	const multisigTxid = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// 1-of-1 bare multisig: OP_1 <pubkey> OP_1 OP_CHECKMULTISIG
	const bareMultisigScriptHex = "51210281feb90c058c3436f8bc361930ae99fcfb530a699cdad141d7244bfcad521a1f51ae"

	tx := btcjson.TxRawResult{
		Vin: []btcjson.Vin{{Txid: multisigTxid, Vout: 0}},
	}
	vinZeroTxs := map[string]*btcjson.TxRawResult{
		multisigTxid: {
			Vout: []btcjson.Vout{
				{
					ScriptPubKey: btcjson.ScriptPubKeyResult{
						Hex:  bareMultisigScriptHex,
						Type: "multisig",
					},
				},
			},
		},
	}

	sender, err := client.getSender(&tx, vinZeroTxs)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "sender utxo must be single-sig")
	c.Assert(sender, Equals, "")
}
