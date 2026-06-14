package ui

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"

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
