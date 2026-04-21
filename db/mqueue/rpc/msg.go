package mqueue_rpc

import (
	"time"

	"github.com/s4wave/spacewave/db/mqueue"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// Msg wraps a MqueueMsg into a mqueue.Message.
type Msg struct {
	*MqueueMsg
}

// NewMsg wraps a Msg into a mqueue.Message.
func NewMsg(msg *MqueueMsg) *Msg {
	return &Msg{MqueueMsg: msg}
}

// ToMqueueMsg converts a Message to a MqueueMsg.
func ToMqueueMsg(msg mqueue.Message) *MqueueMsg {
	if msg == nil {
		return nil
	}
	return &MqueueMsg{
		Id:        msg.GetId(),
		Timestamp: timestamppb.New(msg.GetTimestamp()),
		Data:      msg.GetData(),
	}
}

// GetTimestamp returns the message timestamp.
func (m *Msg) GetTimestamp() time.Time {
	if m == nil || m.Timestamp == nil {
		return time.Time{}
	}
	return m.Timestamp.AsTime()
}

// _ is a type assertion
var _ mqueue.Message = ((*Msg)(nil))
