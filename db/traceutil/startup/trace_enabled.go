//go:build bldr_startup_trace && !tinygo

// Package startuptrace provides compile-time gated startup attribution tracing.
package startuptrace

import (
	"context"
	"runtime/trace"
)

const buildTagged = true

// Task is a startup attribution trace task.
type Task struct {
	task *trace.Task
}

// NewTask creates a startup attribution trace task rooted at ctx.
func NewTask(ctx context.Context, taskType string) (context.Context, Task) {
	ctx, task := trace.NewTask(ctx, taskType)
	return ctx, Task{task: task}
}

// End ends the startup attribution trace task.
func (t Task) End() {
	t.task.End()
}

// Log emits a startup attribution trace log message.
func Log(ctx context.Context, category string, message string) {
	trace.Log(ctx, category, message)
}

// Logf emits a formatted startup attribution trace log message.
func Logf(ctx context.Context, category string, format string, args ...any) {
	trace.Logf(ctx, category, format, args...)
}
