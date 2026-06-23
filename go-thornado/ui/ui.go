package ui

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

//go:embed static
var staticFS embed.FS

const maxRequestBodyBytes = types.MaxShielderProofJSONLength + types.MaxShielderPublicJSONLength + 8*1024

func Static() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

func HandleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	http.ServeFileFS(w, r, staticFS, "static/index.html")
}

func RegisterBrowserAPI(rtr *mux.Router, moduleName string, clientCtx client.Context) {
	rtr.HandleFunc(fmt.Sprintf("/%s/deposit", moduleName), browserHandler(clientCtx, depositMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/shield", moduleName), browserHandler(clientCtx, shieldMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/withdraw", moduleName), browserHandler(clientCtx, withdrawMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/set-ip", moduleName), browserHandler(clientCtx, nodeSetIPMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/set-version", moduleName), browserHandler(clientCtx, nodeSetVersionMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/set-keys", moduleName), browserHandler(clientCtx, nodeSetKeysMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/bond-from-notes", moduleName), browserHandler(clientCtx, nodeBondFromNotesMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/shield-fees", moduleName), browserHandler(clientCtx, nodeShieldFeesMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/auction-create", moduleName), browserHandler(clientCtx, nodeAuctionCreateMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/auction-bid-create", moduleName), browserHandler(clientCtx, nodeAuctionBidCreateMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/auction-select-bid", moduleName), browserHandler(clientCtx, nodeAuctionSelectBidMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/sale-shield", moduleName), browserHandler(clientCtx, nodeSaleShieldMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/pause-chain", moduleName), browserHandler(clientCtx, nodePauseChainMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/node/resume-chain", moduleName), browserHandler(clientCtx, nodeResumeChainMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/config/vote", moduleName), browserHandler(clientCtx, configVoteMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/upgrade/propose", moduleName), browserHandler(clientCtx, upgradeProposeMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/upgrade/approve", moduleName), browserHandler(clientCtx, upgradeApproveMsg)).Methods(http.MethodPost, http.MethodOptions)
	rtr.HandleFunc(fmt.Sprintf("/%s/browser/upgrade/reject", moduleName), browserHandler(clientCtx, upgradeRejectMsg)).Methods(http.MethodPost, http.MethodOptions)
}

type browserResponse struct {
	Owner      string          `json:"owner,omitempty"`
	TxHash     string          `json:"txhash,omitempty"`
	Code       uint32          `json:"code"`
	RawLog     string          `json:"raw_log,omitempty"`
	TxResponse json.RawMessage `json:"tx_response,omitempty"`
}

func browserHandler(clientCtx client.Context, build func(*http.Request) (sdk.Msg, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		msg, owner, err := build(r)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if validator, ok := msg.(interface{ ValidateBasic() error }); ok {
			if err := validator.ValidateBasic(); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
		}
		txCtx := clientCtx
		if strings.TrimSpace(txCtx.BroadcastMode) == "" {
			txCtx = txCtx.WithBroadcastMode(flags.BroadcastSync)
		}
		txBuilder := txCtx.TxConfig.NewTxBuilder()
		if err := txBuilder.SetMsgs(msg); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		txBuilder.SetGasLimit(0)
		txBytes, err := txCtx.TxConfig.TxEncoder()(txBuilder.GetTx())
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		txResponse, err := txCtx.BroadcastTx(txBytes)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
			return
		}
		raw, _ := json.Marshal(txResponse)
		status := http.StatusAccepted
		if txResponse.Code != 0 {
			status = http.StatusBadRequest
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(browserResponse{
			Owner:      owner,
			TxHash:     txResponse.TxHash,
			Code:       txResponse.Code,
			RawLog:     txResponse.RawLog,
			TxResponse: raw,
		})
	}
}

func depositMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		PowToken      string `json:"pow_token"`
		DepositPubkey string `json:"deposit_pubkey"`
		PowDurationMs uint64 `json:"pow_duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	owner, err := ownerFromCompressedPubkey(req.DepositPubkey)
	if err != nil {
		return nil, "", err
	}
	msg := types.NewMsgDepositRequestPow(req.PowToken, req.DepositPubkey)
	msg.PowDurationMs = req.PowDurationMs
	return msg, owner, nil
}

func shieldMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Commitments   []json.RawMessage `json:"commitments"`
		DepositPubkey string            `json:"deposit_pubkey"`
		Signature     string            `json:"signature"`
		DepositID     string            `json:"deposit_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	commitments := make([]string, 0, len(req.Commitments))
	for _, raw := range req.Commitments {
		if len(raw) == 0 {
			continue
		}
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			commitments = append(commitments, asString)
			continue
		}
		commitments = append(commitments, string(raw))
	}
	msg := &types.MsgShielderShield{
		Commitments:   commitments,
		DepositPubkey: strings.TrimSpace(req.DepositPubkey),
		Signature:     strings.TrimSpace(req.Signature),
		DepositId:     strings.TrimSpace(req.DepositID),
	}
	owner, err := ownerFromCompressedPubkey(req.DepositPubkey)
	if err != nil {
		return nil, "", err
	}
	return msg, owner, nil
}

func withdrawMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Proof  json.RawMessage `json:"proof"`
		Public json.RawMessage `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	return types.NewMsgShielderRedeem(req.Proof, req.Public), "", nil
}

func nodeSetIPMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		IP     string `json:"ip"`
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgSetIPAddress(strings.TrimSpace(req.IP), signer), signer.String(), nil
}

func nodeSetVersionMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Version string `json:"version"`
		Signer  string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = constants.SWVersion.String()
	}
	return types.NewMsgSetVersion(version, signer), signer.String(), nil
}

func nodeSetKeysMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		SecpPubKey      string `json:"secp_pubkey"`
		EdPubKey        string `json:"ed_pubkey"`
		ConsensusPubKey string `json:"consensus_pubkey"`
		Signer          string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	secpPubKey, err := common.NewPubKey(req.SecpPubKey)
	if err != nil {
		return nil, "", fmt.Errorf("invalid secp pubkey: %w", err)
	}
	edPubKey, err := common.NewPubKey(req.EdPubKey)
	if err != nil {
		return nil, "", fmt.Errorf("invalid ed pubkey: %w", err)
	}
	return types.NewMsgSetNodeKeys(common.NewPubKeySet(secpPubKey, edPubKey), strings.TrimSpace(req.ConsensusPubKey), signer), signer.String(), nil
}

func nodeBondFromNotesMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		NodePubKey     string          `json:"node_pubkey"`
		OperatorPubKey string          `json:"operator_pubkey"`
		Proof          json.RawMessage `json:"proof"`
		Public         json.RawMessage `json:"public"`
		Signer         string          `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgBondFromNotes(req.NodePubKey, req.OperatorPubKey, req.Proof, req.Public, signer), signer.String(), nil
}

func nodeShieldFeesMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		NodePubKey        string            `json:"node_pubkey"`
		OperatorSignature string            `json:"operator_signature"`
		Commitments       []json.RawMessage `json:"commitments"`
		FeeNotePubKeys    []json.RawMessage `json:"fee_note_pubkeys"`
		Signer            string            `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	signature, err := hex.DecodeString(strings.TrimSpace(req.OperatorSignature))
	if err != nil {
		return nil, "", fmt.Errorf("invalid operator signature hex: %w", err)
	}
	commitments, err := rawStringList(req.Commitments)
	if err != nil {
		return nil, "", err
	}
	feeNotePubKeys, err := rawStringList(req.FeeNotePubKeys)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgShielderShieldFees(req.NodePubKey, signature, commitments, feeNotePubKeys, signer), signer.String(), nil
}

func nodeAuctionCreateMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		NodePubKey   string `json:"node_pubkey"`
		ReserveSats  string `json:"reserve_sats"`
		ExpiryHeight string `json:"expiry_height"`
		Signer       string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	reserve, err := parseUintJSON(req.ReserveSats, "reserve_sats")
	if err != nil {
		return nil, "", err
	}
	expiry, err := parseIntJSON(req.ExpiryHeight, "expiry_height")
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgNodeSlotAuctionCreate(req.NodePubKey, reserve, expiry, signer), signer.String(), nil
}

func nodeAuctionBidCreateMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		AuctionID      string `json:"auction_id"`
		OperatorPubKey string `json:"operator_pubkey"`
		NodePubKey     string `json:"node_pubkey"`
		Signer         string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgNodeSlotAuctionBidCreate(req.AuctionID, req.OperatorPubKey, req.NodePubKey, signer), signer.String(), nil
}

func nodeAuctionSelectBidMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		AuctionID string `json:"auction_id"`
		BidID     string `json:"bid_id"`
		Signer    string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgNodeSlotAuctionSelectBid(req.AuctionID, req.BidID, signer), signer.String(), nil
}

func nodeSaleShieldMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		AuctionID     string            `json:"auction_id"`
		BidID         string            `json:"bid_id"`
		Commitments   []json.RawMessage `json:"commitments"`
		DepositPubKey string            `json:"deposit_pubkey"`
		Signature     string            `json:"signature"`
		Signer        string            `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	commitments, err := rawStringList(req.Commitments)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgNodeSaleShield(req.AuctionID, req.BidID, commitments, req.DepositPubKey, req.Signature, signer), signer.String(), nil
}

func nodePauseChainMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgNodePauseChain(1, signer), signer.String(), nil
}

func nodeResumeChainMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgNodePauseChain(-1, signer), signer.String(), nil
}

func configVoteMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	value, err := parseIntJSON(req.Value, "value")
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgConfig(strings.TrimSpace(req.Key), value, signer), signer.String(), nil
}

func upgradeProposeMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Name   string `json:"name"`
		Height string `json:"height"`
		Info   string `json:"info"`
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	height, err := parseIntJSON(req.Height, "height")
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgProposeUpgrade(req.Name, height, req.Info, signer), signer.String(), nil
}

func upgradeApproveMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Name   string `json:"name"`
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgApproveUpgrade(req.Name, signer), signer.String(), nil
}

func upgradeRejectMsg(r *http.Request) (sdk.Msg, string, error) {
	var req struct {
		Name   string `json:"name"`
		Signer string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", err
	}
	signer, err := parseSigner(req.Signer)
	if err != nil {
		return nil, "", err
	}
	return types.NewMsgRejectUpgrade(req.Name, signer), signer.String(), nil
}

func parseSigner(raw string) (cosmos.AccAddress, error) {
	signer, err := sdk.AccAddressFromBech32(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid signer address: %w", err)
	}
	return cosmos.AccAddress(signer), nil
}

func rawStringList(values []json.RawMessage) ([]string, error) {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		if len(raw) == 0 {
			continue
		}
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			out = append(out, strings.TrimSpace(asString))
			continue
		}
		out = append(out, strings.TrimSpace(string(raw)))
	}
	return out, nil
}

func parseUintJSON(raw, name string) (uint64, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return value, nil
}

func parseIntJSON(raw, name string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return value, nil
}

func ownerFromCompressedPubkey(pubkeyHex string) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(pubkeyHex))
	if err != nil {
		return "", fmt.Errorf("invalid secp256k1 pubkey")
	}
	if len(raw) == 32 {
		raw = append([]byte{0x02}, raw...)
	}
	if len(raw) != 33 {
		return "", fmt.Errorf("invalid secp256k1 pubkey length")
	}
	pubkey := &sdksecp256k1.PubKey{Key: raw}
	return sdk.AccAddress(pubkey.Address()).String(), nil
}
