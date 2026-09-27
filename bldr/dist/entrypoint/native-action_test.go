//go:build !js

package dist_entrypoint

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestNativeActionPrecedesWritableRuntime proves application attachment cannot
// accidentally enter a distribution runner or build its writable bus.
func TestNativeActionPrecedesWritableRuntime(t *testing.T) {
	failure := errors.New("application action result")
	called := false
	err := runNativeAction(t.Context(), nil, func(context.Context, *logrus.Entry) error {
		called = true
		return failure
	}, func(context.Context, *logrus.Entry, NativeRun) error {
		t.Fatal("application action entered the native runner")
		return nil
	}, func(context.Context, []DistBusHook, []DistBusHook) error {
		t.Fatal("application action constructed a writable distribution")
		return nil
	})
	if !called || err != failure {
		t.Fatalf("action called=%v, error=%v", called, err)
	}
}

// TestNativeActionPreservesRunner proves the default path calls run once.
func TestNativeActionPreservesRunner(t *testing.T) {
	for _, withRunner := range []bool{false, true} {
		calls := 0
		var runner NativeRunner
		if withRunner {
			runner = func(ctx context.Context, _ *logrus.Entry, run NativeRun) error {
				return run(ctx, nil, nil)
			}
		}
		err := runNativeAction(t.Context(), nil, nil, runner, func(context.Context, []DistBusHook, []DistBusHook) error {
			calls++
			return nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("runner=%v: calls=%d, error=%v", withRunner, calls, err)
		}
	}
}
