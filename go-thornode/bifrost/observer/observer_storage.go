package observer

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"

	"gitlab.com/thorchain/thornode/v3/bifrost/db"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/config"
)

// ObserverStorage save the ondeck tx in item to key value store, in case bifrost restart
type ObserverStorage struct {
	db *leveldb.DB
}

const (
	LegacyOnDeckKey           = "ondeck-tx"
	LegacyMigrationHandledKey = "ondeck-migration-handled"
	OnDeckTxKeyPrefix         = "txs:"
)

// NewObserverStorage create a new instance of LevelDBScannerStorage
func NewObserverStorage(path string, opts config.LevelDBOptions) (*ObserverStorage, error) {
	ldb, err := db.NewLevelDB(path, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create observer storage: %w", err)
	}

	return &ObserverStorage{db: ldb}, nil
}

// createTxKey creates a unique key for a TxIn based on prefix, chain, mempool, blockheight
func (s *ObserverStorage) createTxKey(txIn *types.TxIn, finalizeHeight int64) string {
	if finalizeHeight == 0 && len(txIn.TxArray) > 0 {
		finalizeHeight = txIn.TxArray[0].BlockHeight + txIn.ConfirmationRequired
	}

	return fmt.Sprintf("%s%s:%d",
		OnDeckTxKeyPrefix,
		txIn.Chain.String(),
		finalizeHeight)
}

func (s *ObserverStorage) MigrateLegacy() ([]*types.TxIn, error) {
	_, err := s.db.Get([]byte(LegacyMigrationHandledKey), nil)
	if !errors.Is(err, leveldb.ErrNotFound) {
		return nil, nil
	}
	buf, err := s.db.Get([]byte(LegacyOnDeckKey), nil)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("fail to get ondeck tx from key value store: %w", err)
	}
	var result []*types.TxIn
	if err = json.Unmarshal(buf, &result); err != nil {
		return nil, fmt.Errorf("fail to unmarshal ondeck tx: %w", err)
	}

	// Migrate the ondeck tx to the new format
	for _, txIn := range result {
		key := s.createTxKey(txIn, 0)
		data, err := json.Marshal(txIn)
		if err != nil {
			return nil, fmt.Errorf("fail to marshal ondeck tx to json: %w", err)
		}
		if err := s.db.Put([]byte(key), data, nil); err != nil {
			return nil, fmt.Errorf("fail to put ondeck tx to key value store: %w", err)
		}
	}

	if err := s.db.Delete([]byte(LegacyOnDeckKey), nil); err != nil {
		return nil, fmt.Errorf("fail to delete legacy ondeck tx from key value store: %w", err)
	}

	if err := s.db.Put([]byte(LegacyMigrationHandledKey), []byte("true"), nil); err != nil {
		return nil, fmt.Errorf("fail to put migration handled flag to key value store: %w", err)
	}

	return result, nil
}

// GetOnDeckTxs retrieve the ondeck tx from key value store
func (s *ObserverStorage) GetOnDeckTxs() ([]*types.TxIn, error) {
	// Check if the legacy migration has been handled
	legacyTxs, err := s.MigrateLegacy()
	if err != nil {
		return nil, fmt.Errorf("fail to migrate legacy ondeck tx: %w", err)
	}

	// If legacy txs exist, return them
	if len(legacyTxs) > 0 {
		return legacyTxs, nil
	}

	// Use a DB iterator to get all keys with the prefix
	iter := s.db.NewIterator(util.BytesPrefix([]byte(OnDeckTxKeyPrefix)), nil)
	defer iter.Release()

	var result []*types.TxIn
	for iter.Next() {
		var txIn types.TxIn
		if err := json.Unmarshal(iter.Value(), &txIn); err != nil {
			return nil, fmt.Errorf("fail to unmarshal ondeck tx: %w", err)
		}
		result = append(result, &txIn)
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("error iterating through ondeck txs: %w", err)
	}

	return result, nil
}

// AddOrUpdateTx adds or updates a single TxIn in storage
func (s *ObserverStorage) AddOrUpdateTx(txIn *types.TxIn) error {
	key := s.createTxKey(txIn, 0)
	data, err := json.Marshal(txIn)
	if err != nil {
		return fmt.Errorf("fail to marshal ondeck tx to json: %w", err)
	}
	return s.db.Put([]byte(key), data, nil)
}

// RemoveTx removes a single TxIn from storage
func (s *ObserverStorage) RemoveTx(txIn *types.TxIn, finalizeHeight int64) error {
	key := s.createTxKey(txIn, finalizeHeight)
	return s.db.Delete([]byte(key), nil)
}

// RemoveAllTxs removes all TxIn from storage
func (s *ObserverStorage) RemoveAllTxs() error {
	iter := s.db.NewIterator(util.BytesPrefix([]byte(OnDeckTxKeyPrefix)), nil)
	defer iter.Release()

	for iter.Next() {
		if err := s.db.Delete(iter.Key(), nil); err != nil {
			return fmt.Errorf("fail to delete ondeck tx from key value store: %w", err)
		}
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("error iterating through ondeck txs: %w", err)
	}

	return nil
}

func (s *ObserverStorage) Close() error {
	return s.db.Close()
}
