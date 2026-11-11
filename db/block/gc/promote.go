package block_gc

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// PromoteError stops a Land whose hold onto a pending block needs more
// suspect record reads than one Land makes, promoteReads, to promote it.
// Promote promotes the block in slices; the caller then retries the Land.
type PromoteError struct {
	// Ref is the pending block.
	Ref *block.BlockRef
}

// Error describes the stop.
func (e *PromoteError) Error() string {
	return "block " + e.Ref.MarshalString() + " needs promotion in slices before a hold"
}

// promoteReads bounds the suspect record reads of one promotion step: a Land
// makes at most this many, and Promote this many per transaction, the bound
// each slice of the collector makes.
const promoteReads = sliceRecords

// promoteFrame is one pending block on a promoter's path.
type promoteFrame struct {
	// key is the block.
	key []byte
	// list is its encoded child list as the walk read it.
	list []byte
	// next are the encoded children the walk has not checked yet.
	next []byte
	// stale reports that list was read in an earlier transaction, so the
	// promotion reads it again: a put may have added a child since.
	stale bool
}

// promoter promotes a pending block to clean, as holdStored describes: it
// walks down the pending blocks below it depth first, keeping only the path
// from the block to the current one, and promotes each whose children are
// all clean, deepest first, deleting its suspect record. A promoted block
// stays clean, since only Finish writes a counted record and it writes none
// once the mark clears, so the walk keeps no visited set: a block below two
// parents is clean when the second reaches it. Each check reads one suspect
// record, so a walk costs one read per edge below the pending blocks, and
// one child list read per pending block.
type promoter struct {
	// ref is the block the walk promotes.
	ref *block.BlockRef
	// path is the path from ref to the current block.
	path []promoteFrame
}

// newPromoter starts a promotion of the pending block ref.
func newPromoter(ref *block.BlockRef) *promoter {
	key := BlockKey(ref)
	return &promoter{ref: ref, path: []promoteFrame{{key: key, stale: true}}}
}

// step walks with at most *reads suspect record reads in rec, counting them
// down, taking promoted
// records from state, which the caller writes. It returns whether ref is
// clean now; the walk resumes where it stopped on the next step, in this
// transaction or a later one. It returns a SuspectError for a suspect block
// below ref, which stops the walk.
func (p *promoter) step(ctx context.Context, rec *Records, state *LivenessState, reads *int) (bool, error) {
	for len(p.path) != 0 {
		f := &p.path[len(p.path)-1]

		// Read a list an earlier transaction read again. A missing one is a
		// block the collector freed since, which is absent and so clean.
		if f.stale {
			list, listed, err := rec.tx.Get(ctx, rec.key(kindChildren, f.key))
			if err != nil {
				return false, err
			}
			if !listed {
				p.path = p.path[:len(p.path)-1]
				continue
			}
			if f.list == nil || !bytes.Equal(list, f.list) {
				f.list, f.next = list, list
			}
			f.stale = false
		}

		// Promote a block whose children are all clean.
		if len(f.next) == 0 {
			if err := rec.dropSuspect(ctx, state, f.key); err != nil {
				return false, err
			}
			p.path = p.path[:len(p.path)-1]
			continue
		}

		// Check the next child.
		if *reads <= 0 {
			return false, nil
		}
		*reads--
		child, ok := nextBlockKey(f.next)
		if !ok {
			return false, errBadBlockKey
		}
		f.next = f.next[len(child):]
		trust, err := rec.getTrust(ctx, child)
		if err != nil {
			return false, err
		}
		switch trust {
		case listSuspect:
			ref, _, err := ParseBlockKey(child)
			if err != nil {
				return false, err
			}
			return false, &SuspectError{Ref: ref}
		case listPending:
			p.path = append(p.path, promoteFrame{key: bytes.Clone(child), stale: true})
		}
	}
	return true, nil
}

// resume returns a copy of the walk to continue in a new transaction, with
// every list on its path marked as read in an earlier one. A transaction
// that fails leaves p as it was, so a retry walks from the same place.
func (p *promoter) resume() *promoter {
	path := slices.Clone(p.path)
	for i := range path {
		path[i].stale = true
	}
	return &promoter{ref: p.ref, path: path}
}

// Promote promotes the pending block ref to clean in slices of promoteReads
// suspect record reads, one transaction each, after a Land returned a
// PromoteError for it. The walk keeps its path between slices, so the whole
// promotion reads each edge below the pending blocks once, as promoter
// describes, plus one child list per block on the path at each promotion
// after a slice boundary. It returns a SuspectError for a suspect block
// below ref.
func Promote(ctx context.Context, store Store, ref *block.BlockRef) error {
	p := newPromoter(ref)
	_, err := eachSlice(ctx, store.Update, p, nil, func(rec *Records, _ block.StoreOps, p *promoter) (*promoter, bool, error) {
		// Walk one slice from a copy of the path.
		state, err := rec.GetState(ctx)
		if err != nil {
			return p, false, err
		}
		p = p.resume()
		before := state.GetSuspectLists()
		reads := promoteReads
		done, err := p.step(ctx, rec, state, &reads)
		if err != nil {
			return p, false, err
		}

		// Write the count the promotions took from.
		if state.GetSuspectLists() != before {
			if err := rec.SetState(ctx, state); err != nil {
				return p, false, err
			}
		}
		return p, !done, nil
	})
	return err
}

// _ is a type assertion
var _ error = (*PromoteError)(nil)
