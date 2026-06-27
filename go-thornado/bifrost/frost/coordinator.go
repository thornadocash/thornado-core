package frost

import "context"

// SessionCoordinator runs distributed FROST keygen and signing sessions.
type SessionCoordinator interface {
	RunKeygen(
		ctx context.Context,
		height int64,
		participants []string,
		localParty string,
		minSigners uint16,
	) (localShare []byte, pubKeyCompressed []byte, err error)
	RunSign(
		ctx context.Context,
		sessionID string,
		height int64,
		participants []string,
		localParty string,
		share []byte,
		msg []byte,
		taprootKeyPath bool,
		scriptRoot []byte,
		childTweak []byte,
		partyLeader string,
	) (signature []byte, err error)
}