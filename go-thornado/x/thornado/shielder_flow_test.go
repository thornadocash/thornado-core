package thornado

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"testing"

	"cosmossdk.io/log"
	"github.com/blang/semver"
	"github.com/btcsuite/btcd/btcec"
	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type shielderFlowTestKeeper struct {
	keeper.KVStoreDummy

	feePool        types.FeePool
	deposits       map[string]types.DepositRecord
	commitments    map[string]bool
	noteRecords    map[string]types.StoredShielderNoteRecord
	nullifiers     map[string][]byte
	denominations  map[uint64][]string
	merkleRoots    map[uint64]string
	redeems        map[string]types.ShielderRedeem
	nodeBonds      map[string]types.ShielderNodeBond
	nodeBonders    map[string]types.ShielderNodeBonder
	nodeAccounts   map[string]NodeAccount
	auctions       map[string]types.NodeSlotAuction
	bids           map[string]types.NodeSlotBid
	baseVaults     Vaults
	feeNotePubKey  map[string]bool
	txOuts         []TxOutItem
	txOutByHeight  map[int64]TxOut
	txInVoters     map[string]ObservedTxVoter
	txOutVoters    map[string]ObservedTxVoter
	solvencyVoters map[string]types.SolvencyVoter
	networkFees    map[common.Chain]NetworkFee
	sessions       map[string]types.DepositSession
	powSessions    map[string]types.DepositSession
	addresses      map[string]types.DepositAddress
	pathNonces     map[string]uint64
	configs        map[constants.ConfigName]int64
	nextSlot       uint64
}

func newShielderFlowTestKeeper() *shielderFlowTestKeeper {
	return &shielderFlowTestKeeper{
		deposits:       make(map[string]types.DepositRecord),
		commitments:    make(map[string]bool),
		noteRecords:    make(map[string]types.StoredShielderNoteRecord),
		nullifiers:     make(map[string][]byte),
		denominations:  make(map[uint64][]string),
		merkleRoots:    make(map[uint64]string),
		redeems:        make(map[string]types.ShielderRedeem),
		nodeBonds:      make(map[string]types.ShielderNodeBond),
		nodeBonders:    make(map[string]types.ShielderNodeBonder),
		nodeAccounts:   make(map[string]NodeAccount),
		auctions:       make(map[string]types.NodeSlotAuction),
		bids:           make(map[string]types.NodeSlotBid),
		feeNotePubKey:  make(map[string]bool),
		txOutByHeight:  make(map[int64]TxOut),
		txInVoters:     make(map[string]ObservedTxVoter),
		txOutVoters:    make(map[string]ObservedTxVoter),
		solvencyVoters: make(map[string]types.SolvencyVoter),
		networkFees: map[common.Chain]NetworkFee{
			common.BTCChain: {
				Chain:              common.BTCChain,
				TransactionSize:    221,
				TransactionFeeRate: 14,
			},
		},
		sessions:    make(map[string]types.DepositSession),
		powSessions: make(map[string]types.DepositSession),
		addresses:   make(map[string]types.DepositAddress),
		pathNonces:  make(map[string]uint64),
		configs:     make(map[constants.ConfigName]int64),
	}
}

func (k *shielderFlowTestKeeper) GetConfigInt64(_ cosmos.Context, key constants.ConfigName) int64 {
	if value, ok := k.configs[key]; ok {
		return value
	}
	return constants.NewConfigValue().GetInt64Value(key)
}

func (k *shielderFlowTestKeeper) SetDepositRecord(_ cosmos.Context, deposit types.DepositRecord) error {
	k.deposits[deposit.DepositID.String()] = deposit
	return nil
}

func (k *shielderFlowTestKeeper) GetDepositRecordIteratorAfter(ctx cosmos.Context, cursor string) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	for id, deposit := range k.deposits {
		if strings.TrimSpace(cursor) != "" && id <= cursor {
			continue
		}
		value, _ := json.Marshal(deposit)
		iter.AddItem([]byte(id), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) GetDepositRecordIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.GetDepositRecordIteratorAfter(ctx, "")
}

func (k *shielderFlowTestKeeper) GetDepositRecord(_ cosmos.Context, depositID common.TxID) (types.DepositRecord, error) {
	return k.deposits[depositID.String()], nil
}

func (k *shielderFlowTestKeeper) SetDepositSession(_ cosmos.Context, session types.DepositSession) error {
	k.sessions[session.Owner.String()] = session
	k.powSessions[session.PowToken] = session
	return nil
}

func (k *shielderFlowTestKeeper) GetDepositSession(_ cosmos.Context, owner cosmos.AccAddress) (types.DepositSession, error) {
	return k.sessions[owner.String()], nil
}

func (k *shielderFlowTestKeeper) GetDepositSessionByPowToken(_ cosmos.Context, powToken string) (types.DepositSession, error) {
	return k.powSessions[powToken], nil
}

func (k *shielderFlowTestKeeper) SetDepositAddress(_ cosmos.Context, record types.DepositAddress) error {
	k.addresses[record.Address.String()] = record
	return nil
}

func (k *shielderFlowTestKeeper) GetDepositAddress(_ cosmos.Context, address common.Address) (types.DepositAddress, error) {
	return k.addresses[address.String()], nil
}

func (k *shielderFlowTestKeeper) SetDepositPowTiming(_ cosmos.Context, _ types.DepositPowTiming) error {
	return nil
}

func (k *shielderFlowTestKeeper) AllocateVaultDepositPathIndex(_ cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType) (uint64, uint64, error) {
	key := vaultPubKey.String() + "/" + string(pathType)
	nonce := k.pathNonces[key]
	k.pathNonces[key]++
	pathIndex, err := common.VaultDepositPathIndex(pathType, nonce, common.DepositPathCommitmentRoot)
	return nonce, pathIndex, err
}

func (k *shielderFlowTestKeeper) SetShielderCommitment(_ cosmos.Context, commitment string) error {
	k.commitments[commitment] = true
	return nil
}

func (k *shielderFlowTestKeeper) ShielderCommitmentExists(_ cosmos.Context, commitment string) bool {
	_, ok := k.commitments[commitment]
	return ok
}

func (k *shielderFlowTestKeeper) SetShielderNoteRecord(_ cosmos.Context, record types.StoredShielderNoteRecord) error {
	k.noteRecords[record.Key()] = record
	return nil
}

func (k *shielderFlowTestKeeper) GetShielderNoteRecordIteratorAfter(_ cosmos.Context, cursor string) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	keys := make([]string, 0, len(k.noteRecords))
	for key := range k.noteRecords {
		if strings.TrimSpace(cursor) != "" && key <= cursor {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, _ := json.Marshal(k.noteRecords[key])
		iter.AddItem([]byte(key), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) GetShielderNoteRecordIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.GetShielderNoteRecordIteratorAfter(ctx, "")
}

func (k *shielderFlowTestKeeper) SetShielderDenominationCommitment(_ cosmos.Context, denominationSats uint64, commitment string) error {
	k.denominations[denominationSats] = append(k.denominations[denominationSats], commitment)
	return nil
}

func (k *shielderFlowTestKeeper) GetShielderDenominationCommitments(_ cosmos.Context, denominationSats uint64) ([]string, error) {
	return append([]string{}, k.denominations[denominationSats]...), nil
}

func (k *shielderFlowTestKeeper) SetShielderMerkleRoot(_ cosmos.Context, denominationSats uint64, root string) error {
	k.merkleRoots[denominationSats] = root
	return nil
}

func (k *shielderFlowTestKeeper) ShielderMerkleRootExists(_ cosmos.Context, denominationSats uint64, root string) bool {
	return k.merkleRoots[denominationSats] == root
}

func (k *shielderFlowTestKeeper) SetShielderRedeem(_ cosmos.Context, redeem types.ShielderRedeem) error {
	k.redeems[redeem.WithdrawalID] = redeem
	return nil
}

func (k *shielderFlowTestKeeper) GetShielderRedeem(_ cosmos.Context, withdrawalID string) (types.ShielderRedeem, error) {
	return k.redeems[withdrawalID], nil
}

func (k *shielderFlowTestKeeper) AppendTxOut(_ cosmos.Context, _ int64, item TxOutItem) error {
	k.txOuts = append(k.txOuts, item)
	return nil
}

func (k *shielderFlowTestKeeper) SetFeePool(_ cosmos.Context, pool types.FeePool) error {
	k.feePool = pool
	return nil
}

func (k *shielderFlowTestKeeper) GetFeePool(_ cosmos.Context) (types.FeePool, error) {
	return k.feePool, nil
}

func (k *shielderFlowTestKeeper) SetShielderNodeBond(_ cosmos.Context, bond types.ShielderNodeBond) error {
	k.nodeBonds[bond.NodePubKey] = bond
	return nil
}

func (k *shielderFlowTestKeeper) GetShielderNodeBond(_ cosmos.Context, nodePubKey string) (types.ShielderNodeBond, error) {
	return k.nodeBonds[nodePubKey], nil
}

func (k *shielderFlowTestKeeper) GetShielderNodeBondIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	for nodePubKey, bond := range k.nodeBonds {
		value, _ := json.Marshal(bond)
		iter.AddItem([]byte(nodePubKey), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) SetShielderNullifierSpent(_ cosmos.Context, nullifierHash string, withdrawalID string) error {
	record := types.StoredShielderNullifierRecord{
		NullifierHash: nullifierHash,
		WithdrawalID:  withdrawalID,
	}
	value, _ := json.Marshal(record)
	k.nullifiers[nullifierHash] = value
	return nil
}

func (k *shielderFlowTestKeeper) GetShielderNullifierIteratorAfter(_ cosmos.Context, cursor string) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	keys := make([]string, 0, len(k.nullifiers))
	for key := range k.nullifiers {
		if strings.TrimSpace(cursor) != "" && key <= cursor {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		iter.AddItem([]byte("shielder_nullifier//"+key), k.nullifiers[key])
	}
	return iter
}

func (k *shielderFlowTestKeeper) GetShielderNullifierIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.GetShielderNullifierIteratorAfter(ctx, "")
}

func (k *shielderFlowTestKeeper) SetShielderNodeBonder(_ cosmos.Context, bonder types.ShielderNodeBonder) error {
	k.nodeBonders[bonder.Key()] = bonder
	return nil
}

func (k *shielderFlowTestKeeper) GetShielderNodeBonder(_ cosmos.Context, nodePubKey string, bonder cosmos.AccAddress) (types.ShielderNodeBonder, error) {
	return k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, bonder)], nil
}

