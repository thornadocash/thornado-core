package types

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// all event types support by Thornado
const (
	AddLiquidityEventType           = "add_liquidity"
	BondEventType                   = "bond"
	ReBondEventType                 = "rebond"
	DonateEventType                 = "donate"
	ErrataEventType                 = "errata"
	FeeEventType                    = "fee"
	GasEventType                    = "gas"
	OutboundEventType               = "outbound"
	PendingLiquidity                = "pending_liquidity"
	PoolBalanceChangeEventType      = "pool_balance_change"
	PoolEventType                   = "pool"
	RefundEventType                 = "refund"
	RewardEventType                 = "rewards"
	ScheduledOutboundEventType      = "scheduled_outbound"
	SecurityEventType               = "security"
	SetMimirEventType               = "set_mimir"
	SetNodeMimirEventType           = "set_node_mimir"
	SlashEventType                  = "slash"
	SlashPointEventType             = "slash_points"
	SwapEventType                   = "swap"
	LimitSwapEventType              = "limit_swap"
	ModifyLimitSwapEventType        = "limit_swap_mod"
	LimitSwapCloseEventType         = "limit_swap_close"
	MintBurnType                    = "mint_burn"
	SwitchEventType                 = "switch"
	OperatorRotateEventType         = "operator_rotate"
	TSSKeygenSuccess                = "tss_keygen_success"
	TSSKeygenFailure                = "tss_keygen_failure"
	TSSKeygenMetricEventType        = "tss_keygen"
	TSSKeysignMetricEventType       = "tss_keysign"
	VersionEventType                = "version"
	WithdrawEventType               = "withdraw"
	OraclePriceEvent                = "oracle_price"
	FailedOutboundRecoveryEventType = "failed_outbound_recovery"
)

// PoolMods a list of pool modifications
type PoolMods []PoolMod

// NewPoolMod create a new instance of PoolMod
func NewPoolMod(asset common.Asset, runeAmt cosmos.Uint, runeAdd bool, assetAmt cosmos.Uint, assetAdd bool) PoolMod {
	return PoolMod{
		Asset:    asset,
		RuneAmt:  runeAmt,
		RuneAdd:  runeAdd,
		AssetAmt: assetAmt,
		AssetAdd: assetAdd,
	}
}

// NewEventLimitSwap create a new swap event
func NewEventLimitSwap(source, target common.Coin, txid common.TxID) *EventLimitSwap {
	return &EventLimitSwap{
		Source: source,
		Target: target,
		TxID:   txid,
	}
}

// Type return a string that represent the type, it should not duplicated with other event
func (m *EventLimitSwap) Type() string {
	return LimitSwapEventType
}

// Events convert EventLimitSwap to key value pairs used in cosmos
func (m *EventLimitSwap) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("source", m.Source.String()),
		cosmos.NewAttribute("target", m.Target.String()),
		cosmos.NewAttribute("txid", m.TxID.String()),
	)
	return cosmos.Events{evt}, nil
}

// NewEventModifyLimitSwap create a new modify limit swap event
func NewEventModifyLimitSwap(from common.Address, source, target common.Coin, mod cosmos.Uint) *EventModifyLimitSwap {
	return &EventModifyLimitSwap{
		From:                 from,
		Source:               source,
		Target:               target,
		ModifiedTargetAmount: mod,
	}
}

// Type return a string that represent the type, it should not duplicated with other event
func (m *EventModifyLimitSwap) Type() string {
	return ModifyLimitSwapEventType
}

// Events convert EventModifyLimitSwap to key value pairs used in cosmos
func (m *EventModifyLimitSwap) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("from", m.From.String()),
		cosmos.NewAttribute("source", m.Source.String()),
		cosmos.NewAttribute("target", m.Target.String()),
		cosmos.NewAttribute("modified_target_amount", m.ModifiedTargetAmount.String()),
	)
	return cosmos.Events{evt}, nil
}

// NewEventLimitSwapClose create a new limit swap close event
func NewEventLimitSwapClose(txid common.TxID, reason string, blockHeight int64) *EventLimitSwapClose {
	return &EventLimitSwapClose{
		TxID:        txid,
		Reason:      reason,
		BlockHeight: blockHeight,
	}
}

