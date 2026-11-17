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
// Invariants, which the TLA+ model AuditWalk.tla checks:
//
//   - The walk never writes the Volume. It reads it through WalkStore, which
//     has no write, and writes only the scratch store.
//   - An edge <<child, namer>> is written for each root and for each child of
//     each list that holds its children, as tallyHolders counts them. The
//     check counts only the edges still valid in the view that checks the
//     child: the root still names it, or the parent is still stored and its
//     list still holds. That count is a lower bound on the child's true
//     namers in that view, so holders plus the journal past the cursor below
//     it is a true undercount (no false positive). A namer the edges phase
//     passed before it took up the child can hide one unit until the next
//     walk.
//   - The journal past the cursor that the check adds to the holders is
//     exactly journalTail's for the slice's blocks in the same view: the
//     walk copies each journal record into scratch once, as it found it,
//     and a record's up and down pairs never change once written; netting
//     only moves the cursor and the net offset past them, which the check
//     reads in its view and applies to the copies. Each check copies at most
//     walkSliceEdges pairs and sums at most walkBacklogPairs past them in its
//     own view, so a view's work is bounded whatever a writer's rate. A view
//     that finds more left only copies, releases the view and waits
//     walkBehindWait before the next; after walkBehindViews such views in a
//     row the walk ends with ErrWalkBehind, keeping its scratch to resume.
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
	// walkTail keys a copy of one up or down pair of a journal record by the
	// block it names, then the record's sequence and the pair's index in it,
	// up pairs first, both big-endian uint64. Its value is walkUp or
	// walkDown.
	walkTail byte = 't'
)

// The values of a walkTail record.
const (
	// walkDown is a down pair: -1 to the block's holders.
	walkDown byte = 0
	// walkUp is an up pair: +1 to the block's holders.
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
	// walkBacklogPairs bounds the journal pairs one check view sums past its
	// copy; a view that finds more is behind and only copies.
	walkBacklogPairs = 65536
	// walkBehindViews bounds the views in a row that find the walk behind
	// before it gives up with ErrWalkBehind.
	walkBehindViews = 256
	// walkBehindWait is the wait after a view that found the walk behind,
	// which leaves the store to its writers before the next copy.
	walkBehindWait = 100 * time.Millisecond
)

// ErrWalkTaken reports a scratch store another run of the walk owns.
var ErrWalkTaken = errors.New("audit walk scratch store belongs to another run")