func (k *shielderFlowTestKeeper) GetShielderNodeBonderIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	for key, bonder := range k.nodeBonders {
		value, _ := json.Marshal(bonder)
		iter.AddItem([]byte(key), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) DeleteShielderNodeBonder(_ cosmos.Context, nodePubKey string, bonder cosmos.AccAddress) error {
	delete(k.nodeBonders, types.ShielderNodeBonderKey(nodePubKey, bonder))
	return nil
}

func (k *shielderFlowTestKeeper) AllocateShielderNodeBondSlot(_ cosmos.Context) (uint64, error) {
	slot := k.nextSlot
	k.nextSlot++
	return slot, nil
}

func (k *shielderFlowTestKeeper) SetNodeAccount(_ cosmos.Context, account NodeAccount) error {
	k.nodeAccounts[account.NodeAddress.String()] = account
	return nil
}

func (k *shielderFlowTestKeeper) GetNodeAccount(_ cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error) {
	return k.nodeAccounts[addr.String()], nil
}

func (k *shielderFlowTestKeeper) GetNodeAccountPenaltyPoints(_ cosmos.Context, _ cosmos.AccAddress) (int64, error) {
	return 0, nil
}

func (k *shielderFlowTestKeeper) ListNodesByStatus(_ cosmos.Context, status NodeStatus) (NodeAccounts, error) {
	var accounts NodeAccounts
	for _, account := range k.nodeAccounts {
		if account.Status == status {
			accounts = append(accounts, account)
		}
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		return accounts[i].NodeAddress.String() < accounts[j].NodeAddress.String()
	})
	return accounts, nil
}

func (k *shielderFlowTestKeeper) ListActiveNodes(ctx cosmos.Context) (NodeAccounts, error) {
	return k.ListNodesByStatus(ctx, NodeActive)
}

func (k *shielderFlowTestKeeper) SetNodeSlotAuction(_ cosmos.Context, auction types.NodeSlotAuction) error {
	k.auctions[auction.AuctionID] = auction
	return nil
}

func (k *shielderFlowTestKeeper) GetNodeSlotAuction(_ cosmos.Context, auctionID string) (types.NodeSlotAuction, error) {
	return k.auctions[auctionID], nil
}

func (k *shielderFlowTestKeeper) SetNodeSlotBid(_ cosmos.Context, bid types.NodeSlotBid) error {
	k.bids[bid.BidID] = bid
	return nil
}

func (k *shielderFlowTestKeeper) GetNodeSlotBid(_ cosmos.Context, bidID string) (types.NodeSlotBid, error) {
	return k.bids[bidID], nil
}

func (k *shielderFlowTestKeeper) GetNodeSlotBidIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	for id, bid := range k.bids {
		value, _ := json.Marshal(bid)
		iter.AddItem([]byte(id), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) GetNodeSlotAuctionIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	for id, auction := range k.auctions {
		value, _ := json.Marshal(auction)
		iter.AddItem([]byte(id), value)
	}
	return iter
}

func (k *shielderFlowTestKeeper) SetShielderFeeNotePubKey(_ cosmos.Context, pubKey string) error {
	k.feeNotePubKey[pubKey] = true
	return nil
}

func (k *shielderFlowTestKeeper) ShielderFeeNotePubKeyUsed(_ cosmos.Context, pubKey string) bool {
	_, ok := k.feeNotePubKey[pubKey]
	return ok
}

func (k *shielderFlowTestKeeper) GetNetworkFee(_ cosmos.Context, chain common.Chain) (NetworkFee, error) {
	return k.networkFees[chain], nil
}

func (k *shielderFlowTestKeeper) GetBaseVaultsByStatus(_ cosmos.Context, status VaultStatus) (Vaults, error) {
	var vaults Vaults
	for _, vault := range k.baseVaults {
		if vault.Status == status {
			vaults = append(vaults, vault)
		}
	}
	return vaults, nil
}

type shielderFlowTestManager struct {
	k        *shielderFlowTestKeeper
	gas      shielderFlowTestGasManager
	obs      shielderFlowTestObserverManager
	events   shielderFlowTestEventManager
	txOut    shielderFlowTestTxOutStore
	version  semver.Version
	constant constants.ConfigValues
}

