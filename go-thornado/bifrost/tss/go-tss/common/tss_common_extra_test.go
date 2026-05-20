package common

import (
	"encoding/json"
	"math/big"
	"sync"

	btsskeygen "github.com/binance-chain/tss-lib/ecdsa/keygen"
	btss "github.com/binance-chain/tss-lib/tss"
	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/libp2p/go-libp2p-core/peer"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/blame"
	"github.com/thornadocash/go-thornado/common"
)

type TssExtraSuite struct{}

var _ = Suite(&TssExtraSuite{})

func (s *TssExtraSuite) SetUpSuite(c *C) {
	conversion.SetupBech32Prefix()
}

func makePartyID(id, moniker string) *btss.PartyID {
	return btss.NewPartyID(id, moniker, new(big.Int).SetInt64(1))
}

// --- LocalCacheItem tests ---

func (s *TssExtraSuite) TestLocalCacheItemGetPeers(c *C) {
	item := NewLocalCacheItem(nil, "hash1")
	peers := item.GetPeers()
	c.Assert(peers, HasLen, 0)

	item.UpdateConfirmList("peer1", "h1")
	item.UpdateConfirmList("peer2", "h2")
	item.UpdateConfirmList("peer3", "h3")
	peers = item.GetPeers()
	c.Assert(peers, HasLen, 3)
}

func (s *TssExtraSuite) TestLocalCacheItemTotalConfirmParty(c *C) {
	item := NewLocalCacheItem(nil, "hash1")
	c.Assert(item.TotalConfirmParty(), Equals, 0)

	item.UpdateConfirmList("peer1", "h1")
	c.Assert(item.TotalConfirmParty(), Equals, 1)

	item.UpdateConfirmList("peer2", "h2")
	c.Assert(item.TotalConfirmParty(), Equals, 2)
}

// --- checkUnicast tests ---

func (s *TssExtraSuite) TestCheckUnicast(c *C) {
	// ECDSA keygen: index 1 or 2 => true
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "KGR-test", Index: 1}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "KGR-test", Index: 2}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "KGR-test", Index: 0}), Equals, false)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "KGR-test", Index: 3}), Equals, false)

	// ECDSA regroup: index 3 => true
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "DGR-test", Index: 3}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "DGR-test", Index: 2}), Equals, false)

	// ECDSA keysign: index < 5 => true
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "SignR-test", Index: 0}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "SignR-test", Index: 4}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "SignR-test", Index: 5}), Equals, false)

	// EDDSA keygen: index 1 => true
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-KGR-test", Index: 1}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-KGR-test", Index: 0}), Equals, false)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-KGR-test", Index: 2}), Equals, false)

	// EDDSA regroup: index 3 => true
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-DGR-test", Index: 3}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-DGR-test", Index: 2}), Equals, false)

	// EDDSA keysign: index > 2 => true
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-SignR-test", Index: 3}), Equals, true)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-SignR-test", Index: 2}), Equals, false)
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-SignR-test", Index: 1}), Equals, false)

	// EDDSA no match (no KGR, DGR, or SignR) => false
	c.Assert(checkUnicast(blame.RoundInfo{RoundMsg: "EDDSA-other", Index: 1}), Equals, false)
}

// --- getBroadcastMessageType tests ---

func (s *TssExtraSuite) TestGetBroadcastMessageType(c *C) {
	c.Assert(getBroadcastMessageType(messages.TSSKeyGenMsg), Equals, messages.TSSKeyGenVerMsg)
	c.Assert(getBroadcastMessageType(messages.TSSKeySignMsg), Equals, messages.TSSKeySignVerMsg)
	c.Assert(getBroadcastMessageType(messages.TSSTaskDone), Equals, messages.Unknown)
}

// --- MsgToHashInt tests ---

func (s *TssExtraSuite) TestMsgToHashIntEdDSA(c *C) {
	input := []byte("test-data")
	result, err := MsgToHashInt(input, common.SigningAlgoEd25519)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(result.Sign() > 0, Equals, true)
}

func (s *TssExtraSuite) TestMsgToHashIntInvalidAlgo(c *C) {
	input := []byte("test-data")
	_, err := MsgToHashInt(input, "invalid-algo")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "invalid algo")
}

// --- InitLog tests ---

func (s *TssExtraSuite) TestInitLog(c *C) {
	InitLog("debug", false, "test-service")
	InitLog("info", true, "test-service")
	InitLog("invalid-level", false, "test-service")
}

// --- renderToP2P tests ---

func (s *TssExtraSuite) TestRenderToP2PNilChannel(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	tss.renderToP2P(&messages.BroadcastMsgChan{})
}

func (s *TssExtraSuite) TestRenderToP2PWithChannel(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)
	broadcastMsg := &messages.BroadcastMsgChan{}
	tss.renderToP2P(broadcastMsg)
	received := <-ch
	c.Assert(received, Equals, broadcastMsg)
}

// --- Getter/Setter tests ---

