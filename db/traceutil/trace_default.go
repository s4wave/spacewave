//go:build !tinygo

// Package traceutil wraps runtime/trace so callers compile under TinyGo, which
// lacks the runtime/trace package; the TinyGo build provides no-op equivalents.
package traceutil

import (
	"context"
	"runtime/trace"
)

// Task is a runtime/trace task.
type Task = trace.Task

// NewTask creates a trace task of the given type rooted at ctx.
func NewTask(ctx context.Context, taskType string) (context.Context, *Task) {
	return trace.NewTask(ctx, taskType)
}

// Log emits a single trace log message in the given category.
func Log(ctx context.Context, category string, message string) {
	trace.Log(ctx, category, message)
}

// Logf emits a formatted trace log message in the given category.
func Logf(ctx context.Context, category string, format string, args ...any) {
	trace.Logf(ctx, category, format, args...)
}
