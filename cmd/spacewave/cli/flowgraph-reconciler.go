//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	device_flowgraph "github.com/s4wave/spacewave/core/device/flowgraph"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// startFlowgraphReconciler runs the Flowgraph nodes placed on this Device in
// every Space it serves, for the life of ctx.
func startFlowgraphReconciler(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	invoker srpc.Invoker,
	store *device_policy.PolicyStore,
	forgeWorker *device_flowgraph.ForgeWorkerWatch,
	idleTracker *daemonIdleTracker,
) {
	// Skip a daemon that runs without the services the reconciler reads.
	if b == nil || invoker == nil || store == nil {
		return
	}

	// Run in the background so serving starts without waiting for a Space.
	go func() {
		// Connect the SDK client the reconcilers read the Spaces through.
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("flowgraph reconciler unavailable")
			}
			return
		}
		defer client.close()

		// Mount each served Space, loading its plugins on demand.
		mount := func(ctx context.Context, target flowgraphTarget) (*servedWorld, error) {
			return mountServedWorld(ctx, le, b, client, target)
		}

		// Keep the daemon alive while a node runs, since nodes serve with no
		// client attached.
		hold := func() func() { return idleTracker.serviceAttached("Flowgraph nodes") }
		if err := runFlowgraphReconciler(ctx, le, statePath, b, mount, store, forgeWorker, hold); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("flowgraph reconciler stopped")
		}
	}()
}

// flowgraphTarget is a Space this Device serves and the daemon session it
// reaches the Space through. That session's peer is this Device's peer in the
// Space.
type flowgraphTarget struct {
	// sessionIndex is the daemon's index of the session.
	sessionIndex uint32
	// spaceID is the shared object ID of the Space.
	spaceID string
}

// servedWorld is the mounted World of a served Space.
type servedWorld struct {
	// engine is the Space's World engine.
	engine world.Engine
	// engineID is the ID of engine on the daemon's bus.
	engineID string
	// peerID is the peer of the session the Space is mounted through.
	peerID string
	// release unmounts the World.
	release func()
}

// mountServedWorldFunc mounts the World of a served Space.
type mountServedWorldFunc func(ctx context.Context, target flowgraphTarget) (*servedWorld, error)

// flowgraphRetryBackoff spaces the restarts of a Space's reconciler after its
// mount or run fails.
var flowgraphRetryBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{InitialInterval: 1000, MaxInterval: 30000, Multiplier: 2},
}

// runFlowgraphReconciler runs one Reconciler for each Space this Device
// serves: the Space its setup linked, once the setup is session-ready, and
// each Space the DevicePolicy lists as served. It follows the setup record and
// the policy, starting and stopping Reconcilers as Spaces join and leave the
// set. Each Space's Reconciler mounts and retries on its own, so one Space's
// failure leaves the others running.
func runFlowgraphReconciler(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	mount mountServedWorldFunc,
	store *device_policy.PolicyStore,
	forgeWorker *device_flowgraph.ForgeWorkerWatch,
	hold func() func(),
) error {
	// Label a Device object the Reconcilers write as setup labels it.
	label := defaultDeviceSetupLabel()
	if setup, err := readDeviceSetupRecord(statePath); err == nil && setup.Label != "" {
		label = setup.Label
	}

	// Run each served Space's Reconciler as a keyed routine.
	reconcilers := keyed.NewKeyedWithLogger(func(target flowgraphTarget) (keyed.Routine, struct{}) {
		return func(ctx context.Context) error {
			return runServedSpace(ctx, le.WithField("space-id", target.spaceID), b, mount, target, label, store, forgeWorker, hold)
		}, struct{}{}
	}, le, keyed.WithRetry[flowgraphTarget, struct{}](flowgraphRetryBackoff))
	reconcilers.SetContext(ctx, true)
	defer reconcilers.ClearContext()

	// Follow the setup record and the policy, which together select the
	// Spaces.
	targets := &flowgraphTargets{reconcilers: reconcilers}
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error { return watchDeviceSetupTarget(egCtx, le, statePath, targets.setSetup) })
	eg.Go(func() error { return watchServedSpaces(egCtx, store, targets.setServed) })
	if err := eg.Wait(); ctx.Err() == nil {
		return err
	}
	return nil
}

