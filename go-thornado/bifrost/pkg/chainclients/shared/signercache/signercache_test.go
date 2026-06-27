package signercache

import (
	"fmt"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	. "gopkg.in/check.v1"
)

// mockStorageAccessor is a mock StorageAccessor that can return errors on demand
type mockStorageAccessor struct {
	setSignedErr             error
	setSignedWithExpiryErr   error
	hasSignedResult          bool
	hasSignedExpiredResult   bool
	removeSignedErr          error
	setTransactionHashMapErr error
	getSignedTxHashResult    string
	getSignedTxHashOk        bool
	setLatestRecordedTxErr   error
	getLatestRecordedTxVal   string
	getLatestRecordedTxErr   error
}

func (m *mockStorageAccessor) SetSigned(hash string) error {
	return m.setSignedErr
}

func (m *mockStorageAccessor) SetSignedWithExpiry(hash string, expiryUnixMs int64, transactionHash string) error {
	return m.setSignedWithExpiryErr
}

func (m *mockStorageAccessor) HasSigned(hash string) bool {
	return m.hasSignedResult
}

func (m *mockStorageAccessor) HasSignedWithExpiry(hash string) (signed, expired bool) {
	return m.hasSignedResult, m.hasSignedExpiredResult
}

func (m *mockStorageAccessor) RemoveSigned(transactionHash string) error {
	return m.removeSignedErr
}

func (m *mockStorageAccessor) SetTransactionHashMap(txOutItemHash, transactionHash string) error {
	return m.setTransactionHashMapErr
}

func (m *mockStorageAccessor) GetSignedTxHash(txOutItemHash string) (string, bool) {
	return m.getSignedTxHashResult, m.getSignedTxHashOk
}

func (m *mockStorageAccessor) SetLatestRecordedTx(vaultKey, transactionHash string) error {
	return m.setLatestRecordedTxErr
}

func (m *mockStorageAccessor) GetLatestRecordedTx(vaultKey string) (string, error) {
	return m.getLatestRecordedTxVal, m.getLatestRecordedTxErr
}

func TestPackage(t *testing.T) { TestingT(t) }

type SignerCacheSuite struct{}

var _ = Suite(&SignerCacheSuite{})

func newTestDB(c *C) *leveldb.DB {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	c.Assert(err, IsNil)
	return db
}

// -------------------------------------------------------------------------------------
// CacheStore tests
// -------------------------------------------------------------------------------------

func (s *SignerCacheSuite) TestNewCacheStore(c *C) {
	db := newTestDB(c)
	defer db.Close()

	store := NewCacheStore(db)
	c.Assert(store, NotNil)
	c.Assert(store.db, NotNil)
}

func (s *SignerCacheSuite) TestCacheStoreSetAndHasSigned(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	hash := "abc123"

	// Initially not signed
	c.Assert(store.HasSigned(hash), Equals, false)

	// Set as signed
	err := store.SetSigned(hash)
	c.Assert(err, IsNil)

	// Now should be signed
	c.Assert(store.HasSigned(hash), Equals, true)
}

func (s *SignerCacheSuite) TestCacheStoreSetTransactionHashMap(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	txOutItemHash := "txout-hash-1"
	transactionHash := "tx-hash-1"

	err := store.SetTransactionHashMap(txOutItemHash, transactionHash)
	c.Assert(err, IsNil)

	// Verify the mapping was stored by reading it back
	mapKey := store.getMapKey(transactionHash)
	value, err := db.Get([]byte(mapKey), nil)
	c.Assert(err, IsNil)
	c.Assert(string(value), Equals, txOutItemHash)
}

func (s *SignerCacheSuite) TestCacheStoreRemoveSigned(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	txOutItemHash := "txout-hash-1"
	transactionHash := "tx-hash-1"

	// Set signed and create mapping
	err := store.SetSigned(txOutItemHash)
	c.Assert(err, IsNil)
	err = store.SetTransactionHashMap(txOutItemHash, transactionHash)
	c.Assert(err, IsNil)

	// Verify it's signed
	c.Assert(store.HasSigned(txOutItemHash), Equals, true)

	// Remove signed via transaction hash
	err = store.RemoveSigned(transactionHash)
	c.Assert(err, IsNil)

	// Now should no longer be signed
	c.Assert(store.HasSigned(txOutItemHash), Equals, false)
}

