package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateDenom{}, ModuleName+"/CreateDenom", nil)
	cdc.RegisterConcrete(&MsgMintTokens{}, ModuleName+"/MintTokens", nil)
	cdc.RegisterConcrete(&MsgBurnTokens{}, ModuleName+"/BurnTokens", nil)
	cdc.RegisterConcrete(&MsgChangeDenomAdmin{}, ModuleName+"/ChangeAdmin", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgCreateDenom{},
		&MsgMintTokens{},
		&MsgBurnTokens{},
		&MsgChangeDenomAdmin{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
