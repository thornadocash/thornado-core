package thornado

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	tmhttp "github.com/cometbft/cometbft/rpc/client/http"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
	initManager        = func(_ cosmos.Context, _ *Mgrs) {}
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
		moduleName = BaseName
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
		Addresses:             getVaultChainAddresses(ctx, v),
		Frozen:                v.Frozen,
	}
	return &resp, nil
}

func (qs queryServer) queryBaseVaults(ctx cosmos.Context, _ *types.QueryBaseVaultsRequest) (*types.QueryBaseVaultsResponse, error) {
	vaults, err := qs.mgr.Keeper().GetBaseVaults(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get base vaults: %w", err)
	}

	var vaultsWithFunds []*types.QueryVaultResponse
	for _, vault := range vaults {
		if vault.Status == InactiveVault {
			continue
		}
		if !vault.IsBase() {
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
				Frozen:                vault.Frozen,
				Addresses:             getVaultChainAddresses(ctx, vault),
			})
		}
	}

	return &types.QueryBaseVaultsResponse{BaseVaults: vaultsWithFunds}, nil
}

func getVaultChainAddresses(ctx cosmos.Context, vault Vault) []*types.VaultAddress {
	var result []*types.VaultAddress
	allChains := append(vault.GetChains(), common.BTCChain)
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
	resp.Base = make([]*types.VaultInfo, 0)
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
		if vault.IsBase() {
			switch vault.Status {
			case ActiveVault, RetiringVault:
				resp.Base = append(resp.Base, &types.VaultInfo{
					PubKey:      vault.PubKey.String(),
					PubKeyEddsa: vault.PubKeyEddsa.String(),
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
						Membership:  vault.Membership,
					})
				}
			}
		}
	}
	return &resp, nil
}

func (qs queryServer) queryNetworkFee(ctx cosmos.Context, _ *types.QueryNetworkFeeRequest) (*types.NetworkFee, error) {
	fee, err := qs.mgr.Keeper().GetNetworkFee(ctx, common.BTCChain)
	if err != nil {
		return nil, fmt.Errorf("fail to get BTC network fee: %w", err)
	}
	return &fee, nil
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

	penaltyPts, err := qs.mgr.Keeper().GetNodeAccountPenaltyPoints(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("fail to get node penalty points: %w", err)
	}
	jail, err := qs.mgr.Keeper().GetNodeAccountJail(ctx, nodeAcc.NodeAddress)
	if err != nil {
		return nil, fmt.Errorf("fail to get node jail: %w", err)
	}
	operatorAddress, err := nodeOperatorAddress(ctx, qs.mgr.Keeper(), nodeAcc)
	if err != nil {
		return nil, fmt.Errorf("fail to get node operator address: %w", err)
	}

	result := types.QueryNodeResponse{
		NodeAddress: nodeAcc.NodeAddress.String(),
		Status:      nodeAcc.Status.String(),
		PubKeySet: common.PubKeySet{
			Secp256k1: common.PubKey(nodeAcc.PubKeySet.Secp256k1.String()),
		},
		NodeConsPubKey:      nodeAcc.NodeConsPubKey,
		ActiveBlockHeight:   nodeAcc.ActiveBlockHeight,
		StatusSince:         nodeAcc.StatusSince,
		NodeOperatorAddress: operatorAddress.String(),
		TotalBond:           nodeAcc.Bond.String(),
		SignerMembership:    nodeAcc.GetSignerMembership().Strings(),
		RequestedToLeave:    nodeAcc.RequestedToLeave,
		ForcedToLeave:       nodeAcc.ForcedToLeave,
		LeaveHeight:         int64(nodeAcc.LeaveScore), // OpenAPI can only represent uint64 as int64
		Maintenance:         nodeAcc.Maintenance,
		MissingBlocks:       int64(nodeAcc.MissingBlocks),
		IpAddress:           nodeAcc.IPAddress,
		Version:             nodeAcc.GetVersion().String(),
	}
	result.PeerId = getPeerIDFromPubKey(nodeAcc.PubKeySet.Secp256k1)
	result.PenaltyPoints = penaltyPts

	result.Jail = &types.NodeJail{
		// Since redundant, leave out the node address
		ReleaseHeight: jail.ReleaseHeight,
		Reason:        jail.Reason,
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
	for _, activeNode := range active {
		if !nodeAccounts.Contains(activeNode) {
			nodeAccounts = append(nodeAccounts, activeNode)
		}
	}

	result := make([]*types.QueryNodeResponse, len(nodeAccounts))
	for i, na := range nodeAccounts {
		if na.RequestedToLeave && na.Bond.LTE(cosmos.NewUint(common.One)) {
			// ignore the node , it left and also has very little bond
			// Set the default display for fields which would otherwise be "".
			result[i] = &types.QueryNodeResponse{
				Status:          types.NodeStatus_Unknown.String(),
				TotalBond:       cosmos.ZeroUint().String(),
				Version:         semver.MustParse("0.0.0").String(),
				PreflightStatus: &types.NodePreflightStatus{Status: types.NodeStatus_Unknown.String()},
			}
			continue
		}

		penaltyPts, err := qs.mgr.Keeper().GetNodeAccountPenaltyPoints(ctx, na.NodeAddress)
		if err != nil {
			return nil, fmt.Errorf("fail to get node penalty points: %w", err)
		}
		operatorAddress, err := nodeOperatorAddress(ctx, qs.mgr.Keeper(), na)
		if err != nil {
			return nil, fmt.Errorf("fail to get node operator address: %w", err)
		}

		result[i] = &types.QueryNodeResponse{
			NodeAddress: na.NodeAddress.String(),
			Status:      na.Status.String(),
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(na.PubKeySet.Secp256k1.String()),
			},
			NodeConsPubKey:      na.NodeConsPubKey,
			ActiveBlockHeight:   na.ActiveBlockHeight,
			StatusSince:         na.StatusSince,
			NodeOperatorAddress: operatorAddress.String(),
			TotalBond:           na.Bond.String(),
			SignerMembership:    na.GetSignerMembership().Strings(),
			RequestedToLeave:    na.RequestedToLeave,
			ForcedToLeave:       na.ForcedToLeave,
			LeaveHeight:         int64(na.LeaveScore), // OpenAPI can only represent uint64 as int64
			Maintenance:         na.Maintenance,
			MissingBlocks:       int64(na.MissingBlocks),
			IpAddress:           na.IPAddress,
			Version:             na.GetVersion().String(),
		}
		result[i].PeerId = getPeerIDFromPubKey(na.PubKeySet.Secp256k1)
		result[i].PenaltyPoints = penaltyPts

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

func (qs queryServer) queryNodeMetrics(ctx cosmos.Context, _ *types.QueryNodeMetricsRequest) (*types.QueryNodeMetricsResponse, error) {
	k := qs.mgr.Keeper()
	nextSlot, err := k.GetNextShielderNodeBondSlot(ctx)
	if err != nil {
		return nil, err
	}
	start := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))
	increment := uint64(k.GetConfigInt64(ctx, constants.Node_BondSlotIncrementSats))
	resp := &types.QueryNodeMetricsResponse{
		NextSlot:                 nextSlot,
		NextSlotBondRequiredSats: shielderBondRequiredSats(ctx, k, nextSlot),
		BondStartAmountSats:      start,
		BondSlotIncrementSats:    increment,
	}

	iter := k.GetShielderNodeBondIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var bond types.ShielderNodeBond
		if err := json.Unmarshal(iter.Value(), &bond); err != nil {
			return nil, err
		}
		resp.PendingBondSats += bond.PendingSats
		resp.ConfirmedBondSats += bond.BondSats
		if bond.Sold {
			resp.SoldSlots++
			continue
		}
		node, err := k.GetNodeAccount(ctx, bond.NodeAddress)
		if err != nil || node.NodeAddress.Empty() {
			continue
		}
		switch node.Status {
		case NodeActive:
			resp.ActiveSlots++
		case NodeStandby:
			resp.StandbySlots++
		}
	}
	return resp, nil
}

