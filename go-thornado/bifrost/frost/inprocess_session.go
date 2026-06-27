package frost

import (
	"context"
	"encoding/hex"
	"fmt"

	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"
)

// InProcessSessionCoordinator runs FROST session rounds in-process. Tests only.
type InProcessSessionCoordinator struct{}

func (sc *InProcessSessionCoordinator) RunKeygen(
	_ context.Context,
	_ int64,
	participants []string,
	localParty string,
	minSigners uint16,
) (localShare []byte, pubKeyCompressed []byte, err error) {
	participants = sortedParticipants(participants)
	shares, err := runInProcessKeygen(participants, minSigners)
	if err != nil {
		return nil, nil, err
	}
	share, ok := shares[localParty]
	if !ok {
		return nil, nil, fmt.Errorf("missing local keyshare for %s", localParty)
	}
	decoded, err := frostsessions.DecodeKeyshare(share)
	if err != nil {
		return nil, nil, err
	}
	pubKeyCompressed, err = hex.DecodeString(decoded.PublicKeyCompressed)
	if err != nil {
		return nil, nil, err
	}
	return share, pubKeyCompressed, nil
}

func (sc *InProcessSessionCoordinator) RunSign(
	_ context.Context,
	_ string,
	_ int64,
	participants []string,
	localParty string,
	share []byte,
	msg []byte,
	taprootKeyPath bool,
	scriptRoot []byte,
	childTweak []byte,
	_ string,
) ([]byte, error) {
	participants = sortedParticipants(participants)
	return runInProcessSign(participants, map[string][]byte{localParty: share}, localParty, msg, taprootKeyPath, scriptRoot, childTweak)
}

// RunInProcessKeygenAll materializes all participant shares for tests.
func RunInProcessKeygenAll(participants []string, minSigners uint16) (map[string][]byte, error) {
	return runInProcessKeygen(participants, minSigners)
}

// RunInProcessSign completes a FROST signing session in-process for tests.
func RunInProcessSign(participants []string, shares map[string][]byte, localParty string, msg []byte, taprootKeyPath bool, scriptRoot, childTweak []byte) ([]byte, error) {
	return runInProcessSign(participants, shares, localParty, msg, taprootKeyPath, scriptRoot, childTweak)
}

func runInProcessKeygen(participants []string, minSigners uint16) (map[string][]byte, error) {
	handles := make(map[string]frostsessions.Handle, len(participants))
	for _, p := range participants {
		h, err := frostsessions.KeygenSessionNew(participants, p, minSigners)
		if err != nil {
			return nil, err
		}
		defer func(handle frostsessions.Handle) { _ = frostsessions.SessionFree(handle) }(h)
		handles[p] = h
	}
	if err := runInProcessSessions(participants, handles); err != nil {
		return nil, err
	}
	shares := make(map[string][]byte, len(participants))
	for p, h := range handles {
		share, err := frostsessions.SessionFinish(h)
		if err != nil {
			return nil, err
		}
		shares[p] = share
	}
	return shares, nil
}

func runInProcessSign(participants []string, shares map[string][]byte, localParty string, msg []byte, taprootKeyPath bool, scriptRoot, childTweak []byte) ([]byte, error) {
	handles := make(map[string]frostsessions.Handle, len(participants))
	for _, p := range participants {
		h, err := frostsessions.SignSessionNewWithTweak(participants, p, shares[p], msg, taprootKeyPath, scriptRoot, childTweak)
		if err != nil {
			return nil, err
		}
		defer func(handle frostsessions.Handle) { _ = frostsessions.SessionFree(handle) }(h)
		handles[p] = h
	}
	if err := runInProcessSessions(participants, handles); err != nil {
		return nil, err
	}
	return frostsessions.SessionFinish(handles[localParty])
}

func runInProcessSessions(participants []string, handles map[string]frostsessions.Handle) error {
	for i := 0; i < 1000; i++ {
		progress := false
		for from, h := range handles {
			for {
				msg, err := frostsessions.SessionOutputMessage(h)
				if err != nil {
					return err
				}
				if len(msg) == 0 {
					break
				}
				progress = true
				for idx := 0; idx < len(participants); idx++ {
					to, err := frostsessions.SessionMessageReceiver(h, msg, idx)
					if err != nil {
						return err
					}
					if to == "" {
						break
					}
					target, ok := handles[to]
					if !ok {
						return fmt.Errorf("unknown receiver %q from %q", to, from)
					}
					if _, err := frostsessions.SessionInputMessage(target, msg); err != nil {
						return err
					}
				}
			}
		}
		if !progress {
			for _, h := range handles {
				if _, err := frostsessions.SessionFinish(h); err != nil {
					goto keepGoing
				}
			}
			return nil
		}
	keepGoing:
	}
	return fmt.Errorf("frost sessions did not finish")
}