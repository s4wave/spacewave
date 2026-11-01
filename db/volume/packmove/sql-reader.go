package packmove

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// sqlListBlocks is the number of row keys one listing reads.
const sqlListBlocks = 4096

// sqlFetchBlocks is the number of blocks one data read fetches, so a batch
// stops near its byte budget.
const sqlFetchBlocks = 256

// SQLRows reads one block store's rows of a SQL block store, such as
// block_store_sql.Store.
type SQLRows interface {
	// ListHashes returns up to limit row keys after the key after and before
	// end, nil for no end, in key order. A row key is the marshaled hash of the block.
	ListHashes(ctx context.Context, after, end []byte, limit int) ([][]byte, error)
	// GetStoredBlockBatch returns each block with its refs, nil for a block
	// the store does not hold.
	GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, withData bool) ([]*block.StoredBlock, error)
}

// sqlBatchReader reads the SQL rows in key order, one pack's worth per batch,
// and passes the blocks the volume already holds, which Pack packs.
type sqlBatchReader struct {
	// rows is the SQL block store.
	rows SQLRows
	// src is the volume store, read for the blocks it holds.
	src kvtx.Store
	// prefix is the volume's block key prefix.
	prefix []byte
	// after is the last row key examined, or nil before the first batch.
	after []byte
	// end is the first row key past the range, or nil.
	end []byte
	// packBytes is the block data one batch holds.
	packBytes int
}

// next reads the batch after the last one, or returns nil after the last
// row. A batch's examined-through key is a row key. A batch stops after
// maxExamined rows, so a long run of rows the volume holds still advances the
// log.
func (r *sqlBatchReader) next(ctx context.Context) (*packBatch, error) {
	batch := &packBatch{frame: frame{kind: framePending}}
	for examined := 0; examined < maxExamined && batch.bytes < uint64(r.packBytes) && len(batch.puts) < int(writer.DefaultMaxBlocksPerPack); { //nolint:gosec // the policy is positive.
		// List the next rows.
		keys, err := r.rows.ListHashes(ctx, r.after, r.end, sqlListBlocks)
		if err != nil {
			return nil, err
		}
		if len(keys) == 0 {
			break
		}
		examined += len(keys)

		// Leave the blocks the volume holds to Pack.
		refs, err := r.missing(ctx, keys)
		if err != nil {
			return nil, err
		}

		// Read the rest a chunk at a time until the batch is full.
		r.after = keys[len(keys)-1]
		for start := 0; start < len(refs); start += sqlFetchBlocks {
			chunk := refs[start:min(start+sqlFetchBlocks, len(refs))]
			if err := r.add(ctx, batch, chunk); err != nil {
				return nil, err
			}
			if batch.bytes >= uint64(r.packBytes) || len(batch.puts) >= int(writer.DefaultMaxBlocksPerPack) { //nolint:gosec // the policy is positive.
				// Resume after the last block read; the volume check of
				// the rest repeats.
				r.after = chunk[len(chunk)-1].rowKey
				break
			}
		}
		batch.through = bytes.Clone(r.after)
	}
	if len(batch.through) == 0 {
		return nil, nil
	}
	return batch, nil
}

// sqlRef is a row's block reference and its row key.
type sqlRef struct {
	ref    *block.BlockRef
	rowKey []byte
}

// missing returns the rows of keys whose blocks the volume does not hold, in
// order, read in one snapshot.
func (r *sqlBatchReader) missing(ctx context.Context, keys [][]byte) ([]sqlRef, error) {
	// Decode each row key into a block reference.
	refs := make([]sqlRef, 0, len(keys))
	for _, key := range keys {
		h := &hash.Hash{}
		if err := h.UnmarshalVT(key); err != nil {
			return nil, errors.Wrap(err, "decode row key")
		}
		refs = append(refs, sqlRef{ref: &block.BlockRef{Hash: h}, rowKey: key})
	}

	// Keep those the volume lacks.
	var out []sqlRef
	err := readStable(ctx, r.src, func(tx kvtx.Tx) error {
		out = out[:0]
		for _, ref := range refs {
			key, err := ref.ref.MarshalVT()
			if err != nil {
				return err
			}
			held, err := tx.Exists(ctx, append(bytes.Clone(r.prefix), key...))
			if err != nil {
				return err
			}
			if !held {
				out = append(out, ref)
			}
		}
		return nil
	})
	return out, err
}

// add reads chunk's blocks into batch: a block whose data matches its hash is
// packed with the refs the row records, any other is recorded as bad.
func (r *sqlBatchReader) add(ctx context.Context, batch *packBatch, chunk []sqlRef) error {
	// Read the blocks with their data.
	refs := make([]*block.BlockRef, len(chunk))
	for i, ref := range chunk {
		refs[i] = ref.ref
	}
	stored, err := r.rows.GetStoredBlockBatch(ctx, refs, true)
	if err != nil {
		return err
	}

	// Pack each one whose data matches its hash.
	for i, ref := range refs {
		key, err := ref.MarshalVT()
		if err != nil {
			return err
		}
		row := stored[i]
		if row == nil || ref.VerifyData(row.Data, false) != nil {
			batch.entries = append(batch.entries, logEntry{key: key, bad: true})
			continue
		}
		if !row.RefsKnown {
			batch.untracked++
		}
		batch.entries = append(batch.entries, logEntry{key: key})
		batch.puts = append(batch.puts, &block.PutBatchEntry{Ref: ref, Data: row.Data, Refs: row.Refs, RefsKnown: row.RefsKnown})
		batch.bytes += uint64(len(row.Data))
	}
	return nil
}
