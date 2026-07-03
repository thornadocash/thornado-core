package types

// StoreMigrateVotes is the per-key set of node votes for a state-correction
// migration, JSON-stored (like shielder records) so no protobuf type is needed.
type StoreMigrateVotes struct {
	Votes map[string]string `json:"votes"` // signer bech32 -> proposed value
}
