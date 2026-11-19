package block_gc

import (
	"context"
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// AdoptStore is the Volume an audit runs in.
type AdoptStore interface {
	Store
	// BlockStored reports whether the Volume holds the bytes of ref.
	BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error)
	// ScanBlocks calls fn with runs of the blocks whose bytes the Volume
	// holds, in key order, each run read in a snapshot of its own.
	ScanBlocks(ctx context.Context, fn func(refs []*block.BlockRef) error) error
}

// AdoptResult is what an adoption counted.
type AdoptResult struct {
	// Already reports that the Volume was adopted, so nothing was counted.
	Already bool
	// Blocks counts the blocks stored when the adoption began.
	Blocks uint64
	// Roots counts the named roots when the adoption began.
	Roots uint64
	// Counted counts the stored blocks given a count record: those with more
	// than one holder.
	Counted uint64
	// Queued counts the stored blocks no holder names, queued at zero, so
	// the collector frees them once the Volume is adopted.
	Queued uint64
	// Unknown counts the holders naming a block whose bytes the Volume stores
	// without a child list. Any refuses the adoption.
	Unknown uint64
	// UnknownSample names up to sampleSize of the blocks, from the slices
	// this run counted: a resumed run does not sample the ones before.
	UnknownSample []*block.BlockRef
}

// ErrAdoptUnknown is returned by Adopt when a root or a stored block names a
// block whose bytes the Volume stores without a child list: the children that
// block names would hold nothing, so its counts cannot be known. Write the
// missing child lists, then adopt again.
var ErrAdoptUnknown = errors.New("named blocks are stored without a child list")

// ErrAdoptSuperseded is returned by Adopt when another adoption, in this
// process or another on the same store, or a Volume opening, took the Volume
// from it. It writes nothing more and leaves the other run's records alone.
var ErrAdoptSuperseded = errors.New("another adoption superseded this one")

// Adopt counts the holders of every block of a Volume whose counts nobody
// trusts, from its child lists and named roots alone, and marks it adopted,
// after which the collector nets the logs and may free. It follows the
// protocol Adopt.tla checks.
//
// A claim takes the unadopted Volume under the run's name and clears every
// record the counts derive from, the logs included, while Land keeps no logs
// on an unadopted Volume. The mark then marks it adopting with the up cursor
// at the up log's end and the time stamped, so every Land after it logs its
// holders and the count needs only the Volume as it was at the mark: the
// holders of a block are the roots and the stored blocks whose child lists
// name it. It streams: it scans the roots and the child lists in key order,
// sliceRecords per transaction, yielding to waiting writers between them,
// and keeps only its scan position and one slice in memory, writing each
// holder into the block's count record as it finds it. While it runs, Land
// marks each block it gives birth to ('B') and records the first block each
// root it moves held at the mark ('A'), so a later slice skips a child list
// born after the mark and counts a moved root at its block at the mark; the
// logs past the mark carry those changes. A stored block then keeps holders
// minus one, one being implied, a stored block with none goes on the free
// queue at the up log's end, and a named block the Volume does not store
// keeps all its holders, since the put that stores it brings the holder its
// record implies.
//
// Invariant: no free runs until Grace has passed since the mark
// (Collector.freeable). A root moved off a block before the mark logged no
// down, which would have made the block wait Grace; a reader that saw the
// block may still be about to pin it. The settle's flip waits the same way,
// since that reader's sight is no seed of its walk.
//
// It refuses with ErrAdoptUnknown, returning the Volume to unadopted, when a
// root or a stored block names a block stored without a child list. It counts
// child lists an older store format left unverified as they are: such a list
// may lack a child, which then counts too few holders, and the settle
// generation keeps those blocks, as Collector describes.
//
// Each slice's transaction moves the Volume's adoption cursor
// (LivenessState.adopt_cursor) past the records it counted, beside their
// counts, so the cursor and the counts always agree. An adoption that is
// interrupted, by its context or a crash, leaves the Volume adopting at its
// cursor, and the next Adopt takes the run over under its own name and
// continues strictly after the cursor: the marks and logs since the mark
// cover every Land between the slices. One that fails returns the Volume to
// unadopted and deletes what it wrote, and the next Adopt starts over and
// reaches the same counts. A new run first claims the unadopted Volume under
// its name, clears only while its claim stands, and marks it adopting only
// from its own claim; every later transaction of a run refuses to write
// unless the Volume is still adopting in that run. A run another claimed,
// took over or superseded writes nothing more and returns
// ErrAdoptSuperseded.
//
// Adoption counts only child lists and named roots. A block that older code
// held another way, such as through a bucket's ownership, has no holder here,
// so adopt a Volume only once every writer that uses it holds what it needs
// under named roots.
func Adopt(ctx context.Context, store Store, now func() time.Time) (AdoptResult, error) {
	// Take over an adoption in progress, report an adopted Volume after
	// deleting what a finished adoption may have left, or claim it unadopted
	// for a new run.
	var res AdoptResult
	run := ulid.NewULID()
	start, owner, err := claimAdoption(ctx, store, run, true)
	if err != nil {
		return res, err
	}
	switch start {
	case adoptAdopted:
		res.Already = true
		return res, clearMarks(ctx, store, owner)
	case adoptClaimed:
		if err := markAdopting(ctx, store, run, now); err != nil {
			return res, err
		}
	}

	// Count the holders, refuse a block stored without a child list, and
	// settle the stored blocks' counts, marking the Volume adopted.
	a := &adoption{store: store, run: run, res: &res}
	if err := a.count(ctx); err != nil {
		return res, abandonAdopt(ctx, store, run, err)
	}

	// Delete the run's marks.
	return res, clearMarks(ctx, store, run)
}