// Type return a string that represent the type, it should not duplicated with other event
func (m *EventLimitSwapClose) Type() string {
	return LimitSwapCloseEventType
}

// Events convert EventLimitSwapClose to key value pairs used in cosmos
func (m *EventLimitSwapClose) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("txid", m.TxID.String()),
		cosmos.NewAttribute("reason", m.Reason),
		cosmos.NewAttribute("block_height", fmt.Sprintf("%d", m.BlockHeight)),
	)
	return cosmos.Events{evt}, nil
}

// NewEventSwap create a new swap event
func NewEventSwap(pool common.Asset, swapTarget, fee, swapSlip, liquidityFeeInRune cosmos.Uint, inTx common.Tx, emitAsset common.Coin, synthUnits cosmos.Uint) *EventSwap {
	return &EventSwap{
		Pool:               pool,
		SwapTarget:         swapTarget,
		SwapSlip:           swapSlip,
		LiquidityFee:       fee,
		LiquidityFeeInRune: liquidityFeeInRune,
		InTx:               inTx,
		EmitAsset:          emitAsset,
		SynthUnits:         synthUnits,
		PoolSlip:           cosmos.ZeroUint(),
	}
}

// Type return a string that represent the type, it should not duplicated with other event
func (m *EventSwap) Type() string {
	return SwapEventType
}

// Events convert EventSwap to key value pairs used in cosmos
func (m *EventSwap) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pool", m.Pool.String()),
		cosmos.NewAttribute("swap_target", m.SwapTarget.String()),
		cosmos.NewAttribute("swap_slip", m.SwapSlip.String()),
		cosmos.NewAttribute("liquidity_fee", m.LiquidityFee.String()),
		cosmos.NewAttribute("liquidity_fee_in_rune", m.LiquidityFeeInRune.String()),
		cosmos.NewAttribute("emit_asset", m.EmitAsset.String()),
		cosmos.NewAttribute("pool_slip", m.PoolSlip.String()),
	)
	if !m.SynthUnits.IsZero() {
		evt = evt.AppendAttributes(cosmos.NewAttribute("synth_units", m.SynthUnits.String()))
	}
	evt = evt.AppendAttributes(m.InTx.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventDonate create a new donate event
func NewEventDonate(pool common.Asset, inTx common.Tx) *EventDonate {
	return &EventDonate{
		Pool: pool,
		InTx: inTx,
	}
}

// Type return donate event type
func (m *EventDonate) Type() string {
	return DonateEventType
}

// Events get all events
func (m *EventDonate) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pool", m.Pool.String()))
	evt = evt.AppendAttributes(m.InTx.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventPool create a new pool change event
func NewEventPool(pool common.Asset, status string) *EventPool {
	return &EventPool{
		Pool:   pool,
		Status: status,
	}
}

// Type return pool event type
func (m *EventPool) Type() string {
	return PoolEventType
}

// Events provide an instance of cosmos.Events
func (m *EventPool) Events() (cosmos.Events, error) {
	return cosmos.Events{
		cosmos.NewEvent(m.Type(),
			cosmos.NewAttribute("pool", m.Pool.String()),
			cosmos.NewAttribute("pool_status", m.Status)),
	}, nil
}

// NewEventRewards create a new reward event
func NewEventRewards(bondReward cosmos.Uint, poolRewards []PoolAmt, incomeBurn cosmos.Uint) *EventRewards {
	return &EventRewards{
		BondReward:  bondReward,
		PoolRewards: poolRewards,
		IncomeBurn:  incomeBurn,
	}
}

// Type return reward event type
func (m *EventRewards) Type() string {
	return RewardEventType
}

// Events return a standard cosmos event
func (m *EventRewards) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("bond_reward", m.BondReward.String()),
		cosmos.NewAttribute("income_burn", m.IncomeBurn.String()),
	)
	for _, item := range m.PoolRewards {
		evt = evt.AppendAttributes(cosmos.NewAttribute(item.Asset.String(), strconv.FormatInt(item.Amount, 10)))
	}
	return cosmos.Events{evt}, nil
}

