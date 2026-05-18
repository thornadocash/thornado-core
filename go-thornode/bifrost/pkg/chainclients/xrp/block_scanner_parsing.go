package xrp

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	binarycodec "github.com/Peersyst/xrpl-go/binary-codec"
	"github.com/Peersyst/xrpl-go/xrpl/transaction"
	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"
)

// Partial payment flag
const tfPartialPayment uint32 = 131072

func (c *XrpBlockScanner) processPayment(flatTx map[string]any) (*transaction.Payment, error) {
	// Ignore any txs other than payments
	if flatTx["TransactionType"] != "Payment" {
		return nil, nil
	}

	fee, err := parseAmountFromTx(flatTx["Fee"])
	if err != nil {
		return nil, fmt.Errorf("cannot parse fee: %w", err)
	}

	feeXRP, ok := fee.(txtypes.XRPCurrencyAmount)
	if !ok {
		return nil, fmt.Errorf("fee is not in XRP")
	}

	var memoData []byte
	if flatTx["Memos"] != nil {
		// trunk-ignore(golangci-lint/govet): shadow
		memos, ok := flatTx["Memos"].([]any)
		if !ok {
			return nil, fmt.Errorf("cannot cast memos to []any")
		}
		// optional: add filter for vault addresses
		// if more than 1 memo exist, we only use the first
		if len(memos) < 1 {
			return nil, fmt.Errorf("memos is empty")
		}
		memoObj, ok := memos[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot cast memos[0] to map[string]any")
		}
		memo, ok := memoObj["Memo"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot cast memo to map[string]any")
		}
		memoDataHex, ok := memo["MemoData"].(string)
		if !ok {
			return nil, fmt.Errorf("cannot cast MemoData to string")
		}
		if memoDataHex == "" {
			return nil, fmt.Errorf("MemoData is empty")
		}
		memoData, err = hex.DecodeString(memoDataHex)
		if err != nil {
			return nil, fmt.Errorf("cannot decode memo data")
		}
	}

	sender, ok := flatTx["Account"].(string)
	if !ok {
		return nil, fmt.Errorf("cannot cast sender/account to string")
	}

	to, ok := flatTx["Destination"].(string)
	if !ok {
		return nil, fmt.Errorf("cannot cast to/destination to string")
	}

	return &transaction.Payment{
		BaseTx: transaction.BaseTx{
			Account: txtypes.Address(sender),
			Fee:     feeXRP,
			Memos: []txtypes.MemoWrapper{
				{
					Memo: txtypes.Memo{
						MemoData: string(memoData),
					},
				},
			},
		},
		Destination: txtypes.Address(to),
	}, nil
}

// Currently, we are requesting txs in json, so this should return immediately
func (c *XrpBlockScanner) decodeMetaBlobIfNecessary(rawTx transaction.FlatTransaction) (map[string]any, error) {
	meta, ok := rawTx["meta"].(map[string]any)
	if ok {
		return meta, nil
	}

	metaBlob, ok := rawTx["meta_blob"].(string)
	if !ok {
		return nil, fmt.Errorf("fail to parse any meta info from tx, meta_blob not a string")
	}

	meta, err := binarycodec.Decode(metaBlob)
	if err != nil {
		return nil, fmt.Errorf("fail to decode meta blob, %w", err)
	}

	return meta, nil
}

// Currently, we are requesting txs in json, so this should return immediately
func (c *XrpBlockScanner) decodeTxBlobIfNecessary(rawTx transaction.FlatTransaction) (map[string]any, error) {
	flatTx, ok := rawTx["tx_json"].(map[string]any)
	if ok {
		return flatTx, nil
	}

	txBlob, ok := rawTx["tx_blob"].(string)
	if !ok {
		return nil, fmt.Errorf("fail to parse tx blob from tx, tx_blob not a string")
	}

	flatTx, err := binarycodec.Decode(txBlob)
	if err != nil {
		return nil, fmt.Errorf("fail to decode tx blob, %w", err)
	}

	return flatTx, nil
}

func (c *XrpBlockScanner) getDeliveredAmount(tx, meta map[string]any) (txtypes.CurrencyAmount, error) {
	// The delivered_amount field is generated on-demand for the request,
	// and is not included in the binary format for transaction metadata,
	// nor is it used when calculating the hash of the transaction metadata.
	amount, err := parseAmountFromTx(meta["delivered_amount"])
	if err == nil {
		return amount, nil
	}

	// the DeliveredAmount field is included in the binary format for partial payment transactions after 2014-01-20.
	amount, err = parseAmountFromTx(meta["DeliveredAmount"])
	if err == nil {
		return amount, nil
	}

	// check that partial payment flag is not set
	flags := getFlags(tx)
	if (flags & tfPartialPayment) == tfPartialPayment {
		return nil, fmt.Errorf("partial payment flag set and delivered amount field not set")
	}

	// get Amount
	amount, err = parseAmountFromTx(tx["Amount"])
	if err != nil {
		amount, err = parseAmountFromTx(tx["DeliverMax"])
		if err != nil {
			return nil, fmt.Errorf("cannot parse amount or deliver max fields: %w", err)
		}
	}

	return amount, nil
}

func getFlags(tx map[string]any) uint32 {
	flags, ok := tx["Flags"].(uint32)
	if !ok {
		return 0
	}
	return flags
}

func parseAmountFromTx(amountAny any) (txtypes.CurrencyAmount, error) {
	amountJson, err := json.Marshal(amountAny)
	if err != nil {
		return nil, fmt.Errorf("cannot json marshal amount: %w", err)
	}
	amount, err := txtypes.UnmarshalCurrencyAmount(amountJson)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal currency amount: %w", err)
	}

	return amount, nil
}
