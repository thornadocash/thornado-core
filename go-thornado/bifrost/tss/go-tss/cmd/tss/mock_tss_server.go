package main

import (
	"errors"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/blame"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/common"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keygen"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/tss"
	tcommon "github.com/thornadocash/go-thornado/common"
)

type MockTssServer struct {
	failToStart   bool
	failToKeyGen  bool
	failToKeySign bool
}

func (mts *MockTssServer) Start() error {
	if mts.failToStart {
		return errors.New("you ask for it")
	}
	return nil
}

func (mts *MockTssServer) Stop() {
}

func (mts *MockTssServer) GetLocalPeerID() string {
	return conversion.GetRandomPeerID().String()
}

func (mts *MockTssServer) GetKnownPeers() []tss.PeerInfo {
	return []tss.PeerInfo{}
}

func (mts *MockTssServer) Keygen(req keygen.Request) (keygen.Response, error) {
	if mts.failToKeyGen {
		return keygen.Response{}, errors.New("you ask for it")
	}
	return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, conversion.GetRandomPubKey(), "whatever", common.Success, blame.Blame{}), nil
}

func (mts *MockTssServer) KeygenAllAlgo(req keygen.Request) ([]keygen.Response, error) {
	if mts.failToKeyGen {
		return []keygen.Response{{}}, errors.New("you ask for it")
	}
	return []keygen.Response{
		keygen.NewResponse(tcommon.SigningAlgoSecp256k1, conversion.GetRandomPubKey(), "whatever", common.Success, blame.Blame{}),
		keygen.NewResponse(tcommon.SigningAlgoEd25519, conversion.GetRandomPubKey(), "whatever", common.Success, blame.Blame{}),
	}, nil
}

func (mts *MockTssServer) KeySign(req keysign.Request) (keysign.Response, error) {
	if mts.failToKeySign {
		return keysign.Response{}, errors.New("you ask for it")
	}
	newSig := keysign.NewSignature("", "", "", "")
	return keysign.NewResponse([]keysign.Signature{newSig}, common.Success, blame.Blame{}), nil
}
