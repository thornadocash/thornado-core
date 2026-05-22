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

func (s *queryServer) Ban(c context.Context, req *types.QueryBanRequest) (*types.BanVoter, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryBan(ctx, req)
}

func (s *queryServer) MimirValues(c context.Context, req *types.QueryMimirValuesRequest) (*types.QueryMimirValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryMimirValues(ctx, req)
}

func (s *queryServer) MimirWithKey(c context.Context, req *types.QueryMimirWithKeyRequest) (*types.QueryMimirWithKeyResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryMimirWithKey(ctx, req)
}

func (s *queryServer) MimirAdminValues(c context.Context, req *types.QueryMimirAdminValuesRequest) (*types.QueryMimirAdminValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryMimirAdminValues(ctx, req)
}

func (s *queryServer) MimirNodesAllValues(c context.Context, req *types.QueryMimirNodesAllValuesRequest) (*types.QueryMimirNodesAllValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryMimirNodesAllValues(ctx, req)
}

func (s *queryServer) MimirNodesValues(c context.Context, req *types.QueryMimirNodesValuesRequest) (*types.QueryMimirNodesValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryMimirNodesValues(ctx, req)
}

func (s *queryServer) MimirNodeValues(c context.Context, req *types.QueryMimirNodeValuesRequest) (*types.QueryMimirNodeValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryMimirNodeValues(ctx, req)
}

func (s *queryServer) InboundAddresses(c context.Context, req *types.QueryInboundAddressesRequest) (*types.QueryInboundAddressesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryInboundAddresses(ctx, req)
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

func (s *queryServer) ConstantValues(c context.Context, req *types.QueryConstantValuesRequest) (*types.QueryConstantValuesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryConstantValues(ctx, req)
}

func (s *queryServer) LastBlocks(c context.Context, req *types.QueryLastBlocksRequest) (*types.QueryLastBlocksResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryLastBlockHeights(ctx, "")
}

func (s *queryServer) ChainsLastBlock(c context.Context, req *types.QueryChainsLastBlockRequest) (*types.QueryLastBlocksResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryLastBlockHeights(ctx, req.Chain)
}

func (s *queryServer) Vault(c context.Context, req *types.QueryVaultRequest) (*types.QueryVaultResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVault(ctx, req)
}

func (s *queryServer) AsgardVaults(c context.Context, req *types.QueryAsgardVaultsRequest) (*types.QueryAsgardVaultsResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryAsgardVaults(ctx, req)
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

func (s *queryServer) TxStages(c context.Context, req *types.QueryTxStagesRequest) (*types.QueryTxStagesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxStages(ctx, req)
}

func (s *queryServer) TxStatus(c context.Context, req *types.QueryTxStatusRequest) (*types.QueryTxStatusResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxStatus(ctx, req)
}

func (s *queryServer) TxVoters(c context.Context, req *types.QueryTxVotersRequest) (*types.QueryObservedTxVoter, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxVoters(ctx, req)
}

func (s *queryServer) TxVotersOld(c context.Context, req *types.QueryTxVotersRequest) (*types.QueryObservedTxVoter, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTxVoters(ctx, req)
}

func (s *queryServer) Tx(c context.Context, req *types.QueryTxRequest) (*types.QueryTxResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTx(ctx, req)
}

func (s *queryServer) ShielderDeposit(c context.Context, req *types.QueryShielderDepositRequest) (*types.QueryShielderDepositResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderDeposit(ctx, req)
}

func (s *queryServer) ShielderFeePool(c context.Context, req *types.QueryShielderFeePoolRequest) (*types.QueryShielderFeePoolResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderFeePool(ctx, req)
}

func (s *queryServer) ShielderSession(c context.Context, req *types.QueryShielderSessionRequest) (*types.QueryShielderSessionResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderSession(ctx, req)
}

func (s *queryServer) ShielderBond(c context.Context, req *types.QueryShielderBondRequest) (*types.QueryShielderBondResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderBond(ctx, req)
}

func (s *queryServer) NodeSlotAuction(c context.Context, req *types.QueryNodeSlotAuctionRequest) (*types.QueryNodeSlotAuctionResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlotAuction(ctx, req)
}

func (s *queryServer) NodeSlotBid(c context.Context, req *types.QueryNodeSlotBidRequest) (*types.QueryNodeSlotBidResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryNodeSlotBid(ctx, req)
}

func (s *queryServer) VaultDepositAddress(c context.Context, req *types.QueryVaultDepositAddressRequest) (*types.QueryVaultDepositAddressResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryVaultDepositAddress(ctx, req)
}

func (s *queryServer) ShielderFeeEntitlement(c context.Context, req *types.QueryShielderFeeEntitlementRequest) (*types.QueryShielderFeeEntitlementResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryShielderFeeEntitlement(ctx, req)
}

func (s *queryServer) Block(c context.Context, req *types.QueryBlockRequest) (*types.QueryBlockResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryBlock(ctx, req)
}

func (s *queryServer) TssKeygenMetric(c context.Context, req *types.QueryTssKeygenMetricRequest) (*types.QueryTssKeygenMetricResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTssKeygenMetric(ctx, req)
}

func (s *queryServer) TssMetric(c context.Context, req *types.QueryTssMetricRequest) (*types.QueryTssMetricResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryTssMetric(ctx, req)
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

func (s *queryServer) Codes(c context.Context, req *types.QueryCodesRequest) (*types.QueryCodesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	return &types.QueryCodesResponse{}, nil
}

func (s *queryServer) Export(c context.Context, req *types.QueryExportRequest) (*types.QueryExportResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	contentBz, err := queryExport(ctx, s.mgr)
	if err != nil {
		return nil, err
	}

	return &types.QueryExportResponse{
		Content: contentBz,
	}, nil
}

func (s *queryServer) Account(c context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryAccount(ctx, req)
}

func (s *queryServer) Balances(c context.Context, req *types.QueryBalancesRequest) (*types.QueryBalancesResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryBalances(ctx, req)
}

func (s *queryServer) Eip712TypedData(c context.Context, req *types.QueryEip712TypedDataRequest) (*types.QueryEip712TypedDataResponse, error) {
	ctx := s.unwrapSdkContext(c)
	return s.queryEip712TypedData(ctx, req)
}

func (s *queryServer) ContractInfo(c context.Context, req *types.QueryContractInfoRequest) (*types.QueryContractInfoResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryContractInfo(ctx, req)
}

func (s *queryServer) ContractInfos(c context.Context, req *types.QueryContractInfosRequest) (*types.QueryContractInfosResponse, error) {
	if err := checkHeightParam(req.Height); err != nil {
		return nil, err
	}
	ctx := s.unwrapSdkContext(c)
	return s.queryContractInfos(ctx, req)
}
