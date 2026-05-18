package thorchain

import (
	"fmt"
	"strings"

	"cosmossdk.io/math"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HelperSuite struct{}

var _ = Suite(&HelperSuite{})

type TestRefundBondKeeper struct {
	keeper.KVStoreDummy
	pool    Pool
	na      NodeAccount
	vaults  Vaults
	modules map[string]int64
	consts  constants.ConstantValues
}

func (k *TestRefundBondKeeper) GetConfigInt64(ctx cosmos.Context, key constants.ConstantName) int64 {
	return k.consts.GetInt64Value(key)
}

func (k *TestRefundBondKeeper) GetAsgardVaultsByStatus(_ cosmos.Context, _ VaultStatus) (Vaults, error) {
	return k.vaults, nil
}

func (k *TestRefundBondKeeper) VaultExists(_ cosmos.Context, pk common.PubKey) bool {
	return true
}

func (k *TestRefundBondKeeper) GetLeastSecure(ctx cosmos.Context, vaults Vaults, signingTransPeriod int64) Vault {
	return vaults[0]
}

func (k *TestRefundBondKeeper) GetPool(_ cosmos.Context, asset common.Asset) (Pool, error) {
	if k.pool.Asset.Equals(asset) {
		return k.pool, nil
	}
	return NewPool(), errKaboom
}

func (k *TestRefundBondKeeper) SetNodeAccount(_ cosmos.Context, na NodeAccount) error {
	k.na = na
	return nil
}

func (k *TestRefundBondKeeper) SetPool(_ cosmos.Context, p Pool) error {
	if k.pool.Asset.Equals(p.Asset) {
		k.pool = p
		return nil
	}
	return errKaboom
}

func (k *TestRefundBondKeeper) SetBondProviders(ctx cosmos.Context, _ BondProviders) error {
	return nil
}

func (k *TestRefundBondKeeper) GetBondProviders(ctx cosmos.Context, add cosmos.AccAddress) (BondProviders, error) {
	return BondProviders{}, nil
}

func (k *TestRefundBondKeeper) SendFromModuleToModule(_ cosmos.Context, from, to string, coins common.Coins) error {
	k.modules[from] -= int64(coins[0].Amount.Uint64())
	k.modules[to] += int64(coins[0].Amount.Uint64())
	return nil
}

func (k *TestRefundBondKeeper) SendFromModuleToAccount(_ cosmos.Context, from string, _ cosmos.AccAddress, coins common.Coins) error {
	k.modules[from] -= int64(coins[0].Amount.Uint64())
	return nil
}

func (s *HelperSuite) TestRefundBondHappyPath(c *C) {
	ctx, _ := setupKeeperForTest(c)
	na := GetRandomValidatorNode(NodeActive)
	na.Bond = cosmos.NewUint(12098 * common.One)
	pk := GetRandomPubKey()
	na.PubKeySet.Secp256k1 = pk
	keeper := &TestRefundBondKeeper{
		modules: make(map[string]int64),
		consts:  constants.GetConstantValues(GetCurrentVersion()),
	}
	na.Status = NodeStandby
	mgr := NewDummyMgrWithKeeper(keeper)
	tx := GetRandomTx()
	tx.FromAddress, _ = common.NewAddress(na.BondAddress.String())
	err := refundBond(ctx, tx, na.NodeAddress, cosmos.ZeroUint(), &na, mgr)
	c.Assert(err, IsNil)
	items, err := mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 0)
}

func (s *HelperSuite) TestRefundBondDisableRequestToLeaveNode(c *C) {
	ctx, _ := setupKeeperForTest(c)
	na := GetRandomValidatorNode(NodeActive)
	na.Bond = cosmos.NewUint(12098 * common.One)
	pk := GetRandomPubKey()
	na.PubKeySet.Secp256k1 = pk
	keeper := &TestRefundBondKeeper{
		modules: make(map[string]int64),
		consts:  constants.GetConstantValues(GetCurrentVersion()),
	}
	na.Status = NodeStandby
	na.RequestedToLeave = true
	mgr := NewDummyMgrWithKeeper(keeper)
	tx := GetRandomTx()
	err := refundBond(ctx, tx, na.NodeAddress, cosmos.ZeroUint(), &na, mgr)
	c.Assert(err, IsNil)
	items, err := mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 0)
	c.Assert(err, IsNil)
	c.Assert(keeper.na.Status == NodeDisabled, Equals, true)
}

func (s *HelperSuite) TestDollarsPerRune(c *C) {
	ctx, k := setupKeeperForTest(c)
	mgr := NewDummyMgrWithKeeper(k)
	mgr.Keeper().SetMimir(ctx, "TorAnchor-ETH-BUSD-BD1", 1) // enable BUSD pool as a TOR anchor
	busd, err := common.NewAsset("ETH.BUSD-BD1")
	c.Assert(err, IsNil)
	pool := NewPool()
	pool.Asset = busd
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(85515078103667)
	pool.BalanceAsset = cosmos.NewUint(709802235538353)
	pool.Decimals = 8
	c.Assert(k.SetPool(ctx, pool), IsNil)

	runeUSDPrice := telem(mgr.Keeper().DollarsPerRune(ctx))
	c.Assert(runeUSDPrice, Equals, float32(8.300317))

	// Now try with a second pool, identical depths.
	mgr.Keeper().SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1) // enable USDC pool as a TOR anchor
	usdc, err := common.NewAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")
	c.Assert(err, IsNil)
	pool = NewPool()
	pool.Asset = usdc
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(85515078103667)
	pool.BalanceAsset = cosmos.NewUint(709802235538353)
	pool.Decimals = 8
	c.Assert(k.SetPool(ctx, pool), IsNil)

	runeUSDPrice = telem(mgr.Keeper().DollarsPerRune(ctx))
	c.Assert(runeUSDPrice, Equals, float32(8.300317))
}

func (s *HelperSuite) TestTelem(c *C) {
	value := cosmos.NewUint(12047733)
	c.Assert(value.Uint64(), Equals, uint64(12047733))
	c.Assert(telem(value), Equals, float32(0.12047733))
}

type addGasFeesKeeperHelper struct {
	keeper.Keeper
	errGetNetwork bool
	errSetNetwork bool
	errGetPool    bool
	errSetPool    bool
}

func newAddGasFeesKeeperHelper(keeper keeper.Keeper) *addGasFeesKeeperHelper {
	return &addGasFeesKeeperHelper{
		Keeper: keeper,
	}
}

func (h *addGasFeesKeeperHelper) GetNetwork(ctx cosmos.Context) (Network, error) {
	if h.errGetNetwork {
		return Network{}, errKaboom
	}
	return h.Keeper.GetNetwork(ctx)
}

func (h *addGasFeesKeeperHelper) SetNetwork(ctx cosmos.Context, data Network) error {
	if h.errSetNetwork {
		return errKaboom
	}
	return h.Keeper.SetNetwork(ctx, data)
}

func (h *addGasFeesKeeperHelper) SetPool(ctx cosmos.Context, pool Pool) error {
	if h.errSetPool {
		return errKaboom
	}
	return h.Keeper.SetPool(ctx, pool)
}

func (h *addGasFeesKeeperHelper) GetPool(ctx cosmos.Context, asset common.Asset) (Pool, error) {
	if h.errGetPool {
		return Pool{}, errKaboom
	}
	return h.Keeper.GetPool(ctx, asset)
}

