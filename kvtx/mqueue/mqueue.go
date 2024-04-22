package kvtx_mqueue

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"time"

	// "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/mqueue"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// MQueue implements a Hydra Object-Store message queue.
// head key: points to next msg to peek
// tail key: points to the next message ID (after last pushed)
type MQueue struct {
	store   kvtx.Store
	conf    *Config
	pollDur time.Duration
	wakeCh  chan struct{}
	mtx     sync.Mutex
}

var (
	metaKey       = []byte("meta")
	messagePrefix = []byte("m/")
	minPollDur    = time.Millisecond * 100
	defPollDur    = time.Second * 10
)

// NewMQueue constructs a new message queue in an object store.
func NewMQueue(store kvtx.Store, conf *Config) mqueue.Queue {
	pollDur, _ := conf.ParsePollDur(minPollDur, defPollDur)
	wakeCh := make(chan struct{})
	return &MQueue{
		store:   store,
		conf:    conf,
		wakeCh:  wakeCh,
		pollDur: pollDur,
	}
}

// Peek returns the next message, if any.
func (m *MQueue) Peek(ctx context.Context) (mqueue.Message, bool, error) {
	var write bool
	tx, err := m.store.NewTransaction(ctx, write)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()

	for {
		// return the message
		headID, _, err := m.GetHeadTail(ctx, tx)
		if err != nil || headID == 0 {
			return nil, false, err
		}
		msg, ok, err := m.GetMessageByID(ctx, tx, headID)
		if err != nil || ok {
			return msg, ok, err
		}
		// not found, skip to next message + ack this one.
		if !write {
			tx.Discard()
			write = true
			tx, err = m.store.NewTransaction(ctx, write)
			if err != nil {
				return nil, false, err
			}
		}
		err = m.ackLocked(ctx, tx, headID)
		if err != nil {
			return nil, false, err
		}
	}
}

// Ack acknowledges the head message by ID, if the head message matches the
// given match ID.
func (m *MQueue) Ack(ctx context.Context, id uint64) error {
	if id == 0 {
		return nil
	}

	// TODO - this can be optimized with CAS and other operations.
	tx, err := m.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	return m.ackLocked(ctx, tx, id)
}

