//go:build !tinygo

package packsweep

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/sync/errgroup"
)

// DefaultMoves is the packs the trash moves at once when the options set
// none. Each move is two renames on the store's server.
const DefaultMoves = 8

// moveRecordBatch is the moved packs whose records one transaction writes.
const moveRecordBatch = 256

// TrashOptions configures Trash.
type TrashOptions struct {
	// CoverOptions pulls the log and walks W before any pack moves.
	CoverOptions
	// Mover moves the packs' objects within the store's bucket.
	Mover block_store_s3.ObjectMover
	// Hider hides the packs' objects under delete markers, and brings them
	// back. The trash needs it for Method TrashHide, and the restore for
	// each pack whose record holds that method.
	Hider block_store_s3.ObjectHider
	// Method is how the trash sets aside each pack whose trash begins in
	// this run; a pack already trashing keeps the method its record holds.
	Method TrashMethod
	// Bucket is the store's bucket.
	Bucket string
	// Prefix starts the store's object keys.
	Prefix string
	// Moves is the packs moved at once. Zero uses DefaultMoves.
	Moves int
}

// TrashResult reports a trash step.
type TrashResult struct {
	// Moved is the packs whose entry and file this run set aside, moved to
	// the trash or hidden.
	Moved uint64
	// Hidden is the packs of Moved this run hid rather than moved.
	Hidden uint64
	// Lifted is the packs a read lifted since their pass, dropped from the
	// cycle.
	Lifted uint64
	// Gone is the packs the backend no longer holds, dropped from the cycle.
	Gone uint64
	// Missed is the trashing packs a read missed after their move, left for
	// restore.
	Missed uint64
	// Live is the packs holding a key some root reached since their pass:
	// the quarantined are lifted, and the trashing left for restore.
	Live uint64
	// Blocked is why no pack moved, empty when the log let them.
	Blocked string
}

// JSON describes the trash step.
func (r *TrashResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"moved", jsonUint(a, r.Moved)},
		jsonField{"hidden", jsonUint(a, r.Hidden)},
		jsonField{"lifted", jsonUint(a, r.Lifted)},
		jsonField{"gone", jsonUint(a, r.Gone)},
		jsonField{"missed", jsonUint(a, r.Missed)},
		jsonField{"live", jsonUint(a, r.Live)},
		jsonField{"blocked", a.NewString(r.Blocked)},
	)
}

