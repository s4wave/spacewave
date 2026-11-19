package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// The streaming audit walk checks a Volume's counts and finds its holes while
// writers and collection passes keep running, holding in memory only the
// slice it is checking. It reads the Volume in short views and keeps its
// edges, candidates and progress in a scratch store of its own.
//
// Invariants, which the TLA+ model AuditWalk.tla checks, except the wide
// block's mark and the logs not netted yet, which it does not model:
//
//   - The walk never writes the Volume. It reads it through WalkStore, which
//     has no write, and writes only the scratch store.
//   - An edge <<child, namer>> is written for each root and for each child of
//     each list, as tallyHolders counts them. The check counts only the
//     edges still valid in the view that checks the child: the root still
//     names it, or the parent still has its list. That count is a lower
//     bound on the child's true namers in that view, so holders plus the
//     logs not netted yet below it is a true undercount (no false positive).
//     A namer the edges phase passed before it took up the child can hide
//     one unit until the next walk.
//   - A wide block, one with more than walkSliceEdges edges, is checked in
//     views of its own, walkSliceEdges edges to a view, and its valid edges
//     are summed across them. The sum bounds the true namers of the last
//     view only if no namer changed between the views, so each view reads a
//     WalkMark: the whole liveness state record, and the block's own
//     listing, holders, log sum and release pending. In one adoption a namer
//     leaves or joins only by a log page (UpSeq and DownSeq), by netting
//     (UpCursor, and the down pages it deletes, which the log sum moves
//     with), or by a free's paged release (Release, and the block's holders
//     once the release pages it). An unadopted Volume's Lands write no logs,
//     and each adoption names a new AdoptionRun, so a cycle through
//     unadopted moves the mark. Whether a parent counts through its old list
//     turns on the generation, in the state, and on its run entry, which
//     leaves the run only by the sweep or a rebirth, each of which logs the
//     downs of the children it releases. The undercount check runs only when
//     every view read the first view's mark; otherwise the block counts as
//     Wide and its undercount goes unchecked. The hole test needs no mark:
//     one valid edge in any view names the block, and the confirm phase
//     rereads every candidate.
//   - The log entries not netted yet that the check adds to the holders are
//     exactly logTail's for the slice's blocks in the same view: the walk
//     copies each log page's entries into scratch once, as it found them,
//     and a page's keys never change once written; netting takes whole
//     pages in sequence order, so a copy is netted exactly once its sequence
//     falls below its log's first page left, the up cursor or the first
//     down page, which the check reads in its view and applies to the
//     copies. It copies only the entries of blocks not checked yet, and
//     keeps those of the slice it checks only while that slice is
//     unfinished. Each check copies at most walkSliceEdges entries and sums
//     at most walkBacklogEntries entries in all: those past its copy, and the
//     slice's copies in scratch not netted yet, which grow with netting's
//     lag. A view's work is so bounded whatever a writer's rate or
//     netting's stall. A view that finds more left only copies, releases the
//     view and waits walkBehindWait before the next; after walkBehindViews
//     such views in a row the walk ends with ErrWalkBehind, keeping its
//     scratch to resume. A stopped copy nets nothing, so one whose logs not
//     netted yet name a slice's blocks more than walkBacklogEntries times
//     stays behind.
//   - A named block with neither bytes, a list nor a hole record is a
//     candidate, not a hole: a writer may store a parent ahead of its child.
//     It is reported only if it is still named and absent once the confirm
//     delay has passed, and then as unconfirmed, with the class of the roots
//     that reach it. A writer that holds a parent ahead of its child for
//     longer than the delay makes an unconfirmed candidate that is no hole.
//   - A scratch commit carries the progress record only in the last commit of
//     a slice, and every other scratch write is idempotent, so a restart of
//     the same run redoes at most one slice and counts it once. Every scratch
//     commit first checks that the progress record names this run, so a
//     second runner on the same store fails instead of mixing its edges in.
//     A run older than walkRunAge starts over: its edges miss every namer
//     written since, and its clock for a hole's class is a day old.
//
// One view reads at most: walkSliceEdges log entries to copy, with the
// page that passes them; walkBacklogEntries entries past the copy and among
// the slice's copies in scratch; a binary search of the paged release and
// the release pages that overlap the slice's keys; and, for each of at most
// walkSliceChildren blocks, four point reads, with walkSliceEdges edges of
// one point read each, the root or the parent's list. The lists phase
// writes walkSliceEdges edges and then the rest of the list it stopped in,
// which the largest block bounds. Any process that holds the Volume open
// may run the walk beside its writers, since it only reads; the debug
// command walks a stopped copy, because a second process may not open a
// live Volume.

// Scratch record kinds of the walk. Every key begins with its kind.
const (
	// walkEdge keys an edge by the block named, then the namer: walkRootNamer
	// and the root's key after its kind, or walkListNamer and the parent's
	// block key. A block key fixes its own length, so the child and the namer
	// split without a separator. A root edge's value is the root's lease
	// expiry when the walk read it, big-endian Unix milliseconds, zero for
	// none; a list edge's is empty.
	walkEdge byte = 'e'
	// walkCandidate keys a hole candidate by its block key. Its value is when
	// the check found it, big-endian Unix milliseconds.
	walkCandidate byte = 'c'
	// walkProgressKind is the one WalkProgress record.
	walkProgressKind byte = 'p'
	// walkTail keys a copy of one entry of an up or down log page by the
	// block it names, then walkUp or walkDown, the page's sequence and the
	// entry's index in it, both big-endian uint64. Its value is empty.
	walkTail byte = 't'
)

// The log bytes of a walkTail key.
const (
	// walkDown is a down entry: -1 to the block's holders.
	walkDown byte = 0
	// walkUp is an up entry: +1 to the block's holders.
	walkUp byte = 1
)

// The namer kinds of an edge.
const (
	// walkRootNamer begins the namer of an edge from a named root.
	walkRootNamer byte = 0
	// walkListNamer begins the namer of an edge from a parent's child list.
	walkListNamer byte = 1
)

