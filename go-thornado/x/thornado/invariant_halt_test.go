package thornado

import (
	"fmt"
	"testing"

	"cosmossdk.io/log"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

type invariantHaltKeeperFake struct {
	routes  []common.InvariantRoute
	configs map[string]int64
}

func (k *invariantHaltKeeperFake) InvariantRoutes() []common.InvariantRoute {
	return k.routes
}

func (k *invariantHaltKeeperFake) GetConfig(_ cosmos.Context, key string) (int64, error) {
	return k.configs[key], nil
}

func (k *invariantHaltKeeperFake) GetConfigInt64(_ cosmos.Context, key constants.ConfigName) int64 {
	return k.configs[key.String()]
}

func (k *invariantHaltKeeperFake) SetConfig(_ cosmos.Context, key string, value int64) {
	k.configs[key] = value
}

type invariantHaltEventMgr struct {
	events int
}

func (m *invariantHaltEventMgr) EmitEvent(cosmos.Context, EmitEventItem) error {
	m.events++
	return nil
}

func (m *invariantHaltEventMgr) EmitGasEvent(cosmos.Context, *EventGas) error {
	return nil
}

func (m *invariantHaltEventMgr) EmitFeeEvent(cosmos.Context, *EventFee) error {
	return nil
}

func TestHaltOnBrokenVaultBackingInvariant(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(42).WithLogger(log.NewNopLogger())
	k := &invariantHaltKeeperFake{
		routes: []common.InvariantRoute{
			common.NewInvariantRoute("vault_backing", func(cosmos.Context) ([]string, bool) {
				return []string{"controlled=0 liabilities=1"}, true
			}),
		},
		configs: make(map[string]int64),
	}
	events := &invariantHaltEventMgr{}

	if !haltOnBrokenVaultBackingInvariant(ctx, k, events) {
		t.Fatal("expected broken vault backing invariant to halt")
	}

	signingKey := fmt.Sprintf(constants.ConfigTemplateHaltSigning, common.BTCChain)
	if got := k.configs[constants.Halt_SolvencyCheck.String()]; got != 42 {
		t.Fatalf("expected %s=42, got %d", constants.Halt_SolvencyCheck.String(), got)
	}
	if got := k.configs[signingKey]; got != 42 {
		t.Fatalf("expected %s=42, got %d", signingKey, got)
	}
	if events.events != 2 {
		t.Fatalf("expected two config events, got %d", events.events)
	}

	if !haltOnBrokenVaultBackingInvariant(ctx, k, events) {
		t.Fatal("expected broken vault backing invariant to stay halted")
	}
	if events.events != 2 {
		t.Fatalf("expected no duplicate config events, got %d", events.events)
	}
}

func TestHaltOnHealthyVaultBackingInvariant(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(42).WithLogger(log.NewNopLogger())
	k := &invariantHaltKeeperFake{
		routes: []common.InvariantRoute{
			common.NewInvariantRoute("vault_backing", func(cosmos.Context) ([]string, bool) {
				return nil, false
			}),
		},
		configs: make(map[string]int64),
	}

	if haltOnBrokenVaultBackingInvariant(ctx, k, &invariantHaltEventMgr{}) {
		t.Fatal("healthy vault backing invariant should not halt")
	}
	if len(k.configs) != 0 {
		t.Fatalf("expected no config writes, got %v", k.configs)
	}
}

func TestSolvencyRecoveryClearsMatchingSigningHalt(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	signingKey := fmt.Sprintf(constants.ConfigTemplateHaltSigning, common.BTCChain)
	k := &invariantHaltKeeperFake{
		configs: map[string]int64{
			constants.Halt_SolvencyCheck.String(): 42,
			signingKey:                            42,
		},
	}
	events := &invariantHaltEventMgr{}

	k.SetConfig(ctx, constants.Halt_SolvencyCheck.String(), 0)
	clearMatchingSolvencySigningHalt(ctx, k, events, common.BTCChain, 42)

	if got := k.configs[constants.Halt_SolvencyCheck.String()]; got != 0 {
		t.Fatalf("expected %s=0, got %d", constants.Halt_SolvencyCheck.String(), got)
	}
	if got := k.configs[signingKey]; got != 0 {
		t.Fatalf("expected %s=0, got %d", signingKey, got)
	}
	if events.events != 1 {
		t.Fatalf("expected one signing halt clear event, got %d", events.events)
	}
}

func TestSolvencyRecoveryKeepsUnrelatedSigningHalt(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	signingKey := fmt.Sprintf(constants.ConfigTemplateHaltSigning, common.BTCChain)
	k := &invariantHaltKeeperFake{
		configs: map[string]int64{
			constants.Halt_SolvencyCheck.String(): 42,
			signingKey:                            99,
		},
	}
	events := &invariantHaltEventMgr{}

	k.SetConfig(ctx, constants.Halt_SolvencyCheck.String(), 0)
	clearMatchingSolvencySigningHalt(ctx, k, events, common.BTCChain, 42)

	if got := k.configs[signingKey]; got != 99 {
		t.Fatalf("expected unrelated %s=99 to remain, got %d", signingKey, got)
	}
	if events.events != 0 {
		t.Fatalf("expected no signing halt clear event, got %d", events.events)
	}
}

func TestShouldSkipSolvencyHaltAction(t *testing.T) {
	tests := []struct {
		name       string
		haltHeight int64
		blockHeight int64
		want       bool
	}{
		{name: "unset", haltHeight: 0, blockHeight: 100, want: false},
		{name: "stale", haltHeight: 42, blockHeight: 100, want: false},
		{name: "current", haltHeight: 100, blockHeight: 100, want: true},
		{name: "future", haltHeight: 101, blockHeight: 100, want: true},
		{name: "indefinite", haltHeight: 1, blockHeight: 100, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipSolvencyHaltAction(tt.haltHeight, tt.blockHeight); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
