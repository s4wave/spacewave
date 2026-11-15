package plugin_space

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	plugin_objecttype "github.com/s4wave/spacewave/core/plugin/objecttype"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// Readiness test plugin identifiers.
const (
	pluginReadinessPluginID   = "test-readiness-plugin"
	pluginReadinessPlatformID = "test/platform"
	pluginReadinessTypeID     = "test/readiness"
)

func TestReconcileProcessConfigsClearsWhenStoreUnavailable(t *testing.T) {
	// Seed the controller with a stale process configuration.
	c := &Controller{
		processConfigs: map[string]processConfig{
			"stale-process": {typeID: "test/process"},
		},
	}
	c.processes = keyed.NewKeyed(func(key string) (keyed.Routine, processConfig) {
		return nil, c.processConfigs[key]
	})
	c.processes.SyncKeys([]string{"stale-process"}, false)

	// Clear process configurations when their backing store is unavailable.
	c.reconcileProcessConfigs(logrus.NewEntry(logrus.New()), nil)

	// Verify reconciliation removes the stale key and stored process config.
	if got := c.processes.GetKeys(); len(got) != 0 {
		t.Fatalf("process keys after unavailable store = %v, want none", got)
	}
	if got := len(c.processConfigs); got != 0 {
		t.Fatalf("process config count after unavailable store = %d, want 0", got)
	}
}

