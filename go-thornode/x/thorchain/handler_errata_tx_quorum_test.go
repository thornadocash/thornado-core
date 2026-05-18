package thorchain

import (
	"github.com/cometbft/cometbft/crypto/secp256k1"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerErrataTxQuorumSuite struct{}

var _ = Suite(&HandlerErrataTxQuorumSuite{})

func (s *HandlerErrataTxQuorumSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// makeErrataTx creates an ErrataTx struct with a given TxID and chain.
func makeErrataTx(txID common.TxID, chain common.Chain) *common.ErrataTx {
	return &common.ErrataTx{
		Id:    txID,
		Chain: chain,
	}
}

// attestErrataTx creates an attestation for an ErrataTx using the given private key.
func attestErrataTx(c *C, privKey secp256k1.PrivKey, er *common.ErrataTx) *common.Attestation {
	data, err := er.GetSignablePayload()
	c.Assert(err, IsNil)

	signature, err := privKey.Sign(data)
	c.Assert(err, IsNil)

	return &common.Attestation{
		PubKey:    privKey.PubKey().Bytes(),
		Signature: signature,
	}
}

// --- Run / validate tests ---

func (s *HandlerErrataTxQuorumSuite) TestRunInvalidMessageType(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	// Pass a wrong message type
	msg := NewMsgNetworkFee(1024, common.ETHChain, 1, 10000, GetRandomBech32Addr())
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestValidateEmptySigner(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	msg := &types.MsgErrataTxQuorum{
		QuoErrata: nil,
		Signer:    cosmos.AccAddress{},
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestValidateNilQuoErrata(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	msg := &types.MsgErrataTxQuorum{
		QuoErrata: nil,
		Signer:    GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestValidateNilErrataTx(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	msg := &types.MsgErrataTxQuorum{
		QuoErrata: &common.QuorumErrataTx{
			ErrataTx: nil,
		},
		Signer: GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestValidateEmptyTxID(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	msg := &types.MsgErrataTxQuorum{
		QuoErrata: &common.QuorumErrataTx{
			ErrataTx: &common.ErrataTx{
				Id:    "",
				Chain: common.ETHChain,
			},
		},
		Signer: GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestValidateEmptyChain(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	msg := &types.MsgErrataTxQuorum{
		QuoErrata: &common.QuorumErrataTx{
			ErrataTx: &common.ErrataTx{
				Id:    GetRandomTxHash(),
				Chain: common.EmptyChain,
			},
		},
		Signer: GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestValidateValidMsg(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)
	quoErrata := &common.QuorumErrataTx{
		ErrataTx: er,
	}
	msg := types.NewMsgErrataTxQuorum(quoErrata, GetRandomBech32Addr())
	err := handler.validate(ctx, *msg)
	c.Assert(err, IsNil)
}

// --- handle tests ---

func (s *HandlerErrataTxQuorumSuite) TestHandleFailListActiveValidators(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestErrataTxQuorumKeeper{
		Keeper:                    mgr.Keeper(),
		failListActiveNodeAccount: true,
	}
	mgr.K = failKeeper
	handler := NewErrataTxQuorumHandler(mgr)

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)
	quoErrata := &common.QuorumErrataTx{ErrataTx: er}
	msg := types.NewMsgErrataTxQuorum(quoErrata, GetRandomBech32Addr())

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleFailGetErrataTxVoter(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestErrataTxQuorumKeeper{
		Keeper:               mgr.Keeper(),
		failGetErrataTxVoter: true,
	}
	mgr.K = failKeeper
	handler := NewErrataTxQuorumHandler(mgr)

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(failKeeper.Keeper.SetNodeAccount(ctx, na), IsNil)

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)
	quoErrata := &common.QuorumErrataTx{ErrataTx: er}
	msg := types.NewMsgErrataTxQuorum(quoErrata, na.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleInvalidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)

	// Attestation with random key (not an active node)
	badAtt := attestErrataTx(c, secp256k1.GenPrivKey(), er)
	// Attestation with empty signature
	emptyAtt := &common.Attestation{
		PubKey:    []byte("some-pubkey"),
		Signature: []byte{},
	}

	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{nil, emptyAtt, badAtt},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	// Invalid attestations are logged but handler still returns success
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleValidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	// Create 5 nodes with known private keys
	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 4th node
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 5th node

	txID := GetRandomTxHash()
	er := makeErrataTx(txID, common.ETHChain)

	// Create valid attestations from 3 nodes (below 2/3 consensus with 5 nodes)
	att1 := attestErrataTx(c, privKey1, er)
	att2 := attestErrataTx(c, privKey2, er)
	att3 := attestErrataTx(c, privKey3, er)

	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)

	// Check that the voter was set
	voter, err := mgr.Keeper().GetErrataTxVoter(ctx, txID, common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(len(voter.Signers) > 0, Equals, true)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleDuplicateAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 2nd node

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)

	// Same attestation duplicated
	att := attestErrataTx(c, privKey1, er)
	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{att, att, att},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleNoAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	na1, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)

	// No attestations
	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleMixedValidInvalidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 4th node
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 5th node

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)

	att1 := attestErrataTx(c, privKey1, er)
	att2 := attestErrataTx(c, privKey2, er)
	att3 := attestErrataTx(c, privKey3, er)
	badAtt := attestErrataTx(c, secp256k1.GenPrivKey(), er) // not active node

	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{att1, badAtt, att2, att3},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na1.NodeAddress)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

// --- Full Run flow ---

func (s *HandlerErrataTxQuorumSuite) TestRunFullFlow(c *C) {
	ctx, mgr := setupManagerForTest(c)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 5th node to stay below consensus

	txID := GetRandomTxHash()
	er := makeErrataTx(txID, common.ETHChain)

	att1 := attestErrataTx(c, privKey1, er)
	att2 := attestErrataTx(c, privKey2, er)
	att3 := attestErrataTx(c, privKey3, er)

	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na1.NodeAddress)

	handler := NewErrataTxQuorumHandler(mgr)
	result, err := handler.Run(ctx, msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerErrataTxQuorumSuite) TestRunHandleError(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestErrataTxQuorumKeeper{
		Keeper:                    mgr.Keeper(),
		failListActiveNodeAccount: true,
	}
	mgr.K = failKeeper
	handler := NewErrataTxQuorumHandler(mgr)

	er := makeErrataTx(GetRandomTxHash(), common.ETHChain)
	quoErrata := &common.QuorumErrataTx{ErrataTx: er}
	msg := types.NewMsgErrataTxQuorum(quoErrata, GetRandomBech32Addr())

	// Run should return error from handle, covering the error logging path in Run
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestRunValidationFailure(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	// MsgErrataTxQuorum with invalid fields should fail validation
	msg := &types.MsgErrataTxQuorum{
		QuoErrata: &common.QuorumErrataTx{
			ErrataTx: &common.ErrataTx{
				// Empty fields - no Id, no Chain
			},
		},
		Signer: GetRandomBech32Addr(),
	}
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxQuorumSuite) TestHandleEmptyVoterCreatesNew(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewErrataTxQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // need at least 2 active

	txID := GetRandomTxHash()
	er := makeErrataTx(txID, common.ETHChain)

	att1 := attestErrataTx(c, privKey1, er)
	quoErrata := &common.QuorumErrataTx{
		ErrataTx:     er,
		Attestations: []*common.Attestation{att1},
	}

	msg := types.NewMsgErrataTxQuorum(quoErrata, na1.NodeAddress)

	// Voter should not exist yet
	voter, err := mgr.Keeper().GetErrataTxVoter(ctx, txID, common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(len(voter.Signers), Equals, 0)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)

	// Voter should exist now (set by deferred function)
	voter, err = mgr.Keeper().GetErrataTxVoter(ctx, txID, common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(len(voter.Signers) > 0, Equals, true)
}

// --- Test keeper for failure injection ---

type TestErrataTxQuorumKeeper struct {
	keeper.Keeper
	failListActiveNodeAccount bool
	failGetErrataTxVoter      bool
}

func (k *TestErrataTxQuorumKeeper) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if k.failListActiveNodeAccount {
		return NodeAccounts{}, errKaboom
	}
	return k.Keeper.ListActiveValidators(ctx)
}

func (k *TestErrataTxQuorumKeeper) GetErrataTxVoter(ctx cosmos.Context, txID common.TxID, chain common.Chain) (keeper.ErrataTxVoter, error) {
	if k.failGetErrataTxVoter {
		return keeper.ErrataTxVoter{}, errKaboom
	}
	return k.Keeper.GetErrataTxVoter(ctx, txID, chain)
}
