//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	device_flowgraph "github.com/s4wave/spacewave/core/device/flowgraph"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// startFlowgraphReconciler runs the Flowgraph nodes placed on the Device of
// the setup Space for the life of ctx.
func startFlowgraphReconciler(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	invoker srpc.Invoker,
	store *device_policy.PolicyStore,
) {
	// Skip a daemon that runs without the services the reconciler reads.
	if b == nil || invoker == nil || store == nil {
		return
	}

	// Run in the background so serving starts without waiting for the Space.
	go func() {
		// Connect the SDK client the reconciler reads the Space through.
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("flowgraph reconciler unavailable")
			}
			return
		}
		defer client.close()

		if err := runFlowgraphReconciler(ctx, le, statePath, b, client, store); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("flowgraph reconciler stopped")
		}
	}()
}

// runFlowgraphReconciler runs the reconciler for the Device this daemon set
// up. It returns when the setup is not ready, since the Device object does not
// exist yet.
func runFlowgraphReconciler(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	client *sdkClient,
	store *device_policy.PolicyStore,
) error {
	// Find the Device this daemon set up.
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil || !ok {
		return err
	}

	// Mount the Space's World.
	engine, release, err := mountDeviceWorld(ctx, client, record)
	if err != nil {
		return err
	}
	defer release()

	// Reconcile until the daemon stops.
	return device_flowgraph.NewReconciler(le, b, engine, store, record.DeviceObjectKey, record.PeerID).Run(ctx)
}

// mountDeviceWorld mounts the World of the Space that holds the Device of
// record. Call release when done with the engine.
func mountDeviceWorld(ctx context.Context, client *sdkClient, record *deviceSetupRecord) (world.Engine, func(), error) {
	// Decode the Space the Device object lives in.
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		return nil, nil, err
	}

	// Mount the session that owns the Space, then the Space and its World.
	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return nil, nil, err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		sess.Release()
		return nil, nil, err
	}
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		return nil, nil, err
	}
	return engine, func() {
		engineCleanup()
		spaceCleanup()
		sess.Release()
	}, nil
}
