package block_gc

import (
	"bytes"
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// The settle of an old generation.
//
// A Volume whose child lists came from a store format before 7 holds lists
// that may lack a child: the old reference graph wrote a block's edges in
// batches, and a crash between two batches left the list cut. A count drawn
// from a cut list misses a holder, so a free could take a block a live block
// still references. The settle proves which old blocks are live before
// anything frees. The generation, in the state record, moves Open, Sweeping,
// Settled, and a copied put of a block a Settled Volume does not store moves
// it back to Open, as Records.reopen describes.
//
// A list is young or verified when it was written from its block's content:
// by a Land or a refill, or by the typed walk, which decodes the block. Its
// value begins with listMark. Every other list is unverified.
//
// Invariants, while the generation is Open:
//
//   - Nothing is deleted: the free parks each block it would free, and
//     enters it in the run, as Records.park describes.
//   - The front holds every child of a young or verified block that has no
//     list from content, stored or not. Land pushes those children of each
//     block it writes from content, and each such block a root moves onto.
//     The walk pushes those children of each block it decodes. So the walk
//     never descends below a verified block, and no young block needs
//     seeding.
//   - A block the Volume lacks, as a peer's cache lacks what it never read,
//     has no list, so no count holds a stored block only it references. The
//     walk reads it through the full read chain, peers included, without
//     filling the Volume, and writes its list from content without its
//     bytes, as an evicted block keeps its list. The list then holds what lies
//     below. An absent block no read resolves stays on the front while it has
//     holders, and holds the flip.
//   - Once its completion entry exists, the unmarked run records every block
//     whose list is unverified: the run build adds each, and every write that
//     marks a list deletes the block's entry in the same transaction. One
//     writer adds an unverified list: a Land of a copied put, whose refs the
//     writer did not read from content, through Records.putCopied, which
//     writes the block's run entry and front entry in the same transaction,
//     so the run stays complete and the walk decodes the block.
//
// Before the flip the collector seeds the front, in slices: it pushes every
// owed block a named root or a down page not netted yet references, as
// seed describes, the model's WalkStart. The flip is one state write, to
// Sweeping, in a transaction that finds the Volume freeable, as
// Collector.freeable describes, no list rewrite awaiting a recount, no
// recorded hole, the run complete, the front empty and every ripe Dead
// block parked: the model's WalkEnd. Every block still unverified is then
// unreachable: no seed references it, and no young or verified block does,
// since the front is empty. The run records those blocks and the parked
// ones.
//
// While Sweeping, an empty run entry identifies an unmarked block, which every
// site that asks whether a block is stored answers absent, as Unmarked
// describes. The flip, which requires no paged release in progress, freezes
// each unmarked block's count record and counts its holders from the flip on
// apart, as kindUnmarkedCount describes. A stored block holds its children
// until its free, and the free skips an unmarked block, so its old list still
// holds them. The sweep deletes each unmarked block, releasing the holder its
// old list gave each child outside the run, as releaseOld decides. A put of an
// unmarked block gives birth to it again: its entry turns reborn, its old list
// releases the same way, and it keeps the holders it gained since the flip. The
// sweep marks each entry it deletes swept rather than deleting it, so the run
// stays the run at the flip. Once no unmarked block is left, the sweep deletes
// the run and the state settles.

// AbsentError is returned by a Land that holds a hidden block, as
// Records.Hidden describes: one the settle unmarked, which no live block
// references and the sweep will delete, or one the free parked while Open. The
// writer puts it again, as for a block the store holds without a child list.
type AbsentError struct {
	// Ref is the block.
	Ref *block.BlockRef
}

// Error describes the block.
func (e *AbsentError) Error() string {
	return "block " + e.Ref.MarshalString() + " reads absent until it is put again"
}

// Unwrap returns block.ErrRefsUnknown, which tells a writer to put the
// block again.
func (e *AbsentError) Unwrap() error {
	return block.ErrRefsUnknown
}

// Unmarked reports whether the block at key is unmarked: the generation is
// Sweeping and the block's run entry is neither reborn nor swept. It reads
// nothing more than the generation outside Sweeping.
func (r *Records) Unmarked(ctx context.Context, key []byte) (bool, error) {
	gen, err := r.generation(ctx)
	if err != nil || gen != Generation_GENERATION_SWEEPING {
		return false, err
	}
	return r.inRun(ctx, key)
}

// inRun reports whether the block at key has an empty run entry, so is
// neither reborn nor swept since the flip. It reads one key and not the
// generation, so the caller knows the generation is Sweeping.
func (r *Records) inRun(ctx context.Context, key []byte) (bool, error) {
	value, found, err := r.tx.Get(ctx, r.key(kindUnmarked, key))
	return found && len(value) == 0, err
}

// countKind returns the kind of the count record of the block at key:
// kindUnmarkedCount while it is unmarked, since the flip froze its kindCount
// record, and kindCount otherwise. Outside Sweeping it reads nothing more
// than the generation. While Sweeping, it reads one run entry.
func (r *Records) countKind(ctx context.Context, key []byte) (byte, error) {
	unmarked, err := r.Unmarked(ctx, key)
	if err != nil || !unmarked {
		return kindCount, err
	}
	return kindUnmarkedCount, nil
}

// releaseOld ends the old generation's records of the unmarked block at
// key as it leaves the run, by the sweep or a rebirth, and returns the block
// keys of the children outside the run its old list holds, which lose that
// holder: a stored block holds its children until its free, and the free
// skips an unmarked block, so its old list holds them still. It moves its
// kindUnmarkedCount record, the holders it gained and lost since the flip,
// over its frozen kindCount record. A child inside the run keeps no holder
// from it, since the flip dropped the holders of every unmarked block.
//
// A block whose park released its holds gave every child its down when it
// parked, so it returns none and deletes its released key, the record of
// that fact, in the same transaction: no path gives such a list a second
// down. The frozen kindCount record still names the key, since nothing
// changes an unmarked block's kindCount after the flip. An adoption that
// recounted after the flip deleted the key with the count, and its recount
// gave the list its holds back, so the list then releases as any other.
func (r *Records) releaseOld(ctx context.Context, key []byte) ([][]byte, error) {
	// Release nothing for a released park, deleting its key, and otherwise
	// the old list's children outside the run.
	var released [][]byte
	ripe, _, gone, err := r.parkState(ctx, key)
	switch {
	case err != nil:
		return nil, err
	case gone:
		if err := r.tx.Delete(ctx, r.releasedKey(ripe, key)); err != nil {
			return nil, err
		}
	default:
		list, _, err := r.childList(ctx, key)
		if err != nil {
			return nil, err
		}
		if released, err = r.outsideRun(ctx, bytes.Clone(listKeys(list))); err != nil {
			return nil, err
		}
	}

	// Move the count since the flip over the frozen one.
	count, found, err := r.tx.Get(ctx, r.key(kindUnmarkedCount, key))
	switch {
	case err != nil:
		return nil, err
	case !found:
		return released, r.tx.Delete(ctx, r.key(kindCount, key))
	}
	if err := r.tx.Set(ctx, r.key(kindCount, key), bytes.Clone(count)); err != nil {
		return nil, err
	}
	return released, r.tx.Delete(ctx, r.key(kindUnmarkedCount, key))
}

// listState reads the list record of the block at key: whether it exists,
// and whether it was written from content.
func (r *Records) listState(ctx context.Context, key []byte) (listed, marked bool, err error) {
	value, found, err := r.childList(ctx, key)
	return found, found && listMarked(value), err
}

// pushOwed pushes the block at key onto the front when the walk owes it a
// decode, as its list is not from content or it has none, whether or not
// the Volume stores its bytes, and reports whether it did. An absent block
// has no list, so no count holds a stored block only it references: it stays on
// the front while it has holders, until the walk reads it through the full
// read chain and writes its list, as ListRebuild describes. The walk owes
// a written-off hole no decode, so no push returns one to the front, as
// Records.writeOffHole describes. A block without a list pushed at one holder
// may be one nothing references, which only the reference scan pops, so the
// push notes it for PushedUnborn.
//
// Cost: one list read, and for a block without a list a count read and one
// point read more, or two while a hole is recorded.
func (r *Records) pushOwed(ctx context.Context, key []byte) (bool, error) {
	listed, marked, err := r.listState(ctx, key)
	if err != nil || marked {
		return false, err
	}
	if !listed {
		if off, err := r.writtenOff(ctx, key); err != nil || off {
			return false, err
		}
		holders, _, err := r.holders(ctx, key)
		if err != nil {
			return false, err
		}
		r.pushedUnborn = r.pushedUnborn || holders == 1
	}
	return true, r.addFront(ctx, key)
}

// pushChildren pushes each child in packed, block keys as a list holds
// them, that the walk owes a decode onto the front, skipping a child skip
// reports.
func (r *Records) pushChildren(ctx context.Context, packed []byte, skip func(key []byte) bool) error {
	for len(packed) != 0 {
		key, ok := nextBlockKey(packed)
		if !ok {
			return errBadBlockKey
		}
		packed = packed[len(key):]
		if skip != nil && skip(key) {
			continue
		}
		if _, err := r.pushOwed(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// markedYoung finishes a write that marked the list of the block at key,
// packed its children, while the generation is Open: the block leaves the
// front, and the run unless the free parked it, as Records.park describes,
// and the children the walk owes join the front, which keeps the front
// invariant. It reads the block's count and parked key.
func (r *Records) markedYoung(ctx context.Context, key, packed []byte, skip func(key []byte) bool) error {
	// Leave the run unless parked.
	parked, err := r.parked(ctx, key)
	if err != nil {
		return err
	}
	if !parked {
		if err := r.tx.Delete(ctx, r.key(kindUnmarked, key)); err != nil {
			return err
		}
	}

	// Leave the front and push the children.
	if err := r.tx.Delete(ctx, r.key(kindFront, key)); err != nil {
		return err
	}
	return r.pushChildren(ctx, packed, skip)
}

// putCopied writes children, refs a put copied rather than read from the
// content of ref, as its unverified list while the generation is Open, and
// enters the block in the run and on the front in the same transaction: the
// run then still names every unverified list, and the walk decodes the
// block, marks its list from content and pushes what it lacks. It is the one
// writer of an unmarked list, as the run's invariant describes.
func (r *Records) putCopied(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef) error {
	key := BlockKey(ref)
	if err := r.tx.Set(ctx, r.key(kindChildren, key), packChildren(nil, children)); err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.key(kindUnmarked, key), nil); err != nil {
		return err
	}
	return r.addFront(ctx, key)
}

// reopen moves a Settled Volume back to Open for a copied put of a block it
// does not store, which Records.putCopied then writes, in the same
// transaction, and sets state, which the caller writes. The reopened Volume
// is one that was Open all along with the copied block in it:
//
//   - Settled holds no run, front, park, unmarked count or pop mark: the
//     sweep deleted every unmarked block and every released key with it, the
//     collector moved every other parked block to the free queue, the flip
//     deleted the pop mark, and the settle's last step
//     deleted the run. Every list is marked, since while Sweeping and
//     Settled no writer stores an unmarked one. So the only unverified list
//     is the copied block's, which putCopied enters in the run, and reopen
//     writes the run's completion entry: the run is complete, as the build
//     would find it.
//   - The front needs only the copied block, which putCopied pushes even
//     when a stored parent already names it: Land lets a parent land before
//     its child and pushes nothing while Settled, so without that entry no
//     seed or walk would reach the block before the flip unmarked it under
//     its parent. Every other block a list names is marked or has no list,
//     and one with no list is a block the Volume lacks, which needs no walk
//     until a put stores it: from content it is born marked, and copied it
//     enters the run and the front as this one does. The collector seeds the
//     front again from the roots and downs, as Collector.restartSettle
//     describes.
//   - A child the copied list lacks may have been freed while Settled: that
//     is the reopen's one difference from the first Open, which came from an
//     adoption before any free. The walk decodes the copied block and reads
//     such a child back through the full read chain, and while it reads
//     absent and held it stays on the front and holds the flip, so no list
//     it would give is cut short again. With no peer holding the child the
//     Volume stays Open and every free parks, until a put stores it.
//   - Every free from the commit on parks, and the free queue it reads is
//     the one Settled left: a queued block parks, enters the run, and waits
//     for the flip.
//   - The adoption, its grace clock (LivenessState.AdoptAtUnixMilli), the
//     logs and their cursors, a paged release in progress, the recorded
//     holes and a pending recount (kindRewritten) are not the settle's and
//     stay: the flip waits on the grace, the release, every hole and the
//     recount as it does for a Volume Open all along.
//
// Cost: one write beside the state, once per reopen.
func (r *Records) reopen(ctx context.Context, state *LivenessState) error {
	state.Generation = Generation_GENERATION_OPEN
	r.genKnown, r.gen, r.moved = true, Generation_GENERATION_OPEN, true
	return r.tx.Set(ctx, r.key(kindUnmarked, runComplete), nil)
}

// keepParked finishes a copied put of the parked block at key, after
// Records.repark deleted its count, parked key, queue entry, front entry and
// run entry, keeping its stored list: an unverified one goes back into the
// run and onto the front, so the walk decodes it, and a marked one needs
// neither. The block's count record is then absent, its implied holder,
// which the put's holders or its down settle, as Land describes.
func (r *Records) keepParked(ctx context.Context, key []byte) error {
	listed, marked, err := r.listState(ctx, key)
	if err != nil || !listed || marked {
		return err
	}
	if err := r.tx.Set(ctx, r.key(kindUnmarked, key), nil); err != nil {
		return err
	}
	return r.addFront(ctx, key)
}

// mergeList writes the union of the list value of the block at key and
// refs, its content's refs, marked, and returns the union's block keys and
// whether the list gained a child. With strict, refs that lack a child the
// list references write nothing and report dropped: a stored list can only miss
// children, so such refs come from a wrong decoder. A caller is strict only
// where the refused list stays unverified, since a marked list left lacking
// holds nothing back from the sweep. A list that gains a child logs no
// holder for it, so the caller logs the holder or records that the counts
// owe a recount.
func (r *Records) mergeList(ctx context.Context, key, value []byte, refs []*block.BlockRef, strict bool) (packed []byte, gained, dropped bool, err error) {
	// Refuse refs that drop a child, then write the marked union.
	old, encoded := listKeys(value), EncodeChildren(refs)
	if strict && !containsChildren(encoded, old) {
		return nil, false, true, nil
	}
	union, err := unionChildren(old, encoded)
	if err != nil {
		return nil, false, false, err
	}
	gained = !bytes.Equal(union, old)
	return union, gained, false, r.tx.Set(ctx, r.key(kindChildren, key), markList(union))
}

// noteRewrite records, on a Volume that keeps the logs, that a list gained
// a child no holder counts, so the flip returns the Volume to unadopted and
// an adoption recounts the lists first.
func (r *Records) noteRewrite(ctx context.Context, state *LivenessState) error {
	if state.GetAdoption() == Adoption_ADOPTION_UNADOPTED {
		return nil
	}
	return r.tx.Set(ctx, r.key(kindRewritten), nil)
}

// recountOwed reports whether the counts owe the recount noteRewrite
// records. While they do a count may miss a holder, so no step that trusts
// a zero count runs: the free, the pop and the flip. Nothing writes the
// record while Sweeping: the flip refuses to run while it exists, and each
// writer runs only while Open, so a free while Sweeping trusts its counts.
// It reads one key.
func (r *Records) recountOwed(ctx context.Context) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindRewritten))
}

