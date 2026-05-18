package params

import (
	"encoding/json"
	"fmt"
	"io"

	"google.golang.org/protobuf/reflect/protoreflect"

	"cosmossdk.io/x/tx/signing/aminojson"
	sdk "github.com/cosmos/cosmos-sdk/types"

	apicommon "gitlab.com/thorchain/thornode/v3/api/common"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func bech32Encoder(_ *aminojson.Encoder, v protoreflect.Value, w io.Writer) error {
	switch bz := v.Interface().(type) {
	case []byte:
		bz, err := json.Marshal(sdk.AccAddress(bz).String())
		if err != nil {
			return fmt.Errorf("failed to marshal bech32 address: %w", err)
		}
		_, err = w.Write(bz)
		return err
	default:
		return fmt.Errorf("unsupported type %T", bz)
	}
}

func assetEncoder(_ *aminojson.Encoder, v protoreflect.Value, w io.Writer) error {
	fra, ok := v.Interface().(protoreflect.Message)
	if !ok {
		return fmt.Errorf("unsupported protoreflect message type %T", v.Interface())
	}

	a, ok := fra.Interface().(*apicommon.Asset)
	if !ok {
		return fmt.Errorf("unsupported type %T", fra.Interface())
	}

	asset := common.Asset{
		Chain:   common.Chain(a.Chain),
		Symbol:  common.Symbol(a.Symbol),
		Ticker:  common.Ticker(a.Ticker),
		Synth:   a.Synth,
		Trade:   a.Trade,
		Secured: a.Secured,
	}

	bz, err := json.Marshal(asset)
	if err != nil {
		return err
	}
	_, err = w.Write(bz)
	return err
}

func keygenTypeEncoder(_ *aminojson.Encoder, v protoreflect.Value, w io.Writer) error {
	pm, ok := v.Interface().(protoreflect.EnumNumber)
	if !ok {
		return fmt.Errorf("unsupported protoreflect message type %T", v.Interface())
	}

	name, ok := types.KeygenType_name[int32(pm)]
	if !ok {
		return fmt.Errorf("unknown keygen type: %d", pm)
	}

	bz, err := json.Marshal(name)
	if err != nil {
		return fmt.Errorf("failed to marshal keygen type: %w", err)
	}
	_, err = w.Write(bz)
	return err
}
