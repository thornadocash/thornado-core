package thornado

import (
	"sort"

	abci "github.com/cometbft/cometbft/abci/types"

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
	for _, na := range data.NodeAccounts {
		if err := na.Valid(); err != nil {
			return err
		}
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
	return configs
}

func initGenesis(ctx cosmos.Context, k keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	reserveAddr, _ := k.GetModuleAddress(ReserveName)
	ctx.Logger().Info("Reserve Module", "address", reserveAddr.String())
	bondAddr, _ := k.GetModuleAddress(BondName)
	ctx.Logger().Info("Bond Module", "address", bondAddr.String())
	asgardAddr, _ := k.GetModuleAddress(AsgardName)
	ctx.Logger().Info("Asgard Module", "address", asgardAddr.String())
	treasuryAddr, _ := k.GetModuleAddress(TreasuryName)
	ctx.Logger().Info("Treasury Module", "address", treasuryAddr.String())
	return nil
}

func ExportGenesis(ctx cosmos.Context, k keeper.Keeper) GenesisState {
	return DefaultGenesisState()
}