func (qs queryServer) queryNodeSlot(ctx cosmos.Context, req *types.QueryNodeSlotRequest) (*types.QueryNodeSlotResponse, error) {
	iter := qs.mgr.Keeper().GetShielderNodeBondIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var bond types.ShielderNodeBond
		if err := json.Unmarshal(iter.Value(), &bond); err != nil {
			return nil, err
		}
		if bond.Slot != req.Slot {
			continue
		}
		resp, err := qs.shielderBondResponse(ctx, bond)
		if err != nil {
			return nil, err
		}
		return &types.QueryNodeSlotResponse{Bond: resp}, nil
	}
	return nil, fmt.Errorf("node slot not found: %d", req.Slot)
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

func selectQueryObservedTxVoter(inboundVoter, outboundVoter ObservedTxVoter) ObservedTxVoter {
	if len(inboundVoter.Txs) == 0 {
		return outboundVoter
	}
	if len(outboundVoter.Txs) == 0 {
		return inboundVoter
	}
	if outboundVoter.FinalisedHeight > 0 || !outboundVoter.Tx.IsEmpty() || len(outboundVoter.Actions) > 0 || len(outboundVoter.OutTxs) > 0 {
		return outboundVoter
	}
	return inboundVoter
}

func (qs queryServer) queryTxVoters(ctx cosmos.Context, req *types.QueryTxVotersRequest) (*types.QueryObservedTxVoter, error) {
	hash, voter, err := extractVoter(ctx, req.TxId, qs.mgr)
	if err != nil {
		return nil, err
	}
	outVoter, err := qs.mgr.Keeper().GetObservedTxOutVoter(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("fail to get observed tx out voter: %w", err)
	}
	voter = selectQueryObservedTxVoter(voter, outVoter)
	if len(voter.Txs) == 0 {
		return nil, fmt.Errorf("tx: %s doesn't exist", hash)
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
				estConfMs -= (currentHeight - countStartHeight) * common.BTCChain.ApproximateBlockMilliseconds()
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

			remainSec := remainBlocks * common.BTCChain.ApproximateBlockMilliseconds() / 1000
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
	// when no TxIn voter don't check TxOut voter, as TxOut BTCChain observation or not matters little to the user once signed and broadcast
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
	// when no TxIn voter don't check TxOut voter, as TxOut BTCChain observation or not matters little to the user once signed and broadcast
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
	outVoter, err := qs.mgr.Keeper().GetObservedTxOutVoter(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("fail to get observed tx out voter: %w", err)
	}
	voter = selectQueryObservedTxVoter(voter, outVoter)
	if len(voter.Txs) == 0 {
		return nil, fmt.Errorf("tx: %s doesn't exist", hash)
	}

	nodeAccounts, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get node accounts: %w", err)
	}
	keysignMetric, err := qs.mgr.Keeper().GetFrostKeysignMetric(ctx, hash)
	if err != nil {
		ctx.Logger().Error("fail to get keysign metrics", "error", err)
	}

	result := types.QueryTxResponse{
		ObservedTx:      castObservedTx(*voter.GetTx(nodeAccounts)),
		ConsensusHeight: voter.Height,
		FinalisedHeight: voter.FinalisedHeight,
		OutboundHeight:  voter.OutboundHeight,
		KeysignMetric:   keysignMetric,
		OutTxs:          voter.OutTxs,
		Actions:         voter.Actions,
		Stages:          newTxStagesResponse(ctx, voter),
		Txs:             castObservedTxs(voter.Txs),
	}
	if voter.Actions != nil {
		result.PlannedOutTxs = make([]*types.PlannedOutTx, len(voter.Actions))
		for i := range voter.Actions {
			result.PlannedOutTxs[i] = &types.PlannedOutTx{
				Chain:     voter.Actions[i].Chain.String(),
				ToAddress: voter.Actions[i].ToAddress.String(),
				Coin:      &voter.Actions[i].Coin,
			}
		}
	}

	return &result, nil
}

type txOutView string

const (
	txOutViewOut      txOutView = "out"
	txOutViewInternal txOutView = "internal"
	txOutViewAll      txOutView = "all"
)

func (qs queryServer) queryTxOut(ctx cosmos.Context, req *types.QueryTxOutRequest, view txOutView) (*types.QueryTxOutResponse, error) {
	height := ctx.BlockHeight()
	if req.Height != "" {
		parsed, err := strconv.ParseInt(req.Height, 10, 64)
		if err != nil {
			return nil, err
		}
		height = parsed
		txOut, err := qs.mgr.Keeper().GetTxOut(ctx, height)
		if err != nil {
			return nil, err
		}
		txOut = filterTxOutView(txOut, view)
		return &types.QueryTxOutResponse{Txout: *txOut, Txouts: []TxOut{*txOut}}, nil
	}

	txOuts := make([]TxOut, 0)
	iterator := qs.mgr.Keeper().GetTxOutIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var txOut TxOut
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iterator.Value(), &txOut); err != nil {
			return nil, err
		}
		if txOut.Status == TxOutStatusPendingBatch || txOut.Status == TxOutStatusPendingSign || txOut.Status == TxOutStatusPendingRetry || view == txOutViewAll {
			txOut = *filterTxOutView(&txOut, view)
			if txOut.IsEmpty() {
				continue
			}
			txOuts = append(txOuts, txOut)
		}
	}
	sort.SliceStable(txOuts, func(i, j int) bool {
		if txOuts[i].Epoch == txOuts[j].Epoch {
			return txOuts[i].Height < txOuts[j].Height
		}
		return txOuts[i].Epoch < txOuts[j].Epoch
	})

	txOut, err := qs.mgr.Keeper().GetTxOut(ctx, height)
	if err != nil {
		return nil, err
	}
	txOut = filterTxOutView(txOut, view)
	return &types.QueryTxOutResponse{Txout: *txOut, Txouts: txOuts}, nil
}

func filterTxOutView(txOut *TxOut, view txOutView) *TxOut {
	if txOut == nil || view == txOutViewAll {
		return txOut
	}
	filtered := &TxOut{
		Height:           txOut.Height,
		Epoch:            txOut.Epoch,
		Status:           txOut.Status,
		SigningLeader:    txOut.SigningLeader,
		SigningAttempt:   txOut.SigningAttempt,
		RetryUntilHeight: txOut.RetryUntilHeight,
	}
	for _, item := range txOut.TxArray {
		switch view {
		case txOutViewOut:
			if types.IsBatchableTxOutType(item.TxType) {
				filtered.TxArray = append(filtered.TxArray, item)
			}
		case txOutViewInternal:
			if types.IsInternalTxOutType(item.TxType) {
				filtered.TxArray = append(filtered.TxArray, item)
			}
		}
	}
	return filtered
}

