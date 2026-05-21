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

type shielderNoteCommitment struct {
	DenominationSats uint64 `json:"denomination_sats"`
	Commitment       string `json:"commitment"`
}

func RegisterShielderPowToken(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken string) (types.ShielderSession, error) {
	if owner.Empty() {
		return types.ShielderSession{}, fmt.Errorf("missing shielder owner")
	}
	powToken = strings.TrimSpace(powToken)
	if powToken == "" {
		return types.ShielderSession{}, fmt.Errorf("missing shielder pow token")
	}

	vault, _, err := currentBTCVaultAddress(ctx, k)
	if err != nil {
		return types.ShielderSession{}, err
	}
	pathIndex, err := k.AllocateVaultDepositPathIndex(ctx, vault.PubKey)
	if err != nil {
		return types.ShielderSession{}, err
	}
	address, err := vault.DeriveBTCAddress(pathIndex)
	if err != nil {
		return types.ShielderSession{}, err
	}

	session := types.ShielderSession{
		Owner:            owner,
		PowToken:         powToken,
		DepositAddress:   address,
		VaultPubKey:      vault.PubKey,
		DepositPathIndex: pathIndex,
		CreatedHeight:    ctx.BlockHeight(),
		Status:           types.ShielderStatusAddressIssued,
	}
	mapping := types.ShielderDepositAddress{
		Address:       address,
		VaultPubKey:   vault.PubKey,
		PathIndex:     pathIndex,
		Owner:         owner,
		PowToken:      powToken,
		CreatedHeight: ctx.BlockHeight(),
	}
	if err := k.SetShielderDepositAddress(ctx, mapping); err != nil {
		return types.ShielderSession{}, err
	}
	return session, k.SetShielderSession(ctx, session)
}

func MatchShielderDeposit(ctx cosmos.Context, mgr Manager, tx ObservedTx) (types.ShielderDeposit, error) {
	k := mgr.Keeper()
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

	mapping, err := k.GetShielderDepositAddress(ctx, tx.Tx.ToAddress)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if mapping.VaultPubKey.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit address not registered")
	}
	session, err := k.GetShielderSession(ctx, mapping.Owner)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	if session.DepositAddress.IsEmpty() {
		return types.ShielderDeposit{}, fmt.Errorf("shielder session not found")
	}
	if !tx.Tx.ToAddress.Equals(session.DepositAddress) || !mapping.VaultPubKey.Equals(session.VaultPubKey) || mapping.PathIndex != session.DepositPathIndex {
		return types.ShielderDeposit{}, fmt.Errorf("shielder deposit mapping mismatch")
	}

	deposit := types.ShielderDeposit{
		DepositID:        tx.Tx.ID,
		Owner:            session.Owner,
		AmountSats:       coin.Amount.Uint64(),
		DepositAddress:   tx.Tx.ToAddress,
		VaultPubKey:      session.VaultPubKey,
		DepositPathIndex: session.DepositPathIndex,
		MatchedHeight:    ctx.BlockHeight(),
		Status:           types.ShielderStatusDepositMatched,
	}
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}

	session.DepositID = tx.Tx.ID
	session.Status = types.ShielderStatusDepositMatched
	if err := k.SetShielderSession(ctx, session); err != nil {
		return types.ShielderDeposit{}, err
	}
	if err := queueVaultPathSweep(ctx, mgr, tx, session.VaultPubKey, session.DepositPathIndex); err != nil {
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

	noteCommitments, err := parseShielderNoteCommitments(commitments, deposit.AmountSats)
	if err != nil {
		return types.ShielderDeposit{}, err
	}
	seen := make(map[string]struct{}, len(noteCommitments))
	var total uint64
	for _, note := range noteCommitments {
		if note.Commitment == "" {
			return types.ShielderDeposit{}, fmt.Errorf("empty shielder commitment")
		}
		if _, ok := seen[note.Commitment]; ok {
			return types.ShielderDeposit{}, fmt.Errorf("duplicate shielder commitment")
		}
		if k.ShielderCommitmentExists(ctx, note.Commitment) {
			return types.ShielderDeposit{}, fmt.Errorf("shielder commitment already exists")
		}
		total += note.DenominationSats
		seen[note.Commitment] = struct{}{}
	}
	if total != deposit.AmountSats {
		return types.ShielderDeposit{}, fmt.Errorf("shielder commitment denominations do not match deposit amount")
	}

	deposit.Commitments = make([]string, 0, len(noteCommitments))
	for _, note := range noteCommitments {
		deposit.Commitments = append(deposit.Commitments, note.Commitment)
	}
	deposit.Status = types.ShielderStatusCommitted
	if err := k.SetShielderDeposit(ctx, deposit); err != nil {
		return types.ShielderDeposit{}, err
	}
	byDenomination := make(map[uint64][]string)
	for _, note := range noteCommitments {
		if err := k.SetShielderCommitment(ctx, note.Commitment, depositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderDenominationCommitment(ctx, note.DenominationSats, note.Commitment, depositID); err != nil {
			return types.ShielderDeposit{}, err
		}
		byDenomination[note.DenominationSats] = append(byDenomination[note.DenominationSats], note.Commitment)
	}
	for denomination := range byDenomination {
		leaves, err := k.GetShielderDenominationCommitments(ctx, denomination)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		root, err := ComputeShielderMerkleRoot(leaves)
		if err != nil {
			return types.ShielderDeposit{}, err
		}
		if err := k.SetShielderMerkleRoot(ctx, denomination, root); err != nil {
			return types.ShielderDeposit{}, err
		}
	}
	return deposit, nil
}

func parseShielderNoteCommitments(raw []string, depositAmountSats uint64) ([]shielderNoteCommitment, error) {
	result := make([]shielderNoteCommitment, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("empty shielder commitment")
		}
		if strings.HasPrefix(item, "{") {
			var note shielderNoteCommitment
			if err := json.Unmarshal([]byte(item), &note); err != nil {
				return nil, fmt.Errorf("invalid shielder commitment: %w", err)
			}
			note.Commitment = strings.TrimSpace(note.Commitment)
			if note.DenominationSats == 0 {
				return nil, fmt.Errorf("missing shielder commitment denomination")
			}
			if note.Commitment == "" {
				return nil, fmt.Errorf("missing shielder commitment")
			}
			result = append(result, note)
			continue
		}
		if len(raw) != 1 {
			return nil, fmt.Errorf("split shielder commitments require denomination_sats")
		}
		result = append(result, shielderNoteCommitment{DenominationSats: depositAmountSats, Commitment: item})
	}
	return result, nil
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
	if !k.ShielderMerkleRootExists(ctx, publicInputs.DenominationSats, publicInputs.MerkleRoot) {
		return types.ShielderWithdrawal{}, fmt.Errorf("unknown shielder merkle root")
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
		TxType:      types.TxOutTypeOut,
	}
	return k.AppendTxOut(ctx, ctx.BlockHeight(), item)
}