// The walk's bounds.
const (
	// walkCommitRecords bounds the records one scratch commit writes. Each
	// lands at a random place in key order and dirties about one leaf page,
	// so a commit stays well under the 128 WAL frames past which back to back
	// commits keep a SQLite log from checkpointing.
	walkCommitRecords = 48
	// walkSliceEdges bounds the edges one view of the lists yields; the list
	// that passes it finishes in the same view. sliceRecords bounds the roots
	// and lists one view reads.
	walkSliceEdges = 4096
	// walkSliceChildren bounds the named blocks one check view checks.
	walkSliceChildren = 256
	// walkConfirmSlice bounds the candidates one confirm view rereads.
	walkConfirmSlice = 64
	// walkClassBound bounds the blocks the class walk of one hole visits.
	walkClassBound = 4096
	// walkBacklogEntries bounds the log entries one check view sums past its
	// copy; a view that finds more is behind and only copies.
	walkBacklogEntries = 65536
	// walkBehindViews bounds the views in a row that find the walk behind
	// before it gives up with ErrWalkBehind.
	walkBehindViews = 256
	// walkBehindWait is the wait after a view that found the walk behind,
	// which leaves the store to its writers before the next copy.
	walkBehindWait = 100 * time.Millisecond
	// walkRunAge bounds the age of a run that resumes; an older run starts
	// over. Only an open checks it, since only a run that stopped can sit
	// idle; a run that keeps going ages with its own work. Age loses
	// detection and never reports falsely: an edge older than a namer misses
	// it, which lowers the bound, and a lease is held past an older start
	// longer, so a hole is classed HELD more often, not less.
	walkRunAge = 24 * time.Hour
	// walkClearRecords bounds the records one commit of a restart deletes.
	// They lie together in key order, so a commit dirties few pages.
	walkClearRecords = 4096
)

// ErrWalkTaken reports a scratch store another run of the walk owns.
var ErrWalkTaken = errors.New("audit walk scratch store belongs to another run")

// ErrWalkBehind reports a walk whose copy of the logs stayed behind a
// writer for walkBehindViews views in a row. Its scratch store holds the copy
// so far, so a later run of the same name resumes it.
var ErrWalkBehind = errors.New("audit walk fell behind the logs' writers")

// errBadWalkRecord reports a scratch record the walk did not write.
var errBadWalkRecord = errors.New("audit walk scratch record is malformed")

// WalkStore is what the streaming audit reads of a Volume: views of its
// liveness records and whether it holds a block's bytes. It has no write.
type WalkStore interface {
	// View runs fn in one read snapshot of the liveness records.
	View(ctx context.Context, fn func(rec *Records) error) error
	// BlockStored reports whether the Volume holds the bytes of ref.
	BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error)
}

// Walk runs the streaming audit of store, keeping its records in scratch, as
// the invariants above describe, and returns once the run is done. scratch
// must be a store of the walk's own, empty or holding this run's progress;
// the caller removes it after. The walk keeps in memory one slice: at most
// walkSliceChildren named blocks and their log deltas, walkSliceEdges
// edges, or walkClassBound blocks of one hole's class walk. Each check view
// reads the logs past the walk's last copy of them, so a long tail slows
// the walk, never a writer.
func Walk(ctx context.Context, store WalkStore, scratch kvtx.Store, opts WalkOptions) (*WalkResult, error) {
	w, err := openWalk(ctx, store, scratch, opts)
	if err != nil {
		return nil, err
	}
	for {
		switch w.prog.GetPhase() {
		case WalkPhase_WALK_PHASE_ROOTS:
			err = w.roots(ctx)
		case WalkPhase_WALK_PHASE_LISTS:
			err = w.lists(ctx)
		case WalkPhase_WALK_PHASE_CHECK:
			err = w.check(ctx)
		case WalkPhase_WALK_PHASE_CONFIRM:
			err = w.confirm(ctx)
		case WalkPhase_WALK_PHASE_DONE:
			w.res.Counts = w.prog.GetCounts()
			w.res.Started = time.UnixMilli(w.prog.GetStartedUnixMilli())
			w.res.Resumes = w.prog.GetResumes()
			return w.res, w.view(ctx, func(rec *Records) error {
				state, err := rec.GetState(ctx)
				w.res.Adoption = state.GetAdoption()
				return err
			})
		default:
			err = errBadWalkRecord
		}
		if err != nil {
			return nil, err
		}
	}
}

// walker is one run of the walk.
type walker struct {
	// store is the Volume.
	store WalkStore
	// scratch is the walk's own store.
	scratch kvtx.Store
	// opts are the walk's options, with their defaults set.
	opts WalkOptions
	// prog is the run's progress as the next commit writes it.
	prog *WalkProgress
	// res collects the samples.
	res *WalkResult
	// behind counts the check views in a row that found the walk behind.
	behind int
}

// walkRecord is one scratch record to write.
type walkRecord struct {
	key, value []byte
}