func (qs queryServer) queryDeposit(ctx cosmos.Context, req *types.QueryDepositRequest) (*types.QueryDepositResponse, error) {
	depositID, err := common.NewTxID(req.DepositId)
	if err != nil {
		return nil, err
	}
	deposit, err := qs.mgr.Keeper().GetDepositRecord(ctx, depositID)
	if err != nil {
		return nil, err
	}
	if deposit.DepositID.IsEmpty() {
		return nil, errors.New("deposit not found")
	}
	return depositQueryResponse(deposit), nil
}

func depositQueryResponse(deposit types.DepositRecord) *types.QueryDepositResponse {
	return &types.QueryDepositResponse{
		DepositId:                deposit.DepositID.String(),
		Owner:                    deposit.Owner.String(),
		AmountSats:               deposit.AmountSats,
		DepositAddress:           deposit.DepositAddress.String(),
		VaultPubKey:              deposit.VaultPubKey.String(),
		DepositPathIndex:         deposit.DepositPathIndex,
		Status:                   deposit.Status,
		Settlement:               deposit.Settlement,
		AuctionId:                deposit.AuctionID,
		NodePubKey:               deposit.NodePubKey,
		NodeSlot:                 deposit.NodeSlot,
		BondConfirmed:            deposit.BondConfirmed,
		CommitmentCount:          0,
		InboundTxId:              deposit.InboundTxID.String(),
		BtcConfirmations:         deposit.BTCConfirmations,
		BtcConfirmationsRequired: deposit.BTCConfirmationsRequired,
		BtcObservedHeight:        deposit.BTCObservedHeight,
		CreatedHeight:            deposit.CreatedHeight,
		ExpiresAtHeight:          deposit.ExpiresAtHeight,
		PurgeAtHeight:            deposit.PurgeAtHeight,
		RefundEligibleHeight:     deposit.RefundEligibleHeight,
		RefundQueuedHeight:       deposit.RefundQueuedHeight,
	}
}

func (qs queryServer) queryShielderRedeem(ctx cosmos.Context, req *types.QueryShielderRedeemRequest) (*types.QueryShielderRedeemResponse, error) {
	withdrawal, err := qs.mgr.Keeper().GetShielderRedeem(ctx, strings.TrimSpace(req.WithdrawalId))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(withdrawal.WithdrawalID) == "" {
		return nil, errors.New("shielder redeem not found")
	}
	return shielderRedeemResponse(ctx, qs.mgr.Keeper(), withdrawal), nil
}

func (qs queryServer) queryShielderNullifier(ctx cosmos.Context, req *types.QueryShielderNullifierRequest) (*types.QueryShielderNullifierResponse, error) {
	nullifier := strings.TrimSpace(req.NullifierHash)
	withdrawal, err := qs.mgr.Keeper().GetShielderRedeemByNullifier(ctx, nullifier)
	if err != nil {
		return nil, err
	}
	return shielderNullifierResponse(ctx, qs.mgr.Keeper(), nullifier, withdrawal), nil
}

func (qs queryServer) queryShielderSync(ctx cosmos.Context, req *types.QueryShielderSyncRequest) (*types.QueryShielderSyncResponse, error) {
	k := qs.mgr.Keeper()

	limit := int(req.GetLimit())
	legacyFullSync := limit == 0 && strings.TrimSpace(req.GetDepositCursor()) == "" && strings.TrimSpace(req.GetNoteCursor()) == "" && strings.TrimSpace(req.GetNullifierCursor()) == ""
	if limit == 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	fromHeight := req.GetFromHeight()
	if fromHeight < 0 {
		fromHeight = 0
	}

	deposits, nextDepositCursor, totalDeposits, moreDeposits, err := queryShielderSyncDeposits(ctx, k, strings.TrimSpace(req.GetDepositCursor()), limit, legacyFullSync, fromHeight)
	if err != nil {
		return nil, err
	}
	notes, nextNoteCursor, totalNotes, moreNotes, err := queryShielderSyncNotes(ctx, k, strings.TrimSpace(req.GetNoteCursor()), limit, legacyFullSync, fromHeight)
	if err != nil {
		return nil, err
	}
	nullifiers, nextNullifierCursor, totalNullifiers, moreNullifiers, err := queryShielderSyncNullifiers(ctx, k, strings.TrimSpace(req.GetNullifierCursor()), limit, legacyFullSync, fromHeight)
	if err != nil {
		return nil, err
	}

	return &types.QueryShielderSyncResponse{
		Notes:               notes,
		Nullifiers:          nullifiers,
		Deposits:            deposits,
		NextDepositCursor:   nextDepositCursor,
		NextNoteCursor:      nextNoteCursor,
		NextNullifierCursor: nextNullifierCursor,
		HasMore:             moreDeposits || moreNotes || moreNullifiers,
		TotalDeposits:       totalDeposits,
		TotalNotes:          totalNotes,
		TotalNullifiers:     totalNullifiers,
		FromHeight:          fromHeight,
	}, nil
}

func queryShielderSyncDeposits(ctx cosmos.Context, k keeper.Keeper, cursor string, limit int, full bool, fromHeight int64) ([]*types.QueryDepositResponse, string, uint64, bool, error) {
	iter := k.GetDepositRecordIteratorAfter(ctx, cursor)
	defer iter.Close()

	deposits := make([]*types.QueryDepositResponse, 0)
	var last string
	more := false
	for ; iter.Valid(); iter.Next() {
		var deposit types.DepositRecord
		if err := json.Unmarshal(iter.Value(), &deposit); err != nil {
			return nil, "", 0, false, err
		}
		key := strings.TrimSpace(deposit.DepositID.String())
		if key == "" {
			continue
		}
		if !syncRecordInBirthday(depositSyncHeight(deposit), fromHeight) {
			continue
		}
		if !full && len(deposits) >= limit {
			more = true
			break
		}
		deposits = append(deposits, depositQueryResponse(deposit))
		last = key
	}
	if !more {
		last = ""
	}
	return deposits, last, shielderSyncPageTotal(len(deposits), more), more, nil
}

func queryShielderSyncNotes(ctx cosmos.Context, k keeper.Keeper, cursor string, limit int, full bool, fromHeight int64) ([]*types.ShielderNoteRecord, string, uint64, bool, error) {
	iter := k.GetShielderNoteRecordIteratorAfter(ctx, cursor)
	defer iter.Close()

	notes := make([]*types.ShielderNoteRecord, 0)
	var last string
	more := false
	for ; iter.Valid(); iter.Next() {
		var record types.StoredShielderNoteRecord
		if err := json.Unmarshal(iter.Value(), &record); err != nil {
			return nil, "", 0, false, err
		}
		key := strings.TrimSpace(record.Commitment)
		if key == "" {
			continue
		}
		if !syncRecordInBirthday(record.CreatedHeight, fromHeight) {
			continue
		}
		if !full && len(notes) >= limit {
			more = true
			break
		}
		notes = append(notes, &types.ShielderNoteRecord{
			Commitment:       strings.TrimSpace(record.Commitment),
			DenominationSats: record.DenominationSats,
			CreatedHeight:    record.CreatedHeight,
		})
		last = key
	}
	if !more {
		last = ""
	}
	return notes, last, shielderSyncPageTotal(len(notes), more), more, nil
}

