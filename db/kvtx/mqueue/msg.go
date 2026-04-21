package kvtx_mqueue

import (
	"time"

	"github.com/s4wave/spacewave/db/mqueue"
)

// mQueueMessage implements a message queue message.
type mQueueMessage struct {
	id        uint64
	wrapper   *MQMessageWrapper
	timestamp time.Time
}

func newMQueueMessageFromWrapper(id uint64, wrapper *MQMessageWrapper) *mQueueMessage {
	ts := wrapper.GetTimestamp().AsTime()
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