func TestProcessResolvesObjectTypeAfterPluginReload(t *testing.T) {
	// Run the process against a real bus and plugin type registration.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	c := &Controller{
		BusController: bus.NewBusController(le, b, &Config{}, ControllerID, Version, controllerDescrip),
	}

	// Observe each factory's lifetime as the type registration changes.
	started := make(chan context.Context, 2)
	ot := objecttype.NewObjectType("test/reload-process", func(
		ctx context.Context,
		_ *logrus.Entry,
		_ bus.Bus,
		_ world.Engine,
		_ world.WorldState,
		_ string,
	) (srpc.Invoker, func(), error) {
		// Hold the factory open until its type is withdrawn or the lookup stops.
		started <- ctx
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	register := func() func() {
		// Publish the object type with the same controller used by loaded plugins.
		t.Helper()
		release, err := b.AddController(ctx, plugin_objecttype.NewController(
			map[string]objecttype.ObjectType{ot.GetObjectTypeID(): ot},
		), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		return release
	}
	release := register()

	// Start the keyed routine without a retry policy hiding a lost lookup.
	run, stop := context.WithCancel(ctx)
	defer stop()
	exited := make(chan error, 1)
	go func() {
		exited <- c.runProcess(run, nil, "test/process", ot.GetObjectTypeID())
	}()
	waitStarted := func() context.Context {
		t.Helper()
		select {
		case processCtx := <-started:
			return processCtx
		case err := <-exited:
			t.Fatalf("process exited instead of waiting for its type: %v", err)
		case <-ctx.Done():
			t.Fatal("process did not resolve its object type")
		}
		return nil
	}
	first := waitStarted()

	// Removing the controller withdraws its directive value and stops the process.
	release()
	select {
	case <-first.Done():
	case <-ctx.Done():
		t.Fatal("process kept using the withdrawn object type")
	}

	// Restoring the same type resumes the standing lookup without a keyed retry.
	register()
	second := waitStarted()
	if second == first {
		t.Fatal("restored object type reused the canceled process")
	}

	// Cancellation ends the lookup and waits for the active factory to stop.
	stop()
	select {
	case err := <-exited:
		if err != context.Canceled {
			t.Fatalf("process exit = %v, want context canceled", err)
		}
	case <-ctx.Done():
		t.Fatal("process did not stop after cancellation")
	}
	if second.Err() == nil {
		t.Fatal("process returned before canceling the restored factory")
	}
}

func TestLookupObjectTypeWaitsForDesiredPluginRegistration(t *testing.T) {
	// Start a testbed with enough time for the plugin registration lifecycle.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// The Space approves one plugin whose Manifest is stored in the Space World.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{pluginReadinessPluginID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	storeKey := "test/plugin-manifests"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, storeKey); err != nil {
		t.Fatal(err)
	}
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		tb.WorldState,
		tb.Volume.GetPeerID(),
		"test/manifests/readiness",
		[]string{storeKey},
		createReadinessManifest(t, ctx, tb),
	); err != nil {
		t.Fatal(err)
	}

	// The manifest source serves nothing, so the plugin must resolve its
	// Manifest from the Space World.
	// Register a source that leaves the Space World as the manifest provider.
	source, _, err := controllerbus_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}

	// Wire the manifest source into the resolver and install the readiness loader.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus, WithManifestSource(source)))
	loader := newPluginReadinessLoadController(tb.Bus)
	releaseLoader, err := tb.Bus.AddController(ctx, loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoader()
	_, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer spaceRef.Release()

	// Wait for the approved plugin to begin loading and resolve its Manifest.
	select {
	case <-loader.loadStarted:
	case <-ctx.Done():
		t.Fatal("Space settings did not start the desired plugin")
	}
	select {
	case <-loader.manifestResolved:
	case <-ctx.Done():
		t.Fatal("desired plugin did not resolve the imported Manifest")
	}

	// A typed lookup waits until the desired plugin registers its type.
	type lookupResult struct {
		objectType objecttype.ObjectType
		ref        directive.Reference
		err        error
	}

	// Start a typed lookup while the desired plugin is still registering.
	lookupCtx := objecttype.WithEngineID(ctx, tb.EngineID)
	lookupResultCh := make(chan lookupResult, 1)
	go func() {
		ot, ref, err := objecttype.ExLookupObjectType(lookupCtx, tb.Bus, pluginReadinessTypeID)
		lookupResultCh <- lookupResult{objectType: ot, ref: ref, err: err}
	}()
	select {
	case result := <-lookupResultCh:
		if result.ref != nil {
			result.ref.Release()
		}
		t.Fatalf("typed lookup completed before registration: objectType=%v err=%v", result.objectType, result.err)
	case <-loader.lookupObserved:
	}

	// Allow registration and wait for the typed lookup result.
	close(loader.allowRegistration)
	var result lookupResult
	select {
	case result = <-lookupResultCh:
	case <-ctx.Done():
		t.Fatal("typed lookup did not resolve after registration")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.objectType == nil || result.objectType.GetObjectTypeID() != pluginReadinessTypeID {
		t.Fatalf("typed lookup returned %v", result.objectType)
	}
	if result.ref == nil {
		t.Fatal("typed lookup returned no reference")
	}
	result.ref.Release()

	// Once every desired plugin is loaded, an unknown type resolves to nothing.
	unknown, unknownRef, err := objecttype.ExLookupObjectType(lookupCtx, tb.Bus, "test/unknown-readiness")
	if unknownRef != nil {
		unknownRef.Release()
	}
	if err != nil {
		t.Fatal(err)
	}
	if unknown != nil {
		t.Fatalf("unknown typed lookup returned %v", unknown)
	}
}

// createReadinessManifest retains the readiness plugin Manifest in the test
// World's bucket.
func createReadinessManifest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
) *bldr_manifest.ManifestRef {
	t.Helper()
	return createReadinessManifestRev(t, ctx, tb, 1)
}

// createReadinessManifestRev writes revision rev of the readiness manifest.
func createReadinessManifestRev(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	rev uint64,
) *bldr_manifest.ManifestRef {
	// Describe the readiness manifest.
	t.Helper()
	meta := bldr_manifest.NewManifestMeta(
		pluginReadinessPluginID,
		bldr_manifest.BuildType_DEV,
		pluginReadinessPlatformID,
		rev,
	)

	// Stage the manifest until the test ends so the caller can adopt it.
	stage, err := tb.Engine.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stage.Release)
	var manifestRef *bldr_manifest.ManifestRef
	if err := stage.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Write the manifest block.
		transaction, blocks := cursor.BuildTransactionAtRef(nil, nil)
		blocks.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		rootRef, _, err := transaction.Write(ctx, true)
		if err != nil {
			return err
		}

		// Point the manifest ref at the written root.
		objectRef := cursor.GetRef().CloneVT()
		objectRef.RootRef = rootRef
		manifestRef = bldr_manifest.NewManifestRef(meta, objectRef)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return manifestRef
}

