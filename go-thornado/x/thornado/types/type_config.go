package types

import (
	"strings"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (m NodeConfigs) Has(key string, acc cosmos.AccAddress) bool {
	for _, mim := range m.Configs {
		if mim.Signer.Equals(acc) && strings.EqualFold(mim.Key, key) {
			return true
		}
	}
	return false
}

func (m NodeConfigs) Get(key string, acc cosmos.AccAddress) (int64, bool) {
	for _, mim := range m.Configs {
		if mim.Signer.Equals(acc) && strings.EqualFold(mim.Key, key) {
			return mim.Value, true
		}
	}
	return 0, false
}

func (m *NodeConfigs) Set(key string, val int64, acc cosmos.AccAddress) {
	for i, mim := range m.Configs {
		if mim.Signer.Equals(acc) && strings.EqualFold(mim.Key, key) {
			m.Configs[i].Value = val
			return
		}
	}
	m.Configs = append(m.Configs, NodeConfig{
		Key:    key,
		Value:  val,
		Signer: acc,
	})
}

func (m *NodeConfigs) Delete(key string, acc cosmos.AccAddress) {
	for i, mim := range m.Configs {
		if mim.Signer.Equals(acc) && strings.EqualFold(mim.Key, key) {
			m.Configs = append(m.Configs[:i], m.Configs[i+1:]...)
			return
		}
	}
}

func (m NodeConfigs) countActive(key string, active []cosmos.AccAddress, maj func(_, _ int) bool) (int64, bool) {
	counter := make(map[int64]int) // count how many votes are for each value
	voted := make(map[string]bool) // track signers that have already voted
	for _, config := range m.Configs {
		// skip mismatching keys
		if !strings.EqualFold(config.Key, key) {
			continue
		}

		// skip signers we've already seend (no duplicates allowed)
		if v, ok := voted[config.Signer.String()]; v && ok {
			continue
		}

		for _, acc := range active {
			// skip if not the config's signer
			if !acc.Equals(config.Signer) {
				continue
			}

			voted[config.Signer.String()] = true // mark signer as voted
			if _, ok := counter[config.Value]; !ok {
				counter[config.Value] = 0
			}
			counter[config.Value]++
			break // Having confirmed the config's signer is active, go to the next config.
		}
	}

	// analyze-ignore(map-iteration)
	for val, count := range counter {
		if maj(count, len(active)) {
			return val, true
		}
	}

	return 0, false
}

func (m NodeConfigs) HasSuperMajority(key string, nas []cosmos.AccAddress) (int64, bool) {
	return m.countActive(key, nas, HasSuperMajority)
}

func (m NodeConfigs) HasSimpleMajority(key string, nas []cosmos.AccAddress) (int64, bool) {
	return m.countActive(key, nas, HasSimpleMajority)
}

func (m NodeConfigs) HasMinority(key string, nas []cosmos.AccAddress) (int64, bool) {
	// NOT IMPLEMENTED
	// Minotirty is a bit tricky, because a set can have multiple minorities, which can result in a potential consensus failure
	return 0, false
}

// ValueOfEconomic - fetches the value of a given config based on 2/3rds consensus
func (m NodeConfigs) ValueOfEconomic(key string, active []cosmos.AccAddress) int64 {
	voteCount := make(map[int64]int)
	hasVoted := make(map[string]bool)
	totalValidVotes := 0

	for _, config := range m.Configs {
		if !strings.EqualFold(config.Key, key) {
			continue
		}

		if hasVoted[config.Signer.String()] {
			continue // Skip this vote since the node already voted
		}

		// Ensure that the vote is only from active nodes
		for _, addr := range active {
			if addr.Equals(config.Signer) {
				voteCount[config.Value]++
				totalValidVotes++
				hasVoted[config.Signer.String()] = true
				break
			}
		}
	}

	mostVotedValue := int64(-1)
	maxVotes := 0
	// analyze-ignore(map-iteration)
	for value, count := range voteCount {
		if count > maxVotes {
			mostVotedValue = value
			maxVotes = count
		}
	}

	// Check if maxVotes is at least two-thirds of totalValidVotes using integer arithmetic
	if 3*maxVotes < 2*len(active) {
		return -1
	}

	return mostVotedValue
}

// ValueOfOperational - fetches the value of a given config based most votes (above min vote)
func (m NodeConfigs) ValueOfOperational(key string, minVotes int64, active []cosmos.AccAddress) int64 {
	voteCount := make(map[int64]int)
	hasVoted := make(map[string]bool)

	for _, config := range m.Configs {
		if !strings.EqualFold(config.Key, key) {
			continue
		}

		if hasVoted[config.Signer.String()] {
			continue // Skip this vote since the node already voted
		}

		// Ensure that the vote is only from active nodes
		for _, addr := range active {
			if addr.Equals(config.Signer) {
				voteCount[config.Value]++
				hasVoted[config.Signer.String()] = true
				break
			}
		}
	}

	mostVotedValue := int64(-1)
	maxVotes := 0
	tie := false
	// analyze-ignore(map-iteration)
	for value, count := range voteCount {
		if count > maxVotes {
			mostVotedValue = value
			maxVotes = count
			tie = false
		} else if count == maxVotes {
			tie = true
		}
	}

	if tie || int64(maxVotes) < minVotes {
		return -1
	}

	return mostVotedValue
}
