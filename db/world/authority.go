package world

import (
	"context"
)

// Turn is one writer's chance to propose the next head.
// An Authority that orders writers across processes holds them off for the
// turn, so the base a writer reads stays the head until it proposes. The
// engine releases a turn as soon as Admit returns, so an Authority that
// pipelines gives the next turn the admitted candidate as its Base, not yet
// durable, and the engine builds on it without publishing it.
type Turn interface {
	// Base returns the head the authority held when the turn began.
	Base() Head
	// Admit proposes candidate as the head after candidate.Base.
	// Call it at most once per turn.
	// Returns the candidate's Admission.
	// An error means nothing was proposed and nothing of candidate is held.
	Admit(ctx context.Context, candidate Candidate) (Admission, error)
	// Release ends the turn.
	// An Admission returned by Admit outlives it.
	Release()
}

// Authority decides which candidate becomes a World's head and when a head is
// durable. One World engine serves every World over an Authority, and a local
// World and a Space World differ only in theirs.
//
// The local Authority is a coordinator lease and a compare-and-swap on the
// World's named root: a turn holds the lease, and a candidate is accepted
// and durable at once when the swap lands, or superseded when another writer
// moved the root first. The Space Authority queues the candidate as an
// operation, accepts it when the sequencer places it after Base, and reports
// it durable when a checkpoint records its root.
type Authority interface {
	// Head returns the current head.
	Head(ctx context.Context) (Head, error)
	// WatchHead calls fn with the current head, then with each later head in
	// the order the Authority decides them, until release is called.
	// A head arrives once when accepted and again when durable.
	// fn runs on one goroutine at a time and must not begin a turn.
	WatchHead(ctx context.Context, fn func(Head)) (release func(), err error)
	// Begin waits for a write turn.
	// Returns the turn, which the caller must release.
	Begin(ctx context.Context) (Turn, error)
}
