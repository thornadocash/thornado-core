package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/addr"
	tnet "github.com/libp2p/go-libp2p-testing/net"
	maddr "github.com/multiformats/go-multiaddr"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
)

type FileStateMgrExtraSuite struct{}

var _ = Suite(&FileStateMgrExtraSuite{})

func (s *FileStateMgrExtraSuite) SetUpSuite(c *C) {
	conversion.SetupBech32Prefix()
}

// ── NewFileStateMgr ──────────────────────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestNewFileStateMgr_EmptyFolder(c *C) {
	fsm, err := NewFileStateMgr("")
	c.Assert(err, IsNil)
	c.Assert(fsm, NotNil)
	c.Assert(fsm.folder, Equals, "")
}

// ── getFilePathName with empty folder ────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestGetFilePathName_EmptyFolder(c *C) {
	fsm, err := NewFileStateMgr("")
	c.Assert(err, IsNil)

	// Valid pubkey with no folder should return just the filename
	fname, err := fsm.getFilePathName("thorpub1addwnpepqf90u7n3nr2jwsw4t2gzhzqfdlply8dlzv3mdj4dr22uvhe04azq5gac3gq")
	c.Assert(err, IsNil)
	c.Assert(fname, Equals, "localstate-thorpub1addwnpepqf90u7n3nr2jwsw4t2gzhzqfdlply8dlzv3mdj4dr22uvhe04azq5gac3gq.json")
}

func (s *FileStateMgrExtraSuite) TestGetFilePathName_InvalidNotOnCurve(c *C) {
	fsm, err := NewFileStateMgr("")
	c.Assert(err, IsNil)

	// Invalid pubkey
	_, err = fsm.getFilePathName("notavalidkey")
	c.Assert(err, NotNil)
}

// ── GetLocalState error paths ────────────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestGetLocalState_EmptyPubKey(c *C) {
	fsm, err := NewFileStateMgr(c.MkDir())
	c.Assert(err, IsNil)

	_, err = fsm.GetLocalState("")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "pub key is empty")
}

func (s *FileStateMgrExtraSuite) TestGetLocalState_FileNotExist(c *C) {
	fsm, err := NewFileStateMgr(c.MkDir())
	c.Assert(err, IsNil)

	_, err = fsm.GetLocalState("thorpub1addwnpepqf90u7n3nr2jwsw4t2gzhzqfdlply8dlzv3mdj4dr22uvhe04azq5gac3gq")
	c.Assert(err, NotNil)
}

func (s *FileStateMgrExtraSuite) TestGetLocalState_BadJSON(c *C) {
	dir := c.MkDir()
	fsm, err := NewFileStateMgr(dir)
	c.Assert(err, IsNil)

	// Write invalid JSON to the expected file
	pubKey := "thorpub1addwnpepqf90u7n3nr2jwsw4t2gzhzqfdlply8dlzv3mdj4dr22uvhe04azq5gac3gq"
	fname := filepath.Join(dir, "localstate-"+pubKey+".json")
	err = os.WriteFile(fname, []byte("not-json{{{"), 0o600)
	c.Assert(err, IsNil)

	_, err = fsm.GetLocalState(pubKey)
	c.Assert(err, NotNil)
}

// ── SaveAddressBook error paths ──────────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestSaveAddressBook_EmptyFolder(c *C) {
	fsm, err := NewFileStateMgr("")
	c.Assert(err, IsNil)

	err = fsm.SaveAddressBook(map[peer.ID]addr.AddrList{})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "base file path is invalid")
}

func (s *FileStateMgrExtraSuite) TestSaveAddressBook_SkipsLoopback(c *C) {
	dir := c.MkDir()
	fsm, err := NewFileStateMgr(dir)
	c.Assert(err, IsNil)

	// Create a loopback address and a non-loopback
	loopback, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/6668")
	c.Assert(err, IsNil)
	nonLoopback, err := maddr.NewMultiaddr("/ip4/192.168.1.1/tcp/6668")
	c.Assert(err, IsNil)

	// Use a real peer ID from tnet
	id := tnet.RandIdentityOrFatal(nil)

	testAddr := make(map[peer.ID]addr.AddrList)
	testAddr[id.ID()] = []maddr.Multiaddr{loopback, nonLoopback}

	err = fsm.SaveAddressBook(testAddr)
	c.Assert(err, IsNil)

	// Read back: loopback should be skipped, only non-loopback saved
	addrs, err := fsm.RetrieveP2PAddresses()
	c.Assert(err, IsNil)
	c.Assert(addrs, HasLen, 1)
}

// ── RetrieveP2PAddresses error paths ─────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestRetrieveP2PAddresses_EmptyFolder(c *C) {
	fsm, err := NewFileStateMgr("")
	c.Assert(err, IsNil)

	_, err = fsm.RetrieveP2PAddresses()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "base file path is invalid")
}

func (s *FileStateMgrExtraSuite) TestRetrieveP2PAddresses_FileNotExist(c *C) {
	dir := c.MkDir()
	fsm, err := NewFileStateMgr(dir)
	c.Assert(err, IsNil)

	_, err = fsm.RetrieveP2PAddresses()
	c.Assert(err, NotNil) // file doesn't exist
}

func (s *FileStateMgrExtraSuite) TestRetrieveP2PAddresses_InvalidMultiaddr(c *C) {
	dir := c.MkDir()
	fsm, err := NewFileStateMgr(dir)
	c.Assert(err, IsNil)

	// Write invalid multiaddr to the file
	fname := filepath.Join(dir, "address_book.seed")
	err = os.WriteFile(fname, []byte("not-a-valid-multiaddr\n"), 0o600)
	c.Assert(err, IsNil)

	_, err = fsm.RetrieveP2PAddresses()
	c.Assert(err, NotNil)
}

// ── KeygenLocalState UnmarshalJSON ───────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestUnmarshalJSON_NewFormat(c *C) {
	state := KeygenLocalState{
		PubKey:          "testpub",
		LocalData:       []byte(`{"key":"value"}`),
		ParticipantKeys: []string{"A", "B"},
		LocalPartyKey:   "A",
	}
	data, err := json.Marshal(state)
	c.Assert(err, IsNil)

	var result KeygenLocalState
	err = json.Unmarshal(data, &result)
	c.Assert(err, IsNil)
	c.Assert(result.PubKey, Equals, "testpub")
	c.Assert(result.LocalPartyKey, Equals, "A")
}

func (s *FileStateMgrExtraSuite) TestUnmarshalJSON_InvalidBothFormats(c *C) {
	var result KeygenLocalState
	err := json.Unmarshal([]byte(`not valid json at all`), &result)
	c.Assert(err, NotNil)
}

// ── MockLocalStateManager ────────────────────────────────────────────────

func (s *FileStateMgrExtraSuite) TestMockLocalStateManager(c *C) {
	var mgr LocalStateManager = &MockLocalStateManager{}

	err := mgr.SaveLocalState(KeygenLocalState{})
	c.Assert(err, IsNil)

	state, err := mgr.GetLocalState("test")
	c.Assert(err, IsNil)
	c.Assert(state, DeepEquals, KeygenLocalState{})

	err = mgr.SaveAddressBook(nil)
	c.Assert(err, IsNil)

	addrs, err := mgr.RetrieveP2PAddresses()
	c.Assert(err, IsNil)
	c.Assert(addrs, IsNil)
}
