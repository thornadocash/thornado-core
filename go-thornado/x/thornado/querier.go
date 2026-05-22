package thornado

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	tmhttp "github.com/cometbft/cometbft/rpc/client/http"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/evm/ethereum/eip712"
	"github.com/rs/zerolog/log"
	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	keeperv1 "github.com/thornadocash/go-thornado/x/thornado/keeper/v1"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

var (
	initManager = func(_ cosmos.Context, _ *Mgrs) {}
	queryExport = func(_ sdk.Context, _ *Mgrs) ([]byte, error) {
		return nil, fmt.Errorf("export query not supported")
	}
	tendermintClient   *tmhttp.HTTP
	initTendermintOnce = sync.Once{}
)

func initTendermint() {
	// get tendermint port from config
	portSplit := strings.Split(config.GetThornado().Tendermint.RPC.ListenAddress, ":")
	port := portSplit[len(portSplit)-1]

	// setup tendermint client
	var err error
	tendermintClient, err = tmhttp.New(fmt.Sprintf("tcp://localhost:%s", port), "/websocket")
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create tendermint client")
	}
}

func getPeerIDFromPubKey(pubkey common.PubKey) string {
	peerID, err := conversion.GetPeerIDFromPubKey(pubkey.String())
	if err != nil {
		// Don't break the entire endpoint if something goes wrong with the Peer ID derivation.
		return err.Error()
	}

	return peerID.String()
}

func (qs queryServer) queryBalanceModule(ctx cosmos.Context, req *types.QueryBalanceModuleRequest) (*types.QueryBalanceModuleResponse, error) {
	moduleName := req.Name
	if len(moduleName) == 0 {
		moduleName = AsgardName
	}

	modAddr := qs.mgr.Keeper().GetModuleAccAddress(moduleName)
	bal := qs.mgr.Keeper().GetBalance(ctx, modAddr)
	balance := types.QueryBalanceModuleResponse{
		Name:    moduleName,
		Address: modAddr,
		Coins:   bal,
	}
	return &balance, nil
}

func (qs queryServer) queryVault(ctx cosmos.Context, req *types.QueryVaultRequest) (*types.QueryVaultResponse, error) {
	if len(req.PubKey) < 1 {
		return nil, errors.New("missing vault pub_key parameter")
	}
	pubkey, err := common.NewPubKey(req.PubKey)
	if err != nil {
		return nil, fmt.Errorf("%s is invalid pubkey", req.PubKey)
	}
	v, err := qs.mgr.Keeper().GetVault(ctx, pubkey)
	if err != nil {
		return nil, fmt.Errorf("fail to get vault with pubkey(%s),err:%w", pubkey, err)
	}
	if v.IsEmpty() {
		return nil, errors.New("vault not found")
	}

	resp := types.QueryVaultResponse{
		BlockHeight:           v.BlockHeight,
		PubKey:                v.PubKey.String(),
		PubKeyEddsa:           v.PubKeyEddsa.String(),
		Coins:                 v.Coins,
		Type:                  v.Type.String(),
		Status:                v.Status.String(),
		StatusSince:           v.StatusSince,
		Membership:            v.Membership,
		Chains:                v.Chains,
		InboundTxCount:        v.InboundTxCount,
		OutboundTxCount:       v.OutboundTxCount,
		PendingTxBlockHeights: v.PendingTxBlockHeights,
		Routers:               castVaultRouters(v.Routers),
		Addresses:             getVaultChainAddresses(ctx, v),
		Frozen:                v.Frozen,
	}
	return &resp, nil
}

func (qs queryServer) queryAsgardVaults(ctx cosmos.Context, _ *types.QueryAsgardVaultsRequest) (*types.QueryAsgardVaultsResponse, error) {
	vaults, err := qs.mgr.Keeper().GetAsgardVaults(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get asgard vaults: %w", err)
	}

	var vaultsWithFunds []*types.QueryVaultResponse
	for _, vault := range vaults {
		if vault.Status == InactiveVault {
			continue
		}
		if !vault.IsAsgard() {
			continue
		}
		// Being in a RetiringVault blocks a node from unbonding, so display them even if having no funds.
		if vault.HasFunds() || vault.Status == ActiveVault || vault.Status == RetiringVault {
			vaultsWithFunds = append(vaultsWithFunds, &types.QueryVaultResponse{
				BlockHeight:           vault.BlockHeight,
				PubKey:                vault.PubKey.String(),
				PubKeyEddsa:           vault.PubKeyEddsa.String(),
				Coins:                 vault.Coins,
				Type:                  vault.Type.String(),
				Status:                vault.Status.String(),
				StatusSince:           vault.StatusSince,
				Membership:            vault.Membership,
				Chains:                vault.Chains,
				InboundTxCount:        vault.InboundTxCount,
				OutboundTxCount:       vault.OutboundTxCount,
				PendingTxBlockHeights: vault.PendingTxBlockHeights,
				Routers:               castVaultRouters(vault.Routers),
				Frozen:                vault.Frozen,
				Addresses:             getVaultChainAddresses(ctx, vault),
			})
		}
	}

	return &types.QueryAsgardVaultsResponse{AsgardVaults: vaultsWithFunds}, nil
}

func getVaultChainAddresses(ctx cosmos.Context, vault Vault) []*types.VaultAddress {
	var result []*types.VaultAddress
	allChains := append(vault.GetChains(), common.Thornado)
	for _, c := range allChains.Distinct() {
		if vault.PubKeyEddsa.IsEmpty() && c.GetSigningAlgo() != common.SigningAlgoEd25519 {
			// this is an eddsa chain, but the vault doesn't have an eddsa pubkey, skip.
			continue
		}
		addr, err := vault.GetAddress(c)
		if err != nil {
			ctx.Logger().Error("fail to get address", "chain", c.String(), "error", err)
			continue
		}

		result = append(result,
			&types.VaultAddress{
				Chain:   c.String(),
				Address: addr.String(),
			})
	}
	return result
}

// TODO: remove these vault pubkeys once we are done attempting recoveries from them
var whitelistPubkeys = map[string]bool{
	"thorpub1addwnpepqdc348zt7v8pqrncxzf0gz47jz5jcey9tcfpvv7zlsj50qfgmw7nuj296rh": true,
	"thorpub1addwnpepqwku796ak4ke2hj356m8yfq549m3fs96t57sukgg9we7u6tsqs6ajrsandz": true,
	"thorpub1addwnpepqwm8e5jdyjkm43hlf9mkm38vqvhj8cf6l3y9a745cn076u223vvyzxlaz2u": true,
	"thorpub1addwnpepqwht8xtersz90wsyu6fgx9tj5s3lx4w6q5adcm3mxzetgaa3rxv9sshhjmu": true,
}

func (qs queryServer) queryVaultsPubkeys(ctx cosmos.Context, _ *types.QueryVaultsPubkeysRequest) (*types.QueryVaultsPubkeysResponse, error) {
	var resp types.QueryVaultsPubkeysResponse
	resp.Asgard = make([]*types.VaultInfo, 0)
	resp.Inactive = make([]*types.VaultInfo, 0)
	iter := qs.mgr.Keeper().GetVaultIterator(ctx)

	active, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return nil, err
	}
	cutOffAge := ctx.BlockHeight() - config.GetThornado().VaultPubkeysCutoffBlocks
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var vault Vault
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &vault); err != nil {
			ctx.Logger().Error("fail to unmarshal vault", "error", err)
			return nil, fmt.Errorf("fail to unmarshal vault: %w", err)
		}
		if vault.IsAsgard() {
			switch vault.Status {
			case ActiveVault, RetiringVault:
				resp.Asgard = append(resp.Asgard, &types.VaultInfo{
					PubKey:      vault.PubKey.String(),
					PubKeyEddsa: vault.PubKeyEddsa.String(),
					Routers:     castVaultRouters(vault.Routers),
					Membership:  vault.Membership,
				})
			case InactiveVault:
				// skip inactive vaults that have never received an inbound
				if vault.InboundTxCount == 0 {
					continue
				}

				// skip inactive vaults older than the cutoff age
				if vault.BlockHeight < cutOffAge && !whitelistPubkeys[vault.PubKey.String()] {
					continue
				}

				activeMembers, err := vault.GetMembers(active.GetNodeAddresses())
				if err != nil {
					ctx.Logger().Error("fail to get active members of vault", "error", err)
					continue
				}
				allMembers := vault.Membership
				if HasSuperMajority(len(activeMembers), len(allMembers)) {
					resp.Inactive = append(resp.Inactive, &types.VaultInfo{
						PubKey:      vault.PubKey.String(),
						PubKeyEddsa: vault.PubKeyEddsa.String(),
						Routers:     castVaultRouters(vault.Routers),
						Membership:  vault.Membership,
					})
				}
			}
		}
	}
	return &resp, nil
}

func (qs queryServer) queryVaultSolvency(ctx cosmos.Context, _ *types.QueryVaultSolvencyRequest) (*types.QueryVaultSolvencyResponse, error) {
	// Get the network manager to calculate solvency
	networkMgr, ok := qs.mgr.NetworkMgr().(*NetworkMgr)
	if !ok {
		return nil, fmt.Errorf("network manager is not the correct type")
	}

	// Calculate network solvency (both over and under-solvency)
	solvencyAmounts, err := networkMgr.calculateNetworkSolvency(ctx, qs.mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate network solvency: %w", err)
	}

	// Build response with solvency per asset
	// Positive amounts indicate over-solvency, negative amounts indicate under-solvency
	resp := &types.QueryVaultSolvencyResponse{
		Assets: make([]*types.VaultSolvencyAsset, 0, len(solvencyAmounts)),
	}

	for _, assetAmt := range solvencyAmounts {
		resp.Assets = append(resp.Assets, &types.VaultSolvencyAsset{
			Asset:  assetAmt.Asset,
			Amount: assetAmt.Amount,
		})
	}

	return resp, nil
}

