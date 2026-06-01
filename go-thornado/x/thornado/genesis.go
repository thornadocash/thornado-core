package thornado

import (
	"fmt"
	"sort"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

func ValidateGenesis(data GenesisState) error {
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
	if err := validateGenesisNodeAccounts(data.NodeAccounts); err != nil {
		return err
	}
	for _, vault := range data.Vaults {
		if err := vault.Valid(); err != nil {
			return err
		}
	}
	for _, nf := range data.NetworkFees {
		if err := nf.Valid(); err != nil {
			return err
		}
	}
	return nil
}

func validateGenesisNodeAccounts(nodeAccounts NodeAccounts) error {
	seenConsensus := make(map[string]string)
	seenSecp256k1 := make(map[string]string)
	seenEd25519 := make(map[string]string)

	for _, na := range nodeAccounts {
		if err := na.Valid(); err != nil {
			return err
		}

		if na.NodeConsPubKey != "" {
			if other, ok := seenConsensus[na.NodeConsPubKey]; ok {
				return fmt.Errorf("duplicate node consensus pubkey %s for %s and %s", na.NodeConsPubKey, other, na.NodeAddress)
			}
			seenConsensus[na.NodeConsPubKey] = na.NodeAddress.String()
		}

		secpEmpty := na.PubKeySet.Secp256k1.IsEmpty()
		edEmpty := na.PubKeySet.Ed25519.IsEmpty()
		if secpEmpty != edEmpty {
			return fmt.Errorf("node account %s has incomplete pubkey set", na.NodeAddress)
		}
		if secpEmpty && edEmpty {
			if na.Status == NodeActive || na.Status == NodeSelected {
				return fmt.Errorf("active node account %s cannot have empty pubkey set", na.NodeAddress)
			}
			continue
		}

		secpAddr, err := na.PubKeySet.Secp256k1.GetThorAddress()
		if err != nil {
			return fmt.Errorf("node account %s has invalid secp256k1 pubkey: %w", na.NodeAddress, err)
		}
		if !secpAddr.Equals(na.NodeAddress) {
			return fmt.Errorf("node account %s does not match secp256k1 pubkey address %s", na.NodeAddress, secpAddr)
		}

		secpKey := na.PubKeySet.Secp256k1.String()
		if other, ok := seenSecp256k1[secpKey]; ok {
			return fmt.Errorf("duplicate node secp256k1 pubkey %s for %s and %s", secpKey, other, na.NodeAddress)
		}
		seenSecp256k1[secpKey] = na.NodeAddress.String()

		edKey := na.PubKeySet.Ed25519.String()
		if other, ok := seenEd25519[edKey]; ok {
			return fmt.Errorf("duplicate node ed25519 pubkey %s for %s and %s", edKey, other, na.NodeAddress)
		}
		seenEd25519[edKey] = na.NodeAddress.String()
	}

	return nil
}

func DefaultGenesisState() GenesisState {
	return GenesisState{
		ObservedTxInVoters:  make(ObservedTxVoters, 0),
		ObservedTxOutVoters: make(ObservedTxVoters, 0),
		TxOuts:              make([]TxOut, 0),
		NodeAccounts:        make(NodeAccounts, 0),
		Vaults:              make(Vaults, 0),
		LastChainHeights:    make([]LastChainHeight, 0),
		Network:             NewNetwork(),
		NetworkFees:         make([]NetworkFee, 0),
		Configs:             make([]Config, 0),
		NodeConfigs:         make([]NodeConfig, 0),
		ConfigDefaults:      DefaultGenesisConfigDefaults(),
	}
}

func DefaultGenesisConfigDefaults() []Config {
	values := constants.NewConfigValue().GetConfigValsByKeyname().Int64Values
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	configs := make([]Config, 0, len(keys))
	for _, key := range keys {
		configs = append(configs, Config{
			Key:   key,
			Value: values[key],
		})
	}
	configs = append(configs, Config{
		Key:   constants.ConfigKeyNodePauseChainGlobal,
		Value: 0,
	})
	return configs
}

func initGenesis(ctx cosmos.Context, k keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	reserveAddr, _ := k.GetModuleAddress(ReserveName)
	ctx.Logger().Info("Reserve Module", "address", reserveAddr.String())
	bondAddr, _ := k.GetModuleAddress(BondName)
	ctx.Logger().Info("Bond Module", "address", bondAddr.String())
	baseAddr, _ := k.GetModuleAddress(BaseName)
	ctx.Logger().Info("Base Module", "address", baseAddr.String())
	treasuryAddr, _ := k.GetModuleAddress(TreasuryName)
	ctx.Logger().Info("Treasury Module", "address", treasuryAddr.String())

	k.SetVersionWithCtx(ctx, constants.SWVersion)
	if err := k.SetNetwork(ctx, data.Network); err != nil {
		ctx.Logger().Error("fail to set network genesis", "error", err)
	}
	if data.LastSignedHeight > 0 {
		if err := k.SetLastSignedHeight(ctx, data.LastSignedHeight); err != nil {
			ctx.Logger().Error("fail to set last signed height genesis", "error", err)
		}
	}
	for _, height := range data.LastChainHeights {
		chain, err := common.NewChain(height.Chain)
		if err != nil {
			ctx.Logger().Error("fail to parse last chain height genesis", "chain", height.Chain, "error", err)
			continue
		}
		if err := k.SetLastChainHeight(ctx, chain, height.Height); err != nil {
			ctx.Logger().Error("fail to set last chain height genesis", "chain", height.Chain, "error", err)
		}
	}
	for _, voter := range data.ObservedTxInVoters {
		k.SetObservedTxInVoter(ctx, voter)
	}
	for _, voter := range data.ObservedTxOutVoters {
		k.SetObservedTxOutVoter(ctx, voter)
	}
	for _, txOut := range data.TxOuts {
		out := txOut
		if err := k.SetTxOut(ctx, &out); err != nil {
			ctx.Logger().Error("fail to set tx out genesis", "height", txOut.Height, "error", err)
		}
	}
	for _, na := range data.NodeAccounts {
		if err := k.SetNodeAccount(ctx, na); err != nil {
			ctx.Logger().Error("fail to set node account genesis", "node", na.NodeAddress.String(), "error", err)
		}
	}
	for _, vault := range data.Vaults {
		if err := k.SetVault(ctx, vault); err != nil {
			ctx.Logger().Error("fail to set vault genesis", "pubkey", vault.PubKey.String(), "error", err)
		}
	}
	for _, nf := range data.NetworkFees {
		if err := k.SaveNetworkFee(ctx, nf.Chain, nf); err != nil {
			ctx.Logger().Error("fail to set network fee genesis", "chain", nf.Chain.String(), "error", err)
		}
	}
	configDefaults := data.ConfigDefaults
	if len(configDefaults) == 0 {
		configDefaults = DefaultGenesisConfigDefaults()
	}
	for _, cfg := range configDefaults {
		k.SetConfig(ctx, cfg.Key, cfg.Value)
	}
	for _, cfg := range data.Configs {
		k.SetConfig(ctx, cfg.Key, cfg.Value)
	}
	for _, cfg := range data.NodeConfigs {
		if err := k.SetNodeConfig(ctx, cfg.Key, cfg.Value, cfg.Signer); err != nil {
			ctx.Logger().Error("fail to set node config genesis", "key", cfg.Key, "signer", cfg.Signer.String(), "error", err)
		}
	}
	return nil
}

func ExportGenesis(ctx cosmos.Context, k keeper.Keeper) GenesisState {
	return DefaultGenesisState()
}
