package block_gc

import (
	"bytes"
	"context"
	"crypto/rand"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// unrefCandidates bounds the front entries one reference scan settles, and
// the entries the collector remembers a scan found referenced.
const unrefCandidates = 4096

// sliceLists is how many roots or child lists one read snapshot of the
// reference scan reads.
const sliceLists = 4096

// unrefPhase is the step a reference scan is at.
type unrefPhase uint8

const (
	// unrefIdle collects the candidates a front scan offers.
	unrefIdle unrefPhase = iota
	// unrefMarking writes each candidate's mark.
	unrefMarking
	// unrefRoots reads the named roots.
	unrefRoots
	// unrefLists reads the stored child lists.
	unrefLists
	// unrefPopping pops the candidates no holder references.
	unrefPopping
)

// unrefScan is the collector's reference scan, which takes off the front
// the entries of unborn blocks that nothing references. An unborn block is
// one the store lacks, with neither a child list nor a hole record. Its
// count includes the implied holder its birth would take back, so once its
// last parent releases it reads one holder for good, and the front's proof
// at zero never pops it. One real holder kept after a loss reads the same,
// so the count cannot tell the two apart. The scan can: it reads every root
// and every stored list once.
//
// Invariant, which FrontUnreferenced.tla checks: the scan pops an entry
// only when, since the transaction that wrote the entry's mark, it read
// every named root and every stored child list and found none that holds
// its children and references the block, and the mark still stands in the
// pop's transaction. The mark is the scan's own token, random per scan, so
// a mark another collector on the same store wrote, or one a restart left
// behind, never stands for this scan. Records.addFront writes the front key
// with no value, so any push since the mark deletes it. A list whose park
// released its children does not hold.
// Every writer that makes a root or an old list hold the block afterwards
// pushes the block in the same transaction, which deletes the mark: the
// walk when it verifies a list, the seeding, the unpark of a released list
// and a readoption, as Records.pushOwed, Records.unpark and
// Records.unrunParked describe. A copied put that keeps a parked block's
// unverified list puts that block on the front, so the walk's verify pushes
// the list's children before the flip. A list born from content, and a
// copied put that keeps a marked list, is the one holder that pushes no
// absent child, with or without the scan, as lander.holdOutside shows, so
// the front never held an entry for it: the block is off the front whether
// such a parent lands before the pop or after it. A candidate whose mark a
// push deleted stays, and the scan notes it missed, so the next front scan
// offers it again.
//
// A block the store lost after the last recount, with no hole record and
// one real parent, reads as a candidate too, and the scan keeps it, since
// that parent holds. Its count stays wrong, with or without the scan: a
// later put of the block takes back an implied holder the count never
// included.
//
// Cost: per scan, one sequential read of the roots and of the child lists,
// values included, in read snapshots of sliceLists records that hold no
// writer, and for each candidate two write transactions of up to four point
// reads. A scan runs only when a front scan kept a candidate, and each
// candidate it finds referenced is not offered again until its count falls
// to one again or a new adoption counts.
type unrefScan struct {
	// phase is the scan's step.
	phase unrefPhase
	// cands maps each candidate's block key to whether the scan found a
	// holder that references it.
	cands map[string]bool
	// keys are the candidates in key order, and at the index the marking or
	// the popping resumes at.
	keys [][]byte
	at   int
	// token is the mark this scan writes under each candidate's front key.
	token []byte
	// from is the root or list key after which the read resumes, nil at the
	// start.
	from []byte
	// left counts the candidates no holder was found for yet.
	left int
	// adoptAt is the adoption mark the marking read. A scan ends unpopped
	// once another adoption counted.
	adoptAt int64
	// missed reports that a front scan kept a candidate this scan could not
	// take, so the front is scanned again once the scan ends.
	missed bool
	// held are the candidates a scan found referenced. A front scan does not
	// offer them again until the netting takes one to a single holder or an
	// adoption counts. Once it holds unrefCandidates entries, no candidate
	// is offered: a Volume with that many unrecorded losses needs the audit,
	// not one list scan per front scan.
	held map[string]struct{}
	// stale and wait are what the last marking or popping slice found: the
	// generation left Open or another adoption counted, or the Volume is
	// not freeable yet.
	stale, wait bool
}

// offer takes a candidate a front scan kept, or notes it missed.
func (u *unrefScan) offer(key []byte) {
	// Skip one a scan found referenced, and every one once that set is full.
	if _, ok := u.held[string(key)]; ok || len(u.held) >= unrefCandidates {
		return
	}

	// Take it while no scan runs and the set has room.
	if _, ok := u.cands[string(key)]; ok {
		return
	}
	if u.phase != unrefIdle || len(u.cands) >= unrefCandidates {
		u.missed = true
		return
	}
	if u.cands == nil {
		u.cands = make(map[string]bool)
	}
	u.cands[string(key)] = false
}

// forget drops the candidates remembered as referenced.
func (u *unrefScan) forget() {
	clear(u.held)
}

// wakeUnreferenced notes that the netting took the front block at key to
// one holder, which may leave it unreferenced: the front is scanned again,
// and the block is offered again though a scan found it referenced. The
// netting calls it in its transaction, which may not commit; a wake that
// did not commit costs one front scan.
func (c *Collector) wakeUnreferenced(key []byte) {
	c.frontWoke = true
	delete(c.unref.held, string(key))
}

// unbornAtOne reports whether the block at key reads as the reference scan
// requires of a candidate: one holder, neither a child list nor a hole
// record, and no stored bytes.
//
// Cost: one count read, and for a block at one a list read, one or two hole
// reads and an existence read of the bytes.
func (r *Records) unbornAtOne(ctx context.Context, blocks block.StoreOps, key []byte) (bool, error) {
	holders, _, err := r.holders(ctx, key)
	if err != nil || holders != 1 {
		return false, err
	}
	return r.unborn(ctx, blocks, key)
}

// unborn reports whether the store holds neither a child list, a hole
// record nor bytes for the block at key.
func (r *Records) unborn(ctx context.Context, blocks block.StoreOps, key []byte) (bool, error) {
	// Refuse a listed block and a recorded hole.
	listed, err := r.listed(ctx, key)
	if err != nil || listed {
		return false, err
	}
	open, err := r.holesOpen(ctx)
	if err != nil {
		return false, err
	}
	if open {
		hole, err := r.hasHole(ctx, key)
		if err != nil || hole {
			return false, err
		}
	}

	// Refuse one whose bytes remain, which the walk decodes.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return false, err
	}
	stored, err := blocks.GetBlockExists(ctx, ref)
	return !stored, err
}