// rebirth gives birth again to the unmarked block ref while the generation
// is Sweeping: it leaves the run as releaseOld describes, keeping the
// holders it gained since the flip, its run entry turns reborn, and it
// returns the block keys of the children its old list still held, which
// lose that holder. The caller writes the new list.
func (r *Records) rebirth(ctx context.Context, ref *block.BlockRef) ([][]byte, error) {
	key := BlockKey(ref)
	released, err := r.releaseOld(ctx, key)
	if err != nil {
		return nil, err
	}
	return released, r.tx.Set(ctx, r.key(kindUnmarked, key), runReborn)
}

// outsideRun returns the block keys in packed that have no run entry: the
// blocks outside the run at the flip, since no entry is deleted while
// Sweeping. A child the sweep deleted and a put stored again stays inside,
// since its new count never had the old list's holder.
func (r *Records) outsideRun(ctx context.Context, packed []byte) ([][]byte, error) {
	var out [][]byte
	for len(packed) != 0 {
		key, ok := nextBlockKey(packed)
		if !ok {
			return nil, errBadBlockKey
		}
		packed = packed[len(key):]
		inRun, err := r.tx.Exists(ctx, r.key(kindUnmarked, key))
		if err != nil {
			return nil, err
		}
		if !inRun {
			out = append(out, key)
		}
	}
	return out, nil
}

