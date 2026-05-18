package rpc

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type RpcTestSuite struct{}

var _ = Suite(&RpcTestSuite{})

func (s *RpcTestSuite) TestTransaction(c *C) {
	jsonTransaction := `{
		"message": {
				"accountKeys": [
						"6xQrLQAyBKZssuLt1zott5EogZmuvghf4moEYCEQ5xbm",
						"BEcrPLugbAY1zEJGUNYgzcsxMi72rPeVwG6qKm96LK5g",
						"11111111111111111111111111111111"
				],
				"header": {
						"numReadonlySignedAccounts": 0,
						"numReadonlyUnsignedAccounts": 1,
						"numRequiredSignatures": 1
				},
				"instructions": [
						{
								"accounts": [0, 1],
								"data": "3Bxs4AyoNP6YduTV",
								"programIdIndex": 2
						}
				],
				"recentBlockhash": "8NTRjgyGwuzz91akC6vRHG9ccvZnQt5eHkkJYP5tUKsC"
		},
		"signatures": [
				"54fQyXqzMdt5h4HJPo4iPoU9zD5uK7rp4NTryz4zDTFrRARGoeSbSfrhEi7ZVTZ714TZ5HX4mVRm2KY9LyzdT7p9"
		]
	}`

	// Decode JSON into Go struct
	var transaction RPCTxnData
	err := json.Unmarshal([]byte(jsonTransaction), &transaction)
	c.Check(err, IsNil)

	// Further steps to deserialize the base64 data
	decodedData, err := base64.StdEncoding.DecodeString(transaction.Message.Instructions[0].Data)
	c.Assert(err, IsNil)
	c.Assert(decodedData, NotNil)
}
