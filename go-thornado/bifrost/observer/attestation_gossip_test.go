package observer

import (
	"context"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/ebifrost"
)

func TestObservedTxKeySeparatesSignedPayloadWhenTxIDIsPresent(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID("ABC123"),
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
			Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
			Gas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))},
		},
		BlockHeight:    10,
		ObservedPubKey: common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz"),
	}
	finalTx := tx
	finalTx.BlockHeight = 20
	finalTx.FinaliseHeight = 20
	lateFinalTx := finalTx
	otherNonFinalTx := tx
	otherNonFinalTx.BlockHeight = 11

	require.Equal(t,
		newObservedTxKey(finalTx, false, false),
		newObservedTxKey(lateFinalTx, false, false),
	)
	require.NotEqual(t,
		newObservedTxKey(tx, false, false),
		newObservedTxKey(otherNonFinalTx, false, false),
	)
	require.NotEqual(t,
		newObservedTxKey(tx, false, false),
		newObservedTxKey(finalTx, false, false),
	)
}

func TestObservedTxKeySeparatesObservedPubKey(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID("ABC123"),
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
			Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		},
		BlockHeight:    10,
		ObservedPubKey: common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz"),
	}
	other := tx
	other.ObservedPubKey = common.PubKey("tthorpub1addwnpepqvhs4e7hw09eldp9h95yetzqkdvkwgztut3a0cx578unaceajr48kl3qmhx")

	require.NotEqual(t,
		newObservedTxKey(tx, false, false),
		newObservedTxKey(other, false, false),
	)
}

func TestObservedTxKeySeparatesSourceInputs(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID("ABC123"),
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
			Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
			Gas:         common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))},
			SourceInputs: []common.TxInput{{
				TxID:       common.TxID("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
				Vout:       0,
				AmountSats: 1000,
			}},
		},
		BlockHeight:    10,
		FinaliseHeight: 10,
		ObservedPubKey: common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz"),
	}
	other := tx
	other.Tx.SourceInputs = append([]common.TxInput(nil), tx.Tx.SourceInputs...)
	other.Tx.SourceInputs[0].AmountSats = 2000

	require.NotEqual(t,
		newObservedTxKey(tx, true, false),
		newObservedTxKey(other, true, false),
	)
}

func TestObservedTxKeyIgnoresAllowFutureObservationWhenFinal(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID("ABC123"),
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
			Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		},
		BlockHeight:    10,
		FinaliseHeight: 10,
		ObservedPubKey: common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz"),
	}

	require.Equal(t,
		newObservedTxKey(tx, false, false),
		newObservedTxKey(tx, false, true),
	)
}

func TestObservedTxKeySeparatesAllowFutureObservationWhenNotFinal(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID("ABC123"),
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
			Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		},
		BlockHeight:    10,
		ObservedPubKey: common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz"),
	}

	require.NotEqual(t,
		newObservedTxKey(tx, false, false),
		newObservedTxKey(tx, false, true),
	)
}

func TestObservedTxKeyUsesPayloadHashWhenTxIDIsEmpty(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
			Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		},
		BlockHeight: 10,
	}
	other := tx
	other.BlockHeight = 11

	require.NotEqual(t,
		newObservedTxKey(tx, true, false),
		newObservedTxKey(other, true, false),
	)
}

func TestCommittedObservedTxDoesNotRemoveDeckWithoutQuorum(t *testing.T) {
	localPubKey := []byte("local-pubkey")
	removedDeck := false
	gossip := &AttestationGossip{
		pubKey: localPubKey,
		activeVals: map[peer.ID]bool{
			"node1": true,
			"node2": true,
			"node3": true,
			"node4": true,
		},
		observerHandleObservedTxCommitted: func(tx common.ObservedTx) {
			removedDeck = true
		},
	}
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
		BlockHeight:    630,
		FinaliseHeight: 630,
	}

	qtx := common.QuorumTx{
		ObsTx:        tx,
		Attestations: []*common.Attestation{{PubKey: localPubKey}},
	}
	payload, err := qtx.Marshal()
	require.NoError(t, err)

	gossip.handleQuorumTxCommitted(&ebifrost.EventNotification{Payload: payload})

	require.False(t, removedDeck)
}

func TestCommittedObservedTxRemovesDeckWhenLocalAttestationHasQuorum(t *testing.T) {
	localPubKey := []byte("local-pubkey")
	var removed common.ObservedTx
	removedDeck := false
	gossip := &AttestationGossip{
		pubKey: localPubKey,
		activeVals: map[peer.ID]bool{
			"node1": true,
			"node2": true,
			"node3": true,
			"node4": true,
		},
		observerHandleObservedTxCommitted: func(tx common.ObservedTx) {
			removedDeck = true
			removed = tx
		},
	}
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
		BlockHeight:    630,
		FinaliseHeight: 630,
	}

	qtx := common.QuorumTx{
		ObsTx: tx,
		Attestations: []*common.Attestation{
			{PubKey: localPubKey},
			{PubKey: []byte("other-pubkey-1")},
			{PubKey: []byte("other-pubkey-2")},
		},
	}
	payload, err := qtx.Marshal()
	require.NoError(t, err)

	gossip.handleQuorumTxCommitted(&ebifrost.EventNotification{Payload: payload})

	require.True(t, removedDeck)
	require.Equal(t, tx.Tx.ID, removed.Tx.ID)
}