func newShielderFlowTestManager(k *shielderFlowTestKeeper) *shielderFlowTestManager {
	return &shielderFlowTestManager{
		k:        k,
		version:  semver.MustParse("9999999.0.0"),
		constant: constants.NewConfigValue(),
		gas:      shielderFlowTestGasManager{maxGas: common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
	}
}

func (m *shielderFlowTestManager) GetConstants() constants.ConfigValues { return m.constant }
func (m *shielderFlowTestManager) GetVersion() semver.Version           { return m.version }
func (m *shielderFlowTestManager) Keeper() keeper.Keeper                { return m.k }
func (m *shielderFlowTestManager) GasMgr() GasManager                   { return &m.gas }
func (m *shielderFlowTestManager) EventMgr() EventManager               { return &m.events }
func (m *shielderFlowTestManager) TxOutStore() TxOutStore               { return &m.txOut }
func (m *shielderFlowTestManager) NetworkMgr() NetworkManager           { return nil }
func (m *shielderFlowTestManager) NodeMgr() NodeManager                 { return nil }
func (m *shielderFlowTestManager) ObMgr() ObserverManager               { return &m.obs }
func (m *shielderFlowTestManager) PenaltyManager() PenaltyManager       { return nil }

type shielderFlowTestEventManager struct{}

func (m *shielderFlowTestEventManager) EmitEvent(cosmos.Context, EmitEventItem) error { return nil }
func (m *shielderFlowTestEventManager) EmitGasEvent(cosmos.Context, *EventGas) error  { return nil }
func (m *shielderFlowTestEventManager) EmitFeeEvent(cosmos.Context, *EventFee) error  { return nil }

type shielderFlowTestObserverManager struct{}

func (m *shielderFlowTestObserverManager) BeginBlock() {}
func (m *shielderFlowTestObserverManager) EndBlock(cosmos.Context, keeper.Keeper) {
}
func (m *shielderFlowTestObserverManager) AppendObserver(common.Chain, []cosmos.AccAddress) {
}
func (m *shielderFlowTestObserverManager) List() []cosmos.AccAddress { return nil }

type shielderFlowTestGasManager struct {
	maxGas common.Coin
}

func (m *shielderFlowTestGasManager) BeginBlock() {}
func (m *shielderFlowTestGasManager) EndBlock(cosmos.Context, keeper.Keeper, EventManager) {
}
func (m *shielderFlowTestGasManager) AddGasAsset(common.Asset, common.Gas, bool) {}
func (m *shielderFlowTestGasManager) ProcessGas(cosmos.Context, keeper.Keeper)   {}
func (m *shielderFlowTestGasManager) GetGas() common.Gas                         { return nil }
func (m *shielderFlowTestGasManager) GetAssetOutboundFee(cosmos.Context, common.Asset) (cosmos.Uint, error) {
	return cosmos.ZeroUint(), nil
}
func (m *shielderFlowTestGasManager) GetGasDetails(cosmos.Context, common.Chain) (common.Coin, int64, error) {
	return m.maxGas, 1, nil
}
func (m *shielderFlowTestGasManager) GetMaxGas(cosmos.Context, common.Chain) (common.Coin, error) {
	return m.maxGas, nil
}
func (m *shielderFlowTestGasManager) GetGasRate(cosmos.Context, common.Chain) cosmos.Uint {
	return cosmos.OneUint()
}
func (m *shielderFlowTestGasManager) GetNetworkFee(cosmos.Context, common.Chain) (NetworkFee, error) {
	return NetworkFee{}, nil
}

type shielderFlowTestTxOutStore struct {
	items []TxOutItem
}

func (s *shielderFlowTestTxOutStore) EndBlock(cosmos.Context, Manager) error { return nil }
func (s *shielderFlowTestTxOutStore) GetBlockOut(cosmos.Context) (*TxOut, error) {
	return &TxOut{}, nil
}
func (s *shielderFlowTestTxOutStore) ClearOutboundItems(cosmos.Context) {}
func (s *shielderFlowTestTxOutStore) GetOutboundItems(cosmos.Context) ([]TxOutItem, error) {
	return append([]TxOutItem{}, s.items...), nil
}
func (s *shielderFlowTestTxOutStore) TryAddTxOutItem(_ cosmos.Context, _ Manager, item TxOutItem, _ cosmos.Uint) (bool, error) {
	s.items = append(s.items, item)
	return true, nil
}
func (s *shielderFlowTestTxOutStore) UnSafeAddTxOutItem(cosmos.Context, Manager, TxOutItem, int64) error {
	return nil
}
func (s *shielderFlowTestTxOutStore) GetOutboundItemByToAddress(cosmos.Context, common.Address) []TxOutItem {
	return nil
}
func (s *shielderFlowTestTxOutStore) CalcTxOutHeight(cosmos.Context, semver.Version, TxOutItem) (int64, error) {
	return 0, nil
}
func (s *shielderFlowTestTxOutStore) DiscoverOutbounds(cosmos.Context, cosmos.Uint, common.Coin, TxOutItem, Vaults) ([]TxOutItem, cosmos.Uint) {
	return nil, cosmos.ZeroUint()
}

func TestUserShieldAndUnshieldFlowQueuesNetWithdrawal(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	amount := uint64(1_000_000)
	depositID := GetRandomTxHash()
	owner := GetRandomBech32Addr()
	deposit := types.DepositRecord{
		DepositID:      depositID,
		Owner:          owner,
		AmountSats:     amount,
		DepositAddress: GetRandomBTCAddress(),
		VaultPubKey:    GetRandomPubKey(),
		Status:         types.DepositStatusDepositMatched,
	}
	k.deposits[depositID.String()] = deposit

	deposit, err := ShieldUserDepositIntoPool(ctx, k, deposit, []string{flowNote(t, amount, "USER_NOTE")})
	if err != nil {
		t.Fatal(err)
	}
	if deposit.Status != types.DepositStatusCommitted || deposit.Settlement != types.DepositSettlementUser {
		t.Fatalf("unexpected user shield state: %#v", deposit)
	}
	redeem := types.ShielderRedeem{
		WithdrawalID:  "redeem-user",
		NullifierHash: "nullifier-user",
		MerkleRoot:    "root-user",
		Recipient:     GetRandomBTCAddress(),
		AmountSats:    amount,
		FeeSats:       123,
		InHash:        GetRandomTxHash(),
		VaultPubKey:   deposit.VaultPubKey,
		Status:        types.ShielderRedeemStatusAuthorized,
	}
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, deposit.VaultPubKey, amount+100_000)
	queued, err := QueueAuthorizedWithdrawalTxOut(ctx, k, redeem)
	if err != nil {
		t.Fatal(err)
	}
	fee := withdrawalFeeSats(ctx, k, amount)
	if queued.Status != types.DepositStatusKeysignQueued || queued.FeeSats != fee {
		t.Fatalf("unexpected redeem state: %#v", queued)
	}
	if len(k.txOuts) != 1 {
		t.Fatalf("expected one withdrawal txout, got %d", len(k.txOuts))
	}
	if got := k.txOuts[0].Coin.Amount.Uint64(); got != amount-fee {
		t.Fatalf("unexpected withdrawal amount: %d", got)
	}
	expectedGas, err := btcExactGasCoin(deposit.VaultPubKey, common.MainVaultPathIndex, []common.Address{redeem.Recipient}, []types.TxOutInput{sourceInput}, 14)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.txOuts[0].MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount.Uint64(); got != expectedGas.Amount.Uint64() {
		t.Fatalf("unexpected withdrawal max gas: %d/%d", got, expectedGas.Amount.Uint64())
	}
	if got := k.txOuts[0].GasRate; got != 14 {
		t.Fatalf("unexpected withdrawal gas rate: %d", got)
	}
	if len(k.txOuts[0].SourceInputs) != 1 || !k.txOuts[0].SourceInputs[0].TxId.Equals(sourceInput.TxId) {
		t.Fatalf("unexpected withdrawal source inputs: %#v", k.txOuts[0].SourceInputs)
	}
	if k.txOuts[0].GetTxType() != types.TxOutTypeOut {
		t.Fatalf("unexpected txout type: %s", k.txOuts[0].GetTxType())
	}
	if k.feePool.TotalCollectedSats != fee {
		t.Fatalf("withdrawal fee was not collected: %#v", k.feePool)
	}
}

func TestBondFromNotesConfirmsStandbyNode(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operator := GetRandomPubKey()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	owner, err := operator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	amount := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))

	bond, err := confirmBondFromNoteSpend(ctx, k, owner, nodePubKey, operator, amount)
	if err != nil {
		t.Fatal(err)
	}
	if bond.BondSats != amount || bond.PendingSats != 0 || !bond.FeeShareActive || k.feePool.TotalSlots != 1 {
		t.Fatalf("unexpected node bond state: %#v fee=%#v", bond, k.feePool)
	}
	nodeAccount := k.nodeAccounts[owner.String()]
	if nodeAccount.NodeConsPubKey != nodePubKey || nodeAccount.Bond.Uint64() != amount {
		t.Fatalf("node account was not created: %#v", nodeAccount)
	}

	bond, err = confirmBondFromNoteSpend(ctx, k, owner, nodePubKey, operator, amount)
	if err != nil {
		t.Fatal(err)
	}
	if bond.BondSats != amount*2 || bond.PendingSats != 0 || !bond.FeeShareActive || k.feePool.TotalSlots != 1 {
		t.Fatalf("unexpected node bond top-up state: %#v fee=%#v", bond, k.feePool)
	}
}

func TestBondFromNotesKeepsRegisteredOperatorAcrossBonders(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operator := GetRandomPubKey()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	operatorAddress, err := operator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	secondBonder := GetRandomBech32Addr()
	if secondBonder.Equals(operatorAddress) {
		t.Fatal("test setup produced matching second bonder")
	}
	amount := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))

	if _, err := confirmBondFromNoteSpend(ctx, k, operatorAddress, nodePubKey, operator, amount); err != nil {
		t.Fatal(err)
	}
	if _, err := confirmBondFromNoteSpend(ctx, k, secondBonder, nodePubKey, operator, amount); err != nil {
		t.Fatal(err)
	}

	nodeAccount := k.nodeAccounts[operatorAddress.String()]
	if nodeAccount.NodeConsPubKey != nodePubKey || nodeAccount.Bond.Uint64() != amount*2 {
		t.Fatalf("unexpected node account after top-up: %#v", nodeAccount)
	}
	if nodeAccount.BondAddress.String() != operatorAddress.String() {
		t.Fatalf("node operator was overwritten by bonder: got %s want %s", nodeAccount.BondAddress, operatorAddress)
	}
}

func TestBondFromNotesRequiresFirstBonderToBeOperator(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operator := GetRandomPubKey()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	operatorAddress, err := operator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	firstBonder := GetRandomBech32Addr()
	if firstBonder.Equals(operatorAddress) {
		t.Fatal("test setup produced matching first bonder")
	}
	amount := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))

	if _, err := confirmBondFromNoteSpend(ctx, k, firstBonder, nodePubKey, operator, amount); err == nil || !strings.Contains(err.Error(), "first node bonder must be the operator") {
		t.Fatalf("expected first bonder/operator rejection, got %v", err)
	}
	if _, err := confirmBondFromNoteSpend(ctx, k, operatorAddress, nodePubKey, operator, amount); err != nil {
		t.Fatal(err)
	}
}

