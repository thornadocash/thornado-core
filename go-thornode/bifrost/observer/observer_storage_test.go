package observer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

func setupTestDB(t *testing.T) (*ObserverStorage, string) {
	tempDir, err := os.MkdirTemp("", "observer-storage-test")
	require.NoError(t, err)

	opts := config.LevelDBOptions{
		FilterBitsPerKey:              10,
		CompactionTableSizeMultiplier: 1,
		WriteBuffer:                   4194304,
		BlockCacheCapacity:            8388608,
		CompactOnInit:                 true,
	}

	storage, err := NewObserverStorage(tempDir, opts)
	require.NoError(t, err)
	require.NotNil(t, storage)

	return storage, tempDir
}

func cleanupTestDB(t *testing.T, storage *ObserverStorage, tempDir string) {
	err := storage.Close()
	require.NoError(t, err)
	err = os.RemoveAll(tempDir)
	require.NoError(t, err)
}

func createTestTxIn(chain common.Chain, blockHeight int64) *types.TxIn {
	return &types.TxIn{
		Chain: chain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: blockHeight,
				Tx:          "tx-data",
				Memo:        "memo",
			},
		},
	}
}

func TestNewObserverStorage(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer os.RemoveAll(tempDir)

		assert.NotNil(t, storage)
		assert.NotNil(t, storage.db)

		err := storage.Close()
		assert.NoError(t, err)
	})

	t.Run("creation failure", func(t *testing.T) {
		// Create a file where we want to create the DB to force an error
		tempDir, err := os.MkdirTemp("", "observer-storage-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create a file with the same name as the path to cause an error
		dbPath := filepath.Join(tempDir, "invalid-db")
		err = os.WriteFile(dbPath, []byte("test"), 0o600)
		require.NoError(t, err)

		opts := config.LevelDBOptions{}
		storage, err := NewObserverStorage(dbPath, opts)

		assert.Error(t, err)
		assert.Nil(t, storage)
	})
}

func TestCreateTxKey(t *testing.T) {
	storage, tempDir := setupTestDB(t)
	defer cleanupTestDB(t, storage, tempDir)

	testCases := []struct {
		name        string
		txIn        *types.TxIn
		expectedKey string
	}{
		{
			name:        "BTC mempool transaction",
			txIn:        createTestTxIn(common.BTCChain, 100),
			expectedKey: "txs:BTC:100",
		},
		{
			name:        "ETH non-mempool transaction",
			txIn:        createTestTxIn(common.ETHChain, 200),
			expectedKey: "txs:ETH:200",
		},
		{
			name: "transaction with no items",
			txIn: &types.TxIn{
				Chain:   common.BTCChain,
				MemPool: true,
				TxArray: []*types.TxInItem{},
			},
			expectedKey: "txs:BTC:0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := storage.createTxKey(tc.txIn, 0)
			assert.Equal(t, tc.expectedKey, key)
		})
	}
}

func TestGetOnDeckTxs(t *testing.T) {
	t.Run("empty storage", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		txs, err := storage.GetOnDeckTxs()
		assert.NoError(t, err)
		assert.Empty(t, txs)
	})

	t.Run("legacy storage format", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		// Manually add a legacy format record
		legacyTxs := []*types.TxIn{
			createTestTxIn(common.BTCChain, 100),
			createTestTxIn(common.ETHChain, 200),
		}
		data, err := json.Marshal(legacyTxs)
		require.NoError(t, err)

		err = storage.db.Put([]byte(LegacyOnDeckKey), data, nil)
		require.NoError(t, err)

		txs, err := storage.GetOnDeckTxs()
		assert.NoError(t, err)
		assert.Len(t, txs, 2)
		assert.Equal(t, common.BTCChain, txs[0].Chain)
		assert.Equal(t, common.ETHChain, txs[1].Chain)

		// Verify legacy transactions were migrated
		for _, tx := range legacyTxs {
			key := storage.createTxKey(tx, 0)
			data, err = storage.db.Get([]byte(key), nil)
			assert.NoError(t, err)

			var storedTx types.TxIn
			err = json.Unmarshal(data, &storedTx)
			assert.NoError(t, err)
			assert.Equal(t, tx.Chain, storedTx.Chain)
			assert.Equal(t, tx.TxArray[0].BlockHeight, storedTx.TxArray[0].BlockHeight)
		}

		// Verify migration flag was set
		val, err := storage.db.Get([]byte(LegacyMigrationHandledKey), nil)
		assert.NoError(t, err)
		assert.Equal(t, "true", string(val))
	})

	t.Run("new storage format", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		// Add transactions in new format
		tx1 := createTestTxIn(common.BTCChain, 100)
		tx2 := createTestTxIn(common.ETHChain, 200)

		err := storage.AddOrUpdateTx(tx1)
		require.NoError(t, err)
		err = storage.AddOrUpdateTx(tx2)
		require.NoError(t, err)

		// Make GetOnDeckTxs skip legacy check by setting migration flag
		err = storage.db.Put([]byte(LegacyMigrationHandledKey), []byte("true"), nil)
		require.NoError(t, err)

		txs, err := storage.GetOnDeckTxs()
		assert.NoError(t, err)
		assert.Len(t, txs, 2)

		// Check that we got both transactions (order may vary)
		foundBTC := false
		foundETH := false
		for _, tx := range txs {
			if tx.Chain.Equals(common.BTCChain) {
				foundBTC = true
			}
			if tx.Chain.Equals(common.ETHChain) {
				foundETH = true
			}
		}
		assert.True(t, foundBTC, "BTC transaction not found")
		assert.True(t, foundETH, "ETH transaction not found")
	})

	t.Run("error unmarshalling transaction", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		// Add invalid JSON data
		key := "txs:BTC:100"
		err := storage.db.Put([]byte(key), []byte("invalid json"), nil)
		require.NoError(t, err)

		// Make GetOnDeckTxs skip legacy check
		err = storage.db.Put([]byte(LegacyMigrationHandledKey), []byte("true"), nil)
		require.NoError(t, err)

		txs, err := storage.GetOnDeckTxs()
		assert.Error(t, err)
		assert.Nil(t, txs)
		assert.Contains(t, err.Error(), "fail to unmarshal ondeck tx")
	})
}

