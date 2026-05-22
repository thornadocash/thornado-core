package keeperv1

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cosmos/cosmos-sdk/runtime"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	kvTypes "github.com/thornadocash/go-thornado/x/thornado/keeper/types"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func (k KVStore) setShielderJSON(ctx cosmos.Context, key []byte, record any) error {
	buf, err := json.Marshal(record)
	if err != nil {
		return dbError(ctx, fmt.Sprintf("marshal shielder record: %s", key), err)
	}
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(key, buf)
	return nil
}

func (k KVStore) getShielderJSON(ctx cosmos.Context, key []byte, record any) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}
	if err := json.Unmarshal(store.Get(key), record); err != nil {
		return true, dbError(ctx, fmt.Sprintf("unmarshal shielder record: %s", key), err)
	}
	return true, nil
}

func (k KVStore) SetShielderSession(ctx cosmos.Context, session types.ShielderSession) error {
	if err := session.Valid(); err != nil {
		return err
	}
	if err := k.setShielderJSON(ctx, k.GetKey(prefixShielderSession, session.Key()), session); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderPowToken, session.PowToken), session.Key())
}

func (k KVStore) GetShielderSession(ctx cosmos.Context, owner cosmos.AccAddress) (types.ShielderSession, error) {
	record := types.ShielderSession{Owner: owner}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderSession, owner.String()), &record)
	return record, err
}

func (k KVStore) GetShielderSessionByPowToken(ctx cosmos.Context, powToken string) (types.ShielderSession, error) {
	var owner string
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderPowToken, strings.TrimSpace(powToken)), &owner)
	if err != nil {
		return types.ShielderSession{}, err
	}
	if !found || owner == "" {
		return types.ShielderSession{}, fmt.Errorf("shielder pow token not found")
	}
	addr, err := cosmos.AccAddressFromBech32(owner)
	if err != nil {
		return types.ShielderSession{}, err
	}
	return k.GetShielderSession(ctx, addr)
}

func (k KVStore) SetShielderDepositAddress(ctx cosmos.Context, record types.ShielderDepositAddress) error {
	if err := record.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderDepositAddress, record.Key()), record)
}

func (k KVStore) GetShielderDepositAddress(ctx cosmos.Context, address common.Address) (types.ShielderDepositAddress, error) {
	record := types.ShielderDepositAddress{Address: address}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderDepositAddress, address.String()), &record)
	return record, err
}

func (k KVStore) GetShielderDepositAddressIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderDepositAddress)
}

func (k KVStore) GetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey) (uint64, error) {
	if vaultPubKey.IsEmpty() {
		return 0, fmt.Errorf("missing vault pubkey")
	}
	var index uint64
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixVaultDepositPathIndex, vaultPubKey.String()), &index)
	if err != nil {
		return 0, err
	}
	if !found || index == 0 {
		return common.FirstDepositPathIndex, nil
	}
	return index, nil
}

func (k KVStore) SetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, index uint64) error {
	if vaultPubKey.IsEmpty() {
		return fmt.Errorf("missing vault pubkey")
	}
	if index == 0 {
		return fmt.Errorf("vault deposit path index cannot be zero")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixVaultDepositPathIndex, vaultPubKey.String()), index)
}

func (k KVStore) AllocateVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey) (uint64, error) {
	index, err := k.GetNextVaultDepositPathIndex(ctx, vaultPubKey)
	if err != nil {
		return 0, err
	}
	if err := k.SetNextVaultDepositPathIndex(ctx, vaultPubKey, index+1); err != nil {
		return 0, err
	}
	return index, nil
}

func (k KVStore) SetShielderDeposit(ctx cosmos.Context, deposit types.ShielderDeposit) error {
	if err := deposit.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderDeposit, deposit.Key()), deposit)
}

func (k KVStore) GetShielderDeposit(ctx cosmos.Context, depositID common.TxID) (types.ShielderDeposit, error) {
	record := types.ShielderDeposit{DepositID: depositID}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderDeposit, depositID.String()), &record)
	return record, err
}

func (k KVStore) GetShielderDepositIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderDeposit)
}

func (k KVStore) SetShielderCommitment(ctx cosmos.Context, commitment string, depositID common.TxID) error {
	commitment = strings.TrimSpace(commitment)
	if commitment == "" {
		return fmt.Errorf("missing shielder commitment")
	}
	if depositID.IsEmpty() {
		return fmt.Errorf("missing shielder commitment deposit id")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderCommitment, commitment), depositID.String())
}