// flowgraphTargets holds the two sources of the served Space set and keeps
// the keyed Reconcilers in step with their union.
type flowgraphTargets struct {
	// reconcilers runs a Reconciler for each target.
	reconcilers *keyed.Keyed[flowgraphTarget, struct{}]

	// mtx guards the fields below and orders the syncs.
	mtx sync.Mutex
	// setup is the setup Space, or nil before the setup is session-ready.
	setup *flowgraphTarget
	// served is the policy's list of served Spaces.
	served []*device_policy.ServedSpace
}

// setSetup replaces the setup Space.
func (t *flowgraphTargets) setSetup(setup *flowgraphTarget) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.setup = setup
	t.syncLocked()
}

// setServed replaces the policy's served Spaces.
func (t *flowgraphTargets) setServed(served []*device_policy.ServedSpace) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.served = served
	t.syncLocked()
}

// syncLocked starts a Reconciler for each new target and stops each removed
// one, leaving the rest running. Each Space gets one Reconciler, since two
// would apply the same ConfigSet keys. The setup Space keeps its setup
// session when the policy also lists it.
func (t *flowgraphTargets) syncLocked() {
	keys := make([]flowgraphTarget, 0, len(t.served)+1)
	if t.setup != nil {
		keys = append(keys, *t.setup)
	}
	for _, served := range t.served {
		if t.setup != nil && served.GetSpaceId() == t.setup.spaceID {
			continue
		}
		keys = append(keys, flowgraphTarget{sessionIndex: served.GetSessionIndex(), spaceID: served.GetSpaceId()})
	}
	t.reconcilers.SyncKeys(keys, false)
}

// runServedSpace mounts the World of target and reconciles this Device's
// nodes in it until ctx ends. label labels a Device object the function writes.
func runServedSpace(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	mount mountServedWorldFunc,
	target flowgraphTarget,
	label string,
	store *device_policy.PolicyStore,
	forgeWorker *device_flowgraph.ForgeWorkerWatch,
	hold func() func(),
) error {
	// Mount the Space through its session.
	served, err := mount(ctx, target)
	if err != nil {
		return err
	}
	defer served.release()

	// Name this Device by the session peer, as enrollment does, and write its
	// object when the Space lacks one, so the Space's graph can place nodes
	// on it.
	deviceKey, err := ensureServedDeviceObject(ctx, served, label)
	if err != nil {
		return err
	}

	// Reconcile until the Space leaves the set or the daemon stops.
	return device_flowgraph.NewReconciler(le, b, served.engine, served.engineID, target.spaceID, store, forgeWorker, deviceKey, served.peerID, hold).Run(ctx)
}

// ensureServedDeviceObject returns the key of this Device's object in the
// served World, first writing the object, labeled label, when the World has
// none. An object that exists is left as its writers keep it.
func ensureServedDeviceObject(ctx context.Context, served *servedWorld, label string) (string, error) {
	// Look the Device object up in a read transaction.
	deviceKey, err := deviceObjectKey(served.peerID)
	if err != nil {
		return "", err
	}
	tx, err := served.engine.NewTransaction(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "new transaction")
	}
	obj, found, err := tx.GetObject(ctx, deviceKey)
	world.ReleaseObjectState(obj)
	tx.Discard()
	if err != nil || found {
		return deviceKey, err
	}

	// Write the object as a session-ready Device.
	record := &deviceSetupRecord{SetupState: deviceSetupStateSessionReady, PeerID: served.peerID, Label: label}
	if _, err := upsertLinkedDeviceObjectInWorld(ctx, served.engine, record, time.Now(), nil); err != nil {
		return "", errors.Wrap(err, "write device object")
	}
	return deviceKey, nil
}