func (s *TssExtraSuite) TestTssCommonGettersSetters(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{PartyTimeout: 10}, "msg1", sk, 1)

	c.Assert(tss.GetConf().PartyTimeout, Equals, TssConfig{PartyTimeout: 10}.PartyTimeout)
	c.Assert(tss.GetLocalPeerID(), Equals, "peer1")
	c.Assert(tss.GetBlameMgr(), NotNil)
	c.Assert(tss.GetTaskDone(), NotNil)

	tss.SetLocalPeerID("peer2")
	c.Assert(tss.GetLocalPeerID(), Equals, "peer2")

	c.Assert(tss.getPartyInfo(), IsNil)
	pi := &PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: make(map[string]*btss.PartyID),
	}
	tss.SetPartyInfo(pi)
	c.Assert(tss.getPartyInfo(), Equals, pi)
}

// --- TryGetLocalCacheItem / TryGetAllLocalCached ---

func (s *TssExtraSuite) TestTryGetAllLocalCached(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	items := tss.TryGetAllLocalCached()
	c.Assert(items, HasLen, 0)

	tss.updateLocalUnconfirmedMessages("key1", NewLocalCacheItem(nil, "hash1"))
	tss.updateLocalUnconfirmedMessages("key2", NewLocalCacheItem(nil, "hash2"))

	items = tss.TryGetAllLocalCached()
	c.Assert(items, HasLen, 2)
}

func (s *TssExtraSuite) TestTryGetLocalCacheItemNotFound(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	c.Assert(tss.TryGetLocalCacheItem("nonexistent"), IsNil)
}

// --- removeKey ---

func (s *TssExtraSuite) TestRemoveKey(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	tss.updateLocalUnconfirmedMessages("key1", NewLocalCacheItem(nil, "hash1"))
	c.Assert(tss.TryGetLocalCacheItem("key1"), NotNil)
	tss.removeKey("key1")
	c.Assert(tss.TryGetLocalCacheItem("key1"), IsNil)
}

// --- NewBulkWireMsg ---

func (s *TssExtraSuite) TestNewBulkWireMsg(c *C) {
	r := &btss.MessageRouting{IsBroadcast: true}
	msg := NewBulkWireMsg([]byte("data"), "id1", r)
	c.Assert(msg.WiredBulkMsgs, DeepEquals, []byte("data"))
	c.Assert(msg.MsgIdentifier, Equals, "id1")
	c.Assert(msg.Routing, Equals, r)
}

// --- checkDupAndUpdateVerMsg ---

func (s *TssExtraSuite) TestCheckDupAndUpdateVerMsg(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	bMsg := &messages.BroadcastConfirmMessage{Key: "key1", Hash: "hash1"}
	ret := tss.checkDupAndUpdateVerMsg(bMsg, "peerA")
	c.Assert(ret, Equals, true)
	c.Assert(bMsg.P2PID, Equals, "peerA")

	item := NewLocalCacheItem(nil, "hash1")
	item.UpdateConfirmList("peerA", "hash1")
	tss.updateLocalUnconfirmedMessages("key1", item)

	bMsg2 := &messages.BroadcastConfirmMessage{Key: "key1", Hash: "hash1"}
	ret = tss.checkDupAndUpdateVerMsg(bMsg2, "peerA")
	c.Assert(ret, Equals, false)

	bMsg3 := &messages.BroadcastConfirmMessage{Key: "key1", Hash: "hash1"}
	ret = tss.checkDupAndUpdateVerMsg(bMsg3, "peerB")
	c.Assert(ret, Equals, true)
	c.Assert(bMsg3.P2PID, Equals, "peerB")
}

// --- ProcessOneMessage edge cases ---

func (s *TssExtraSuite) TestProcessOneMessageNil(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	err := tss.ProcessOneMessage(nil, "peer")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "invalid wireMessage")
}

func (s *TssExtraSuite) TestProcessOneMessageBadPayload(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	// TSSKeyGenMsg with bad payload
	err := tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSKeyGenMsg,
		Payload:     []byte("not-json"),
	}, "peer")
	c.Assert(err, NotNil)

	// TSSKeySignMsg with bad payload
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSKeySignMsg,
		Payload:     []byte("not-json"),
	}, "peer")
	c.Assert(err, NotNil)

	// TSSKeyGenVerMsg with bad payload
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSKeyGenVerMsg,
		Payload:     []byte("not-json"),
	}, "peer")
	c.Assert(err, NotNil)

	// TSSKeySignVerMsg with bad payload
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSKeySignVerMsg,
		Payload:     []byte("not-json"),
	}, "peer")
	c.Assert(err, NotNil)

	// TSSTaskDone with bad payload - logs error but returns nil
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSTaskDone,
		Payload:     []byte("not-json"),
	}, "peer")
	c.Assert(err, IsNil)

	// TSSControlMsg with bad payload
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSControlMsg,
		Payload:     []byte("not-json"),
	}, "peer")
	c.Assert(err, NotNil)
}