func TestOperatorRotateMovesOperatorBonder(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	oldOperatorAddress, err := oldOperator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	newOperatorAddress, err := newOperator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	nodePubKey := GetRandomBech32ConsensusPubKey()
	amount := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:     nodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    oldOperatorAddress,
		Slot:           3,
		BondSats:       amount,
		FeeShareActive: true,
		Bonders:        []string{oldOperatorAddress.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, oldOperatorAddress)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        oldOperatorAddress,
		PrincipalSats: amount,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeAccounts[oldOperatorAddress.String()] = NewNodeAccount(oldOperatorAddress, NodeStandby, common.EmptyPubKeySet, nodePubKey, cosmos.NewUint(amount), common.Address(oldOperatorAddress.String()), 1)
	k.auctions["open-rotate"] = types.NodeSlotAuction{
		AuctionID:            "open-rotate",
		Seller:               oldOperatorAddress,
		SellerOperatorPubKey: oldOperator,
		SellerNodePubKey:     nodePubKey,
		Slot:                 3,
		OriginalBondSats:     amount,
		ReserveSats:          amount,
		ExpiryHeight:         100,
		Status:               types.NodeSlotAuctionOpen,
	}

	msg := types.NewMsgOperatorRotate(oldOperatorAddress, newOperatorAddress, newOperator.String(), common.Coin{})
	handler := NewOperatorRotateHandler(newShielderFlowTestManager(k))
	if _, err := handler.Run(ctx, msg); err != nil {
		t.Fatal(err)
	}

	bond := k.nodeBonds[nodePubKey]
	if !bond.OperatorPubKey.Equals(newOperator) || !bond.NodeAddress.Equals(oldOperatorAddress) {
		t.Fatalf("unexpected rotated bond: %#v", bond)
	}
	if len(bond.Bonders) != 1 || bond.Bonders[0] != newOperatorAddress.String() {
		t.Fatalf("operator bonder index was not rotated: %#v", bond.Bonders)
	}
	if _, ok := k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, oldOperatorAddress)]; ok {
		t.Fatal("old operator bonder row was not removed")
	}
	newBonder := k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, newOperatorAddress)]
	if newBonder.PrincipalSats != amount || !newBonder.Bonder.Equals(newOperatorAddress) {
		t.Fatalf("new operator bonder row was not moved: %#v", newBonder)
	}
	nodeAccount := k.nodeAccounts[oldOperatorAddress.String()]
	if nodeAccount.BondAddress.String() != newOperatorAddress.String() || !nodeAccount.NodeAddress.Equals(oldOperatorAddress) {
		t.Fatalf("unexpected rotated node account: %#v", nodeAccount)
	}
	auction := k.auctions["open-rotate"]
	if !auction.Seller.Equals(newOperatorAddress) || !auction.SellerOperatorPubKey.Equals(newOperator) {
		t.Fatalf("open auction seller was not rotated: %#v", auction)
	}
	if _, err := resolveNodeAccountByAddressAndSigner(ctx, k, oldOperatorAddress, oldOperatorAddress); err == nil {
		t.Fatal("old operator should no longer control rotated node")
	}
	resolved, err := resolveNodeAccountByAddressAndSigner(ctx, k, oldOperatorAddress, newOperatorAddress)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.NodeAddress.Equals(oldOperatorAddress) {
		t.Fatalf("new operator resolved wrong node: %#v", resolved)
	}
}

func TestOperatorRotateRejectsExistingBonderTarget(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	oldOperatorAddress, err := oldOperator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	newOperatorAddress, err := newOperator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	nodePubKey := GetRandomBech32ConsensusPubKey()
	amount := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:     nodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    oldOperatorAddress,
		Slot:           3,
		BondSats:       amount * 2,
		FeeShareActive: true,
		Bonders:        []string{oldOperatorAddress.String(), newOperatorAddress.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, oldOperatorAddress)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        oldOperatorAddress,
		PrincipalSats: amount,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, newOperatorAddress)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        newOperatorAddress,
		PrincipalSats: amount,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeAccounts[oldOperatorAddress.String()] = NewNodeAccount(oldOperatorAddress, NodeStandby, common.EmptyPubKeySet, nodePubKey, cosmos.NewUint(amount*2), common.Address(oldOperatorAddress.String()), 1)

	msg := types.NewMsgOperatorRotate(oldOperatorAddress, newOperatorAddress, newOperator.String(), common.Coin{})
	handler := NewOperatorRotateHandler(newShielderFlowTestManager(k))
	if _, err := handler.Run(ctx, msg); err == nil || !strings.Contains(err.Error(), "already a bonder") {
		t.Fatalf("expected existing bonder target rejection, got %v", err)
	}
}

func TestBondFromNotesAccumulatesPendingBeforeSlotThreshold(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operator := GetRandomPubKey()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	owner, err := operator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	required := uint64(k.GetConfigInt64(ctx, constants.Node_BondStartAmountSats))

	bond, err := confirmBondFromNoteSpend(ctx, k, owner, nodePubKey, operator, required/2)
	if err != nil {
		t.Fatal(err)
	}
	if bond.BondSats != 0 || bond.PendingSats != required/2 || bond.FeeShareActive || k.feePool.TotalSlots != 0 {
		t.Fatalf("unexpected pending bond state: %#v fee=%#v", bond, k.feePool)
	}
	if _, ok := k.nodeAccounts[owner.String()]; ok {
		t.Fatal("node account should not be created before bond threshold")
	}

	bond, err = confirmBondFromNoteSpend(ctx, k, owner, nodePubKey, operator, required/2)
	if err != nil {
		t.Fatal(err)
	}
	if bond.BondSats != required || bond.PendingSats != 0 || !bond.FeeShareActive || k.feePool.TotalSlots != 1 {
		t.Fatalf("unexpected activated bond state: %#v fee=%#v", bond, k.feePool)
	}
	nodeAccount := k.nodeAccounts[owner.String()]
	if nodeAccount.NodeConsPubKey != nodePubKey || nodeAccount.Bond.Uint64() != required {
		t.Fatalf("node account was not created after threshold: %#v", nodeAccount)
	}
}

func TestNodeBondDepositPathRejected(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	owner := GetRandomBech32Addr()
	depositID := GetRandomTxHash()
	k.deposits[depositID.String()] = types.DepositRecord{
		DepositID:      depositID,
		Owner:          owner,
		AmountSats:     100_000_000,
		DepositAddress: GetRandomBTCAddress(),
		VaultPubKey:    GetRandomPubKey(),
		OperatorPubKey: GetRandomPubKey(),
		NodePubKey:     GetRandomBech32ConsensusPubKey(),
		Status:         types.DepositStatusDepositMatched,
	}
	_, err := PostShielderShield(ctx, k, owner, depositID, []string{flowNote(t, 100_000_000, "BOND_NOTE")})
	if err == nil || !strings.Contains(err.Error(), "MsgBondFromNotes") {
		t.Fatalf("expected bond deposit shield rejection, got %v", err)
	}
}

func TestNodeFeeShieldAndUnshieldFlow(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	operatorPriv, operatorPubKey := flowOperatorKey(t)
	owner, err := operatorPubKey.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	nodePubKey := GetRandomBech32ConsensusPubKey()
	claim := uint64(1_000_000)
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:     nodePubKey,
		OperatorPubKey: operatorPubKey,
		NodeAddress:    owner,
		Slot:           0,
		BondSats:       100_000_000,
		FeeShareActive: true,
		Bonders:        []string{owner.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, owner)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        owner,
		PrincipalSats: 100_000_000,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.feePool = types.FeePool{
		TotalSlots:         1,
		FeePerSlotShare:    claim,
		TotalCollectedSats: claim,
	}
	notes := []shielderNoteCommitment{{DenominationSats: claim, Commitment: "FEE_NOTE"}}
	notePubKeys := []string{"02b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3"}
	signature := flowSignCompact(t, operatorPriv, shielderFeeClaimPayload(nodePubKey, owner, claim, claim, notes, notePubKeys))
	deposit, err := ShieldShielderFees(ctx, k, owner, nodePubKey, signature, []string{flowNote(t, claim, "FEE_NOTE")}, notePubKeys)
	if err != nil {
		t.Fatal(err)
	}
	if deposit.Settlement != types.DepositSettlementOperatorFee || deposit.Status != types.DepositStatusCommitted {
		t.Fatalf("unexpected fee shield state: %#v", deposit)
	}
	if k.nodeBonds[nodePubKey].FeeDebtSats != claim || k.feePool.TotalClaimedSats != claim {
		t.Fatalf("fee share counter did not advance: bond=%#v pool=%#v", k.nodeBonds[nodePubKey], k.feePool)
	}

	redeem := types.ShielderRedeem{
		WithdrawalID:  "redeem-fee",
		NullifierHash: "nullifier-fee",
		MerkleRoot:    "root-fee",
		Recipient:     GetRandomBTCAddress(),
		AmountSats:    claim,
		InHash:        GetRandomTxHash(),
		VaultPubKey:   GetRandomPubKey(),
		Status:        types.ShielderRedeemStatusAuthorized,
	}
	addTestBTCVaultSourceInput(t, ctx, k, redeem.VaultPubKey, claim+100_000)
	if _, err := QueueAuthorizedWithdrawalTxOut(ctx, k, redeem); err != nil {
		t.Fatal(err)
	}
	if len(k.txOuts) != 1 {
		t.Fatalf("expected fee note withdrawal txout, got %d", len(k.txOuts))
	}
}

