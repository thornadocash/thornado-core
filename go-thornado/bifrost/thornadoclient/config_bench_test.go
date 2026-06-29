package thornadoclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
)

func BenchmarkGetConfigValueCachedHTTPResponse(b *testing.B) {
	const configCount = 128

	mux := http.NewServeMux()
	mux.HandleFunc(ConfigEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{")
		for i := 0; i < configCount; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, "%q:{%q:%d}", fmt.Sprintf("Key_%03d", i), "value", i)
		}
		fmt.Fprint(w, "}")
	})
	mux.HandleFunc(ConfigDefaultsEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Signer":{"Concurrency":{"value":4}}}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	httpClient := retryablehttp.NewClient()
	httpClient.Logger = nil
	bridge := &thornadoBridge{
		cfg: config.BifrostClientConfiguration{
			ChainHost: server.URL,
		},
		httpClient: httpClient,
	}

	value, err := bridge.GetConfigValue(constants.Signer_Concurrency.String())
	if err != nil {
		b.Fatal(err)
	}
	if value != 4 {
		b.Fatalf("expected config value 4, got %d", value)
	}
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, err = bridge.GetConfigValue(constants.Signer_Concurrency.String())
		if err != nil {
			b.Fatal(err)
		}
		if value != 4 {
			b.Fatalf("expected config value 4, got %d", value)
		}
	}
}
