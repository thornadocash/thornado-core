package blockscanner

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
)

type startHeightStorage struct {
	pos int64
}

func (s *startHeightStorage) GetScanPos() (int64, error) {
	return s.pos, nil
}

func (s *startHeightStorage) SetScanPos(block int64) error {
	s.pos = block
	return nil
}

func (s *startHeightStorage) SetBlockScanStatus(Block, BlockScanStatus) error {
	return nil
}

func (s *startHeightStorage) RemoveBlockStatus(int64) error {
	return nil
}

func (s *startHeightStorage) GetBlocksForRetry(bool) ([]Block, error) {
	return nil, nil
}

func (s *startHeightStorage) GetInternalDb() *leveldb.DB {
	return nil
}

func (s *startHeightStorage) Close() error {
	return nil
}

type startHeightBridge struct {
	thornadoclient.ThornadoBridge
	lastObserved int64
}

func (b startHeightBridge) WaitToCatchUp() error {
	return nil
}

func (b startHeightBridge) GetLastObservedInHeight(common.Chain) (int64, error) {
	return b.lastObserved, nil
}

type startHeightFetcher struct {
	height int64
}

func (f startHeightFetcher) FetchMemPool(int64) (types.TxIn, error) {
	return types.TxIn{}, nil
}

func (f startHeightFetcher) FetchTxs(int64, int64) (types.TxIn, error) {
	return types.TxIn{}, nil
}

func (f startHeightFetcher) GetHeight() (int64, error) {
	return f.height, nil
}

func (f startHeightFetcher) GetNetworkFee() (uint64, uint64) {
	return 0, 0
}

type overridingStartHeightFetcher struct {
	startHeightFetcher
	startHeight int64
}

func (f overridingStartHeightFetcher) GetScannerStartHeight() (int64, error) {
	return f.startHeight, nil
}

func newStartHeightScanner(pos, lastObserved, configuredStart int64) *BlockScanner {
	return &BlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID:           common.BTCChain,
			StartBlockHeight:  configuredStart,
			MaxResumeBlockLag: time.Hour,
		},
		logger:         zerolog.Nop(),
		scannerStorage: &startHeightStorage{pos: pos},
		thornadoBridge: startHeightBridge{lastObserved: lastObserved},
		chainScanner:   startHeightFetcher{height: 1_000},
	}
}

func TestGetStartHeightUsesPersistedPositionBeforeConfiguredStart(t *testing.T) {
	scanner := newStartHeightScanner(500, 0, 10)
	height, err := scanner.GetStartHeight()
	if err != nil {
		t.Fatal(err)
	}
	if height != 500 {
		t.Fatalf("expected persisted scan position 500, got %d", height)
	}
}

func TestGetStartHeightUsesConfiguredStartOnlyForFreshScanner(t *testing.T) {
	scanner := newStartHeightScanner(0, 0, 10)
	height, err := scanner.GetStartHeight()
	if err != nil {
		t.Fatal(err)
	}
	if height != 9 {
		t.Fatalf("expected previous height 9 before configured start 10, got %d", height)
	}
}

func TestGetStartHeightClampsStoredPositionAheadOfFetcherTip(t *testing.T) {
	scanner := newStartHeightScanner(4_486, 4_486, 0)
	scanner.chainScanner = startHeightFetcher{height: 2_650}
	height, err := scanner.GetStartHeight()
	if err != nil {
		t.Fatal(err)
	}
	if height != 2_647 {
		t.Fatalf("expected clamped previous height 2647, got %d", height)
	}
}

func TestGetStartHeightUsesStoredPositionWhenConsensusCappedToFetcherTip(t *testing.T) {
	scanner := newStartHeightScanner(1_469, 4_486, 0)
	scanner.chainScanner = startHeightFetcher{height: 2_650}
	height, err := scanner.GetStartHeight()
	if err != nil {
		t.Fatal(err)
	}
	if height != 1_469 {
		t.Fatalf("expected persisted scan position 1469, got %d", height)
	}
}

func TestGetStartHeightUsesScannerSpecificOverride(t *testing.T) {
	scanner := newStartHeightScanner(0, 105, 0)
	scanner.chainScanner = overridingStartHeightFetcher{
		startHeightFetcher: startHeightFetcher{height: 36},
		startHeight:        0,
	}
	height, err := scanner.GetStartHeight()
	if err != nil {
		t.Fatal(err)
	}
	if height != 0 {
		t.Fatalf("expected scanner-specific previous height 0, got %d", height)
	}
}