func TestAddOrUpdateTx(t *testing.T) {
	t.Run("add new transaction", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		tx := createTestTxIn(common.BTCChain, 100)
		err := storage.AddOrUpdateTx(tx)
		assert.NoError(t, err)

		// Verify it was stored
		key := storage.createTxKey(tx, 0)
		data, err := storage.db.Get([]byte(key), nil)
		assert.NoError(t, err)

		var storedTx types.TxIn
		err = json.Unmarshal(data, &storedTx)
		assert.NoError(t, err)
		assert.Equal(t, common.BTCChain, storedTx.Chain)
		assert.Equal(t, int64(100), storedTx.TxArray[0].BlockHeight)
		assert.Equal(t, "tx-data", storedTx.TxArray[0].Tx)
	})

	t.Run("update existing transaction", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		// Add initial transaction
		tx := createTestTxIn(common.BTCChain, 100)
		err := storage.AddOrUpdateTx(tx)
		assert.NoError(t, err)

		// Update the transaction
		tx.TxArray[0].Tx = "updated-tx-data"
		err = storage.AddOrUpdateTx(tx)
		assert.NoError(t, err)

		// Verify it was updated
		key := storage.createTxKey(tx, 0)
		data, err := storage.db.Get([]byte(key), nil)
		assert.NoError(t, err)

		var storedTx types.TxIn
		err = json.Unmarshal(data, &storedTx)
		assert.NoError(t, err)
		assert.Equal(t, "updated-tx-data", storedTx.TxArray[0].Tx)
	})
}

func TestRemoveTx(t *testing.T) {
	t.Run("remove existing transaction", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		// Add a transaction
		tx := createTestTxIn(common.BTCChain, 100)
		err := storage.AddOrUpdateTx(tx)
		assert.NoError(t, err)

		// Remove it
		err = storage.RemoveTx(tx, 0)
		assert.NoError(t, err)

		// Verify it's gone
		key := storage.createTxKey(tx, 0)
		_, err = storage.db.Get([]byte(key), nil)
		assert.True(t, errors.Is(err, leveldb.ErrNotFound))
	})

	t.Run("remove non-existent transaction", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer cleanupTestDB(t, storage, tempDir)

		// Try to remove a transaction that doesn't exist
		tx := createTestTxIn(common.BTCChain, 100)
		err := storage.RemoveTx(tx, 0)

		// This should not error, as leveldb.Delete doesn't error for non-existent keys
		assert.NoError(t, err)
	})
}

func TestClose(t *testing.T) {
	t.Run("close database", func(t *testing.T) {
		storage, tempDir := setupTestDB(t)
		defer os.RemoveAll(tempDir)

		err := storage.Close()
		assert.NoError(t, err)

		// Verify it's closed by trying to use it (should error)
		tx := createTestTxIn(common.BTCChain, 100)
		err = storage.AddOrUpdateTx(tx)
		assert.Error(t, err)
	})
}
