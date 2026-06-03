package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func GetCmdShielder() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shielder",
		Short: "Shielder shield and redeem transaction subcommands",
	}

	cmd.AddCommand(GetCmdShielderShield())
	cmd.AddCommand(GetCmdShielderRedeem())
	cmd.AddCommand(GetCmdShielderShieldFees())
	cmd.AddCommand(GetCmdNodeSlotAuctionCreate())
	cmd.AddCommand(GetCmdNodeSlotAuctionBidPow())
	cmd.AddCommand(GetCmdNodeSlotAuctionSelectBid())
	cmd.AddCommand(GetCmdNodeSlotAuctionShield())
	return cmd
}

func GetCmdDepositRequestPow() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "request-deposit [pow-token] [deposit-pubkey]",
		Short: "register POW token and request a Bitcoin deposit address",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			operatorPubKey, err := cmd.Flags().GetString("operator-pubkey")
			if err != nil {
				return err
			}
			nodePubKey, err := cmd.Flags().GetString("node-pubkey")
			if err != nil {
				return err
			}
			powDurationMs, err := cmd.Flags().GetUint64("pow-duration-ms")
			if err != nil {
				return err
			}
			msg := types.NewMsgDepositRequestPow(args[0], args[1], operatorPubKey, nodePubKey)
			msg.PowDurationMs = powDurationMs
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("operator-pubkey", "", "operator mnemonic root pubkey for node bond deposits")
	cmd.Flags().String("node-pubkey", "", "node consensus pubkey to bond for")
	cmd.Flags().Uint64("pow-duration-ms", 0, "local proof-of-work solve time in milliseconds")
	return cmd
}

func GetCmdShielderShield() *cobra.Command {
	return &cobra.Command{
		Use:   "shield [commitments-json-or-csv] [deposit-pubkey] [signature]",
		Short: "insert Shielder commitments for a settled Bitcoin deposit",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			commitments, err := parseCommitmentsArg(args[0])
			if err != nil {
				return err
			}

			msg := &types.MsgShielderShield{
				Commitments:   commitments,
				DepositPubkey: strings.TrimSpace(args[1]),
				Signature:     strings.TrimSpace(args[2]),
			}
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdShielderRedeem() *cobra.Command {
	return &cobra.Command{
		Use:   "redeem [proof-json-or-file] [public-json-or-file]",
		Short: "spend a Shielder note and request a Bitcoin withdrawal",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proof, err := readJSONArg(args[0])
			if err != nil {
				return fmt.Errorf("invalid proof: %w", err)
			}
			public, err := readJSONArg(args[1])
			if err != nil {
				return fmt.Errorf("invalid public inputs: %w", err)
			}

			msg := types.NewMsgShielderRedeem(proof, public)
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdShielderShieldFees() *cobra.Command {
	return &cobra.Command{
		Use:   "shield-fees [node-pubkey] [operator-signature-hex] [commitments-json-or-csv] [fee-note-pubkeys-json-or-csv]",
		Short: "settle and shield node fee share into private Shielder notes",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			signature, err := hex.DecodeString(strings.TrimSpace(args[1]))
			if err != nil {
				return fmt.Errorf("invalid operator signature hex: %w", err)
			}
			commitments, err := parseCommitmentsArg(args[2])
			if err != nil {
				return err
			}
			feeNotePubKeys, err := parseCommitmentsArg(args[3])
			if err != nil {
				return err
			}
			msg := types.NewMsgShielderShieldFees(args[0], signature, commitments, feeNotePubKeys, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeSlotAuctionCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "auction-create [seller-node-pubkey] [reserve-sats] [expiry-height]",
		Short: "list a standby node slot for auction",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			reserve, err := parseUintArg(args[1], "reserve-sats")
			if err != nil {
				return err
			}
			expiry, err := parseIntArg(args[2], "expiry-height")
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeSlotAuctionCreate(args[0], reserve, expiry, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeSlotAuctionBidPow() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auction-bid-pow [auction-id] [pow-token] [operator-pubkey] [node-pubkey]",
		Short: "request a Bitcoin deposit address for a node slot auction bid",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			powDurationMs, err := cmd.Flags().GetUint64("pow-duration-ms")
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeSlotAuctionBidPow(args[0], args[1], args[2], args[3], clientCtx.GetFromAddress())
			msg.PowDurationMs = powDurationMs
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().Uint64("pow-duration-ms", 0, "local proof-of-work solve time in milliseconds")
	return cmd
}

func GetCmdNodeSlotAuctionSelectBid() *cobra.Command {
	return &cobra.Command{
		Use:   "auction-select-bid [auction-id] [bid-id]",
		Short: "select a winning bid for a node slot auction",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeSlotAuctionSelectBid(args[0], args[1], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeSlotAuctionShield() *cobra.Command {
	return &cobra.Command{
		Use:   "auction-shield [auction-id] [bid-id] [seller-commitments-json-or-csv]",
		Short: "shield a winning node slot bid into seller notes and unwithdrawable bond commitments",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			commitments, err := parseCommitmentsArg(args[2])
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeSlotAuctionShield(args[0], args[1], commitments, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func parseUintArg(raw, name string) (uint64, error) {
	var v uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &v); err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return v, nil
}

func parseIntArg(raw, name string) (int64, error) {
	var v int64
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &v); err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return v, nil
}

func parseCommitmentsArg(raw string) ([]string, error) {
	bz, err := readMaybeFile(raw)
	if err != nil {
		return nil, err
	}

	var commitments []string
	if json.Unmarshal(bz, &commitments) == nil {
		return commitments, nil
	}

	for _, part := range strings.Split(string(bz), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			commitments = append(commitments, part)
		}
	}
	if len(commitments) == 0 {
		return nil, fmt.Errorf("missing commitments")
	}
	return commitments, nil
}

func readJSONArg(raw string) ([]byte, error) {
	bz, err := readMaybeFile(raw)
	if err != nil {
		return nil, err
	}
	if !json.Valid(bz) {
		return nil, fmt.Errorf("not valid json")
	}
	return bz, nil
}

func readMaybeFile(raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "@") {
		return os.ReadFile(strings.TrimPrefix(raw, "@"))
	}
	if strings.HasSuffix(raw, ".json") {
		if bz, err := os.ReadFile(raw); err == nil {
			return bz, nil
		}
	}
	return []byte(raw), nil
}