func (s *SignerCacheSuite) TestCacheStoreRemoveSignedNotFound(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	// Remove non-existent transaction hash should succeed (not found is OK)
	err := store.RemoveSigned("non-existent-hash")
	c.Assert(err, IsNil)
}

func (s *SignerCacheSuite) TestCacheStoreSetAndGetLatestRecordedTx(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	vaultKey := "vault-pubkey-1"
	transactionHash := "tx-hash-abc"

	// Set latest recorded tx
	err := store.SetLatestRecordedTx(vaultKey, transactionHash)
	c.Assert(err, IsNil)

	// Get latest recorded tx
	result, err := store.GetLatestRecordedTx(vaultKey)
	c.Assert(err, IsNil)
	c.Assert(result, Equals, transactionHash)
}

func (s *SignerCacheSuite) TestCacheStoreGetLatestRecordedTxNotFound(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	// Get non-existent vault key
	result, err := store.GetLatestRecordedTx("non-existent")
	c.Assert(err, NotNil)
	c.Assert(result, Equals, "")
}

func (s *SignerCacheSuite) TestCacheStoreClose(c *C) {
	db := newTestDB(c)
	store := NewCacheStore(db)

	err := store.Close()
	c.Assert(err, IsNil)
}

func (s *SignerCacheSuite) TestCacheStoreKeyFormats(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	// Verify key prefix formats
	c.Assert(store.getSignedKey("hash1"), Equals, "signed-v6-hash1")
	c.Assert(store.getMapKey("txhash1"), Equals, "tx-map-v6-txhash1")
	c.Assert(store.getVaultKey("vaultkey1"), Equals, "vault-v6-vaultkey1")
}

func (s *SignerCacheSuite) TestCacheStoreSetSignedWithExpiryNotExpired(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	hash := "abc123"
	expiry := time.Now().Add(time.Hour).UnixMilli()
	c.Assert(store.SetSignedWithExpiry(hash, expiry, "tx-abc"), IsNil)

	c.Assert(store.HasSigned(hash), Equals, true)
	signed, expired := store.HasSignedWithExpiry(hash)
	c.Assert(signed, Equals, true)
	c.Assert(expired, Equals, false)

	// Tx hash is retrievable from the same entry.
	tx, ok := store.GetSignedTxHash(hash)
	c.Assert(ok, Equals, true)
	c.Assert(tx, Equals, "tx-abc")
}

func (s *SignerCacheSuite) TestCacheStoreSetSignedWithExpiryPastExpiryStillSigned(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	hash := "abc123"
	// Expiry in the past. HasSigned still returns true; HasSignedWithExpiry reports
	// expired so callers can gate chain-side verification. The entry is not
	// removed on read — a re-sign could double-spend if the recorded broadcast
	// landed but observation consensus hasn't caught up.
	expiry := time.Now().Add(-time.Minute).UnixMilli()
	c.Assert(store.SetSignedWithExpiry(hash, expiry, "tx-abc"), IsNil)

	c.Assert(store.HasSigned(hash), Equals, true)
	signed, expired := store.HasSignedWithExpiry(hash)
	c.Assert(signed, Equals, true)
	c.Assert(expired, Equals, true)

	tx, ok := store.GetSignedTxHash(hash)
	c.Assert(ok, Equals, true)
	c.Assert(tx, Equals, "tx-abc")

	// Subsequent check returns the same — nothing was cleaned up.
	c.Assert(store.HasSigned(hash), Equals, true)
}

func (s *SignerCacheSuite) TestCacheStoreLegacySetSignedNeverExpires(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	hash := "legacy"
	c.Assert(store.SetSigned(hash), IsNil)
	c.Assert(store.HasSigned(hash), Equals, true)

	// Legacy entries never report as expired.
	signed, expired := store.HasSignedWithExpiry(hash)
	c.Assert(signed, Equals, true)
	c.Assert(expired, Equals, false)

	// Legacy entries do not carry a tx hash.
	_, ok := store.GetSignedTxHash(hash)
	c.Assert(ok, Equals, false)
}

func (s *SignerCacheSuite) TestCacheStoreHasSignedWithExpiryAbsent(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	signed, expired := store.HasSignedWithExpiry("missing")
	c.Assert(signed, Equals, false)
	c.Assert(expired, Equals, false)
}