type addGasFeeTestHelper struct {
	ctx cosmos.Context
	na  NodeAccount
	mgr Manager
}

func newAddGasFeeTestHelper(c *C) addGasFeeTestHelper {
	ctx, mgr := setupManagerForTest(c)
	keeper := newAddGasFeesKeeperHelper(mgr.Keeper())
	mgr.K = keeper
	pool := NewPool()
	pool.Asset = common.ETHAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	poolBTC := NewPool()
	poolBTC.Asset = common.BTCAsset
	poolBTC.BalanceAsset = cosmos.NewUint(100 * common.One)
	poolBTC.BalanceRune = cosmos.NewUint(100 * common.One)
	poolBTC.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, poolBTC), IsNil)

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, na.PubKeySet.Secp256k1, common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	// TODO:  Perhaps make this vault entirely unrelated to the NodeAccount pubkey, such as with an addGasFeeTestHelper 'vault' field.
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)
	version := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(version)
	mgr.gasMgr = newGasMgr(constAccessor, keeper)
	return addGasFeeTestHelper{
		ctx: ctx,
		mgr: mgr,
		na:  na,
	}
}

func (s *HelperSuite) TestAddGasFees(c *C) {
	testCases := []struct {
		name        string
		txCreator   func(helper addGasFeeTestHelper) ObservedTx
		runner      func(helper addGasFeeTestHelper, tx ObservedTx) error
		expectError bool
		validator   func(helper addGasFeeTestHelper, c *C)
	}{
		{
			name: "empty Gas should just return nil",
			txCreator: func(helper addGasFeeTestHelper) ObservedTx {
				return GetRandomObservedTx()
			},

			expectError: false,
		},
		{
			name: "normal ETH gas",
			txCreator: func(helper addGasFeeTestHelper) ObservedTx {
				tx := ObservedTx{
					Tx: common.Tx{
						ID:          GetRandomTxHash(),
						Chain:       common.ETHChain,
						FromAddress: GetRandomETHAddress(),
						ToAddress:   GetRandomETHAddress(),
						Coins: common.Coins{
							common.NewCoin(common.ETHAsset, cosmos.NewUint(5*common.One)),
							common.NewCoin(common.RuneAsset(), cosmos.NewUint(8*common.One)),
						},
						Gas: common.Gas{
							common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
						},
						Memo: "",
					},
					Status:         common.Status_done,
					OutHashes:      nil,
					BlockHeight:    helper.ctx.BlockHeight(),
					Signers:        []string{helper.na.NodeAddress.String()},
					ObservedPubKey: helper.na.PubKeySet.Secp256k1,
				}
				return tx
			},
			runner: func(helper addGasFeeTestHelper, tx ObservedTx) error {
				return addGasFees(helper.ctx, helper.mgr, tx)
			},
			expectError: false,
			validator: func(helper addGasFeeTestHelper, c *C) {
				expected := common.NewCoin(common.ETHAsset, cosmos.NewUint(10000))
				c.Assert(helper.mgr.GasMgr().GetGas(), HasLen, 1)
				c.Assert(helper.mgr.GasMgr().GetGas()[0].Equals(expected), Equals, true)
			},
		},
		{
			name: "normal BTC gas",
			txCreator: func(helper addGasFeeTestHelper) ObservedTx {
				tx := ObservedTx{
					Tx: common.Tx{
						ID:          GetRandomTxHash(),
						Chain:       common.BTCChain,
						FromAddress: GetRandomBTCAddress(),
						ToAddress:   GetRandomBTCAddress(),
						Coins: common.Coins{
							common.NewCoin(common.BTCAsset, cosmos.NewUint(5*common.One)),
						},
						Gas: common.Gas{
							common.NewCoin(common.BTCAsset, cosmos.NewUint(2000)),
						},
						Memo: "",
					},
					Status:         common.Status_done,
					OutHashes:      nil,
					BlockHeight:    helper.ctx.BlockHeight(),
					Signers:        []string{helper.na.NodeAddress.String()},
					ObservedPubKey: helper.na.PubKeySet.Secp256k1,
				}
				return tx
			},
			runner: func(helper addGasFeeTestHelper, tx ObservedTx) error {
				return addGasFees(helper.ctx, helper.mgr, tx)
			},
			expectError: false,
			validator: func(helper addGasFeeTestHelper, c *C) {
				expected := common.NewCoin(common.BTCAsset, cosmos.NewUint(2000))
				c.Assert(helper.mgr.GasMgr().GetGas(), HasLen, 1)
				c.Assert(helper.mgr.GasMgr().GetGas()[0].Equals(expected), Equals, true)
			},
		},
	}
	for _, tc := range testCases {
		helper := newAddGasFeeTestHelper(c)
		tx := tc.txCreator(helper)
		var err error
		if tc.runner == nil {
			err = addGasFees(helper.ctx, helper.mgr, tx)
		} else {
			err = tc.runner(helper, tx)
		}

		if err != nil && !tc.expectError {
			c.Errorf("test case: %s,didn't expect error however it got : %s", tc.name, err)
			c.FailNow()
		}
		if err == nil && tc.expectError {
			c.Errorf("test case: %s, expect error however it didn't", tc.name)
			c.FailNow()
		}
		if !tc.expectError && tc.validator != nil {
			tc.validator(helper, c)
			continue
		}
	}
}

func (s *HelperSuite) TestEmitPoolStageCostEvent(c *C) {
	ctx, mgr := setupManagerForTest(c)
	emitPoolBalanceChangedEvent(ctx,
		NewPoolMod(common.BTCAsset, cosmos.NewUint(1000), false, cosmos.ZeroUint(), false), "test", mgr)
	found := false
	for _, e := range ctx.EventManager().Events() {
		if strings.EqualFold(e.Type, types.PoolBalanceChangeEventType) {
			found = true
			break
		}
	}
	c.Assert(found, Equals, true)
}

func (s *HelperSuite) TestIsSynthMintPause(c *C) {
	ctx, mgr := setupManagerForTest(c)

	mgr.Keeper().SetMimir(ctx, constants.MaxSynthPerPoolDepth.String(), 1500)

	pool := types.Pool{
		Asset:        common.BTCAsset,
		BalanceAsset: cosmos.NewUint(100 * common.One),
		BalanceRune:  cosmos.NewUint(100 * common.One),
	}
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	coins := cosmos.NewCoins(cosmos.NewCoin("btc/btc", cosmos.NewInt(29*common.One))) // 29% utilization
	c.Assert(mgr.coinKeeper.MintCoins(ctx, ModuleName, coins), IsNil)

	c.Assert(isSynthMintPaused(ctx, mgr, common.BTCAsset, cosmos.ZeroUint()), IsNil)

	// A swap that outputs 0.5 synth BTC would not surpass the synth utilization cap (29% -> 29.5%)
	c.Assert(isSynthMintPaused(ctx, mgr, common.BTCAsset, cosmos.NewUint(0.5*common.One)), IsNil)
	// A swap that outputs 1 synth BTC would not surpass the synth utilization cap (29% -> 30%)
	c.Assert(isSynthMintPaused(ctx, mgr, common.BTCAsset, cosmos.NewUint(1*common.One)), IsNil)
	// A swap that outputs 1.1 synth BTC would surpass the synth utilization cap (29% -> 30.1%)
	c.Assert(isSynthMintPaused(ctx, mgr, common.BTCAsset, cosmos.NewUint(1.1*common.One)), NotNil)

	coins = cosmos.NewCoins(cosmos.NewCoin("btc/btc", cosmos.NewInt(1*common.One))) // 30% utilization
	c.Assert(mgr.coinKeeper.MintCoins(ctx, ModuleName, coins), IsNil)

	c.Assert(isSynthMintPaused(ctx, mgr, common.BTCAsset, cosmos.ZeroUint()), IsNil)

	coins = cosmos.NewCoins(cosmos.NewCoin("btc/btc", cosmos.NewInt(1*common.One))) // 31% utilization
	c.Assert(mgr.coinKeeper.MintCoins(ctx, ModuleName, coins), IsNil)

	c.Assert(isSynthMintPaused(ctx, mgr, common.BTCAsset, cosmos.ZeroUint()), NotNil)
}

