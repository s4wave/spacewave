package world_block

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
)

// localTurn is a write turn of a LocalAuthority: the coordinator's lease on
// the root, or this process's turn when there is no coordinator.
type localTurn struct {
	// a is the authority.
	a *LocalAuthority
	// base is the head the turn read under the lease.
	base world.Head
	// lease is the coordinator's lease, or nil.
	lease coord.WriteLease
	// unlock ends the in-process turn, or is nil.
	unlock func()
	// release ends the turn once.
	release sync.Once
}

// Base returns the head the turn read under the lease.
func (t *localTurn) Base() world.Head {
	return t.base
}

// Admit stores the candidate's entries and swaps the root from its base to its
// root in one Volume transaction. The candidate is accepted and durable when
// the swap lands, and superseded when another writer moved the root.
func (t *localTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	// Refuse a candidate built on another base than the turn's.
	sameBase := candidate.Base.EqualsRef(t.base.Root) || (candidate.Base.GetEmpty() && t.base.Root.GetEmpty())
	if !sameBase {
		return nil, errors.New("world candidate base is not the turn's base")
	}

	// Store the entries and swap the root.
	err := t.a.store.SetRoots(ctx, candidate.Entries, []block.NamedRoot{{
		Name:     t.a.name,
		Ref:      candidate.Root,
		Previous: candidate.Base,
	}})

	// A root another writer moved supersedes the candidate.
	if errors.Is(err, block.ErrRootMoved) {
		head, herr := t.a.Head(ctx)
		if herr != nil {
			return nil, herr
		}
		return &decidedAdmission{seqno: candidate.Seqno, decision: world.Superseded, head: head}, nil
	}
	if err != nil {
		return nil, err
	}

	// Record the head and announce it.
	head := world.Head{Root: candidate.Root.Clone(), Seqno: candidate.Seqno, Durable: true}
	t.published(ctx, head)
	return &decidedAdmission{seqno: candidate.Seqno, decision: world.Accepted, head: head}, nil
}

// Release ends the turn.
func (t *localTurn) Release() {
	t.release.Do(func() {
		if t.lease != nil {
			if err := t.lease.Release(context.Background()); err != nil {
				t.a.le.WithError(err).Warn("unable to release the World write lease")
			}
		}
		if t.unlock != nil {
			t.unlock()
		}
	})
}

// published records a head this turn swapped in, wakes this process's head
// watches, and announces it to the other processes' through the lease. The
// head is durable already, so an announcement that fails only delays them
// until their next coordinator event.
func (t *localTurn) published(ctx context.Context, head world.Head) {
	// Wake this process's watches.
	t.base = head
	t.a.observe(head)

	// Announce the root to the other processes.
	if t.lease == nil {
		return
	}
	_, err := t.lease.Publish(ctx, coord.Event{RootChanged: &bucket.ObjectRef{RootRef: head.Root.Clone()}})
	if err != nil && !errors.Is(err, coord.ErrUnsupported) {
		t.a.le.WithError(err).Warn("unable to announce the World head")
	}
}

// _ is a type assertion
var _ world.Turn = (*localTurn)(nil)