// settle runs the settle's steps of one collector pass while frees are on,
// each in slices: while Open, it builds the run, takes the free-proven
// entries off the front, and flips when the flip's checks pass. While
// Sweeping, it sweeps. It reports whether it stopped at past with work left.
func (c *Collector) settle(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Read the generation.
	var state *LivenessState
	err := c.store.View(ctx, func(rec *Records) error {
		var err error
		state, err = rec.GetState(ctx)
		return err
	})
	if err != nil {
		return false, err
	}

	// Forget the settle's positions once the generation leaves Sweeping,
	// whichever way it left, as restartSettle describes. Reading the
	// generation, rather than the drop's result, also covers a drop whose
	// commit was durable but reported an error.
	gen := state.GetGeneration()
	if c.lastGen == Generation_GENERATION_SWEEPING && gen != Generation_GENERATION_SWEEPING {
		c.restartSettle()
	}
	c.lastGen = gen
	if gen == Generation_GENERATION_SETTLED {
		return false, nil
	}

	// Sweep a flipped Volume, resuming where the last pass stopped: a sweep
	// that reaches the run's end has deleted every unmarked block, since
	// none is added or turns unmarked while Sweeping, as sweepFrom
	// describes. Then the run goes and the state settles.
	if gen == Generation_GENERATION_SWEEPING {
		if !c.sweptRun {
			var reached bool
			more, err := viewSlices(ctx, c, &c.sweepFrom, past, &c.page, func(rec *Records, page *keyPage, from []byte) error {
				return rec.readKeys(ctx, page, kindUnmarked, from, c.share(sliceFrees), nil)
			}, func(rec *Records, blocks block.StoreOps, from []byte, page *keyPage) ([]byte, bool, error) {
				next, more, end, err := c.sweepSlice(ctx, rec, blocks, res, from, page)
				reached = end
				return next, more, err
			})
			if err != nil || more || !reached {
				return more, err
			}
			c.sweptRun = true
		}
		return viewSlices(ctx, c, &c.dropFrom, past, &c.page, func(rec *Records, page *keyPage, from []byte) error {
			return rec.readKeys(ctx, page, kindUnmarked, from, c.share(sliceRecords), nil)
		}, func(rec *Records, _ block.StoreOps, from []byte, page *keyPage) ([]byte, bool, error) {
			return c.dropRunSlice(ctx, rec, res, from, page)
		})
	}

	// Build the run and seed the front, each resuming where the last pass
	// stopped.
	more, err := viewSlices(ctx, c, &c.runFrom, past, &c.page, func(rec *Records, page *keyPage, from []byte) error {
		return rec.readKeys(ctx, page, kindChildren, from, c.share(sliceRecords), func(value func() ([]byte, error)) (bool, error) {
			v, err := value()
			return err == nil && !listMarked(v), err
		})
	}, func(rec *Records, _ block.StoreOps, from []byte, page *keyPage) ([]byte, bool, error) {
		return c.runSlice(ctx, rec, res, from, page)
	})
	if err != nil || more {
		return more, err
	}
	if more, err := c.seed(ctx, past, res); err != nil || more {
		return more, err
	}

	// Take the free-proven entries off the front, resuming where the last
	// pass stopped, while an event since the last whole scan could let a pop
	// succeed, as frontIdle describes. Then try the flip, which reads the
	// whole front itself.
	if more, err := c.scanFront(ctx, state, past, res); err != nil || more {
		return more, err
	}

	// Settle the unborn entries the scan kept at one holder, which no count
	// proves unreferenced, as unrefScan describes.
	if more, err := c.settleUnreferenced(ctx, state, past, res); err != nil || more {
		return more, err
	}
	proof, err := c.readFlipProof(ctx)
	if err != nil {
		return false, err
	}
	return false, c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		return c.flip(ctx, rec, res, proof)
	})
}

