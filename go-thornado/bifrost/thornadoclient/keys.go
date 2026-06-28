package thornadoclient

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sync"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	ckeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptokeysed25519 "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptokeyssecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/common/crypto/ed25519"
)

const (
	// folder name for thornado thorcli
	thornadoCliFolderName = `.thornado`
)

var stdinMu sync.Mutex

type Keyring interface {
	ckeys.Keyring
}

// Keys manages all the keys used by thornado
type Keys struct {
	signerName string
	password   string // TODO this is a bad way , need to fix it
	kb         Keyring
	signerMu   sync.Mutex
	signerInfo *ckeys.Record
}

type passwordReader struct {
	mu     sync.Mutex
	data   []byte
	offset int
}

func newPasswordReader(password string) *passwordReader {
	return &passwordReader{data: []byte(password + "\n")}
}

func (r *passwordReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range p {
		p[i] = r.data[r.offset]
		r.offset = (r.offset + 1) % len(r.data)
	}
	return len(p), nil
}

func withNonInteractiveStdin(fn func() error) error {
	stdinMu.Lock()
	defer stdinMu.Unlock()
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer devNull.Close()
	oldStdIn := os.Stdin
	os.Stdin = devNull
	defer func() {
		os.Stdin = oldStdIn
	}()
	return fn()
}

// NewKeysWithKeybase create a new instance of Keys
func NewKeysWithKeybase(kb ckeys.Keyring, name, password string) *Keys {
	return &Keys{
		signerName: name,
		password:   password,
		kb:         kb,
	}
}

// GetKeyringKeybase return keyring and key info
func GetKeyringKeybase(chainHomeFolder, signerName, password string) (ckeys.Keyring, *ckeys.Record, error) {
	if len(signerName) == 0 {
		return nil, nil, fmt.Errorf("signer name is empty")
	}
	if len(password) == 0 {
		return nil, nil, fmt.Errorf("password is empty")
	}

	buf := newPasswordReader(password)
	kb, err := getKeybase(chainHomeFolder, buf)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to get keybase,err:%w", err)
	}
	var si *ckeys.Record
	if err := withNonInteractiveStdin(func() error {
		var err error
		si, err = kb.Key(signerName)
		return err
	}); err != nil {
		return nil, nil, fmt.Errorf("fail to get signer info(%s): %w", signerName, err)
	}
	return kb, si, nil
}

// getKeybase will create an instance of Keybase
func getKeybase(thornadoHome string, reader io.Reader) (ckeys.Keyring, error) {
	cliDir := thornadoHome
	if len(thornadoHome) == 0 {
		usr, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("fail to get current user,err:%w", err)
		}
		cliDir = filepath.Join(usr.HomeDir, thornadoCliFolderName)
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	return ckeys.New(sdk.KeyringServiceName(), ckeys.BackendFile, cliDir, reader, cdc)
}

// GetSignerInfo return signer info
func (k *Keys) GetSignerInfo() *ckeys.Record {
	k.signerMu.Lock()
	defer k.signerMu.Unlock()
	if k.signerInfo != nil {
		return k.signerInfo
	}
	var record *ckeys.Record
	if err := withNonInteractiveStdin(func() error {
		var err error
		record, err = k.kb.Key(k.signerName)
		return err
	}); err != nil {
		panic(err)
	}
	k.signerInfo = record
	return k.signerInfo
}

// GetPrivateKey return the ecdsa private key
func (k *Keys) GetPrivateKey() (*cryptokeyssecp256k1.PrivKey, error) {
	// return k.kb.ExportPrivateKeyObject(k.signerName)
	var privKeyArmor string
	if err := withNonInteractiveStdin(func() error {
		var err error
		privKeyArmor, err = k.kb.ExportPrivKeyArmor(k.signerName, k.password)
		return err
	}); err != nil {
		return nil, err
	}
	priKey, _, err := crypto.UnarmorDecryptPrivKey(privKeyArmor, k.password)
	if err != nil {
		return nil, fmt.Errorf("fail to unarmor private key: %w", err)
	}
	secpKey, ok := priKey.(*cryptokeyssecp256k1.PrivKey)
	if !ok {
		return nil, fmt.Errorf("fail to cast private key to secp256k1 private key")
	}
	return secpKey, nil
}

// GetPrivateKeyEDDSA return the eddsa private key
func (k *Keys) GetPrivateKeyEDDSA() (*cryptokeysed25519.PrivKey, error) {
	signerNameEDDSA := ed25519.SignerNameEDDSA(k.signerName)
	// return k.kb.ExportPrivateKeyObject(k.signerName)
	var privKeyArmor string
	if err := withNonInteractiveStdin(func() error {
		var err error
		privKeyArmor, err = k.kb.ExportPrivKeyArmor(signerNameEDDSA, k.password)
		return err
	}); err != nil {
		return nil, err
	}
	priKey, _, err := crypto.UnarmorDecryptPrivKey(privKeyArmor, k.password)
	if err != nil {
		return nil, fmt.Errorf("fail to unarmor private key: %w", err)
	}
	eddsaKey, ok := priKey.(*cryptokeysed25519.PrivKey)
	if !ok {
		return nil, fmt.Errorf("fail to cast private key to eddsa private key")
	}
	return eddsaKey, nil
}

// GetKeybase return the keybase
func (k *Keys) GetKeybase() ckeys.Keyring {
	return k.kb
}