// queryRUNEProvider
// queryRUNEProviders
func (qs queryServer) queryInboundAddresses(ctx cosmos.Context, _ *types.QueryInboundAddressesRequest) (*types.QueryInboundAddressesResponse, error) {
	active, err := qs.mgr.Keeper().GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		ctx.Logger().Error("fail to get active vaults", "error", err)
		return nil, fmt.Errorf("fail to get active vaults: %w", err)
	}

	var resp []*types.QueryInboundAddressResponse
	constAccessor := qs.mgr.GetConstants()
	signingTransactionPeriod := constAccessor.GetInt64Value(constants.SigningTransactionPeriod)

	k := qs.mgr.Keeper()
	if k == nil {
		ctx.Logger().Error("keeper is nil, can't fulfill query")
		return nil, errors.New("keeper is nil, can't fulfill query")
	}
	// select vault that is most secure
	vault := k.GetMostSecure(ctx, active, signingTransactionPeriod)

	chains := vault.GetChains()

	if len(chains) == 0 {
		chains = common.Chains{common.RuneAsset().Chain}
	}

	isGlobalTradingPaused := k.IsGlobalTradingHalted(ctx)

	for _, chain := range chains {
		// tx send to thornado doesn't need an address , thus here skip it
		if chain == common.Thornado {
			continue
		}

		isChainTradingPaused := k.IsChainTradingHalted(ctx, chain)
		isChainLpPaused := false

		vaultAddress, err := vault.GetAddress(chain)
		if err != nil {
			ctx.Logger().Error("fail to get address for chain", "error", err)
			return nil, fmt.Errorf("fail to get address for chain: %w", err)
		}
		cc := vault.GetContract(chain)
		gasRate := qs.mgr.GasMgr().GetGasRate(ctx, chain)
		networkFeeInfo, err := qs.mgr.GasMgr().GetNetworkFee(ctx, chain)
		if err != nil {
			ctx.Logger().Error("fail to get network fee info", "error", err)
			return nil, fmt.Errorf("fail to get network fee info: %w", err)
		}

		// Retrieve the outbound fee for the chain's gas asset - fee will be zero if no network fee has been posted/the pool doesn't exist
		outboundFee, _ := qs.mgr.GasMgr().GetAssetOutboundFee(ctx, chain.GetGasAsset(), false)

		gasUnits, _ := chain.GetGasUnits()
		pubKey, err := vault.AlgoPubKey(chain)
		if err != nil {
			ctx.Logger().Error("fail to get pubkey for chain", "error", err)
			return nil, fmt.Errorf("fail to get pubkey for chain: %w", err)
		}

		addr := types.QueryInboundAddressResponse{
			Chain:                chain.String(),
			PubKey:               pubKey.String(),
			Address:              vaultAddress.String(),
			Router:               cc.Router.String(),
			Halted:               isGlobalTradingPaused || isChainTradingPaused,
			GlobalTradingPaused:  isGlobalTradingPaused,
			ChainTradingPaused:   isChainTradingPaused,
			ChainLpActionsPaused: isChainLpPaused,
			ObservedFeeRate:      cosmos.NewUint(networkFeeInfo.TransactionFeeRate).String(),
			GasRate:              gasRate.String(),
			GasRateUnits:         gasUnits,
			OutboundTxSize:       cosmos.NewUint(networkFeeInfo.TransactionSize).String(),
			OutboundFee:          outboundFee.String(),
			DustThreshold:        chain.DustThreshold().String(),
		}

		resp = append(resp, &addr)
	}

	return &types.QueryInboundAddressesResponse{
		InboundAddresses: resp,
	}, nil
}

// queryNode return the Node information related to the request node address
// /thornado/node/{nodeaddress}
func (qs queryServer) queryNode(ctx cosmos.Context, req *types.QueryNodeRequest) (*types.QueryNodeResponse, error) {
	if len(req.Address) == 0 {
		return nil, errors.New("node address not provided")
	}
	nodeAddress := req.Address
	addr, err := cosmos.AccAddressFromBech32(nodeAddress)
	if err != nil {
		return nil, cosmos.ErrUnknownRequest("invalid account address")
	}

	nodeAcc, err := qs.mgr.Keeper().GetNodeAccount(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("fail to get node accounts: %w", err)
	}

	slashPts, err := qs.mgr.Keeper().GetNodeAccountSlashPoints(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("fail to get node slash points: %w", err)
	}
	jail, err := qs.mgr.Keeper().GetNodeAccountJail(ctx, nodeAcc.NodeAddress)
	if err != nil {
		return nil, fmt.Errorf("fail to get node jail: %w", err)
	}

	active, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get all active node account: %w", err)
	}

	result := types.QueryNodeResponse{
		NodeAddress: nodeAcc.NodeAddress.String(),
		Status:      nodeAcc.Status.String(),
		PubKeySet: common.PubKeySet{
			Secp256k1: common.PubKey(nodeAcc.PubKeySet.Secp256k1.String()),
			Ed25519:   common.PubKey(nodeAcc.PubKeySet.Ed25519.String()),
		},
		NodeConsPubKey:      nodeAcc.NodeConsPubKey,
		ActiveBlockHeight:   nodeAcc.ActiveBlockHeight,
		StatusSince:         nodeAcc.StatusSince,
		NodeOperatorAddress: nodeAcc.BondAddress.String(),
		TotalBond:           nodeAcc.Bond.String(),
		SignerMembership:    nodeAcc.GetSignerMembership().Strings(),
		RequestedToLeave:    nodeAcc.RequestedToLeave,
		ForcedToLeave:       nodeAcc.ForcedToLeave,
		LeaveHeight:         int64(nodeAcc.LeaveScore), // OpenAPI can only represent uint64 as int64
		Maintenance:         nodeAcc.Maintenance,
		MissingBlocks:       int64(nodeAcc.MissingBlocks),
		IpAddress:           nodeAcc.IPAddress,
		Version:             nodeAcc.GetVersion().String(),
		CurrentAward:        cosmos.ZeroUint().String(), // Default display for if not overwritten.
	}
	result.PeerId = getPeerIDFromPubKey(nodeAcc.PubKeySet.Secp256k1)
	result.SlashPoints = slashPts

	result.Jail = &types.NodeJail{
		// Since redundant, leave out the node address
		ReleaseHeight: jail.ReleaseHeight,
		Reason:        jail.Reason,
	}

	// CurrentAward is an estimation of reward for node in active status
	// Node in other status should not have current reward
	if nodeAcc.Status == NodeActive && !nodeAcc.Bond.IsZero() {
		var network Network
		network, err = qs.mgr.Keeper().GetNetwork(ctx)
		if err != nil {
			return nil, fmt.Errorf("fail to get network: %w", err)
		}
		var vaults []Vault
		vaults, err = qs.mgr.Keeper().GetAsgardVaultsByStatus(ctx, ActiveVault)
		if err != nil {
			return nil, fmt.Errorf("fail to get active vaults: %w", err)
		}
		if len(vaults) == 0 {
			return nil, fmt.Errorf("no active vaults")
		}

		totalEffectiveBond, bondHardCap := getTotalEffectiveBond(active)

		lastChurnHeight := vaults[0].StatusSince

		var reward cosmos.Uint
		reward, err = getNodeCurrentRewards(ctx, qs.mgr, nodeAcc, lastChurnHeight, network.BondRewardRune, totalEffectiveBond, bondHardCap)
		if err != nil {
			return nil, fmt.Errorf("fail to get current node rewards: %w", err)
		}

		result.CurrentAward = reward.String()
	}

	// TODO: Represent this map as the field directly, instead of making an array?
	// It would then always be represented in alphabetical order.
	chainHeights, err := qs.mgr.Keeper().GetLastObserveHeight(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("fail to get last observe chain height: %w", err)
	}
	// analyze-ignore(map-iteration)
	for c, h := range chainHeights {
		result.ObserveChains = append(result.ObserveChains, &types.ChainHeight{
			Chain:  c.String(),
			Height: h,
		})
	}

	preflightCheckResult, err := getNodePreflightResult(ctx, qs.mgr, nodeAcc)
	if err != nil {
		ctx.Logger().Error("fail to get node preflight result", "error", err)
	} else {
		result.PreflightStatus = &preflightCheckResult
	}
	return &result, nil
}

func getNodePreflightResult(ctx cosmos.Context, mgr *Mgrs, nodeAcc NodeAccount) (types.NodePreflightStatus, error) {
	constAccessor := mgr.GetConstants()
	preflightResult := types.NodePreflightStatus{}
	status, err := mgr.NodeMgr().NodeAccountPreflightCheck(ctx, nodeAcc, constAccessor)
	if err == nil && status == NodeSelected {
		candidates := NodeAccounts{}
		for _, candidateStatus := range []NodeStatus{NodeWhiteListed, NodeStandby, NodeSelected} {
			nodes, listErr := mgr.Keeper().ListNodesByStatus(ctx, candidateStatus)
			if listErr != nil {
				return preflightResult, listErr
			}
			candidates = append(candidates, nodes...)
		}
		selected := mgr.NodeMgr().selectHighestBondedNode(ctx, candidates)
		if selected.IsEmpty() || !selected.NodeAddress.Equals(nodeAcc.NodeAddress) {
			status = NodeWhiteListed
			err = fmt.Errorf("insufficient bond")
		}
	}
	preflightResult.Status = status.String()
	if err != nil {
		preflightResult.Reason = err.Error()
		preflightResult.Code = 1
	} else {
		preflightResult.Reason = "OK"
		preflightResult.Code = 0
	}
	return preflightResult, nil
}