// TestSharedSpaceApprovalObjectChangesNothing checks that a member who writes
// an approval-shaped object into the shared Space admits nothing: only the
// device-local decision admits a listed plugin's root.
func TestSharedSpaceApprovalObjectChangesNothing(t *testing.T) {
	// Start a testbed with the device's approvals on the bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	approvals := plugin_approval.NewApprovals(tb.Bus, tb.EngineVolumeID, "test-plugin-approvals")
	releaseApprovals, err := tb.Bus.AddController(ctx, plugin_approval.NewController(tb.Logger, approvals), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseApprovals()
	const pluginID, root = "shared-plugin", "shared-root"
	peerID := tb.Volume.GetPeerID().String()

	// The Space lists the plugin, and a member writes an ALLOWED record for
	// this peer into the Space World at the device's own key.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{pluginID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	forged, err := (&s4wave_plugin_approval.PluginApproval{
		PluginId:     pluginID,
		ManifestRoot: root,
		State:        s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	objKey := string(plugin_approval.Key(peerID, pluginID, root))
	forgedObj, _, err := world.CreateWorldObject(ctx, tb.WorldState, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(byteslice.NewByteSlice(&forged), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(forgedObj)

	// Run the Space controller with a manifest source that serves nothing.
	source, _, err := controllerbus_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus, WithManifestSource(source)))
	_, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: peerID,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer spaceRef.Release()

	// The root stays unadmitted while the forged object sits in the Space.
	counts := make(chan int, 16)
	_, releaseWatch, err := bus.ExecCollectValuesWatch[bldr_plugin.AdmitPluginRootValue](
		ctx,
		tb.Bus,
		bldr_plugin.NewAdmitPluginRoot(peerID, pluginID, root),
		true,
		func(_ []error, vals []bldr_plugin.AdmitPluginRootValue) error {
			counts <- len(vals)
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseWatch()
	waitCount := func(want int) {
		t.Helper()
		for {
			select {
			case got := <-counts:
				if got == want {
					return
				}
				t.Logf("admitting values: %d", got)
			case <-ctx.Done():
				t.Fatalf("admitting values never reached %d", want)
			}
		}
	}
	waitCount(0)
	if ok, err := approvals.Admits(ctx, peerID, pluginID, root); ok || err != nil {
		t.Fatalf("forged Space object: admitted %v, error %v", ok, err)
	}

	// The device's own decision admits the same root through the same watch.
	if err := approvals.Set(ctx, peerID, &s4wave_plugin_approval.PluginApproval{
		PluginId:     pluginID,
		ManifestRoot: root,
		State:        s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
	}); err != nil {
		t.Fatal(err)
	}
	waitCount(1)
}

// TestAdmissionGatePendingUntilAnswered checks that the gate reports a root
// pending until its admission directive answers, so the first pass does not
// load a fallback it would swap out a moment later.
func TestAdmissionGatePendingUntilAnswered(t *testing.T) {
	// Answer admission for the slow root only once answer is closed.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	slow, unknown := testRootRef(t, "slow"), testRootRef(t, "unknown")
	answer := make(chan struct{})
	approver := callback.NewCallbackController(
		controller.NewInfo("test/slow-approvals", controller.MustParseVersion("0.0.1"), ""),
		nil,
		func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			d, ok := inst.GetDirective().(bldr_plugin.AdmitPluginRoot)
			if !ok || d.AdmitPluginRootManifestRoot() != manifestRoot(slow) {
				return nil, nil
			}
			return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
				select {
				case <-answer:
				case <-ctx.Done():
					return ctx.Err()
				}
				_, _ = handler.AddValue(bldr_plugin.AdmitSourceApproved)
				return nil
			}), nil)
		},
		nil,
	)
	release, err := b.AddController(ctx, approver, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	woken := make(chan struct{}, 16)
	gate := newAdmissionGate(b, "peer", func() { woken <- struct{}{} })
	defer gate.release()

	// waitFor wakes until the gate's verdict on refs matches.
	waitFor := func(refs []*bldr_manifest.ManifestRef, wantAdmitted int, wantPending bool) {
		t.Helper()
		for {
			admitted, open := gate.admitted(ctx, "colors", refs)
			if len(admitted) == wantAdmitted && (len(open) != len(admitted)) == wantPending {
				return
			}
			select {
			case <-woken:
			case <-ctx.Done():
				t.Fatalf("gate never reported %d admitted, pending %v", wantAdmitted, wantPending)
			}
		}
	}

	// A root no resolver answers settles as refused; the slow one stays
	// pending until its resolver answers, then reads as admitted.
	waitFor([]*bldr_manifest.ManifestRef{unknown}, 0, false)
	if admitted, open := gate.admitted(ctx, "colors", []*bldr_manifest.ManifestRef{slow}); len(admitted) != 0 || len(open) != 1 {
		t.Fatalf("unanswered root: %d admitted, %d open, want pending", len(admitted), len(open))
	}
	close(answer)
	waitFor([]*bldr_manifest.ManifestRef{slow}, 1, false)
}

