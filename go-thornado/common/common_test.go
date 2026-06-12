package common

import (
	"reflect"
	"testing"

	"github.com/thornadocash/go-thornado/common/cosmos"
	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type CommonSuite struct{}

var _ = Suite(&CommonSuite{})

func (s CommonSuite) TestGetUncappedShare(c *C) {
	part := cosmos.NewUint(149506590)
	total := cosmos.NewUint(50165561086)
	alloc := cosmos.NewUint(50000000)
	share := GetUncappedShare(part, total, alloc)
	c.Assert(share.Equal(cosmos.NewUint(149013)), Equals, true)
}

func (s CommonSuite) TestGetSafeShare(c *C) {
	part := cosmos.NewUint(14950659000000000)
	total := cosmos.NewUint(50165561086)
	alloc := cosmos.NewUint(50000000)
	share := GetSafeShare(part, total, alloc)
	c.Assert(share.Equal(cosmos.NewUint(50000000)), Equals, true)
}

func (s CommonSuite) TestObservedTxSignablePayloadCoversWrapperAndInbound(c *C) {
	limit := cosmos.NewUint(1000)
	base := ObservedTx{
		Tx: Tx{
			ID:          "tx1",
			Chain:       BTCChain,
			FromAddress: "from",
			ToAddress:   "to",
		},
		Status:                Status_done,
		OutHashes:             []string{"out1"},
		BlockHeight:           100,
		Signers:               []string{"signer1"},
		ObservedPubKey:        "pubkey",
		KeysignMs:             250,
		FinaliseHeight:        106,
		Aggregator:            "agg",
		AggregatorTarget:      "target",
		AggregatorTargetLimit: &limit,
	}
	basePayload, err := base.GetSignablePayloadWithInbound(false)
	c.Assert(err, IsNil)

	covered := map[string]func(*ObservedTx){
		"Tx":                    func(o *ObservedTx) { o.Tx.ID = "different" },
		"Status":                func(o *ObservedTx) { o.Status = Status_reverted },
		"OutHashes":             func(o *ObservedTx) { o.OutHashes = []string{"changed"} },
		"BlockHeight":           func(o *ObservedTx) { o.BlockHeight = 999 },
		"Signers":               func(o *ObservedTx) { o.Signers = []string{"changed"} },
		"ObservedPubKey":        func(o *ObservedTx) { o.ObservedPubKey = "changed" },
		"FinaliseHeight":        func(o *ObservedTx) { o.FinaliseHeight = base.BlockHeight },
		"Aggregator":            func(o *ObservedTx) { o.Aggregator = "changed" },
		"AggregatorTarget":      func(o *ObservedTx) { o.AggregatorTarget = "changed" },
		"AggregatorTargetLimit": func(o *ObservedTx) { l := cosmos.NewUint(2000); o.AggregatorTargetLimit = &l },
	}
	for name, mutate := range covered {
		mutated := base
		mutate(&mutated)
		got, err := mutated.GetSignablePayloadWithInbound(false)
		c.Assert(err, IsNil, Commentf("field %s", name))
		c.Assert(got, Not(DeepEquals), basePayload, Commentf("field %s must be signed", name))
	}

	excluded := map[string]func(*ObservedTx){
		"KeysignMs": func(o *ObservedTx) { o.KeysignMs = 9999 },
	}
	for name, mutate := range excluded {
		mutated := base
		mutate(&mutated)
		got, err := mutated.GetSignablePayloadWithInbound(false)
		c.Assert(err, IsNil, Commentf("field %s", name))
		c.Assert(got, DeepEquals, basePayload, Commentf("field %s must be excluded", name))
	}

	inboundPayload, err := base.GetSignablePayloadWithInbound(true)
	c.Assert(err, IsNil)
	c.Assert(inboundPayload, Not(DeepEquals), basePayload)

	attestTx := AttestTx{ObsTx: base, Inbound: true}
	got, err := attestTx.GetSignablePayload()
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, inboundPayload)

	accountedFor := make(map[string]bool, len(covered)+len(excluded))
	for k := range covered {
		accountedFor[k] = true
	}
	for k := range excluded {
		accountedFor[k] = true
	}
	txType := reflect.TypeOf(base)
	for i := 0; i < txType.NumField(); i++ {
		field := txType.Field(i).Name
		c.Assert(accountedFor[field], Equals, true, Commentf("ObservedTx field %q must be classified as signed or deliberately excluded", field))
	}
}