// ackLocked acks a message.
func (m *MQueue) ackLocked(ctx context.Context, tx kvtx.Tx, id uint64) error {
	head, tail, err := m.GetHeadTail(ctx, tx)
	if err != nil {
		return err
	}

	if head != id {
		return nil
	}

	// Delete the message
	if err := m.deleteMessageByID(ctx, tx, id); err != nil {
		return err
	}

	if tail <= head+1 {
		head = 0
		tail = 0
	} else {
		head++
	}
	if err := m.SetHeadTail(ctx, tx, head, tail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Push pushes a message to the queue.
// Note: The data buffer may be reused for GetData() in the message.
func (m *MQueue) Push(ctx context.Context, data []byte) (mqueue.Message, error) {
	ts := time.Now()
	tx, err := m.store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	head, tail, err := m.GetHeadTail(ctx, tx)
	if err != nil {
		return nil, err
	}

	// write at tail
	mid := tail
	tail++
	if head == 0 {
		head = mid
	}
	mts := timestamp.New(ts)
	key := m.getMessageKey(mid)
	wrapper := &MQMessageWrapper{Timestamp: mts, Data: data}
	wrapperData, err := wrapper.MarshalVT()
	if err != nil {
		return nil, err
	}
	if err := tx.Set(ctx, key, wrapperData); err != nil {
		return nil, err
	}
	if err := m.SetHeadTail(ctx, tx, head, tail); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return newMQueueMessageFromWrapper(mid, wrapper), nil
}

// deleteMessageByID deletes a message by ID.
func (m *MQueue) deleteMessageByID(ctx context.Context, tx kvtx.Tx, id uint64) error {
	key := m.getMessageKey(id)
	return tx.Delete(ctx, key)
}

// GetMessageByID returns a message by numeric ID.
func (m *MQueue) GetMessageByID(ctx context.Context, tx kvtx.Tx, id uint64) (mqueue.Message, bool, error) {
	key := m.getMessageKey(id)
	data, ok, err := tx.Get(ctx, key)
	if !ok || err != nil {
		return nil, ok, err
	}

	wrapper := &MQMessageWrapper{}
	if err := wrapper.UnmarshalVT(data); err != nil {
		return nil, false, err
	}

	return newMQueueMessageFromWrapper(id, wrapper), true, nil
}

func (m *MQueue) getMessageKey(id uint64) (key []byte) {
	return bytes.Join([][]byte{
		messagePrefix,
		[]byte(strconv.FormatUint(id, 10)),
	}, nil)
}

// GetHeadTail returns the head and tail.
// If returns 0, then no messages.
func (m *MQueue) GetHeadTail(ctx context.Context, tx kvtx.Tx) (head, tail uint64, err error) {
	defer func() {
		if err == nil {
			if head+1 > tail {
				tail = head + 1
			}
		}
	}()

	var ok bool
	var data []byte
	data, ok, err = tx.Get(ctx, metaKey)
	if err != nil || !ok {
		return
	}
	meta := &MQQueueMeta{}
	if ok && len(data) != 0 {
		err = meta.UnmarshalVT(data)
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
func (m *MQueue) SetHeadTail(ctx context.Context, tx kvtx.Tx, head, tail uint64) (err error) {
	if head == 0 {
		if err := tx.Delete(ctx, metaKey); err != nil {
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
	dat, err := meta.MarshalVT()
	if err != nil {
		return err
	}

	return tx.Set(ctx, metaKey, dat)
}

// DeleteQueue deletes an entire queue.
func (m *MQueue) DeleteQueue(ctx context.Context) error {
	tx, err := m.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	head, tail, err := m.GetHeadTail(ctx, tx)
	if err != nil {
		return err
	}

	if head != 0 {
		if tail <= head {
			tail = head + 1
		}
	}
	for i := head; i < tail; i++ {
		if err := m.deleteMessageByID(ctx, tx, i); err != nil {
			return err
		}
	}
	if err := tx.Delete(ctx, metaKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Wait() waits for the next message, or context cancellation.
//
// Returns the message. Equiv to Peek if a message is available.
// Acks the message immediately if ack is true.
func (m *MQueue) Wait(ctx context.Context, ack bool) (mqueue.Message, error) {
	if pollDur := m.pollDur; pollDur != 0 {
		return m.WaitPolling(ctx, ack, pollDur)
	}
	return m.WaitSingleWriter(ctx, ack)
}

// Wake wakes the mqueue listeners.
func (m *MQueue) Wake() {
	m.mtx.Lock()
WakeLoop:
	for {
		select {
		case m.wakeCh <- struct{}{}:
		default:
			break WakeLoop
		}
	}
	m.mtx.Unlock()
}

// PeekAck runs the locked peek/ack operation for waiters.
func (m *MQueue) PeekAck(ctx context.Context, ack bool) (mqueue.Message, bool, error) {
	m.mtx.Lock()
	msg, msgOk, err := m.Peek(ctx)
	if err == nil && msg != nil && ack {
		err = m.Ack(ctx, msg.GetId())
	}
	m.mtx.Unlock()
	return msg, msgOk, err
}

// WaitSingleWriter checks Peek, then waits for Wake(). does not poll.
func (m *MQueue) WaitSingleWriter(ctx context.Context, ack bool) (mqueue.Message, error) {
	for {
		msg, msgOk, err := m.PeekAck(ctx, ack)
		if (msgOk && msg != nil) || err != nil {
			return msg, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.wakeCh:
			// woken, recheck
		}
	}
}

// WaitPolling checks Peek with a polling duration.
// if pollDur == 0, returns immediately
func (m *MQueue) WaitPolling(ctx context.Context, ack bool, pollDur time.Duration) (mqueue.Message, error) {
	for {
		msg, msgOk, err := m.PeekAck(ctx, ack)
		if (msgOk && msg != nil) || err != nil || pollDur == 0 {
			return msg, err
		}

		checkNext := time.NewTimer(pollDur)
		select {
		case <-ctx.Done():
			checkNext.Stop()
			return nil, ctx.Err()
		case <-m.wakeCh:
			checkNext.Stop()
		case <-checkNext.C:
		}
	}
}

// _ is a type assertion
var _ mqueue.Queue = ((*MQueue)(nil))
