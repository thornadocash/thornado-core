package evm

import (
	"testing"

	ecommon "github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	. "gopkg.in/check.v1"
)

func TestUtils(t *testing.T) { TestingT(t) }

type UtilsTestSuite struct{}

var _ = Suite(&UtilsTestSuite{})

func (s *UtilsTestSuite) TestIsSmartContractCall(c *C) {
	// nil tx data and no logs => not a smart contract call
	tx := etypes.NewTransaction(0, ecommon.Address{}, nil, 0, nil, nil)
	receipt := &etypes.Receipt{}
	c.Assert(IsSmartContractCall(tx, receipt), Equals, false)

	// tx with data but no logs => not a smart contract call
	tx = etypes.NewTransaction(0, ecommon.Address{}, nil, 0, nil, []byte{0x01, 0x02, 0x03, 0x04})
	c.Assert(IsSmartContractCall(tx, receipt), Equals, false)

	// tx with data and logs => smart contract call
	receipt = &etypes.Receipt{
		Logs: []*etypes.Log{{Address: ecommon.HexToAddress("0x1234")}},
	}
	c.Assert(IsSmartContractCall(tx, receipt), Equals, true)
}

func (s *UtilsTestSuite) TestDepositEventRouter(c *C) {
	routerAddr := ecommon.HexToAddress("0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44")
	depositTopic := ecommon.HexToHash(depositEvent)
	transferTopic := ecommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	// nil logs => nil
	c.Assert(DepositEventRouter(nil), IsNil)

	// empty logs => nil
	c.Assert(DepositEventRouter([]*etypes.Log{}), IsNil)

	// no deposit event in logs => nil
	logs := []*etypes.Log{
		{
			Address: routerAddr,
			Topics:  []ecommon.Hash{transferTopic},
		},
	}
	c.Assert(DepositEventRouter(logs), IsNil)

	// deposit event from router => returns router address
	logs = []*etypes.Log{
		{
			Address: routerAddr,
			Topics:  []ecommon.Hash{depositTopic},
		},
	}
	result := DepositEventRouter(logs)
	c.Assert(result, NotNil)
	c.Assert(*result, Equals, routerAddr)

	// deposit event among other logs (aggregator scenario) => returns correct router
	aggregatorAddr := ecommon.HexToAddress("0x0ccD5Dd5BcF1Af77dc358d1E2F06eE880EF63C3c")
	logs = []*etypes.Log{
		{
			Address: aggregatorAddr,
			Topics:  []ecommon.Hash{transferTopic},
		},
		{
			Address: ecommon.HexToAddress("0x3b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483"),
			Topics:  []ecommon.Hash{transferTopic},
		},
		{
			Address: routerAddr,
			Topics:  []ecommon.Hash{depositTopic},
		},
		{
			Address: aggregatorAddr,
			Topics:  []ecommon.Hash{ecommon.HexToHash("0x1234")},
		},
	}
	result = DepositEventRouter(logs)
	c.Assert(result, NotNil)
	c.Assert(*result, Equals, routerAddr)

	// log with empty topics => skipped
	logs = []*etypes.Log{
		{
			Address: routerAddr,
			Topics:  []ecommon.Hash{},
		},
	}
	c.Assert(DepositEventRouter(logs), IsNil)
}
