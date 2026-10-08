package device_flowgraph

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// ForgeWorkerWatch holds the Forge Worker declaration of the accepted
// forge-worker node and the key of the Device that hosts it. The Reconciler
// sets it; the plugin host reads it. Each change to either value is a new
// revision.
type ForgeWorkerWatch struct {
	// bcast guards the fields below and wakes watchers after every change.
	bcast broadcast.Broadcast
	// revision counts the changes, starting at one for the empty value.
	revision uint64
	// deviceKey is the object key of the Device, or empty before it is known.
	deviceKey string
	// declaration is the accepted declaration, or nil while no node is accepted.
	declaration *s4wave_device.ForgeWorkerDeclaration
}

// NewForgeWorkerWatch constructs a ForgeWorkerWatch declaring no Worker.
func NewForgeWorkerWatch() *ForgeWorkerWatch {
	return &ForgeWorkerWatch{revision: 1}
}

// Set replaces the Device key and declaration, and wakes the watchers when
// either changed. A nil declaration declares no Worker.
func (w *ForgeWorkerWatch) Set(deviceKey string, declaration *s4wave_device.ForgeWorkerDeclaration) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Ignore a value the watchers already hold.
		if deviceKey == w.deviceKey && declaration.EqualVT(w.declaration) {
			return
		}

		// Record the change and wake the watchers.
		w.deviceKey = deviceKey
		w.declaration = declaration.CloneVT()
		w.revision++
		broadcast()
	})
}

// WaitForgeWorker returns the encoded declaration, the Device key and the
// revision once the revision differs from last. A zero last returns the current
// value at once. The declaration is empty while no Worker is declared.
func (w *ForgeWorkerWatch) WaitForgeWorker(ctx context.Context, last uint64) ([]byte, string, uint64, error) {
	for {
		var (
			declaration *s4wave_device.ForgeWorkerDeclaration
			deviceKey   string
			revision    uint64
			waitCh      <-chan struct{}
		)
		w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			declaration = w.declaration
			deviceKey = w.deviceKey
			revision = w.revision
			waitCh = getWaitCh()
		})
		if revision != last {
			data, err := declaration.MarshalVT()
			if err != nil {
				return nil, "", 0, errors.Wrap(err, "encode forge worker declaration")
			}
			return data, deviceKey, revision, nil
		}
		select {
		case <-ctx.Done():
			return nil, "", 0, ctx.Err()
		case <-waitCh:
		}
	}
}
