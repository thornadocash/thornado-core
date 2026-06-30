package p2p

import (
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
)

func TestPartyCoordinator(t *testing.T) {
	ApplyDeadline.Store(false)
	hosts := setupHosts(t, 4)
	var pcs []PartyCoordinator
	peers := hostPubKeys(t, hosts)

	timeout := time.Second * 10
	for _, el := range hosts {
		pcs = append(pcs, *NewPartyCoordinator(el, timeout))
	}

	defer func() {
		for _, el := range pcs {
			el.Stop()
		}
	}()

	msgID := conversion.RandStringBytesMask(64)
	wg := sync.WaitGroup{}

	for _, el := range pcs {
		wg.Add(1)

		go func(coordinator PartyCoordinator) {
			defer wg.Done()
			// we simulate different nodes join at different time
			time.Sleep(time.Second * time.Duration(rand.Int()%10)) // nolint:gosec
			onlinePeers, err := coordinator.JoinPartyWithRetry(msgID, peers)
			if err != nil {
				t.Error(err)
			}
			assert.Nil(t, err)
			assert.Len(t, onlinePeers, 4)
		}(el)
	}

	wg.Wait()
}

func TestPartyCoordinatorTimeOut(t *testing.T) {
	ApplyDeadline.Store(false)
	timeout := time.Second
	hosts := setupHosts(t, 4)
	var pcs []*PartyCoordinator
	for _, el := range hosts {
		pcs = append(pcs, NewPartyCoordinator(el, timeout))
	}
	sort.Slice(pcs, func(i, j int) bool {
		return pcs[i].host.ID().String() > pcs[j].host.ID().String()
	})
	var peers []string
	var expected []string
	for _, el := range pcs {
		peers = append(peers, coordinatorPubKey(t, el))
	}
	for _, el := range pcs[:2] {
		expected = append(expected, el.host.ID().String())
	}

	// Stop the party coordinators that should not participate
	// This prevents them from handling any incoming streams
	for _, el := range pcs[2:] {
		el.Stop()
	}

	defer func() {
		for _, el := range pcs[:2] {
			el.Stop()
		}
	}()

	msgID := conversion.RandStringBytesMask(64)
	wg := sync.WaitGroup{}
	sort.Strings(expected)

	for _, el := range pcs[:2] {
		wg.Add(1)
		go func(coordinator *PartyCoordinator) {
			defer wg.Done()
			onlinePeers, err := coordinator.JoinPartyWithRetry(msgID, peers)
			assert.Errorf(t, err, ErrJoinPartyTimeout.Error())
			var onlinePeersStr []string
			for _, el := range onlinePeers {
				onlinePeersStr = append(onlinePeersStr, el.String())
			}
			sort.Strings(onlinePeersStr)
			assert.NotEmpty(t, onlinePeersStr)
			assert.Subset(t, expected, onlinePeersStr)
			assert.Contains(t, onlinePeersStr, coordinator.host.ID().String())
		}(el)
	}

	wg.Wait()
}