// NewEventRefund create a new EventRefund
func NewEventRefund(code uint32, reason string, inTx common.Tx, fee common.Fee) *EventRefund {
	return &EventRefund{
		Code:   code,
		Reason: reason,
		InTx:   inTx,
		Fee:    fee,
	}
}

// Type return reward event type
func (m *EventRefund) Type() string {
	return RefundEventType
}

// Events return events
func (m *EventRefund) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("code", strconv.FormatUint(uint64(m.Code), 10)),
		cosmos.NewAttribute("reason", m.Reason),
	)
	evt = evt.AppendAttributes(m.InTx.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventBond create a new Bond Events
func NewEventBond(amount cosmos.Uint, bondType BondType, txIn common.Tx, nodeAccount *NodeAccount, bondAddress cosmos.AccAddress) *EventBond {
	return &EventBond{
		Amount:      amount,
		BondType:    bondType,
		TxIn:        txIn,
		NodeAddress: nodeAccount.NodeAddress,
		BondAddress: bondAddress,
	}
}

// Type return bond event Type
func (m *EventBond) Type() string {
	return BondEventType
}

// Events return all the event attributes
func (m *EventBond) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("amount", m.Amount.String()),
		cosmos.NewAttribute("bond_type", m.BondType.String()),
		cosmos.NewAttribute("node_address", m.NodeAddress.String()),
		cosmos.NewAttribute("bond_address", m.BondAddress.String()))
	evt = evt.AppendAttributes(m.TxIn.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventReBond create a new ReBond Event
func NewEventReBond(
	amount cosmos.Uint, txIn common.Tx,
	nodeAccount *NodeAccount,
	oldProvider, newProvider cosmos.AccAddress,
) *EventReBond {
	return &EventReBond{
		Amount:         amount,
		TxIn:           txIn,
		NodeAddress:    nodeAccount.NodeAddress,
		OldBondAddress: oldProvider,
		NewBondAddress: newProvider,
	}
}

// Type return bond event Type
func (m *EventReBond) Type() string {
	return ReBondEventType
}

// Events return all the event attributes
func (m *EventReBond) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("amount", m.Amount.String()),
		cosmos.NewAttribute("node_address", m.NodeAddress.String()),
		cosmos.NewAttribute("old_bond_address", m.OldBondAddress.String()),
		cosmos.NewAttribute("new_bond_address", m.NewBondAddress.String()))
	evt = evt.AppendAttributes(m.TxIn.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventGas create a new EventGas instance
func NewEventGas() *EventGas {
	return &EventGas{
		Pools: make([]GasPool, 0),
	}
}

// UpsertGasPool update the Gas Pools hold by EventGas instance
// if the given gasPool already exist, then it merge the gasPool with internal one , otherwise add it to the list
func (m *EventGas) UpsertGasPool(pool GasPool) {
	for i, p := range m.Pools {
		if p.Asset == pool.Asset {
			m.Pools[i].RuneAmt = p.RuneAmt.Add(pool.RuneAmt)
			m.Pools[i].AssetAmt = p.AssetAmt.Add(pool.AssetAmt)
			return
		}
	}
	m.Pools = append(m.Pools, pool)
}

// Type return event type
func (m *EventGas) Type() string {
	return GasEventType
}

// Events return a standard cosmos events
func (m *EventGas) Events() (cosmos.Events, error) {
	events := make(cosmos.Events, 0, len(m.Pools))
	for _, item := range m.Pools {
		evt := cosmos.NewEvent(m.Type(),
			cosmos.NewAttribute("asset", item.Asset.String()),
			cosmos.NewAttribute("asset_amt", item.AssetAmt.String()),
			cosmos.NewAttribute("rune_amt", item.RuneAmt.String()),
			cosmos.NewAttribute("transaction_count", strconv.FormatInt(item.Count, 10)))
		events = append(events, evt)
	}
	return events, nil
}

// NewEventScheduledOutbound creates a new scheduled outbound event.
func NewEventScheduledOutbound(tx TxOutItem) *EventScheduledOutbound {
	return &EventScheduledOutbound{
		OutTx: tx,
	}
}

// Type returns the scheduled outbound event type.
func (m *EventScheduledOutbound) Type() string {
	return ScheduledOutboundEventType
}

// Events returns the cosmos events for the scheduled outbound event.
func (m *EventScheduledOutbound) Events() (cosmos.Events, error) {
	attrs := []cosmos.Attribute{
		cosmos.NewAttribute("chain", m.OutTx.Chain.String()),
		cosmos.NewAttribute("to_address", m.OutTx.ToAddress.String()),
		cosmos.NewAttribute("vault_pub_key", m.OutTx.VaultPubKey.String()),
		cosmos.NewAttribute("coin_asset", m.OutTx.Coin.Asset.String()),
		cosmos.NewAttribute("coin_amount", m.OutTx.Coin.Amount.String()),
		cosmos.NewAttribute("coin_decimals", strconv.FormatInt(m.OutTx.Coin.Decimals, 10)),
		cosmos.NewAttribute("memo", m.OutTx.Memo),
		cosmos.NewAttribute("gas_rate", strconv.FormatInt(m.OutTx.GasRate, 10)),
		cosmos.NewAttribute("in_hash", m.OutTx.InHash.String()),
		cosmos.NewAttribute("out_hash", m.OutTx.OutHash.String()),
		cosmos.NewAttribute("module_name", m.OutTx.ModuleName),
	}

	for i, gas := range m.OutTx.MaxGas {
		attrs = append(attrs, cosmos.NewAttribute(fmt.Sprintf("max_gas_asset_%d", i), gas.Asset.String()))
		attrs = append(attrs, cosmos.NewAttribute(fmt.Sprintf("max_gas_amount_%d", i), gas.Amount.String()))
		attrs = append(attrs, cosmos.NewAttribute(fmt.Sprintf("max_gas_decimals_%d", i), strconv.FormatInt(gas.Decimals, 10)))
	}

	return cosmos.Events{cosmos.NewEvent(m.Type(), attrs...)}, nil
}

// NewEventSecurity creates a new security event.
func NewEventSecurity(tx common.Tx, msg string) *EventSecurity {
	return &EventSecurity{
		Msg: msg,
		Tx:  tx,
	}
}

// Type returns the security event type.
func (m *EventSecurity) Type() string {
	return SecurityEventType
}

// Events returns the cosmos events for the security event.
func (m *EventSecurity) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(), cosmos.NewAttribute("msg", m.Msg))
	evt = evt.AppendAttributes(m.Tx.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventSlash create a new slash event
func NewEventSlash(pool common.Asset, slashAmount []PoolAmt) *EventSlash {
	return &EventSlash{
		Pool:        pool,
		SlashAmount: slashAmount,
	}
}

// Type return slash event type
func (m *EventSlash) Type() string {
	return SlashEventType
}

// Events return a standard cosmos events
func (m *EventSlash) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pool", m.Pool.String()))
	for _, item := range m.SlashAmount {
		evt = evt.AppendAttributes(cosmos.NewAttribute(item.Asset.String(), strconv.FormatInt(item.Amount, 10)))
	}
	return cosmos.Events{evt}, nil
}

