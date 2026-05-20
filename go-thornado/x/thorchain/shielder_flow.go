package thorchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

type ShielderWithdrawalRequest struct {
	Owner  cosmos.AccAddress
	Proof  json.RawMessage
	Public json.RawMessage
}

type shielderWithdrawalPublicInputs struct {
	NullifierHash    string `json:"nullifier_hash"`
	MerkleRoot       string `json:"merkle_root"`
	DenominationSats uint64 `json:"denomination_sats"`
	Recipient        string `json:"recipient"`
	FeeSats          uint64 `json:"fee_sats"`
}

func RegisterShielderPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken string) (types.ShielderSession, error) {
	if owner.Empty() {
		return types.ShielderSession{}, fmt.Errorf("missing shielder owner")
	}
	powToken = strings.TrimSpace(powToken)
	if powToken == "" {
		return types.ShielderSession{}, fmt.Errorf("missing shielder pow token")
	}

	vault, address, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.ShielderSession{}, err
	}

	session := types.ShielderSession{
		Owner:          owner,
		PowToken:       powToken,
		DepositAddress: address,
		VaultPubKey:    vault.PubKey,
		CreatedHeight:  ctx.BlockHeight(),
		Status:         types.ShielderStatusAddressIssued,
	}
	return session, k.SetShielderSession(ctx, session)
}

func MatchShielderDeposit(ctx cosmos.Context, k keeper.Keeper, tx ObservedTx) (types.ShielderDeposit, error) {
	if tx.Tx.ID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder deposit tx id")
	}
	if !tx.Tx.Chain.Equals(common.BTCChain) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit must be bitcoin")
	}
	coin := tx.Tx.Coins.GetCoin(common.BTCAsset)
	if coin.IsEmpty() || coin.Amount.IsZero() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder bitcoin deposit amount")
	}

	session, err := k.GetShielderSessionByPowToken(ctx, tx.Tx.Memo)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if !tx.Tx.ToAddress.Equals(session.DepositAddress) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit address mismatch")
	}

	deposit := types.ShielderDeposit{
		DepositID:      tx.Tx.ID,
		Owner:          session.Owner,
		AmountSats:     coin.Amount.Uint64(),
		DepositAddress: tx.Tx.ToAddress,
		VaultPubKey:    session.VaultPubKey,
		MatchedHeight:  ctx.BlockHeight(),
		Status:         types.ShielderStatusDepositMatched,
	}
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}

	session.DepositID = tx.Tx.ID
	session.Status = types.ShielderStatusDepositMatched
	if err := k.SetShielderSession(ctx, session); err != nil {
		return types.ShielderDeposit{}, err
	}

	return deposit, nil
}

func PostShielderCommitments(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, depositID common.TxID, commitments []string) (types.ShielderDeposit, error) {
	if owner.Empty() {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder owner")
	}
	if len(commitments) == 0 {
		return types.ShielderDeposit{}, fmt.Errorf("missing shielder commitments")
	}

	deposit, err := k.GetShielderDeposit(ctx, depositID)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if deposit.DepositID.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit not found")
	}
	if !deposit.Owner.Equals(owner) {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit owner mismatch")
	}
	if len(deposit.Commitments) > 0 {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit already split")
	}

	seen := make(map[string]struct{}, len(commitments))
	for _, commitment := range commitments {
		commitment = strings.TrimSpace(commitment)
		if commitment == "" {
			return types.ShielderDeposit{}, fmt.Errorf("empty shielder commitment")
		}
		if _, ok := seen[commitment]; ok {
			return types.ShielderDeposit{}, fmt.Errorf("duplicate shielder commitment")
		}
		if k.ShielderCommitmentExists(ctx, commitment) {
			return types.ShielderDeposit{}, fmt.Errorf("shielder commitment already exists")
		}
		seen[commitment] = struct{}{}
	}

	deposit.Commitments = append([]string(nil), commitments...)
	deposit.Status = types.ShielderStatusCommitted
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	for _, commitment := range commitments {
		if err := k.SetShielderCommitment(ctx, commitment, depositID); err != nil {
			return types.ShielderDeposit{}, err
		}
	}
	return deposit, nil
}

