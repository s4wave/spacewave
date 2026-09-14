//go:build !js

package dist_entrypoint

import (
	"context"

	"github.com/sirupsen/logrus"
)

// NativeRun runs the distribution until cancellation or failure. It releases
// the distribution bus and all hook resources before returning.
type NativeRun func(ctx context.Context, preBuildHooks, postStartHooks []DistBusHook) error

// NativeRunner owns a native event loop. It calls run once, cancels run's
// context when the user quits, and waits for run to return before returning.
// A runner may use startup hooks to attach controls to the distribution bus.
type NativeRunner func(ctx context.Context, le *logrus.Entry, run NativeRun) error