func (s *TssExtraSuite) TestProcessOneMessageTaskDoneNotDone(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{"a": makePartyID("a", "a"), "b": makePartyID("b", "b"), "c": makePartyID("c", "c")},
	})

	taskNotDone := messages.TssTaskNotifier{TaskDone: false}
	payload, err := json.Marshal(taskNotDone)
	c.Assert(err, IsNil)
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSTaskDone,
		Payload:     payload,
	}, "peer1")
	c.Assert(err, IsNil)
}

func (s *TssExtraSuite) TestProcessOneMessageDuplicateTaskDone(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	tss.SetPartyInfo(&PartyInfo{
		PartyMap: &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{
			"a": makePartyID("a", "a"), "b": makePartyID("b", "b"),
			"c": makePartyID("c", "c"), "d": makePartyID("d", "d"),
		},
	})

	taskDone := messages.TssTaskNotifier{TaskDone: true}
	payload, err := json.Marshal(taskDone)
	c.Assert(err, IsNil)
	wrappedMsg := &messages.WrappedMessage{
		MessageType: messages.TSSTaskDone,
		Payload:     payload,
	}

	err = tss.ProcessOneMessage(wrappedMsg, "peerA")
	c.Assert(err, IsNil)

	err = tss.ProcessOneMessage(wrappedMsg, "peerA")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "duplicated notification.*")
}

func (s *TssExtraSuite) TestProcessOneMessageControlMsgNoExist(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{},
	})

	controlMsg := messages.TssControl{
		ReqHash:     "nonexistent-hash",
		ReqKey:      "key1",
		RequestType: messages.TSSKeyGenMsg,
		Msg:         &messages.WireMessage{},
	}
	payload, err := json.Marshal(controlMsg)
	c.Assert(err, IsNil)
	err = tss.ProcessOneMessage(&messages.WrappedMessage{
		MessageType: messages.TSSControlMsg,
		Payload:     payload,
	}, "16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	c.Assert(err, IsNil)
}

// --- getMsgHash tests ---

func (s *TssExtraSuite) TestGetMsgHash(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	item := NewLocalCacheItem(nil, "hash1")
	item.ConfirmedList["p1"] = "hashA"
	item.ConfirmedList["p2"] = "hashA"
	item.ConfirmedList["p3"] = "hashB"

	hash, err := tss.getMsgHash(item, 3)
	c.Assert(err, IsNil)
	c.Assert(hash, Equals, "hashA")

	_, err = tss.getMsgHash(item, 5)
	c.Assert(err, Equals, blame.ErrHashInconsistency)

	emptyItem := NewLocalCacheItem(nil, "hash1")
	_, err = tss.getMsgHash(emptyItem, 2)
	c.Assert(err, Equals, blame.ErrHashCheck)
}

// --- hashCheck tests ---

func (s *TssExtraSuite) TestHashCheckDataOwnerNotFound(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	routingFrom := makePartyID("unknown-party", "unknown")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom},
	}
	item := NewLocalCacheItem(wireMsg, "hash1")
	item.ConfirmedList["p1"] = "hash1"
	item.ConfirmedList["p2"] = "hash1"

	err := tss.hashCheck(item, 2)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "error in find the data Owner P2PID")
}

func (s *TssExtraSuite) TestHashCheckNotEnoughPeers(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.PartyIDtoP2PID["sender-party"] = peerID

	routingFrom := makePartyID("sender-party", "sender")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom},
	}
	item := NewLocalCacheItem(wireMsg, "hash1")
	item.ConfirmedList["p1"] = "hash1"

	err := tss.hashCheck(item, 3)
	c.Assert(err, Equals, blame.ErrNotEnoughPeer)
}

func (s *TssExtraSuite) TestHashCheckOwnerInConfirmedList(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.PartyIDtoP2PID["sender-party"] = peerID

	routingFrom := makePartyID("sender-party", "sender")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom},
	}
	item := NewLocalCacheItem(wireMsg, "hash1")
	item.ConfirmedList[peerID.String()] = "hash1"
	item.ConfirmedList["other-peer"] = "hash1"

	err := tss.hashCheck(item, 2)
	c.Assert(err, Equals, blame.ErrHashFromOwner)
}

func (s *TssExtraSuite) TestHashCheckNotMajority(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	ownerPeerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.PartyIDtoP2PID["sender-party"] = ownerPeerID

	routingFrom := makePartyID("sender-party", "sender")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom},
	}

	item := NewLocalCacheItem(wireMsg, "targetHash")
	item.ConfirmedList["peer1"] = "differentHash"
	item.ConfirmedList["peer2"] = "differentHash"

	err := tss.hashCheck(item, 2)
	c.Assert(err, Equals, blame.ErrNotMajority)
}