func (s *SignerCacheSuite) TestCacheStoreGetSignedTxHash(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	// Absent entry.
	_, ok := store.GetSignedTxHash("unknown")
	c.Assert(ok, Equals, false)

	// An expiry-only entry (empty tx hash) returns false for GetSignedTxHash.
	c.Assert(store.SetSignedWithExpiry("empty-tx", time.Now().Add(time.Hour).UnixMilli(), ""), IsNil)
	_, ok = store.GetSignedTxHash("empty-tx")
	c.Assert(ok, Equals, false)

	// An expiry entry with a tx hash returns the hash.
	c.Assert(store.SetSignedWithExpiry("item-1", time.Now().Add(time.Hour).UnixMilli(), "tx-1"), IsNil)
	tx, ok := store.GetSignedTxHash("item-1")
	c.Assert(ok, Equals, true)
	c.Assert(tx, Equals, "tx-1")
}

func (s *SignerCacheSuite) TestCacheStoreRemoveSignedClearsSignedFlag(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	txOutItemHash := "txout-hash-1"
	transactionHash := "tx-hash-1"

	c.Assert(store.SetSignedWithExpiry(txOutItemHash, time.Now().Add(time.Hour).UnixMilli(), transactionHash), IsNil)
	c.Assert(store.SetTransactionHashMap(txOutItemHash, transactionHash), IsNil)
	c.Assert(store.HasSigned(txOutItemHash), Equals, true)
	sig, ok := store.GetSignedTxHash(txOutItemHash)
	c.Assert(ok, Equals, true)
	c.Assert(sig, Equals, transactionHash)

	c.Assert(store.RemoveSigned(transactionHash), IsNil)

	c.Assert(store.HasSigned(txOutItemHash), Equals, false)
	_, ok = store.GetSignedTxHash(txOutItemHash)
	c.Assert(ok, Equals, false)
}

