package space_world

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
)

// rootMoveQueue holds the root moves of admissions, in the order the
// committer's turn made them, until a lander stores them. It lets the turn
// update the adapter's held roots and root sets without waiting on the
// store's writer: the move lands in the admission's durable step, or earlier
// when another root move of the adapter needs it landed first.
//
// Each queued move carries its Previous, so the store refuses a misordered
// landing with block.ErrRootMoved instead of losing a root. A landing that
// fails drops every queued move, since each later move chains through an
// earlier one, fails the admissions that made them, and marks the adapter's
// held view stale until the next rootMtx holder reads it again.
//
// Invariant, which the collector relies on to free nothing a durable state
// names: a payload or candidate block a pending admission needs stays held,
// by a live stage, a kept page the store does not hold yet, or a durable root,
// from Admit until the collector can see the durable root that holds it, and
// that root commits no later than the state write that queues the operation.
// The writer's stage ends with the transaction's retirement, before the
// durable step, so the moves of a candidate it holds pages of (Staged) land in
// the turn and never queue. The overflow stage lasts until the admission's
// newest move lands or is dropped. A sweep keeps the pages the queued moves
// reach.
// opQueue.execute lands an admission's moves before it writes the state, and
// a queued move that requires its blocks held keeps RequireHeld, so a block
// freed after the turn's check refuses the landing and fails the admission
// instead of being named.
//
// An admission that finds a block already held by another admission's queued
// move borrows that hold. depend makes the borrow visible to a drop: it
// queues an empty marker move behind the moves the admission saw, so a drop
// fails it too, and it fails the admission at once when a drop came between
// begin and depend, since its view then named moves no landing will store.
// A lander drops without rootMtx, so a drop can fall between two moves of one
// turn, such as its payload move and its candidate move, however the turn
// holds rootMtx. push therefore refuses a move for an owner whose turn began
// before the latest drop: every move an owner queues belongs to one drop
// epoch and is dropped with the others. RootMoveQueue.tla models it as
// DurableStateHolds and DurableGraphWhole.
//
// Locks: rootMtx, then landMtx, then mtx. A lander holds landMtx across the
// store's SetRoots and never needs rootMtx, so a turn that queues a move under
// rootMtx waits on no landing. mtx is never held across a call into the store.
type rootMoveQueue struct {
	// landMtx serializes landings.
	landMtx csync.Mutex
	// mtx guards the fields below and each queued admission's
	// admissionMoves.
	mtx sync.Mutex
	// moves are the queued moves, oldest first.
	moves []*queuedRootMove
	// seq numbers the most recently queued move.
	seq uint64
	// done is the newest seq that landed or was dropped.
	done uint64
	// stale is set when a dropped move left the adapter's held roots and root
	// sets ahead of the store.
	stale bool
	// epoch counts the drops, and dropped is the error of the latest one.
	epoch   uint64
	dropped error
}

// begin records the drop epoch before owner's turn reads the held view.
func (q *rootMoveQueue) begin(owner *admissionMoves) {
	q.mtx.Lock()
	owner.epoch = q.epoch
	q.mtx.Unlock()
}

// depend ends owner's turn: it fails with the drop's error when a drop came
// since begin, and otherwise queues an empty marker move unless owner's newest
// move is the last one queued, so every queued hold owner's view borrowed
// fails owner when it drops. The caller holds no lock of the queue.
func (q *rootMoveQueue) depend(owner *admissionMoves) error {
	// Fail on a drop since begin, or queue the marker behind what owner saw.
	q.mtx.Lock()
	defer q.mtx.Unlock()
	if owner.epoch != q.epoch {
		return q.dropped
	}
	if n := len(q.moves); n != 0 && q.moves[n-1].seq != owner.seq {
		q.appendLocked(&queuedRootMove{owner: owner})
	}
	return nil
}

// push queues a move that roots makes with entries for owner and records it as
// owner's newest move. It refuses with the drop's error when a drop came since
// owner's begin: that drop removed owner's earlier moves, such as the strict
// move that holds a payload the new move reaches, so every move of an owner
// belongs to one drop epoch and no move outlives the holds queued before it.
// The caller holds rootMtx.
func (q *rootMoveQueue) push(owner *admissionMoves, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Append the move unless a drop since begin removed owner's earlier moves.
	q.mtx.Lock()
	defer q.mtx.Unlock()
	if owner.epoch != q.epoch {
		return q.dropped
	}
	q.appendLocked(&queuedRootMove{owner: owner, entries: entries, roots: roots})
	return nil
}

