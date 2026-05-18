package signer

import (
	"fmt"

	. "gopkg.in/check.v1"

	btypes "gitlab.com/thorchain/thornode/v3/bifrost/blockscanner/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	ttypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type ThorchainBlockScanSuite struct{}

var _ = Suite(&ThorchainBlockScanSuite{})

// ----- mock bridge for block scanner tests -----

type mockBlockScanBridge struct {
	thorclient.ThorchainBridge
	blockHeight    int64
	blockHeightErr error
	keygenBlock    ttypes.KeygenBlock
	keygenErr      error
	keysign        types.TxOut
	keysignErr     error
}

func (b *mockBlockScanBridge) GetBlockHeight() (int64, error) {
	return b.blockHeight, b.blockHeightErr
}

func (b *mockBlockScanBridge) GetKeygenBlock(height int64, pk string) (ttypes.KeygenBlock, error) {
	return b.keygenBlock, b.keygenErr
}

func (b *mockBlockScanBridge) GetKeysign(height int64, pk string) (types.TxOut, error) {
	return b.keysign, b.keysignErr
}

// ----- tests -----

func (s *ThorchainBlockScanSuite) TestNewThorchainBlockScan_NilStorage(c *C) {
	m := GetMetricForTest(c)
	pubkeyMgr := pubkeymanager.NewMockPoolAddressValidator()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, nil, &mockBlockScanBridge{}, m, pubkeyMgr)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "scanStorage is nil")
	c.Assert(bs, IsNil)
}

func (s *ThorchainBlockScanSuite) TestNewThorchainBlockScan_NilMetrics(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()
	pubkeyMgr := pubkeymanager.NewMockPoolAddressValidator()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, &mockBlockScanBridge{}, nil, pubkeyMgr)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "metric is nil")
	c.Assert(bs, IsNil)
}

func (s *ThorchainBlockScanSuite) TestNewThorchainBlockScan_Success(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()
	pubkeyMgr := pubkeymanager.NewMockPoolAddressValidator()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, &mockBlockScanBridge{}, m, pubkeyMgr)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
}

func (s *ThorchainBlockScanSuite) TestGetTxOutMessages(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, &mockBlockScanBridge{}, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)
	ch := bs.GetTxOutMessages()
	c.Assert(ch, NotNil)
}

func (s *ThorchainBlockScanSuite) TestGetKeygenMessages(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, &mockBlockScanBridge{}, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)
	ch := bs.GetKeygenMessages()
	c.Assert(ch, NotNil)
}

func (s *ThorchainBlockScanSuite) TestGetHeight(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{blockHeight: 42}
	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	height, err := bs.GetHeight()
	c.Assert(err, IsNil)
	c.Assert(height, Equals, int64(42))
}

func (s *ThorchainBlockScanSuite) TestGetHeight_Error(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{blockHeightErr: fmt.Errorf("node not ready")}
	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	_, err = bs.GetHeight()
	c.Assert(err, NotNil)
}

func (s *ThorchainBlockScanSuite) TestGetNetworkFee(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, &mockBlockScanBridge{}, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	size, rate := bs.GetNetworkFee()
	c.Assert(size, Equals, uint64(0))
	c.Assert(rate, Equals, uint64(0))
}

func (s *ThorchainBlockScanSuite) TestFetchMemPool(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, &mockBlockScanBridge{}, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	txIn, err := bs.FetchMemPool(100)
	c.Assert(err, IsNil)
	c.Assert(txIn, DeepEquals, types.TxIn{})
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_KeygenError(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{
		keysign:   types.TxOut{},
		keygenErr: fmt.Errorf("keygen fail"),
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	_, err = bs.FetchTxs(100, 0)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to get keygen from thorchain.*")
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_KeygenUnavailable(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{
		keysign: types.TxOut{},
		keygenBlock: ttypes.KeygenBlock{
			Height: 0, // height 0 means unavailable
		},
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	_, err = bs.FetchTxs(100, 0)
	c.Assert(err, Equals, btypes.ErrUnavailableBlock)
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_KeygenWithEntries(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{
		keysign: types.TxOut{},
		keygenBlock: ttypes.KeygenBlock{
			Height: 100,
			Keygens: []ttypes.Keygen{
				{
					Type: ttypes.KeygenType_AsgardKeygen,
				},
			},
		},
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	// FetchTxs will try to send keygen on channel. Need a goroutine to read it.
	done := make(chan struct{})
	go func() {
		kb := <-bs.GetKeygenMessages()
		c.Assert(kb.Height, Equals, int64(100))
		c.Assert(len(kb.Keygens), Equals, 1)
		close(done)
	}()

	_, err = bs.FetchTxs(100, 0)
	c.Assert(err, IsNil)
	<-done
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_TxOutError(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{
		keygenBlock: ttypes.KeygenBlock{Height: 100},
		keysignErr:  fmt.Errorf("keysign error"),
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	_, err = bs.FetchTxs(100, 0)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to get keysign from block scanner.*")
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_TxOutUnavailable(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{
		keygenBlock: ttypes.KeygenBlock{Height: 100},
		keysignErr:  btypes.ErrUnavailableBlock,
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	_, err = bs.FetchTxs(100, 0)
	c.Assert(err, Equals, btypes.ErrUnavailableBlock)
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_TxOutEmpty(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &mockBlockScanBridge{
		keygenBlock: ttypes.KeygenBlock{Height: 100},
		keysign:     types.TxOut{TxArray: nil},
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	txIn, err := bs.FetchTxs(100, 0)
	c.Assert(err, IsNil)
	c.Assert(txIn, DeepEquals, types.TxIn{})
}

func (s *ThorchainBlockScanSuite) TestFetchTxs_TxOutWithItems(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	vaultPubKey, err := common.NewPubKey(pubkeymanager.MockPubkey)
	c.Assert(err, IsNil)

	bridge := &mockBlockScanBridge{
		keygenBlock: ttypes.KeygenBlock{Height: 100},
		keysign: types.TxOut{
			Height: 100,
			TxArray: []types.TxArrayItem{
				{
					Chain:       common.ETHChain,
					ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
					VaultPubKey: vaultPubKey,
				},
			},
		},
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	done := make(chan struct{})
	go func() {
		txOut := <-bs.GetTxOutMessages()
		c.Assert(txOut.Height, Equals, int64(100))
		c.Assert(len(txOut.TxArray), Equals, 1)
		close(done)
	}()

	_, err = bs.FetchTxs(100, 0)
	c.Assert(err, IsNil)
	<-done
}

func (s *ThorchainBlockScanSuite) TestProcessKeygenBlock_NoKeygens(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	// Height 100, no keygens - should not send on channel
	bridge := &mockBlockScanBridge{
		keygenBlock: ttypes.KeygenBlock{
			Height:  100,
			Keygens: nil,
		},
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	err = bs.processKeygenBlock(100)
	c.Assert(err, IsNil)
}

func (s *ThorchainBlockScanSuite) TestProcessTxOutBlock_EmptyPubKey(c *C) {
	m := GetMetricForTest(c)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	// GetSignPubKeys returns a pubkey with value, but bridge returns empty TxOut
	bridge := &mockBlockScanBridge{
		keysign: types.TxOut{TxArray: nil},
	}

	bs, err := NewThorchainBlockScan(config.BifrostBlockScannerConfiguration{}, storage, bridge, m, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	err = bs.processTxOutBlock(100)
	c.Assert(err, IsNil)
}