// clearMarks deletes the born and moved-root marks of run once it marked the
// Volume adopted, stopping, deleting nothing more, once the Volume leaves
// that state: a newer run's marks are not run's to delete, and its settle
// counts a born block as old without its mark.
func clearMarks(ctx context.Context, store Store, run string) error {
	adopted := func(state *LivenessState) bool {
		return state.GetAdoption() == Adoption_ADOPTION_ADOPTED && state.GetAdoptionRun() == run
	}
	return clearKinds(ctx, store, adopted, kindAdoptBorn, kindAdoptRoot)
}

// markAdopting clears what an earlier run and the logs left while run's
// claim stands, then marks the Volume adopting in run with a fresh cursor,
// starting fresh logs.
func markAdopting(ctx context.Context, store Store, run string, now func() time.Time) error {
	// Clear while the claim stands.
	if err := clearRun(ctx, store, run); err != nil {
		return err
	}

	// Mark the Volume adopting from the claim.
	return store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Refuse a lost claim.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if !claimedBy(state, run) {
			return ErrAdoptSuperseded
		}

		// Start the run at the first record of its first pass.
		state.Adoption = Adoption_ADOPTION_ADOPTING
		state.AdoptionRun = run
		state.AdoptCursor = &AdoptCursor{Run: run}
		state.UpCursor = state.GetUpSeq()
		state.AdoptAtUnixMilli = now().UnixMilli()
		return rec.SetState(ctx, state)
	})
}

// inRun returns the state of a Volume still adopting in run, whose cursor
// run wrote or took over, or ErrAdoptSuperseded.
func inRun(ctx context.Context, rec *Records, run string) (*LivenessState, error) {
	state, err := rec.GetState(ctx)
	if err != nil {
		return nil, err
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTING || state.GetAdoptionRun() != run || !ownCursor(state) {
		return nil, ErrAdoptSuperseded
	}
	return state, nil
}

// ownCursor reports whether state carries a cursor the run it names wrote.
func ownCursor(state *LivenessState) bool {
	cur := state.GetAdoptCursor()
	return cur != nil && cur.GetRun() == state.GetAdoptionRun()
}

// ResetAdoption returns a Volume an adoption left adopting to unadopted,
// dropping its cursor, so it logs nothing more. A Volume whose collector
// does not free calls it when it opens: no background adoption takes the
// run over there, and the switch that turns frees on returns the Volume to
// unadopted anyway.
func ResetAdoption(ctx context.Context, store Store) error {
	_, _, err := claimAdoption(ctx, store, "", false)
	return err
}

// SetFrees records whether the collector's frees are on, as a writable open
// does, and returns the state it leaves. Turning them on is the switch: the
// same write returns the Volume to unadopted, adopted or not, ending any
// adoption or release in progress, so the next adoption counts every holder
// again before the first free. That recount gives each named block without
// a list one holder more than its namers, a hole no audit recorded included,
// so the hole's next put leaves it held. Turning them off writes only the
// field: the collector then frees nothing, and the next switch-on counts
// again over whatever was written meanwhile. A stored switch whose frees
// mark is stale reads as off (Records.GetState), so a write by a build that
// knew nothing of the switch also makes the next switch-on count again.
func SetFrees(ctx context.Context, store Store, frees bool) (*LivenessState, error) {
	var state *LivenessState
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Read the state; a switch already as asked writes nothing.
		var err error
		state, err = rec.GetState(ctx)
		if err != nil || state.GetFrees() == frees {
			return err
		}

		// Record the switch, and on switching on return to unadopted.
		state.Frees = frees
		if frees {
			state.Adoption = Adoption_ADOPTION_UNADOPTED
			state.AdoptionRun = ""
			state.AdoptCursor = nil
			state.Release = nil
		}
		return rec.SetState(ctx, state)
	})
	return state, err
}

