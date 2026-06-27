package observer

import (
	"testing"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
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

func TestCommittedFinalObservedTxNeedsSupermajorityBeforeDeckRemoval(t *testing.T) {
	gossip := &AttestationGossip{
		activeVals: map[peer.ID]bool{
			"node1": true,
			"node2": true,
			"node3": true,
			"node4": true,
			"node5": true,
			"node6": true,
			"node7": true,
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

	require.False(t, gossip.shouldNotifyObserverObservedTxCommitted(common.QuorumTx{
		ObsTx:        tx,
		Attestations: make([]*common.Attestation, 3),
	}))
	require.True(t, gossip.shouldNotifyObserverObservedTxCommitted(common.QuorumTx{
		ObsTx:        tx,
		Attestations: make([]*common.Attestation, 5),
	}))
}

func TestCommittedNonFinalObservedTxKeepsExistingDeckRemovalBehavior(t *testing.T) {
	gossip := &AttestationGossip{}
	tx := common.ObservedTx{
		Tx: common.Tx{
			ID:    common.TxID("ABC123"),
			Chain: common.BTCChain,
		},
		BlockHeight:    593,
		FinaliseHeight: 624,
	}

	require.True(t, gossip.shouldNotifyObserverObservedTxCommitted(common.QuorumTx{
		ObsTx:        tx,
		Attestations: make([]*common.Attestation, 1),
	}))
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
