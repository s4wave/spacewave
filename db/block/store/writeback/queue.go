package block_store_writeback

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// markedPrefix prefixes the key that records a block awaiting upload, so a
// repeated write finds it by hash: marked/<hash>. The value is the block's
// queue key.
const markedPrefix = "marked/"

// queuePrefix prefixes the queue entry of each block awaiting upload, ordered
// by write time: queue/<16 hex digits of Unix nanoseconds>/<hash>. The value
// is the block size, followed by @ and the Unix nanoseconds of the latest
// repeated write once there is one.
const queuePrefix = "queue/"

// queueTimeLen is the length of the write time in a queue key.
const queueTimeLen = 16

// queueItem is a queue entry as a scan reads it. key and value belong to the
// iterator and hold only during the scan's callback; pending copies them.
type queueItem struct {
	// key names this entry for the duration of the callback.
	key []byte
	// value holds its encoding for the duration of the callback.
	value []byte
}

// queueEnd sorts after every queue key, so a floor at it starts a scan past
// the queue.
const queueEnd = "queue0"

// scanQueue calls cb with each queue entry in write order, unparsed, so a
// scan pays only for the fields it reads, from the key from,
// or from the queue's start when from is nil, until cb returns an error.
//
// A floor below every live entry skips the deletes the store keeps for the
// entries already cleared, which an iteration from the queue's start walks
// one by one until the store checkpoints.
func scanQueue(ctx context.Context, tx kvtx.Tx, from []byte, cb func(queueItem) error) error {
	// Seek to the earliest possible live entry before scanning.
	it := tx.Iterate(ctx, []byte(queuePrefix), true, false)
	defer it.Close()
	ok := false
	if from != nil {
		if err := it.Seek(from); err != nil {
			return err
		}
		ok = it.Valid()
	} else {
		ok = it.Next()
	}
	for ; ok; ok = it.Next() {
		value, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(queueItem{key: it.Key(), value: value}); err != nil {
			return err
		}
	}
	return it.Err()
}

// markedKey returns the key recording that a block awaits upload.
func markedKey(h *hash.Hash) []byte {
	return []byte(markedPrefix + h.MarshalString())
}

// queueTime returns the write time a queue key records for at: the zero time
// sorts first, as the Unix epoch. It drops the monotonic reading, so the
// in-memory queue head compares by wall clock like the parsed keys.
func queueTime(at time.Time) time.Time {
	if at.IsZero() {
		return time.Unix(0, 0)
	}
	return time.Unix(0, max(at.UnixNano(), 0))
}

// queueFloor returns a key at or before every queue key of a block written
// at at or later, which queueTime has normalized.
func queueFloor(at time.Time) []byte {
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(at.UnixNano()))
	return hex.AppendEncode([]byte(queuePrefix), stamp[:])
}

// queueKey returns the queue key of a block written at at, which queueTime
// has normalized.
func queueKey(at time.Time, h *hash.Hash) []byte {
	// Encode the write time as fixed-width hex, so keys sort by time.
	key := append(queueFloor(at), '/')
	return append(key, h.MarshalString()...)
}

// at decodes the block's write time from the key.
func (q queueItem) at() (time.Time, error) {
	// Decode the timestamp without retaining iterator-owned bytes.
	rest := q.key[len(queuePrefix):]
	if len(rest) <= queueTimeLen || rest[queueTimeLen] != '/' {
		return time.Time{}, errors.Errorf("malformed upload queue key %q", q.key)
	}
	var stamp [8]byte
	if _, err := hex.Decode(stamp[:], rest[:queueTimeLen]); err != nil {
		return time.Time{}, errors.Wrap(err, "parse upload queue time")
	}
	nanos := binary.BigEndian.Uint64(stamp[:])
	if nanos > math.MaxInt64 {
		return time.Time{}, errors.Errorf("upload queue time out of range in %q", q.key)
	}
	return time.Unix(0, int64(nanos)), nil
}