func RequestShielderWithdrawal(ctx cosmos.Context, k keeper.Keeper, req ShielderWithdrawalRequest) (types.ShielderWithdrawal, error) {
	if req.Owner.Empty() {
		return types.ShielderWithdrawal{}, fmt.Errorf("missing shielder withdrawal owner")
	}
	if err := VerifyShielderWithdrawalJSON(req.Proof, req.Public); err != nil {
		return types.ShielderWithdrawal{}, err
	}

	publicInputs, err := parseShielderWithdrawalPublicInputs(req.Public)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if k.ShielderNullifierSpent(ctx, publicInputs.NullifierHash) {
		return types.ShielderWithdrawal{}, fmt.Errorf("shielder nullifier already spent")
	}

	recipient, err := common.NewAddress(publicInputs.Recipient)
	if err != nil {
		return types.ShielderWithdrawal{}, fmt.Errorf("invalid shielder withdrawal recipient: %w", err)
	}
	if !recipient.GetChain().Equals(common.BTCChain) {
		return types.ShielderWithdrawal{}, fmt.Errorf("shielder withdrawal recipient must be bitcoin")
	}

	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}

	withdrawalID := shielderWithdrawalID(publicInputs.NullifierHash, recipient.String())
	inHash, err := common.NewTxID(withdrawalID)
	if err != nil {
		return types.ShielderWithdrawal{}, err
	}
	withdrawal := types.ShielderWithdrawal{
		WithdrawalID:    withdrawalID,
		Owner:           req.Owner,
		NullifierHash:   publicInputs.NullifierHash,
		MerkleRoot:      publicInputs.MerkleRoot,
		Recipient:       recipient,
		AmountSats:      publicInputs.DenominationSats,
		FeeSats:         publicInputs.FeeSats,
		InHash:          inHash,
		VaultPubKey:     vault.PubKey,
		RequestedHeight: ctx.BlockHeight(),
		Status:          types.ShielderStatusKeysignQueued,
		Proof:           append(json.RawMessage(nil), req.Proof...),
		Public:          append(json.RawMessage(nil), req.Public...),
	}
	if err := withdrawal.Valid(); err != nil {
		return types.ShielderWithdrawal{}, err
	}

	if err := k.SetShielderWithdrawal(ctx, withdrawal); err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if err := k.SetShielderNullifierSpent(ctx, withdrawal.NullifierHash, withdrawal.WithdrawalID); err != nil {
		return types.ShielderWithdrawal{}, err
	}
	if err := queueShielderWithdrawalKeysign(ctx, k, withdrawal); err != nil {
		return types.ShielderWithdrawal{}, err
	}

	return withdrawal, nil
}

func parseShielderWithdrawalPublicInputs(raw json.RawMessage) (shielderWithdrawalPublicInputs, error) {
	var publicInputs shielderWithdrawalPublicInputs
	if err := json.Unmarshal(raw, &publicInputs); err != nil {
		return publicInputs, fmt.Errorf("invalid shielder public inputs: %w", err)
	}
	if strings.TrimSpace(publicInputs.NullifierHash) == "" {
		return publicInputs, fmt.Errorf("missing shielder nullifier hash")
	}
	if strings.TrimSpace(publicInputs.MerkleRoot) == "" {
		return publicInputs, fmt.Errorf("missing shielder merkle root")
	}
	if publicInputs.DenominationSats == 0 {
		return publicInputs, fmt.Errorf("missing shielder denomination")
	}
	if publicInputs.FeeSats >= publicInputs.DenominationSats {
		return publicInputs, fmt.Errorf("shielder fee exceeds denomination")
	}
	return publicInputs, nil
}

func queueShielderWithdrawalKeysign(ctx cosmos.Context, k keeper.Keeper, withdrawal types.ShielderWithdrawal) error {
	amount := withdrawal.AmountSats - withdrawal.FeeSats
	item := TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   withdrawal.Recipient,
		VaultPubKey: withdrawal.VaultPubKey,
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(amount)),
		Memo:        "SHIELDER:" + withdrawal.WithdrawalID,
		MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(withdrawal.FeeSats))},
		GasRate:     1,
		InHash:      withdrawal.InHash,
		ModuleName:  AsgardName,
	}
	return k.AppendTxOut(ctx, ctx.BlockHeight(), item)
}

func currentBTCVaultAddress(ctx cosmos.Context, k keeper.Keeper) (Vault, common.Address, error) {
	vaults, err := k.GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return Vault{}, common.NoAddress, err
	}
	if len(vaults) == 0 {
		return Vault{}, common.NoAddress, fmt.Errorf("no active shielder bitcoin vault")
	}
	for _, vault := range vaults {
		address, err := vault.GetAddress(common.BTCChain)
		if err == nil && !address.IsEmpty() {
			return vault, address, nil
		}
	}
	return Vault{}, common.NoAddress, fmt.Errorf("no active shielder bitcoin vault address")
}

func shielderWithdrawalID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