func (k KVStore) ShielderCommitmentExists(ctx cosmos.Context, commitment string) bool {
	return k.has(ctx, k.GetKey(prefixShielderCommitment, strings.TrimSpace(commitment)))
}

func (k KVStore) SetShielderDenominationCommitment(ctx cosmos.Context, denominationSats uint64, commitment string, depositID common.TxID) error {
	commitment = strings.TrimSpace(commitment)
	if denominationSats == 0 {
		return fmt.Errorf("missing shielder commitment denomination")
	}
	if commitment == "" {
		return fmt.Errorf("missing shielder commitment")
	}
	if depositID.IsEmpty() {
		return fmt.Errorf("missing shielder commitment deposit id")
	}
	return k.setShielderJSON(ctx, shielderDenominationCommitmentKey(denominationSats, commitment), depositID.String())
}

func (k KVStore) GetShielderDenominationCommitments(ctx cosmos.Context, denominationSats uint64) ([]string, error) {
	if denominationSats == 0 {
		return nil, fmt.Errorf("missing shielder commitment denomination")
	}
	iter := k.getIterator(ctx, kvTypes.DbPrefix(shielderDenominationPrefix(denominationSats)))
	defer iter.Close()

	var commitments []string
	for ; iter.Valid(); iter.Next() {
		key := string(iter.Key())
		idx := strings.LastIndex(key, "/")
		if idx < 0 || idx == len(key)-1 {
			continue
		}
		commitments = append(commitments, key[idx+1:])
	}
	sort.Strings(commitments)
	return commitments, nil
}

func (k KVStore) SetShielderMerkleRoot(ctx cosmos.Context, denominationSats uint64, root string) error {
	root = strings.TrimSpace(root)
	if denominationSats == 0 {
		return fmt.Errorf("missing shielder merkle root denomination")
	}
	if root == "" {
		return fmt.Errorf("missing shielder merkle root")
	}
	return k.setShielderJSON(ctx, shielderMerkleRootKey(denominationSats, root), true)
}

func (k KVStore) ShielderMerkleRootExists(ctx cosmos.Context, denominationSats uint64, root string) bool {
	if denominationSats == 0 {
		return false
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	return k.has(ctx, shielderMerkleRootKey(denominationSats, root))
}

func (k KVStore) GetShielderMerkleRootIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderMerkleRoot)
}

func shielderDenominationPrefix(denominationSats uint64) string {
	return fmt.Sprintf("%s%020d/", prefixShielderDenomCommitment, denominationSats)
}

func shielderDenominationCommitmentKey(denominationSats uint64, commitment string) []byte {
	return []byte(shielderDenominationPrefix(denominationSats) + strings.ToUpper(strings.TrimSpace(commitment)))
}

func shielderMerkleRootKey(denominationSats uint64, root string) []byte {
	return []byte(fmt.Sprintf("%s%020d/%s", prefixShielderMerkleRoot, denominationSats, strings.ToUpper(strings.TrimSpace(root))))
}

func (k KVStore) SetShielderWithdrawal(ctx cosmos.Context, withdrawal types.ShielderWithdrawal) error {
	if err := withdrawal.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderWithdrawal, withdrawal.Key()), withdrawal)
}

func (k KVStore) GetShielderWithdrawal(ctx cosmos.Context, withdrawalID string) (types.ShielderWithdrawal, error) {
	record := types.ShielderWithdrawal{WithdrawalID: withdrawalID}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderWithdrawal, withdrawalID), &record)
	return record, err
}

func (k KVStore) GetShielderWithdrawalByNullifier(ctx cosmos.Context, nullifierHash string) (types.ShielderWithdrawal, error) {
	var withdrawalID string
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderNullifier, strings.TrimSpace(nullifierHash)), &withdrawalID)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if !found || withdrawalID == "" {
		return types.ShielderWithdrawal{}, nil
	}
	return k.GetShielderWithdrawal(ctx, withdrawalID)
}

func (k KVStore) SetShielderNullifierSpent(ctx cosmos.Context, nullifierHash string, withdrawalID string) error {
	nullifierHash = strings.TrimSpace(nullifierHash)
	if nullifierHash == "" {
		return fmt.Errorf("missing shielder nullifier")
	}
	if strings.TrimSpace(withdrawalID) == "" {
		return fmt.Errorf("missing shielder withdrawal id")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderNullifier, nullifierHash), withdrawalID)
}