func TestCommittedObservedTxDoesNotRemoveDeckForOtherNodeAttestation(t *testing.T) {
	localPubKey := []byte("local-pubkey")
	removedDeck := false
	gossip := &AttestationGossip{
		pubKey: localPubKey,
		observerHandleObservedTxCommitted: func(tx common.ObservedTx) {
			removedDeck = true
		},
	}
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
		BlockHeight:    630,
		FinaliseHeight: 630,
	}

	qtx := common.QuorumTx{
		ObsTx:        tx,
		Attestations: []*common.Attestation{{PubKey: []byte("other-pubkey")}},
	}
	payload, err := qtx.Marshal()
	require.NoError(t, err)

	gossip.handleQuorumTxCommitted(&ebifrost.EventNotification{Payload: payload})

	require.False(t, removedDeck)
}

func TestAttestObservedTxDoesNotRebroadcastLocalDuplicate(t *testing.T) {
	privKey := secp256k1.GenPrivKey()
	sendCount := 0
	gossip := &AttestationGossip{
		host:    NewMockHost([]peer.ID{"node1"}),
		privKey: privKey,
		pubKey:  privKey.PubKey().Bytes(),
		activeVals: map[peer.ID]bool{
			"node1": true,
			"node2": true,
			"node3": true,
			"node4": true,
		},
		observedTxs:     make(map[txKey]*AttestationState[*attestableObservedTx]),
		observedTxsPool: NewAttestationStatePool[*attestableObservedTx](),
		batcher: &AttestationBatcher{
			maxBatchSize:  100,
			forceSendChan: make(chan struct{}, 1),
		},
		grpcClient: &MockGRPCClient{
			sendQuorumTxFunc: func(ctx context.Context, quorumTx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
				sendCount++
				return &ebifrost.SendQuorumTxResult{}, nil
			},
		},
	}
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:          common.TxID("ABC123"),
			Chain:       common.BTCChain,
			FromAddress: common.Address("from"),
			ToAddress:   common.Address("to"),
		},
		BlockHeight:    630,
		FinaliseHeight: 630,
	}

	require.NoError(t, gossip.AttestObservedTx(context.Background(), &tx, false, false))
	require.NoError(t, gossip.AttestObservedTx(context.Background(), &tx, false, false))

	gossip.batcher.mu.Lock()
	defer gossip.batcher.mu.Unlock()
	require.Len(t, gossip.batcher.observedTxBatch, 1)
	require.Equal(t, 1, sendCount)
}

func TestSendObservedTxAttestationsResubmitsSentUncommitted(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
		BlockHeight:    630,
		FinaliseHeight: 630,
	}
	state := &AttestationState[*attestableObservedTx]{
		Item: &attestableObservedTx{ObservedTx: &tx},
		attestations: []attestationSentState{
			{attestation: &common.Attestation{PubKey: []byte("pubkey1"), Signature: []byte("sig1")}, sent: true},
			{attestation: &common.Attestation{PubKey: []byte("pubkey2"), Signature: []byte("sig2")}, sent: true, committed: true},
		},
	}

	var sent *common.QuorumTx
	gossip := &AttestationGossip{
		grpcClient: &MockGRPCClient{
			sendQuorumTxFunc: func(ctx context.Context, quorumTx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
				sent = quorumTx
				return &ebifrost.SendQuorumTxResult{}, nil
			},
		},
	}

	gossip.sendObservedTxAttestationsToThornado(context.Background(), tx, state, true, false, false)

	require.NotNil(t, sent)
	require.Len(t, sent.Attestations, 1)
	require.Equal(t, []byte("pubkey1"), sent.Attestations[0].PubKey)
	require.Equal(t, 0, state.UnsentCount())
	require.Equal(t, 1, state.UncommittedCount())
	require.False(t, state.lastAttestationsSent.IsZero())
}

func TestMaybeRecoverObservedTxWhenLocalAttestationMissing(t *testing.T) {
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
		BlockHeight:    630,
		FinaliseHeight: 630,
	}
	k := newObservedTxKey(tx, true, false)
	state := &AttestationState[*attestableObservedTx]{
		Item: &attestableObservedTx{ObservedTx: &tx, inbound: true},
		attestations: []attestationSentState{
			{attestation: &common.Attestation{PubKey: []byte("remote-1"), Signature: []byte("sig-1")}},
		},
	}
	gossip := &AttestationGossip{
		host:                      NewMockHost([]peer.ID{"local"}),
		pubKey:                    []byte("local"),
		activeVals:                map[peer.ID]bool{"local": true},
		observerRecoverObservedTx: func(context.Context, common.ObservedTx, bool, bool) {},
		observedTxRecovery:        make(map[txKey]time.Time),
	}

	req, ok := gossip.maybeRecoverObservedTx(k, state)
	require.True(t, ok)
	require.Equal(t, tx.Tx.ID, req.tx.Tx.ID)
	require.True(t, req.inbound)

	_, ok = gossip.maybeRecoverObservedTx(k, state)
	require.False(t, ok)

	gossip.observedTxRecovery[k] = time.Now().Add(-defaultObservedTxRecoveryRetry - time.Second)
	state.attestations = append(state.attestations, attestationSentState{
		attestation: &common.Attestation{PubKey: []byte("local"), Signature: []byte("local-sig")},
	})
	_, ok = gossip.maybeRecoverObservedTx(k, state)
	require.False(t, ok)
}

