package thornado

import (
	math "math"
	"strings"

	"github.com/blang/semver"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256r1"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const ActiveNodePriority = int64(math.MaxInt64)

type AnteDecorator struct {
	keeper keeper.Keeper
}

func NewAnteDecorator(keeper keeper.Keeper) AnteDecorator {
	return AnteDecorator{
		keeper: keeper,
	}
}

func (ad AnteDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	if err = ad.rejectMultipleDepositMsgs(tx.GetMsgs()); err != nil {
		return ctx, err
	}

	// TODO remove on hard fork, when all signers will be allowed (v47+)
	if err = ad.rejectInvalidSigners(tx); err != nil {
		return ctx, err
	}

	// run the message-specific ante for each msg, all must succeed
	version, _ := ad.keeper.GetVersionWithCtx(ctx)
	for _, msg := range tx.GetMsgs() {
		newCtx, err = ad.anteHandleMessage(ctx, version, msg)
		if err != nil {
			return ctx, err
		}
	}
	if ad.isIntrinsicAuthNativeTx(tx.GetMsgs()) {
		return newCtx, nil
	}

	return next(newCtx, tx, simulate)
}

func (ad AnteDecorator) isIntrinsicAuthNativeTx(msgs []cosmos.Msg) bool {
	if len(msgs) == 0 {
		return false
	}
	for _, msg := range msgs {
		switch msg.(type) {
		case *types.MsgDepositRequestPow,
			*types.MsgShielderShield,
			*types.MsgShielderRedeem:
		default:
			return false
		}
	}
	return true
}

// rejectInvalidSigners reject txs if they are signed with secp256r1 keys
func (ad AnteDecorator) rejectInvalidSigners(tx sdk.Tx) error {
	sigTx, okTx := tx.(authsigning.SigVerifiableTx)
	if !okTx {
		return cosmos.ErrUnknownRequest("invalid transaction type")
	}
	sigs, err := sigTx.GetSignaturesV2()
	if err != nil {
		return err
	}
	for _, sig := range sigs {
		pubkey := sig.PubKey
		switch pubkey := pubkey.(type) {
		case *secp256r1.PubKey:
			return cosmos.ErrUnknownRequest("secp256r1 keys not allowed")
		case multisig.PubKey:
			for _, pk := range pubkey.GetPubKeys() {
				if _, okPk := pk.(*secp256r1.PubKey); okPk {
					return cosmos.ErrUnknownRequest("secp256r1 keys not allowed")
				}
			}
		}
	}
	return nil
}

// rejectMultipleDepositMsgs only one deposit msg allowed per tx
func (ad AnteDecorator) rejectMultipleDepositMsgs(msgs []cosmos.Msg) error {
	hasDeposit := false
	for _, msg := range msgs {
		if ad.isDeposit(msg) {
			if hasDeposit {
				return cosmos.ErrUnknownRequest("only one deposit msg per tx")
			}
			hasDeposit = true
		}
	}
	return nil
}

// isDeposit returns true if the msg is a deposit
func (ad AnteDecorator) isDeposit(msg cosmos.Msg) bool {
	switch m := msg.(type) {
	case *types.MsgDeposit:
		return true
	case *types.MsgSend:
		return m.ToAddress.Equals(ad.keeper.GetModuleAccAddress(ModuleName))
	case *banktypes.MsgSend:
		return m.ToAddress == ad.keeper.GetModuleAccAddress(ModuleName).String()
	default:
		return false
	}
}

// anteHandleMessage calls the msg-specific ante handling for a given msg
func (ad AnteDecorator) anteHandleMessage(ctx sdk.Context, version semver.Version, msg cosmos.Msg) (sdk.Context, error) {
	// ideally each handler would impl an ante func and we could instantiate
	// handlers and call ante, but handlers require mgr which is unavailable
	switch m := msg.(type) {

	// consensus handlers
	case *types.MsgErrataTx:
		return ErrataTxAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgErrataTxQuorum:
		return ErrataTxQuorumAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgNetworkFee:
		return NetworkFeeAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgNetworkFeeQuorum:
		return NetworkFeeQuorumAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgObservedTxIn:
		return ObservedTxInAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgObservedTxOut:
		return ObservedTxOutAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgObservedTxQuorum:
		return ObservedTxQuorumAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSolvency:
		return SolvencyAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSolvencyQuorum:
		return SolvencyQuorumAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgKeygenVault:
		return FrostAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgFrostKeysignFail:
		return FrostKeysignFailAnteHandler(ctx, version, ad.keeper, *m)

	// cli handlers (non-consensus)
	case *types.MsgSetIPAddress:
		return IPAddressAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgConfig:
		return ConfigAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgNodePauseChain:
		return NodePauseChainAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSetNodeKeys:
		return SetNodeKeysAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSetVersion:
		return VersionAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgProposeUpgrade, *types.MsgApproveUpgrade, *types.MsgRejectUpgrade:
		legacyMsg, ok := msg.(sdk.LegacyMsg)
		if !ok {
			return ctx, cosmos.ErrUnknownRequest("invalid message type")
		}
		return ActiveNodeAnteHandler(ctx, version, ad.keeper, legacyMsg.GetSigners()[0])

	// native handlers (non-consensus)
	case *types.MsgSend, *banktypes.MsgSend:
		return ctx, cosmos.ErrUnknownRequest("native sends are disabled")
	case *types.MsgDeposit:
		return ctx, cosmos.ErrUnknownRequest("native deposits are disabled")
	case *types.MsgDepositRequestPow:
		return DepositRequestPowAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgShielderShield:
		return ShielderShieldAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgShielderRedeem:
		return ShielderRedeemAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgShielderShieldFees:
		return ShielderShieldFeesAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgNodeSlotAuctionCreate:
		return NodeSlotAuctionCreateAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgNodeSlotAuctionBidCreate:
		return NodeSlotAuctionBidCreateAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgNodeSlotAuctionSelectBid:
		return NodeSlotAuctionSelectBidAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgNodeSaleShield:
		return NodeSaleShieldAnteHandler(ctx, ad.keeper, *m)
	case *types.MsgBondFromNotes:
		return BondFromNotesAnteHandler(ctx, ad.keeper, *m)
	default:
		return ctx, cosmos.ErrUnknownRequest("invalid message type")
	}
}

func ObservedTxQuorumAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg types.MsgObservedTxQuorum) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	newCtx, err := activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
	if err != nil {
		return ctx, err
	}
	tx := msg.QuoTx.ObsTx
	var voter types.ObservedTxVoter
	if msg.QuoTx.Inbound {
		voter, err = ensureVaultAndGetTxInVoter(ctx, tx.ObservedPubKey, tx.Tx.ID, k)
	} else {
		voter, err = ensureVaultAndGetTxOutVoter(ctx, k, tx.ObservedPubKey, tx.Tx.ID, msg.GetSigners(), tx.KeysignMs)
	}
	if err != nil {
		return ctx, err
	}
	if err := reserveObservedTxAttestations(ctx, k, voter, tx, msg.GetSigners(), msg.QuoTx.Inbound); err != nil {
		return ctx, err
	}
	return newCtx, nil
}

func NetworkFeeQuorumAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg types.MsgNetworkFeeQuorum) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	newCtx, err := activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
	if err != nil {
		return ctx, err
	}
	nf := msg.QuoNetFee.NetworkFee
	if nf.TransactionRate > uint64(math.MaxInt64) || nf.TransactionSize > uint64(math.MaxInt64) {
		return ctx, cosmos.ErrUnknownRequest("transaction rate or size exceeds int64 max")
	}
	voter, err := k.GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	if err != nil {
		return ctx, err
	}
	if err := reserveNetworkFeeAttestations(ctx, k, voter, msg.GetSigners()); err != nil {
		return ctx, err
	}
	return newCtx, nil
}

func SolvencyQuorumAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg types.MsgSolvencyQuorum) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	newCtx, err := activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
	if err != nil {
		return ctx, err
	}
	s := msg.QuoSolvency.Solvency
	voter, err := k.GetSolvencyVoter(ctx, s.Id, s.Chain)
	if err != nil {
		return ctx, err
	}
	if voter.Empty() {
		voter = types.NewSolvencyVoter(s.Id, s.Chain, s.PubKey, s.Coins, s.Height)
	}
	if err := reserveSolvencyAttestations(ctx, k, voter, msg.GetSigners()); err != nil {
		return ctx, err
	}
	return newCtx, nil
}

func ErrataTxQuorumAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg types.MsgErrataTxQuorum) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	newCtx, err := activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
	if err != nil {
		return ctx, err
	}
	er := msg.QuoErrata.ErrataTx
	voter, err := k.GetErrataTxVoter(ctx, er.Id, er.Chain)
	if err != nil {
		return ctx, err
	}
	if err := reserveErrataTxAttestations(ctx, k, voter, msg.GetSigners()); err != nil {
		return ctx, err
	}
	return newCtx, nil
}

func reserveObservedTxAttestations(ctx cosmos.Context, k keeper.Keeper, voter types.ObservedTxVoter, tx common.ObservedTx, signers []cosmos.AccAddress, inbound bool) error {
	for _, signer := range signers {
		if !voter.Add(tx, signer) {
			return cosmos.ErrUnknownRequest("observed tx attestation already submitted")
		}
	}
	if !ctx.IsCheckTx() {
		return nil
	}
	if inbound {
		k.SetObservedTxInVoter(ctx, voter)
	} else {
		k.SetObservedTxOutVoter(ctx, voter)
	}
	return nil
}

func reserveNetworkFeeAttestations(ctx cosmos.Context, k keeper.Keeper, voter types.ObservedNetworkFeeVoter, signers []cosmos.AccAddress) error {
	for _, signer := range signers {
		if !voter.Sign(signer) {
			return cosmos.ErrUnknownRequest("network fee attestation already submitted")
		}
	}
	if ctx.IsCheckTx() {
		k.SetObservedNetworkFeeVoter(ctx, voter)
	}
	return nil
}

func reserveSolvencyAttestations(ctx cosmos.Context, k keeper.Keeper, voter types.SolvencyVoter, signers []cosmos.AccAddress) error {
	for _, signer := range signers {
		if !voter.Sign(signer) {
			return cosmos.ErrUnknownRequest("solvency attestation already submitted")
		}
	}
	if ctx.IsCheckTx() {
		k.SetSolvencyVoter(ctx, voter)
	}
	return nil
}

func reserveErrataTxAttestations(ctx cosmos.Context, k keeper.Keeper, voter types.ErrataTxVoter, signers []cosmos.AccAddress) error {
	for _, signer := range signers {
		if !voter.Sign(signer) {
			return cosmos.ErrUnknownRequest("errata tx attestation already submitted")
		}
	}
	if ctx.IsCheckTx() {
		k.SetErrataTxVoter(ctx, voter)
	}
	return nil
}

func DepositRequestPowAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgDepositRequestPow) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	owner, err := AccAddressFromCompressedSecp256k1(msg.DepositPubkey)
	if err != nil {
		return ctx, err
	}
	return depositPowAnte(ctx, k, owner, msg.PowToken, msg.PowDurationMs)
}

