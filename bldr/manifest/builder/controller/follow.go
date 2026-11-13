//go:build !js

package bldr_manifest_builder_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
)

// FollowBuilder runs the builder controller for conf and calls onResult with
// each build it finishes, built or failed, in order, until ctx ends or
// onResult returns an error, which FollowBuilder returns.
//
// A build that a newer change cancels publishes nothing: the builder replaces
// its result promise and FollowBuilder waits on the new one. When the
// controller's Execute fails, the loader replaces the instance, and
// FollowBuilder follows the replacement, calling onStart with each instance
// before its first result.
func FollowBuilder(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
	onStart func(*Controller),
	onResult func(*bldr_manifest_builder.BuilderResult, error) error,
) error {
	for {
		// Follow one instance until the loader disposes of it.
		err := followBuilderInstance(ctx, b, conf, onStart, onResult)
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err != nil {
			return err
		}
	}
}

// followBuilderInstance follows one running builder controller instance. It
// returns nil when the loader disposes of the instance while ctx is live.
func followBuilderInstance(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
	onStart func(*Controller),
	onResult func(*bldr_manifest_builder.BuilderResult, error) error,
) error {
	// Wait for a running instance, ending instCtx when it is disposed.
	instCtx, instCancel := context.WithCancel(ctx)
	defer instCancel()
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*Controller](
		instCtx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		instCancel,
	)
	if err != nil {
		if instCtx.Err() != nil {
			return nil
		}
		return err
	}
	defer ctrlRef.Release()
	if onStart != nil {
		onStart(ctrl)
	}

	// Await each result promise, moving to its replacement when a newer
	// change cancels the build before it resolves.
	results := ctrl.GetResultPromise()
	for {
		prom, changed := results.GetPromise()
		if prom != nil {
			result, err := prom.AwaitWithCancelCh(instCtx, changed)
			if instCtx.Err() != nil {
				return nil
			}
			select {
			case <-changed:
				// Replaced before or as it resolved: the newer build decides.
				continue
			default:
			}
			if err := onResult(result, err); err != nil {
				return err
			}
		}

		// Wait for the next build's promise.
		select {
		case <-instCtx.Done():
			return nil
		case <-changed:
		}
	}
}
