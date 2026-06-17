package thornado

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
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

	feePool       types.FeePool
	deposits      map[string]types.DepositRecord
	commitments   map[string]bool
	denominations map[uint64][]string
	merkleRoots   map[uint64]string
	redeems       map[string]types.ShielderRedeem
	nodeBonds     map[string]types.ShielderNodeBond
	nodeAccounts  map[string]NodeAccount
	auctions      map[string]types.NodeSlotAuction
	bids          map[string]types.NodeSlotBid
	baseVaults    Vaults
	feeNotePubKey map[string]bool
	txOuts        []TxOutItem
	nextSlot      uint64
}

func newShielderFlowTestKeeper() *shielderFlowTestKeeper {
	return &shielderFlowTestKeeper{
		deposits:      make(map[string]types.DepositRecord),
		commitments:   make(map[string]bool),
		denominations: make(map[uint64][]string),
		merkleRoots:   make(map[uint64]string),
		redeems:       make(map[string]types.ShielderRedeem),
		nodeBonds:     make(map[string]types.ShielderNodeBond),
		nodeAccounts:  make(map[string]NodeAccount),
		auctions:      make(map[string]types.NodeSlotAuction),
		bids:          make(map[string]types.NodeSlotBid),
		feeNotePubKey: make(map[string]bool),
	}
}

func (k *shielderFlowTestKeeper) GetConfigInt64(_ cosmos.Context, key constants.ConfigName) int64 {
	return constants.NewConfigValue().GetInt64Value(key)
}

func (k *shielderFlowTestKeeper) SetDepositRecord(_ cosmos.Context, deposit types.DepositRecord) error {
	k.deposits[deposit.DepositID.String()] = deposit
	return nil
}

func (k *shielderFlowTestKeeper) GetDepositRecord(_ cosmos.Context, depositID common.TxID) (types.DepositRecord, error) {
	return k.deposits[depositID.String()], nil
}

func (k *shielderFlowTestKeeper) SetShielderCommitment(_ cosmos.Context, commitment string) error {
	k.commitments[commitment] = true
	return nil
}

func (k *shielderFlowTestKeeper) ShielderCommitmentExists(_ cosmos.Context, commitment string) bool {
	_, ok := k.commitments[commitment]
	return ok
}

func (k *shielderFlowTestKeeper) SetShielderNoteRecord(_ cosmos.Context, _ types.StoredShielderNoteRecord) error {
	return nil
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

func (k *shielderFlowTestKeeper) SetShielderFeeNotePubKey(_ cosmos.Context, pubKey string) error {
	k.feeNotePubKey[pubKey] = true
	return nil
}

func (k *shielderFlowTestKeeper) ShielderFeeNotePubKeyUsed(_ cosmos.Context, pubKey string) bool {
	_, ok := k.feeNotePubKey[pubKey]
	return ok
}

func (k *shielderFlowTestKeeper) GetNetworkFee(_ cosmos.Context, _ common.Chain) (NetworkFee, error) {
	return NetworkFee{}, nil
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
func (m *shielderFlowTestManager) EventMgr() EventManager               { return nil }
func (m *shielderFlowTestManager) TxOutStore() TxOutStore               { return &m.txOut }
func (m *shielderFlowTestManager) NetworkMgr() NetworkManager           { return nil }
func (m *shielderFlowTestManager) NodeMgr() NodeManager                 { return nil }
func (m *shielderFlowTestManager) ObMgr() ObserverManager               { return nil }
func (m *shielderFlowTestManager) PenaltyManager() PenaltyManager       { return nil }

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
	if _, err := QueueAuthorizedWithdrawalTxOut(ctx, k, redeem); err != nil {
		t.Fatal(err)
	}
	if len(k.txOuts) != 1 {
		t.Fatalf("expected fee note withdrawal txout, got %d", len(k.txOuts))
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
	entitlementID, err := nodeSlotSaleEntitlementID(auctionID, bidID)
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
	newBond := k.nodeBonds[newNodePubKey]
	if newBond.Slot != 4 || newBond.BondSats != bidAmount || !newBond.FeeShareActive {
		t.Fatalf("new node did not receive sold slot: %#v", newBond)
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
	if _, err := QueueAuthorizedWithdrawalTxOut(ctx, k, redeem); err != nil {
		t.Fatal(err)
	}
	if len(k.txOuts) != 1 || k.txOuts[0].GetTxType() != types.TxOutTypeOut {
		t.Fatalf("sale seller note did not unshield through normal withdrawal: %#v", k.txOuts)
	}
}

func TestExpiredDepositRefundSubtractsFee(t *testing.T) {
	SetupConfigForTest()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
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
