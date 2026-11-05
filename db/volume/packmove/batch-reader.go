package packmove

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile/writer"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// maxExamined bounds the block keys one batch passes, so a long run of blocks
// outside the World or with bad data does not hold one snapshot open.
const maxExamined = 1 << 15

// packBatch is the blocks of one pack, read from the volume.
type packBatch struct {
	// frame is the batch's record for the pack log: the key it examined
	// through, its packed and bad blocks, and the packed data size.
	frame
	// puts are the packed blocks with their references, in the order of the
	// frame's packed entries.
	puts []*block.PutBatchEntry
	// untracked counts the packed blocks the reference graph did not track.
	untracked uint64
	// outside counts the blocks the batch passed because they are outside the
	// World.
	outside uint64
	// seq is the batch's place in the order the reader read the batches.
	seq uint64
}

// batchReader reads the volume's blocks in key order, one batch per snapshot,
// so a handle sharing the file with a writer never reads a snapshot for long.
type batchReader struct {
	// src is the volume store.
	src kvtx.Store
	// prefix is the volume's block key prefix.
	prefix []byte
	// after is the last key examined, relative to prefix, or nil before the
	// first batch.
	after []byte
	// end is the first key, relative to prefix, past the range, or nil.
	end []byte
	// packBytes is the block data one batch holds.
	packBytes int
	// outside is the blocks to leave alone.
	outside *OutsideWorld
}

// next reads the batch after the last one, or returns nil at the end of the
// blocks. It stops after the block that brings the batch to packBytes or to
// the packfile's block limit.
func (r *batchReader) next(ctx context.Context) (*packBatch, error) {
	// Read the batch from a snapshot the writer did not pass.
	var batch *packBatch
	err := ReadStable(ctx, r.src, func(tx kvtx.Tx) error {
		var err error
		batch, err = r.read(ctx, tx)
		return err
	})
	if err != nil || batch == nil {
		return nil, err
	}
	r.after = batch.through
	return batch, nil
}

// read reads the next batch from tx: the blocks to pack with their references,
// and the blocks it found bad.
func (r *batchReader) read(ctx context.Context, tx kvtx.Tx) (*packBatch, error) {
	// Collect the keys and data of the batch.
	batch, err := r.collect(ctx, tx)
	if err != nil || len(batch.through) == 0 {
		return nil, err
	}

	// Read each packed block's references from the graph in the same
	// snapshot.
	if err := attachRefs(ctx, tx, batch); err != nil {
		return nil, err
	}
	return batch, nil
}

// attachRefs sets the references of each block the batch packs from the
// volume's child lists in tx, and counts the blocks stored without one.
func attachRefs(ctx context.Context, tx kvtx.Tx, batch *packBatch) error {
	rec := volume_kvtx.OpenRecords(tx)
	for _, put := range batch.puts {
		children, listed, err := rec.GetChildren(ctx, put.Ref)
		if err != nil {
			return errors.Wrap(err, "read block references")
		}
		if !listed {
			batch.untracked++
		}
		put.Refs = append(put.Refs, children...)
	}
	return nil
}

// collect reads the keys and data of the next batch from tx.
func (r *batchReader) collect(ctx context.Context, tx kvtx.Tx) (*packBatch, error) {
	// Start after the last key examined.
	it := tx.Iterate(ctx, r.prefix, true, false)
	defer it.Close()
	var start []byte
	if r.after != nil {
		start = append(bytes.Clone(r.prefix), r.after...)
	}
	if err := it.Seek(start); err != nil {
		return nil, err
	}
	if start != nil && it.Valid() && bytes.Equal(it.Key(), start) {
		it.Next()
	}

	// Examine blocks until the batch is full.
	batch := &packBatch{frame: frame{kind: framePending}}
	var packed, examined int
	for it.Valid() && batch.bytes < uint64(r.packBytes) && packed < int(writer.DefaultMaxBlocksPerPack) && examined < maxExamined { //nolint:gosec // the policy is positive.
		key := bytes.Clone(it.Key()[len(r.prefix):])
		if r.end != nil && bytes.Compare(key, r.end) >= 0 {
			break
		}
		batch.through = key
		examined++
		if r.outside.Has(key) {
			batch.outside++
			it.Next()
			continue
		}

		// Pack a block whose data matches its hash, and record any other.
		data, err := it.ValueCopy(nil)
		if err != nil {
			return nil, err
		}
		ref := &block.BlockRef{}
		if ref.UnmarshalVT(key) != nil || ref.VerifyData(data, false) != nil {
			batch.entries = append(batch.entries, logEntry{key: key, bad: true})
			it.Next()
			continue
		}
		batch.entries = append(batch.entries, logEntry{key: key})
		batch.puts = append(batch.puts, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		batch.bytes += uint64(len(data))
		packed++
		it.Next()
	}
	return batch, it.Err()
}
