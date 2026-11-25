package block

import "context"

// ColdReadGate admits one reader's reads of blocks that no cache above the
// store holds, such as a view run whose first view waits for an admission
// slot only once it needs the store. A cursor calls EnterCold before each
// store read, after a decoded block cache miss; a read the decoded cache
// answers never reaches the gate, so a reader whose blocks are warm never
// waits on it. The reader calls Lend before it waits on work another reader
// does, such as a shared value another view computes, since that reader may
// itself wait at the gate for what this one holds. It also calls the other
// reader's Inherit, so that reader's admission is not ordered behind readers
// less urgent than the one waiting on it.
type ColdReadGate interface {
	// EnterCold waits until the reader may read from the store. It returns at
	// once while the reader holds its admission, and fails only when ctx ends.
	EnterCold(ctx context.Context) error
	// Lend gives back the admission EnterCold took, if any; the next
	// EnterCold takes it again.
	Lend()
	// Inherit admits the reader at least as urgently as the reader of
	// waiter, which waits on this reader's work, until the returned func
	// ends the loan.
	Inherit(waiter context.Context) (end func())
}

// coldReadGateKey is the context key of the ColdReadGate.
type coldReadGateKey struct{}

// WithColdReadGate returns ctx with gate admitting its store reads.
func WithColdReadGate(ctx context.Context, gate ColdReadGate) context.Context {
	return context.WithValue(ctx, coldReadGateKey{}, gate)
}

// ColdReadGateOf returns the context's gate, or nil.
func ColdReadGateOf(ctx context.Context) ColdReadGate {
	gate, _ := ctx.Value(coldReadGateKey{}).(ColdReadGate)
	return gate
}

// enterColdRead waits at the context's gate, if it carries one.
func enterColdRead(ctx context.Context) error {
	if gate := ColdReadGateOf(ctx); gate != nil {
		return gate.EnterCold(ctx)
	}
	return nil
}

// LendColdRead lends back the admission of the context's gate, if it carries
// one. Call it before waiting on work another reader does.
func LendColdRead(ctx context.Context) {
	if gate := ColdReadGateOf(ctx); gate != nil {
		gate.Lend()
	}
}