// appendLocked queues m with the next seq as its owner's newest move. The
// caller holds mtx.
func (q *rootMoveQueue) appendLocked(m *queuedRootMove) {
	q.seq++
	m.seq = q.seq
	q.moves = append(q.moves, m)
	m.owner.seq = q.seq
}

// releaseWith runs release once owner's newest move lands or is dropped, and
// reports false, having run nothing, when that already happened or owner
// queued no move.
func (q *rootMoveQueue) releaseWith(owner *admissionMoves, release func()) bool {
	// Attach release to owner's newest move while it is queued.
	q.mtx.Lock()
	defer q.mtx.Unlock()
	i := slices.IndexFunc(q.moves, func(m *queuedRootMove) bool { return m.seq == owner.seq })
	if i < 0 {
		return false
	}
	q.moves[i].release = append(q.moves[i].release, release)
	return true
}

// entries returns the queued entries by the marshaled ref of the block each
// stores, in a new map the caller may extend.
func (q *rootMoveQueue) entries() map[string]*block.PutBatchEntry {
	// Index every queued move's entries.
	q.mtx.Lock()
	defer q.mtx.Unlock()
	out := make(map[string]*block.PutBatchEntry)
	for _, m := range q.moves {
		for _, e := range m.entries {
			out[e.Ref.MarshalString()] = e
		}
	}
	return out
}

// refs returns the roots the queued moves name and the blocks their entries
// reference, which a sweep keeps: the move stores the kept pages they reach.
func (q *rootMoveQueue) refs() []*block.BlockRef {
	// Collect the roots and child refs of every queued move.
	q.mtx.Lock()
	defer q.mtx.Unlock()
	var refs []*block.BlockRef
	for _, m := range q.moves {
		for _, root := range m.roots {
			refs = append(refs, root.Ref)
		}
		for _, e := range m.entries {
			refs = append(refs, e.Refs...)
		}
	}
	return refs
}

// isStale reports whether a dropped move left the held view ahead of the store.
func (q *rootMoveQueue) isStale() bool {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	return q.stale
}

// fresh clears stale once the caller read the held view again under rootMtx.
func (q *rootMoveQueue) fresh() {
	q.mtx.Lock()
	q.stale = false
	q.mtx.Unlock()
}

// landAll lands every queued move on store.
func (q *rootMoveQueue) landAll(ctx context.Context, store block.RootOps) error {
	unlock, err := q.landMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return q.landLocked(ctx, store, 0)
}

// landFor lands owner's moves and those queued before them on store, and
// returns the error that dropped one of owner's moves, if any.
func (q *rootMoveQueue) landFor(ctx context.Context, store block.RootOps, owner *admissionMoves) error {
	// Take the landing turn.
	unlock, err := q.landMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	// Land through owner's newest move unless an earlier landing did.
	q.mtx.Lock()
	seq, done := owner.seq, q.done
	q.mtx.Unlock()
	if seq > done {
		if err := q.landLocked(ctx, store, seq); err != nil && ctx.Err() != nil {
			return err
		}
	}

	// Report a drop of any of owner's moves.
	q.mtx.Lock()
	defer q.mtx.Unlock()
	return owner.err
}

// landLocked lands the queued moves up to through, or all of them for zero,
// in one SetRoots on store. The caller holds landMtx, so only it pops the
// queue. A refused or failed SetRoots drops every queued move.
func (q *rootMoveQueue) landLocked(ctx context.Context, store block.RootOps, through uint64) error {
	// Take the prefix to land.
	q.mtx.Lock()
	n := len(q.moves)
	if through != 0 {
		n = slices.IndexFunc(q.moves, func(m *queuedRootMove) bool { return m.seq > through })
		if n < 0 {
			n = len(q.moves)
		}
	}
	batch := slices.Clone(q.moves[:n])
	q.mtx.Unlock()
	if len(batch) == 0 {
		return nil
	}

	// Store them as one move from the first Previous of each name to its
	// last Ref.
	// A batch of markers stores nothing.
	entries, roots, err := collapseRootMoves(batch)
	if err == nil && (len(entries) != 0 || len(roots) != 0) {
		err = store.SetRoots(ctx, entries, roots)
	}

	// Pop them and release their stages, or drop the queue.
	switch {
	case err == nil:
		q.mtx.Lock()
		q.moves = q.moves[n:]
		q.done = batch[n-1].seq
		q.mtx.Unlock()
		releaseMoves(batch)
	case ctx.Err() == nil:
		q.dropLocked(err)
	}
	return err
}

