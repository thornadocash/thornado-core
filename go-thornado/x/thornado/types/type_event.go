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
	BondEventType                   = "bond"
	FeeEventType                    = "fee"
	GasEventType                    = "gas"
	OutboundEventType               = "outbound"
	ScheduledOutboundEventType      = "scheduled_outbound"
	SecurityEventType               = "security"
	SetConfigEventType              = "set_config"
	SetNodeConfigEventType          = "set_node_config"
	PenaltyPointEventType           = "penalty_points"
	MintBurnType                    = "mint_burn"
	OperatorRotateEventType         = "operator_rotate"
	FROSTKeygenSuccess                = "frost_keygen_success"
	FROSTKeygenFailure                = "frost_keygen_failure"
	FROSTKeygenMetricEventType        = "frost_keygen"
	FROSTKeysignMetricEventType       = "frost_keysign"
	VersionEventType                = "version"
	OraclePriceEvent                = "oracle_price"
	FailedOutboundRecoveryEventType = "failed_outbound_recovery"
)

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

// NewEventFee create a new EventFee
func NewEventFee(txID common.TxID, fee common.Fee) *EventFee {
	return &EventFee{
		TxID: txID,
		Fee:  fee,
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
		cosmos.NewAttribute("coins", m.Fee.Coins.String()))
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

// NewEventFrostKeygenSuccess create a new EventFrostKeygenSuccess
func NewEventFrostKeygenSuccess(pubkey common.PubKey, height int64, members []string) *EventFrostKeygenSuccess {
	return &EventFrostKeygenSuccess{
		PubKey:  pubkey,
		Height:  height,
		Members: members,
	}
}

// Type  return a string which represent the type of this event
func (m *EventFrostKeygenSuccess) Type() string {
	return FROSTKeygenSuccess
}

// Events return cosmos sdk events
func (m *EventFrostKeygenSuccess) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pubkey", m.PubKey.String()),
		cosmos.NewAttribute("height", strconv.FormatInt(m.Height, 10)),
		cosmos.NewAttribute("members", strings.Join(m.Members, ", ")),
	)
	return cosmos.Events{evt}, nil
}

// NewEventFrostKeygenFailure create a new EventFrostKeygenFailure
func NewEventFrostKeygenFailure(reason, round string, unicast bool, height int64, blame []string) *EventFrostKeygenFailure {
	return &EventFrostKeygenFailure{
		FailReason: reason,
		IsUnicast:  unicast,
		Round:      round,
		Height:     height,
		BlameNodes: blame,
	}
}

// Type  return a string which represent the type of this event
func (m *EventFrostKeygenFailure) Type() string {
	return FROSTKeygenFailure
}

// Events return cosmos sdk events
func (m *EventFrostKeygenFailure) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("reason", m.FailReason),
		cosmos.NewAttribute("round", m.Round),
		cosmos.NewAttribute("is_unicast", fmt.Sprintf("%v", m.IsUnicast)),
		cosmos.NewAttribute("height", strconv.FormatInt(m.Height, 10)),
		cosmos.NewAttribute("blame", strings.Join(m.BlameNodes, ", ")),
	)
	return cosmos.Events{evt}, nil
}

// NewEventFrostKeygenMetric create a new EventFrostMetric
func NewEventFrostKeygenMetric(pubkey common.PubKey, medianDurationMS int64) *EventFrostKeygenMetric {
	return &EventFrostKeygenMetric{
		PubKey:           pubkey,
		MedianDurationMs: medianDurationMS,
	}
}

// Type  return a string which represent the type of this event
func (m *EventFrostKeygenMetric) Type() string {
	return FROSTKeygenMetricEventType
}

// Events return cosmos sdk events
func (m *EventFrostKeygenMetric) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("pubkey", m.PubKey.String()),
		cosmos.NewAttribute("median_duration_ms", strconv.FormatInt(m.MedianDurationMs, 10)))
	return cosmos.Events{evt}, nil
}

// NewEventFrostKeysignMetric create a new EventFrostMetric
func NewEventFrostKeysignMetric(txID common.TxID, medianDurationMS int64) *EventFrostKeysignMetric {
	return &EventFrostKeysignMetric{
		TxID:             txID,
		MedianDurationMs: medianDurationMS,
	}
}

// Type  return a string which represent the type of this event
func (m *EventFrostKeysignMetric) Type() string {
	return FROSTKeysignMetricEventType
}

// Events return cosmos sdk events
func (m *EventFrostKeysignMetric) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("txid", m.TxID.String()),
		cosmos.NewAttribute("median_duration_ms", strconv.FormatInt(m.MedianDurationMs, 10)))
	return cosmos.Events{evt}, nil
}

// NewEventPenaltyPoint create a new penalty point event
func NewEventPenaltyPoint(addr cosmos.AccAddress, penaltyPoints int64, reason string) *EventPenaltyPoint {
	return &EventPenaltyPoint{
		NodeAddress:   addr,
		PenaltyPoints: penaltyPoints,
		Reason:        reason,
	}
}

// Type return a string which represent the type of this event
func (m *EventPenaltyPoint) Type() string {
	return PenaltyPointEventType
}

// Events return cosmos sdk events
func (m *EventPenaltyPoint) Events() (cosmos.Events, error) {
	evt := cosmos.NewEvent(m.Type(),
		cosmos.NewAttribute("node_address", m.NodeAddress.String()),
		cosmos.NewAttribute("penalty_points", strconv.FormatInt(m.PenaltyPoints, 10)),
		cosmos.NewAttribute("reason", m.Reason))
	return cosmos.Events{evt}, nil
}

func NewEventSetConfig(key, value string) *EventSetConfig {
	// NewEventSetConfig create a new instance of EventSetConfig
	return &EventSetConfig{
		Key:   key,
		Value: value,
	}
}

// Type return a string which represent the type of this event
func (m *EventSetConfig) Type() string {
	return SetConfigEventType
}

// Events return cosmos sdk events
func (m *EventSetConfig) Events() (cosmos.Events, error) {
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

// NewEventSetNodeConfig create a new instance of EventSetNodeConfig
func NewEventSetNodeConfig(key, value, address string) *EventSetNodeConfig {
	return &EventSetNodeConfig{
		Key:     key,
		Value:   value,
		Address: address,
	}
}

// Type return a string which represent the type of this event
func (m *EventSetNodeConfig) Type() string {
	return SetNodeConfigEventType
}

// Events return cosmos sdk events
func (m *EventSetNodeConfig) Events() (cosmos.Events, error) {
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
func NewEventFailedOutboundRecovery(inTxID common.TxID, coin common.Coin, recoveryType string, destination common.Address) *EventFailedOutboundRecovery {
	return &EventFailedOutboundRecovery{
		InTxID:       inTxID,
		Coin:         coin,
		RecoveryType: recoveryType,
		Destination:  destination,
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
	)
	return cosmos.Events{evt}, nil
}
