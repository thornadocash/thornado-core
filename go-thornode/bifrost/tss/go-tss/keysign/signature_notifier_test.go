package keysign

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	tsslibcommon "github.com/binance-chain/tss-lib/common"
	"github.com/libp2p/go-libp2p-core/peer"
	tnet "github.com/libp2p/go-libp2p-testing/net"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/common"
)

func TestSignatureNotifierHappyPath(t *testing.T) {
	conversion.SetupBech32Prefix()
	poolPubKey := `thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak`
	messageToSign := "yhEwrxWuNBGnPT/L7PNnVWg7gFWNzCYTV+GuX3tKRH8="
	buf, err := base64.StdEncoding.DecodeString(messageToSign)
	assert.Nil(t, err)
	messageID, err := common.MsgToHashString(buf)
	assert.Nil(t, err)
	p2p.ApplyDeadline.Store(false)
	id1 := tnet.RandIdentityOrFatal(t)
	id2 := tnet.RandIdentityOrFatal(t)
	id3 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	// add peers to mock net

	a1 := tnet.RandLocalTCPAddress()
	a2 := tnet.RandLocalTCPAddress()
	a3 := tnet.RandLocalTCPAddress()

	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	if err != nil {
		t.Fatal(err)
	}
	p1 := h1.ID()
	h2, err := mn.AddPeer(id2.PrivateKey(), a2)
	if err != nil {
		t.Fatal(err)
	}
	p2 := h2.ID()
	h3, err := mn.AddPeer(id3.PrivateKey(), a3)
	if err != nil {
		t.Fatal(err)
	}
	p3 := h3.ID()
	if err := mn.LinkAll(); err != nil {
		t.Error(err)
	}
	if err := mn.ConnectAllButSelf(); err != nil {
		t.Error(err)
	}
	n1 := NewSignatureNotifier(h1)
	n2 := NewSignatureNotifier(h2)
	n3 := NewSignatureNotifier(h3)
	assert.NotNil(t, n1)
	assert.NotNil(t, n2)
	assert.NotNil(t, n3)
	sigFile := "../test_data/signature_notify/sig1.json"
	content, err := os.ReadFile(sigFile)
	assert.Nil(t, err)
	assert.NotNil(t, content)
	var signature tsslibcommon.SignatureData
	err = json.Unmarshal(content, &signature)
	assert.Nil(t, err)
	sigChan := make(chan string)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		sig, err := n1.WaitForSignature(messageID, [][]byte{buf}, poolPubKey, time.Second*30, sigChan)
		assert.Nil(t, err)
		assert.NotNil(t, sig)
	}()

	assert.Nil(t, n2.BroadcastSignature(messageID, []*tsslibcommon.SignatureData{&signature}, []peer.ID{
		p1, p3,
	}))
	assert.Nil(t, n3.BroadcastSignature(messageID, []*tsslibcommon.SignatureData{&signature}, []peer.ID{
		p1, p2,
	}))
	wg.Wait()
}

