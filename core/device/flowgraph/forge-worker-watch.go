package device_flowgraph

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// ForgeWorkerWatch holds, for each Space World this Device serves, the Forge
// Worker declaration of its accepted forge-worker node and the key of the
// Device that hosts it. Each Space's Reconciler sets its own World's value.
// The Worker of that World reads it. Each change to a World's value takes the
// next revision of one counter shared by every World, so a revision never
// repeats for a World even after its value returns to empty.
type ForgeWorkerWatch struct {
	// bcast guards the fields below and wakes watchers after every change.
	bcast broadcast.Broadcast
	// revision is the last revision assigned to any World's value. The empty
	// value of a World never set has revision one.
	revision uint64
	// worlds holds the value of each World set at least once, by engine ID.
	worlds map[string]*forgeWorkerValue
}

// forgeWorkerValue is one World's declaration and its revision.
type forgeWorkerValue struct {
	// revision is the revision of the last change to this value.
	revision uint64
	// deviceKey is the object key of the Device in this World.
	deviceKey string
	// declaration is the accepted declaration, or nil while no node is accepted.
	declaration *s4wave_device.ForgeWorkerDeclaration
}

// NewForgeWorkerWatch constructs a ForgeWorkerWatch declaring no Worker in
// any World.
func NewForgeWorkerWatch() *ForgeWorkerWatch {
	return &ForgeWorkerWatch{revision: 1, worlds: make(map[string]*forgeWorkerValue)}
}

// Set replaces the Device key and declaration of the World engineID, and wakes
// the watchers when either changed. A nil declaration declares no Worker.
func (w *ForgeWorkerWatch) Set(engineID, deviceKey string, declaration *s4wave_device.ForgeWorkerDeclaration) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Ignore a value the watchers already hold.
		current := w.worlds[engineID]
		if current == nil {
			current = &forgeWorkerValue{revision: 1}
		}
		if deviceKey == current.deviceKey && declaration.EqualVT(current.declaration) {
			return
		}

		// Record the change under the next revision and wake the watchers.
		w.revision++
		w.worlds[engineID] = &forgeWorkerValue{revision: w.revision, deviceKey: deviceKey, declaration: declaration.CloneVT()}
		broadcast()
	})
}

// WaitForgeWorker returns the encoded declaration, the Device key and the
// revision of the World engineID once its revision differs from last. A zero
// last returns the current value at once. The declaration is empty while the
// World declares no Worker.
func (w *ForgeWorkerWatch) WaitForgeWorker(ctx context.Context, engineID string, last uint64) ([]byte, string, uint64, error) {
	for {
		// Read the World's value, or the empty value of a World never set.
		value := &forgeWorkerValue{revision: 1}
		var waitCh <-chan struct{}
		w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if current := w.worlds[engineID]; current != nil {
				value = current
			}
			waitCh = getWaitCh()
		})

		// Return a changed value, or wait for the next change.
		if value.revision != last {
			data, err := value.declaration.MarshalVT()
			if err != nil {
				return nil, "", 0, errors.Wrap(err, "encode forge worker declaration")
			}
			return data, value.deviceKey, value.revision, nil
		}
		select {
		case <-ctx.Done():
			return nil, "", 0, ctx.Err()
		case <-waitCh:
		}
	}
}
