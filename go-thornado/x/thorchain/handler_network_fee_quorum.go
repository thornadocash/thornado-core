package thorchain

import (
	"fmt"
	"math"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

// NetworkFeeQuorumHandler a handler to process MsgNetworkFeeQuorum messages
type NetworkFeeQuorumHandler struct {
	mgr Manager
}

// NewNetworkFeeQuorumHandler create a new instance of network fee handler
func NewNetworkFeeQuorumHandler(mgr Manager) NetworkFeeQuorumHandler {
	return NetworkFeeQuorumHandler{
		mgr: mgr,
	}
}

// Run is the main entry point for network fee logic
func (h NetworkFeeQuorumHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*types.MsgNetworkFeeQuorum)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgNetworkFeeQuorum failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to process MsgNetworkFeeQuorum", "error", err)
	}
	return result, err
}

func (h NetworkFeeQuorumHandler) validate(ctx cosmos.Context, msg types.MsgNetworkFeeQuorum) error {
	return msg.ValidateBasic()
}

func (h NetworkFeeQuorumHandler) handle(ctx cosmos.Context, msg types.MsgNetworkFeeQuorum) (*cosmos.Result, error) {
	active, err := h.mgr.Keeper().ListActiveValidators(ctx)
	if err != nil {
		err = wrapError(ctx, err, "fail to get list of active node accounts")
		return nil, err
	}

	if msg.QuoNetFee == nil || msg.QuoNetFee.NetworkFee == nil {
		return nil, cosmos.ErrUnknownRequest("QuoNetFee and NetworkFee cannot be nil")
	}
	nf := msg.QuoNetFee.NetworkFee

	if nf.TransactionRate > uint64(math.MaxInt64) || nf.TransactionSize > uint64(math.MaxInt64) {
		return nil, fmt.Errorf("transaction rate or size exceeds int64 max")
	}
	voter, err := h.mgr.Keeper().GetObservedNetworkFeeVoter(ctx, nf.Height, nf.Chain, int64(nf.TransactionRate), int64(nf.TransactionSize))
	if err != nil {
		return nil, err
	}

	defer func() {
		h.mgr.Keeper().SetObservedNetworkFeeVoter(ctx, voter)
	}()

	signBz, err := nf.GetSignablePayload()
	if err != nil {
		ctx.Logger().Error("fail to marshal network fee sign payload", "error", err)
		return nil, fmt.Errorf("fail to marshal network fee sign payload: %w", err)
	}

	attestations := deduplicateAttestations(msg.QuoNetFee.Attestations, len(active))
	for _, att := range attestations {
		accAddr, err := verifyQuorumAttestation(active, signBz, att)
		if err != nil {
			ctx.Logger().Error("fail to verify quorum network fee attestation", "error", err)
			continue
		}

		if err := processNetworkFeeAttestation(ctx, h.mgr, &voter, accAddr, active, nf, false); err != nil {
			return nil, fmt.Errorf("fail to process network fee attestation: %w", err)
		}
	}

	return &cosmos.Result{}, nil
}
