package world

import "context"

// CommitReceipt separates accepted preparation from completed durable publication.
// Wait cancellation only stops waiting; it never implies that publication rolled
// back. The receipt can be waited again. A nil result means the normal Commit
// durability and publication contract has completed, not merely queue admission.
type CommitReceipt interface {
	Done() <-chan struct{}
	Wait(context.Context) error
}

// SubmittableTx optionally releases the producer's mutable transaction after
// sealing and admission, while returning an explicit durable completion fence.
// A successor may prepare against that producer's private ordered revision.
// Canonical readers and other writers still observe only durable state.
//
// Ordinary Commit remains synchronous. Resource callers can use concurrent
// Commit requests without changing the existing wire contract. Implementations
// without run-ahead may return a receipt already completed by synchronous Commit.
type SubmittableTx interface {
	Tx
	Submit(context.Context) (CommitReceipt, error)
}
