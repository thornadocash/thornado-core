package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gorilla/mux"

	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestRegisterBrowserAPIUsesPlainPaths(t *testing.T) {
	rtr := mux.NewRouter()
	RegisterBrowserAPI(rtr, "thornado", client.Context{})

	req := httptest.NewRequest(http.MethodOptions, "/thornado/deposit", nil)
	rec := httptest.NewRecorder()
	rtr.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected /thornado/deposit OPTIONS to be registered, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodOptions, "/thornado/gasless/deposit", nil)
	rec = httptest.NewRecorder()
	rtr.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected old gasless path to be unregistered, got %d", rec.Code)
	}

	for _, path := range []string{
		"/thornado/browser/node/set-ip",
		"/thornado/browser/node/set-version",
		"/thornado/browser/node/set-keys",
		"/thornado/browser/node/bond-from-notes",
		"/thornado/browser/node/shield-fees",
		"/thornado/browser/node/auction-create",
		"/thornado/browser/node/auction-bid-create",
		"/thornado/browser/node/auction-select-bid",
		"/thornado/browser/node/sale-shield",
		"/thornado/browser/node/pause-chain",
		"/thornado/browser/node/resume-chain",
		"/thornado/browser/config/vote",
		"/thornado/browser/upgrade/propose",
		"/thornado/browser/upgrade/approve",
		"/thornado/browser/upgrade/reject",
	} {
		req = httptest.NewRequest(http.MethodOptions, path, nil)
		rec = httptest.NewRecorder()
		rtr.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected %s OPTIONS to be registered, got %d", path, rec.Code)
		}
	}
}

func TestStandaloneHandlerServesUIAndProxiesNodeAPI(t *testing.T) {
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("node:" + r.URL.Path))
	}))
	t.Cleanup(node.Close)
	nodeURL, err := url.Parse(node.URL)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewStandaloneHandler(StandaloneConfig{NodeURL: nodeURL})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/thornado/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected UI index, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Thornado") {
		t.Fatalf("expected UI index body, got %q", body[:min(len(body), 80)])
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/thornado/block", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected proxied API response, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "node:/thornado/block" {
		t.Fatalf("expected proxied node path, got %q", string(body))
	}
}

func TestNodeBrowserMessageBuilders(t *testing.T) {
	signer, err := ownerFromCompressedPubkey("02" + strings.Repeat("11", 32))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		body  string
		build func(*http.Request) (sdk.Msg, string, error)
		want  sdk.Msg
	}{
		{
			name:  "set ip",
			body:  `{"ip":"127.0.0.1","signer":"` + signer + `"}`,
			build: nodeSetIPMsg,
			want:  &types.MsgSetIPAddress{},
		},
		{
			name:  "set version",
			body:  `{"signer":"` + signer + `"}`,
			build: nodeSetVersionMsg,
			want:  &types.MsgSetVersion{},
		},
		{
			name:  "auction create",
			body:  `{"node_pubkey":"tthorv1pub1addwnpepqd4vrm257qdesvw0h6z259qgg27kvqxm79n4mn8twv3drmadvh676a4yxm9","reserve_sats":"100000000","expiry_height":"100","signer":"` + signer + `"}`,
			build: nodeAuctionCreateMsg,
			want:  &types.MsgNodeSlotAuctionCreate{},
		},
		{
			name:  "config vote",
			body:  `{"key":"Node_SetDesired","value":"5","signer":"` + signer + `"}`,
			build: configVoteMsg,
			want:  &types.MsgConfig{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, _, err := tt.build(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := reflect.TypeOf(msg), reflect.TypeOf(tt.want); got != want {
				t.Fatalf("expected %s, got %s", want, got)
			}
		})
	}
}
