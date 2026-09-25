//go:build !js

package paylog

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/volume/crashtest"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/logindex"
)

// crashEngines are the engines the crash test runs on. The log engine
// checkpoints often and in the foreground, so its device calls keep one order
// and the crash points cover its checkpoints.
var crashEngines = []engine{boltEngine, logEngine(logindex.Options{CheckpointBytes: 128, Foreground: true})}

// TestCrashRecovery runs the crash workload on the memory device with every
// crash engine.
func TestCrashRecovery(t *testing.T) {
	for _, e := range crashEngines {
		t.Run(e.name, func(t *testing.T) {
			crashtest.Run(t, crashtest.DurableBlocks, device.NewMemory, func(ctx context.Context, d *device.Memory) (crashtest.Target, error) {
				return e.openStore(ctx, d)
			})
		})
	}
}