func queueVaultPathSweep(ctx cosmos.Context, mgr Manager, tx ObservedTx, sourcePubKey common.PubKey, pathIndex uint64) error {
	if sourcePubKey.IsEmpty() {
		return fmt.Errorf("missing sweep vault pubkey")
	}
	coin := tx.Tx.Coins.GetCoin(common.BTCAsset)
	if coin.IsEmpty() || coin.Amount.IsZero() {
		return fmt.Errorf("missing sweep bitcoin amount")
	}
	currentVault, currentRoot, err := currentBTCVaultAddress(ctx, mgr.Keeper())
	if err != nil {
		return err
	}
	sourceAddr, err := common.DeriveBTCTaprootAddress(sourcePubKey, pathIndex)
	if err != nil {
		return err
	}
	if sourcePubKey.Equals(currentVault.PubKey) && pathIndex == common.MainVaultPathIndex && tx.Tx.ToAddress.Equals(currentRoot) {
		return nil
	}

	maxGasCoin, err := mgr.GasMgr().GetMaxGas(ctx, common.BTCChain)
	if err != nil {
		return fmt.Errorf("fail to get bitcoin sweep max gas: %w", err)
	}
	amount := coin.Amount
	if gas := maxGasCoin.Amount; !gas.IsZero() {
		if amount.LTE(gas) {
			return fmt.Errorf("sweep amount is not enough to pay gas")
		}
		amount = amount.Sub(gas)
	}
	gasRate := int64(1)
	if nf, err := mgr.Keeper().GetNetworkFee(ctx, common.BTCChain); err == nil && nf.TransactionFeeRate > 0 {
		gasRate = int64(nf.TransactionFeeRate)
	}

	txType := types.TxOutTypeSweep
	if pathIndex == common.MainVaultPathIndex {
		txType = types.TxOutTypeMigrate
	}
	item := TxOutItem{
		Chain:          common.BTCChain,
		ToAddress:      currentRoot,
		VaultPubKey:    sourcePubKey,
		Coin:           common.NewCoin(common.BTCAsset, amount),
		MaxGas:         common.Gas{maxGasCoin},
		GasRate:        gasRate,
		InHash:         tx.Tx.ID,
		ModuleName:     AsgardName,
		VaultPathIndex: pathIndex,
		TxType:         txType,
	}
	ctx.Logger().Info("queued memoless bitcoin vault path sweep",
		"from", sourceAddr.String(),
		"to", currentRoot.String(),
		"vault_pub_key", sourcePubKey.String(),
		"path_index", pathIndex,
		"amount", amount.String(),
	)
	return mgr.Keeper().AppendTxOut(ctx, ctx.BlockHeight(), item)
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
		address, err := vault.DeriveBTCAddress(common.MainVaultPathIndex)
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