// NewEventErrata create a new errata event
func NewEventErrata(txID common.TxID, pools PoolMods) *EventErrata {
	return &EventErrata{
		TxID:  txID,
		Pools: pools,
	}
}

// Type return slash event type
func (m *EventErrata) Type() string {
	return ErrataEventType
}

// Events return a cosmos.Events type
func (m *EventErrata) Events() (cosmos.Events, error) {
	events := make(cosmos.Events, 0, len(m.Pools))
	for _, item := range m.Pools {
		evt := cosmos.NewEvent(m.Type(),
			cosmos.NewAttribute("in_tx_id", m.TxID.String()),
			cosmos.NewAttribute("asset", item.Asset.String()),
			cosmos.NewAttribute("rune_amt", item.RuneAmt.String()),
			cosmos.NewAttribute("rune_add", strconv.FormatBool(item.RuneAdd)),
			cosmos.NewAttribute("asset_amt", item.AssetAmt.String()),
			cosmos.NewAttribute("asset_add", strconv.FormatBool(item.AssetAdd)))
		events = append(events, evt)
	}
	return events, nil
}

// NewEventFee create a new EventFee
func NewEventFee(txID common.TxID, fee common.Fee, synthUnits cosmos.Uint) *EventFee {
	return &EventFee{
		TxID:       txID,
		Fee:        fee,
		SynthUnits: synthUnits,
	}
}