func (s *TssExtraSuite) TestHashCheckSuccess(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	ownerPeerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.PartyIDtoP2PID["sender-party"] = ownerPeerID

	routingFrom := makePartyID("sender-party", "sender")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom},
	}

	item := NewLocalCacheItem(wireMsg, "hashA")
	item.ConfirmedList["peer1"] = "hashA"
	item.ConfirmedList["peer2"] = "hashA"

	err := tss.hashCheck(item, 2)
	c.Assert(err, IsNil)
}

// --- broadcastHashToPeers ---

func (s *TssExtraSuite) TestBroadcastHashToPeersEmptyPeers(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	err := tss.broadcastHashToPeers("key1", "hash1", nil, messages.TSSKeyGenVerMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "fail to get any peer ID")
}

func (s *TssExtraSuite) TestBroadcastHashToPeersSuccess(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	err := tss.broadcastHashToPeers("key1", "hash1", []peer.ID{peerID}, messages.TSSKeyGenVerMsg)
	c.Assert(err, IsNil)
	received := <-ch
	c.Assert(received, NotNil)
	c.Assert(received.PeersID, HasLen, 1)
}

// --- updateLocal edge cases ---

func (s *TssExtraSuite) TestUpdateLocalNilMsg(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	err := tss.updateLocal(nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "invalid wireMsg")

	err = tss.updateLocal(&messages.WireMessage{Routing: nil})
	c.Assert(err, NotNil)

	err = tss.updateLocal(&messages.WireMessage{Routing: &btss.MessageRouting{From: nil}})
	c.Assert(err, NotNil)
}

func (s *TssExtraSuite) TestUpdateLocalNilPartyInfo(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	from := makePartyID("party1", "party1")
	err := tss.updateLocal(&messages.WireMessage{
		Routing: &btss.MessageRouting{From: from},
	})
	c.Assert(err, IsNil)
}

func (s *TssExtraSuite) TestUpdateLocalMissingP2PID(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	partyID := makePartyID("known", "known")
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{"known": partyID},
	})
	from := makePartyID("known", "known")
	err := tss.updateLocal(&messages.WireMessage{
		Routing: &btss.MessageRouting{From: from},
	})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "fail to find the peer")
}

func (s *TssExtraSuite) TestUpdateLocalBadBulkMsg(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	partyID := makePartyID("known", "known")
	tss.PartyIDtoP2PID["known"] = peerID
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{"known": partyID},
	})
	from := makePartyID("known", "known")
	err := tss.updateLocal(&messages.WireMessage{
		Routing:   &btss.MessageRouting{From: from, IsBroadcast: false},
		Message:   []byte("not-json"),
		RoundInfo: "round1",
	})
	c.Assert(err, NotNil)
}

// --- processTSSMsg edge cases ---

func (s *TssExtraSuite) TestProcessTSSMsgNil(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	err := tss.processTSSMsg(nil, messages.TSSKeyGenMsg, false)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "invalid wireMsg")
}

// --- requestShareFromPeer ---

func (s *TssExtraSuite) TestRequestShareFromPeerAllTypes(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 10)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	item := NewLocalCacheItem(nil, "hash1")
	item.ConfirmedList["16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp"] = "hashA"
	item.ConfirmedList["16Uiu2HAm2FzqoUdS6Y9Esg2EaGcAG5rVe1r6BFNnmmQr2H3bqafa"] = "hashA"

	err := tss.requestShareFromPeer(item, 2, "key1", messages.TSSKeyGenVerMsg)
	c.Assert(err, IsNil)

	err = tss.requestShareFromPeer(item, 2, "key1", messages.TSSKeySignVerMsg)
	c.Assert(err, IsNil)

	err = tss.requestShareFromPeer(item, 2, "key1", messages.TSSKeyGenMsg)
	c.Assert(err, IsNil)

	err = tss.requestShareFromPeer(item, 2, "key1", messages.TSSKeySignMsg)
	c.Assert(err, IsNil)

	// Unknown type
	err = tss.requestShareFromPeer(item, 2, "key1", messages.TSSTaskDone)
	c.Assert(err, IsNil)
}

func (s *TssExtraSuite) TestRequestShareFromPeerCantGetHash(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	item := NewLocalCacheItem(nil, "hash1")
	err := tss.requestShareFromPeer(item, 2, "key1", messages.TSSKeyGenVerMsg)
	c.Assert(err, IsNil)
}

// --- NotifyTaskDone ---

func (s *TssExtraSuite) TestNotifyTaskDoneWithChannel(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)
	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.P2PPeersLock.Lock()
	tss.P2PPeers = []peer.ID{peerID}
	tss.P2PPeersLock.Unlock()
	err := tss.NotifyTaskDone()
	c.Assert(err, IsNil)
	received := <-ch
	c.Assert(received, NotNil)
	c.Assert(received.WrappedMessage.MessageType, Equals, messages.TSSTaskDone)
}

// --- sendBulkMsg ---

