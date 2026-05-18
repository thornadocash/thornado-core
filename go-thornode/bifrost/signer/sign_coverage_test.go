package signer

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/keysign"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
	ttypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type SignCoverageSuite struct{}

var _ = Suite(&SignCoverageSuite{})

func (s *SignCoverageSuite) SetUpSuite(c *C) {
	thorchain.SetupConfigForTest()
}

// ----- extended mock bridge -----

type mockSignBridge struct {
	thorclient.ThorchainBridge
	blockHeight    int64
	blockHeightErr error
	constants      map[string]int64
	constantsErr   error
	mimirs         map[string]int64
	mimirErr       error
	vault          ttypes.Vault
	vaultErr       error
	keysign        types.TxOut
	keysignErr     error
	catchingUp     bool
	catchingUpErr  error
	cfg            config.BifrostClientConfiguration
	keygenStdTxMsg cosmos.Msg
	keygenStdTxErr error
	broadcastTxID  common.TxID
	broadcastErr   error
	broadcastCalls int
}

func (b *mockSignBridge) GetBlockHeight() (int64, error) {
	return b.blockHeight, b.blockHeightErr
}

func (b *mockSignBridge) GetConstants() (map[string]int64, error) {
	if b.constantsErr != nil {
		return nil, b.constantsErr
	}
	return b.constants, nil
}

func (b *mockSignBridge) GetMimir(key string) (int64, error) {
	if b.mimirErr != nil {
		return 0, b.mimirErr
	}
	val, ok := b.mimirs[key]
	if !ok {
		return 0, nil
	}
	return val, nil
}

func (b *mockSignBridge) GetMimirWithRef(template, ref string) (int64, error) {
	key := fmt.Sprintf(template, ref)
	return b.GetMimir(key)
}

func (b *mockSignBridge) GetVault(pubkey string) (ttypes.Vault, error) {
	if b.vaultErr != nil {
		return ttypes.Vault{}, b.vaultErr
	}
	return b.vault, nil
}

func (b *mockSignBridge) GetKeysign(blockHeight int64, pk string) (types.TxOut, error) {
	return b.keysign, b.keysignErr
}

func (b *mockSignBridge) IsCatchingUp() (bool, error) {
	return b.catchingUp, b.catchingUpErr
}

func (b *mockSignBridge) GetConfig() config.BifrostClientConfiguration {
	return b.cfg
}

func (b *mockSignBridge) GetKeygenStdTx(poolPubKey common.PubKey, secp256k1Signature, keysharesBackup []byte, blame []ttypes.Blame, inputPks common.PubKeys, keygenType ttypes.KeygenType, chains common.Chains, height, keygenTime int64, poolPubKeyEddsa common.PubKey, keysharesBackupEddsa []byte) (cosmos.Msg, error) {
	return b.keygenStdTxMsg, b.keygenStdTxErr
}

func (b *mockSignBridge) Broadcast(msgs ...cosmos.Msg) (common.TxID, error) {
	b.broadcastCalls++
	return b.broadcastTxID, b.broadcastErr
}

// ----- mock bridge for broadcast retry -----

type mockBroadcastRetryBridge struct {
	thorclient.ThorchainBridge
	keygenStdTxMsg cosmos.Msg
	keygenStdTxErr error
	broadcastFn    func() (common.TxID, error)
}

func (b *mockBroadcastRetryBridge) GetKeygenStdTx(poolPubKey common.PubKey, secp256k1Signature, keysharesBackup []byte, blame []ttypes.Blame, inputPks common.PubKeys, keygenType ttypes.KeygenType, chains common.Chains, height, keygenTime int64, poolPubKeyEddsa common.PubKey, keysharesBackupEddsa []byte) (cosmos.Msg, error) {
	return b.keygenStdTxMsg, b.keygenStdTxErr
}

func (b *mockBroadcastRetryBridge) Broadcast(msgs ...cosmos.Msg) (common.TxID, error) {
	return b.broadcastFn()
}

// ----- mock chain client that returns round 7 keysign error -----

type MockRound7ErrorChainClient struct {
	MockChainClient
}

func (b *MockRound7ErrorChainClient) SignTx(tai types.TxOutItem, height int64) ([]byte, []byte, *types.TxInItem, error) {
	return nil, nil, nil, tss.NewKeysignError(ttypes.Blame{
		FailReason: "round 7 failure",
		Round:      "SignRound7Message",
	})
}

// ----- mock chain client with unhealthy scanner -----

type MockUnhealthyChainClient struct {
	MockChainClient
}

func (b *MockUnhealthyChainClient) IsBlockScannerHealthy() bool {
	return false
}

// ----- mock chain client with sign error -----

type MockSignErrorChainClient struct {
	MockChainClient
	signErr    error
	checkpoint []byte
}

func (b *MockSignErrorChainClient) SignTx(tai types.TxOutItem, height int64) ([]byte, []byte, *types.TxInItem, error) {
	return nil, b.checkpoint, nil, b.signErr
}

// ----- mock chain client that returns signed tx -----

type MockSuccessChainClient struct {
	MockChainClient
	broadcastErr error
	broadcastTxs int
}

func (b *MockSuccessChainClient) SignTx(tai types.TxOutItem, height int64) ([]byte, []byte, *types.TxInItem, error) {
	return []byte("signed-tx"), nil, nil, nil
}

