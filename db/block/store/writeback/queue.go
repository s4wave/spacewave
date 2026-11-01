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

// queueEntry is a decoded queue entry.
type queueEntry struct {
	// key is the entry's key, owned by the entry.
	key []byte
	// at is the block's write time.
	at time.Time
	// hash is the block hash.
	hash *hash.Hash
	// size is the block size in bytes.
	size int64
	// value is the entry's value, owned by the entry. A repeated write of the
	// block changes it.
	value []byte
}

// scanQueue calls cb with each queue entry in write order until cb returns an
// error.
func scanQueue(ctx context.Context, tx kvtx.Tx, cb func(queueEntry) error) error {
	it := tx.Iterate(ctx, []byte(queuePrefix), true, false)
	defer it.Close()
	for it.Next() {
		entry, err := parseQueueKey(it.Key())
		if err != nil {
			return err
		}
		value, err := it.Value()
		if err != nil {
			return err
		}
		entry.value = bytes.Clone(value)
		size, _, _ := bytes.Cut(value, []byte("@"))
		entry.size, _ = strconv.ParseInt(string(size), 10, 64)
		if err := cb(entry); err != nil {
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

// queueKey returns the queue key of a block written at at, which queueTime
// has normalized.
func queueKey(at time.Time, h *hash.Hash) []byte {
	// Encode the write time as fixed-width hex, so keys sort by time.
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(at.UnixNano()))
	key := []byte(queuePrefix)
	key = hex.AppendEncode(key, stamp[:])
	key = append(key, '/')
	return append(key, h.MarshalString()...)
}

// parseQueueKey decodes a queue key into an entry without its size.
func parseQueueKey(key []byte) (queueEntry, error) {
	// Split the key into its write time and hash.
	rest := key[len(queuePrefix):]
	if len(rest) <= queueTimeLen || rest[queueTimeLen] != '/' {
		return queueEntry{}, errors.Errorf("malformed upload queue key %q", key)
	}

	// Decode the write time.
	var stamp [8]byte
	if _, err := hex.Decode(stamp[:], rest[:queueTimeLen]); err != nil {
		return queueEntry{}, errors.Wrap(err, "parse upload queue time")
	}
	nanos := binary.BigEndian.Uint64(stamp[:])
	if nanos > math.MaxInt64 {
		return queueEntry{}, errors.Errorf("upload queue time out of range in %q", key)
	}

	// Decode the block hash.
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(rest[queueTimeLen+1:])); err != nil {
		return queueEntry{}, errors.Wrap(err, "parse upload queue hash")
	}
	return queueEntry{
		key:  bytes.Clone(key),
		at:   time.Unix(0, int64(nanos)),
		hash: h,
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
// for the first time.
//
// A block that is queued already keeps its first write time, so it uploads
// with its first write. Its queue value changes, so an upload that read the
// block before this write finds the entry changed and keeps it: the upload
// could have read the block absent, as a swept block that this write stored
// again.
func queueMarks(ctx context.Context, tx kvtx.Tx, marks []Mark) (queueDelta, error) {
	var delta queueDelta
	now := time.Now()
	for _, mark := range marks {
		// Find the block's marker, which names its queue entry.
		key := markedKey(mark.Hash)
		queued, found, err := tx.Get(ctx, key)
		if err != nil {
			return queueDelta{}, err
		}

		// Change the queue value of a block that is queued already.
		if found && len(queued) != 0 {
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
