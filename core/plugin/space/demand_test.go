package plugin_space

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/s4wave/spacewave/testbed"
)

// TestPluginDemandLoadsOnlyDemandedPlugins checks that the Space loads only
// the listed plugins its demand selects, including the account's, and that
// withdrawing one plugin stops exactly that plugin's load without restarting
// the others.
func TestPluginDemandLoadsOnlyDemandedPlugins(t *testing.T) {
	// Bound the test and build the testbed.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// List three plugins in the Space.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{"bridge", "server", "viewer"}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	// Demand two of them and the account's plugin.
	var mtx sync.Mutex
	demanded := []string{"bridge", "inspector", "server"}
	isDemanded := func(pluginID string) bool {
		mtx.Lock()
		defer mtx.Unlock()
		return slices.Contains(demanded, pluginID)
	}
	accountIDs := []string{"inspector"}
	getAccountIDs := func() []string {
		mtx.Lock()
		defer mtx.Unlock()
		return accountIDs
	}

	// Record every LoadPlugin the Space adds.
	loads := newLoadRecorder()
	releaseLoads, err := tb.Bus.AddController(ctx, loads, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoads()

	// Start the Space controller with the demand.
	tb.StaticResolver.AddFactory(NewFactory(
		tb.Bus,
		WithPluginDemand(isDemanded),
		WithAccountPlugins(getAccountIDs),
	))
	space, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
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
	loads.wait(ctx, t, []string{"bridge", "inspector", "server"})

	// The account plugin's manifest is approved like a listed one.
	var approved []string
	space.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		approved = space.pluginIDs
	})
	if !slices.Contains(approved, "inspector") {
		t.Fatalf("approved plugin IDs = %v, want inspector", approved)
	}

	// The account drops its plugin.
	mtx.Lock()
	accountIDs = nil
	mtx.Unlock()
	space.NotifyChanged()
	loads.wait(ctx, t, []string{"bridge", "server"})

	// Withdraw the bridge.
	mtx.Lock()
	demanded = []string{"server"}
	mtx.Unlock()
	space.NotifyChanged()
	loads.wait(ctx, t, []string{"server"})

	// The server kept its first load.
	if starts := loads.getStarts("server"); starts != 1 {
		t.Fatalf("server loads started = %d, want 1", starts)
	}
}

// TestHostedSpaceLoadsWorldManifests checks that a Space hosted in a plugin,
// whose loads go to a host that cannot read its World, names the World's
// admitted manifest in each unpinned load, keeps it while a newer manifest
// waits for approval, replaces it once the newer one is allowed, and sends a
// plain load for a plugin with no manifest there.
func TestHostedSpaceLoadsWorldManifests(t *testing.T) {
	// Bound the test and start the hosted Space with the device's approvals.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	h := startHostedSpace(ctx, t)

	// Each plugin loads: the readiness plugin from the World's manifest.
	first := h.storeManifest(ctx, t, "test/manifests/first", 1)
	h.setApproval(ctx, t, first, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{first}))
	h.loads.waitManifests(ctx, t, "absent", nil)

	// A newer revision waiting for approval leaves the admitted load running.
	starts := h.loads.getStarts(pluginReadinessPluginID)
	second := h.storeManifest(ctx, t, "test/manifests/second", 2)
	<-time.After(200 * time.Millisecond)
	if got := h.loads.getStarts(pluginReadinessPluginID); got != starts {
		t.Fatalf("an unapproved revision started %d loads", got-starts)
	}
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{first}))

	// Allowing the newer revision replaces the load.
	h.setApproval(ctx, t, second, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{second}))
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	if starts := h.loads.getStarts("absent"); starts != 1 {
		t.Fatalf("absent loads started = %d, want 1", starts)
	}
}

// TestHostedSpaceLoadsOnlyAdmittedRoots checks that a hosted Space, whose
// host runs every root it is sent, never sends a World root the device has
// not allowed: the plugin loads without it until the device's person allows
// the root, and loses it again when they decline.
func TestHostedSpaceLoadsOnlyAdmittedRoots(t *testing.T) {
	// Bound the test and start the hosted Space with the device's approvals.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	h := startHostedSpace(ctx, t)
	ref := h.storeManifest(ctx, t, "test/manifests/first", 1)
	keys := artifactKeys([]*bldr_manifest.ManifestRef{ref})

	// With no approval, the load reaching the host names no World root.
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, nil)
	if starts := h.loads.getStarts(pluginReadinessPluginID); starts != 1 {
		t.Fatalf("unapproved: readiness loads started = %d, want 1", starts)
	}

	// Allowing the root sends a load that names it.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, keys)

	// Declining it withdraws that load.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_DECLINED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, nil)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
}

// hostedSpace is a Space hosted on the testbed bus, recording the loads it
// sends there.
type hostedSpace struct {
	// tb is the testbed whose bus hosts the Space.
	tb *testbed.Testbed
	// approvals is the device's approval store answering admission.
	approvals *plugin_approval.Approvals
	// loads records the loads the Space sends to the host.
	loads *loadRecorder
}