func depositPowAnte(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, powToken string, powDurationMs uint64) (cosmos.Context, error) {
	if err := validateDepositPowToken(ctx, k, owner, powToken); err != nil {
		return ctx, err
	}
	if existing, err := k.GetDepositSessionByPowToken(ctx, strings.TrimSpace(powToken)); err == nil && !existing.DepositAddress.IsEmpty() {
		expiry := getConfigDurationBlocks(ctx, k, constants.Deposit_PowExpiryMinutes)
		if expiry <= 0 || existing.CreatedHeight+expiry >= ctx.BlockHeight() {
			return ctx, cosmos.ErrUnknownRequest("deposit pow token already used")
		}
	}
	if ctx.IsCheckTx() {
		if _, err := RegisterDepositPowToken(ctx, k, owner, powToken, powDurationMs); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func ShielderShieldAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgShielderShield) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	owner, err := AccAddressFromCompressedSecp256k1(msg.DepositPubkey)
	if err != nil {
		return ctx, err
	}
	session, err := k.GetDepositSession(ctx, owner)
	if err != nil {
		return ctx, err
	}
	if session.DepositID.IsEmpty() {
		return ctx, cosmos.ErrUnknownRequest("deposit not matched")
	}
	depositID := session.DepositID
	return shielderShieldAnte(ctx, k, owner, depositID, msg.Commitments, msg.DepositPubkey, msg.Signature)
}

func shielderShieldAnte(ctx cosmos.Context, k keeper.Keeper, owner cosmos.AccAddress, depositID common.TxID, commitments []string, depositPubkey, signature string) (cosmos.Context, error) {
	deposit, err := k.GetDepositRecord(ctx, depositID)
	if err != nil {
		return ctx, err
	}
	if deposit.DepositID.IsEmpty() {
		return ctx, cosmos.ErrUnknownRequest("deposit not found")
	}
	if !deposit.Owner.Equals(owner) {
		return ctx, cosmos.ErrUnknownRequest("deposit owner mismatch")
	}
	if deposit.AuctionID != "" {
		return ctx, cosmos.ErrUnknownRequest("node sale bids are funded by Shielder redeem bid_deposit")
	}
	switch deposit.Status {
	case types.DepositStatusDepositMatched:
	case types.DepositStatusSettled:
		if deposit.Settlement == "" || deposit.ShieldedSats != 0 {
			return ctx, cosmos.ErrUnknownRequest("duplicate deposit settlement")
		}
	case types.DepositStatusCommitted:
		return ctx, cosmos.ErrUnknownRequest("deposit already shielded")
	default:
		return ctx, cosmos.ErrUnknownRequest("deposit is not matched")
	}
	if deposit.ShieldedSats > deposit.AmountSats {
		return ctx, cosmos.ErrUnknownRequest("deposit shielded amount exceeds deposit amount")
	}
	availableSats := deposit.AmountSats - deposit.ShieldedSats
	if availableSats == 0 {
		return ctx, cosmos.ErrUnknownRequest("deposit already fully shielded")
	}
	noteCommitments, err := parseShielderNoteCommitments(commitments, availableSats, deposit.IsNodeBond() || deposit.Settlement == types.DepositSettlementOperatorBond)
	if err != nil {
		return ctx, err
	}
	authorizedAmountSats := shielderNoteCommitmentTotal(noteCommitments)
	noteCommitments, floorRemainder, err := applyShielderNoteFloor(ctx, k, noteCommitments, availableSats, false)
	if err != nil {
		return ctx, err
	}
	if floorRemainder > 0 {
		availableSats -= floorRemainder
	}
	amountSats := shielderNoteCommitmentTotal(noteCommitments)
	if amountSats == 0 {
		return ctx, cosmos.ErrUnknownRequest("missing shielder commitment amount")
	}
	if amountSats != availableSats {
		return ctx, cosmos.ErrUnknownRequest("shielder commitment denominations must match deposit amount")
	}
	if strings.TrimSpace(depositPubkey) != "" || strings.TrimSpace(signature) != "" {
		if err := VerifyShieldAuthorization(depositPubkey, signature, depositPubkey, authorizedAmountSats, commitments); err != nil {
			return ctx, err
		}
	}
	if ctx.IsCheckTx() {
		deposit.Status = types.DepositStatusSettled
		if err := k.SetDepositRecord(ctx, deposit); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func ShielderRedeemAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgShielderRedeem) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	return shielderRedeemAnte(ctx, k, msg.Proof, msg.Public)
}