func (s *SignerCacheSuite) TestCacheStoreHasSignedUnexpectedValueLength(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	// Write a value of unexpected length directly.
	c.Assert(db.Put([]byte(store.getSignedKey("bad")), []byte{1, 2, 3}, nil), IsNil)

	// Conservatively treat unrecognized formats as signed-and-not-expired so the
	// SOL client respects the cache without trying to verify on chain (nothing to
	// verify) and without re-broadcasting. Entry must remain in storage.
	c.Assert(store.HasSigned("bad"), Equals, true)
	signed, expired := store.HasSignedWithExpiry("bad")
	c.Assert(signed, Equals, true)
	c.Assert(expired, Equals, false)
	_, err := db.Get([]byte(store.getSignedKey("bad")), nil)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// CacheManager tests
// -------------------------------------------------------------------------------------

func (s *SignerCacheSuite) TestNewSignerCacheManagerNilDB(c *C) {
	mgr, err := NewSignerCacheManager(nil)
	c.Assert(err, NotNil)
	c.Assert(mgr, IsNil)
	c.Assert(err.Error(), Equals, "db parameter is nil")
}

func (s *SignerCacheSuite) TestNewSignerCacheManagerSuccess(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)
	c.Assert(mgr, NotNil)
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedAndHasSigned(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	txOutItemHash := "txout-hash-1"
	vaultKey := "vault-key-1"
	transactionHash := "tx-hash-1"

	// Initially not signed
	c.Assert(mgr.HasSigned(txOutItemHash), Equals, false)

	// Set signed
	err = mgr.SetSigned(txOutItemHash, vaultKey, transactionHash)
	c.Assert(err, IsNil)

	// Now should be signed
	c.Assert(mgr.HasSigned(txOutItemHash), Equals, true)
	sig, ok := mgr.GetSignedTxHash(txOutItemHash)
	c.Assert(ok, Equals, true)
	c.Assert(sig, Equals, transactionHash)
}

func (s *SignerCacheSuite) TestCacheManagerGetLatestRecordedTx(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	vaultKey := "vault-key-1"
	transactionHash := "tx-hash-1"

	// Set signed to also set vault latest recorded tx
	err = mgr.SetSigned("txout-hash-1", vaultKey, transactionHash)
	c.Assert(err, IsNil)

	// Get latest recorded tx
	result, err := mgr.GetLatestRecordedTx(vaultKey)
	c.Assert(err, IsNil)
	c.Assert(result, Equals, transactionHash)
}

func (s *SignerCacheSuite) TestCacheManagerRemoveSigned(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	txOutItemHash := "txout-hash-1"
	vaultKey := "vault-key-1"
	transactionHash := "tx-hash-1"

	// Set signed
	err = mgr.SetSigned(txOutItemHash, vaultKey, transactionHash)
	c.Assert(err, IsNil)
	c.Assert(mgr.HasSigned(txOutItemHash), Equals, true)

	// Remove signed
	mgr.RemoveSigned(transactionHash)

	// Should no longer be signed
	c.Assert(mgr.HasSigned(txOutItemHash), Equals, false)
}

func (s *SignerCacheSuite) TestCacheManagerRemoveSignedNonExistent(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	// Remove non-existent - should not panic or error
	mgr.RemoveSigned("non-existent-hash")
}

func (s *SignerCacheSuite) TestCacheManagerMultipleTxOutItems(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	// Sign multiple txs
	err = mgr.SetSigned("txout-1", "vault-1", "tx-1")
	c.Assert(err, IsNil)
	err = mgr.SetSigned("txout-2", "vault-1", "tx-2")
	c.Assert(err, IsNil)
	err = mgr.SetSigned("txout-3", "vault-2", "tx-3")
	c.Assert(err, IsNil)

	// All should be signed
	c.Assert(mgr.HasSigned("txout-1"), Equals, true)
	c.Assert(mgr.HasSigned("txout-2"), Equals, true)
	c.Assert(mgr.HasSigned("txout-3"), Equals, true)

	// Latest recorded tx for vault-1 should be tx-2
	result, err := mgr.GetLatestRecordedTx("vault-1")
	c.Assert(err, IsNil)
	c.Assert(result, Equals, "tx-2")

	// Latest recorded tx for vault-2 should be tx-3
	result, err = mgr.GetLatestRecordedTx("vault-2")
	c.Assert(err, IsNil)
	c.Assert(result, Equals, "tx-3")

	// Remove txout-2
	mgr.RemoveSigned("tx-2")
	c.Assert(mgr.HasSigned("txout-2"), Equals, false)
	// Others still signed
	c.Assert(mgr.HasSigned("txout-1"), Equals, true)
	c.Assert(mgr.HasSigned("txout-3"), Equals, true)
}

// -------------------------------------------------------------------------------------
// CacheManager error path tests (using mock StorageAccessor)
// -------------------------------------------------------------------------------------

func (s *SignerCacheSuite) TestCacheManagerSetSignedErrorOnSetSigned(c *C) {
	mock := &mockStorageAccessor{
		setSignedErr: fmt.Errorf("db write error"),
	}
	mgr := &CacheManager{
		storageAccessor: mock,
	}

	err := mgr.SetSigned("txout-hash", "vault-key", "tx-hash")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "fail to set signed cache db write error")
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedErrorOnSetTransactionHashMap(c *C) {
	mock := &mockStorageAccessor{
		setTransactionHashMapErr: fmt.Errorf("map write error"),
	}
	mgr := &CacheManager{
		storageAccessor: mock,
	}

	err := mgr.SetSigned("txout-hash", "vault-key", "tx-hash")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "map write error")
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedErrorOnSetLatestRecordedTx(c *C) {
	mock := &mockStorageAccessor{
		setLatestRecordedTxErr: fmt.Errorf("vault write error"),
	}
	mgr := &CacheManager{
		storageAccessor: mock,
	}

	err := mgr.SetSigned("txout-hash", "vault-key", "tx-hash")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "vault write error")
}

func (s *SignerCacheSuite) TestCacheManagerRemoveSignedError(c *C) {
	mock := &mockStorageAccessor{
		removeSignedErr: fmt.Errorf("remove error"),
	}
	mgr := &CacheManager{
		storageAccessor: mock,
	}

	// RemoveSigned logs the error but doesn't return it - should not panic
	mgr.RemoveSigned("tx-hash")
}

