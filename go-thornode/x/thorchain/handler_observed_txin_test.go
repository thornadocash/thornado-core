package thorchain

import (
	"errors"
	"fmt"

	"github.com/blang/semver"
	se "github.com/cosmos/cosmos-sdk/types/errors"
	"gopkg.in/check.v1"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerObservedTxInSuite struct{}

type TestObservedTxInValidateKeeper struct {
	keeper.KVStoreDummy
	activeNodeAccount NodeAccount
	standbyAccount    NodeAccount
}

func (k *TestObservedTxInValidateKeeper) GetNodeAccount(_ cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error) {
	if addr.Equals(k.standbyAccount.NodeAddress) {
		return k.standbyAccount, nil
	}
	if addr.Equals(k.activeNodeAccount.NodeAddress) {
		return k.activeNodeAccount, nil
	}
	return NodeAccount{}, errKaboom
}

func (k *TestObservedTxInValidateKeeper) SetNodeAccount(_ cosmos.Context, na NodeAccount) error {
	if na.NodeAddress.Equals(k.standbyAccount.NodeAddress) {
		k.standbyAccount = na
		return nil
	}
	return errKaboom
}

var _ = Suite(&HandlerObservedTxInSuite{})

func (s *HandlerObservedTxInSuite) TestValidate(c *C) {
	var err error
	ctx, _ := setupKeeperForTest(c)
	activeNodeAccount := GetRandomValidatorNode(NodeActive)
	standbyAccount := GetRandomValidatorNode(NodeStandby)
	keeper := &TestObservedTxInValidateKeeper{
		activeNodeAccount: activeNodeAccount,
		standbyAccount:    standbyAccount,
	}

	handler := NewObservedTxInHandler(NewDummyMgrWithKeeper(keeper))

	// happy path
	pk := GetRandomPubKey()
	txs := ObservedTxs{NewObservedTx(GetRandomTx(), 12, pk, 12)}
	txs[0].Tx.ToAddress, err = pk.GetAddress(txs[0].Tx.Coins[0].Asset.Chain)
	c.Assert(err, IsNil)
	msg := NewMsgObservedTxIn(txs, activeNodeAccount.NodeAddress)
	err = handler.validate(ctx, *msg)
	c.Assert(err, IsNil)

	// inactive node account
	msg = NewMsgObservedTxIn(txs, GetRandomBech32Addr())
	err = handler.validate(ctx, *msg)
	c.Assert(errors.Is(err, se.ErrUnauthorized), Equals, true)

	// invalid msg
	msg = &MsgObservedTxIn{}
	err = handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

type TestObservedTxInFailureKeeper struct {
	keeper.KVStoreDummy
	pool Pool
}

func (k *TestObservedTxInFailureKeeper) GetPool(_ cosmos.Context, _ common.Asset) (Pool, error) {
	if k.pool.IsEmpty() {
		return NewPool(), nil
	}
	return k.pool, nil
}

func (k *TestObservedTxInFailureKeeper) GetVault(_ cosmos.Context, pubKey common.PubKey) (Vault, error) {
	return Vault{
		PubKey:      pubKey,
		PubKeyEddsa: pubKey,
	}, nil
}

func (s *HandlerObservedTxInSuite) TestFailure(c *C) {
	ctx, _ := setupKeeperForTest(c)
	// w := getHandlerTestWrapper(c, 1, true, false)

	keeper := &TestObservedTxInFailureKeeper{
		pool: Pool{
			Asset:        common.ETHAsset,
			BalanceRune:  cosmos.NewUint(200),
			BalanceAsset: cosmos.NewUint(300),
		},
	}
	mgr := NewDummyMgrWithKeeper(keeper)

	tx := NewObservedTx(GetRandomTx(), 12, GetRandomPubKey(), 12)
	err := refundTx(ctx, tx, mgr, CodeInvalidMemo, "Invalid memo", "")
	c.Assert(err, IsNil)
	items, err := mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Check(items, HasLen, 1)
}

type TestObservedTxInHandleKeeper struct {
	keeper.KVStoreDummy
	nas                  NodeAccounts
	voter                ObservedTxVoter
	vaultExists          bool
	height               int64
	msg                  MsgSwap
	pool                 Pool
	observing            []cosmos.AccAddress
	vault                Vault
	txOut                *TxOut
	setLastObserveHeight bool
	referenceMemos       map[string]ReferenceMemo // key: asset:reference
}

func (k *TestObservedTxInHandleKeeper) SetSwapQueueItem(_ cosmos.Context, msg MsgSwap, _ int) error {
	k.msg = msg
	return nil
}

func (k *TestObservedTxInHandleKeeper) ListActiveValidators(_ cosmos.Context) (NodeAccounts, error) {
	return k.nas, nil
}

func (k *TestObservedTxInHandleKeeper) GetObservedTxInVoter(_ cosmos.Context, _ common.TxID) (ObservedTxVoter, error) {
	return k.voter, nil
}

func (k *TestObservedTxInHandleKeeper) SetObservedTxInVoter(_ cosmos.Context, voter ObservedTxVoter) {
	k.voter = voter
}

func (k *TestObservedTxInHandleKeeper) VaultExists(_ cosmos.Context, _ common.PubKey) bool {
	return k.vaultExists
}

func (k *TestObservedTxInHandleKeeper) SetLastChainHeight(_ cosmos.Context, _ common.Chain, height int64) error {
	k.height = height
	return nil
}

func (k *TestObservedTxInHandleKeeper) GetPool(_ cosmos.Context, _ common.Asset) (Pool, error) {
	if k.pool.IsEmpty() {
		return NewPool(), nil
	}
	return k.pool, nil
}

func (k *TestObservedTxInHandleKeeper) AddObservingAddresses(_ cosmos.Context, addrs []cosmos.AccAddress) error {
	k.observing = addrs
	return nil
}

func (k *TestObservedTxInHandleKeeper) GetVault(_ cosmos.Context, key common.PubKey) (Vault, error) {
	if k.vault.PubKey.Equals(key) {
		return k.vault, nil
	}
	return GetRandomVault(), errKaboom
}

func (k *TestObservedTxInHandleKeeper) GetAsgardVaults(_ cosmos.Context) (Vaults, error) {
	return Vaults{k.vault}, nil
}

func (k *TestObservedTxInHandleKeeper) SetVault(_ cosmos.Context, vault Vault) error {
	if k.vault.PubKey.Equals(vault.PubKey) {
		k.vault = vault
		return nil
	}
	return errKaboom
}

func (k *TestObservedTxInHandleKeeper) GetLowestActiveVersion(_ cosmos.Context) semver.Version {
	return GetCurrentVersion()
}

func (k *TestObservedTxInHandleKeeper) IsActiveObserver(_ cosmos.Context, addr cosmos.AccAddress) bool {
	return addr.Equals(k.nas[0].NodeAddress)
}

func (k *TestObservedTxInHandleKeeper) GetTxOut(ctx cosmos.Context, blockHeight int64) (*TxOut, error) {
	if k.txOut != nil && k.txOut.Height == blockHeight {
		return k.txOut, nil
	}
	return nil, errKaboom
}

func (k *TestObservedTxInHandleKeeper) SetTxOut(ctx cosmos.Context, blockOut *TxOut) error {
	if k.txOut.Height == blockOut.Height {
		k.txOut = blockOut
		return nil
	}
	return errKaboom
}

func (k *TestObservedTxInHandleKeeper) SetLastObserveHeight(ctx cosmos.Context, chain common.Chain, address cosmos.AccAddress, height int64) error {
	k.setLastObserveHeight = true
	return nil
}

func (k *TestObservedTxInHandleKeeper) ReferenceMemoExists(_ cosmos.Context, asset common.Asset, reference string) bool {
	if k.referenceMemos == nil {
		return false
	}
	key := fmt.Sprintf("%s:%s", asset.String(), reference)
	_, exists := k.referenceMemos[key]
	return exists
}

func (k *TestObservedTxInHandleKeeper) GetReferenceMemo(_ cosmos.Context, asset common.Asset, reference string) (ReferenceMemo, error) {
	if k.referenceMemos == nil {
		return ReferenceMemo{}, fmt.Errorf("reference memo not found")
	}
	key := fmt.Sprintf("%s:%s", asset.String(), reference)
	if memo, exists := k.referenceMemos[key]; exists {
		return memo, nil
	}
	return ReferenceMemo{}, fmt.Errorf("reference memo not found")
}

func (k *TestObservedTxInHandleKeeper) SetReferenceMemo(_ cosmos.Context, memo ReferenceMemo) {
	if k.referenceMemos == nil {
		k.referenceMemos = make(map[string]ReferenceMemo)
	}
	key := fmt.Sprintf("%s:%s", memo.Asset.String(), memo.Reference)
	k.referenceMemos[key] = memo
}

func (k *TestObservedTxInHandleKeeper) GetMimir(_ cosmos.Context, key string) (int64, error) {
	// For tests, memoless transactions are not halted
	if key == "HaltMemoless" {
		return 0, nil
	}
	return 0, fmt.Errorf("mimir not found")
}

func (k *TestObservedTxInHandleKeeper) GetConfigInt64(_ cosmos.Context, key constants.ConstantName) int64 {
	// Set reasonable defaults for test
	switch key {
	case constants.MemolessTxnTTL:
		return 100 // blocks
	case constants.MemolessTxnMaxUse:
		return 10 // max usage
	case constants.MemolessTxnRefCount:
		return 99999 // memoless txn reference id counts
	default:
		return 0
	}
}

func (s *HandlerObservedTxInSuite) TestHandle(c *C) {
	s.testHandleWithVersion(c)
	s.testHandleWithConfirmation(c)
}

func (s *HandlerObservedTxInSuite) testHandleWithConfirmation(c *C) {
	var err error
	ctx, mgr := setupManagerForTest(c)
	tx := GetRandomTx()
	tx.Memo = "SWAP:BTC.BTC:" + GetRandomBTCAddress().String()
	obTx := NewObservedTx(tx, 12, GetRandomPubKey(), 15)
	txs := ObservedTxs{obTx}
	pk := GetRandomPubKey()
	txs[0].Tx.ToAddress, err = pk.GetAddress(txs[0].Tx.Coins[0].Asset.Chain)
	c.Assert(err, IsNil)
	vault := GetRandomVault()
	vault.PubKey = obTx.ObservedPubKey

	keeper := &TestObservedTxInHandleKeeper{
		nas: NodeAccounts{
			GetRandomValidatorNode(NodeActive),
			GetRandomValidatorNode(NodeActive),
			GetRandomValidatorNode(NodeActive),
			GetRandomValidatorNode(NodeActive),
		},
		vault: vault,
		pool: Pool{
			Asset:        common.ETHAsset,
			BalanceRune:  cosmos.NewUint(200),
			BalanceAsset: cosmos.NewUint(300),
		},
		vaultExists: true,
	}
	mgr.K = keeper
	handler := NewObservedTxInHandler(mgr)

	// first not confirmed message
	msg := NewMsgObservedTxIn(txs, keeper.nas[0].NodeAddress)
	_, err = handler.handle(ctx, *msg)
	c.Assert(err, IsNil)
	voter, err := keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.Txs, HasLen, 1)
	// tx has not reach consensus yet, thus fund should not be credit to vault
	c.Assert(keeper.vault.HasFunds(), Equals, false)
	c.Assert(voter.UpdatedVault, Equals, false)
	c.Assert(voter.FinalisedHeight, Equals, int64(0))
	c.Assert(voter.Height, Equals, int64(0))
	mgr.ObMgr().EndBlock(ctx, keeper)

	// second not confirmed message
	msg1 := NewMsgObservedTxIn(txs, keeper.nas[1].NodeAddress)
	_, err = handler.handle(ctx, *msg1)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.Txs, HasLen, 1)
	c.Assert(voter.UpdatedVault, Equals, false)
	c.Assert(voter.FinalisedHeight, Equals, int64(0))
	c.Assert(voter.Height, Equals, int64(0))
	c.Assert(keeper.vault.HasFunds(), Equals, false)

	// third not confirmed message
	msg2 := NewMsgObservedTxIn(txs, keeper.nas[2].NodeAddress)
	_, err = handler.handle(ctx, *msg2)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.Txs, HasLen, 1)
	c.Assert(voter.UpdatedVault, Equals, false)
	c.Assert(voter.FinalisedHeight, Equals, int64(0))
	c.Check(keeper.height, Equals, int64(12))
	ethCoin := keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.ZeroUint()), Equals, true)
	// make sure the logic has not been processed , as tx has not been finalised , still waiting for confirmation
	c.Check(keeper.msg.Tx.ID.Equals(tx.ID), Equals, false)

	// fourth not confirmed message
	msg3 := NewMsgObservedTxIn(txs, keeper.nas[3].NodeAddress)
	_, err = handler.handle(ctx, *msg3)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.Txs, HasLen, 1)
	c.Assert(voter.UpdatedVault, Equals, false)
	c.Assert(voter.FinalisedHeight, Equals, int64(0))
	c.Check(keeper.height, Equals, int64(12))
	ethCoin = keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.ZeroUint()), Equals, true)
	c.Check(keeper.msg.Tx.ID.Equals(tx.ID), Equals, false)

	//  first finalised message
	txs[0].BlockHeight = 15
	fMsg := NewMsgObservedTxIn(txs, keeper.nas[0].NodeAddress)
	_, err = handler.handle(ctx, *fMsg)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.UpdatedVault, Equals, false)
	c.Assert(voter.FinalisedHeight, Equals, int64(0))
	c.Assert(voter.Height, Equals, int64(18))
	ethCoin = keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.ZeroUint()), Equals, true)
	c.Check(keeper.msg.Tx.ID.Equals(tx.ID), Equals, false)

	// second finalised message
	fMsg1 := NewMsgObservedTxIn(txs, keeper.nas[1].NodeAddress)
	_, err = handler.handle(ctx, *fMsg1)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.UpdatedVault, Equals, false)
	c.Assert(voter.FinalisedHeight, Equals, int64(0))
	c.Assert(voter.Height, Equals, int64(18))
	ethCoin = keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.ZeroUint()), Equals, true)
	c.Check(keeper.msg.Tx.ID.Equals(tx.ID), Equals, false)

	// third finalised message
	fMsg2 := NewMsgObservedTxIn(txs, keeper.nas[2].NodeAddress)
	_, err = handler.handle(ctx, *fMsg2)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.UpdatedVault, Equals, true)
	c.Assert(voter.FinalisedHeight, Equals, int64(18))
	c.Assert(voter.Height, Equals, int64(18))
	// make sure fund has been credit to vault correctly
	ethCoin = keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.OneUint()), Equals, true)
	c.Check(keeper.msg.Tx.ID.String(), Equals, tx.ID.String())

	// third finalised message
	fMsg3 := NewMsgObservedTxIn(txs, keeper.nas[3].NodeAddress)
	_, err = handler.handle(ctx, *fMsg3)
	c.Assert(err, IsNil)
	voter, err = keeper.GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)
	c.Assert(voter.UpdatedVault, Equals, true)
	c.Assert(voter.FinalisedHeight, Equals, int64(18))
	c.Assert(voter.Height, Equals, int64(18))
	// make sure fund has not been doubled
	ethCoin = keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.OneUint()), Equals, true)
	c.Check(keeper.msg.Tx.ID.String(), Equals, tx.ID.String())
}

