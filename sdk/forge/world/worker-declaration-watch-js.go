//go:build js && !tinygo

package s4wave_forge_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// workerDeclarationWatch reports the absent declaration of a browser runtime.
//
// A browser has no daemon and no native host Root, so its Worker declares no
// Docker capacity and runs only targets that need none.
type workerDeclarationWatch struct {
	// ctx ends the wait after the initial empty declaration.
	ctx context.Context
	// sent records that the empty declaration was delivered.
	sent bool
	// release cancels ctx.
	release context.CancelFunc
}

// openWorkerDeclarationWatch returns a watch with no declaration source.
func openWorkerDeclarationWatch(ctx context.Context, _ bus.Bus) (*workerDeclarationWatch, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &workerDeclarationWatch{ctx: ctx, release: cancel}, nil
}

// Recv returns no declaration once, then waits until the watch closes.
func (w *workerDeclarationWatch) Recv() (*s4wave_device.ForgeWorkerDeclaration, string, error) {
	// Deliver the empty declaration that starts the Worker without capacity.
	if !w.sent {
		w.sent = true
		return nil, "", nil
	}

	// No later declaration exists; wait for Close or cancellation.
	<-w.ctx.Done()
	return nil, "", w.ctx.Err()
}

// Close ends a pending wait.
func (w *workerDeclarationWatch) Close() { w.release() }
