//go:build !js

package dist_entrypoint

import (
	"context"

	"github.com/s4wave/spacewave/bldr/entrypoint/compose"
	"github.com/sirupsen/logrus"
)

// runNativeAction dispatches application launch before entering a distribution
// runner. A runner still owns exactly one run invocation when selected.
func runNativeAction(ctx context.Context, le *logrus.Entry, action compose.NativeAction, runner NativeRunner, run NativeRun) error {
	// Application composition may attach to an existing runtime without a bus.
	if action != nil {
		return action(ctx, le)
	}

	// Preserve the native runner's one-run contract for distribution launches.
	if runner != nil {
		return runner(ctx, le, run)
	}
	return run(ctx, nil, nil)
}