// Type get a string represent the event type
func (m *EventFee) Type() string {
	return FeeEventType
}

// Events return events of cosmos.Event type
func (m *EventFee) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("tx_id", m.TxID.String()),
		cosmos.NewAttribute("coins", m.Fee.Coins.String()),
		cosmos.NewAttribute("pool_deduct", m.Fee.PoolDeduct.String()))
	if !m.SynthUnits.IsZero() {
		evt = evt.AppendAttributes(
			cosmos.NewAttribute("synth_units", m.SynthUnits.String()),
		)
	}
	return cosmos.Events{evt}, nil
}

// NewEventOutbound create a new instance of EventOutbound
func NewEventOutbound(inTxID common.TxID, tx common.Tx) *EventOutbound {
	return &EventOutbound{
		InTxID: inTxID,
		Tx:     tx,
	}
}

// Type return a string which represent the type of this event
func (m *EventOutbound) Type() string {
	return OutboundEventType
}

// Events return sdk events
func (m *EventOutbound) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("in_tx_id", m.InTxID.String()))
	evt = evt.AppendAttributes(m.Tx.ToAttributes()...)
	return cosmos.Events{evt}, nil
}

// NewEventTssKeygenSuccess create a new EventTssKeygenSuccess
func NewEventTssKeygenSuccess(pubkey common.PubKey, height int64, members []string) *EventTssKeygenSuccess {
	return &EventTssKeygenSuccess{
		PubKey:  pubkey,
		Height:  height,
		Members: members,
	}
}

// Type  return a string which represent the type of this event
func (m *EventTssKeygenSuccess) Type() string {
	return TSSKeygenSuccess
}

// Events return cosmos sdk events
func (m *EventTssKeygenSuccess) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pubkey", m.PubKey.String()),
		cosmos.NewAttribute("height", strconv.FormatInt(m.Height, 10)),
		cosmos.NewAttribute("members", strings.Join(m.Members, ", ")),
	)
	return cosmos.Events{evt}, nil
}

// NewEventTssKeygenFailure create a new EventTssKeygenFailure
func NewEventTssKeygenFailure(reason, round string, unicast bool, height int64, blame []string) *EventTssKeygenFailure {
	return &EventTssKeygenFailure{
		FailReason: reason,
		IsUnicast:  unicast,
		Round:      round,
		Height:     height,
		BlameNodes: blame,
	}
}

// Type  return a string which represent the type of this event
func (m *EventTssKeygenFailure) Type() string {
	return TSSKeygenFailure
}

// Events return cosmos sdk events
func (m *EventTssKeygenFailure) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("reason", m.FailReason),
		cosmos.NewAttribute("round", m.Round),
		cosmos.NewAttribute("is_unicast", fmt.Sprintf("%v", m.IsUnicast)),
		cosmos.NewAttribute("height", strconv.FormatInt(m.Height, 10)),
		cosmos.NewAttribute("blame", strings.Join(m.BlameNodes, ", ")),
	)
	return cosmos.Events{evt}, nil
}

// NewEventTssKeygenMetric create a new EventTssMetric
func NewEventTssKeygenMetric(pubkey common.PubKey, medianDurationMS int64) *EventTssKeygenMetric {
	return &EventTssKeygenMetric{
		PubKey:           pubkey,
		MedianDurationMs: medianDurationMS,
	}
}

// Type  return a string which represent the type of this event
func (m *EventTssKeygenMetric) Type() string {
	return TSSKeygenMetricEventType
}

// Events return cosmos sdk events
func (m *EventTssKeygenMetric) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pubkey", m.PubKey.String()),
		cosmos.NewAttribute("median_duration_ms", strconv.FormatInt(m.MedianDurationMs, 10)))
	return cosmos.Events{evt}, nil
}

