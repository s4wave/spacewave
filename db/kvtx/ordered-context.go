package kvtx

import "context"

// orderedCommitContextKey marks a context whose commits may commit ordered.
type orderedCommitContextKey struct{}

// WithOrderedCommitContext marks the write transactions committed with ctx as
// allowed to commit with write ordering in place of a full durability flush,
// as CommitOrdered does. Use it for writes a crash may lose, such as a copy of
// data another store holds. Stores without ordered commits ignore the mark.
func WithOrderedCommitContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, orderedCommitContextKey{}, true)
}

// IsOrderedCommitContext reports whether ctx carries WithOrderedCommitContext.
func IsOrderedCommitContext(ctx context.Context) bool {
	ordered, _ := ctx.Value(orderedCommitContextKey{}).(bool)
	return ordered
}
