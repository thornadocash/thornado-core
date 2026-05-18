package evm

import (
	"math/big"
	"testing"
	"time"

	ecommon "github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm/types"
	btypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/common/tokenlist"
	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type UtilsSuite struct{}

var _ = Suite(&UtilsSuite{})

func (s *UtilsSuite) TestIsSmartContractCall(c *C) {
	// tx with < 4 bytes data, should not be smart contract call
	tx := etypes.NewTransaction(0, ecommon.HexToAddress("0x1"), big.NewInt(1), 21000, big.NewInt(1), []byte{0x01, 0x02})
	receipt := &etypes.Receipt{Logs: []*etypes.Log{{}}}
	c.Assert(IsSmartContractCall(tx, receipt), Equals, false)

	// tx with >= 4 bytes data but no logs, should not be smart contract call
	tx = etypes.NewTransaction(0, ecommon.HexToAddress("0x1"), big.NewInt(1), 21000, big.NewInt(1), []byte{0x01, 0x02, 0x03, 0x04})
	receipt = &etypes.Receipt{Logs: []*etypes.Log{}}
	c.Assert(IsSmartContractCall(tx, receipt), Equals, false)

	// tx with >= 4 bytes data and logs, should be smart contract call
	tx = etypes.NewTransaction(0, ecommon.HexToAddress("0x1"), big.NewInt(1), 21000, big.NewInt(1), []byte{0x01, 0x02, 0x03, 0x04})
	receipt = &etypes.Receipt{Logs: []*etypes.Log{{}}}
	c.Assert(IsSmartContractCall(tx, receipt), Equals, true)

	// tx with no data, should not be smart contract call
	tx = etypes.NewTransaction(0, ecommon.HexToAddress("0x1"), big.NewInt(1), 21000, big.NewInt(1), nil)
	receipt = &etypes.Receipt{Logs: []*etypes.Log{{}}}
	c.Assert(IsSmartContractCall(tx, receipt), Equals, false)
}

func (s *UtilsSuite) TestGetContractABI(c *C) {
	// valid ABIs
	vaultABI, erc20ABI, err := GetContractABI(routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)
	c.Assert(vaultABI, NotNil)
	c.Assert(erc20ABI, NotNil)

	// invalid router ABI
	_, _, err = GetContractABI("invalid json", erc20ContractABI)
	c.Assert(err, NotNil)

	// invalid erc20 ABI
	_, _, err = GetContractABI(routerContractABI, "invalid json")
	c.Assert(err, NotNil)
}

func (s *UtilsSuite) TestIsNative(c *C) {
	c.Assert(IsNative(NativeTokenAddr), Equals, true)
	c.Assert(IsNative("0x0000000000000000000000000000000000000000"), Equals, true)
	c.Assert(IsNative("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"), Equals, false)
	c.Assert(IsNative(""), Equals, false)
}

func (s *UtilsSuite) TestSanitiseSymbol(c *C) {
	c.Assert(sanitiseSymbol("USDC"), Equals, "USDC")
	c.Assert(sanitiseSymbol("USDC.e"), Equals, "USDCe")
	c.Assert(sanitiseSymbol("some-token"), Equals, "sometoken")
	c.Assert(sanitiseSymbol("token+plus"), Equals, "tokenplus")
	c.Assert(sanitiseSymbol("null\u0000byte"), Equals, "nullbyte")
	c.Assert(sanitiseSymbol(""), Equals, "")
}

type SmartContractSuite struct{}

var _ = Suite(&SmartContractSuite{})

func (s *SmartContractSuite) TestGetContractABI_EmptyStrings(c *C) {
	// empty ABI strings should parse as empty ABIs (valid JSON array)
	vaultABI, erc20ABI, err := GetContractABI("[]", "[]")
	c.Assert(err, IsNil)
	c.Assert(vaultABI, NotNil)
	c.Assert(erc20ABI, NotNil)
}

// ----- SignedTxItem tests for BlockMetaAccessor -----

func (s *UtilsSuite) TestSignedTxItems(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	accessor, err := NewLevelDBBlockMetaAccessor(PrefixBlockMeta, PrefixSignedTxItem, db)
	c.Assert(err, IsNil)

	// Get signed tx items when empty
	items, err := accessor.GetSignedTxItems()
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 0)

	// Add signed tx items
	item1 := types.SignedTxItem{Hash: "0xabc123", Height: 100, VaultPubKey: "pubkey1"}
	item2 := types.SignedTxItem{Hash: "0xdef456", Height: 101, VaultPubKey: "pubkey2"}
	c.Assert(accessor.AddSignedTxItem(item1), IsNil)
	c.Assert(accessor.AddSignedTxItem(item2), IsNil)

	// Verify key format
	key := accessor.getSignedTxItemKey("0xabc123")
	c.Assert(key, Equals, PrefixSignedTxItem+"0xabc123")

	// Get all signed tx items
	items, err = accessor.GetSignedTxItems()
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 2)

	// Remove a signed tx item
	c.Assert(accessor.RemoveSignedTxItem("0xabc123"), IsNil)
	items, err = accessor.GetSignedTxItems()
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 1)
	c.Assert(items[0].Hash, Equals, "0xdef456")

	// Remove non-existent item should not error
	c.Assert(accessor.RemoveSignedTxItem("0xnonexistent"), IsNil)
}