func (s *SignerCacheSuite) TestCacheStoreRemoveSignedDBError(c *C) {
	db := newTestDB(c)
	store := NewCacheStore(db)

	// Set a transaction hash map entry
	err := store.SetTransactionHashMap("txout-hash-1", "tx-hash-1")
	c.Assert(err, IsNil)

	// Close the DB to force errors on subsequent operations
	db.Close()

	// RemoveSigned should return an error when trying to get map key (db is closed)
	err = store.RemoveSigned("tx-hash-1")
	c.Assert(err, NotNil)
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedWithExpiry(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	txOutItemHash := "txout-hash-1"
	vaultKey := "vault-key-1"
	transactionHash := "tx-hash-1"

	c.Assert(mgr.HasSigned(txOutItemHash), Equals, false)
	_, ok := mgr.GetSignedTxHash(txOutItemHash)
	c.Assert(ok, Equals, false)

	expiry := time.Now().Add(time.Hour).UnixMilli()
	c.Assert(mgr.SetSignedWithExpiry(txOutItemHash, vaultKey, transactionHash, expiry), IsNil)

	c.Assert(mgr.HasSigned(txOutItemHash), Equals, true)

	sig, ok := mgr.GetSignedTxHash(txOutItemHash)
	c.Assert(ok, Equals, true)
	c.Assert(sig, Equals, transactionHash)

	latest, err := mgr.GetLatestRecordedTx(vaultKey)
	c.Assert(err, IsNil)
	c.Assert(latest, Equals, transactionHash)
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedWithExpiryPastExpiryStillSigned(c *C) {
	db := newTestDB(c)
	defer db.Close()

	mgr, err := NewSignerCacheManager(db)
	c.Assert(err, IsNil)

	hash := "txout-hash-1"
	expired := time.Now().Add(-time.Minute).UnixMilli()
	c.Assert(mgr.SetSignedWithExpiry(hash, "vault", "tx", expired), IsNil)

	// Past-expiry entries are preserved so chain-side verification can decide
	// whether to keep or remove them. HasSignedWithExpiry surfaces the expiry
	// status for callers that need to gate on it.
	c.Assert(mgr.HasSigned(hash), Equals, true)
	signed, isExpired := mgr.HasSignedWithExpiry(hash)
	c.Assert(signed, Equals, true)
	c.Assert(isExpired, Equals, true)

	tx, ok := mgr.GetSignedTxHash(hash)
	c.Assert(ok, Equals, true)
	c.Assert(tx, Equals, "tx")
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedWithExpiryErrorOnSetSigned(c *C) {
	mock := &mockStorageAccessor{
		setSignedWithExpiryErr: fmt.Errorf("db write error"),
	}
	mgr := &CacheManager{storageAccessor: mock}

	err := mgr.SetSignedWithExpiry("txout-hash", "vault-key", "tx-hash", 1)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "fail to set signed cache with expiry: db write error")
}

func (s *SignerCacheSuite) TestCacheManagerSetSignedWithExpiryErrorOnTxHashMap(c *C) {
	mock := &mockStorageAccessor{
		setTransactionHashMapErr: fmt.Errorf("reverse index error"),
	}
	mgr := &CacheManager{storageAccessor: mock}

	err := mgr.SetSignedWithExpiry("txout-hash", "vault-key", "tx-hash", 1)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "reverse index error")
}

func (s *SignerCacheSuite) TestCacheManagerGetSignedTxHashDelegates(c *C) {
	mock := &mockStorageAccessor{
		getSignedTxHashResult: "sig-xyz",
		getSignedTxHashOk:     true,
	}
	mgr := &CacheManager{storageAccessor: mock}

	sig, ok := mgr.GetSignedTxHash("txout-hash")
	c.Assert(ok, Equals, true)
	c.Assert(sig, Equals, "sig-xyz")
}

func (s *SignerCacheSuite) TestCacheStoreSetLatestRecordedTxOverwrite(c *C) {
	db := newTestDB(c)
	defer db.Close()
	store := NewCacheStore(db)

	vaultKey := "vault-key-1"

	// Set first tx
	err := store.SetLatestRecordedTx(vaultKey, "tx-1")
	c.Assert(err, IsNil)
	result, err := store.GetLatestRecordedTx(vaultKey)
	c.Assert(err, IsNil)
	c.Assert(result, Equals, "tx-1")

	// Overwrite with second tx
	err = store.SetLatestRecordedTx(vaultKey, "tx-2")
	c.Assert(err, IsNil)
	result, err = store.GetLatestRecordedTx(vaultKey)
	c.Assert(err, IsNil)
	c.Assert(result, Equals, "tx-2")
}
