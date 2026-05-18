package params

import (
	gogoproto "github.com/cosmos/gogoproto/proto"
	"google.golang.org/protobuf/reflect/protoregistry"

	txsigning "cosmossdk.io/x/tx/signing"
	"cosmossdk.io/x/tx/signing/aminojson"
	"cosmossdk.io/x/tx/signing/textual"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
)

func TxConfig(cdc codec.Codec, textualCoinMetadataQueryFn textual.CoinMetadataQueryFn) (client.TxConfig, error) {
	enabledSignModes := []signing.SignMode{
		signing.SignMode_SIGN_MODE_DIRECT,
		signing.SignMode_SIGN_MODE_DIRECT_AUX,
	}
	if textualCoinMetadataQueryFn != nil {
		enabledSignModes = append(enabledSignModes, signing.SignMode_SIGN_MODE_TEXTUAL)
	}
	aminoEncoder := aminojson.NewEncoder(aminojson.EncoderOptions{
		FileResolver: gogoproto.HybridResolver,
		TypeResolver: protoregistry.GlobalTypes,
		EnumAsString: false, // ensure enum as string is disabled
	})
	aminoEncoder.DefineFieldEncoding("bech32", bech32Encoder)
	aminoEncoder.DefineFieldEncoding("asset", assetEncoder)
	aminoEncoder.DefineFieldEncoding("keygen_type", keygenTypeEncoder)
	aminoHandler := aminojson.NewSignModeHandler(aminojson.SignModeHandlerOptions{
		FileResolver: gogoproto.HybridResolver,
		TypeResolver: protoregistry.GlobalTypes,
		Encoder:      &aminoEncoder,
	})
	txConfigOpts := tx.ConfigOptions{
		EnabledSignModes:           enabledSignModes,
		TextualCoinMetadataQueryFn: textualCoinMetadataQueryFn,
		CustomSignModes: []txsigning.SignModeHandler{
			aminoHandler,
		},
		ProtoEncoder: ebifrost.TxEncoder(tx.DefaultTxEncoder()),
		ProtoDecoder: ebifrost.TxDecoder(cdc, tx.DefaultTxDecoder(cdc)),
		JSONEncoder:  ebifrost.JSONTxEncoder(cdc, tx.DefaultJSONTxEncoder(cdc)),
		JSONDecoder:  ebifrost.JSONTxDecoder(cdc, tx.DefaultJSONTxDecoder(cdc)),
	}
	return tx.NewTxConfigWithOptions(
		cdc,
		txConfigOpts,
	)
}