// testRootRef returns a manifest ref whose root is the hash of name.
func testRootRef(t *testing.T, name string) *bldr_manifest.ManifestRef {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(name))
	if err != nil {
		t.Fatal(err)
	}
	return &bldr_manifest.ManifestRef{ManifestRef: &bucket.ObjectRef{RootRef: &block.BlockRef{Hash: h}}}
}

// testRunningPlugin is a running plugin with no RPC client.
type testRunningPlugin struct{}

// GetRpcClient returns nil.
func (testRunningPlugin) GetRpcClient() srpc.Client { return nil }

// TestRevokeWhilePendingNamesTheAwaitedRoot checks that a plugin whose running
// root loses its admission while a newer root's admission has not answered
// stops reading as running and is loaded naming the newer root, so the host
// says which root awaits approval instead of the Space showing the stopped
// load.
func TestRevokeWhilePendingNamesTheAwaitedRoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	const pluginID = "colors"
	oldRef, newRef := testRootRef(t, "old"), testRootRef(t, "new")

	// Approvals admit the old root until revoke closes and never answer for
	// the new one. A load that names only the old root runs; any other load
	// waits, as a host awaiting approval does.
	revoke := make(chan struct{})
	resolver := func(fn func(context.Context, directive.ResolverHandler) error) ([]directive.Resolver, error) {
		return directive.R(directive.NewFuncResolver(fn), nil)
	}
	hold := func(ctx context.Context, _ directive.ResolverHandler) error {
		<-ctx.Done()
		return ctx.Err()
	}
	host := callback.NewCallbackController(
		controller.NewInfo("test/approvals-and-host", controller.MustParseVersion("0.0.1"), ""),
		nil,
		func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			switch d := inst.GetDirective().(type) {
			case bldr_plugin.AdmitPluginRoot:
				if d.AdmitPluginRootManifestRoot() != manifestRoot(oldRef) {
					return resolver(hold)
				}
				return resolver(func(ctx context.Context, handler directive.ResolverHandler) error {
					id, _ := handler.AddValue(bldr_plugin.AdmitSourceApproved)
					select {
					case <-revoke:
						handler.RemoveValue(id)
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			case bldr_plugin.LoadPlugin:
				refs := d.LoadPluginManifests()
				if len(refs) != 1 || manifestRoot(refs[0]) != manifestRoot(oldRef) {
					return resolver(hold)
				}
				return resolver(func(_ context.Context, handler directive.ResolverHandler) error {
					_, _ = handler.AddValue(bldr_plugin.RunningPlugin(testRunningPlugin{}))
					return nil
				})
			}
			return nil, nil
		},
		nil,
	)
	releaseHost, err := tb.Bus.AddController(ctx, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHost()

	// The Space pins the plugin to an installation the test resolves.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{
			PluginIds:           []string{pluginID},
			PluginInstallations: map[string]*space_world.SpacePluginInstallation{pluginID: {ManifestKeys: []string{"install"}}},
		},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	c := &Controller{BusController: bus.NewBusController(tb.Logger, tb.Bus, &Config{
		SpaceId:       "space-test",
		SessionPeerId: "peer",
	}, ControllerID, Version, controllerDescrip)}
	woken := make(chan struct{}, 64)
	loads := &pluginLoads{
		refs:   make(map[string]pluginReference),
		pinned: make(map[string]pinnedManifests),
		gate:   newAdmissionGate(tb.Bus, "peer", func() { woken <- struct{}{} }),
	}
	defer loads.gate.release()
	pin := func(refs ...*bldr_manifest.ManifestRef) {
		loads.pinned[pluginID] = pinnedManifests{keys: []string{"install"}, refs: refs}
	}

	// settle reconciles on each wake until done reports true.
	settle := func(what string, done func() bool) {
		t.Helper()
		for {
			c.reconcilePlugins(ctx, tb.WorldState, loads)
			_, loadedCh := c.loadedPlugins.GetAndWaitCh()
			if done() {
				return
			}
			select {
			case <-woken:
			case <-loadedCh:
			case <-ctx.Done():
				t.Fatalf("never reached: %s", what)
			}
		}
	}
	running := func() bool {
		ids, _ := c.loadedPlugins.GetAndWaitCh()
		return slices.Contains(ids, pluginID)
	}
	names := func(want *bldr_manifest.ManifestRef) bool {
		ms := loads.refs[pluginID].manifests
		return len(ms) == 1 && manifestRoot(ms[0]) == manifestRoot(want)
	}

	// The old root runs; a newer root whose admission has not answered
	// leaves it running.
	pin(oldRef)
	settle("old root running", func() bool { return running() && names(oldRef) })
	pin(newRef, oldRef)
	c.reconcilePlugins(ctx, tb.WorldState, loads)
	if !running() || !names(oldRef) {
		t.Fatalf("pending newer root: running %v, load names old %v", running(), names(oldRef))
	}

	// Revoking the old root stops it reading as running, and the load names
	// the root that awaits approval.
	close(revoke)
	settle("load naming the awaited root", func() bool { return names(newRef) })
	if running() {
		t.Fatal("revoked plugin still reads as running")
	}
	if pending, _ := c.loadedPlugins.HasPendingAndWaitCh(); !pending {
		t.Fatal("plugin awaiting approval reads as settled")
	}
}

// TestPendingRootLoadNamesPeer checks that a Space with no session peer never
// sends a load naming a root whose admission has not answered: such a load is
// only sent to a host that asks for each root, so it must name the peer.
func TestPendingRootLoadNamesPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	const pluginID = "colors"
	ref := testRootRef(t, "pending")

	// Approvals never answer; record each load that names a root.
	hold := func(ctx context.Context, _ directive.ResolverHandler) error {
		<-ctx.Done()
		return ctx.Err()
	}
	sent := make(chan string, 16)
	host := callback.NewCallbackController(
		controller.NewInfo("test/pending-approvals", controller.MustParseVersion("0.0.1"), ""),
		nil,
		func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			switch d := inst.GetDirective().(type) {
			case bldr_plugin.AdmitPluginRoot:
				return directive.R(directive.NewFuncResolver(hold), nil)
			case bldr_plugin.LoadPlugin:
				if len(d.LoadPluginManifests()) != 0 {
					sent <- d.LoadPluginPeerID()
				}
				return directive.R(directive.NewFuncResolver(hold), nil)
			}
			return nil, nil
		},
		nil,
	)
	releaseHost, err := tb.Bus.AddController(ctx, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHost()

	// The Space pins the plugin to a root, with no session peer.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{
			PluginIds:           []string{pluginID},
			PluginInstallations: map[string]*space_world.SpacePluginInstallation{pluginID: {ManifestKeys: []string{"install"}}},
		},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	c := &Controller{BusController: bus.NewBusController(tb.Logger, tb.Bus, &Config{
		SpaceId: "space-test",
	}, ControllerID, Version, controllerDescrip)}
	loads := &pluginLoads{
		refs:   make(map[string]pluginReference),
		pinned: map[string]pinnedManifests{pluginID: {keys: []string{"install"}, refs: []*bldr_manifest.ManifestRef{ref}}},
		gate:   newAdmissionGate(tb.Bus, c.GetConfig().GetSessionPeerId(), func() {}),
	}
	defer loads.gate.release()

	// With no peer the root is refused at once: the pinned plugin is withdrawn
	// and no load names the root.
	c.reconcilePlugins(ctx, tb.WorldState, loads)
	if _, ok := loads.refs[pluginID]; ok {
		t.Fatal("Space with no peer loaded a pinned root")
	}
	select {
	case peerID := <-sent:
		t.Fatalf("Space sent a load naming a root, peer %q", peerID)
	default:
	}
}
