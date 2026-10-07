//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
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

		mount := func(ctx context.Context, record *deviceSetupRecord) (world.Engine, func(), error) {
			return mountDeviceWorld(ctx, client, record)
		}
		if err := runFlowgraphReconciler(ctx, le, statePath, b, mount, store); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("flowgraph reconciler stopped")
		}
	}()
}

// runFlowgraphReconciler runs the reconciler for the Device this daemon set
// up. It waits for the Device setup to become session-ready, since the Device
// object does not exist before then. mount opens the World of the Space that
// holds the Device.
func runFlowgraphReconciler(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	mount func(context.Context, *deviceSetupRecord) (world.Engine, func(), error),
	store *device_policy.PolicyStore,
) error {
	// Find the Device this daemon set up.
	record, err := waitDeviceSetupReady(ctx, le, statePath)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	// Mount the Space's World.
	engine, release, err := mount(ctx, record)
	if err != nil {
		return err
	}
	defer release()

	// Reconcile until the daemon stops.
	return device_flowgraph.NewReconciler(le, b, engine, store, record.DeviceObjectKey, record.PeerID).Run(ctx)
}

// waitDeviceSetupReady returns the setup record once the Device setup is
// session-ready. It reads the record again after each change to the setup
// state directory, which the setup commands write.
func waitDeviceSetupReady(ctx context.Context, le *logrus.Entry, statePath string) (*deviceSetupRecord, error) {
	// Subscribe before the first read, so a write between them is not missed.
	// The directory may not exist before setup writes its first record.
	dir := filepath.Dir(deviceSetupRecordPath(statePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create device setup state directory")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(err, "watch device setup state")
	}
	defer watcher.Close()
	if err := watcher.Add(dir); err != nil {
		return nil, errors.Wrap(err, "watch device setup state")
	}

	// Read the record after each change until it is ready.
	for {
		record, ok, err := deviceLauncherProjectionTarget(statePath)
		switch {
		case err == nil && ok:
			return record, nil
		case err != nil:
			// A write in progress reads as a partial record. Its completion is
			// the next change.
			le.WithError(err).Debug("device setup record not readable yet")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-watcher.Errors:
			return nil, errors.Wrap(err, "watch device setup state")
		case <-watcher.Events:
		}
	}
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
