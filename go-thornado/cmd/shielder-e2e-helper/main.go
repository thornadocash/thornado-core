package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/btcsuite/btcd/btcec"
	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	prefix "github.com/thornadocash/go-thornado/cmd"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/go-wrappers/shielder"
)

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func hasLeadingZeroBits(bytes []byte, bits int) bool {
	fullZeroBytes := bits / 8
	remainingBits := bits % 8
	for i := 0; i < fullZeroBytes; i++ {
		if bytes[i] != 0 {
			return false
		}
	}
	if remainingBits == 0 {
		return true
	}
	mask := byte(0xff << (8 - remainingBits))
	return bytes[fullZeroBytes]&mask == 0
}

func main() {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(prefix.Bech32PrefixAccAddr, prefix.Bech32PrefixAccPub)
	cfg.SetBech32PrefixForValidator(prefix.Bech32PrefixValAddr, prefix.Bech32PrefixValPub)
	cfg.SetBech32PrefixForConsensusNode(prefix.Bech32PrefixConsAddr, prefix.Bech32PrefixConsPub)
	cfg.Seal()

	if len(os.Args) < 2 {
		die("usage: shielder-e2e-helper {pubkey|receipt|commitments|merkle-root|withdrawal|withdrawal-policy|shield-withdrawal}")
	}
	switch os.Args[1] {
	case "receipt-simple":
		if len(os.Args) != 4 {
			die("usage: shielder-e2e-helper receipt-simple [amount-sats] [client-seed]")
		}
		var amount uint64
		if _, err := fmt.Sscanf(os.Args[2], "%d", &amount); err != nil {
			die("invalid amount-sats: %v", err)
		}
		out, err := shielder.DeriveShieldReceipt(os.Args[3], amount, os.Args[3])
		if err != nil {
			die("%v", err)
		}
		fmt.Println(out)
	case "sign-hex":
		if len(os.Args) != 4 {
			die("usage: shielder-e2e-helper sign-hex [digest-hex] [privkey-hex]")
		}
		digest, err := hex.DecodeString(strings.TrimSpace(os.Args[2]))
		if err != nil {
			die("invalid digest hex: %v", err)
		}
		privBytes, err := hex.DecodeString(strings.TrimSpace(os.Args[3]))
		if err != nil {
			die("invalid privkey hex: %v", err)
		}
		priv, _ := btcec.PrivKeyFromBytes(btcec.S256(), privBytes)
		sig, err := priv.Sign(digest)
		if err != nil {
			die("%v", err)
		}
		s := sig.S
		halfOrder := new(big.Int).Rsh(btcec.S256().N, 1)
		if s.Cmp(halfOrder) == 1 {
			s = new(big.Int).Sub(btcec.S256().N, s)
		}
		out := make([]byte, 64)
		sig.R.FillBytes(out[:32])
		s.FillBytes(out[32:])
		fmt.Println(hex.EncodeToString(out))
	case "btc-address":
		if len(os.Args) != 4 {
			die("usage: shielder-e2e-helper btc-address [vault-pubkey] [path-index]")
		}
		var pathIndex uint64
		if _, err := fmt.Sscanf(os.Args[3], "%d", &pathIndex); err != nil {
			die("invalid path-index: %v", err)
		}
		pk, err := common.NewPubKey(os.Args[2])
		if err != nil {
			die("%v", err)
		}
		addr, err := common.DeriveBTCTaprootAddress(pk, pathIndex)
		if err != nil {
			die("%v", err)
		}
		fmt.Println(addr.String())
	case "pubkey":
		if len(os.Args) != 3 {
			die("usage: shielder-e2e-helper pubkey [client-seed]")
		}
		out, err := shielder.ClientPubKeyFromSecret(os.Args[2])
		if err != nil {
			die("%v", err)
		}
		fmt.Println(out)
	case "owner-address":
		if len(os.Args) != 3 {
			die("usage: shielder-e2e-helper owner-address [compressed-pubkey]")
		}
		raw, err := hex.DecodeString(strings.TrimSpace(os.Args[2]))
		if err != nil {
			die("invalid compressed-pubkey: %v", err)
		}
		if len(raw) == 32 {
			raw = append([]byte{0x02}, raw...)
		}
		if len(raw) != 33 {
			die("invalid compressed-pubkey length")
		}
		if raw[0] != 0x02 && raw[0] != 0x03 {
			die("invalid compressed-pubkey prefix")
		}
		pubkey := &sdksecp256k1.PubKey{Key: raw}
		fmt.Println(sdk.AccAddress(pubkey.Address()).String())
	case "pow-token":
		if len(os.Args) != 5 {
			die("usage: shielder-e2e-helper pow-token [owner] [difficulty-bits] [label]")
		}
		var bits int
		if _, err := fmt.Sscanf(os.Args[3], "%d", &bits); err != nil {
			die("invalid difficulty-bits: %v", err)
		}
		for nonce := uint64(0); ; nonce++ {
			token := fmt.Sprintf("%s:%d", os.Args[4], nonce)
			sum := sha256.Sum256([]byte(os.Args[2] + ":" + token))
			if hasLeadingZeroBits(sum[:], bits) {
				fmt.Println(token)
				return
			}
		}
	case "receipt":
		if len(os.Args) != 6 {
			die("usage: shielder-e2e-helper receipt [deposit-id] [path-index] [amount-sats] [client-seed]")
		}
		var pathIndex, amount uint64
		if _, err := fmt.Sscanf(os.Args[3], "%d", &pathIndex); err != nil {
			die("invalid path-index: %v", err)
		}
		if _, err := fmt.Sscanf(os.Args[4], "%d", &amount); err != nil {
			die("invalid amount-sats: %v", err)
		}
		out, err := shielder.DeriveShieldReceiptForDeposit(os.Args[2], pathIndex, amount, os.Args[5])
		if err != nil {
			die("%v", err)
		}
		fmt.Println(out)
	case "commitments":
		if len(os.Args) != 3 {
			die("usage: shielder-e2e-helper commitments [receipt-json]")
		}
		var receipt struct {
			Notes []json.RawMessage `json:"notes"`
		}
		if err := json.Unmarshal([]byte(os.Args[2]), &receipt); err != nil {
			die("invalid receipt json: %v", err)
		}
		out := make([]string, 0, len(receipt.Notes))
		for _, note := range receipt.Notes {
			out = append(out, string(note))
		}
		bz, err := json.Marshal(out)
		if err != nil {
			die("%v", err)
		}
		fmt.Println(string(bz))
	case "protocol-commitments":
		if len(os.Args) != 3 {
			die("usage: shielder-e2e-helper protocol-commitments [amount-sats]")
		}
		var amount uint64
		if _, err := fmt.Sscanf(os.Args[2], "%d", &amount); err != nil {
			die("invalid amount-sats: %v", err)
		}
		note := struct {
			DenominationSats uint64 `json:"denomination_sats"`
		}{DenominationSats: amount}
		item, err := json.Marshal(note)
		if err != nil {
			die("%v", err)
		}
		bz, err := json.Marshal([]string{string(item)})
		if err != nil {
			die("%v", err)
		}
		fmt.Println(string(bz))
	case "commitment-objects":
		if len(os.Args) != 3 {
			die("usage: shielder-e2e-helper commitment-objects [receipt-json]")
		}
		var receipt struct {
			Notes []struct {
				DenominationSats uint64 `json:"denomination_sats"`
				OwnerPubkey      string `json:"owner_pubkey"`
				Signature        string `json:"signature"`
				Commitment       string `json:"commitment"`
			} `json:"notes"`
		}
		if err := json.Unmarshal([]byte(os.Args[2]), &receipt); err != nil {
			die("invalid receipt json: %v", err)
		}
		out, err := json.Marshal(receipt.Notes)
		if err != nil {
			die("%v", err)
		}
		fmt.Println(string(out))
	case "shield-authorization":
		if len(os.Args) != 6 {
			die("usage: shielder-e2e-helper shield-authorization [client-seed] [deposit-id] [amount-sats] [commitments-json]")
		}
		var amount uint64
		if _, err := fmt.Sscanf(os.Args[4], "%d", &amount); err != nil {
			die("invalid amount-sats: %v", err)
		}
		out, err := shielder.ShieldAuthorization(os.Args[2], os.Args[3], amount, os.Args[5])
		if err != nil {
			die("%v", err)
		}
		fmt.Println(out)
	case "merkle-root":
		if len(os.Args) != 3 {
			die("usage: shielder-e2e-helper merkle-root [leaves-json]")
		}
		rootHex, err := shielder.MerkleRoot(os.Args[2])
		if err != nil {
			die("%v", err)
		}
		root := strings.TrimPrefix(strings.TrimSpace(rootHex), "0x")
		raw, err := hex.DecodeString(root)
		if err != nil {
			die("invalid merkle root: %v", err)
		}
		for left, right := 0, len(raw)-1; left < right; left, right = left+1, right-1 {
			raw[left], raw[right] = raw[right], raw[left]
		}
		fmt.Println(new(big.Int).SetBytes(raw).String())
	case "withdrawal":
		if len(os.Args) != 7 {
			die("usage: shielder-e2e-helper withdrawal [note-json] [client-seed] [leaves-json] [recipient] [fee-sats]")
		}
		var fee uint64
		if _, err := fmt.Sscanf(os.Args[6], "%d", &fee); err != nil {
			die("invalid fee-sats: %v", err)
		}
		out, err := shielder.ShielderWithdrawalFromReceipt(os.Args[2], os.Args[3], os.Args[4], os.Args[5], fee)
		if err != nil {
			die("%v", err)
		}
		fmt.Println(out)
	case "withdrawal-policy":
		if len(os.Args) != 10 {
			die("usage: shielder-e2e-helper withdrawal-policy [note-json] [client-seed] [leaves-json] [recipient] [fee-sats] [recipient-policy] [node-pubkey] [bid-id]")
		}
		var fee uint64
		if _, err := fmt.Sscanf(os.Args[6], "%d", &fee); err != nil {
			die("invalid fee-sats: %v", err)
		}
		out, err := shielder.ShielderWithdrawalFromReceipt(os.Args[2], os.Args[3], os.Args[4], os.Args[5], fee)
		if err != nil {
			die("%v", err)
		}
		var pair []json.RawMessage
		if err := json.Unmarshal([]byte(out), &pair); err != nil {
			die("invalid withdrawal json: %v", err)
		}
		if len(pair) != 2 {
			die("withdrawal json must be [proof, public]")
		}
		var public map[string]any
		if err := json.Unmarshal(pair[1], &public); err != nil {
			die("invalid public json: %v", err)
		}
		if policy := strings.TrimSpace(os.Args[7]); policy != "" {
			public["recipient_policy"] = policy
		}
		if nodePubKey := strings.TrimSpace(os.Args[8]); nodePubKey != "" {
			public["node_pub_key"] = nodePubKey
		}
		if bidID := strings.TrimSpace(os.Args[9]); bidID != "" {
			public["bid_id"] = bidID
		}
		patched, err := json.Marshal(public)
		if err != nil {
			die("%v", err)
		}
		next, err := json.Marshal([]json.RawMessage{pair[0], patched})
		if err != nil {
			die("%v", err)
		}
		fmt.Println(string(next))
	case "shield-withdrawal":
		if len(os.Args) != 4 {
			die("usage: shielder-e2e-helper shield-withdrawal [withdrawal-json] [out-prefix]")
		}
		var pair []json.RawMessage
		if err := json.Unmarshal([]byte(os.Args[2]), &pair); err != nil {
			die("invalid withdrawal json: %v", err)
		}
		if len(pair) != 2 {
			die("withdrawal json must be [proof, public]")
		}
		if err := os.WriteFile(os.Args[3]+".proof.json", pair[0], 0o600); err != nil {
			die("%v", err)
		}
		if err := os.WriteFile(os.Args[3]+".public.json", pair[1], 0o600); err != nil {
			die("%v", err)
		}
	case "fee-payload":
		if len(os.Args) != 8 {
			die("usage: shielder-e2e-helper fee-payload [node-pubkey] [owner] [accrued] [fee-share] [commitments-json] [note-pubkeys-json]")
		}
		var accrued, feeShare uint64
		if _, err := fmt.Sscanf(os.Args[4], "%d", &accrued); err != nil {
			die("invalid accrued: %v", err)
		}
		if _, err := fmt.Sscanf(os.Args[5], "%d", &feeShare); err != nil {
			die("invalid fee-share: %v", err)
		}
		var notes []struct {
			DenominationSats uint64 `json:"denomination_sats"`
			Commitment       string `json:"commitment"`
		}
		var pubkeys []string
		if err := json.Unmarshal([]byte(os.Args[6]), &notes); err != nil {
			die("invalid commitments json: %v", err)
		}
		if err := json.Unmarshal([]byte(os.Args[7]), &pubkeys); err != nil {
			die("invalid note pubkeys json: %v", err)
		}
		parts := []string{"thornado:fee-claim:v1", os.Args[2], os.Args[3], fmt.Sprintf("%d", accrued), fmt.Sprintf("%d", feeShare)}
		for i, note := range notes {
			parts = append(parts, fmt.Sprintf("%d:%s:%s", note.DenominationSats, note.Commitment, pubkeys[i]))
		}
		sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
		fmt.Println(hex.EncodeToString(sum[:]))
	default:
		die("unknown command: %s", os.Args[1])
	}
}
