package tss

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/thorclient"
	tctypes "github.com/thornadocash/go-thornado/bifrost/thorclient/types"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/blame"
	tsscommon "github.com/thornadocash/go-thornado/bifrost/tss/go-tss/common"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	stypes "github.com/thornadocash/go-thornado/x/thorchain/types"
)

type KeySignTestSuite struct{}

var _ = Suite(&KeySignTestSuite{})

// ---------------------------------------------------------------------------
// Mock tssServer
// ---------------------------------------------------------------------------

type mockTssServer struct {
	resp keysign.Response
	err  error
}

func (m *mockTssServer) KeySign(req keysign.Request) (keysign.Response, error) {
	return m.resp, m.err
}

// ---------------------------------------------------------------------------
// Mock ThorchainBridge (full interface)
// ---------------------------------------------------------------------------

type mockBridgeForKeysign struct {
	version     semver.Version
	versionErr  error
	blockHeight int64
	blockErr    error
}

var _ thorclient.ThorchainBridge = (*mockBridgeForKeysign)(nil)

func (m *mockBridgeForKeysign) EnsureNodeWhitelisted() error                { return nil }
func (m *mockBridgeForKeysign) EnsureNodeWhitelistedWithTimeout() error     { return nil }
func (m *mockBridgeForKeysign) FetchNodeStatus() (stypes.NodeStatus, error) { return 0, nil }
func (m *mockBridgeForKeysign) FetchActiveNodes() ([]common.PubKey, error)  { return nil, nil }
func (m *mockBridgeForKeysign) GetAsgards() (stypes.Vaults, error)          { return nil, nil }
func (m *mockBridgeForKeysign) GetVault(string) (stypes.Vault, error) {
	return stypes.Vault{}, nil
}

func (m *mockBridgeForKeysign) GetConfig() config.BifrostClientConfiguration {
	return config.BifrostClientConfiguration{}
}
func (m *mockBridgeForKeysign) GetConstants() (map[string]int64, error) { return nil, nil }
func (m *mockBridgeForKeysign) GetContext() client.Context              { return client.Context{} }
func (m *mockBridgeForKeysign) GetContractAddress() ([]thorclient.PubKeyContractAddressPair, error) {
	return nil, nil
}
func (m *mockBridgeForKeysign) GetErrataMsg(common.TxID, common.Chain) sdk.Msg { return nil }
func (m *mockBridgeForKeysign) GetKeygenStdTx(common.PubKey, []byte, []byte, []stypes.Blame, common.PubKeys, stypes.KeygenType, common.Chains, int64, int64, common.PubKey, []byte) (sdk.Msg, error) {
	return nil, nil
}

func (m *mockBridgeForKeysign) GetKeysignParty(common.PubKey) (common.PubKeys, error) {
	return nil, nil
}
func (m *mockBridgeForKeysign) GetMimir(string) (int64, error)                { return 0, nil }
func (m *mockBridgeForKeysign) GetMimirWithRef(string, string) (int64, error) { return 0, nil }
func (m *mockBridgeForKeysign) GetInboundOutbound(common.ObservedTxs) (common.ObservedTxs, common.ObservedTxs, error) {
	return nil, nil, nil
}
func (m *mockBridgeForKeysign) GetPubKeys() ([]thorclient.PubKeyContractAddressPair, error) {
	return nil, nil
}

func (m *mockBridgeForKeysign) GetAsgardPubKeys() ([]thorclient.PubKeyContractAddressPair, error) {
	return nil, nil
}

func (m *mockBridgeForKeysign) GetSolvencyMsg(int64, common.Chain, common.PubKey, common.Coins) *stypes.MsgSolvency {
	return nil
}

func (m *mockBridgeForKeysign) GetThorchainVersion() (semver.Version, error) {
	return m.version, m.versionErr
}
func (m *mockBridgeForKeysign) IsCatchingUp() (bool, error)              { return false, nil }
func (m *mockBridgeForKeysign) HasNetworkFee(common.Chain) (bool, error) { return false, nil }
func (m *mockBridgeForKeysign) GetNetworkFee(common.Chain) (uint64, uint64, error) {
	return 0, 0, nil
}

func (m *mockBridgeForKeysign) PostKeysignFailure(stypes.Blame, int64, string, common.Coins, common.PubKey) (common.TxID, error) {
	return "", nil
}

