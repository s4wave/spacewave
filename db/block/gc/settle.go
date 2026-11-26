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
// still names. The settle proves which old blocks are live before anything
// frees. The generation, in the state record, moves Open, Sweeping,
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
//     block it writes from content, and each such block a root moves onto;
//     the walk pushes those children of each block it decodes. So the walk
//     never descends below a verified block, and no young block needs
//     seeding.
//   - A block the Volume lacks, as a peer's cache lacks what it never read,
//     has no list, so no count holds a stored block only it names. The walk
//     reads it through the full read chain, peers included, without filling
//     the Volume, and writes its list from content without its bytes, as an
//     evicted block keeps its list; the list then holds what lies below. An
//     absent block no read resolves stays on the front while it has holders,
//     and holds the flip.
//   - Once its completion entry exists, the unmarked run names every block
//     whose list is unverified: the run build adds each, and every write that
//     marks a list deletes the block's entry in the same transaction. One
//     writer adds an unverified list: a Land of a copied put, whose refs the
//     writer did not read from content, through Records.putCopied, which
//     writes the block's run entry and front entry in the same transaction,
//     so the run stays complete and the walk decodes the block.
//
// Before the flip the collector seeds the front, in slices: it pushes every
// owed block a named root or a down page not netted yet names, as
// seed describes, the model's WalkStart. The flip is one state write, to
// Sweeping, in a transaction that finds the Volume freeable, as
// Collector.freeable describes, no list rewrite awaiting a recount, no
// recorded hole, the run complete, the front empty and every ripe Dead
// block parked: the model's WalkEnd. Every block still unverified is then
// unreachable: no seed names it, and no young or verified block does, since
// the front is empty. The run names those blocks and the parked ones.
//
// While Sweeping, an empty run entry names an unmarked block, which every
// site that asks whether a block is stored answers absent, as Unmarked
// describes. The flip, which requires no paged release in progress, freezes
// each unmarked block's count record and counts its holders from the flip on
// apart, as kindUnmarkedCount describes. A stored block holds its children
// until its free, and the free skips an unmarked block, so its old list
// still holds them. The sweep deletes each unmarked block, releasing the
// holder its old list gave each child outside the run, as releaseOld
// decides. A put
// of an unmarked block gives birth to it again: its entry turns reborn, its
// old list releases the same way, and it keeps the holders it gained since
// the flip. The sweep marks each entry it deletes swept rather than
// deleting it, so the run stays the run at the flip. Once no unmarked block
// is left, the sweep deletes the run and the state settles.

// AbsentError is returned by a Land that holds a hidden block, as
// Records.Hidden describes: one the settle unmarked, which no live block
// names and the sweep will delete, or one the free parked while Open. The
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
	value, found, err := r.tx.Get(ctx, r.key(kindUnmarked, key))
	return found && len(value) == 0, err
}

// countKind returns the kind of the count record of the block at key:
// kindUnmarkedCount while it is unmarked, since the flip froze its kindCount
// record, and kindCount otherwise. Outside Sweeping it reads nothing more
// than the generation; while Sweeping, one run entry.
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
		list, _, err := r.tx.Get(ctx, r.key(kindChildren, key))
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
	value, found, err := r.tx.Get(ctx, r.key(kindChildren, key))
	return found, found && listMarked(value), err
}

// pushOwed pushes the block at key onto the front when the walk owes it a
// decode, as its list is not from content or it has none, whether or not
// the Volume stores its bytes, and reports whether it did. An absent block
// has no list, so no count holds a stored block only it names: it stays on
// the front while it has holders, until the walk reads it through the full
// read chain and writes its list, as ListRebuild describes.
func (r *Records) pushOwed(ctx context.Context, key []byte) (bool, error) {
	_, marked, err := r.listState(ctx, key)
	if err != nil || marked {
		return false, err
	}
	return true, r.tx.Set(ctx, r.key(kindFront, key), nil)
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
	return r.tx.Set(ctx, r.key(kindFront, key), nil)
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
	return r.tx.Set(ctx, r.key(kindFront, key), nil)
}

