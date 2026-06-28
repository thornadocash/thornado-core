package p2p

import (
	"context"
	crand "crypto/rand"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	tnet "github.com/libp2p/go-libp2p-testing/net"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
)

func init() {
	ApplyDeadline.Store(false)
}

func setupHosts(t *testing.T, n int) []host.Host {
	mn := mocknet.New(context.Background())
	var hosts []host.Host
	for i := 0; i < n; i++ {

		privKey, _, err := crypto.GenerateSecp256k1Key(crand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		a := tnet.RandLocalTCPAddress()
		h, err := mn.AddPeer(privKey, a)
		if err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, h)
	}

	if err := mn.LinkAll(); err != nil {
		t.Error(err)
	}
	if err := mn.ConnectAllButSelf(); err != nil {
		t.Error(err)
	}
	return hosts
}

func hostPubKey(t *testing.T, h host.Host) string {
	t.Helper()
	pubKey, err := conversion.GetPubKeyFromPeerID(h.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	return pubKey
}

func hostPubKeys(t *testing.T, hosts []host.Host) []string {
	t.Helper()
	peers := make([]string, 0, len(hosts))
	for _, h := range hosts {
		peers = append(peers, hostPubKey(t, h))
	}
	return peers
}

func coordinatorPubKey(t *testing.T, pc *PartyCoordinator) string {
	t.Helper()
	return hostPubKey(t, pc.host)
}

func leaderAppearsLastTest(t *testing.T, msgID string, peers []string, pcs []*PartyCoordinator) {
	wg := sync.WaitGroup{}

	for _, el := range pcs[1:] {
		wg.Add(1)
		go func(coordinator *PartyCoordinator) {
			defer wg.Done()
			// we simulate different nodes join at different time
			time.Sleep(time.Millisecond * time.Duration(rand.Int()%100)) // nolint:gosec
			sigChan := make(chan string)
			onlinePeers, _, err := coordinator.JoinPartyWithLeader(msgID, 10, peers, 3, sigChan)
			assert.Nil(t, err)
			assert.Len(t, onlinePeers, 4)
		}(el)
	}

	time.Sleep(time.Second * 2)
	// we start the leader firstly
	wg.Add(1)
	go func(coordinator *PartyCoordinator) {
		defer wg.Done()
		sigChan := make(chan string)
		// we simulate different nodes join at different time
		onlinePeers, _, err := coordinator.JoinPartyWithLeader(msgID, 10, peers, 3, sigChan)
		assert.Nil(t, err)
		assert.Len(t, onlinePeers, 4)
	}(pcs[0])
	wg.Wait()
}

func leaderAppersFirstTest(t *testing.T, msgID string, peers []string, pcs []*PartyCoordinator) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	// we start the leader firstly
	go func(coordinator *PartyCoordinator) {
		defer wg.Done()
		// we simulate different nodes join at different time
		sigChan := make(chan string)
		onlinePeers, _, err := coordinator.JoinPartyWithLeader(msgID, 10, peers, 3, sigChan)
		assert.Nil(t, err)
		assert.Len(t, onlinePeers, 4)
	}(pcs[0])
	time.Sleep(time.Second)
	for _, el := range pcs[1:] {
		wg.Add(1)
		go func(coordinator *PartyCoordinator) {
			defer wg.Done()
			// we simulate different nodes join at different time
			time.Sleep(time.Millisecond * time.Duration(rand.Int()%100)) // nolint:gosec
			sigChan := make(chan string)
			onlinePeers, _, err := coordinator.JoinPartyWithLeader(msgID, 10, peers, 3, sigChan)
			assert.Nil(t, err)
			assert.Len(t, onlinePeers, 4)
		}(el)
	}
	wg.Wait()
}

func TestNewPartyCoordinator(t *testing.T) {
	hosts := setupHosts(t, 4)
	var pcs []*PartyCoordinator
	peers := hostPubKeys(t, hosts)

	timeout := time.Second * 4
	for _, el := range hosts {
		pcs = append(pcs, NewPartyCoordinator(el, timeout))
	}

	defer func() {
		for _, el := range pcs {
			el.Stop()
		}
	}()

	msgID := conversion.RandStringBytesMask(64)
	leader, err := LeaderNode(msgID, 10, peers)
	assert.Nil(t, err)

	// we sort the slice to ensure the leader is the first one easy for testing
	for i, el := range pcs {
		if coordinatorPubKey(t, el) == leader {
			if i == 0 {
				break
			}
			temp := pcs[0]
			pcs[0] = el
			pcs[i] = temp
			break
		}
	}
	assert.Equal(t, coordinatorPubKey(t, pcs[0]), leader)
	// now we test the leader appears firstly and the the members
	leaderAppersFirstTest(t, msgID, peers, pcs)

	msgID = conversion.RandStringBytesMask(64)
	leader, err = LeaderNode(msgID, 10, peers)
	assert.Nil(t, err)
	for i, el := range pcs {
		if coordinatorPubKey(t, el) == leader {
			if i == 0 {
				break
			}
			temp := pcs[0]
			pcs[0] = el
			pcs[i] = temp
			break
		}
	}
	assert.Equal(t, coordinatorPubKey(t, pcs[0]), leader)
	leaderAppearsLastTest(t, msgID, peers, pcs)
}

