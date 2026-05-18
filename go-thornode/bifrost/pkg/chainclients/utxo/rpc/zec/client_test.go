package zec

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, *Client) {
	ts := httptest.NewServer(handler)
	c := NewClient(ts.URL, 5*time.Second, http.Header{})
	return ts, c
}

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:8232", 10*time.Second, http.Header{})
	assert.Equal(t, "http://localhost:8232", c.url)
	assert.Equal(t, 10*time.Second, c.timeout)
	assert.Equal(t, "application/json", c.header.Get("Content-Type"))
	assert.Equal(t, "application/json", c.header.Get("Accept"))
	assert.NotNil(t, c.regexp)
	assert.NotNil(t, c.http)
}

func TestCallSuccess(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		assert.Equal(t, "getblockcount", req["method"])

		resp := Response{Result: json.RawMessage(`12345`)}
		data, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "getblockcount")
	require.NoError(t, err)
	assert.Equal(t, 12345, result)
}

func TestCallGetblockReplacesNonce(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Return a getblock response with a hex nonce (Zcash format)
		raw := `{"result":{"hash":"abc","nonce":"00000000000000000000000000000000000000000000000000000000deadbeef","bits":"1234"}}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(raw))
	})
	defer ts.Close()

	var result map[string]interface{}
	err := c.Call(&result, "getblock", "abc", 2)
	require.NoError(t, err)
	// nonce should be replaced with 0 (numeric)
	assert.Equal(t, float64(0), result["nonce"])
}

func TestCallNonGetblockDoesNotReplaceNonce(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		raw := `{"result":{"nonce":"00000000000000000000000000000000000000000000000000000000deadbeef"}}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(raw))
	})
	defer ts.Close()

	var result map[string]interface{}
	err := c.Call(&result, "getinfo")
	require.NoError(t, err)
	// nonce should remain as hex string since method is not getblock
	assert.Equal(t, "00000000000000000000000000000000000000000000000000000000deadbeef", result["nonce"])
}

func TestCallRPCError(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"error":{"code":-1,"message":"some rpc error"}}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "badmethod")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some rpc error")
}

func TestCallRPCErrorNoMessage(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"error":{"code":-5}}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "badmethod")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "json-rpc error -5")
}

func TestCallEmptyResult(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// No result field at all -> empty json.RawMessage
		resp := `{}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "getblockcount")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no result")
}

func TestCallNilResult(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"result":"ok"}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	// Passing nil result should succeed without unmarshalling
	err := c.Call(nil, "ping")
	require.NoError(t, err)
}

func TestCallNonPointerResult(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"result":"ok"}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	// Non-pointer result should return error
	err := c.Call("not-a-pointer", "getblockcount")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be pointer or nil")
}

func TestCallBadJSON(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "getblockcount")
	require.Error(t, err)
}

func TestCallHTTPError(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "getblockcount")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "server error")
}

func TestCallBadURL(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", 1*time.Second, http.Header{})

	var result int
	err := c.Call(&result, "getblockcount")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to perform request")
}

func TestPostBoolParams(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		params, ok := req["params"].([]interface{})
		assert.True(t, ok)
		// booleans should be converted to 0/1
		assert.Equal(t, float64(1), params[0])
		assert.Equal(t, float64(0), params[1])
		assert.Equal(t, "str", params[2])

		resp := `{"result":"ok"}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	err := c.Call(nil, "test", true, false, "str")
	require.NoError(t, err)
}

func TestPostNoParams(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		params, ok := req["params"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, params, 0)

		resp := `{"result":"ok"}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	err := c.Call(nil, "test")
	require.NoError(t, err)
}

func TestBatchCallSuccess(t *testing.T) {
	callCount := 0
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := `{"result":42}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	var r1, r2 int
	batch := []rpc.BatchElem{
		{Method: "getblockcount", Result: &r1},
		{Method: "getblockcount", Result: &r2},
	}
	err := c.BatchCall(batch)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, 42, r1)
	assert.Equal(t, 42, r2)
}

func TestBatchCallError(t *testing.T) {
	callCount := 0
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`error`))
			return
		}
		resp := `{"result":42}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	var r1, r2 int
	batch := []rpc.BatchElem{
		{Method: "getblockcount", Result: &r1},
		{Method: "getblockcount", Result: &r2},
	}
	err := c.BatchCall(batch)
	require.Error(t, err)
	assert.Equal(t, 42, r1)
}

func TestBatchCallEmpty(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	})
	defer ts.Close()

	err := c.BatchCall(nil)
	require.NoError(t, err)
}

func TestErrorString(t *testing.T) {
	e := &Error{Code: -1, Message: "test error"}
	assert.Equal(t, "test error", e.Error())

	e2 := &Error{Code: -5}
	assert.Equal(t, "json-rpc error -5", e2.Error())
}

func TestPostInvalidURL(t *testing.T) {
	c := NewClient("://invalid", 1*time.Second, http.Header{})
	var result int
	err := c.Call(&result, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestCallUnmarshalResultError(t *testing.T) {
	ts, c := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Return a string result but try to unmarshal into int
		resp := `{"result":"not-a-number"}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	})
	defer ts.Close()

	var result int
	err := c.Call(&result, "test")
	require.Error(t, err)
}