func queryShielderSyncNullifiers(ctx cosmos.Context, k keeper.Keeper, cursor string, limit int, full bool, fromHeight int64) ([]*types.ShielderSpentNullifier, string, uint64, bool, error) {
	iter := k.GetShielderNullifierIteratorAfter(ctx, cursor)
	defer iter.Close()

	records := make([]shielderSyncNullifierRecord, 0)
	txOutLookupKeys := make(map[string]bool)
	var last string
	more := false
	for ; iter.Valid(); iter.Next() {
		nullifier := strings.TrimLeft(strings.TrimPrefix(string(iter.Key()), "shielder_nullifier/"), "/")
		if nullifier == "" {
			continue
		}
		record, err := decodeShielderSyncNullifierRecord(nullifier, iter.Value())
		if err != nil {
			return nil, "", 0, false, err
		}
		withdrawal := types.ShielderRedeem{}
		if record.CreatedHeight == 0 && fromHeight > 0 {
			withdrawal, _ = k.GetShielderRedeem(ctx, strings.TrimSpace(record.WithdrawalID))
			record.CreatedHeight = withdrawal.RequestedHeight
		}
		if !syncRecordInBirthday(record.CreatedHeight, fromHeight) {
			continue
		}
		if !full && len(records) >= limit {
			more = true
			break
		}
		if strings.TrimSpace(withdrawal.WithdrawalID) == "" {
			withdrawal, _ = k.GetShielderRedeem(ctx, strings.TrimSpace(record.WithdrawalID))
		}
		for _, key := range shielderWithdrawalTxOutLookupKeys(withdrawal) {
			txOutLookupKeys[key] = true
		}
		records = append(records, shielderSyncNullifierRecord{
			nullifier:     strings.TrimSpace(nullifier),
			withdrawalID:  strings.TrimSpace(record.WithdrawalID),
			withdrawal:    withdrawal,
			createdHeight: record.CreatedHeight,
		})
		last = nullifier
	}
	if !more {
		last = ""
	}
	txOutLinks := shielderWithdrawalTxOutLinks(ctx, k, txOutLookupKeys)
	nullifiers := make([]*types.ShielderSpentNullifier, 0, len(records))
	for _, record := range records {
		nullifiers = append(nullifiers, shielderSpentNullifierResponseWithLink(
			record.nullifier,
			record.withdrawalID,
			record.withdrawal,
			record.createdHeight,
			shielderWithdrawalTxOutLinkFromMap(txOutLinks, record.withdrawal),
		))
	}
	return nullifiers, last, shielderSyncPageTotal(len(nullifiers), more), more, nil
}

type shielderSyncNullifierRecord struct {
	nullifier     string
	withdrawalID  string
	withdrawal    types.ShielderRedeem
	createdHeight int64
}

func shielderSyncPageTotal(count int, more bool) uint64 {
	if more {
		return uint64(count + 1)
	}
	return uint64(count)
}

func syncRecordInBirthday(createdHeight, fromHeight int64) bool {
	return fromHeight <= 0 || createdHeight >= fromHeight
}

func depositSyncHeight(deposit types.DepositRecord) int64 {
	height := deposit.CreatedHeight
	for _, candidate := range []int64{
		deposit.MatchedHeight,
		deposit.BTCObservedHeight,
		deposit.RefundEligibleHeight,
		deposit.RefundQueuedHeight,
	} {
		if candidate > height {
			height = candidate
		}
	}
	return height
}

func decodeShielderSyncNullifierRecord(nullifier string, raw []byte) (types.StoredShielderNullifierRecord, error) {
	var record types.StoredShielderNullifierRecord
	if err := json.Unmarshal(raw, &record); err == nil && strings.TrimSpace(record.WithdrawalID) != "" {
		if strings.TrimSpace(record.NullifierHash) == "" {
			record.NullifierHash = strings.TrimSpace(nullifier)
		}
		return record, nil
	}
	var withdrawalID string
	if err := json.Unmarshal(raw, &withdrawalID); err != nil {
		return types.StoredShielderNullifierRecord{}, err
	}
	return types.StoredShielderNullifierRecord{
		NullifierHash: strings.TrimSpace(nullifier),
		WithdrawalID:  strings.TrimSpace(withdrawalID),
	}, nil
}

func (qs queryServer) queryShielderRedeemQuote(ctx cosmos.Context, req *types.QueryShielderRedeemQuoteRequest) (*types.QueryShielderRedeemQuoteResponse, error) {
	if req.AmountSats == 0 {
		return nil, fmt.Errorf("missing withdrawal amount")
	}
	fee := withdrawalFeeSats(ctx, qs.mgr.Keeper(), req.AmountSats)
	if fee >= req.AmountSats {
		return nil, fmt.Errorf("withdrawal fee exceeds amount")
	}
	return &types.QueryShielderRedeemQuoteResponse{
		AmountSats:     req.AmountSats,
		FeeSats:        fee,
		NetSats:        req.AmountSats - fee,
		FeeBasisPoints: withdrawalFeeBp(ctx, qs.mgr.Keeper()),
		FeeMinSats:     0,
	}, nil
}

func (qs queryServer) queryFeePool(ctx cosmos.Context, _ *types.QueryFeePoolRequest) (*types.QueryFeePoolResponse, error) {
	pool, err := distributeFeePool(ctx, qs.mgr.Keeper())
	if err != nil {
		return nil, err
	}
	return &types.QueryFeePoolResponse{
		PendingSats:        pool.PendingSats,
		TotalSlots:         pool.TotalSlots,
		FeePerSlotShare:    pool.FeePerSlotShare,
		TotalCollectedSats: pool.TotalCollectedSats,
		TotalClaimedSats:   pool.TotalClaimedSats,
	}, nil
}

func (qs queryServer) queryDepositSession(ctx cosmos.Context, req *types.QueryDepositSessionRequest) (*types.QueryDepositSessionResponse, error) {
	owner, err := cosmos.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, err
	}
	session, err := qs.mgr.Keeper().GetDepositSession(ctx, owner)
	if err != nil {
		return nil, err
	}
	if session.Owner.Empty() {
		return nil, errors.New("deposit session not found")
	}
	return &types.QueryDepositSessionResponse{
		Owner:                    session.Owner.String(),
		DepositAddress:           session.DepositAddress.String(),
		VaultPubKey:              session.VaultPubKey.String(),
		DepositPathIndex:         session.DepositPathIndex,
		OperatorPubKey:           session.OperatorPubKey.String(),
		NodePubKey:               session.NodePubKey,
		AuctionId:                session.AuctionID,
		CreatedHeight:            session.CreatedHeight,
		Status:                   session.Status,
		DepositId:                session.DepositID.String(),
		InboundTxId:              session.InboundTxID.String(),
		BtcConfirmations:         session.BTCConfirmations,
		BtcConfirmationsRequired: session.BTCConfirmationsRequired,
		BtcObservedHeight:        session.BTCObservedHeight,
		ExpiresAtHeight:          session.ExpiresAtHeight,
		PurgeAtHeight:            session.PurgeAtHeight,
		RefundEligibleHeight:     session.RefundEligibleHeight,
	}, nil
}

