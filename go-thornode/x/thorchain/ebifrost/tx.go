package ebifrost

import (
	"errors"
	"fmt"

	protov2 "google.golang.org/protobuf/proto"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

var _ sdk.Tx = wInjectTx{}

type wInjectTx struct {
	Tx  *types.InjectTx
	cdc codec.Codec
}

func NewInjectTx(cdc codec.Codec, msgs []sdk.Msg) wInjectTx {
	msgsAny := make([]*codectypes.Any, len(msgs))
	for i, msg := range msgs {
		msgAny, err := codectypes.NewAnyWithValue(msg)
		if err != nil {
			panic(err)
		}
		msgsAny[i] = msgAny
	}

	return wInjectTx{
		Tx: &types.InjectTx{
			Messages: msgsAny,
		},
		cdc: cdc,
	}
}

func (w wInjectTx) GetMsgs() []sdk.Msg {
	msgs, err := sdktx.GetMsgs(w.Tx.Messages, "thorchain.InjectTx")
	if err != nil {
		panic(err)
	}
	return msgs
}

func (w wInjectTx) GetMsgsV2() ([]protov2.Message, error) {
	msgsv2 := make([]protov2.Message, len(w.Tx.Messages))
	for i, msg := range w.Tx.Messages {
		_, msgv2, err := w.cdc.GetMsgAnySigners(msg)
		if err != nil {
			return nil, err
		}

		msgsv2[i] = msgv2
	}

	return msgsv2, nil
}

func (w wInjectTx) AsAny() *codectypes.Any {
	tx := sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: w.Tx.Messages,
		},
	}
	return codectypes.UnsafePackAny(&tx)
}

func TxEncoder(txEncoder sdk.TxEncoder) sdk.TxEncoder {
	return func(tx sdk.Tx) ([]byte, error) {
		itx, ok := tx.(wInjectTx)
		if !ok {
			// not an inject tx, let upstream encoder handle it
			if txEncoder != nil {
				return txEncoder(tx)
			} else {
				return nil, fmt.Errorf("tx is not of type wInjectTx")
			}
		}

		txBz, err := itx.Tx.Marshal()
		if err != nil {
			return nil, err
		}

		return txBz, nil
	}
}

func TxDecoder(cdc codec.Codec, txDecoder sdk.TxDecoder) sdk.TxDecoder {
	return func(txBytes []byte) (sdk.Tx, error) {
		var err error
		if txDecoder != nil {
			// try to decode using the upstream decoder first
			var tx sdk.Tx
			tx, err = txDecoder(txBytes)
			if err == nil {
				return tx, nil
			}
		}

		itx := wInjectTx{
			cdc: cdc,
			Tx:  new(types.InjectTx),
		}
		if err2 := itx.Tx.Unmarshal(txBytes); err2 != nil {
			return nil, errors.Join(err, err2)
		}

		if err3 := sdktx.UnpackInterfaces(cdc.InterfaceRegistry(), itx.Tx.Messages); err3 != nil {
			return nil, errors.Join(err, err3)
		}

		return itx, nil
	}
}

func JSONTxEncoder(cdc codec.Codec, txEncoder sdk.TxEncoder) sdk.TxEncoder {
	return func(tx sdk.Tx) ([]byte, error) {
		itx, ok := tx.(wInjectTx)
		if !ok {
			// not an inject tx, let upstream encoder handle it
			if txEncoder != nil {
				return txEncoder(tx)
			} else {
				return nil, fmt.Errorf("tx is not of type wInjectTx")
			}
		}

		txM, err := cdc.MarshalJSON(itx.Tx)
		if err != nil {
			return nil, err
		}

		return txM, nil
	}
}

func JSONTxDecoder(cdc codec.Codec, jsonTxDecoder sdk.TxDecoder) sdk.TxDecoder {
	return func(txBytes []byte) (sdk.Tx, error) {
		var err error
		if jsonTxDecoder != nil {
			// try to decode using the upstream decoder first
			var tx sdk.Tx
			tx, err = jsonTxDecoder(txBytes)
			if err == nil {
				return tx, err
			}
		}

		var itx types.InjectTx

		if err2 := cdc.UnmarshalJSON(txBytes, &itx); err2 != nil {
			return nil, errors.Join(err, err2)
		}

		if err3 := sdktx.UnpackInterfaces(cdc.InterfaceRegistry(), itx.Messages); err3 != nil {
			return nil, errors.Join(err, err3)
		}

		return wInjectTx{cdc: cdc, Tx: &itx}, nil
	}
}
