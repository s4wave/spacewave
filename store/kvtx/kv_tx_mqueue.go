package kvtx

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"time"

	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/store/mqueue"
	"github.com/aperturerobotics/timestamp"
	"github.com/golang/protobuf/proto"
)

// mQueue implements a kvtx message queue.
// head key: points to next msg to peek
// tail key: points to the next message ID (after last pushed)
type mQueue struct {
	prefix  []byte
	headKey []byte
	tailKey []byte
	kvtx    *KVTx
}

// binaryOrder is the binary order used.
var binaryOrder = binary.LittleEndian

var (
	mQueueHeadKey          = []byte("head")
	mQueueTailKey          = []byte("tail")
	mQueueMsgMetaKeySuffix = []byte("-meta")

	mQueueMetaBucketIDKey     = "bucket-id"
	mQueueMetaReconcilerIDKey = "reconciler-id"
)

// readMQueueMeta reads a mqueue meta key.
// may return nil, nil
func readMQueueMeta(tx Tx, key []byte) (*MQQueueMeta, error) {
	data, ok, err := tx.Get(key)
	if err != nil || !ok {
		return nil, err
	}
	meta := &MQQueueMeta{}
	if err := proto.Unmarshal(data, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// listFilledMQueues lists the filled message queues.
func listFilledMQueues(kvtx *KVTx, prefix []byte) ([]bucket_store.BucketReconcilerPair, error) {
	tx, err := kvtx.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	var res []bucket_store.BucketReconcilerPair
	err = tx.ScanPrefix(prefix, func(key []byte) error {
		mqMeta, err := readMQueueMeta(tx, key)
		if err != nil || mqMeta.GetMeta() == nil {
			return err
		}
		mm := mqMeta.GetMeta()
		bid := mm[mQueueMetaBucketIDKey]
		if bid == "" {
			return nil
		}
		rid := mm[mQueueMetaReconcilerIDKey]
		if rid == "" {
			return nil
		}
		res = append(res, bucket_store.BucketReconcilerPair{
			BucketID:     bid,
			ReconcilerID: rid,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func newMQueue(kvtx *KVTx, bucketID, reconcilerID string) *mQueue {
	prefix := kvtx.kvkey.GetBucketReconcilerMQueuePrefix(bucketID, reconcilerID)
	return &mQueue{
		kvtx: kvtx,

		prefix: prefix,
		headKey: bytes.Join([][]byte{
			prefix,
			mQueueHeadKey,
		}, nil),
		tailKey: bytes.Join([][]byte{
			prefix,
			mQueueTailKey,
		}, nil),
	}
}

// Peek returns the next message, if any.
func (m *mQueue) Peek() (mqueue.Message, bool, error) {
	tx, err := m.kvtx.store.NewTransaction(false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()

	// return the message
	headID, _, err := m.GetHeadTail(tx)
	if err != nil || headID == 0 {
		return nil, false, err
	}

	return m.GetMessageByID(tx, headID)
}

// Ack acknowledges the head message by ID, if the head message matches the
// given match ID.
func (m *mQueue) Ack(id uint64) error {
	if id == 0 {
		return nil
	}

	tx, err := m.kvtx.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// TODO - this can be optimized with CAS and other operations.
	head, tail, err := m.GetHeadTail(tx)
	if err != nil {
		return err
	}

	if head != id {
		return nil
	}

	// Delete the message
	if err := m.deleteMessageByID(tx, id); err != nil {
		return err
	}

	if tail <= head+1 {
		head = 0
		tail = 0
	} else {
		head++
	}
	if err := m.SetHeadTail(tx, head, tail); err != nil {
		return err
	}

	return tx.Commit(m.kvtx.ctx)
}

// Push pushes a message to the queue.
// Note: The data buffer may be reused for GetData() in the message.
func (m *mQueue) Push(data []byte) (mqueue.Message, error) {
	ts := time.Now()
	tx, err := m.kvtx.store.NewTransaction(true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	head, tail, err := m.GetHeadTail(tx)
	if err != nil {
		return nil, err
	}

	// write at tail
	mid := tail
	tail++
	if head == 0 {
		head = mid
	}
	key, metaKey := m.getMessageKey(mid)
	meta := &MQMessageMeta{}
	mts := timestamp.ToTimestamp(ts)
	meta.Timestamp = &mts
	metaData, err := proto.Marshal(meta)
	if err != nil {
		return nil, err
	}
	if err := tx.Set(metaKey, metaData, 0); err != nil {
		return nil, err
	}
	if err := tx.Set(key, data, 0); err != nil {
		return nil, err
	}
	if err := m.SetHeadTail(tx, head, tail); err != nil {
		return nil, err
	}
	if err := tx.Commit(m.kvtx.ctx); err != nil {
		return nil, err
	}
	return newMQueueMessage(mid, data, ts), nil
}

// deleteMessageByID deletes a message by ID.
func (m *mQueue) deleteMessageByID(tx Tx, id uint64) error {
	key, metaKey := m.getMessageKey(id)
	if err := tx.Delete(key); err != nil {
		return err
	}
	if err := tx.Delete(metaKey); err != nil {
		return err
	}
	return nil
}

// GetMessageByID returns a message by numeric ID.
func (m *mQueue) GetMessageByID(tx Tx, id uint64) (mqueue.Message, bool, error) {
	key, metaKey := m.getMessageKey(id)
	metaData, ok, err := tx.Get(metaKey)
	if !ok || err != nil {
		return nil, ok, err
	}

	meta := &MQMessageMeta{}
	if err := proto.Unmarshal(metaData, meta); err != nil {
		return nil, false, err
	}

	data, ok, err := tx.Get(key)
	if !ok || err != nil {
		return nil, ok, err
	}

	return newMQueueMessage(id, data, meta.GetTimestamp().ToTime()), true, nil
}

func (m *mQueue) getMessageKey(id uint64) (key []byte, metaKey []byte) {
	metaKey = bytes.Join([][]byte{
		m.prefix,
		[]byte(strconv.FormatUint(id, 10)),
		mQueueMsgMetaKeySuffix,
	}, nil)
	key = metaKey[:len(metaKey)-len(mQueueMsgMetaKeySuffix)]
	return
}

// GetHeadTail returns the head and tail.
// If returns 0, then no messages.
func (m *mQueue) GetHeadTail(tx Tx) (head, tail uint64, err error) {
	data, found, err := tx.Get(m.headKey)
	if err != nil {
		return 0, 0, err
	}

	// head contains little-endian head message ID.
	if found && len(data) == 8 {
		head = binaryOrder.Uint64(data)
	}

	data, found, err = tx.Get(m.tailKey)
	if err != nil {
		return 0, 0, err
	}

	// head contains little-endian head message ID.
	if found && len(data) == 8 {
		tail = binaryOrder.Uint64(data)
	}
	if head+1 > tail {
		tail = head + 1
	}
	return
}

// SetHeadTail sets the head and tail.
// Automatically adjusts the values in some conditions.
// If zero, delete the keys.
func (m *mQueue) SetHeadTail(tx Tx, head, tail uint64) (err error) {
	if head == 0 {
		if err := tx.Delete(m.headKey); err != nil {
			return err
		}
		_ = tx.Delete(m.tailKey)
		return nil
	}

	if tail < head+1 {
		tail = head + 1
	}

	vals := make([]byte, 16)
	headVal := vals[:8]
	tailVal := vals[8:]
	binary.PutUvarint(headVal, head)
	binary.PutUvarint(tailVal, tail)

	if err := tx.Set(m.headKey, headVal, 0); err != nil {
		return err
	}
	return tx.Set(m.tailKey, tailVal, 0)
}

// DeleteQueue deletes an entire queue.
func (m *mQueue) DeleteQueue() error {
	ktx, err := m.kvtx.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer ktx.Discard()

	head, tail, err := m.GetHeadTail(ktx)
	if err != nil {
		return err
	}

	if head != 0 {
		if tail <= head {
			tail = head + 1
		}
	}
	for i := head; i < tail; i++ {
		if err := m.deleteMessageByID(ktx, i); err != nil {
			return err
		}
	}
	if err := ktx.Delete(m.tailKey); err != nil {
		return err
	}
	if err := ktx.Delete(m.headKey); err != nil {
		return err
	}
	return ktx.Commit(m.kvtx.ctx)
}

// _ is a type assertion
var _ mqueue.Queue = ((*mQueue)(nil))
