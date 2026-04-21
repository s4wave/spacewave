package world_block

import (
	"context"
	"encoding/binary"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/pkg/errors"
)

// gcJournalSubBlock is the sub-block index for the GC deferred journal.
const gcJournalSubBlock = 6

// gcJournal implements block_gc.WALAppender by writing ref edge batches
// to a world-owned kvtx tree. Entries are keyed by sequential uint64 and
// valued with binary-encoded ref edge batches. The journal lives inside
// the encrypted world state so it is replicated with the world.
type gcJournal struct {
	tree kvtx.BlockTx
	seq  uint64
}

// newGCJournal creates a journal over the given kv tree.
// It scans existing entries to restore the sequence counter.
func newGCJournal(tree kvtx.BlockTx) (*gcJournal, error) {
	j := &gcJournal{tree: tree}
	// Scan to find the highest existing sequence key.
	err := tree.ScanPrefixKeys(context.Background(), nil, func(key []byte) error {
		if len(key) == 8 {
			seq := binary.BigEndian.Uint64(key)
			if seq > j.seq {
				j.seq = seq
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan gc journal")
	}
	return j, nil
}

// Append writes a ref edge batch to the journal.
func (j *gcJournal) Append(_ context.Context, adds, removes []block_gc.RefEdge) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	j.seq++
	var key [8]byte
	binary.BigEndian.PutUint64(key[:], j.seq)
	val := encodeRefBatch(adds, removes)
	return j.tree.Set(context.Background(), key[:], val)
}

// Entries returns the number of pending journal entries.
func (j *gcJournal) Entries() uint64 {
	return j.seq
}

// Iterate calls cb for each journal entry in sequence order.
func (j *gcJournal) Iterate(ctx context.Context, cb func(adds, removes []block_gc.RefEdge) error) error {
	return j.tree.ScanPrefix(ctx, nil, func(key, value []byte) error {
		adds, removes, err := decodeRefBatch(value)
		if err != nil {
			return err
		}
		return cb(adds, removes)
	})
}

// Clear removes all journal entries and resets the sequence counter.
func (j *gcJournal) Clear(ctx context.Context) error {
	var keys [][]byte
	err := j.tree.ScanPrefixKeys(ctx, nil, func(key []byte) error {
		k := make([]byte, len(key))
		copy(k, key)
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := j.tree.Delete(ctx, k); err != nil {
			return err
		}
	}
	j.seq = 0
	return nil
}

// encodeRefBatch serializes adds and removes into a binary batch.
// Format: [4B numAdds][4B numRemoves][edges...]
// Each edge: [2B subjectLen][subject][2B objectLen][object]
func encodeRefBatch(adds, removes []block_gc.RefEdge) []byte {
	size := 8
	for i := range adds {
		size += 4 + len(adds[i].Subject) + len(adds[i].Object)
	}
	for i := range removes {
		size += 4 + len(removes[i].Subject) + len(removes[i].Object)
	}

	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(adds)))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(removes)))
	off := 8
	for i := range adds {
		off = encodeEdge(buf, off, &adds[i])
	}
	for i := range removes {
		off = encodeEdge(buf, off, &removes[i])
	}
	return buf[:off]
}

func encodeEdge(buf []byte, off int, e *block_gc.RefEdge) int {
	binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(e.Subject)))
	off += 2
	copy(buf[off:], e.Subject)
	off += len(e.Subject)
	binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(e.Object)))
	off += 2
	copy(buf[off:], e.Object)
	off += len(e.Object)
	return off
}

// decodeRefBatch deserializes a binary batch into adds and removes.
func decodeRefBatch(data []byte) (adds, removes []block_gc.RefEdge, err error) {
	if len(data) < 8 {
		return nil, nil, errors.New("gc journal entry too short")
	}
	numAdds := binary.BigEndian.Uint32(data[0:4])
	numRemoves := binary.BigEndian.Uint32(data[4:8])
	off := 8

	adds = make([]block_gc.RefEdge, numAdds)
	for i := range adds {
		adds[i], off, err = decodeEdge(data, off)
		if err != nil {
			return nil, nil, err
		}
	}
	removes = make([]block_gc.RefEdge, numRemoves)
	for i := range removes {
		removes[i], off, err = decodeEdge(data, off)
		if err != nil {
			return nil, nil, err
		}
	}
	return adds, removes, nil
}

func decodeEdge(data []byte, off int) (block_gc.RefEdge, int, error) {
	if off+2 > len(data) {
		return block_gc.RefEdge{}, off, errors.New("gc journal edge truncated")
	}
	sLen := int(binary.BigEndian.Uint16(data[off : off+2]))
	off += 2
	if off+sLen > len(data) {
		return block_gc.RefEdge{}, off, errors.New("gc journal subject truncated")
	}
	subject := string(data[off : off+sLen])
	off += sLen

	if off+2 > len(data) {
		return block_gc.RefEdge{}, off, errors.New("gc journal object len truncated")
	}
	oLen := int(binary.BigEndian.Uint16(data[off : off+2]))
	off += 2
	if off+oLen > len(data) {
		return block_gc.RefEdge{}, off, errors.New("gc journal object truncated")
	}
	object := string(data[off : off+oLen])
	off += oLen

	return block_gc.RefEdge{Subject: subject, Object: object}, off, nil
}

// _ is a type assertion
var _ block_gc.WALAppender = (*gcJournal)(nil)