// mergeList writes the union of the list value of the block at key and
// refs, its content's refs, marked, and returns the union's block keys and
// whether the list gained a child. With strict, refs that lack a child the
// list names write nothing and report dropped: a stored list can only miss
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
// entries off the front, and flips when the flip's checks pass; while
// Sweeping, it sweeps. It reports whether it stopped at past with work left.
func (c *Collector) settle(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Read the generation.
	var gen Generation
	err := c.store.View(ctx, func(rec *Records) error {
		var err error
		gen, err = rec.generation(ctx)
		return err
	})
	if err != nil || gen == Generation_GENERATION_SETTLED {
		return false, err
	}

	// Sweep a flipped Volume: one pass from the start of the run that
	// reaches its end deletes every unmarked block, since none is added
	// while Sweeping, and then the run goes and the state settles.
	if gen == Generation_GENERATION_SWEEPING {
		if !c.sweptRun {
			var reached bool
			more, err := eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, blocks block.StoreOps, from []byte) ([]byte, bool, error) {
				next, more, end, err := c.sweepSlice(ctx, rec, blocks, res, from)
				reached = end
				return next, more, err
			})
			if err != nil || more || !reached {
				return more, err
			}
			c.sweptRun = true
		}
		return eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
			return c.dropRunSlice(ctx, rec, res, from)
		})
	}

	// Build the run and seed the front, each resuming where the last pass
	// stopped.
	more, err := resumeSlices(ctx, c.update, &c.runFrom, past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
		return c.runSlice(ctx, rec, res, from)
	})
	if err != nil || more {
		return more, err
	}
	if more, err := c.seed(ctx, past, res); err != nil || more {
		return more, err
	}

	// Take the free-proven entries off the front, resuming where the last
	// pass stopped, then, at the front's end, try the flip, which reads the
	// whole front itself.
	more, err = resumeSlices(ctx, c.update, &c.front, past, func(rec *Records, blocks block.StoreOps, from frontPos) (frontPos, bool, error) {
		return c.frontSlice(ctx, rec, blocks, res, from)
	})
	if err != nil || more {
		return more, err
	}
	if c.front.done {
		res.Front, res.FrontDone = c.front.kept, true
		c.front = frontPos{}
	}
	return false, c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		return c.flip(ctx, rec, res)
	})
}

// seed pushes onto the front, once per collector while Open, each block the
// walk owes a decode that a named root or a down page not netted yet names,
// which a reader or writer may still hold: the named roots in slices, then
// the down pages in slices, resuming where the last pass stopped. It
// reports whether it stopped at past with work left.
//
// Invariant: once the seeding is done, every owed block such a seed would
// name is on the front or was taken off it by the walk or the free proof.
// A root a Land moves onto a block after its slice passed the root pushes
// the block, as holdStored does while Open; a down it writes names
// the block the root held, which a slice passed or a Land pushed; and a page
// the netting deletes leaves the seeds. So the flip needs no seeding
// of its own, only the empty front.
//
// Cost: one sequential read of the named roots and of the down pages left,
// and one list read per root and per down, once per collector; each slice
// holds the writer for at most sliceRecords roots or pages.
func (c *Collector) seed(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Seed the named roots first.
	if c.seeded {
		return false, nil
	}
	if !c.seededRoots {
		more, err := resumeSlices(ctx, c.update, &c.seedRoot, past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
			return c.seedRootSlice(ctx, rec, res, from)
		})
		if err != nil || more {
			return more, err
		}
		c.seededRoots = true
	}

	// Then the down pages not netted yet.
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

// seedRootSlice pushes the block of each of up to a slice of named roots
// after the root key from onto the front when the walk owes it a decode,
// while the generation is Open, and returns the last root key it read and
// whether more may follow.
func (c *Collector) seedRootSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Read a slice of roots while Open.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}
	budget := c.share(sliceRecords)
	roots, err := rec.scanAfter(ctx, kindRoot, from, budget)
	if err != nil {
		return from, false, err
	}

	// Push each root's block when the walk owes it a decode.
	for _, root := range roots {
		from = root.key
		entry := &RootEntry{}
		if err := entry.UnmarshalVT(root.value); err != nil {
			return from, false, err
		}
		pushed, err := rec.pushOwed(ctx, entry.GetRef())
		if err != nil {
			return from, false, err
		}
		if pushed {
			res.Seeded++
		}
	}
	return from, len(roots) == budget, nil
}