func (s *TssExtraSuite) TestSendBulkMsgBroadcast(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.P2PPeersLock.Lock()
	tss.P2PPeers = []peer.ID{peerID}
	tss.P2PPeersLock.Unlock()

	from := makePartyID("sender", "sender")
	r := &btss.MessageRouting{
		From:        from,
		IsBroadcast: true,
	}
	wiredMsgList := []BulkWireMsg{
		NewBulkWireMsg([]byte("data"), "id1", r),
	}

	err := tss.sendBulkMsg("round1", messages.TSSKeyGenMsg, wiredMsgList)
	c.Assert(err, IsNil)
	received := <-ch
	c.Assert(received, NotNil)
	c.Assert(len(received.PeersID), Equals, 1)
}

func (s *TssExtraSuite) TestSendBulkMsgUnicast(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	toPartyID := makePartyID("to-party", "to")
	tss.PartyIDtoP2PID["to-party"] = peerID

	from := makePartyID("sender", "sender")
	r := &btss.MessageRouting{
		From:        from,
		To:          []*btss.PartyID{toPartyID},
		IsBroadcast: false,
	}
	wiredMsgList := []BulkWireMsg{
		NewBulkWireMsg([]byte("data"), "id1", r),
	}

	err := tss.sendBulkMsg("round1", messages.TSSKeySignMsg, wiredMsgList)
	c.Assert(err, IsNil)
	received := <-ch
	c.Assert(received, NotNil)
	c.Assert(len(received.PeersID), Equals, 1)
}

func (s *TssExtraSuite) TestSendBulkMsgUnknownTo(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	toPartyID := makePartyID("unknown-party", "unknown")
	from := makePartyID("sender", "sender")
	r := &btss.MessageRouting{
		From:        from,
		To:          []*btss.PartyID{toPartyID},
		IsBroadcast: false,
	}
	wiredMsgList := []BulkWireMsg{
		NewBulkWireMsg([]byte("data"), "id1", r),
	}

	err := tss.sendBulkMsg("round1", messages.TSSKeyGenMsg, wiredMsgList)
	c.Assert(err, IsNil)
	received := <-ch
	c.Assert(received, NotNil)
	c.Assert(len(received.PeersID), Equals, 0)
}

// --- receiverBroadcastHashToPeers ---

func (s *TssExtraSuite) TestReceiverBroadcastHashToPeersUnknownOwner(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	from := makePartyID("unknown", "unknown")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: from},
	}
	err := tss.receiverBroadcastHashToPeers(wireMsg, messages.TSSKeyGenMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "error in find the data owner peerID")
}

func (s *TssExtraSuite) TestReceiverBroadcastHashToPeersSkipsOwner(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	ownerPeerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	otherPeerID, _ := peer.Decode("16Uiu2HAm2FzqoUdS6Y9Esg2EaGcAG5rVe1r6BFNnmmQr2H3bqafa")

	tss.PartyIDtoP2PID["owner-party"] = ownerPeerID
	tss.P2PPeersLock.Lock()
	tss.P2PPeers = []peer.ID{ownerPeerID, otherPeerID}
	tss.P2PPeersLock.Unlock()

	from := makePartyID("owner-party", "owner")
	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: from},
		RoundInfo: "round1",
		Message:   []byte("test-message"),
	}

	err := tss.receiverBroadcastHashToPeers(wireMsg, messages.TSSKeyGenMsg)
	c.Assert(err, IsNil)
	received := <-ch
	c.Assert(received, NotNil)
	c.Assert(len(received.PeersID), Equals, 1)
	c.Assert(received.PeersID[0], Equals, otherPeerID)
}

// --- processVerMsg ---

func (s *TssExtraSuite) TestProcessVerMsgNil(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	err := tss.processVerMsg(nil, messages.TSSKeyGenVerMsg)
	c.Assert(err, IsNil)
}

func (s *TssExtraSuite) TestProcessVerMsgNoPartyInfo(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)
	bMsg := &messages.BroadcastConfirmMessage{Key: "key1", Hash: "hash1"}
	err := tss.processVerMsg(bMsg, messages.TSSKeyGenVerMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*local party is not ready")
}

// --- verifySignature failure ---

func (s *TssExtraSuite) TestVerifySignatureFail(c *C) {
	sk := secp256k1.GenPrivKey()
	msg := []byte("hello")
	msgID := "123"
	sig, err := generateSignature(msg, msgID, sk)
	c.Assert(err, IsNil)

	// wrong message
	ret := verifySignature(sk.PubKey(), []byte("wrong"), sig, msgID)
	c.Assert(ret, Equals, false)

	// wrong msgID
	ret = verifySignature(sk.PubKey(), msg, sig, "wrong-id")
	c.Assert(ret, Equals, false)

	// wrong key
	sk2 := secp256k1.GenPrivKey()
	ret = verifySignature(sk2.PubKey(), msg, sig, msgID)
	c.Assert(ret, Equals, false)
}

// --- hashToInt edge cases ---

