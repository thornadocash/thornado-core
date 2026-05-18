package types

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MsgNetworkFeeSuite struct{}

var _ = Suite(&MsgNetworkFeeSuite{})

func (MsgNetworkFeeSuite) TestMsgNetworkFee(c *C) {
	msg := NewMsgNetworkFee(1024, common.ETHChain, 1, 37500, GetRandomBech32Addr())
	EnsureMsgBasicCorrect(msg, c)

	testCases := []struct {
		blockHeight        int64
		name               string
		chain              common.Chain
		transactionSize    uint64
		transactionFeeRate uint64
		signer             cosmos.AccAddress
		expectErr          bool
	}{
		{
			name:               "empty chain should return error",
			blockHeight:        1024,
			chain:              common.EmptyChain,
			transactionSize:    100,
			transactionFeeRate: 100,
			signer:             GetRandomBech32Addr(),
			expectErr:          true,
		},
		{
			name:               "invalid transaction size should return error",
			blockHeight:        1024,
			chain:              common.ETHChain,
			transactionSize:    0,
			transactionFeeRate: 100,
			signer:             GetRandomBech32Addr(),
			expectErr:          true,
		},
		{
			name:               "invalid transaction fee rate should return error",
			blockHeight:        1024,
			chain:              common.ETHChain,
			transactionSize:    100,
			transactionFeeRate: 0,
			signer:             GetRandomBech32Addr(),
			expectErr:          true,
		},
		{
			name:               "empty signer should return error",
			blockHeight:        1024,
			chain:              common.ETHChain,
			transactionSize:    100,
			transactionFeeRate: 100,
			signer:             cosmos.AccAddress(""),
			expectErr:          true,
		},
		{
			name:               "negative block height should return error",
			blockHeight:        -1024,
			chain:              common.ETHChain,
			transactionSize:    100,
			transactionFeeRate: 100,
			signer:             GetRandomBech32Addr(),
			expectErr:          true,
		},
		{
			name:               "happy path",
			blockHeight:        1024,
			chain:              common.ETHChain,
			transactionSize:    100,
			transactionFeeRate: 100,
			signer:             GetRandomBech32Addr(),
			expectErr:          false,
		},
	}
	for _, tc := range testCases {
		msg = NewMsgNetworkFee(tc.blockHeight, tc.chain, tc.transactionSize, tc.transactionFeeRate, tc.signer)

		err := msg.ValidateBasic()
		if tc.expectErr {
			c.Assert(err, NotNil, Commentf("name:%s", tc.name))
		} else {
			EnsureMsgBasicCorrect(msg, c)
		}

	}
}