// seedDownSlice pushes onto the front each block the walk owes a decode
// that up to a slice of down pages names, from the down sequence from,
// while the generation is Open, and returns the sequence after the last
// page it read and whether more may follow. Netting deletes each page it nets, so every page left
// names a block a root or list let go of within Grace, which a reader may
// still pin.
func (c *Collector) seedDownSlice(ctx context.Context, rec *Records, res *CollectResult, from uint64) (uint64, bool, error) {
	// Read only while Open.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}

	// Push the owed blocks each page names.
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

// runSlice adds a run entry for each unverified list of up to a slice of
// child list records after the key position from, while the generation is
// Open, and writes the run's completion entry once it passes the last.
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
// Cost: one sequential read of the list records, values included, and one
// write per unverified list, once per Volume.
func (c *Collector) runSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Build only while Open, and once.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}
	done, err := rec.tx.Exists(ctx, rec.key(kindUnmarked, runComplete))
	if err != nil || done {
		return from, false, err
	}

	// Take the next lists within the slice's budget, noting each unmarked
	// list's key, packed end to end, and the last key read, so a slice
	// allocates nothing per list.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	budget, n, full := c.share(sliceRecords), 0, false
	var last, unverified []byte
	var ends []int
	err = rec.scan(ctx, rec.key(kindChildren), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Stop at a full slice; note each unmarked list.
		if n == budget {
			full = true
			return false, nil
		}
		n++
		last = append(last[:0], key...)
		v, err := value()
		if err == nil && !listMarked(v) {
			unverified = append(unverified, key...)
			ends = append(ends, len(unverified))
		}
		return true, err
	})
	if err != nil {
		return from, false, err
	}
	if n != 0 {
		from = last
	}

	// Enter each unmarked list in the run, and complete the run past the last.
	start := 0
	for _, end := range ends {
		if err := rec.tx.Set(ctx, rec.key(kindUnmarked, unverified[start:end]), nil); err != nil {
			return from, false, err
		}
		start = end
	}
	res.Unmarked += uint64(len(ends))
	if full {
		return from, true, nil
	}
	return from, false, rec.tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil)
}

// frontPos is the front scan's position: the key after which it resumes,
// nil at the head, the entries it kept since it left the head, and whether
// it reached the front's end. It moves only with a committed slice, so the
// count matches the entries the cursor passed.
type frontPos struct {
	key  []byte
	kept uint64
	done bool
}

// frontSlice takes off the front each of up to a slice of entries after the
// position pos, whose block the free proof holds for, and returns the
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
func (c *Collector) frontSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, pos frontPos) (frontPos, bool, error) {
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

	// Read the next entries' keys into the scratch, packed end to end, so a
	// slice allocates nothing per entry.
	var seek []byte
	if pos.key != nil {
		seek = append(bytes.Clone(pos.key), 0)
	}
	budget, full := c.share(sliceRecords), false
	packed, ends := c.frontKeys[:0], c.frontEnds[:0]
	err = rec.scan(ctx, rec.key(kindFront), seek, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		if len(ends) == budget {
			full = true
			return false, nil
		}
		packed = append(packed, key...)
		ends = append(ends, len(packed))
		return true, nil
	})
	c.frontKeys, c.frontEnds = packed, ends
	if err != nil {
		return pos, false, err
	}
	res.FrontScanned += uint64(len(ends))

	// Pop each the proof holds for.
	cursor := state.GetUpCursor()
	var poppedRipe uint64
	popped, start, last, kept := false, 0, 0, pos.kept
	for _, end := range ends {
		key := packed[start:end:end]
		last, start = start, end
		pop, ripe, err := c.frontProven(ctx, rec, blocks, cursor, key)
		if err != nil {
			return pos, false, err
		}
		if !pop {
			kept++
			continue
		}
		if err := rec.tx.Delete(ctx, rec.key(kindFront, key)); err != nil {
			return pos, false, err
		}
		popped, poppedRipe = true, max(poppedRipe, ripe)
		res.Popped++
	}

	// Resume after the last key, copied out of the scratch, or at the head
	// once the scan reached the front's end. Then raise the pop mark past
	// what this slice popped.
	next := frontPos{kept: kept, done: !full}
	if full {
		next.key = bytes.Clone(packed[last:])
	}
	if !popped {
		return next, full, nil
	}
	return next, full, rec.raisePopMark(ctx, poppedRipe)
}

