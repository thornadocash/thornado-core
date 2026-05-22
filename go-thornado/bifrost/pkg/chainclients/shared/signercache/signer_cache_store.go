package signercache

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	signedCachePrefix = "signed-v6-"
	txMapPrefix       = "tx-map-v6-"
	vaultCachePrefix  = "vault-v6-"
)

// signedExpiryBytes is the length of the expiry-timestamp prefix written by
// SetSignedWithExpiry: an 8-byte big-endian int64 Unix-millisecond value. The
// remainder of the value (if any) is the broadcast transaction hash as raw bytes.
// Legacy values written by SetSigned are a single byte {1}. Value length
// discriminates the formats:
//
//	len == 1        → legacy flag
//	len >= 8        → expiry + optional tx hash
//	anything else   → unrecognized (treated conservatively as signed, not expired)
//
// HasSignedWithExpiry surfaces the expiry so callers (e.g. the Solana client) can
// gate chain-side verification on it. Cache entries are never invalidated on read
// — a past-expiry entry may still correspond to a broadcast that did land but
// whose observation hasn't reached thornado consensus. Invalidation happens
// explicitly via RemoveSigned, after the caller confirms the recorded broadcast
// is absent from the chain.
const signedExpiryBytes = 8

// CacheStore manage the key value store used to store what tx out items have been signed before
type CacheStore struct {
	logger zerolog.Logger
	db     *leveldb.DB
}

// NewCacheStore create a new instance of CacheStore
func NewCacheStore(db *leveldb.DB) *CacheStore {
	return &CacheStore{
		db:     db,
		logger: log.With().Str("module", "signer-cache").Logger(),
	}
}

// SetSigned records a tx out item as signed with no expiry. Used by chains that rely
// on their own on-chain observation or unstuck routine to clear the cache.
func (s *CacheStore) SetSigned(hash string) error {
	key := s.getSignedKey(hash)
	s.logger.Debug().Msgf("key:%s set to signed", key)
	return s.db.Put([]byte(key), []byte{1}, nil)
}

// SetSignedWithExpiry records a tx out item as signed with an absolute expiry and
// the broadcast transaction hash. The encoded value is 8 bytes big-endian int64
// Unix-ms expiry followed by the tx hash bytes. HasSignedWithExpiry surfaces the
// expiry status; GetSignedTxHash surfaces the tx hash. The entry is never removed
// on read — callers must explicitly RemoveSigned after verifying the recorded
// broadcast is absent from the chain, otherwise a lagging observation can cause a
// double-spend.
func (s *CacheStore) SetSignedWithExpiry(hash string, expiryUnixMs int64, transactionHash string) error {
	key := s.getSignedKey(hash)
	buf := make([]byte, signedExpiryBytes+len(transactionHash))
	binary.BigEndian.PutUint64(buf[:signedExpiryBytes], uint64(expiryUnixMs))
	copy(buf[signedExpiryBytes:], transactionHash)
	s.logger.Debug().Int64("expiry_unix_ms", expiryUnixMs).Msgf("key:%s set to signed with expiry", key)
	return s.db.Put([]byte(key), buf, nil)
}

func (s *CacheStore) getSignedKey(hash string) string {
	return fmt.Sprintf("%s%s", signedCachePrefix, hash)
}

func (s *CacheStore) getMapKey(txHash string) string {
	return fmt.Sprintf("%s%s", txMapPrefix, txHash)
}

func (s *CacheStore) getVaultKey(vaultKey string) string {
	return fmt.Sprintf("%s%s", vaultCachePrefix, vaultKey)
}

// HasSigned returns true if a signed-cache entry exists for the given hash.
// Use HasSignedWithExpiry if you also need to know whether the entry is past its
// expiry timestamp.
func (s *CacheStore) HasSigned(hash string) bool {
	signed, _ := s.HasSignedWithExpiry(hash)
	return signed
}

