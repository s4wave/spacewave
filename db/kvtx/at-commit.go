package kvtx

import "context"

// AtCommitTxOps is a write transaction that can defer a write to its commit.
// A transaction that reads without the store's writer validates its reads at
// commit and reruns its body when a later commit changed one, so a body that
// reads a key every writer changes, such as a counter or the head of a log,
// would conflict with every commit. A deferred write reads that key at the
// commit point instead, holding the writer, so it never conflicts.
type AtCommitTxOps interface {
	// AtCommit runs fn when the transaction commits, after its reads
	// validated and while it holds the writer, against the state every
	// earlier commit left plus this transaction's writes. Deferred writes run
	// in the order they were added. fn's reads are not validated, since no
	// commit can come between them and the commit, and fn's error discards
	// the transaction. A transaction that already holds the writer runs fn at
	// once. fn must not commit or discard tx.
	AtCommit(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// AtCommit defers fn to the commit of tx when tx supports it, and
// otherwise runs it at once, which is the same when tx holds the writer
// from its start. Either way the transaction commits as if fn ran at its
// commit point. The body must not read a key fn writes after deferring it,
// since a deferred fn has not run yet; fn should prefetch nothing, and the
// body should prefetch the keys fn reads that may be cold, so the writer does
// not wait on the disk.
func AtCommit(ctx context.Context, tx Tx, fn func(ctx context.Context, tx Tx) error) error {
	if d, ok := tx.(AtCommitTxOps); ok {
		return d.AtCommit(ctx, fn)
	}
	return fn(ctx, tx)
}