func (m *mockBridgeForKeysign) PostNetworkFee(int64, common.Chain, uint64, uint64) (common.TxID, error) {
	return "", nil
}
func (m *mockBridgeForKeysign) RagnarokInProgress() (bool, error) { return false, nil }
func (m *mockBridgeForKeysign) WaitToCatchUp() error              { return nil }
func (m *mockBridgeForKeysign) GetBlockHeight() (int64, error) {
	return m.blockHeight, m.blockErr
}

func (m *mockBridgeForKeysign) GetBlockTimestamp(int64) (time.Time, error) {
	return time.Time{}, nil
}
func (m *mockBridgeForKeysign) GetLastObservedInHeight(common.Chain) (int64, error) { return 0, nil }
func (m *mockBridgeForKeysign) GetLastSignedOutHeight(common.Chain) (int64, error)  { return 0, nil }
func (m *mockBridgeForKeysign) Broadcast(...sdk.Msg) (common.TxID, error)           { return "", nil }
func (m *mockBridgeForKeysign) BroadcastWithBlocking(...sdk.Msg) (common.TxID, error) {
	return "", nil
}

func (m *mockBridgeForKeysign) GetKeysign(int64, string) (tctypes.TxOut, error) {
	return tctypes.TxOut{}, nil
}
func (m *mockBridgeForKeysign) GetNodeAccount(string) (*stypes.NodeAccount, error) { return nil, nil }
func (m *mockBridgeForKeysign) GetNodeAccounts() ([]*stypes.QueryNodeResponse, error) {
	return nil, nil
}

func (m *mockBridgeForKeysign) GetKeygenBlock(int64, string) (stypes.KeygenBlock, error) {
	return stypes.KeygenBlock{}, nil
}

func (m *mockBridgeForKeysign) GetReferenceMemo(common.Chain, string) (string, error) {
	return "", nil
}
func (m *mockBridgeForKeysign) GetReferenceMemoByTxHash(string) (string, error) { return "", nil }

// ---------------------------------------------------------------------------
// KeysignError Tests
// ---------------------------------------------------------------------------

type KeysignErrorSuite struct{}

var _ = Suite(&KeysignErrorSuite{})

func (s *KeysignErrorSuite) TestNewKeysignError(c *C) {
	b := stypes.Blame{
		FailReason: "test reason",
		Round:      "round1",
		BlameNodes: []stypes.Node{{Pubkey: "pk1"}},
	}
	ke := NewKeysignError(b)
	c.Assert(ke.Blame, DeepEquals, b)
}

func (s *KeysignErrorSuite) TestKeysignErrorString(c *C) {
	b := stypes.Blame{
		FailReason: "something went wrong",
		Round:      "round2",
		BlameNodes: []stypes.Node{{Pubkey: "pk1"}, {Pubkey: "pk2"}},
	}
	ke := NewKeysignError(b)
	errStr := ke.Error()
	c.Assert(errStr, Matches, ".*something went wrong.*round2.*pk1.*pk2.*")
}

func (s *KeysignErrorSuite) TestIsRound7(c *C) {
	ke := NewKeysignError(stypes.Blame{Round: "SignRound7Message"})
	c.Assert(ke.IsRound7(), Equals, true)

	ke2 := NewKeysignError(stypes.Blame{Round: "SignRound3Message"})
	c.Assert(ke2.IsRound7(), Equals, false)

	ke3 := NewKeysignError(stypes.Blame{})
	c.Assert(ke3.IsRound7(), Equals, false)
}

// ---------------------------------------------------------------------------
// KeySign (tss_signer) Tests
// ---------------------------------------------------------------------------

func (s *KeySignTestSuite) TestNewKeySign(c *C) {
	server := &mockTssServer{}
	bridge := &mockBridgeForKeysign{}
	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	c.Assert(ks, NotNil)
	c.Assert(ks.server, Equals, server)
}

func (s *KeySignTestSuite) TestGetPrivKey(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	c.Assert(ks.GetPrivKey(), IsNil)
}

func (s *KeySignTestSuite) TestGetAddr(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	c.Assert(ks.GetAddr(), IsNil)
}

func (s *KeySignTestSuite) TestExportAsMnemonic(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	mnemonic, err := ks.ExportAsMnemonic()
	c.Assert(err, IsNil)
	c.Assert(mnemonic, Equals, "")
}

func (s *KeySignTestSuite) TestExportAsPrivateKey(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	pk, err := ks.ExportAsPrivateKey()
	c.Assert(err, IsNil)
	c.Assert(pk, Equals, "")
}

func (s *KeySignTestSuite) TestExportAsKeyStore(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	keyStore, err := ks.ExportAsKeyStore("password")
	c.Assert(err, IsNil)
	c.Assert(keyStore, IsNil)
}