// adoptStart is the state in which an Adopt finds the Volume.
type adoptStart int

const (
	// adoptClaimed is a Volume claimed unadopted under the run.
	adoptClaimed adoptStart = iota
	// adoptResumed is a Volume adopting at its cursor, taken over by the run.
	adoptResumed
	// adoptAdopted is an adopted Volume, left as it was.
	adoptAdopted
)

// claimAdoption claims a Volume that is not adopted for run, superseding any
// run that marked or claimed it, and reports where it started. With resume
// set it takes over an adoption in progress at its cursor, keeping its
// mark, logs and counts, under run's name; otherwise it returns that Volume
// to unadopted. A claim drops the cursor and ends any release in progress.
// An empty run claims the Volume for none. It returns the run that owns the
// Volume after it: run, or for an adopted Volume the run that adopted it.
func claimAdoption(ctx context.Context, store Store, run string, resume bool) (adoptStart, string, error) {
	var start adoptStart
	var owner string
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Leave an adopted Volume.
		start, owner = adoptClaimed, run
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() == Adoption_ADOPTION_ADOPTED {
			start, owner = adoptAdopted, state.GetAdoptionRun()
			return nil
		}

		// Take over an adoption in progress at its cursor. A cursor that
		// does not name the run in progress starts over: a build without
		// the cursor claimed the Volume since and kept the field unread.
		if resume && state.GetAdoption() == Adoption_ADOPTION_ADOPTING && ownCursor(state) {
			start = adoptResumed
			state.AdoptionRun = run
			state.AdoptCursor.Run = run
			return rec.SetState(ctx, state)
		}

		// Claim it, ending any release in progress: the run recounts every
		// holder, and the claim's clear deletes the release pages.
		if claimedBy(state, run) {
			return nil
		}
		state.Adoption = Adoption_ADOPTION_UNADOPTED
		state.AdoptionRun = run
		state.AdoptCursor = nil
		state.Release = nil
		return rec.SetState(ctx, state)
	})
	return start, owner, err
}

// abandonAdopt ends run after it failed or was interrupted, and returns
// cause. An interrupted run writes nothing: the Volume stays adopting at the
// cursor its last slice committed, and the next Adopt takes it over, so a
// Close that cancels a large adoption neither waits for a cleanup nor loses
// the counting done. A failed run returns the Volume to unadopted, dropping
// its cursor, and deletes the counts, queue and marks it wrote. It touches
// nothing once another run holds the Volume: a run superseded it, and the
// records are that run's.
func abandonAdopt(ctx context.Context, store Store, run string, cause error) error {
	// Leave an interrupted run at its cursor.
	if ctx.Err() != nil {
		return cause
	}

	// Mark the Volume unadopted, abandoned by run, while run still holds it.
	var ours bool
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Leave a Volume another run holds.
		state, err := inRun(ctx, rec, run)
		if errors.Is(err, ErrAdoptSuperseded) {
			return nil
		}
		if err != nil {
			return err
		}
		ours = true
		state.Adoption = Adoption_ADOPTION_UNADOPTED
		state.AdoptCursor = nil
		return rec.SetState(ctx, state)
	})

	// Delete what the run wrote, stopping once another run starts.
	if err == nil && ours {
		err = clearRun(ctx, store, run)
	}
	if err != nil {
		return errors.Wrapf(cause, "and returning the volume to unadopted failed: %v", err)
	}
	return cause
}

