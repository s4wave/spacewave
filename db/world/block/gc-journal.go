package world_block

import (
	"context"
	"encoding/binary"
	stderrors "errors"

	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// gcJournalSubBlock is the sub-block index for the GC deferred journal.
const gcJournalSubBlock = 6

var (
	gcJournalSeqKey      = []byte("seq")
	gcJournalCountKey    = []byte("count")
	errGCJournalTakeDone = stderrors.New("gc journal take done")
)

// gcJournal implements block_gc.WALAppender by writing ref edge batches
// to a world-owned kvtx tree. Entries are keyed by sequential uint64 and
// valued with binary-encoded ref edge batches. The journal lives inside
// the encrypted world state so it is replicated with the world.
type gcJournal struct {
	tree  kvtx.BlockTx
	seq   uint64
	count uint64
}

// newGCJournal creates a journal over the given kv tree.
// It reads the sequence counter from metadata. Older journals without
// metadata are scanned once and upgraded on the next write.
func newGCJournal(ctx context.Context, tree kvtx.BlockTx, write bool) (*gcJournal, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	j := &gcJournal{tree: tree}
	seqData, found, err := tree.Get(ctx, gcJournalSeqKey)
	if err != nil {
		return nil, errors.Wrap(err, "get gc journal sequence")
	}
	if found {
		if len(seqData) != 8 {
			return nil, errors.New("gc journal sequence metadata has invalid length")
		}
		j.seq = binary.BigEndian.Uint64(seqData)
		countData, found, err := tree.Get(ctx, gcJournalCountKey)
		if err != nil {
			return nil, errors.Wrap(err, "get gc journal count")
		}
		if found {
			if len(countData) != 8 {
				return nil, errors.New("gc journal count metadata has invalid length")
			}
			j.count = binary.BigEndian.Uint64(countData)
			return j, nil
		}
	}

	var count uint64
	err = tree.ScanPrefixKeys(ctx, nil, func(key []byte) error {
		if isGCJournalEntryKey(key) {
			seq := binary.BigEndian.Uint64(key)
			if seq > j.seq {
				j.seq = seq
			}
			count++
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan gc journal")
	}
	j.count = count
	if write && (j.seq != 0 || j.count != 0) {
		if err := j.storeSeq(ctx, j.seq); err != nil {
			return nil, errors.Wrap(err, "store gc journal sequence")
		}
		if err := j.storeCount(ctx, j.count); err != nil {
			return nil, errors.Wrap(err, "store gc journal count")
		}
	}
	return j, nil
}

// Append writes a ref edge batch to the journal.
func (j *gcJournal) Append(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for len(adds)+len(removes) != 0 {
		nextAdds, nextRemoves, remainingAdds, remainingRemoves, _ := splitRefBatch(
			adds,
			removes,
			defaultGCJournalReconcileEdgeLimit,
		)
		if err := j.appendBatch(ctx, nextAdds, nextRemoves); err != nil {
			return err
		}
		adds = remainingAdds
		removes = remainingRemoves
	}
	return nil
}

func (j *gcJournal) appendBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	nextSeq := j.seq + 1
	var key [8]byte
	binary.BigEndian.PutUint64(key[:], nextSeq)
	val := encodeRefBatch(adds, removes)
	if err := j.tree.Set(ctx, key[:], val); err != nil {
		return err
	}
	if err := j.storeSeq(ctx, nextSeq); err != nil {
		return err
	}
	if err := j.storeCount(ctx, j.count+1); err != nil {
		return err
	}
	j.seq = nextSeq
	j.count++
	return nil
}

// Entries returns the number of pending journal entries.
func (j *gcJournal) Entries() uint64 {
	return j.count
}

// Iterate calls cb for each journal entry in sequence order.
func (j *gcJournal) Iterate(ctx context.Context, cb func(adds, removes []block_gc.RefEdge) error) error {
	return j.tree.ScanPrefix(ctx, nil, func(key, value []byte) error {
		if !isGCJournalEntryKey(key) {
			return nil
		}
		adds, removes, err := decodeRefBatch(value)
		if err != nil {
			return err
		}
		return cb(adds, removes)
	})
}

// Take returns up to maxEntries and maxEdges journal entries in sequence order.
// Limits of zero disable the corresponding bound.
func (j *gcJournal) Take(
	ctx context.Context,
	maxEntries uint64,
	maxEdges uint64,
) ([]gcJournalEntry, error) {
	var out []gcJournalEntry
	var edges uint64
	err := j.tree.ScanPrefix(ctx, nil, func(key, value []byte) error {
		if !isGCJournalEntryKey(key) {
			return nil
		}
		if maxEntries != 0 && uint64(len(out)) >= maxEntries {
			return errGCJournalTakeDone
		}
		adds, removes, err := decodeRefBatch(value)
		if err != nil {
			return err
		}
		entryEdges := uint64(len(adds) + len(removes))
		if maxEdges != 0 && edges >= maxEdges {
			return errGCJournalTakeDone
		}
		var remainingAdds, remainingRemoves []block_gc.RefEdge
		if maxEdges != 0 && edges+entryEdges > maxEdges {
			var takenEdges uint64
			adds, removes, remainingAdds, remainingRemoves, takenEdges = splitRefBatch(
				adds,
				removes,
				maxEdges-edges,
			)
			entryEdges = takenEdges
		}
		k := make([]byte, len(key))
		copy(k, key)
		out = append(out, gcJournalEntry{
			key:              k,
			adds:             adds,
			removes:          removes,
			remainingAdds:    remainingAdds,
			remainingRemoves: remainingRemoves,
		})
		edges += entryEdges
		if len(remainingAdds) != 0 || len(remainingRemoves) != 0 {
			return errGCJournalTakeDone
		}
		return nil
	})
	if err != nil {
		if stderrors.Is(err, errGCJournalTakeDone) {
			return out, nil
		}
		return nil, err
	}
	return out, nil
}

// DeleteApplied removes already-applied journal entries.
func (j *gcJournal) DeleteApplied(ctx context.Context, entries []gcJournalEntry) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, entry := range entries {
		if len(entry.remainingAdds) != 0 || len(entry.remainingRemoves) != 0 {
			data := encodeRefBatch(entry.remainingAdds, entry.remainingRemoves)
			if err := j.tree.Set(ctx, entry.key, data); err != nil {
				return err
			}
			continue
		}
		if err := j.tree.Delete(ctx, entry.key); err != nil {
			return err
		}
		j.count--
	}
	if j.count == 0 {
		if err := j.tree.Delete(ctx, gcJournalCountKey); err != nil {
			return err
		}
		return nil
	}
	return j.storeCount(ctx, j.count)
}

// Clear removes all journal entries and resets the sequence counter.
func (j *gcJournal) Clear(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var keys [][]byte
	err := j.tree.ScanPrefixKeys(ctx, nil, func(key []byte) error {
		if !isGCJournalEntryKey(key) {
			return nil
		}
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
	if err := j.tree.Delete(ctx, gcJournalSeqKey); err != nil {
		return err
	}
	if err := j.tree.Delete(ctx, gcJournalCountKey); err != nil {
		return err
	}
	j.seq = 0
	j.count = 0
	return nil
}

func (j *gcJournal) storeSeq(ctx context.Context, seq uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], seq)
	return j.tree.Set(ctx, gcJournalSeqKey, buf[:])
}

func (j *gcJournal) storeCount(ctx context.Context, count uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], count)
	return j.tree.Set(ctx, gcJournalCountKey, buf[:])
}

