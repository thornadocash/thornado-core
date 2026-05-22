package signer

import (
	"errors"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/blockscanner"
	btypes "github.com/thornadocash/go-thornado/bifrost/blockscanner/types"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type ThornadoBlockScan struct {
	logger         zerolog.Logger
	wg             *sync.WaitGroup
	stopChan       chan struct{}
	txOutChan      chan types.TxOut
	keygenChan     chan ttypes.KeygenBlock
	cfg            config.BifrostBlockScannerConfiguration
	scannerStorage blockscanner.ScannerStorage
	thornado      thornadoclient.ThornadoBridge
	errCounter     *prometheus.CounterVec
	pubkeyMgr      pubkeymanager.PubKeyValidator
}

// NewThornadoBlockScan create a new instance of thornado block scanner
func NewThornadoBlockScan(cfg config.BifrostBlockScannerConfiguration, scanStorage blockscanner.ScannerStorage, thornado thornadoclient.ThornadoBridge, m *metrics.Metrics, pubkeyMgr pubkeymanager.PubKeyValidator) (*ThornadoBlockScan, error) {
	if scanStorage == nil {
		return nil, errors.New("scanStorage is nil")
	}
	if m == nil {
		return nil, errors.New("metric is nil")
	}
	return &ThornadoBlockScan{
		logger:         log.With().Str("module", "blockscanner").Str("chain", "THOR").Logger(),
		wg:             &sync.WaitGroup{},
		stopChan:       make(chan struct{}),
		txOutChan:      make(chan types.TxOut),
		keygenChan:     make(chan ttypes.KeygenBlock),
		cfg:            cfg,
		scannerStorage: scanStorage,
		thornado:      thornado,
		errCounter:     m.GetCounterVec(metrics.ThornadoBlockScannerError),
		pubkeyMgr:      pubkeyMgr,
	}, nil
}

// GetMessages return the channel
func (b *ThornadoBlockScan) GetTxOutMessages() <-chan types.TxOut {
	return b.txOutChan
}

func (b *ThornadoBlockScan) GetKeygenMessages() <-chan ttypes.KeygenBlock {
	return b.keygenChan
}

func (b *ThornadoBlockScan) GetHeight() (int64, error) {
	return b.thornado.GetBlockHeight()
}

// ThornadoBlockScan's GetNetworkFee only exists to satisfy the BlockScannerFetcher interface
// and should never be called, since broadcast network fees are for external chains' observed fees.
func (b *ThornadoBlockScan) GetNetworkFee() (transactionSize, transactionFeeRate uint64) {
	b.logger.Error().Msg("ThornadoBlockScan GetNetworkFee was called (which should never happen)")
	return 0, 0
}

func (c *ThornadoBlockScan) FetchMemPool(height int64) (types.TxIn, error) {
	return types.TxIn{}, nil
}

func (b *ThornadoBlockScan) FetchTxs(height, _ int64) (types.TxIn, error) {
	if err := b.processTxOutBlock(height); err != nil {
		return types.TxIn{}, err
	}
	if err := b.processKeygenBlock(height); err != nil {
		return types.TxIn{}, err
	}
	return types.TxIn{}, nil
}

func (b *ThornadoBlockScan) processKeygenBlock(blockHeight int64) error {
	pk := b.pubkeyMgr.GetNodePubKey(common.SigningAlgoSecp256k1)
	keygen, err := b.thornado.GetKeygenBlock(blockHeight, pk.String())
	if err != nil {
		return fmt.Errorf("fail to get keygen from thornado: %w", err)
	}

	// custom error (to be dropped and not logged) because the block is
	// available yet
	if keygen.Height == 0 {
		return btypes.ErrUnavailableBlock
	}

	if len(keygen.Keygens) > 0 {
		b.keygenChan <- keygen
	}
	return nil
}

func (b *ThornadoBlockScan) processTxOutBlock(blockHeight int64) error {
	for _, pk := range b.pubkeyMgr.GetSignPubKeys() {
		if len(pk.String()) == 0 {
			continue
		}
		tx, err := b.thornado.GetKeysign(blockHeight, pk.String())
		if err != nil {
			if errors.Is(err, btypes.ErrUnavailableBlock) {
				// custom error (to be dropped and not logged) because the block is
				// available yet
				return btypes.ErrUnavailableBlock
			}
			return fmt.Errorf("fail to get keysign from block scanner: %w", err)
		}

		if len(tx.TxArray) == 0 {
			b.logger.Debug().Int64("block", blockHeight).Msg("nothing to process")
			continue
		}
		b.txOutChan <- tx
	}
	return nil
}