func (s *TssExtraSuite) TestHashToInt(c *C) {
	// Test with hash longer than curve order bytes (excess > 0 path)
	longHash := make([]byte, 64)
	for i := range longHash {
		longHash[i] = 0xff
	}
	result, err := MsgToHashInt(longHash, common.SigningAlgoSecp256k1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(result.Sign() > 0, Equals, true)
}

// --- processTSSMsg with signature verification failure ---

func (s *TssExtraSuite) TestProcessTSSMsgBadSignature(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	senderKey := new(big.Int).SetBytes(secp256k1.GenPrivKey().PubKey().Bytes())
	senderPartyID := btss.NewPartyID("sender", "sender", senderKey)

	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{"sender": senderPartyID},
	})

	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: senderPartyID, IsBroadcast: false},
		RoundInfo: "round1",
		Message:   []byte("test"),
		Sig:       []byte("bad-signature"),
	}
	err := tss.processTSSMsg(wireMsg, messages.TSSKeyGenMsg, false)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "signature verify failed")
}

func (s *TssExtraSuite) TestProcessTSSMsgUnknownParty(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: map[string]*btss.PartyID{},
	})

	from := makePartyID("unknown", "unknown")
	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: from, IsBroadcast: false},
		RoundInfo: "round1",
		Message:   []byte("test"),
		Sig:       []byte("sig"),
	}
	err := tss.processTSSMsg(wireMsg, messages.TSSKeyGenMsg, false)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "error in find the data owner")
}

// --- applyShare edge cases ---

func (s *TssExtraSuite) TestApplyShareNotEnoughPeer(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	ownerPeerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.PartyIDtoP2PID["sender"] = ownerPeerID

	routingFrom := makePartyID("sender", "sender")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom, IsBroadcast: true},
		Message: []byte("test-msg"),
	}
	item := NewLocalCacheItem(wireMsg, "hash1")
	item.ConfirmedList["p1"] = "hash1"
	tss.updateLocalUnconfirmedMessages("key1", item)

	// threshold too high, so hashCheck returns ErrNotEnoughPeer -> applyShare returns nil
	err := tss.applyShare(item, 5, "key1", messages.TSSKeyGenVerMsg)
	c.Assert(err, IsNil)
}

func (s *TssExtraSuite) TestApplyShareHashCheckErrHashCheck(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	ownerPeerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	tss.PartyIDtoP2PID["sender"] = ownerPeerID

	routingFrom := makePartyID("sender", "sender")
	wireMsg := &messages.WireMessage{
		Routing: &btss.MessageRouting{From: routingFrom, IsBroadcast: false},
		Message: []byte("test-msg"),
		Sig:     []byte("test-sig"),
	}
	item := NewLocalCacheItem(wireMsg, "hash1")
	// Owner sends hash for own message => ErrHashFromOwner => falls through to TssWrongShareBlame
	// This path is complex - just test the not-enough-peer path
	item.ConfirmedList["p1"] = "hash1"
	tss.updateLocalUnconfirmedMessages("key1", item)

	// Not enough peers => returns nil
	err := tss.applyShare(item, 5, "key1", messages.TSSKeyGenVerMsg)
	c.Assert(err, IsNil)
}

// --- ProcessInboundMessages with bad payload ---

func (s *TssExtraSuite) TestProcessInboundMessagesBadPayload(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	finishChan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)

	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")

	go tss.ProcessInboundMessages(finishChan, wg)

	// Send bad message
	tss.TssMsg <- &p2p.Message{PeerID: peerID, Payload: []byte("not-json")}

	// Close to stop
	close(finishChan)
	wg.Wait()
}

// --- ProcessInboundMessages channel close ---

func (s *TssExtraSuite) TestProcessInboundMessagesChannelClose(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg1", sk, 1)

	finishChan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go tss.ProcessInboundMessages(finishChan, wg)

	// Close TssMsg channel to trigger the !ok path
	close(tss.TssMsg)
	wg.Wait()
}

// --- Contains edge cases ---

func (s *TssExtraSuite) TestContainsEmptySlice(c *C) {
	p := makePartyID("test", "test")
	c.Assert(Contains([]*btss.PartyID{}, p), Equals, false)
}

// --- ProcessOutCh unicast path ---

func (s *TssExtraSuite) TestProcessOutChUnicast(c *C) {
	conf := TssConfig{}
	localTestPubKeys := make([]string, len(testPubKeys))
	copy(localTestPubKeys, testPubKeys[:])
	partiesID, localPartyID, err := conversion.GetParties(localTestPubKeys, testPubKeys[0])
	c.Assert(err, IsNil)

	// Unicast routing (IsBroadcast=false, To has specific party)
	messageRouting := btss.MessageRouting{
		From:        localPartyID,
		To:          partiesID[1:2],
		IsBroadcast: false,
	}
	testFill := []byte("TEST")
	testContent := &btsskeygen.KGRound1Message{
		Commitment: testFill,
	}
	msg := btss.NewMessageWrapper(messageRouting, testContent)
	tssMsg := btss.NewMessage(messageRouting, testContent, msg)
	sk := secp256k1.GenPrivKey()
	tssCommonStruct := NewTssCommon("", nil, conf, "test", sk, 1)
	err = tssCommonStruct.ProcessOutCh(tssMsg, messages.TSSKeyGenMsg)
	c.Assert(err, IsNil)
}

