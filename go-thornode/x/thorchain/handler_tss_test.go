package thorchain

import (
	"errors"
	"fmt"
	"sort"

	"github.com/blang/semver"
	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

type HandlerTssSuite struct{}

var _ = Suite(&HandlerTssSuite{})

type tssHandlerTestHelper struct {
	ctx           cosmos.Context
	version       semver.Version
	keeper        *tssKeeperHelper
	poolPk        common.PubKey
	poolPkEddsa   common.PubKey
	constAccessor constants.ConstantValues
	nodeAccount   NodeAccount
	mgr           Manager
	members       common.PubKeys
	signer        cosmos.AccAddress
}

type tssKeeperHelper struct {
	keeper.Keeper
	errListActiveAccounts bool
	errGetTssVoter        bool
	errFailSaveVault      bool
	errFailGetNodeAccount bool
	errFailGetNetwork     bool
	errFailSetNetwork     bool
	errFailSetNodeAccount bool
}

func (k *tssKeeperHelper) GetNodeAccountByPubKey(ctx cosmos.Context, pk common.PubKey) (NodeAccount, error) {
	if k.errFailGetNodeAccount {
		return NodeAccount{}, errKaboom
	}
	return k.Keeper.GetNodeAccountByPubKey(ctx, pk)
}

func (k *tssKeeperHelper) SetVault(ctx cosmos.Context, vault Vault) error {
	if k.errFailSaveVault {
		return errKaboom
	}
	return k.Keeper.SetVault(ctx, vault)
}

func (k *tssKeeperHelper) GetTssVoter(ctx cosmos.Context, id string) (TssVoter, error) {
	if k.errGetTssVoter {
		return TssVoter{}, errKaboom
	}
	return k.Keeper.GetTssVoter(ctx, id)
}

func (k *tssKeeperHelper) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if k.errListActiveAccounts {
		return NodeAccounts{}, errKaboom
	}
	return k.Keeper.ListActiveValidators(ctx)
}

func (k *tssKeeperHelper) GetNetwork(ctx cosmos.Context) (Network, error) {
	if k.errFailGetNetwork {
		return Network{}, errKaboom
	}
	return k.Keeper.GetNetwork(ctx)
}

func (k *tssKeeperHelper) SetNetwork(ctx cosmos.Context, data Network) error {
	if k.errFailSetNetwork {
		return errKaboom
	}
	return k.Keeper.SetNetwork(ctx, data)
}

func (k *tssKeeperHelper) SetNodeAccount(ctx cosmos.Context, na NodeAccount) error {
	if k.errFailSetNodeAccount {
		return errKaboom
	}
	return k.Keeper.SetNodeAccount(ctx, na)
}

func newTssKeeperHelper(keeper keeper.Keeper) *tssKeeperHelper {
	return &tssKeeperHelper{
		Keeper: keeper,
	}
}

func newTssHandlerTestHelper(c *C) tssHandlerTestHelper {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(1023)

	keeperHelper := newTssKeeperHelper(k)
	FundModule(c, ctx, k, BondName, 500*common.One)
	// active account
	nodeAccount := GetRandomValidatorNode(NodeActive)
	nodeAccount.Bond = cosmos.NewUint(100 * common.One)
	c.Assert(keeperHelper.SetNodeAccount(ctx, nodeAccount), IsNil)

	mgr := NewDummyMgr()
	var members common.PubKeys
	for i := 0; i < 8; i++ {
		members = append(members, GetRandomPubKey())
	}
	sort.SliceStable(members, func(i, j int) bool {
		return members[i].String() < members[j].String()
	})
	signer, err := members[0].GetThorAddress()
	c.Assert(err, IsNil)
	nodeReady := GetRandomValidatorNode(NodeReady)
	nodeReady.NodeAddress = signer
	nodeReady.Bond = cosmos.NewUint(1000000 * common.One)
	c.Assert(keeperHelper.SetNodeAccount(ctx, nodeReady), IsNil)
	keygenBlock := NewKeygenBlock(ctx.BlockHeight())
	keygenBlock.Keygens = []Keygen{
		{
			Type:    AsgardKeygen,
			Members: members.Strings(),
		},
	}
	keeperHelper.SetKeygenBlock(ctx, keygenBlock)
	keygenTime := int64(1024)
	poolPk := GetRandomPubKey()
	fakeSig := []byte("fakeSignature")
	poolPkEddsa := GetRandomPubKey()
	msg, err := NewMsgTssPoolV2(members.Strings(), poolPk, fakeSig, nil, AsgardKeygen, ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), signer, keygenTime, poolPkEddsa, nil)
	c.Assert(err, IsNil)
	voter := NewTssVoter(msg.ID, members.Strings(), poolPk, poolPkEddsa)
	keeperHelper.SetTssVoter(ctx, voter)

	asgardVault := NewVaultV2(ctx.BlockHeight(), ActiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.RuneAsset().Chain}.Strings(), []ChainContract{}, poolPkEddsa)
	c.Assert(keeperHelper.SetVault(ctx, asgardVault), IsNil)

	return tssHandlerTestHelper{
		ctx:           ctx,
		version:       mgr.GetVersion(),
		keeper:        keeperHelper,
		poolPk:        poolPk,
		poolPkEddsa:   poolPkEddsa,
		constAccessor: constants.GetConstantValues(GetCurrentVersion()),
		nodeAccount:   nodeAccount,
		mgr:           mgr,
		members:       members,
		signer:        signer,
	}
}