// Estimates current rewards for the NodeAccount taking into account bond-weighted rewards and slash points
func getNodeCurrentRewards(ctx cosmos.Context, mgr *Mgrs, nodeAcc NodeAccount, lastChurnHeight int64, totalBondReward, totalEffectiveBond, bondHardCap cosmos.Uint) (cosmos.Uint, error) {
	slashPts, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nodeAcc.NodeAddress)
	if err != nil {
		return cosmos.ZeroUint(), fmt.Errorf("fail to get node slash points: %w", err)
	}

	// Find number of blocks since the last churn (the last bond reward payout)
	totalActiveBlocks := ctx.BlockHeight() - lastChurnHeight

	// find number of blocks they were well behaved (ie active - slash points)
	earnedBlocks := totalActiveBlocks - slashPts
	if earnedBlocks < 0 {
		earnedBlocks = 0
	}

	naEffectiveBond := nodeAcc.Bond
	if naEffectiveBond.GT(bondHardCap) {
		naEffectiveBond = bondHardCap
	}

	// reward = totalBondReward * (naEffectiveBond / totalEffectiveBond) * (unslashed blocks since last churn / blocks since last churn)
	reward := common.GetUncappedShare(naEffectiveBond, totalEffectiveBond, totalBondReward)
	reward = common.GetUncappedShare(cosmos.NewUint(uint64(earnedBlocks)), cosmos.NewUint(uint64(totalActiveBlocks)), reward)
	return reward, nil
}

// queryNodes return all the nodes that has bond
// /thornado/nodes
func (qs queryServer) queryNodes(ctx cosmos.Context, _ *types.QueryNodesRequest) (*types.QueryNodesResponse, error) {
	nodeAccounts, err := qs.mgr.Keeper().ListNodesWithBond(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get node accounts: %w", err)
	}

	active, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get all active node account: %w", err)
	}

	network, err := qs.mgr.Keeper().GetNetwork(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get network: %w", err)
	}

	vaults, err := qs.mgr.Keeper().GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return nil, fmt.Errorf("fail to get active vaults: %w", err)
	}
	if len(vaults) == 0 {
		return nil, fmt.Errorf("no active vaults")
	}

	totalEffectiveBond, bondHardCap := getTotalEffectiveBond(active)

	lastChurnHeight := vaults[0].StatusSince
	result := make([]*types.QueryNodeResponse, len(nodeAccounts))
	for i, na := range nodeAccounts {
		if na.RequestedToLeave && na.Bond.LTE(cosmos.NewUint(common.One)) {
			// ignore the node , it left and also has very little bond
			// Set the default display for fields which would otherwise be "".
			result[i] = &types.QueryNodeResponse{
				Status:          types.NodeStatus_Unknown.String(),
				TotalBond:       cosmos.ZeroUint().String(),
				Version:         semver.MustParse("0.0.0").String(),
				CurrentAward:    cosmos.ZeroUint().String(),
				PreflightStatus: &types.NodePreflightStatus{Status: types.NodeStatus_Unknown.String()},
			}
			continue
		}

		slashPts, err := qs.mgr.Keeper().GetNodeAccountSlashPoints(ctx, na.NodeAddress)
		if err != nil {
			return nil, fmt.Errorf("fail to get node slash points: %w", err)
		}

		result[i] = &types.QueryNodeResponse{
			NodeAddress: na.NodeAddress.String(),
			Status:      na.Status.String(),
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(na.PubKeySet.Secp256k1.String()),
				Ed25519:   common.PubKey(na.PubKeySet.Ed25519.String()),
			},
			NodeConsPubKey:      na.NodeConsPubKey,
			ActiveBlockHeight:   na.ActiveBlockHeight,
			StatusSince:         na.StatusSince,
			NodeOperatorAddress: na.BondAddress.String(),
			TotalBond:           na.Bond.String(),
			SignerMembership:    na.GetSignerMembership().Strings(),
			RequestedToLeave:    na.RequestedToLeave,
			ForcedToLeave:       na.ForcedToLeave,
			LeaveHeight:         int64(na.LeaveScore), // OpenAPI can only represent uint64 as int64
			Maintenance:         na.Maintenance,
			MissingBlocks:       int64(na.MissingBlocks),
			IpAddress:           na.IPAddress,
			Version:             na.GetVersion().String(),
			CurrentAward:        cosmos.ZeroUint().String(), // Default display for if not overwritten.
		}
		result[i].PeerId = getPeerIDFromPubKey(na.PubKeySet.Secp256k1)
		result[i].SlashPoints = slashPts
		if na.Status == NodeActive {
			var reward cosmos.Uint
			reward, err = getNodeCurrentRewards(ctx, qs.mgr, na, lastChurnHeight, network.BondRewardRune, totalEffectiveBond, bondHardCap)
			if err != nil {
				return nil, fmt.Errorf("fail to get current node rewards: %w", err)
			}

			result[i].CurrentAward = reward.String()
		}

		var jail Jail
		jail, err = qs.mgr.Keeper().GetNodeAccountJail(ctx, na.NodeAddress)
		if err != nil {
			return nil, fmt.Errorf("fail to get node jail: %w", err)
		}
		result[i].Jail = &types.NodeJail{
			// Since redundant, leave out the node address
			ReleaseHeight: jail.ReleaseHeight,
			Reason:        jail.Reason,
		}

		// TODO: Represent this map as the field directly, instead of making an array?
		// It would then always be represented in alphabetical order.
		chainHeights, err := qs.mgr.Keeper().GetLastObserveHeight(ctx, na.NodeAddress)
		if err != nil {
			return nil, fmt.Errorf("fail to get last observe chain height: %w", err)
		}
		// analyze-ignore(map-iteration)
		for c, h := range chainHeights {
			result[i].ObserveChains = append(result[i].ObserveChains, &types.ChainHeight{
				Chain:  c.String(),
				Height: h,
			})
		}

		preflightCheckResult, err := getNodePreflightResult(ctx, qs.mgr, na)
		if err != nil {
			ctx.Logger().Error("fail to get node preflight result", "error", err)
		} else {
			result[i].PreflightStatus = &preflightCheckResult
		}

	}

	return &types.QueryNodesResponse{Nodes: result}, nil
}

func extractVoter(ctx cosmos.Context, tx_id string, mgr *Mgrs) (common.TxID, ObservedTxVoter, error) {
	if len(tx_id) == 0 {
		return "", ObservedTxVoter{}, errors.New("tx id not provided")
	}
	hash, err := common.NewTxID(tx_id)
	if err != nil {
		ctx.Logger().Error("fail to parse tx id", "error", err)
		return "", ObservedTxVoter{}, fmt.Errorf("fail to parse tx id: %w", err)
	}
	voter, err := mgr.Keeper().GetObservedTxInVoter(ctx, hash)
	if err != nil {
		ctx.Logger().Error("fail to get observed tx voter", "error", err)
		return "", ObservedTxVoter{}, fmt.Errorf("fail to get observed tx voter: %w", err)
	}
	return hash, voter, nil
}

func (qs queryServer) queryTxVoters(ctx cosmos.Context, req *types.QueryTxVotersRequest) (*types.QueryObservedTxVoter, error) {
	hash, voter, err := extractVoter(ctx, req.TxId, qs.mgr)
	if err != nil {
		return nil, err
	}
	// when tx in voter doesn't exist , double check tx out voter
	if len(voter.Txs) == 0 {
		voter, err = qs.mgr.Keeper().GetObservedTxOutVoter(ctx, hash)
		if err != nil {
			return nil, fmt.Errorf("fail to get observed tx out voter: %w", err)
		}
		if len(voter.Txs) == 0 {
			return nil, fmt.Errorf("tx: %s doesn't exist", hash)
		}
	}

	var txs []types.QueryObservedTx
	// Leave this nil (null rather than []) if the source is nil.
	if voter.Txs != nil {
		txs = make([]types.QueryObservedTx, len(voter.Txs))
		for i := range voter.Txs {
			txs[i] = castObservedTx(voter.Txs[i])
		}
	}

	return &types.QueryObservedTxVoter{
		TxID:            voter.TxID,
		Tx:              castObservedTx(voter.Tx),
		Height:          voter.Height,
		Txs:             txs,
		Actions:         voter.Actions,
		OutTxs:          voter.OutTxs,
		FinalisedHeight: voter.FinalisedHeight,
		UpdatedVault:    voter.UpdatedVault,
		Reverted:        voter.Reverted,
		OutboundHeight:  voter.OutboundHeight,
	}, nil
}

// Get the largest number of signers for a not-final (pre-confirmation-counting) and final Txs respectively.
func countSigners(voter ObservedTxVoter) (int64, int64) {
	var notFinalCount, finalCount int
	for i, refTx := range voter.Txs {
		signersMap := make(map[string]bool)
		final := refTx.IsFinal()
		for f, tx := range voter.Txs {
			// Earlier Txs already checked against all, so no need to check,
			// but do include the signers of the current Txs.
			if f < i {
				continue
			}
			// Count larger number of signers for not-final and final observations separately.
			if tx.IsFinal() != final {
				continue
			}
			if !refTx.Tx.EqualsEx(tx.Tx) {
				continue
			}

			for _, signer := range tx.GetSigners() {
				signersMap[signer.String()] = true
			}
		}
		if final && len(signersMap) > finalCount {
			finalCount = len(signersMap)
		} else if !final && len(signersMap) > notFinalCount {
			notFinalCount = len(signersMap)
		}
	}
	return int64(notFinalCount), int64(finalCount)
}