// frontProven reports whether the free proof holds for the front entry of
// the block at key, as frontSlice describes, and its ripe sequence. It reads
// the count first, since most entries a scan meets are held.
func (c *Collector) frontProven(ctx context.Context, rec *Records, blocks block.StoreOps, cursor uint64, key []byte) (bool, uint64, error) {
	// Require exactly a block key, then zero holders, ripe at a sequence the
	// cursor reached.
	if bkey, ok := nextBlockKey(key); !ok || len(bkey) != len(key) {
		return false, 0, errBadBlockKey
	}
	holders, ripe, err := rec.holders(ctx, key)
	if err != nil || holders != 0 || ripe > cursor {
		return false, 0, err
	}

	// A listed block needs its queue or parked entry, and one the store
	// does not list needs its bytes gone.
	listed, err := rec.listed(ctx, key)
	if err != nil {
		return false, 0, err
	}
	if listed {
		proven, err := rec.tx.Exists(ctx, rec.queueKey(ripe, key))
		if err == nil && !proven {
			proven, err = rec.tx.Exists(ctx, rec.releasedKey(ripe, key))
		}
		if err == nil && !proven {
			proven, err = rec.tx.Exists(ctx, rec.parkedKey(ripe, key))
		}
		return proven, ripe, err
	}
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return false, 0, err
	}
	stored, err := blocks.GetBlockExists(ctx, ref)
	return !stored, ripe, err
}

// flip moves an Open Volume to Sweeping when the flip's checks pass, as the
// settle describes, or, when a list rewrite awaits a recount, returns the
// Volume to unadopted. The settle calls it only once seed is done, so the
// empty front covers every seed.
//
// Cost: point reads of the run's completion entry and the pop mark and up
// to four prefix seeks, in one transaction, once per collector pass until
// the flip.
func (c *Collector) flip(ctx context.Context, rec *Records, res *CollectResult) error {
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
	// This is the settle's one return to unadopted, and it runs only while
	// Open. A readopt while Sweeping, such as frees turning on or a format
	// upgrade, clears both count records of each unmarked block and counts
	// every stored list again: SetHolders writes an unmarked block's count
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

	// Require the run complete, the front empty and no recorded hole.
	done, err := rec.tx.Exists(ctx, rec.key(kindUnmarked, runComplete))
	if err != nil || !done {
		return err
	}
	for _, kind := range []byte{kindFront, kindHole} {
		if found, err := rec.anyKey(ctx, rec.key(kind)); err != nil || found {
			return err
		}
	}

	// Require every Dead block at or below the pop mark parked, and so in
	// the run. A marked block that names a child the front popped is Dead at
	// a ripe sequence no later than the child's; left present, a hold onto
	// it would revive its list over children the sweep deletes. A queued
	// block above the mark names no popped child, so a block that dies
	// later cannot hold the flip.
	mark, popped, err := rec.popMark(ctx)
	if err != nil {
		return err
	}
	if popped {
		ripe, err := rec.ripeQueued(ctx, min(mark, state.GetUpCursor()))
		if err != nil || ripe {
			return err
		}
	}

	// Flip.
	state.Generation = Generation_GENERATION_SWEEPING
	res.Flipped = true
	if err := rec.tx.Delete(ctx, rec.key(kindPopMark)); err != nil {
		return err
	}
	return rec.SetState(ctx, state)
}

