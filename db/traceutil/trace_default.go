//go:build !tinygo

package traceutil

import (
	"context"
	"runtime/trace"
)

type Task = trace.Task

func NewTask(ctx context.Context, taskType string) (context.Context, *Task) {
	return trace.NewTask(ctx, taskType)
}

func Log(ctx context.Context, category string, message string) {
	trace.Log(ctx, category, message)
}

func Logf(ctx context.Context, category string, format string, args ...any) {
	trace.Logf(ctx, category, format, args...)
}
