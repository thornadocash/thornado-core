package thorchain

import (
	"github.com/cometbft/cometbft/crypto/secp256k1"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerSolvencyQuorumSuite struct{}

var _ = Suite(&HandlerSolvencyQuorumSuite{})

func (s *HandlerSolvencyQuorumSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// makeSolvency creates a Solvency struct with a valid Id (hash).
func makeSolvency(c *C, chain common.Chain, pubKey common.PubKey, coins common.Coins, height int64) *common.Solvency {
	sol := &common.Solvency{
		Chain:  chain,
		PubKey: pubKey,
		Coins:  coins,
		Height: height,
	}
	id, err := sol.Hash()
	c.Assert(err, IsNil)
	sol.Id = id
	return sol
}

// attestSolvency creates an attestation for a Solvency message using the given private key.
func attestSolvency(c *C, privKey secp256k1.PrivKey, sol *common.Solvency) *common.Attestation {
	data, err := sol.GetSignablePayload()
	c.Assert(err, IsNil)

	signature, err := privKey.Sign(data)
	c.Assert(err, IsNil)

	return &common.Attestation{
		PubKey:    privKey.PubKey().Bytes(),
		Signature: signature,
	}
}

// --- Run / validate tests ---

func (s *HandlerSolvencyQuorumSuite) TestRunInvalidMessageType(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	// Pass a wrong message type
	msg := NewMsgNetworkFee(1024, common.ETHChain, 1, 10000, GetRandomBech32Addr())
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestValidateEmptySigner(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	// Empty signer should fail ValidateBasic
	msg := &types.MsgSolvencyQuorum{
		QuoSolvency: nil,
		Signer:      cosmos.AccAddress{},
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestValidateNilQuoSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	msg := &types.MsgSolvencyQuorum{
		QuoSolvency: nil,
		Signer:      GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestValidateNilSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	msg := &types.MsgSolvencyQuorum{
		QuoSolvency: &common.QuorumSolvency{
			Solvency: nil,
		},
		Signer: GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestValidateValidMsg(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	privKey := secp256k1.GenPrivKey()
	sol := makeSolvency(c, common.ETHChain, GetRandomPubKey(),
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	att := attestSolvency(c, privKey, sol)
	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{att},
	}
	msg := &types.MsgSolvencyQuorum{
		QuoSolvency: quoSol,
		Signer:      GetRandomBech32Addr(),
	}
	err := handler.validate(ctx, *msg)
	c.Assert(err, IsNil)
}

// --- handle tests ---

func (s *HandlerSolvencyQuorumSuite) TestHandleNilQuoSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	msg := types.MsgSolvencyQuorum{
		QuoSolvency: nil,
		Signer:      GetRandomBech32Addr(),
	}
	result, err := handler.handle(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleNilSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	msg := types.MsgSolvencyQuorum{
		QuoSolvency: &common.QuorumSolvency{
			Solvency: nil,
		},
		Signer: GetRandomBech32Addr(),
	}
	result, err := handler.handle(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleFailListActiveValidators(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestSolvencyQuorumKeeper{
		Keeper:                    mgr.Keeper(),
		failListActiveNodeAccount: true,
	}
	mgr.K = failKeeper
	handler := NewSolvencyQuorumHandler(mgr)

	sol := makeSolvency(c, common.ETHChain, GetRandomPubKey(),
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	quoSol := &common.QuorumSolvency{
		Solvency: sol,
	}
	msg, err := types.NewMsgSolvencyQuorum(quoSol, GetRandomBech32Addr())
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleFailGetSolvencyVoter(c *C) {
	ctx, mgr := setupManagerForTest(c)

	failKeeper := &TestSolvencyQuorumKeeper{
		Keeper:               mgr.Keeper(),
		failGetSolvencyVoter: true,
	}
	mgr.K = failKeeper
	handler := NewSolvencyQuorumHandler(mgr)

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(failKeeper.Keeper.SetNodeAccount(ctx, na), IsNil)

	sol := makeSolvency(c, common.ETHChain, GetRandomPubKey(),
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	quoSol := &common.QuorumSolvency{
		Solvency: sol,
	}
	msg, err := types.NewMsgSolvencyQuorum(quoSol, na.NodeAddress)
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil) // returns &cosmos.Result{}
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleInvalidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	na, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	vaultPubKey := GetRandomPubKey()
	sol := makeSolvency(c, common.ETHChain, vaultPubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)

	// Attestation with random key (not an active node)
	badAtt := attestSolvency(c, secp256k1.GenPrivKey(), sol)
	// Attestation with empty signature
	emptyAtt := &common.Attestation{
		PubKey:    []byte("some-pubkey"),
		Signature: []byte{},
	}

	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{nil, emptyAtt, badAtt},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na.NodeAddress)
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	// Invalid attestations are logged but handler still returns success
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleValidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	// Create 4 nodes with known private keys
	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 4th node

	vaultPubKey := GetRandomPubKey()
	vault := NewVault(1024, ActiveVault, AsgardVault, vaultPubKey, []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Set up ETH pool so solvency check can value gaps in RUNE
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	sol := makeSolvency(c, common.ETHChain, vaultPubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))),
		1024,
	)

	// Create valid attestations from 3 nodes (enough for 2/3 consensus with 4 nodes)
	att1 := attestSolvency(c, privKey1, sol)
	att2 := attestSolvency(c, privKey2, sol)
	att3 := attestSolvency(c, privKey3, sol)

	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na1.NodeAddress)
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)

	// Check that the voter was set
	voter, err := mgr.Keeper().GetSolvencyVoter(ctx, sol.Id, sol.Chain)
	c.Assert(err, IsNil)
	c.Assert(voter.Empty(), Equals, false)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleDuplicateAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 2nd node

	vaultPubKey := GetRandomPubKey()
	vault := NewVault(1024, ActiveVault, AsgardVault, vaultPubKey, []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Set up ETH pool so solvency check can value gaps in RUNE
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	sol := makeSolvency(c, common.ETHChain, vaultPubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)

	// Same attestation duplicated
	att := attestSolvency(c, privKey1, sol)
	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{att, att, att},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na1.NodeAddress)
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleMixedValidInvalidAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // 4th node

	vaultPubKey := GetRandomPubKey()
	vault := NewVault(1024, ActiveVault, AsgardVault, vaultPubKey, []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	sol := makeSolvency(c, common.ETHChain, vaultPubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))),
		1024,
	)

	att1 := attestSolvency(c, privKey1, sol)
	att2 := attestSolvency(c, privKey2, sol)
	att3 := attestSolvency(c, privKey3, sol)
	badAtt := attestSolvency(c, secp256k1.GenPrivKey(), sol) // not active node

	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{att1, badAtt, att2, att3},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na1.NodeAddress)
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

// --- Full Run flow ---

func (s *HandlerSolvencyQuorumSuite) TestRunFullFlow(c *C) {
	ctx, mgr := setupManagerForTest(c)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey2 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	_, privKey3 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx)

	vaultPubKey := GetRandomPubKey()
	vault := NewVault(1024, ActiveVault, AsgardVault, vaultPubKey, []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	sol := makeSolvency(c, common.ETHChain, vaultPubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))),
		1024,
	)

	att1 := attestSolvency(c, privKey1, sol)
	att2 := attestSolvency(c, privKey2, sol)
	att3 := attestSolvency(c, privKey3, sol)

	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{att1, att2, att3},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na1.NodeAddress)
	c.Assert(err, IsNil)

	handler := NewSolvencyQuorumHandler(mgr)
	result, err := handler.Run(ctx, msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

func (s *HandlerSolvencyQuorumSuite) TestRunValidationFailure(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	// MsgSolvencyQuorum with invalid fields should fail validation
	msg := &types.MsgSolvencyQuorum{
		QuoSolvency: &common.QuorumSolvency{
			Solvency: &common.Solvency{
				// Empty fields - no Id, no Chain, no PubKey, Height=0
			},
		},
		Signer: GetRandomBech32Addr(),
	}
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleEmptyVoterCreatesNew(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	na1, privKey1 := nodeWithPrivKey(c, mgr.Keeper(), ctx)
	nodeWithPrivKey(c, mgr.Keeper(), ctx) // need at least 2 active

	vaultPubKey := GetRandomPubKey()
	vault := NewVault(1024, ActiveVault, AsgardVault, vaultPubKey, []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Set up ETH pool so solvency check can value gaps in RUNE
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	sol := makeSolvency(c, common.ETHChain, vaultPubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)

	att1 := attestSolvency(c, privKey1, sol)
	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{att1},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na1.NodeAddress)
	c.Assert(err, IsNil)

	// Voter should not exist yet
	voter, err := mgr.Keeper().GetSolvencyVoter(ctx, sol.Id, sol.Chain)
	c.Assert(err, IsNil)
	c.Assert(voter.Empty(), Equals, true)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)

	// Voter should exist now (set by deferred function)
	voter, err = mgr.Keeper().GetSolvencyVoter(ctx, sol.Id, sol.Chain)
	c.Assert(err, IsNil)
	c.Assert(voter.Empty(), Equals, false)
}

func (s *HandlerSolvencyQuorumSuite) TestHandleNoAttestations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	handler := NewSolvencyQuorumHandler(mgr)

	na1, _ := nodeWithPrivKey(c, mgr.Keeper(), ctx)

	sol := makeSolvency(c, common.ETHChain, GetRandomPubKey(),
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)

	// No attestations
	quoSol := &common.QuorumSolvency{
		Solvency:     sol,
		Attestations: []*common.Attestation{},
	}

	msg, err := types.NewMsgSolvencyQuorum(quoSol, na1.NodeAddress)
	c.Assert(err, IsNil)

	result, err := handler.handle(ctx, *msg)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
}

// --- Test keeper for failure injection ---

type TestSolvencyQuorumKeeper struct {
	keeper.Keeper
	failListActiveNodeAccount bool
	failGetSolvencyVoter      bool
}

func (k *TestSolvencyQuorumKeeper) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if k.failListActiveNodeAccount {
		return NodeAccounts{}, errKaboom
	}
	return k.Keeper.ListActiveValidators(ctx)
}

func (k *TestSolvencyQuorumKeeper) GetSolvencyVoter(ctx cosmos.Context, id common.TxID, chain common.Chain) (keeper.SolvencyVoter, error) {
	if k.failGetSolvencyVoter {
		return keeper.SolvencyVoter{}, errKaboom
	}
	return k.Keeper.GetSolvencyVoter(ctx, id, chain)
}
