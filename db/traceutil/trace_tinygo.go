//go:build tinygo

package traceutil

import "context"

type Task struct{}

func NewTask(ctx context.Context, _ string) (context.Context, *Task) {
	return ctx, &Task{}
}

func (t *Task) End() {}

func Log(_ context.Context, _ string, _ string) {}

func Logf(_ context.Context, _ string, _ string, _ ...any) {}
