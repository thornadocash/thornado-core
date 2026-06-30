package frost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKeygenPartyThresholdRequiresEveryTargetedMember(t *testing.T) {
	participants := []string{"node1", "node2", "node3", "node4"}

	require.Equal(t, 4, keygenPartyThreshold(participants))
	require.Equal(t, uint16(3), frostMinSigners(len(participants)))
}

func TestFrostSessionLockHonorsContextWhileWaiting(t *testing.T) {
	lock := newFrostSessionLock()
	require.NoError(t, lock.lock(context.Background()))
	defer lock.unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	require.ErrorIs(t, lock.lock(ctx), context.DeadlineExceeded)
}