func (s *KeySignTestSuite) TestGetSignature(c *C) {
	rVal := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20})
	sVal := base64.StdEncoding.EncodeToString([]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40})
	sig, err := getSignature(rVal, sVal)
	c.Assert(err, IsNil)
	c.Assert(len(sig), Equals, 64)

	// Invalid R base64
	_, err = getSignature("invalid-base64!!!", sVal)
	c.Assert(err, NotNil)

	// Invalid S base64
	_, err = getSignature(rVal, "invalid-base64!!!")
	c.Assert(err, NotNil)
}

func (s *KeySignTestSuite) TestGetSignatureHighS(c *C) {
	rBytes := make([]byte, 32)
	rBytes[0] = 0x01
	rVal := base64.StdEncoding.EncodeToString(rBytes)

	// S value larger than halfOrder should be normalized
	sBytes := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE, 0xBA, 0xAE, 0xDC, 0xE6, 0xAF, 0x48, 0xA0, 0x3B, 0xBF, 0xD2, 0x5E, 0x8C, 0xD0, 0x36, 0x41, 0x40}
	sVal := base64.StdEncoding.EncodeToString(sBytes)
	sig, err := getSignature(rVal, sVal)
	c.Assert(err, IsNil)
	c.Assert(len(sig), Equals, 64)
}

func (s *KeySignTestSuite) TestRemoteSignEmptyMsg(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	sig, recoveryID, err := ks.RemoteSign(nil, common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, IsNil)
	c.Assert(sig, IsNil)
	c.Assert(recoveryID, IsNil)

	sig, recoveryID, err = ks.RemoteSign([]byte{}, common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, IsNil)
	c.Assert(sig, IsNil)
	c.Assert(recoveryID, IsNil)
}

func (s *KeySignTestSuite) TestRemoteSignSecp256k1Success(c *C) {
	msg := []byte("test msg")
	encodedMsg := base64.StdEncoding.EncodeToString(msg)

	rBytes := make([]byte, 32)
	rBytes[0] = 0x01
	sBytes := make([]byte, 32)
	sBytes[0] = 0x02
	rB64 := base64.StdEncoding.EncodeToString(rBytes)
	sB64 := base64.StdEncoding.EncodeToString(sBytes)
	recoveryB64 := base64.StdEncoding.EncodeToString([]byte{0x00})

	server := &mockTssServer{
		resp: keysign.Response{
			Signatures: []keysign.Signature{
				{Msg: encodedMsg, R: rB64, S: sB64, RecoveryID: recoveryB64},
			},
			Status: tsscommon.Success,
		},
	}

	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	sig, recovery, err := ks.RemoteSign(msg, common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, IsNil)
	c.Assert(sig, NotNil)
	c.Assert(len(sig), Equals, 64)
	c.Assert(recovery, NotNil)
}

func (s *KeySignTestSuite) TestRemoteSignEd25519Success(c *C) {
	msg := []byte("test msg")
	encodedMsg := base64.StdEncoding.EncodeToString(msg)

	sigBytes := make([]byte, 64)
	sigBytes[0] = 0x01
	sB64 := base64.StdEncoding.EncodeToString(sigBytes)

	server := &mockTssServer{
		resp: keysign.Response{
			Signatures: []keysign.Signature{
				{Msg: encodedMsg, S: sB64},
			},
			Status: tsscommon.Success,
		},
	}

	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	sig, recovery, err := ks.RemoteSign(msg, common.SigningAlgoEd25519, "poolpub")
	c.Assert(err, IsNil)
	c.Assert(sig, NotNil)
	c.Assert(len(sig), Equals, 64)
	c.Assert(recovery, IsNil)
}

