package provider_spacewave

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
)

// pendingUploadMutationLimit bounds two records per block plus the summary.
// This fits the OPFS engine's 4096-record transaction contract.
const pendingUploadMutationLimit = 1024

// pendingUploadStateKey stores the summary committed with every queue mutation.
const pendingUploadStateKey = "sync/pending"

// pendingUploadOrderPrefix stores complete blocks by insertion sequence.
const pendingUploadOrderPrefix = "dirty-order/"

// readPendingUploadState reads committed accounting, or a new empty queue.
// An unaccounted existing queue is left intact and reported as an error.
func readPendingUploadState(ctx context.Context, tx kvtx.Tx) (*PendingUploadState, error) {
	// Read the durable summary without scanning the pending records.
	data, found, err := tx.Get(ctx, []byte(pendingUploadStateKey))
	if err != nil {
		return nil, err
	}
	state := &PendingUploadState{}
	if found {
		if err := state.UnmarshalVT(data); err != nil {
			return nil, errors.Wrap(err, "decode pending upload state")
		}
		return state, nil
	}

	// Refuse to silently discard pending bytes from an incompatible saved store.
	iter := tx.Iterate(ctx, []byte("dirty/"), true, false)
	defer iter.Close()
	if iter.Next() {
		return nil, errors.New("pending upload records have no queue summary")
	}
	return state, iter.Err()
}

// writePendingUploadState records the summary in the marker transaction.
func writePendingUploadState(ctx context.Context, tx kvtx.Tx, state *PendingUploadState) error {
	data, err := state.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, []byte(pendingUploadStateKey), data)
}

// markPendingUploads inserts absent blocks and their sequence index in tx.
// The returned state becomes visible only after the caller commits tx.
func markPendingUploads(ctx context.Context, tx kvtx.Tx, marks []block_store_writeback.Mark) (*PendingUploadState, error) {
	// Load the summary from the same transaction that enforces hash uniqueness.
	state, err := readPendingUploadState(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, mark := range marks {
		key := []byte("dirty/" + mark.Hash.MarshalString())
		found, err := tx.Exists(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			continue
		}

		// Assign an immutable position so a flush can capture a finite cutoff.
		state.LastSequence++
		entry := &PendingUploadBlock{Sequence: state.GetLastSequence(), SizeBytes: mark.Size, Hash: mark.Hash.MarshalString()}
		data, err := entry.MarshalVT()
		if err != nil {
			return nil, err
		}
		if err := tx.Set(ctx, key, nil); err != nil {
			return nil, err
		}
		if err := tx.Set(ctx, pendingUploadOrderKey(entry.GetSequence()), data); err != nil {
			return nil, err
		}
		if state.GetCount() == 0 {
			state.PendingSinceNanos = time.Now().UnixNano()
		}
		state.Count++
		state.SizeBytes += mark.Size
	}

	// Keep counters and the original deadline atomic with the indexed records.
	return state, writePendingUploadState(ctx, tx, state)
}

// acknowledgePendingUploads removes only the acquired insertion of each block.
// Repeated acknowledgements cannot remove a later insertion of the same hash.
func acknowledgePendingUploads(ctx context.Context, tx kvtx.Tx, blocks []dirtyCandidate) (*PendingUploadState, error) {
	// Account from the actual durable records, not a caller's cached byte total.
	state, err := readPendingUploadState(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, block := range blocks {
		orderKey := pendingUploadOrderKey(block.sequence)
		data, found, err := tx.Get(ctx, orderKey)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		entry := &PendingUploadBlock{}
		if err := entry.UnmarshalVT(data); err != nil {
			return nil, err
		}
		if entry.GetSequence() != block.sequence || entry.GetHash() != block.hash.MarshalString() {
			return nil, errors.New("pending upload record does not match acquired block")
		}
		if err := tx.Delete(ctx, block.key); err != nil {
			return nil, err
		}
		if err := tx.Delete(ctx, orderKey); err != nil {
			return nil, err
		}
		state.Count--
		state.SizeBytes -= entry.GetSizeBytes()
	}

	// Emptying a queue clears its deadline but never reuses sequence positions.
	if state.GetCount() == 0 {
		state.PendingSinceNanos = 0
	}
	return state, writePendingUploadState(ctx, tx, state)
}

// pendingUploadOrderKey encodes the queue's monotonically ordered position.
func pendingUploadOrderKey(sequence uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte(pendingUploadOrderPrefix), sequence)
}