func (s *HandlerObservedTxInSuite) testHandleWithVersion(c *C) {
	var err error
	ctx, mgr := setupManagerForTest(c)

	tx := GetRandomTx()
	tx.Memo = "SWAP:BTC.BTC:" + GetRandomBTCAddress().String()
	obTx := NewObservedTx(tx, 12, GetRandomPubKey(), 12)
	txs := ObservedTxs{obTx}
	pk := GetRandomPubKey()
	txs[0].Tx.ToAddress, err = pk.GetAddress(txs[0].Tx.Coins[0].Asset.Chain)

	vault := GetRandomVault()
	vault.PubKey = obTx.ObservedPubKey

	keeper := &TestObservedTxInHandleKeeper{
		nas:   NodeAccounts{GetRandomValidatorNode(NodeActive)},
		voter: NewObservedTxVoter(tx.ID, make(ObservedTxs, 0)),
		vault: vault,
		pool: Pool{
			Asset:        common.ETHAsset,
			BalanceRune:  cosmos.NewUint(200),
			BalanceAsset: cosmos.NewUint(300),
		},
		vaultExists: true,
	}
	mgr.K = keeper
	handler := NewObservedTxInHandler(mgr)

	c.Assert(err, IsNil)
	msg := NewMsgObservedTxIn(txs, keeper.nas[0].NodeAddress)
	_, err = handler.handle(ctx, *msg)
	c.Assert(err, IsNil)
	mgr.ObMgr().EndBlock(ctx, keeper)
	c.Check(keeper.msg.Tx.ID.String(), Equals, tx.ID.String())
	c.Check(keeper.observing, HasLen, 1)
	c.Check(keeper.height, Equals, int64(12))
	ethCoin := keeper.vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.OneUint()), Equals, true)
}