func (s *KeySignTestSuite) TestRemoteSignError(c *C) {
	server := &mockTssServer{
		err: fmt.Errorf("keysign failed"),
	}

	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	_, _, err = ks.RemoteSign([]byte("test msg"), common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to tss sign.*")
}

func (s *KeySignTestSuite) TestRemoteSignBlame(c *C) {
	server := &mockTssServer{
		resp: keysign.Response{
			Status: tsscommon.Fail,
			Blame: blame.Blame{
				FailReason: "timeout",
				Round:      "round1",
				BlameNodes: []blame.Node{{Pubkey: "pk1"}},
			},
		},
	}

	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	_, _, err = ks.RemoteSign([]byte("test msg"), common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, NotNil)
	// The error is wrapped via fmt.Errorf, so use errors.As
	var ke KeysignError
	c.Assert(errors.As(err, &ke), Equals, true)
	c.Assert(ke.Blame.FailReason, Equals, "timeout")
	c.Assert(len(ke.Blame.BlameNodes), Equals, 1)
}

func (s *KeySignTestSuite) TestRemoteSignNotSelected(c *C) {
	msg := []byte("test msg")
	encodedMsg := base64.StdEncoding.EncodeToString(msg)

	server := &mockTssServer{
		resp: keysign.Response{
			Signatures: []keysign.Signature{
				{Msg: encodedMsg, R: "", S: ""},
			},
			Status: tsscommon.Success,
		},
	}

	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	sig, recovery, err := ks.RemoteSign(msg, common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, IsNil)
	c.Assert(sig, IsNil)
	c.Assert(recovery, IsNil)
}

func (s *KeySignTestSuite) TestRemoteSignBlockHeightError(c *C) {
	server := &mockTssServer{}
	bridge := &mockBridgeForKeysign{
		version:  semver.MustParse("1.0.0"),
		blockErr: fmt.Errorf("fail to get block height"),
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	_, _, err = ks.RemoteSign([]byte("test msg"), common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to get block height.*")
}

func (s *KeySignTestSuite) TestRemoteSignMsgNotFound(c *C) {
	rBytes := make([]byte, 32)
	rBytes[0] = 0x01
	sBytes := make([]byte, 32)
	sBytes[0] = 0x02
	rB64 := base64.StdEncoding.EncodeToString(rBytes)
	sB64 := base64.StdEncoding.EncodeToString(sBytes)

	server := &mockTssServer{
		resp: keysign.Response{
			Signatures: []keysign.Signature{
				{Msg: "different_msg", R: rB64, S: sB64, RecoveryID: base64.StdEncoding.EncodeToString([]byte{0x00})},
			},
			Status: tsscommon.Success,
		},
	}

	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}

	ks, err := NewKeySign(server, bridge)
	c.Assert(err, IsNil)
	ks.Start()
	defer ks.Stop()

	_, _, err = ks.RemoteSign([]byte("test msg"), common.SigningAlgoSecp256k1, "poolpub")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*didn't find signature.*")
}

func (s *KeySignTestSuite) TestGetVersionCached(c *C) {
	bridge := &mockBridgeForKeysign{
		version: semver.MustParse("2.0.0"),
	}
	ks, _ := NewKeySign(&mockTssServer{}, bridge)

	v := ks.getVersion()
	c.Assert(v.String(), Equals, "2.0.0")

	// Second call within block time should use cache
	bridge.version = semver.MustParse("3.0.0")
	v = ks.getVersion()
	c.Assert(v.String(), Equals, "2.0.0")
}

func (s *KeySignTestSuite) TestGetVersionError(c *C) {
	bridge := &mockBridgeForKeysign{
		versionErr: fmt.Errorf("fail"),
	}
	ks, _ := NewKeySign(&mockTssServer{}, bridge)

	v := ks.getVersion()
	c.Assert(v.String(), Equals, "0.0.0")
}

func (s *KeySignTestSuite) TestSetTssKeySignTasksFail(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})

	tasks := []*tssKeySignTask{
		{Resp: make(chan tssKeySignResult, 1)},
		{Resp: make(chan tssKeySignResult, 1)},
	}

	testErr := fmt.Errorf("test error")
	ks.setTssKeySignTasksFail(tasks, testErr)

	for _, t := range tasks {
		select {
		case result := <-t.Resp:
			c.Assert(result.Err, Equals, testErr)
		default:
			c.Fatal("expected result on channel")
		}
	}
}

func (s *KeySignTestSuite) TestStartStop(c *C) {
	ks, _ := NewKeySign(&mockTssServer{}, &mockBridgeForKeysign{})
	ks.Start()
	time.Sleep(50 * time.Millisecond)
	ks.Stop()
}