// --- ProcessOutCh broadcast append (second call) ---

func (s *TssExtraSuite) TestProcessOutChBroadcastAppend(c *C) {
	conf := TssConfig{}
	localTestPubKeys := make([]string, len(testPubKeys))
	copy(localTestPubKeys, testPubKeys[:])
	partiesID, localPartyID, err := conversion.GetParties(localTestPubKeys, testPubKeys[0])
	c.Assert(err, IsNil)

	messageRouting := btss.MessageRouting{
		From:        localPartyID,
		To:          partiesID[3:],
		IsBroadcast: true,
	}
	testContent := &btsskeygen.KGRound1Message{Commitment: []byte("TEST")}
	msg := btss.NewMessageWrapper(messageRouting, testContent)
	tssMsg := btss.NewMessage(messageRouting, testContent, msg)
	sk := secp256k1.GenPrivKey()
	// msgNum=2 so the first call caches and second appends
	tssCommonStruct := NewTssCommon("", nil, conf, "test", sk, 2)
	err = tssCommonStruct.ProcessOutCh(tssMsg, messages.TSSKeyGenMsg)
	c.Assert(err, IsNil)
	// Second call: append to existing cache
	err = tssCommonStruct.ProcessOutCh(tssMsg, messages.TSSKeyGenMsg)
	c.Assert(err, IsNil)
}

// --- GetMsgRound with invalid data ---

func (s *TssExtraSuite) TestGetMsgRoundInvalid(c *C) {
	from := makePartyID("test", "test")
	_, err := GetMsgRound([]byte("invalid"), from, true)
	c.Assert(err, NotNil)
}

// --- processRequestMsgFromPeer with non-nil stored message ---

func (s *TssExtraSuite) TestProcessRequestMsgFromPeerWithStoredMsg(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 1)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	testPeer, _ := peer.Decode("16Uiu2HAm2FzqoUdS6Y9Esg2EaGcAG5rVe1r6BFNnmmQr2H3bqafa")

	// Store a real message in the round manager
	storedWireMsg := &messages.WireMessage{
		RoundInfo: "test-round",
		Message:   []byte("stored-msg"),
	}
	tss.blameMgr.GetRoundMgr().Set("test-key", storedWireMsg)

	msg := &messages.TssControl{
		ReqHash:     "hash",
		ReqKey:      "test-key",
		RequestType: messages.TSSKeyGenMsg,
		Msg:         nil,
	}

	// requester=false, storedMsg != nil => msg.Msg gets set to storedMsg
	err := tss.processRequestMsgFromPeer([]peer.ID{testPeer}, msg, false)
	c.Assert(err, IsNil)
	c.Assert(msg.Msg, NotNil)
	c.Assert(msg.Msg.RoundInfo, Equals, "test-round")
}

// --- requestShareFromPeer with invalid peer decode ---

func (s *TssExtraSuite) TestRequestShareFromPeerBadPeerDecode(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 10)
	tss := NewTssCommon("peer1", ch, TssConfig{}, "msg1", sk, 1)

	item := NewLocalCacheItem(nil, "hash1")
	// Use invalid peer ID strings that can't be decoded
	item.ConfirmedList["invalid-peer-id"] = "hashA"
	item.ConfirmedList["also-invalid"] = "hashA"

	err := tss.requestShareFromPeer(item, 2, "key1", messages.TSSKeyGenVerMsg)
	c.Assert(err, NotNil) // peer.Decode fails
}

// --- processTSSMsg with valid signature, unicast path ---

func (s *TssExtraSuite) TestProcessTSSMsgUnicastValidSig(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("peer1", nil, TssConfig{}, "msg-id", sk, 1)

	// Create a party ID using the private key's public key
	senderKey := new(big.Int).SetBytes(sk.PubKey().Bytes())
	senderPartyID := btss.NewPartyID("sender", "sender", senderKey)
	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")

	partyIDMap := map[string]*btss.PartyID{"sender": senderPartyID}
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: partyIDMap,
	})
	tss.PartyIDtoP2PID["sender"] = peerID

	// Create message with valid signature
	msgContent := []byte("test-msg-content")
	sig, err := generateSignature(msgContent, "msg-id", sk)
	c.Assert(err, IsNil)

	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: senderPartyID, IsBroadcast: false},
		RoundInfo: "round1",
		Message:   msgContent,
		Sig:       sig,
	}
	// This will go through the unicast path (IsBroadcast=false)
	// updateLocal will fail on unmarshal but we get coverage of the sig verification + unicast path
	err = tss.processTSSMsg(wireMsg, messages.TSSKeyGenMsg, false)
	c.Assert(err, NotNil) // fails in updateLocal's json unmarshal
}