// flipProof is what a read snapshot found of the flip's conditions, which
// the flip's transaction confirms with point reads, as flip describes.
//
// Invariant: between the snapshot and the flip's transaction the free
// queue gains no entry while the adoption mark stands, and the front and
// the holes gain no key while the add revision stands, and no hole loses
// its write-off but by leaving. Only the collector's
// transactions and the adoption write queue entries, as Collector.queueFrom
// describes; the collector runs one step at a time, and the settle runs
// nothing between the snapshot and the flip; an adoption stamps a new
// AdoptAtUnixMilli, which the flip compares. A Land only deletes queue
// entries. Every front and hole key is written through Records.addFront,
// Records.addHole or Records.writeOffHole, which raise the add revision in
// the same transaction.
// FlipProof.tla models the writers, the pop bound and the point checks
// that confirm the proof.
type flipProof struct {
	// read reports that the snapshot found the flip's conditions holding
	// but the queue's: Open, freeable, no recount owed, the run complete,
	// no front key, and the holes-clear mark or every hole written off.
	// Without it the flip does not run in this pass.
	read bool
	// rev is the add revision the snapshot read.
	rev uint64
	// adoptAt is the adoption mark the snapshot read.
	adoptAt int64
	// popped reports that the snapshot read a pop mark.
	popped bool
	// upTo is the bound the snapshot read for a ripe queued block: the
	// lower of the pop mark and the up cursor, when popped.
	upTo uint64
	// ripe reports that the snapshot's free queue held a block ripe at or
	// below upTo. Deleting a queue entry never makes the queue riper, so
	// the flip relies on it while upTo and the adoption mark stand, as the
	// adoption is the one writer outside the collector that queues a block.
	ripe bool
}

// readFlipProof reads the flip's conditions in a read snapshot, which holds
// no writer, and returns them as a proof the flip's transaction confirms.
// It seeks the front's prefix, stepping over its deleted keys, and, only
// when a pop mark asks for the queue's check, reads the free queue's first
// entry, stepping over the entries deleted before it, to decide whether a
// block is ripe at or below the mark.
//
// Cost: per pass that reaches the flip, the state, the recount mark, the
// run's completion entry, the holes-clear mark, the add revision and the
// pop mark, then one seek of the front and one of the queue, all in a
// snapshot. While a hole is recorded and the front is empty, it also reads
// the hole keys up to the first one nobody wrote off.
func (c *Collector) readFlipProof(ctx context.Context) (flipProof, error) {
	var proof flipProof
	err := c.store.View(ctx, func(rec *Records) error {
		var err error
		proof, err = c.proveFlip(ctx, rec)
		return err
	})
	return proof, err
}

// proveFlip reads the flip's proof from rec, as readFlipProof describes.
func (c *Collector) proveFlip(ctx context.Context, rec *Records) (flipProof, error) {
	// Require every condition of the flip but the queue's.
	var proof flipProof
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN || !c.freeable(state) {
		return proof, err
	}
	for _, check := range []func(context.Context) (bool, error){rec.recountOwed, rec.runIncomplete} {
		if fails, err := check(ctx); err != nil || fails {
			return proof, err
		}
	}
	if fronted, err := rec.anyKey(ctx, rec.key(kindFront)); err != nil || fronted {
		return proof, err
	}
	if holds, err := rec.holesHold(ctx); err != nil || holds {
		return proof, err
	}
	if proof.rev, err = rec.addRev(ctx); err != nil {
		return proof, err
	}
	proof.read, proof.adoptAt = true, state.GetAdoptAtUnixMilli()

	// Read the queue's head when a pop mark asks for its check.
	mark, popped, err := rec.popMark(ctx)
	if err != nil || !popped {
		return proof, err
	}
	proof.popped, proof.upTo = true, min(mark, state.GetUpCursor())
	proof.ripe, _, err = rec.ripeQueued(ctx, nil, proof.upTo)
	return proof, err
}

// clearHoles writes the holes-clear mark during each collector pass that
// finds it absent and finds no hole key in a read snapshot, even with frees
// off or before adoption. Its write requires the snapshot's add revision
// unchanged, as kindHolesClear describes. A flip without the mark waits for
// the next pass to establish it.
//
// Cost: per pass, one point read; while the mark is absent, one seek of the
// hole prefix in a snapshot, and, when it finds none, one transaction of a
// point read and a write.
func (c *Collector) clearHoles(ctx context.Context) error {
	// Read the mark, and without it the hole prefix and the revision.
	var write bool
	var rev uint64
	err := c.store.View(ctx, func(rec *Records) error {
		// Find the mark gone, no hole, and the revision to match.
		open, err := rec.holesOpen(ctx)
		if err != nil || !open {
			return err
		}
		holes, err := rec.anyKey(ctx, rec.key(kindHole))
		if err != nil || holes {
			return err
		}
		rev, err = rec.addRev(ctx)
		write = err == nil
		return err
	})
	if err != nil || !write {
		return err
	}

	// Write the mark unless a hole may have been recorded since.
	return c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		now, err := rec.addRev(ctx)
		if err != nil || now != rev {
			return err
		}
		return rec.tx.Set(ctx, rec.key(kindHolesClear), nil)
	})
}

