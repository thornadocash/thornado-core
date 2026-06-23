package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cmtcfg "github.com/cometbft/cometbft/config"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/go-bip39"
	"github.com/spf13/cobra"

	errorsmod "cosmossdk.io/errors"
	sdkunsafe "cosmossdk.io/math/unsafe"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	sdkcodec "github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	ckeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"

	thornadocmd "github.com/thornadocash/go-thornado/cmd"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	thornadoed25519 "github.com/thornadocash/go-thornado/common/crypto/ed25519"
)

const (
	flagOperatorName     = "operator-name"
	flagOperatorMnemonic = "operator-mnemonic"
	flagOperatorPassword = "operator-password"
	flagSkipOperatorKey  = "skip-operator-key"
)

type thornadoInitInfo struct {
	Moniker             string          `json:"moniker" yaml:"moniker"`
	ChainID             string          `json:"chain_id" yaml:"chain_id"`
	NodeID              string          `json:"node_id" yaml:"node_id"`
	OperatorName        string          `json:"operator_name" yaml:"operator_name"`
	OperatorAddress     string          `json:"operator_address,omitempty" yaml:"operator_address,omitempty"`
	NodeSecp256K1PubKey string          `json:"node_secp256k1_pubkey,omitempty" yaml:"node_secp256k1_pubkey,omitempty"`
	NodeConsensusPubKey string          `json:"node_consensus_pub_key" yaml:"node_consensus_pub_key"`
	OperatorHDPath      string          `json:"operator_hd_path" yaml:"operator_hd_path"`
	ConsensusHDPath     string          `json:"consensus_hd_path" yaml:"consensus_hd_path"`
	P2PHDPath           string          `json:"p2p_hd_path" yaml:"p2p_hd_path"`
	OperatorMnemonic    string          `json:"operator_mnemonic,omitempty" yaml:"operator_mnemonic,omitempty"`
	AppMessage          json.RawMessage `json:"app_message" yaml:"app_message"`
}