// adoption is the state of one Adopt call.
type adoption struct {
	// store is the Volume.
	store Store
	// run names the adoption's run.
	run string
	// res is the result.
	res *AdoptResult
}

// count runs the adoption's passes from the cursor: it counts every holder
// into the count records, refuses a block stored without a child list, then
// settles each stored block's count, marking the Volume adopted.
func (a *adoption) count(ctx context.Context) error {
	// Count each root at the block it held when the adoption began.
	err := a.pass(ctx, AdoptPhase_ADOPT_PHASE_ROOTS, a.countRoots, func(_ *Records, _ *LivenessState, cur *AdoptCursor) error {
		cur.Phase = AdoptPhase_ADOPT_PHASE_CHILDREN
		return nil
	})
	if err != nil {
		return err
	}

	// Count the children of each block stored when the adoption began, then
	// refuse when a holder names a block stored without a child list.
	err = a.pass(ctx, AdoptPhase_ADOPT_PHASE_CHILDREN, a.countChildren, func(_ *Records, _ *LivenessState, cur *AdoptCursor) error {
		if cur.GetUnknown() != 0 {
			return errors.Wrapf(ErrAdoptUnknown, "%d holders, sample %v", cur.GetUnknown(), a.res.UnknownSample)
		}
		cur.Phase = AdoptPhase_ADOPT_PHASE_SETTLE
		return nil
	})
	if err != nil {
		return err
	}

	// Settle each stored block: one holder is implied, and a block with none
	// is at zero with children that count it, on the free queue, so the free
	// releases them. The last slice marks the Volume adopted.
	return a.pass(ctx, AdoptPhase_ADOPT_PHASE_SETTLE, a.settleLists, func(rec *Records, state *LivenessState, _ *AdoptCursor) error {
		// Cap the pop mark clearRun raised at the up log's end. The recount
		// gave each block it left at zero the up log's end when it settled
		// that block as its ripe sequence, no later than the end now, so the
		// capped mark still covers each, while a block that dies later
		// cannot hold the flip. The recount returned each unverified child
		// of a list written from content to the front, and only a pop at the
		// child's later ripe sequence takes it off, which raises the mark
		// past its parent again.
		if err := rec.capPopMark(ctx, state.GetUpSeq()); err != nil {
			return err
		}
		state.Adoption = Adoption_ADOPTION_ADOPTED
		state.AdoptCursor = nil
		return nil
	})
}

// adoptSlice handles the records of one pass after the cursor's key in one
// slice's transaction, adding what it counts to cur, and returns the last
// key it handled and whether more may follow.
type adoptSlice func(ctx context.Context, rec *Records, blocks block.StoreOps, state *LivenessState, cur *AdoptCursor) ([]byte, bool, error)

// pass runs the slices of phase p from the cursor while the run holds the
// Volume, sliceRecords records per transaction, yielding to waiting writers
// between them. Each slice's transaction moves the cursor past the records
// it handled, in the state record it writes beside their counts; the last
// one calls end, which moves the cursor on, refuses or finishes, in the same
// transaction. A cursor past p skips the pass.
func (a *adoption) pass(ctx context.Context, p AdoptPhase, slice adoptSlice, end func(rec *Records, state *LivenessState, cur *AdoptCursor) error) error {
	_, err := eachSlice(ctx, a.store.Update, struct{}{}, nil, func(rec *Records, blocks block.StoreOps, _ struct{}) (struct{}, bool, error) {
		// Read the cursor while the run holds the Volume.
		state, err := inRun(ctx, rec, a.run)
		if err != nil {
			return struct{}{}, false, err
		}
		cur := state.GetAdoptCursor()
		if cur.GetPhase() != p {
			return struct{}{}, false, nil
		}

		// Handle the slice, then move the cursor past it, or end the pass.
		last, more, err := slice(ctx, rec, blocks, state, cur)
		if err != nil {
			return struct{}{}, false, err
		}
		if more {
			cur.After = last
		} else {
			cur.After = nil
			if err := end(rec, state, cur); err != nil {
				a.report(cur)
				return struct{}{}, false, err
			}
		}
		a.report(cur)
		return struct{}{}, more, rec.SetState(ctx, state)
	})
	return err
}

