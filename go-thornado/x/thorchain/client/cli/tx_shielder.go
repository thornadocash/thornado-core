package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

func GetCmdShielder() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shielder",
		Short: "Shielder private custody transaction subcommands",
	}

	cmd.AddCommand(GetCmdShielderRegisterPow())
	cmd.AddCommand(GetCmdShielderPostCommitments())
	cmd.AddCommand(GetCmdShielderWithdraw())
	return cmd
}

func GetCmdShielderRegisterPow() *cobra.Command {
	return &cobra.Command{
		Use:   "register-pow [pow-token]",
		Short: "register POW token and request a Bitcoin deposit address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := types.NewMsgShielderRegisterPow(args[0], clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdShielderPostCommitments() *cobra.Command {
	return &cobra.Command{
		Use:   "post-commitments [deposit-id] [commitments-json-or-csv]",
		Short: "post split-note commitments for a matched Bitcoin deposit",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			depositID, err := common.NewTxID(args[0])
			if err != nil {
				return fmt.Errorf("invalid deposit id: %w", err)
			}
			commitments, err := parseCommitmentsArg(args[1])
			if err != nil {
				return err
			}

			msg := types.NewMsgShielderPostCommitments(depositID, commitments, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
}

func GetCmdShielderWithdraw() *cobra.Command {
	return &cobra.Command{
		Use:   "withdraw [proof-json-or-file] [public-json-or-file]",
		Short: "verify a Shielder withdrawal proof and queue Bitcoin keysign",
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

			msg := types.NewMsgShielderRequestWithdrawal(proof, public, clientCtx.GetFromAddress())
			if err = msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
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
