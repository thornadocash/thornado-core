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

func (k KVStore) SetDepositSession(ctx cosmos.Context, session types.DepositSession) error {
	if err := session.Valid(); err != nil {
		return err
	}
	if err := k.setShielderJSON(ctx, k.GetKey(prefixDepositSession, session.Key()), session); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderPowToken, session.PowToken), session.Key())
}

func (k KVStore) GetDepositSession(ctx cosmos.Context, owner cosmos.AccAddress) (types.DepositSession, error) {
	record := types.DepositSession{Owner: owner}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixDepositSession, owner.String()), &record)
	return record, err
}

func (k KVStore) GetDepositSessionByPowToken(ctx cosmos.Context, powToken string) (types.DepositSession, error) {
	var owner string
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderPowToken, strings.TrimSpace(powToken)), &owner)
	if err != nil {
		return types.DepositSession{}, err
	}
	if !found || owner == "" {
		return types.DepositSession{}, fmt.Errorf("deposit pow token not found")
	}
	addr, err := cosmos.AccAddressFromBech32(owner)
	if err != nil {
		return types.DepositSession{}, err
	}
	return k.GetDepositSession(ctx, addr)
}

func (k KVStore) SetDepositPowTiming(ctx cosmos.Context, record types.DepositPowTiming) error {
	if err := record.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixDepositPowTiming, record.Key()), record)
}

func (k KVStore) GetDepositPowTiming(ctx cosmos.Context, powToken string) (types.DepositPowTiming, error) {
	record := types.DepositPowTiming{PowToken: strings.TrimSpace(powToken)}
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixDepositPowTiming, record.Key()), &record)
	if !found {
		return types.DepositPowTiming{}, err
	}
	return record, err
}

func (k KVStore) GetDepositPowTimingIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixDepositPowTiming)
}

func (k KVStore) SetDepositPowDifficultyState(ctx cosmos.Context, state types.DepositPowDifficultyState) error {
	return k.setShielderJSON(ctx, k.GetKey(prefixDepositPowDifficulty, "current"), state)
}

func (k KVStore) GetDepositPowDifficultyState(ctx cosmos.Context) (types.DepositPowDifficultyState, error) {
	var state types.DepositPowDifficultyState
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixDepositPowDifficulty, "current"), &state)
	return state, err
}

func (k KVStore) SetDepositAddress(ctx cosmos.Context, record types.DepositAddress) error {
	if err := record.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixDepositAddress, record.Key()), record)
}

func (k KVStore) GetDepositAddress(ctx cosmos.Context, address common.Address) (types.DepositAddress, error) {
	record := types.DepositAddress{Address: address}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixDepositAddress, address.String()), &record)
	return record, err
}

func (k KVStore) DeleteDepositAddress(ctx cosmos.Context, address common.Address) error {
	if address.IsEmpty() {
		return nil
	}
	k.del(ctx, k.GetKey(prefixDepositAddress, address.String()))
	return nil
}

func (k KVStore) GetDepositAddressIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixDepositAddress)
}

func vaultDepositPathCursorKey(vaultPubKey common.PubKey, pathType common.VaultDepositPathType) string {
	return vaultPubKey.String()
}

func (k KVStore) GetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType) (uint64, error) {
	if vaultPubKey.IsEmpty() {
		return 0, fmt.Errorf("missing vault pubkey")
	}
	var index uint64
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixVaultDepositPathIndex, vaultDepositPathCursorKey(vaultPubKey, pathType)), &index)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	return index, nil
}

func (k KVStore) SetNextVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType, index uint64) error {
	if vaultPubKey.IsEmpty() {
		return fmt.Errorf("missing vault pubkey")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixVaultDepositPathIndex, vaultDepositPathCursorKey(vaultPubKey, pathType)), index)
}

func (k KVStore) AllocateVaultDepositPathIndex(ctx cosmos.Context, vaultPubKey common.PubKey, pathType common.VaultDepositPathType) (uint64, uint64, error) {
	depositIndex, err := k.GetNextVaultDepositPathIndex(ctx, vaultPubKey, pathType)
	if err != nil {
		return 0, 0, err
	}
	pathIndex, err := common.VaultDepositPathIndex(pathType, depositIndex, common.DepositPathCommitmentRoot)
	if err != nil {
		return 0, 0, err
	}
	if err := k.SetNextVaultDepositPathIndex(ctx, vaultPubKey, pathType, depositIndex+1); err != nil {
		return 0, 0, err
	}
	return depositIndex, pathIndex, nil
}

func (k KVStore) SetDepositRecord(ctx cosmos.Context, deposit types.DepositRecord) error {
	if err := deposit.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixDepositRecord, deposit.Key()), deposit)
}

func (k KVStore) GetDepositRecord(ctx cosmos.Context, depositID common.TxID) (types.DepositRecord, error) {
	record := types.DepositRecord{DepositID: depositID}
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixDepositRecord, depositID.String()), &record)
	if !found {
		return types.DepositRecord{}, err
	}
	return record, err
}

func (k KVStore) GetDepositRecordIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixDepositRecord)
}

func (k KVStore) SetShielderCommitment(ctx cosmos.Context, commitment string) error {
	commitment = strings.TrimSpace(commitment)
	if commitment == "" {
		return fmt.Errorf("missing shielder commitment")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderCommitment, commitment), true)
}

func (k KVStore) ShielderCommitmentExists(ctx cosmos.Context, commitment string) bool {
	return k.has(ctx, k.GetKey(prefixShielderCommitment, strings.TrimSpace(commitment)))
}

