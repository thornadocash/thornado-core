package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	. "gopkg.in/check.v1"

	_ "embed"
)

type ApiTestSuite struct {
	server *httptest.Server
	api    *TronApi
}

var _ = Suite(&ApiTestSuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *ApiTestSuite) SetUpSuite(c *C) {
	s.server = NewMockServer()
	c.Assert(s.server, NotNil)
	c.Assert(s.server.URL, Not(Equals), "")

	s.api = NewTronApi(s.server.URL, time.Second)

	c.Assert(s.api, NotNil)

	fmt.Println(s.server.URL)
}

func (s *ApiTestSuite) TestGetLatestBlock(c *C) {
	block, err := s.api.GetLatestBlock()
	c.Assert(err, IsNil)
	c.Assert(block.Header.RawData.Number, Equals, int64(55088560))
	c.Assert(len(block.Transactions), Equals, 3)
}

func (s *ApiTestSuite) TestGetBlock(c *C) {
	block, err := s.api.GetBlock(55088560)
	c.Assert(err, IsNil)
	c.Assert(block.Header.RawData.Number, Equals, int64(55088560))
	c.Assert(len(block.Transactions), Equals, 3)

	// not found
	block, err = s.api.GetBlock(55088561)
	c.Assert(err, NotNil)
}

func (s *ApiTestSuite) TestGetTransactionInfo(c *C) {
	info, err := s.api.GetTransactionInfo("ec7c95841b6a3fc4ce0adfaa3fd77dd74dbe004895ad0fd02bf63eb3ca5865d5")
	c.Assert(err, IsNil)
	c.Assert(info.Fee, Equals, uint64(27704850))

	// not found
	info, err = s.api.GetTransactionInfo("d522be861c068f764c2b068be505ab6dcb52ee9435085d6c6e3255ea5b12ddb6")
	c.Assert(err, NotNil)
}

func (s *ApiTestSuite) TestGetBalance(c *C) {
	balance, err := s.api.GetBalance("TU6nEM4GTca2L5AuDTnY1qp1rkQ2t8NxvM")
	c.Assert(err, IsNil)
	c.Assert(balance, Equals, uint64(2123313700))

	// not found
	balance, err = s.api.GetBalance("TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow")
	c.Assert(err, NotNil)
	c.Assert(balance, Equals, uint64(0))
}

func (s *ApiTestSuite) TestCreateTransaction(c *C) {
	// mocked response anyway...
	tx, err := s.api.CreateTransaction("from", "to", 4, "memo")
	c.Assert(err, IsNil)
	c.Assert(len(tx.RawData.Contract), Equals, 1)
}

func (s *ApiTestSuite) TestTriggerSmartContract(c *C) {
	tx, err := s.api.TriggerSmartContract("from", "to", "selector", "input", 123)
	c.Assert(err, IsNil)
	c.Assert(len(tx.RawData.Contract), Equals, 1)
}

func (s *ApiTestSuite) TestRehashAndBroadcastTransaction(c *C) {
	data := []byte(`
	{
	  "raw_data": {
		"contract": [
		  {
			"parameter": {
			  "type_url": "type.googleapis.com/protocol.TransferContract",
			  "value": {
				"amount": 500000000,
				"owner_address": "4102363c37fdfdbdd839b5d3df2848f176d2ae6d89",
				"to_address": "412867f90e345015cd2234f562c5767296b717157b"
			  }
			},
			"type": "TransferContract"
		  }
		]
	  },
	  "raw_data_hex": "0a0237462208a3d2c3a57b2a3a7040d8afb997cd3252312b3a74723a7474686f72316863763565636e636639676c36723267787438647265676d666d7876383063343076337772735a69080112650a2d747970652e676f6f676c65617069732e636f6d2f70726f746f636f6c2e5472616e73666572436f6e747261637412340a154102363c37fdfdbdd839b5d3df2848f176d2ae6d891215412867f90e345015cd2234f562c5767296b717157b1880cab5ee0170f8dab597cd32",
	  "ret": [
		{
		  "contractRet": "SUCCESS"
		}
	  ],
	  "txID": "ee746482c2598904b0a65e6df52a887ec7ec5565d14cb13f19b4509145b20c99"
	}`)

	var tx Transaction

	err := json.Unmarshal(data, &tx)
	c.Assert(err, IsNil)
	c.Assert(len(tx.Signature), Equals, 0)

	tx.RawData.RefBlockBytes = "3746"
	tx.RawData.RefBlockHash = "a3d2c3a57b2a3a70"
	tx.RawData.Timestamp = 1738705563000
	tx.RawData.Expiration = 1738705623000
	tx.RawData.Data = "2b3a74723a7474686f72316863763565636e636639676c36723267787438647265676d666d787638306334307633777273"

	err = tx.Rehash()
	c.Assert(err, IsNil)
	c.Assert(tx.TxId, Equals, "e1cd4454d71d8973e89155bc2c2a91aa56fd470c8ce64547ce8c69c789c21f0c")

	txBytes, err := json.Marshal(tx)
	c.Assert(err, IsNil)
	c.Assert(txBytes, NotNil)

	response, err := s.api.BroadcastTransaction(txBytes)
	c.Assert(err, IsNil)
	c.Assert(response.TxId, Equals, "77ddfa7093cc5f745c0d3a54abb89ef070f983343c05e0f89e5a52f3e5401299")
}

func (s *ApiTestSuite) TestGetChainParameters(c *C) {
	params, err := s.api.GetChainParameters()
	c.Assert(err, IsNil)
	c.Assert(params.EnergyFee, Equals, int64(210))
}

func (s *ApiTestSuite) TestEstimateEnergy(c *C) {
	energy, err := s.api.EstimateEnergy("from", "contract", "selector", "input")
	c.Assert(err, IsNil)
	c.Assert(energy, Equals, int64(900000))
}

func (s *ApiTestSuite) TestConvertAddress(c *C) {
	testCases := []struct {
		In  string
		Out string
	}{
		{
			In:  "TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow",
			Out: "41e2f72d7827bd1efbb6d55d6ffb1db94aab37b587",
		},
		{
			In:  "41e2f72d7827bd1efbb6d55d6ffb1db94aab37b587",
			Out: "TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow",
		},
		{
			In:  "0xe2f72d7827bd1efbb6d55d6ffb1db94aab37b587",
			Out: "TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow",
		},
		{
			In:  "bc1qelysxslyl86yq57yx9lzhl50pfayj9wng54p3g",
			Out: "",
		},
		{
			In:  "",
			Out: "",
		},
	}

	for _, tc := range testCases {
		address, err := ConvertAddress(tc.In)
		if tc.Out == "" {
			c.Assert(err, NotNil)
			c.Assert(address, Equals, tc.Out)
			c.Assert(err, ErrorMatches, "address not valid")
			continue
		}
		c.Assert(err, IsNil)
		c.Assert(address, Equals, tc.Out)
	}
}
