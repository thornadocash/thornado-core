package types

import (
	"encoding/json"
	"fmt"

	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/gogoproto/jsonpb"

	"github.com/thornadocash/go-thornado/common"
	openapi "github.com/thornadocash/go-thornado/openapi/gen"
)

// Implementation of JSONPBMarshaler for all query types that require it
// JSONPBMarshaler is a custom json marshaler
// We use it to marshal query responses into the expected openapi type
// It is necessary for query responses that are slices, contain maps, or contain int64 parameters.
// The proto marshaler marshals int64 parameter to strings while openapi expects int64

var (
	_ jsonpb.JSONPBMarshaler = &QueryAsgardVaultsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryBalanceModuleResponse{}
	_ jsonpb.JSONPBMarshaler = &BanVoter{}
	_ jsonpb.JSONPBMarshaler = &QueryBlockResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryConstantValuesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryDerivedPoolResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryDerivedPoolsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryInboundAddressesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryKeygenResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryKeysignResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryLastBlocksResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryLiquidityProviderResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryLiquidityProvidersResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryMimirAdminValuesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryMimirNodesAllValuesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryMimirNodesValuesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryMimirValuesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryMimirWithKeyResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryNodeResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryNodesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryObservedTxVoter{}
	_ jsonpb.JSONPBMarshaler = &QueryOutboundFeesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryOutboundResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryPoolResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryPoolSlipsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryPoolsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryQueueResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryQuoteSwapResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryRagnarokResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryRuneProviderResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryRuneProvidersResponse{}
	_ jsonpb.JSONPBMarshaler = &QuerySaverResponse{}
	_ jsonpb.JSONPBMarshaler = &QuerySaversResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryStreamingSwapResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryStreamingSwapsResponse{}
	_ jsonpb.JSONPBMarshaler = &SwapperClout{}
	_ jsonpb.JSONPBMarshaler = &QuerySwapQueueResponse{}
	_ jsonpb.JSONPBMarshaler = &QuerySwapDetailsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryThornameResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTradeAccountsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTradeUnitsResponse{}
	_ jsonpb.JSONPBMarshaler = &QuerySecuredAssetsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTssKeygenMetricResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTssMetricResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTxResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTxStagesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryTxStatusResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryUpgradeProposalResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryUpgradeProposalsResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryUpgradeVotesResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryVaultResponse{}
	_ jsonpb.JSONPBMarshaler = &QueryVersionResponse{}

	_ jsonpb.JSONPBMarshaler = &QuerySupplyResponse{}

	// Query responses that do not require a json marshaler override
	// _ jsonpb.JSONPBMarshaler = &QueryInvariantResponse{}
	// _ jsonpb.JSONPBMarshaler = &QueryInvariantsResponse{}
	// _ jsonpb.JSONPBMarshaler = &QueryNetworkResponse{}
	// _ jsonpb.JSONPBMarshaler = &QueryRunePoolResponse{}
	// _ jsonpb.JSONPBMarshaler = &QueryTradeUnitResponse{}
	// _ jsonpb.JSONPBMarshaler = &QueryVaultsPubkeysResponse{}

)

func jsonify(r any) ([]byte, error) {
	res, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("fail to marshal response to json: %w", err)
	}
	return res, nil
}

func (m *QueryAsgardVaultsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.AsgardVaults)
}

// QueryBalanceModuleResponse
func (m *QueryBalanceModuleResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// BanVoter
func (m *BanVoter) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryConstantValuesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	type constantVals struct {
		Int64Values  map[string]int64  `json:"int_64_values"`
		BoolValues   map[string]bool   `json:"bool_values"`
		StringValues map[string]string `json:"string_values"`
	}

	c := constantVals{
		Int64Values:  make(map[string]int64),
		BoolValues:   make(map[string]bool),
		StringValues: make(map[string]string),
	}

	for _, kv := range m.BoolValues {
		c.BoolValues[kv.Name] = kv.Value
	}

	for _, kv := range m.Int_64Values {
		c.Int64Values[kv.Name] = kv.Value
	}

	for _, kv := range m.StringValues {
		c.StringValues[kv.Name] = kv.Value
	}

	return jsonify(c)
}