// drop drops every queued move with err, as a failed landing does, without
// storing any of them.
func (q *rootMoveQueue) drop(ctx context.Context, err error) error {
	// Drop in the landing turn, so no landing pops the queue meanwhile.
	unlock, lerr := q.landMtx.Lock(ctx)
	if lerr != nil {
		return lerr
	}
	defer unlock()
	q.dropLocked(err)
	return nil
}

// dropLocked fails the admissions of every queued move with err, marks the
// held view stale and releases the moves' stages. The caller holds landMtx.
func (q *rootMoveQueue) dropLocked(err error) {
	// Empty the queue, failing each owner, then end the stages outside mtx.
	q.mtx.Lock()
	ended := q.moves
	for _, m := range ended {
		m.owner.err = err
	}
	q.done = q.seq
	q.moves, q.stale = nil, true

	// Fail the turns that read the view before the drop, as depend checks.
	q.epoch++
	q.dropped = err
	q.mtx.Unlock()
	releaseMoves(ended)
}

// queued reports whether owner has a move that has not landed or been dropped.
func (q *rootMoveQueue) queued(owner *admissionMoves) bool {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	return owner.seq > q.done
}

// releaseMoves ends the stages that held the blocks of moves.
func releaseMoves(moves []*queuedRootMove) {
	for _, m := range moves {
		for _, release := range m.release {
			release()
		}
	}
}

// collapseRootMoves merges moves into one: per name, in order of first
// appearance, the first move's Previous and the last move's Ref, requiring its
// blocks held when any move did, and the entries the merged roots reach, in
// queue order. An entry a later move replaced, such as an earlier set block of
// the same name, is left out, since storing it would put a block nothing
// holds. A move whose Previous is not the Ref the move before it on the same
// name set is refused with block.ErrRootMoved, since merging it would skip a
// block.
func collapseRootMoves(moves []*queuedRootMove) ([]*block.PutBatchEntry, []block.NamedRoot, error) {
	// Merge the roots by name.
	var roots []block.NamedRoot
	at := make(map[string]int)
	byRef := make(map[string]*block.PutBatchEntry)
	for _, m := range moves {
		for _, e := range m.entries {
			byRef[e.Ref.MarshalString()] = e
		}
		for _, root := range m.roots {
			i, ok := at[root.Name]
			if !ok {
				at[root.Name] = len(roots)
				roots = append(roots, root)
				continue
			}
			if !refEqual(root.Previous, roots[i].Ref) {
				return nil, nil, block.RootMoved(root.Name)
			}
			roots[i].Ref, roots[i].Lease = root.Ref, root.Lease
			roots[i].RequireHeld = roots[i].RequireHeld || root.RequireHeld
		}
	}

	// Keep the entries the merged roots reach.
	reached := make(map[string]struct{}, len(byRef))
	var walk []*block.BlockRef
	for _, root := range roots {
		walk = append(walk, root.Ref)
	}
	for len(walk) != 0 {
		ref := walk[len(walk)-1]
		walk = walk[:len(walk)-1]
		if ref.GetEmpty() {
			continue
		}
		key := ref.MarshalString()
		e := byRef[key]
		if _, ok := reached[key]; ok || e == nil {
			continue
		}
		reached[key] = struct{}{}
		walk = append(walk, e.Refs...)
	}
	var entries []*block.PutBatchEntry
	for _, m := range moves {
		for _, e := range m.entries {
			key := e.Ref.MarshalString()
			if _, ok := reached[key]; ok {
				entries = append(entries, e)
				delete(reached, key)
			}
		}
	}
	return entries, roots, nil
}