// report copies what the cursor counted into the result.
func (a *adoption) report(cur *AdoptCursor) {
	// Copy the counters the cursor carries across runs.
	a.res.Blocks = cur.GetBlocks()
	a.res.Roots = cur.GetRoots()
	a.res.Counted = cur.GetCounted()
	a.res.Queued = cur.GetQueued()
	a.res.Unknown = cur.GetUnknown()
}

// countChildren counts a holder of each child of the blocks of the next
// child lists, skipping a block born since the adoption began.
func (a *adoption) countChildren(ctx context.Context, rec *Records, blocks block.StoreOps, _ *LivenessState, cur *AdoptCursor) ([]byte, bool, error) {
	return scanLists(ctx, rec, cur.GetAfter(), func(recs []adoptRecord) error {
		// Read the generation once for the slice.
		gen, err := rec.generation(ctx)
		if err != nil {
			return err
		}
		h := make(holds)
		for _, r := range recs {
			// Skip a block born since the adoption began.
			ref, born, err := a.listed(ctx, rec, r)
			if err != nil {
				return err
			}
			if born {
				continue
			}

			// Count a holder of each child. While Open a list written from
			// content returns each unverified child to the settle's front:
			// the free proof may have popped the child while a short list
			// left it Dead, and once this count holds it again the walk must
			// verify it before the flip, or the sweep deletes it.
			returns := listMarked(r.value) && gen == Generation_GENERATION_OPEN
			for packed := listKeys(r.value); len(packed) != 0; {
				key, ok := nextBlockKey(packed)
				if !ok {
					return errors.Wrapf(errBadBlockKey, "child list of %s", ref.MarshalString())
				}
				packed = packed[len(key):]
				h.add(key, returns)
			}
		}
		return a.hold(ctx, rec, blocks, cur, h)
	})
}

// settleLists settles the blocks of the next child lists.
func (a *adoption) settleLists(ctx context.Context, rec *Records, _ block.StoreOps, state *LivenessState, cur *AdoptCursor) ([]byte, bool, error) {
	return scanLists(ctx, rec, cur.GetAfter(), func(recs []adoptRecord) error {
		for _, r := range recs {
			if err := a.settle(ctx, rec, state, cur, r); err != nil {
				return err
			}
		}
		return nil
	})
}

// scanLists calls fn with up to sliceRecords child list records after the
// key after, in key order, and returns the last key and whether more may
// follow.
func scanLists(ctx context.Context, rec *Records, after []byte, fn func(recs []adoptRecord) error) ([]byte, bool, error) {
	recs, err := rec.scanAfter(ctx, kindChildren, after, sliceRecords)
	if err != nil || len(recs) == 0 {
		return nil, false, err
	}
	if err := fn(recs); err != nil {
		return nil, false, err
	}
	return recs[len(recs)-1].key, len(recs) == sliceRecords, nil
}

// settle writes the holders the count gave the block of the list record r,
// or settles it at zero on the free queue, ripe at the up log's end in
// state, when it has none. It skips a block born since the adoption began.
func (a *adoption) settle(ctx context.Context, rec *Records, state *LivenessState, cur *AdoptCursor, r adoptRecord) error {
	// Skip a block born since the adoption began.
	ref, born, err := a.listed(ctx, rec, r)
	if err != nil || born {
		return err
	}

	// Write the holders of a block a holder names.
	cur.Blocks++
	n, err := rec.tally(ctx, ref)
	if err != nil {
		return err
	}
	if n != 0 {
		if n > 1 {
			cur.Counted++
		}
		return rec.SetHolders(ctx, ref, n, 0)
	}

	// Settle a block no holder names at zero, ripe at the up log's end.
	cur.Queued++
	ripe := state.GetUpSeq()
	if err := rec.SetHolders(ctx, ref, 0, ripe); err != nil {
		return err
	}
	return rec.tx.Set(ctx, rec.queueKey(ripe, BlockKey(ref)), nil)
}