func (s *HandlerTssSuite) TestTssHandler(c *C) {
	fakeSig := []byte("fakeSignature")
	keygenTime := int64(1024)
	testCases := []struct {
		name           string
		messageCreator func(helper tssHandlerTestHelper) cosmos.Msg
		runner         func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error)
		validator      func(helper tssHandlerTestHelper, msg cosmos.Msg, result *cosmos.Result, c *C)
		expectedResult error
	}{
		{
			name: "invalid message should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				return NewMsgNoOp(GetRandomObservedTx(), helper.signer, "")
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: errInvalidMessage,
		},
		{
			name: "Not signed by an active account should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				msg, err := NewMsgTssPool(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), GetRandomValidatorNode(NodeActive).NodeAddress, keygenTime)
				c.Assert(err, IsNil)
				return msg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "empty signer should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				msg, err := NewMsgTssPool(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), cosmos.AccAddress{}, keygenTime)
				c.Assert(err, IsNil)
				return msg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrInvalidAddress,
		},
		{
			name: "empty id should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPool(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime)
				c.Assert(err, IsNil)
				tssMsg.ID = ""
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "empty member pubkeys should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPool(nil, GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "less than two member pubkeys should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPool(common.PubKeys{GetRandomPubKey()}.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "there are empty pubkeys in member pubkey should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPool([]string{GetRandomPubKey().String(), GetRandomPubKey().String(), ""}, GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "success while pool pub key is empty should return error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPool(helper.members.Strings(), common.EmptyPubKey, fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "invalid pool pub key should return error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPool(helper.members.Strings(), "whatever", fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "fail to get tss voter should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				helper.keeper.errGetTssVoter = true
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: errKaboom,
		},
		{
			name: "fail to save vault should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), helper.poolPk, fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, helper.poolPkEddsa, nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
					voter.Secp256K1Signatures = append(voter.Secp256K1Signatures, string(fakeSig))
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				helper.keeper.errFailSaveVault = true
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: errKaboom,
		},
		{
			name: "not having consensus should not perform any actions",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				for i := 0; i < 8; i++ {
					na := GetRandomValidatorNode(NodeActive)
					_ = helper.keeper.SetNodeAccount(helper.ctx, na)
				}
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: nil,
		},
		{
			name: "if signer already sign the voter, it should just return",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, _ := helper.keeper.Keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				if voter.PoolPubKey.IsEmpty() {
					voter.PoolPubKey = tssMsg.PoolPubKey
					voter.PoolPubKeyEddsa = tssMsg.PoolPubKeyEddsa
					voter.PubKeys = tssMsg.PubKeys
				}
				voter.Sign(tssMsg.Signer, tssMsg.Chains, "")
				helper.keeper.Keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: nil,
		},
		{
			name: "normal success",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: nil,
		},
		{
			name: "When tss message integrity compromised, it should result an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				tssMsg.PoolPubKey = GetRandomPubKey()
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "blame node not in keygen participants should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				sort.SliceStable(helper.members, func(i, j int) bool {
					return helper.members[i].String() < helper.members[j].String()
				})
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{Pubkey: "whatever"},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnknownRequest,
		},
		{
			name: "fail to keygen retry should be slashed",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				thorAddr, _ := helper.members[3].GetThorAddress()
				na, _ := helper.keeper.GetNodeAccount(helper.ctx, thorAddr)
				na.UpdateStatus(NodeActive, helper.ctx.BlockHeight())
				_ = helper.keeper.SetNodeAccount(helper.ctx, na)
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{
							Pubkey: helper.members[3].String(),
						},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				constAccessor := constants.GetConstantValues(helper.version)
				observeSlashPoints := constAccessor.GetInt64Value(constants.ObserveSlashPoints)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.mgr.Slasher().IncSlashPoints(helper.ctx, observeSlashPoints, voter.GetSigners()...)
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				m, _ := msg.(*MsgTssPool)
				voter, _ := helper.keeper.GetTssVoter(helper.ctx, m.ID)
				if voter.PoolPubKey.IsEmpty() {
					voter.PoolPubKey = m.PoolPubKey
					voter.PoolPubKeyEddsa = m.PoolPubKeyEddsa
					voter.PubKeys = m.PubKeys
				}
				addr, _ := helper.members[3].GetThorAddress()
				voter.Sign(addr, common.Chains{common.ETHChain}.Strings(), string(fakeSig))
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return handler.Run(helper.ctx, msg)
			},
			validator: func(helper tssHandlerTestHelper, msg cosmos.Msg, result *cosmos.Result, c *C) {
				// make sure node get slashed
				pubKey := helper.members[3]
				na, err := helper.keeper.GetNodeAccountByPubKey(helper.ctx, pubKey)
				c.Assert(err, IsNil)
				slashPts, err := helper.keeper.GetNodeAccountSlashPoints(helper.ctx, na.NodeAddress)
				c.Assert(err, IsNil)
				c.Assert(slashPts > 0, Equals, true)
			},
			expectedResult: nil,
		},
		{
			name: "fail to keygen but can't get network data should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{Pubkey: helper.members[3].String()},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				helper.keeper.errFailGetNetwork = true
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: errKaboom,
		},
		{
			name: "fail to keygen retry and none active account should be slashed with bond",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{Pubkey: helper.members[3].String()},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				vd := Network{
					BondRewardRune: cosmos.NewUint(5000 * common.One),
					TotalBondUnits: cosmos.NewUint(10000),
				}
				_ = helper.keeper.SetNetwork(helper.ctx, vd)
				return handler.Run(helper.ctx, msg)
			},
			validator: func(helper tssHandlerTestHelper, msg cosmos.Msg, result *cosmos.Result, c *C) {
				// make sure node get slashed
				pubKey := helper.members[3]
				na, err := helper.keeper.GetNodeAccountByPubKey(helper.ctx, pubKey)
				c.Assert(err, IsNil)
				c.Assert(na.Bond.Equal(cosmos.ZeroUint()), Equals, true)
				jail, err := helper.keeper.GetNodeAccountJail(helper.ctx, na.NodeAddress)
				c.Assert(err, IsNil)
				c.Check(jail.ReleaseHeight > 0, Equals, true)
			},
			expectedResult: nil,
		},
		{
			name: "fail to keygen and none active account, fail to set network data should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{Pubkey: helper.members[3].String()},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				vd := Network{
					BondRewardRune: cosmos.NewUint(5000 * common.One),
					TotalBondUnits: cosmos.NewUint(10000),
				}
				_ = helper.keeper.SetNetwork(helper.ctx, vd)
				helper.keeper.errFailSetNetwork = true
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: nil,
		},
		{
			name: "fail to keygen and fail to get node account should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{Pubkey: helper.members[3].String()},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				helper.keeper.errFailGetNodeAccount = true
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: errKaboom,
		},
		{
			name: "fail to keygen and fail to set node account should return an error",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				b := Blame{
					FailReason: "who knows",
					BlameNodes: []Node{
						{Pubkey: helper.members[3].String()},
					},
				}
				tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), []Blame{b}, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				voter, err := helper.keeper.GetTssVoter(helper.ctx, tssMsg.ID)
				c.Assert(err, IsNil)
				for _, pk := range helper.members {
					addr, err := pk.GetThorAddress()
					c.Assert(err, IsNil)
					if addr.Equals(helper.signer) {
						continue
					}
					voter.Signers = append(voter.Signers, addr.String())
				}
				helper.keeper.SetTssVoter(helper.ctx, voter)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				helper.keeper.errFailSetNodeAccount = true
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: errKaboom,
		},
		{
			name: "When members in message doesn't match members in keygen blocks should fail",
			messageCreator: func(helper tssHandlerTestHelper) cosmos.Msg {
				members := common.PubKeys{
					GetRandomPubKey(),
					GetRandomPubKey(),
					helper.members[0],
				}
				tssMsg, err := NewMsgTssPoolV2(members.Strings(), GetRandomPubKey(), fakeSig, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), helper.signer, keygenTime, GetRandomPubKey(), nil)
				c.Assert(err, IsNil)
				return tssMsg
			},
			runner: func(handler TssHandler, msg cosmos.Msg, helper tssHandlerTestHelper) (*cosmos.Result, error) {
				return handler.Run(helper.ctx, msg)
			},
			expectedResult: se.ErrUnauthorized,
		},
	}

	// bypass signature checks on TssPool messages
	verifySecp256K1Signature = func(_ common.PubKey, _ []byte) error {
		return nil
	}

	for _, tc := range testCases {
		helper := newTssHandlerTestHelper(c)
		handler := NewTssHandler(NewDummyMgrWithKeeper(helper.keeper))
		msg := tc.messageCreator(helper)
		result, err := tc.runner(handler, msg, helper)
		if tc.expectedResult == nil {
			c.Assert(err, IsNil)
		} else {
			c.Assert(errors.Is(err, tc.expectedResult), Equals, true, Commentf("name:%s, %s", tc.name, err))
		}
		if tc.validator != nil {
			tc.validator(helper, msg, result, c)
		}
	}
}

