package space_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// spaceAdmissionState is where a Space candidate is in its lifecycle.
type spaceAdmissionState int

const (
	// admissionQueued means the operation waits in the queue or in
	// QueueOperation.
	admissionQueued spaceAdmissionState = iota
	// admissionPlaced means the driver applied the operation's position, which
	// the installed head does not hold yet.
	admissionPlaced
	// admissionDecided means the decision and the head are final.
	admissionDecided
)

// spaceAdmission is a Space candidate, or an operation queued without one,
// from its admission until the driver decides it. Its fields after op are
// guarded by the adapter's bcast.
type spaceAdmission struct {
	// a is the adapter.
	a *Adapter
	// candidate is the admitted candidate, or zero for an operation queued
	// without a World of its own.
	candidate world.Candidate
	// op is the encoded WorldOp the queue writes.
	op []byte
	// payloads are the operation's payload roots, held before it is queued.
	payloads []*block.BlockRef
	// done closes once state is admissionDecided.
	done chan struct{}

	// state is the lifecycle state.
	state spaceAdmissionState
	// localID is the operation's local ID once QueueOperation returned.
	localID string
	// hash identifies the operation once the driver applied it.
	hash []byte
	// decision is the decision once decided.
	decision world.Decision
	// reason is why replay refused the operation, when it did.
	reason string
	// head is the installed head once decided.
	head world.Head
	// err is the queue's failure, or the refusal as an error.
	err error
}

// Seqno returns the World seqno at the candidate's root.
func (s *spaceAdmission) Seqno() uint64 {
	return s.candidate.Seqno
}

// Head returns the installed head when the candidate was decided: for an
// accepted one, the head that holds its change. It is the zero Head before.
func (s *spaceAdmission) Head() world.Head {
	var head world.Head
	s.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.state == admissionDecided {
			head = s.head
		}
	})
	return head
}

// WaitDecided waits until the driver applied the candidate's position and
// the installed head holds it, or the queue failed it.
func (s *spaceAdmission) WaitDecided(ctx context.Context) (world.Decision, error) {
	// Wait for the decision.
	select {
	case <-ctx.Done():
		return world.Undecided, context.Cause(ctx)
	case <-s.done:
	}

	// Read it.
	var decision world.Decision
	var err error
	s.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		decision, err = s.decision, s.err
	})
	return decision, err
}

// WaitDurable returns the head's seqno once accepted: a decided candidate is
// durable, since the SharedObject holds its operation durably. A candidate
// replay refused returns world.ErrSuperseded or its refusal.
func (s *spaceAdmission) WaitDurable(ctx context.Context) (uint64, error) {
	decision, err := s.WaitDecided(ctx)
	switch {
	case err != nil:
		return 0, err
	case decision == world.Superseded:
		return 0, world.ErrSuperseded
	case decision != world.Accepted:
		return 0, errors.Wrap(sobject.ErrRejectedOp, s.reason)
	}
	return s.Head().Seqno, nil
}

// Release drops an admission whose operation has not reached
// QueueOperation, so it is never queued.
func (s *spaceAdmission) Release() {
	if s.a.queue.remove(s) {
		s.a.fail(s, world.ErrSuperseded)
	}
}

// _ is a type assertion
var _ world.Admission = (*spaceAdmission)(nil)
