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

	// Keep root reads from reporting the swap before its evidence is confirmed.
	t.a.proposed.Store(candidate.Root)
	defer t.a.proposed.Store(nil)

	// Store the entries and swap the root. The move does not require its
	// blocks held: the candidate's boundary pages are the base's, which the
	// root holds until this move, or blocks an open stage holds, so the
	// check would pay its reads on every commit to catch only an operation
	// that links a released tree, and no repeat of the commit could store
	// that tree.
	err := t.a.store.SetRoots(ctx, candidate.Entries, []block.NamedRoot{{
		Name:     t.a.name,
		Ref:      candidate.Root,
		Previous: candidate.Base,
	}})

	// A failed swap has no evidence to confirm and must not hide another writer.
	if err != nil {
		t.a.proposed.Store(nil)
	}

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
	if candidate.ConfirmDurable != nil {
		candidate.ConfirmDurable(head)
	}
	t.published(ctx, head)
	return &decidedAdmission{seqno: candidate.Seqno, decision: world.Accepted, head: head}, nil
}

// Release ends the turn, or the wait for it when Begin failed.
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
		t.a.turnEnded()
	})
}

// published records a head this turn swapped in, wakes this process's head
// watches, and announces it to the other processes' watches through the lease.
// The head is durable already, so an announcement that fails only delays them
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

// WaitBase returns at once: the base a lease reads is durable.
func (t *localTurn) WaitBase(context.Context) error {
	return nil
}

// _ is a type assertion
var _ world.Turn = (*localTurn)(nil)