func (s *HelperSuite) TestUpdateTxOutGas(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Create ObservedVoter and add a TxOut
	txVoter := GetRandomObservedTxVoter()
	txOut := GetRandomTxOutItem()
	txVoter.Actions = append(txVoter.Actions, txOut)
	mgr.Keeper().SetObservedTxInVoter(ctx, txVoter)

	// Try to set new gas, should return error as TxOut InHash doesn't match
	newGas := common.Gas{common.NewCoin(common.DOGEAsset, cosmos.NewUint(2000000))}
	err := updateTxOutGas(ctx, mgr.K, txOut, newGas)
	c.Assert(err.Error(), Equals, fmt.Sprintf("fail to find tx out in ObservedTxVoter %s", txOut.InHash))

	// Update TxOut InHash to match, should update gas
	txOut.InHash = txVoter.TxID
	txVoter.Actions[1] = txOut
	mgr.Keeper().SetObservedTxInVoter(ctx, txVoter)

	// Err should be Nil
	err = updateTxOutGas(ctx, mgr.K, txOut, newGas)
	c.Assert(err, IsNil)

	// Keeper should have updated gas of TxOut in Actions
	txVoter, err = mgr.Keeper().GetObservedTxInVoter(ctx, txVoter.TxID)
	c.Assert(err, IsNil)

	didUpdate := false
	for _, item := range txVoter.Actions {
		if item.Equals(txOut) && item.MaxGas.Equals(newGas) {
			didUpdate = true
			break
		}
	}

	c.Assert(didUpdate, Equals, true)
}

func (s *HelperSuite) TestUpdateTxOutGasRate(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Create ObservedVoter and add a TxOut
	txVoter := GetRandomObservedTxVoter()
	txOut := GetRandomTxOutItem()
	txVoter.Actions = append(txVoter.Actions, txOut)
	mgr.Keeper().SetObservedTxInVoter(ctx, txVoter)

	// Try to set new gas rate, should return error as TxOut InHash doesn't match
	newGasRate := int64(25)
	err := updateTxOutGasRate(ctx, mgr.K, txOut, newGasRate)
	c.Assert(err.Error(), Equals, fmt.Sprintf("fail to find tx out in ObservedTxVoter %s", txOut.InHash))

	// Update TxOut InHash to match, should update gas
	txOut.InHash = txVoter.TxID
	txVoter.Actions[1] = txOut
	mgr.Keeper().SetObservedTxInVoter(ctx, txVoter)

	// Err should be Nil
	err = updateTxOutGasRate(ctx, mgr.K, txOut, newGasRate)
	c.Assert(err, IsNil)

	// Now that the actions have been updated (dependent on Equals which checks GasRate),
	// update the GasRate in the outbound queue item.
	txOut.GasRate = newGasRate

	// Keeper should have updated gas of TxOut in Actions
	txVoter, err = mgr.Keeper().GetObservedTxInVoter(ctx, txVoter.TxID)
	c.Assert(err, IsNil)

	didUpdate := false
	for _, item := range txVoter.Actions {
		if item.Equals(txOut) && item.GasRate == newGasRate {
			didUpdate = true
			break
		}
	}

	c.Assert(didUpdate, Equals, true)
}

func (s *HelperSuite) TestPOLPoolValue(c *C) {
	ctx, mgr := setupManagerForTest(c)

	polAddress, err := mgr.Keeper().GetModuleAddress(ReserveName)
	c.Assert(err, IsNil)

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(2000 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(20 * common.One)
	btcPool.LPUnits = cosmos.NewUint(1600)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	coin := common.NewCoin(common.BTCAsset.GetSyntheticAsset(), cosmos.NewUint(10*common.One))
	c.Assert(mgr.Keeper().MintToModule(ctx, ModuleName, coin), IsNil)

	lps := LiquidityProviders{
		{
			Asset:             btcPool.Asset,
			RuneAddress:       GetRandomRUNEAddress(),
			AssetAddress:      GetRandomBTCAddress(),
			LastAddHeight:     5,
			Units:             btcPool.LPUnits.QuoUint64(2),
			PendingRune:       cosmos.ZeroUint(),
			PendingAsset:      cosmos.ZeroUint(),
			AssetDepositValue: cosmos.ZeroUint(),
			RuneDepositValue:  cosmos.ZeroUint(),
		},
		{
			Asset:             btcPool.Asset,
			RuneAddress:       polAddress,
			AssetAddress:      common.NoAddress,
			LastAddHeight:     10,
			Units:             btcPool.LPUnits.QuoUint64(2),
			PendingRune:       cosmos.ZeroUint(),
			PendingAsset:      cosmos.ZeroUint(),
			AssetDepositValue: cosmos.ZeroUint(),
			RuneDepositValue:  cosmos.ZeroUint(),
		},
	}
	for _, lp := range lps {
		mgr.Keeper().SetLiquidityProvider(ctx, lp)
	}

	value, err := polPoolValue(ctx, mgr)
	c.Assert(err, IsNil)
	c.Check(value.Uint64(), Equals, uint64(150023441162), Commentf("%d", value.Uint64()))
}

// This including the test of getTotalEffectiveBond.
func (s *HelperSuite) TestSecurityBond(c *C) {
	nas := make(NodeAccounts, 0)
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(0), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ := getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(0), Commentf("%d", totalEffectiveBond.Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
	}
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(10), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ = getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(10), Commentf("%d", totalEffectiveBond.Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
	}
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(30), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ = getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(30), Commentf("%d", totalEffectiveBond.Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
	}
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(30), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ = getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(50), Commentf("%d", totalEffectiveBond.Uint64()))
	// Only 20 of the top-bond's node is effective.

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
		NodeAccount{Bond: cosmos.NewUint(40)},
	}
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(60), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ = getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(90), Commentf("%d", totalEffectiveBond.Uint64()))
	// Only 30 of the top-bond's node is effective.

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
		NodeAccount{Bond: cosmos.NewUint(40)},
		NodeAccount{Bond: cosmos.NewUint(50)},
	}
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(100), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ = getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(140), Commentf("%d", totalEffectiveBond.Uint64()))
	// Only 40 of the top-bond's node is effective.

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
		NodeAccount{Bond: cosmos.NewUint(40)},
		NodeAccount{Bond: cosmos.NewUint(50)},
		NodeAccount{Bond: cosmos.NewUint(60)},
	}
	c.Assert(getEffectiveSecurityBond(nas).Uint64(), Equals, uint64(100), Commentf("%d", getEffectiveSecurityBond(nas).Uint64()))
	totalEffectiveBond, _ = getTotalEffectiveBond(nas)
	c.Assert(totalEffectiveBond.Uint64(), Equals, uint64(180), Commentf("%d", totalEffectiveBond.Uint64()))
	// Only 40 each of the top-bonds two nodes is effective.
}