// scanFront scans the front in slices from where the last pass stopped,
// unless the last whole scan left it idle and nothing since could let a pop
// succeed, as frontIdle describes; state is the state the pass read. It
// reports whether it stopped at past with work left.
//
// Cost: per pass with the front idle, nothing; per event, one scan of the
// whole front in read snapshots, each slice then holding the writer for at
// most sliceRecords entries and up to six point reads per entry, whatever
// the deleted front keys. An entry at one holder costs up to five. While a
// damaged entry stands, every pass that reaches the settle scans, as before
// the idle.
func (c *Collector) scanFront(ctx context.Context, state *LivenessState, past func() bool, res *CollectResult) (bool, error) {
	// Skip an idle front no event woke.
	cursor, adoptAt := state.GetUpCursor(), state.GetAdoptAtUnixMilli()
	woke := c.frontWoke || c.unbornPush.Load() || (c.frontWake != 0 && cursor >= c.frontWake)
	if c.frontIdle && !woke && adoptAt == c.frontAdoptAt {
		return false, nil
	}

	// Start a scan at the head, from which each event counts again. A new
	// adoption counted every holder, so the reference scan's findings lapse.
	if c.frontIdle || c.front.key == nil {
		if adoptAt != c.frontAdoptAt {
			c.unref.forget()
		}
		c.frontIdle, c.frontWake, c.frontWoke, c.frontAdoptAt = false, 0, false, adoptAt
		c.unbornPush.Store(false)
		c.front = frontPos{}
	}
	more, err := viewSlices(ctx, c, &c.front, past, &c.page, func(rec *Records, page *keyPage, from frontPos) error {
		return rec.readKeys(ctx, page, kindFront, from.key, c.share(sliceRecords), nil)
	}, func(rec *Records, blocks block.StoreOps, from frontPos, page *keyPage) (frontPos, bool, error) {
		return c.frontSlice(ctx, rec, blocks, res, from, page)
	})
	if err != nil || more || !c.front.done {
		return more, err
	}

	// At the front's end, go idle until the least ripe sequence a pop
	// refused for the cursor, or an event, unless a damaged entry may pop
	// with no event.
	res.Front, res.FrontDone = c.front.kept, true
	c.frontIdle, c.frontWake = !c.front.damaged, minWake(c.frontWake, c.front.wake)
	c.front = frontPos{}
	return false, nil
}

// wakeFront notes that the netting queued a front block at ripe, which a
// front scan pops once the up cursor reaches ripe. The netting calls it in
// its transaction, which may not commit; a wake that did not commit costs
// one scan.
func (c *Collector) wakeFront(ripe uint64) {
	c.frontWake = minWake(c.frontWake, ripe)
}

// minWake returns the lesser of two wake sequences, where 0 is none.
func minWake(a, b uint64) uint64 {
	if a == 0 || (b != 0 && b < a) {
		return b
	}
	return a
}

// seed pushes onto the front, once per collector while Open, each block the
// walk owes a decode that a named root or a down page not netted yet
// references, which a reader or writer may still hold: the named roots in
// slices, then the down pages in slices, resuming where the last pass
// stopped. It reports whether it stopped at past with work left.
//
// Invariant: once the seeding is done, every owed block such a seed would
// reference is on the front or was taken off it by the walk or the free proof.
// A root a Land moves onto a block after its slice passed the root pushes
// the block, as holdStored does while Open. A down the Land writes references
// the block the root held, which a slice passed or a Land pushed. A page
// the netting deletes leaves the seeds. So the flip needs no seeding
// of its own, only the empty front.
//
// Cost: one sequential read of the named roots and of the down pages left,
// and one list read per root and per down, once per collector. Each slice
// holds the writer for at most sliceRecords roots or pages.
func (c *Collector) seed(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Seed the named roots first.
	if c.seeded {
		return false, nil
	}
	if !c.seededRoots {
		more, err := viewSlices(ctx, c, &c.seedRoot, past, &c.page, func(rec *Records, page *keyPage, from []byte) error {
			return rec.readKeys(ctx, page, kindRoot, from, c.share(sliceRecords), nil)
		}, func(rec *Records, _ block.StoreOps, from []byte, page *keyPage) ([]byte, bool, error) {
			return c.seedRootSlice(ctx, rec, res, from, page)
		})
		if err != nil || more {
			return more, err
		}
		c.seededRoots = true
	}

	// Then seed the down pages not netted yet.
	more, err := resumeSlices(ctx, c.update, &c.seedDown, past, func(rec *Records, _ block.StoreOps, from uint64) (uint64, bool, error) {
		return c.seedDownSlice(ctx, rec, res, from)
	})
	c.seeded = err == nil && !more
	return more, err
}

// resumeSlices runs slice as eachSlice does, from the position at pos, and
// on success stores at pos the position the last committed slice returned,
// so a later call resumes there. It reports whether it stopped at past with
// work left.
func resumeSlices[P any](
	ctx context.Context,
	update func(context.Context, func(*Records, block.StoreOps) error) error,
	pos *P,
	past func() bool,
	slice func(rec *Records, blocks block.StoreOps, from P) (P, bool, error),
) (bool, error) {
	reached := *pos
	more, err := eachSlice(ctx, update, *pos, past, func(rec *Records, blocks block.StoreOps, from P) (P, bool, error) {
		next, more, err := slice(rec, blocks, from)
		reached = next
		return next, more, err
	})
	if err == nil {
		*pos = reached
	}
	return more, err
}

// keyPage is a slice's keys as a read snapshot found them, packed end to
// end so a slice allocates nothing per key: the keys the slice decides, the
// last key the read passed, and whether more follow it.
type keyPage struct {
	packed []byte
	ends   []int
	last   []byte
	full   bool
}

// each calls fn with each key of p, which aliases p's buffer.
func (p *keyPage) each(fn func(key []byte) error) error {
	start := 0
	for _, end := range p.ends {
		if err := fn(p.packed[start:end:end]); err != nil {
			return err
		}
		start = end
	}
	return nil
}

// resume returns the position a slice that decided p resumes from: a copy
// of the last key p's read passed, or from when it passed none.
func (p *keyPage) resume(from []byte) []byte {
	if len(p.last) == 0 {
		return from
	}
	return bytes.Clone(p.last)
}

// readKeys reads into p up to n records of kind after the key position
// from, nil for the kind's start, passing over the empty key of a kind's
// completion entry: the key of each whose value keep reports, or of every
// one when keep is nil, the last key read, and whether more follow. It
// reuses p's buffers. It steps over every deleted key before the first it
// reads, so a collector calls it only in a read snapshot, as viewSlices
// does.
func (r *Records) readKeys(ctx context.Context, p *keyPage, kind byte, from []byte, n int, keep func(value func() ([]byte, error)) (bool, error)) error {
	// Seek past from.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	p.packed, p.ends, p.last, p.full = p.packed[:0], p.ends[:0], p.last[:0], false

	// Read until the page fills.
	read := 0
	return r.scan(ctx, r.key(kind), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Skip the completion key, and stop once the page holds n.
		if len(key) == 0 {
			return true, nil
		}
		if read == n {
			p.full = true
			return false, nil
		}
		read++
		p.last = append(p.last[:0], key...)
		if keep != nil {
			if kept, err := keep(value); err != nil || !kept {
				return err == nil, err
			}
		}
		p.packed = append(p.packed, key...)
		p.ends = append(p.ends, len(p.packed))
		return true, nil
	})
}

