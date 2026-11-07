//go:build !tinygo

package packsweep

import (
	"context"
	"slices"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// RestoreResult reports a restore step.
type RestoreResult struct {
	// Restored is the missed packs this run moved back and registered again.
	Restored uint64
	// Dropped is the packs back in the reads whose records this run dropped:
	// those a restore killed before it dropped them registered, and those a
	// read lifted before their trash began.
	Dropped uint64
}

// JSON describes the restore step.
func (r *RestoreResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"restored", jsonUint(a, r.Restored)},
		jsonField{"dropped", jsonUint(a, r.Dropped)},
	)
}

// Restore returns each trashing pack a read missed to the store: it moves the
// pack's file, then its entry, back from the trash, has the backend register
// the pack again, and drops its record, so the pack leaves the cycle and a
// later one judges it again. The trash and the removal leave such a pack at
// StageTrash or StageMoved. Each step repeats cleanly: a move whose source is
// gone and whose destination is there counts as done, and a run killed after
// the backend's call finds the pack registered and drops its record.
func Restore(ctx context.Context, opts *TrashOptions) (*RestoreResult, error) {
	// Take the packs at the trash and read their states.
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	var todo []*PackRecord
	for _, p := range records {
		if p.Stage == StageTrash || p.Stage == StageMoved {
			todo = append(todo, p)
		}
	}
	res := &RestoreResult{}
	if len(todo) == 0 {
		return res, nil
	}
	status, err := backendStatus(ctx, opts.Backend, todo)
	if err != nil {
		return nil, err
	}

	// Restore the missed packs, and drop the registered; leave the rest to
	// the trash and the removal.
	var restore []*PackRecord
	var drop []string
	for i, p := range todo {
		switch h := status.Packs[i]; {
		case h.Absent:
		case h.State == PackRegistered:
			drop = append(drop, p.ID)
		case h.State == PackTrashing && !h.Event.IsZero():
			restore = append(restore, p)
		}
	}
	if err := writePackRecords(ctx, opts.Store, nil, drop); err != nil {
		return nil, err
	}
	res.Dropped = uint64(len(drop))

	// Restore a batch at a time.
	for batch := range slices.Chunk(restore, MaxBackendPacks) {
		if err := restoreBatch(ctx, opts, batch, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// restoreBatch moves a batch of missed packs back from the trash, then has
// the backend register those that moved and drops their records.
func restoreBatch(ctx context.Context, opts *TrashOptions, batch []*PackRecord, res *RestoreResult) error {
	// Move each pack's file, then its entry.
	moved, moveErr := moveEach(ctx, opts, batch, restoreMoves)
	if len(moved) == 0 {
		return moveErr
	}

	// Register the packs that moved, and check the backend did.
	ids := make([]string, len(moved))
	for i, p := range moved {
		ids[i] = p.ID
	}
	holds, err := opts.Backend.Restore(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "restore")
	}
	if len(holds) != len(ids) {
		return errors.Errorf("the backend answered %d of %d packs", len(holds), len(ids))
	}
	for i, h := range holds {
		if h.ID != ids[i] || h.Absent || h.State != PackRegistered {
			return errors.Errorf("pack %s is in state %d after its restore", ids[i], h.State)
		}
	}

	// Drop their records, then report any failed move.
	if err := writePackRecords(ctx, opts.Store, nil, ids); err != nil {
		return err
	}
	res.Restored += uint64(len(ids))
	return moveErr
}

// restoreMoves are the moves of pack id back from the trash: its file, then
// its entry, so the store lists no pack whose file is gone.
func restoreMoves(prefix, id string) [][2]string {
	return [][2]string{
		{block_store_s3.TrashPackKey(prefix, id), block_store_s3.PackObjectKey(prefix, id)},
		{block_store_s3.TrashEntryKey(prefix, id), block_store_s3.EntryObjectKey(prefix, id)},
	}
}
