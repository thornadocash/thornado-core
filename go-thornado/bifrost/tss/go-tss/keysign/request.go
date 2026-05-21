package keysign

import "github.com/thornadocash/go-thornado/common"

// Request request to sign a message
type Request struct {
	PoolPubKey    string   `json:"pool_pub_key"` // pub key of the pool that we would like to send this message from
	Messages      []string `json:"messages"`     // base64 encoded message to be signed
	SignerPubKeys []string `json:"signer_pub_keys"`
	PathIndexes   []uint64 `json:"path_indexes,omitempty"`
	BlockHeight   int64    `json:"block_height"`
	Version       string   `json:"tss_version"`
	Algo          string   `json:"algo"`
}

func NewRequestWithPathIndexes(pk string, algo common.SigningAlgo, msgs []string, pathIndexes []uint64, blockHeight int64, signers []string, version string) Request {
	req := NewRequest(pk, algo, msgs, blockHeight, signers, version)
	req.PathIndexes = pathIndexes
	return req
}

func NewRequest(pk string, algo common.SigningAlgo, msgs []string, blockHeight int64, signers []string, version string) Request {
	return Request{
		PoolPubKey:    pk,
		Algo:          string(algo),
		Messages:      msgs,
		SignerPubKeys: signers,
		BlockHeight:   blockHeight,
		Version:       version,
	}
}