// settleUnreferenced runs the reference scan from where the last pass left
// it, as unrefScan describes, and reports whether work is left: it stopped
// at past, or it ended with a candidate missed, which the next pass's front
// scan offers. state is the state the pass read. A scan that waits to pop
// on a Volume state reads as not freeable costs a pass nothing.
func (c *Collector) settleUnreferenced(ctx context.Context, state *LivenessState, past func() bool, res *CollectResult) (bool, error) {
	u := &c.unref
	if c.opts.KeepUnreferenced || (u.phase == unrefPopping && !c.freeable(state)) {
		return false, nil
	}
	for {
		switch u.phase {
		case unrefIdle:
			// Start a scan over the candidates the front scan kept.
			if len(u.cands) == 0 {
				return c.endUnreferenced(), nil
			}
			u.keys = u.keys[:0]
			for key := range u.cands {
				u.keys = append(u.keys, []byte(key))
			}
			slices.SortFunc(u.keys, bytes.Compare)
			u.phase, u.at, u.token = unrefMarking, 0, []byte(rand.Text())

		case unrefMarking:
			// Mark each candidate that still reads as one.
			more, err := resumeSlices(ctx, c.update, &u.at, past, func(rec *Records, blocks block.StoreOps, from int) (int, bool, error) {
				return c.unrefMarkSlice(ctx, rec, blocks, from)
			})
			if err != nil || more {
				return more, err
			}
			if u.left = len(u.cands); u.stale || u.left == 0 {
				return c.endUnreferenced(), nil
			}
			u.phase, u.from = unrefRoots, nil

		case unrefRoots, unrefLists:
			// Read the roots, then the lists, until every candidate has a
			// holder or the lists end.
			more, err := c.unrefRead(ctx, past, res)
			if err != nil || more {
				return more, err
			}
			switch {
			case u.left == 0:
				return c.endUnreferenced(), nil
			case u.phase == unrefRoots:
				u.phase, u.from = unrefLists, nil
			default:
				u.phase, u.at = unrefPopping, 0
			}

		case unrefPopping:
			// Pop each candidate no holder references, once the Volume is
			// freeable.
			more, err := resumeSlices(ctx, c.update, &u.at, past, func(rec *Records, blocks block.StoreOps, from int) (int, bool, error) {
				return c.unrefPopSlice(ctx, rec, blocks, res, from)
			})
			if err != nil || more || u.wait && !u.stale {
				return more, err
			}
			return c.endUnreferenced(), nil
		}
	}
}