// countRoots counts each name after the cursor's that held a root when the
// adoption began at the block it held then. It scans the root table and the
// moved roots' marks as one sequence of names in key order, so it reads each
// name once: a name a Land moved before the scan reached it has a mark
// holding its starting block, and one moved after had none to read, so the
// scan read its starting block from the table.
func (a *adoption) countRoots(ctx context.Context, rec *Records, blocks block.StoreOps, _ *LivenessState, cur *AdoptCursor) ([]byte, bool, error) {
	// Read the next names of either kind.
	names, err := rec.scanNamesAfter(ctx, cur.GetAfter(), sliceRecords, kindRoot, kindAdoptRoot)
	if err != nil || len(names) == 0 {
		return nil, false, err
	}
	h := make(holds)
	for _, name := range names {
		// Read the starting block from the mark, or else the table.
		start, moved, err := rec.tx.Get(ctx, rec.key(kindAdoptRoot, name))
		if err != nil {
			return nil, false, err
		}
		if !moved {
			value, _, err := rec.tx.Get(ctx, rec.key(kindRoot, name))
			if err != nil {
				return nil, false, err
			}
			entry := &RootEntry{}
			if err := entry.UnmarshalVT(value); err != nil {
				return nil, false, err
			}
			start = entry.GetRef()
		}

		// Count the holder of a name that held a block.
		if len(start) != 0 {
			if _, rest, err := ParseBlockKey(start); err != nil || len(rest) != 0 {
				return nil, false, errBadBlockKey
			}
			cur.Roots++
			h.add(start, false)
		}
	}
	if err := a.hold(ctx, rec, blocks, cur, h); err != nil {
		return nil, false, err
	}
	return names[len(names)-1], len(names) == sliceRecords, nil
}

// listed returns the block of a child list record and whether Land gave
// birth to it since the mark, so its holders come from the logs and the
// adoption neither counts its children nor settles it.
func (a *adoption) listed(ctx context.Context, rec *Records, r adoptRecord) (*block.BlockRef, bool, error) {
	ref, rest, err := ParseBlockKey(r.key)
	if err != nil || len(rest) != 0 {
		return nil, false, errBadBlockKey
	}
	born, err := rec.tx.Exists(ctx, rec.key(kindAdoptBorn, BlockKey(ref)))
	return ref, born, err
}

// holds is the holders one slice of the count gives each block, by block
// key, so the slice reads each block's count and list once, in key order.
type holds map[string]holding

// holding is the holders a slice gives one block, and whether one of them
// returns it to the front: a list written from content while Open.
type holding struct {
	n    uint64
	open bool
}

// add counts one holder of the block at key.
func (h holds) add(key []byte, open bool) {
	e := h[string(key)]
	e.n++
	e.open = e.open || open
	h[string(key)] = e
}

// hold adds the holders h counts to the count records, and adds each holder
// of a block the Volume stores without a child list to cur's unknown. While Open a
// block a holder opens returns to the front unless its list is from content:
// the free proof may have popped it while it was Dead, and the walk must
// verify it before the flip. It reads the counts, the lists and the unlisted
// blocks in three batches in key order, so a slice costs batched reads
// rather than a random point read per holder.
func (a *adoption) hold(ctx context.Context, rec *Records, blocks block.StoreOps, cur *AdoptCursor, h holds) error {
	// Order the blocks by key.
	keys := slices.Sorted(maps.Keys(h))
	if len(keys) == 0 {
		return nil
	}

	// Add the holders to the counts, read in one batch.
	reads := make([][]byte, len(keys))
	for i, key := range keys {
		reads[i] = rec.key(kindCount, []byte(key))
	}
	counts, counted, err := kvtx.GetBatch(ctx, rec.tx, reads)
	if err != nil {
		return err
	}
	var opens bool
	for i, key := range keys {
		var n uint64
		if counted[i] {
			var size int
			if n, size = binary.Uvarint(counts[i]); size <= 0 {
				return errCountMalformed
			}
		}
		if err := rec.tx.Set(ctx, reads[i], binary.AppendUvarint(nil, n+h[key].n)); err != nil {
			return err
		}
		opens = opens || h[key].open
	}

	// Read the lists in one batch: their values when a holder opens a block,
	// and otherwise only whether each exists.
	for i, key := range keys {
		reads[i] = rec.key(kindChildren, []byte(key))
	}
	var lists [][]byte
	var listed []bool
	if opens {
		lists, listed, err = kvtx.GetBatch(ctx, rec.tx, reads)
	} else {
		listed, err = kvtx.ExistsBatch(ctx, rec.tx, reads)
	}
	if err != nil {
		return err
	}

	// Return each opened block whose list is not from content to the front,
	// and gather the unlisted blocks.
	var unlisted []*block.BlockRef
	var unlistedHolders []uint64
	for i, key := range keys {
		if !listed[i] {
			ref, _, err := ParseBlockKey([]byte(key))
			if err != nil {
				return err
			}
			unlisted = append(unlisted, ref)
			unlistedHolders = append(unlistedHolders, h[key].n)
			continue
		}
		if h[key].open && !listMarked(lists[i]) {
			if err := rec.tx.Set(ctx, rec.key(kindFront, []byte(key)), nil); err != nil {
				return err
			}
		}
	}

	// Note the holders of each unlisted block whose bytes the Volume stores.
	if len(unlisted) == 0 {
		return nil
	}
	found, err := blocks.GetBlockExistsBatch(ctx, unlisted)
	if err != nil {
		return err
	}
	for i, ok := range found {
		if ok {
			cur.Unknown += unlistedHolders[i]
			a.res.UnknownSample = sample(a.res.UnknownSample, unlisted[i])
		}
	}
	return nil
}