func (k KVStore) ShielderNullifierSpent(ctx cosmos.Context, nullifierHash string) bool {
	return k.has(ctx, k.GetKey(prefixShielderNullifier, strings.TrimSpace(nullifierHash)))
}

func (k KVStore) GetNextShielderNodeBondSlot(ctx cosmos.Context) (uint64, error) {
	var slot uint64
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderNodeBondSlot, "next"), &slot)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	return slot, nil
}

func (k KVStore) SetNextShielderNodeBondSlot(ctx cosmos.Context, slot uint64) error {
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderNodeBondSlot, "next"), slot)
}

func (k KVStore) AllocateShielderNodeBondSlot(ctx cosmos.Context) (uint64, error) {
	slot, err := k.GetNextShielderNodeBondSlot(ctx)
	if err != nil {
		return 0, err
	}
	if err := k.SetNextShielderNodeBondSlot(ctx, slot+1); err != nil {
		return 0, err
	}
	return slot, nil
}

func (k KVStore) SetShielderNodeBond(ctx cosmos.Context, bond types.ShielderNodeBond) error {
	if err := bond.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderNodeBond, bond.Key()), bond)
}

func (k KVStore) GetShielderNodeBond(ctx cosmos.Context, nodePubKey string) (types.ShielderNodeBond, error) {
	record := types.ShielderNodeBond{NodePubKey: strings.TrimSpace(nodePubKey)}
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderNodeBond, record.Key()), &record)
	if err != nil || !found {
		return types.ShielderNodeBond{}, err
	}
	return record, err
}

func (k KVStore) GetShielderNodeBondIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderNodeBond)
}

func (k KVStore) SetShielderFeePool(ctx cosmos.Context, pool types.ShielderFeePool) error {
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderFeePool, "global"), pool)
}

func (k KVStore) GetShielderFeePool(ctx cosmos.Context) (types.ShielderFeePool, error) {
	var pool types.ShielderFeePool
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderFeePool, "global"), &pool)
	return pool, err
}

func (k KVStore) SetShielderFeeNotePubKey(ctx cosmos.Context, pubKey common.PubKey, depositID common.TxID) error {
	if pubKey.IsEmpty() {
		return fmt.Errorf("missing shielder fee note pubkey")
	}
	if depositID.IsEmpty() {
		return fmt.Errorf("missing shielder fee note deposit id")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderFeeNotePubKey, pubKey.String()), depositID.String())
}

func (k KVStore) ShielderFeeNotePubKeyUsed(ctx cosmos.Context, pubKey common.PubKey) bool {
	if pubKey.IsEmpty() {
		return false
	}
	return k.has(ctx, k.GetKey(prefixShielderFeeNotePubKey, pubKey.String()))
}

func (k KVStore) SetNodeSlotAuction(ctx cosmos.Context, auction types.NodeSlotAuction) error {
	if err := auction.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixNodeSlotAuction, auction.Key()), auction)
}

func (k KVStore) GetNodeSlotAuction(ctx cosmos.Context, auctionID string) (types.NodeSlotAuction, error) {
	record := types.NodeSlotAuction{AuctionID: strings.TrimSpace(auctionID)}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixNodeSlotAuction, record.Key()), &record)
	return record, err
}

func (k KVStore) GetNodeSlotAuctionIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixNodeSlotAuction)
}

func (k KVStore) SetNodeSlotBid(ctx cosmos.Context, bid types.NodeSlotBid) error {
	if err := bid.Valid(); err != nil {
		return err
	}
	if err := k.setShielderJSON(ctx, k.GetKey(prefixNodeSlotBid, bid.Key()), bid); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixNodeSlotAuctionBid, bid.AuctionID, bid.BidID), bid.BidID)
}

func (k KVStore) GetNodeSlotBid(ctx cosmos.Context, bidID string) (types.NodeSlotBid, error) {
	record := types.NodeSlotBid{BidID: strings.TrimSpace(bidID)}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixNodeSlotBid, record.Key()), &record)
	return record, err
}

func (k KVStore) GetNodeSlotBidIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixNodeSlotBid)
}