// QueryDerivedPoolResponse
func (m *QueryDerivedPoolResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryDerivedPoolsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Pools)
}

func (m *QueryInboundAddressesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.InboundAddresses)
}

// QueryInvariantResponse
// No override needed (contains no int64 parameters)
// func (m *QueryInvariantResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
// 	return jsonify(m)
// }

// QueryInvariantsResponse
// No override needed (contains no int64 parameters)
// func (m *QueryInvariantsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
// 	return jsonify(m)
// }

// QueryKeygenResponse
func (m *QueryKeygenResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// QueryKeysignResponse
func (m *QueryKeysignResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryLastBlocksResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.LastBlocks)
}

// QueryLiquidityProviderResponse
func (m *QueryLiquidityProviderResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryLiquidityProvidersResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.LiquidityProviders)
}

func (m *QueryMimirAdminValuesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	v := make(map[string]int64)

	for _, kv := range m.AdminMimirs {
		v[kv.Key] = kv.Value
	}

	return jsonify(v)
}

// QueryMimirNodesAllValuesResponse
func (m *QueryMimirNodesAllValuesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryMimirNodesValuesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	v := make(map[string]int64)

	for _, kv := range m.Mimirs {
		v[kv.Key] = kv.Value
	}

	return jsonify(v)
}

func (m *QueryMimirValuesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	v := make(map[string]int64)

	for _, kv := range m.Mimirs {
		v[kv.Key] = kv.Value
	}

	return jsonify(v)
}

func (m *QueryMimirWithKeyResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Value)
}

// QueryNetworkResponse
// No override needed (contains no int64 parameters)
// func (m *QueryNetworkResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
// 	return jsonify(m)
// }

// QueryNodeResponse
func (m *QueryNodeResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryNodesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Nodes)
}

// ObservedTxVoter
func (m *QueryObservedTxVoter) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryOutboundFeesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.OutboundFees)
}

func (m *QueryOutboundResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.TxOutItems)
}

// QueryPoolResponse
func (m *QueryPoolResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryPoolsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Pools)
}

// QueryPoolSlipsResponse MarshalJSONPB converts to equivalent openapi type before marshaling
func (m *QueryPoolSlipsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	result := make([]openapi.PoolSlipResponseInner, len(m.PoolSlips))
	for i, slip := range m.PoolSlips {
		result[i].Asset = slip.Asset
		result[i].PoolSlip = slip.PoolSlip
		result[i].RollupCount = slip.RollupCount
		result[i].LongRollup = slip.LongRollup
		result[i].Rollup = slip.Rollup
	}

	// for single asset queries, if omitting SummedRollup when it equals 0 is okay, we don't need this openapi conversion
	if len(m.PoolSlips) == 1 {
		result[0].SummedRollup = &m.PoolSlips[0].SummedRollup
	}

	return jsonify(result)
}

// QueryQueueResponse
func (m *QueryQueueResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// QueryQuoteSwapResponse
func (m *QueryQuoteSwapResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryRagnarokResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.InProgress)
}

// QueryRunePoolResponse
// No override needed (contains no int64 parameters)
// func (m *QueryRunePoolResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
// 	return jsonify(m)
// }

// QueryRuneProviderResponse
func (m *QueryRuneProviderResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryRuneProvidersResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Providers)
}

// QuerySaverResponse
func (m *QuerySaverResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QuerySaversResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Savers)
}

// QueryStreamingSwapResponse
// QuerySupplyResponse
func (m *QuerySupplyResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryStreamingSwapResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryStreamingSwapsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.StreamingSwaps)
}

