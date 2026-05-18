package thorchain

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	abci "github.com/cometbft/cometbft/abci/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// ValidateGenesis validate genesis is valid or not
func ValidateGenesis(data GenesisState) error {
	for _, record := range data.Pools {
		if err := record.Valid(); err != nil {
			return err
		}
	}

	for _, record := range data.SwapperClout {
		if err := record.Valid(); err != nil {
			return err
		}
	}

	for _, voter := range data.ObservedTxInVoters {
		if err := voter.Valid(); err != nil {
			return err
		}
	}

	for _, voter := range data.ObservedTxOutVoters {
		if err := voter.Valid(); err != nil {
			return err
		}
	}

	for _, out := range data.TxOuts {
		if err := out.Valid(); err != nil {
			return err
		}
	}

	for _, ta := range data.NodeAccounts {
		if err := ta.Valid(); err != nil {
			return err
		}
	}

	for _, vault := range data.Vaults {
		if err := vault.Valid(); err != nil {
			return err
		}
	}

	if data.LastSignedHeight < 0 {
		return errors.New("last signed height cannot be negative")
	}
	for _, c := range data.LastChainHeights {
		if c.Height < 0 {
			return fmt.Errorf("invalid chain(%s) height", c.Chain)
		}
	}

	for _, item := range data.AdvSwapQueueItems {
		if err := item.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid swap msg: %w", err)
		}
	}

	for _, item := range data.SwapQueueItems {
		if err := item.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid swap msg: %w", err)
		}
	}

	for _, item := range data.StreamingSwaps {
		if err := item.Valid(); err != nil {
			return fmt.Errorf("invalid streaming swap: %w", err)
		}
	}

	for _, nf := range data.NetworkFees {
		if err := nf.Valid(); err != nil {
			return fmt.Errorf("invalid network fee: %w", err)
		}
	}

	for _, cc := range data.ChainContracts {
		if cc.IsEmpty() {
			return fmt.Errorf("chain contract cannot be empty")
		}
	}

	return nil
}

// DefaultGenesisState the default values THORNode put in the Genesis
func DefaultGenesisState() GenesisState {
	return GenesisState{
		Pools:                   make([]Pool, 0),
		NodeAccounts:            NodeAccounts{},
		BondProviders:           make([]BondProviders, 0),
		TxOuts:                  make([]TxOut, 0),
		LiquidityProviders:      make(LiquidityProviders, 0),
		Vaults:                  make(Vaults, 0),
		ObservedTxInVoters:      make(ObservedTxVoters, 0),
		ObservedTxOutVoters:     make(ObservedTxVoters, 0),
		LastSignedHeight:        0,
		LastChainHeights:        make([]LastChainHeight, 0),
		Mimirs:                  make([]Mimir, 0),
		NodeMimirs:              make([]NodeMimir, 0),
		Network:                 NewNetwork(),
		OutboundFeeWithheldRune: common.Coins{},
		OutboundFeeSpentRune:    common.Coins{},
		POL:                     NewProtocolOwnedLiquidity(),
		AdvSwapQueueItems:       make([]MsgSwap, 0),
		SwapQueueItems:          make([]MsgSwap, 0),
		StreamingSwaps:          make([]StreamingSwap, 0),
		NetworkFees:             make([]NetworkFee, 0),
		ChainContracts:          make([]ChainContract, 0),
		THORNames:               make([]THORName, 0),
		SwapperClout:            make([]SwapperClout, 0),
		TradeAccounts:           make([]TradeAccount, 0),
		TradeUnits:              make([]TradeUnit, 0),
		SecuredAssets:           make([]SecuredAsset, 0),
		RuneProviders:           make([]RUNEProvider, 0),
		RunePool:                NewRUNEPool(),
		AffiliateCollectors:     []AffiliateFeeCollector{},
		PolReserveDeposits:      make([]POLReserveDeposit, 0),
	}
}