// Call newTxStagesResponse from both queryTxStatus (which includes the stages) and queryTxStages.
// TODO: Deprecate InboundObserved.Started field in favour of the observation counting.
func newTxStagesResponse(ctx cosmos.Context, voter ObservedTxVoter) (result types.QueryTxStagesResponse) {
	result.InboundObserved.PreConfirmationCount, result.InboundObserved.FinalCount = countSigners(voter)
	result.InboundObserved.Completed = !voter.Tx.IsEmpty()

	// If not Completed, fill in Started and do not proceed.
	if !result.InboundObserved.Completed {
		obStart := (len(voter.Txs) != 0)
		result.InboundObserved.Started = obStart
		return result
	}

	// Current block height is relevant in the confirmation counting and outbound stages.
	currentHeight := ctx.BlockHeight()

	// Only fill in InboundConfirmationCounted when confirmation counting took place.
	if voter.Height != 0 {
		var confCount types.InboundConfirmationCountedStage

		// Set the Completed state first.
		extObsHeight := voter.Tx.BlockHeight
		extConfDelayHeight := voter.Tx.FinaliseHeight
		confCount.Completed = !(extConfDelayHeight > extObsHeight)

		// Only fill in other fields if not Completed.
		if !confCount.Completed {
			countStartHeight := voter.Height
			confCount.CountingStartHeight = countStartHeight
			confCount.Chain = voter.Tx.Tx.Chain.String()
			confCount.ExternalObservedHeight = extObsHeight
			confCount.ExternalConfirmationDelayHeight = extConfDelayHeight

			estConfMs := voter.Tx.Tx.Chain.ApproximateBlockMilliseconds() * (extConfDelayHeight - extObsHeight)
			if currentHeight > countStartHeight {
				estConfMs -= (currentHeight - countStartHeight) * common.Thornado.ApproximateBlockMilliseconds()
			}
			estConfSec := estConfMs / 1000
			// Floor at 0.
			if estConfSec < 0 {
				estConfSec = 0
			}
			confCount.RemainingConfirmationSeconds = estConfSec
		}

		result.InboundConfirmationCounted = &confCount
	}

	var inboundFinalised types.InboundFinalisedStage
	inboundFinalised.Completed = (voter.FinalisedHeight != 0)
	result.InboundFinalised = &inboundFinalised

	// Only fill ExternalOutboundDelay and ExternalOutboundKeysign for inbound transactions with an external outbound;
	// namely, transactions with an outbound_height .
	if voter.OutboundHeight == 0 {
		return result
	}

	// Only display the OutboundDelay stage when there's a delay.
	if voter.OutboundHeight > voter.FinalisedHeight {
		var outDelay types.OutboundDelayStage

		// Set the Completed state first.
		outDelay.Completed = (currentHeight >= voter.OutboundHeight)

		// Only fill in other fields if not Completed.
		if !outDelay.Completed {
			remainBlocks := voter.OutboundHeight - currentHeight
			outDelay.RemainingDelayBlocks = remainBlocks

			remainSec := remainBlocks * common.Thornado.ApproximateBlockMilliseconds() / 1000
			outDelay.RemainingDelaySeconds = remainSec
		}

		result.OutboundDelay = &outDelay
	}

	var outSigned types.OutboundSignedStage

	// Set the Completed state first.
	outSigned.Completed = (voter.Tx.Status != common.Status_incomplete)

	// Only fill in other fields if not Completed.
	if !outSigned.Completed {
		scheduledHeight := voter.OutboundHeight
		outSigned.ScheduledOutboundHeight = scheduledHeight

		// Only fill in BlocksSinceScheduled if the outbound delay is complete.
		if currentHeight >= scheduledHeight {
			sinceScheduled := currentHeight - scheduledHeight
			outSigned.BlocksSinceScheduled = &types.ProtoInt64{Value: sinceScheduled}
		}
	}

	result.OutboundSigned = &outSigned

	return result
}

func (qs queryServer) queryTxStages(ctx cosmos.Context, req *types.QueryTxStagesRequest) (*types.QueryTxStagesResponse, error) {
	// First, get the ObservedTxVoter of interest.
	_, voter, err := extractVoter(ctx, req.TxId, qs.mgr)
	if err != nil {
		return nil, err
	}
	// when no TxIn voter don't check TxOut voter, as TxOut Thornado observation or not matters little to the user once signed and broadcast
	// Rather than a "tx: %s doesn't exist" result, allow a response to an existing-but-unobserved hash with Observation.Started 'false'.

	result := newTxStagesResponse(ctx, voter)

	return &result, nil
}

func (qs queryServer) queryTxStatus(ctx cosmos.Context, req *types.QueryTxStatusRequest) (*types.QueryTxStatusResponse, error) {
	// First, get the ObservedTxVoter of interest.
	_, voter, err := extractVoter(ctx, req.TxId, qs.mgr)
	if err != nil {
		return nil, err
	}
	// when no TxIn voter don't check TxOut voter, as TxOut Thornado observation or not matters little to the user once signed and broadcast
	// Rather than a "tx: %s doesn't exist" result, allow a response to an existing-but-unobserved hash with Stages.Observation.Started 'false'.

	var result types.QueryTxStatusResponse

	// If there's a consensus Tx, display that.
	// If not, but there's at least one observation, display the first observation's Tx.
	// If there are no observations yet, don't display a Tx (only showing the 'Observation' stage with 'Started' false).
	if !voter.Tx.Tx.IsEmpty() {
		result.Tx = &voter.Tx.Tx
	} else if len(voter.Txs) > 0 {
		result.Tx = &voter.Txs[0].Tx
	}

	// Leave this nil (null rather than []) if the source is nil.
	if voter.Actions != nil {
		result.PlannedOutTxs = make([]*types.PlannedOutTx, len(voter.Actions))
		for i := range voter.Actions {
			result.PlannedOutTxs[i] = &types.PlannedOutTx{
				Chain:     voter.Actions[i].Chain.String(),
				ToAddress: voter.Actions[i].ToAddress.String(),
				Coin:      &voter.Actions[i].Coin,
				Refund:    strings.HasPrefix(voter.Actions[i].GetMemo(), "REFUND"),
			}
		}
	}

	// Leave this nil (null rather than []) if the source is nil.
	if voter.OutTxs != nil {
		result.OutTxs = voter.OutTxs
	}

	result.Stages = newTxStagesResponse(ctx, voter)

	return &result, nil
}

func (qs queryServer) queryTx(ctx cosmos.Context, req *types.QueryTxRequest) (*types.QueryTxResponse, error) {
	hash, voter, err := extractVoter(ctx, req.TxId, qs.mgr)
	if err != nil {
		return nil, err
	}
	if len(voter.Txs) == 0 {
		voter, err = qs.mgr.Keeper().GetObservedTxOutVoter(ctx, hash)
		if err != nil {
			return nil, fmt.Errorf("fail to get observed tx out voter: %w", err)
		}
		if len(voter.Txs) == 0 {
			return nil, fmt.Errorf("tx: %s doesn't exist", hash)
		}
	}

	nodeAccounts, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get node accounts: %w", err)
	}
	keysignMetric, err := qs.mgr.Keeper().GetTssKeysignMetric(ctx, hash)
	if err != nil {
		ctx.Logger().Error("fail to get keysign metrics", "error", err)
	}

	result := types.QueryTxResponse{
		ObservedTx:      castObservedTx(*voter.GetTx(nodeAccounts)),
		ConsensusHeight: voter.Height,
		FinalisedHeight: voter.FinalisedHeight,
		OutboundHeight:  voter.OutboundHeight,
		KeysignMetric:   keysignMetric,
	}

	return &result, nil
}

func (qs queryServer) queryShielderDeposit(ctx cosmos.Context, req *types.QueryShielderDepositRequest) (*types.QueryShielderDepositResponse, error) {
	depositID, err := common.NewTxID(req.DepositId)
	if err != nil {
		return nil, err
	}
	deposit, err := qs.mgr.Keeper().GetShielderDeposit(ctx, depositID)
	if err != nil {
		return nil, err
	}
	if deposit.DepositID.IsEmpty() {
		return nil, errors.New("shielder deposit not found")
	}
	return &types.QueryShielderDepositResponse{
		DepositId:        deposit.DepositID.String(),
		Owner:            deposit.Owner.String(),
		AmountSats:       deposit.AmountSats,
		DepositAddress:   deposit.DepositAddress.String(),
		VaultPubKey:      deposit.VaultPubKey.String(),
		DepositPathIndex: deposit.DepositPathIndex,
		Status:           deposit.Status,
		Settlement:       deposit.Settlement,
		AuctionId:        deposit.AuctionID,
		NodePubKey:       deposit.NodePubKey,
		NodeSlot:         deposit.NodeSlot,
		BondConfirmed:    deposit.BondConfirmed,
		CommitmentCount:  uint64(len(deposit.Commitments)),
	}, nil
}

