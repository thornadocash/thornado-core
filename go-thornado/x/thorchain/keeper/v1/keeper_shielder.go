package keeperv1

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/runtime"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
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