func (s *KeySignTestSuite) TestToLocalTSSSignerSuccess(c *C) {
	msg1 := base64.StdEncoding.EncodeToString([]byte("msg1"))

	rBytes := make([]byte, 32)
	rBytes[0] = 0x01
	sBytes := make([]byte, 32)
	sBytes[0] = 0x02
	rB64 := base64.StdEncoding.EncodeToString(rBytes)
	sB64 := base64.StdEncoding.EncodeToString(sBytes)
	recoveryB64 := base64.StdEncoding.EncodeToString([]byte{0x00})

	server := &mockTssServer{
		resp: keysign.Response{
			Signatures: []keysign.Signature{
				{Msg: msg1, R: rB64, S: sB64, RecoveryID: recoveryB64},
			},
			Status: tsscommon.Success,
		},
	}
	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}
	ks, _ := NewKeySign(server, bridge)

	tasks := []*tssKeySignTask{
		{PoolPubKey: "poolpub", Algo: common.SigningAlgoSecp256k1, Msg: msg1, Resp: make(chan tssKeySignResult, 1)},
	}

	ks.wg.Add(1)
	ks.toLocalTSSSigner("poolpub", common.SigningAlgoSecp256k1, tasks)

	result := <-tasks[0].Resp
	c.Assert(result.Err, IsNil)
	c.Assert(result.R, Equals, rB64)
	c.Assert(result.S, Equals, sB64)
}

func (s *KeySignTestSuite) TestToLocalTSSSignerServerError(c *C) {
	server := &mockTssServer{
		err: fmt.Errorf("server error"),
	}
	bridge := &mockBridgeForKeysign{
		version:     semver.MustParse("1.0.0"),
		blockHeight: 100,
	}
	ks, _ := NewKeySign(server, bridge)

	tasks := []*tssKeySignTask{
		{PoolPubKey: "poolpub", Algo: common.SigningAlgoSecp256k1, Msg: "msg1", Resp: make(chan tssKeySignResult, 1)},
	}

	ks.wg.Add(1)
	ks.toLocalTSSSigner("poolpub", common.SigningAlgoSecp256k1, tasks)

	result := <-tasks[0].Resp
	c.Assert(result.Err, NotNil)
	c.Assert(result.Err.Error(), Matches, ".*fail tss keysign.*")
}

func (s *KeySignTestSuite) TestToLocalTSSSignerBlockHeightError(c *C) {
	server := &mockTssServer{}
	bridge := &mockBridgeForKeysign{
		version:  semver.MustParse("1.0.0"),
		blockErr: fmt.Errorf("block height error"),
	}
	ks, _ := NewKeySign(server, bridge)

	tasks := []*tssKeySignTask{
		{PoolPubKey: "poolpub", Algo: common.SigningAlgoSecp256k1, Msg: "msg1", Resp: make(chan tssKeySignResult, 1)},
	}

	ks.wg.Add(1)
	ks.toLocalTSSSigner("poolpub", common.SigningAlgoSecp256k1, tasks)

	result := <-tasks[0].Resp
	c.Assert(result.Err, NotNil)
	c.Assert(result.Err.Error(), Matches, ".*fail to get block height.*")
}

// ---------------------------------------------------------------------------
// encryptAES / decryptAES tests
// ---------------------------------------------------------------------------

type AESTestSuite struct{}

var _ = Suite(&AESTestSuite{})

func (s *AESTestSuite) TestEncryptDecryptAES(c *C) {
	key := make([]byte, 32)
	key[0] = 0x01
	data := []byte("hello world test data for aes encryption")

	encrypted, err := encryptAES(key, data)
	c.Assert(err, IsNil)
	c.Assert(encrypted, Not(DeepEquals), data)

	decrypted, err := decryptAES(key, encrypted)
	c.Assert(err, IsNil)
	c.Assert(string(decrypted), Equals, string(data))
}

func (s *AESTestSuite) TestEncryptAESBadKeyLen(c *C) {
	key := make([]byte, 16)
	_, err := encryptAES(key, []byte("data"))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "failed to encrypt: key must be 32 bytes")
}

func (s *AESTestSuite) TestDecryptAESBadKeyLen(c *C) {
	key := make([]byte, 16)
	_, err := decryptAES(key, []byte("data"))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "failed to decrypt: key must be 32 bytes")
}

func (s *AESTestSuite) TestDecryptAESBadData(c *C) {
	key := make([]byte, 32)
	key[0] = 0x01
	// Data needs to be at least nonce size (12 for AES-GCM) + some ciphertext, but garbage
	badData := make([]byte, 32)
	for i := range badData {
		badData[i] = byte(i)
	}
	_, err := decryptAES(key, badData)
	c.Assert(err, NotNil)
}

func (s *AESTestSuite) TestDecryptAEADBadCiphertext(c *C) {
	key := make([]byte, 32)
	key[0] = 0x01

	encrypted, err := encryptAES(key, []byte("test data"))
	c.Assert(err, IsNil)

	encrypted[len(encrypted)-1] ^= 0xFF
	_, err = decryptAES(key, encrypted)
	c.Assert(err, NotNil)
}
