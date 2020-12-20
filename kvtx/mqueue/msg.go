package kvtx_mqueue

import (
	"time"

	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/timestamp"
)

// mQueueMessage implements a message queue message.
type mQueueMessage struct {
	id        uint64
	wrapper   *MQMessageWrapper
	timestamp time.Time
}

func newMQueueMessage(id uint64, data []byte, ts time.Time) *mQueueMessage {
	tts := timestamp.ToTimestamp(ts)
	return &mQueueMessage{
		id:        id,
		timestamp: ts,
		wrapper: &MQMessageWrapper{
			Data:      data,
			Timestamp: &tts,
		},
	}
}

func newMQueueMessageFromWrapper(id uint64, wrapper *MQMessageWrapper) *mQueueMessage {
	ts := wrapper.GetTimestamp().ToTime()
	return &mQueueMessage{
		id:        id,
		timestamp: ts,
		wrapper:   wrapper,
	}
}

// GetId returns the numeric message identifier.
func (m *mQueueMessage) GetId() uint64 {
	return m.id
}

// GetTimestamp returns the message timestamp.
func (m *mQueueMessage) GetTimestamp() time.Time {
	return m.timestamp
}

// GetData returns the inner message data.
func (m *mQueueMessage) GetData() []byte {
	return m.wrapper.GetData()
}

// _ is a type assertion
var _ mqueue.Message = ((*mQueueMessage)(nil))
