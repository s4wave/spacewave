package object_mqueue

import (
	"encoding/binary"
	"strconv"
	"time"

	// "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/timestamp"
	"github.com/golang/protobuf/proto"
)

// mQueue implements a Hydra Object-Store message queue.
// head key: points to next msg to peek
// tail key: points to the next message ID (after last pushed)
type mQueue struct {
	store object.ObjectStore
}

// binaryOrder is the binary order used.
var binaryOrder = binary.LittleEndian

var (
	metaKey = "meta"
)

// NewMQueue constructs a new message queue in an object store.
func NewMQueue(store object.ObjectStore) mqueue.Queue {
	return &mQueue{store}
}

// Peek returns the next message, if any.
func (m *mQueue) Peek() (mqueue.Message, bool, error) {
	// return the message
	headID, _, err := m.GetHeadTail()
	if err != nil || headID == 0 {
		return nil, false, err
	}

	return m.GetMessageByID(headID)
}

// Ack acknowledges the head message by ID, if the head message matches the
// given match ID.
func (m *mQueue) Ack(id uint64) error {
	if id == 0 {
		return nil
	}

	// TODO - this can be optimized with CAS and other operations.
	head, tail, err := m.GetHeadTail()
	if err != nil {
		return err
	}

	if head != id {
		return nil
	}

	// Delete the message
	if err := m.deleteMessageByID(id); err != nil {
		return err
	}

	if tail <= head+1 {
		head = 0
		tail = 0
	} else {
		head++
	}
	return m.SetHeadTail(head, tail)
}

// Push pushes a message to the queue.
// Note: The data buffer may be reused for GetData() in the message.
func (m *mQueue) Push(data []byte) (mqueue.Message, error) {
	ts := time.Now()
	head, tail, err := m.GetHeadTail()
	if err != nil {
		return nil, err
	}

	// write at tail
	mid := tail
	tail++
	if head == 0 {
		head = mid
	}
	mts := timestamp.ToTimestamp(ts)
	key := m.getMessageKey(mid)
	wrapper := &MQMessageWrapper{Timestamp: &mts, Data: data}
	wrapperData, err := proto.Marshal(wrapper)
	if err != nil {
		return nil, err
	}
	if err := m.store.SetObject(key, wrapperData); err != nil {
		return nil, err
	}
	if err := m.SetHeadTail(head, tail); err != nil {
		return nil, err
	}
	return newMQueueMessageFromWrapper(mid, wrapper), nil
}

// deleteMessageByID deletes a message by ID.
func (m *mQueue) deleteMessageByID(id uint64) error {
	key := m.getMessageKey(id)
	return m.store.DeleteObject(key)
}

// GetMessageByID returns a message by numeric ID.
func (m *mQueue) GetMessageByID(id uint64) (mqueue.Message, bool, error) {
	key := m.getMessageKey(id)
	data, ok, err := m.store.GetObject(key)
	if !ok || err != nil {
		return nil, ok, err
	}

	wrapper := &MQMessageWrapper{}
	if err := proto.Unmarshal(data, wrapper); err != nil {
		return nil, false, err
	}

	return newMQueueMessageFromWrapper(id, wrapper), true, nil
}

func (m *mQueue) getMessageKey(id uint64) (key string) {
	return strconv.FormatUint(id, 10)
}

// GetHeadTail returns the head and tail.
// If returns 0, then no messages.
func (m *mQueue) GetHeadTail() (head, tail uint64, err error) {
	defer func() {
		if err == nil {
			if head+1 > tail {
				tail = head + 1
			}
		}
	}()

	var ok bool
	var data []byte
	data, ok, err = m.store.GetObject(metaKey)
	if err != nil || !ok {
		return
	}
	meta := &MQQueueMeta{}
	if ok && len(data) != 0 {
		err = proto.Unmarshal(data, meta)
		if err == nil {
			head = meta.GetHead()
			tail = meta.GetTail()
		}
	} else {
		meta.Head = 0
		meta.Tail = 0
	}
	return
}

// SetHeadTail sets the head and tail.
// Automatically adjusts the values in some conditions.
// If zero, delete the keys.
func (m *mQueue) SetHeadTail(head, tail uint64) (err error) {
	if head == 0 {
		if err := m.store.DeleteObject(metaKey); err != nil {
			return err
		}
		return nil
	}

	if tail < head+1 {
		tail = head + 1
	}

	meta := &MQQueueMeta{}
	meta.Head = head
	meta.Tail = tail
	dat, err := proto.Marshal(meta)
	if err != nil {
		return err
	}

	return m.store.SetObject(metaKey, dat)
}

// DeleteQueue deletes an entire queue.
func (m *mQueue) DeleteQueue() error {
	head, tail, err := m.GetHeadTail()
	if err != nil {
		return err
	}

	if head != 0 {
		if tail <= head {
			tail = head + 1
		}
	}
	for i := head; i < tail; i++ {
		if err := m.deleteMessageByID(i); err != nil {
			return err
		}
	}
	if err := m.store.DeleteObject(metaKey); err != nil {
		return err
	}
	return nil
}

// _ is a type assertion
var _ mqueue.Queue = ((*mQueue)(nil))
