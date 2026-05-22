package thornado

import (
	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/thornadocash/go-thornado/common/cosmos"
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
		ChainContracts:      make([]ChainContract, 0),
		Mimirs:              make([]Mimir, 0),
		NodeMimirs:          make([]NodeMimir, 0),
	}
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