// viewSlices runs slice as resumeSlices does, each slice in two steps: read
// fills page in a read snapshot, which holds no writer, from the position
// the slice starts at, and slice then decides each entry of the page in a
// write transaction from point reads. A slice so holds the
// writer for its keys' point reads however many deleted keys lie before
// them, as after a restart, which starts every position at its kind's
// start, or once earlier slices deleted the keys they read. A key a writer
// adds between the snapshot and the transaction waits for a later scan, as
// one added behind the position does; one it deletes the slice's point read
// finds gone.
//
// Cost: per slice, one snapshot read of up to the page's keys, stepping
// over the deleted keys before them, and the write transaction's point
// reads.
func viewSlices[P, S any](
	ctx context.Context,
	c *Collector,
	pos *P,
	past func() bool,
	page *S,
	read func(rec *Records, page *S, from P) error,
	slice func(rec *Records, blocks block.StoreOps, from P, page *S) (P, bool, error),
) (bool, error) {
	at := *pos
	update := func(ctx context.Context, fn func(*Records, block.StoreOps) error) error {
		if err := c.store.View(ctx, func(rec *Records) error { return read(rec, page, at) }); err != nil {
			return err
		}
		return c.update(ctx, fn)
	}
	return resumeSlices(ctx, update, pos, past, func(rec *Records, blocks block.StoreOps, from P) (P, bool, error) {
		next, more, err := slice(rec, blocks, from, page)
		at = next
		return next, more, err
	})
}

// seedRootSlice pushes onto the front the block of each named root page
// names, read after the root key from in a read snapshot, when the walk
// owes it a decode, while the generation is Open, and returns the last root
// key the page read and whether more may follow. It reads each root's entry
// in its transaction and skips a root deleted since the snapshot; a root a
// Land moved since pushes its new block itself, as seed describes.
func (c *Collector) seedRootSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte, page *keyPage) ([]byte, bool, error) {
	// Seed only while Open.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}

	// Push each root's block when the walk owes it a decode.
	err = page.each(func(key []byte) error {
		// Skip a root deleted since the snapshot.
		entry, err := rec.rootAt(ctx, key)
		if err != nil || entry == nil {
			return err
		}
		pushed, err := rec.pushOwed(ctx, entry.GetRef())
		if pushed {
			res.Seeded++
		}
		return err
	})
	if err != nil {
		return from, false, err
	}
	return page.resume(from), page.full, nil
}

// seedDownSlice pushes onto the front each block the walk owes a decode
// referenced by up to a slice of down pages, from the down sequence from,
// while the generation is Open, and returns the sequence after the last
// page it read and whether more may follow. Netting deletes each page it
// nets, so every page left references a block a root or list let go of within
// Grace, which a reader may still pin.
func (c *Collector) seedDownSlice(ctx context.Context, rec *Records, res *CollectResult, from uint64) (uint64, bool, error) {
	// Read only while Open, from no earlier than the down cursor, before
	// which every page is netted and deleted.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return from, false, err
	}
	from = max(from, state.GetDownCursor())

	// Push the owed blocks each page references.
	budget, n := c.share(sliceRecords), 0
	err = rec.scanDowns(ctx, from, func(p downPage) (bool, error) {
		if n == budget {
			return false, nil
		}
		n++
		from = p.seq + 1
		return true, eachKey(p.keys, func(key []byte) error {
			pushed, err := rec.pushOwed(ctx, key)
			if pushed {
				res.Seeded++
			}
			return err
		})
	})
	return from, n == budget, err
}

// runSlice adds a run entry for each list page names that its transaction
// still reads unverified, the unmarked lists a read snapshot found among a
// slice of child list records after the key position from, while the
// generation is Open, and writes the run's completion entry once page
// reached the last. A list marked or freed since the snapshot needs no
// entry; one stored unmarked since then wrote its own, as follows.
//
// Invariant: every write while Open that stores an unmarked list enters the
// block in the run in the same transaction, so a list unmarked at the flip
// is in the run whether or not the scan passed it, and the run at the flip is
// every list still unmarked, whatever writes interleave with the scan.
// PutChildren, the union rewrite and ListRebuild mark; the one writer of an
// unmarked list, Records.putCopied for a Land's copied put, writes the run
// entry with it, and Records.keepParked writes it back for a parked block
// whose kept list is unmarked. A writer that stores an unmarked list while
// Open without its run entry breaks the run.
//
// Cost: one sequential snapshot read of the list records, values included,
// and per unverified list one point read and one write, once per Volume.
func (c *Collector) runSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte, page *keyPage) ([]byte, bool, error) {
	// Build only while Open, and once.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}
	incomplete, err := rec.runIncomplete(ctx)
	if err != nil || !incomplete {
		return from, false, err
	}

	// Enter each list still unmarked in the run, and complete the run once
	// the page reached the last.
	err = page.each(func(key []byte) error {
		listed, marked, err := rec.listState(ctx, key)
		if err != nil || !listed || marked {
			return err
		}
		res.Unmarked++
		return rec.tx.Set(ctx, rec.key(kindUnmarked, key), nil)
	})
	if err != nil {
		return from, false, err
	}
	from = page.resume(from)
	if page.full {
		return from, true, nil
	}
	return from, false, rec.tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil)
}

// frontPos is the front scan's position: the key after which it resumes,
// nil at the head, the entries it kept since it left the head, the least
// ripe sequence of a block at zero it kept because the up cursor had not
// reached it, 0 for none, whether it kept a damaged entry, and whether it
// reached the front's end. It moves only with a committed slice, so the
// count matches the entries the cursor passed.
type frontPos struct {
	key     []byte
	kept    uint64
	wake    uint64
	damaged bool
	done    bool
}