func (qs queryServer) queryDepositAddressTxs(ctx cosmos.Context, req *types.QueryDepositAddressTxsRequest) (*types.QueryDepositAddressTxsResponse, error) {
	address, err := common.NewAddress(req.Address)
	if err != nil {
		return nil, err
	}

	iter := qs.mgr.Keeper().GetObservedTxInVoterIterator(ctx)
	defer iter.Close()

	resp := &types.QueryDepositAddressTxsResponse{
		Address: address.String(),
		Txs:     make([]*types.QueryTxResponse, 0),
	}
	seen := make(map[string]bool)
	for ; iter.Valid(); iter.Next() {
		var voter ObservedTxVoter
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &voter); err != nil {
			return nil, err
		}
		if !voterHasDepositAddress(voter, address) {
			continue
		}
		txid := voter.TxID.String()
		if seen[txid] {
			continue
		}
		seen[txid] = true
		tx, err := qs.queryTx(ctx, &types.QueryTxRequest{TxId: txid})
		if err != nil {
			return nil, err
		}
		resp.Txs = append(resp.Txs, tx)
	}

	sort.SliceStable(resp.Txs, func(i, j int) bool {
		left := resp.Txs[i]
		right := resp.Txs[j]
		if left.ObservedTx.BlockHeight == right.ObservedTx.BlockHeight {
			return left.ObservedTx.Tx.ID.String() < right.ObservedTx.Tx.ID.String()
		}
		return left.ObservedTx.BlockHeight < right.ObservedTx.BlockHeight
	})
	return resp, nil
}

func voterHasDepositAddress(voter ObservedTxVoter, address common.Address) bool {
	if !voter.Tx.IsEmpty() && voter.Tx.Tx.ToAddress.Equals(address) {
		return true
	}
	for _, tx := range voter.Txs {
		if tx.Tx.ToAddress.Equals(address) {
			return true
		}
	}
	return false
}

func (qs queryServer) queryNodeBond(ctx cosmos.Context, req *types.QueryNodeBondRequest) (*types.QueryNodeBondResponse, error) {
	bond, err := qs.mgr.Keeper().GetShielderNodeBond(ctx, req.NodePubKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bond.NodePubKey) == "" {
		return nil, errors.New("shielder node bond not found")
	}
	return qs.shielderBondResponse(ctx, bond)
}

func (qs queryServer) shielderBondResponse(ctx cosmos.Context, bond types.ShielderNodeBond) (*types.QueryNodeBondResponse, error) {
	node, err := qs.mgr.Keeper().GetNodeAccount(ctx, bond.NodeAddress)
	if err != nil {
		return nil, err
	}
	penaltyPoints, err := qs.mgr.Keeper().GetNodeAccountPenaltyPoints(ctx, bond.NodeAddress)
	if err != nil {
		return nil, err
	}
	pool, err := qs.mgr.Keeper().GetFeePool(ctx)
	if err != nil {
		return nil, err
	}
	if err := settleNodeFeeShare(ctx, qs.mgr.Keeper(), &bond, pool); err != nil {
		return nil, err
	}
	bonders, err := getNodeBonders(ctx, qs.mgr.Keeper(), bond.NodePubKey)
	if err != nil {
		return nil, err
	}
	bonderResponses := make([]*types.QueryNodeBonder, 0, len(bonders))
	for _, bonder := range bonders {
		bonderResponses = append(bonderResponses, &types.QueryNodeBonder{
			Bonder:              bonder.Bonder.String(),
			PendingSats:         bonder.PendingSats,
			PrincipalSats:       bonder.PrincipalSats,
			ClaimableFeeSats:    nodeBonderClaimableSats(ctx, qs.mgr.Keeper(), bond, bonder),
			PendingFeeDepositId: bonder.PendingFeeDepositID.String(),
			SaleEntitlementId:   bonder.SaleEntitlementID.String(),
			SalePayoutSats:      bonder.SalePayoutSats,
			CreatedHeight:       bonder.CreatedHeight,
			UpdatedHeight:       bonder.UpdatedHeight,
		})
	}
	return &types.QueryNodeBondResponse{
		NodePubKey:                  bond.NodePubKey,
		OperatorPubKey:              bond.OperatorPubKey.String(),
		NodeAddress:                 bond.NodeAddress.String(),
		Slot:                        bond.Slot,
		PendingSats:                 bond.PendingSats,
		BondSats:                    bond.BondSats,
		FeeDebtSats:                 bond.FeeDebtSats,
		FeeShareActive:              bond.FeeShareActive,
		PendingFeeDepositId:         bond.PendingFeeDepositID.String(),
		Sold:                        bond.Sold,
		SoldAuctionId:               bond.SoldAuctionID,
		CreatedHeight:               bond.CreatedHeight,
		UpdatedHeight:               bond.UpdatedHeight,
		NodeStatus:                  node.Status.String(),
		PenaltyPoints:               penaltyPoints,
		OperatorFeeBasisPoints:      bond.OperatorFeeBasisPoints,
		OperatorFeeAccruedSats:      bond.OperatorFeeAccruedSats,
		PendingOperatorFeeDepositId: bond.PendingOperatorFeeDepositID.String(),
		Bonders:                     bonderResponses,
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
	return nodeSlotAuctionResponse(auction), nil
}

func nodeSlotAuctionResponse(auction types.NodeSlotAuction) *types.QueryNodeSlotAuctionResponse {
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
	}
}

func (qs queryServer) queryNodeSlotAuctions(ctx cosmos.Context, _ *types.QueryNodeSlotAuctionsRequest) (*types.QueryNodeSlotAuctionsResponse, error) {
	iter := qs.mgr.Keeper().GetNodeSlotAuctionIterator(ctx)
	defer iter.Close()
	auctions := make([]*types.QueryNodeSlotAuctionResponse, 0)
	for ; iter.Valid(); iter.Next() {
		var auction types.NodeSlotAuction
		if err := json.Unmarshal(iter.Value(), &auction); err != nil {
			return nil, err
		}
		auctions = append(auctions, nodeSlotAuctionResponse(auction))
	}
	sort.Slice(auctions, func(i, j int) bool {
		return auctions[i].CreatedHeight < auctions[j].CreatedHeight
	})
	return &types.QueryNodeSlotAuctionsResponse{Auctions: auctions}, nil
}

func (qs queryServer) queryNodeSlotBid(ctx cosmos.Context, req *types.QueryNodeSlotBidRequest) (*types.QueryNodeSlotBidResponse, error) {
	bid, err := qs.mgr.Keeper().GetNodeSlotBid(ctx, req.BidId)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bid.BidID) == "" {
		return nil, errors.New("node slot bid not found")
	}
	return nodeSlotBidResponse(bid), nil
}

func nodeSlotBidResponse(bid types.NodeSlotBid) *types.QueryNodeSlotBidResponse {
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
	}
}