// ----- TokenManager additional tests -----

func (s *UtilsSuite) TestTokenManagerConvertAmount(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, testWhiteList, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// ConvertAmount for native token (1e18 -> 1e8)
	// 1e18 / (1e8 * 100) = 1e18 / 1e10 = 1e8
	amt := big.NewInt(0).Mul(big.NewInt(1e18), big.NewInt(1))
	result := manager.ConvertAmount(NativeTokenAddr, amt)
	c.Assert(result.Equal(cosmos.NewUint(1e8)), Equals, true)

	// ConvertThorchainAmountToWei
	weiAmt := manager.ConvertThorchainAmountToWei(big.NewInt(1e8))
	expected := big.NewInt(0).Mul(big.NewInt(1e8), big.NewInt(common.One*100))
	c.Assert(weiAmt.Cmp(expected), Equals, 0)

	// ConvertSigningAmount for native token
	sigAmt := manager.ConvertSigningAmount(big.NewInt(100), NativeTokenAddr)
	expectedSig := big.NewInt(0).Mul(big.NewInt(100), big.NewInt(common.One*100))
	c.Assert(sigAmt.Cmp(expectedSig), Equals, 0)

	// Save a 6-decimal token and test conversion
	// ConvertSigningAmount: 100 * 1e10 (to wei) = 1e12, then * 10^6 / 10^18 = 1
	c.Assert(manager.SaveTokenMeta("USDC", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", 6), IsNil)
	sigAmt = manager.ConvertSigningAmount(big.NewInt(100), "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	c.Assert(sigAmt.Int64(), Equals, int64(1))

	// ConvertAmount for non-native token with different decimals
	// ConvertAmount: 1e6 * 10^18 / 10^6 = 1e18, then / 1e10 = 1e8
	c.Assert(manager.SaveTokenMeta("TKN6", "0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49", 6), IsNil)
	result = manager.ConvertAmount("0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49", big.NewInt(1000000))
	c.Assert(result.Equal(cosmos.NewUint(1e8)), Equals, true)
}

func (s *UtilsSuite) TestTokenManagerGetTokens(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, testWhiteList, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// GetTokens when empty
	tokens, err := manager.GetTokens()
	c.Assert(err, IsNil)
	c.Assert(tokens, HasLen, 0)

	// Save some tokens
	c.Assert(manager.SaveTokenMeta("TKN", "0xabc", 18), IsNil)
	c.Assert(manager.SaveTokenMeta("USDC", "0xdef", 6), IsNil)

	tokens, err = manager.GetTokens()
	c.Assert(err, IsNil)
	c.Assert(tokens, HasLen, 2)
}

func (s *UtilsSuite) TestTokenManagerGetAssetFromTokenAddress(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, testWhiteList, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// Native token should return native asset
	asset, err := manager.GetAssetFromTokenAddress(NativeTokenAddr)
	c.Assert(err, IsNil)
	c.Assert(asset.Equals(common.ETHAsset), Equals, true)

	// Unknown non-whitelisted token should error
	_, err = manager.GetAssetFromTokenAddress("0x1111111111111111111111111111111111111111")
	c.Assert(err, NotNil)

	// Save a token and retrieve its asset
	c.Assert(manager.SaveTokenMeta("TKN", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", 18), IsNil)
	asset, err = manager.GetAssetFromTokenAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	c.Assert(err, IsNil)
	c.Assert(asset.IsEmpty(), Equals, false)
}

func (s *UtilsSuite) TestTokenManagerNewTokenManager_Errors(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	// nil ethClient should error
	_, err = NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, nil, nil, routerContractABI, erc20ContractABI)
	c.Assert(err, NotNil)

	// invalid router ABI
	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)
	_, err = NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, nil, ethClient, "invalid", erc20ContractABI)
	c.Assert(err, NotNil)
}

func (s *UtilsSuite) TestTokenManagerGetTokenDecimalsForTHORChain(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, testWhiteList, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// Native token returns 0
	c.Assert(manager.GetTokenDecimalsForTHORChain(NativeTokenAddr), Equals, int64(0))

	// Unknown token returns 0
	c.Assert(manager.GetTokenDecimalsForTHORChain("0x1111111111111111111111111111111111111111"), Equals, int64(0))

	// Token with decimals >= 8 (THORChainDecimals) returns 0
	c.Assert(manager.SaveTokenMeta("TKN18", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", 18), IsNil)
	c.Assert(manager.GetTokenDecimalsForTHORChain("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"), Equals, int64(0))

	// Token with decimals < 8 returns actual decimals
	c.Assert(manager.SaveTokenMeta("USDC", "0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49", 6), IsNil)
	c.Assert(manager.GetTokenDecimalsForTHORChain("0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49"), Equals, int64(6))
}

// ----- SmartContractLogParser: test maxLogs exceeded -----

func (s *UtilsSuite) TestSmartContractLogParser_MaxLogs(c *C) {
	vaultABI, _, err := GetContractABI(routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)
	parser := NewSmartContractLogParser(mockIsValidContractAddr, mockAssetResolver, mockTokenDecimalResolver, mockAmountConverter, vaultABI, common.ETHAsset, 1)

	// Too many logs should return false
	logs := []*etypes.Log{{}, {}}
	txInItem := &btypes.TxInItem{}
	isVaultTransfer, err := parser.GetTxInItem(logs, txInItem)
	c.Assert(err, IsNil)
	c.Assert(isVaultTransfer, Equals, false)
}

// ----- Test TransferOut with empty memo (memoless outbound) -----

func (s *SmartContractLogParserTestSuite) TestGetTxInItem_TransferOutMemoless(c *C) {
	vaultABI, _, err := GetContractABI(routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)
	parser := NewSmartContractLogParser(mockIsValidContractAddr, mockAssetResolver, mockTokenDecimalResolver, mockAmountConverter, vaultABI, common.ETHAsset, 10)

	// TransferOut with empty memo (memoless outbound) should be treated as valid
	txInItem := &btypes.TxInItem{
		Sender: "0x3fd2D4cE97B082d4BcE3f9fee2A3D60668D2f473",
	}
	logItem := (s).getTransferOutEvent(
		"0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44",
		"0x6c4a2eeb8531e3c18bca51104df7eb2377708263",
		"0x9eca25ee04fdcc9d9cdff377aa8da019dba38437",
		NativeTokenAddr,
		big.NewInt(1024000),
		"",
	)
	isVaultTransfer, err := parser.GetTxInItem([]*etypes.Log{logItem}, txInItem)
	c.Assert(err, IsNil)
	c.Assert(isVaultTransfer, Equals, false)
	c.Assert(txInItem.To, Equals, "0x9EcA25ee04FDCc9d9CDFF377aa8da019Dba38437")
	c.Assert(txInItem.Memo, Equals, "")
	c.Assert(txInItem.Coins.IsEmpty(), Equals, false)
}

// ----- Test TransferOut with empty asset -----

func (s *SmartContractLogParserTestSuite) TestGetTxInItem_TransferOutEmptyAsset(c *C) {
	vaultABI, _, err := GetContractABI(routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// asset resolver that returns empty asset
	emptyAssetResolver := func(token string) (common.Asset, error) {
		return common.EmptyAsset, nil
	}
	parser := NewSmartContractLogParser(mockIsValidContractAddr, emptyAssetResolver, mockTokenDecimalResolver, mockAmountConverter, vaultABI, common.ETHAsset, 10)

	txInItem := &btypes.TxInItem{
		Sender: "0x3fd2D4cE97B082d4BcE3f9fee2A3D60668D2f473",
	}
	logItem := (s).getTransferOutEvent(
		"0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44",
		"0x6c4a2eeb8531e3c18bca51104df7eb2377708263",
		"0x9eca25ee04fdcc9d9cdff377aa8da019dba38437",
		NativeTokenAddr,
		big.NewInt(1024000),
		"OUT:"+common.TxID("ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890").String(),
	)
	isVaultTransfer, err := parser.GetTxInItem([]*etypes.Log{logItem}, txInItem)
	c.Assert(err, IsNil)
	c.Assert(isVaultTransfer, Equals, false)
}

// ----- Test Deposit with empty asset from resolver -----

func (s *SmartContractLogParserTestSuite) TestGetTxInItem_DepositEmptyAsset(c *C) {
	vaultABI, _, err := GetContractABI(routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	emptyAssetResolver := func(token string) (common.Asset, error) {
		return common.EmptyAsset, nil
	}
	parser := NewSmartContractLogParser(mockIsValidContractAddr, emptyAssetResolver, mockTokenDecimalResolver, mockAmountConverter, vaultABI, common.ETHAsset, 10)

	txInItem := &btypes.TxInItem{
		Sender: "0x3fd2d4ce97b082d4bce3f9fee2a3d60668d2f473",
	}
	isVaultTransfer, err := parser.GetTxInItem([]*etypes.Log{
		(s).getDepositEvent("0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44", "0x6c4a2eeb8531e3c18bca51104df7eb2377708263", NativeTokenAddr, big.NewInt(1024000), "ADD:ETH.ETH"),
	}, txInItem)
	c.Assert(err, IsNil)
	c.Assert(isVaultTransfer, Equals, false)
	c.Assert(txInItem.To, Equals, "")
}

// Test tokenlist whitelist check with case-insensitive matching

func (s *UtilsSuite) TestTokenManager_ConvertSigningAmount_DefaultDecimals(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	wl := []tokenlist.ERC20Token{
		{Address: "0xToken1", Symbol: "TK1"},
	}
	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, wl, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// Token with default decimals (18) - should pass through without conversion
	c.Assert(manager.SaveTokenMeta("TK1", "0xToken1", 18), IsNil)
	sigAmt := manager.ConvertSigningAmount(big.NewInt(100), "0xToken1")
	expected := big.NewInt(0).Mul(big.NewInt(100), big.NewInt(common.One*100))
	c.Assert(sigAmt.Cmp(expected), Equals, 0)
}

func (s *UtilsSuite) TestTokenManager_ConvertAmount_NonNativeToken(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, testWhiteList, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// Save a token with same decimals as default (18): 1e18 / 1e10 = 1e8
	c.Assert(manager.SaveTokenMeta("TKN18", "0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49", 18), IsNil)
	amt := big.NewInt(1e18)
	result := manager.ConvertAmount("0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49", amt)
	c.Assert(result.Equal(cosmos.NewUint(1e8)), Equals, true)

	// Save a token with different decimals (6): 1e6 * 10^18 / 10^6 = 1e18, / 1e10 = 1e8
	c.Assert(manager.SaveTokenMeta("USDC6", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", 6), IsNil)
	amt2 := big.NewInt(1000000) // 1 USDC
	result2 := manager.ConvertAmount("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", amt2)
	c.Assert(result2.Equal(cosmos.NewUint(1e8)), Equals, true)
}

func (s *UtilsSuite) TestTokenManager_ConvertSigningAmount_NonDefaultDecimals(c *C) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	defer db.Close()

	ethClient, err := getTestEthClient(c)
	c.Assert(err, IsNil)

	manager, err := NewTokenManager(db, "tm-", common.ETHAsset, 18, time.Second, testWhiteList, ethClient, routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)

	// Token with non-default decimals (9): 100 * 1e10 = 1e12, * 10^9 / 10^18 = 1000
	c.Assert(manager.SaveTokenMeta("TKN9", "0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49", 9), IsNil)
	sigAmt := manager.ConvertSigningAmount(big.NewInt(100), "0xB0b86991c6218b36c1d19D4a2e9Eb0cE3606eB49")
	c.Assert(sigAmt.Int64(), Equals, int64(1000))

	// Unknown non-whitelisted token - error path, returns default wei conversion (100 * 1e10)
	sigAmt = manager.ConvertSigningAmount(big.NewInt(100), "0x1111111111111111111111111111111111111111")
	c.Assert(sigAmt.Int64(), Equals, int64(1e12))
}

// ----- Test TransferAllowance with asset error from resolver -----

func (s *SmartContractLogParserTestSuite) TestGetTxInItem_TransferAllowanceAssetError(c *C) {
	vaultABI, _, err := GetContractABI(routerContractABI, erc20ContractABI)
	c.Assert(err, IsNil)
	parser := NewSmartContractLogParser(mockIsValidContractAddr, mockAssetResolver, mockTokenDecimalResolver, mockAmountConverter, vaultABI, common.ETHAsset, 10)

	// TransferAllowance with empty resolved asset should be ignored
	emptyAssetResolver := func(token string) (common.Asset, error) {
		return common.EmptyAsset, nil
	}
	parser2 := NewSmartContractLogParser(mockIsValidContractAddr, emptyAssetResolver, mockTokenDecimalResolver, mockAmountConverter, vaultABI, common.ETHAsset, 10)

	txInItem := &btypes.TxInItem{
		Sender: "0x3fd2D4cE97B082d4BcE3f9fee2A3D60668D2f473",
	}
	isVaultTransfer, err := parser2.GetTxInItem([]*etypes.Log{
		s.getTransferAllowanceEvent("0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44",
			"0x3fd2D4cE97B082d4BcE3f9fee2A3D60668D2f473",
			"0x9eca25ee04fdcc9d9cdff377aa8da019dba38437",
			NativeTokenAddr, big.NewInt(102400), "MIGRATE:1024"),
	}, txInItem)
	c.Assert(err, IsNil)
	c.Assert(isVaultTransfer, Equals, false)

	// TransferAllowance with non-migrate memo should be ignored
	txInItem = &btypes.TxInItem{
		Sender: "0x3fd2D4cE97B082d4BcE3f9fee2A3D60668D2f473",
	}
	isVaultTransfer, err = parser.GetTxInItem([]*etypes.Log{
		s.getTransferAllowanceEvent("0xE65e9d372F8cAcc7b6dfcd4af6507851Ed31bb44",
			"0x3fd2D4cE97B082d4BcE3f9fee2A3D60668D2f473",
			"0x9eca25ee04fdcc9d9cdff377aa8da019dba38437",
			NativeTokenAddr, big.NewInt(102400), "ADD:ETH.ETH"),
	}, txInItem)
	c.Assert(err, IsNil)
	c.Assert(isVaultTransfer, Equals, false)
	c.Assert(txInItem.To, Equals, "")
}