func TestSignatureNotifierBroadcastFirst(t *testing.T) {
	conversion.SetupBech32Prefix()
	poolPubKey := `thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak`
	messageToSign := "yhEwrxWuNBGnPT/L7PNnVWg7gFWNzCYTV+GuX3tKRH8="
	buf, err := base64.StdEncoding.DecodeString(messageToSign)
	assert.Nil(t, err)
	messageID, err := common.MsgToHashString(buf)
	assert.Nil(t, err)
	p2p.ApplyDeadline.Store(false)
	id1 := tnet.RandIdentityOrFatal(t)
	id2 := tnet.RandIdentityOrFatal(t)
	id3 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	// add peers to mock net

	a1 := tnet.RandLocalTCPAddress()
	a2 := tnet.RandLocalTCPAddress()
	a3 := tnet.RandLocalTCPAddress()

	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	if err != nil {
		t.Fatal(err)
	}
	p1 := h1.ID()
	h2, err := mn.AddPeer(id2.PrivateKey(), a2)
	if err != nil {
		t.Fatal(err)
	}
	p2 := h2.ID()
	h3, err := mn.AddPeer(id3.PrivateKey(), a3)
	if err != nil {
		t.Fatal(err)
	}
	p3 := h3.ID()
	if err := mn.LinkAll(); err != nil {
		t.Error(err)
	}
	if err := mn.ConnectAllButSelf(); err != nil {
		t.Error(err)
	}
	n1 := NewSignatureNotifier(h1)
	n2 := NewSignatureNotifier(h2)
	n3 := NewSignatureNotifier(h3)
	assert.NotNil(t, n1)
	assert.NotNil(t, n2)
	assert.NotNil(t, n3)
	sigFile := "../test_data/signature_notify/sig1.json"
	content, err := os.ReadFile(sigFile)
	assert.Nil(t, err)
	assert.NotNil(t, content)
	var signature tsslibcommon.SignatureData
	err = json.Unmarshal(content, &signature)
	assert.Nil(t, err)
	sigChan := make(chan string)

	assert.NotContains(t, n1.notifiers, messageID)

	assert.Nil(t, n2.BroadcastSignature(messageID, []*tsslibcommon.SignatureData{&signature}, []peer.ID{
		p1, p3,
	}))

	assert.Nil(t, n3.BroadcastSignature(messageID, []*tsslibcommon.SignatureData{&signature}, []peer.ID{
		p1, p2,
	}))

	n1.notifierLock.Lock()
	assert.Contains(t, n1.notifiers, messageID)
	notifier := n1.notifiers[messageID]
	n1.notifierLock.Unlock()
	notifier.mu.Lock()
	assert.False(t, notifier.readyToProcess())
	assert.Equal(t, defaultNotifierTTL, notifier.ttl)
	notifier.mu.Unlock()

	sig, err := n1.WaitForSignature(messageID, [][]byte{buf}, poolPubKey, time.Second*30, sigChan)
	assert.Nil(t, err)
	assert.NotNil(t, sig)

	n1.notifierLock.Lock()
	assert.NotContains(t, n1.notifiers, messageID)
	n1.notifierLock.Unlock()

	// check ttl logic and cleanup
	n3.notifierLock.Lock()
	assert.Contains(t, n3.notifiers, messageID)
	notifier = n3.notifiers[messageID]
	n3.notifierLock.Unlock()
	notifier.mu.Lock()
	notifier.ttl = 0
	notifier.mu.Unlock()

	n3.Start()
	defer n3.Stop()

	// let cleanup goroutine run
	time.Sleep(time.Second)

	n3.notifierLock.Lock()
	assert.NotContains(t, n3.notifiers, messageID)
	n3.notifierLock.Unlock()
}

func TestSignatureNotifierTimeout(t *testing.T) {
	conversion.SetupBech32Prefix()
	poolPubKey := `thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak`
	messageToSign := "yhEwrxWuNBGnPT/L7PNnVWg7gFWNzCYTV+GuX3tKRH8="
	buf, err := base64.StdEncoding.DecodeString(messageToSign)
	assert.Nil(t, err)
	messageID, err := common.MsgToHashString(buf)
	assert.Nil(t, err)
	p2p.ApplyDeadline.Store(false)

	id1 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	a1 := tnet.RandLocalTCPAddress()
	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	assert.Nil(t, err)

	n1 := NewSignatureNotifier(h1)
	assert.NotNil(t, n1)

	sigChan := make(chan string)
	// Use a very short timeout to trigger the timeout path
	sig, err := n1.WaitForSignature(messageID, [][]byte{buf}, poolPubKey, time.Millisecond*100, sigChan)
	assert.NotNil(t, err)
	assert.Nil(t, sig)
	assert.Contains(t, err.Error(), "timeout")
}

func TestSignatureNotifierSigChan(t *testing.T) {
	conversion.SetupBech32Prefix()
	poolPubKey := `thorpub1addwnpepq0ul3xt882a6nm6m7uhxj4tk2n82zyu647dyevcs5yumuadn4uamqx7neak`
	messageToSign := "yhEwrxWuNBGnPT/L7PNnVWg7gFWNzCYTV+GuX3tKRH8="
	buf, err := base64.StdEncoding.DecodeString(messageToSign)
	assert.Nil(t, err)
	messageID, err := common.MsgToHashString(buf)
	assert.Nil(t, err)
	p2p.ApplyDeadline.Store(false)

	id1 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	a1 := tnet.RandLocalTCPAddress()
	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	assert.Nil(t, err)

	n1 := NewSignatureNotifier(h1)
	assert.NotNil(t, n1)

	sigChan := make(chan string, 1)
	// Send signal before WaitForSignature to trigger sigChan path
	go func() {
		time.Sleep(time.Millisecond * 50)
		sigChan <- "done"
	}()
	sig, err := n1.WaitForSignature(messageID, [][]byte{buf}, poolPubKey, time.Second*30, sigChan)
	assert.Equal(t, p2p.ErrSigGenerated, err)
	assert.Nil(t, sig)
}