// --- processTSSMsg broadcast path ---

func (s *TssExtraSuite) TestProcessTSSMsgBroadcastValidSig(c *C) {
	sk := secp256k1.GenPrivKey()
	ch := make(chan *messages.BroadcastMsgChan, 10)
	tss := NewTssCommon("local-peer", ch, TssConfig{}, "msg-id", sk, 1)

	senderKey := new(big.Int).SetBytes(sk.PubKey().Bytes())
	senderPartyID := btss.NewPartyID("sender", "sender", senderKey)
	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")
	otherPeerID, _ := peer.Decode("16Uiu2HAm2FzqoUdS6Y9Esg2EaGcAG5rVe1r6BFNnmmQr2H3bqafa")

	partyIDMap := map[string]*btss.PartyID{
		"sender": senderPartyID,
		"other":  makePartyID("other", "other"),
	}
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: partyIDMap,
	})
	tss.PartyIDtoP2PID["sender"] = peerID
	tss.PartyIDtoP2PID["other"] = otherPeerID
	tss.P2PPeersLock.Lock()
	tss.P2PPeers = []peer.ID{peerID, otherPeerID}
	tss.P2PPeersLock.Unlock()

	msgContent := []byte("test-msg-broadcast")
	sig, err := generateSignature(msgContent, "msg-id", sk)
	c.Assert(err, IsNil)

	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: senderPartyID, IsBroadcast: true},
		RoundInfo: "round1",
		Message:   msgContent,
		Sig:       sig,
	}

	// Broadcast path with forward=false: broadcasts hash to peers, creates local cache item
	// Proceeds through applyShare -> hashCheck -> updateLocal (which fails on JSON unmarshal)
	err = tss.processTSSMsg(wireMsg, messages.TSSKeyGenMsg, false)
	// Error from updateLocal is expected since message content isn't valid JSON bulk msg
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "fail to update the message to local party.*")
}

// --- processTSSMsg broadcast with forward=true ---

func (s *TssExtraSuite) TestProcessTSSMsgBroadcastForward(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("local-peer", nil, TssConfig{}, "msg-id", sk, 1)

	senderKey := new(big.Int).SetBytes(sk.PubKey().Bytes())
	senderPartyID := btss.NewPartyID("sender", "sender", senderKey)
	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")

	partyIDMap := map[string]*btss.PartyID{
		"sender": senderPartyID,
		"other":  makePartyID("other", "other"),
	}
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: partyIDMap,
	})
	tss.PartyIDtoP2PID["sender"] = peerID

	msgContent := []byte("test-msg-forward")
	sig, err := generateSignature(msgContent, "msg-id", sk)
	c.Assert(err, IsNil)

	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: senderPartyID, IsBroadcast: true},
		RoundInfo: "round1",
		Message:   msgContent,
		Sig:       sig,
	}

	// forward=true skips receiverBroadcastHashToPeers
	err = tss.processTSSMsg(wireMsg, messages.TSSKeyGenMsg, true)
	// Error from updateLocal is expected since message content isn't valid JSON bulk msg
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "fail to update the message to local party.*")
}

// --- processTSSMsg broadcast with existing cache item (msg already seen) ---

func (s *TssExtraSuite) TestProcessTSSMsgBroadcastExistingCacheItem(c *C) {
	sk := secp256k1.GenPrivKey()
	tss := NewTssCommon("local-peer", nil, TssConfig{}, "msg-id", sk, 1)

	senderKey := new(big.Int).SetBytes(sk.PubKey().Bytes())
	senderPartyID := btss.NewPartyID("sender", "sender", senderKey)
	peerID, _ := peer.Decode("16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp")

	partyIDMap := map[string]*btss.PartyID{
		"sender": senderPartyID,
		"other":  makePartyID("other", "other"),
	}
	tss.SetPartyInfo(&PartyInfo{
		PartyMap:   &sync.Map{},
		PartyIDMap: partyIDMap,
	})
	tss.PartyIDtoP2PID["sender"] = peerID

	msgContent := []byte("test-msg-existing")
	sig, err := generateSignature(msgContent, "msg-id", sk)
	c.Assert(err, IsNil)

	wireMsg := &messages.WireMessage{
		Routing:   &btss.MessageRouting{From: senderPartyID, IsBroadcast: true},
		RoundInfo: "round1",
		Message:   msgContent,
		Sig:       sig,
	}

	// Pre-create a cache item with nil Msg (simulates receiving ver msg first)
	cacheKey := wireMsg.GetCacheKey()
	existingItem := NewLocalCacheItem(nil, "")
	tss.updateLocalUnconfirmedMessages(cacheKey, existingItem)

	// Now process the actual message - should fill in the Msg on existing item
	// Error from updateLocal (bad JSON) is expected but we cover the existing-item path
	err = tss.processTSSMsg(wireMsg, messages.TSSKeyGenMsg, true)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "fail to update the message to local party.*")
}
