package thornado

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type queryServer struct {
	mgr      *Mgrs
	kbs      cosmos.KeybaseStore
	regInit  bool
	txConfig client.TxConfig
}

var _ types.QueryServer = &queryServer{}

func NewQueryServerImpl(mgr *Mgrs, txConfig client.TxConfig, kbs cosmos.KeybaseStore) types.QueryServer {
	return &queryServer{mgr: mgr, txConfig: txConfig, kbs: kbs}
}

func (s *queryServer) unwrapSdkContext(c context.Context) sdk.Context {
	ctx := sdk.UnwrapSDKContext(c)
	ctx = ctx.WithLogger(ctx.Logger().With("height", ctx.BlockHeight()))
	if s.regInit {
		return ctx
	}
	initManager(ctx, s.mgr) // NOOP except regtest
	s.regInit = true
	return ctx
}

func checkHeightParam(height string) error {
	if len(height) > 0 {
		_, err := strconv.ParseInt(height, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid height param, %w", err)
		}
	}
	return nil
}

func (s *queryServer) Node(c context.Context, req *types.QueryNodeRequest) (*types.QueryNodeResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNode(ctx, req)
}

func (s *queryServer) Nodes(c context.Context, req *types.QueryNodesRequest) (*types.QueryNodesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodes(ctx, req)
}

func (s *queryServer) NodeMetrics(c context.Context, req *types.QueryNodeMetricsRequest) (*types.QueryNodeMetricsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeMetrics(ctx, req)
}

func (s *queryServer) NodeSlot(c context.Context, req *types.QueryNodeSlotRequest) (*types.QueryNodeSlotResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlot(ctx, req)
}

func (s *queryServer) ConfigValues(c context.Context, req *types.QueryConfigValuesRequest) (*types.QueryConfigValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryConfigValues(ctx, req)
}

func (s *queryServer) ConfigNodesAllValues(c context.Context, req *types.QueryConfigNodesAllValuesRequest) (*types.QueryConfigNodesAllValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryConfigNodesAllValues(ctx, req)
}

func (s *queryServer) ConfigNodesValues(c context.Context, req *types.QueryConfigNodesValuesRequest) (*types.QueryConfigNodesValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryConfigNodesValues(ctx, req)
}

func (s *queryServer) ConfigNodeValues(c context.Context, req *types.QueryConfigNodeValuesRequest) (*types.QueryConfigNodeValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryConfigNodeValues(ctx, req)
}

func (s *queryServer) Version(c context.Context, req *types.QueryVersionRequest) (*types.QueryVersionResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVersion(ctx, req)
}

func (s *queryServer) Invariant(c context.Context, req *types.QueryInvariantRequest) (*types.QueryInvariantResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryInvariant(ctx, req)
}

func (s *queryServer) Invariants(c context.Context, req *types.QueryInvariantsRequest) (*types.QueryInvariantsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryInvariants(ctx, req)
}

func (s *queryServer) BalanceModule(c context.Context, req *types.QueryBalanceModuleRequest) (*types.QueryBalanceModuleResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryBalanceModule(ctx, req)
}

func (s *queryServer) ConfigDefaults(c context.Context, req *types.QueryConfigDefaultsRequest) (*types.QueryConfigDefaultsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryConfigDefaults(ctx, req)
}

func (s *queryServer) LastBlocks(c context.Context, req *types.QueryLastBlocksRequest) (*types.QueryLastBlocksResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryLastBlockHeights(ctx, "")
}

func (s *queryServer) NetworkFeeCurrent(c context.Context, req *types.QueryNetworkFeeRequest) (*types.NetworkFee, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNetworkFee(ctx, req)
}

func (s *queryServer) Vault(c context.Context, req *types.QueryVaultRequest) (*types.QueryVaultResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVault(ctx, req)
}

func (s *queryServer) BaseVaults(c context.Context, req *types.QueryBaseVaultsRequest) (*types.QueryBaseVaultsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryBaseVaults(ctx, req)
}

func (s *queryServer) VaultsPubkeys(c context.Context, req *types.QueryVaultsPubkeysRequest) (*types.QueryVaultsPubkeysResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVaultsPubkeys(ctx, req)
}

func (s *queryServer) VaultSolvency(c context.Context, req *types.QueryVaultSolvencyRequest) (*types.QueryVaultSolvencyResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVaultSolvency(ctx, req)
}

func (s *queryServer) Tx(c context.Context, req *types.QueryTxRequest) (*types.QueryTxResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTx(ctx, req)
}

func (s *queryServer) TxOut(c context.Context, req *types.QueryTxOutRequest) (*types.QueryTxOutResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxOut(ctx, req, txOutViewAll)
}

func (s *queryServer) TxOutOut(c context.Context, req *types.QueryTxOutRequest) (*types.QueryTxOutResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxOut(ctx, req, txOutViewOut)
}

func (s *queryServer) TxOutInternal(c context.Context, req *types.QueryTxOutRequest) (*types.QueryTxOutResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxOut(ctx, req, txOutViewInternal)
}

func (s *queryServer) TxOutAll(c context.Context, req *types.QueryTxOutRequest) (*types.QueryTxOutResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxOut(ctx, req, txOutViewAll)
}

