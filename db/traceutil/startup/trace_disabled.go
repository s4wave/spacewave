//go:build !bldr_startup_trace || tinygo

// Package startuptrace provides compile-time gated startup attribution tracing.
package startuptrace

import "context"

const buildTagged = false

// WithGraphLookupScope marks a context used by the startup graph lookup.
func WithGraphLookupScope(ctx context.Context) context.Context {
	return ctx
}

// GraphLookupScope reports whether ctx is used by the startup graph lookup.
func GraphLookupScope(context.Context) bool {
	return false
}

// Task is a no-op startup attribution trace task.
type Task struct{}

// NewTask returns ctx and a no-op startup attribution trace task.
func NewTask(ctx context.Context, _ string) (context.Context, Task) {
	return ctx, Task{}
}

// End ends the no-op startup attribution trace task.
func (t Task) End() {}

// Log is a no-op startup attribution trace log.
func Log(_ context.Context, _ string, _ string) {}

// Logf is a no-op formatted startup attribution trace log.
func Logf(_ context.Context, _ string, _ string, _ ...any) {}