func (qs queryServer) queryShielderFeePool(ctx cosmos.Context, _ *types.QueryShielderFeePoolRequest) (*types.QueryShielderFeePoolResponse, error) {
	pool, err := qs.mgr.Keeper().GetShielderFeePool(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryShielderFeePoolResponse{
		PendingSats:        pool.PendingSats,
		TotalSlots:         pool.TotalSlots,
		FeePerSlotShare:    pool.FeePerSlotShare,
		TotalCollectedSats: pool.TotalCollectedSats,
		TotalClaimedSats:   pool.TotalClaimedSats,
	}, nil
}

func (qs queryServer) queryShielderSession(ctx cosmos.Context, req *types.QueryShielderSessionRequest) (*types.QueryShielderSessionResponse, error) {
	owner, err := cosmos.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, err
	}
	session, err := qs.mgr.Keeper().GetShielderSession(ctx, owner)
	if err != nil {
		return nil, err
	}
	if session.Owner.Empty() {
		return nil, errors.New("shielder session not found")
	}
	return &types.QueryShielderSessionResponse{
		Owner:            session.Owner.String(),
		DepositAddress:   session.DepositAddress.String(),
		VaultPubKey:      session.VaultPubKey.String(),
		DepositPathIndex: session.DepositPathIndex,
		OperatorPubKey:   session.OperatorPubKey.String(),
		NodePubKey:       session.NodePubKey,
		AuctionId:        session.AuctionID,
		CreatedHeight:    session.CreatedHeight,
		Status:           session.Status,
		DepositId:        session.DepositID.String(),
	}, nil
}

func (qs queryServer) queryShielderBond(ctx cosmos.Context, req *types.QueryShielderBondRequest) (*types.QueryShielderBondResponse, error) {
	bond, err := qs.mgr.Keeper().GetShielderNodeBond(ctx, req.NodePubKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bond.NodePubKey) == "" {
		return nil, errors.New("shielder node bond not found")
	}
	node, err := qs.mgr.Keeper().GetNodeAccount(ctx, bond.NodeAddress)
	if err != nil {
		return nil, err
	}
	slashPoints, err := qs.mgr.Keeper().GetNodeAccountSlashPoints(ctx, bond.NodeAddress)
	if err != nil {
		return nil, err
	}
	return &types.QueryShielderBondResponse{
		NodePubKey:          bond.NodePubKey,
		OperatorPubKey:      bond.OperatorPubKey.String(),
		NodeAddress:         bond.NodeAddress.String(),
		Slot:                bond.Slot,
		PendingSats:         bond.PendingSats,
		BondSats:            bond.BondSats,
		FeeDebtSats:         bond.FeeDebtSats,
		FeeShareActive:      bond.FeeShareActive,
		PendingFeeDepositId: bond.PendingFeeDepositID.String(),
		Sold:                bond.Sold,
		SoldAuctionId:       bond.SoldAuctionID,
		CreatedHeight:       bond.CreatedHeight,
		UpdatedHeight:       bond.UpdatedHeight,
		NodeStatus:          node.Status.String(),
		SlashPoints:         slashPoints,
	}, nil
}

func (qs queryServer) queryNodeSlotAuction(ctx cosmos.Context, req *types.QueryNodeSlotAuctionRequest) (*types.QueryNodeSlotAuctionResponse, error) {
	auction, err := qs.mgr.Keeper().GetNodeSlotAuction(ctx, req.AuctionId)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(auction.AuctionID) == "" {
		return nil, errors.New("node slot auction not found")
	}
	return &types.QueryNodeSlotAuctionResponse{
		AuctionId:            auction.AuctionID,
		Seller:               auction.Seller.String(),
		SellerOperatorPubKey: auction.SellerOperatorPubKey.String(),
		SellerNodePubKey:     auction.SellerNodePubKey,
		Slot:                 auction.Slot,
		OriginalBondSats:     auction.OriginalBondSats,
		ReserveSats:          auction.ReserveSats,
		ExpiryHeight:         auction.ExpiryHeight,
		SelectedBidId:        auction.SelectedBidID,
		Status:               auction.Status,
		CreatedHeight:        auction.CreatedHeight,
		UpdatedHeight:        auction.UpdatedHeight,
	}, nil
}

func (qs queryServer) queryNodeSlotBid(ctx cosmos.Context, req *types.QueryNodeSlotBidRequest) (*types.QueryNodeSlotBidResponse, error) {
	bid, err := qs.mgr.Keeper().GetNodeSlotBid(ctx, req.BidId)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bid.BidID) == "" {
		return nil, errors.New("node slot bid not found")
	}
	return &types.QueryNodeSlotBidResponse{
		BidId:          bid.BidID,
		AuctionId:      bid.AuctionID,
		Bidder:         bid.Bidder.String(),
		OperatorPubKey: bid.OperatorPubKey.String(),
		NodePubKey:     bid.NodePubKey,
		DepositId:      bid.DepositID.String(),
		AmountSats:     bid.AmountSats,
		Selected:       bid.Selected,
		Settled:        bid.Settled,
		CreatedHeight:  bid.CreatedHeight,
		UpdatedHeight:  bid.UpdatedHeight,
	}, nil
}

func (qs queryServer) queryVaultDepositAddress(ctx cosmos.Context, req *types.QueryVaultDepositAddressRequest) (*types.QueryVaultDepositAddressResponse, error) {
	address, err := common.NewAddress(req.Address)
	if err != nil {
		return nil, err
	}
	record, err := qs.mgr.Keeper().GetShielderDepositAddress(ctx, address)
	if err != nil {
		return nil, err
	}
	if record.Address.IsEmpty() {
		return nil, errors.New("vault deposit address not found")
	}
	return &types.QueryVaultDepositAddressResponse{
		Address:        record.Address.String(),
		VaultPubKey:    record.VaultPubKey.String(),
		PathIndex:      record.PathIndex,
		Owner:          record.Owner.String(),
		OperatorPubKey: record.OperatorPubKey.String(),
		NodePubKey:     record.NodePubKey,
		AuctionId:      record.AuctionID,
		CreatedHeight:  record.CreatedHeight,
	}, nil
}

func (qs queryServer) queryShielderFeeEntitlement(ctx cosmos.Context, req *types.QueryShielderFeeEntitlementRequest) (*types.QueryShielderFeeEntitlementResponse, error) {
	bond, err := qs.mgr.Keeper().GetShielderNodeBond(ctx, req.NodePubKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bond.NodePubKey) == "" {
		return nil, errors.New("shielder node bond not found")
	}
	pool, err := qs.mgr.Keeper().GetShielderFeePool(ctx)
	if err != nil {
		return nil, err
	}
	accrued := pool.FeePerSlotShare
	claimable := uint64(0)
	if bond.FeeShareActive && accrued > bond.FeeDebtSats && bond.PendingFeeDepositID.IsEmpty() {
		claimable = accrued - bond.FeeDebtSats
	}
	return &types.QueryShielderFeeEntitlementResponse{
		NodePubKey:          bond.NodePubKey,
		ClaimableSats:       claimable,
		AccruedSats:         accrued,
		FeeDebtSats:         bond.FeeDebtSats,
		FeePerSlotShare:     pool.FeePerSlotShare,
		FeeShareActive:      bond.FeeShareActive,
		PendingFeeDepositId: bond.PendingFeeDepositID.String(),
	}, nil
}

func extractBlockHeight(ctx cosmos.Context, heightStr string) (int64, error) {
	if len(heightStr) == 0 {
		return -1, errors.New("block height not provided")
	}
	height, err := strconv.ParseInt(heightStr, 0, 64)
	if err != nil {
		ctx.Logger().Error("fail to parse block height", "error", err)
		return -1, fmt.Errorf("fail to parse block height: %w", err)
	}
	if height > ctx.BlockHeight() {
		return -1, fmt.Errorf("block height not available yet")
	}
	return height, nil
}

func (qs queryServer) queryKeygen(ctx cosmos.Context, req *types.QueryKeygenRequest) (*types.QueryKeygenResponse, error) {
	height, err := extractBlockHeight(ctx, req.Height)
	if err != nil {
		return nil, err
	}

	keygenBlock, err := qs.mgr.Keeper().GetKeygenBlock(ctx, height)
	if err != nil {
		ctx.Logger().Error("fail to get keygen block", "error", err)
		return nil, fmt.Errorf("fail to get keygen block: %w", err)
	}

	if len(req.PubKey) > 0 {
		var pk common.PubKey
		pk, err = common.NewPubKey(req.PubKey)
		if err != nil {
			ctx.Logger().Error("fail to parse pubkey", "error", err)
			return nil, fmt.Errorf("fail to parse pubkey: %w", err)
		}
		// only return those keygen contains the request pub key
		newKeygenBlock := NewKeygenBlock(keygenBlock.Height)
		for _, keygen := range keygenBlock.Keygens {
			if keygen.GetMembers().Contains(pk) {
				newKeygenBlock.Keygens = append(newKeygenBlock.Keygens, keygen)
			}
		}
		keygenBlock = newKeygenBlock
	}

	buf, err := json.Marshal(keygenBlock)
	if err != nil {
		ctx.Logger().Error("fail to marshal keygen block to json", "error", err)
		return nil, fmt.Errorf("fail to marshal keygen block to json: %w", err)
	}
	// TODO: confirm this signing mode which is only for ledger devices.
	// Not applicable if ledger devices will never be used.
	// SIGN_MODE_LEGACY_AMINO_JSON will be removed in the future for SIGN_MODE_TEXTUAL
	signingMode := signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
	sig, _, err := qs.kbs.Keybase.Sign("thornado", buf, signingMode)
	if err != nil {
		ctx.Logger().Error("fail to sign keygen", "error", err)
		return nil, fmt.Errorf("fail to sign keygen: %w", err)
	}

	return &types.QueryKeygenResponse{
		KeygenBlock: &keygenBlock,
		Signature:   base64.StdEncoding.EncodeToString(sig),
	}, nil
}