func isGCJournalEntryKey(key []byte) bool {
	return len(key) == 8
}

type gcJournalEntry struct {
	key              []byte
	adds             []block_gc.RefEdge
	removes          []block_gc.RefEdge
	remainingAdds    []block_gc.RefEdge
	remainingRemoves []block_gc.RefEdge
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
	binary.BigEndian.PutUint32(buf[0:4], mustGCJournalUint32Len(len(adds)))
	binary.BigEndian.PutUint32(buf[4:8], mustGCJournalUint32Len(len(removes)))
	off := 8
	for i := range adds {
		off = encodeEdge(buf, off, &adds[i])
	}
	for i := range removes {
		off = encodeEdge(buf, off, &removes[i])
	}
	return buf[:off]
}

func splitRefBatch(
	adds []block_gc.RefEdge,
	removes []block_gc.RefEdge,
	maxEdges uint64,
) (
	[]block_gc.RefEdge,
	[]block_gc.RefEdge,
	[]block_gc.RefEdge,
	[]block_gc.RefEdge,
	uint64,
) {
	if maxEdges == 0 {
		return nil, nil, adds, removes, 0
	}
	if uint64(len(adds)) >= maxEdges {
		n := int(maxEdges)
		return adds[:n], nil, adds[n:], removes, maxEdges
	}
	removeLimit := int(maxEdges) - len(adds)
	if removeLimit >= len(removes) {
		return adds, removes, nil, nil, uint64(len(adds) + len(removes))
	}
	return adds, removes[:removeLimit], nil, removes[removeLimit:], maxEdges
}

func encodeEdge(buf []byte, off int, e *block_gc.RefEdge) int {
	binary.BigEndian.PutUint16(buf[off:off+2], mustGCJournalUint16Len(len(e.Subject)))
	off += 2
	copy(buf[off:], e.Subject)
	off += len(e.Subject)
	binary.BigEndian.PutUint16(buf[off:off+2], mustGCJournalUint16Len(len(e.Object)))
	off += 2
	copy(buf[off:], e.Object)
	off += len(e.Object)
	return off
}

func mustGCJournalUint16Len(v int) uint16 {
	if v < 0 || v > 0xffff {
		panic("world-block: gc journal length overflows uint16")
	}
	return uint16(v)
}

func mustGCJournalUint32Len(v int) uint32 {
	if v < 0 || uint64(v) > 0xffffffff {
		panic("world-block: gc journal length overflows uint32")
	}
	return uint32(v) // #nosec G115 -- bounded above.
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