func (b *MockSuccessChainClient) BroadcastTx(_ types.TxOutItem, tx []byte) (string, error) {
	b.broadcastTxs++
	return "TXHASH123", b.broadcastErr
}

// ----- tests -----

func (s *SignCoverageSuite) TestGetChain(c *C) {
	sign := &Signer{
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		logger: log.With().Str("module", "test").Logger(),
	}

	// Found
	chain, err := sign.getChain(common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(chain, NotNil)

	// Not found
	chain, err = sign.getChain(common.BTCChain)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "not supported")
	c.Assert(chain, IsNil)
}

func (s *SignCoverageSuite) TestShouldSign(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	otherPubKey := common.PubKey("tthorpub1addwnpepqfup3y8p0egd7ml7vrnlxgl3wvnp89mpn0tjpj0p2nm2gh0n9hlrvrtylay")

	sign := &Signer{
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	// Should sign - vault pub key matches
	c.Assert(sign.shouldSign(types.TxOutItem{VaultPubKey: vaultPubKey}), Equals, true)

	// Should not sign - different vault pub key
	c.Assert(sign.shouldSign(types.TxOutItem{VaultPubKey: otherPubKey}), Equals, false)

	// Should sign - eddsa pub key matches
	c.Assert(sign.shouldSign(types.TxOutItem{VaultPubKeyEddsa: vaultPubKey}), Equals, true)
}

func (s *SignCoverageSuite) TestIsTssKeysign(c *C) {
	localECDSA := common.PubKey("ecdsa-local")
	localEDDSA := common.PubKey("eddsa-local")

	sign := &Signer{
		localPubKeyECDSA: localECDSA,
		localPubKeyEDDSA: localEDDSA,
	}

	// TSS keysign - different pubkey
	c.Assert(sign.isTssKeysign(common.PubKey("other")), Equals, true)

	// Not TSS keysign - matches ECDSA local
	c.Assert(sign.isTssKeysign(localECDSA), Equals, false)

	// Not TSS keysign - matches EDDSA local
	c.Assert(sign.isTssKeysign(localEDDSA), Equals, false)
}

func (s *SignCoverageSuite) TestRunWithContext_Success(c *C) {
	ctx := context.Background()
	checkpoint, txIn, err := runWithContext(ctx, func() ([]byte, *types.TxInItem, error) {
		return []byte("checkpoint"), &types.TxInItem{Memo: "test"}, nil
	})
	c.Assert(err, IsNil)
	c.Assert(string(checkpoint), Equals, "checkpoint")
	c.Assert(txIn.Memo, Equals, "test")
}

func (s *SignCoverageSuite) TestRunWithContext_Error(c *C) {
	ctx := context.Background()
	checkpoint, txIn, err := runWithContext(ctx, func() ([]byte, *types.TxInItem, error) {
		return nil, nil, fmt.Errorf("sign failed")
	})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "sign failed")
	c.Assert(checkpoint, IsNil)
	c.Assert(txIn, IsNil)
}

func (s *SignCoverageSuite) TestRunWithContext_Timeout(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := runWithContext(ctx, func() ([]byte, *types.TxInItem, error) {
		time.Sleep(500 * time.Millisecond)
		return nil, nil, nil
	})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*deadline exceeded.*")
}

func (s *SignCoverageSuite) TestIsStopped(c *C) {
	sign := &Signer{
		stopChan: make(chan struct{}),
	}
	c.Assert(sign.isStopped(), Equals, false)
	close(sign.stopChan)
	c.Assert(sign.isStopped(), Equals, true)
}