func TestNodeFeeShieldAllowsMinFeeSizedClaim(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operatorPriv, operatorPubKey := flowOperatorKey(t)
	owner, err := operatorPubKey.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	nodePubKey := GetRandomBech32ConsensusPubKey()
	claim := uint64(100_000)
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:     nodePubKey,
		OperatorPubKey: operatorPubKey,
		NodeAddress:    owner,
		Slot:           0,
		BondSats:       100_000_000,
		FeeShareActive: true,
		Bonders:        []string{owner.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, owner)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        owner,
		PrincipalSats: 100_000_000,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.feePool = types.FeePool{
		TotalSlots:         1,
		FeePerSlotShare:    claim,
		TotalCollectedSats: claim,
	}
	notes := []shielderNoteCommitment{{DenominationSats: claim, Commitment: "FEE_NOTE_MIN"}}
	notePubKeys := []string{"02b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3"}
	signature := flowSignCompact(t, operatorPriv, shielderFeeClaimPayload(nodePubKey, owner, claim, claim, notes, notePubKeys))
	deposit, err := ShieldShielderFees(ctx, k, owner, nodePubKey, signature, []string{flowNote(t, claim, "FEE_NOTE_MIN")}, notePubKeys)
	if err != nil {
		t.Fatal(err)
	}
	if deposit.AmountSats != claim || deposit.Settlement != types.DepositSettlementOperatorFee || deposit.Status != types.DepositStatusCommitted {
		t.Fatalf("unexpected fee shield state: %#v", deposit)
	}
}

func TestNodeFeeShieldSplitsBetweenBondersAndOperatorCommission(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operatorPubKey := GetRandomPubKey()
	operator, err := operatorPubKey.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	secondBonder := GetRandomBech32Addr()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	gross := uint64(1_000_000)
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:             nodePubKey,
		OperatorPubKey:         operatorPubKey,
		NodeAddress:            operator,
		Slot:                   0,
		BondSats:               400_000_000,
		FeeShareActive:         true,
		OperatorFeeBasisPoints: 1000,
		Bonders:                []string{operator.String(), secondBonder.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, operator)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        operator,
		PrincipalSats: 100_000_000,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, secondBonder)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        secondBonder,
		PrincipalSats: 300_000_000,
		CreatedHeight: 2,
		UpdatedHeight: 2,
	}
	k.feePool = types.FeePool{
		TotalSlots:         1,
		FeePerSlotShare:    gross,
		TotalCollectedSats: gross,
	}

	operatorClaim := uint64(325_000)
	operatorDeposit, err := ShieldShielderFees(ctx, k, operator, nodePubKey, nil, []string{flowNote(t, operatorClaim, "OP_FEE_NOTE")}, []string{"02b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3"})
	if err != nil {
		t.Fatal(err)
	}
	if operatorDeposit.AmountSats != operatorClaim {
		t.Fatalf("unexpected operator claim: %#v", operatorDeposit)
	}
	secondClaim := uint64(675_000)
	secondDeposit, err := ShieldShielderFees(ctx, k, secondBonder, nodePubKey, nil, []string{flowNote(t, secondClaim, "BONDER_FEE_NOTE")}, []string{"03b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3"})
	if err != nil {
		t.Fatal(err)
	}
	if secondDeposit.AmountSats != secondClaim {
		t.Fatalf("unexpected bonder claim: %#v", secondDeposit)
	}
	if k.nodeBonds[nodePubKey].OperatorFeeAccruedSats != 0 || k.feePool.TotalClaimedSats != gross {
		t.Fatalf("unexpected fee accounting: bond=%#v pool=%#v", k.nodeBonds[nodePubKey], k.feePool)
	}
}

func TestNodeOperatorCanSetFeeAndDoesNotRepriceOldAccrual(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operatorPubKey := GetRandomPubKey()
	operator, err := operatorPubKey.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	bonder := GetRandomBech32Addr()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:             nodePubKey,
		OperatorPubKey:         operatorPubKey,
		NodeAddress:            operator,
		Slot:                   0,
		BondSats:               200_000_000,
		FeeShareActive:         true,
		OperatorFeeBasisPoints: 1000,
		Bonders:                []string{operator.String(), bonder.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, operator)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        operator,
		PrincipalSats: 100_000_000,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, bonder)] = types.ShielderNodeBonder{
		NodePubKey:    nodePubKey,
		Bonder:        bonder,
		PrincipalSats: 100_000_000,
		CreatedHeight: 2,
		UpdatedHeight: 2,
	}
	k.feePool = types.FeePool{
		TotalSlots:         1,
		FeePerSlotShare:    1_000_000,
		TotalCollectedSats: 1_000_000,
	}

	bond, err := SetNodeOperatorFeeBasisPoints(ctx, k, operator, nodePubKey, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if bond.OperatorFeeBasisPoints != 2000 {
		t.Fatalf("operator fee was not updated: %#v", bond)
	}
	if bond.OperatorFeeAccruedSats != 100_000 {
		t.Fatalf("old operator commission not frozen before update: %d", bond.OperatorFeeAccruedSats)
	}
	if claim := nodeBonderClaimableSats(ctx, k, bond, k.nodeBonders[types.ShielderNodeBonderKey(nodePubKey, bonder)]); claim != 450_000 {
		t.Fatalf("old bonder claim repriced unexpectedly: %d", claim)
	}

	k.feePool.FeePerSlotShare = 2_000_000
	k.feePool.TotalCollectedSats = 2_000_000
	bond, err = SetNodeOperatorFeeBasisPoints(ctx, k, operator, nodePubKey, 0)
	if err != nil {
		t.Fatal(err)
	}
	if bond.OperatorFeeAccruedSats != 300_000 {
		t.Fatalf("new operator commission not accrued at updated rate: %d", bond.OperatorFeeAccruedSats)
	}
}

func TestNodeOperatorFeeRejectsNonOperator(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operatorPubKey := GetRandomPubKey()
	operator, err := operatorPubKey.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	nonOperator := GetRandomBech32Addr()
	nodePubKey := GetRandomBech32ConsensusPubKey()
	k.nodeBonds[nodePubKey] = types.ShielderNodeBond{
		NodePubKey:     nodePubKey,
		OperatorPubKey: operatorPubKey,
		NodeAddress:    operator,
	}

	if _, err := SetNodeOperatorFeeBasisPoints(ctx, k, nonOperator, nodePubKey, 1000); err == nil {
		t.Fatal("expected non-operator fee update to fail")
	}
	if _, err := SetNodeOperatorFeeBasisPoints(ctx, k, operator, nodePubKey, types.MaxNodeOperatorFeeBasisPoints+1); err == nil {
		t.Fatal("expected excessive operator fee to fail")
	}
}