func (s *HelperSuite) TestGetHardBondCap(c *C) {
	nas := make(NodeAccounts, 0)
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(0), Commentf("%d", getHardBondCap(nas).Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
	}
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(10), Commentf("%d", getHardBondCap(nas).Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
	}
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(20), Commentf("%d", getHardBondCap(nas).Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
	}
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(20), Commentf("%d", getHardBondCap(nas).Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
		NodeAccount{Bond: cosmos.NewUint(40)},
	}
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(30), Commentf("%d", getHardBondCap(nas).Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
		NodeAccount{Bond: cosmos.NewUint(40)},
		NodeAccount{Bond: cosmos.NewUint(50)},
	}
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(40), Commentf("%d", getHardBondCap(nas).Uint64()))

	nas = NodeAccounts{
		NodeAccount{Bond: cosmos.NewUint(10)},
		NodeAccount{Bond: cosmos.NewUint(20)},
		NodeAccount{Bond: cosmos.NewUint(30)},
		NodeAccount{Bond: cosmos.NewUint(40)},
		NodeAccount{Bond: cosmos.NewUint(50)},
		NodeAccount{Bond: cosmos.NewUint(60)},
	}
	c.Assert(getHardBondCap(nas).Uint64(), Equals, uint64(40), Commentf("%d", getHardBondCap(nas).Uint64()))
}

func (s *HelperSuite) TestIsTronZeroGasTx(c *C) {
	testCases := []struct {
		Chain  common.Chain
		Asset  common.Asset
		Amount uint64
		Result bool
	}{
		{
			Chain:  common.THORChain,
			Asset:  common.TCY,
			Amount: 1,
			Result: false,
		},
		{
			Chain:  common.ETHChain,
			Asset:  common.ETHAsset,
			Amount: 123456789,
			Result: false,
		},
		{
			Chain:  common.TRONChain,
			Asset:  common.ETHAsset,
			Amount: 1,
			Result: false,
		},
		{
			Chain:  common.TRONChain,
			Asset:  common.TRXAsset,
			Amount: 2,
			Result: false,
		},
		{
			Chain:  common.TRONChain,
			Asset:  common.TRXAsset,
			Amount: 1,
			Result: true,
		},
	}

	for _, tc := range testCases {
		coin := common.Coin{Asset: tc.Asset, Amount: math.NewUint(tc.Amount)}

		tx := GetRandomTx()
		tx.Chain = tc.Chain
		tx.Gas = common.Gas{}.Add(coin)

		obsTx := NewObservedTx(tx, 7, GetRandomPubKey(), 3)

		c.Assert(isTronZeroGasTx(obsTx), Equals, tc.Result)
	}
}

func (HandlerSuite) TestIsSignedByActiveNodeAccounts(c *C) {
	ctx, mgr := setupManagerForTest(c)

	r := isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{})
	c.Check(r, Equals, false,
		Commentf("empty signers should return false"))

	nodeAddr := GetRandomBech32Addr()
	r = isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{nodeAddr})
	c.Check(r, Equals, false,
		Commentf("empty node account should return false"))

	nodeAccount1 := GetRandomValidatorNode(NodeWhiteListed)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, nodeAccount1), IsNil)
	r = isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{nodeAccount1.NodeAddress})
	c.Check(r, Equals, false,
		Commentf("non-active node account should return false"))

	nodeAccount1.Status = NodeActive
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, nodeAccount1), IsNil)
	r = isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{nodeAccount1.NodeAddress})
	c.Check(r, Equals, true,
		Commentf("active node account should return true"))

	r = isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{nodeAccount1.NodeAddress, nodeAddr})
	c.Check(r, Equals, false,
		Commentf("should return false if any signer is not an active validator"))

	nodeAccount1.Type = NodeTypeVault
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, nodeAccount1), IsNil)
	r = isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{nodeAccount1.NodeAddress})
	c.Check(r, Equals, false,
		Commentf("non-validator node should return false"))

	asgardAddr := mgr.Keeper().GetModuleAccAddress(AsgardName)
	r = isSignedByActiveNodeAccounts(ctx, mgr.Keeper(), []cosmos.AccAddress{asgardAddr})
	c.Check(r, Equals, true,
		Commentf("asgard module address should return true"))
}

func (HandlerSuite) TestWillSwapSucceed(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Set up some pools
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(100_000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	pool2 := NewPool()
	pool2.Asset = common.ETHAsset
	pool2.Status = PoolAvailable
	pool2.BalanceRune = cosmos.NewUint(100_000 * common.One)
	pool2.BalanceAsset = cosmos.NewUint(1000 * common.One)
	pool2.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool2), IsNil)

	// Set Network fees
	networkFee := NewNetworkFee(common.ETHChain, 1, 1000)
	c.Assert(mgr.Keeper().SaveNetworkFee(ctx, common.ETHChain, networkFee), IsNil)

	networkFee = NewNetworkFee(common.BTCChain, 1000, 10)
	c.Assert(mgr.Keeper().SaveNetworkFee(ctx, common.BTCChain, networkFee), IsNil)

	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(common.One))},
		common.Gas{
			{Asset: common.BTCAsset, Amount: cosmos.NewUint(37500)},
		},
		"",
	)

	// swap from BTC to ETH

	// no limit, should succeed
	msg := NewMsgSwap(tx, common.ETHAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, true)

	// no limit, but small swap, should fail
	tx.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1))}
	msg = NewMsgSwap(tx, common.ETHAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, false)

	// limit too high, should fail
	tx.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(common.One))}
	msg = NewMsgSwap(tx, common.ETHAsset, GetRandomBTCAddress(), cosmos.NewUint(100*common.One), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, false)

	// limit not too high, should succeed
	tx.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(common.One))}
	msg = NewMsgSwap(tx, common.ETHAsset, GetRandomBTCAddress(), cosmos.NewUint(1*common.One), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, true)

	runeTx := common.NewTx(
		GetRandomTxHash(),
		GetRandomTHORAddress(),
		GetRandomTHORAddress(),
		common.Coins{common.NewCoin(common.RuneNative, cosmos.NewUint(common.One*50))},
		common.Gas{
			{Asset: common.RuneNative, Amount: cosmos.NewUint(20000)},
		},
		"",
	)

	// swaps from RUNE

	// swap from RUNE no limit, should succeed
	msg = NewMsgSwap(runeTx, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, true)

	// swap from RUNE, no limit, but small swap, should fail
	runeTx.Coins = common.Coins{common.NewCoin(common.RuneNative, cosmos.NewUint(1))}
	msg = NewMsgSwap(runeTx, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, false)

	// swap from RUNE, limit too high, should fail
	runeTx.Coins = common.Coins{common.NewCoin(common.RuneNative, cosmos.NewUint(common.One*50))}
	msg = NewMsgSwap(runeTx, common.BTCAsset, GetRandomBTCAddress(), cosmos.NewUint(100*common.One), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, false)

	// swap from RUNE, limit not too high, should succeed
	msg = NewMsgSwap(runeTx, common.BTCAsset, GetRandomBTCAddress(), cosmos.NewUint(0.01*common.One), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, true)

	// swaps to RUNE

	// swap to RUNE, no limit, should succeed
	msg = NewMsgSwap(tx, common.RuneNative, GetRandomTHORAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, true)

	// swap to RUNE, limit too high, should fail
	tx.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(common.One))}
	msg = NewMsgSwap(tx, common.RuneNative, GetRandomTHORAddress(), cosmos.NewUint(100_000*common.One), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, false)

	// swap to RUNE, limit not too high, should succeed
	tx.Coins = common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(common.One))}
	msg = NewMsgSwap(tx, common.RuneNative, GetRandomTHORAddress(), cosmos.NewUint(1*common.One), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(willSwapOutputExceedLimitAndFees(ctx, mgr, *msg), Equals, true)
}