// initGenesis read the data in GenesisState and apply it to data store
func initGenesis(ctx cosmos.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Pools {
		if err := keeper.SetPool(ctx, record); err != nil {
			panic(err)
		}
	}

	for _, lp := range data.LiquidityProviders {
		keeper.SetLiquidityProvider(ctx, lp)
	}

	for _, rp := range data.RuneProviders {
		keeper.SetRUNEProvider(ctx, rp)
	}
	keeper.SetRUNEPool(ctx, data.RunePool)

	validators := make([]abci.ValidatorUpdate, 0, len(data.NodeAccounts))
	for _, nodeAccount := range data.NodeAccounts {
		if nodeAccount.Status == NodeActive {
			// Only Active node will become validator
			pk, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, nodeAccount.ValidatorConsPubKey)
			if err != nil {
				ctx.Logger().Error("fail to parse consensus public key", "key", nodeAccount.ValidatorConsPubKey, "error", err)
				panic(err)
			}
			validators = append(validators, abci.Ed25519ValidatorUpdate(pk.Bytes(), 100))
		}

		if err := keeper.SetNodeAccount(ctx, nodeAccount); err != nil {
			// we should panic
			panic(err)
		}
	}

	for _, vault := range data.Vaults {
		if err := keeper.SetVault(ctx, vault); err != nil {
			panic(err)
		}
	}

	for _, bp := range data.BondProviders {
		if err := keeper.SetBondProviders(ctx, bp); err != nil {
			panic(err)
		}
	}

	for _, voter := range data.ObservedTxInVoters {
		keeper.SetObservedTxInVoter(ctx, voter)
	}

	for _, voter := range data.ObservedTxOutVoters {
		keeper.SetObservedTxOutVoter(ctx, voter)
	}

	for idx := range data.TxOuts {
		if err := keeper.SetTxOut(ctx, &data.TxOuts[idx]); err != nil {
			ctx.Logger().Error("fail to save tx out during genesis", "error", err)
			panic(err)
		}
	}

	if data.LastSignedHeight > 0 {
		if err := keeper.SetLastSignedHeight(ctx, data.LastSignedHeight); err != nil {
			panic(err)
		}
	}

	for _, c := range data.LastChainHeights {
		chain, err := common.NewChain(c.Chain)
		if err != nil {
			panic(err)
		}
		if err = keeper.SetLastChainHeight(ctx, chain, c.Height); err != nil {
			panic(err)
		}
	}
	if err := keeper.SetNetwork(ctx, data.Network); err != nil {
		panic(err)
	}

	for i := range data.OutboundFeeWithheldRune {
		if err := keeper.AddToOutboundFeeWithheldRune(ctx, data.OutboundFeeWithheldRune[i].Asset, data.OutboundFeeWithheldRune[i].Amount); err != nil {
			panic(err)
		}
	}
	for i := range data.OutboundFeeSpentRune {
		if err := keeper.AddToOutboundFeeSpentRune(ctx, data.OutboundFeeSpentRune[i].Asset, data.OutboundFeeSpentRune[i].Amount); err != nil {
			panic(err)
		}
	}

	if err := keeper.SetPOL(ctx, data.POL); err != nil {
		panic(err)
	}

	for _, item := range data.AdvSwapQueueItems {
		if err := keeper.SetAdvSwapQueueItem(ctx, item); err != nil {
			panic(err)
		}
	}

	for _, item := range data.SwapQueueItems {
		if err := keeper.SetSwapQueueItem(ctx, item, 0); err != nil {
			panic(err)
		}
	}

	for _, item := range data.StreamingSwaps {
		keeper.SetStreamingSwap(ctx, item)
	}

	for _, nf := range data.NetworkFees {
		if err := keeper.SaveNetworkFee(ctx, nf.Chain, nf); err != nil {
			panic(err)
		}
	}

	for _, cc := range data.ChainContracts {
		keeper.SetChainContract(ctx, cc)
	}

	for _, n := range data.THORNames {
		keeper.SetTHORName(ctx, n)
	}

	for _, clout := range data.SwapperClout {
		if err := keeper.SetSwapperClout(ctx, clout); err != nil {
			panic(err)
		}
	}

	for _, acct := range data.TradeAccounts {
		keeper.SetTradeAccount(ctx, acct)
	}
	for _, unit := range data.TradeUnits {
		keeper.SetTradeUnit(ctx, unit)
	}
	for _, prd := range data.PolReserveDeposits {
		if err := keeper.SetPOLReserveDeposit(ctx, prd); err != nil {
			panic(err)
		}
	}

	for _, a := range data.SecuredAssets {
		keeper.SetSecuredAsset(ctx, a)
	}

	ttl := keeper.GetConfigInt64(ctx, constants.MemolessTxnTTL)
	for _, ref := range data.ReferenceMemos {
		if !ref.IsExpired(ctx.BlockHeight(), ttl) {
			keeper.SetReferenceMemo(ctx, ref)
		}
	}

	// Mint coins into the reserve
	if data.Reserve > 0 {
		coin := common.NewCoin(common.RuneNative, cosmos.NewUint(data.Reserve))
		if err := keeper.MintToModule(ctx, ModuleName, coin); err != nil {
			panic(err)
		}
		if err := keeper.SendFromModuleToModule(ctx, ModuleName, ReserveName, common.NewCoins(coin)); err != nil {
			panic(err)
		}
	}

	for _, item := range data.Mimirs {
		if len(item.Key) == 0 {
			continue
		}
		keeper.SetMimir(ctx, item.Key, item.Value)
	}

	for _, item := range data.NodeMimirs {
		if len(item.Key) == 0 {
			continue
		}
		if err := keeper.SetNodeMimir(ctx, item.Key, item.Value, item.Signer); err != nil {
			panic(err)
		}
	}

	for _, item := range data.AffiliateCollectors {
		keeper.SetAffiliateCollector(ctx, item)
	}

	for _, item := range data.TcyClaimers {
		if err := keeper.SetTCYClaimer(ctx, item); err != nil {
			panic(err)
		}
	}

	for _, item := range data.TcyStakers {
		if err := keeper.SetTCYStaker(ctx, item); err != nil {
			panic(err)
		}
	}

	reserveAddr, _ := keeper.GetModuleAddress(ReserveName)
	ctx.Logger().Info("Reserve Module", "address", reserveAddr.String())
	bondAddr, _ := keeper.GetModuleAddress(BondName)
	ctx.Logger().Info("Bond Module", "address", bondAddr.String())
	asgardAddr, _ := keeper.GetModuleAddress(AsgardName)
	ctx.Logger().Info("Asgard Module", "address", asgardAddr.String())
	treasuryAddr, _ := keeper.GetModuleAddress(TreasuryName)
	ctx.Logger().Info("Treasury Module", "address", treasuryAddr.String())
	runePoolAddr, _ := keeper.GetModuleAddress(RUNEPoolName)
	ctx.Logger().Info("RUNEPool Module", "address", runePoolAddr.String())
	ClaimingAddr, _ := keeper.GetModuleAddress(TCYClaimingName)
	ctx.Logger().Info("Claiming Module", "address", ClaimingAddr.String())
	tcyStakeAddr, _ := keeper.GetModuleAddress(TCYStakeName)
	ctx.Logger().Info("TCYStake Module", "address", tcyStakeAddr.String())

	return validators
}