// watchDeviceSetupTarget calls set with the setup Space after each change to
// the setup state directory, which the setup commands write, or with nil while
// the setup is not session-ready.
func watchDeviceSetupTarget(ctx context.Context, le *logrus.Entry, statePath string, set func(*flowgraphTarget)) error {
	// Subscribe before the first read, so a write between them is not missed.
	// The directory may not exist before setup writes its first record.
	dir := filepath.Dir(deviceSetupRecordPath(statePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.Wrap(err, "create device setup state directory")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "watch device setup state")
	}
	defer watcher.Close()
	if err := watcher.Add(dir); err != nil {
		return errors.Wrap(err, "watch device setup state")
	}

	// Read the record after each change.
	for {
		record, ok, err := deviceUpdateProjectionTarget(statePath)
		switch {
		case err != nil:
			// A write in progress reads as a partial record. Its completion is
			// the next change, so keep the current target until then.
			le.WithError(err).Debug("device setup record not readable yet")
		case !ok:
			set(nil)
		default:
			spaceID, err := decodeDeviceResourceID(record.ResourceID)
			if err != nil {
				return err
			}
			set(&flowgraphTarget{sessionIndex: record.SessionIndex, spaceID: spaceID})
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-watcher.Errors:
			return errors.Wrap(err, "watch device setup state")
		case <-watcher.Events:
		}
	}
}

// watchServedSpaces calls set with the policy's served Spaces, then again
// after each policy change.
func watchServedSpaces(ctx context.Context, store *device_policy.PolicyStore, set func([]*device_policy.ServedSpace)) error {
	var last *device_policy.DevicePolicy
	for {
		policy, err := store.WaitChange(ctx, last)
		if err != nil {
			return err
		}
		last = policy
		set(policy.GetServedSpace())
	}
}

// mountServedWorld mounts the World of target's Space through its session,
// and reads the session's peer and the ID of the World engine. Until release,
// a lookup of a node type on b in this World runs the Space's plugins that
// hint at it.
func mountServedWorld(ctx context.Context, le *logrus.Entry, b bus.Bus, client *sdkClient, target flowgraphTarget) (*servedWorld, error) {
	// Mount the session and read the peer this Device runs as in it.
	sess, err := client.mountSession(ctx, target.sessionIndex)
	if err != nil {
		return nil, err
	}
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		sess.Release()
		return nil, errors.Wrap(err, "get session info")
	}

	// Mount the Space and its World.
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, target.spaceID)
	if err != nil {
		sess.Release()
		return nil, err
	}
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		return nil, err
	}
	release := func() {
		engineCleanup()
		spaceCleanup()
		sess.Release()
	}

	// Read the ID the World engine has on the daemon's bus.
	engineInfo, err := engine.GetEngineInfo(ctx)
	if err != nil {
		release()
		return nil, errors.Wrap(err, "get world engine info")
	}
	engineID := engineInfo.GetEngineInfo().GetEngineId()

	// Start the plugins of the node types the Device looks up in this World.
	releaseLoader, err := b.AddController(ctx, device_flowgraph.NewPluginLoader(le, engineID, func(ctx context.Context, typeID string) error {
		return holdNodeTypePlugins(ctx, spaceSvc, typeID)
	}), nil)
	if err != nil {
		release()
		return nil, err
	}
	releaseWorld := release
	release = func() {
		releaseLoader()
		releaseWorld()
	}
	return &servedWorld{engine: engine, engineID: engineID, peerID: info.GetPeerId(), release: release}, nil
}

// holdNodeTypePlugins runs the Space's plugins that hint at typeID until ctx
// ends or the hold stream fails.
func holdNodeTypePlugins(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient, typeID string) error {
	// Open the hold, which lasts as long as the stream.
	strm, err := spaceSvc.HoldNodeTypePlugins(ctx, &s4wave_space.HoldNodeTypePluginsRequest{NodeTypeIds: []string{typeID}})
	if err != nil {
		return err
	}
	defer strm.Close()

	// Drain the held plugin reports until the stream ends.
	for {
		if _, err := strm.Recv(); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}