func (HandlerSuite) TestNewSwapMemo(c *C) {
	ctx, mgr := setupManagerForTest(c)
	addr := GetRandomBTCAddress()
	memo := NewSwapMemo(ctx, mgr, common.BTCAsset, addr, cosmos.ZeroUint(), "test", cosmos.ZeroUint())
	c.Assert(memo, Equals, fmt.Sprintf("=:BTC.BTC:%s:0:test:0", addr.String()))

	memo = NewSwapMemo(ctx, mgr, common.BTCAsset, addr, cosmos.NewUint(100), "test", cosmos.NewUint(50))
	c.Assert(memo, Equals, fmt.Sprintf("=:BTC.BTC:%s:100:test:50", addr.String()))

	addr = GetRandomTHORAddress()
	memo = NewSwapMemo(ctx, mgr, common.RuneNative, addr, cosmos.NewUint(0), "", cosmos.NewUint(0))
	c.Assert(memo, Equals, fmt.Sprintf("=:THOR.RUNE:%s:0::0", addr.String()))
}

func (HandlerSuite) TestIsPeriodLastBlock(c *C) {
	ctx, _ := setupManagerForTest(c)
	var blockVar int64

	blockVar = 10
	ctx = ctx.WithBlockHeight(10)
	result := IsPeriodLastBlock(ctx, blockVar)
	c.Assert(result, Equals, true)

	blockVar = 100
	ctx = ctx.WithBlockHeight(100)
	result = IsPeriodLastBlock(ctx, blockVar)
	c.Assert(result, Equals, true)

	blockVar = 90
	ctx = ctx.WithBlockHeight(89)
	result = IsPeriodLastBlock(ctx, blockVar)
	c.Assert(result, Equals, false)

	blockVar = 100
	ctx = ctx.WithBlockHeight(101)
	result = IsPeriodLastBlock(ctx, blockVar)
	c.Assert(result, Equals, false)
}

type SettleSwapTestKeeper struct {
	keeper.KVStoreDummy
	advSwapItems  map[string]types.MsgSwap
	failRemove    bool
	voter         ObservedTxVoter
	pools         map[common.Asset]Pool
	overrideVault bool
	vaultStatus   VaultStatus
	vaultCoins    common.Coins
}

func (k *SettleSwapTestKeeper) GetObservedTxInVoter(_ cosmos.Context, txID common.TxID) (ObservedTxVoter, error) {
	if !k.voter.Tx.IsEmpty() {
		return k.voter, nil
	}
	return ObservedTxVoter{}, fmt.Errorf("voter not found")
}

func (k *SettleSwapTestKeeper) RemoveAdvSwapQueueIndex(_ cosmos.Context, msg types.MsgSwap) error {
	if k.failRemove {
		return fmt.Errorf("failed to remove from index")
	}
	return nil
}

func (k *SettleSwapTestKeeper) RemoveAdvSwapQueueItem(_ cosmos.Context, txID common.TxID, index int) error {
	if k.failRemove {
		return fmt.Errorf("failed to remove from queue")
	}
	delete(k.advSwapItems, txID.String())
	return nil
}

func (k *SettleSwapTestKeeper) GetPool(_ cosmos.Context, asset common.Asset) (Pool, error) {
	if pool, ok := k.pools[asset.GetLayer1Asset()]; ok {
		return pool, nil
	}
	return Pool{}, fmt.Errorf("pool not found: %s", asset)
}

func (k *SettleSwapTestKeeper) GetVault(_ cosmos.Context, pk common.PubKey) (Vault, error) {
	if k.overrideVault {
		vault := NewVault(0, k.vaultStatus, AsgardVault, pk, common.Chains{common.BTCChain, common.ETHChain}.Strings(), nil)
		vault.Coins = k.vaultCoins
		return vault, nil
	}
	// Return a dummy vault for testing
	vault := NewVault(0, ActiveVault, AsgardVault, pk, common.Chains{common.BTCChain, common.ETHChain}.Strings(), nil)
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(10*common.One)),
		common.NewCoin(common.ETHAsset, cosmos.NewUint(10*common.One)),
	}
	return vault, nil
}

func (k *SettleSwapTestKeeper) GetTHORNameIterator(ctx cosmos.Context) cosmos.Iterator {
	return nil
}

func (k *SettleSwapTestKeeper) GetTHORName(ctx cosmos.Context, _ string) (THORName, error) {
	return THORName{}, fmt.Errorf("thorname not found")
}

type SettleSwapRetryVaultErrorKeeper struct {
	keeper.Keeper
}

func (k *SettleSwapRetryVaultErrorKeeper) GetVault(_ cosmos.Context, _ common.PubKey) (Vault, error) {
	return Vault{}, fmt.Errorf("vault not found")
}

func seedSettleSwapRetryState(c *C, ctx cosmos.Context, k keeper.Keeper, tx common.Tx, observedPubKey common.PubKey) {
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, btcPool), IsNil)

	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceRune = cosmos.NewUint(100 * common.One)
	ethPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, ethPool), IsNil)

	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.BTCChain, common.ETHChain}.Strings(), nil)
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(k.SetVault(ctx, vault), IsNil)

	voter := NewObservedTxVoter(tx.ID, nil)
	voter.Tx = ObservedTx{
		Tx:             tx,
		ObservedPubKey: observedPubKey,
	}
	k.SetObservedTxInVoter(ctx, voter)
}

