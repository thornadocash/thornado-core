package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTronRpc(t *testing.T) {
	rpc := NewTronRpc("http://localhost:8545", 5*time.Second)
	assert.NotNil(t, rpc)
	assert.Equal(t, "http://localhost:8545", rpc.url)
	assert.Equal(t, 5*time.Second, rpc.timeout)
	assert.NotNil(t, rpc.http)
}

func TestEthCall_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "2.0", req["jsonrpc"])
		assert.Equal(t, "eth_call", req["method"])
		assert.Equal(t, float64(1), req["id"])

		params, ok := req["params"].([]any)
		require.True(t, ok)
		require.Len(t, params, 2)

		callParams, ok := params[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "0xdeadbeef", callParams["data"])
		assert.Equal(t, "latest", params[1])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0xabc123"}`))
	}))
	defer server.Close()

	rpc := NewTronRpc(server.URL, 5*time.Second)
	data, err := rpc.EthCall("0xcontract", "deadbeef")
	require.NoError(t, err)

	var resp Response
	require.NoError(t, json.Unmarshal(data, &resp))
	assert.Equal(t, "0xabc123", resp.Result)
}

func TestEthCall_DataAlreadyHasPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		params, ok := req["params"].([]any)
		require.True(t, ok)
		callParams, ok := params[0].(map[string]any)
		require.True(t, ok)
		// Should not double-prefix
		assert.Equal(t, "0xdeadbeef", callParams["data"])

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	rpc := NewTronRpc(server.URL, 5*time.Second)
	_, err := rpc.EthCall("0xcontract", "0xdeadbeef")
	require.NoError(t, err)
}

func TestEthCall_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	rpc := NewTronRpc(server.URL, 5*time.Second)
	_, err := rpc.EthCall("0xcontract", "deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "response status: 500")
}

func TestEthCall_BadURL(t *testing.T) {
	rpc := NewTronRpc("http://127.0.0.1:1", 1*time.Second)
	_, err := rpc.EthCall("0xcontract", "deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to perform request")
}

func TestEthCall_InvalidURL(t *testing.T) {
	rpc := NewTronRpc("://bad-url", 1*time.Second)
	_, err := rpc.EthCall("0xcontract", "deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestNewMockServer(t *testing.T) {
	server := NewMockServer()
	defer server.Close()

	rpc := NewTronRpc(server.URL, 5*time.Second)
	data, err := rpc.EthCall("0xcontract", "deadbeef")
	require.NoError(t, err)

	var resp Response
	require.NoError(t, json.Unmarshal(data, &resp))
	assert.Equal(t, "0x00000000000000000000000000000000000000000000000000000000d18bcc80", resp.Result)
}

func TestNewMockServer_BadRequest(t *testing.T) {
	server := NewMockServer()
	defer server.Close()

	// Send non-JSON body to trigger bad request path
	resp, err := http.Post(server.URL, "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestResponse_Unmarshal(t *testing.T) {
	data := `{"error":{"code":-32000,"message":"execution reverted","data":"0x"},"result":""}`
	var resp Response
	require.NoError(t, json.Unmarshal([]byte(data), &resp))
	assert.Equal(t, int64(-32000), resp.Error.Code)
	assert.Equal(t, "execution reverted", resp.Error.Message)
	assert.Equal(t, "0x", resp.Error.Data)
	assert.Empty(t, resp.Result)
}