// endUnreferenced ends the reference scan, remembers the candidates it
// found referenced, and reports whether the front needs another scan for a
// candidate it missed.
func (c *Collector) endUnreferenced() bool {
	// Remember the referenced candidates while the set has room.
	u := &c.unref
	for key, referenced := range u.cands {
		if !referenced || u.stale || len(u.held) >= unrefCandidates {
			continue
		}
		if u.held == nil {
			u.held = make(map[string]struct{})
		}
		u.held[key] = struct{}{}
	}

	// Return to idle, and scan the front again for what this scan missed or
	// left marked.
	again := u.missed || u.stale
	clear(u.cands)
	*u = unrefScan{cands: u.cands, keys: u.keys[:0], held: u.held}
	if again {
		c.frontWoke = true
	}
	return again
}

// unrefMarkSlice writes the scan's token as the mark of up to a slice of
// candidates from the index from that still read as candidates, and drops
// the others. The mark does not raise the add revision: it adds no entry,
// and a front scan reads a marked key as an entry all the same. The first
// slice records the adoption mark, and a slice that reads the generation
// not Open or another adoption mark ends the scan as stale.
func (c *Collector) unrefMarkSlice(ctx context.Context, rec *Records, blocks block.StoreOps, from int) (int, bool, error) {
	// Mark only while Open, under one adoption.
	u := &c.unref
	state, err := rec.GetState(ctx)
	if err != nil {
		return from, false, err
	}
	if from == 0 {
		u.adoptAt = state.GetAdoptAtUnixMilli()
	}
	u.stale = state.GetGeneration() != Generation_GENERATION_OPEN || state.GetAdoptAtUnixMilli() != u.adoptAt
	if u.stale {
		return from, false, nil
	}

	// Mark each entry still on the front whose block is unborn at one.
	end := min(from+c.share(sliceRecords), len(u.keys))
	for _, key := range u.keys[from:end] {
		fronted, err := rec.hasFront(ctx, key)
		if err != nil {
			return from, false, err
		}
		if fronted {
			if fronted, err = rec.unbornAtOne(ctx, blocks, key); err != nil {
				return from, false, err
			}
		}
		if !fronted {
			delete(u.cands, string(key))
			continue
		}
		if err := rec.tx.Set(ctx, rec.key(kindFront, key), u.token); err != nil {
			return from, false, err
		}
	}
	return end, end < len(u.keys), nil
}