func (HelperSuite) TestSettleSwap(c *C) {
	ctx, mgr := setupManagerForTest(c)
	// Test 1: Successful swap settlement with outbound
	txID := GetRandomTxHash()

	// Create a pool for BTC to avoid refund errors
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable

	keeper := &SettleSwapTestKeeper{
		advSwapItems: make(map[string]types.MsgSwap),
		pools:        make(map[common.Asset]Pool),
		voter: ObservedTxVoter{
			Tx: ObservedTx{
				Tx: common.Tx{
					ID: txID,
				},
				ObservedPubKey: GetRandomPubKey(),
			},
		},
	}
	keeper.pools[common.BTCAsset] = btcPool
	mgr.K = keeper
	mgr.txOutStore = NewTxStoreDummy()
	ethAddr := GetRandomETHAddress()
	tx := common.NewTx(
		txID,
		ethAddr,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1))},
		"SWAP:ETH.ETH",
	)
	msg := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.ETHAsset,
		Destination: ethAddr,
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.NewUint(1000), // Full deposit used
			Out:     cosmos.NewUint(750),
		},
	}

	err := settleSwap(ctx, mgr, msg, "test settlement")
	c.Assert(err, IsNil)

	// Verify outbound was scheduled
	outItems, err := mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(len(outItems), Equals, 1)
	c.Assert(outItems[0].Coin.Amount.Equal(cosmos.NewUint(750)), Equals, true)
	c.Assert(outItems[0].Coin.Asset.Equals(common.ETHAsset), Equals, true)

	// Test 2: Settlement with refund of remaining deposit
	mgr.TxOutStore().ClearOutboundItems(ctx)
	msg2 := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.ETHAsset,
		Destination: ethAddr,
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.NewUint(600), // Only 600 used
			Out:     cosmos.NewUint(550),
		},
	}

	err = settleSwap(ctx, mgr, msg2, "partial swap settlement")
	c.Assert(err, IsNil)

	// Verify both outbound and refund were scheduled
	outItems, err = mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(len(outItems), Equals, 2) // One for swap out, one for refund

	// Test 3: Settlement with aggregator
	mgr.TxOutStore().ClearOutboundItems(ctx)
	aggTargetLimit := cosmos.NewUint(100)
	msg3 := types.MsgSwap{
		Tx:                      tx,
		TargetAsset:             common.ETHAsset,
		Destination:             ethAddr,
		Aggregator:              "0x69800327b38A4CeF30367Dec3f64c2f2386f3848",
		AggregatorTargetAddress: "0x1234567890123456789012345678901234567890",
		AggregatorTargetLimit:   &aggTargetLimit,
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.NewUint(1000),
			Out:     cosmos.NewUint(900),
		},
	}

	err = settleSwap(ctx, mgr, msg3, "swap with aggregator")
	c.Assert(err, IsNil)

	outItems, err = mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(len(outItems), Equals, 1)
	c.Assert(outItems[0].Aggregator, Equals, "0x69800327b38A4CeF30367Dec3f64c2f2386f3848")
	c.Assert(outItems[0].AggregatorTargetAsset, Equals, "0x1234567890123456789012345678901234567890")

	// Test 4: Settlement with no outbound (all deposit refunded)
	mgr.TxOutStore().ClearOutboundItems(ctx)
	msg4 := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.ETHAsset,
		Destination: ethAddr,
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.ZeroUint(), // Nothing swapped
			Out:     cosmos.ZeroUint(),
		},
	}

	err = settleSwap(ctx, mgr, msg4, "no swap occurred")
	c.Assert(err, IsNil)

	// Only refund should be scheduled
	outItems, err = mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(len(outItems), Equals, 1)
	c.Assert(outItems[0].Coin.Amount.Equal(cosmos.NewUint(1000)), Equals, true)
	c.Assert(outItems[0].Coin.Asset.Equals(common.BTCAsset), Equals, true) // Refund in original asset

	// Test 5: Settlement with savers add memo (should not schedule outbound)
	mgr.TxOutStore().ClearOutboundItems(ctx)
	saversTx := common.NewTx(
		GetRandomTxHash(),
		ethAddr,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(1000))},
		common.Gas{common.NewCoin(common.RuneAsset(), cosmos.NewUint(1))},
		"+:BTC/BTC", // Add liquidity memo (savers)
	)
	msg5 := types.MsgSwap{
		Tx:          saversTx,
		TargetAsset: common.BTCAsset,
		Destination: GetRandomBTCAddress(),
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.NewUint(1000),
			Out:     cosmos.NewUint(900),
		},
	}

	err = settleSwap(ctx, mgr, msg5, "savers add settlement")
	c.Assert(err, IsNil)

	// No outbound should be scheduled for savers add
	outItems, err = mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(len(outItems), Equals, 0)

	// Test 6: Error handling - fail to remove from queue (should still succeed)
	keeper.failRemove = true
	err = settleSwap(ctx, mgr, msg, "test with removal failure")
	c.Assert(err, IsNil) // Should not fail, just log errors
	keeper.failRemove = false
}

func (HelperSuite) TestSettleSwapDoesNotDuplicateOutboundOnRetry(c *C) {
	ctx, mgr := setupManagerForTest(c)

	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1))},
		"SWAP:ETH.ETH",
	)

	baseKeeper := mgr.Keeper()
	observedPubKey := GetRandomPubKey()
	seedSettleSwapRetryState(c, ctx, baseKeeper, tx, observedPubKey)
	mgr.K = &SettleSwapRetryVaultErrorKeeper{Keeper: baseKeeper}

	msg := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.ETHAsset,
		Destination: GetRandomETHAddress(),
		TradeTarget: cosmos.NewUint(1),
		SwapType:    types.SwapType_limit,
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.NewUint(600),
			Out:     cosmos.NewUint(550),
		},
	}

	c.Assert(settleSwap(ctx, mgr, msg, "expired limit order"), NotNil)
	c.Assert(settleSwap(ctx, mgr, msg, "expired limit order"), NotNil)

	outItems, err := mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	c.Assert(len(outItems), Equals, 0, Commentf("failed settlement must not leave a partial outbound behind"))
}

func (HelperSuite) TestSettleSwapRefundFromInactiveVault(c *C) {
	ctx, mgr := setupManagerForTest(c)

	txID := GetRandomTxHash()
	observedPubKey := GetRandomPubKey()

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable

	// Keeper returns an inactive vault with no coins (simulates churned-out vault)
	k := &SettleSwapTestKeeper{
		advSwapItems: make(map[string]types.MsgSwap),
		pools:        make(map[common.Asset]Pool),
		voter: ObservedTxVoter{
			Tx: ObservedTx{
				Tx: common.Tx{
					ID: txID,
				},
				ObservedPubKey: observedPubKey,
			},
		},
		overrideVault: true,
		vaultStatus:   InactiveVault,
		vaultCoins:    common.Coins{}, // empty — funds migrated away
	}
	k.pools[common.BTCAsset] = btcPool
	mgr.K = k
	mgr.txOutStore = NewTxStoreDummy()

	btcAddr := GetRandomBTCAddress()
	tx := common.NewTx(
		txID,
		btcAddr,
		GetRandomBTCAddress(),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1))},
		"SWAP:ETH.ETH",
	)
	msg := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.ETHAsset,
		Destination: GetRandomETHAddress(),
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1000),
			In:      cosmos.NewUint(600), // Only 600 used, 400 to refund
			Out:     cosmos.NewUint(550),
		},
	}

	err := settleSwap(ctx, mgr, msg, "expired limit order")
	c.Assert(err, IsNil)

	outItems, err := mgr.TxOutStore().GetOutboundItems(ctx)
	c.Assert(err, IsNil)
	// Expect 2 items: swap outbound + refund
	c.Assert(len(outItems), Equals, 2)

	// Find the refund item (the one in the original BTC asset)
	var refundItem *TxOutItem
	for i, item := range outItems {
		if item.Coin.Asset.Equals(common.BTCAsset) {
			refundItem = &outItems[i]
			break
		}
	}
	c.Assert(refundItem, NotNil)
	c.Assert(refundItem.Coin.Amount.Equal(cosmos.NewUint(400)), Equals, true)
	// VaultPubKey should be empty so prepareTxOutItem uses dynamic vault discovery
	c.Assert(refundItem.VaultPubKey.IsEmpty(), Equals, true)
}

