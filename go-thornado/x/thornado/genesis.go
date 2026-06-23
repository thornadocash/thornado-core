package thornado

import (
	"fmt"
	"sort"
	"strings"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
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
		if secpEmpty {
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

		if !edEmpty {
			edKey := na.PubKeySet.Ed25519.String()
			if other, ok := seenEd25519[edKey]; ok {
				return fmt.Errorf("duplicate node ed25519 pubkey %s for %s and %s", edKey, other, na.NodeAddress)
			}
			seenEd25519[edKey] = na.NodeAddress.String()
		}
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
	state := DefaultGenesisState()

	if network, err := k.GetNetwork(ctx); err == nil {
		state.Network = network
	} else {
		ctx.Logger().Error("fail to export network genesis", "error", err)
	}
	if height, err := k.GetLastSignedHeight(ctx); err == nil {
		state.LastSignedHeight = height
	} else {
		ctx.Logger().Error("fail to export last signed height genesis", "error", err)
	}
	if heights, err := k.GetLastChainHeights(ctx); err == nil {
		for chain, height := range heights {
			state.LastChainHeights = append(state.LastChainHeights, LastChainHeight{
				Chain:  chain.String(),
				Height: height,
			})
		}
		sort.Slice(state.LastChainHeights, func(i, j int) bool {
			return state.LastChainHeights[i].Chain < state.LastChainHeights[j].Chain
		})
	} else {
		ctx.Logger().Error("fail to export last chain heights genesis", "error", err)
	}

	appendObservedTxVoters(ctx, k, k.GetObservedTxInVoterIterator(ctx), &state.ObservedTxInVoters, "observed tx in voter")
	appendObservedTxVoters(ctx, k, k.GetObservedTxOutVoterIterator(ctx), &state.ObservedTxOutVoters, "observed tx out voter")
	appendTxOuts(ctx, k, &state.TxOuts)
	appendNodeAccounts(ctx, k, &state.NodeAccounts)
	appendVaults(ctx, k, &state.Vaults)
	appendNetworkFees(ctx, k, &state.NetworkFees)
	appendConfigs(ctx, k, &state.Configs)
	appendNodeConfigs(ctx, k, &state.NodeConfigs)

	return state
}

func appendObservedTxVoters(ctx cosmos.Context, k keeper.Keeper, iter cosmos.Iterator, voters *ObservedTxVoters, label string) {
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var voter ObservedTxVoter
		if err := k.Cdc().Unmarshal(iter.Value(), &voter); err != nil {
			ctx.Logger().Error("fail to export genesis record", "type", label, "key", string(iter.Key()), "error", err)
			continue
		}
		*voters = append(*voters, voter)
	}
}

func appendTxOuts(ctx cosmos.Context, k keeper.Keeper, txOuts *[]TxOut) {
	iter := k.GetTxOutIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var txOut TxOut
		if err := k.Cdc().Unmarshal(iter.Value(), &txOut); err != nil {
			ctx.Logger().Error("fail to export tx out genesis", "key", string(iter.Key()), "error", err)
			continue
		}
		*txOuts = append(*txOuts, txOut)
	}
	sort.Slice(*txOuts, func(i, j int) bool {
		return (*txOuts)[i].Height < (*txOuts)[j].Height
	})
}

func appendNodeAccounts(ctx cosmos.Context, k keeper.Keeper, nodeAccounts *NodeAccounts) {
	iter := k.GetNodeAccountIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var node NodeAccount
		if err := k.Cdc().Unmarshal(iter.Value(), &node); err != nil {
			ctx.Logger().Error("fail to export node account genesis", "key", string(iter.Key()), "error", err)
			continue
		}
		*nodeAccounts = append(*nodeAccounts, node)
	}
	sort.Slice(*nodeAccounts, func(i, j int) bool {
		return strings.Compare((*nodeAccounts)[i].NodeAddress.String(), (*nodeAccounts)[j].NodeAddress.String()) < 0
	})
}

func appendVaults(ctx cosmos.Context, k keeper.Keeper, vaults *Vaults) {
	iter := k.GetVaultIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var vault Vault
		if err := k.Cdc().Unmarshal(iter.Value(), &vault); err != nil {
			ctx.Logger().Error("fail to export vault genesis", "key", string(iter.Key()), "error", err)
			continue
		}
		*vaults = append(*vaults, vault)
	}
	sort.Slice(*vaults, func(i, j int) bool {
		return strings.Compare((*vaults)[i].PubKey.String(), (*vaults)[j].PubKey.String()) < 0
	})
}

func appendNetworkFees(ctx cosmos.Context, k keeper.Keeper, networkFees *[]NetworkFee) {
	iter := k.GetNetworkFeeIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var fee NetworkFee
		if err := k.Cdc().Unmarshal(iter.Value(), &fee); err != nil {
			ctx.Logger().Error("fail to export network fee genesis", "key", string(iter.Key()), "error", err)
			continue
		}
		*networkFees = append(*networkFees, fee)
	}
	sort.Slice(*networkFees, func(i, j int) bool {
		return strings.Compare((*networkFees)[i].Chain.String(), (*networkFees)[j].Chain.String()) < 0
	})
}

func appendConfigs(ctx cosmos.Context, k keeper.Keeper, configs *[]Config) {
	defaults := DefaultGenesisConfigDefaults()
	defaultValueByKey := make(map[string]int64, len(defaults))
	canonicalKeyByUpper := make(map[string]string, len(defaults))
	for _, cfg := range defaults {
		defaultValueByKey[cfg.Key] = cfg.Value
		canonicalKeyByUpper[strings.ToUpper(cfg.Key)] = cfg.Key
	}

	iter := k.GetConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var value types.ProtoInt64
		if err := k.Cdc().Unmarshal(iter.Value(), &value); err != nil {
			ctx.Logger().Error("fail to export config genesis", "key", string(iter.Key()), "error", err)
			continue
		}

		key := genesisIteratorKeyName(iter.Key())
		if canonical, ok := canonicalKeyByUpper[strings.ToUpper(key)]; ok {
			key = canonical
		}
		if defaultValue, ok := defaultValueByKey[key]; ok && defaultValue == value.Value {
			continue
		}
		*configs = append(*configs, Config{Key: key, Value: value.Value})
	}
	sort.Slice(*configs, func(i, j int) bool {
		return (*configs)[i].Key < (*configs)[j].Key
	})
}

func appendNodeConfigs(ctx cosmos.Context, k keeper.Keeper, nodeConfigs *[]types.NodeConfig) {
	iter := k.GetNodeConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var configs types.NodeConfigs
		if err := k.Cdc().Unmarshal(iter.Value(), &configs); err != nil {
			ctx.Logger().Error("fail to export node config genesis", "key", string(iter.Key()), "error", err)
			continue
		}
		*nodeConfigs = append(*nodeConfigs, configs.Configs...)
	}
	sort.Slice(*nodeConfigs, func(i, j int) bool {
		left := (*nodeConfigs)[i]
		right := (*nodeConfigs)[j]
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		return left.Signer.String() < right.Signer.String()
	})
}

func genesisIteratorKeyName(key []byte) string {
	text := string(key)
	if idx := strings.LastIndex(text, "/"); idx >= 0 {
		return text[idx+1:]
	}
	return text
}
