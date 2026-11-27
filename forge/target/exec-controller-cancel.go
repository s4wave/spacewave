package forge_target

import "context"

type execCancelSignalKey struct{}

// WithExecCancelSignal attaches a durable execution cancellation signal.
func WithExecCancelSignal(ctx context.Context, signal <-chan struct{}) context.Context {
	return context.WithValue(ctx, execCancelSignalKey{}, signal)
}

// execCancelSignal returns the durable execution cancellation signal.
func execCancelSignal(ctx context.Context) <-chan struct{} {
	signal, _ := ctx.Value(execCancelSignalKey{}).(<-chan struct{})
	return signal
}
