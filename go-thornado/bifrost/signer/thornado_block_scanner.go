package signer

import (
	"context"
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
	"github.com/thornadocash/go-thornado/constants"
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
	thornado       thornadoclient.ThornadoBridge
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
		logger:         log.With().Str("module", "blockscanner").Str("chain", "BTC").Logger(),
		wg:             &sync.WaitGroup{},
		stopChan:       make(chan struct{}),
		txOutChan:      make(chan types.TxOut),
		keygenChan:     make(chan ttypes.KeygenBlock),
		cfg:            cfg,
		scannerStorage: scanStorage,
		thornado:       thornado,
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
	timeout := b.cfg.HTTPRequestTimeout
	if timeout <= 0 {
		timeout = constants.ThornadoBlockTime
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	status, err := b.thornado.GetContext().Client.Status(ctx)
	if err != nil {
		return 0, err
	}
	height := status.SyncInfo.LatestBlockHeight
	const scannerLagBlocks = 2
	if height <= scannerLagBlocks {
		return 0, nil
	}
	return height - scannerLagBlocks, nil
}

func (b *ThornadoBlockScan) NetworkFeeUpdateEnabled() bool {
	return false
}

// ThornadoBlockScan's GetNetworkFee only exists to satisfy the BlockScannerFetcher interface.
// The generic block scanner skips timed network-fee updates for this scanner.
func (b *ThornadoBlockScan) GetNetworkFee() (transactionSize, transactionFeeRate uint64) {
	b.logger.Debug().Msg("ThornadoBlockScan GetNetworkFee called")
	return 0, 0
}

func (c *ThornadoBlockScan) FetchMemPool(height int64) (types.TxIn, error) {
	return types.TxIn{}, nil
}

func (b *ThornadoBlockScan) FetchTxs(height, _ int64) (types.TxIn, error) {
	if err := b.processTxOutBlock(height); err != nil {
		return types.TxIn{}, err
	}
	go func(blockHeight int64) {
		if err := b.processKeygenBlock(blockHeight); err != nil {
			b.logger.Error().Err(err).Int64("block", blockHeight).Msg("fail to process keygen block")
		}
	}(height)
	return types.TxIn{}, nil
}

func (b *ThornadoBlockScan) processKeygenBlock(blockHeight int64) error {
	pk := b.pubkeyMgr.GetNodePubKey(common.SigningAlgoSecp256k1)
	keygen, err := b.thornado.GetKeygenBlock(blockHeight, pk.String())
	if err != nil {
		return fmt.Errorf("fail to get keygen from thornado: %w", err)
	}

	if keygen.Height == 0 {
		return nil
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
				b.logger.Debug().Int64("block", blockHeight).Stringer("pubkey", pk).Msg("skipping unavailable txout block")
				continue
			}
			return fmt.Errorf("fail to get keysign from block scanner: %w", err)
		}

		if len(tx.TxArray) == 0 {
			b.logger.Debug().Int64("block", blockHeight).Msg("nothing to process")
			continue
		}
		b.txOutChan <- tx
	}
	pendingTxOuts, err := b.thornado.GetPendingTxOutKeysigns()
	if err != nil {
		return fmt.Errorf("fail to get pending keysigns from txout queue: %w", err)
	}
	for _, tx := range pendingTxOuts {
		if len(tx.TxArray) == 0 {
			continue
		}
		if tx.Height == blockHeight {
			continue
		}
		b.logger.Debug().
			Int64("scanner_block", blockHeight).
			Int64("txout_height", tx.Height).
			Int("txout_count", len(tx.TxArray)).
			Msg("discovered pending txout from queue")
		b.txOutChan <- tx
	}
	return nil
}