// HasSignedWithExpiry returns whether a signed-cache entry exists for the given
// hash, and whether its expiry timestamp has passed. Legacy entries (written by
// SetSigned without an expiry) report expired=false. Unknown formats report
// signed=true, expired=false — treated conservatively so we never re-sign on the
// basis of a value we don't understand.
//
// Entries are never removed on read. Invalidation happens explicitly via
// RemoveSigned, and callers that re-sign based on expiry must first verify the
// recorded broadcast is absent from the chain: deleting on expiry alone can
// double-spend when thornado observation consensus lags actual on-chain inclusion.
func (s *CacheStore) HasSignedWithExpiry(hash string) (signed, expired bool) {
	key := s.getSignedKey(hash)
	value, err := s.db.Get([]byte(key), nil)
	if err != nil {
		s.logger.Debug().Msgf("key:%s has signed: false", key)
		return false, false
	}
	switch {
	case len(value) == 1:
		s.logger.Debug().Msgf("key:%s has signed: true (legacy)", key)
		return true, false
	case len(value) >= signedExpiryBytes:
		expiryMs := int64(binary.BigEndian.Uint64(value[:signedExpiryBytes]))
		isExpired := time.Now().UnixMilli() >= expiryMs
		s.logger.Debug().Int64("expiry_unix_ms", expiryMs).Bool("expired", isExpired).
			Msgf("key:%s has signed: true", key)
		return true, isExpired
	default:
		s.logger.Warn().Int("len", len(value)).Str("hash", hash).
			Msg("unexpected signed cache value length, treating as signed (not expired)")
		return true, false
	}
}

// RemoveSigned removes the corresponding TxOutItem from the signer cache. The provided
// transaction hash should be for the broadcast transaction - it is internally mapped to
// the cache key for the TxOutItem.
func (s *CacheStore) RemoveSigned(transactionHash string) error {
	mapKey := s.getMapKey(transactionHash)
	value, err := s.db.Get([]byte(mapKey), nil)
	if err != nil {
		// bifrost didn't sign this tx , so it is fine
		if errors.Is(err, leveldb.ErrNotFound) {
			return nil
		}
		s.logger.Err(err).Msg("fail to check map key exist")
		return err
	}
	txOutItemHash := string(value)
	signedKey := s.getSignedKey(txOutItemHash)
	if err = s.db.Delete([]byte(signedKey), nil); err != nil {
		s.logger.Error().Err(err).Msgf("fail to remove %s from signed cache", txOutItemHash)
		return fmt.Errorf("fail to remove signed cache, err: %w", err)
	}
	return nil
}

// SetTransactionHashMap map a transaction hash to a tx out item hash
func (s *CacheStore) SetTransactionHashMap(txOutItemHash, transactionHash string) error {
	key := s.getMapKey(transactionHash)
	return s.db.Put([]byte(key), []byte(txOutItemHash), nil)
}

// GetSignedTxHash returns the broadcast transaction hash recorded for the given tx out
// item hash, or ("", false) if none is recorded. Only entries written by
// SetSignedWithExpiry carry a tx hash; legacy SetSigned entries return ("", false).
func (s *CacheStore) GetSignedTxHash(txOutItemHash string) (string, bool) {
	key := s.getSignedKey(txOutItemHash)
	value, err := s.db.Get([]byte(key), nil)
	if err != nil || len(value) <= signedExpiryBytes {
		return "", false
	}
	return string(value[signedExpiryBytes:]), true
}

// SetLatestRecordedTx map a vault and transaction inbound or outbound to transaction hash
func (s *CacheStore) SetLatestRecordedTx(vaultKey, transactionHash string) error {
	key := s.getVaultKey(vaultKey)
	return s.db.Put([]byte(key), []byte(transactionHash), nil)
}

func (s *CacheStore) GetLatestRecordedTx(vaultKey string) (string, error) {
	key := s.getVaultKey(vaultKey)
	hash, err := s.db.Get([]byte(key), nil)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Close underlying db
func (s *CacheStore) Close() error {
	return s.db.Close()
}