// NewEventTssKeysignMetric create a new EventTssMetric
func NewEventTssKeysignMetric(txID common.TxID, medianDurationMS int64) *EventTssKeysignMetric {
	return &EventTssKeysignMetric{
		TxID:             txID,
		MedianDurationMs: medianDurationMS,
	}
}

// Type  return a string which represent the type of this event
func (m *EventTssKeysignMetric) Type() string {
	return TSSKeysignMetricEventType
}

// Events return cosmos sdk events
func (m *EventTssKeysignMetric) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("txid", m.TxID.String()),
		cosmos.NewAttribute("median_duration_ms", strconv.FormatInt(m.MedianDurationMs, 10)))
	return cosmos.Events{evt}, nil
}

// NewEventSlashPoint create a new slash point event
func NewEventSlashPoint(addr cosmos.AccAddress, slashPoints int64, reason string) *EventSlashPoint {
	return &EventSlashPoint{
		NodeAddress: addr,
		SlashPoints: slashPoints,
		Reason:      reason,
	}
}

// Type return a string which represent the type of this event
func (m *EventSlashPoint) Type() string {
	return SlashPointEventType
}

// Events return cosmos sdk events
func (m *EventSlashPoint) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("node_address", m.NodeAddress.String()),
		cosmos.NewAttribute("slash_points", strconv.FormatInt(m.SlashPoints, 10)),
		cosmos.NewAttribute("reason", m.Reason))
	return cosmos.Events{evt}, nil
}

// NewEventPoolBalanceChanged create a new instance of EventPoolBalanceChanged
func NewEventPoolBalanceChanged(poolMod PoolMod, reason string) *EventPoolBalanceChanged {
	return &EventPoolBalanceChanged{
		PoolChange: poolMod,
		Reason:     reason,
	}
}

// Type return a string which represent the type of this event
func (m *EventPoolBalanceChanged) Type() string {
	return PoolBalanceChangeEventType
}

// Events return cosmos sdk events
func (m *EventPoolBalanceChanged) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("asset", m.PoolChange.Asset.String()),
		cosmos.NewAttribute("rune_amt", m.PoolChange.RuneAmt.String()),
		cosmos.NewAttribute("rune_add", strconv.FormatBool(m.PoolChange.RuneAdd)),
		cosmos.NewAttribute("asset_amt", m.PoolChange.AssetAmt.String()),
		cosmos.NewAttribute("asset_add", strconv.FormatBool(m.PoolChange.AssetAdd)),
		cosmos.NewAttribute("reason", m.GetReason()))
	return cosmos.Events{evt}, nil
}

func NewEventSetMimir(key, value string) *EventSetMimir {
	// NewEventSetMimir create a new instance of EventSetMimir
	return &EventSetMimir{
		Key:   key,
		Value: value,
	}
}

// Type return a string which represent the type of this event
func (m *EventSetMimir) Type() string {
	return SetMimirEventType
}

// Events return cosmos sdk events
func (m *EventSetMimir) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("key", m.Key),
		cosmos.NewAttribute("value", m.Value),
	)
	return cosmos.Events{evt}, nil
}

// NewEventMintBurn create a new instance of EventMintBurn
func NewEventMintBurn(t MintBurnSupplyType, denom string, amt cosmos.Uint, reason string) *EventMintBurn {
	return &EventMintBurn{
		Supply: t,
		Denom:  denom,
		Amount: amt,
		Reason: reason,
	}
}

func (m *EventMintBurn) Type() string {
	return MintBurnType
}

// Events return cosmos sdk events
func (m *EventMintBurn) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("supply", m.Supply.String()),
		cosmos.NewAttribute("denom", m.Denom),
		cosmos.NewAttribute("amount", m.Amount.String()),
		cosmos.NewAttribute("reason", m.Reason))
	return cosmos.Events{evt}, nil
}

// NewEventSetNodeMimir create a new instance of EventSetNodeMimir
func NewEventSetNodeMimir(key, value, address string) *EventSetNodeMimir {
	return &EventSetNodeMimir{
		Key:     key,
		Value:   value,
		Address: address,
	}
}