// startHostedSpace starts a Space hosted on a testbed bus that holds the
// device's approvals, listing the readiness plugin and one with no manifest.
func startHostedSpace(ctx context.Context, t *testing.T) *hostedSpace {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// List the plugins and create the manifest store.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{"absent", pluginReadinessPluginID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, hostedManifestStore); err != nil {
		t.Fatal(err)
	}

	// Answer admission from the device's approvals and record the loads.
	approvals := plugin_approval.NewApprovals(tb.Bus, tb.EngineVolumeID, "test-plugin-approvals")
	releaseApprovals, err := tb.Bus.AddController(ctx, plugin_approval.NewController(tb.Logger, approvals), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseApprovals)
	loads := newLoadRecorder()
	releaseLoads, err := tb.Bus.AddController(ctx, loads, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseLoads)

	// Start the Space hosted on the testbed bus.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus, WithLoadTarget(tb.Bus)))
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
	t.Cleanup(spaceRef.Release)
	return &hostedSpace{tb: tb, approvals: approvals, loads: loads}
}

// hostedManifestStore is the manifest store the hosted Space lists.
const hostedManifestStore = "test/plugin-manifests"

// storeManifest stores a readiness manifest revision in the World.
func (h *hostedSpace) storeManifest(ctx context.Context, t *testing.T, objKey string, rev uint64) *bldr_manifest.ManifestRef {
	t.Helper()
	ref := createReadinessManifestRev(t, ctx, h.tb, rev)
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx, h.tb.WorldState, h.tb.Volume.GetPeerID(), objKey, []string{hostedManifestStore}, ref,
	); err != nil {
		t.Fatal(err)
	}
	return ref
}

// setApproval records the device's decision on ref's root.
func (h *hostedSpace) setApproval(ctx context.Context, t *testing.T, ref *bldr_manifest.ManifestRef, state s4wave_plugin_approval.PluginApprovalState) {
	t.Helper()
	if err := h.approvals.Set(ctx, h.tb.Volume.GetPeerID().String(), &s4wave_plugin_approval.PluginApproval{
		PluginId:     pluginReadinessPluginID,
		ManifestRoot: manifestRoot(ref),
		State:        state,
	}); err != nil {
		t.Fatal(err)
	}
}

// loadRecorder resolves every LoadPlugin directive and records which plugins
// have a live load.
type loadRecorder struct {
	// bcast guards live, starts and manifests.
	bcast broadcast.Broadcast
	// live counts the live loads by plugin ID.
	live map[string]int
	// starts counts the loads started by plugin ID.
	starts map[string]int
	// manifests is the artifact keys the last started load of each plugin ID
	// names.
	manifests map[string][]string
}

// newLoadRecorder constructs a loadRecorder.
func newLoadRecorder() *loadRecorder {
	return &loadRecorder{
		live:      make(map[string]int),
		starts:    make(map[string]int),
		manifests: make(map[string][]string),
	}
}

// wait waits until exactly want have a live load.
func (r *loadRecorder) wait(ctx context.Context, t *testing.T, want []string) {
	t.Helper()
	for {
		// Snapshot the live plugins.
		var got []string
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			for id, n := range r.live {
				if n != 0 {
					got = append(got, id)
				}
			}
			ch = getWaitCh()
		})
		slices.Sort(got)
		if slices.Equal(got, want) {
			return
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			t.Fatalf("live loads = %v, want %v", got, want)
		case <-ch:
		}
	}
}

// getStarts returns how many loads of pluginID started.
func (r *loadRecorder) getStarts(pluginID string) int {
	var n int
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		n = r.starts[pluginID]
	})
	return n
}

// waitManifests waits until the last load of pluginID names exactly want.
func (r *loadRecorder) waitManifests(ctx context.Context, t *testing.T, pluginID string, want []string) {
	t.Helper()
	for {
		// Snapshot the last load's manifests.
		var got []string
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			got = r.manifests[pluginID]
			ch = getWaitCh()
		})
		if slices.Equal(got, want) {
			return
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			t.Fatalf("%s load manifests = %v, want %v", pluginID, got, want)
		case <-ch:
		}
	}
}

// GetControllerInfo returns information about the controller.
func (r *loadRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/plugin-load-recorder",
		controller.MustParseVersion("0.0.1"),
		"records plugin loads",
	)
}

// Execute executes the controller.
func (r *loadRecorder) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close releases any resources used by the controller.
func (r *loadRecorder) Close() error {
	return nil
}

// HandleDirective marks a plugin live while its LoadPlugin resolver runs.
func (r *loadRecorder) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	pluginID := load.LoadPluginID()
	var keys []string
	for _, ref := range load.LoadPluginManifests() {
		keys = append(keys, bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef()))
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		// Mark the load live until the directive is released.
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.live[pluginID]++
			r.starts[pluginID]++
			r.manifests[pluginID] = keys
			broadcast()
		})
		<-ctx.Done()
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.live[pluginID]--
			broadcast()
		})
		return nil
	}), nil)
}

// _ is a type assertion
var _ controller.Controller = (*loadRecorder)(nil)