func (s *queryServer) Deposit(c context.Context, req *types.QueryDepositRequest) (*types.QueryDepositResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryDeposit(ctx, req)
}

func (s *queryServer) ShielderRedeem(c context.Context, req *types.QueryShielderRedeemRequest) (*types.QueryShielderRedeemResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderRedeem(ctx, req)
}

func (s *queryServer) ShielderNullifier(c context.Context, req *types.QueryShielderNullifierRequest) (*types.QueryShielderNullifierResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderNullifier(ctx, req)
}

func (s *queryServer) ShielderSync(c context.Context, req *types.QueryShielderSyncRequest) (*types.QueryShielderSyncResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderSync(ctx, req)
}

func (s *queryServer) ShielderRedeemQuote(c context.Context, req *types.QueryShielderRedeemQuoteRequest) (*types.QueryShielderRedeemQuoteResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderRedeemQuote(ctx, req)
}

func (s *queryServer) FeePool(c context.Context, req *types.QueryFeePoolRequest) (*types.QueryFeePoolResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryFeePool(ctx, req)
}

func (s *queryServer) Gas(c context.Context, req *types.QueryGasRequest) (*types.QueryGasResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryGas(ctx, req)
}

func (s *queryServer) DepositSession(c context.Context, req *types.QueryDepositSessionRequest) (*types.QueryDepositSessionResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryDepositSession(ctx, req)
}

func (s *queryServer) DepositAddressTxs(c context.Context, req *types.QueryDepositAddressTxsRequest) (*types.QueryDepositAddressTxsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryDepositAddressTxs(ctx, req)
}

func (s *queryServer) NodeBond(c context.Context, req *types.QueryNodeBondRequest) (*types.QueryNodeBondResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeBond(ctx, req)
}

func (s *queryServer) NodeSlotAuction(c context.Context, req *types.QueryNodeSlotAuctionRequest) (*types.QueryNodeSlotAuctionResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlotAuction(ctx, req)
}

func (s *queryServer) NodeSlotAuctions(c context.Context, req *types.QueryNodeSlotAuctionsRequest) (*types.QueryNodeSlotAuctionsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlotAuctions(ctx, req)
}

func (s *queryServer) NodeSlotBid(c context.Context, req *types.QueryNodeSlotBidRequest) (*types.QueryNodeSlotBidResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlotBid(ctx, req)
}

func (s *queryServer) NodeSlotAuctionBids(c context.Context, req *types.QueryNodeSlotAuctionBidsRequest) (*types.QueryNodeSlotAuctionBidsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlotAuctionBids(ctx, req)
}

func (s *queryServer) VaultDepositAddress(c context.Context, req *types.QueryVaultDepositAddressRequest) (*types.QueryVaultDepositAddressResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVaultDepositAddress(ctx, req)
}

func (s *queryServer) NodeFeeEntitlement(c context.Context, req *types.QueryNodeFeeEntitlementRequest) (*types.QueryNodeFeeEntitlementResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeFeeEntitlement(ctx, req)
}

func (s *queryServer) NodeFeeEntitlements(c context.Context, req *types.QueryNodeFeeEntitlementsRequest) (*types.QueryNodeFeeEntitlementsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeFeeEntitlements(ctx, req)
}

func (s *queryServer) Block(c context.Context, req *types.QueryBlockRequest) (*types.QueryBlockResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryBlock(ctx, req)
}

func (s *queryServer) FrostKeygenMetric(c context.Context, req *types.QueryFrostKeygenMetricRequest) (*types.QueryFrostKeygenMetricResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryFrostKeygenMetric(ctx, req)
}

func (s *queryServer) FrostMetric(c context.Context, req *types.QueryFrostMetricRequest) (*types.QueryFrostMetricResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryFrostMetric(ctx, req)
}

func (s *queryServer) Keysign(c context.Context, req *types.QueryKeysignRequest) (*types.QueryKeysignResponse, error) {
	ctx := s.unwrapSdkContext(c)
	return s.queryKeysign(ctx, req.Height, "")
}

func (s *queryServer) KeysignPubkey(c context.Context, req *types.QueryKeysignPubkeyRequest) (*types.QueryKeysignResponse, error) {
	ctx := s.unwrapSdkContext(c)
	return s.queryKeysign(ctx, req.Height, req.PubKey)
}

func (s *queryServer) Keygen(c context.Context, req *types.QueryKeygenRequest) (*types.QueryKeygenResponse, error) {
	ctx := s.unwrapSdkContext(c)
	return s.queryKeygen(ctx, req)
}

func (s *queryServer) UpgradeProposal(c context.Context, req *types.QueryUpgradeProposalRequest) (*types.QueryUpgradeProposalResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryUpgradeProposal(ctx, req)
}

func (s *queryServer) UpgradeProposals(c context.Context, req *types.QueryUpgradeProposalsRequest) (*types.QueryUpgradeProposalsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryUpgradeProposals(ctx, req)
}

func (s *queryServer) UpgradeVotes(c context.Context, req *types.QueryUpgradeVotesRequest) (*types.QueryUpgradeVotesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryUpgradeVotes(ctx, req)
}

func (s *queryServer) Account(c context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryAccount(ctx, req)
}