func (s *HandlerTssSuite) TestKeygenSuccessHandler(c *C) {
	helper := newTssHandlerTestHelper(c)
	handler := NewTssHandler(NewDummyMgrWithKeeper(helper.keeper))
	slasher := handler.mgr.Slasher()
	dummySlasher, ok := slasher.(*DummySlasher)
	c.Assert(ok, Equals, true)
	keygenTime := int64(1024)
	poolPubKey := GetRandomPubKey()
	poolEddsaPubKey := GetRandomPubKey()
	failKeyGenSlashPoints := helper.constAccessor.GetInt64Value(constants.FailKeygenSlashPoints)
	lackOfObservationPenalty := helper.constAccessor.GetInt64Value(constants.LackOfObservationPenalty)
	for idx, item := range helper.members {
		thorAddr, err := item.GetThorAddress()
		c.Assert(err, IsNil)
		tssMsg, err := NewMsgTssPoolV2(helper.members.Strings(), poolPubKey, nil, nil, AsgardKeygen, helper.ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), thorAddr, keygenTime, poolEddsaPubKey, nil)
		c.Assert(err, IsNil)
		err = handler.handle(helper.ctx, tssMsg)
		c.Assert(err, IsNil)
		if HasSuperMajority(idx+1, len(helper.members)) {
			// ensure the late vote members get slashed
			for _, m := range helper.members[idx+1:] {
				slashThorAddr, err := m.GetThorAddress()
				c.Assert(err, IsNil)
				points, pointsOk := dummySlasher.pts[slashThorAddr.String()]
				c.Assert(pointsOk, Equals, true)
				c.Assert(points == failKeyGenSlashPoints+lackOfObservationPenalty, Equals, true)
				j, err := helper.keeper.GetNodeAccountJail(helper.ctx, slashThorAddr)
				c.Assert(err, IsNil)
				c.Assert(j.ReleaseHeight > helper.ctx.BlockHeight(), Equals, true)
			}
		}
	}
	// no one should be slashed
	for _, item := range helper.members {
		thorAddr, err := item.GetThorAddress()
		c.Assert(err, IsNil)
		points, pointsOk := dummySlasher.pts[thorAddr.String()]
		c.Assert(pointsOk, Equals, true)
		c.Assert(points == 0, Equals, true)
		j, err := helper.keeper.GetNodeAccountJail(helper.ctx, thorAddr)
		c.Assert(err, IsNil)
		c.Assert(j.ReleaseHeight <= helper.ctx.BlockHeight(), Equals, true)
	}
}