func (qs queryServer) queryNodeSlotAuctionBids(ctx cosmos.Context, req *types.QueryNodeSlotAuctionBidsRequest) (*types.QueryNodeSlotAuctionBidsResponse, error) {
	iter := qs.mgr.Keeper().GetNodeSlotBidIterator(ctx)
	defer iter.Close()
	bids := make([]*types.QueryNodeSlotBidResponse, 0)
	for ; iter.Valid(); iter.Next() {
		var bid types.NodeSlotBid
		if err := json.Unmarshal(iter.Value(), &bid); err != nil {
			return nil, err
		}
		if bid.AuctionID == strings.TrimSpace(req.AuctionId) {
			bids = append(bids, nodeSlotBidResponse(bid))
		}
	}
	sort.Slice(bids, func(i, j int) bool {
		if bids[i].AmountSats == bids[j].AmountSats {
			return bids[i].CreatedHeight < bids[j].CreatedHeight
		}
		return bids[i].AmountSats > bids[j].AmountSats
	})
	return &types.QueryNodeSlotAuctionBidsResponse{Bids: bids}, nil
}

func (qs queryServer) queryVaultDepositAddress(ctx cosmos.Context, req *types.QueryVaultDepositAddressRequest) (*types.QueryVaultDepositAddressResponse, error) {
	address, err := common.NewAddress(req.Address)
	if err != nil {
		return nil, err
	}
	record, err := qs.mgr.Keeper().GetDepositAddress(ctx, address)
	if err != nil {
		return nil, err
	}
	if record.Address.IsEmpty() {
		return nil, errors.New("vault deposit address not found")
	}
	return &types.QueryVaultDepositAddressResponse{
		Address:         record.Address.String(),
		VaultPubKey:     record.VaultPubKey.String(),
		PathIndex:       record.PathIndex,
		Owner:           record.Owner.String(),
		OperatorPubKey:  record.OperatorPubKey.String(),
		NodePubKey:      record.NodePubKey,
		AuctionId:       record.AuctionID,
		CreatedHeight:   record.CreatedHeight,
		ExpiresAtHeight: record.ExpiresAtHeight,
		PurgeAtHeight:   record.PurgeAtHeight,
	}, nil
}

func (qs queryServer) queryNodeFeeEntitlement(ctx cosmos.Context, req *types.QueryNodeFeeEntitlementRequest) (*types.QueryNodeFeeEntitlementResponse, error) {
	bond, err := qs.mgr.Keeper().GetShielderNodeBond(ctx, req.NodePubKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bond.NodePubKey) == "" {
		return nil, errors.New("shielder node bond not found")
	}
	return qs.shielderFeeEntitlementResponse(ctx, bond)
}

func (qs queryServer) shielderFeeEntitlementResponse(ctx cosmos.Context, bond types.ShielderNodeBond) (*types.QueryNodeFeeEntitlementResponse, error) {
	pool, err := qs.mgr.Keeper().GetFeePool(ctx)
	if err != nil {
		return nil, err
	}
	if err := settleNodeFeeShare(ctx, qs.mgr.Keeper(), &bond, pool); err != nil {
		return nil, err
	}
	bonders, err := getNodeBonders(ctx, qs.mgr.Keeper(), bond.NodePubKey)
	if err != nil {
		return nil, err
	}
	claimable := uint64(0)
	if bond.FeeShareActive {
		claimable += bond.OperatorFeeAccruedSats
		for _, bonder := range bonders {
			if bonder.PendingFeeDepositID.IsEmpty() {
				claimable += nodeBonderClaimableSats(ctx, qs.mgr.Keeper(), bond, bonder)
			}
		}
	}
	return &types.QueryNodeFeeEntitlementResponse{
		NodePubKey:          bond.NodePubKey,
		ClaimableSats:       claimable,
		AccruedSats:         pool.FeePerSlotShare,
		FeeDebtSats:         bond.FeeDebtSats,
		FeePerSlotShare:     pool.FeePerSlotShare,
		FeeShareActive:      bond.FeeShareActive,
		PendingFeeDepositId: bond.PendingFeeDepositID.String(),
	}, nil
}

