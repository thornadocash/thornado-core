// Please put all the test related function to here
package types

import (
	"math/rand"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"github.com/thornadocash/go-thornado/cmd"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

// GetRandomNode creates a random node node account, used for testing
func GetRandomNode(status NodeStatus) NodeAccount {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s) // #nosec G404 this is a method only used for test purpose
	accts := simtypes.RandomAccounts(r, 1)

	k, _ := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeConsPub, accts[0].PubKey)
	pubKeys := common.PubKeySet{
		Secp256k1: GetRandomPubKey(),
	}
	addr, _ := pubKeys.Secp256k1.GetThorAddress()
	bondAddr := common.Address(addr.String())
	na := NewNodeAccount(addr, status, pubKeys, k, cosmos.NewUint(100*common.One), bondAddr, 1)
	na.Version = constants.SWVersion.String()
	if na.Status == NodeStatus_Active {
		na.ActiveBlockHeight = 10
		na.Bond = cosmos.NewUint(1000 * common.One)
	}
	na.IPAddress = "192.168.0.1"
	na.Type = NodeType_TypeNode

	return na
}

// GetRandomVaultNode creates a random vault node account, used for testing
func GetRandomVaultNode(status NodeStatus) NodeAccount {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s) // #nosec G404 this is a method only used for test purpose
	accts := simtypes.RandomAccounts(r, 1)

	k, _ := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeConsPub, accts[0].PubKey)
	pubKeys := common.PubKeySet{
		Secp256k1: GetRandomPubKey(),
	}
	addr, _ := pubKeys.Secp256k1.GetThorAddress()
	bondAddr := common.Address(addr.String())
	na := NewNodeAccount(addr, status, pubKeys, k, cosmos.NewUint(100*common.One), bondAddr, 1)
	na.Version = constants.SWVersion.String()
	if na.Status == NodeStatus_Active {
		na.ActiveBlockHeight = 10
		na.Bond = cosmos.NewUint(1000 * common.One)
	}
	na.IPAddress = "192.168.0.1"
	na.Type = NodeType_TypeVault

	return na
}

func GetRandomObservedTx() common.ObservedTx {
	return common.NewObservedTx(GetRandomTx(), 33, GetRandomPubKey(), 33)
}

func GetRandomTxOutItem() TxOutItem {
	return TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   GetRandomBTCAddress(),
		VaultPubKey: GetRandomPubKey(),
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(100000)),
		MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(100))},
		InHash:      GetRandomTxHash(),
	}
}

func GetRandomObservedTxVoter() ObservedTxVoter {
	observedTx := GetRandomObservedTx()
	return ObservedTxVoter{
		TxID:    GetRandomTxHash(),
		Tx:      observedTx,
		Height:  10,
		Txs:     common.ObservedTxs{observedTx},
		Actions: []TxOutItem{GetRandomTxOutItem()},
	}
}

// GetRandomTx
func GetRandomTx() common.Tx {
	return common.NewTx(
		GetRandomTxHash(),
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.OneUint())},
		common.Gas{
			{Asset: common.BTCAsset, Amount: cosmos.NewUint(37500)},
		},
	)
}

// GetRandomBech32Addr is an account address used for test
func GetRandomBech32Addr() cosmos.AccAddress {
	name := common.RandHexString(10)
	return cosmos.AccAddress(crypto.AddressHash([]byte(name)))
}

func GetRandomBech32ConsensusPubKey() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404 this is a method only used for test purpose
	accts := simtypes.RandomAccounts(r, 1)
	result, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeConsPub, accts[0].PubKey)
	if err != nil {
		panic(err)
	}
	return result
}

func GetRandomThornadoAddress() common.Address {
	name := common.RandHexString(10)
	str, _ := common.ConvertAndEncode(cmd.Bech32PrefixAccAddr, crypto.AddressHash([]byte(name)))
	addr, _ := common.NewAddress(str)
	return addr
}

func GetRandomBTCAddress() common.Address {
	pubKey := GetRandomPubKey()
	addr, _ := pubKey.GetAddress(common.BTCChain)
	return addr
}

// GetRandomTxHash create a random txHash used for test purpose
func GetRandomTxHash() common.TxID {
	txHash, _ := common.NewTxID(common.RandHexString(64))
	return txHash
}

// GetRandomPubKeySet return a random common.PubKeySet for test purpose
func GetRandomPubKeySet() common.PubKeySet {
	return common.NewPubKeySet(GetRandomPubKey())
}

func GetRandomVault() Vault {
	return NewVaultV2(32, VaultStatus_ActiveVault, VaultType_BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomEd25519PubKey())
}

func GetRandomPubKey() common.PubKey {
	r := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404
	accts := simtypes.RandomAccounts(r, 1)
	bech32PubKey, _ := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, accts[0].PubKey)
	pk, _ := common.NewPubKey(bech32PubKey)
	return pk
}

func GetRandomEd25519PubKey() common.PubKey {
	privKey := ed25519.GenPrivKey()
	bech32PubKey, _ := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, privKey.PubKey())
	pk, _ := common.NewPubKey(bech32PubKey)
	return pk
}

func GetRandomPubkeyForChain(chain common.Chain) common.PubKey {
	if chain.GetSigningAlgo() == common.SigningAlgoEd25519 {
		return GetRandomEd25519PubKey()
	}

	return GetRandomPubKey()
}

// SetupConfigForTest used for test purpose
func SetupConfigForTest() {
	config := cosmos.GetConfig()
	config.SetBech32PrefixForAccount(cmd.Bech32PrefixAccAddr, cmd.Bech32PrefixAccPub)
	config.SetBech32PrefixForValidator(cmd.Bech32PrefixValAddr, cmd.Bech32PrefixValPub)
	config.SetBech32PrefixForConsensusNode(cmd.Bech32PrefixConsAddr, cmd.Bech32PrefixConsPub)
	config.SetCoinType(cmd.ThornadoCoinType)
	config.SetPurpose(cmd.ThornadoCoinPurpose)
}

// GetCurrentVersion - intended for unit tests, fetches the current version of
// Thornado via `version` file
// #nosec G304 this is a method only used for test purpose
func GetCurrentVersion() semver.Version {
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "../../..")
	dat, err := os.ReadFile(path.Join(dir, "version"))
	if err != nil {
		panic(err)
	}
	v, err := semver.Make(strings.TrimSpace(string(dat)))
	if err != nil {
		panic(err)
	}
	return v
}