// SwapperClout
func (m *SwapperClout) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// Use openapi type since its swaptype is a string (i.e. "market", "limit") and not the int representation ("0", "1")
func (m *QuerySwapQueueResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	result := make([]openapi.MsgSwap, 0)
	for _, msg := range m.SwapQueue {
		// Only display the SwapType if it is "limit", not if "market".
		var swapType *string
		if msg.SwapType != SwapType_market {
			swapType = wrapString(msg.SwapType.String())
		}
		// TODO: After adv swap queue implementation,
		// always display the SwapType?

		result = append(result, openapi.MsgSwap{
			Tx:                      castTx(msg.Tx),
			TargetAsset:             msg.TargetAsset.String(),
			Destination:             wrapString(msg.Destination.String()),
			TradeTarget:             msg.TradeTarget.String(),
			AffiliateAddress:        wrapString(msg.AffiliateAddress.String()),
			AffiliateBasisPoints:    msg.AffiliateBasisPoints.String(),
			Signer:                  wrapString(msg.Signer.String()),
			Aggregator:              wrapString(msg.Aggregator),
			AggregatorTargetAddress: wrapString(msg.AggregatorTargetAddress),
			AggregatorTargetLimit:   wrapUintPtr(msg.AggregatorTargetLimit),
			SwapType:                swapType,
			StreamQuantity:          wrapInt64(int64(msg.StreamQuantity)),
			StreamInterval:          wrapInt64(int64(msg.StreamInterval)),
		})
	}
	return jsonify(result)
}

// QuerySwapDetailsResponse
func (m *QuerySwapDetailsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	if m.Swap == nil {
		return jsonify(openapi.SwapDetailsResponse{
			Status:    wrapString(m.Status),
			QueueType: wrapString(m.QueueType),
		})
	}

	// Only display the SwapType if it is "limit", not if "market".
	var swapType *string
	if m.Swap.SwapType != SwapType_market {
		swapType = wrapString(m.Swap.SwapType.String())
	}

	// Convert SwapState if present
	var state *openapi.SwapState
	if m.Swap.State != nil {
		state = &openapi.SwapState{
			Interval:          wrapInt64(int64(m.Swap.State.Interval)),
			Quantity:          wrapInt64(int64(m.Swap.State.Quantity)),
			Ttl:               wrapInt64(int64(m.Swap.State.Ttl)),
			Count:             wrapInt64(int64(m.Swap.State.Count)),
			LastHeight:        wrapInt64(m.Swap.State.LastHeight),
			Deposit:           wrapString(m.Swap.State.Deposit.String()),
			Withdrawn:         wrapString(m.Swap.State.Withdrawn.String()),
			In:                wrapString(m.Swap.State.In.String()),
			Out:               wrapString(m.Swap.State.Out.String()),
			FailedSwaps:       convertUint64SliceToInt64(m.Swap.State.FailedSwaps),
			FailedSwapReasons: m.Swap.State.FailedSwapReasons,
		}
	}

	result := openapi.SwapDetailsResponse{
		Swap: &openapi.MsgSwap{
			Tx:                      castTx(m.Swap.Tx),
			TargetAsset:             m.Swap.TargetAsset.String(),
			Destination:             wrapString(m.Swap.Destination.String()),
			TradeTarget:             m.Swap.TradeTarget.String(),
			AffiliateAddress:        wrapString(m.Swap.AffiliateAddress.String()),
			AffiliateBasisPoints:    m.Swap.AffiliateBasisPoints.String(),
			Signer:                  wrapString(m.Swap.Signer.String()),
			Aggregator:              wrapString(m.Swap.Aggregator),
			AggregatorTargetAddress: wrapString(m.Swap.AggregatorTargetAddress),
			AggregatorTargetLimit:   wrapUintPtr(m.Swap.AggregatorTargetLimit),
			SwapType:                swapType,
			StreamQuantity:          wrapInt64(int64(m.Swap.StreamQuantity)),
			StreamInterval:          wrapInt64(int64(m.Swap.StreamInterval)),
			InitialBlockHeight:      wrapInt64(m.Swap.InitialBlockHeight),
			State:                   state,
		},
		Status:    wrapString(m.Status),
		QueueType: wrapString(m.QueueType),
	}
	return jsonify(result)
}

