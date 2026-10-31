package kvtx

import "context"

// backgroundCommitContextKey marks a context whose commits are background
// writes.
type backgroundCommitContextKey struct{}

// WithBackgroundCommitContext marks the write transactions committed with ctx
// as background writes, such as uploads, evictions and cache fills, which no
// request waits on. A store that holds back writers which outpace its
// maintenance holds these back first and lets other commits run on further.
// Stores without such a bound ignore the mark.
func WithBackgroundCommitContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundCommitContextKey{}, true)
}

// IsBackgroundCommitContext reports whether ctx carries
// WithBackgroundCommitContext.
func IsBackgroundCommitContext(ctx context.Context) bool {
	background, _ := ctx.Value(backgroundCommitContextKey{}).(bool)
	return background
}

// WithoutBackgroundCommitContext clears the background mark of ctx, for a
// commit that carries other writers' work along with the caller's.
func WithoutBackgroundCommitContext(ctx context.Context) context.Context {
	if !IsBackgroundCommitContext(ctx) {
		return ctx
	}
	return context.WithValue(ctx, backgroundCommitContextKey{}, false)
}
