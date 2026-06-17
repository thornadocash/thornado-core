package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/common"
)

func TestObservedTxVoterSetRevertedUpdatesTxSlice(t *testing.T) {
	voter := GetRandomObservedTxVoter()
	require.Len(t, voter.Txs, 1)
	require.Equal(t, common.Status_incomplete, voter.Txs[0].Status)

	voter.SetReverted()

	require.True(t, voter.Reverted)
	require.Equal(t, common.Status_reverted, voter.Tx.Status)
	require.Equal(t, common.Status_reverted, voter.Txs[0].Status)
}
