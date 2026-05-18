package blockscanner

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/syndtr/goleveldb/leveldb"
	"gitlab.com/thorchain/thornode/v3/bifrost/db"
	"gitlab.com/thorchain/thornode/v3/config"
)

// LevelDBScannerStorageSolana is a scanner storage backed by level db
type LevelDBScannerStorageSolana struct {
	db *leveldb.DB
}

// BlockStatusItem indicate the status of a block
type SolanaAccountScanStatus struct {
	LastSignature string `json:"last_signature"`
	LastSlot      uint64 `json:"last_slot"`
}

func accountScanStatusKey(account string) string {
	return fmt.Sprintf("scanstat-%s", account)
}

// NewLevelDBScannerStorageSolana create a new instance of LevelDBScannerStorageSolana
func NewLevelDBScannerStorageSolana(db *leveldb.DB) (*LevelDBScannerStorageSolana, error) {
	return &LevelDBScannerStorageSolana{db: db}, nil
}

// GetScanPos get current Scan Pos
func (ldbss *LevelDBScannerStorageSolana) GetScanStatus(account string) (string, uint64, error) {
	key := accountScanStatusKey(account)
	buf, err := ldbss.db.Get([]byte(key), nil)
	if err != nil {
		return "", 0, err
	}
	pos, _ := binary.Uvarint(buf)
	sig := string(buf[8:])
	return sig, pos, nil
}

// SetScanPos save current scan pos
func (ldbss *LevelDBScannerStorageSolana) SetScanStatus(account, lastSignature string, lastSlot uint64) error {
	buf := make([]byte, 8+len(lastSignature))
	_ = binary.PutUvarint(buf, lastSlot)
	copy(buf[8:], lastSignature)

	key := accountScanStatusKey(account)
	return ldbss.db.Put([]byte(key), buf, nil)
}

// GetScanPos get current Scan Pos
func (ldbss *LevelDBScannerStorageSolana) GetScanPos() (uint64, error) {
	buf, err := ldbss.db.Get([]byte(ScanPosKey), nil)
	if err != nil {
		return 0, err
	}
	pos, _ := binary.Uvarint(buf)
	return pos, nil
}

// SetScanPos save current scan pos
func (ldbss *LevelDBScannerStorageSolana) SetScanPos(slot uint64) error {
	buf := make([]byte, 8)
	n := binary.PutUvarint(buf, slot)
	return ldbss.db.Put([]byte(ScanPosKey), buf[:n], nil)
}

func (ldbss *LevelDBScannerStorageSolana) Close() error {
	return ldbss.db.Close()
}

// ScannerStorage define the method need to be used by scanner
type ScannerStorageSolana interface {
	GetScanStatus(account string) (string, uint64, error)
	SetScanStatus(account, lastSignature string, lastSlot uint64) error
	GetScanPos() (uint64, error)
	SetScanPos(block uint64) error
	GetInternalDb() *leveldb.DB
	io.Closer
}

// BlockScannerStorage
type BlockScannerStorageSolana struct {
	*LevelDBScannerStorageSolana
	db *leveldb.DB
}

func NewBlockScannerStorageSolana(levelDbFolder string, opts config.LevelDBOptions) (*BlockScannerStorageSolana, error) {
	ldb, err := db.NewLevelDB(levelDbFolder, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create level db: %w", err)
	}

	levelDbStorage, err := NewLevelDBScannerStorageSolana(ldb)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}
	return &BlockScannerStorageSolana{
		LevelDBScannerStorageSolana: levelDbStorage,
		db:                          ldb,
	}, nil
}

func (s *BlockScannerStorageSolana) GetInternalDb() *leveldb.DB {
	return s.db
}