// Test migrate memo
func (s *HandlerObservedTxInSuite) TestMigrateMemo(c *C) {
	var err error
	ctx, _ := setupKeeperForTest(c)

	vault := GetRandomVault()
	addr, err := vault.PubKey.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	newVault := GetRandomVault()
	txout := NewTxOut(12)
	newVaultAddr, err := newVault.PubKey.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)

	txout.TxArray = append(txout.TxArray, TxOutItem{
		Chain:       common.ETHChain,
		InHash:      common.BlankTxID,
		ToAddress:   newVaultAddr,
		VaultPubKey: vault.PubKey,
		Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(1024)),
		Memo:        NewMigrateMemo(1).String(),
	})
	tx := NewObservedTx(common.Tx{
		ID:    GetRandomTxHash(),
		Chain: common.ETHChain,
		Coins: common.Coins{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(1024)),
		},
		Memo:        NewMigrateMemo(12).String(),
		FromAddress: addr,
		ToAddress:   newVaultAddr,
		Gas: common.Gas{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
		},
	}, 13, vault.PubKey, 13)

	txs := ObservedTxs{tx}
	keeper := &TestObservedTxInHandleKeeper{
		nas:   NodeAccounts{GetRandomValidatorNode(NodeActive)},
		voter: NewObservedTxVoter(tx.Tx.ID, make(ObservedTxs, 0)),
		vault: vault,
		pool: Pool{
			Asset:        common.ETHAsset,
			BalanceRune:  cosmos.NewUint(200),
			BalanceAsset: cosmos.NewUint(300),
		},
		vaultExists: true,
		txOut:       txout,
	}

	handler := NewObservedTxInHandler(NewDummyMgrWithKeeper(keeper))

	c.Assert(err, IsNil)
	msg := NewMsgObservedTxIn(txs, keeper.nas[0].NodeAddress)
	_, err = handler.handle(ctx, *msg)
	c.Assert(err, IsNil)
}

