//go:build js && !tinygo

package s4wave_forge_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// workerPolicyWatch reports the absent daemon policy of a browser runtime.
//
// A browser has no daemon and no native host Root, so its Worker declares no
// Docker capacity and runs only targets that need none.
type workerPolicyWatch struct {
	// ctx ends the wait after the initial empty snapshot.
	ctx context.Context
	// sent records that the empty snapshot was delivered.
	sent bool
	// release cancels ctx.
	release context.CancelFunc
}

// openWorkerPolicyWatch returns a watch with no policy source.
func openWorkerPolicyWatch(ctx context.Context, _ bus.Bus) (*workerPolicyWatch, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &workerPolicyWatch{ctx: ctx, release: cancel}, nil
}

// Recv returns an empty snapshot once, then waits until the watch closes.
func (w *workerPolicyWatch) Recv() (*device_policy.DevicePolicy, string, error) {
	// Deliver the empty snapshot that starts the Worker without capacity.
	if !w.sent {
		w.sent = true
		return nil, "", nil
	}

	// No later snapshot exists; wait for Close or cancellation.
	<-w.ctx.Done()
	return nil, "", w.ctx.Err()
}

// Close ends a pending wait.
func (w *workerPolicyWatch) Close() { w.release() }
