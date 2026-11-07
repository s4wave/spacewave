//go:build !tinygo

package packsweep

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sync/errgroup"
)

// DefaultMoves is the packs the trash moves at once when the options set
// none. Each move is two renames on the store's server.
const DefaultMoves = 8

// moveRecordBatch is the moved packs whose records one transaction writes.
const moveRecordBatch = 256

// TrashOptions configures Trash.
type TrashOptions struct {
	// Store is the scratch store.
	Store kvtx.Store
	// Backend is the backend's side of the cycle.
	Backend Backend
	// Mover moves the packs' objects within the store's bucket.
	Mover block_store_s3.ObjectMover
	// Bucket is the store's bucket.
	Bucket string
	// Prefix starts the store's object keys.
	Prefix string
	// Moves is the packs moved at once. Zero uses DefaultMoves.
	Moves int
}

// TrashResult reports a trash step.
type TrashResult struct {
	// Moved is the packs whose entry and file this run moved to the trash.
	Moved uint64
	// Lifted is the packs a read lifted since their pass, dropped from the
	// cycle.
	Lifted uint64
	// Gone is the packs the backend no longer holds, dropped from the cycle.
	Gone uint64
	// Missed is the trashing packs a read missed after their move, left for
	// restore.
	Missed uint64
}

// JSON describes the trash step.
func (r *TrashResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"moved", jsonUint(a, r.Moved)},
		jsonField{"lifted", jsonUint(a, r.Lifted)},
		jsonField{"gone", jsonUint(a, r.Gone)},
		jsonField{"missed", jsonUint(a, r.Missed)},
	)
}

// Trash moves each passed pack to the trash: it records the pack, has the
// backend mark it trashing, reads its miss record, then moves its entry and
// then its file to the trash and records the move. A read that falls back to
// a trashing pack and finds it gone records a miss, which keeps the backend
// from removing it; the pack goes back by restore. Every step repeats
// cleanly, so a run killed anywhere resumes: a move whose source is gone and
// whose destination is there counts as done. A pack lifted between the status
// and the backend's call fails that call, and the next run drops it.
func Trash(ctx context.Context, opts *TrashOptions) (*TrashResult, error) {
	// Take the passed and trashing packs, and read their states.
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	var todo []*PackRecord
	for _, p := range records {
		if p.Stage == StagePass || p.Stage == StageTrash {
			todo = append(todo, p)
		}
	}
	res := &TrashResult{}
	if len(todo) == 0 {
		return res, nil
	}
	status, err := backendStatus(ctx, opts.Backend, todo)
	if err != nil {
		return nil, err
	}

	// Sort each pack by its state: begin trashing the quarantined, move the
	// trashing with no miss, and drop the lifted and the gone.
	var begin, move []*PackRecord
	var del []string
	for i, p := range todo {
		switch h := status.Packs[i]; {
		case h.Absent:
			del = append(del, p.ID)
			res.Gone++
		case h.State == PackRegistered:
			del = append(del, p.ID)
			res.Lifted++
		case h.State == PackTrashing && !h.Event.IsZero():
			res.Missed++
		case h.State == PackTrashing:
			move = append(move, p)
		case h.State == PackQuarantined:
			begin = append(begin, p)
		default:
			return nil, errors.Errorf("pack %s is in state %d with a trash record", p.ID, h.State)
		}
	}
	if err := writePackRecords(ctx, opts.Store, nil, del); err != nil {
		return nil, err
	}

	// Begin trashing a batch at a time, then move every trashing pack.
	for batch := range slices.Chunk(begin, MaxBackendPacks) {
		if err := beginTrashBatch(ctx, opts, batch); err != nil {
			return nil, err
		}
		move = append(move, batch...)
	}
	for batch := range slices.Chunk(move, moveRecordBatch) {
		if err := moveBatch(ctx, opts, batch, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// beginTrashBatch records a batch of passed packs as trashing, then has the
// backend mark them trashing.
func beginTrashBatch(ctx context.Context, opts *TrashOptions, batch []*PackRecord) error {
	// Record the packs before the call.
	ids := make([]string, len(batch))
	for i, p := range batch {
		p.Stage, ids[i] = StageTrash, p.ID
	}
	if err := writePackRecords(ctx, opts.Store, batch, nil); err != nil {
		return err
	}

	// Mark them trashing, and check the backend did.
	holds, err := opts.Backend.BeginTrash(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "begin the trash")
	}
	if len(holds) != len(ids) {
		return errors.Errorf("the backend answered %d of %d packs", len(holds), len(ids))
	}
	for i, h := range holds {
		if h.ID != ids[i] || h.State != PackTrashing {
			return errors.Errorf("pack %s is in state %d after the trash began", ids[i], h.State)
		}
	}
	return nil
}

// moveBatch moves a batch of trashing packs to the trash, at most
// opts.Moves at once, then records them moved.
func moveBatch(ctx context.Context, opts *TrashOptions, batch []*PackRecord, res *TrashResult) error {
	// Move each pack's entry, then its file.
	moved, moveErr := moveEach(ctx, opts, batch, trashMoves)

	// Record the packs moved, even when another move failed.
	for _, p := range moved {
		p.Stage = StageMoved
	}
	if err := writePackRecords(ctx, opts.Store, moved, nil); err != nil {
		return err
	}
	res.Moved += uint64(len(moved))
	return moveErr
}

// moveEach makes the moves of each pack of batch, at most opts.Moves packs at
// once and each pack's moves in order, and returns the packs whose moves all
// succeeded with the first failure.
func moveEach(ctx context.Context, opts *TrashOptions, batch []*PackRecord, moves func(prefix, id string) [][2]string) ([]*PackRecord, error) {
	// Bound the packs moved at once.
	limit := opts.Moves
	if limit <= 0 {
		limit = DefaultMoves
	}
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(limit)
	var mtx sync.Mutex
	var moved []*PackRecord

	// Move each pack, keeping those that moved.
	for _, p := range batch {
		eg.Go(func() error {
			// Move the pack, then keep it.
			if err := movePack(egCtx, opts, moves(opts.Prefix, p.ID)); err != nil {
				return err
			}
			mtx.Lock()
			moved = append(moved, p)
			mtx.Unlock()
			return nil
		})
	}
	err := eg.Wait()
	return moved, err
}

// trashMoves are the moves of pack id to the trash: its entry, then its file,
// so the store lists no pack whose file is gone.
func trashMoves(prefix, id string) [][2]string {
	return [][2]string{
		{block_store_s3.EntryObjectKey(prefix, id), block_store_s3.TrashEntryKey(prefix, id)},
		{block_store_s3.PackObjectKey(prefix, id), block_store_s3.TrashPackKey(prefix, id)},
	}
}

// movePack makes moves in order, each from its first key to its second.
func movePack(ctx context.Context, opts *TrashOptions, moves [][2]string) error {
	for _, m := range moves {
		if err := opts.Mover.MoveObject(ctx, opts.Bucket, m[0], m[1]); err != nil {
			return errors.Wrapf(err, "move %s to %s", m[0], m[1])
		}
	}
	return nil
}
