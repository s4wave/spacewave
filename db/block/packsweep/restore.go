//go:build !tinygo

package packsweep

import (
	"bytes"
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
)

// RestoreOptions configures Restore.
type RestoreOptions struct {
	// TrashOptions reach the scratch store, the backend and the store's
	// objects, as the trash does.
	TrashOptions
	// Packs names packs to restore besides the ones reads missed: a trashing
	// pack, or one the backend removed whose objects are still in the trash.
	Packs []string
}

// RestoreResult reports a restore step.
type RestoreResult struct {
	// Restored is the trashing packs this run moved back and registered
	// again.
	Restored uint64
	// Adopted is the removed packs this run moved back and registered anew.
	Adopted uint64
	// Dropped is the packs back in the reads whose records this run dropped:
	// those a restore killed before it dropped them registered, and those a
	// read lifted before their trash began.
	Dropped uint64
}

// JSON describes the restore step.
func (r *RestoreResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"restored", jsonUint(a, r.Restored)},
		jsonField{"adopted", jsonUint(a, r.Adopted)},
		jsonField{"dropped", jsonUint(a, r.Dropped)},
	)
}

// Restore returns packs from the trash to the store: each trashing pack a
// read missed, which the trash and the removal leave at StageTrash or
// StageMoved, and each pack opts.Packs names. It moves a pack's file, then its
// entry, back from the trash, has the backend register it again, a removed
// pack by adopting it with the keys its own index lists, and drops its record,
// so the pack leaves the cycle and a later one judges it again. Each step
// repeats cleanly: a move whose source is gone and whose destination is there
// counts as done, and a run killed after the backend's call finds the pack
// registered and drops its record.
func Restore(ctx context.Context, opts *RestoreOptions) (*RestoreResult, error) {
	// Take the packs at the trash, the removed ones only when named, and
	// check each named one is there.
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	var todo []*PackRecord
	for _, p := range records {
		if p.Stage == StageTrash || p.Stage == StageMoved || (p.Stage == StageRemoved && slices.Contains(opts.Packs, p.ID)) {
			todo = append(todo, p)
		}
	}
	for _, id := range opts.Packs {
		if !slices.ContainsFunc(todo, func(p *PackRecord) bool { return p.ID == id }) {
			return nil, errors.Errorf("restore pack %s: the cycle has not trashed it", id)
		}
	}
	res := &RestoreResult{}
	if len(todo) == 0 {
		return res, nil
	}

	// Sort the packs by their states.
	restore, adopt, err := sortRestore(ctx, opts, todo, res)
	if err != nil {
		return nil, err
	}

	// Restore the trashing packs, then adopt the removed, a batch at a time.
	for batch := range slices.Chunk(restore, MaxBackendPacks) {
		if err := restoreBatch(ctx, opts, batch, res); err != nil {
			return nil, err
		}
	}
	for batch := range slices.Chunk(adopt, MaxBackendAdopt) {
		if err := adoptBatch(ctx, opts, batch, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// sortRestore reads the states of the packs at the trash, drops the records
// of the registered, and returns the trashing packs to restore, those a read
// missed or opts.Packs names, and the removed packs opts.Packs names to adopt.
func sortRestore(ctx context.Context, opts *RestoreOptions, todo []*PackRecord, res *RestoreResult) ([]*PackRecord, []*PackRecord, error) {
	// Read the states.
	status, err := backendStatus(ctx, opts.Backend, todo)
	if err != nil {
		return nil, nil, err
	}

	// Sort each pack, refusing a named one no restore brings back.
	var restore, adopt []*PackRecord
	var drop []string
	for i, p := range todo {
		named := slices.Contains(opts.Packs, p.ID)
		switch h := status.Packs[i]; {
		case h.State == PackRegistered && !h.Absent:
			drop = append(drop, p.ID)
		case h.Absent && named && p.Stage >= StageMoved:
			adopt = append(adopt, p)
		case h.State == PackTrashing && !h.Absent && (named || !h.Event.IsZero()):
			restore = append(restore, p)
		case named:
			return nil, nil, errors.Errorf("restore pack %s: it is at %s in state %d", p.ID, p.Stage, h.State)
		}
	}

	// Drop the records of the registered.
	if err := writePackRecords(ctx, opts.Store, nil, drop); err != nil {
		return nil, nil, err
	}
	res.Dropped = uint64(len(drop))
	return restore, adopt, nil
}

// restoreBatch moves a batch of trashing packs back from the trash, then has
// the backend register those that moved and drops their records.
func restoreBatch(ctx context.Context, opts *RestoreOptions, batch []*PackRecord, res *RestoreResult) error {
	// Move each pack's file, then its entry.
	moved, moveErr := moveEach(ctx, &opts.TrashOptions, batch, restoreMoves)
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

// adoptBatch moves a batch of removed packs back from the trash, reads the
// entry and the index of each that moved, has the backend adopt them, and
// drops their records.
func adoptBatch(ctx context.Context, opts *RestoreOptions, batch []*PackRecord, res *RestoreResult) error {
	// Move each pack's file, then its entry.
	moved, moveErr := moveEach(ctx, &opts.TrashOptions, batch, restoreMoves)
	if len(moved) == 0 {
		return moveErr
	}

	// Read each pack's entry and keys, and adopt them.
	packs := make([]AdoptedPack, len(moved))
	ids := make([]string, len(moved))
	for i, p := range moved {
		ap, err := readAdopted(ctx, &opts.TrashOptions, p.ID)
		if err != nil {
			return err
		}
		packs[i], ids[i] = ap, p.ID
	}
	if err := opts.Backend.Adopt(ctx, packs); err != nil {
		return errors.Wrap(err, "adopt")
	}

	// Drop their records, then report any failed move.
	if err := writePackRecords(ctx, opts.Store, nil, ids); err != nil {
		return err
	}
	res.Adopted += uint64(len(ids))
	return moveErr
}

// readAdopted reads the entry of pack id from the store and the keys its
// index lists, from the end of its file.
func readAdopted(ctx context.Context, opts *TrashOptions, id string) (AdoptedPack, error) {
	// Read the entry.
	body, err := opts.Mover.GetObject(ctx, opts.Bucket, block_store_s3.EntryObjectKey(opts.Prefix, id))
	if err != nil {
		return AdoptedPack{}, errors.Wrap(err, "read the entry of pack "+id)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return AdoptedPack{}, errors.Wrap(err, "read the entry of pack "+id)
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(data); err != nil || entry.GetId() != id {
		return AdoptedPack{}, errors.Errorf("the entry object of pack %s is not its entry", id)
	}

	// Read the keys the index lists.
	key := block_store_s3.PackObjectKey(opts.Prefix, id)
	read := func(ctx context.Context, off int64, length int) ([]byte, error) {
		return opts.Mover.GetObjectRange(ctx, opts.Bucket, key, off, length)
	}
	tail, err := packfile.ReadIndexTail(ctx, entry.GetSizeBytes(), packfile.IndexTailReadSize(entry.GetBlockCount()), read)
	if err != nil {
		return AdoptedPack{}, errors.Wrap(err, "read the index of pack "+id)
	}
	var keys [][]byte
	err = packfile.ScanIndexTail(tail, entry.GetSizeBytes(), func(e *kvfile.IndexEntry) error {
		keys = append(keys, bytes.Clone(e.GetKey()))
		return nil
	})
	if err != nil {
		return AdoptedPack{}, errors.Wrap(err, "read the index of pack "+id)
	}
	return AdoptedPack{Entry: entry, Keys: keys}, nil
}

// restoreMoves are the moves of pack id back from the trash: its file, then
// its entry, so the store lists no pack whose file is gone.
func restoreMoves(prefix, id string) [][2]string {
	return [][2]string{
		{block_store_s3.TrashPackKey(prefix, id), block_store_s3.PackObjectKey(prefix, id)},
		{block_store_s3.TrashEntryKey(prefix, id), block_store_s3.EntryObjectKey(prefix, id)},
	}
}