func thornadoInitCmd(mbm module.BasicManager, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [moniker]",
		Short: "Initialize Thornado node files and operator keys from one root secret",
		Long:  "Initialize Thornado configuration, genesis, CometBFT keys, and local operator keys from one root mnemonic.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			cdc := clientCtx.Codec

			serverCtx := server.GetServerContextFromCmd(cmd)
			config := serverCtx.Config
			config.SetRoot(clientCtx.HomeDir)

			chainID, _ := cmd.Flags().GetString(flags.FlagChainID)
			switch {
			case chainID != "":
			case clientCtx.ChainID != "":
				chainID = clientCtx.ChainID
			default:
				chainID = fmt.Sprintf("test-chain-%v", sdkunsafe.Str(6))
			}

			mnemonic, generated, err := resolveRootMnemonic(cmd)
			if err != nil {
				return err
			}

			initHeight, _ := cmd.Flags().GetInt64(flags.FlagInitHeight)
			if initHeight < 1 {
				initHeight = 1
			}

			overwrite, _ := cmd.Flags().GetBool(genutilcli.FlagOverwrite)
			nodeID, valPubKey, err := initializeDerivedNodeValidatorFiles(config, mnemonic, overwrite)
			if err != nil {
				return err
			}
			nodeConsPubKey, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeConsPub, valPubKey)
			if err != nil {
				return fmt.Errorf("encode consensus pubkey: %w", err)
			}

			config.Moniker = args[0]
			genFile := config.GenesisFile()
			defaultDenom, _ := cmd.Flags().GetString(genutilcli.FlagDefaultBondDenom)

			_, err = os.Stat(genFile)
			if !overwrite && !os.IsNotExist(err) {
				return fmt.Errorf("genesis.json file already exists: %v", genFile)
			}

			if defaultDenom != "" {
				sdk.DefaultBondDenom = defaultDenom
			}
			appGenState := mbm.DefaultGenesis(cdc)
			appState, err := json.MarshalIndent(appGenState, "", " ")
			if err != nil {
				return errorsmod.Wrap(err, "Failed to marshal default genesis state")
			}

			appGenesis := &genutiltypes.AppGenesis{}
			if _, err := os.Stat(genFile); err != nil {
				if !os.IsNotExist(err) {
					return err
				}
			} else {
				appGenesis, err = genutiltypes.AppGenesisFromFile(genFile)
				if err != nil {
					return errorsmod.Wrap(err, "Failed to read genesis doc from file")
				}
			}

			appGenesis.AppName = version.AppName
			appGenesis.AppVersion = version.Version
			appGenesis.ChainID = chainID
			appGenesis.AppState = appState
			appGenesis.InitialHeight = initHeight
			appGenesis.Consensus = &genutiltypes.ConsensusGenesis{
				Validators: nil,
				Params:     cmttypes.DefaultConsensusParams(),
			}

			consensusKey, err := cmd.Flags().GetString(genutilcli.FlagConsensusKeyAlgo)
			if err != nil {
				return errorsmod.Wrap(err, "Failed to get consensus key algo")
			}
			appGenesis.Consensus.Params.Validator.PubKeyTypes = []string{consensusKey}

			if err = genutil.ExportGenesisFile(appGenesis, genFile); err != nil {
				return errorsmod.Wrap(err, "Failed to export genesis file")
			}
			cmtcfg.WriteConfigFile(filepath.Join(config.RootDir, "config", "config.toml"), config)

			operatorName, _ := cmd.Flags().GetString(flagOperatorName)
			operatorName = strings.TrimSpace(operatorName)
			if operatorName == "" {
				operatorName = strings.TrimSpace(args[0])
			}

			info := thornadoInitInfo{
				Moniker:             config.Moniker,
				ChainID:             chainID,
				NodeID:              nodeID,
				OperatorName:        operatorName,
				NodeConsensusPubKey: nodeConsPubKey,
				OperatorHDPath:      thornadocmd.ThornadoHDPath,
				ConsensusHDPath:     thornadocmd.ThornadoConsensusHDPath,
				P2PHDPath:           thornadocmd.ThornadoP2PHDPath,
				AppMessage:          appState,
			}
			if generated {
				info.OperatorMnemonic = mnemonic
			}

			skipOperatorKey, _ := cmd.Flags().GetBool(flagSkipOperatorKey)
			if !skipOperatorKey {
				keyInfo, err := ensureDerivedOperatorKeys(cmd, config.RootDir, operatorName, mnemonic)
				if err != nil {
					return err
				}
				info.OperatorAddress = keyInfo.Address
				info.NodeSecp256K1PubKey = keyInfo.NodeSecp256K1PubKey
			}

			return displayThornadoInitInfo(info)
		},
	}

	cmd.Flags().String(flags.FlagHome, defaultNodeHome, "node's home directory")
	cmd.Flags().BoolP(genutilcli.FlagOverwrite, "o", false, "overwrite the genesis and derived node key files")
	cmd.Flags().Bool(genutilcli.FlagRecover, false, "provide root seed phrase to recover existing keys")
	cmd.Flags().String(flags.FlagChainID, "", "genesis file chain-id, if left blank will be randomly created")
	cmd.Flags().String(genutilcli.FlagDefaultBondDenom, "", "genesis file default denomination, if left blank default value is 'stake'")
	cmd.Flags().Int64(flags.FlagInitHeight, 1, "specify the initial block height at genesis")
	cmd.Flags().String(genutilcli.FlagConsensusKeyAlgo, cmted25519.KeyType, "algorithm to use for the consensus key")
	cmd.Flags().String(flagOperatorName, "", "local operator key name; defaults to moniker")
	cmd.Flags().String(flagOperatorMnemonic, "", "root mnemonic to derive operator and node keys")
	cmd.Flags().String(flagOperatorPassword, "", "operator keyring password; prefer SIGNER_PASSWD")
	cmd.Flags().Bool(flagSkipOperatorKey, false, "skip writing local operator keyring keys")
	_ = cmd.Flags().MarkHidden(flagOperatorPassword)

	return cmd
}

func resolveRootMnemonic(cmd *cobra.Command) (string, bool, error) {
	if raw, _ := cmd.Flags().GetString(flagOperatorMnemonic); strings.TrimSpace(raw) != "" {
		mnemonic := strings.TrimSpace(raw)
		if !bip39.IsMnemonicValid(mnemonic) {
			return "", false, fmt.Errorf("invalid operator mnemonic")
		}
		return mnemonic, false, nil
	}

	recover, _ := cmd.Flags().GetBool(genutilcli.FlagRecover)
	if recover {
		inBuf := bufio.NewReader(cmd.InOrStdin())
		mnemonic, err := input.GetString("Enter your root mnemonic", inBuf)
		if err != nil {
			return "", false, err
		}
		mnemonic = strings.TrimSpace(mnemonic)
		if !bip39.IsMnemonicValid(mnemonic) {
			return "", false, fmt.Errorf("invalid mnemonic")
		}
		return mnemonic, false, nil
	}

	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", false, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", false, err
	}
	return mnemonic, true, nil
}