func TestLateJoinerGetsPartyClosedAfterLeaderForms(t *testing.T) {
	hosts := setupHosts(t, 4)
	var pcs []*PartyCoordinator
	peers := hostPubKeys(t, hosts)

	timeout := 2 * time.Second
	for _, el := range hosts {
		pcs = append(pcs, NewPartyCoordinator(el, timeout))
	}
	defer func() {
		for _, el := range pcs {
			el.Stop()
		}
	}()

	msgID := conversion.RandStringBytesMask(64)
	leader := coordinatorPubKey(t, pcs[0])
	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	for _, coordinator := range pcs[:3] {
		wg.Add(1)
		go func(coordinator *PartyCoordinator) {
			defer wg.Done()
			sigChan := make(chan string)
			onlinePeers, _, err := coordinator.JoinPartyWithLeaderInitiator(msgID, 10, peers, 2, sigChan, leader)
			if err != nil {
				errCh <- err
				return
			}
			if len(onlinePeers) != 3 {
				errCh <- assert.AnError
			}
		}(coordinator)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		assert.NoError(t, err)
	}

	sigChan := make(chan string)
	start := time.Now()
	_, _, err := pcs[3].JoinPartyWithLeaderInitiator(msgID, 10, peers, 2, sigChan, leader)
	assert.Equal(t, ErrPartyClosed, err)
	assert.Less(t, time.Since(start), timeout)
}

func TestNewPartyCoordinatorTimeOut(t *testing.T) {
	timeout := time.Second * 3
	hosts := setupHosts(t, 4)
	var pcs []*PartyCoordinator
	for _, el := range hosts {
		pcs = append(pcs, NewPartyCoordinator(el, timeout))
	}
	sort.Slice(pcs, func(i, j int) bool {
		return pcs[i].host.ID().String() > pcs[j].host.ID().String()
	})
	peers := make([]string, 0, len(pcs))
	for _, el := range pcs {
		peers = append(peers, coordinatorPubKey(t, el))
	}

	defer func() {
		for _, el := range pcs {
			el.Stop()
		}
	}()

	msgID := conversion.RandStringBytesMask(64)
	wg := sync.WaitGroup{}
	leader, err := LeaderNode(msgID, 10, peers)
	assert.Nil(t, err)

	// we sort the slice to ensure the leader is the first one easy for testing
	for i, el := range pcs {
		if coordinatorPubKey(t, el) == leader {
			if i == 0 {
				break
			}
			temp := pcs[0]
			pcs[0] = el
			pcs[i] = temp
			break
		}
	}
	assert.Equal(t, coordinatorPubKey(t, pcs[0]), leader)

	// we test the leader is offline
	for _, el := range pcs[1:] {
		wg.Add(1)
		go func(coordinator *PartyCoordinator) {
			defer wg.Done()
			sigChan := make(chan string)
			_, _, err := coordinator.JoinPartyWithLeader(msgID, 10, peers, 3, sigChan)
			assert.Equal(t, err, ErrLeaderNotReady)
		}(el)

	}
	wg.Wait()
	// we test one of node is not ready
	var expected []string
	for _, el := range pcs[:3] {
		expected = append(expected, el.host.ID().String())
		sort.Strings(expected)
		wg.Add(1)
		go func(coordinator *PartyCoordinator) {
			defer wg.Done()
			sigChan := make(chan string)
			onlinePeers, _, err := coordinator.JoinPartyWithLeader(msgID, 10, peers, 3, sigChan)
			assert.Equal(t, ErrJoinPartyTimeout, err)
			var onlinePeersStr []string
			for _, el := range onlinePeers {
				onlinePeersStr = append(onlinePeersStr, el.String())
			}
			sort.Strings(onlinePeersStr)
			sort.Strings(expected[:3])
			assert.EqualValues(t, expected, onlinePeersStr)
		}(el)
	}
	wg.Wait()
}

func TestGetPeerIDs(t *testing.T) {
	mn := mocknet.New(context.Background())
	// add peers to mock net

	a1 := tnet.RandLocalTCPAddress()
	privKey, _, err := crypto.GenerateSecp256k1Key(crand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h1, err := mn.AddPeer(privKey, a1)
	if err != nil {
		t.Fatal(err)
	}
	p1 := h1.ID()
	p1PubKey := hostPubKey(t, h1)
	timeout := time.Second * 2
	pc := NewPartyCoordinator(h1, timeout)
	r, err := pc.getPeerIDs([]string{})
	assert.Nil(t, err)
	assert.Len(t, r, 0)
	input := []string{
		p1PubKey,
	}
	r1, err := pc.getPeerIDs(input)
	assert.Nil(t, err)
	assert.Len(t, r1, 1)
	assert.Equal(t, r1[0], p1)
	input = append(input, "whatever")
	r2, err := pc.getPeerIDs(input)
	assert.NotNil(t, err)
	assert.Len(t, r2, 0)
}
