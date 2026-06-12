package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

const (
	// MinKeysharesBackupEntropy was selected based on a few spot checks of the entropy in
	// encrypted keyshares for mocknet, which were always greater than 7, this is just a
	// sanity check and is safe to set lower.
	MinKeysharesBackupEntropy   float64 = 7
	MinKeysharesBackupSize      int     = 1024
	MinFrostKeysharesBackupSize int     = 256
	MaxKeysharesBackupSize      int     = 256 * 1024
	MaxFrostPubKeys               int     = 100
	MaxFrostChains                int     = 16
	MaxFrostBlames                int     = 100
	MaxFrostBlameDataSize         int     = 4096
	MaxFrostBlameSignatureSize    int     = 256
	MaxFrostTextLength            int     = 256
	MaxFrostSignatureSize         int     = 64
)

// MatchMnemonic will match substrings that look like a 12+ word mnemonic.
var MatchMnemonic = regexp.MustCompile(`([a-zA-Z]+ ){11}[a-zA-Z]+`)

var (
	_ sdk.Msg              = &MsgKeygenVault{}
	_ sdk.HasValidateBasic = &MsgKeygenVault{}
	_ sdk.LegacyMsg        = &MsgKeygenVault{}
)

// NewMsgKeygenVault is a constructor function for MsgKeygenVault
func NewMsgKeygenVault(pks []string, vaultpk common.PubKey, secp256k1Signature, keysharesBackup []byte, keygenType KeygenType, height int64, bl []Blame, chains []string, signer cosmos.AccAddress, keygenTime int64) (*MsgKeygenVault, error) {
	id, err := getFrostID(pks, vaultpk, height, bl)
	if err != nil {
		return nil, fmt.Errorf("fail to get frost id: %w", err)
	}
	return &MsgKeygenVault{
		ID:                 id,
		PubKeys:            pks,
		VaultPubKey:        vaultpk,
		Height:             height,
		KeygenType:         keygenType,
		Blame:              bl,
		Chains:             chains,
		Signer:             signer,
		KeygenTime:         keygenTime,
		KeysharesBackup:    keysharesBackup,
		Secp256K1Signature: secp256k1Signature,
	}, nil
}