// sweepSlice deletes each unmarked block of up to a slice of run entries
// after the key position from, while the generation is Sweeping and the
// Volume adopted: its bytes, list and proof, with one down for each child
// its old list still holds, as releaseOld decides, and marks its run entry
// swept. Its count keeps the
// holders it gained since the flip, as a free keeps a block's netted
// holders. It skips a reborn or swept entry. A child inside the run at the
// flip keeps no holder from the old list, so the sweep of an unmarked
// parent releases nothing to it, even after a put stored it again. It
// reports whether it reached the run's end.
//
// Cost: per unmarked block, its two count reads, its list read, the
// deletes, and one run entry read per child; per slice, one down page and
// the state write.
func (c *Collector) sweepSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte) ([]byte, bool, bool, error) {
	// Sweep only an adopted Volume, since the downs need its logs.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING {
		return from, false, false, err
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return from, false, false, nil
	}

	// Take the next entries.
	keys, unmarked, full, err := c.runEntries(ctx, rec, from, sliceFrees)
	if err != nil {
		return from, false, false, err
	}

	// Delete each unmarked block, and log the release of its children.
	var down [][]byte
	for i, key := range keys {
		from = key
		if !unmarked[i] {
			continue
		}
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return from, false, false, err
		}
		children, err := rec.releaseOld(ctx, key)
		if err != nil {
			return from, false, false, err
		}
		down = append(down, children...)
		if err := blocks.RmBlock(ctx, ref); err != nil {
			return from, false, false, err
		}
		for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.DeleteProof} {
			if err := del(ctx, ref); err != nil {
				return from, false, false, err
			}
		}
		if err := rec.tx.Set(ctx, rec.key(kindUnmarked, key), runSwept); err != nil {
			return from, false, false, err
		}
		res.Swept++
	}
	if len(down) != 0 {
		return from, full, !full, rec.appendDowns(ctx, state, c.opts.Now(), down)
	}
	return from, full, !full, rec.SetState(ctx, state)
}

// dropRunSlice deletes up to a slice of run entries after the key position
// from, once a sweep pass left no unmarked block, and, past the last,
// deletes the completion entry and settles the generation.
func (c *Collector) dropRunSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Drop only while Sweeping.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING {
		return from, false, err
	}

	// Delete the next entries.
	keys, _, full, err := c.runEntries(ctx, rec, from, sliceRecords)
	if err != nil {
		return from, false, err
	}
	for _, key := range keys {
		from = key
		if err := rec.tx.Delete(ctx, rec.key(kindUnmarked, key)); err != nil {
			return from, false, err
		}
	}
	if full {
		return from, true, nil
	}

	// Past the last, delete the completion entry and settle.
	if err := rec.tx.Delete(ctx, rec.key(kindUnmarked, runComplete)); err != nil {
		return from, false, err
	}
	c.restartSettle()
	state.Generation = Generation_GENERATION_SETTLED
	res.Settled = true
	return from, false, rec.SetState(ctx, state)
}

// restartSettle forgets the settle's positions once the Volume settles, so
// a Volume a copied put reopens builds, seeds and scans its front from the
// start, as at the collector's start. A position kept from the generation
// before would skip work: a seeding done then names no block a copied put
// stored since, and a finished sweep pass would drop the next run unswept.
func (c *Collector) restartSettle() {
	c.runFrom, c.sweptRun = nil, false
	c.seedRoot, c.seedDown, c.seededRoots, c.seeded = nil, 0, false, false
	c.front = frontPos{}
}

// runEntries returns the block keys of up to a slice of base run entries
// after the key position from, past the completion entry, whether each is
// still unmarked, neither reborn nor swept, and whether the slice was full.
func (c *Collector) runEntries(ctx context.Context, rec *Records, from []byte, base int) ([][]byte, []bool, bool, error) {
	// Scan past from, skipping the completion entry, until the slice fills.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	budget := c.share(base)
	var keys [][]byte
	var unmarked []bool
	full := false
	err := rec.scan(ctx, rec.key(kindUnmarked), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Skip the completion entry, stop at a full slice, and take the entry.
		if len(key) == 0 {
			return true, nil
		}
		if len(keys) == budget {
			full = true
			return false, nil
		}
		v, err := value()
		keys = append(keys, bytes.Clone(key))
		unmarked = append(unmarked, len(v) == 0)
		return true, err
	})
	return keys, unmarked, full, err
}

// appendDowns appends down, block keys losing one holder each, to the down
// log at now, and writes state.
func (r *Records) appendDowns(ctx context.Context, state *LivenessState, now time.Time, down [][]byte) error {
	if err := r.appendLogs(ctx, state, now, nil, down); err != nil {
		return err
	}
	return r.SetState(ctx, state)
}