func getLiquidityProviders(ctx cosmos.Context, k keeper.Keeper, asset common.Asset) LiquidityProviders {
	liquidityProviders := make(LiquidityProviders, 0)
	iterator := k.GetLiquidityProviderIterator(ctx, asset)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var lp LiquidityProvider
		k.Cdc().MustUnmarshal(iterator.Value(), &lp)
		if lp.Units.IsZero() && lp.PendingRune.IsZero() && lp.PendingAsset.IsZero() {
			continue
		}
		liquidityProviders = append(liquidityProviders, lp)
	}
	return liquidityProviders
}

func getValidPools(ctx cosmos.Context, k keeper.Keeper) Pools {
	var pools Pools
	iterator := k.GetPoolIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var pool Pool
		k.Cdc().MustUnmarshal(iterator.Value(), &pool)
		if pool.IsEmpty() {
			continue
		}
		if pool.Status == PoolSuspended {
			continue
		}
		pools = append(pools, pool)
	}
	return pools
}

// ExportGenesis export the data in Genesis
func ExportGenesis(ctx cosmos.Context, k keeper.Keeper) GenesisState {
	var iterator cosmos.Iterator
	pools := getValidPools(ctx, k)
	var liquidityProviders LiquidityProviders
	for _, pool := range pools {
		liquidityProviders = append(liquidityProviders, getLiquidityProviders(ctx, k, pool.Asset)...)
	}

	var nodeAccounts NodeAccounts
	vaultStatus := make(map[string]types.VaultStatus)
	iterator = k.GetNodeAccountIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var na NodeAccount
		k.Cdc().MustUnmarshal(iterator.Value(), &na)
		if na.IsEmpty() {
			continue
		}
		if na.Bond.IsZero() {
			continue
		}

		// filter inactive vaults from signer membership
		membership := make([]string, 0)
		for _, pk := range na.GetSignerMembership() {
			status, ok := vaultStatus[pk.String()]
			if !ok {
				vault, err := k.GetVault(ctx, pk)
				if err != nil {
					continue
				}
				status = vault.Status
				vaultStatus[pk.String()] = status
			}
			if status == types.VaultStatus_InactiveVault || status == types.VaultStatus_InitVault {
				continue
			}
			membership = append(membership, pk.String())
		}
		na.SignerMembership = membership

		nodeAccounts = append(nodeAccounts, na)
	}

	bps := make([]BondProviders, 0)
	for _, na := range nodeAccounts {
		bp, err := k.GetBondProviders(ctx, na.NodeAddress)
		if err != nil {
			panic(err)
		}
		bps = append(bps, bp)
	}

	tcyClaimers := make([]TCYClaimer, 0)
	iterator = k.GetTCYClaimerIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var claimer TCYClaimer
		k.Cdc().MustUnmarshal(iterator.Value(), &claimer)
		if claimer.IsEmpty() {
			continue
		}
		if claimer.Amount.IsZero() {
			continue
		}

		tcyClaimers = append(tcyClaimers, claimer)
	}

	tcyStakers := make([]TCYStaker, 0)
	stakers, err := k.ListTCYStakers(ctx)
	if err != nil {
		panic(err)
	}
	for _, staker := range stakers {
		if staker.IsEmpty() {
			continue
		}
		if staker.Amount.IsZero() {
			continue
		}

		tcyStakers = append(tcyStakers, staker)
	}

	var observedTxInVoters ObservedTxVoters
	var outs []TxOut
	startBlockHeight := ctx.BlockHeight() - k.GetConstants().GetInt64Value(constants.SigningTransactionPeriod)
	if startBlockHeight < 1 {
		startBlockHeight = 1
	}
	endBlockHeight := ctx.BlockHeight() + 17200

	for height := startBlockHeight; height < endBlockHeight; height++ {
		var txOut *keeper.TxOut
		txOut, err = k.GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("fail to get tx out", "error", err, "height", height)
			continue
		}
		if txOut.IsEmpty() {
			continue
		}
		includeTxOut := false
		for _, item := range txOut.TxArray {
			var vault types.Vault
			vault, err = k.GetVault(ctx, item.VaultPubKey)
			if err != nil {
				ctx.Logger().Error("fail to get vault", "error", err, "pubkey", item.VaultPubKey.String())
				continue
			}
			if vault.Status == types.VaultStatus_InactiveVault || vault.Status == types.VaultStatus_InitVault {
				// skip txouts from inactive vaults
				continue
			}

			if item.OutHash.IsEmpty() {
				// Set all these txouts if even one is still pending.
				includeTxOut = true
			} else {
				// If a TxOutItem has already been fulfilled,
				// don't export its ObservedTxInVoter.
				continue
			}

			if item.InHash.IsEmpty() || item.InHash.Equals(common.BlankTxID) {
				continue
			}
			var txInVoter ObservedTxVoter
			txInVoter, err = k.GetObservedTxInVoter(ctx, item.InHash)
			if err != nil {
				ctx.Logger().Error("fail to get observed tx in", "error", err, "hash", item.InHash.String())
				continue
			}
			observedTxInVoters = append(observedTxInVoters, txInVoter)
		}
		if includeTxOut {
			outs = append(outs, *txOut)
		}
	}

	lastSignedHeight, err := k.GetLastSignedHeight(ctx)
	if err != nil {
		panic(err)
	}

	chainHeights, err := k.GetLastChainHeights(ctx)
	if err != nil {
		panic(err)
	}
	lastChainHeights := make([]LastChainHeight, 0)
	// analyze-ignore(map-iteration)
	for k, v := range chainHeights {
		lastChainHeights = append(lastChainHeights, LastChainHeight{
			Chain:  k.String(),
			Height: v,
		})
	}
	// Let's sort it , so it is deterministic
	sort.Slice(lastChainHeights, func(i, j int) bool {
		return lastChainHeights[i].Chain < lastChainHeights[j].Chain
	})
	network, err := k.GetNetwork(ctx)
	if err != nil {
		panic(err)
	}

	pol, err := k.GetPOL(ctx)
	if err != nil {
		panic(err)
	}

	vaults := make(Vaults, 0)
	iterVault := k.GetVaultIterator(ctx)
	defer iterVault.Close()
	for ; iterVault.Valid(); iterVault.Next() {
		var vault Vault
		k.Cdc().MustUnmarshal(iterVault.Value(), &vault)
		if !vault.IsAsgard() {
			continue // filter non-asgard vault types
		}
		if vault.Status == types.VaultStatus_InactiveVault || vault.Status == types.VaultStatus_InitVault {
			continue // filter abandoned vaults
		}
		vaults = append(vaults, vault)
	}

	ttl := k.GetConfigInt64(ctx, constants.MemolessTxnTTL)
	memos := make([]ReferenceMemo, 0)
	refIter := k.GetReferenceMemoIterator(ctx)
	defer refIter.Close()
	for ; refIter.Valid(); refIter.Next() {
		var mem ReferenceMemo
		k.Cdc().MustUnmarshal(refIter.Value(), &mem)
		if mem.IsExpired(ctx.BlockHeight(), ttl) {
			continue
		}
		memos = append(memos, mem)
	}

	swapMsgs := make([]MsgSwap, 0)
	iterMsgSwap := k.GetAdvSwapQueueItemIterator(ctx)
	defer iterMsgSwap.Close()
	for ; iterMsgSwap.Valid(); iterMsgSwap.Next() {
		var m MsgSwap
		k.Cdc().MustUnmarshal(iterMsgSwap.Value(), &m)
		swapMsgs = append(swapMsgs, m)
	}

	swapQ := make([]MsgSwap, 0)
	iterSwapQ := k.GetSwapQueueIterator(ctx)
	defer iterSwapQ.Close()
	for ; iterSwapQ.Valid(); iterSwapQ.Next() {
		var m MsgSwap
		k.Cdc().MustUnmarshal(iterSwapQ.Value(), &m)
		swapQ = append(swapQ, m)
	}

	streamSwaps := make([]StreamingSwap, 0)
	iterStreamingSwap := k.GetStreamingSwapIterator(ctx)
	defer iterStreamingSwap.Close()
	for ; iterStreamingSwap.Valid(); iterStreamingSwap.Next() {
		var s StreamingSwap
		k.Cdc().MustUnmarshal(iterStreamingSwap.Value(), &s)
		streamSwaps = append(streamSwaps, s)
	}

	networkFees := make([]NetworkFee, 0)
	iterNetworkFee := k.GetNetworkFeeIterator(ctx)
	defer iterNetworkFee.Close()
	for ; iterNetworkFee.Valid(); iterNetworkFee.Next() {
		var nf NetworkFee
		k.Cdc().MustUnmarshal(iterNetworkFee.Value(), &nf)
		networkFees = append(networkFees, nf)
	}

	chainContracts := make([]ChainContract, 0)
	iter := k.GetChainContractIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var cc ChainContract
		k.Cdc().MustUnmarshal(iter.Value(), &cc)
		chainContracts = append(chainContracts, cc)
	}

	names := make([]THORName, 0)
	iterNames := k.GetTHORNameIterator(ctx)
	defer iterNames.Close()
	for ; iterNames.Valid(); iterNames.Next() {
		var n THORName
		k.Cdc().MustUnmarshal(iterNames.Value(), &n)
		names = append(names, n)
	}

	mimirs := make([]Mimir, 0)
	mimirIter := k.GetMimirIterator(ctx)
	defer mimirIter.Close()
	for ; mimirIter.Valid(); mimirIter.Next() {
		value := types.ProtoInt64{}
		k.Cdc().MustUnmarshal(mimirIter.Value(), &value)
		mimirs = append(mimirs, Mimir{
			Key:   strings.ReplaceAll(string(mimirIter.Key()), "mimir//", ""),
			Value: value.GetValue(),
		})
	}

	nodeMimirs := make([]NodeMimir, 0)
	nodeMimirIter := k.GetNodeMimirIterator(ctx)
	defer nodeMimirIter.Close()
	for ; nodeMimirIter.Valid(); nodeMimirIter.Next() {
		value := NodeMimirs{}
		k.Cdc().MustUnmarshal(nodeMimirIter.Value(), &value)
		nodeMimirs = append(nodeMimirs, value.GetMimirs()...)
	}

	clouts := make([]SwapperClout, 0)
	iterClouts := k.GetSwapperCloutIterator(ctx)
	defer iterClouts.Close()
	for ; iterClouts.Valid(); iterClouts.Next() {
		var addr common.Address
		parts := strings.Split(string(iterClouts.Key()), "/")
		addr, err = common.NewAddress(parts[len(parts)-1])
		if err != nil {
			continue
		}
		var clout SwapperClout
		clout, err = k.GetSwapperClout(ctx, addr)
		if err != nil {
			continue
		}
		clouts = append(clouts, clout)
	}

	runeProviders := make([]RUNEProvider, 0)
	iterRUNEProviders := k.GetRUNEProviderIterator(ctx)
	defer iterRUNEProviders.Close()
	for ; iterRUNEProviders.Valid(); iterRUNEProviders.Next() {
		var rp RUNEProvider
		k.Cdc().MustUnmarshal(iterRUNEProviders.Value(), &rp)
		runeProviders = append(runeProviders, rp)
	}

	runePool, err := k.GetRUNEPool(ctx)
	if err != nil {
		ctx.Logger().Error("fail to get rune pool", "error", err)
	}

	tradeAccts := make([]TradeAccount, 0)
	iterTradeAccts := k.GetTradeAccountIterator(ctx)
	defer iterTradeAccts.Close()
	for ; iterTradeAccts.Valid(); iterTradeAccts.Next() {
		var acct TradeAccount
		k.Cdc().MustUnmarshal(iterTradeAccts.Value(), &acct)
		tradeAccts = append(tradeAccts, acct)
	}
	tradeUnits := make([]TradeUnit, 0)
	iterTradeUnits := k.GetTradeUnitIterator(ctx)
	defer iterTradeUnits.Close()
	for ; iterTradeUnits.Valid(); iterTradeUnits.Next() {
		var unit TradeUnit
		k.Cdc().MustUnmarshal(iterTradeUnits.Value(), &unit)
		tradeUnits = append(tradeUnits, unit)
	}

	securedAssets := make([]SecuredAsset, 0)
	iterSecuredAssets := k.GetSecuredAssetIterator(ctx)
	defer iterSecuredAssets.Close()
	for ; iterSecuredAssets.Valid(); iterSecuredAssets.Next() {
		var a SecuredAsset
		k.Cdc().MustUnmarshal(iterSecuredAssets.Value(), &a)
		securedAssets = append(securedAssets, a)
	}

	polReserveDeposits := make([]POLReserveDeposit, 0)
	iterPOLReserveDeposits := k.GetPOLReserveDepositIterator(ctx)
	defer iterPOLReserveDeposits.Close()
	for ; iterPOLReserveDeposits.Valid(); iterPOLReserveDeposits.Next() {
		var prd POLReserveDeposit
		k.Cdc().MustUnmarshal(iterPOLReserveDeposits.Value(), &prd)
		polReserveDeposits = append(polReserveDeposits, prd)
	}

	// Use Coin struct to represent these Asset-Amount pairs.
	outboundFeeWithheldRune := common.Coins{}
	outboundFeeSpentRune := common.Coins{}
	iterOFWR := k.GetOutboundFeeWithheldRuneIterator(ctx)
	defer iterOFWR.Close()
	for ; iterOFWR.Valid(); iterOFWR.Next() {
		var asset common.Asset
		parts := strings.Split(string(iterOFWR.Key()), "/")
		asset, err = common.NewAsset(parts[len(parts)-1])
		if err != nil {
			continue
		}
		var amount cosmos.Uint
		amount, err = k.GetOutboundFeeWithheldRune(ctx, asset)
		if err != nil {
			continue
		}
		outboundFeeWithheldRune = append(outboundFeeWithheldRune, common.NewCoin(asset, amount))
	}
	iterOGRR := k.GetOutboundFeeSpentRuneIterator(ctx)
	defer iterOGRR.Close()
	for ; iterOGRR.Valid(); iterOGRR.Next() {
		var asset common.Asset
		parts := strings.Split(string(iterOGRR.Key()), "/")
		asset, err = common.NewAsset(parts[len(parts)-1])
		if err != nil {
			continue
		}
		var amount cosmos.Uint
		amount, err = k.GetOutboundFeeSpentRune(ctx, asset)
		if err != nil {
			continue
		}
		outboundFeeSpentRune = append(outboundFeeSpentRune, common.NewCoin(asset, amount))
	}

	affiliateCollectors, err := k.GetAffiliateCollectors(ctx)
	if err != nil {
		panic(err)
	}

	return GenesisState{
		Pools:                   pools,
		LiquidityProviders:      liquidityProviders,
		ObservedTxInVoters:      observedTxInVoters,
		TxOuts:                  outs,
		NodeAccounts:            nodeAccounts,
		BondProviders:           bps,
		Vaults:                  vaults,
		ReferenceMemos:          memos,
		LastSignedHeight:        lastSignedHeight,
		LastChainHeights:        lastChainHeights,
		Network:                 network,
		OutboundFeeWithheldRune: outboundFeeWithheldRune,
		OutboundFeeSpentRune:    outboundFeeSpentRune,
		POL:                     pol,
		AdvSwapQueueItems:       swapMsgs,
		SwapQueueItems:          swapQ,
		StreamingSwaps:          streamSwaps,
		NetworkFees:             networkFees,
		ChainContracts:          chainContracts,
		THORNames:               names,
		Mimirs:                  mimirs,
		NodeMimirs:              nodeMimirs,
		SwapperClout:            clouts,
		TradeAccounts:           tradeAccts,
		TradeUnits:              tradeUnits,
		SecuredAssets:           securedAssets,
		RuneProviders:           runeProviders,
		RunePool:                runePool,
		AffiliateCollectors:     affiliateCollectors,
		TcyClaimers:             tcyClaimers,
		TcyStakers:              tcyStakers,
		PolReserveDeposits:      polReserveDeposits,
	}
}
