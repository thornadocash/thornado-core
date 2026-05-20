package thorchain

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

// ObservedTxQuorumHandler to handle MsgObservedTx
type ObservedTxQuorumHandler struct {
	mgr Manager
}

// NewObservedTxQuorumHandler create a new instance of ObservedTxQuorumHandler
func NewObservedTxQuorumHandler(mgr Manager) ObservedTxQuorumHandler {
	return ObservedTxQuorumHandler{
		mgr: mgr,
	}
}

// Run is the main entry point of ObservedTxQuorumHandler
func (h ObservedTxQuorumHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*types.MsgObservedTxQuorum)
	if !ok {
		return nil, errInvalidMessage
	}
	err := h.validate(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("MsgObservedTxQuorum failed validation", "error", err)
		return nil, err
	}

	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to handle MsgObservedTxQuorum message", "error", err)
	}
	return result, err
}

func (h ObservedTxQuorumHandler) validate(ctx cosmos.Context, msg types.MsgObservedTxQuorum) error {
	return msg.ValidateBasic()
}

func (h ObservedTxQuorumHandler) handle(ctx cosmos.Context, msg types.MsgObservedTxQuorum) (*cosmos.Result, error) {
	k := h.mgr.Keeper()

	activeNodeAccounts, err := k.ListActiveValidators(ctx)
	if err != nil {
		return nil, wrapError(ctx, err, "fail to get list of active node accounts")
	}
	handler := NewInternalHandler(h.mgr)

	if msg.QuoTx == nil {
		return nil, cosmos.ErrUnknownRequest("QuoTx cannot be nil")
	}
	quoTx := msg.QuoTx
	obsTx := quoTx.ObsTx
	inbound := quoTx.Inbound

	// check we are sending to a valid vault
	var voter types.ObservedTxVoter
	if inbound {
		voter, err = ensureVaultAndGetTxInVoter(ctx, obsTx.ObservedPubKey, obsTx.Tx.ID, k)
		if err != nil {
			ctx.Logger().Error("fail to ensure vault and get tx in voter", "error", err)
			return &cosmos.Result{}, nil
		}
	} else {
		voter, err = ensureVaultAndGetTxOutVoter(ctx, k, obsTx.ObservedPubKey, obsTx.Tx.ID, msg.GetSigners(), obsTx.KeysignMs)
		if err != nil {
			ctx.Logger().Error("fail to ensure vault and get tx out voter", "error", err)
			return &cosmos.Result{}, nil
		}
	}

	signBz, err := obsTx.GetSignablePayload()
	if err != nil {
		ctx.Logger().Error("fail to marshal tx sign payload", "error", err)
		return &cosmos.Result{}, nil
	}

	var crossedQuorum bool
	var accAddrs []cosmos.AccAddress
	attestations := deduplicateAttestations(msg.QuoTx.Attestations, len(activeNodeAccounts))
	for _, att := range attestations {
		accAddr, err := verifyQuorumAttestation(activeNodeAccounts, signBz, att)
		if err != nil {
			ctx.Logger().Error("fail to verify quorum tx in attestation", "error", err)
			continue
		}

		accAddrs = append(accAddrs, accAddr)

		var isQuorum bool
		if inbound {
			voter, isQuorum = processTxInAttestation(ctx, h.mgr, voter, activeNodeAccounts, obsTx, accAddr, false)
		} else {
			voter, isQuorum = processTxOutAttestation(ctx, h.mgr, voter, activeNodeAccounts, obsTx, accAddr, false)
		}
		if isQuorum {
			// we crossed over quorum with this attestation.
			crossedQuorum = true
		}
	}

	if inbound {
		if err := handleObservedTxInQuorum(ctx, h.mgr, msg.Signer, activeNodeAccounts, handler, obsTx, voter, accAddrs, crossedQuorum); err != nil {
			return nil, wrapError(ctx, err, "fail to handle observed tx in quorum")
		}
	} else {
		if err := handleObservedTxOutQuorum(ctx, h.mgr, msg.Signer, activeNodeAccounts, handler, obsTx, voter, accAddrs, crossedQuorum); err != nil {
			return nil, wrapError(ctx, err, "fail to handle observed tx out quorum")
		}
	}

	return &cosmos.Result{}, nil
}
