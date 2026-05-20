package keysign

import (
	bc "github.com/binance-chain/tss-lib/common"

	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/common"
)

type TssKeySign interface {
	GetTssKeySignChannels() chan *p2p.Message
	GetTssCommonStruct() *common.TssCommon
	SignMessage(msgToSign [][]byte, localStateItem storage.KeygenLocalState, parties []string) ([]*bc.SignatureData, error)
}