// adoptionKinds are the kinds of record an adoption writes or replaces, which
// it clears before it counts the lists and after a failed run: the counts,
// free queue and parked records, the up and down logs, its marks, release
// pages, the record that a recount is due, and the counts the settle froze
// at the flip.
var adoptionKinds = []byte{
	kindCount, kindQueue, kindParked, kindUpLog, kindDownLog, kindAdoptBorn,
	kindAdoptRoot, kindReleasePage, kindRewritten, kindUnmarkedCount,
}

// claimedBy reports whether state is unadopted under run's claim.
func claimedBy(state *LivenessState, run string) bool {
	return state.GetAdoption() == Adoption_ADOPTION_UNADOPTED && state.GetAdoptionRun() == run
}

// clearRun takes each parked block out of the run, then deletes every
// record of the adoptionKinds, stopping, deleting nothing more, once the
// Volume leaves run's claim. Land writes none of them while the Volume is
// unadopted, so the clear ends with none left. A parked block is in the run, as Records.park describes, and the recount may
// find it held, so it leaves the run with its parked record; if it is Dead
// again, the free parks it again.
func clearRun(ctx context.Context, store Store, run string) error {
	// Take the parked blocks out of the run while claimed. The recount gives
	// a block Dead before it a new ripe sequence, so a pop mark covers every
	// queued block until Adopt caps it once the recount is done.
	claimed := func(state *LivenessState) bool { return claimedBy(state, run) }
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		state, err := rec.GetState(ctx)
		if err != nil || !claimed(state) {
			return err
		}
		if _, popped, err := rec.popMark(ctx); err != nil || !popped {
			return err
		}
		return rec.raisePopMark(ctx, math.MaxUint64)
	})
	if err != nil {
		return err
	}
	_, err = eachSlice(ctx, store.Update, []byte(nil), nil, func(rec *Records, _ block.StoreOps, after []byte) ([]byte, bool, error) {
		state, err := rec.GetState(ctx)
		if err != nil || !claimed(state) {
			return nil, false, err
		}
		return rec.unrunParked(ctx, after)
	})
	if err != nil {
		return err
	}

	// Clear the adoption's records.
	return clearKinds(ctx, store, claimed, adoptionKinds...)
}

// clearKinds deletes every record of the kinds, sliceRecords per
// transaction, yielding to waiting writers between transactions. With keep
// set it stops, deleting nothing more, at the first transaction whose state
// keep refuses.
func clearKinds(ctx context.Context, store Store, keep func(*LivenessState) bool, kinds ...byte) error {
	for _, kind := range kinds {
		var stop bool
		_, err := eachSlice(ctx, store.Update, []byte(nil), nil, func(rec *Records, _ block.StoreOps, after []byte) ([]byte, bool, error) {
			// Stop at a state keep refuses, then delete the records of the
			// kind from the last key the slice before reached.
			stop = false
			if keep != nil {
				state, err := rec.GetState(ctx)
				if err != nil {
					return nil, false, err
				}
				if stop = !keep(state); stop {
					return nil, false, nil
				}
			}
			recs, err := rec.scanAfter(ctx, kind, after, sliceRecords)
			if err != nil {
				return nil, false, err
			}
			for _, r := range recs {
				if err := rec.tx.Delete(ctx, rec.key(kind, r.key)); err != nil {
					return nil, false, err
				}
				after = r.key
			}
			return after, len(recs) == sliceRecords, nil
		})
		if err != nil || stop {
			return err
		}
	}
	return nil
}
