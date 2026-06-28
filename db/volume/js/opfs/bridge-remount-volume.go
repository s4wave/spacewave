//go:build js

package volume_opfs

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume"
)

// bridgeRemountVolume wraps an OPFS volume so a worker-bridge port swap forces a
// remount.
//
// After a swap the OPFS worker is fresh with an empty handle id space, so every
// cached directory and file handle from the root down is stale. The volume
// cannot reopen them in place because it does not own the directory chain, so
// Execute ends with a transient error when the bridge swaps. The volume
// controller turns that into a remount, which reconstructs the whole handle tree
// from a fresh GetRoot over the new port. With the local browser driver there is
// no bridge to swap, so the swap waiter blocks for the volume lifetime and this
// wrapper just tracks the underlying volume.
type bridgeRemountVolume struct {
	*Opfs
	// executeStore runs the underlying store's background task. A field so the
	// remount lifecycle can be exercised without a browser bridge.
	executeStore func(context.Context) error
	// waitSwap blocks until the OPFS bridge port is swapped for a fresh worker.
	// A field for the same reason as executeStore.
	waitSwap func(context.Context) error
}

// newBridgeRemountVolume wraps an OPFS volume with bridge-swap remount handling.
func newBridgeRemountVolume(vol *Opfs) *bridgeRemountVolume {
	return &bridgeRemountVolume{
		Opfs:         vol,
		executeStore: vol.Execute,
		waitSwap:     opfs.WaitBridgeSwap,
	}
}

// Execute runs the volume until its store execution fails, the context is
// canceled, or the OPFS bridge port is swapped for a fresh worker.
//
// The OPFS store has no background task: its Execute returns nil immediately. A
// nil store return is therefore not volume shutdown, so Execute keeps the volume
// mounted and waits for a swap or context cancellation. Only a non-nil store
// error or a bridge swap ends Execute and drives the controller's remount.
func (v *bridgeRemountVolume) Execute(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	storeErr := make(chan error, 1)
	go func() { storeErr <- v.executeStore(ctx) }()
	swapErr := make(chan error, 1)
	go func() { swapErr <- v.waitSwap(ctx) }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-storeErr:
			if err != nil {
				return err
			}
			storeErr = nil
		case err := <-swapErr:
			if err != nil {
				return err
			}
			return errors.New("opfs bridge port swapped; remounting to rebuild stale handles")
		}
	}
}

// _ is a type assertion.
var _ volume.Volume = (*bridgeRemountVolume)(nil)