func (s *HelperSuite) TestSettleSwapLimitSwapBasic(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test: Basic limit swap settlement should work without errors
	txID := GetRandomTxHash()
	currentBlockHeight := int64(100)
	ctx = ctx.WithBlockHeight(currentBlockHeight)

	ethAddr := GetRandomETHAddress()
	tx := common.NewTx(
		txID,
		ethAddr,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(1*common.One))},
		common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(1))},
		"swap:BTC.BTC",
	)

	limitSwapMsg := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.BTCAsset,
		Destination: GetRandomBTCAddress(),
		SwapType:    types.SwapType_limit,
		TradeTarget: cosmos.NewUint(5000000), // 0.05 BTC
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1 * common.One),
			In:      cosmos.NewUint(1 * common.One),
			Out:     cosmos.NewUint(5000000),
		},
	}

	// Settle the limit swap
	err := settleSwap(ctx, mgr, limitSwapMsg, "limit swap expired")
	c.Assert(err, IsNil)

	// Verify that settleSwap correctly handled the limit swap
	// The specific event emission logic is tested separately
	c.Assert(limitSwapMsg.IsLimitSwap(), Equals, true)
}

func (s *HelperSuite) TestSettleSwapMarketSwapBasic(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test: Market swap settlement should work without errors
	txID := GetRandomTxHash()
	currentBlockHeight := int64(100)
	ctx = ctx.WithBlockHeight(currentBlockHeight)

	ethAddr := GetRandomETHAddress()
	tx := common.NewTx(
		txID,
		ethAddr,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(1*common.One))},
		common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(1))},
		"swap:BTC.BTC",
	)

	marketSwapMsg := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.BTCAsset,
		Destination: GetRandomBTCAddress(),
		SwapType:    types.SwapType_market, // Market swap, not limit
		State: &types.SwapState{
			Deposit: cosmos.NewUint(1 * common.One),
			In:      cosmos.NewUint(1 * common.One),
			Out:     cosmos.NewUint(5000000),
		},
	}

	// Settle the market swap
	err := settleSwap(ctx, mgr, marketSwapMsg, "market swap completed")
	c.Assert(err, IsNil)

	// Verify that settleSwap correctly handled the market swap
	c.Assert(marketSwapMsg.IsLimitSwap(), Equals, false)
}

func (s *HelperSuite) TestSettleSwapLimitSwapWithDifferentReasons(c *C) {
	ctx, mgr := setupManagerForTest(c)

	currentBlockHeight := int64(150)
	ctx = ctx.WithBlockHeight(currentBlockHeight)

	ethAddr := GetRandomETHAddress()

	testCases := []struct {
		reason string
		txID   common.TxID
	}{
		{"limit swap completed", GetRandomTxHash()},
		{"limit swap cancelled", GetRandomTxHash()},
		{"limit swap expired", GetRandomTxHash()},
		{"limit swap failed", GetRandomTxHash()},
	}

	for i, testCase := range testCases {
		tx := common.NewTx(
			testCase.txID,
			ethAddr,
			GetRandomETHAddress(),
			common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(1*common.One))},
			common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(1))},
			"swap:BTC.BTC",
		)

		limitSwapMsg := types.MsgSwap{
			Tx:          tx,
			TargetAsset: common.BTCAsset,
			Destination: GetRandomBTCAddress(),
			SwapType:    types.SwapType_limit,
			TradeTarget: cosmos.NewUint(5000000),
			State: &types.SwapState{
				Deposit: cosmos.NewUint(1 * common.One),
				In:      cosmos.NewUint(1 * common.One),
				Out:     cosmos.NewUint(5000000),
			},
		}

		// Settle the limit swap with different reason
		err := settleSwap(ctx, mgr, limitSwapMsg, testCase.reason)
		c.Assert(err, IsNil, Commentf("Test case %d failed: %s", i, testCase.reason))

		// Verify it's still recognized as a limit swap
		c.Assert(limitSwapMsg.IsLimitSwap(), Equals, true)
	}
}

func (s *HelperSuite) TestSettleSwapStreamingLimitSwap(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test: Streaming limit swap (quantity > 1) should settle correctly
	txID := GetRandomTxHash()
	currentBlockHeight := int64(200)
	ctx = ctx.WithBlockHeight(currentBlockHeight)

	ethAddr := GetRandomETHAddress()
	tx := common.NewTx(
		txID,
		ethAddr,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(10*common.One))},
		common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(1))},
		"swap:BTC.BTC",
	)

	streamingLimitSwapMsg := types.MsgSwap{
		Tx:          tx,
		TargetAsset: common.BTCAsset,
		Destination: GetRandomBTCAddress(),
		SwapType:    types.SwapType_limit,
		TradeTarget: cosmos.NewUint(50000000), // 0.5 BTC
		State: &types.SwapState{
			Quantity: 10, // Streaming swap with 10 sub-swaps
			Count:    10, // All sub-swaps completed
			Deposit:  cosmos.NewUint(10 * common.One),
			In:       cosmos.NewUint(10 * common.One),
			Out:      cosmos.NewUint(50000000),
		},
	}

	// Settle the streaming limit swap
	err := settleSwap(ctx, mgr, streamingLimitSwapMsg, "streaming limit swap completed")
	c.Assert(err, IsNil)

	// Verify it's recognized as a limit swap
	c.Assert(streamingLimitSwapMsg.IsLimitSwap(), Equals, true)
	c.Assert(streamingLimitSwapMsg.State.Quantity, Equals, uint64(10))
}

func (s *HelperSuite) TestLeadingZeros(c *C) {
	// Test case where string length is less than the given length
	// The function should pad the string with leading zeros.
	c.Assert(leadingZeros(5, "123"), Equals, "00123")

	// Test case where string length is greater than the given length
	// The function should truncate the string to the given length.
	c.Assert(leadingZeros(2, "12345"), Equals, "12")

	// Test case where string length is equal to the given length
	// The function should return the string as it is.
	c.Assert(leadingZeros(5, "12345"), Equals, "12345")

	// Test case where string is empty
	// The function should return a string with the given length filled with zeros.
	c.Assert(leadingZeros(5, ""), Equals, "00000")

	// Test case where length is zero
	// The function should return an empty string regardless of the input string.
	c.Assert(leadingZeros(0, "12345"), Equals, "")
}

