package thorchain

import (
	"fmt"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

type HandlerNetworkFeeHelpersSuite struct{}

var _ = Suite(&HandlerNetworkFeeHelpersSuite{})

func (s *HandlerNetworkFeeHelpersSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// setupNetworkFeeHelpersTest creates common test fixtures for network fee helper tests.
func setupNetworkFeeHelpersTest(c *C) (cosmos.Context, *Mgrs, NodeAccounts) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(1024)

	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}
	for _, na := range nas {
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)
	}

	return ctx, mgr, nas
}

func (s *HandlerNetworkFeeHelpersSuite) TestDuplicateSignerWithSlash(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// First sign succeeds
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, true)
	c.Assert(err, IsNil)

	ptsBefore, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[0].NodeAddress)
	c.Assert(err, IsNil)

	// Duplicate sign with shouldSlashForDuplicate=true should slash
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, true)
	c.Assert(err, IsNil)

	ptsAfter, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[0].NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(ptsAfter > ptsBefore, Equals, true, Commentf("expected extra slash for duplicate"))
}

func (s *HandlerNetworkFeeHelpersSuite) TestDuplicateSignerWithoutSlash(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// First sign succeeds
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	ptsBefore, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[0].NodeAddress)
	c.Assert(err, IsNil)

	// Duplicate sign with shouldSlashForDuplicate=false should NOT add extra slash
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	ptsAfter, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[0].NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(ptsAfter, Equals, ptsBefore, Commentf("no extra slash for duplicate when shouldSlashForDuplicate=false"))
}

func (s *HandlerNetworkFeeHelpersSuite) TestBeforeConsensusSlash(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// Sign with just one node - not enough for consensus (need 3 of 4)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	// Should have inc'd slash points (before consensus)
	pts, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[0].NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(pts > 0, Equals, true, Commentf("should have slash points before consensus"))

	// Voter should not have BlockHeight set yet
	c.Assert(voter.BlockHeight, Equals, int64(0))
}

func (s *HandlerNetworkFeeHelpersSuite) TestReachConsensus(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// Sign with 3 of 4 nodes to reach consensus
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[1].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	// Third sign should trigger consensus
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	// BlockHeight should be set to current block height
	c.Assert(voter.BlockHeight, Equals, ctx.BlockHeight())

	// Network fee should be saved
	savedFee, err := mgr.Keeper().GetNetworkFee(ctx, common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(savedFee.TransactionFeeRate, Equals, uint64(256))
	c.Assert(savedFee.TransactionSize, Equals, uint64(100))
}

func (s *HandlerNetworkFeeHelpersSuite) TestAfterConsensusWithinFlex(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// Bring voter to consensus
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[1].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	c.Assert(voter.BlockHeight > 0, Equals, true)

	// Late observer within flex period should get slash points decremented
	ptsBefore, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[3].NodeAddress)
	c.Assert(err, IsNil)

	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[3].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	ptsAfter, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[3].NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(ptsAfter < ptsBefore, Equals, true, Commentf("late observer within flex should get slash decremented"))
}

func (s *HandlerNetworkFeeHelpersSuite) TestAfterConsensusBeyondFlex(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// Bring voter to consensus
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[1].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	c.Assert(voter.BlockHeight > 0, Equals, true)

	// Move block height far beyond flex period
	ctx = ctx.WithBlockHeight(voter.BlockHeight + 10000)

	ptsBefore, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[3].NodeAddress)
	c.Assert(err, IsNil)

	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[3].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	ptsAfter, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[3].NodeAddress)
	c.Assert(err, IsNil)
	// Beyond flex period: no dec, so slash points should not decrease
	c.Assert(ptsAfter >= ptsBefore, Equals, true, Commentf("beyond flex period, slash pts should not decrease"))
}

func (s *HandlerNetworkFeeHelpersSuite) TestSaveNetworkFeeFail(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	failKeeper := &TestNetworkFeeHelpersKeeper{
		Keeper:             mgr.Keeper(),
		failSaveNetworkFee: true,
	}
	mgr.K = failKeeper

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := failKeeper.Keeper.GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// Need to use the real keeper to create active validators
	for _, na := range nas {
		c.Assert(failKeeper.Keeper.SetNodeAccount(ctx, na), IsNil)
	}

	// Sign with 3 of 4 to reach consensus
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[1].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	// Third sign triggers consensus and SaveNetworkFee call → should fail
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, nf, false)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "(?s).*fail to save network fee.*")
}

func (s *HandlerNetworkFeeHelpersSuite) TestNonSignersGetSlashed(c *C) {
	ctx, mgr, nas := setupNetworkFeeHelpersTest(c)

	nf := &common.NetworkFee{
		Chain:           common.ETHChain,
		Height:          500,
		TransactionRate: 256,
		TransactionSize: 100,
	}

	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)

	// nas[3] does NOT sign — they should get LackOfObservationPenalty after consensus
	ptsBefore, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[3].NodeAddress)
	c.Assert(err, IsNil)

	// Sign with 3 of 4 to reach consensus (nas[0], nas[1], nas[2])
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[1].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)
	err = processNetworkFeeAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, nf, false)
	c.Assert(err, IsNil)

	ptsAfter, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[3].NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(ptsAfter > ptsBefore, Equals, true, Commentf("non-signer should get slashed at consensus"))
}

// --- Test keeper for failure injection ---

type TestNetworkFeeHelpersKeeper struct {
	keeper.Keeper
	failSaveNetworkFee bool
}

func (k *TestNetworkFeeHelpersKeeper) SaveNetworkFee(ctx cosmos.Context, chain common.Chain, networkFee NetworkFee) error {
	if k.failSaveNetworkFee {
		return fmt.Errorf("kaboom")
	}
	return k.Keeper.SaveNetworkFee(ctx, chain, networkFee)
}