func shielderRedeemAnte(ctx cosmos.Context, k keeper.Keeper, proof, public []byte) (cosmos.Context, error) {
	publicInputs, err := parseShielderRedeemPublicInputs(public)
	if err != nil {
		return ctx, err
	}
	policy := normalizeShielderRedeemPolicy(publicInputs.RecipientPolicy)
	if err := validateShielderRedeemPolicy(ctx, k, policy, publicInputs); err != nil {
		return ctx, err
	}
	if k.ShielderNullifierSpent(ctx, publicInputs.NullifierHash) {
		return ctx, cosmos.ErrUnknownRequest("shielder nullifier already spent")
	}
	if !k.ShielderMerkleRootExists(ctx, publicInputs.DenominationSats, publicInputs.MerkleRoot) {
		return ctx, cosmos.ErrUnknownRequest("unknown shielder merkle root")
	}
	if _, err := shielderRedeemRecipient(publicInputs, policy); err != nil {
		return ctx, err
	}
	if err := RejectLeakyShielderRedeemProof(ctx, k, proof); err != nil {
		return ctx, err
	}
	if err := VerifyShielderRedeemJSON(proof, public); err != nil {
		return ctx, err
	}
	if ctx.IsCheckTx() {
		recipient, _ := shielderRedeemRecipient(publicInputs, policy)
		withdrawalID := shielderRedeemID(publicInputs.NullifierHash, recipient.String(), policy)
		if err := k.SetShielderNullifierSpent(ctx, publicInputs.NullifierHash, withdrawalID); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func BondFromNotesAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgBondFromNotes) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	bond, err := k.GetShielderNodeBond(ctx, msg.NodePubKey)
	if err != nil {
		return ctx, err
	}
	if !bond.NodeAddress.Empty() && !bond.NodeAddress.Equals(msg.Signer) {
		return ctx, cosmos.ErrUnknownRequest("bond signer mismatch")
	}
	operator, err := common.NewPubKey(msg.OperatorPubKey)
	if err != nil {
		return ctx, err
	}
	operatorAddress, err := operator.GetThorAddress()
	if err != nil {
		return ctx, err
	}
	if !operatorAddress.Equals(msg.Signer) {
		return ctx, cosmos.ErrUnknownRequest("bond owner mismatch")
	}
	return shielderRedeemAnte(ctx, k, msg.Proof, msg.Public)
}

func ShielderShieldFeesAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgShielderShieldFees) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	bond, err := k.GetShielderNodeBond(ctx, msg.NodePubKey)
	if err != nil {
		return ctx, err
	}
	if bond.NodePubKey == "" {
		return ctx, cosmos.ErrUnknownRequest("shielder node bond not found")
	}
	if !bond.FeeShareActive {
		return ctx, cosmos.ErrUnknownRequest("shielder node has no active fee share")
	}
	if !bond.NodeAddress.Equals(msg.Signer) {
		return ctx, cosmos.ErrUnknownRequest("shielder fee signer mismatch")
	}
	if !bond.PendingFeeDepositID.IsEmpty() {
		return ctx, cosmos.ErrUnknownRequest("shielder fee settlement already pending shield")
	}
	pool, err := distributeFeePool(ctx, k)
	if err != nil {
		return ctx, err
	}
	if pool.FeePerSlotShare <= bond.FeeDebtSats {
		return ctx, cosmos.ErrUnknownRequest("no shielder fees claimable")
	}
	noteCommitments, err := parseShielderNoteCommitments(msg.Commitments, pool.FeePerSlotShare-bond.FeeDebtSats, false)
	if err != nil {
		return ctx, err
	}
	noteCommitments, _, err = applyShielderNoteFloor(ctx, k, noteCommitments, pool.FeePerSlotShare-bond.FeeDebtSats, false)
	if err != nil {
		return ctx, err
	}
	notePubKeys, err := parseShielderFeeNotePubKeys(msg.FeeNotePubKeys, len(noteCommitments))
	if err != nil {
		return ctx, err
	}
	for _, pubKey := range notePubKeys {
		if k.ShielderFeeNotePubKeyUsed(ctx, pubKey) {
			return ctx, cosmos.ErrUnknownRequest("shielder fee note pubkey already used")
		}
	}
	if ctx.IsCheckTx() {
		bond.FeeDebtSats = pool.FeePerSlotShare
		bond.UpdatedHeight = ctx.BlockHeight()
		if err := k.SetShielderNodeBond(ctx, bond); err != nil {
			return ctx, err
		}
		for _, pubKey := range notePubKeys {
			if err := k.SetShielderFeeNotePubKey(ctx, pubKey); err != nil {
				return ctx, err
			}
		}
	}
	return ctx, nil
}

func NodeSlotAuctionCreateAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgNodeSlotAuctionCreate) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	if _, err := validateNodeSlotAuctionCreate(ctx, k, msg.Signer, msg.NodePubKey, msg.ExpiryHeight); err != nil {
		return ctx, err
	}
	if ctx.IsCheckTx() {
		if _, err := CreateNodeSlotAuction(ctx, k, msg.Signer, msg.NodePubKey, msg.ReserveSats, msg.ExpiryHeight); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func NodeSlotAuctionBidCreateAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgNodeSlotAuctionBidCreate) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	if ctx.IsCheckTx() {
		if _, err := CreateNodeSlotBid(ctx, k, msg.Signer, msg.AuctionId, msg.OperatorPubKey, msg.NodePubKey); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func NodeSlotAuctionSelectBidAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgNodeSlotAuctionSelectBid) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	auction, bid, err := validateNodeSlotBidSelection(ctx, k, msg.Signer, msg.AuctionId, msg.BidId)
	if err != nil {
		return ctx, err
	}
	if ctx.IsCheckTx() {
		auction.Status = types.NodeSlotAuctionSelected
		auction.SelectedBidID = bid.BidID
		bid.Selected = true
		if err := k.SetNodeSlotAuction(ctx, auction); err != nil {
			return ctx, err
		}
		if err := k.SetNodeSlotBid(ctx, bid); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

func NodeSaleShieldAnteHandler(ctx cosmos.Context, k keeper.Keeper, msg types.MsgNodeSaleShield) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	_, _, deposit, err := validateNodeSlotSaleEntitlementShield(ctx, k, msg.Signer, msg.AuctionId, msg.BidId, msg.Commitments)
	if err != nil {
		return ctx, err
	}
	noteCommitments, err := parseShielderNoteCommitments(msg.Commitments, deposit.AmountSats, false)
	if err != nil {
		return ctx, err
	}
	authorizedAmountSats := shielderNoteCommitmentTotal(noteCommitments)
	if err := VerifyShieldAuthorization(msg.DepositPubkey, msg.Signature, msg.DepositPubkey, authorizedAmountSats, msg.Commitments); err != nil {
		return ctx, err
	}
	if ctx.IsCheckTx() {
		deposit.Status = types.DepositStatusCommitted
		if err := k.SetDepositRecord(ctx, deposit); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

// InfiniteGasDecorator uses an infinite gas meter to prevent out-of-gas panics
// and allow non-versioned changes to be made without breaking consensus,
// as long as the resulting state is consistent.
type InfiniteGasDecorator struct {
	keeper keeper.Keeper
}

func NewGasDecorator(keeper keeper.Keeper) InfiniteGasDecorator {
	return InfiniteGasDecorator{
		keeper: keeper,
	}
}

func (d InfiniteGasDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	return next(ctx, tx, simulate)
}
