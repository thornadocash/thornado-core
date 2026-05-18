//go:build !mocknet
// +build !mocknet

package config

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

const (
	rpcPort      = 27147
	p2pPort      = 27146
	ebifrostPort = 50051
)

func getSeedAddrs() (addrs []string) {
	if config.Thornode.SeedNodesEndpoint == "" {
		log.Warn().Msg("no seed nodes endpoint provided")
		return
	}

	// get nodes
	res, err := http.Get(config.Thornode.SeedNodesEndpoint)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get thorchain nodes")
	}

	// parse nodes
	var nodes []types.NodeAccount
	if err = json.NewDecoder(res.Body).Decode(&nodes); err != nil {
		log.Fatal().Err(err).Msg("failed to decode thorchain nodes")
	}
	res.Body.Close()

	// include active nodes with an ip address
	var seeds []string
	for _, node := range nodes {
		if node.Status != types.NodeStatus_Active {
			continue
		}
		if node.IPAddress == "" || node.IPAddress == "0.0.0.0" {
			continue
		}
		seeds = append(seeds, node.IPAddress)
	}

	// randomly shuffle seeds
	rand.Shuffle(len(seeds), func(i, j int) {
		seeds[i], seeds[j] = seeds[j], seeds[i]
	})

	log.Info().Msgf("found %d thorchain seeds", len(seeds))

	return seeds
}

func assertBifrostHasSeeds() {
	if config.Thornode.SeedNodesEndpoint == "" {
		log.Warn().Msg("no seed nodes endpoint provided, skipping seed file check")
		return
	}

	// fail if seed file is missing or empty since bifrost will hang
	seedPath := os.ExpandEnv("$HOME/.thornode/address_book.seed")
	fi, err := os.Stat(seedPath)
	if os.IsNotExist(err) {
		log.Fatal().Msg("no seed file found")
	}
	if err != nil {
		log.Fatal().Err(err).Msg("failed to stat seed file")
	}
	if fi.Size() == 0 {
		log.Fatal().Msg("seed file is empty")
	}
}