type ObservedTxInHandlerTestHelper struct {
	keeper.Keeper
	failListActiveValidators bool
	failVaultExist           bool
	failGetObservedTxInVote  bool
	failGetVault             bool
	failSetVault             bool
}

func NewObservedTxInHandlerTestHelper(k keeper.Keeper) *ObservedTxInHandlerTestHelper {
	return &ObservedTxInHandlerTestHelper{
		Keeper: k,
	}
}

func (h *ObservedTxInHandlerTestHelper) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if h.failListActiveValidators {
		return NodeAccounts{}, errKaboom
	}
	return h.Keeper.ListActiveValidators(ctx)
}

func (h *ObservedTxInHandlerTestHelper) VaultExists(ctx cosmos.Context, pk common.PubKey) bool {
	if h.failVaultExist {
		return false
	}
	return h.Keeper.VaultExists(ctx, pk)
}

func (h *ObservedTxInHandlerTestHelper) GetObservedTxInVoter(ctx cosmos.Context, hash common.TxID) (ObservedTxVoter, error) {
	if h.failGetObservedTxInVote {
		return ObservedTxVoter{}, errKaboom
	}
	return h.Keeper.GetObservedTxInVoter(ctx, hash)
}

func (h *ObservedTxInHandlerTestHelper) GetVault(ctx cosmos.Context, pk common.PubKey) (Vault, error) {
	if h.failGetVault {
		return Vault{}, errKaboom
	}
	return h.Keeper.GetVault(ctx, pk)
}

func (h *ObservedTxInHandlerTestHelper) SetVault(ctx cosmos.Context, vault Vault) error {
	if h.failSetVault {
		return errKaboom
	}
	return h.Keeper.SetVault(ctx, vault)
}

func setupAnLegitObservedTx(ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper, c *C) *MsgObservedTxIn {
	activeNodeAccount := GetRandomValidatorNode(NodeActive)
	pk := GetRandomPubKey()
	tx := GetRandomTx()
	tx.Coins = common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One*3)),
	}
	tx.Memo = "SWAP:RUNE"
	addr, err := pk.GetAddress(tx.Coins[0].Asset.Chain)
	c.Assert(err, IsNil)
	tx.ToAddress = addr
	obTx := NewObservedTx(tx, ctx.BlockHeight(), pk, ctx.BlockHeight())
	txs := ObservedTxs{obTx}
	txs[0].Tx.ToAddress, err = pk.GetAddress(txs[0].Tx.Coins[0].Asset.Chain)
	c.Assert(err, IsNil)
	vault := GetRandomVault()
	vault.PubKey = obTx.ObservedPubKey
	c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
	c.Assert(helper.SetVault(ctx, vault), IsNil)
	p := NewPool()
	p.Asset = common.ETHAsset
	p.BalanceRune = cosmos.NewUint(100 * common.One)
	p.BalanceAsset = cosmos.NewUint(100 * common.One)
	p.Status = PoolAvailable
	c.Assert(helper.Keeper.SetPool(ctx, p), IsNil)
	return NewMsgObservedTxIn(ObservedTxs{
		obTx,
	}, activeNodeAccount.NodeAddress)
}

func (HandlerObservedTxInSuite) TestObservedTxHandler_validations(c *C) {
	testCases := []struct {
		name            string
		messageProvider func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg
		validator       func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string)
	}{
		{
			name: "invalid message should return an error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				return NewMsgNetworkFee(ctx.BlockHeight(), common.ETHChain, 1, 10000, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil)
				c.Check(errors.Is(err, errInvalidMessage), Equals, true)
			},
		},
		{
			name: "message fail validation should return an error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				return NewMsgObservedTxIn(ObservedTxs{
					NewObservedTx(GetRandomTx(), ctx.BlockHeight(), GetRandomPubKey(), ctx.BlockHeight()),
				}, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil)
			},
		},
		{
			name: "signer vote for the same tx should be slashed , and not doing anything else",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				voter, err := helper.Keeper.GetObservedTxInVoter(ctx, m.Txs[0].Tx.ID)
				c.Assert(err, IsNil)
				voter.Add(m.Txs[0], m.Signer)
				helper.Keeper.SetObservedTxInVoter(ctx, voter)
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to list active node accounts should result in an error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				helper.failListActiveValidators = true
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "vault not exist should not result in an error, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				helper.failVaultExist = true
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to get observedTxInVoter should not result in an error, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				helper.failGetObservedTxInVote = true
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "empty memo should not result in an error, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				m.Txs[0].Tx.Memo = ""
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
				txOut, err := helper.GetTxOut(ctx, ctx.BlockHeight())
				c.Assert(err, IsNil, Commentf(name))
				c.Assert(txOut.IsEmpty(), Equals, false)
			},
		},
		{
			name: "fail to get vault, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				helper.failGetVault = true
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to set vault, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				helper.failSetVault = true
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "if the vault is not asgard, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				vault, err := helper.Keeper.GetVault(ctx, m.Txs[0].ObservedPubKey)
				c.Assert(err, IsNil)
				vault.Type = UnknownVault
				c.Assert(helper.Keeper.SetVault(ctx, vault), IsNil)
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "inactive vault, it should continue",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				vault, err := helper.Keeper.GetVault(ctx, m.Txs[0].ObservedPubKey)
				c.Assert(err, IsNil)
				vault.Status = InactiveVault
				c.Assert(helper.Keeper.SetVault(ctx, vault), IsNil)
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "chain halt, it should refund",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				helper.Keeper.SetMimir(ctx, "HaltTrading", 1)
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
				txOut, err := helper.GetTxOut(ctx, ctx.BlockHeight())
				c.Assert(err, IsNil, Commentf(name))
				c.Assert(txOut.IsEmpty(), Equals, false)
			},
		},
		{
			name: "normal provision, it should success",
			messageProvider: func(c *C, ctx cosmos.Context, helper *ObservedTxInHandlerTestHelper) cosmos.Msg {
				m := setupAnLegitObservedTx(ctx, helper, c)
				m.Txs[0].Tx.Memo = "add:ETH.ETH"
				return m
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ObservedTxInHandlerTestHelper, name string) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
	}
	versions := []semver.Version{
		GetCurrentVersion(),
	}
	for _, tc := range testCases {
		for _, ver := range versions {
			ctx, mgr := setupManagerForTest(c)
			helper := NewObservedTxInHandlerTestHelper(mgr.Keeper())
			mgr.K = helper
			mgr.currentVersion = ver
			handler := NewObservedTxInHandler(mgr)
			msg := tc.messageProvider(c, ctx, helper)
			result, err := handler.Run(ctx, msg)
			tc.validator(c, ctx, result, err, helper, tc.name)
		}
	}
}