func TestBidDepositRedeemCreditsBidWithoutOutbound(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	seller := GetRandomBech32Addr()
	bidder := GetRandomBech32Addr()
	oldNodePubKey := GetRandomBech32ConsensusPubKey()
	newNodePubKey := GetRandomBech32ConsensusPubKey()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	auctionID := "auction-bid-redeem"
	bidID := "bid-redeem"
	amount := uint64(k.GetConfigInt64(ctx, constants.NodeSale_BidAmountMinSats))
	k.baseVaults = Vaults{{
		PubKey: GetRandomPubKey(),
		Status: ActiveVault,
		Type:   BaseVault,
	}}
	k.auctions[auctionID] = types.NodeSlotAuction{
		AuctionID:            auctionID,
		Seller:               seller,
		SellerOperatorPubKey: oldOperator,
		SellerNodePubKey:     oldNodePubKey,
		Slot:                 7,
		OriginalBondSats:     amount,
		ReserveSats:          amount,
		ExpiryHeight:         100,
		Status:               types.NodeSlotAuctionOpen,
	}
	k.nodeBonds[oldNodePubKey] = types.ShielderNodeBond{
		NodePubKey:     oldNodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    seller,
		Slot:           7,
		BondSats:       amount,
		FeeShareActive: true,
		Bonders:        []string{seller.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(oldNodePubKey, seller)] = types.ShielderNodeBonder{
		NodePubKey:    oldNodePubKey,
		Bonder:        seller,
		PrincipalSats: amount,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeAccounts[seller.String()] = NewNodeAccount(seller, NodeStandby, common.EmptyPubKeySet, oldNodePubKey, cosmos.NewUint(amount), common.Address(seller.String()), 1)
	k.bids[bidID] = types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auctionID,
		Bidder:         bidder,
		OperatorPubKey: newOperator,
		NodePubKey:     newNodePubKey,
		DepositAddress: common.BondEscrowAddress,
	}
	redeem := types.ShielderRedeem{
		WithdrawalID:    "redeem-bid",
		NullifierHash:   "nullifier-bid",
		MerkleRoot:      "root-bid",
		Recipient:       common.BondEscrowAddress,
		RecipientPolicy: types.ShielderRedeemPolicyBidDeposit,
		BidID:           bidID,
		AmountSats:      amount,
		InHash:          GetRandomTxHash(),
		VaultPubKey:     GetRandomPubKey(),
		Status:          types.ShielderRedeemStatusAuthorized,
	}

	settled, err := FinalizeBidDepositFromNoteSpend(ctx, k, redeem)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Status != types.ShielderRedeemStatusSettled {
		t.Fatalf("unexpected bid redeem status: %#v", settled)
	}
	if len(k.txOuts) != 0 {
		t.Fatalf("bid redeem should not create an outbound txout: %#v", k.txOuts)
	}
	bid := k.bids[bidID]
	if bid.AmountSats != amount || !bid.DepositID.IsEmpty() {
		t.Fatalf("bid was not credited: %#v", bid)
	}
	if _, _, err := SelectNodeSlotBid(ctx, k, seller, auctionID, bidID); err != nil {
		t.Fatalf("credited bid was not selectable: %v", err)
	}
}

func TestNodeBidDepositAndSaleShieldThenSellerUnshield(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	seller := GetRandomBech32Addr()
	bidder := GetRandomBech32Addr()
	oldNodePubKey := GetRandomBech32ConsensusPubKey()
	newNodePubKey := GetRandomBech32ConsensusPubKey()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	shieldPriv, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	depositPubkey := hex.EncodeToString(shieldPriv.PubKey().SerializeCompressed())
	originalBond := uint64(1_000_000)
	bidAmount := uint64(1_500_000)
	auctionID := "auction-1"
	bidID := "bid-1"
	k.baseVaults = Vaults{{
		PubKey: GetRandomPubKey(),
		Status: ActiveVault,
		Type:   BaseVault,
	}}
	k.nodeBonds[oldNodePubKey] = types.ShielderNodeBond{
		NodePubKey:     oldNodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    seller,
		Slot:           4,
		BondSats:       originalBond,
		FeeShareActive: true,
		Bonders:        []string{seller.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(oldNodePubKey, seller)] = types.ShielderNodeBonder{
		NodePubKey:    oldNodePubKey,
		Bonder:        seller,
		PrincipalSats: originalBond,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeAccounts[seller.String()] = NewNodeAccount(seller, NodeStandby, common.EmptyPubKeySet, oldNodePubKey, cosmos.NewUint(originalBond), common.Address(seller.String()), 1)
	k.auctions[auctionID] = types.NodeSlotAuction{
		AuctionID:            auctionID,
		Seller:               seller,
		SellerOperatorPubKey: oldOperator,
		SellerNodePubKey:     oldNodePubKey,
		Slot:                 4,
		OriginalBondSats:     originalBond,
		ReserveSats:          originalBond,
		ExpiryHeight:         100,
		Status:               types.NodeSlotAuctionOpen,
	}
	k.bids[bidID] = types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auctionID,
		Bidder:         bidder,
		OperatorPubKey: newOperator,
		NodePubKey:     newNodePubKey,
		AmountSats:     bidAmount,
		DepositAddress: common.BondEscrowAddress,
	}

	if _, _, err := SelectNodeSlotBid(ctx, k, seller, auctionID, bidID); err != nil {
		t.Fatal(err)
	}
	entitlementID, err := nodeSlotSaleEntitlementID(auctionID, bidID, seller)
	if err != nil {
		t.Fatal(err)
	}
	entitlement := k.deposits[entitlementID.String()]
	if entitlement.Status != types.DepositStatusSettled || entitlement.SellerPayoutSats != originalBond {
		t.Fatalf("seller entitlement was not created: %#v", entitlement)
	}
	commitments := []string{flowNote(t, originalBond, "SELLER_NOTE")}
	signature := flowShieldAuthorization(t, shieldPriv, depositPubkey, originalBond, commitments)
	deposit, err := ShieldNodeSlotSaleEntitlement(ctx, k, seller, auctionID, bidID, depositPubkey, signature, commitments)
	if err != nil {
		t.Fatal(err)
	}
	if deposit.Settlement != types.DepositSettlementOperatorSale || deposit.SellerPayoutSats != originalBond || deposit.ProtocolBondSats != bidAmount-originalBond {
		t.Fatalf("unexpected sale shield state: %#v", deposit)
	}
	if !k.ShielderCommitmentExists(ctx, "SELLER_NOTE") {
		t.Fatal("seller payout note was not inserted")
	}
	if !k.nodeBonds[oldNodePubKey].Sold || k.nodeBonds[oldNodePubKey].FeeShareActive {
		t.Fatalf("old node slot was not sold: %#v", k.nodeBonds[oldNodePubKey])
	}
	oldNode := k.nodeAccounts[seller.String()]
	if oldNode.Status == NodeSelected || !oldNode.Bond.IsZero() {
		t.Fatalf("old node account kept selected/bonded state after sale: %#v", oldNode)
	}
	newBond := k.nodeBonds[newNodePubKey]
	if newBond.Slot != 4 || newBond.BondSats != bidAmount || !newBond.FeeShareActive {
		t.Fatalf("new node did not receive sold slot: %#v", newBond)
	}
	newOperatorAddress, err := newOperator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	newNode := k.nodeAccounts[newOperatorAddress.String()]
	if newNode.NodeConsPubKey != newNodePubKey || newNode.BondAddress.String() != newOperatorAddress.String() {
		t.Fatalf("buyer was not installed as new operator: %#v", newNode)
	}

	redeem := types.ShielderRedeem{
		WithdrawalID:  "redeem-sale",
		NullifierHash: "nullifier-sale",
		MerkleRoot:    "root-sale",
		Recipient:     GetRandomBTCAddress(),
		AmountSats:    originalBond,
		InHash:        GetRandomTxHash(),
		VaultPubKey:   deposit.VaultPubKey,
		Status:        types.ShielderRedeemStatusAuthorized,
	}
	addTestBTCVaultSourceInput(t, ctx, k, redeem.VaultPubKey, originalBond+100_000)
	if _, err := QueueAuthorizedWithdrawalTxOut(ctx, k, redeem); err != nil {
		t.Fatal(err)
	}
	if len(k.txOuts) != 1 || k.txOuts[0].GetTxType() != types.TxOutTypeOut {
		t.Fatalf("sale seller note did not unshield through normal withdrawal: %#v", k.txOuts)
	}
}

func TestNodeSaleDistributesPrincipalRecoveryAcrossBonders(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	operator := GetRandomBech32Addr()
	secondBonder := GetRandomBech32Addr()
	bidder := GetRandomBech32Addr()
	oldNodePubKey := GetRandomBech32ConsensusPubKey()
	newNodePubKey := GetRandomBech32ConsensusPubKey()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	auctionID := "auction-pro-rata"
	bidID := "bid-pro-rata"
	k.baseVaults = Vaults{{
		PubKey: GetRandomPubKey(),
		Status: ActiveVault,
		Type:   BaseVault,
	}}
	k.nodeBonds[oldNodePubKey] = types.ShielderNodeBond{
		NodePubKey:     oldNodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    operator,
		Slot:           11,
		BondSats:       4_000_000,
		FeeShareActive: true,
		Bonders:        []string{operator.String(), secondBonder.String()},
	}
	k.nodeBonders[types.ShielderNodeBonderKey(oldNodePubKey, operator)] = types.ShielderNodeBonder{
		NodePubKey:    oldNodePubKey,
		Bonder:        operator,
		PrincipalSats: 1_000_000,
		CreatedHeight: 1,
		UpdatedHeight: 1,
	}
	k.nodeBonders[types.ShielderNodeBonderKey(oldNodePubKey, secondBonder)] = types.ShielderNodeBonder{
		NodePubKey:    oldNodePubKey,
		Bonder:        secondBonder,
		PrincipalSats: 3_000_000,
		CreatedHeight: 2,
		UpdatedHeight: 2,
	}
	k.nodeAccounts[operator.String()] = NewNodeAccount(operator, NodeStandby, common.EmptyPubKeySet, oldNodePubKey, cosmos.NewUint(4_000_000), common.Address(operator.String()), 1)
	k.auctions[auctionID] = types.NodeSlotAuction{
		AuctionID:            auctionID,
		Seller:               operator,
		SellerOperatorPubKey: oldOperator,
		SellerNodePubKey:     oldNodePubKey,
		Slot:                 11,
		OriginalBondSats:     4_000_000,
		ReserveSats:          2_000_000,
		ExpiryHeight:         100,
		Status:               types.NodeSlotAuctionOpen,
	}
	k.bids[bidID] = types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auctionID,
		Bidder:         bidder,
		OperatorPubKey: newOperator,
		NodePubKey:     newNodePubKey,
		AmountSats:     2_000_000,
		DepositAddress: common.BondEscrowAddress,
	}

	if _, _, err := SelectNodeSlotBid(ctx, k, operator, auctionID, bidID); err != nil {
		t.Fatal(err)
	}
	operatorID, err := nodeSlotSaleEntitlementID(auctionID, bidID, operator)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := nodeSlotSaleEntitlementID(auctionID, bidID, secondBonder)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.deposits[operatorID.String()].AmountSats; got != 500_000 {
		t.Fatalf("unexpected operator principal recovery: %d", got)
	}
	if got := k.deposits[secondID.String()].AmountSats; got != 1_500_000 {
		t.Fatalf("unexpected second bonder principal recovery: %d", got)
	}
	if k.nodeBonders[types.ShielderNodeBonderKey(oldNodePubKey, operator)].PrincipalSats != 0 || k.nodeBonders[types.ShielderNodeBonderKey(oldNodePubKey, secondBonder)].PrincipalSats != 0 {
		t.Fatalf("old bonder principal was not cleared: %#v", k.nodeBonders)
	}
	newBond := k.nodeBonds[newNodePubKey]
	if newBond.BondSats != 2_000_000 || newBond.Slot != 11 || !newBond.FeeShareActive {
		t.Fatalf("new bond did not receive sold slot: %#v", newBond)
	}
	if got := k.nodeBonders[types.ShielderNodeBonderKey(newNodePubKey, bidder)].PrincipalSats; got != 2_000_000 {
		t.Fatalf("new bonder principal not recorded: %d", got)
	}
}

func TestAuctionRejectsBelowReserveWithoutMutation(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	seller := GetRandomBech32Addr()
	bidder := GetRandomBech32Addr()
	oldNodePubKey := GetRandomBech32ConsensusPubKey()
	newNodePubKey := GetRandomBech32ConsensusPubKey()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	auctionID := "auction-below-reserve"
	bidID := "bid-below-reserve"
	k.nodeBonds[oldNodePubKey] = types.ShielderNodeBond{
		NodePubKey:     oldNodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    seller,
		Slot:           7,
		BondSats:       1_000_000,
		FeeShareActive: true,
	}
	k.nodeAccounts[seller.String()] = NewNodeAccount(seller, NodeStandby, common.EmptyPubKeySet, oldNodePubKey, cosmos.NewUint(1_000_000), common.Address(seller.String()), 1)
	k.auctions[auctionID] = types.NodeSlotAuction{
		AuctionID:            auctionID,
		Seller:               seller,
		SellerOperatorPubKey: oldOperator,
		SellerNodePubKey:     oldNodePubKey,
		Slot:                 7,
		OriginalBondSats:     1_000_000,
		ReserveSats:          2_000_000,
		ExpiryHeight:         100,
		Status:               types.NodeSlotAuctionOpen,
	}
	k.bids[bidID] = types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auctionID,
		Bidder:         bidder,
		OperatorPubKey: newOperator,
		NodePubKey:     newNodePubKey,
		AmountSats:     1_500_000,
		DepositAddress: common.BondEscrowAddress,
	}

	if _, _, err := SelectNodeSlotBid(ctx, k, seller, auctionID, bidID); err == nil || !strings.Contains(err.Error(), "below reserve") {
		t.Fatalf("expected below-reserve bid rejection, got %v", err)
	}
	if k.auctions[auctionID].Status != types.NodeSlotAuctionOpen || k.auctions[auctionID].SelectedBidID != "" {
		t.Fatalf("below-reserve bid mutated auction: %#v", k.auctions[auctionID])
	}
	if k.bids[bidID].Selected || k.bids[bidID].Settled {
		t.Fatalf("below-reserve bid mutated bid: %#v", k.bids[bidID])
	}
	if k.nodeBonds[oldNodePubKey].Sold || k.nodeBonds[newNodePubKey].NodePubKey != "" {
		t.Fatalf("below-reserve bid mutated bonds: old=%#v new=%#v", k.nodeBonds[oldNodePubKey], k.nodeBonds[newNodePubKey])
	}
}