func TestSignatureNotifierBroadcastFailed(t *testing.T) {
	conversion.SetupBech32Prefix()
	p2p.ApplyDeadline.Store(false)

	id1 := tnet.RandIdentityOrFatal(t)
	id2 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	a1 := tnet.RandLocalTCPAddress()
	a2 := tnet.RandLocalTCPAddress()

	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	assert.Nil(t, err)
	p1 := h1.ID()
	h2, err := mn.AddPeer(id2.PrivateKey(), a2)
	assert.Nil(t, err)
	p2 := h2.ID()

	assert.Nil(t, mn.LinkAll())
	assert.Nil(t, mn.ConnectAllButSelf())

	n1 := NewSignatureNotifier(h1)
	n2 := NewSignatureNotifier(h2)
	assert.NotNil(t, n1)
	assert.NotNil(t, n2)

	// BroadcastFailed sends nil signatures
	err = n1.BroadcastFailed("test-message-id", []peer.ID{p2})
	assert.Nil(t, err)

	// Broadcasting to self should be skipped
	err = n1.BroadcastFailed("test-message-id-2", []peer.ID{p1, p2})
	assert.Nil(t, err)
}

func TestSignatureNotifierReleaseStream(t *testing.T) {
	conversion.SetupBech32Prefix()
	p2p.ApplyDeadline.Store(false)

	id1 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	a1 := tnet.RandLocalTCPAddress()
	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	assert.Nil(t, err)

	n1 := NewSignatureNotifier(h1)
	assert.NotNil(t, n1)

	// ReleaseStream should not panic even with unknown message ID
	n1.ReleaseStream("unknown-message-id")
}

func TestSignatureNotifierGetNotifier(t *testing.T) {
	conversion.SetupBech32Prefix()
	p2p.ApplyDeadline.Store(false)

	id1 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	a1 := tnet.RandLocalTCPAddress()
	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	assert.Nil(t, err)

	n1 := NewSignatureNotifier(h1)

	// getNotifier creates a new notifier
	notifier1, err := n1.getNotifier("msg1", [][]byte{[]byte("hello")}, "pk", nil)
	assert.Nil(t, err)
	assert.NotNil(t, notifier1)

	// getNotifier returns existing notifier
	notifier2, err := n1.getNotifier("msg1", nil, "", nil)
	assert.Nil(t, err)
	assert.Equal(t, notifier1, notifier2)

	// getNotifier with empty messageID should fail
	notifier3, err := n1.getNotifier("", nil, "", nil)
	assert.NotNil(t, err)
	assert.Nil(t, notifier3)
}

func TestSignatureNotifierRemoveNotifier(t *testing.T) {
	conversion.SetupBech32Prefix()
	p2p.ApplyDeadline.Store(false)

	id1 := tnet.RandIdentityOrFatal(t)
	mn := mocknet.New(context.Background())
	a1 := tnet.RandLocalTCPAddress()
	h1, err := mn.AddPeer(id1.PrivateKey(), a1)
	assert.Nil(t, err)

	sn := NewSignatureNotifier(h1)

	// Create a notifier
	n, err := sn.getNotifier("msg-remove", [][]byte{[]byte("data")}, "pk", nil)
	assert.Nil(t, err)
	assert.NotNil(t, n)

	sn.notifierLock.Lock()
	assert.Contains(t, sn.notifiers, "msg-remove")
	sn.notifierLock.Unlock()

	// Remove it
	sn.removeNotifier(n)

	sn.notifierLock.Lock()
	assert.NotContains(t, sn.notifiers, "msg-remove")
	sn.notifierLock.Unlock()
}
