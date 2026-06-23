package cli

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func GetCmdNodeOperator() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "node",
		Aliases: []string{"operator"},
		Short:   "Node operator transactions",
	}

	cmd.AddCommand(GetCmdNodeRegister())
	cmd.AddCommand(GetCmdNodeSetKeys())
	cmd.AddCommand(GetCmdNodeSetIP())
	cmd.AddCommand(GetCmdNodeSetVersion())
	cmd.AddCommand(GetCmdNodeMaint())
	cmd.AddCommand(GetCmdNodeLeave())
	cmd.AddCommand(GetCmdNodeRotateOperator())
	cmd.AddCommand(GetCmdNodePauseChain())
	cmd.AddCommand(GetCmdNodeResumeChain())
	cmd.AddCommand(GetCmdNodeBond())
	cmd.AddCommand(GetCmdNodeBondProvider())
	cmd.AddCommand(GetCmdNodeFees())
	cmd.AddCommand(GetCmdNodeBid())
	cmd.AddCommand(GetCmdNodeSale())
	cmd.AddCommand(GetCmdNodeUpgrade())

	return cmd
}

func GetCmdNodeRotateOperator() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate-operator [new-operator-pubkey]",
		Short: "rotate the registered node operator key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			operatorPubKey, err := common.NewPubKey(args[0])
			if err != nil {
				return fmt.Errorf("invalid operator pubkey: %w", err)
			}
			operatorAddress, err := operatorPubKey.GetThorAddress()
			if err != nil {
				return fmt.Errorf("invalid operator pubkey address: %w", err)
			}
			msg := types.NewMsgOperatorRotate(clientCtx.GetFromAddress(), operatorAddress, operatorPubKey.String(), common.Coin{})
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeRegister() *cobra.Command {
	return &cobra.Command{
		Use:   "register [secp256k1-pubkey] [consensus-pubkey] [ip-address]",
		Short: "register node keys, ip address, and current software version",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			keysMsg, err := newSetNodeKeysMsg(args[0], args[1], clientCtx.GetFromAddress())
			if err != nil {
				return err
			}
			ipMsg := types.NewMsgSetIPAddress(args[2], clientCtx.GetFromAddress())
			versionMsg := types.NewMsgSetVersion(constants.SWVersion.String(), clientCtx.GetFromAddress())
			if err := keysMsg.ValidateBasic(); err != nil {
				return err
			}
			if err := ipMsg.ValidateBasic(); err != nil {
				return err
			}
			if err := versionMsg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), keysMsg, ipMsg, versionMsg)
		},
	}
}

