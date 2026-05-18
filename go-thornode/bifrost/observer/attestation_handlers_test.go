package observer

import (
	"bytes"
	"sync"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func TestHandleStreamBatchedAttestations(t *testing.T) {
	agVal1, _, _, _, _, _ := setupTestGossip(t)
	agVal2, _, _, _, _, _ := setupTestGossip(t)

	numVals := 2
	agVals := []*AttestationGossip{agVal1, agVal2}
	valPrivs := make([]*secp256k1.PrivKey, numVals)
	valPubs := make([]common.PubKey, numVals)
	for i := 0; i < numVals; i++ {
		priv := agVals[i].privKey
		var ok bool
		valPrivs[i], ok = priv.(*secp256k1.PrivKey)
		require.True(t, ok, "Should be able to cast private key to secp256k1")
		bech32Pub, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, valPrivs[i].PubKey())
		require.NoError(t, err, "Should be able to convert pubkey to bech32")
		valPubs[i] = common.PubKey(bech32Pub)
	}

	// Set active vals
	agVal1.setActiveValidators(valPubs)
	agVal2.setActiveValidators(valPubs)

	val1Reader := bytes.NewBuffer(nil)
	val1Writer := &bytes.Buffer{}

	val2Reader := val1Writer
	val2Writer := val1Reader

	var sharedMu sync.Mutex

	val2Stream := &MockStream{
		reader: val2Reader,
		writer: val2Writer,
		peer:   agVal1.host.ID(),
		mu:     &sharedMu,
	}

	val1Stream := &MockStream{
		reader: val1Reader,
		writer: val1Writer,
		peer:   agVal2.host.ID(),
		mu:     &sharedMu,
	}

	// Create test data for each attestation type

	// 1. ObservedTx
	observedTx := &common.ObservedTx{
		Tx: common.Tx{
			ID: "tx-id",
		},
	}
	obsTxSignBz, err := observedTx.GetSignablePayload()
	require.NoError(t, err, "Should be able to get signable payload")

	// 2. NetworkFee
	networkFee := &common.NetworkFee{
		Height:          1,
		Chain:           common.ETHChain,
		TransactionSize: 1000,
		TransactionRate: 10,
	}
	nfSignBz, err := networkFee.GetSignablePayload()
	require.NoError(t, err, "Should be able to get signable payload")

	// 3. Solvency
	solvency := &common.Solvency{
		Height: 1,
		Chain:  common.ETHChain,
		PubKey: common.PubKey("pubkey"),
		Coins: []common.Coin{
			{
				Asset:    common.ETHAsset,
				Amount:   cosmos.NewUint(100),
				Decimals: 18,
			},
		},
	}
	// Set the solvency ID
	id, err := solvency.Hash()
	require.NoError(t, err, "Should be able to hash solvency")
	solvency.Id = id

	solSignBz, err := solvency.GetSignablePayload()
	require.NoError(t, err, "Should be able to get signable payload")

	// 4. Errata
	errata := &common.ErrataTx{
		Chain: common.ETHChain,
		Id:    "tx-id",
	}
	errSignBz, err := errata.GetSignablePayload()
	require.NoError(t, err, "Should be able to get signable payload")

	// Create signatures for Val1
	val1ObsTxSig, err := valPrivs[0].Sign(obsTxSignBz)
	require.NoError(t, err, "Should be able to sign payload")
	val1NFSig, err := valPrivs[0].Sign(nfSignBz)
	require.NoError(t, err, "Should be able to sign payload")
	val1SolvencySig, err := valPrivs[0].Sign(solSignBz)
	require.NoError(t, err, "Should be able to sign payload")
	val1ErrataSig, err := valPrivs[0].Sign(errSignBz)
	require.NoError(t, err, "Should be able to sign payload")

	// Create attestations for Val1
	val1ObsTxAttestation := &common.Attestation{
		PubKey:    valPrivs[0].PubKey().Bytes(),
		Signature: val1ObsTxSig,
	}
	val1NFAttestation := &common.Attestation{
		PubKey:    valPrivs[0].PubKey().Bytes(),
		Signature: val1NFSig,
	}
	val1SolvencyAttestation := &common.Attestation{
		PubKey:    valPrivs[0].PubKey().Bytes(),
		Signature: val1SolvencySig,
	}
	val1ErrataAttestation := &common.Attestation{
		PubKey:    valPrivs[0].PubKey().Bytes(),
		Signature: val1ErrataSig,
	}

	// Create attestation messages for Val1
	val1AttestTx := &common.AttestTx{
		ObsTx:       *observedTx,
		Attestation: val1ObsTxAttestation,
	}
	val1AttestNF := &common.AttestNetworkFee{
		NetworkFee:  networkFee,
		Attestation: val1NFAttestation,
	}
	val1AttestSolvency := &common.AttestSolvency{
		Solvency:    solvency,
		Attestation: val1SolvencyAttestation,
	}
	val1AttestErrata := &common.AttestErrataTx{
		ErrataTx:    errata,
		Attestation: val1ErrataAttestation,
	}

	// Create a batch with all attestation types
	batch := common.AttestationBatch{
		AttestTxs:         []*common.AttestTx{val1AttestTx},
		AttestNetworkFees: []*common.AttestNetworkFee{val1AttestNF},
		AttestSolvencies:  []*common.AttestSolvency{val1AttestSolvency},
		AttestErrataTxs:   []*common.AttestErrataTx{val1AttestErrata},
	}

	batchBz, err := batch.Marshal()
	require.NoError(t, err, "Should be able to marshal attestation batch")

	// Val1 sends batched attestations to Val2
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Write batch data to stream
		writeErr := p2p.WriteStreamWithBuffer(batchBz, val1Stream)
		require.NoError(t, writeErr, "Should be able to write to stream")
	}()

	go func() {
		defer wg.Done()
		// Handle batched attestations
		agVal2.handleStreamBatchedAttestations(val2Stream)
	}()

	wg.Wait()

	// Check that Val2 has received and processed all attestations
	agVal2.mu.Lock()

	// Check ObservedTx
	require.Len(t, agVal2.observedTxs, 1, "Should have 1 observed tx")
	for _, tx := range agVal2.observedTxs {
		require.Len(t, tx.attestations, 1, "Should have 1 attestation")
		require.True(t, bytes.Equal(tx.attestations[0].attestation.PubKey, val1ObsTxAttestation.PubKey))
		require.True(t, bytes.Equal(tx.attestations[0].attestation.Signature, val1ObsTxAttestation.Signature))
	}

	// Check NetworkFee
	require.Len(t, agVal2.networkFees, 1, "Should have 1 network fee")
	for _, tx := range agVal2.networkFees {
		require.Len(t, tx.attestations, 1, "Should have 1 attestation")
		require.True(t, bytes.Equal(tx.attestations[0].attestation.PubKey, val1NFAttestation.PubKey))
		require.True(t, bytes.Equal(tx.attestations[0].attestation.Signature, val1NFAttestation.Signature))
	}

	// Check Solvency
	require.Len(t, agVal2.solvencies, 1, "Should have 1 solvency")
	for _, tx := range agVal2.solvencies {
		require.Len(t, tx.attestations, 1, "Should have 1 attestation")
		require.True(t, bytes.Equal(tx.attestations[0].attestation.PubKey, val1SolvencyAttestation.PubKey))
		require.True(t, bytes.Equal(tx.attestations[0].attestation.Signature, val1SolvencyAttestation.Signature))
	}

	// Check ErrataTx
	require.Len(t, agVal2.errataTxs, 1, "Should have 1 errata tx")
	for _, tx := range agVal2.errataTxs {
		require.Len(t, tx.attestations, 1, "Should have 1 attestation")
		require.True(t, bytes.Equal(tx.attestations[0].attestation.PubKey, val1ErrataAttestation.PubKey))
		require.True(t, bytes.Equal(tx.attestations[0].attestation.Signature, val1ErrataAttestation.Signature))
	}

	agVal2.mu.Unlock()

	// Test batch size limits
	maxBatchSize := int(agVal1.batcher.getMaxBatchSize()) //nolint:staticcheck

	// Create oversized batches for testing limits
	oversizedObsTxBatch := make([]*common.AttestTx, maxBatchSize+5)
	for i := 0; i < maxBatchSize+5; i++ {
		oversizedObsTxBatch[i] = val1AttestTx
	}

	oversizedNetworkFeeBatch := make([]*common.AttestNetworkFee, maxBatchSize+5)
	for i := 0; i < maxBatchSize+5; i++ {
		oversizedNetworkFeeBatch[i] = val1AttestNF
	}

	oversizedSolvencyBatch := make([]*common.AttestSolvency, maxBatchSize+5)
	for i := 0; i < maxBatchSize+5; i++ {
		oversizedSolvencyBatch[i] = val1AttestSolvency
	}

	oversizedErrataBatch := make([]*common.AttestErrataTx, maxBatchSize+5)
	for i := 0; i < maxBatchSize+5; i++ {
		oversizedErrataBatch[i] = val1AttestErrata
	}

	oversizedBatch := common.AttestationBatch{
		AttestTxs:         oversizedObsTxBatch,
		AttestNetworkFees: oversizedNetworkFeeBatch,
		AttestSolvencies:  oversizedSolvencyBatch,
		AttestErrataTxs:   oversizedErrataBatch,
	}

	oversizedBatchBz, err := oversizedBatch.Marshal()
	require.NoError(t, err, "Should be able to marshal oversized batch")

	// Reset Val2's data
	agVal2.observedTxs = make(map[txKey]*AttestationState[*common.ObservedTx])
	agVal2.networkFees = make(map[common.NetworkFee]*AttestationState[*common.NetworkFee])
	agVal2.solvencies = make(map[common.TxID]*AttestationState[*common.Solvency])
	agVal2.errataTxs = make(map[common.ErrataTx]*AttestationState[*common.ErrataTx])

	// Test with oversized batch
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Write oversized batch to stream
		err := p2p.WriteStreamWithBuffer(oversizedBatchBz, val1Stream)
		require.NoError(t, err, "Should be able to write to stream")
	}()

	go func() {
		defer wg.Done()
		// Handle oversized batch
		agVal2.handleStreamBatchedAttestations(val2Stream)
	}()

	wg.Wait()

	// Check that Val2 processed only up to maxBatchSize items
	agVal2.mu.Lock()
	require.Len(t, agVal2.observedTxs, 1, "Should process only up to max batch size")
	require.Len(t, agVal2.networkFees, 1, "Should process only up to max batch size")
	require.Len(t, agVal2.solvencies, 1, "Should process only up to max batch size")
	require.Len(t, agVal2.errataTxs, 1, "Should process only up to max batch size")
	agVal2.mu.Unlock()
}