// frontSlice takes off the front each entry of page, the entries a read
// snapshot found after the position pos, that is still on the front and
// whose block the free proof holds for, and returns the
// position the next slice resumes at, done once it read the front's end, and
// whether more may follow. It pops only while the
// generation is Open, and only a block at zero and queued or parked at a
// ripe sequence the up cursor passed, as the free takes it. Nothing holds such a block or can
// within the grace period: a Land that names it dequeues it until its up
// nets, so a queue entry proves no Land holds it, and the free parks only a
// queued block, after which every Land reads it absent, as Records.Hidden
// describes. Its
// count misses only a holder whose list is unverified and short; that
// holder, if anything holds it, is a seed the flip pushes or a block below a
// front entry, which holds the flip until the walk decodes it. A list that
// gained a child logged no holder for it, so while kindRewritten exists the
// counts may miss a holder whose list is marked, and nothing pops: the
// recount returns to the front only a child with an unmarked list, as
// adoption.hold describes, so an absent gained child popped then would never
// return, and the sweep would delete the stored blocks only it names. The
// record lasts until the next flip attempt returns the Volume to unadopted
// and the adoption that follows deletes it, so the front waits for one
// recount at most. A block popped before the rewrite that the rewritten list
// names gains a holder in that recount, which returns it to the front, as
// adoption.hold describes. A slice that pops raises the pop mark to the latest
// ripe sequence it popped, as kindPopMark describes, at the cost of one key
// read and write per such slice. A list lasts from the block's put to its
// free, and the free parks while Open, so an entry the store does not list
// is damage that lost the list. While the bytes remain it stays, for the
// walk decodes them and writes the list again; once they are gone too
// nothing can decode the block, which names no child the walk could still
// find, and it pops at zero with a ripe sequence the cursor passed, without
// its queue entry, which the free deletes as stale once the list is gone, so
// damage cannot keep the generation Open.
func (c *Collector) frontSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, pos frontPos, page *keyPage) (frontPos, bool, error) {
	// Pop only while Open, and while the free proof can hold.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return pos, false, err
	}
	if !c.freeable(state) {
		return pos, false, nil
	}
	if recount, err := rec.recountOwed(ctx); err != nil || recount {
		return pos, false, err
	}

	// Pop each entry still on the front that the proof holds for.
	res.FrontScanned += uint64(len(page.ends))
	cursor := state.GetUpCursor()
	var poppedRipe uint64
	popped, kept, wake, damaged := false, pos.kept, pos.wake, pos.damaged
	err = page.each(func(key []byte) error {
		// Skip a key popped since the snapshot.
		fronted, err := rec.hasFront(ctx, key)
		if err != nil || !fronted {
			return err
		}

		// Keep one the proof does not pop, noting when it ripens.
		proof, ripe, err := c.frontProven(ctx, rec, blocks, cursor, key)
		if err != nil {
			return err
		}
		if proof != frontPop {
			kept++
			switch proof {
			case frontRipening:
				wake = minWake(wake, ripe)
			case frontDamaged:
				damaged = true
			case frontUnborn:
				if !c.opts.KeepUnreferenced {
					c.unref.offer(key)
				}
			}
			return nil
		}

		// Pop the rest.
		if err := rec.tx.Delete(ctx, rec.key(kindFront, key)); err != nil {
			return err
		}
		popped, poppedRipe = true, max(poppedRipe, ripe)
		res.Popped++
		return nil
	})
	if err != nil {
		return pos, false, err
	}

	// Resume after the last key read, or at the head once the read reached
	// the front's end. Then raise the pop mark past what this slice popped.
	next := frontPos{kept: kept, wake: wake, damaged: damaged, done: !page.full}
	if page.full {
		next.key = bytes.Clone(page.last)
	}
	if !popped {
		return next, page.full, nil
	}
	return next, page.full, rec.raisePopMark(ctx, poppedRipe)
}

// frontProof is a front entry's free proof as frontProven reads it. Only
// an event the front's idle watches turns a held entry poppable, as
// Collector.frontIdle describes; a ripening one turns poppable once the up
// cursor reaches its ripe sequence, and a damaged one may turn poppable with
// no event at all.
type frontProof uint8

const (
	// frontHeld is an entry the proof refuses until an event.
	frontHeld frontProof = iota
	// frontPop is an entry the proof holds for.
	frontPop
	// frontRipening is a block at zero whose ripe sequence the up cursor
	// has not reached.
	frontRipening
	// frontDamaged is a block at zero, ripe, that the store does not list
	// while its bytes remain: it pops once the bytes go too, which no
	// record write announces.
	frontDamaged
	// frontUnborn is a block at one holder the store holds neither bytes,
	// a list nor a hole record for. The proof at zero never pops it, so the
	// reference scan takes it, as unrefScan describes.
	frontUnborn
)

// frontProven reads the free proof for the front entry of the block at key,
// as frontSlice describes, and returns it with the ripe sequence of a block
// at zero. It reads the count first, since most entries a scan meets are
// held.
func (c *Collector) frontProven(ctx context.Context, rec *Records, blocks block.StoreOps, cursor uint64, key []byte) (frontProof, uint64, error) {
	// Preserve the collector's four proof states and every error's ripe sequence.
	cause, ripe, err := readFrontCause(ctx, rec, blocks, cursor, key)
	switch cause {
	case FrontPoppable:
		return frontPop, ripe, err
	case FrontRipening:
		return frontRipening, ripe, err
	case FrontDamaged:
		return frontDamaged, ripe, err
	case FrontUnborn:
		return frontUnborn, ripe, err
	default:
		return frontHeld, ripe, err
	}
}