// Type return a string which represent the type of this event
func (m *EventSetNodeMimir) Type() string {
	return SetNodeMimirEventType
}

// Events return cosmos sdk events
func (m *EventSetNodeMimir) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("key", m.Key),
		cosmos.NewAttribute("value", m.Value),
		cosmos.NewAttribute("address", m.Address),
	)
	return cosmos.Events{evt}, nil
}

// NewEventVersion create a new instance of EventVersion
func NewEventVersion(version semver.Version) *EventVersion {
	return &EventVersion{
		Version: version.String(),
	}
}

func (m *EventVersion) Type() string {
	return VersionEventType
}

// Events return cosmos sdk events
func (m *EventVersion) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("version", m.Version),
	)
	return cosmos.Events{evt}, nil
}

// NewEventSwitch creates a new switch event.
func NewEventSwitch(
	amt cosmos.Uint,
	asset common.Asset,
	assetAddress common.Address,
	runeAddress common.Address,
	txID common.TxID,
) *EventSwitch {
	return &EventSwitch{
		Amount:       amt,
		Asset:        asset,
		AssetAddress: assetAddress,
		RuneAddress:  runeAddress,
		TxID:         txID,
	}
}

// Type return the deposit event type
func (m *EventSwitch) Type() string {
	return SwitchEventType
}

// Events return the cosmos event
func (m *EventSwitch) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("amount", m.Amount.String()),
		cosmos.NewAttribute("asset", m.Asset.String()),
		cosmos.NewAttribute("rune_address", m.RuneAddress.String()),
		cosmos.NewAttribute("asset_address", m.AssetAddress.String()),
		cosmos.NewAttribute("tx_id", m.TxID.String()))
	return cosmos.Events{evt}, nil
}

// NewEventOperatorRotate creates a new rotate event.
func NewEventOperatorRotate(
	signer, nodeAddress, operatorAddress cosmos.AccAddress,
) *EventOperatorRotate {
	return &EventOperatorRotate{
		Signer:          signer,
		NodeAddress:     nodeAddress,
		OperatorAddress: operatorAddress,
	}
}

// Type return the rotate event type.
func (m *EventOperatorRotate) Type() string {
	return OperatorRotateEventType
}

// Events return the cosmos event.
func (m *EventOperatorRotate) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("signer", m.Signer.String()),
		cosmos.NewAttribute("node_address", m.NodeAddress.String()),
		cosmos.NewAttribute("operator_address", m.OperatorAddress.String()),
	)
	return cosmos.Events{evt}, nil
}

// NewEventOraclePrice create a new EventOraclePrice
func NewEventOraclePrice(symbol, price string) *EventOraclePrice {
	return &EventOraclePrice{
		Symbol: symbol,
		Price:  price,
	}
}

// Type return oracle price event type
func (m *EventOraclePrice) Type() string { return OraclePriceEvent }

// Events return events
func (m EventOraclePrice) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("symbol", m.Symbol),
		cosmos.NewAttribute("price", m.Price),
	)
	return cosmos.Events{evt}, nil
}

// NewEventFailedOutboundRecovery create a new EventFailedOutboundRecovery
func NewEventFailedOutboundRecovery(inTxID common.TxID, coin common.Coin, recoveryType string, destination common.Address, memo string) *EventFailedOutboundRecovery {
	return &EventFailedOutboundRecovery{
		InTxID:       inTxID,
		Coin:         coin,
		RecoveryType: recoveryType,
		Destination:  destination,
		Memo:         memo,
	}
}

// Type return failed outbound recovery event type
func (m *EventFailedOutboundRecovery) Type() string {
	return FailedOutboundRecoveryEventType
}

// Events return events
func (m *EventFailedOutboundRecovery) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("in_tx_id", m.InTxID.String()),
		cosmos.NewAttribute("coin", m.Coin.String()),
		cosmos.NewAttribute("recovery_type", m.RecoveryType),
		cosmos.NewAttribute("destination", m.Destination.String()),
		cosmos.NewAttribute("memo", m.Memo),
	)
	return cosmos.Events{evt}, nil
}
