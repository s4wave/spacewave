package block_store_writeback

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"math"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// readsPrefix prefixes the saved chunks of the read record eviction keeps
// blocks by: reads/<16 hex digits of the generation start>/<16 hex digits of
// the save time>. The value is the concatenated keys of the reads saved.
const readsPrefix = "reads/"

// readsStampLen is the length of a hex encoded time stamp in a reads key.
const readsStampLen = 16

// ReadChunk is one saved chunk of a read generation.
type ReadChunk struct {
	// Generation is when the generation started.
	Generation time.Time
	// Keys is the concatenated read keys of the chunk.
	Keys []byte
}

// appendReadsStamp appends the fixed-width hex form of t, which sorts by time.
func appendReadsStamp(key []byte, t time.Time) []byte {
	// Encode the nanoseconds big endian in hex.
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(t.UnixNano()))
	return hex.AppendEncode(key, stamp[:])
}

// parseReadsStamp decodes a fixed-width hex time stamp.
func parseReadsStamp(stamp []byte) (time.Time, error) {
	// Decode the hex digits.
	var raw [8]byte
	if len(stamp) != readsStampLen {
		return time.Time{}, errors.Errorf("malformed reads stamp %q", stamp)
	}
	if _, err := hex.Decode(raw[:], stamp); err != nil {
		return time.Time{}, errors.Wrapf(err, "malformed reads stamp %q", stamp)
	}

	// Read the nanoseconds as a time.
	nanos := binary.BigEndian.Uint64(raw[:])
	if nanos > math.MaxInt64 {
		return time.Time{}, errors.Errorf("reads stamp %q out of range", stamp)
	}
	return time.Unix(0, int64(nanos)), nil
}

// parseReadsGeneration returns the generation start of a reads key.
func parseReadsGeneration(key []byte) (time.Time, error) {
	// Parse the stamp after the prefix.
	rest := key[len(readsPrefix):]
	if len(rest) < readsStampLen {
		return time.Time{}, errors.Errorf("malformed reads key %q", key)
	}
	return parseReadsStamp(rest[:readsStampLen])
}

// SaveReads saves one chunk of reads to the generation started at generation,
// so the reads outlive a restart.
func (s *Store) SaveReads(ctx context.Context, generation time.Time, keys []byte) error {
	// Key the chunk by its generation and the save time.
	key := appendReadsStamp([]byte(readsPrefix), generation)
	key = append(key, '/')
	key = appendReadsStamp(key, time.Now())

	// Write the chunk.
	return kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, key, bytes.Clone(keys))
	})
}

// LoadReads returns the saved chunks of reads, oldest generation first.
func (s *Store) LoadReads(ctx context.Context) ([]ReadChunk, error) {
	// Read every saved chunk with its generation.
	var chunks []ReadChunk
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		chunks = chunks[:0]
		return tx.ScanPrefix(ctx, []byte(readsPrefix), func(key, value []byte) error {
			generation, err := parseReadsGeneration(key)
			if err != nil {
				return err
			}
			chunks = append(chunks, ReadChunk{Generation: generation, Keys: bytes.Clone(value)})
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	// Order the chunks by generation: the scan is not sorted.
	slices.SortStableFunc(chunks, func(a, b ReadChunk) int {
		return a.Generation.Compare(b.Generation)
	})
	return chunks, nil
}

// DropReadsBefore deletes the saved reads of the generations started before
// generation.
func (s *Store) DropReadsBefore(ctx context.Context, generation time.Time) error {
	// Delete the older chunks in one transaction.
	return kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Collect the keys of the older generations, then delete them.
		var drop [][]byte
		err := tx.ScanPrefixKeys(ctx, []byte(readsPrefix), func(key []byte) error {
			started, err := parseReadsGeneration(key)
			if err != nil || started.Before(generation) {
				drop = append(drop, bytes.Clone(key))
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Delete the collected keys.
		for _, key := range drop {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	})
}