func (s *HelperSuite) TestApplyMemolessOutboundLogic(c *C) {
	// Generate random transaction hashes for testing
	txHash1 := GetRandomTxHash()
	txHash2 := GetRandomTxHash()
	txHash3 := GetRandomTxHash()
	txHash4 := GetRandomTxHash()
	txHash5 := GetRandomTxHash()
	txHash6 := GetRandomTxHash()
	refundHash := GetRandomTxHash()

	tests := []struct {
		name                   string
		memo                   string
		originalMemo           string
		aggregator             string
		enableMemolessOutbound int64
		expectedMemo           string
		expectedOriginalMemo   string
	}{
		// Memoless enabled scenarios
		{
			name:                   "basic memo clearing - regular memo gets cleared",
			memo:                   fmt.Sprintf("OUT:%s", txHash1),
			originalMemo:           "",
			aggregator:             "",
			enableMemolessOutbound: 1,
			expectedMemo:           "",
			expectedOriginalMemo:   fmt.Sprintf("OUT:%s", txHash1),
		},
		{
			name:                   "MIGRATE prefix preservation",
			memo:                   "MIGRATE:123",
			originalMemo:           "",
			aggregator:             "",
			enableMemolessOutbound: 1,
			expectedMemo:           "MIGRATE:123",
			expectedOriginalMemo:   "MIGRATE:123",
		},
		{
			name:                   "RAGNAROK prefix preservation",
			memo:                   "RAGNAROK:456",
			originalMemo:           "",
			aggregator:             "",
			enableMemolessOutbound: 1,
			expectedMemo:           "RAGNAROK:456",
			expectedOriginalMemo:   "RAGNAROK:456",
		},
		{
			name:                   "aggregator preservation",
			memo:                   fmt.Sprintf("OUT:%s", txHash2),
			originalMemo:           "",
			aggregator:             "0x123",
			enableMemolessOutbound: 1,
			expectedMemo:           fmt.Sprintf("OUT:%s", txHash2),
			expectedOriginalMemo:   fmt.Sprintf("OUT:%s", txHash2),
		},
		{
			name:                   "passthrough data preservation",
			memo:                   fmt.Sprintf("OUT:%s|extra|data", txHash3),
			originalMemo:           "",
			aggregator:             "",
			enableMemolessOutbound: 1,
			expectedMemo:           fmt.Sprintf("OUT:%s|extra|data", txHash3),
			expectedOriginalMemo:   fmt.Sprintf("OUT:%s|extra|data", txHash3),
		},
		{
			name:                   "original memo persistence - not overwritten",
			memo:                   fmt.Sprintf("OUT:%s", txHash4),
			originalMemo:           fmt.Sprintf("OUT:%s", txHash5),
			aggregator:             "",
			enableMemolessOutbound: 1,
			expectedMemo:           "",
			expectedOriginalMemo:   fmt.Sprintf("OUT:%s", txHash5),
		},
		// Memoless disabled scenarios
		{
			name:                   "memo restoration when disabled",
			memo:                   "",
			originalMemo:           fmt.Sprintf("OUT:%s", txHash6),
			aggregator:             "",
			enableMemolessOutbound: 0,
			expectedMemo:           fmt.Sprintf("OUT:%s", txHash6),
			expectedOriginalMemo:   fmt.Sprintf("OUT:%s", txHash6),
		},
		{
			name:                   "no restoration when original memo empty",
			memo:                   fmt.Sprintf("REFUND:%s", refundHash),
			originalMemo:           "",
			aggregator:             "",
			enableMemolessOutbound: 0,
			expectedMemo:           fmt.Sprintf("REFUND:%s", refundHash),
			expectedOriginalMemo:   fmt.Sprintf("REFUND:%s", refundHash),
		},
	}

	for _, tt := range tests {
		c.Logf("Running test: %s", tt.name)

		// Create TxOutItem with test data
		toi := TxOutItem{
			Memo:         tt.memo,
			OriginalMemo: tt.originalMemo,
			Aggregator:   tt.aggregator,
		}

		// Apply the memoless outbound logic
		applyMemolessOutboundLogic(GetCurrentVersion(), &toi, tt.enableMemolessOutbound)

		// Verify results
		c.Assert(toi.Memo, Equals, tt.expectedMemo, Commentf("Test: %s - Memo mismatch", tt.name))
		c.Assert(toi.OriginalMemo, Equals, tt.expectedOriginalMemo, Commentf("Test: %s - OriginalMemo mismatch", tt.name))
	}
}

func (s *HelperSuite) TestGetMaxSwapQuantityRapidInterval(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.Keeper()

	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceRune = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, ethPool), IsNil)

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(2000 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(10 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, btcPool), IsNil)

	k.SetMimir(ctx, "L1SlipMinBps", 100)             // 1%
	k.SetMimir(ctx, "StreamingSwapMaxLength", 14400) // max non-native streaming length

	// interval=0 (rapid) should not force max quantity to zero.
	quantity, err := getMaxSwapQuantity(ctx, mgr, common.ETHAsset, common.BTCAsset, StreamingSwap{
		Interval: 0,
		Deposit:  cosmos.NewUint(1000 * common.One),
	})
	c.Assert(err, IsNil)
	c.Assert(quantity, Equals, uint64(1000), Commentf("rapid interval should preserve non-zero quantity cap"))

	// interval=0 should still be bounded by max length constraints using effective interval=1.
	quantity, err = getMaxSwapQuantity(ctx, mgr, common.ETHAsset, common.BTCAsset, StreamingSwap{
		Interval: 0,
		Deposit:  cosmos.NewUint(50000 * common.One),
	})
	c.Assert(err, IsNil)
	c.Assert(quantity, Equals, uint64(14400), Commentf("rapid interval should respect max length cap"))
}

func (s *HelperSuite) TestIsStableToStable(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.Keeper()

	busd, _ := common.NewAsset("ETH.BUSD-BD1")
	usdc, _ := common.NewAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")

	// Set up pools
	for _, asset := range []common.Asset{busd, usdc} {
		pool := NewPool()
		pool.Asset = asset
		pool.Status = PoolAvailable
		pool.BalanceRune = cosmos.NewUint(1000 * common.One)
		pool.BalanceAsset = cosmos.NewUint(1000 * common.One)
		pool.Decimals = 8
		c.Assert(k.SetPool(ctx, pool), IsNil)
	}

	// No anchors configured — should return false
	c.Assert(isStableToStable(ctx, k, busd, usdc), Equals, false)

	// Enable both as TOR anchors
	k.SetMimir(ctx, "TorAnchor-ETH-BUSD-BD1", 1)
	k.SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)

	// Both are anchors — should return true
	c.Assert(isStableToStable(ctx, k, busd, usdc), Equals, true)

	// One stable, one non-stable — should return false
	c.Assert(isStableToStable(ctx, k, busd, common.BTCAsset), Equals, false)
	c.Assert(isStableToStable(ctx, k, common.ETHAsset, usdc), Equals, false)

	// RUNE to stable — should return false
	c.Assert(isStableToStable(ctx, k, common.RuneAsset(), usdc), Equals, false)

	// Synth stablecoin should also match via GetLayer1Asset
	synthBusd, _ := common.NewAsset("ETH/BUSD-BD1")
	synthUsdc, _ := common.NewAsset("ETH/USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")
	c.Assert(isStableToStable(ctx, k, synthBusd, synthUsdc), Equals, true)
}

func (s *HelperSuite) TestGetMinSlipBpsStableOverride(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.Keeper()

	busd, _ := common.NewAsset("ETH.BUSD-BD1")

	// Set up pool
	pool := NewPool()
	pool.Asset = busd
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	pool.Decimals = 8
	c.Assert(k.SetPool(ctx, pool), IsNil)

	// Set L1SlipMinBps to 10
	k.SetMimir(ctx, "L1SlipMinBps", 10)

	// Without stable override, should return L1 min bps
	result := getMinSlipBps(ctx, k, busd, false)
	c.Assert(result.Uint64(), Equals, uint64(10))

	// With stable override but StableSlipMinBps=0, should fall through to L1
	result = getMinSlipBps(ctx, k, busd, true)
	c.Assert(result.Uint64(), Equals, uint64(10))

	// Set StableSlipMinBps to 3
	k.SetMimir(ctx, "StableSlipMinBps", 3)

	// With stable override and non-zero StableSlipMinBps, should return stable value
	result = getMinSlipBps(ctx, k, busd, true)
	c.Assert(result.Uint64(), Equals, uint64(3))

	// Without stable override, should still return L1 value
	result = getMinSlipBps(ctx, k, busd, false)
	c.Assert(result.Uint64(), Equals, uint64(10))
}
