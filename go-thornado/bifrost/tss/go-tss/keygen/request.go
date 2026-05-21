package keygen

import "github.com/thornadocash/go-thornado/common"

// Request request to do keygen
type Request struct {
	Keys        []string           `json:"keys"`
	BlockHeight int64              `json:"block_height"`
	Version     string             `json:"tss_version"`
	Algo        common.SigningAlgo `json:"algo,omitempty"`
	Engine      string             `json:"engine,omitempty"`
}

// NewRequest creeate a new instance of keygen.Request
func NewRequest(keys []string, blockHeight int64, version string, algo common.SigningAlgo) Request {
	return Request{
		Keys:        keys,
		BlockHeight: blockHeight,
		Version:     version,
		Algo:        algo,
	}
}
