package viewstream

import "context"

// priorityKey carries the caller-owned first-read priority.
type priorityKey struct{}

// WithPriority binds a scheduling priority to the context. Smaller builds first.
func WithPriority(ctx context.Context, priority int) context.Context {
	return context.WithValue(ctx, priorityKey{}, priority)
}

// Priority reads the priority or returns fallback when no caller supplied one.
func Priority(ctx context.Context, fallback int) int {
	p, ok := ctx.Value(priorityKey{}).(int)
	if !ok {
		return fallback
	}
	return p
}
