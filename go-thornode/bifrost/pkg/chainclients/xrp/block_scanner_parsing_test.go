package xrp

import (
	"testing"

	"github.com/Peersyst/xrpl-go/xrpl/transaction"
	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessPayment(t *testing.T) {
	scanner := &XrpBlockScanner{}

	tests := []struct {
		name       string
		tx         map[string]any
		want       *transaction.Payment
		wantErr    bool
		wantErrStr string
	}{
		{
			name: "successful payment with memo",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender123",
				"Destination":     "rReceiver456",
				"Memos": []any{
					map[string]any{
						"Memo": map[string]any{
							"MemoData": "48656C6C6F", // "Hello" in hex
						},
					},
				},
			},
			want: &transaction.Payment{
				BaseTx: transaction.BaseTx{
					Account: txtypes.Address("rSender123"),
					Fee:     txtypes.XRPCurrencyAmount(10),
					Memos: []txtypes.MemoWrapper{
						{
							Memo: txtypes.Memo{
								MemoData: "Hello",
							},
						},
					},
				},
				Destination: txtypes.Address("rReceiver456"),
			},
			wantErr: false,
		},
		{
			name: "non-payment transaction",
			tx: map[string]any{
				"TransactionType": "AccountSet",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "invalid fee",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "invalid",
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot parse fee",
		},
		{
			name: "fee is not xrp",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee": map[string]any{
					"Issuer":   "rIssuer",
					"Currency": "StableCoin",
					"Value":    "1000000",
				},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "fee is not in XRP",
		},
		{
			name: "invalid Memos type",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Memos":           []string{},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast memos to []any",
		},
		{
			name: "invalid memo object",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Memos": []any{
					"Memo1",
				},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast memos[0] to map[string]any",
		},
		{
			name: "invalid memo map structure",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Memos": []any{
					map[string]any{
						"Memo": "Memo",
					},
				},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast memo to map[string]any",
		},
		{
			name: "invalid MemoData type",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Memos": []any{
					map[string]any{
						"Memo": map[string]any{
							"MemoData": 1234,
						},
					},
				},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast MemoData to string",
		},
		{
			name: "MemoData is empty",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Memos": []any{
					map[string]any{
						"Memo": map[string]any{
							"MemoData": "",
						},
					},
				},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "MemoData is empty",
		},
		{
			name: "MemoData is not hex",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Memos": []any{
					map[string]any{
						"Memo": map[string]any{
							"MemoData": "12345Z",
						},
					},
				},
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot decode memo data",
		},
		{
			name: "missing required fields (sender)",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast sender/account to string",
		},
		{
			name: "invalid sender field",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Sender":          1234,
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast sender/account to string",
		},
		{
			name: "missing required fields (destination)",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender",
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast to/destination to string",
		},
		{
			name: "invalid destination field",
			tx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender",
				"Destination":     1234,
			},
			want:       nil,
			wantErr:    true,
			wantErrStr: "cannot cast to/destination to string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanner.processPayment(tt.tx)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeMetaBlobIfNecessary(t *testing.T) {
	scanner := &XrpBlockScanner{}

	tests := []struct {
		name    string
		rawTx   transaction.FlatTransaction
		want    map[string]any
		wantErr bool
	}{
		{
			name: "already decoded meta",
			rawTx: transaction.FlatTransaction{
				"meta": map[string]any{
					"field": "value",
				},
			},
			want: map[string]any{
				"field": "value",
			},
			wantErr: false,
		},
		{
			name: "meta blob needs decoding",
			rawTx: transaction.FlatTransaction{
				"meta_blob": "201C00000063F8E5110061250000011155F028573AE0CD59767665FEE6283F34AFD95AEF9FACF537B16BA89458356219E2567C6A1ADA196308FB97B8975DC0A4F8517CF9963361FADF1E22AC839D8CCF34CEE66240000000057BCED8E1E7220000000024000000052D000000006240000000059A5358811478BAB4EAB9BEA87099E7382683A82C9409BD1E5AE1E1E5110061250000011155FBFDCCE43AB01A1657939FB97D4B6D12E040347541BB45FBDF92D426D50CC76656BD775007FE9D9A22B97570BA7D57FD0A052ECBE6FF7ABE810EC1F09886E16C3CE624000000046240000000059A5362E1E7220000000024000000052D000000006240000000057BCED88114776FDCAB500131BB6C6D91B20B8D779DA5CA3F21E1E1F1031000",
			},
			want: map[string]any{
				"AffectedNodes": []any{
					map[string]any{
						"ModifiedNode": map[string]any{
							"FinalFields": map[string]any{
								"Account":    "rUrMkdV3bNo3PyeMrq279ZHRyjYL3zNLDB",
								"Balance":    "93999960",
								"Flags":      uint32(0x0),
								"OwnerCount": uint32(0x0),
								"Sequence":   uint32(0x5),
							},
							"LedgerEntryType": "AccountRoot",
							"LedgerIndex":     "7C6A1ADA196308FB97B8975DC0A4F8517CF9963361FADF1E22AC839D8CCF34CE",
							"PreviousFields": map[string]any{
								"Balance": "91999960",
							},
							"PreviousTxnID":     "F028573AE0CD59767665FEE6283F34AFD95AEF9FACF537B16BA89458356219E2",
							"PreviousTxnLgrSeq": uint32(0x111),
						},
					},
					map[string]any{
						"ModifiedNode": map[string]any{
							"FinalFields": map[string]any{
								"Account":    "rBtXR3qPjhLoLnEHKouDJqpXcLjSQMaTSJ",
								"Balance":    "91999960",
								"Flags":      uint32(0x0),
								"OwnerCount": uint32(0x0),
								"Sequence":   uint32(0x5),
							},
							"LedgerEntryType": "AccountRoot",
							"LedgerIndex":     "BD775007FE9D9A22B97570BA7D57FD0A052ECBE6FF7ABE810EC1F09886E16C3C",
							"PreviousFields": map[string]any{
								"Balance":  "93999970",
								"Sequence": uint32(0x4),
							},
							"PreviousTxnID":     "FBFDCCE43AB01A1657939FB97D4B6D12E040347541BB45FBDF92D426D50CC766",
							"PreviousTxnLgrSeq": uint32(0x111),
						},
					},
				},
				"TransactionIndex":  uint32(0x63),
				"TransactionResult": "tesSUCCESS",
			},
			wantErr: false,
		},
		{
			name: "invalid meta blob",
			rawTx: transaction.FlatTransaction{
				"meta_blob": "invalid",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanner.decodeMetaBlobIfNecessary(tt.rawTx)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeTxBlobIfNecessary(t *testing.T) {
	scanner := &XrpBlockScanner{}

	tests := []struct {
		name    string
		rawTx   transaction.FlatTransaction
		want    map[string]any
		wantErr bool
	}{
		{
			name: "already decoded tx_json",
			rawTx: transaction.FlatTransaction{
				"tx_json": map[string]any{
					"field": "value",
				},
			},
			want: map[string]any{
				"field": "value",
			},
			wantErr: false,
		},
		{
			name: "tx blob needs decoding",
			rawTx: transaction.FlatTransaction{
				"tx_blob": "12000021000004D22400000004201B000001246140000000001E848068400000000000000A7321ED258BBCB9D302EF46F3CB4C11D384FC6F147A929535A35F9CDF0E4F6829DC28317440FD2B05C8A63D87946B01A94B3B7C9E754B1BE43FA2BA093248AAD07590CBA2DD857C0B7DE269FB4018FF59BC8046EBD91880E15FB9EC4B79408245D4AA639A0E8114776FDCAB500131BB6C6D91B20B8D779DA5CA3F21831478BAB4EAB9BEA87099E7382683A82C9409BD1E5A",
			},
			want: map[string]any{
				"Account":            "rBtXR3qPjhLoLnEHKouDJqpXcLjSQMaTSJ",
				"Amount":             "2000000",
				"Destination":        "rUrMkdV3bNo3PyeMrq279ZHRyjYL3zNLDB",
				"Fee":                "10",
				"LastLedgerSequence": uint32(0x124),
				"NetworkID":          uint32(0x4d2),
				"Sequence":           uint32(0x4),
				"SigningPubKey":      "ED258BBCB9D302EF46F3CB4C11D384FC6F147A929535A35F9CDF0E4F6829DC2831",
				"TransactionType":    "Payment",
				"TxnSignature":       "FD2B05C8A63D87946B01A94B3B7C9E754B1BE43FA2BA093248AAD07590CBA2DD857C0B7DE269FB4018FF59BC8046EBD91880E15FB9EC4B79408245D4AA639A0E",
			},
			wantErr: false,
		},
		{
			name: "invalid tx blob",
			rawTx: transaction.FlatTransaction{
				"tx_blob": "invalid",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanner.decodeTxBlobIfNecessary(tt.rawTx)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetDeliveredAmount(t *testing.T) {
	scanner := &XrpBlockScanner{}

	tests := []struct {
		name    string
		tx      map[string]any
		meta    map[string]any
		want    txtypes.CurrencyAmount
		wantErr bool
	}{
		{
			name: "delivered_amount present",
			tx:   map[string]any{},
			meta: map[string]any{
				"delivered_amount": "1000000",
			},
			want:    txtypes.XRPCurrencyAmount(1000000),
			wantErr: false,
		},
		{
			name: "DeliveredAmount present",
			tx:   map[string]any{},
			meta: map[string]any{
				"DeliveredAmount": "1000000",
			},
			want:    txtypes.XRPCurrencyAmount(1000000),
			wantErr: false,
		},
		{
			name: "use Amount field when no partial payment",
			tx: map[string]any{
				"Flags":  uint32(0),
				"Amount": "1000000",
			},
			meta:    map[string]any{},
			want:    txtypes.XRPCurrencyAmount(1000000),
			wantErr: false,
		},
		{
			name: "use DeliverMax field when no partial payment and no Amount",
			tx: map[string]any{
				"DeliverMax": "1000000",
			},
			meta:    map[string]any{},
			want:    txtypes.XRPCurrencyAmount(1000000),
			wantErr: false,
		},
		{
			name: "no valid fields",
			tx: map[string]any{
				"TotalAmount": "1000000",
			},
			meta:    map[string]any{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "partial payment flag set but no delivered amount",
			tx: map[string]any{
				"Flags":  tfPartialPayment,
				"Amount": "1000000",
			},
			meta:    map[string]any{},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanner.getDeliveredAmount(tt.tx, tt.meta)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetFlags(t *testing.T) {
	tests := []struct {
		name string
		tx   map[string]any
		want uint32
	}{
		{
			name: "flags present",
			tx: map[string]any{
				"Flags": uint32(131072),
			},
			want: 131072,
		},
		{
			name: "flags absent",
			tx:   map[string]any{},
			want: 0,
		},
		{
			name: "flags wrong type",
			tx: map[string]any{
				"Flags": "invalid",
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getFlags(tt.tx)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAmountFromTx(t *testing.T) {
	tests := []struct {
		name    string
		amount  any
		want    txtypes.CurrencyAmount
		wantErr bool
	}{
		{
			name:    "valid XRP amount",
			amount:  "1000000",
			want:    txtypes.XRPCurrencyAmount(1000000),
			wantErr: false,
		},
		{
			name: "valid issued currency amount",
			amount: map[string]any{
				"value":    "100",
				"currency": "USD",
				"issuer":   "rIssuer123",
			},
			want: txtypes.IssuedCurrencyAmount{
				Value:    "100",
				Currency: "USD",
				Issuer:   txtypes.Address("rIssuer123"),
			},
			wantErr: false,
		},
		{
			name:    "invalid amount",
			amount:  make(chan int),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAmountFromTx(tt.amount)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