func TestExpiredAuctionRejectsSelectionAndBidRedeemWithoutBondTransfer(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext().WithBlockHeight(200)
	k := newShielderFlowTestKeeper()
	seller := GetRandomBech32Addr()
	bidder := GetRandomBech32Addr()
	oldNodePubKey := GetRandomBech32ConsensusPubKey()
	newNodePubKey := GetRandomBech32ConsensusPubKey()
	oldOperator := GetRandomPubKey()
	newOperator := GetRandomPubKey()
	auctionID := "auction-expired"
	bidID := "bid-expired"
	k.nodeBonds[oldNodePubKey] = types.ShielderNodeBond{
		NodePubKey:     oldNodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    seller,
		Slot:           8,
		BondSats:       1_000_000,
		FeeShareActive: true,
	}
	k.nodeAccounts[seller.String()] = NewNodeAccount(seller, NodeStandby, common.EmptyPubKeySet, oldNodePubKey, cosmos.NewUint(1_000_000), common.Address(seller.String()), 1)
	k.auctions[auctionID] = types.NodeSlotAuction{
		AuctionID:            auctionID,
		Seller:               seller,
		SellerOperatorPubKey: oldOperator,
		SellerNodePubKey:     oldNodePubKey,
		Slot:                 8,
		OriginalBondSats:     1_000_000,
		ReserveSats:          1_000_000,
		ExpiryHeight:         100,
		Status:               types.NodeSlotAuctionOpen,
	}
	k.bids[bidID] = types.NodeSlotBid{
		BidID:          bidID,
		AuctionID:      auctionID,
		Bidder:         bidder,
		OperatorPubKey: newOperator,
		NodePubKey:     newNodePubKey,
		DepositAddress: common.BondEscrowAddress,
	}
	redeem := types.ShielderRedeem{
		WithdrawalID:    "redeem-expired-bid",
		NullifierHash:   "nullifier-expired-bid",
		MerkleRoot:      "root-expired-bid",
		Recipient:       common.BondEscrowAddress,
		RecipientPolicy: types.ShielderRedeemPolicyBidDeposit,
		BidID:           bidID,
		AmountSats:      1_000_000,
		InHash:          GetRandomTxHash(),
		VaultPubKey:     GetRandomPubKey(),
		Status:          types.ShielderRedeemStatusAuthorized,
	}

	if _, err := FinalizeBidDepositFromNoteSpend(ctx, k, redeem); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired bid redeem rejection, got %v", err)
	}
	if k.bids[bidID].AmountSats != 0 || k.redeems[redeem.WithdrawalID].WithdrawalID != "" {
		t.Fatalf("expired bid redeem mutated state: bid=%#v redeem=%#v", k.bids[bidID], k.redeems[redeem.WithdrawalID])
	}
	if _, _, err := SelectNodeSlotBid(ctx, k, seller, auctionID, bidID); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired bid selection rejection, got %v", err)
	}
	if k.auctions[auctionID].Status != types.NodeSlotAuctionExpired {
		t.Fatalf("expired auction should only be marked expired, got %#v", k.auctions[auctionID])
	}
	if k.nodeBonds[oldNodePubKey].Sold || k.nodeBonds[newNodePubKey].NodePubKey != "" {
		t.Fatalf("expired auction transferred bond: old=%#v new=%#v", k.nodeBonds[oldNodePubKey], k.nodeBonds[newNodePubKey])
	}
}

func TestSoldNodeMetadataDoesNotRestoreAuctionEligibility(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	oldNodePubKey := GetRandomBech32ConsensusPubKey()
	oldOperator := GetRandomPubKey()
	seller, err := oldOperator.GetThorAddress()
	if err != nil {
		t.Fatal(err)
	}
	k.nodeBonds[oldNodePubKey] = types.ShielderNodeBond{
		NodePubKey:     oldNodePubKey,
		OperatorPubKey: oldOperator,
		NodeAddress:    seller,
		Slot:           9,
		BondSats:       0,
		FeeShareActive: false,
		Sold:           true,
		SoldAuctionID:  "settled-auction",
	}
	nodeAccount := NewNodeAccount(seller, NodeStandby, common.EmptyPubKeySet, oldNodePubKey, cosmos.ZeroUint(), common.Address(seller.String()), 1)
	nodeAccount.IPAddress = "127.0.0.1"
	nodeAccount.Version = constants.SWVersion.String()
	if err := k.SetNodeAccount(ctx, nodeAccount); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateNodeSlotAuction(ctx, k, seller, oldNodePubKey, 100_000_000, 100); err == nil || !strings.Contains(err.Error(), "no active bonded slot") {
		t.Fatalf("expected sold node auction rejection, got %v", err)
	}
	if len(k.auctions) != 0 {
		t.Fatalf("sold node auction create wrote auction: %#v", k.auctions)
	}
	bond := k.nodeBonds[oldNodePubKey]
	if !bond.Sold || bond.BondSats != 0 || bond.FeeShareActive {
		t.Fatalf("sold node metadata restored bond eligibility: %#v", bond)
	}
}