func (qs queryServer) queryNodeFeeEntitlements(ctx cosmos.Context, _ *types.QueryNodeFeeEntitlementsRequest) (*types.QueryNodeFeeEntitlementsResponse, error) {
	iter := qs.mgr.Keeper().GetShielderNodeBondIterator(ctx)
	defer iter.Close()
	entitlements := make([]*types.QueryNodeFeeEntitlementResponse, 0)
	for ; iter.Valid(); iter.Next() {
		var bond types.ShielderNodeBond
		if err := json.Unmarshal(iter.Value(), &bond); err != nil {
			return nil, err
		}
		entitlement, err := qs.shielderFeeEntitlementResponse(ctx, bond)
		if err != nil {
			return nil, err
		}
		entitlements = append(entitlements, entitlement)
	}
	sort.Slice(entitlements, func(i, j int) bool {
		return entitlements[i].NodePubKey < entitlements[j].NodePubKey
	})
	return &types.QueryNodeFeeEntitlementsResponse{Entitlements: entitlements}, nil
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
	sig, err := qs.kbs.Sign(buf)
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
	if txs.Status != "" && txs.Status != TxOutStatusPendingSign {
		txs = &TxOut{
			Height:           height,
			Epoch:            txs.Epoch,
			Status:           txs.Status,
			SigningLeader:    txs.SigningLeader,
			SigningAttempt:   txs.SigningAttempt,
			RetryUntilHeight: txs.RetryUntilHeight,
		}
	}

	if !pk.IsEmpty() {
		newTxs := &TxOut{
			Height:           txs.Height,
			Epoch:            txs.Epoch,
			Status:           txs.Status,
			SigningLeader:    txs.SigningLeader,
			SigningAttempt:   txs.SigningAttempt,
			RetryUntilHeight: txs.RetryUntilHeight,
		}
		for _, tx := range txs.TxArray {
			if pk.Equals(tx.VaultPubKey) {
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
	sig, err := qs.kbs.Sign(buf)
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
		baseVaults, err := qs.mgr.Keeper().GetBaseVaultsByStatus(ctx, ActiveVault)
		if err != nil {
			return nil, fmt.Errorf("fail to get active base: %w", err)
		}
		for _, vault := range baseVaults {
			chains = vault.GetChains().Distinct()
			break
		}
		if len(chains) == 0 {
			lastChainHeights, err := qs.mgr.Keeper().GetLastChainHeights(ctx)
			if err != nil {
				return nil, fmt.Errorf("fail to get last chain heights: %w", err)
			}
			for chain := range lastChainHeights {
				chains = append(chains, chain)
			}
		}
	}
	var result []*types.ChainsLastBlock
	for _, c := range chains {
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

func (qs queryServer) queryConfigDefaults(_ cosmos.Context, _ *types.QueryConfigDefaultsRequest) (*types.QueryConfigDefaultsResponse, error) {
	constAccessor := qs.mgr.GetConstants()
	cv := constAccessor.GetConfigValsByKeyname()

	proto := types.QueryConfigDefaultsResponse{}
	// analyze-ignore(map-iteration)
	for k, v := range cv.Int64Values {
		proto.Int_64Values = append(proto.Int_64Values, &types.Int64Constants{
			Name:        k,
			Value:       v,
			Group:       constants.ConfigGroup(k),
			Description: constants.ConfigDescription(k),
		})
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.BoolValues {
		proto.BoolValues = append(proto.BoolValues, &types.BoolConstants{
			Name:        k,
			Value:       v,
			Group:       constants.ConfigGroup(k),
			Description: constants.ConfigDescription(k),
		})
	}
	// analyze-ignore(map-iteration)
	for k, v := range cv.StringValues {
		proto.StringValues = append(proto.StringValues, &types.StringConstants{
			Name:        k,
			Value:       v,
			Group:       constants.ConfigGroup(k),
			Description: constants.ConfigDescription(k),
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

func (qs queryServer) queryConfigValues(ctx cosmos.Context, _ *types.QueryConfigValuesRequest) (*types.QueryConfigValuesResponse, error) {
	resp := types.QueryConfigValuesResponse{
		Configs: make([]*types.Config, 0),
	}

	// collect all keys with set values, not displaying those with votes but no set value
	keeper := qs.mgr.Keeper()
	iter := keeper.GetConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key := strings.TrimPrefix(string(iter.Key()), "config//")
		value, err := keeper.GetConfig(ctx, key)
		if err != nil {
			ctx.Logger().Error("fail to get config value", "error", err)
			continue
		}
		if value < 0 {
			ctx.Logger().Error("negative config value set", "key", key, "value", value)
			continue
		}
		resp.Configs = append(resp.Configs, &types.Config{
			Key:         key,
			Value:       value,
			Group:       constants.ConfigGroup(key),
			Description: constants.ConfigDescription(key),
		})
	}
	key := "Deposit_PowDifficultyCurrent"
	resp.Configs = append(resp.Configs, &types.Config{
		Key:         key,
		Value:       currentDepositPowDifficulty(ctx, keeper),
		Group:       constants.ConfigGroup(key),
		Description: constants.ConfigDescription(key),
	})

	return &resp, nil
}

func (qs queryServer) queryConfigNodesAllValues(ctx cosmos.Context, _ *types.QueryConfigNodesAllValuesRequest) (*types.QueryConfigNodesAllValuesResponse, error) {
	resp := types.QueryConfigNodesAllValuesResponse{
		Configs: make([]types.NodeConfig, 0),
	}

	iter := qs.mgr.Keeper().GetNodeConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		m := NodeConfigs{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &m); err != nil {
			ctx.Logger().Error("fail to unmarshal node config value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal node config value: %w", err)
		}
		resp.Configs = append(resp.Configs, m.Configs...)
	}

	return &resp, nil
}

func (qs queryServer) queryConfigNodesValues(ctx cosmos.Context, _ *types.QueryConfigNodesValuesRequest) (*types.QueryConfigNodesValuesResponse, error) {
	activeNodes, err := qs.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to fetch active node accounts", "error", err)
		return nil, fmt.Errorf("fail to fetch active node accounts: %w", err)
	}
	active := activeNodes.GetNodeAddresses()

	resp := types.QueryConfigNodesValuesResponse{
		Configs: make([]*types.Config, 0),
	}

	iter := qs.mgr.Keeper().GetNodeConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		configs := NodeConfigs{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &configs); err != nil {
			ctx.Logger().Error("fail to unmarshal node config value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal node config value: %w", err)
		}
		k := strings.TrimPrefix(string(iter.Key()), "nodeconfig//")
		if v, ok := configs.HasSuperMajority(k, active); ok {
			resp.Configs = append(resp.Configs, &types.Config{
				Key:         k,
				Value:       v,
				Group:       constants.ConfigGroup(k),
				Description: constants.ConfigDescription(k),
			})
		}
	}

	return &resp, nil
}

func (qs queryServer) queryConfigNodeValues(ctx cosmos.Context, req *types.QueryConfigNodeValuesRequest) (*types.QueryConfigNodeValuesResponse, error) {
	acc, err := cosmos.AccAddressFromBech32(req.Address)
	if err != nil {
		ctx.Logger().Error("fail to parse thor address", "error", err)
		return nil, fmt.Errorf("fail to parse thor address: %w", err)
	}

	resp := types.QueryConfigNodeValuesResponse{
		NodeConfigs: make([]*types.Config, 0),
	}

	iter := qs.mgr.Keeper().GetNodeConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		configs := NodeConfigs{}
		if err := qs.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &configs); err != nil {
			ctx.Logger().Error("fail to unmarshal node config v2 value", "error", err)
			return nil, fmt.Errorf("fail to unmarshal node config value: %w", err)
		}

		k := strings.TrimPrefix(string(iter.Key()), "nodeconfig//")
		if v, ok := configs.Get(k, acc); ok {
			resp.NodeConfigs = append(resp.NodeConfigs, &types.Config{
				Key:         k,
				Value:       v,
				Group:       constants.ConfigGroup(k),
				Description: constants.ConfigDescription(k),
			})
		}
	}

	return &resp, nil
}

func (qs queryServer) queryFrostKeygenMetric(ctx cosmos.Context, req *types.QueryFrostKeygenMetricRequest) (*types.QueryFrostKeygenMetricResponse, error) {
	if len(req.PubKey) == 0 {
		return nil, fmt.Errorf("missing pub_key parameter")
	}
	pkey, err := common.NewPubKey(req.PubKey)
	if err != nil {
		return nil, fmt.Errorf("fail to parse pubkey(%s) err:%w", req.PubKey, err)
	}

	var result []*types.FrostKeygenMetric
	m, err := qs.mgr.Keeper().GetFrostKeygenMetric(ctx, pkey)
	if err != nil {
		return nil, fmt.Errorf("fail to get frost keygen metric for pubkey(%s):%w", pkey, err)
	}
	result = append(result, m)

	return &types.QueryFrostKeygenMetricResponse{Metrics: result}, nil
}

func (qs queryServer) queryFrostMetric(ctx cosmos.Context, _ *types.QueryFrostMetricRequest) (*types.QueryFrostMetricResponse, error) {
	var pubKeys common.PubKeys
	// get all active base
	vaults, err := qs.mgr.Keeper().GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return nil, fmt.Errorf("fail to get active baseVaults:%w", err)
	}
	for _, v := range vaults {
		pubKeys = append(pubKeys, v.PubKey)
	}
	var keygenMetrics []*types.FrostKeygenMetric
	for _, pkey := range pubKeys {
		var m *types.FrostKeygenMetric
		m, err = qs.mgr.Keeper().GetFrostKeygenMetric(ctx, pkey)
		if err != nil {
			return nil, fmt.Errorf("fail to get frost keygen metric for pubkey(%s):%w", pkey, err)
		}
		if len(m.NodeFrostTimes) == 0 {
			continue
		}
		keygenMetrics = append(keygenMetrics, m)
	}
	keysignMetric, err := qs.mgr.Keeper().GetLatestFrostKeysignMetric(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to get keysign metric:%w", err)
	}

	return &types.QueryFrostMetricResponse{
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

func castObservedTxs(observedTxs ObservedTxs) []types.QueryObservedTx {
	result := make([]types.QueryObservedTx, len(observedTxs))
	for i := range observedTxs {
		result[i] = castObservedTx(observedTxs[i])
	}
	return result
}

type shielderWithdrawalTxOutLink struct {
	Status           string
	Height           int64
	Epoch            uint64
	TxType           string
	OutHash          string
	OutVout          uint32
	Outpoint         string
	SigningAttempt   uint64
	RetryUntilHeight int64
}

func shielderRedeemResponse(ctx cosmos.Context, k keeper.Keeper, withdrawal types.ShielderRedeem) *types.QueryShielderRedeemResponse {
	link := shielderWithdrawalTxOut(ctx, k, withdrawal)
	return &types.QueryShielderRedeemResponse{
		WithdrawalId:    withdrawal.WithdrawalID,
		NullifierHash:   withdrawal.NullifierHash,
		MerkleRoot:      withdrawal.MerkleRoot,
		Recipient:       withdrawal.Recipient.String(),
		AmountSats:      withdrawal.AmountSats,
		FeeSats:         withdrawal.FeeSats,
		InHash:          withdrawal.InHash.String(),
		VaultPubKey:     withdrawal.VaultPubKey.String(),
		RequestedHeight: withdrawal.RequestedHeight,
		Status:          withdrawal.Status,
		TxoutStatus:     link.Status,
		TxoutHeight:     link.Height,
		TxoutEpoch:      link.Epoch,
		TxType:          link.TxType,
		OutHash:         link.OutHash,
		OutVout:         link.OutVout,
		Outpoint:        link.Outpoint,
	}
}

func shielderNullifierResponse(ctx cosmos.Context, k keeper.Keeper, nullifier string, withdrawal types.ShielderRedeem) *types.QueryShielderNullifierResponse {
	resp := &types.QueryShielderNullifierResponse{
		NullifierHash: strings.TrimSpace(nullifier),
		Spent:         strings.TrimSpace(withdrawal.WithdrawalID) != "",
		WithdrawalId:  strings.TrimSpace(withdrawal.WithdrawalID),
	}
	if !resp.Spent {
		return resp
	}
	link := shielderWithdrawalTxOut(ctx, k, withdrawal)
	resp.WithdrawalStatus = withdrawal.Status
	resp.AmountSats = withdrawal.AmountSats
	resp.FeeSats = withdrawal.FeeSats
	resp.InHash = withdrawal.InHash.String()
	resp.TxoutStatus = link.Status
	resp.TxoutHeight = link.Height
	resp.TxoutEpoch = link.Epoch
	resp.TxType = link.TxType
	resp.OutHash = link.OutHash
	resp.OutVout = link.OutVout
	resp.Outpoint = link.Outpoint
	return resp
}

func shielderSpentNullifierResponse(ctx cosmos.Context, k keeper.Keeper, nullifier, withdrawalID string, withdrawal types.ShielderRedeem, createdHeight int64) *types.ShielderSpentNullifier {
	return shielderSpentNullifierResponseWithLink(nullifier, withdrawalID, withdrawal, createdHeight, shielderWithdrawalTxOut(ctx, k, withdrawal))
}

func shielderSpentNullifierResponseWithLink(nullifier, withdrawalID string, withdrawal types.ShielderRedeem, createdHeight int64, link shielderWithdrawalTxOutLink) *types.ShielderSpentNullifier {
	resp := &types.ShielderSpentNullifier{
		NullifierHash: strings.TrimSpace(nullifier),
		WithdrawalId:  strings.TrimSpace(withdrawalID),
		CreatedHeight: createdHeight,
	}
	if strings.TrimSpace(withdrawal.WithdrawalID) == "" {
		return resp
	}
	resp.WithdrawalStatus = withdrawal.Status
	resp.AmountSats = withdrawal.AmountSats
	resp.FeeSats = withdrawal.FeeSats
	resp.InHash = withdrawal.InHash.String()
	resp.TxoutStatus = link.Status
	resp.TxoutHeight = link.Height
	resp.TxoutEpoch = link.Epoch
	resp.TxType = link.TxType
	resp.OutHash = link.OutHash
	resp.OutVout = link.OutVout
	resp.Outpoint = link.Outpoint
	return resp
}

func shielderWithdrawalTxOut(ctx cosmos.Context, k keeper.Keeper, withdrawal types.ShielderRedeem) shielderWithdrawalTxOutLink {
	wanted := make(map[string]bool)
	for _, key := range shielderWithdrawalTxOutLookupKeys(withdrawal) {
		wanted[key] = true
	}
	return shielderWithdrawalTxOutLinkFromMap(shielderWithdrawalTxOutLinks(ctx, k, wanted), withdrawal)
}

func shielderWithdrawalTxOutLookupKeys(withdrawal types.ShielderRedeem) []string {
	withdrawalID := strings.TrimSpace(withdrawal.WithdrawalID)
	inHash := strings.TrimSpace(withdrawal.InHash.String())
	if withdrawalID == "" && inHash == "" {
		return nil
	}
	keys := make([]string, 0, 2)
	if withdrawalID != "" {
		keys = append(keys, strings.ToLower(withdrawalID))
	}
	if inHash != "" && !strings.EqualFold(inHash, withdrawalID) {
		keys = append(keys, strings.ToLower(inHash))
	}
	return keys
}

func shielderWithdrawalTxOutLinkFromMap(links map[string]shielderWithdrawalTxOutLink, withdrawal types.ShielderRedeem) shielderWithdrawalTxOutLink {
	for _, key := range shielderWithdrawalTxOutLookupKeys(withdrawal) {
		if link, ok := links[key]; ok {
			return link
		}
	}
	return shielderWithdrawalTxOutLink{}
}

func shielderWithdrawalTxOutLinks(ctx cosmos.Context, k keeper.Keeper, wanted map[string]bool) map[string]shielderWithdrawalTxOutLink {
	links := make(map[string]shielderWithdrawalTxOutLink)
	if len(wanted) == 0 {
		return links
	}
	iterator := k.GetTxOutIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iterator.Value(), &txOut); err != nil {
			continue
		}
		for _, item := range txOut.TxArray {
			itemInHash := strings.TrimSpace(item.InHash.String())
			if itemInHash == "" {
				continue
			}
			itemKey := strings.ToLower(itemInHash)
			if !wanted[itemKey] {
				continue
			}
			link := shielderWithdrawalTxOutLink{
				Status:           txOut.Status,
				Height:           txOut.Height,
				Epoch:            txOut.Epoch,
				TxType:           item.GetTxType(),
				OutHash:          strings.TrimSpace(item.OutHash.String()),
				OutVout:          item.OutVout,
				SigningAttempt:   txOut.SigningAttempt,
				RetryUntilHeight: txOut.RetryUntilHeight,
			}
			if link.OutHash != "" {
				link.Outpoint = fmt.Sprintf("%s:%d", link.OutHash, link.OutVout)
			}
			mergeShielderWithdrawalTxOutLink(links, itemKey, link)
		}
	}
	return links
}

func mergeShielderWithdrawalTxOutLink(links map[string]shielderWithdrawalTxOutLink, key string, link shielderWithdrawalTxOutLink) {
	current, ok := links[key]
	if !ok {
		links[key] = link
		return
	}
	if current.OutHash != "" {
		return
	}
	if link.OutHash != "" || current.Status == "" {
		links[key] = link
	}
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
