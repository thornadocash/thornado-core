package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func GetCmdNodeQuery() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Node operator status queries",
	}

	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeStatus()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodes()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeMetrics()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeSlot()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeBond()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeFees()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeFeePool()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeAuctions()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeAuction()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeAuctionBids()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeBid()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeUpgrades()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeUpgrade()))
	cmd.AddCommand(withQueryFlags(GetCmdQueryNodeUpgradeVotes()))

	return cmd
}

func GetCmdQueryNodeStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status [node-address]",
		Short: "show node status",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.Node(ctx, &types.QueryNodeRequest{Address: args[0]})
		}),
	}
}

func GetCmdQueryNodes() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list nodes",
		Args:  cobra.ExactArgs(0),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, _ []string) (interface{}, error) {
			return qc.Nodes(ctx, &types.QueryNodesRequest{})
		}),
	}
}

func GetCmdQueryNodeMetrics() *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "show node set metrics and next slot requirements",
		Args:  cobra.ExactArgs(0),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, _ []string) (interface{}, error) {
			return qc.NodeMetrics(ctx, &types.QueryNodeMetricsRequest{})
		}),
	}
}

func GetCmdQueryNodeSlot() *cobra.Command {
	return &cobra.Command{
		Use:   "slot [slot]",
		Short: "show node slot",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			slot, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid slot: %w", err)
			}
			return qc.NodeSlot(ctx, &types.QueryNodeSlotRequest{Slot: slot})
		}),
	}
}

func GetCmdQueryNodeBond() *cobra.Command {
	return &cobra.Command{
		Use:   "bond [node-pubkey]",
		Short: "show node bond state",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.NodeBond(ctx, &types.QueryNodeBondRequest{NodePubKey: args[0]})
		}),
	}
}

func GetCmdQueryNodeFees() *cobra.Command {
	return &cobra.Command{
		Use:   "fees [node-pubkey]",
		Short: "show node fee entitlement",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.NodeFeeEntitlement(ctx, &types.QueryNodeFeeEntitlementRequest{NodePubKey: args[0]})
		}),
	}
}

func GetCmdQueryNodeFeePool() *cobra.Command {
	return &cobra.Command{
		Use:   "fee-pool",
		Short: "show public fee pool",
		Args:  cobra.ExactArgs(0),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, _ []string) (interface{}, error) {
			return qc.FeePool(ctx, &types.QueryFeePoolRequest{})
		}),
	}
}

func GetCmdQueryNodeAuctions() *cobra.Command {
	return &cobra.Command{
		Use:   "auctions",
		Short: "list node slot auctions",
		Args:  cobra.ExactArgs(0),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, _ []string) (interface{}, error) {
			return qc.NodeSlotAuctions(ctx, &types.QueryNodeSlotAuctionsRequest{})
		}),
	}
}

func GetCmdQueryNodeAuction() *cobra.Command {
	return &cobra.Command{
		Use:   "auction [auction-id]",
		Short: "show node slot auction",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.NodeSlotAuction(ctx, &types.QueryNodeSlotAuctionRequest{AuctionId: args[0]})
		}),
	}
}

func GetCmdQueryNodeAuctionBids() *cobra.Command {
	return &cobra.Command{
		Use:   "auction-bids [auction-id]",
		Short: "list bids for a node slot auction",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.NodeSlotAuctionBids(ctx, &types.QueryNodeSlotAuctionBidsRequest{AuctionId: args[0]})
		}),
	}
}

func GetCmdQueryNodeBid() *cobra.Command {
	return &cobra.Command{
		Use:   "bid [bid-id]",
		Short: "show node slot bid",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.NodeSlotBid(ctx, &types.QueryNodeSlotBidRequest{BidId: args[0]})
		}),
	}
}

func GetCmdQueryNodeUpgrades() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrades",
		Short: "list upgrade proposals",
		Args:  cobra.ExactArgs(0),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, _ []string) (interface{}, error) {
			return qc.UpgradeProposals(ctx, &types.QueryUpgradeProposalsRequest{})
		}),
	}
}

func GetCmdQueryNodeUpgrade() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade [name]",
		Short: "show upgrade proposal",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.UpgradeProposal(ctx, &types.QueryUpgradeProposalRequest{Name: args[0]})
		}),
	}
}

func GetCmdQueryNodeUpgradeVotes() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade-votes [name]",
		Short: "show upgrade votes",
		Args:  cobra.ExactArgs(1),
		RunE: queryNode(func(ctx context.Context, qc types.QueryClient, args []string) (interface{}, error) {
			return qc.UpgradeVotes(ctx, &types.QueryUpgradeVotesRequest{Name: args[0]})
		}),
	}
}

func queryNode(fn func(context.Context, types.QueryClient, []string) (interface{}, error)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientQueryContext(cmd)
		if err != nil {
			return err
		}
		clientCtx.OutputFormat = "json"
		res, err := fn(cmd.Context(), types.NewQueryClient(clientCtx), args)
		if err != nil {
			return err
		}
		return clientCtx.PrintObjectLegacy(res)
	}
}

func withQueryFlags(cmd *cobra.Command) *cobra.Command {
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