// QueryThornameResponse
func (m *QueryThornameResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryTradeAccountsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.TradeAccounts)
}

// QueryTradeUnitResponse
// No override needed (contains no int64 parameters)
// func (m *QueryTradeUnitResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
// 	return jsonify(m)
// }

func (m *QueryTradeUnitsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.TradeUnits)
}

func (m *QuerySecuredAssetsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Assets)
}

func (m *QueryTssKeygenMetricResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.Metrics)
}

// QueryTssMetricResponse
func (m *QueryTssMetricResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// QueryTxResponse
func (m *QueryTxResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// QueryTxStagesResponse's MarshalJSONPB cast the protobuf type to the openapi type before marshaling
func (m *QueryTxStagesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(castTxStagesResponse(*m))
}

// QueryTxStatusResponse's MarshalJSONPB cast the protobuf type to the openapi type before marshaling
// casting is required due to the tx stages type
func (m *QueryTxStatusResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	var result openapi.TxStatusResponse

	if m.Tx != nil {
		tx := castTx(*m.Tx)
		result.Tx = &tx
	}

	if m.PlannedOutTxs != nil {
		result.PlannedOutTxs = make([]openapi.PlannedOutTx, len(m.PlannedOutTxs))
		for i, p := range m.PlannedOutTxs {
			result.PlannedOutTxs[i] = openapi.PlannedOutTx{
				Chain:     p.Chain,
				ToAddress: p.ToAddress,
				Coin:      castCoin(*p.Coin),
				Refund:    p.Refund,
			}
		}
	}

	if m.OutTxs != nil {
		result.OutTxs = make([]openapi.Tx, len(m.OutTxs))
		for i := range m.OutTxs {
			result.OutTxs[i] = castTx(m.OutTxs[i])
		}
	}

	result.Stages = castTxStagesResponse(m.Stages)

	return jsonify(result)
}

func (m *QueryUpgradeProposalResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

func (m *QueryUpgradeProposalsResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.UpgradeProposals)
}

func (m *QueryUpgradeVotesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m.UpgradeVotes)
}

// QueryVaultResponse
func (m *QueryVaultResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// QueryVaultsPubkeysResponse
// No override needed (contains no int64 parameters)
// func (m *QueryVaultsPubkeysResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
// 	return jsonify(m)
// }

// QueryVersionResponse
func (m *QueryVersionResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return jsonify(m)
}

// QueryExportResponse
// RegressionTest only
func (m *QueryExportResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return m.Content, nil
}

// Backwards compatibility for /auth/accounts/{address} endpoint
func (m *QueryAccountResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	res := map[string]any{
		"result": map[string]any{
			"value": m,
		},
	}
	return jsonify(res)
}

func (m *QueryBalancesResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	res := map[string]any{
		"result": m.Balances,
	}
	return jsonify(res)
}

// QueryOpenAPiBlockTx overrides the openapi type with a custom Tx field for marshaling.
type QueryOpenApiBlockTx struct {
	openapi.BlockTx
	Tx json.RawMessage `json:"tx,omitempty"`
}

// QueryOpenApiBlockResponse overrides the openapi type with a custom Txs field for marshaling.
type QueryOpenApiBlockResponse struct {
	openapi.BlockResponse
	Txs []QueryOpenApiBlockTx `json:"txs"`
}

func (m *QueryBlockResponse) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	res := QueryOpenApiBlockResponse{
		BlockResponse: openapi.BlockResponse{
			Id: openapi.BlockResponseId{
				Hash: m.Id.Hash,
				Parts: openapi.BlockResponseIdParts{
					Total: m.Id.Parts.Total,
					Hash:  m.Id.Parts.Hash,
				},
			},
			Header: openapi.BlockResponseHeader{
				Version: openapi.BlockResponseHeaderVersion{
					Block: m.Header.Version.Block,
					App:   m.Header.Version.App,
				},
				ChainId: m.Header.ChainId,
				Height:  m.Header.Height,
				Time:    m.Header.Time,
				LastBlockId: openapi.BlockResponseId{
					Hash: m.Header.LastBlockId.Hash,
					Parts: openapi.BlockResponseIdParts{
						Total: m.Header.LastBlockId.Parts.Total,
						Hash:  m.Header.LastBlockId.Parts.Hash,
					},
				},
				LastCommitHash:     m.Header.LastCommitHash,
				DataHash:           m.Header.DataHash,
				ValidatorsHash:     m.Header.ValidatorsHash,
				NextValidatorsHash: m.Header.NextValidatorsHash,
				ConsensusHash:      m.Header.ConsensusHash,
				AppHash:            m.Header.AppHash,
				LastResultsHash:    m.Header.LastResultsHash,
				EvidenceHash:       m.Header.EvidenceHash,
				ProposerAddress:    m.Header.ProposerAddress,
			},
			BeginBlockEvents:    []map[string]string{},
			EndBlockEvents:      []map[string]string{},
			FinalizeBlockEvents: []map[string]string{},
		},
		Txs: make([]QueryOpenApiBlockTx, len(m.Txs)),
	}

	for _, event := range m.BeginBlockEvents {
		res.BeginBlockEvents = append(res.BeginBlockEvents, eventMap(event))
	}

	for _, event := range m.EndBlockEvents {
		res.EndBlockEvents = append(res.EndBlockEvents, eventMap(event))
	}

	for _, event := range m.FinalizeBlockEvents {
		res.FinalizeBlockEvents = append(res.FinalizeBlockEvents, eventMap(event))
	}

	for i, tx := range m.Txs {
		res.Txs[i].Hash = tx.Hash
		res.Txs[i].Tx = tx.Tx
		res.Txs[i].Result.Code = &tx.Result.Code
		res.Txs[i].Result.Data = wrapString(tx.Result.Data)
		res.Txs[i].Result.Log = wrapString(tx.Result.Log)
		res.Txs[i].Result.Info = wrapString(tx.Result.Info)
		res.Txs[i].Result.GasWanted = wrapString(tx.Result.GasWanted)
		res.Txs[i].Result.GasUsed = wrapString(tx.Result.GasUsed)
		res.Txs[i].Result.Events = []map[string]string{}
		for _, event := range tx.Result.Events {
			res.Txs[i].Result.Events = append(res.Txs[i].Result.Events, eventMap(event))
		}
	}

	return jsonify(res)
}

func eventMap(e *BlockEvent) map[string]string {
	blockEventMap := make(map[string]string)
	for _, kvpair := range e.EventKvPair {
		blockEventMap[kvpair.Key] = kvpair.Value
	}

	return blockEventMap
}

func wrapString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func wrapInt64(d int64) *int64 {
	if d == 0 {
		return nil
	}
	return &d
}

func wrapUintPtr(uintPtr *sdkmath.Uint) *string {
	if uintPtr == nil {
		return nil
	}
	return wrapString(uintPtr.String())
}

func castCoin(sourceCoin common.Coin) openapi.Coin {
	return openapi.Coin{
		Asset:    sourceCoin.Asset.String(),
		Amount:   sourceCoin.Amount.String(),
		Decimals: wrapInt64(sourceCoin.Decimals),
	}
}

func castCoins(sourceCoins ...common.Coin) []openapi.Coin {
	// Leave this nil (null rather than []) if the source is nil.
	if sourceCoins == nil {
		return nil
	}

	coins := make([]openapi.Coin, len(sourceCoins))
	for i := range sourceCoins {
		coins[i] = castCoin(sourceCoins[i])
	}
	return coins
}

func castTx(tx common.Tx) openapi.Tx {
	return openapi.Tx{
		Id:          wrapString(tx.ID.String()),
		Chain:       wrapString(tx.Chain.String()),
		FromAddress: wrapString(tx.FromAddress.String()),
		ToAddress:   wrapString(tx.ToAddress.String()),
		Coins:       castCoins(tx.Coins...),
		Gas:         castCoins(tx.Gas...),
		Memo:        wrapString(tx.Memo),
	}
}

// We cast from the protobuf type to openapi to match the expected output
// If these discrepancies are okay, we can remove this casting and remove the optional declaration from OutboundSignedStage's BlocksSinceScheduled
// The disconnect is in 3 of the embedded types:
//
//	InboundObservedStage: if not completed and started == false, started would have been omitted
//	OutboundDelay: if not completed, RemainingDelayBlocks, RemainingDelaySeconds would be respectively omitted if 0
//	OutboundSignedStage: if not completed, ScheduledOutboundHeight would be omitted if 0 (not likely...)
//	                     if not completed and current height minus scheduled height equals 0, BlocksSinceScheduled would be omitted
func castTxStagesResponse(in QueryTxStagesResponse) (result openapi.TxStagesResponse) {
	result.InboundObserved.PreConfirmationCount = wrapInt64(in.InboundObserved.PreConfirmationCount)
	result.InboundObserved.FinalCount = in.InboundObserved.FinalCount
	result.InboundObserved.Completed = in.InboundObserved.Completed
	if !result.InboundObserved.Completed {
		result.InboundObserved.Started = &in.InboundObserved.Started
		return result
	}

	if in.InboundConfirmationCounted != nil {
		result.InboundConfirmationCounted = &openapi.InboundConfirmationCountedStage{
			CountingStartHeight:             wrapInt64(in.InboundConfirmationCounted.CountingStartHeight),
			Chain:                           wrapString(in.InboundConfirmationCounted.Chain),
			ExternalObservedHeight:          wrapInt64(in.InboundConfirmationCounted.ExternalObservedHeight),
			ExternalConfirmationDelayHeight: wrapInt64(in.InboundConfirmationCounted.ExternalConfirmationDelayHeight),
			RemainingConfirmationSeconds:    &in.InboundConfirmationCounted.RemainingConfirmationSeconds,
			Completed:                       in.InboundConfirmationCounted.Completed,
		}
	}

	if in.InboundFinalised != nil {
		result.InboundFinalised = &openapi.InboundFinalisedStage{
			Completed: in.InboundFinalised.Completed,
		}
	}

	if in.SwapStatus != nil {
		result.SwapStatus = &openapi.SwapStatus{
			Pending: in.SwapStatus.Pending,
		}
		if in.SwapStatus.Streaming != nil {
			result.SwapStatus.Streaming = &openapi.StreamingStatus{
				Interval: in.SwapStatus.Streaming.Interval,
				Quantity: in.SwapStatus.Streaming.Quantity,
				Count:    in.SwapStatus.Streaming.Count,
			}
		}
	}

	if in.SwapFinalised != nil {
		result.SwapFinalised = &openapi.SwapFinalisedStage{
			Completed: in.SwapFinalised.Completed,
		}
	}

	if in.OutboundDelay != nil {
		result.OutboundDelay = &openapi.OutboundDelayStage{
			Completed: in.OutboundDelay.Completed,
		}
		if !in.OutboundDelay.Completed {
			result.OutboundDelay.RemainingDelayBlocks = &in.OutboundDelay.RemainingDelayBlocks
			result.OutboundDelay.RemainingDelaySeconds = &in.OutboundDelay.RemainingDelaySeconds
		}
	}

	if in.OutboundSigned != nil {
		result.OutboundSigned = &openapi.OutboundSignedStage{
			Completed: in.OutboundSigned.Completed,
		}
		if !in.OutboundSigned.Completed {
			result.OutboundSigned.ScheduledOutboundHeight = &in.OutboundSigned.ScheduledOutboundHeight
			if in.OutboundSigned.BlocksSinceScheduled != nil {
				blocksSinceScheduled := in.OutboundSigned.GetBlocksSinceScheduled()
				result.OutboundSigned.BlocksSinceScheduled = &blocksSinceScheduled.Value
			}
		}
	}

	return result
}

func convertUint64SliceToInt64(slice []uint64) []int64 {
	if slice == nil {
		return nil
	}
	result := make([]int64, len(slice))
	for i, v := range slice {
		result[i] = int64(v)
	}
	return result
}
