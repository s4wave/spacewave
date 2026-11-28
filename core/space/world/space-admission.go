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
	// admissionPlaced means replay applied the operation at a position the
	// replay holds above the installed checkpoint, or one a reorder put back
	// and the missing set holds until replay applies it again, so the head
	// installHead builds holds it. The evidence is that position alone: an
	// install that drops it returns the admission to admissionQueued or
	// proves it admissionCovered (reconcilePlacedLocked).
	admissionPlaced
	// admissionCovered means an installed checkpoint is proven to hold the
	// operation: its position is on the checkpoint's covered chain, or it was
	// placed at a position the checkpoint's World covers. Every later head
	// descends from that checkpoint, so installHead accepts it.
	admissionCovered
	// admissionDecided means the decision and the head are final.
	admissionDecided
)

// spaceAdmission is a Space candidate, or an operation queued without one,
// from its admission until the driver decides it. The receipt is published
// by the signer; the adapter's bcast guards the decision and replay evidence.
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
	// handlers identifies the handlers that identified payloads.
	handlers *PayloadHandlers
	// done closes once state is admissionDecided.
	done chan struct{}
	// receipt is the operation's signed identity before it can enter replay.
	receipt sobject.SignedOperationReceipt
	// after is the pending admission whose candidate this candidate was built
	// on, or nil when it was built on the installed head.
	after *spaceAdmission
	// afterHead is this device's LastWrite in the installed head the turn
	// began on, the write the candidate was built after when after is nil.
	afterHead []byte

	// state is the lifecycle state.
	state spaceAdmissionState
	// localID is the queue callback's ID for the legacy matcher.
	localID string
	// pos is the operation's position in this device's chain once
	// QueueOperation returned, which an installed checkpoint decides.
	pos *sobject.SOOperationPosition
	// hash identifies the replay position the driver matched to this admission.
	hash []byte
	// expected is the ExpectedWrite the queue wrote into the operation, or nil.
	expected *ExpectedWrite
	// decision is the decision once decided.
	decision world.Decision
	// reason is why replay refused the operation, when it did.
	reason string
	// head is the head once decided: for an accepted one, the reportable
	// head, whose retained roots are held.
	head world.Head
	// err is the queue's failure, or the refusal as an error.
	err error
	// settle is the unknown write of an earlier mount this admission settles,
	// or nil for a live write. An adopted admission has no op or candidate.
	settle *settleEntry
	// sent is the settle entry of this live write, registered before the
	// queue passed it to QueueOperationWithReceipt, or nil. decide tells it
	// when the write ends unknown and drops it otherwise.
	sent *settleEntry
	// abandoned is set when the committer stopped waiting before the
	// decision, or released the admission, so decide does not keep its heads
	// live for a publication that never comes.
	abandoned bool
	// moves is the admission's place in the adapter's rootMoveQueue.
	moves admissionMoves
}

// Seqno returns the World seqno at the candidate's root.
func (s *spaceAdmission) Seqno() uint64 {
	return s.candidate.Seqno
}

// Head returns the head when the candidate was decided: for an accepted one,
// the reportable head, which holds its change and whose retained roots are
// held. It is the zero Head before.
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
	// Wait for the decision, preferring one already made when ctx ends at
	// the same time, as when an unmount decides and then retires the engine.
	select {
	case <-s.done:
	case <-ctx.Done():
	}

	// Read it, or mark the wait abandoned under the lock decide takes.
	decision, err := world.Undecided, context.Cause(ctx)
	s.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.state != admissionDecided {
			s.abandoned = true
			return
		}
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

// OpHash returns the signed operation's hash once the SharedObject signed
// it, or nil before.
func (s *spaceAdmission) OpHash() []byte {
	return s.receipt.OpHash()
}

// Release drops an admission whose operation has not reached
// QueueOperation, so it is never queued, and ends the liveness an accepted
// one gave its heads for the committer's publication. A Release before the
// decision marks the admission abandoned in the same section, so a later
// Accepted decision holds nothing that no Release would drop.
func (s *spaceAdmission) Release() {
	s.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.abandoned = true
		delete(s.a.accepted, s)
	})
	if s.a.queue.remove(s) {
		s.a.fail(s, world.ErrSuperseded)
	}
}

// _ is a type assertion
var _ world.Admission = (*spaceAdmission)(nil)