func (qs queryServer) queryKeysign(ctx cosmos.Context, heightStr, pubKey string) (*types.QueryKeysignResponse, error) {
	height, err := extractBlockHeight(ctx, heightStr)
	if err != nil {
		return nil, err
	}

	pk := common.EmptyPubKey
	if len(pubKey) > 0 {
		pk, err = common.NewPubKey(pubKey)
		if err != nil {
			ctx.Logger().Error("fail to parse pubkey", "error", err)
			return nil, fmt.Errorf("fail to parse pubkey: %w", err)
		}
	}

	txs, err := qs.mgr.Keeper().GetTxOut(ctx, height)
	if err != nil {
		ctx.Logger().Error("fail to get tx out array from key value store", "error", err)
		return nil, fmt.Errorf("fail to get tx out array from key value store: %w", err)
	}

	if !pk.IsEmpty() {
		newTxs := &TxOut{
			Height: txs.Height,
		}
		for _, tx := range txs.TxArray {
			if pk.Equals(tx.VaultPubKey) {
				zero := cosmos.ZeroUint()
				if tx.CloutSpent == nil {
					tx.CloutSpent = &zero
				}
				newTxs.TxArray = append(newTxs.TxArray, tx)
			}
		}
		txs = newTxs
	}

	buf, err := json.Marshal(txs)
	if err != nil {
		ctx.Logger().Error("fail to marshal keysign block to json", "error", err)
		return nil, fmt.Errorf("fail to marshal keysign block to json: %w", err)
	}
	// TODO: confirm this signing mode which is only for ledger devices.
	// Not applicable if ledger devices will never be used.
	// SIGN_MODE_LEGACY_AMINO_JSON will be removed in the future for SIGN_MODE_TEXTUAL
	signingMode := signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
	sig, _, err := qs.kbs.Keybase.Sign("thornado", buf, signingMode)
	if err != nil {
		ctx.Logger().Error("fail to sign keysign", "error", err)
		return nil, fmt.Errorf("fail to sign keysign: %w", err)
	}

	return &types.QueryKeysignResponse{
		Keysign:   txs,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// queryOutQueue - iterates over txout, counting how many transactions are waiting to be sent
func (qs queryServer) queryLastBlockHeights(ctx cosmos.Context, chain string) (*types.QueryLastBlocksResponse, error) {
	var chains common.Chains
	if len(chain) > 0 {
		var err error
		chain, err := common.NewChain(chain)
		if err != nil {
			ctx.Logger().Error("fail to parse chain", "error", err, "chain", chain)
			return nil, fmt.Errorf("fail to retrieve chain: %w", err)
		}
		chains = append(chains, chain)
	} else {
		asgards, err := qs.mgr.Keeper().GetAsgardVaultsByStatus(ctx, ActiveVault)
		if err != nil {
			return nil, fmt.Errorf("fail to get active asgard: %w", err)
		}
		for _, vault := range asgards {
			chains = vault.GetChains().Distinct()
			break
		}
	}
	var result []*types.ChainsLastBlock
	for _, c := range chains {
		if c == common.Thornado {
			continue
		}
		chainHeight, err := qs.mgr.Keeper().GetLastChainHeight(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("fail to get last chain height: %w", err)
		}

		signed, err := qs.mgr.Keeper().GetLastSignedHeight(ctx)
		if err != nil {
			return nil, fmt.Errorf("fail to get last sign height: %w", err)
		}
		result = append(result, &types.ChainsLastBlock{
			Chain:          c.String(),
			LastObservedIn: chainHeight,
			LastSignedOut:  signed,
			Thornado:       ctx.BlockHeight(),
		})
	}

	return &types.QueryLastBlocksResponse{LastBlocks: result}, nil
}

func (qs queryServer) queryConstantValues(_ cosmos.Context, _ *types.QueryConstantValuesRequest) (*types.QueryConstantValuesResponse, error) {
	constAccessor := qs.mgr.GetConstants()
	cv := constAccessor.GetConstantValsByKeyname()

	proto := types.QueryConstantValuesResponse{}
	// analyze-ignore(map-iteration)
	for k, v := range cv.Int64Values {
		proto.Int_64Values = append(proto.Int_64Values, &types.Int64Constants{
			Name:  k,
			Value: v,
		})
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.BoolValues {
		proto.BoolValues = append(proto.BoolValues, &types.BoolConstants{
			Name:  k,
			Value: v,
		})
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.StringValues {
		proto.StringValues = append(proto.StringValues, &types.StringConstants{
			Name:  k,
			Value: v,
		})
	}

	return &proto, nil
}

func (qs queryServer) queryVersion(ctx cosmos.Context, _ *types.QueryVersionRequest) (*types.QueryVersionResponse, error) {
	v, hasV := qs.mgr.Keeper().GetVersionWithCtx(ctx)
	if !hasV {
		// re-compute version if not stored
		v = qs.mgr.Keeper().GetLowestActiveVersion(ctx)
	}

	minJoinLast, minJoinLastChangedHeight := qs.mgr.Keeper().GetMinJoinLast(ctx)

	ver := types.QueryVersionResponse{
		Current:         v.String(),
		Next:            minJoinLast.String(),
		NextSinceHeight: minJoinLastChangedHeight, // omitted if 0
		Querier:         constants.SWVersion.String(),
	}
	return &ver, nil
}

func (qs queryServer) queryUpgradeProposals(ctx cosmos.Context, req *types.QueryUpgradeProposalsRequest) (*types.QueryUpgradeProposalsResponse, error) {
	res := make([]*types.QueryUpgradeProposalResponse, 0)

	k := qs.mgr.Keeper()
	iter := k.GetUpgradeProposalIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key, value := iter.Key(), iter.Value()

		nameSplit := strings.Split(string(key), "/")
		name := nameSplit[len(nameSplit)-1]

		var upgrade types.Upgrade
		if err := k.Cdc().Unmarshal(value, &upgrade); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proposed upgrade: %w", err)
		}

		p, err := qs.queryUpgradeProposal(ctx, &types.QueryUpgradeProposalRequest{Name: name})
		if err != nil {
			return nil, fmt.Errorf("failed to query upgrade proposal: %w", err)
		}

		res = append(res, p)
	}

	return &types.QueryUpgradeProposalsResponse{UpgradeProposals: res}, nil
}

func (qs queryServer) queryUpgradeProposal(ctx cosmos.Context, req *types.QueryUpgradeProposalRequest) (*types.QueryUpgradeProposalResponse, error) {
	if len(req.Name) == 0 {
		return nil, errors.New("upgrade name not provided")
	}

	k := qs.mgr.Keeper()

	proposal, err := k.GetProposedUpgrade(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("fail to get upgrade proposal: %w", err)
	}

	if proposal == nil {
		return nil, fmt.Errorf("upgrade proposal not found: %s", req.Name)
	}

	uq, err := keeperv1.UpgradeApprovedByMajority(ctx, k, req.Name)
	if err != nil {
		return nil, fmt.Errorf("fail to check upgrade approval: %w", err)
	}

	approval := big.NewRat(int64(uq.ApprovingVals), int64(uq.TotalActive))
	approvalFlt, _ := approval.Float64()
	approvalStr := fmt.Sprintf("%.2f", approvalFlt*100)

	vtq := int64(uq.NeededForQuorum)

	// gather the approvers and rejecters
	approvers := []string{}
	rejecters := []string{}
	iter := k.GetUpgradeVoteIterator(ctx, req.Name)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key, value := iter.Key(), iter.Value()
		addr := cosmos.AccAddress(bytes.TrimPrefix(key, []byte(keeperv1.VotePrefix(req.Name))))
		if bytes.Equal(value, []byte{0x1}) {
			approvers = append(approvers, addr.String())
		} else {
			rejecters = append(rejecters, addr.String())
		}
	}

	res := types.QueryUpgradeProposalResponse{
		Name:            req.Name,
		Height:          proposal.Height,
		Info:            proposal.Info,
		Approved:        uq.Approved,
		ApprovedPercent: approvalStr,
		NodesToQuorum:   vtq,
		Approvers:       approvers,
		Rejecters:       rejecters,
	}

	return &res, nil
}

func (qs queryServer) queryAccount(ctx cosmos.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	b32, err := cosmos.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, fmt.Errorf("fail to parse address: %w", err)
	}
	acc := qs.mgr.Keeper().GetAccount(ctx, b32)

	var pubKey string
	pk := acc.GetPubKey()
	if pk != nil {
		pubKey = pk.String()
	}

	return &types.QueryAccountResponse{
		Address:       acc.GetAddress().String(),
		PubKey:        pubKey,
		AccountNumber: strconv.FormatUint(acc.GetAccountNumber(), 10),
		Sequence:      strconv.FormatUint(acc.GetSequence(), 10),
	}, nil
}

func (qs queryServer) queryBalances(ctx cosmos.Context, req *types.QueryBalancesRequest) (*types.QueryBalancesResponse, error) {
	b32, err := cosmos.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, fmt.Errorf("fail to parse address: %w", err)
	}
	b := qs.mgr.Keeper().GetBalance(ctx, b32)

	balances := make([]*types.Amount, len(b))
	for i, bal := range b {
		balances[i] = &types.Amount{
			Denom:  bal.Denom,
			Amount: bal.Amount.String(),
		}
	}

	return &types.QueryBalancesResponse{
		Balances: balances,
	}, nil
}