// ErrWalkBehind reports a walk whose copy of the journal stayed behind a
// writer for walkBehindViews views in a row. Its scratch store holds the copy
// so far, so a later run of the same name resumes it.
var ErrWalkBehind = errors.New("audit walk fell behind the journal's writers")

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
// walkSliceChildren named blocks and their journal deltas, walkSliceEdges
// edges, or walkClassBound blocks of one hole's class walk. Each check view
// reads the journal past the walk's last copy of it, so a long tail slows
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
			return w.res, nil
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
	w := &walker{store: store, scratch: scratch, opts: opts, res: &WalkResult{}}

	// Resume the run's progress.
	prog, err := w.readProgress(ctx)
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
		return w, nil
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
	value, found, err := tx.Get(ctx, []byte{walkProgressKind})
	if err != nil || !found {
		return nil, err
	}
	prog := &WalkProgress{}
	return prog, prog.UnmarshalVT(value)
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
	err := w.store.View(ctx, func(rec *Records) error {
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

// lists writes an edge for each child of each list that holds its children,
// in the next slice of lists.
func (w *walker) lists(ctx context.Context) error {
	// Read the slice in one view.
	from, more := past(w.prog.GetAfter())
	var recs []walkRecord
	var read uint64
	after, done := w.prog.GetAfter(), true
	err := w.store.View(ctx, func(rec *Records) error {
		// Read the hold rule, unless the lists are done.
		if !more {
			return nil
		}
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}

		// Scan the lists past the progress.
		released := releasesAtZero(state)
		return rec.scan(ctx, rec.key(kindChildren), from, func(key []byte, value func() ([]byte, error)) (bool, error) {
			// Stop at the bounds.
			if read == sliceRecords || len(recs) >= walkSliceEdges {
				done = false
				return false, nil
			}
			read++
			after = bytes.Clone(key)

			// Skip a list that does not hold its children.
			parent, rest, err := ParseBlockKey(key)
			if err != nil || len(rest) != 0 {
				return false, errBadBlockKey
			}
			if released {
				held, err := counts(ctx, rec, state, parent)
				if err != nil || !held {
					return err == nil, err
				}
			}

			// Write an edge for each child.
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

// releasesAtZero reports whether a stored list holds its children only while
// they count it, as tallyHolders reads the rule: in a Volume adopted under
// the release-at-zero rule.
func releasesAtZero(state *LivenessState) bool {
	return state.GetAdoption() == Adoption_ADOPTION_ADOPTED && state.GetHoldRule() != HoldRule_HOLD_RULE_HOLD_UNTIL_FREE
}

// check checks the next slice of named blocks in one view, against the edges
// still valid in it, and writes a candidate for each named block it finds
// absent.
func (w *walker) check(ctx context.Context) error {
	// Take the slice's blocks from the edges.
	keys, done, err := w.nextChildren(ctx)
	if err != nil {
		return err
	}
	keep := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keep[string(key)] = struct{}{}
	}

	// Collect what the view finds: the absent blocks, the journal copy and
	// whether the walk is behind its writers.
	total := w.prog.GetCounts()
	var found []*block.BlockRef
	var copied []walkRecord
	var next uint64
	var behind bool

	// Check each against the counts in one view, copying the journal
	// records written since the last slice.
	err = w.viewScratch(ctx, func(edges *Records) error {
		return w.store.View(ctx, func(rec *Records) error {
			// Copy the journal past the last copy, and read the rest for the
			// slice's blocks.
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			var live map[string]int64
			copied, next, live, behind, err = copyTail(ctx, rec, state, w.prog.GetTail(), keep)
			if err != nil || behind {
				return err
			}

			// Add the copies of the journal past the cursor and the fill
			// records, then read the paged release, for the slice's blocks.
			delta := live
			if err := tailDelta(ctx, edges, rec, state, keep, copied, delta); err != nil {
				return err
			}
			pending, err := releasePending(ctx, rec, state, keep)
			if err != nil {
				return err
			}

			// Check each block.
			for _, key := range keys {
				ref, absent, err := w.checkChild(ctx, edges, rec, state, key, delta[string(key)], pending[string(key)])
				if err != nil {
					return err
				}
				if absent {
					found = append(found, ref)
				}
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	w.prog.Tail = next
	total.TailPairs += uint64(len(copied))

	// Behind a writer, keep the copy, wait and copy again, up to the bound.
	if behind {
		if w.behind++; w.behind > walkBehindViews {
			return errors.Wrapf(ErrWalkBehind, "%d views past %d pairs", walkBehindViews, walkBacklogPairs)
		}
		if err := w.write(ctx, copied); err != nil {
			return err
		}
		return w.opts.Wait(ctx, walkBehindWait)
	}
	w.behind = 0

	// Write a candidate for each block whose bytes are absent too.
	now := millis(w.opts.Now().UnixMilli())
	recs := copied
	for _, ref := range found {
		stored, err := w.store.BlockStored(ctx, ref)
		if err != nil {
			return err
		}
		if stored {
			total.Unknown++
			continue
		}
		recs = append(recs, walkRecord{key: walkKey(walkCandidate, BlockKey(ref)), value: now})
	}

	// Advance and write.
	total.Checked += uint64(len(keys))
	total.Candidates += uint64(len(recs[len(copied):]))
	if len(keys) != 0 {
		w.prog.After = keys[len(keys)-1]
	}
	if done {
		w.prog.Phase, w.prog.After = WalkPhase_WALK_PHASE_CONFIRM, nil
	}
	return w.write(ctx, recs)
}

// copyTail reads the journal records from the later of from and the cursor
// in rec. It returns the pairs of the records it copies, up to the one that
// passes walkSliceEdges pairs, as walkTail records, and the sequence past
// the last of them. It sums the pairs of the records past those for the
// blocks in keep alone, which it returns as live. Those records are past the
// cursor's, so none needs the net offset. Once it has read walkBacklogPairs
// pairs past its copy it stops and reports the walk behind; live is then
// partial and unused.
func copyTail(
	ctx context.Context,
	rec *Records,
	state *LivenessState,
	from uint64,
	keep map[string]struct{},
) (recs []walkRecord, next uint64, live map[string]int64, behind bool, err error) {
	// Start at the copy or the cursor, whichever is later.
	next = max(from, state.GetCursor())
	live = make(map[string]int64)
	var past int
	err = rec.ScanJournal(ctx, next, func(seq uint64, r *JournalRecord) (bool, error) {
		// Past the bound, sum the record for the slice's blocks, unless the
		// backlog passes its own bound.
		if len(recs) >= walkSliceEdges {
			if past += len(r.GetUp()) + len(r.GetDown()); past > walkBacklogPairs {
				behind = true
				return false, nil
			}
			for d, keys := range map[int64][][]byte{1: r.GetUp(), -1: r.GetDown()} {
				for _, key := range keys {
					if _, ok := keep[string(key)]; ok {
						live[string(key)] += d
					}
				}
			}
			return true, nil
		}

		// Copy the up pairs, then the down pairs, as journalTail indexes them.
		var i uint64
		pair := func(key []byte, sign byte) {
			k := walkKey(walkTail, key, binary.BigEndian.AppendUint64(nil, seq), binary.BigEndian.AppendUint64(nil, i))
			recs = append(recs, walkRecord{key: k, value: []byte{sign}})
			i++
		}
		for _, key := range r.GetUp() {
			pair(key, walkUp)
		}
		for _, key := range r.GetDown() {
			pair(key, walkDown)
		}
		next = seq + 1
		return true, nil
	})
	return recs, next, live, behind, err
}

// tailDelta adds to delta what the journal past the cursor and the fill
// records do to each block in keep, as journalTail and fillTail sum them in
// the view rec: the copies in edges and in copied, from the cursor on,
// skipping the entries of the record at the cursor its net offset already
// applied. delta holds on entry the sums copyTail read past its copies.
func tailDelta(ctx context.Context, edges, rec *Records, state *LivenessState, keep map[string]struct{}, copied []walkRecord, delta map[string]int64) error {
	// add sums one copy past the cursor and the net offset.
	add := func(key, at, value []byte) error {
		// Skip the pairs netting already applied.
		if len(at) != 16 || len(value) != 1 {
			return errBadWalkRecord
		}
		seq, i := binary.BigEndian.Uint64(at), binary.BigEndian.Uint64(at[8:])
		if seq < state.GetCursor() || (seq == state.GetCursor() && i < state.GetNetOffset()) {
			return nil
		}

		// Count an up as one more holder and a down as one fewer.
		d := int64(-1)
		if value[0] == walkUp {
			d = 1
		}
		delta[string(key)] += d
		return nil
	}

	// Sum each block's copies in scratch from the cursor on.
	from := binary.BigEndian.AppendUint64(nil, state.GetCursor())
	for key := range keep {
		err := edges.scan(ctx, walkKey(walkTail, []byte(key)), from, func(at []byte, value func() ([]byte, error)) (bool, error) {
			v, err := value()
			if err != nil {
				return false, err
			}
			return true, add([]byte(key), at, v)
		})
		if err != nil {
			return err
		}
	}

	// Sum the copies this view made, then the fill records.
	for _, c := range copied {
		ref, at, err := ParseBlockKey(c.key[1:])
		if err != nil {
			return err
		}
		key := BlockKey(ref)
		if _, ok := keep[string(key)]; ok {
			if err := add(key, at, c.value); err != nil {
				return err
			}
		}
	}
	err := fillTail(ctx, rec, func(key []byte, d int64) {
		if _, ok := keep[string(key)]; ok {
			delta[string(key)] += d
		}
	})
	return err
}

// nextChildren returns the block keys of the next walkSliceChildren named
// blocks past the progress, in key order, and whether they end the edges.
func (w *walker) nextChildren(ctx context.Context) ([][]byte, bool, error) {
	// Start past the progress.
	from, more := past(w.prog.GetAfter())
	if !more {
		return nil, true, nil
	}
	var keys [][]byte
	done := true
	err := w.viewScratch(ctx, func(edges *Records) error {
		return edges.scan(ctx, []byte{walkEdge}, from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
			// Skip the rest of the last block's edges.
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return false, err
			}
			child := BlockKey(ref)
			if len(keys) != 0 && bytes.Equal(keys[len(keys)-1], child) {
				return true, nil
			}

			// Take the next block, up to the bound.
			if len(keys) == walkSliceChildren {
				done = false
				return false, nil
			}
			keys = append(keys, child)
			return true, nil
		})
	})
	return keys, done, err
}

// releasePending counts, for each block in keep, how many times the paged
// cascade in progress has not moved it yet, and returns the counts negated
// for a paged re-hold, as tallyHolders applies them.
func releasePending(ctx context.Context, rec *Records, state *LivenessState, keep map[string]struct{}) (map[string]int64, error) {
	// A paged re-hold adds back what the release took.
	pending := make(map[string]int64)
	step := int64(1)
	if state.GetRelease().GetRehold() {
		step = -1
	}
	err := rec.scanReleasePending(ctx, state.GetRelease(), func(child *block.BlockRef) error {
		key := string(BlockKey(child))
		if _, ok := keep[key]; ok {
			pending[key] += step
		}
		return nil
	})
	return pending, err
}

// checkChild checks the named block at key in the view rec, counting an
// undercount, and returns its ref and whether it is named with neither a
// list nor a hole record. delta is what the journal past the cursor does to
// its holders, and pending what the paged release does to its namers.
func (w *walker) checkChild(
	ctx context.Context,
	edges, rec *Records,
	state *LivenessState,
	key []byte,
	delta, pending int64,
) (*block.BlockRef, bool, error) {
	// Count the namers still valid, with the paged release, as tallyHolders
	// does: a re-hold takes back only what is there.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return nil, false, err
	}
	seen, err := validNamers(ctx, edges, rec, state, key)
	if err != nil {
		return nil, false, err
	}
	named := max(int64(seen)+pending, 0) //nolint:gosec // counts stay far below the int64 range.

	// Compare a stored block's holders with its namers, once the Volume
	// keeps counts and unless the sweep owns the block, as checkStored does.
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	if listed {
		unmarked, err := rec.Unmarked(ctx, key)
		if err != nil || unmarked || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return ref, false, err
		}
		holders, _, err := rec.GetHolders(ctx, ref)
		if err != nil {
			return nil, false, err
		}
		if int64(holders)+delta < named { //nolint:gosec // counts stay far below the int64 range.
			w.prog.GetCounts().Undercount++
			w.res.UndercountSample = sample(w.res.UndercountSample, ref)
		}
		return ref, false, nil
	}

	// A named block without a list or a hole record may be a hole.
	if seen == 0 {
		return ref, false, nil
	}
	hole, err := rec.tx.Exists(ctx, rec.key(kindHole, key))
	return ref, err == nil && !hole, err
}

// validNamers counts the edges of the block at key that still hold in the
// view rec.
func validNamers(ctx context.Context, edges, rec *Records, state *LivenessState, key []byte) (uint64, error) {
	var seen uint64
	err := edges.scan(ctx, walkKey(walkEdge, key), nil, func(namer []byte, _ func() ([]byte, error)) (bool, error) {
		valid, err := edgeHolds(ctx, rec, state, key, namer)
		if valid {
			seen++
		}
		return err == nil, err
	})
	return seen, err
}

// edgeHolds reports whether namer still names the block at key in the view
// rec: the root still holds the block, or the parent is still stored and
// its list holds its children, as tallyHolders counts a list.
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
		listed, err := rec.HasChildren(ctx, parent)
		if err != nil || !listed || !releasesAtZero(state) {
			return listed, err
		}
		return counts(ctx, rec, state, parent)
	}
	return false, errBadWalkRecord
}

// confirm rereads the next slice of candidates once the delay has passed
// since the check found them, and reports each still named and absent as
// unconfirmed, with its class.
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
	err = w.viewScratch(ctx, func(edges *Records) error {
		// Reread the candidates in one view.
		var absent [][]byte
		err := w.store.View(ctx, func(rec *Records) error {
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			for _, key := range keys {
				ref, missing, err := stillMissing(ctx, edges, rec, state, key)
				if err != nil {
					return err
				}
				if ref != nil && missing {
					absent = append(absent, key)
					continue
				}
				total.Settled++
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
	if len(keys) != 0 {
		w.prog.After = keys[len(keys)-1]
	}
	if done {
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
// neither a list nor a hole record, in the view rec, and returns its ref.
func stillMissing(ctx context.Context, edges, rec *Records, state *LivenessState, key []byte) (*block.BlockRef, bool, error) {
	// Settle a candidate a put stored or an audit recorded.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return nil, false, err
	}
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil || listed {
		return ref, false, err
	}
	hole, err := rec.tx.Exists(ctx, rec.key(kindHole, key))
	if err != nil || hole {
		return ref, false, err
	}

	// Otherwise it is missing while a namer still holds.
	seen, err := validNamers(ctx, edges, rec, state, key)
	return ref, seen != 0, err
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