func (s HandlerObservedTxInSuite) TestSwapWithAffiliate(c *C) {
	ctx, mgr := setupManagerForTest(c)

	queue := newSwapQueue(mgr.Keeper())

	affAddr := GetRandomTHORAddress()

	msg := NewMsgSwap(common.Tx{
		ID:          common.TxID("5E1DF027321F1FE37CA19B9ECB11C2B4ABEC0D8322199D335D9CE4C39F85F115"),
		FromAddress: GetRandomETHAddress(),
		ToAddress:   GetRandomETHAddress(),
		Gas: common.Gas{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
		},
		Chain: common.ETHChain,
		Coins: common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(2*common.One))},
		Memo:  "=:THOR.RUNE:" + GetRandomTHORAddress().String() + "::" + affAddr.String() + ":1000",
	}, common.RuneAsset(), GetRandomTHORAddress(), cosmos.ZeroUint(), affAddr, cosmos.NewUint(1000),
		"",
		"", nil,
		types.SwapType_market,
		0, 0, types.SwapVersion_v1, GetRandomBech32Addr(),
	)
	// no affiliate fees
	c.Assert(addSwap(ctx, mgr, *msg), IsNil)

	// Check if the swap was added to the appropriate queue
	// Route based on message version (V1 goes to regular queue, V2 goes to advanced queue)
	if msg.IsV2() {
		// Swap should be in advanced queue
		swap, err := mgr.Keeper().GetAdvSwapQueueItem(ctx, msg.Tx.ID, 0)
		c.Assert(err, IsNil)
		c.Check(swap.Tx.Coins[0].Amount.Uint64(), Equals, uint64(200000000))
	} else {
		// Swap should be in regular queue
		swaps, err := queue.FetchQueue(ctx)
		c.Assert(err, IsNil)
		c.Assert(swaps, HasLen, 1, Commentf("%d", len(swaps)))
		c.Check(swaps[0].msg.Tx.Coins[0].Amount.Uint64(), Equals, uint64(200000000))
	}
}

func (s HandlerObservedTxInSuite) TestAddSwapEmitsLimitEventOnlyForLimitSwaps(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup target pool required by validation for RUNE -> BTC swaps.
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(10000 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	countEvents := func(eventType string) int {
		count := 0
		for _, e := range ctx.EventManager().Events() {
			if e.Type == eventType {
				count++
			}
		}
		return count
	}

	limitSwapEventsBefore := countEvents(types.LimitSwapEventType)

	// V2 market swap should not emit a limit_swap event.
	marketMsg := NewMsgSwap(common.Tx{
		ID:          GetRandomTxHash(),
		FromAddress: GetRandomTHORAddress(),
		ToAddress:   GetRandomTHORAddress(),
		Gas:         common.Gas{},
		Chain:       common.THORChain,
		Coins:       common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(2*common.One))},
		Memo:        "=:BTC.BTC:" + GetRandomBTCAddress().String(),
	}, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(),
		"",
		"", nil,
		types.SwapType_market,
		0, 0, types.SwapVersion_v2, GetRandomBech32Addr(),
	)
	c.Assert(addSwap(ctx, mgr, *marketMsg), IsNil)
	c.Assert(countEvents(types.LimitSwapEventType), Equals, limitSwapEventsBefore)

	// V2 limit swap should emit exactly one limit_swap event.
	limitMsg := NewMsgSwap(common.Tx{
		ID:          GetRandomTxHash(),
		FromAddress: GetRandomTHORAddress(),
		ToAddress:   GetRandomTHORAddress(),
		Gas:         common.Gas{},
		Chain:       common.THORChain,
		Coins:       common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(2*common.One))},
		Memo:        "=<:BTC.BTC:" + GetRandomBTCAddress().String() + ":1",
	}, common.BTCAsset, GetRandomBTCAddress(), cosmos.NewUint(1), common.NoAddress, cosmos.ZeroUint(),
		"",
		"", nil,
		types.SwapType_limit,
		0, 0, types.SwapVersion_v2, GetRandomBech32Addr(),
	)
	c.Assert(addSwap(ctx, mgr, *limitMsg), IsNil)
	c.Assert(countEvents(types.LimitSwapEventType), Equals, limitSwapEventsBefore+1)
}

func (s HandlerObservedTxInSuite) TestAddSwapDoesNotEmitLimitEventWhenQueueInsertFails(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Put advanced swap queue into market-only mode so v2 limit swaps are rejected.
	mgr.Keeper().SetMimir(ctx, "EnableAdvSwapQueue", int64(AdvSwapQueueModeMarketOnly))

	// Setup target pool required by validation for RUNE -> BTC swaps.
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(10000 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	countEvents := func(eventType string) int {
		count := 0
		for _, e := range ctx.EventManager().Events() {
			if e.Type == eventType {
				count++
			}
		}
		return count
	}

	limitSwapEventsBefore := countEvents(types.LimitSwapEventType)

	limitMsg := NewMsgSwap(common.Tx{
		ID:          GetRandomTxHash(),
		FromAddress: GetRandomTHORAddress(),
		ToAddress:   GetRandomTHORAddress(),
		Gas:         common.Gas{},
		Chain:       common.THORChain,
		Coins:       common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(2*common.One))},
		Memo:        "=<:BTC.BTC:" + GetRandomBTCAddress().String() + ":1",
	}, common.BTCAsset, GetRandomBTCAddress(), cosmos.NewUint(1), common.NoAddress, cosmos.ZeroUint(),
		"",
		"", nil,
		types.SwapType_limit,
		0, 0, types.SwapVersion_v2, GetRandomBech32Addr(),
	)

	err := addSwap(ctx, mgr, *limitMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "limit swaps are not allowed in market-only mode")
	c.Assert(countEvents(types.LimitSwapEventType), Equals, limitSwapEventsBefore)
}