func GetCmdNodeSetKeys() *cobra.Command {
	return &cobra.Command{
		Use:   "set-keys [secp256k1-pubkey] [consensus-pubkey]",
		Short: "update node signing and consensus keys",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg, err := newSetNodeKeysMsg(args[0], args[1], clientCtx.GetFromAddress())
			if err != nil {
				return err
			}
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeSetIP() *cobra.Command {
	return &cobra.Command{
		Use:   "set-ip [ip-address]",
		Short: "update node ip address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := types.NewMsgSetIPAddress(args[0], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeSetVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "set-version",
		Short: "update node software version",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := types.NewMsgSetVersion(constants.SWVersion.String(), clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeMaint() *cobra.Command {
	return &cobra.Command{
		Use:     "maint [node-address]",
		Aliases: []string{"maintenance"},
		Short:   "toggle node maintenance mode",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			nodeAddr, err := cosmos.AccAddressFromBech32(args[0])
			if err != nil {
				return fmt.Errorf("invalid node address: %w", err)
			}
			msg := types.NewMsgMaint(nodeAddr, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeLeave() *cobra.Command {
	return &cobra.Command{
		Use:   "leave [node-address]",
		Short: "request node churn-out",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			nodeAddr, err := cosmos.AccAddressFromBech32(args[0])
			if err != nil {
				return fmt.Errorf("invalid node address: %w", err)
			}
			msg := types.NewMsgLeave(nodeAddr, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeBond() *cobra.Command {
	return newNodeBondCommand(
		"bond [node-pubkey] [operator-pubkey] [proof-json-or-file] [public-json-or-file]",
		"bond a node by spending shielded notes into bond escrow",
	)
}

func newNodeBondCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			proof, err := readJSONArg(args[2])
			if err != nil {
				return fmt.Errorf("invalid proof: %w", err)
			}
			public, err := readJSONArg(args[3])
			if err != nil {
				return fmt.Errorf("invalid public inputs: %w", err)
			}
			msg := types.NewMsgBondFromNotes(args[0], args[1], proof, public, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeBondProvider() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bond-provider",
		Aliases: []string{"bond-providers", "bonder", "bonders"},
		Short:   "Bond provider transactions",
	}
	cmd.AddCommand(newNodeBondCommand(
		"bond [node-pubkey] [operator-pubkey] [proof-json-or-file] [public-json-or-file]",
		"add shielded bond to a node as a bond provider",
	))
	cmd.AddCommand(newNodeFeesShieldCommand(
		"fees [node-pubkey] [commitments-json-or-csv] [fee-note-pubkeys-json-or-csv]",
		"claim and shield bond-provider fee share",
		func(args []string) (nodePubKey string, signature []byte, commitmentsArg string, feeNotePubKeysArg string, err error) {
			return args[0], nil, args[1], args[2], nil
		},
		3,
	))
	return cmd
}

func GetCmdNodeFees() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fees",
		Short: "Node fee transactions",
	}
	cmd.AddCommand(GetCmdNodeFeesSet())
	cmd.AddCommand(GetCmdNodeFeesShield())
	return cmd
}

func GetCmdNodeFeesSet() *cobra.Command {
	return &cobra.Command{
		Use:   "set [node-pubkey] [operator-fee-basis-points]",
		Short: "set the operator commission on bonder fee income",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			feeBasisPoints, err := parseUintArg(args[1], "operator-fee-basis-points")
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeOperatorFeeSet(args[0], feeBasisPoints, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeFeesShield() *cobra.Command {
	return newNodeFeesShieldCommand(
		"shield [node-pubkey] [operator-signature-hex|-] [commitments-json-or-csv] [fee-note-pubkeys-json-or-csv]",
		"settle and shield node fee share into private notes",
		func(args []string) (nodePubKey string, signature []byte, commitmentsArg string, feeNotePubKeysArg string, err error) {
			signature, err = parseOptionalHexArg(args[1], "operator-signature-hex")
			return args[0], signature, args[2], args[3], err
		},
		4,
	)
}

func newNodeFeesShieldCommand(
	use string,
	short string,
	parseArgs func(args []string) (nodePubKey string, signature []byte, commitmentsArg string, feeNotePubKeysArg string, err error),
	exactArgs int,
) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(exactArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			nodePubKey, signature, commitmentsArg, feeNotePubKeysArg, err := parseArgs(args)
			if err != nil {
				return err
			}
			commitments, err := parseCommitmentsArg(commitmentsArg)
			if err != nil {
				return err
			}
			feeNotePubKeys, err := parseCommitmentsArg(feeNotePubKeysArg)
			if err != nil {
				return err
			}
			msg := types.NewMsgShielderShieldFees(nodePubKey, signature, commitments, feeNotePubKeys, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeBid() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid",
		Short: "Node sale bid transactions",
	}
	cmd.AddCommand(GetCmdNodeBidCreate())
	return cmd
}

func GetCmdNodeBidCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create [auction-id] [operator-pubkey] [node-pubkey]",
		Short: "create a funded bid for a node slot auction",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeSlotAuctionBidCreate(args[0], args[1], args[2], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeSale() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sale",
		Short: "Node sale transactions",
	}
	cmd.AddCommand(GetCmdNodeSaleCreate())
	cmd.AddCommand(GetCmdNodeSaleSelectBid())
	cmd.AddCommand(GetCmdNodeSaleShieldEntitlement())
	return cmd
}

func GetCmdNodeSaleCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create [seller-node-pubkey] [reserve-sats] [expiry-height]",
		Short: "list a standby node slot for sale",
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

func GetCmdNodeSaleSelectBid() *cobra.Command {
	return &cobra.Command{
		Use:   "select-bid [auction-id] [bid-id]",
		Short: "select the winning bid for a node slot sale",
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

func GetCmdNodeSaleShieldEntitlement() *cobra.Command {
	return &cobra.Command{
		Use:   "shield [auction-id] [bid-id] [seller-commitments-json-or-csv] [deposit-pubkey] [signature]",
		Short: "shield a settled node sale seller entitlement",
		Args:  cobra.ExactArgs(5),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			commitments, err := parseCommitmentsArg(args[2])
			if err != nil {
				return err
			}
			msg := types.NewMsgNodeSaleShield(args[0], args[1], commitments, args[3], args[4], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeUpgrade() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Node upgrade voting transactions",
	}
	cmd.AddCommand(GetCmdNodeUpgradePropose())
	cmd.AddCommand(GetCmdNodeUpgradeApprove())
	cmd.AddCommand(GetCmdNodeUpgradeReject())
	return cmd
}

func GetCmdNodeUpgradePropose() *cobra.Command {
	const flagInfo = "info"
	cmd := &cobra.Command{
		Use:   "propose [name] [height]",
		Short: "propose an upgrade for active node vote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			height, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid upgrade height: %w", err)
			}
			info, _ := cmd.Flags().GetString(flagInfo)
			msg := types.NewMsgProposeUpgrade(args[0], height, info, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(flagInfo, "", "upgrade plan info")
	return cmd
}

func GetCmdNodeUpgradeApprove() *cobra.Command {
	return &cobra.Command{
		Use:   "approve [name]",
		Short: "approve a proposed upgrade",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := types.NewMsgApproveUpgrade(args[0], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdNodeUpgradeReject() *cobra.Command {
	return &cobra.Command{
		Use:   "reject [name]",
		Short: "reject a proposed upgrade",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := types.NewMsgRejectUpgrade(args[0], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func newSetNodeKeysMsg(secp256k1Raw, consensusRaw string, signer cosmos.AccAddress) (*types.MsgSetNodeKeys, error) {
	secp256k1Key, err := common.NewPubKey(secp256k1Raw)
	if err != nil {
		return nil, fmt.Errorf("invalid secp256k1 pubkey: %w", err)
	}
	nodeConsPubKey, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, consensusRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid consensus pubkey: %w", err)
	}
	nodeConsPubKeyStr, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeConsPub, nodeConsPubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid consensus pubkey encoding: %w", err)
	}
	return types.NewMsgSetNodeKeys(common.NewPubKeySet(secp256k1Key), nodeConsPubKeyStr, signer), nil
}

func parseOptionalHexArg(raw, name string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" || strings.EqualFold(raw, "none") || strings.EqualFold(raw, "null") {
		return nil, nil
	}
	out, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	return out, nil
}
