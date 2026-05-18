package thorchain

import (
	"math"

	"github.com/cometbft/cometbft/crypto/secp256k1"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerNetworkFeeQuorumSuite struct{}

var _ = Suite(&HandlerNetworkFeeQuorumSuite{})

func (s *HandlerNetworkFeeQuorumSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// makeNetworkFee creates a NetworkFee with given parameters.
func makeNetworkFee(chain common.Chain, height int64, rate, size uint64) *common.NetworkFee {
	return &common.NetworkFee{
		Chain:           chain,
		Height:          height,
		TransactionRate: rate,
		TransactionSize: size,
	}
}

// attestNetworkFee creates an attestation for a NetworkFee using the given private key.
func attestNetworkFee(c *C, privKey secp256k1.PrivKey, nf *common.NetworkFee) *common.Attestation {
	data, err := nf.GetSignablePayload()
	c.Assert(err, IsNil)

	signature, err := privKey.Sign(data)
	c.Assert(err, IsNil)

	return &common.Attestation{
		PubKey:    privKey.PubKey().Bytes(),
		Signature: signature,
	}
}

// --- Run / validate tests ---

func (s *HandlerNetworkFeeQuorumSuite) TestRunInvalidMessageType(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	// Pass a wrong message type
	msg := NewMsgNetworkFee(1024, common.ETHChain, 1, 10000, GetRandomBech32Addr())
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateNilQuoNetFee(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	msg := &types.MsgNetworkFeeQuorum{
		QuoNetFee: nil,
		Signer:    GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateNilNetworkFee(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	msg := &types.MsgNetworkFeeQuorum{
		QuoNetFee: &common.QuorumNetworkFee{
			NetworkFee: nil,
		},
		Signer: GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateEmptySigner(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	msg := &types.MsgNetworkFeeQuorum{
		QuoNetFee: nil,
		Signer:    cosmos.AccAddress{},
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateNegativeHeight(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.ETHChain, -1, 256, 100)
	msg := types.NewMsgNetworkFeeQuorum(
		&common.QuorumNetworkFee{NetworkFee: nf},
		GetRandomBech32Addr(),
	)
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateEmptyChain(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.EmptyChain, 1024, 256, 100)
	msg := types.NewMsgNetworkFeeQuorum(
		&common.QuorumNetworkFee{NetworkFee: nf},
		GetRandomBech32Addr(),
	)
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateZeroTransactionSize(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 0)
	msg := types.NewMsgNetworkFeeQuorum(
		&common.QuorumNetworkFee{NetworkFee: nf},
		GetRandomBech32Addr(),
	)
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateZeroTransactionRate(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.ETHChain, 1024, 0, 100)
	msg := types.NewMsgNetworkFeeQuorum(
		&common.QuorumNetworkFee{NetworkFee: nf},
		GetRandomBech32Addr(),
	)
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestValidateValidMsg(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)
	msg := types.NewMsgNetworkFeeQuorum(
		&common.QuorumNetworkFee{NetworkFee: nf},
		GetRandomBech32Addr(),
	)
	err := handler.validate(ctx, *msg)
	c.Assert(err, IsNil)
}

// --- handle tests ---

func (s *HandlerNetworkFeeQuorumSuite) TestHandleFailListActiveValidators(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestNetworkFeeQuorumKeeper{
		Keeper:                    mgr.Keeper(),
		failListActiveNodeAccount: true,
	}
	mgr.K = failKeeper
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)
	quoNetFee := &common.QuorumNetworkFee{NetworkFee: nf}
	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, GetRandomBech32Addr())

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleNilQuoNetFee(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	msg := &types.MsgNetworkFeeQuorum{
		QuoNetFee: nil,
		Signer:    na.NodeAddress,
	}
	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleNilNetworkFeeInQuoNetFee(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	msg := &types.MsgNetworkFeeQuorum{
		QuoNetFee: &common.QuorumNetworkFee{NetworkFee: nil},
		Signer:    na.NodeAddress,
	}
	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleTransactionRateExceedsInt64Max(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	nf := makeNetworkFee(common.ETHChain, 1024, math.MaxUint64, 100)
	quoNetFee := &common.QuorumNetworkFee{NetworkFee: nf}
	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "transaction rate or size exceeds int64 max")
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleTransactionSizeExceedsInt64Max(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, math.MaxUint64)
	quoNetFee := &common.QuorumNetworkFee{NetworkFee: nf}
	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "transaction rate or size exceeds int64 max")
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleFailGetObservedNetworkFeeVoter(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestNetworkFeeQuorumKeeper{
		Keeper:                         mgr.Keeper(),
		failGetObservedNetworkFeeVoter: true,
	}
	mgr.K = failKeeper
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, failKeeper.Keeper, ctx)
	_ = na

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)
	quoNetFee := &common.QuorumNetworkFee{NetworkFee: nf}
	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleInvalidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	// Attestation with random key (not an active node)
	badAtt := attestNetworkFee(c, secp256k1.GenPrivKey(), nf)
	// Attestation with empty signature
	emptyAtt := &common.Attestation{
		PubKey:    []byte("some-pubkey"),
		Signature: []byte{},
	}

	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{nil, emptyAtt, badAtt},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na.NodeAddress)

	// Invalid attestations are logged but handler still returns success
	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleNoAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)
	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleDuplicateAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 2nd node

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	att := attestNetworkFee(c, privKey1, nf)
	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{att, att, att},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleValidAttestationsReachConsensus(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	// Create 4 nodes with known private keys
	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 4th node

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	// 3 valid attestations (enough for 2/3 consensus with 4 nodes)
	att1 := attestNetworkFee(c, privKey1, nf)
	att2 := attestNetworkFee(c, privKey2, nf)
	att3 := attestNetworkFee(c, privKey3, nf)

	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)

	// Verify the network fee was saved
	savedFee, err := mgr.Keeper().GetNetworkFee(ctx, common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(savedFee.TransactionFeeRate, Equals, uint64(256))
	c.Assert(savedFee.TransactionSize, Equals, uint64(100))
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleMixedValidInvalidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 4th node

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	att1 := attestNetworkFee(c, privKey1, nf)
	att2 := attestNetworkFee(c, privKey2, nf)
	att3 := attestNetworkFee(c, privKey3, nf)
	badAtt := attestNetworkFee(c, secp256k1.GenPrivKey(), nf) // not active node

	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{att1, badAtt, att2, att3},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleFailSaveNetworkFee(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestNetworkFeeQuorumKeeper{
		Keeper:             mgr.Keeper(),
		failSaveNetworkFee: true,
	}
	mgr.K = failKeeper
	handler := NewNetworkFeeQuorumHandler(mgr)

	// Create 4 nodes with known private keys
	na1, privKey1 := nodeWithPrivKey(c, failKeeper.Keeper, ctx)
	_, privKey2 := nodeWithPrivKey(c, failKeeper.Keeper, ctx)
	_, privKey3 := nodeWithPrivKey(c, failKeeper.Keeper, ctx)
	nodeWithPrivKey(c, failKeeper.Keeper, ctx) // 4th node

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	att1 := attestNetworkFee(c, privKey1, nf)
	att2 := attestNetworkFee(c, privKey2, nf)
	att3 := attestNetworkFee(c, privKey3, nf)

	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

// --- Full Run flow ---

func (s *HandlerNetworkFeeQuorumSuite) TestRunFullFlow(c *C) {
	ctx, mgr := setupManagerForTest(c)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	att1 := attestNetworkFee(c, privKey1, nf)
	att2 := attestNetworkFee(c, privKey2, nf)
	att3 := attestNetworkFee(c, privKey3, nf)

	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na1.NodeAddress)

	handler := NewNetworkFeeQuorumHandler(mgr)
	result, err := handler.Run(ctx, msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestRunHandleError(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestNetworkFeeQuorumKeeper{
		Keeper:                    mgr.Keeper(),
		failListActiveNodeAccount: true,
	}
	mgr.K = failKeeper
	handler := NewNetworkFeeQuorumHandler(mgr)

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)
	quoNetFee := &common.QuorumNetworkFee{NetworkFee: nf}
	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, GetRandomBech32Addr())

	// Run should return error from handle, covering the error logging path in Run
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestRunValidationFailure(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	// MsgNetworkFeeQuorum with invalid fields should fail validation
	msg := &types.MsgNetworkFeeQuorum{
		QuoNetFee: &common.QuorumNetworkFee{
			NetworkFee: &common.NetworkFee{
				// Empty chain, zero height
			},
		},
		Signer: GetRandomBech32Addr(),
	}
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerNetworkFeeQuorumSuite) TestHandleVoterSetAfterHandle(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewNetworkFeeQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // need at least 2 active

	nf := makeNetworkFee(common.ETHChain, 1024, 256, 100)

	att1 := attestNetworkFee(c, privKey1, nf)
	quoNetFee := &common.QuorumNetworkFee{
		NetworkFee:   nf,
		Attestations: []*common.Attestation{att1},
	}

	msg := types.NewMsgNetworkFeeQuorum(quoNetFee, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)

	// Voter should have been set (deferred set)
	voter, err := mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	c.Assert(err, IsNil)
	c.Assert(len(voter.Signers) > 0, Equals, true)
}

// --- Test keeper for failure injection ---

type TestNetworkFeeQuorumKeeper struct {
	keeper.Keeper
	failListActiveNodeAccount      bool
	failGetObservedNetworkFeeVoter bool
	failSaveNetworkFee             bool
}

func (k *TestNetworkFeeQuorumKeeper) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if k.failListActiveNodeAccount {
		return NodeAccounts{}, errKaboom
	}
	return k.Keeper.ListActiveValidators(ctx)
}

func (k *TestNetworkFeeQuorumKeeper) GetObservedNetworkFeeVoter(ctx cosmos.Context, height int64, chain common.Chain, rate, size int64) (keeper.ObservedNetworkFeeVoter, error) {
	if k.failGetObservedNetworkFeeVoter {
		return keeper.ObservedNetworkFeeVoter{}, errKaboom
	}
	return k.Keeper.GetObservedNetworkFeeVoter(ctx, height, chain, rate, size)
}

func (k *TestNetworkFeeQuorumKeeper) SaveNetworkFee(ctx cosmos.Context, chain common.Chain, networkFee NetworkFee) error {
	if k.failSaveNetworkFee {
		return errKaboom
	}
	return k.Keeper.SaveNetworkFee(ctx, chain, networkFee)
}