func (qs queryServer) queryUpgradeVotes(ctx cosmos.Context, req *types.QueryUpgradeVotesRequest) (*types.QueryUpgradeVotesResponse, error) {
	if len(req.Name) == 0 {
		return nil, errors.New("upgrade name not provided")
	}

	prefix := []byte(keeperv1.VotePrefix(req.Name))
	res := make([]*types.UpgradeVote, 0)

	iter := qs.mgr.Keeper().GetUpgradeVoteIterator(ctx, req.Name)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key, value := iter.Key(), iter.Value()

		addr := cosmos.AccAddress(bytes.TrimPrefix(key, prefix))

		var vote string
		if bytes.Equal(value, []byte{0x1}) {
			vote = "approve"
		} else {
			vote = "reject"
		}

		v := types.UpgradeVote{
			NodeAddress: addr.String(),
			Vote:        vote,
		}

		res = append(res, &v)
	}

	return &types.QueryUpgradeVotesResponse{UpgradeVotes: res}, nil
}

func (qs queryServer) queryMimirWithKey(ctx cosmos.Context, req *types.QueryMimirWithKeyRequest) (*types.QueryMimirWithKeyResponse, error) {
	if len(req.Key) == 0 {
		return nil, fmt.Errorf("no mimir key")
	}

	v, err := qs.mgr.Keeper().GetMimir(ctx, req.Key)
	if err != nil {
		return nil, fmt.Errorf("fail to get mimir with key:%s, err : %w", req.Key, err)
	}
	return &types.QueryMimirWithKeyResponse{
		Value: v,
	}, nil
}

func (qs queryServer) queryMimirValues(ctx cosmos.Context, _ *types.QueryMimirValuesRequest) (*types.QueryMimirValuesResponse, error) {
	resp := types.QueryMimirValuesResponse{
		Mimirs: make([]*types.Mimir, 0),
	}

	// collect all keys with set values, not displaying those with votes but no set value
	keeper := qs.mgr.Keeper()
	iter := keeper.GetMimirIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key := strings.TrimPrefix(string(iter.Key()), "mimir//")
		value, err := keeper.GetMimir(ctx, key)
		if err != nil {
			ctx.Logger().Error("fail to get mimir value", "error", err)
			continue
		}
		if value < 0 {
			ctx.Logger().Error("negative mimir value set", "key", key, "value", value)
			continue
		}
		resp.Mimirs = append(resp.Mimirs, &types.Mimir{
			Key:   key,
			Value: value,
		})
	}

	return &resp, nil
}

func (qs queryServer) queryMimirAdminValues(ctx cosmos.Context, _ *types.QueryMimirAdminValuesRequest) (*types.QueryMimirAdminValuesResponse, error) {
	resp := types.QueryMimirAdminValuesResponse{
		AdminMimirs: make([]*types.Mimir, 0),
	}

	iter := qs.mgr.Keeper().GetMimirIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		value := types.ProtoInt64{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &value); err != nil {
			ctx.Logger().Error("fail to unmarshal mimir value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal mimir value: %w", err)
		}
		k := strings.TrimPrefix(string(iter.Key()), "mimir//")
		resp.AdminMimirs = append(resp.AdminMimirs, &types.Mimir{
			Key:   k,
			Value: value.GetValue(),
		})

	}
	return &resp, nil
}

func (qs queryServer) queryMimirNodesAllValues(ctx cosmos.Context, _ *types.QueryMimirNodesAllValuesRequest) (*types.QueryMimirNodesAllValuesResponse, error) {
	resp := types.QueryMimirNodesAllValuesResponse{
		Mimirs: make([]types.NodeMimir, 0),
	}

	iter := qs.mgr.Keeper().GetNodeMimirIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		m := NodeMimirs{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &m); err != nil {
			ctx.Logger().Error("fail to unmarshal node mimir value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal node mimir value: %w", err)
		}
		resp.Mimirs = append(resp.Mimirs, m.Mimirs...)
	}

	return &resp, nil
}

func (qs queryServer) queryMimirNodesValues(ctx cosmos.Context, _ *types.QueryMimirNodesValuesRequest) (*types.QueryMimirNodesValuesResponse, error) {
	activeNodes, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to fetch active node accounts", "error", err)
		return nil, fmt.Errorf("fail to fetch active node accounts: %w", err)
	}
	active := activeNodes.GetNodeAddresses()

	resp := types.QueryMimirNodesValuesResponse{
		Mimirs: make([]*types.Mimir, 0),
	}

	iter := qs.mgr.Keeper().GetNodeMimirIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		mimirs := NodeMimirs{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &mimirs); err != nil {
			ctx.Logger().Error("fail to unmarshal node mimir value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal node mimir value: %w", err)
		}
		k := strings.TrimPrefix(string(iter.Key()), "nodemimir//")
		if v, ok := mimirs.HasSuperMajority(k, active); ok {
			resp.Mimirs = append(resp.Mimirs, &types.Mimir{
				Key:   k,
				Value: v,
			})
		}
	}

	return &resp, nil
}

func (qs queryServer) queryMimirNodeValues(ctx cosmos.Context, req *types.QueryMimirNodeValuesRequest) (*types.QueryMimirNodeValuesResponse, error) {
	acc, err := cosmos.AccAddressFromBech32(req.Address)
	if err != nil {
		ctx.Logger().Error("fail to parse thor address", "error", err)
		return nil, fmt.Errorf("fail to parse thor address: %w", err)
	}

	resp := types.QueryMimirNodeValuesResponse{
		NodeMimirs: make([]*types.Mimir, 0),
	}

	iter := qs.mgr.Keeper().GetNodeMimirIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		mimirs := NodeMimirs{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &mimirs); err != nil {
			ctx.Logger().Error("fail to unmarshal node mimir v2 value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal node mimir value: %w", err)
		}

		k := strings.TrimPrefix(string(iter.Key()), "nodemimir//")
		if v, ok := mimirs.Get(k, acc); ok {
			resp.NodeMimirs = append(resp.NodeMimirs, &types.Mimir{
				Key:   k,
				Value: v,
			})
		}
	}

	return &resp, nil
}

func (qs queryServer) queryBan(ctx cosmos.Context, req *types.QueryBanRequest) (*types.BanVoter, error) {
	if len(req.Address) == 0 {
		return nil, errors.New("node address not available")
	}
	addr, err := cosmos.AccAddressFromBech32(req.Address)
	if err != nil {
		ctx.Logger().Error("invalid node address", "error", err)
		return nil, fmt.Errorf("invalid node address: %w", err)
	}

	ban, err := qs.mgr.Keeper().GetBanVoter(ctx, addr)
	if err != nil {
		ctx.Logger().Error("fail to get ban voter", "error", err)
		return nil, fmt.Errorf("fail to get ban voter: %w", err)
	}

	return &ban, nil
}

func (qs queryServer) queryTssKeygenMetric(ctx cosmos.Context, req *types.QueryTssKeygenMetricRequest) (*types.QueryTssKeygenMetricResponse, error) {
	if len(req.PubKey) == 0 {
		return nil, fmt.Errorf("missing pub_key parameter")
	}
	pkey, err := common.NewPubKey(req.PubKey)
	if err != nil {
		return nil, fmt.Errorf("fail to parse pubkey(%s) err:%w", req.PubKey, err)
	}

	var result []*types.TssKeygenMetric
	m, err := qs.mgr.Keeper().GetTssKeygenMetric(ctx, pkey)
	if err != nil {
		return nil, fmt.Errorf("fail to get tss keygen metric for pubkey(%s):%w", pkey, err)
	}
	result = append(result, m)

	return &types.QueryTssKeygenMetricResponse{Metrics: result}, nil
}

func (qs queryServer) queryTssMetric(ctx cosmos.Context, _ *types.QueryTssMetricRequest) (*types.QueryTssMetricResponse, error) {
	var pubKeys common.PubKeys
	// get all active asgard
	vaults, err := qs.mgr.Keeper().GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return nil, fmt.Errorf("fail to get active asgards:%w", err)
	}
	for _, v := range vaults {
		pubKeys = append(pubKeys, v.PubKey)
	}
	var keygenMetrics []*types.TssKeygenMetric
	for _, pkey := range pubKeys {
		var m *types.TssKeygenMetric
		m, err = qs.mgr.Keeper().GetTssKeygenMetric(ctx, pkey)
		if err != nil {
			return nil, fmt.Errorf("fail to get tss keygen metric for pubkey(%s):%w", pkey, err)
		}
		if len(m.NodeTssTimes) == 0 {
			continue
		}
		keygenMetrics = append(keygenMetrics, m)
	}
	keysignMetric, err := qs.mgr.Keeper().GetLatestTssKeysignMetric(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get keysign metric:%w", err)
	}

	return &types.QueryTssMetricResponse{
		Keygen:  keygenMetrics,
		Keysign: keysignMetric,
	}, nil
}

func (qs queryServer) queryInvariants(_ cosmos.Context, _ *types.QueryInvariantsRequest) (*types.QueryInvariantsResponse, error) {
	result := types.QueryInvariantsResponse{}
	for _, route := range qs.mgr.Keeper().InvariantRoutes() {
		result.Invariants = append(result.Invariants, route.Route)
	}
	return &result, nil
}

