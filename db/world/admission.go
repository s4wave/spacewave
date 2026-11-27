package world

import (
	"context"
)

// Admission is one Candidate's place in its Authority's order, from Admit
// until the head moves past it or it is released.
type Admission interface {
	// Seqno returns the World seqno at the candidate's root.
	Seqno() uint64
	// Head returns the head the Authority held when it decided, so a
	// superseded writer learns what to rebase onto.
	// For an accepted candidate it is the head that holds the candidate's
	// change, which the engine publishes when WaitDurable returns: the
	// candidate's Root, or a root an Authority that replays operations
	// rebased onto ones the writer did not build on. The Authority keeps
	// that head's blocks until Release, and WatchHead may report it later.
	// Before WaitDecided returns a decision, it returns the zero Head.
	Head() Head
	// WaitDecided waits until the Authority accepts, supersedes or rejects
	// the candidate.
	// A rejection returns its reason with Rejected.
	// An error with Undecided means the wait ended first and the outcome is
	// unknown.
	WaitDecided(ctx context.Context) (Decision, error)
	// WaitDurable waits until a record the Authority keeps covers the
	// accepted candidate, and returns the World seqno that record reaches,
	// at least Seqno.
	// A candidate that was not accepted returns ErrSuperseded or its
	// rejection.
	// The wait ends with ctx or with the Authority's lifetime. An error
	// after Accepted, ctx's included, means the candidate's durability is
	// unknown, not that it failed: the writer reconciles with
	// Authority.Head, where a head at the candidate's root or past its seqno
	// means it is durable. A writer that must learn the outcome passes a
	// context that is not canceled.
	WaitDurable(ctx context.Context) (uint64, error)
	// OpHash returns the hash of the signed operation the Authority orders
	// the candidate as, once it signed one. It returns nil before that, and
	// for an Authority that orders no signed operation. An unknown outcome
	// after admission names the operation by it (DurabilityUnknownError), so
	// the writer can wait for it to settle. The caller must not modify it.
	OpHash() []byte
	// Release drops the admission.
	// Call it once, after WaitDurable returns, or to abandon the candidate.
	// Releasing a candidate the Authority has not accepted releases what
	// Admit holds of it. An accepted candidate stays the Authority's
	// promise and becomes durable regardless.
	Release()
}