func initializeDerivedNodeValidatorFiles(config *cmtcfg.Config, mnemonic string, overwrite bool) (string, cryptotypes.PubKey, error) {
	nodeKeyFile := config.NodeKeyFile()
	if err := os.MkdirAll(filepath.Dir(nodeKeyFile), 0o700); err != nil {
		return "", nil, fmt.Errorf("could not create node key directory: %w", err)
	}
	var nodeKey *p2p.NodeKey
	if !overwrite {
		if existing, err := p2p.LoadNodeKey(nodeKeyFile); err == nil {
			nodeKey = existing
		} else if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	if nodeKey == nil {
		privKey, err := deriveCometEd25519PrivKey(mnemonic, thornadocmd.ThornadoP2PHDPath)
		if err != nil {
			return "", nil, fmt.Errorf("derive p2p key: %w", err)
		}
		nodeKey = &p2p.NodeKey{PrivKey: privKey}
		if err := nodeKey.SaveAs(nodeKeyFile); err != nil {
			return "", nil, err
		}
	}

	pvKeyFile := config.PrivValidatorKeyFile()
	if err := os.MkdirAll(filepath.Dir(pvKeyFile), 0o700); err != nil {
		return "", nil, fmt.Errorf("could not create priv validator key directory: %w", err)
	}
	pvStateFile := config.PrivValidatorStateFile()
	if err := os.MkdirAll(filepath.Dir(pvStateFile), 0o700); err != nil {
		return "", nil, fmt.Errorf("could not create priv validator state directory: %w", err)
	}

	var filePV *privval.FilePV
	if !overwrite {
		if _, err := os.Stat(pvKeyFile); err == nil {
			filePV = privval.LoadFilePV(pvKeyFile, pvStateFile)
		} else if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	if filePV == nil {
		privKey, err := deriveCometEd25519PrivKey(mnemonic, thornadocmd.ThornadoConsensusHDPath)
		if err != nil {
			return "", nil, fmt.Errorf("derive consensus key: %w", err)
		}
		filePV = privval.NewFilePV(privKey, pvKeyFile, pvStateFile)
		filePV.Save()
	}

	tmValPubKey, err := filePV.GetPubKey()
	if err != nil {
		return "", nil, err
	}
	valPubKey, err := cryptocodec.FromCmtPubKeyInterface(tmValPubKey)
	if err != nil {
		return "", nil, err
	}
	return string(nodeKey.ID()), valPubKey, nil
}

func deriveCometEd25519PrivKey(mnemonic, path string) (cmted25519.PrivKey, error) {
	key, err := thornadoed25519.GetPrivateKeyFromMnemonic(mnemonic, "", path)
	if err != nil {
		return nil, err
	}
	if len(key) != cmted25519.PrivateKeySize {
		return nil, fmt.Errorf("unexpected ed25519 private key length %d", len(key))
	}
	return cmted25519.PrivKey(key), nil
}

type operatorKeyInfo struct {
	Address             string
	NodeSecp256K1PubKey string
}

func ensureDerivedOperatorKeys(cmd *cobra.Command, home, operatorName, mnemonic string) (operatorKeyInfo, error) {
	password, err := resolveOperatorPassword(cmd)
	if err != nil {
		return operatorKeyInfo{}, err
	}

	kb, err := newOperatorKeyring(home, password)
	if err != nil {
		return operatorKeyInfo{}, err
	}
	record, err := kb.Key(operatorName)
	if err != nil {
		record, err = kb.NewAccount(operatorName, mnemonic, ckeys.DefaultBIP39Passphrase, thornadocmd.ThornadoHDPath, hd.Secp256k1)
		if err != nil {
			return operatorKeyInfo{}, fmt.Errorf("create operator key: %w", err)
		}
	}
	addr, err := record.GetAddress()
	if err != nil {
		return operatorKeyInfo{}, err
	}
	pubKey, err := record.GetPubKey()
	if err != nil {
		return operatorKeyInfo{}, err
	}
	nodePubKey, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey)
	if err != nil {
		return operatorKeyInfo{}, err
	}
	if _, err = common.NewPubKey(nodePubKey); err != nil {
		return operatorKeyInfo{}, err
	}
	return operatorKeyInfo{
		Address:             addr.String(),
		NodeSecp256K1PubKey: nodePubKey,
	}, nil
}

func resolveOperatorPassword(cmd *cobra.Command) (string, error) {
	if password, _ := cmd.Flags().GetString(flagOperatorPassword); strings.TrimSpace(password) != "" {
		return strings.TrimSpace(password), nil
	}
	if password := strings.TrimSpace(os.Getenv(cosmos.EnvSignerPassword)); password != "" {
		return password, nil
	}
	inBuf := bufio.NewReader(cmd.InOrStdin())
	password, err := input.GetPassword("Enter operator keyring password:", inBuf)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(password), nil
}

type cyclingPasswordReader struct {
	data   []byte
	offset int
}

func newCyclingPasswordReader(password string) *cyclingPasswordReader {
	return &cyclingPasswordReader{data: []byte(password + "\n")}
}

func (r *cyclingPasswordReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.data[r.offset]
		r.offset = (r.offset + 1) % len(r.data)
	}
	return len(p), nil
}

func newOperatorKeyring(home, password string) (ckeys.Keyring, error) {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := sdkcodec.NewProtoCodec(registry)
	return ckeys.New(sdk.KeyringServiceName(), ckeys.BackendFile, home, newCyclingPasswordReader(password), cdc, func(options *ckeys.Options) {
		options.SupportedAlgos = ckeys.SigningAlgoList{hd.Secp256k1}
	})
}

func displayThornadoInitInfo(info thornadoInitInfo) error {
	out, err := json.MarshalIndent(info, "", " ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stderr, "%s\n", string(sdk.MustSortJSON(out)))
	return err
}
