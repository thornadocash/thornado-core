package params_test

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/stretchr/testify/require"
	appparams "gitlab.com/thorchain/thornode/v3/app/params"
	tssMessages "gitlab.com/thorchain/thornode/v3/bifrost/p2p/messages"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	thorchaintypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestAminoThorchainMessages(t *testing.T) {
	ec := appparams.MakeEncodingConfig()
	thorchaintypes.RegisterLegacyAminoCodec(ec.Amino)
	txConfig, err := appparams.TxConfig(ec.Codec, nil)
	require.NoError(t, err, "failed to create tx config")

	testCases := []struct {
		name            string
		message         func() sdk.Msg
		expectedSignDoc string
	}{
		{
			name: "MsgSend",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgSend(
					sdk.AccAddress("sender"),
					sdk.AccAddress("recipient"),
					sdk.NewCoins(sdk.NewCoin("rune", math.NewInt(100))),
				)
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgSend","value":{"amount":[{"amount":"100","denom":"rune"}],"from_address":"cosmos1wdjkuer9wgh76ts6","to_address":"cosmos1wfjkx6tsd9jkuaqhtdv59"}}],"sequence":"456"}`,
		},
		{
			name: "MsgDeposit",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgDeposit(common.NewCoins(common.NewCoin(common.ETHAsset, math.NewUint(100000000))), "NODE:thor1a69c00hnmcy5d4taqlv2ljzgaxnfshzrxzscam", sdk.AccAddress("sender"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgDeposit","value":{"coins":[{"amount":"100000000","asset":"ETH.ETH"}],"memo":"NODE:thor1a69c00hnmcy5d4taqlv2ljzgaxnfshzrxzscam","signer":"cosmos1wdjkuer9wgh76ts6"}}],"sequence":"456"}`,
		},
		{
			name:            "MsgBan",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgBan(sdk.AccAddress("ban"), sdk.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgBan","value":{"node_address":"cosmos1vfskutyqdem","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
		{
			name: "MsgErrataTx",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgErrataTx(common.TxID("txid"), common.AVAXChain, sdk.AccAddress("signer"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgErrataTx","value":{"chain":"AVAX","signer":"cosmos1wd5kwmn9wgr5dmap","tx_id":"txid"}}],"sequence":"456"}`,
		},
		{
			name:            "MsgMimir",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgMimir("key", 123, sdk.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgMimir","value":{"key":"key","signer":"cosmos1wd5kwmn9wgr5dmap","value":"123"}}],"sequence":"456"}`,
		},
		{
			name: "MsgNetworkFee",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgNetworkFee(1234567890, common.ETHChain, 1000000000, 1000000, sdk.AccAddress("signer"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgNetworkFee","value":{"block_height":"1234567890","chain":"ETH","signer":"cosmos1wd5kwmn9wgr5dmap","transaction_fee_rate":"1000000","transaction_size":"1000000000"}}],"sequence":"456"}`,
		},
		{
			name:            "MsgNodePauseChain",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgNodePauseChain(1, sdk.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgNodePauseChain","value":{"signer":"cosmos1wd5kwmn9wgr5dmap","value":"1"}}],"sequence":"456"}`,
		},
		{
			name: "MsgObservedTxIn",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgObservedTxIn(common.ObservedTxs{common.NewObservedTx(common.NewTx(common.TxID("txID"), common.Address("from"), common.Address("to"), common.NewCoins(common.NewCoin(common.ATOMAsset, math.NewUint(100000000))), common.Gas(common.NewCoins(common.NewCoin(common.ATOMAsset, math.NewUint(12345)))), "memo"), 54321, common.PubKey("pubkey"), 54323)}, sdk.AccAddress("signer"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/ObservedTxIn","value":{"signer":"cosmos1wd5kwmn9wgr5dmap","txs":[{"block_height":"54321","finalise_height":"54323","observed_pub_key":"pubkey","tx":{"chain":"GAIA","coins":[{"amount":"100000000","asset":"GAIA.ATOM"}],"from_address":"from","gas":[{"amount":"12345","asset":"GAIA.ATOM"}],"id":"txID","memo":"memo","to_address":"to"}}]}}],"sequence":"456"}`,
		},
		{
			name: "MsgObservedTxOut",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgObservedTxOut(common.ObservedTxs{common.NewObservedTx(common.NewTx(common.TxID("txID"), common.Address("from"), common.Address("to"), common.NewCoins(common.NewCoin(common.ATOMAsset, math.NewUint(100000000))), common.Gas(common.NewCoins(common.NewCoin(common.ATOMAsset, math.NewUint(12345)))), "memo"), 54321, common.PubKey("pubkey"), 54323)}, sdk.AccAddress("signer"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/ObservedTxOut","value":{"signer":"cosmos1wd5kwmn9wgr5dmap","txs":[{"block_height":"54321","finalise_height":"54323","observed_pub_key":"pubkey","tx":{"chain":"GAIA","coins":[{"amount":"100000000","asset":"GAIA.ATOM"}],"from_address":"from","gas":[{"amount":"12345","asset":"GAIA.ATOM"}],"id":"txID","memo":"memo","to_address":"to"}}]}}],"sequence":"456"}`,
		},
		{
			name:            "MsgSetIPAddress",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgSetIPAddress("45.64.26.34", cosmos.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgSetIPAddress","value":{"ip_address":"45.64.26.34","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
		{
			name: "MsgSetNodeKeys",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgSetNodeKeys(common.PubKeySet{Ed25519: common.PubKey("ed25519"), Secp256k1: common.PubKey("secp256k1")}, "thorvalconspub123", sdk.AccAddress("signer"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgSetNodeKeys","value":{"pub_key_set_set":{"ed25519":"ed25519","secp256k1":"secp256k1"},"signer":"cosmos1wd5kwmn9wgr5dmap","validator_cons_pub_key":"thorvalconspub123"}}],"sequence":"456"}`,
		},
		{
			name: "MsgSolvency",
			message: func() sdk.Msg {
				msg, err := thorchaintypes.NewMsgSolvency(common.ETHChain, common.PubKey("pubkey"), common.Coins{common.NewCoin(common.ETHAsset, math.NewUint(100000000))}, 1234435345, sdk.AccAddress("signer"))
				require.NoError(t, err)
				return msg
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgSolvency","value":{"chain":"ETH","coins":[{"amount":"100000000","asset":"ETH.ETH"}],"height":"1234435345","id":"84ABD92B5050A0A5C6F991C65E82BBCD8EEB723DDF61310B83BF5BCC8CE3B5C0","pub_key":"pubkey","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
		{
			name: "MsgTssKeysignFail",
			message: func() sdk.Msg {
				msg, err := thorchaintypes.NewMsgTssKeysignFail(1234567890, thorchaintypes.Blame{FailReason: "fail reason", IsUnicast: true, Round: tssMessages.KEYSIGN1aUnicast, BlameNodes: []thorchaintypes.Node{{Pubkey: "blame", BlameData: []byte("blame data"), BlameSignature: []byte("blame signature")}}}, "memo", common.NewCoins(common.NewCoin(common.AVAXAsset, math.NewUint(12323142345))), sdk.AccAddress("signer"), common.PubKey("pubkey"))
				require.NoError(t, err)
				return msg
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/TssKeysignFail","value":{"blame":{"blame_nodes":[{"blame_data":"YmxhbWUgZGF0YQ==","blame_signature":"YmxhbWUgc2lnbmF0dXJl","pubkey":"blame"}],"fail_reason":"fail reason","is_unicast":true,"round":"SignRound1Message1"},"coins":[{"amount":"12323142345","asset":"AVAX.AVAX"}],"height":"1234567890","id":"a564405be4a54733cf450e63c0ba1f7ab0f976f0934e1d7087ad30ff41a174f2","memo":"memo","pub_key":"pubkey","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
		{
			name: "MsgTssPool",
			message: func() sdk.Msg {
				msg, err := thorchaintypes.NewMsgTssPool([]string{"pk1", "pk2"}, common.PubKey("pool"), []byte("secp256k1sig"), []byte("ksbackup"), thorchaintypes.KeygenType_AsgardKeygen, 1234567890, []thorchaintypes.Blame{{FailReason: "fail reason", IsUnicast: true, BlameNodes: []thorchaintypes.Node{{Pubkey: "blame", BlameData: []byte("blame data"), BlameSignature: []byte("blame signature")}}}}, []string{string(common.BCHChain), string(common.AVAXChain)}, cosmos.AccAddress("signer"), 123)
				require.NoError(t, err)
				return msg
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/TssPool","value":{"blame":[{"blame_nodes":[{"blame_data":"YmxhbWUgZGF0YQ==","blame_signature":"YmxhbWUgc2lnbmF0dXJl","pubkey":"blame"}],"fail_reason":"fail reason","is_unicast":true}],"chains":["BCH","AVAX"],"height":"1234567890","id":"95fb31a6efb06bda7cd3c2b12637a1cc631cf9186a868edafa505551071901eb","keygen_time":"123","keygen_type":"AsgardKeygen","keyshares_backup":"a3NiYWNrdXA=","pool_pub_key":"pool","pub_keys":["pk1","pk2"],"secp256k1_signature":"c2VjcDI1Nmsxc2ln","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
		{
			name:            "MsgSetVersion",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgSetVersion("v0.1.0", sdk.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgSetVersion","value":{"signer":"cosmos1wd5kwmn9wgr5dmap","version":"v0.1.0"}}],"sequence":"456"}`,
		},
		{
			name: "MsgProposeUpgrade",
			message: func() sdk.Msg {
				return thorchaintypes.NewMsgProposeUpgrade("v0.1.0", 1234567890, "info", sdk.AccAddress("signer"))
			},
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgProposeUpgrade","value":{"name":"v0.1.0","signer":"cosmos1wd5kwmn9wgr5dmap","upgrade":{"height":"1234567890","info":"info"}}}],"sequence":"456"}`,
		},
		{
			name:            "MsgApproveUpgrade",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgApproveUpgrade("v0.1.0", sdk.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgApproveUpgrade","value":{"name":"v0.1.0","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
		{
			name:            "MsgRejectUpgrade",
			message:         func() sdk.Msg { return thorchaintypes.NewMsgRejectUpgrade("v0.1.0", sdk.AccAddress("signer")) },
			expectedSignDoc: `{"account_number":"123","chain_id":"thorchain-1","fee":{"amount":[{"amount":"100","denom":"rune"}],"gas":"200000"},"memo":"memo","msgs":[{"type":"thorchain/MsgRejectUpgrade","value":{"name":"v0.1.0","signer":"cosmos1wd5kwmn9wgr5dmap"}}],"sequence":"456"}`,
		},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := txConfig.NewTxBuilder()
			require.NoError(t, tx.SetMsgs(tc.message()), "failed to set message")
			tx.SetFeeAmount(sdk.NewCoins(sdk.NewCoin("rune", math.NewInt(100))))
			tx.SetGasLimit(200000)
			tx.SetMemo("memo")

			signDocBz, err := authsigning.GetSignBytesAdapter(ctx, txConfig.SignModeHandler(), signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, authsigning.SignerData{
				Address:       sdk.AccAddress("sender").String(),
				ChainID:       "thorchain-1",
				AccountNumber: 123,
				Sequence:      456,
			}, tx.GetTx())
			require.NoError(t, err, "failed to get signDoc")
			require.Equal(t, tc.expectedSignDoc, string(signDocBz), "invalid signDoc")
		})
	}
}