func (qs queryServer) queryInvariant(ctx cosmos.Context, req *types.QueryInvariantRequest) (*types.QueryInvariantResponse, error) {
	if len(req.Path) < 1 {
		return nil, fmt.Errorf("invalid path: %v", req.Path)
	}
	for _, route := range qs.mgr.Keeper().InvariantRoutes() {
		if strings.EqualFold(route.Route, req.Path) {
			msg, broken := route.Invariant(ctx)
			result := types.QueryInvariantResponse{
				Invariant: route.Route,
				Broken:    broken,
				Msg:       msg,
			}
			return &result, nil
		}
	}
	return nil, fmt.Errorf("invariant not registered: %s", req.Path)
}

func (qs queryServer) queryBlock(ctx cosmos.Context, req *types.QueryBlockRequest) (*types.QueryBlockResponse, error) {
	initTendermintOnce.Do(initTendermint)
	height := ctx.BlockHeight()
	if parsed, err := strconv.ParseInt(req.Height, 10, 64); err == nil {
		height = parsed
	}

	// get the block and results from tendermint rpc
	block, err := tendermintClient.Block(ctx.Context(), &height)
	if err != nil {
		return nil, fmt.Errorf("fail to get block from tendermint rpc: %w", err)
	}
	results, err := tendermintClient.BlockResults(ctx.Context(), &height)
	if err != nil {
		return nil, fmt.Errorf("fail to get block results from tendermint rpc: %w", err)
	}

	res := types.QueryBlockResponse{
		Id: &types.BlockResponseId{
			Hash: block.BlockID.Hash.String(),
			Parts: &types.BlockResponseIdParts{
				Total: int64(block.BlockID.PartSetHeader.Total),
				Hash:  block.BlockID.PartSetHeader.Hash.String(),
			},
		},
		Header: &types.BlockResponseHeader{
			Version: &types.BlockResponseHeaderVersion{
				Block: strconv.FormatUint(block.Block.Version.Block, 10),
				App:   strconv.FormatUint(block.Block.Version.App, 10),
			},
			ChainId: block.Block.ChainID,
			Height:  block.Block.Height,
			Time:    block.Block.Time.Format(time.RFC3339Nano),
			LastBlockId: &types.BlockResponseId{
				Hash: block.Block.LastBlockID.Hash.String(),
				Parts: &types.BlockResponseIdParts{
					Total: int64(block.Block.LastBlockID.PartSetHeader.Total),
					Hash:  block.Block.LastBlockID.PartSetHeader.Hash.String(),
				},
			},
			LastCommitHash:     block.Block.LastCommitHash.String(),
			DataHash:           block.Block.DataHash.String(),
			ValidatorsHash:     block.Block.ValidatorsHash.String(),
			NextValidatorsHash: block.Block.NextValidatorsHash.String(),
			ConsensusHash:      block.Block.ConsensusHash.String(),
			AppHash:            block.Block.AppHash.String(),
			LastResultsHash:    block.Block.LastResultsHash.String(),
			EvidenceHash:       block.Block.EvidenceHash.String(),
			ProposerAddress:    block.Block.ProposerAddress.String(),
		},
		Txs: make([]*types.QueryBlockTx, len(block.Block.Txs)),
	}

	// parse the events
	for _, event := range results.FinalizeBlockEvents {
		foundMode := false
		for _, attr := range event.Attributes {
			if attr.Key == "mode" {
				if attr.Value == "BeginBlock" {
					res.BeginBlockEvents = append(res.BeginBlockEvents, blockEvent(sdk.Event(event)))
					foundMode = true
				}
				if attr.Value == "EndBlock" {
					res.EndBlockEvents = append(res.EndBlockEvents, blockEvent(sdk.Event(event)))
					foundMode = true
				}
				continue
			}
		}
		if !foundMode {
			res.FinalizeBlockEvents = append(res.FinalizeBlockEvents, blockEvent(sdk.Event(event)))
		}
	}

	for i, tx := range block.Block.Txs {
		// decode the protobuf and encode to json

		dtx, err := qs.txConfig.TxDecoder()(tx)
		if err != nil {
			return nil, fmt.Errorf("fail to decode tx: %w", err)
		}

		etx, err := qs.txConfig.TxJSONEncoder()(dtx)
		if err != nil {
			return nil, fmt.Errorf("fail to encode tx: %w", err)
		}

		resultTx := results.TxsResults[i]

		// Attempt to unmarshal the tx result's data, if it of type MsgEmpty, don't include it as it's not useful
		var emptyMsg types.MsgEmpty
		err = qs.mgr.cdc.UnmarshalInterface(resultTx.Data, &emptyMsg)
		if err == nil {
			resultTx.Data = nil
		}

		res.Txs[i] = &types.QueryBlockTx{
			Tx:   etx,
			Hash: strings.ToUpper(hex.EncodeToString(tx.Hash())),
			Result: &types.BlockTxResult{
				Code:      int64(resultTx.Code),
				Data:      string(resultTx.Data),
				Log:       resultTx.Log,
				Info:      resultTx.Info,
				GasWanted: strconv.FormatInt(resultTx.GasWanted, 10),
				GasUsed:   strconv.FormatInt(resultTx.GasUsed, 10),
				Events:    make([]*types.BlockEvent, len(resultTx.Events)),
			},
		}

		for j, event := range resultTx.Events {
			res.Txs[i].Result.Events[j] = blockEvent(sdk.Event(event))
		}
	}

	return &res, nil
}

// -------------------------------------------------------------------------------------
// Generic Helpers
// -------------------------------------------------------------------------------------

func castObservedTx(observedTx ObservedTx) types.QueryObservedTx {
	// Only display the Status if it is "done", not if "incomplete".
	status := ""
	if observedTx.Status != common.Status_incomplete {
		status = observedTx.Status.String()
	}
	return types.QueryObservedTx{
		Tx:                    observedTx.Tx,
		Status:                status,
		OutHashes:             observedTx.OutHashes,
		BlockHeight:           observedTx.BlockHeight,
		Signers:               observedTx.Signers,
		ObservedPubKey:        observedTx.ObservedPubKey,
		KeysignMs:             observedTx.KeysignMs,
		FinaliseHeight:        observedTx.FinaliseHeight,
		Aggregator:            observedTx.Aggregator,
		AggregatorTarget:      observedTx.AggregatorTarget,
		AggregatorTargetLimit: observedTx.AggregatorTargetLimit,
	}
}

func castVaultRouters(chainContracts []ChainContract) []*types.VaultRouter {
	// Leave this nil (null rather than []) if the source is nil.
	if chainContracts == nil {
		return nil
	}

	routers := make([]*types.VaultRouter, len(chainContracts))
	for i := range chainContracts {
		routers[i] = &types.VaultRouter{
			Chain:  chainContracts[i].Chain.String(),
			Router: chainContracts[i].Router.String(),
		}
	}
	return routers
}

func blockEvent(e sdk.Event) *types.BlockEvent {
	event := types.BlockEvent{}
	event.EventKvPair = append(event.EventKvPair, &types.EventKeyValuePair{
		Key:   "type",
		Value: e.Type,
	})

	for _, a := range e.Attributes {
		event.EventKvPair = append(event.EventKvPair, &types.EventKeyValuePair{
			Key:   a.Key,
			Value: a.Value,
		})
	}
	return &event
}

func eventMap(e sdk.Event) map[string]string {
	m := map[string]string{}
	m["type"] = e.Type
	for _, a := range e.Attributes {
		m[a.Key] = a.Value
	}
	return m
}

func simulate(ctx cosmos.Context, mgr Manager, msg sdk.Msg) (sdk.Events, error) {
	return nil, errors.New("swap simulation is not part of the Thornado custody fork")
}

// runePerDollarIgnoreHalt mirrors keeper.RunePerDollar, ignoring halts by using
// dollarsPerRuneIgnoreHalt to return the last known price instead of "0"
func runePerDollarIgnoreHalt(ctx cosmos.Context, k keeper.Keeper) cosmos.Uint {
	runePerDollar := dollarsPerRuneIgnoreHalt(ctx, k)

	one := cosmos.NewUint(common.One)

	return common.GetUncappedShare(one, runePerDollar, one)
}

// dollarsPerRuneIgnoreHalt mirrors keeper.DollarsPerRune, but ignoring halts if all
// anchor chains are unavailable with them. This is used for the TOR price on pools to
// ensure a best effort price is returned whenever possible instead of zero.
func dollarsPerRuneIgnoreHalt(ctx cosmos.Context, k keeper.Keeper) cosmos.Uint {
	return cosmos.ZeroUint()
}

func (qs queryServer) queryEip712TypedData(_ cosmos.Context, req *types.QueryEip712TypedDataRequest) (*types.QueryEip712TypedDataResponse, error) {
	typedData, err := eip712.GetEIP712TypedDataForMsg(req.SignBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to EIP-712 typed data: %w", err)
	}

	// Convert to JSON for output
	data, err := json.Marshal(typedData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EIP-712 typed data: %w", err)
	}
	return &types.QueryEip712TypedDataResponse{TypedData: string(data)}, nil
}

// querySupply returns the RUNE supply breakdown.
func (qs queryServer) queryContractInfo(ctx cosmos.Context, req *types.QueryContractInfoRequest) (*types.QueryContractInfoResponse, error) {
	return nil, errors.New("wasm contract queries are not part of the Thornado custody fork")
}

func (qs queryServer) queryContractInfos(ctx cosmos.Context, req *types.QueryContractInfosRequest) (*types.QueryContractInfosResponse, error) {
	return nil, errors.New("wasm contract queries are not part of the Thornado custody fork")
}
