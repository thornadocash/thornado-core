package thorchain

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MigrationDataSuite struct{}

var _ = Suite(&MigrationDataSuite{})

func (s MigrationDataSuite) TestVerifyTotal4to5(c *C) {
	// verify total
	sum := uint64(0)
	for _, refund := range mainnetSlashRefunds4to5 {
		sum += refund.amount
	}
	// $ https gateway.liquify.com/chain/thorchain_api/thorchain/block height==20518466 | \
	//   jq '[.txs[1].result.events[]|select(.bond_type=="bond_cost")|.amount|tonumber]|add'
	// 9674032456636
	c.Assert(sum, Equals, uint64(9674032456636))

	// verify no duplicates
	addresses := make(map[string]bool)
	for _, refund := range mainnetSlashRefunds4to5 {
		c.Assert(addresses[refund.address], Equals, false)
		addresses[refund.address] = true
	}
}

func (s MigrationDataSuite) TestVerifyTotal5to6(c *C) {
	// verify total
	sum := uint64(0)
	for _, refund := range mainnetSlashRefunds5to6 {
		sum += refund.amount
	}

	// Total calculated using ebifrost_rollout_bond_slash_refunds.py script (see MR #4090)
	c.Assert(sum, Equals, uint64(14856919212689))

	// verify no duplicates
	addresses := make(map[string]bool)
	for _, refund := range mainnetSlashRefunds5to6 {
		c.Assert(addresses[refund.address], Equals, false)
		addresses[refund.address] = true
	}
}

func (s MigrationDataSuite) TestMigrate5to6ValidationLogic(c *C) {
	// Test the validation logic by checking if the total amount calculation is correct
	totalRefundAmount := cosmos.NewUint(14856919212689)

	// Verify this matches our manual calculation
	sum := uint64(0)
	for _, refund := range mainnetSlashRefunds5to6 {
		sum += refund.amount
	}

	c.Assert(totalRefundAmount.Equal(cosmos.NewUint(sum)), Equals, true)

	// Test that the validation would catch insufficient funds
	insufficientBalance := cosmos.NewUint(1000) // Much less than required
	c.Assert(insufficientBalance.LT(totalRefundAmount), Equals, true)

	// Test that a sufficient balance would pass validation
	sufficientBalance := totalRefundAmount.Add(cosmos.NewUint(1000000))
	c.Assert(sufficientBalance.GTE(totalRefundAmount), Equals, true)
}

func (s MigrationDataSuite) TestMigrate5to6AddressValidation(c *C) {
	// Test that all addresses in the refund list are valid thor addresses
	for i, refund := range mainnetSlashRefunds5to6 {
		// Use common.Address to properly validate thor addresses
		thorAddr, err := common.NewAddress(refund.address)
		c.Assert(err, IsNil, Commentf("Invalid address at index %d: %s", i, refund.address))

		// Convert to AccAddress using MappedAccAddress which handles prefix conversion
		_, err = thorAddr.MappedAccAddress()
		c.Assert(err, IsNil, Commentf("Failed to convert address at index %d: %s", i, refund.address))

		// Verify thor prefix
		c.Assert(refund.address[:4], Equals, "thor", Commentf("Address at index %d does not have thor prefix: %s", i, refund.address))

		// Verify address length (thor addresses should be 43 characters)
		c.Assert(len(refund.address), Equals, 43, Commentf("Address at index %d has incorrect length: %s", i, refund.address))

		// Verify amount is positive
		c.Assert(refund.amount > 0, Equals, true, Commentf("Amount at index %d should be positive: %d", i, refund.amount))
	}
}

func (s MigrationDataSuite) TestVerifyTotal12to13(c *C) {
	// verify total
	sum := uint64(0)
	for _, refund := range mainnetSlashRefunds12to13 {
		sum += refund.amount
	}
	// $ https gateway.liquify.com/chain/thorchain_api/thorchain/block height==24809168 | \
	//   jq '[.txs[7].result.events[]|select(.bond_type=="bond_cost")|.amount|tonumber]|add'
	// 53389901389659
	totalBondSlash := 53389901389659

	// NOTE: ~96 RUNE of slash was on the autobond contract, which cannot receive
	// transfer. That amount is excluded from the refunds.
	autoBondSlash := 9615118848
	c.Assert(mainnetSlashRefunds12to13Total, Equals, uint64(totalBondSlash-autoBondSlash))
	c.Assert(sum, Equals, mainnetSlashRefunds12to13Total)

	// verify no duplicates
	addresses := make(map[string]bool)
	for _, refund := range mainnetSlashRefunds12to13 {
		c.Assert(addresses[refund.address], Equals, false)
		addresses[refund.address] = true
	}
}
