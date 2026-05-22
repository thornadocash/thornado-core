package messages

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
)

// ThornadoTSSMessageType  represent the message type used in Thornado TSS
type ThornadoTSSMessageType uint8

const (
	// TSSKeyGenMsg is a keygen protocol message.
	TSSKeyGenMsg ThornadoTSSMessageType = iota
	// TSSKeySignMsg is a keysign protocol message.
	TSSKeySignMsg
	// TSSKeyGenVerMsg is the message we create on top to make sure everyone received the same message
	TSSKeyGenVerMsg
	// TSSKeySignVerMsg is the message we create to make sure every party receive the same broadcast message
	TSSKeySignVerMsg
	// TSSControlMsg is the message we create to exchange Tss share
	TSSControlMsg
	// TSSTaskDone is the message of Tss process notification
	TSSTaskDone
	// TSSFrostKeyGenMsg is a FROST keygen protocol message.
	TSSFrostKeyGenMsg
	// TSSFrostKeySignMsg is a FROST keysign protocol message.
	TSSFrostKeySignMsg
	// TSSFrostKeySignResultMsg is a FROST keysign result message.
	TSSFrostKeySignResultMsg
	// Unknown is the message indicates the undefined message type
	Unknown
)

// String implement fmt.Stringer
func (msgType ThornadoTSSMessageType) String() string {
	switch msgType {
	case TSSKeyGenMsg:
		return "TSSKeyGenMsg"
	case TSSKeySignMsg:
		return "TSSKeySignMsg"
	case TSSKeyGenVerMsg:
		return "TSSKeyGenVerMsg"
	case TSSKeySignVerMsg:
		return "TSSKeySignVerMsg"
	case TSSFrostKeyGenMsg:
		return "TSSFrostKeyGenMsg"
	case TSSFrostKeySignMsg:
		return "TSSFrostKeySignMsg"
	case TSSFrostKeySignResultMsg:
		return "TSSFrostKeySignResultMsg"
	default:
		return "Unknown"
	}
}

// WrappedMessage is a message with type in it
type WrappedMessage struct {
	MessageType ThornadoTSSMessageType `json:"message_type"`
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

type TssControl struct {
	ReqHash     string                 `json:"reqest_hash"`
	ReqKey      string                 `json:"request_key"`
	RequestType ThornadoTSSMessageType `json:"request_type"`
	Msg         *WireMessage           `json:"message_body"`
}

type TssTaskNotifier struct {
	TaskDone bool `json:"task_done"`
}
