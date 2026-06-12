package messages

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
)

// ThornadoFROSTMessageType  represent the message type used in Thornado FROST
type ThornadoFROSTMessageType uint8

const (
	// FROSTKeyGenMsg is a keygen protocol message.
	FROSTKeyGenMsg ThornadoFROSTMessageType = iota
	// FROSTKeySignMsg is a keysign protocol message.
	FROSTKeySignMsg
	// FROSTKeyGenVerMsg is the message we create on top to make sure everyone received the same message
	FROSTKeyGenVerMsg
	// FROSTKeySignVerMsg is the message we create to make sure every party receive the same broadcast message
	FROSTKeySignVerMsg
	// FROSTControlMsg is the message we create to exchange Frost share
	FROSTControlMsg
	// FROSTTaskDone is the message of Frost process notification
	FROSTTaskDone
	// FROSTFrostKeyGenMsg is a FROST keygen protocol message.
	FROSTFrostKeyGenMsg
	// FROSTFrostKeySignMsg is a FROST keysign protocol message.
	FROSTFrostKeySignMsg
	// FROSTFrostKeySignResultMsg is a FROST keysign result message.
	FROSTFrostKeySignResultMsg
	// Unknown is the message indicates the undefined message type
	Unknown
)

// String implement fmt.Stringer
func (msgType ThornadoFROSTMessageType) String() string {
	switch msgType {
	case FROSTKeyGenMsg:
		return "FROSTKeyGenMsg"
	case FROSTKeySignMsg:
		return "FROSTKeySignMsg"
	case FROSTKeyGenVerMsg:
		return "FROSTKeyGenVerMsg"
	case FROSTKeySignVerMsg:
		return "FROSTKeySignVerMsg"
	case FROSTFrostKeyGenMsg:
		return "FROSTFrostKeyGenMsg"
	case FROSTFrostKeySignMsg:
		return "FROSTFrostKeySignMsg"
	case FROSTFrostKeySignResultMsg:
		return "FROSTFrostKeySignResultMsg"
	default:
		return "Unknown"
	}
}

// WrappedMessage is a message with type in it
type WrappedMessage struct {
	MessageType ThornadoFROSTMessageType `json:"message_type"`
	MsgID       string                 `json:"message_id"`
	Payload     []byte                 `json:"payload"`
}

// BroadcastMsgChan is the channel structure for keygen/keysign submit message to p2p network
type BroadcastMsgChan struct {
	WrappedMessage WrappedMessage
	PeersID        []peer.ID
}

// BroadcastConfirmMessage is used to broadcast to all parties what message they receive
type BroadcastConfirmMessage struct {
	P2PID string `json:"P2PID"`
	Key   string `json:"key"`
	Hash  string `json:"hash"`
}

type WireParty struct {
	ID string `json:"id"`
}

type WireRouting struct {
	From WireParty `json:"from"`
}

// WireMessage is the p2p wire payload used by signer coordination.
type WireMessage struct {
	Routing   *WireRouting `json:"routing"`
	RoundInfo string       `json:"round_info"`
	Message   []byte       `json:"message"`
	Sig       []byte       `json:"signature"`
}

// GetCacheKey return the key we used to cache it locally
func (m *WireMessage) GetCacheKey() string {
	if m == nil || m.Routing == nil {
		return fmt.Sprintf("-%s", m.RoundInfo)
	}
	return fmt.Sprintf("%s-%s", m.Routing.From.ID, m.RoundInfo)
}

type FrostControl struct {
	ReqHash     string                 `json:"reqest_hash"`
	ReqKey      string                 `json:"request_key"`
	RequestType ThornadoFROSTMessageType `json:"request_type"`
	Msg         *WireMessage           `json:"message_body"`
}

type FrostTaskNotifier struct {
	TaskDone bool `json:"task_done"`
}
