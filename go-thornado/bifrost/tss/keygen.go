package tss

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keygen"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/tss"

	"github.com/thornadocash/go-thornado/bifrost/thorclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

// KeyGen is
type KeyGen struct {
	keys           *thorclient.Keys
	logger         zerolog.Logger
	client         *http.Client
	server         *tss.TssServer
	bridge         thorclient.ThorchainBridge
	currentVersion semver.Version
	lastCheck      time.Time
}

// NewTssKeyGen create a new instance of TssKeyGen which will look after TSS key stuff
func NewTssKeyGen(keys *thorclient.Keys, server *tss.TssServer, bridge thorclient.ThorchainBridge) (*KeyGen, error) {
	if keys == nil {
		return nil, fmt.Errorf("keys is nil")
	}
	return &KeyGen{
		keys:   keys,
		logger: log.With().Str("module", "tss_keygen").Logger(),
		client: &http.Client{
			Timeout: time.Second * 130,
		},
		server: server,
		bridge: bridge,
	}, nil
}

func (kg *KeyGen) getVersion() semver.Version {
	requestTime := time.Now()
	// analyze-ignore(float-comparison)
	if !kg.currentVersion.Equals(semver.Version{}) && requestTime.Sub(kg.lastCheck).Seconds() < constants.ThornadoBlockTime.Seconds() {
		return kg.currentVersion
	}
	version, err := kg.bridge.GetThorchainVersion()
	if err != nil {
		kg.logger.Err(err).Msg("fail to get current thorchain version")
		return kg.currentVersion
	}
	kg.currentVersion = version
	kg.lastCheck = requestTime
	return kg.currentVersion
}

