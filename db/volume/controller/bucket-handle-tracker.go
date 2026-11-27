package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
)

// bucketHandleTracker publishes the current resolution of one bucket.
type bucketHandleTracker struct {
	// waitVolume waits for the backing volume.
	waitVolume func(context.Context) (volume.Volume, error)
	// restart repeats bucket resolution through the reference registry.
	restart func()
	// bucketID identifies the tracked bucket.
	bucketID string
	// handleCtr publishes the current bucket handle or resolution error.
	handleCtr *ccontainer.CContainer[*bucketHandle]
}

// execute executes the bucket handle management routine.
func (b *bucketHandleTracker) execute(ctx context.Context) (exErr error) {
	// Clear stale resolution and publish any new failure when this attempt ends.
	b.handleCtr.SetValue(nil)
	defer func() {
		if exErr != nil {
			if exErr == context.Canceled {
				b.handleCtr.SetValue(nil)
			} else {
				b.handleCtr.SetValue(&bucketHandle{bucketID: b.bucketID, err: exErr})
			}
		}
	}()

	// Resolve the backing volume and its current bucket configuration.
	vol, err := b.waitVolume(ctx)
	if err != nil {
		return err
	}

	// Load the bucket configuration before constructing its handle.
	bc, err := vol.GetBucketConfig(ctx, b.bucketID)
	if err != nil {
		return err
	}

	// Publish the handle, which retains the resolved configuration
	// independently of later updates.
	b.handleCtr.SetValue(&bucketHandle{
		bucketID:   b.bucketID,
		v:          vol,
		bucketConf: bc,
	})
	return nil
}

// updateBucketConfig overrides the bucket config in the current handle.
//
// A nil configuration clears the handle and restarts resolution.
// An existing handle receives the configuration. Otherwise resolution restarts.
func (b *bucketHandleTracker) updateBucketConfig(conf *bucket.Config) *bucketHandle {
	// Restart resolution when the caller invalidates the configuration.
	if conf == nil {
		b.handleCtr.SetValue(nil)
		b.restart()
		return nil
	}

	// Update a resolved handle without mutating its previous snapshot.
	conf = conf.CloneVT()
	handle := b.handleCtr.SwapValue(func(val *bucketHandle) *bucketHandle {
		return val.withConfig(conf)
	})
	if handle != nil {
		return handle
	}
	b.restart()
	return nil
}

// wait waits for a resolved bucket and reports its construction failure.
func (b *bucketHandleTracker) wait(ctx context.Context) (*bucketHandle, error) {
	// Wait for a resolution before inspecting its construction result.
	h, err := b.handleCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}

	// Return the construction failure with the same tracker reference lifetime.
	if h.err != nil {
		return nil, h.err
	}
	return h, nil
}

// publish emits each changed bucket resolution until the directive ends.
func (b *bucketHandleTracker) publish(ctx context.Context, handler directive.ResolverHandler) error {
	// Retain the last published handle while waiting for its replacement.
	var handle *bucketHandle
	for {
		// Wait for the next resolution, including a cleared or failed handle.
		var err error
		handle, err = b.handleCtr.WaitValueChange(ctx, handle, nil)
		if err != nil {
			return err
		}

		// Replace the previous directive value with the current resolution.
		handler.ClearValues()
		if handle != nil {
			_, _ = handler.AddValue(handle)
		}
	}
}