func (s *SignCoverageSuite) TestStorageList(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{storage: storage}
	c.Assert(sign.storageList(), HasLen, 0)

	err = storage.Set(NewTxOutStoreItem(10, types.TxOutItem{Memo: "test"}, 0))
	c.Assert(err, IsNil)
	c.Assert(sign.storageList(), HasLen, 1)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_BlockHeightError(c *C) {
	bridge := &mockSignBridge{
		blockHeightErr: fmt.Errorf("node down"),
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{Chain: common.ETHChain},
		Height:    50,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "node down")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_ConstantsError(c *C) {
	bridge := &mockSignBridge{
		blockHeight:  100,
		constantsErr: fmt.Errorf("constants unavailable"),
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{Chain: common.ETHChain},
		Height:    50,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_Round7RetryMaxAttempts(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 1000,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{
			"MAXOUTBOUNDATTEMPTS": 2,
		},
		vault: ttypes.Vault{Status: ttypes.VaultStatus_ActiveVault},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	// Height 100 with block height 1000 and signing period 300 => attempt = 3, max = 2
	item := TxOutStoreItem{
		TxOutItem:   types.TxOutItem{Chain: common.ETHChain},
		Height:      100,
		Round7Retry: true,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_Round7RetryMimirError(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirErr: fmt.Errorf("mimir error"),
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	item := TxOutStoreItem{
		TxOutItem:   types.TxOutItem{Chain: common.ETHChain},
		Height:      50,
		Round7Retry: true,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_TooOld(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 1000,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	// Height 50, blockHeight 1000, signing period 300 => 1000-300 = 700 > 50-0 => skip
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{Chain: common.ETHChain},
		Height:    50,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil) // nil means discard
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_ChainNotSupported(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains:            map[common.Chain]chainclients.ChainClient{},
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{Chain: common.ETHChain},
		Height:    90,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "not supported")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_HaltSigningGlobal(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{
			"HALTSIGNING": 50,
		},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_HaltSigningChain(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{
			"HALTSIGNING":    0,
			"HaltSigningETH": 50,
		},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_ShouldNotSign(c *C) {
	otherPubKey := common.PubKey("tthorpub1addwnpepqfup3y8p0egd7ml7vrnlxgl3wvnp89mpn0tjpj0p2nm2gh0n9hlrvrtylay")
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: otherPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_EmptyToAddress(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "", // empty
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_UnhealthyScanner(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockUnhealthyChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*block scanner.*unhealthy.*")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_OutHashAlreadySet(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			OutHash:     "ABCDEF1234567890",
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_KeysignError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockSignErrorChainClient{
				signErr: fmt.Errorf("sign failed"),
			},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "sign failed")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_SignReturnsCheckpoint(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockSignErrorChainClient{
				signErr:    fmt.Errorf("sign failed"),
				checkpoint: []byte("checkpoint-data"),
			},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	checkpoint, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(string(checkpoint), Equals, "checkpoint-data")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_EmptySignedTx(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			// MockChainClient.SignTx returns nil,nil,nil,nil when no ks is set
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
		storage:   storage,
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_SuccessfulBroadcast(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	cc := &MockSuccessChainClient{}

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
	c.Assert(cc.broadcastTxs, Equals, 1)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_BroadcastError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	cc := &MockSuccessChainClient{broadcastErr: fmt.Errorf("broadcast failed")}

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Memo:        "test-memo",
		},
		Height: 90,
	}
	_, _, err = sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "broadcast failed")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_TssKeysignMetric(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	otherPubKey := common.PubKey("tthorpub1addwnpepqfup3y8p0egd7ml7vrnlxgl3wvnp89mpn0tjpj0p2nm2gh0n9hlrvrtylay")

	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	cc := &MockSuccessChainClient{}

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    otherPubKey, // Different from vault, so it's TSS
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	_, _, err = sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_AlreadySigned(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	coin := common.NewCoin(common.ETHAsset, cosmos.NewUint(1000))
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{},
		keysign: types.TxOut{
			Height: 90,
			TxArray: []types.TxArrayItem{
				{
					Chain:       common.ETHChain,
					ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
					VaultPubKey: vaultPubKey,
					Coin:        coin,
					OutHash:     "ALREADY_SIGNED_HASH",
				},
			},
		},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Coins:       common.Coins{coin},
			Height:      90,
		},
		Height: 90,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_KeysignGetError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:     map[string]int64{},
		keysignErr: fmt.Errorf("keysign fetch failed"),
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "keysign fetch failed")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_Round7RetryVaultError(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{
			"MAXOUTBOUNDATTEMPTS": 100,
		},
		vaultErr: fmt.Errorf("vault not found"),
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	item := TxOutStoreItem{
		TxOutItem:   types.TxOutItem{Chain: common.ETHChain},
		Height:      90,
		Round7Retry: true,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "vault not found")
}

func (s *SignCoverageSuite) TestSignAndBroadcast_Round7InactiveVaultTooOld(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 1000,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{
			"MAXOUTBOUNDATTEMPTS": 100,
		},
		vault: ttypes.Vault{Status: ttypes.VaultStatus_InactiveVault},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 0}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
	}

	// Round 7 retry on inactive vault, too old => discard
	item := TxOutStoreItem{
		TxOutItem:   types.TxOutItem{Chain: common.ETHChain},
		Height:      50,
		Round7Retry: true,
	}
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_WithCheckpointSet(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
		m:         GetMetricForTest(c),
		storage:   storage,
	}

	// Item with checkpoint set - should pass through to SignTx
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height:     90,
		Checkpoint: []byte("some-checkpoint"),
	}
	// MockChainClient.SignTx returns nil since ks is nil, so we'll get nil/nil/nil
	checkpoint, obs, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(obs, IsNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_WithExistingSignedTx(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	cc := &MockSuccessChainClient{}

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height:   90,
		SignedTx: []byte("previously-signed-tx"),
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(cc.broadcastTxs, Equals, 1)
}

func (s *SignCoverageSuite) TestProcessTransactions_MimirError(c *C) {
	bridge := &mockSignBridge{
		mimirErr: fmt.Errorf("mimir error"),
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		thorchainBridge: bridge,
	}

	// Should not panic, just log error and return
	sign.processTransactions()
	c.Assert(sign.pipeline, IsNil)
}

func (s *SignCoverageSuite) TestProcessTransactions_DefaultConcurrency(c *C) {
	bridge := &mockSignBridge{
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
		vault:   ttypes.Vault{Status: ttypes.VaultStatus_ActiveVault},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		thorchainBridge: bridge,
		storage:         storage,
		stopChan:        make(chan struct{}),
	}

	sign.processTransactions()
	c.Assert(sign.pipeline, NotNil)
	c.Assert(sign.pipeline.concurrency, Equals, int64(10)) // default
}

func (s *SignCoverageSuite) TestProcessTransactions_ConcurrencyChange(c *C) {
	bridge := &mockSignBridge{
		mimirs: map[string]int64{
			constants.SignerConcurrency.String(): 5,
		},
		keysign: types.TxOut{},
		vault:   ttypes.Vault{Status: ttypes.VaultStatus_ActiveVault},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		thorchainBridge: bridge,
		storage:         storage,
		stopChan:        make(chan struct{}),
	}

	sign.processTransactions()
	c.Assert(sign.pipeline, NotNil)
	c.Assert(sign.pipeline.concurrency, Equals, int64(5))

	// Change mimir
	bridge.mimirs[constants.SignerConcurrency.String()] = 3
	sign.processTransactions()
	c.Assert(sign.pipeline.concurrency, Equals, int64(3))
}

func (s *SignCoverageSuite) TestProcessTxnOut(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:   log.With().Str("module", "test").Logger(),
		wg:       &sync.WaitGroup{},
		stopChan: make(chan struct{}),
		storage:  storage,
	}

	ch := make(chan types.TxOut, 1)
	ch <- types.TxOut{
		Height: 100,
		TxArray: []types.TxArrayItem{
			{
				Chain:       common.ETHChain,
				ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(1000)),
			},
		},
	}
	close(ch)

	sign.wg.Add(1)
	go sign.processTxnOut(ch, 1)
	sign.wg.Wait()

	items := storage.List()
	c.Assert(items, HasLen, 1)
	c.Assert(items[0].Height, Equals, int64(100))
}

func (s *SignCoverageSuite) TestProcessTxnOut_StopChan(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:   log.With().Str("module", "test").Logger(),
		wg:       &sync.WaitGroup{},
		stopChan: make(chan struct{}),
		storage:  storage,
	}

	ch := make(chan types.TxOut, 1)
	close(sign.stopChan) // stop immediately

	sign.wg.Add(1)
	go sign.processTxnOut(ch, 1)
	sign.wg.Wait()

	c.Assert(storage.List(), HasLen, 0)
}

func (s *SignCoverageSuite) TestProcessKeygen_ChannelClose(c *C) {
	sign := &Signer{
		logger:   log.With().Str("module", "test").Logger(),
		wg:       &sync.WaitGroup{},
		stopChan: make(chan struct{}),
	}

	ch := make(chan ttypes.KeygenBlock, 1)
	close(ch)

	sign.wg.Add(1)
	go sign.processKeygen(ch)
	sign.wg.Wait()
}

func (s *SignCoverageSuite) TestProcessKeygen_StopChan(c *C) {
	sign := &Signer{
		logger:   log.With().Str("module", "test").Logger(),
		wg:       &sync.WaitGroup{},
		stopChan: make(chan struct{}),
	}

	ch := make(chan ttypes.KeygenBlock, 1)
	close(sign.stopChan) // stop immediately

	sign.wg.Add(1)
	go sign.processKeygen(ch)
	sign.wg.Wait()
}

func (s *SignCoverageSuite) TestSignAndBroadcast_HaltSigningChainMimirError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)

	callCount := 0
	bridge := &mockSignBridgeWithCallback{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		getMimirFn: func(key string) (int64, error) {
			callCount++
			if strings.Contains(key, "HALTSIGNING") && !strings.Contains(key, "ETH") {
				return 0, nil // global halt signing = 0 (not halted)
			}
			if strings.Contains(key, "ETH") {
				return 0, fmt.Errorf("chain mimir error")
			}
			return 0, nil
		},
		keysign: types.TxOut{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_HaltSigningGlobalMimirError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)

	bridge := &mockSignBridgeWithCallback{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		getMimirFn: func(key string) (int64, error) {
			if key == "HALTSIGNING" {
				return 0, fmt.Errorf("global mimir error")
			}
			return 0, nil
		},
		keysign: types.TxOut{},
	}

	sign := &Signer{
		logger:            log.With().Str("module", "test").Logger(),
		cfg:               config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:   bridge,
		constantsProvider: NewConstantsProvider(bridge),
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: &MockChainClient{},
		},
		pubkeyMgr: pubkeymanager.NewMockPoolAddressValidator(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}
	_, _, err := sign.signAndBroadcast(item)
	c.Assert(err, NotNil)
}

// mockSignBridgeWithCallback allows custom GetMimir behavior
type mockSignBridgeWithCallback struct {
	thorclient.ThorchainBridge
	blockHeight int64
	constants   map[string]int64
	getMimirFn  func(key string) (int64, error)
	keysign     types.TxOut
	keysignErr  error
	vault       ttypes.Vault
	vaultErr    error
}

func (b *mockSignBridgeWithCallback) GetBlockHeight() (int64, error) {
	return b.blockHeight, nil
}

func (b *mockSignBridgeWithCallback) GetConstants() (map[string]int64, error) {
	return b.constants, nil
}

func (b *mockSignBridgeWithCallback) GetMimir(key string) (int64, error) {
	return b.getMimirFn(key)
}

func (b *mockSignBridgeWithCallback) GetMimirWithRef(template, ref string) (int64, error) {
	key := fmt.Sprintf(template, ref)
	return b.GetMimir(key)
}

func (b *mockSignBridgeWithCallback) GetVault(pubkey string) (ttypes.Vault, error) {
	return b.vault, b.vaultErr
}

func (b *mockSignBridgeWithCallback) GetKeysign(height int64, pk string) (types.TxOut, error) {
	return b.keysign, b.keysignErr
}

func (b *mockSignBridgeWithCallback) IsCatchingUp() (bool, error) {
	return false, nil
}

func (b *mockSignBridgeWithCallback) GetConfig() config.BifrostClientConfiguration {
	return config.BifrostClientConfiguration{}
}

func (s *SignCoverageSuite) TestProcessTransaction_StorageRemoveOnSuccess(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	cc := &MockSuccessChainClient{}

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}

	// Store the item first
	err = storage.Set(item)
	c.Assert(err, IsNil)
	c.Assert(storage.List(), HasLen, 1)

	// Process should sign, broadcast, and remove
	sign.processTransaction(item)
	c.Assert(storage.List(), HasLen, 0)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_WithSignedTxRetryBroadcast(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)

	tssServer := newFakeTss("test-memo", true)
	bridge := fakeBridge{nil}
	ks, err := tss.NewKeySign(tssServer, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	cc := &MockChainClient{ks: ks}

	signBridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     signBridge,
		constantsProvider:   NewConstantsProvider(signBridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	// Item already has a signed tx - should skip signing and go to broadcast
	signedTxData := make([]byte, 64)
	for i := range signedTxData {
		signedTxData[i] = byte(i)
	}

	obs := &types.TxInItem{Memo: "observation-data"}
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Memo:        "test-memo",
		},
		Height:      90,
		SignedTx:    signedTxData,
		Observation: obs,
	}

	_, _, err = sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
	c.Assert(cc.broadcastCount, Equals, 1)
	c.Assert(cc.signCount, Equals, 0) // should not re-sign
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_MimirErrors(c *C) {
	bridge := &mockSignBridge{
		mimirErr: fmt.Errorf("mimir error"),
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		thorchainBridge: bridge,
	}

	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	c.Assert(result, Equals, false)
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_KeygenRetryIntervalZero(c *C) {
	bridge := &mockSignBridge{
		mimirs: map[string]int64{
			constants.ChurnRetryInterval.String():  1000,
			constants.KeygenRetryInterval.String(): 0,
		},
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		thorchainBridge: bridge,
	}

	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	c.Assert(result, Equals, false)
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_RetryIntervalTooShort(c *C) {
	bridge := &mockSignBridge{
		mimirs: map[string]int64{
			constants.ChurnRetryInterval.String():  1000,
			constants.KeygenRetryInterval.String(): 1, // very short
		},
	}

	sign := &Signer{
		logger: log.With().Str("module", "test").Logger(),
		cfg: config.Bifrost{
			Signer: config.BifrostSignerConfiguration{
				KeygenTimeout: 10 * time.Minute, // much longer than retry interval
			},
		},
		thorchainBridge: bridge,
	}

	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	c.Assert(result, Equals, false)
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_BlockHeightError(c *C) {
	bridge := &mockSignBridge{
		mimirs: map[string]int64{
			constants.ChurnRetryInterval.String():  1000,
			constants.KeygenRetryInterval.String(): 100,
		},
		blockHeightErr: fmt.Errorf("node down"),
	}

	sign := &Signer{
		logger: log.With().Str("module", "test").Logger(),
		cfg: config.Bifrost{
			Signer: config.BifrostSignerConfiguration{
				KeygenTimeout: 1 * time.Second,
			},
		},
		thorchainBridge: bridge,
	}

	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	c.Assert(result, Equals, false)
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_CloseToChurnRetry(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 990,
		mimirs: map[string]int64{
			constants.ChurnRetryInterval.String():  500,
			constants.KeygenRetryInterval.String(): 100,
		},
	}

	sign := &Signer{
		logger: log.With().Str("module", "test").Logger(),
		cfg: config.Bifrost{
			Signer: config.BifrostSignerConfiguration{
				KeygenTimeout: 1 * time.Second,
			},
		},
		thorchainBridge: bridge,
	}

	// keygenBlock at height 100, current at 990
	// targetRetryHeight = (100 - ((990-100)%100)) + 990 = 1000
	// condition: 1000 > 100 + 500 - 100 = 500 => true => return false
	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	c.Assert(result, Equals, false)
}

func (s *SignCoverageSuite) TestNewTxOutStoreItem(c *C) {
	item := NewTxOutStoreItem(100, types.TxOutItem{Memo: "test"}, 5)
	c.Assert(item.Height, Equals, int64(100))
	c.Assert(item.Status, Equals, TxAvailable)
	c.Assert(item.Index, Equals, int64(5))
	c.Assert(item.TxOutItem.Memo, Equals, "test")
}

func (s *SignCoverageSuite) TestTxOutStoreItem_Key_WithRetrievalKey(c *C) {
	item := TxOutStoreItem{
		TxOutItem:    types.TxOutItem{Memo: "test"},
		RetrievalKey: "custom-key",
	}
	c.Assert(item.Key(), Equals, "custom-key")
}

func (s *SignCoverageSuite) TestSignerStore_GetMissingKey(c *C) {
	store, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer store.Close()

	item, err := store.Get("nonexistent-key")
	c.Assert(err, IsNil) // no error, just empty item
	c.Assert(item.TxOutItem.Memo, Equals, "")
}

func (s *SignCoverageSuite) TestSignerStore_GetInternalDb(c *C) {
	store, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer store.Close()

	db := store.GetInternalDb()
	c.Assert(db, NotNil)
}

// Test that the keygen uses the mock TSS for signing and keygen message parsing
func (s *SignCoverageSuite) TestNewFakeTss(c *C) {
	msg := "test"
	tss := newFakeTss(msg, true)

	result, err := tss.KeySign(keysign.Request{})
	c.Assert(err, IsNil)
	c.Assert(int(result.Status), Equals, 1)
	c.Assert(len(result.Signatures), Equals, 1)

	decoded, err := base64.StdEncoding.DecodeString(result.Signatures[0].Msg)
	c.Assert(err, IsNil)
	c.Assert(string(decoded), Equals, msg)
}

func (s *SignCoverageSuite) TestProcessTransaction_Round7Error(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)

	// Create a TSS keysign that returns round 7 failure
	tssServer := newFakeTss("test-memo", false) // will fail on first attempt
	bridge := fakeBridge{nil}
	ks, err := tss.NewKeySign(tssServer, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	cc := &MockChainClient{ks: ks}

	signBridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     signBridge,
		constantsProvider:   NewConstantsProvider(signBridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Memo:        "test-memo",
			Coins: common.Coins{
				common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
			},
		},
		Height: 90,
	}

	err = storage.Set(item)
	c.Assert(err, IsNil)

	// processTransaction should encounter round 7 error and mark it
	sign.processTransaction(item)

	items := storage.List()
	c.Assert(items, HasLen, 1)
	c.Assert(items[0].Round7Retry, Equals, true)
	c.Assert(items[0].Checkpoint, NotNil)
}

func (s *SignCoverageSuite) TestProcessTransaction_AutoObserve(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)

	bridge := &mockSignBridge{
		blockHeight: 100,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	cc := &MockSuccessChainClient{}

	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100, AutoObserve: true}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 90,
	}

	err = storage.Set(item)
	c.Assert(err, IsNil)

	// process should succeed (AutoObserve=true but obs=nil since mock returns nil)
	sign.processTransaction(item)
	c.Assert(storage.List(), HasLen, 0) // removed on success
}

func (s *SignCoverageSuite) TestSignerStore_ListInvalidJSON(c *C) {
	store, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer store.Close()

	// Write invalid JSON to the store under the txout prefix
	key := txOutPrefix + "invalid-json"
	err = store.db.Put([]byte(key), []byte("not valid json"), nil)
	c.Assert(err, IsNil)

	// List should skip the invalid entry
	items := store.List()
	c.Assert(items, HasLen, 0)
}

func (s *SignCoverageSuite) TestSignerStore_ListEmptyValue(c *C) {
	store, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer store.Close()

	// Write empty value
	key := txOutPrefix + "empty"
	err = store.db.Put([]byte(key), []byte(""), nil)
	c.Assert(err, IsNil)

	items := store.List()
	c.Assert(items, HasLen, 0)
}

func (s *SignCoverageSuite) TestSignerStore_GetWithRetrievalKey(c *C) {
	store, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer store.Close()

	item := NewTxOutStoreItem(12, types.TxOutItem{Memo: "test"}, 1)
	c.Assert(store.Set(item), IsNil)

	// Get the item and verify RetrievalKey is set
	retrieved, err := store.Get(item.Key())
	c.Assert(err, IsNil)
	c.Assert(retrieved.RetrievalKey, Equals, item.Key())
	c.Assert(retrieved.TxOutItem.Memo, Equals, "test")

	// The Key() method should use the retrieval key
	c.Assert(retrieved.Key(), Equals, item.Key())
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_ChurnRetryIntervalDefault(c *C) {
	bridge := &mockSignBridge{
		blockHeight: 200,
		mimirs: map[string]int64{
			constants.ChurnRetryInterval.String():  0, // will use default
			constants.KeygenRetryInterval.String(): 100,
		},
	}

	sign := &Signer{
		logger: log.With().Str("module", "test").Logger(),
		cfg: config.Bifrost{
			Signer: config.BifrostSignerConfiguration{
				KeygenTimeout: 1 * time.Second,
			},
		},
		thorchainBridge: bridge,
	}

	// When churnRetryInterval <= 0, it uses the default from constants
	// The default ChurnRetryInterval is typically large, so the retry should
	// either succeed (schedule) or fail depending on whether close to churn retry
	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	// The result depends on the default ChurnRetryInterval value, but we at least
	// exercise the code path
	_ = result
}

func (s *SignCoverageSuite) TestScheduleKeygenRetry_SecondMimirError(c *C) {
	callCount := 0
	bridge := &mockSignBridgeWithCallback{
		blockHeight: 200,
		constants:   map[string]int64{},
		getMimirFn: func(key string) (int64, error) {
			callCount++
			if callCount == 1 {
				return 1000, nil // ChurnRetryInterval
			}
			return 0, fmt.Errorf("keygen retry mimir error")
		},
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		thorchainBridge: bridge,
	}

	result := sign.scheduleKeygenRetry(ttypes.KeygenBlock{Height: 100})
	c.Assert(result, Equals, false)
}

func (s *SignCoverageSuite) TestSemaphore(c *C) {
	sem := make(semaphore, 3)

	// acquire all
	count := sem.acquire()
	c.Assert(count, Equals, 3)

	// acquire when full returns 0
	count = sem.acquire()
	c.Assert(count, Equals, 0)

	// release some
	sem.release(2)

	// acquire again
	count = sem.acquire()
	c.Assert(count, Equals, 2)

	// release all
	sem.release(3)
}

func (s *SignCoverageSuite) TestVaultChain(c *C) {
	vc1 := vaultChain{Vault: "pub1", Chain: common.ETHChain}
	vc2 := vaultChain{Vault: "pub1", Chain: common.ETHChain}
	vc3 := vaultChain{Vault: "pub1", Chain: common.BTCChain}

	c.Assert(vc1, DeepEquals, vc2)
	c.Assert(vc1 == vc2, Equals, true)
	c.Assert(vc1 == vc3, Equals, false)
}

func (s *SignCoverageSuite) TestSignAndBroadcast_Round7RetryActiveVault(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 200,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs: map[string]int64{
			"MAXOUTBOUNDATTEMPTS": 100,
		},
		vault:   ttypes.Vault{Status: ttypes.VaultStatus_ActiveVault},
		keysign: types.TxOut{},
	}

	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	cc := &MockSuccessChainClient{}
	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		storage:             storage,
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	// Round 7 retry on active vault, not too old => should proceed to sign
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height:      190,
		Round7Retry: true,
	}
	_, _, err = sign.signAndBroadcast(item)
	c.Assert(err, IsNil)
}

// ----- sendKeygenToThorchain tests -----

func (s *SignCoverageSuite) TestSendKeygenToThorchain_Success(c *C) {
	m := GetMetricForTest(c)
	bridge := &mockSignBridge{
		keygenStdTxMsg: &ttypes.MsgTssPool{},
		broadcastTxID:  "TXID123",
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		cfg:             config.Bifrost{},
		thorchainBridge: bridge,
		m:               m,
		errCounter:      m.GetCounterVec(metrics.SignerError),
	}

	err := sign.sendKeygenToThorchain(
		100,
		common.EmptyPubKey,
		nil,
		nil,
		common.PubKeys{},
		ttypes.KeygenType_AsgardKeygen,
		500,
		common.EmptyPubKey,
	)
	c.Assert(err, IsNil)
	c.Assert(bridge.broadcastCalls, Equals, 1)
}

func (s *SignCoverageSuite) TestSendKeygenToThorchain_GetKeygenStdTxError(c *C) {
	m := GetMetricForTest(c)
	bridge := &mockSignBridge{
		keygenStdTxErr: fmt.Errorf("keygen tx error"),
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		cfg:             config.Bifrost{},
		thorchainBridge: bridge,
		m:               m,
		errCounter:      m.GetCounterVec(metrics.SignerError),
	}

	err := sign.sendKeygenToThorchain(
		100,
		common.EmptyPubKey,
		nil,
		nil,
		common.PubKeys{},
		ttypes.KeygenType_AsgardKeygen,
		500,
		common.EmptyPubKey,
	)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to get keygen id.*")
}

func (s *SignCoverageSuite) TestSendKeygenToThorchain_BroadcastFailThenSucceed(c *C) {
	m := GetMetricForTest(c)
	callCount := 0
	bridge := &mockBroadcastRetryBridge{
		keygenStdTxMsg: &ttypes.MsgTssPool{},
		broadcastFn: func() (common.TxID, error) {
			callCount++
			if callCount == 1 {
				return "", fmt.Errorf("broadcast fail")
			}
			return "TXID789", nil
		},
	}

	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		cfg:             config.Bifrost{},
		thorchainBridge: bridge,
		m:               m,
		errCounter:      m.GetCounterVec(metrics.SignerError),
	}

	err := sign.sendKeygenToThorchain(
		100,
		common.EmptyPubKey,
		nil,
		nil,
		common.PubKeys{},
		ttypes.KeygenType_AsgardKeygen,
		500,
		common.EmptyPubKey,
	)
	c.Assert(err, IsNil)
	c.Assert(callCount, Equals, 2) // failed once, succeeded on retry
}

func (s *SignCoverageSuite) TestSendKeygenToThorchain_WithChains(c *C) {
	m := GetMetricForTest(c)
	bridge := &mockSignBridge{
		keygenStdTxMsg: &ttypes.MsgTssPool{},
		broadcastTxID:  "TXID456",
	}

	// Configure chains: ETH active, BTC disabled, GAIA opt to retire
	cfg := config.Bifrost{}
	cfg.Chains.ETH = config.BifrostChainConfiguration{}
	cfg.Chains.BTC = config.BifrostChainConfiguration{Disabled: true}
	cfg.Chains.GAIA = config.BifrostChainConfiguration{OptToRetire: true}
	sign := &Signer{
		logger:          log.With().Str("module", "test").Logger(),
		cfg:             cfg,
		thorchainBridge: bridge,
		m:               m,
		errCounter:      m.GetCounterVec(metrics.SignerError),
	}

	err := sign.sendKeygenToThorchain(
		100,
		common.EmptyPubKey,
		[]byte("sig"),
		[]ttypes.Blame{{FailReason: "test"}},
		common.PubKeys{common.EmptyPubKey},
		ttypes.KeygenType_AsgardKeygen,
		500,
		common.EmptyPubKey,
	)
	c.Assert(err, IsNil)
}

func (s *SignCoverageSuite) TestSendKeygenToThorchain_WithBackupKeyshares(c *C) {
	m := GetMetricForTest(c)
	bridge := &mockSignBridge{
		keygenStdTxMsg: &ttypes.MsgTssPool{},
		broadcastTxID:  "TXID_BACKUP",
	}

	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	sign := &Signer{
		logger: log.With().Str("module", "test").Logger(),
		cfg: config.Bifrost{
			Signer: config.BifrostSignerConfiguration{BackupKeyshares: true},
		},
		thorchainBridge: bridge,
		m:               m,
		errCounter:      m.GetCounterVec(metrics.SignerError),
	}

	// Call with non-empty pubkeys, BackupKeyshares enabled
	// EncryptKeyshares will fail because SIGNER_SEED_PHRASE is not set, but error is logged & continued
	err := sign.sendKeygenToThorchain(
		100,
		vaultPubKey,
		nil,
		nil,
		common.PubKeys{},
		ttypes.KeygenType_AsgardKeygen,
		500,
		vaultPubKey,
	)
	c.Assert(err, IsNil)
}

// ----- processTransaction: storage remove error -----

func (s *SignCoverageSuite) TestProcessTransaction_StorageRemoveError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 200,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	// Create a storage, store an item, then close DB to make Remove fail
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	storage.Close()

	cc := &MockSuccessChainClient{}
	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
		storage:             storage,
		stopChan:            make(chan struct{}),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 190,
	}
	// processTransaction will sign successfully but fail to remove from storage
	// it just logs the error, doesn't return it
	sign.processTransaction(item)
}

// ----- processTransaction: round7 with storage error -----

func (s *SignCoverageSuite) TestProcessTransaction_Round7StorageError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 200,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	// Storage with closed DB to cause Set to fail
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	storage.Close()

	// Chain client that returns a round 7 keysign error
	cc := &MockRound7ErrorChainClient{}
	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
		storage:             storage,
		stopChan:            make(chan struct{}),
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 190,
	}
	// processTransaction hits round7 error, tries to Set item with Round7Retry=true
	// but storage is closed so Set fails - just logs the error
	sign.processTransaction(item)
}

// ----- signAndBroadcast: broadcast failure with storage error -----

func (s *SignCoverageSuite) TestSignAndBroadcast_BroadcastError_StoreError(c *C) {
	vaultPubKey, _ := common.NewPubKey(pubkeymanager.MockPubkey)
	bridge := &mockSignBridge{
		blockHeight: 200,
		constants: map[string]int64{
			constants.SigningTransactionPeriod.String(): 300,
			constants.ChurnInterval.String():            43200,
		},
		mimirs:  map[string]int64{},
		keysign: types.TxOut{},
	}

	// Use a storage that will fail on Set
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	// Close the db to cause Set to fail
	storage.Close()

	cc := &MockSuccessChainClient{broadcastErr: fmt.Errorf("broadcast failed")}
	sign := &Signer{
		logger:              log.With().Str("module", "test").Logger(),
		cfg:                 config.Bifrost{Signer: config.BifrostSignerConfiguration{RescheduleBufferBlocks: 100}},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		chains:              map[common.Chain]chainclients.ChainClient{common.ETHChain: cc},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		m:                   GetMetricForTest(c),
		localPubKeyECDSA:    vaultPubKey,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
		storage:             storage,
	}

	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		},
		Height: 190,
	}
	_, _, err = sign.signAndBroadcast(item)
	// Broadcast fails, and Set also fails but that's logged, the broadcast error is returned
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*broadcast failed.*")
}