// flip moves an Open Volume to Sweeping when the flip's checks pass, as the
// settle describes, or, when a list rewrite awaits a recount, returns the
// Volume to unadopted. The settle calls it only once seed is done, so the
// empty front covers every seed.
//
// proof is what a read snapshot found of the flip's conditions, as
// readFlipProof reads them. The flip seeks no key under the writer: it
// confirms the snapshot's empty front, and that no hole but a written-off
// one is recorded, from one point read of the add revision, which every
// new front and hole key raises, and the snapshot's
// finding of the queue from the adoption mark and the pop mark, as
// flipProof describes. A pass whose proof the point reads do not confirm
// does not flip, and the next pass reads a fresh one. A ripe entry at the
// head means the free's position missed it, so the flip forgets the
// position and the next free scans from the queue's head.
//
// Cost: in one transaction, once per collector pass until the flip, point
// reads of the state, the recount mark, the run's completion entry, the
// add revision and the pop mark. None of it grows with the deleted keys of
// any kind.
func (c *Collector) flip(ctx context.Context, rec *Records, res *CollectResult, proof flipProof) error {
	// Require an Open Volume the free may run in: adopted Grace ago, so a
	// head a reader saw before the mark, which the walk does not seed, is
	// unmarked only once it is Grace old, and with no paged release left.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return err
	}
	if !c.freeable(state) {
		return nil
	}

	// A rewritten list's new children lack their holders: recount first.
	// The settle returns to unadopted only here, and only while Open. A
	// readopt while Sweeping, such as frees turning on or a format upgrade,
	// clears both count records of each unmarked block and counts every
	// stored list again: SetHolders writes an unmarked block's count
	// to kindUnmarkedCount, and its old list holds the children the recount
	// gave it.
	recount, err := rec.recountOwed(ctx)
	if err != nil {
		return err
	}
	if recount {
		rec.unadopt(state)
		res.Unadopted = true
		return rec.SetState(ctx, state)
	}

	// Require the run complete, and the front and the holes as the
	// snapshot found them: no front key, and no hole but a written-off one.
	if !proof.read || proof.adoptAt != state.GetAdoptAtUnixMilli() {
		return nil
	}
	if incomplete, err := rec.runIncomplete(ctx); err != nil || incomplete {
		return err
	}
	if rev, err := rec.addRev(ctx); err != nil || rev != proof.rev {
		return err
	}

	// Require every Dead block at or below the pop mark parked, and so in
	// the run. A marked block that references a child the front popped is
	// Dead at a ripe sequence no later than the child's. Left present, a
	// hold onto it would revive its list over children the sweep deletes. A
	// queued block above the mark references no popped child, so a block
	// that dies later cannot hold the flip.
	// The snapshot decided it from the queue's head; the flip confirms the
	// bound it used still stands, so it seeks no queue key.
	mark, popped, err := rec.popMark(ctx)
	if err != nil {
		return err
	}
	if popped != proof.popped || popped && min(mark, state.GetUpCursor()) != proof.upTo {
		return nil
	}
	if proof.ripe {
		c.forgetQueue()
		return nil
	}

	// Flip.
	state.Generation = Generation_GENERATION_SWEEPING
	res.Flipped = true
	if err := rec.tx.Delete(ctx, rec.key(kindPopMark)); err != nil {
		return err
	}
	return rec.SetState(ctx, state)
}

// sweepSlice deletes each unmarked block of the run entries page names, read
// after the key position from in a read snapshot, while the generation is
// Sweeping and the Volume adopted: its bytes, list and proof, with one down for
// each child its old list still holds, as releaseOld decides, and marks its run
// entry swept. Its count keeps the holders it gained since the flip, as a free
// keeps a block's netted holders. It skips a reborn or swept entry. A child
// inside the run at the flip keeps no holder from the old list, so the sweep of
// an unmarked parent releases nothing to it, even after a put stored it again.
// It reports whether it reached the run's end.
//
// Cost: per entry, one point read of the entry; per unmarked block, its two
// count reads, its list read, the deletes, and one run entry read per
// child; per slice, one down page and the state write.
func (c *Collector) sweepSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte, page *keyPage) ([]byte, bool, bool, error) {
	// Sweep only an adopted Volume, since the downs need its logs.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING {
		return from, false, false, err
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return from, false, false, nil
	}

	// Delete each entry's block that is still unmarked, and log the release
	// of its children.
	var down [][]byte
	err = page.each(func(key []byte) error {
		// Skip a key marked or swept since the snapshot.
		unmarked, err := rec.inRun(ctx, key)
		if err != nil || !unmarked {
			return err
		}

		// Release its children, then delete its bytes, list and proof.
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return err
		}
		children, err := rec.releaseOld(ctx, key)
		if err != nil {
			return err
		}
		down = append(down, children...)
		if err := blocks.RmBlock(ctx, ref); err != nil {
			return err
		}
		for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.DeleteProof} {
			if err := del(ctx, ref); err != nil {
				return err
			}
		}

		// Mark it swept, so a later page skips it.
		if err := rec.tx.Set(ctx, rec.key(kindUnmarked, key), runSwept); err != nil {
			return err
		}
		res.Swept++
		return nil
	})
	if err != nil {
		return from, false, false, err
	}
	from = page.resume(from)
	if len(down) != 0 {
		return from, page.full, !page.full, rec.appendDowns(ctx, state, c.opts.Now(), down)
	}
	return from, page.full, !page.full, rec.SetState(ctx, state)
}

// dropRunSlice deletes the run entries page names, read after the key
// position from in a read snapshot, once a sweep pass left no unmarked
// block, and, when page reached the run's last entry, deletes the
// completion entry and settles the generation. No entry joins the run while
// Sweeping, so a page that reached the end in the snapshot still does.
func (c *Collector) dropRunSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte, page *keyPage) ([]byte, bool, error) {
	// Drop only while Sweeping.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING {
		return from, false, err
	}

	// Delete the page's entries.
	err = page.each(func(key []byte) error {
		return rec.tx.Delete(ctx, rec.key(kindUnmarked, key))
	})
	if err != nil {
		return from, false, err
	}
	from = page.resume(from)
	if page.full {
		return from, true, nil
	}

	// Past the last, delete the completion entry and settle.
	if err := rec.tx.Delete(ctx, rec.key(kindUnmarked, runComplete)); err != nil {
		return from, false, err
	}
	state.Generation = Generation_GENERATION_SETTLED
	res.Settled = true
	return from, false, rec.SetState(ctx, state)
}

// restartSettle forgets the settle's positions once the Volume settles, so
// a Volume a copied put reopens builds, seeds and scans its front from the
// start, as at the collector's start. A position kept from the generation
// before would skip work: a seeding done then names no block a copied put
// stored since, a finished sweep would drop the next run unswept, and a
// sweep or drop cursor would pass over the head of the next run, and a
// finished requeue would leave the next generation's parks parked. The
// settle calls it when it reads the generation leave Sweeping, as lastGen
// describes, which also covers a drop whose commit was durable but
// reported an error.
func (c *Collector) restartSettle() {
	c.runFrom, c.sweepFrom, c.sweptRun, c.dropFrom = nil, nil, false, nil
	c.seedRoot, c.seedDown, c.seededRoots, c.seeded = nil, 0, false, false
	c.front, c.frontIdle, c.frontWake, c.frontWoke, c.frontAdoptAt = frontPos{}, false, 0, false, 0
	c.unref, c.requeueFrom, c.requeued = unrefScan{}, nil, false
}

// appendDowns appends down, block keys losing one holder each, to the down
// log at now, and writes state.
func (r *Records) appendDowns(ctx context.Context, state *LivenessState, now time.Time, down [][]byte) error {
	if err := r.appendLogs(ctx, state, now, nil, down); err != nil {
		return err
	}
	return r.SetState(ctx, state)
}
