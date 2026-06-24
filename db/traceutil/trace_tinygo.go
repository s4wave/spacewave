//go:build tinygo

package traceutil

import "context"

// Task is a no-op trace task for the TinyGo build.
type Task struct{}

// NewTask returns ctx and a no-op task on the TinyGo build.
func NewTask(ctx context.Context, _ string) (context.Context, *Task) {
	return ctx, &Task{}
}

// End ends the task; it is a no-op on the TinyGo build.
func (t *Task) End() {}

// Log is a no-op on the TinyGo build.
func Log(_ context.Context, _ string, _ string) {}

// Logf is a no-op on the TinyGo build.
func Logf(_ context.Context, _ string, _ string, _ ...any) {}
