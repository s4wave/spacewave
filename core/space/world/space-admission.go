//go:build space_adapter_decl

package space_world

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// spaceAdmissionState is where a Space candidate is in its lifecycle.
type spaceAdmissionState int

const (
	// admissionQueued means the operation waits in the queue or in
	// QueueOperation.
	admissionQueued spaceAdmissionState = iota
	// admissionPlaced means the SharedObject holds the operation durably and
	// the driver has not applied it yet.
	admissionPlaced
	// admissionDecided means the driver applied the operation's position, so
	// the decision and the head are final.
	admissionDecided
	// admissionFailed means QueueOperation failed: the operation is not in
	// the order, and the candidate never becomes a head.
	admissionFailed
)

// spaceAdmission is the Admission of a Space candidate, from Admit until the
// driver decides it or the writer releases it. Its state is guarded by the
// adapter's bcast.
type spaceAdmission struct {
	// a is the adapter.
	a *Adapter
	// candidate is the admitted candidate. Its Operation is the encoded
	// SOWorldOp the queue writes.
	candidate world.Candidate
	// localID is the operation's local ID once QueueOperation returned it.
	localID string
	// state is the candidate's lifecycle state.
	state spaceAdmissionState
	// decision is the decision once state is admissionDecided.
	decision world.Decision
	// head is the installed head after the operation's position: the
	// candidate's root, or a rebased root when replay placed operations the
	// writer did not build on before it.
	head world.Head
	// err is the queue's failure or the refusal's reason.
	err error
}

// Seqno returns the World seqno at the candidate's root.
func (s *spaceAdmission) Seqno() uint64 {
	return s.candidate.Seqno
}

// Head returns the head after the candidate's position once decided, or the
// zero Head before.
func (s *spaceAdmission) Head() world.Head {
	return world.Head{}
}

// WaitDecided waits until the driver applies the candidate's position, or the
// queue fails it.
func (s *spaceAdmission) WaitDecided(ctx context.Context) (world.Decision, error) {
	return world.Undecided, errAdapterUnbuilt
}

// WaitDurable returns the head's seqno once accepted: a decided candidate is
// durable, since the SharedObject holds its operation durably.
func (s *spaceAdmission) WaitDurable(ctx context.Context) (uint64, error) {
	return 0, errAdapterUnbuilt
}

// Release drops the admission. A candidate not yet queued leaves the queue,
// and the writer head falls back past it, with every candidate built on it
// following it out.
func (s *spaceAdmission) Release() {}

// _ is a type assertion
var _ world.Admission = (*spaceAdmission)(nil)