func (s *HandlerObservedTxInSuite) TestVaultStatus(c *C) {
	testCases := []struct {
		name                 string
		statusAtConsensus    VaultStatus
		statusAtFinalisation VaultStatus
	}{
		{
			name:                 "should observe if active on consensus and finalisation",
			statusAtConsensus:    ActiveVault,
			statusAtFinalisation: ActiveVault,
		}, {
			name:                 "should observe if active on consensus, inactive on finalisation",
			statusAtConsensus:    ActiveVault,
			statusAtFinalisation: InactiveVault,
		}, {
			name:                 "should not observe if inactive on consensus",
			statusAtConsensus:    InactiveVault,
			statusAtFinalisation: InactiveVault,
		},
	}
	for _, tc := range testCases {
		var err error
		ctx, mgr := setupManagerForTest(c)
		tx := GetRandomTx()
		tx.Memo = "SWAP:BTC.BTC:" + GetRandomBTCAddress().String()
		obTx := NewObservedTx(tx, 12, GetRandomPubKey(), 15)
		txs := ObservedTxs{obTx}
		vault := GetRandomVault()
		vault.PubKey = obTx.ObservedPubKey
		keeper := &TestObservedTxInHandleKeeper{
			nas:         NodeAccounts{GetRandomValidatorNode(NodeActive)},
			voter:       NewObservedTxVoter(tx.ID, make(ObservedTxs, 0)),
			vault:       vault,
			vaultExists: true,
		}
		mgr.K = keeper
		handler := NewObservedTxInHandler(mgr)

		keeper.vault.Status = tc.statusAtConsensus
		msg := NewMsgObservedTxIn(txs, keeper.nas[0].NodeAddress)
		_, err = handler.handle(ctx, *msg)
		c.Assert(err, IsNil, Commentf(tc.name))
		c.Check(keeper.voter.Height, Equals, int64(18), Commentf(tc.name))

		c.Check(keeper.voter.UpdatedVault, Equals, false, Commentf(tc.name))
		c.Check(keeper.vault.InboundTxCount, Equals, int64(0), Commentf(tc.name))

		keeper.vault.Status = tc.statusAtFinalisation
		txs[0].BlockHeight = 15
		msg = NewMsgObservedTxIn(txs, keeper.nas[0].NodeAddress)
		ctx = ctx.WithBlockHeight(30)
		_, err = handler.handle(ctx, *msg)
		c.Assert(err, IsNil, Commentf(tc.name))
		c.Check(keeper.voter.FinalisedHeight, Equals, int64(30), Commentf(tc.name))

		c.Check(keeper.voter.UpdatedVault, Equals, true, Commentf(tc.name))
		c.Check(keeper.vault.InboundTxCount, Equals, int64(1), Commentf(tc.name))
	}
}

func (s *HandlerObservedTxInSuite) TestMemolessTxns(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)
	asset := common.BTCAsset

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 10)

	// Test when memo is not empty
	testTx := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "myMemo")
	memo := fetchMemoFromReference(ctx, mgr, asset, testTx, 15)
	c.Check(memo, check.Equals, "myMemo", check.Commentf("Expected to return the same memo when memo is not empty"))

	testTx2 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx2, 15)
	c.Check(memo, check.Equals, "", check.Commentf("Expected to return empty memo when memo is empty"))

	// Test when reference memo is not expired, happy path
	refMemo := NewReferenceMemo(asset, "the memo", "30000", 10)
	mgr.Keeper().SetReferenceMemo(ctx, refMemo)
	testTx3 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:30000")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx3, 15) // tx observed at height 15, memo created at height 10
	c.Check(memo, check.Equals, "the memo", check.Commentf("Expected to return reference memo when it's not expired"))

	// Test when reference memo is expired
	ctx = ctx.WithBlockHeight(25)
	testTx4 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:30000")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx4, 15) // tx observed at height 15, memo created at height 10
	c.Check(memo, check.Equals, "", check.Commentf("Expected to return empty memo when reference memo is expired"))
}