func (s *HandlerTssSuite) TestObservingSlashing(c *C) {
	ctx, mgr := setupManagerForTest(c)
	height := int64(1024)
	ctx = ctx.WithBlockHeight(height)

	// Check expected slash point amounts
	observeSlashPoints := mgr.GetConstants().GetInt64Value(constants.ObserveSlashPoints)
	lackOfObservationPenalty := mgr.GetConstants().GetInt64Value(constants.LackOfObservationPenalty)
	observeFlex := mgr.GetConstants().GetInt64Value(constants.ObservationDelayFlexibility)
	failKeygenSlashPoints := mgr.GetConstants().GetInt64Value(constants.FailKeygenSlashPoints)
	c.Assert(observeSlashPoints, Equals, int64(1))
	c.Assert(lackOfObservationPenalty, Equals, int64(2))
	c.Assert(observeFlex, Equals, int64(10))
	c.Assert(failKeygenSlashPoints, Equals, int64(720))

	asgardVault := GetRandomVault()
	c.Assert(mgr.Keeper().SetVault(ctx, asgardVault), IsNil)

	nas := NodeAccounts{
		// 6 Active nodes, 1 Standby node; 2/3rd consensus needs 4, 3/3rds success needs 6.
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeStandby),
	}
	for _, item := range nas {
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, item), IsNil)
	}

	var members common.PubKeys
	for i := 0; i < 6; i++ {
		members = append(members, nas[i].PubKeySet.Secp256k1)
	}
	sort.SliceStable(members, func(i, j int) bool {
		return members[i].String() < members[j].String()
	})
	keygenBlock := NewKeygenBlock(ctx.BlockHeight())
	keygenBlock.Keygens = []Keygen{
		{
			Type:    AsgardKeygen,
			Members: members.Strings(),
		},
	}
	mgr.Keeper().SetKeygenBlock(ctx, keygenBlock)
	keygenTime := int64(1024)
	poolPk := GetRandomPubKey()

	msg, err := NewMsgTssPool(members.Strings(), poolPk, nil, nil, AsgardKeygen, ctx.BlockHeight(), nil, common.Chains{common.RuneAsset().Chain}.Strings(), cosmos.AccAddress{}, keygenTime)
	c.Assert(err, IsNil)

	handler := NewTssHandler(mgr)

	broadcast := func(c *C, ctx cosmos.Context, na NodeAccount, msg *MsgTssPool) {
		msg.Signer = na.NodeAddress
		err := handler.handle(ctx, msg)
		c.Assert(err, IsNil)
	}

	checkSlashPoints := func(c *C, ctx cosmos.Context, nas NodeAccounts, expected [7]int64) {
		var slashPoints [7]int64
		for i, na := range nas {
			slashPoint, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, na.NodeAddress)
			c.Assert(err, IsNil)
			slashPoints[i] = slashPoint
		}
		c.Assert(slashPoints == expected, Equals, true, Commentf(fmt.Sprint(slashPoints)))
	}

	checkSlashPoints(c, ctx, nas, [7]int64{0, 0, 0, 0, 0, 0, 0})

	// 3/6 Active nodes observe.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 0, 0, 0})
	broadcast(c, ctx, nas[1], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 1, 0, 0, 0, 0, 0})
	broadcast(c, ctx, nas[2], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 1, 1, 0, 0, 0, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 1, 1, 0, 0, 0, 0})

	// nas[3] observes, reaching consensus (4/6, being exactly the 2/3 threshold).
	// (Active nodes which observed are decremented ObserveSlashPoints;
	//  those which haven't are incremented LackOfObservationPenalty.)
	// Also, those which haven't observed are incremented FailKeygenSlashPoints.
	broadcast(c, ctx, nas[3], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 722, 722, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 722, 722, 0})

	// Within the ObservationDelayFlexibility period, nas[4] observes
	// (and is decremented LackOfObservationPenalty as well as FailKeygenSlashPoints).
	height += observeFlex
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[4], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 722, 0})

	// The ObservationDelayFlexibility period ends, after which nas[5] observes (still within ChurnRetryInterval);
	// it is not incremented ObserveSlashPoints (and it is added to the list of signers)
	// and is also not decremented LackOfObservationPenalty.
	// However, it is decremented FailKeygenSlashPoints as the keygen can still succeed.
	height++
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[5], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 2, 0})

	// nas[5] observes again, this time incremented ObserveSlashPoints for the extra signing.
	broadcast(c, ctx, nas[5], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 3, 0})

	// Note that nas[6], the Standby node, remains unaffected by the Actives nodes' observations.
}
