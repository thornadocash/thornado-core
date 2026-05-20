package thorchain

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

func (qs queryServer) queryReferenceMemo(ctx cosmos.Context, req *types.QueryReferenceMemoRequest) (*types.QueryReferenceMemoResponse, error) {
	asset, err := common.NewAsset(req.Asset)
	if err != nil {
		return nil, fmt.Errorf("invalid asset: %w", err)
	}

	if req.Reference == "" {
		return nil, fmt.Errorf("reference cannot be empty")
	}

	memo, err := qs.mgr.Keeper().GetReferenceMemo(ctx, asset, req.Reference)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference memo: %w", err)
	}

	if memo.Height == 0 {
		return nil, fmt.Errorf("reference memo not found for asset %s and reference %s", req.Asset, req.Reference)
	}

	return &types.QueryReferenceMemoResponse{
		Asset:            memo.Asset,
		Memo:             memo.Memo,
		Reference:        memo.Reference,
		Height:           memo.Height,
		RegistrationHash: memo.RegistrationHash,
		RegisteredBy:     memo.RegisteredBy,
		UsedByTxs:        memo.UsedByTxs,
	}, nil
}

func (qs queryServer) queryReferenceMemoByHash(ctx cosmos.Context, req *types.QueryReferenceMemoByHashRequest) (*types.QueryReferenceMemoByHashResponse, error) {
	if req.Hash == "" {
		return nil, fmt.Errorf("hash cannot be empty")
	}

	hash, err := common.NewTxID(req.Hash)
	if err != nil {
		return nil, fmt.Errorf("invalid hash: %w", err)
	}

	memo, err := qs.mgr.Keeper().GetReferenceMemoByTxnHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("reference memo not found for hash: %s", req.Hash)
	}

	if memo.Height == 0 {
		return nil, fmt.Errorf("reference memo not found for hash: %s", req.Hash)
	}

	return &types.QueryReferenceMemoByHashResponse{
		Asset:            memo.Asset,
		Memo:             memo.Memo,
		Reference:        memo.Reference,
		Height:           memo.Height,
		RegistrationHash: memo.RegistrationHash,
		RegisteredBy:     memo.RegisteredBy,
		UsedByTxs:        memo.UsedByTxs,
	}, nil
}

func (qs queryServer) queryReferenceMemoPreflight(ctx cosmos.Context, req *types.QueryReferenceMemoPreflightRequest) (*types.QueryReferenceMemoPreflightResponse, error) {
	asset, err := common.NewAsset(req.Asset)
	if err != nil {
		return nil, fmt.Errorf("invalid asset: %w", err)
	}

	// Check if memoless transactions are halted
	haltMemoless, err := qs.mgr.Keeper().GetMimir(ctx, constants.HaltMemoless.String())
	if err == nil && haltMemoless > 0 {
		return nil, fmt.Errorf("memoless transactions are currently halted")
	}

	// Check if memoless transactions are enabled
	ttl := qs.mgr.Keeper().GetConfigInt64(ctx, constants.MemolessTxnTTL)
	if ttl <= 0 {
		return nil, fmt.Errorf("memoless transactions are currently disabled")
	}

	amount, err := cosmos.ParseUint(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	// Calculate the reference ID that would be generated from this amount
	reference, err := ExtractReferenceFromAmount(ctx, qs.mgr, asset, amount.Uint64())
	if err != nil {
		return nil, fmt.Errorf("failed to calculate reference: %w", err)
	}

	// Query the reference memo to check availability
	refMemo, err := qs.mgr.Keeper().GetReferenceMemo(ctx, asset, reference)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference memo: %w", err)
	}

	response := &types.QueryReferenceMemoPreflightResponse{
		Reference:  reference,
		UsageCount: refMemo.GetUsageCount(),
		MaxUse:     qs.mgr.Keeper().GetConfigInt64(ctx, constants.MemolessTxnMaxUse),
	}

	// Check if the reference is available (not registered or expired)
	if refMemo.Height == 0 || refMemo.IsExpired(ctx.BlockHeight(), ttl) {
		response.Available = true
		response.CanRegister = true
		response.ExpiresAt = 0
	} else {
		response.Available = false
		response.Memo = refMemo.Memo
		response.ExpiresAt = refMemo.Height + ttl

		// Can register if it's expired
		response.CanRegister = refMemo.IsExpired(ctx.BlockHeight(), ttl)
	}

	return response, nil
}