func (s *HandlerObservedTxInSuite) TestMemoUsageTracking(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)
	asset := common.BTCAsset

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)
	// Set max usage limit to 1
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnMaxUse.String(), 1)

	// Create a reference memo
	refMemo := NewReferenceMemo(asset, "test memo", "12345", ctx.BlockHeight())
	mgr.Keeper().SetReferenceMemo(ctx, refMemo)

	// Create test transaction IDs
	txID1, _ := common.NewTxID("0000000000000000000000000000000000000000000000000000000000000001")
	txID2, _ := common.NewTxID("0000000000000000000000000000000000000000000000000000000000000002")

	// First usage should succeed
	testTx := common.NewTx(txID1, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:12345")
	memo := fetchMemoFromReference(ctx, mgr, asset, testTx, 16) // tx observed at height 16, memo created at height 15
	c.Check(memo, check.Equals, "test memo", check.Commentf("First usage should succeed"))

	// Verify usage count and transaction tracking
	updatedMemo, err := mgr.Keeper().GetReferenceMemo(ctx, asset, "12345")
	c.Assert(err, check.IsNil)
	c.Check(updatedMemo.GetUsageCount(), check.Equals, int64(1))
	c.Check(updatedMemo.HasBeenUsedBy(txID1), check.Equals, true)

	// Second usage should fail (exceed limit of 1)
	testTx2 := common.NewTx(txID2, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:12345")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx2, 17) // tx observed at height 17, memo created at height 15
	c.Check(memo, check.Equals, "", check.Commentf("Second usage should fail"))

	// Verify usage count is 2 (all attempts are tracked for audit purposes, including failures)
	updatedMemo, err = mgr.Keeper().GetReferenceMemo(ctx, asset, "12345")
	c.Assert(err, check.IsNil)
	c.Check(updatedMemo.GetUsageCount(), check.Equals, int64(2))
	c.Check(updatedMemo.HasBeenUsedBy(txID1), check.Equals, true)
	c.Check(updatedMemo.HasBeenUsedBy(txID2), check.Equals, true, check.Commentf("Failed validation should still be tracked for audit purposes"))

	// Test duplicate tracking - same txID called again should not increase count
	testTx3 := common.NewTx(txID1, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:12345")
	fetchMemoFromReference(ctx, mgr, asset, testTx3, 18) // Call with same txID1 again
	updatedMemo, err = mgr.Keeper().GetReferenceMemo(ctx, asset, "12345")
	c.Assert(err, check.IsNil)
	c.Check(updatedMemo.GetUsageCount(), check.Equals, int64(2), check.Commentf("Duplicate tracking should not increase count"))
}

func (s *HandlerObservedTxInSuite) TestMemoUsageTrackingDisabled(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)
	asset := common.BTCAsset

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)
	// Set max usage limit to 0 (disabled)
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnMaxUse.String(), 0)

	// Create a reference memo
	refMemo := NewReferenceMemo(asset, "test memo", "54321", ctx.BlockHeight())
	mgr.Keeper().SetReferenceMemo(ctx, refMemo)

	// Multiple usages should succeed when limit is disabled (0)
	for i := 0; i < 10; i++ {
		// Track each usage with unique transaction ID
		txID, _ := common.NewTxID(fmt.Sprintf("%064d", i+1))
		testTx := common.NewTx(txID, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:54321")
		memo := fetchMemoFromReference(ctx, mgr, asset, testTx, int64(16+i)) // tx observed at heights 16-25, memo created at height 15
		c.Check(memo, check.Equals, "test memo", check.Commentf("Usage %d should succeed when limit disabled", i+1))
	}

	// Verify usage count was incremented for all transactions
	updatedMemo, err := mgr.Keeper().GetReferenceMemo(ctx, asset, "54321")
	c.Assert(err, check.IsNil)
	c.Check(updatedMemo.GetUsageCount(), check.Equals, int64(10))
}

func (s *HandlerObservedTxInSuite) TestMemolessTransactionReferenceGeneration(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)

	// Setup BTC pool with 8 decimals
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Create observed tx with empty memo and specific amount for predictable reference
	tx := GetRandomTx()
	tx.Memo = "" // Empty memo
	tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))
	obTx := NewObservedTx(tx, 12, GetRandomPubKey(), 15)

	// Setup vault
	vault := GetRandomVault()
	vault.PubKey = obTx.ObservedPubKey
	vault.Status = ActiveVault

	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)
	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	// Create reference memo that matches our expected generated reference
	expectedRef := "56789" // last 5 digits of 123456789
	refMemo := NewReferenceMemo(common.BTCAsset, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890", expectedRef, ctx.BlockHeight())
	mgr.Keeper().SetReferenceMemo(ctx, refMemo)

	// Test that empty memo transaction gets reference memo generated
	handler := NewObservedTxInHandler(mgr)
	msg := NewMsgObservedTxIn(ObservedTxs{obTx}, na.NodeAddress)

	// Process the transaction
	_, err := handler.handle(ctx, *msg)
	c.Assert(err, IsNil)

	voter, err := mgr.Keeper().GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)

	// Verify that the voter's memo was updated with reference memo
	c.Assert(voter.Tx.Tx.Memo, check.Matches, "r:\\d{5}", check.Commentf("Expected reference memo pattern"))
	c.Assert(voter.Tx.Tx.Memo, Equals, "r:"+expectedRef, check.Commentf("Expected specific reference memo"))
}

func (s *HandlerObservedTxInSuite) TestMemolessTransactionWithDifferentAssetDecimals(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)

	// Setup GAIA pool with 6 decimals
	gaiaAsset := common.Asset{Chain: common.GAIAChain, Symbol: "ATOM", Ticker: "ATOM", Synth: false}
	gaiaPool := NewPool()
	gaiaPool.Asset = gaiaAsset
	gaiaPool.Decimals = 6 // GAIA has 6 decimals
	gaiaPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	gaiaPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, gaiaPool), IsNil)

	// Create observed tx with empty memo
	tx := GetRandomTx()
	tx.Memo = "" // Empty memo
	// For 6 decimals, amount should be divided by 100, so 123456780000 / 100 = 1234567800, last 5 = 67800
	tx.Coins = common.NewCoins(common.NewCoin(gaiaAsset, cosmos.NewUint(123456780000)))
	obTx := NewObservedTx(tx, 12, GetRandomPubKey(), 15)

	// Setup vault
	vault := GetRandomVault()
	vault.PubKey = obTx.ObservedPubKey
	vault.Status = ActiveVault
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)
	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	// Create reference memo for expected reference
	expectedRef := "67800" // (123456780000 / 100) % 100000 = 1234567800 % 100000 = 67800
	refMemo := NewReferenceMemo(gaiaAsset, "SWAP:BTC.BTC:bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", expectedRef, ctx.BlockHeight())
	mgr.Keeper().SetReferenceMemo(ctx, refMemo)

	// Process the transaction
	handler := NewObservedTxInHandler(mgr)
	msg := NewMsgObservedTxIn(ObservedTxs{obTx}, na.NodeAddress)
	_, err := handler.handle(ctx, *msg)
	c.Assert(err, IsNil)

	voter, err := mgr.Keeper().GetObservedTxInVoter(ctx, tx.ID)
	c.Assert(err, IsNil)

	// Verify the generated reference memo accounts for decimal precision
	c.Assert(voter.Tx.Tx.Memo, Equals, "r:"+expectedRef, check.Commentf("Expected reference memo adjusted for 6 decimals"))
}

