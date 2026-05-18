package keysign

import (
	"encoding/base64"
	"encoding/json"
	"os"

	tsslibcommon "github.com/binance-chain/tss-lib/common"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/common"
)

type NotifierTestSuite struct{}

var _ = Suite(&NotifierTestSuite{})

func (*NotifierTestSuite) SetUpSuite(c *C) {
	conversion.SetupBech32Prefix()
}

func (NotifierTestSuite) TestNewNotifier(c *C) {
	testMSg := [][]byte{[]byte("hello"), []byte("world")}
	poolPubKey := conversion.GetRandomPubKey()
	n, err := newNotifier("", testMSg, poolPubKey, nil)
	c.Assert(err, NotNil)
	c.Assert(n, IsNil)
	n, err = newNotifier("aasfdasdf", nil, poolPubKey, nil)
	c.Assert(err, IsNil)
	c.Assert(n, NotNil)

	n, err = newNotifier("hello", testMSg, "", nil)
	c.Assert(err, IsNil)
	c.Assert(n, NotNil)

	n, err = newNotifier("hello", testMSg, poolPubKey, nil)
	c.Assert(err, IsNil)
	c.Assert(n, NotNil)
	ch := n.resp
	c.Assert(ch, NotNil)
}

func (NotifierTestSuite) TestNotifierHappyPath(c *C) {
	messageToSign := "yhEwrxWuNBGnPT/L7PNnVWg7gFWNzCYTV+GuX3tKRH8="
	buf, err := base64.StdEncoding.DecodeString(messageToSign)
	c.Assert(err, IsNil)
	messageID, err := common.MsgToHashString(buf)
	c.Assert(err, IsNil)
	poolPubKey := `thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak`
	n, err := newNotifier(messageID, [][]byte{buf}, poolPubKey, nil)
	c.Assert(err, IsNil)
	c.Assert(n, NotNil)
	sigFile := "../test_data/signature_notify/sig1.json"
	content, err := os.ReadFile(sigFile)
	c.Assert(err, IsNil)
	c.Assert(content, NotNil)
	var signature tsslibcommon.SignatureData
	err = json.Unmarshal(content, &signature)
	c.Assert(err, IsNil)

	sigInvalidFile := `../test_data/signature_notify/sig_invalid.json`
	contentInvalid, err := os.ReadFile(sigInvalidFile)
	c.Assert(err, IsNil)
	c.Assert(contentInvalid, NotNil)
	var sigInvalid tsslibcommon.SignatureData
	c.Assert(json.Unmarshal(contentInvalid, &sigInvalid), IsNil)
	// valid keysign peer , but invalid signature we should continue to listen
	err = n.processSignature([]*tsslibcommon.SignatureData{&sigInvalid})
	c.Assert(err, NotNil)
	c.Assert(n.processed, Equals, false)
	// valid signature from a keysign peer , we should accept it and bail out
	err = n.processSignature([]*tsslibcommon.SignatureData{&signature})
	c.Assert(err, IsNil)
	c.Assert(n.processed, Equals, true)

	result := <-n.resp
	c.Assert(result, NotNil)
	c.Assert(signature.String() == result[0].String(), Equals, true)
}

func (NotifierTestSuite) TestProcessSignatureEmptyData(c *C) {
	n, err := newNotifier("messageID", [][]byte{[]byte("hello")}, "poolpub", nil)
	c.Assert(err, IsNil)
	// processSignature with empty data should return nil and NOT set processed
	err = n.processSignature(nil)
	c.Assert(err, IsNil)
	c.Assert(n.processed, Equals, false)

	err = n.processSignature([]*tsslibcommon.SignatureData{})
	c.Assert(err, IsNil)
	c.Assert(n.processed, Equals, false)
}

func (NotifierTestSuite) TestProcessSignatureNilSignature(c *C) {
	messageToSign := "yhEwrxWuNBGnPT/L7PNnVWg7gFWNzCYTV+GuX3tKRH8="
	buf, err := base64.StdEncoding.DecodeString(messageToSign)
	c.Assert(err, IsNil)
	messageID, err := common.MsgToHashString(buf)
	c.Assert(err, IsNil)
	poolPubKey := `thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak`
	n, err := newNotifier(messageID, [][]byte{buf}, poolPubKey, nil)
	c.Assert(err, IsNil)

	// SignatureData with nil Signature field should return error
	sigData := &tsslibcommon.SignatureData{}
	err = n.processSignature([]*tsslibcommon.SignatureData{sigData})
	c.Assert(err, NotNil)
	c.Assert(n.processed, Equals, false)
}

func (NotifierTestSuite) TestVerifySignatureInvalidPubKey(c *C) {
	n, err := newNotifier("messageID", [][]byte{[]byte("hello")}, "invalidpubkey", nil)
	c.Assert(err, IsNil)
	sigData := &tsslibcommon.SignatureData{
		Signature: &tsslibcommon.ECSignature{
			R: []byte{1, 2, 3},
			S: []byte{4, 5, 6},
		},
	}
	err = n.verifySignature(sigData, []byte("hello"))
	c.Assert(err, NotNil)
}

func (NotifierTestSuite) TestNotifierUpdateUnset(c *C) {
	n, err := newNotifier("messageID", nil, "", nil)
	c.Assert(err, IsNil)
	c.Assert(n, NotNil)

	c.Assert(n.messages, IsNil)
	c.Assert(n.poolPubKey, Equals, "")
	c.Assert(n.signatures, IsNil)
	c.Assert(n.readyToProcess(), Equals, false)

	fakeMessages := [][]byte{[]byte("hello world")}
	n.updateUnset(fakeMessages, "", nil)

	c.Assert(n.messages, DeepEquals, fakeMessages)
	c.Assert(n.poolPubKey, Equals, "")
	c.Assert(n.signatures, IsNil)
	c.Assert(n.readyToProcess(), Equals, false)

	n.updateUnset(nil, "poolPubKey", nil)

	c.Assert(n.messages, DeepEquals, fakeMessages)
	c.Assert(n.poolPubKey, Equals, "poolPubKey")
	c.Assert(n.signatures, IsNil)
	c.Assert(n.readyToProcess(), Equals, false)

	// fakeSigs := []*tsslibcommon.SignatureData{{Signature: []byte("signature")}}
	// n.updateUnset(nil, "", fakeSigs)

	c.Assert(n.messages, DeepEquals, fakeMessages)
	c.Assert(n.poolPubKey, Equals, "poolPubKey")
	// c.Assert(n.signatures, DeepEquals, fakeSigs)
	c.Assert(n.readyToProcess(), Equals, true)

	n.updateUnset(nil, "", nil)

	c.Assert(n.messages, DeepEquals, fakeMessages)
	c.Assert(n.poolPubKey, Equals, "poolPubKey")
	// c.Assert(n.signatures, DeepEquals, fakeSigs)
	c.Assert(n.readyToProcess(), Equals, true)

	n.processed = true
	c.Assert(n.readyToProcess(), Equals, false)
}
