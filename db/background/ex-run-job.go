package background

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
)

// ExRunJob runs a background job and waits until it is done.
//
// It keeps a background controller with the default config loaded while it
// waits, shared with every other caller, so the controller runs exactly while
// some job needs it. The job retries failed attempts itself. ExRunJob returns
// nil once an attempt succeeds, or an error if ctx ends first or the
// controller cannot load. Returning releases the job.
func ExRunJob(ctx context.Context, b bus.Bus, key string, run JobFunc) error {
	// Keep the background controller loaded while the job is registered.
	_, _, ctrlRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		return err
	}
	defer ctrlRef.Release()

	// Wait for the job to report done.
	_, _, ref, err := bus.ExecWaitValue(ctx, b, NewRunJob(key, run), nil, nil, func(state *JobState) (bool, error) {
		return state.GetDone() != nil, nil
	})
	if err != nil {
		return err
	}
	ref.Release()
	return nil
}