// Trash sets each passed pack aside: it records the pack with opts.Method,
// has the backend mark it trashing, reads its miss record, then moves its
// entry and then its file to the trash, or hides them under delete markers,
// and records the move. A read that falls back to a trashing pack and finds
// it gone records a miss, which keeps the backend from removing it; the pack
// goes back by restore. A read of a hidden pack is served from the version
// under its marker and records the miss the same way. Every step repeats
// cleanly, so a run killed anywhere resumes: a move whose source is gone and
// whose destination is there counts as done, and a hide of a hidden key
// returns the version under it. A pack lifted between the status
// and the backend's call fails that call, and the next run drops it.
//
// A pass is not final. After reading the packs' states, Trash pulls the log
// and walks W, as the recheck does, and checks each pack again: a quarantined
// pack holding a key in L or W is lifted, and a trashing one is marked live
// and left for restore.
func Trash(ctx context.Context, opts *TrashOptions) (*TrashResult, error) {
	// Refuse a hide with no store to hide in.
	if opts.Method == TrashHide && opts.Hider == nil {
		return nil, errors.New("trash: the store cannot hide its objects")
	}

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
		case h.State == PackTrashing && p.Wait == MarkLive:
			res.Live++
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

	// Check the packs again against the log pulled now, and keep the live.
	begin, move, err = recheckTrash(ctx, opts, status, begin, move, res)
	if err != nil || res.Blocked != "" {
		return res, err
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

// recheckTrash checks the packs to begin and to move against the log pulled
// now, and returns those still removable. It lifts a live quarantined pack and
// drops its record, and marks a live trashing pack for restore. It sets
// res.Blocked, with nothing changed, when the log does not cover the cycle.
func recheckTrash(ctx context.Context, opts *TrashOptions, status *BackendStatus, begin, move []*PackRecord, res *TrashResult) ([]*PackRecord, []*PackRecord, error) {
	// Check every pack the step would act on.
	check := append(slices.Clip(begin), move...)
	ok, blocked, err := stillRemovable(ctx, &opts.CoverOptions, status, check)
	if err != nil || blocked != "" {
		res.Blocked = blocked
		return nil, nil, err
	}

	// Keep the removable, and split off the live by state.
	var keepBegin, keepMove, marked []*PackRecord
	var lift []string
	for i, p := range check {
		switch {
		case ok[i] && i < len(begin):
			keepBegin = append(keepBegin, p)
		case ok[i]:
			keepMove = append(keepMove, p)
		case i < len(begin):
			lift = append(lift, p.ID)
		default:
			p.Wait = MarkLive
			marked = append(marked, p)
		}
	}
	res.Live += uint64(len(lift) + len(marked))

	// Mark the live trashing packs, then lift the live quarantined ones and
	// drop their records.
	if err := writePackRecords(ctx, opts.Store, marked, nil); err != nil {
		return nil, nil, err
	}
	for batch := range slices.Chunk(lift, MaxBackendPacks) {
		if _, err := opts.Backend.Lift(ctx, batch); err != nil {
			return nil, nil, errors.Wrap(err, "lift the live packs")
		}
		if err := writePackRecords(ctx, opts.Store, nil, batch); err != nil {
			return nil, nil, err
		}
	}
	return keepBegin, keepMove, nil
}

// beginTrashBatch records a batch of passed packs as trashing, then has the
// backend mark them trashing.
func beginTrashBatch(ctx context.Context, opts *TrashOptions, batch []*PackRecord) error {
	// Record the packs and the trash method before the call. No object of
	// a pack moves before the backend marks it trashing, so a pack recorded
	// here by an earlier run takes this run's method.
	ids := make([]string, len(batch))
	for i, p := range batch {
		p.Stage, p.Method, ids[i] = StageTrash, opts.Method, p.ID
		p.EntryVersion, p.FileVersion = "", ""
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
	// Set aside each pack's entry, then its file.
	moved, moveErr := moveEach(ctx, opts, batch, trashPack)

	// Record the packs moved, even when another move failed.
	for _, p := range moved {
		p.Stage = StageMoved
		if p.Method == TrashHide {
			res.Hidden++
		}
	}
	if err := writePackRecords(ctx, opts.Store, moved, nil); err != nil {
		return err
	}
	res.Moved += uint64(len(moved))
	return moveErr
}

// moveEach runs step on each pack of batch, at most opts.Moves packs at once,
// and returns the packs whose step succeeded with the first failure. A step
// writes only its own pack's record fields.
func moveEach(ctx context.Context, opts *TrashOptions, batch []*PackRecord, step func(context.Context, *TrashOptions, *PackRecord) error) ([]*PackRecord, error) {
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
			if err := step(egCtx, opts, p); err != nil {
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

// trashPack sets the entry and then the file of pack p aside by its
// record's method, so the store lists no pack whose file is gone: it moves
// them to the trash, or hides them and records the versions it kept.
func trashPack(ctx context.Context, opts *TrashOptions, p *PackRecord) error {
	if p.Method != TrashHide {
		return movePack(ctx, opts, trashMoves(opts.Prefix, p.ID))
	}
	return hidePack(ctx, opts, p)
}

// restorePack brings the file and then the entry of pack p back by its
// record's method: it moves them back from the trash, or deletes the
// markers over the versions its hide kept.
func restorePack(ctx context.Context, opts *TrashOptions, p *PackRecord) error {
	if p.Method != TrashHide {
		return movePack(ctx, opts, restoreMoves(opts.Prefix, p.ID))
	}
	return unhidePack(ctx, opts, p)
}

// hidePack hides the entry and then the file of pack p under delete markers,
// recording on p the version each marker covers. Its only calls are
// HideObjectVersion's, which deletes no version; a run killed between them
// hides again, and a hide of a hidden key returns the version under it.
func hidePack(ctx context.Context, opts *TrashOptions, p *PackRecord) error {
	// Hide the entry, then the file, and record both versions.
	if opts.Hider == nil {
		return errors.Errorf("hide pack %s: the store cannot hide its objects", p.ID)
	}
	entry, err := opts.Hider.HideObjectVersion(ctx, opts.Bucket, block_store_s3.EntryObjectKey(opts.Prefix, p.ID))
	if err != nil {
		return errors.Wrap(err, "hide the entry of pack "+p.ID)
	}
	file, err := opts.Hider.HideObjectVersion(ctx, opts.Bucket, block_store_s3.PackObjectKey(opts.Prefix, p.ID))
	if err != nil {
		return errors.Wrap(err, "hide the file of pack "+p.ID)
	}
	p.EntryVersion, p.FileVersion = entry.Version, file.Version
	return nil
}

// unhidePack makes current again the versions of the file and then the entry
// of pack p that its hide kept, deleting only the markers over them. A record
// killed before the hide recorded its versions restores each key's newest
// content version, the one a pack key ever holds.
func unhidePack(ctx context.Context, opts *TrashOptions, p *PackRecord) error {
	// Restore the file, then the entry.
	if opts.Hider == nil {
		return errors.Errorf("unhide pack %s: the store cannot hide its objects", p.ID)
	}
	if err := opts.Hider.UnhideObjectVersion(ctx, opts.Bucket, block_store_s3.PackObjectKey(opts.Prefix, p.ID), p.FileVersion); err != nil {
		return errors.Wrap(err, "unhide the file of pack "+p.ID)
	}
	if err := opts.Hider.UnhideObjectVersion(ctx, opts.Bucket, block_store_s3.EntryObjectKey(opts.Prefix, p.ID), p.EntryVersion); err != nil {
		return errors.Wrap(err, "unhide the entry of pack "+p.ID)
	}
	return nil
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