func TestActiveValidatorSetChangeClearsAttestationState(t *testing.T) {
	oldPeer := peer.ID("old")
	newPeer := peer.ID("new")
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
	}
	obsKey := newObservedTxKey(tx, true, false)
	networkFee := common.NetworkFee{Height: 1, Chain: common.BTCChain}
	solvencyID := common.TxID("SOLVENCY")
	errata := common.ErrataTx{Id: common.TxID("ERRATA"), Chain: common.BTCChain}

	gossip := &AttestationGossip{
		activeVals: map[peer.ID]bool{oldPeer: true},
		observedTxs: map[txKey]*AttestationState[*attestableObservedTx]{
			obsKey: {
				Item: &attestableObservedTx{ObservedTx: &tx, inbound: true},
			},
		},
		networkFees: map[common.NetworkFee]*AttestationState[*common.NetworkFee]{
			networkFee: {Item: &networkFee},
		},
		solvencies: map[common.TxID]*AttestationState[*common.Solvency]{
			solvencyID: {Item: &common.Solvency{Id: solvencyID, Chain: common.BTCChain}},
		},
		errataTxs: map[common.ErrataTx]*AttestationState[*common.ErrataTx]{
			errata: {Item: &errata},
		},
		observedTxsPool: NewAttestationStatePool[*attestableObservedTx](),
		networkFeesPool: NewAttestationStatePool[*common.NetworkFee](),
		solvenciesPool:  NewAttestationStatePool[*common.Solvency](),
		errataTxsPool:   NewAttestationStatePool[*common.ErrataTx](),
		batcher: &AttestationBatcher{
			observedTxBatch: []*common.AttestTx{{ObsTx: tx}},
			networkFeeBatch: []*common.AttestNetworkFee{{NetworkFee: &networkFee}},
			solvencyBatch:   []*common.AttestSolvency{{Solvency: &common.Solvency{Id: solvencyID, Chain: common.BTCChain}}},
			errataTxBatch:   []*common.AttestErrataTx{{ErrataTx: &errata}},
		},
		stateInitPeers: map[peer.ID]bool{oldPeer: true},
	}

	gossip.setActiveValidatorPeers(map[peer.ID]bool{newPeer: true})

	require.Equal(t, map[peer.ID]bool{newPeer: true}, gossip.activeVals)
	require.Empty(t, gossip.observedTxs)
	require.Empty(t, gossip.networkFees)
	require.Empty(t, gossip.solvencies)
	require.Empty(t, gossip.errataTxs)
	require.Empty(t, gossip.batcher.observedTxBatch)
	require.Empty(t, gossip.batcher.networkFeeBatch)
	require.Empty(t, gossip.batcher.solvencyBatch)
	require.Empty(t, gossip.batcher.errataTxBatch)
	require.Nil(t, gossip.stateInitPeers)
}

func TestActiveValidatorSetUnchangedKeepsAttestationState(t *testing.T) {
	activePeer := peer.ID("active")
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
	}
	obsKey := newObservedTxKey(tx, true, false)

	gossip := &AttestationGossip{
		activeVals: map[peer.ID]bool{activePeer: true},
		observedTxs: map[txKey]*AttestationState[*attestableObservedTx]{
			obsKey: {
				Item: &attestableObservedTx{ObservedTx: &tx, inbound: true},
			},
		},
		networkFees:     make(map[common.NetworkFee]*AttestationState[*common.NetworkFee]),
		solvencies:      make(map[common.TxID]*AttestationState[*common.Solvency]),
		errataTxs:       make(map[common.ErrataTx]*AttestationState[*common.ErrataTx]),
		observedTxsPool: NewAttestationStatePool[*attestableObservedTx](),
		networkFeesPool: NewAttestationStatePool[*common.NetworkFee](),
		solvenciesPool:  NewAttestationStatePool[*common.Solvency](),
		errataTxsPool:   NewAttestationStatePool[*common.ErrataTx](),
		batcher: &AttestationBatcher{
			observedTxBatch: []*common.AttestTx{{ObsTx: tx}},
		},
		stateInitPeers: map[peer.ID]bool{activePeer: true},
	}

	gossip.setActiveValidatorPeers(map[peer.ID]bool{activePeer: true})

	require.Len(t, gossip.observedTxs, 1)
	require.Len(t, gossip.batcher.observedTxBatch, 1)
	require.Equal(t, map[peer.ID]bool{activePeer: true}, gossip.stateInitPeers)
}