func (s *HandlerObservedTxInSuite) TestMemolessTransactionErrorCases(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Test case 1: Transaction with no coins
	txNoCoin := GetRandomTx()
	txNoCoin.Memo = ""
	txNoCoin.Coins = common.NewCoins() // No coins
	obTxNoCoin := NewObservedTx(txNoCoin, 12, GetRandomPubKey(), 15)

	vault := GetRandomVault()
	vault.PubKey = obTxNoCoin.ObservedPubKey
	vault.Status = ActiveVault

	keeper := &TestObservedTxInHandleKeeper{
		nas:         NodeAccounts{GetRandomValidatorNode(NodeActive)},
		voter:       NewObservedTxVoter(txNoCoin.ID, make(ObservedTxs, 0)),
		vault:       vault,
		vaultExists: true,
	}
	mgr.K = keeper

	// Process transaction with no coins - should not generate reference memo
	handler := NewObservedTxInHandler(mgr)
	msg := NewMsgObservedTxIn(ObservedTxs{obTxNoCoin}, keeper.nas[0].NodeAddress)
	_, err := handler.handle(ctx, *msg)
	c.Assert(err, IsNil)

	// Memo should remain empty since reference generation failed
	c.Assert(keeper.voter.Tx.Tx.Memo, Equals, "", check.Commentf("Memo should remain empty when no coins"))

	// Test case 2: Transaction with zero amount
	txZeroAmount := GetRandomTx()
	txZeroAmount.Memo = ""
	txZeroAmount.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.ZeroUint()))
	obTxZero := NewObservedTx(txZeroAmount, 12, GetRandomPubKey(), 15)

	keeper.voter = NewObservedTxVoter(txZeroAmount.ID, make(ObservedTxs, 0))
	vault.PubKey = obTxZero.ObservedPubKey

	msg = NewMsgObservedTxIn(ObservedTxs{obTxZero}, keeper.nas[0].NodeAddress)
	_, err = handler.handle(ctx, *msg)
	c.Assert(err, IsNil)

	// Memo should remain empty since amount is zero
	c.Assert(keeper.voter.Tx.Tx.Memo, Equals, "", check.Commentf("Memo should remain empty when amount is zero"))
}

func (s *HandlerObservedTxInSuite) TestMemolessTransactionWithExistingMemo(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(15)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Create observed tx with existing memo
	existingMemo := "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890"
	tx := GetRandomTx()
	tx.Memo = existingMemo
	tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))
	obTx := NewObservedTx(tx, 12, GetRandomPubKey(), 15)

	vault := GetRandomVault()
	vault.PubKey = obTx.ObservedPubKey
	vault.Status = ActiveVault

	keeper := &TestObservedTxInHandleKeeper{
		nas:         NodeAccounts{GetRandomValidatorNode(NodeActive)},
		voter:       NewObservedTxVoter(tx.ID, make(ObservedTxs, 0)),
		vault:       vault,
		vaultExists: true,
	}
	mgr.K = keeper

	// Process transaction with existing memo
	handler := NewObservedTxInHandler(mgr)
	msg := NewMsgObservedTxIn(ObservedTxs{obTx}, keeper.nas[0].NodeAddress)
	_, err := handler.handle(ctx, *msg)
	c.Assert(err, IsNil)

	// Memo should remain the original memo (no reference memo generation for non-empty memos)
	c.Assert(keeper.voter.Tx.Tx.Memo, Equals, existingMemo, check.Commentf("Existing memo should be preserved"))
}

func (s *HandlerObservedTxInSuite) TestFetchMemoFromReferenceHeightValidation(c *C) {
	ctx, mgr := setupManagerForTest(c)
	asset := common.BTCAsset

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Test case 1: Transaction observed BEFORE memo creation (should fail)
	refMemo1 := NewReferenceMemo(asset, "test memo 1", "11111", 50) // memo created at height 50
	mgr.Keeper().SetReferenceMemo(ctx, refMemo1)

	testTx1 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:11111")
	memo := fetchMemoFromReference(ctx, mgr, asset, testTx1, 30) // tx observed at height 30 (BEFORE memo creation)
	c.Check(memo, check.Equals, "", check.Commentf("Transaction observed before memo creation should be rejected"))

	// Test case 2: Transaction observed AT SAME HEIGHT as memo creation (should fail for safety)
	refMemo2 := NewReferenceMemo(asset, "test memo 2", "22222", 60) // memo created at height 60
	mgr.Keeper().SetReferenceMemo(ctx, refMemo2)

	testTx2 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:22222")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx2, 60) // tx observed at height 60 (SAME as memo creation)
	c.Check(memo, check.Equals, "", check.Commentf("Transaction observed at same height as memo creation should be rejected"))

	// Test case 3: Transaction observed AFTER memo creation (should succeed)
	refMemo3 := NewReferenceMemo(asset, "test memo 3", "33333", 70) // memo created at height 70
	mgr.Keeper().SetReferenceMemo(ctx, refMemo3)

	ctx = ctx.WithBlockHeight(80) // current height is 80, memo not expired (70 + 100 > 80)
	testTx3 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:33333")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx3, 75) // tx observed at height 75 (AFTER memo creation)
	c.Check(memo, check.Equals, "test memo 3", check.Commentf("Transaction observed after memo creation should succeed"))

	// Test case 4: Transaction observed way after memo creation (should succeed)
	refMemo4 := NewReferenceMemo(asset, "test memo 4", "44444", 10) // memo created at height 10
	mgr.Keeper().SetReferenceMemo(ctx, refMemo4)

	ctx = ctx.WithBlockHeight(50) // current height is 50, memo not expired (10 + 100 > 50)
	testTx4 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:44444")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx4, 45) // tx observed at height 45 (way after memo creation)
	c.Check(memo, check.Equals, "test memo 4", check.Commentf("Transaction observed way after memo creation should succeed"))

	// Test case 5: Edge case - transaction observed 1 block after memo creation (should succeed)
	refMemo5 := NewReferenceMemo(asset, "test memo 5", "55555", 100) // memo created at height 100
	mgr.Keeper().SetReferenceMemo(ctx, refMemo5)

	ctx = ctx.WithBlockHeight(150) // current height is 150, memo not expired (100 + 100 > 150 is false, but we test height validation first)
	testTx5 := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, "r:55555")
	memo = fetchMemoFromReference(ctx, mgr, asset, testTx5, 101) // tx observed at height 101 (1 block after memo creation)
	c.Check(memo, check.Equals, "test memo 5", check.Commentf("Transaction observed 1 block after memo creation should succeed"))
}