// openWalk resumes this run's progress from scratch, or begins the run.
func openWalk(ctx context.Context, store WalkStore, scratch kvtx.Store, opts WalkOptions) (*walker, error) {
	// Set the defaults.
	if opts.Run == "" {
		return nil, errors.New("audit walk needs a run name")
	}
	if opts.Delay == 0 {
		opts.Delay = DefaultGrace
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Wait == nil {
		opts.Wait = waitTimer
	}
	w := &walker{
		store:   store,
		scratch: scratch,
		opts:    opts,
		res:     &WalkResult{LongestView: make(map[WalkPhase]time.Duration)},
	}

	// Resume the run's progress, unless it is too old. A progress record
	// this build cannot read, such as one an older build left, names no run
	// it can resume or refuse, so the walk drops it and starts over.
	prog, err := w.readProgress(ctx)
	if errors.Is(err, errBadWalkRecord) {
		if err := w.dropProgress(ctx); err != nil {
			return nil, err
		}
		if err := w.clear(ctx); err != nil {
			return nil, err
		}
		w.res.Restarted = true
		prog, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	if prog != nil {
		if prog.GetRun() != opts.Run {
			return nil, ErrWalkTaken
		}
		if prog.Counts == nil {
			prog.Counts = &WalkCounts{}
		}
		w.prog = prog
		if opts.Now().Sub(time.UnixMilli(prog.GetStartedUnixMilli())) <= walkRunAge {
			w.prog.Resumes++
			return w, w.write(ctx, nil)
		}
		if err := w.clear(ctx); err != nil {
			return nil, err
		}
		w.res.Restarted = true
	}

	// Otherwise begin it.
	w.prog = &WalkProgress{
		Run:              opts.Run,
		Phase:            WalkPhase_WALK_PHASE_ROOTS,
		StartedUnixMilli: opts.Now().UnixMilli(),
		Counts:           &WalkCounts{},
	}
	return w, w.write(ctx, nil)
}

// clear deletes the run's edges, candidates and log copies from
// scratch, walkClearRecords to a commit, leaving its progress record so a
// second runner is still refused.
func (w *walker) clear(ctx context.Context) error {
	for _, kind := range []byte{walkEdge, walkCandidate, walkTail} {
		for n := walkClearRecords; n == walkClearRecords; {
			err := w.update(ctx, func(tx kvtx.Tx) error {
				// Take the next keys of the kind, then delete them.
				var keys [][]byte
				err := NewRecords(tx, nil).scan(ctx, []byte{kind}, nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
					keys = append(keys, walkKey(kind, key))
					return len(keys) < walkClearRecords, nil
				})
				if err != nil {
					return err
				}
				n = len(keys)
				for _, key := range keys {
					if err := tx.Delete(ctx, key); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// waitTimer waits for d or until ctx ends.
func waitTimer(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// readProgress returns the progress record in scratch, nil when none.
func (w *walker) readProgress(ctx context.Context) (*WalkProgress, error) {
	tx, err := w.scratch.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return getProgress(ctx, tx)
}

// getProgress reads the progress record in tx, nil when none.
func getProgress(ctx context.Context, tx kvtx.TxOps) (*WalkProgress, error) {
	// Read and decode the record; one that does not decode is malformed.
	value, found, err := tx.Get(ctx, []byte{walkProgressKind})
	if err != nil || !found {
		return nil, err
	}
	prog := &WalkProgress{}
	if err := prog.UnmarshalVT(value); err != nil {
		return nil, errors.Wrapf(errBadWalkRecord, "progress: %v", err)
	}
	return prog, nil
}

// dropProgress deletes a progress record this build cannot read.
func (w *walker) dropProgress(ctx context.Context) error {
	// Delete the record in its own commit, without the owner check, which
	// would read it.
	tx, err := w.scratch.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, []byte{walkProgressKind}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// write commits recs to scratch, walkCommitRecords at a time, with the
// progress in the last commit. Each commit first checks that the progress
// in scratch names this run.
func (w *walker) write(ctx context.Context, recs []walkRecord) error {
	for {
		n := min(len(recs), walkCommitRecords)
		last := n == len(recs)
		err := w.update(ctx, func(tx kvtx.Tx) error {
			// Write the slice's records.
			for _, r := range recs[:n] {
				if err := tx.Set(ctx, r.key, r.value); err != nil {
					return err
				}
			}
			if !last {
				return nil
			}

			// Write the progress with the last.
			value, err := w.prog.MarshalVT()
			if err != nil {
				return err
			}
			return tx.Set(ctx, []byte{walkProgressKind}, value)
		})
		if err != nil || last {
			return err
		}
		recs = recs[n:]
	}
}

// update runs fn in one scratch write transaction that this run owns, and
// commits it.
func (w *walker) update(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	// Open the transaction and check the owner.
	tx, err := w.scratch.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	prog, err := getProgress(ctx, tx)
	if err != nil {
		return err
	}
	if prog != nil && prog.GetRun() != w.prog.GetRun() {
		return ErrWalkTaken
	}

	// Write and commit.
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// view runs fn in one view of the Volume, and keeps in the result the
// longest view of the phase.
func (w *walker) view(ctx context.Context, fn func(rec *Records) error) error {
	// Time the view against the phase's longest.
	start := time.Now()
	err := w.store.View(ctx, fn)
	phase := w.prog.GetPhase()
	w.res.LongestView[phase] = max(w.res.LongestView[phase], time.Since(start))
	return err
}

// viewScratch runs fn over a read snapshot of scratch, as records under no
// prefix, so the walk scans its kinds with Records.scan.
func (w *walker) viewScratch(ctx context.Context, fn func(edges *Records) error) error {
	tx, err := w.scratch.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(NewRecords(tx, nil))
}

// walkKey returns kind followed by the parts.
func walkKey(kind byte, parts ...[]byte) []byte {
	key := []byte{kind}
	for _, p := range parts {
		key = append(key, p...)
	}
	return key
}

// past returns the first key after every key that begins with prefix, nil
// when none follows, so a scan resumes past a finished key and all its
// extensions. An empty prefix starts the scan.
func past(prefix []byte) ([]byte, bool) {
	if len(prefix) == 0 {
		return nil, true
	}
	next := bytes.Clone(prefix)
	for i := len(next) - 1; i >= 0; i-- {
		if next[i] != 0xff {
			next[i]++
			return next[:i+1], true
		}
	}
	return nil, false
}

// millis encodes t as big-endian Unix milliseconds.
func millis(t int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(t)) //nolint:gosec // times and expiries after 1970.
}

// readMillis decodes a value millis wrote.
func readMillis(value []byte) (int64, error) {
	if len(value) != 8 {
		return 0, errBadWalkRecord
	}
	return int64(binary.BigEndian.Uint64(value)), nil //nolint:gosec // times and expiries after 1970.
}

// roots writes an edge for each root of the next slice of roots.
func (w *walker) roots(ctx context.Context) error {
	// Read the slice in one view. One root's name may begin another's, so
	// the slice resumes at the next key, not past every key the last begins.
	var from []byte
	if after := w.prog.GetAfter(); len(after) != 0 {
		from = append(bytes.Clone(after), 0)
	}
	var recs []walkRecord
	after, done := w.prog.GetAfter(), true
	err := w.view(ctx, func(rec *Records) error {
		return rec.scan(ctx, rec.key(kindRoot), from, func(key []byte, value func() ([]byte, error)) (bool, error) {
			// Stop at the bound.
			if len(recs) == sliceRecords {
				done = false
				return false, nil
			}

			// Write the root's edge.
			entry, err := decodeMsg[RootEntry](value)
			if err != nil {
				return false, err
			}
			if _, rest, err := ParseBlockKey(entry.GetRef()); err != nil || len(rest) != 0 {
				return false, errBadBlockKey
			}
			k := walkKey(walkEdge, entry.GetRef(), []byte{walkRootNamer}, key)
			recs = append(recs, walkRecord{key: k, value: millis(entry.GetExpiresUnixMilli())})
			after = bytes.Clone(key)
			return true, nil
		})
	})
	if err != nil {
		return err
	}

	// Advance and write.
	total := w.prog.GetCounts()
	total.Roots += uint64(len(recs))
	total.Edges += uint64(len(recs))
	w.prog.After = after
	if done {
		w.prog.Phase, w.prog.After = WalkPhase_WALK_PHASE_LISTS, nil
	}
	return w.write(ctx, recs)
}

// lists writes an edge for each child of each list, in the next slice of
// lists: a stored block holds its children until its free.
func (w *walker) lists(ctx context.Context) error {
	// Read the slice in one view.
	from, more := past(w.prog.GetAfter())
	var recs []walkRecord
	var read uint64
	after, done := w.prog.GetAfter(), true
	err := w.view(ctx, func(rec *Records) error {
		// Scan the lists past the progress, unless they are done.
		if !more {
			return nil
		}
		return rec.scan(ctx, rec.key(kindChildren), from, func(key []byte, value func() ([]byte, error)) (bool, error) {
			// Stop at the bounds.
			if read == sliceRecords || len(recs) >= walkSliceEdges {
				done = false
				return false, nil
			}
			read++
			after = bytes.Clone(key)

			// Write an edge for each child.
			if _, rest, err := ParseBlockKey(key); err != nil || len(rest) != 0 {
				return false, errBadBlockKey
			}
			children, err := decodeValue(value)
			if err != nil {
				return false, err
			}
			namer := walkKey(walkListNamer, key)
			for _, child := range children {
				recs = append(recs, walkRecord{key: walkKey(walkEdge, BlockKey(child), namer)})
			}
			return true, nil
		})
	})
	if err != nil {
		return err
	}

	// Advance and write.
	total := w.prog.GetCounts()
	total.Lists += read
	total.Edges += uint64(len(recs))
	w.prog.After = after
	if done {
		w.prog.Phase, w.prog.After = WalkPhase_WALK_PHASE_CHECK, nil
	}
	return w.write(ctx, recs)
}

// check checks the next slice of named blocks in one view, against the edges
// still valid in it, and writes a candidate for each named block it finds
// absent. A wide block takes the slice alone, as checkWide describes.
func (w *walker) check(ctx context.Context) error {
	// Resume a wide block under way, or take the next slice.
	if edge := w.prog.GetEdgeAfter(); len(edge) != 0 {
		ref, _, err := ParseBlockKey(edge)
		if err != nil {
			return err
		}
		return w.checkWide(ctx, BlockKey(ref))
	}
	keys, wide, done, err := w.nextChildren(ctx)
	if err != nil {
		return err
	}
	if wide {
		return w.checkWide(ctx, keys[0])
	}

	// Check each against the counts in one view, copying the log pages
	// written since the last slice.
	var t *tally
	var found []bytesCheck
	err = w.viewScratch(ctx, func(edges *Records) error {
		return w.view(ctx, func(rec *Records) error {
			// Read the log and release sums of the slice's blocks.
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			if t, err = w.readTally(ctx, edges, rec, state, keys); err != nil || t.behind {
				return err
			}

			// Check each block. The slice holds at most walkSliceEdges
			// edges, so each block's edges fit the view.
			for _, key := range keys {
				seen, _, _, more, err := scanNamers(ctx, edges, rec, state, key, nil, walkSliceEdges, false)
				if err != nil {
					return err
				}
				if more {
					return errBadWalkRecord
				}
				check, ok, err := w.checkChild(ctx, rec, state, key, seen, true, t.delta(key), t.pending[string(key)])
				if err != nil {
					return err
				}
				if ok {
					found = append(found, check)
				}
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	if t.behind {
		return w.behindOr(ctx, t)
	}
	w.behind = 0

	// Write a candidate for each block whose bytes are absent too, with the
	// copies of the blocks past the slice, which only a later slice reads.
	var recs []walkRecord
	if !done {
		recs = t.past(keys[len(keys)-1])
	}
	n := len(recs)
	if recs, err = w.candidates(ctx, found, recs); err != nil {
		return err
	}

	// Advance and write.
	total := w.prog.GetCounts()
	total.TailPairs += uint64(n)
	total.Checked += uint64(len(keys))
	w.advance(t)
	if len(keys) != 0 {
		w.prog.After = keys[len(keys)-1]
	}
	if done {
		w.prog.Phase, w.prog.After = WalkPhase_WALK_PHASE_CONFIRM, nil
	}
	return w.write(ctx, recs)
}

// checkWide checks the next walkSliceEdges edges of the wide block at key,
// the one past the progress, in one view, and finishes the block in the
// view that reads its last edge. It sums the valid edges across the views,
// and checks the sum for an undercount only if every view read the same
// WalkMark as the first, as the invariants describe.
func (w *walker) checkWide(ctx context.Context, key []byte) error {
	// Read the next edges and the mark in one view, and finish the block in
	// the view that reads its last edge.
	from := w.resumeEdge(key)
	var (
		t           *tally
		seen        uint64
		last        []byte
		more, moved bool
		found       []bytesCheck
	)
	err := w.viewScratch(ctx, func(edges *Records) error {
		return w.view(ctx, func(rec *Records) error {
			// Read the sums and the mark.
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			if t, err = w.readTally(ctx, edges, rec, state, [][]byte{key}); err != nil || t.behind {
				return err
			}
			mark, err := readMark(ctx, rec, state, key, t)
			if err != nil {
				return err
			}
			if first := w.prog.GetWideMark(); first == nil {
				w.prog.WideMark = mark
			} else if !first.EqualVT(mark) {
				moved = true
			}

			// Count the valid edges, and check the block after its last.
			seen, _, last, more, err = scanNamers(ctx, edges, rec, state, key, from, walkSliceEdges, false)
			if err != nil || more {
				return err
			}
			sound := !moved && !w.prog.GetWideMoved()
			check, ok, err := w.checkChild(ctx, rec, state, key, w.prog.GetWideSeen()+seen, sound, t.delta(key), t.pending[string(key)])
			if ok {
				found = append(found, check)
			}
			return err
		})
	})
	if err != nil {
		return err
	}
	if t.behind {
		return w.behindOr(ctx, t)
	}
	w.behind = 0
	total := w.prog.GetCounts()
	w.advance(t)

	// Between views, keep the copies of the block and those past it.
	if more {
		recs := t.past(nil)
		total.TailPairs += uint64(len(recs))
		w.prog.EdgeAfter = append(bytes.Clone(key), last...)
		w.prog.WideSeen += seen
		w.prog.WideMoved = w.prog.GetWideMoved() || moved
		return w.write(ctx, recs)
	}

	// After the last, keep only the copies past it, and advance.
	recs := t.past(key)
	n := len(recs)
	if recs, err = w.candidates(ctx, found, recs); err != nil {
		return err
	}
	total.TailPairs += uint64(n)
	total.Checked++
	w.prog.After, w.prog.EdgeAfter = key, nil
	w.prog.WideMark, w.prog.WideSeen, w.prog.WideMoved = nil, 0, false
	return w.write(ctx, recs)
}

// behindOr keeps the copy of a view t found behind its writers and waits
// before the next view, up to walkBehindViews such views in a row.
func (w *walker) behindOr(ctx context.Context, t *tally) error {
	// Give up past the bound, otherwise keep the copy and wait.
	if w.behind++; w.behind > walkBehindViews {
		return errors.Wrapf(ErrWalkBehind, "%d views past %d entries", walkBehindViews, walkBacklogEntries)
	}
	recs := t.past(nil)
	w.prog.GetCounts().TailPairs += uint64(len(recs))
	w.advance(t)
	if err := w.write(ctx, recs); err != nil {
		return err
	}
	return w.opts.Wait(ctx, walkBehindWait)
}

// bytesCheck is a named block whose bytes the walk reads after the view
// that checked it.
type bytesCheck struct {
	// ref is the block.
	ref *block.BlockRef
	// listed is set when the block has a child list; otherwise it has neither
	// a list nor a hole record.
	listed bool
}

// candidates appends to recs a candidate for each unlisted block of found
// whose bytes the Volume lacks too, counting the rest as unknown, and counts
// each listed block of found whose bytes the Volume lacks.
func (w *walker) candidates(ctx context.Context, found []bytesCheck, recs []walkRecord) ([]walkRecord, error) {
	total := w.prog.GetCounts()
	now := millis(w.opts.Now().UnixMilli())
	for _, check := range found {
		stored, err := w.store.BlockStored(ctx, check.ref)
		if err != nil {
			return nil, err
		}
		switch {
		case stored && check.listed:
		case check.listed:
			total.ListedAbsent++
			w.res.ListedAbsentSample = sample(w.res.ListedAbsentSample, check.ref)
		case stored:
			total.Unknown++
		default:
			total.Candidates++
			recs = append(recs, walkRecord{key: walkKey(walkCandidate, BlockKey(check.ref)), value: now})
		}
	}
	return recs, nil
}

// resumeEdge returns the edge key, after the block key, that the next view
// of the block at key reads from: past the progress's last edge when it is
// the block's, otherwise the block's first edge.
func (w *walker) resumeEdge(key []byte) []byte {
	edge := w.prog.GetEdgeAfter()
	ref, namer, err := ParseBlockKey(edge)
	if err != nil || !bytes.Equal(BlockKey(ref), key) {
		return nil
	}

	// One root's name may begin another's, so resume at the next key.
	return append(bytes.Clone(namer), 0)
}

// tally is what one check view reads beyond the counts for its blocks.
type tally struct {
	// copied are the log entries the view copied, of blocks past the
	// progress, as walkTail records.
	copied []walkRecord
	// upNext and downNext are the sequences past the last up and down pages
	// the view copied.
	upNext, downNext uint64
	// logs is what the logs not netted yet do to each block.
	logs map[string]int64
	// pending is what the paged release does to each block's namers.
	pending map[string]int64
	// behind reports that the view found more than walkBacklogEntries entries
	// past its copy; only copied and the next sequences are then read.
	behind bool
}

// delta returns what the logs not netted yet do to the holders of the block
// at key.
func (t *tally) delta(key []byte) int64 {
	return t.logs[string(key)]
}

// past returns the copies of the blocks past the block at key, or all of
// them for a nil key: the copies a later view still reads. A copy's key
// holds its block's key and then more, so a copy of that block sorts after
// key too, and only its prefix tells it apart.
func (t *tally) past(key []byte) []walkRecord {
	if key == nil {
		return t.copied
	}
	var recs []walkRecord
	for _, c := range t.copied {
		if at := c.key[1:]; bytes.Compare(at, key) > 0 && !bytes.HasPrefix(at, key) {
			recs = append(recs, c)
		}
	}
	return recs
}

// advance moves the progress's log copy positions to t's.
func (w *walker) advance(t *tally) {
	w.prog.UpTail, w.prog.DownTail = t.upNext, t.downNext
}

// readTally reads in the view rec the log and release sums of the blocks at
// keys, in key order, copying the logs past the walk's last copy as copyTail
// describes.
func (w *walker) readTally(ctx context.Context, edges, rec *Records, state *LivenessState, keys [][]byte) (*tally, error) {
	// Copy the logs past the last copy, and sum the rest for the blocks.
	keep := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keep[string(key)] = struct{}{}
	}
	downCursor, err := rec.downCursor(ctx, state)
	if err != nil {
		return nil, err
	}
	t := &tally{
		upNext:   max(w.prog.GetUpTail(), state.GetUpCursor()),
		downNext: max(w.prog.GetDownTail(), downCursor),
		logs:     make(map[string]int64),
	}
	read, err := t.copyTail(ctx, rec, w.prog.GetAfter(), keep)
	if err != nil || t.behind {
		return t, err
	}

	// Add the copies of the logs not netted yet against what is left of the
	// budget, then read the paged release.
	if t.behind, err = tailDelta(ctx, edges, state.GetUpCursor(), downCursor, keep, t.copied, t.logs, walkBacklogEntries-read); err != nil || t.behind {
		return t, err
	}
	t.pending, err = releasePending(ctx, rec, state, keys)
	return t, err
}

// readMark reads the WalkMark of the block at key in the view rec, with t's
// sums for it.
func readMark(ctx context.Context, rec *Records, state *LivenessState, key []byte, t *tally) (*WalkMark, error) {
	// Read the block's listing and holders, then the mark.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return nil, err
	}
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil {
		return nil, err
	}
	holders, _, err := rec.GetHolders(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &WalkMark{
		State:   state.CloneVT(),
		Listed:  listed,
		Holders: holders,
		Logs:    t.logs[string(key)],
		Pending: t.pending[string(key)],
	}, nil
}

// copyTail reads the up pages from t.upNext, then the down pages from
// t.downNext, each at or past its log's first page not netted yet. It
// copies the entries of the pages it reads, up to the page that passes
// walkSliceEdges entries, as walkTail records, skipping the entries of
// blocks at or before after, which the walk has checked, and moves t's next
// sequences past them. It sums the entries of the pages past those for the
// blocks in keep alone, into t.logs, and returns how many entries it read
// past its copy. Once it has read walkBacklogEntries entries past its copy it
// stops and reports t behind; t.logs is then partial and unused.
//
// A page never changes its keys once written, and the logs net whole pages
// in sequence order, deleting each, so a copy is not netted yet exactly
// while its sequence is at or past its log's first page left: the up cursor,
// or the first down page.
func (t *tally) copyTail(ctx context.Context, rec *Records, after []byte, keep map[string]struct{}) (int, error) {
	// page copies one log page, or past the bound sums it for keep.
	var copied, past int
	page := func(log byte, seq uint64, keys []byte, next *uint64) (bool, error) {
		// Past the bound, sum the page for the slice's blocks, unless the
		// backlog passes its own bound.
		d := int64(-1)
		if log == walkUp {
			d = 1
		}
		if copied >= walkSliceEdges {
			err := eachKey(keys, func(key []byte) error {
				past++
				if _, ok := keep[string(key)]; ok {
					t.logs[string(key)] += d
				}
				return nil
			})
			t.behind = past > walkBacklogEntries
			return !t.behind, err
		}

		// Copy the page's entries.
		*next = seq + 1
		var i uint64
		at := binary.BigEndian.AppendUint64([]byte{log}, seq)
		return true, eachKey(keys, func(key []byte) error {
			if bytes.Compare(key, after) > 0 {
				k := walkKey(walkTail, key, at, binary.BigEndian.AppendUint64(nil, i))
				t.copied = append(t.copied, walkRecord{key: k})
			}
			copied++
			i++
			return nil
		})
	}
	err := rec.scanUps(ctx, t.upNext, func(seq uint64, keys []byte) (bool, error) {
		return page(walkUp, seq, keys, &t.upNext)
	})
	if err != nil || t.behind {
		return past, err
	}
	err = rec.scanDowns(ctx, t.downNext, func(p downPage) (bool, error) {
		return page(walkDown, p.seq, p.keys, &t.downNext)
	})
	return past, err
}

// tailDelta adds to delta what the logs not netted yet do to each block in
// keep, as logTail sums it in the same view: the copies in edges and in
// copied whose sequence is at or past their log's cursor, upCursor or
// downCursor. delta holds on entry the sums copyTail read past its copies.
// It reads at most budget copies in scratch and reports the view behind past
// it: the copies of a block grow with netting's lag, which a stalled
// collector leaves unbounded.
func tailDelta(
	ctx context.Context,
	edges *Records,
	upCursor, downCursor uint64,
	keep map[string]struct{},
	copied []walkRecord,
	delta map[string]int64,
	budget int,
) (bool, error) {
	// add sums one copy not netted yet: an up as one more holder and a down
	// as one fewer.
	add := func(key, at []byte) error {
		if len(at) != 17 {
			return errBadWalkRecord
		}
		seq := binary.BigEndian.Uint64(at[1:])
		switch at[0] {
		case walkUp:
			if seq >= upCursor {
				delta[string(key)]++
			}
		case walkDown:
			if seq >= downCursor {
				delta[string(key)]--
			}
		default:
			return errBadWalkRecord
		}
		return nil
	}

	// Sum each block's copies in scratch from each cursor on, within the
	// budget.
	var read int
	for key := range keep {
		for _, l := range [...]struct {
			log    byte
			cursor uint64
		}{{walkUp, upCursor}, {walkDown, downCursor}} {
			prefix := walkKey(walkTail, []byte(key), []byte{l.log})
			err := edges.scan(ctx, prefix, binary.BigEndian.AppendUint64(nil, l.cursor), func(at []byte, _ func() ([]byte, error)) (bool, error) {
				if read++; read > budget {
					return false, nil
				}
				return true, add([]byte(key), append([]byte{l.log}, at...))
			})
			if err != nil || read > budget {
				return read > budget, err
			}
		}
	}

	// Sum the copies this view made.
	for _, c := range copied {
		ref, at, err := ParseBlockKey(c.key[1:])
		if err != nil {
			return false, err
		}
		key := BlockKey(ref)
		if _, ok := keep[string(key)]; ok {
			if err := add(key, at); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

// nextChildren returns the block keys of the next named blocks past the
// progress, in key order, at most walkSliceChildren of them with at most
// walkSliceEdges edges in all, and whether they end the edges. A block with
// more edges than walkSliceEdges comes alone, reported wide.
func (w *walker) nextChildren(ctx context.Context) (keys [][]byte, wide, done bool, err error) {
	// Start past the progress.
	from, more := past(w.prog.GetAfter())
	if !more {
		return nil, false, true, nil
	}
	var n int
	done = true
	err = w.viewScratch(ctx, func(edges *Records) error {
		return edges.scan(ctx, []byte{walkEdge}, from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
			// Take each block once, up to the bound.
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return false, err
			}
			child := BlockKey(ref)
			if len(keys) == 0 || !bytes.Equal(keys[len(keys)-1], child) {
				if len(keys) == walkSliceChildren {
					done = false
					return false, nil
				}
				keys = append(keys, child)
			}

			// Stop at the edge that passes the bound: a block alone is
			// wide, and any other waits for the next slice.
			if n++; n <= walkSliceEdges {
				return true, nil
			}
			done = false
			if wide = len(keys) == 1; !wide {
				keys = keys[:len(keys)-1]
			}
			return false, nil
		})
	})
	return keys, wide, done, err
}

// releasePending counts, for each of keys, in key order, how many times the
// paged release in progress has not moved it yet, as tallyHolders counts
// them. The release names its children in block key
// order, since it pages a child list and every list is in that order:
// EncodeChildren sorts, and mergeList's union merges two sorted lists. So it
// finds the first not below keys[0] by a binary search from the
// release's Next, then merges forward, reading only the pages that overlap
// the keys' range.
func releasePending(ctx context.Context, rec *Records, state *LivenessState, keys [][]byte) (map[string]int64, error) {
	// Nothing is pending without keys or a release in progress.
	pending := make(map[string]int64)
	rel := state.GetRelease()
	if len(keys) == 0 || rel.GetNext() >= rel.GetChildren() {
		return pending, nil
	}

	// Find the first child not below the first key.
	cur := newReleaseCursor(rec, rel)
	lo, hi := rel.GetNext(), rel.GetChildren()
	for lo < hi {
		cur.next = lo + (hi-lo)/2
		child, err := cur.child(ctx)
		if err != nil {
			return nil, err
		}
		if bytes.Compare(BlockKey(child), keys[0]) < 0 {
			lo = cur.next + 1
		} else {
			hi = cur.next
		}
	}

	// Merge the children with the keys.
	cur.next = lo
	for _, key := range keys {
		for {
			child, err := cur.child(ctx)
			if err != nil || child == nil {
				return pending, err
			}
			cmp := bytes.Compare(BlockKey(child), key)
			if cmp > 0 {
				break
			}
			if cmp == 0 {
				pending[string(key)]++
			}
			cur.next++
		}
	}
	return pending, nil
}

// checkChild checks the named block at key in the view rec, given seen, the
// count of its edges that held, and returns the bytes check the walk makes
// after the view, and whether it makes one: for a listed block, and for a
// named block with neither a list nor a hole record. When sound, it counts an
// undercount; otherwise, where it would compare, it counts the block wide.
// delta is what the logs not netted yet do to its holders, and pending
// what the paged release does to its namers.
func (w *walker) checkChild(
	ctx context.Context,
	rec *Records,
	state *LivenessState,
	key []byte,
	seen uint64,
	sound bool,
	delta, pending int64,
) (bytesCheck, bool, error) {
	// Count the namers still valid, with the paged release, as tallyHolders
	// does.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return bytesCheck{}, false, err
	}
	named := max(int64(seen)+pending, 0) //nolint:gosec // counts stay far below the int64 range.

	// Compare a stored block's holders with its namers, once the Volume
	// keeps counts and unless the sweep owns the block, as checkStored does.
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil {
		return bytesCheck{}, false, err
	}
	if listed {
		check := bytesCheck{ref: ref, listed: true}
		unmarked, err := rec.Unmarked(ctx, key)
		total := w.prog.GetCounts()
		if err != nil || unmarked || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			if err == nil {
				total.Declined++
			}
			return check, err == nil, err
		}
		if !sound {
			total.Wide++
			return check, true, nil
		}
		holders, _, err := rec.GetHolders(ctx, ref)
		if err != nil {
			return bytesCheck{}, false, err
		}
		if int64(holders)+delta < named { //nolint:gosec // counts stay far below the int64 range.
			total.Undercount++
			w.res.UndercountSample = sample(w.res.UndercountSample, ref)
		}
		return check, true, nil
	}

	// A named block without a list or a hole record may be a hole.
	if seen == 0 {
		return bytesCheck{}, false, nil
	}
	hole, err := rec.tx.Exists(ctx, rec.key(kindHole, key))
	return bytesCheck{ref: ref}, err == nil && !hole, err
}

// scanNamers reads the edges of the block at key from the namer from on, at
// most budget of them, and counts those that hold in the view rec, stopping
// at the first when first is set. It returns the count, how many it read,
// the last namer read and whether edges remain past it.
func scanNamers(
	ctx context.Context,
	edges, rec *Records,
	state *LivenessState,
	key, from []byte,
	budget int,
	first bool,
) (seen uint64, read int, last []byte, more bool, err error) {
	err = edges.scan(ctx, walkKey(walkEdge, key), from, func(namer []byte, _ func() ([]byte, error)) (bool, error) {
		// Stop at the bound.
		if read == budget {
			more = true
			return false, nil
		}
		read++
		last = bytes.Clone(namer)

		// Count the edge if it holds.
		valid, err := edgeHolds(ctx, rec, state, key, namer)
		if valid {
			seen++
		}
		return err == nil && (!first || !valid), err
	})
	return seen, read, last, more, err
}

// edgeHolds reports whether namer still names the block at key in the view
// rec: the root still holds the block, or the parent is still stored, since
// a stored block holds its children until its free.
func edgeHolds(ctx context.Context, rec *Records, state *LivenessState, key, namer []byte) (bool, error) {
	if len(namer) == 0 {
		return false, errBadWalkRecord
	}
	switch namer[0] {
	case walkRootNamer:
		// Read the root.
		bucketID, name, err := ParseNamedKey(namer[1:])
		if err != nil {
			return false, err
		}
		entry, err := rec.GetRoot(ctx, bucketID, name)
		return entry != nil && bytes.Equal(entry.GetRef(), key), err
	case walkListNamer:
		// Read the parent's list and whether it holds.
		parent, rest, err := ParseBlockKey(namer[1:])
		if err != nil || len(rest) != 0 {
			return false, errBadBlockKey
		}
		return rec.HasChildren(ctx, parent)
	}
	return false, errBadWalkRecord
}

// confirm rereads the next slice of candidates once the delay has passed
// since the check found them, and reports each still named and absent as
// unconfirmed, with its class. A view reads at most walkSliceEdges edges;
// the candidate it stops in resumes in the next view past its last edge.
func (w *walker) confirm(ctx context.Context) error {
	// Read the slice and wait out its delay.
	keys, latest, done, err := w.nextCandidates(ctx)
	if err != nil {
		return err
	}
	if len(keys) != 0 {
		until := time.UnixMilli(latest).Add(w.opts.Delay)
		if d := until.Sub(w.opts.Now()); d > 0 {
			if err := w.opts.Wait(ctx, d); err != nil {
				return err
			}
		}
	}

	// Reread each, then its bytes, and class each still absent.
	total := w.prog.GetCounts()
	var finished int
	var edgeAfter []byte
	err = w.viewScratch(ctx, func(edges *Records) error {
		// Reread the candidates in one view, up to its edges.
		var absent [][]byte
		err := w.view(ctx, func(rec *Records) error {
			// Read the state, then each candidate within the budget.
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			budget := walkSliceEdges
			for _, key := range keys {
				missing, read, last, err := stillMissing(ctx, edges, rec, state, key, w.resumeEdge(key), budget)
				if err != nil {
					return err
				}
				if last != nil {
					edgeAfter = append(bytes.Clone(key), last...)
					return nil
				}
				finished++
				if budget -= read; missing {
					absent = append(absent, key)
				} else {
					total.Settled++
				}
				if budget == 0 {
					return nil
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Read the bytes of each still missing, and class it.
		for _, key := range absent {
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return err
			}
			stored, err := w.store.BlockStored(ctx, ref)
			if err != nil {
				return err
			}
			if stored {
				total.Settled++
				continue
			}
			class, err := w.classify(ctx, edges, key)
			if err != nil {
				return err
			}
			w.countUnconfirmed(ref, class)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Advance and write.
	if finished != 0 {
		w.prog.After = keys[finished-1]
	}
	w.prog.EdgeAfter = edgeAfter
	if done && finished == len(keys) {
		w.prog.Phase, w.prog.After = WalkPhase_WALK_PHASE_DONE, nil
	}
	return w.write(ctx, nil)
}

// nextCandidates returns the next walkConfirmSlice candidates past the
// progress, the latest time the check found one, and whether they end the
// candidates.
func (w *walker) nextCandidates(ctx context.Context) ([][]byte, int64, bool, error) {
	// Start past the progress.
	from, more := past(w.prog.GetAfter())
	if !more {
		return nil, 0, true, nil
	}
	var keys [][]byte
	var latest int64
	done := true
	err := w.viewScratch(ctx, func(edges *Records) error {
		return edges.scan(ctx, []byte{walkCandidate}, from, func(key []byte, value func() ([]byte, error)) (bool, error) {
			// Stop at the bound.
			if len(keys) == walkConfirmSlice {
				done = false
				return false, nil
			}

			// Take the candidate and when it was found.
			v, err := value()
			if err != nil {
				return false, err
			}
			at, err := readMillis(v)
			if err != nil {
				return false, err
			}
			keys = append(keys, bytes.Clone(key))
			latest = max(latest, at)
			return true, nil
		})
	})
	return keys, latest, done, err
}

// stillMissing reports whether the candidate at key is still named, with
// neither a list nor a hole record, in the view rec, reading its edges from
// the namer from on, at most budget of them. It returns how many it read,
// and the last when it read budget edges and found no namer while more
// remain, so the next view resumes past it.
func stillMissing(ctx context.Context, edges, rec *Records, state *LivenessState, key, from []byte, budget int) (bool, int, []byte, error) {
	// Settle a candidate a put stored or an audit recorded.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return false, 0, nil, err
	}
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil || listed {
		return false, 0, nil, err
	}
	hole, err := rec.tx.Exists(ctx, rec.key(kindHole, key))
	if err != nil || hole {
		return false, 0, nil, err
	}

	// Otherwise it is missing while a namer still holds.
	seen, read, last, more, err := scanNamers(ctx, edges, rec, state, key, from, budget, true)
	if err != nil || seen != 0 || !more {
		return seen != 0, read, nil, err
	}
	return false, read, last, nil
}

// countUnconfirmed counts an unconfirmed candidate by its class and samples
// it.
func (w *walker) countUnconfirmed(ref *block.BlockRef, class HoleClass) {
	total := w.prog.GetCounts()
	total.Unconfirmed++
	switch class {
	case HoleClass_HOLE_CLASS_HELD:
		total.UnconfirmedHeld++
	case HoleClass_HOLE_CLASS_LAPSED:
		total.UnconfirmedLapsed++
	case HoleClass_HOLE_CLASS_UNROOTED:
		total.UnconfirmedUnrooted++
	}
	if len(w.res.UnconfirmedSample) < sampleSize {
		w.res.UnconfirmedSample = append(w.res.UnconfirmedSample, UnconfirmedHole{Ref: ref, Class: class})
	}
}

// classify walks up the edges from the block at key and returns which roots
// reach it: held, if a root without a lease or whose lease ran past the
// walk's start does; lapsed, if only roots whose lease lapsed do; unrooted,
// if none does; and unknown, past walkClassBound blocks. It reads the edges
// as the walk wrote them, not as the Volume holds them now.
func (w *walker) classify(ctx context.Context, edges *Records, key []byte) (HoleClass, error) {
	// Walk up from the block through the namers the edges record.
	started := w.prog.GetStartedUnixMilli()
	visited := map[string]struct{}{string(key): {}}
	stack := [][]byte{key}
	var lapsed, held, bounded bool
	for len(stack) != 0 && !held && !bounded {
		// Take the next block and read its namers.
		at := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		err := edges.scan(ctx, walkKey(walkEdge, at), nil, func(namer []byte, value func() ([]byte, error)) (bool, error) {
			// A root ends the walk if held, and marks it lapsed otherwise.
			if len(namer) == 0 {
				return false, errBadWalkRecord
			}
			if namer[0] == walkRootNamer {
				v, err := value()
				if err != nil {
					return false, err
				}
				expires, err := readMillis(v)
				if err != nil {
					return false, err
				}
				held = expires == 0 || expires > started
				lapsed = true
				return !held, nil
			}

			// A parent joins the walk once, up to the bound.
			parent := namer[1:]
			if _, ok := visited[string(parent)]; ok {
				return true, nil
			}
			if len(visited) == walkClassBound {
				bounded = true
				return false, nil
			}
			visited[string(parent)] = struct{}{}
			stack = append(stack, bytes.Clone(parent))
			return true, nil
		})
		if err != nil {
			return HoleClass_HOLE_CLASS_UNKNOWN, err
		}
	}
	switch {
	case held:
		return HoleClass_HOLE_CLASS_HELD, nil
	case bounded:
		return HoleClass_HOLE_CLASS_UNKNOWN, nil
	case lapsed:
		return HoleClass_HOLE_CLASS_LAPSED, nil
	}
	return HoleClass_HOLE_CLASS_UNROOTED, nil
}