func (kg *KeyGen) GenerateNewKey(keygenBlockHeight int64, pKeys common.PubKeys, chains common.Chains) (pk common.PubKeySet, blame []types.Blame, err error) {
	// No need to do key gen
	if len(pKeys) == 0 {
		return common.EmptyPubKeySet, nil, nil
	}

	// add some logging
	defer func() {
		if len(blame) == 0 {
			kg.logger.Info().Int64("height", keygenBlockHeight).Str("pubkey", pk.String()).Msg("tss keygen results success")
		} else {
			for _, b := range blame {
				blames := make([]string, len(b.BlameNodes))
				for i := range b.BlameNodes {
					var pk common.PubKey
					pk, err = common.NewPubKey(b.BlameNodes[i].Pubkey)
					if err != nil {
						kg.logger.Error().Err(err).Int64("height", keygenBlockHeight).Str("pubkey", b.BlameNodes[i].Pubkey).Msg("tss keygen results error")
						continue
					}
					var acc cosmos.AccAddress
					acc, err = pk.GetThorAddress()
					if err != nil {
						kg.logger.Error().Err(err).Int64("height", keygenBlockHeight).Str("pubkey", pk.String()).Msg("tss keygen results error")
						continue
					}
					blames[i] = acc.String()
				}
				sort.Strings(blames)
				kg.logger.Info().Int64("height", keygenBlockHeight).Str("pubkey", pk.String()).Str("round", b.Round).Str("blames", strings.Join(blames, ", ")).Str("reason", b.FailReason).Msg("tss keygen results blame")
			}
		}
	}()

	var keys []string
	for _, item := range pKeys {
		keys = append(keys, item.String())
	}
	keyGenReq := keygen.Request{
		Keys: keys,
	}
	enableFrost, err := kg.bridge.GetMimir(constants.MimirKeyEnableFrostBTC)
	if err != nil {
		kg.logger.Err(err).Msg("fail to get EnableFrostBTC mimir")
	} else if enableFrost == 1 && frostKeygenChains(chains) {
		keyGenReq.Engine = storage.SigningEngineFrost
	}
	currentVersion := kg.getVersion()
	keyGenReq.Version = currentVersion.String()

	// Use the churn try's block to choose the same leader for every node in an Asgard,
	// since a successful keygen requires every node in the Asgard to take part.
	keyGenReq.BlockHeight = keygenBlockHeight

	ch := make(chan bool, 1)
	defer close(ch)
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()

	var responses []keygen.Response
	go func() {
		responses, err = kg.server.KeygenAllAlgo(keyGenReq)
		ch <- true
	}()

	select {
	case <-ch:
		// do nothing
	case <-timer.C:
		panic("tss keygen timeout")
	}

	// Handle KeygenAllAlgo error first, before processing individual responses
	if err != nil {
		// Create blame from the error or use the first response's blame if available
		var b types.Blame
		if len(responses) > 0 && responses[0].Blame.AlreadyBlame() {
			// Use the blame from the first response if available
			b = types.Blame{
				FailReason: responses[0].Blame.FailReason,
				IsUnicast:  responses[0].Blame.IsUnicast,
				Round:      types.CanonicalTssRound(responses[0].Blame.Round, responses[0].Algo),
				BlameNodes: make([]types.Node, len(responses[0].Blame.BlameNodes)),
			}
			for i, n := range responses[0].Blame.BlameNodes {
				b.BlameNodes[i].Pubkey = n.Pubkey
				b.BlameNodes[i].BlameData = n.BlameData
				b.BlameNodes[i].BlameSignature = n.BlameSignature
			}
		} else {
			// Create a generic blame with the error message
			b.FailReason = err.Error()
		}
		blame = append(blame, b)
		return common.EmptyPubKeySet, blame, fmt.Errorf("fail to keygen: %w", err)
	}

	// Process individual response blames (for cases where KeygenAllAlgo succeeded but individual algos had issues)
	for _, resp := range responses {
		// copy blame to our own struct
		b := types.Blame{
			FailReason: resp.Blame.FailReason,
			IsUnicast:  resp.Blame.IsUnicast,
			Round:      types.CanonicalTssRound(resp.Blame.Round, resp.Algo),
			BlameNodes: make([]types.Node, len(resp.Blame.BlameNodes)),
		}
		for i, n := range resp.Blame.BlameNodes {
			b.BlameNodes[i].Pubkey = n.Pubkey
			b.BlameNodes[i].BlameData = n.BlameData
			b.BlameNodes[i].BlameSignature = n.BlameSignature
		}

		// Only add blame if it contains actual blame information
		if !b.IsEmpty() {
			blame = append(blame, b)
		}
	}

	// If there were any individual response blames, return error
	if len(blame) > 0 {
		blame = dedupeKeygenBlameNodes(blame)
		return common.EmptyPubKeySet, blame, fmt.Errorf("fail to keygen: individual algorithm failures")
	}

	// Extract public keys from successful responses
	var ecdsaPubKey common.PubKey
	var eddsaPubKey common.PubKey
	for _, resp := range responses {
		switch resp.Algo {
		case common.SigningAlgoSecp256k1:
			var err error
			ecdsaPubKey, err = common.NewPubKey(resp.PubKey)
			if err != nil {
				return common.EmptyPubKeySet, blame, fmt.Errorf("fail to create ECDSA PubKey: %w", err)
			}
		case common.SigningAlgoEd25519:
			var err error
			eddsaPubKey, err = common.NewPubKey(resp.PubKey)
			if err != nil {
				return common.EmptyPubKeySet, blame, fmt.Errorf("fail to create EDDSA PubKey: %w", err)
			}
		}
	}

	// Ensure both key types were generated
	if ecdsaPubKey.IsEmpty() {
		return common.EmptyPubKeySet, blame, fmt.Errorf("ECDSA PubKey not generated")
	}
	if eddsaPubKey.IsEmpty() {
		return common.EmptyPubKeySet, blame, fmt.Errorf("EDDSA PubKey not generated")
	}

	return common.NewPubKeySet(ecdsaPubKey, eddsaPubKey), blame, nil
}

func dedupeKeygenBlameNodes(blames []types.Blame) []types.Blame {
	seen := make(map[string]struct{})
	result := make([]types.Blame, 0, len(blames))
	for _, b := range blames {
		nodes := make([]types.Node, 0, len(b.BlameNodes))
		for _, n := range b.BlameNodes {
			if _, ok := seen[n.Pubkey]; ok {
				continue
			}
			seen[n.Pubkey] = struct{}{}
			nodes = append(nodes, n)
		}
		b.BlameNodes = nodes
		if !b.IsEmpty() {
			result = append(result, b)
		}
	}
	return result
}

func frostKeygenChains(chains common.Chains) bool {
	if !chains.Has(common.BTCChain) {
		return false
	}
	for _, chain := range chains.Distinct() {
		if chain.IsEmpty() || chain == common.Thornado || chain == common.BTCChain {
			continue
		}
		return false
	}
	return true
}
