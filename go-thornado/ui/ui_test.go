package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/gorilla/mux"
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
	if body := rec.Body.String(); !strings.Contains(body, "Thornado MVP") {
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
