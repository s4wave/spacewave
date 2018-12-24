package kvtx

import (
	"time"

	"github.com/aperturerobotics/hydra/store/mqueue"
)

// mQueueMessage implements a message queue message.
type mQueueMessage struct {
	id        uint64
	data      []byte
	timestamp time.Time
}

func newMQueueMessage(id uint64, data []byte, timestamp time.Time) *mQueueMessage {
	return &mQueueMessage{id: id, data: data, timestamp: timestamp}
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
	return m.data
}

// _ is a type assertion
var _ mqueue.Message = ((*mQueueMessage)(nil))
