package thornado

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTendermintRPCListenAddressUsesRPCFlag(t *testing.T) {
	origArgs := os.Args
	origEnv := os.Getenv("THORNADO_TENDERMINT_RPC_LISTEN_ADDRESS")
	t.Cleanup(func() {
		os.Args = origArgs
		require.NoError(t, os.Setenv("THORNADO_TENDERMINT_RPC_LISTEN_ADDRESS", origEnv))
	})

	require.NoError(t, os.Setenv("THORNADO_TENDERMINT_RPC_LISTEN_ADDRESS", "tcp://127.0.0.1:26657"))
	os.Args = []string{"thornado", "start", "--rpc.laddr", "tcp://127.0.0.1:33360"}

	require.Equal(t, "tcp://127.0.0.1:33360", tendermintRPCListenAddress())
}

func TestTendermintRPCListenAddressUsesRPCFlagEquals(t *testing.T) {
	origArgs := os.Args
	origEnv := os.Getenv("THORNADO_TENDERMINT_RPC_LISTEN_ADDRESS")
	t.Cleanup(func() {
		os.Args = origArgs
		require.NoError(t, os.Setenv("THORNADO_TENDERMINT_RPC_LISTEN_ADDRESS", origEnv))
	})

	require.NoError(t, os.Setenv("THORNADO_TENDERMINT_RPC_LISTEN_ADDRESS", "tcp://127.0.0.1:26657"))
	os.Args = []string{"thornado", "start", "--rpc.laddr=tcp://127.0.0.1:33361"}

	require.Equal(t, "tcp://127.0.0.1:33361", tendermintRPCListenAddress())
}