// size decodes the block size from the value, or 0 when it holds none.
func (q queueItem) size() int64 {
	text, _, _ := bytes.Cut(q.value, []byte("@"))
	size, _ := strconv.ParseInt(string(text), 10, 64)
	return size
}

// pending decodes the whole entry into a block awaiting upload, which owns
// copies of the key and value.
func (q queueItem) pending() (pendingBlock, error) {
	// Decode the write time, then the block hash after it.
	at, err := q.at()
	if err != nil {
		return pendingBlock{}, err
	}
	text := q.key[len(queuePrefix)+queueTimeLen+1:]
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(text)); err != nil {
		return pendingBlock{}, errors.Wrap(err, "parse upload queue hash")
	}
	return pendingBlock{
		queueKey:  bytes.Clone(q.key),
		markedKey: append([]byte(markedPrefix), text...),
		value:     bytes.Clone(q.value),
		ref:       block.NewBlockRef(h),
		size:      q.size(),
		at:        at,
	}, nil
}

// deletePrefix deletes every key that starts with prefix.
func deletePrefix(ctx context.Context, tx kvtx.Tx, prefix string) error {
	// Collect the keys before deleting, so the scan sees no deletes.
	var keys [][]byte
	err := tx.ScanPrefixKeys(ctx, []byte(prefix), func(key []byte) error {
		keys = append(keys, bytes.Clone(key))
		return nil
	})
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := tx.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// queueDelta is the change queued blocks make to the queue's projection.
type queueDelta struct {
	// added is the number of blocks that were not queued before.
	added int
	// addedBytes is the size of those blocks.
	addedBytes int64
	// oldest is the earliest write time of those blocks, while added is not
	// zero.
	oldest time.Time
}

// queueMarks queues the marked blocks in tx and returns the blocks it queued
// for the first time. It reads the blocks' markers in one batch, so a cold
// index costs one round of reads rather than one read per block while tx
// holds the store's writer.
//
// A block that is queued already keeps its first write time, so it uploads
// with its first write. Its queue value changes, so an upload that read the
// block before this write finds the entry changed and keeps it: the upload
// could have read the block absent, as a swept block that this write stored
// again.
func queueMarks(ctx context.Context, tx kvtx.Tx, marks []Mark) (queueDelta, error) {
	// Read every block's marker, which names its queue entry.
	keys := make([][]byte, len(marks))
	for i, mark := range marks {
		keys[i] = markedKey(mark.Hash)
	}
	entries, found, err := kvtx.GetBatch(ctx, tx, keys)
	if err != nil {
		return queueDelta{}, err
	}

	// Queue each block, finding a block listed twice under the entry its
	// first listing queued.
	var delta queueDelta
	now := time.Now()
	queuedHere := make(map[string][]byte)
	for i, mark := range marks {
		key := keys[i]
		queued := entries[i]
		if q, ok := queuedHere[string(key)]; ok {
			queued = q
		} else if !found[i] {
			queued = nil
		}

		// Change the queue value of a block that is queued already.
		if len(queued) != 0 {
			if err := tx.Set(ctx, queued, queueValue(mark.Size, now)); err != nil {
				return queueDelta{}, err
			}
			continue
		}

		// Queue the block at its write time.
		at := queueTime(mark.At)
		entryKey := queueKey(at, mark.Hash)
		if err := tx.Set(ctx, key, entryKey); err != nil {
			return queueDelta{}, err
		}
		if err := tx.Set(ctx, entryKey, queueValue(mark.Size, time.Time{})); err != nil {
			return queueDelta{}, err
		}
		queuedHere[string(key)] = entryKey
		if delta.added == 0 || at.Before(delta.oldest) {
			delta.oldest = at
		}
		delta.added++
		delta.addedBytes += mark.Size
	}
	return delta, nil
}

// queueValue returns the value of a queue entry for a block of size bytes. A
// zero repeated gives the value of the first write.
func queueValue(size int64, repeated time.Time) []byte {
	value := strconv.AppendInt(nil, size, 10)
	if repeated.IsZero() {
		return value
	}
	value = append(value, '@')
	return strconv.AppendInt(value, repeated.UnixNano(), 10)
}