func (k KVStore) SetShielderNoteRecord(ctx cosmos.Context, record types.StoredShielderNoteRecord) error {
	if err := record.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderNotePubKey, record.Key()), record)
}

func (k KVStore) GetShielderNoteRecordIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderNotePubKey)
}

func (k KVStore) SetShielderDenominationCommitment(ctx cosmos.Context, denominationSats uint64, commitment string) error {
	commitment = strings.TrimSpace(commitment)
	if denominationSats == 0 {
		return fmt.Errorf("missing shielder commitment denomination")
	}
	if commitment == "" {
		return fmt.Errorf("missing shielder commitment")
	}
	return k.setShielderJSON(ctx, shielderDenominationCommitmentKey(denominationSats, commitment), true)
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
	return []byte(shielderDenominationPrefix(denominationSats) + strings.TrimSpace(commitment))
}

func shielderMerkleRootKey(denominationSats uint64, root string) []byte {
	return []byte(fmt.Sprintf("%s%020d/%s", prefixShielderMerkleRoot, denominationSats, strings.TrimSpace(root)))
}

func (k KVStore) SetShielderRedeem(ctx cosmos.Context, withdrawal types.ShielderRedeem) error {
	if err := withdrawal.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderRedeem, withdrawal.Key()), withdrawal)
}

func (k KVStore) GetShielderRedeem(ctx cosmos.Context, withdrawalID string) (types.ShielderRedeem, error) {
	record := types.ShielderRedeem{WithdrawalID: withdrawalID}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderRedeem, withdrawalID), &record)
	return record, err
}

func (k KVStore) GetShielderRedeemByNullifier(ctx cosmos.Context, nullifierHash string) (types.ShielderRedeem, error) {
	var withdrawalID string
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderNullifier, strings.TrimSpace(nullifierHash)), &withdrawalID)
	if err != nil {
		return types.ShielderRedeem{}, err
	}
	if !found || withdrawalID == "" {
		return types.ShielderRedeem{}, nil
	}
	return k.GetShielderRedeem(ctx, withdrawalID)
}

func (k KVStore) SetShielderNullifierSpent(ctx cosmos.Context, nullifierHash string, withdrawalID string) error {
	nullifierHash = strings.TrimSpace(nullifierHash)
	if nullifierHash == "" {
		return fmt.Errorf("missing shielder nullifier")
	}
	if strings.TrimSpace(withdrawalID) == "" {
		return fmt.Errorf("missing shielder redeem id")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderNullifier, nullifierHash), withdrawalID)
}

func (k KVStore) ShielderNullifierSpent(ctx cosmos.Context, nullifierHash string) bool {
	return k.has(ctx, k.GetKey(prefixShielderNullifier, strings.TrimSpace(nullifierHash)))
}

func (k KVStore) GetShielderNullifierIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderNullifier)
}

func (k KVStore) GetNextShielderNodeBondSlot(ctx cosmos.Context) (uint64, error) {
	var slot uint64
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderNodeBondSlot, "next"), &slot)
	if err != nil {
		return 0, err
	}
	if !found {
		return 1, nil
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

func (k KVStore) SetShielderNodeBonder(ctx cosmos.Context, bonder types.ShielderNodeBonder) error {
	if err := bonder.Valid(); err != nil {
		return err
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderNodeBonder, bonder.Key()), bonder)
}

func (k KVStore) GetShielderNodeBonder(ctx cosmos.Context, nodePubKey string, bonder cosmos.AccAddress) (types.ShielderNodeBonder, error) {
	record := types.ShielderNodeBonder{
		NodePubKey: strings.TrimSpace(nodePubKey),
		Bonder:     bonder,
	}
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixShielderNodeBonder, record.Key()), &record)
	return record, err
}

func (k KVStore) GetShielderNodeBonderIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixShielderNodeBonder)
}

func (k KVStore) DeleteShielderNodeBonder(ctx cosmos.Context, nodePubKey string, bonder cosmos.AccAddress) error {
	record := types.ShielderNodeBonder{
		NodePubKey: strings.TrimSpace(nodePubKey),
		Bonder:     bonder,
	}
	k.del(ctx, k.GetKey(prefixShielderNodeBonder, record.Key()))
	return nil
}

func (k KVStore) SetFeePool(ctx cosmos.Context, pool types.FeePool) error {
	return k.setShielderJSON(ctx, k.GetKey(prefixFeePool, "global"), pool)
}

func (k KVStore) GetFeePool(ctx cosmos.Context) (types.FeePool, error) {
	var pool types.FeePool
	_, err := k.getShielderJSON(ctx, k.GetKey(prefixFeePool, "global"), &pool)
	return pool, err
}

func (k KVStore) SetShielderFeeNotePubKey(ctx cosmos.Context, pubKey string) error {
	pubKey = strings.TrimSpace(pubKey)
	if pubKey == "" {
		return fmt.Errorf("missing shielder fee note pubkey")
	}
	return k.setShielderJSON(ctx, k.GetKey(prefixShielderFeeNotePubKey, pubKey), true)
}

func (k KVStore) ShielderFeeNotePubKeyUsed(ctx cosmos.Context, pubKey string) bool {
	pubKey = strings.TrimSpace(pubKey)
	if pubKey == "" {
		return false
	}
	return k.has(ctx, k.GetKey(prefixShielderFeeNotePubKey, pubKey))
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
	found, err := k.getShielderJSON(ctx, k.GetKey(prefixNodeSlotBid, record.Key()), &record)
	if err == nil && !found {
		record = types.NodeSlotBid{}
	}
	return record, err
}

func (k KVStore) GetNodeSlotBidIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixNodeSlotBid)
}