// unrefRead reads the roots or the child lists, as the phase says, in read
// snapshots of up to sliceLists records from the scan's position, and notes
// each candidate a root or a holding list references. It stops once every
// candidate has a holder or the records end, and reports whether it stopped
// at past with records left.
func (c *Collector) unrefRead(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	u := &c.unref
	kind := kindChildren
	if u.phase == unrefRoots {
		kind = kindRoot
	}
	for {
		// Read one snapshot's records after the position.
		var last []byte
		var read int
		budget := c.share(sliceLists)
		err := c.store.View(ctx, func(rec *Records) error {
			last, read = last[:0], 0
			seek := u.from
			if seek != nil {
				seek = append(bytes.Clone(seek), 0)
			}
			return rec.scanKind(ctx, kind, seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
				// Stop at the slice's budget, or once every candidate has a
				// holder, and read the record otherwise.
				if read == budget || u.left == 0 {
					return false, nil
				}
				read++
				last = append(last[:0], key...)
				v, err := value()
				if err != nil {
					return false, err
				}
				if kind == kindRoot {
					return true, u.readRoot(v)
				}
				return true, u.readList(ctx, rec, key, v)
			})
		})
		if err != nil {
			return false, err
		}
		res.RefScanned += uint64(read) //nolint:gosec // counts are never negative.

		// Resume after the last record, unless the read reached the end.
		if read < budget || u.left == 0 {
			return false, nil
		}
		u.from = bytes.Clone(last)
		if past != nil && past() {
			return true, nil
		}
		if err := context.Cause(ctx); err != nil {
			return false, err
		}
	}
}

// readRoot notes the candidate the root entry value holds.
func (u *unrefScan) readRoot(value []byte) error {
	entry := &RootEntry{}
	if err := entry.UnmarshalVT(value); err != nil {
		return err
	}
	u.hit(entry.GetRef())
	return nil
}

// readList notes each candidate the child list value of the block at key
// references, unless a park released the list's children, as kindReleased
// describes. It reads the list's park only for a list that references a
// candidate.
func (u *unrefScan) readList(ctx context.Context, rec *Records, key, value []byte) error {
	// Find the candidates among the children.
	var found [][]byte
	err := eachKey(listKeys(value), func(child []byte) error {
		if referenced, ok := u.cands[string(child)]; ok && !referenced {
			found = append(found, child)
		}
		return nil
	})
	if err != nil || len(found) == 0 {
		return err
	}

	// A released list holds nothing.
	_, _, released, err := rec.parkState(ctx, key)
	if err != nil || released {
		return err
	}
	for _, child := range found {
		u.hit(child)
	}
	return nil
}

// hit notes that a holder references the candidate at key, if it is one.
func (u *unrefScan) hit(key []byte) {
	if referenced, ok := u.cands[string(key)]; ok && !referenced {
		u.cands[string(key)] = true
		u.left--
	}
}

// unrefPopSlice takes off the front up to a slice of candidates from the
// index from that the scan found no holder for, each only while its mark
// stands and its block still reads unborn at one. One whose mark a push
// deleted stays for the next scan. It pops under the gates
// of the front's own pop, as frontSlice describes: while Open, freeable and
// with no recount owed. A Volume not freeable yet leaves the scan waiting,
// which is safe for as long as the marks stand; a generation that left Open
// or another adoption ends it as stale.
func (c *Collector) unrefPopSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from int) (int, bool, error) {
	// Pop only while Open under the marking's adoption, and wait while the
	// free proof cannot hold.
	u := &c.unref
	state, err := rec.GetState(ctx)
	if err != nil {
		return from, false, err
	}
	u.stale = state.GetGeneration() != Generation_GENERATION_OPEN || state.GetAdoptAtUnixMilli() != u.adoptAt
	if u.stale {
		return from, false, nil
	}
	recount, err := rec.recountOwed(ctx)
	if err != nil {
		return from, false, err
	}
	if u.wait = recount || !c.freeable(state); u.wait {
		return from, false, nil
	}

	// Pop each unreferenced candidate whose mark no push deleted.
	end := min(from+c.share(sliceRecords), len(u.keys))
	for _, key := range u.keys[from:end] {
		if referenced, ok := u.cands[string(key)]; !ok || referenced {
			continue
		}
		mark, fronted, err := rec.frontMark(ctx, key)
		if err != nil {
			return from, false, err
		}
		if !fronted {
			continue
		}
		if !bytes.Equal(mark, u.token) {
			u.missed = true
			continue
		}
		unborn, err := rec.unbornAtOne(ctx, blocks, key)
		if err != nil {
			return from, false, err
		}
		if !unborn {
			continue
		}
		if err := rec.tx.Delete(ctx, rec.key(kindFront, key)); err != nil {
			return from, false, err
		}
		res.Popped++
		res.Unreferenced++
	}
	return end, end < len(u.keys), nil
}