// getFrostID
func getFrostID(members []string, vaultPk common.PubKey, height int64, bl []Blame) (string, error) {
	// ensure input pubkeys list is deterministically sorted
	sort.SliceStable(members, func(i, j int) bool {
		return members[i] < members[j]
	})

	pubkeys := make([]string, 0)
	for _, b := range bl {
		for _, node := range b.BlameNodes {
			pubkeys = append(pubkeys, node.Pubkey)
		}
	}
	sort.SliceStable(pubkeys, func(i, j int) bool {
		return pubkeys[i] < pubkeys[j]
	})

	sb := strings.Builder{}
	for _, item := range members {
		sb.WriteString("m:" + item)
	}
	for _, item := range pubkeys {
		sb.WriteString("p:" + item)
	}
	sb.WriteString(vaultPk.String())
	sb.WriteString(fmt.Sprintf("%d", height))
	hash := sha256.New()
	_, err := hash.Write([]byte(sb.String()))
	if err != nil {
		return "", fmt.Errorf("fail to get frost id: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgKeygenVault) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if len(m.ID) == 0 {
		return cosmos.ErrUnknownRequest("ID cannot be blank")
	}
	if len(m.PubKeys) < 2 {
		return cosmos.ErrUnknownRequest("Must have at least 2 pub keys")
	}
	if len(m.PubKeys) > MaxFrostPubKeys {
		return cosmos.ErrUnknownRequest("Must have no more then 100 pub keys")
	}
	if len(m.Blame) > MaxFrostBlames {
		return cosmos.ErrUnknownRequest("too many frost blame records")
	}
	if len(m.Secp256K1Signature) > MaxFrostSignatureSize {
		return cosmos.ErrUnknownRequest("secp256k1 signature too large")
	}
	// Validate blame nodes: all blamed pubkeys must be keygen participants and no duplicates.
	// This prevents a malicious quorum from spamming blame against non-participants.
	pubKeySet := make(map[string]struct{}, len(m.PubKeys))
	for _, pk := range m.PubKeys {
		pubKeySet[pk] = struct{}{}
	}
	seenBlameNodes := make(map[string]struct{})
	for _, b := range m.Blame {
		if len(b.FailReason) > MaxFrostTextLength {
			return cosmos.ErrUnknownRequest("frost blame reason too long")
		}
		if len(b.Round) > MaxFrostTextLength {
			return cosmos.ErrUnknownRequest("frost blame round too long")
		}
		for _, node := range b.BlameNodes {
			if len(node.BlameData) > MaxFrostBlameDataSize {
				return cosmos.ErrUnknownRequest("frost blame data too large")
			}
			if len(node.BlameSignature) > MaxFrostBlameSignatureSize {
				return cosmos.ErrUnknownRequest("frost blame signature too large")
			}
			if _, exists := pubKeySet[node.Pubkey]; !exists {
				return cosmos.ErrUnknownRequest("blame node not in keygen participants")
			}
			if _, seen := seenBlameNodes[node.Pubkey]; seen {
				return cosmos.ErrUnknownRequest("duplicate blame node")
			}
			seenBlameNodes[node.Pubkey] = struct{}{}
		}
	}
	pks := m.GetPubKeys()
	if len(m.PubKeys) != len(pks) {
		return cosmos.ErrUnknownRequest("One or more pubkeys were not valid")
	}
	isSignerInPubKeys := false
	for _, pk := range pks {
		if pk.IsEmpty() {
			return cosmos.ErrUnknownRequest("Pubkey cannot be empty")
		}
		signerAddress, err := pk.GetThorAddress()
		if err != nil {
			return cosmos.ErrUnknownRequest("invalid pub key")
		}
		if signerAddress.Equals(m.Signer) {
			isSignerInPubKeys = true
		}
	}
	if !isSignerInPubKeys {
		return cosmos.ErrUnknownRequest("signer is not part of keygen member")
	}
	// VaultPubKey can't be empty only when keygen success
	if m.IsSuccess() {
		if m.VaultPubKey.IsEmpty() {
			return cosmos.ErrUnknownRequest("Pool pubkey cannot be empty")
		}
	}
	// ensure vault pubkey is a valid bech32 pubkey
	if _, err := common.NewPubKey(m.VaultPubKey.String()); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	chains := m.GetChains()
	if len(chains) != len(m.Chains) {
		return cosmos.ErrUnknownRequest("One or more chains were not valid")
	}
	if len(m.Chains) > MaxFrostChains {
		return cosmos.ErrUnknownRequest("too many frost chains")
	}
	if !chains.Has(common.BTCAsset.Chain) {
		return cosmos.ErrUnknownRequest("must support rune asset chain")
	}
	if len(chains) != len(chains.Distinct()) {
		return cosmos.ErrUnknownRequest("cannot have duplicate chains")
	}

	if len(m.KeysharesBackup) != 0 {
		// sanity check encrypted keyshares do not a mnemonic
		if MatchMnemonic.Match(m.KeysharesBackup) {
			return cosmos.ErrUnknownRequest("invalid keyshares backup")
		}

		// sanity check encrypted keyshares are over 1Kb
		if len(m.KeysharesBackup) < MinKeysharesBackupSize {
			return cosmos.ErrUnknownRequest("invalid keyshares backup")
		}
		if len(m.KeysharesBackup) > MaxKeysharesBackupSize {
			return cosmos.ErrUnknownRequest("keyshares backup too large")
		}

		// sanity check probability distribution of keyshares backup bytes
		entropy := common.Entropy(m.KeysharesBackup)
		// analyze-ignore(float-comparison)
		if entropy < MinKeysharesBackupEntropy {
			return cosmos.ErrUnknownRequest("invalid keyshares backup")
		}
	}

	if len(m.KeysharesBackupEddsa) != 0 {
		if MatchMnemonic.Match(m.KeysharesBackupEddsa) {
			return cosmos.ErrUnknownRequest("invalid eddsa keyshares backup")
		}

		if len(m.KeysharesBackupEddsa) < MinKeysharesBackupSize {
			return cosmos.ErrUnknownRequest("invalid eddsa keyshares backup")
		}
		if len(m.KeysharesBackupEddsa) > MaxKeysharesBackupSize {
			return cosmos.ErrUnknownRequest("eddsa keyshares backup too large")
		}

		entropy := common.Entropy(m.KeysharesBackupEddsa)
		// analyze-ignore(float-comparison)
		if entropy < MinKeysharesBackupEntropy {
			return cosmos.ErrUnknownRequest("invalid eddsa keyshares backup")
		}
	}

	if len(m.KeysharesBackupFrost) != 0 {
		if MatchMnemonic.Match(m.KeysharesBackupFrost) {
			return cosmos.ErrUnknownRequest("invalid frost keyshares backup")
		}

		if len(m.KeysharesBackupFrost) < MinFrostKeysharesBackupSize {
			return cosmos.ErrUnknownRequest("invalid frost keyshares backup")
		}
		if len(m.KeysharesBackupFrost) > MaxKeysharesBackupSize {
			return cosmos.ErrUnknownRequest("frost keyshares backup too large")
		}

		entropy := common.Entropy(m.KeysharesBackupFrost)
		// analyze-ignore(float-comparison)
		if entropy < MinKeysharesBackupEntropy {
			return cosmos.ErrUnknownRequest("invalid frost keyshares backup")
		}
	}

	return nil
}

// IsSuccess when blame is empty , then treat it as success
func (m MsgKeygenVault) IsSuccess() bool {
	return len(m.Blame) == 0
}

func (m MsgKeygenVault) GetChains() common.Chains {
	chains := make(common.Chains, 0)
	for _, c := range m.Chains {
		chain, err := common.NewChain(c)
		if err != nil {
			continue
		}
		chains = append(chains, chain)
	}
	return chains
}

func (m MsgKeygenVault) GetPubKeys() common.PubKeys {
	pubkeys := make(common.PubKeys, 0)
	for _, pk := range m.PubKeys {
		pk, err := common.NewPubKey(pk)
		if err != nil {
			continue
		}
		pubkeys = append(pubkeys, pk)
	}
	return pubkeys
}

// GetSigners defines whose signature is required
func (m *MsgKeygenVault) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgKeygenVaultCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgKeygenVault)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgKeygenVault: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