func TestDepositRequestRoutesToOnlyActiveVaultAfterRotation(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	oldVault := NewVaultV2(10, RetiringVault, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	newVault := NewVaultV2(20, ActiveVault, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{oldVault, newVault}
	owner := GetRandomBech32Addr()
	powToken := validDepositPowToken(t, ctx, k, owner, "post-churn")

	session, err := RegisterDepositPowToken(ctx, k, owner, powToken, 25)
	if err != nil {
		t.Fatal(err)
	}
	if !session.VaultPubKey.Equals(newVault.PubKey) {
		t.Fatalf("deposit routed to wrong vault: got %s want %s", session.VaultPubKey, newVault.PubKey)
	}
	oldAddr, err := oldVault.DeriveBTCAddress(session.DepositPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	if session.DepositAddress.Equals(oldAddr) {
		t.Fatalf("deposit address was issued on retiring vault: %s", session.DepositAddress)
	}
	record := k.addresses[session.DepositAddress.String()]
	if !record.VaultPubKey.Equals(newVault.PubKey) || record.PathIndex != session.DepositPathIndex {
		t.Fatalf("deposit address mapping does not match active vault session: session=%#v mapping=%#v", session, record)
	}
}

func TestDepositPowReplayDoesNotAllocateSecondPath(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	vault := NewVaultV2(20, ActiveVault, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	k.baseVaults = Vaults{vault}
	owner := GetRandomBech32Addr()
	powToken := validDepositPowToken(t, ctx, k, owner, "replay")

	first, err := RegisterDepositPowToken(ctx, k, owner, powToken, 25)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterDepositPowToken(ctx, k, owner, powToken, 25); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected POW replay rejection, got %v", err)
	}
	if len(k.addresses) != 1 {
		t.Fatalf("POW replay allocated extra deposit address: %#v", k.addresses)
	}
	stored := k.sessions[owner.String()]
	if !stored.DepositAddress.Equals(first.DepositAddress) || stored.DepositPathIndex != first.DepositPathIndex {
		t.Fatalf("POW replay mutated session: first=%#v stored=%#v", first, stored)
	}
}

func TestExpiredDepositRefundSubtractsFee(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 1
	baseVault := Vault{
		PubKey: GetRandomPubKey(),
		Status: ActiveVault,
		Type:   BaseVault,
	}
	k.baseVaults = Vaults{baseVault}
	mgr := newShielderFlowTestManager(k)
	amount := uint64(1_000_000)
	deposit := types.DepositRecord{
		DepositID:        GetRandomTxHash(),
		AmountSats:       amount,
		ReturnAddress:    GetRandomBTCAddress(),
		VaultPubKey:      GetRandomPubKey(),
		DepositPathIndex: 7,
	}
	sourceInput := addTestBTCVaultSourceInput(t, ctx, k, baseVault.PubKey, amount+100_000)
	if err := queueExpiredDepositReturn(ctx, mgr, deposit); err != nil {
		t.Fatal(err)
	}
	if len(k.txOuts) != 1 {
		t.Fatalf("expected one refund txout, got %d", len(k.txOuts))
	}
	fee := withdrawalFeeSats(ctx, k, amount)
	if got := k.txOuts[0].Coin.Amount.Uint64(); got != amount-fee {
		t.Fatalf("refund did not subtract fee: %d/%d", got, amount-fee)
	}
	expectedGas, err := btcExactGasCoin(baseVault.PubKey, common.MainVaultPathIndex, []common.Address{deposit.ReturnAddress}, []types.TxOutInput{sourceInput}, 14)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.txOuts[0].MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount.Uint64(); got != expectedGas.Amount.Uint64() {
		t.Fatalf("unexpected refund max gas: %d/%d", got, expectedGas.Amount.Uint64())
	}
	if got := k.txOuts[0].GasRate; got != 14 {
		t.Fatalf("unexpected refund gas rate: %d", got)
	}
	if len(k.txOuts[0].SourceInputs) != 1 || !k.txOuts[0].SourceInputs[0].TxId.Equals(sourceInput.TxId) {
		t.Fatalf("unexpected refund source inputs: %#v", k.txOuts[0].SourceInputs)
	}
	if k.txOuts[0].GetTxType() != types.TxOutTypeRefund {
		t.Fatalf("unexpected txout type: %s", k.txOuts[0].GetTxType())
	}
	if !k.txOuts[0].VaultPubKey.Equals(baseVault.PubKey) {
		t.Fatalf("refund should spend from base vault: %s/%s", k.txOuts[0].VaultPubKey, baseVault.PubKey)
	}
	if k.txOuts[0].VaultPathIndex != common.MainVaultPathIndex {
		t.Fatalf("refund should spend from base vault path: %d", k.txOuts[0].VaultPathIndex)
	}
	if k.feePool.TotalCollectedSats != fee {
		t.Fatalf("refund fee was not collected: %#v", k.feePool)
	}
}

func flowNote(t *testing.T, amount uint64, commitment string) string {
	t.Helper()
	raw, err := json.Marshal(shielderNoteCommitment{DenominationSats: amount, Commitment: commitment})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func addTestBTCVaultSourceInput(t *testing.T, ctx cosmos.Context, k *shielderFlowTestKeeper, vaultPubKey common.PubKey, amountSats uint64) types.TxOutInput {
	t.Helper()
	vault, err := k.GetVault(ctx, vaultPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if vault.PubKey.IsEmpty() {
		vault = Vault{
			PubKey: vaultPubKey,
			Status: ActiveVault,
			Type:   BaseVault,
		}
		if err := k.SetVault(ctx, vault); err != nil {
			t.Fatal(err)
		}
	}
	sourceAddr, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		sourceAddr,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(amountSats))),
		common.Gas{},
	)
	tx.SourceVout = 0
	k.SetObservedTxInVoter(ctx, ObservedTxVoter{
		TxID:   txID,
		Height: ctx.BlockHeight(),
		Tx:     common.NewObservedTx(tx, ctx.BlockHeight(), vaultPubKey, ctx.BlockHeight()),
	})
	return types.TxOutInput{TxId: txID, Vout: 0, AmountSats: amountSats}
}

func flowShieldAuthorization(t *testing.T, priv *btcec.PrivateKey, depositPubkey string, amount uint64, commitments []string) string {
	t.Helper()
	notes := make([]shielderNoteCommitment, 0, len(commitments))
	for _, raw := range commitments {
		var note shielderNoteCommitment
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &note); err != nil {
			t.Fatal(err)
		}
		note.Commitment = strings.TrimSpace(note.Commitment)
		notes = append(notes, note)
	}
	commitmentsJSON, err := json.Marshal(notes)
	if err != nil {
		t.Fatal(err)
	}
	digest := hashLengthPrefixedParts([]string{
		"thornado-shielder-v1",
		"shield-authorization",
		strings.TrimSpace(depositPubkey),
		strings.TrimSpace(depositPubkey),
		fmt.Sprintf("%d", amount),
		string(commitmentsJSON),
	})
	signature, err := priv.Sign(digest)
	if err != nil {
		t.Fatal(err)
	}
	s := new(big.Int).Set(signature.S)
	halfOrder := new(big.Int).Rsh(btcec.S256().N, 1)
	if s.Cmp(halfOrder) == 1 {
		s.Sub(btcec.S256().N, s)
		signature.S = s
	}
	return hex.EncodeToString(signature.Serialize())
}

func flowTestContext() cosmos.Context {
	return cosmos.Context{}.WithLogger(log.NewNopLogger())
}

func validDepositPowToken(t *testing.T, ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, prefix string) string {
	t.Helper()
	for i := 0; i < 5_000_000; i++ {
		token := fmt.Sprintf("%s-%d", prefix, i)
		if err := validateDepositPowToken(ctx, k, owner, token); err == nil {
			return token
		}
	}
	t.Fatal("unable to find valid deposit POW token")
	return ""
}

func flowOperatorKey(t *testing.T) (*btcec.PrivateKey, common.PubKey) {
	t.Helper()
	priv, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	pub := &sdksecp256k1.PubKey{Key: priv.PubKey().SerializeCompressed()}
	pubKeyString, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pub)
	if err != nil {
		t.Fatal(err)
	}
	pubKey, err := common.NewPubKey(pubKeyString)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pubKey
}

func flowSignCompact(t *testing.T, priv *btcec.PrivateKey, payload []byte) []byte {
	t.Helper()
	signature, err := priv.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	s := new(big.Int).Set(signature.S)
	halfOrder := new(big.Int).Rsh(btcec.S256().N, 1)
	if s.Cmp(halfOrder) == 1 {
		s.Sub(btcec.S256().N, s)
	}
	out := make([]byte, 64)
	rb := signature.R.Bytes()
	sb := s.Bytes()
	if len(rb) > 32 || len(sb) > 32 {
		t.Fatalf("invalid signature limbs: r=%s s=%s", hex.EncodeToString(rb), hex.EncodeToString(sb))
	}
	copy(out[32-len(rb):32], rb)
	copy(out[64-len(sb):], sb)
	return out
}

func TestFlowSignCompactSanity(t *testing.T) {
	priv, pub := flowOperatorKey(t)
	payload := []byte(fmt.Sprintf("%064x", 1))[:32]
	if err := verifySecp256K1SignaturePayload(pub, flowSignCompact(t, priv, payload), payload); err != nil {
		t.Fatal(err)
	}
}