// ----- storage error tests -----

func (s *SignCoverageSuite) TestStorageSet_ClosedDB(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	storage.Close()

	item := NewTxOutStoreItem(100, types.TxOutItem{
		Chain:     common.ETHChain,
		ToAddress: "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
	}, 0)
	err = storage.Set(item)
	c.Assert(err, NotNil)
}

func (s *SignCoverageSuite) TestStorageBatch_ClosedDB(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	storage.Close()

	items := []TxOutStoreItem{
		NewTxOutStoreItem(100, types.TxOutItem{
			Chain:     common.ETHChain,
			ToAddress: "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
		}, 0),
	}
	err = storage.Batch(items)
	c.Assert(err, NotNil)
}

func (s *SignCoverageSuite) TestStorageGet_UnmarshalError(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	// Write raw invalid JSON data directly
	key := "txout-v4-test-invalid"
	err = storage.db.Put([]byte(key), []byte("not-json"), nil)
	c.Assert(err, IsNil)

	_, err = storage.Get(key)
	c.Assert(err, NotNil)
}

func (s *SignCoverageSuite) TestStorageRemove_ClosedDB(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	storage.Close()

	item := NewTxOutStoreItem(100, types.TxOutItem{
		Chain:     common.ETHChain,
		ToAddress: "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
	}, 0)
	err = storage.Remove(item)
	c.Assert(err, NotNil)
}
